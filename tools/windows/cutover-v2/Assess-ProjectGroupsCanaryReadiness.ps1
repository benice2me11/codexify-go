# Read-only PG-I7 preflight: measure an uninterrupted G10 observation window
# before any production Project Groups canary. Never restarts or modifies SCM,
# services, tasks, tunnel, bindings, or user repositories.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$RunDirectory,
    [ValidateRange(1, 72)][int]$RequiredHours = 24,
    [ValidateRange(15, 3600)][int]$MaxGapSeconds = 60,
    [ValidateRange(5, 3600)][int]$GuardFreshSeconds = 45,
    [switch]$Offline,
    [string]$AsOfUtc = ''
)

$ErrorActionPreference = 'Stop'
if ($AsOfUtc -and !$Offline) {
    throw '-AsOfUtc is restricted to -Offline fixture analysis; live readiness must use the actual clock.'
}
$now = [DateTimeOffset]::UtcNow
if ($AsOfUtc) {
    $now = [DateTimeOffset]::Parse($AsOfUtc)
}
$cutoff = $now.AddHours(-$RequiredHours)
$runPath = [IO.Path]::GetFullPath($RunDirectory)
$evidence = Join-Path $runPath 'evidence'

$sampleFile = $null
if (Test-Path -LiteralPath $evidence -PathType Container) {
    $sampleFile = @(Get-ChildItem -LiteralPath $evidence -Filter 'probe-observations.jsonl' -Recurse -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1)
    if ($sampleFile.Count -gt 0) { $sampleFile = $sampleFile[0] } else { $sampleFile = $null }
}

$records = [System.Collections.Generic.List[object]]::new()
if ($sampleFile) {
    foreach ($line in @(Get-Content -LiteralPath $sampleFile.FullName -Tail 20000 -ErrorAction Stop)) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        try {
            $item = $line | ConvertFrom-Json -ErrorAction Stop
            $stamp = [DateTimeOffset]::Parse([string]$item.time)
            if ($stamp -lt $cutoff.AddSeconds(-$MaxGapSeconds) -or $stamp -gt $now.AddMinutes(1)) {
                continue
            }
            $records.Add([pscustomobject]@{
                Stamp = $stamp
                Action = [string]$item.action
                Reason = [string]$item.reason
            })
        } catch {
            # An incomplete trailing JSONL record is not trusted evidence.
        }
    }
}
$sorted = @($records.ToArray() | Sort-Object Stamp)
$first = $null
$last = $null
$maxObservedGapSeconds = $null
$longGapCount = 0
$nonContinueCount = 0
$holdReasons = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
$lastAgeSeconds = $null
$firstBoundaryLagSeconds = $null
$spanHours = 0.0

if ($sorted.Count -gt 0) {
    $first = $sorted[0].Stamp
    $last = $sorted[-1].Stamp
    $firstBoundaryLagSeconds = [math]::Round(($first - $cutoff).TotalSeconds, 1)
    $lastAgeSeconds = [math]::Round(($now - $last).TotalSeconds, 1)
    $spanHours = [math]::Round(($last - $first).TotalHours, 3)
    $maxObservedGapSeconds = 0.0
    for ($i = 0; $i -lt $sorted.Count; $i++) {
        $rec = $sorted[$i]
        if ($rec.Action -ne 'CONTINUE') {
            $nonContinueCount++
            $reason = $rec.Reason
            if ($reason -match '^[A-Za-z0-9_-]{1,64}$') {
                [void]$holdReasons.Add($reason)
            } else {
                [void]$holdReasons.Add('other_or_unclassified')
            }
        }
        if ($i -eq 0) { continue }
        $gap = ($rec.Stamp - $sorted[$i-1].Stamp).TotalSeconds
        if ($gap -gt $maxObservedGapSeconds) { $maxObservedGapSeconds = $gap }
        if ($gap -gt $MaxGapSeconds) { $longGapCount++ }
    }
}

$reasons = [System.Collections.Generic.List[string]]::new()
if (!$sampleFile) { $reasons.Add('g10_log_missing') }
if ($sorted.Count -eq 0) { $reasons.Add('g10_observations_missing_in_window') }
if ($sorted.Count -gt 0) {
    if ($firstBoundaryLagSeconds -gt $MaxGapSeconds) { $reasons.Add('soak_window_short') }
    if ($lastAgeSeconds -gt $GuardFreshSeconds) { $reasons.Add('guard_observation_stale') }
    if ($longGapCount -gt 0) { $reasons.Add('g10_observation_gap') }
    if ($nonContinueCount -gt 0) { $reasons.Add('non_continue_guard_action') }
}
$soakPass = $reasons.Count -eq 0

$localAssessment = 'OFFLINE_NOT_ASSESSED'
$inspectionErrors = @()
$rustCount = $null
$goCount = $null
$tunnelCount = $null
$latestAction = 'unknown'
if (!$Offline) {
    $inspector = Join-Path $PSScriptRoot 'Inspect-G10Runtime.ps1'
    $raw = & $inspector -RunDirectory $runPath -RecentMinutes 90 -GuardFreshSeconds $GuardFreshSeconds
    $inspection = $raw | ConvertFrom-Json -ErrorAction Stop
    $localAssessment = [string]$inspection.localAssessment
    $inspectionErrors = @($inspection.inspectionWarnings)
    $latestAction = [string]$inspection.guard.action
    $rustCount = $inspection.processes.rustCount
    $goCount = $inspection.processes.goCount
    $tunnelCount = $inspection.processes.tunnelDirectChildren
    if ($localAssessment -ne 'LOCAL_OBSERVED') { $reasons.Add('live_go_only_topology_not_observed') }
    if ($inspectionErrors.Count -gt 0) { $reasons.Add('inspector_warning') }
    if ($inspection.service.state -ine 'Running') { $reasons.Add('service_not_running') }
    if ($inspection.guard.fresh -ne $true -or $latestAction -ne 'CONTINUE') { $reasons.Add('latest_guard_not_continue') }
    if ($inspection.scheduledTasks.Codexify -ne 'Disabled' -or
        $inspection.scheduledTasks.'Codexify Watchdog' -ne 'Disabled') {
        $reasons.Add('rust_task_not_disabled')
    }
}

# Never print GO or PASS for the full rollout: hosted UI and operator rollback
# require separate gates even when this local evidence is sufficient.
$preflight = 'HOLD'
if ($reasons.Count -eq 0 -and !$Offline) {
    $preflight = 'LOCAL_READY_FOR_OPERATOR_REVIEW'
} elseif ($reasons.Count -eq 0 -and $Offline) {
    $preflight = 'OFFLINE_SOAK_ONLY'
}

[pscustomobject]@{
    checkedAtUtc = $now.ToString('o')
    mode = $(if ($Offline) { 'offline' } else { 'live_read_only' })
    decision = $preflight
    productionAcceptance = 'NOT_EVALUATED'
    localAssessment = $localAssessment
    soak = [pscustomobject]@{
        requiredHours = $RequiredHours
        maxPermittedGapSeconds = $MaxGapSeconds
        guardFreshSeconds = $GuardFreshSeconds
        sampleCount = $sorted.Count
        firstSampleUtc = $(if ($first) { $first.ToString('o') } else { $null })
        lastSampleUtc = $(if ($last) { $last.ToString('o') } else { $null })
        observedSpanHours = $spanHours
        firstBoundaryLagSeconds = $firstBoundaryLagSeconds
        lastSampleAgeSeconds = $lastAgeSeconds
        maxObservedGapSeconds = $(if ($null -ne $maxObservedGapSeconds) { [math]::Round($maxObservedGapSeconds,1) } else { $null })
        gapCount = $longGapCount
        nonContinueCount = $nonContinueCount
        holdReasonCodes = @($holdReasons | Sort-Object)
        completeWindow = [bool]$soakPass
    }
    topology = [pscustomobject]@{
        goCount = $goCount
        tunnelDirectChildren = $tunnelCount
        rustCount = $rustCount
        latestGuard = $latestAction
    }
    holdReasons = @($reasons.ToArray())
    inspectionWarnings = $inspectionErrors
    note = 'Read-only PG-I7 local preflight, not Go-only cutover authorization; hosted ChatGPT and operator rollback acceptance are separate gates.'
} | ConvertTo-Json -Depth 8
