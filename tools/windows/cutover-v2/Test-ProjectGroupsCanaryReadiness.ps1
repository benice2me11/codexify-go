# Offline, deterministic tests for the read-only Project Groups canary readiness gate.
# Never queries service/process/task APIs; all records are temporary fixtures.
$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) ('pg-g10-readiness-fixture-' + [guid]::NewGuid().ToString('N'))
$evidence = Join-Path $root 'evidence'
$path = Join-Path $evidence 'probe-observations.jsonl'
$script = Join-Path $PSScriptRoot 'Assess-ProjectGroupsCanaryReadiness.ps1'
$reference = [DateTimeOffset]::Parse('2026-10-16T12:00:00+00:00')

function Assert-Condition {
    param([bool]$Condition, [string]$Message)
    if (!$Condition) { throw $Message }
}

function Write-Probes {
    param([object[]]$Records)
    $rows = foreach ($item in $Records) {
        @{
            time = $item.time
            action = $item.action
            reason = $item.reason
            goOwners = 2
            rustOwners = 0
            controlled = $true
        } | ConvertTo-Json -Compress
    }
    [IO.File]::WriteAllLines($path, [string[]]@($rows), [Text.UTF8Encoding]::new($false))
}

function Inspect-Offline {
    $raw = & $script -RunDirectory $root -Offline -RequiredHours 1 -MaxGapSeconds 60 -GuardFreshSeconds 45 -AsOfUtc $reference.ToString('o')
    return ($raw | ConvertFrom-Json -ErrorAction Stop)
}

try {
    New-Item -ItemType Directory -Path $evidence -Force | Out-Null
    $cleanRows = @(
        for ($seconds = 3600; $seconds -ge 0; $seconds -= 10) {
            [pscustomobject]@{
                time = $reference.AddSeconds(-$seconds).ToString('o')
                action = 'CONTINUE'
                reason = 'within_limits'
                seconds = $seconds
            }
        }
    )
    Write-Probes $cleanRows
    $clean = Inspect-Offline
    Assert-Condition ($clean.soak.completeWindow -eq $true) 'clean 1h fixture was incorrectly rejected'
    Assert-Condition ($clean.decision -eq 'OFFLINE_SOAK_ONLY') 'offline fixture incorrectly authorized live cutover'
    Assert-Condition ($clean.productionAcceptance -eq 'NOT_EVALUATED') 'offline fixture claimed production acceptance'
    Assert-Condition ($clean.soak.gapCount -eq 0) 'clean fixture has artificial gaps'
    $loweredLiveThresholdBlocked = $false
    try {
        & $script -RunDirectory $root -RequiredHours 1 | Out-Null
    } catch {
        $loweredLiveThresholdBlocked = $_.Exception.Message -like '*at least 24h*'
    }
    Assert-Condition $loweredLiveThresholdBlocked 'live readiness allowed reduced soak hours'
    $widenedLiveGapBlocked = $false
    try {
        & $script -RunDirectory $root -MaxGapSeconds 600 | Out-Null
    } catch {
        $widenedLiveGapBlocked = $_.Exception.Message -like '*<=60s gaps*'
    }
    Assert-Condition $widenedLiveGapBlocked 'live readiness allowed 10-minute gaps'

    # A fresh CONTINUE alone must not make a shorter-than-required window pass.
    Write-Probes @($cleanRows | Where-Object { $_.seconds -le 3000 })
    $short = Inspect-Offline
    Assert-Condition ($short.soak.completeWindow -eq $false) 'short soak window unexpectedly accepted'
    Assert-Condition ($short.holdReasons -contains 'soak_window_short') 'short window reason missing'

    # An older CONTINUE record plus a fresh one cannot hide a missing interval.
    Write-Probes @($cleanRows | Where-Object { $_.seconds -le 1800 -or $_.seconds -ge 2400 })
    $gapped = Inspect-Offline
    Assert-Condition ($gapped.soak.completeWindow -eq $false) 'long sample gap was ignored'
    Assert-Condition ($gapped.holdReasons -contains 'g10_observation_gap') 'gap hold reason missing'
    Assert-Condition ($gapped.soak.maxObservedGapSeconds -ge 600) 'max gap metric was not computed'

    # Tunnel recovery HOLD must remain explicit rather than being smoothed into CONTINUE.
    $holdRows = foreach ($row in $cleanRows) {
        if ($row.seconds -eq 1200) {
            [pscustomobject]@{
                time = $row.time; action = 'HOLD'; reason = 'tunnel_child_recovery'; seconds = 1200
            }
        } else { $row }
    }
    Write-Probes @($holdRows)
    $held = Inspect-Offline
    Assert-Condition ($held.soak.completeWindow -eq $false) 'non-CONTINUE sample was ignored'
    Assert-Condition ($held.holdReasons -contains 'non_continue_guard_action') 'hold guard reason missing'
    Assert-Condition ($held.soak.holdReasonCodes -contains 'tunnel_child_recovery') 'safe reason code missing'

    Remove-Item -LiteralPath $path -Force
    $missing = Inspect-Offline
    Assert-Condition ($missing.decision -eq 'HOLD') 'missing log was accepted'
    Assert-Condition ($missing.holdReasons -contains 'g10_log_missing') 'missing log reason omitted'

    Write-Output 'PROJECT_GROUPS_READINESS_OFFLINE_FIXTURES=PASS'
} finally {
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
}
