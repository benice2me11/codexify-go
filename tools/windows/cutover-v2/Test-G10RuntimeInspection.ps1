# Offline fixture tests for Inspect-G10Runtime.ps1. Never query a real SCM,
# scheduled task, tunnel, user profile or running process.
$ErrorActionPreference = 'Stop'
$inspect = Join-Path $PSScriptRoot 'Inspect-G10Runtime.ps1'
$root = Join-Path ([IO.Path]::GetFullPath($env:TEMP)) ('codexify-g10-inspection-fixture-' + [guid]::NewGuid().ToString('N'))
$evidence = Join-Path $root 'evidence'
$active = Join-Path $evidence 'g10-soak-active'
$older = Join-Path $evidence 'g10-soak-old'
New-Item -ItemType Directory -Force -Path $active,$older | Out-Null

function Assert-Equal {
    param($Actual,$Expected,[string]$Message)
    if ($Actual -ne $Expected) {
        throw "$Message (actual=$Actual expected=$Expected)"
    }
}
function Add-Record {
    param([string]$Path,[DateTimeOffset]$At,[string]$Msg,[hashtable]$Fields=@{})
    $data=[ordered]@{time=$At.ToString('o');msg=$Msg}
    foreach($name in $Fields.Keys) { $data[$name]=$Fields[$name] }
    ($data | ConvertTo-Json -Compress) | Add-Content -LiteralPath $Path -Encoding utf8
}
try {
    $now=[DateTimeOffset]::UtcNow
    $oldGuard=Join-Path $older 'probe-observations.jsonl'
    $newGuard=Join-Path $active 'probe-observations.jsonl'
    Add-Record $oldGuard $now.AddHours(-4) 'guard' @{action='ABORT';reason='historical';goOwners=0;rustOwners=1;controlled=$false}
    (Get-Item -LiteralPath $oldGuard).LastWriteTimeUtc=[DateTime]::UtcNow.AddHours(-4)
    Add-Record $newGuard $now.AddSeconds(-2) 'guard' @{action='CONTINUE';reason='within_limits';goOwners=2;rustOwners=0;controlled=$true}
    # An incomplete last line may occur while the watcher is appending JSONL.
    Add-Content -LiteralPath $newGuard -Value '{incomplete' -Encoding utf8

    $log=Join-Path $evidence 'go-trial-capture.log'
    Add-Record $log $now.AddHours(-3) 'codexify-go stopped'
    Add-Record $log $now.AddMinutes(-5) 'codexify-go starting'
    Add-Record $log $now.AddMinutes(-4) 'poll failed fixture' @{component='controlplane';error='HTTP 403'}
    Add-Record $log $now.AddMinutes(-3) 'child health check failed'
    Add-Record $log $now.AddMinutes(-2) 'codexify-go stopped'
    Add-Record $log $now.AddMinutes(-1) 'codexify-go starting'

    $out = & $inspect -Offline -RunDirectory $root -RecentMinutes 60 -GuardFreshSeconds 45 | ConvertFrom-Json
    Assert-Equal $out.mode 'offline' 'offline mode'
    Assert-Equal $out.acceptance 'NOT_EVALUATED' 'snapshot may not mark G10 accepted'
    Assert-Equal $out.guard.action 'CONTINUE' 'most recent guard chosen'
    Assert-Equal $out.guard.evidenceSubdirectory 'g10-soak-active' 'correct evidence folder chosen'
    Assert-Equal $out.guard.fresh $true 'guard freshness'
    Assert-Equal $out.logSummary.goStarts 2 'Go start count'
    Assert-Equal $out.logSummary.cleanGoStops 1 'recent clean stops'
    Assert-Equal $out.logSummary.erroredGoStops 0 'errored Go stop count'
    Assert-Equal $out.logSummary.controlPlanePollFailures 1 'poll failure count'
    Assert-Equal $out.logSummary.tunnelHealthFailures 1 'health failure count'
    Assert-Equal $out.logSummary.http403Mentions 1 '403 should be observed, not trigger Rust rollback'
    Assert-Equal $out.service.state 'not_inspected' 'SCM must never be inspected by fixture'
    Assert-Equal $out.localAssessment 'INSPECTION_ONLY' 'offline assessment'

    # Simulate an old active watcher observation: retain the event, mark stale.
    $staleGuard=Join-Path $active 'probe-observations.jsonl'
    Set-Content -LiteralPath $staleGuard -Encoding utf8 -Value (([pscustomobject]@{
        time=$now.AddMinutes(-4).ToString('o');action='CONTINUE';reason='within_limits'
        goOwners=2;rustOwners=0;controlled=$true
    }) | ConvertTo-Json -Compress)
    (Get-Item -LiteralPath $staleGuard).LastWriteTimeUtc=[DateTime]::UtcNow
    $stale = & $inspect -Offline -RunDirectory $root -GuardFreshSeconds 10 | ConvertFrom-Json
    Assert-Equal $stale.guard.fresh $false 'stale guard must not look healthy'
    if (@($stale.inspectionWarnings) -notcontains 'guard_observation_stale') {
        throw 'missing stale watcher observation warning'
    }
    Write-Output 'G10_OFFLINE_RUNTIME_INSPECTION=PASS'
} finally {
    $prefix=[IO.Path]::GetFullPath($env:TEMP).TrimEnd('\')+'\'
    $resolved=[IO.Path]::GetFullPath($root)
    if (!$resolved.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path -Leaf $resolved) -notmatch '^codexify-g10-inspection-fixture-[a-f0-9]{32}$') {
        throw 'Fixture cleanup was refused because path identity is unexpected'
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force -ErrorAction Stop
}
