Set-StrictMode -Version Latest

# A pure, offline-testable policy: only sustained absence of the Go supervisor
# is eligible for previous-Go recovery. Network failures, worker and tunnel
# self-healing are observations, never reasons to change the service image.
function Get-G11Decision {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]$Snapshot,
        $PreviousState = $null,
        [DateTimeOffset]$Now = [DateTimeOffset]::UtcNow,
        [ValidateRange(2,30)][int]$FailureThreshold = 6,
        [ValidateRange(60,3600)][int]$FailureGraceSeconds = 300,
        [switch]$RecoveryEnabled
    )

    $count = 0
    $since = $Now
    if ($null -ne $PreviousState) {
        try {
            $previousAt = [DateTimeOffset]::Parse([string]$PreviousState.lastCheckUtc)
            $oldSince = [DateTimeOffset]::Parse([string]$PreviousState.outageSinceUtc)
            if ($previousAt -le $Now -and $previousAt -ge $Now.AddMinutes(-3) -and
                [string]$PreviousState.lastReason -eq 'go_supervisor_unavailable') {
                $count = [int]$PreviousState.consecutiveFailures
                $since = $oldSince
            }
        } catch {
            # Old, invalid or future state cannot accelerate recovery.
            $count = 0
            $since = $Now
        }
    }

    $action = 'CONTINUE'
    $reason = 'within_limits'
    $mayRecover = $false
    $fault = $false
    if ([string]$Snapshot.ProbeError) {
        $action = 'OBSERVATION_ERROR'
        $reason = 'local_probe_failed'
    } elseif ([int]$Snapshot.RustCount -gt 0 -or -not [bool]$Snapshot.RustTasksDisabled) {
        $action = 'BLOCKED'
        $reason = 'rust_owner_or_task_detected'
    } elseif ([string]$Snapshot.ImageKind -notin @('current','previous')) {
        $action = 'BLOCKED'
        $reason = 'unexpected_scm_image_or_hash'
    } elseif ([int]$Snapshot.UnexpectedWatchdogCount -gt 0 -or
              [int]$Snapshot.G10WatcherCount -gt 1 -or [bool]$Snapshot.G10TaskUnpinned) {
        $action = 'BLOCKED'
        $reason = 'conflicting_or_unpinned_watchdog'
    } elseif ([int]$Snapshot.GoCount -gt 2 -or [int]$Snapshot.UnexpectedGoCount -gt 0 -or
              [int]$Snapshot.UnexpectedTunnelCount -gt 0 -or
              [int]$Snapshot.SupervisorCount -gt 1 -or [int]$Snapshot.WorkerCount -gt 1) {
        $action = 'BLOCKED'
        $reason = 'duplicate_or_foreign_owner'
    } elseif ($RecoveryEnabled -and ([bool]$Snapshot.G10TaskEnabled -or
              [int]$Snapshot.G10WatcherCount -gt 0)) {
        $action = 'BLOCKED'
        $reason = 'competing_g10_recovery_authority'
    } elseif ([string]$Snapshot.ImageKind -eq 'previous') {
        if ([string]$Snapshot.ServiceState -eq 'Running' -and
            [int]$Snapshot.SupervisorCount -eq 1 -and [int]$Snapshot.GoCount -eq 2 -and
            [int]$Snapshot.WorkerCount -eq 1 -and [int]$Snapshot.TunnelCount -eq 1 -and
            [string]$Snapshot.Health -eq 'healthy') {
            $action = 'GO_PREVIOUS_RECOVERED'
            $reason = 'previous_go_healthy_operator_review'
        } else {
            $action = 'HOLD'
            $reason = 'previous_go_degraded_manual_recovery'
        }
    } elseif ([bool]$Snapshot.G10TaskEnabled -and [int]$Snapshot.G10WatcherCount -eq 0) {
        $action = 'HOLD'
        $reason = 'g10_watcher_missing_or_stale'
    } elseif ([string]$Snapshot.ServiceState -ne 'Running' -or
              [int]$Snapshot.SupervisorCount -ne 1) {
        if ([int]$Snapshot.GoCount -ne 0 -or [int]$Snapshot.TunnelCount -ne 0) {
            $action = 'BLOCKED'
            $reason = 'orphaned_owner_when_supervisor_unavailable'
        } else {
            $fault = $true
            $count++
            $action = 'HOLD'
            $reason = 'go_supervisor_unavailable'
            if ($count -ge $FailureThreshold -and ($Now-$since).TotalSeconds -ge $FailureGraceSeconds) {
                if (-not [bool]$Snapshot.ScmRecoveryValid) {
                    $reason = 'scm_recovery_not_verified'
                } elseif (-not [bool]$Snapshot.RecoveryAssetsValid) {
                    $reason = 'previous_go_assets_unverified'
                } elseif (-not $RecoveryEnabled) {
                    $reason = 'confirmed_failure_operator_recovery_required'
                } elseif ($PreviousState -and $PreviousState.PSObject.Properties['recoveryAttemptedUtc'] -and
                          $PreviousState.recoveryAttemptedUtc) {
                    $reason = 'recovery_already_attempted_operator_required'
                } elseif ([bool]$Snapshot.G10TaskEnabled -or [int]$Snapshot.G10WatcherCount -gt 0) {
                    $action = 'BLOCKED'
                    $reason = 'competing_g10_recovery_authority'
                } else {
                    $action = 'RECOVER_GO'
                    $reason = 'confirmed_local_supervisor_outage'
                    $mayRecover = $true
                }
            }
        }
    } elseif ([int]$Snapshot.GoCount -ne 2 -or [int]$Snapshot.WorkerCount -ne 1) {
        $action = 'HOLD'
        $reason = 'worker_self_healing'
    } elseif ([int]$Snapshot.TunnelCount -ne 1) {
        $action = 'HOLD'
        $reason = 'tunnel_self_healing'
    } elseif ([string]$Snapshot.Health -ne 'healthy') {
        $action = 'HOLD'
        $reason = 'local_health_degraded'
    }

    $next = [pscustomobject]@{
        schemaVersion = 1; lastCheckUtc = $Now.ToString('o')
        lastAction = $action
        lastReason = $(if ($fault) {'go_supervisor_unavailable'} else {$reason})
        consecutiveFailures = $(if ($fault) {$count} else {0})
        outageSinceUtc = $(if ($fault) {$since.ToString('o')} else {$null})
        lastObservedImage = [string]$Snapshot.ImageKind
    }
    return [pscustomobject]@{
        action = $action; reason = $reason; recoveryAllowed = $mayRecover
        consecutiveFailures = $next.consecutiveFailures
        outageSeconds = $(if ($fault) {[int][math]::Max(0,($Now-$since).TotalSeconds)} else {0})
        nextState = $next
    }
}

# No second invocation may update the incident counter or arm recovery while
# the first one is still running. Task Scheduler IgnoreNew is a second guard.
function Enter-G11Mutex {
    [CmdletBinding()]
    param([string]$Name = 'Global\CodexifyGo.G11SteadyState')
    $mutex = New-Object System.Threading.Mutex($false,$Name)
    try {
        $acquired = $false
        try { $acquired = $mutex.WaitOne(0) }
        catch [System.Threading.AbandonedMutexException] { $acquired = $true }
        if (!$acquired) {
            $mutex.Dispose()
            return $null
        }
        return $mutex
    } catch {
        $mutex.Dispose()
        throw
    }
}

function Exit-G11Mutex {
    param($Mutex)
    if ($null -ne $Mutex) {
        try { $Mutex.ReleaseMutex() } finally { $Mutex.Dispose() }
    }
}

function Test-G11RecoveryHandover {
    [CmdletBinding()]
    param(
        [bool]$G10TaskDisabled,
        [int]$G10WatcherCount,
        [bool]$RustTasksDisabled,
        [int]$OtherG11TaskCount,
        [bool]$AssetsVerified,
        [bool]$ProgramDataProtected
    )
    $reasons = @()
    if (!$G10TaskDisabled) { $reasons += 'g10_task_not_disabled' }
    if ($G10WatcherCount -ne 0) { $reasons += 'g10_watcher_still_running' }
    if (!$RustTasksDisabled) { $reasons += 'rust_tasks_not_disabled' }
    if ($OtherG11TaskCount -ne 0) { $reasons += 'other_g11_task_detected' }
    if (!$AssetsVerified) { $reasons += 'recovery_assets_unverified' }
    if (!$ProgramDataProtected) { $reasons += 'unprotected_executable_directory' }
    return [pscustomobject]@{
        ready = ($reasons.Count -eq 0)
        reasons = $reasons
        rustFallback = $false
    }
}

Export-ModuleMember -Function Get-G11Decision,Enter-G11Mutex,Exit-G11Mutex,Test-G11RecoveryHandover
