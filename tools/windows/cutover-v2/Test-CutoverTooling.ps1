$ErrorActionPreference = 'Stop'

$toolDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$modulePath = Join-Path $toolDir 'CutoverGuard.psm1'
$rollbackPath = Join-Path $toolDir 'Rollback-To-Rust.ps1'
$snapshotPath = Join-Path $toolDir 'Prepare-RecoverySnapshot.ps1'
$watchPath = Join-Path $toolDir 'Watch-Cutover.ps1'
if (!(Test-Path -LiteralPath $modulePath)) { throw "missing CutoverGuard.psm1" }
if (!(Test-Path -LiteralPath $rollbackPath)) { throw "missing Rollback-To-Rust.ps1" }
if (!(Test-Path -LiteralPath $snapshotPath)) { throw "missing Prepare-RecoverySnapshot.ps1" }
if (!(Test-Path -LiteralPath $watchPath)) { throw "missing Watch-Cutover.ps1" }

Import-Module $modulePath -Force

function Assert-Equal($actual, $expected, [string]$message) {
    if ($actual -ne $expected) {
        throw ($message + ": got '" + $actual + "', want '" + $expected + "'")
    }
}

$now = [DateTimeOffset]::Parse('2026-10-01T16:30:00Z')
$events9 = 1..9 | ForEach-Object {
    [pscustomobject]@{ time=$now.AddSeconds(-$_ + 1); phase='mcp_start'; rpc_method='tools/call'; operation_hash=("op{0}" -f $_) }
}
$d = Get-CutoverGuardDecision -Now $now -DiagnosticEvents $events9 -TunnelEvents @() -Settled -QuietWindow -OwnerState ([pscustomobject]@{GoOwners=1;RustOwners=0;Controlled=$true}) -RestartTimes @()
Assert-Equal $d.Action 'CONTINUE' 'nine distinct calls must stay below abort threshold'

$events10 = 1..10 | ForEach-Object {
    [pscustomobject]@{ time=$now.AddSeconds(-($_ % 9)); phase='mcp_start'; rpc_method='tools/call'; operation_hash=("op{0}" -f $_) }
}
$d = Get-CutoverGuardDecision -Now $now -DiagnosticEvents $events10 -TunnelEvents @() -Settled -QuietWindow -OwnerState ([pscustomobject]@{GoOwners=1;RustOwners=0;Controlled=$true}) -RestartTimes @()
Assert-Equal $d.Action 'ABORT' 'ten calls in ten seconds must abort'
Assert-Equal $d.Reason 'mcp_calls_10s' 'ten-call reason'

$repeat = 1..5 | ForEach-Object {
    [pscustomobject]@{ time=$now.AddSeconds(-$_); phase='mcp_start'; rpc_method='tools/call'; operation_hash='same-op' }
}
$d = Get-CutoverGuardDecision -Now $now -DiagnosticEvents $repeat -TunnelEvents @() -Settled -QuietWindow -OwnerState ([pscustomobject]@{GoOwners=1;RustOwners=0;Controlled=$true}) -RestartTimes @()
Assert-Equal $d.Action 'ABORT' 'five repeated operation hashes must abort'
Assert-Equal $d.Reason 'repeated_operation_10s' 'repeat reason'

$forwarded = 1..101 | ForEach-Object {
    [pscustomobject]@{ time=$now.AddSeconds(-($_ % 59)); message='dispatcher forwarded command to MCP server'; level='INFO' }
}
$d = Get-CutoverGuardDecision -Now $now -DiagnosticEvents @() -TunnelEvents $forwarded -Settled -QuietWindow -OwnerState ([pscustomobject]@{GoOwners=1;RustOwners=0;Controlled=$true}) -RestartTimes @()
Assert-Equal $d.Action 'ABORT' '101 quiet forwarded events in 60 seconds must abort'
Assert-Equal $d.Reason 'forwarded_60s' 'forwarded reason'

$limited = @([pscustomobject]@{ time=$now; message='RATE_LIMITED retry after'; level='WARN' })
$d = Get-CutoverGuardDecision -Now $now -DiagnosticEvents @() -TunnelEvents $limited -Settled -QuietWindow -OwnerState ([pscustomobject]@{GoOwners=1;RustOwners=0;Controlled=$true}) -RestartTimes @()
Assert-Equal $d.Action 'HOLD' 'rate limit alone must hold rather than auto-attribute'
Assert-Equal $d.Reason 'rate_limited' 'rate-limit reason'

$d = Get-CutoverGuardDecision -Now $now -DiagnosticEvents @() -TunnelEvents @() -Settled -QuietWindow -OwnerState ([pscustomobject]@{GoOwners=1;RustOwners=1;Controlled=$true}) -RestartTimes @()
Assert-Equal $d.Action 'ABORT' 'simultaneous Go/Rust owner must abort'
Assert-Equal $d.Reason 'duplicate_live_owner' 'owner reason'

$restarts = @($now.AddMinutes(-4), $now.AddMinutes(-1))
$d = Get-CutoverGuardDecision -Now $now -DiagnosticEvents @() -TunnelEvents @() -Settled -QuietWindow -OwnerState ([pscustomobject]@{GoOwners=1;RustOwners=0;Controlled=$true}) -RestartTimes $restarts
Assert-Equal $d.Action 'ABORT' 'two unexpected restarts in five minutes must abort'
Assert-Equal $d.Reason 'owned_process_restarts' 'restart reason'

# Reproduce the 2026-10-05 false rollback: one tunnel exit plus respawn
# was observed as two PID-signature transitions two seconds apart.
$tripAt=[DateTimeOffset]::Parse('2026-10-05T02:37:28Z')
$episodes=New-Object 'System.Collections.Generic.List[object]'
foreach($transition in @($tripAt,$tripAt.AddSeconds(2))) {
    if (Test-CutoverRestartEpisode -At $transition -RestartTimes $episodes.ToArray()) {
        [void]$episodes.Add($transition)
    }
}
Assert-Equal $episodes.Count 1 'one exit and respawn must be one restart episode'
$d=Get-CutoverGuardDecision -Now $tripAt.AddSeconds(4) -DiagnosticEvents @() -TunnelEvents @() -Settled -OwnerState ([pscustomobject]@{GoOwners=2;RustOwners=0;Controlled=$true}) -RestartTimes $episodes.ToArray()
Assert-Equal $d.Action 'CONTINUE' 'a recovered tunnel must not cause an automatic Go to Rust rollback'
$secondEpisode=$tripAt.AddSeconds(90)
if (Test-CutoverRestartEpisode -At $secondEpisode -RestartTimes $episodes.ToArray()) {
    [void]$episodes.Add($secondEpisode)
}
Assert-Equal $episodes.Count 2 'a second independent restart must count'
$d=Get-CutoverGuardDecision -Now $secondEpisode.AddSeconds(1) -DiagnosticEvents @() -TunnelEvents @() -Settled -OwnerState ([pscustomobject]@{GoOwners=2;RustOwners=0;Controlled=$true}) -RestartTimes $episodes.ToArray()
Assert-Equal $d.Action 'ABORT' 'two actual restart episodes still abort'
Assert-Equal $d.Reason 'owned_process_restarts' 'two actual restart episodes still trigger safety abort'
# A VPN outage causes PollWatcher to replace tunnel-client-runtime, not the
# Go supervisor/worker. The guard must ignore both tunnel PID signatures.
$ownerExe='C:\Fixture\codexify-go.exe'
$fixedProcesses=@(
    [pscustomobject]@{ProcessId=120;ExecutablePath=$ownerExe;Name='codexify-go.exe'},
    [pscustomobject]@{ProcessId=121;ExecutablePath=$ownerExe;Name='codexify-go.exe'},
    [pscustomobject]@{ProcessId=122;ExecutablePath='C:\Fixture\runtime\openai-tunnel\tunnel-client-runtime.exe';Name='tunnel-client-runtime.exe'}
)
$before=Get-CutoverGoSignature -GoExecutable $ownerExe -Processes $fixedProcesses
$fixedProcesses[2].ProcessId=999
$after=Get-CutoverGoSignature -GoExecutable $ownerExe -Processes $fixedProcesses
Assert-Equal $after $before 'a tunnel-only PID change must not affect Go restart signature'
$fixedProcesses[1].ProcessId=123
$changed=Get-CutoverGoSignature -GoExecutable $ownerExe -Processes $fixedProcesses
if ($changed -eq $before) { throw 'actual Go worker identity change must be visible' }
$d=Get-CutoverGuardDecision -Now $now -DiagnosticEvents @() -TunnelEvents @() -Settled -OwnerState ([pscustomobject]@{GoOwners=3;RustOwners=0;Controlled=$true}) -RestartTimes @()
Assert-Equal $d.Reason 'duplicate_go_processes' 'multiple independent Go owners must still abort'
$vpn403=@([pscustomobject]@{time=$now;msg='poll failed; backing off';error='controlplane client: unexpected status 403: unsupported_country_region_territory'})
$d=Get-CutoverGuardDecision -Now $now -DiagnosticEvents @() -TunnelEvents $vpn403 -Settled -QuietWindow -OwnerState ([pscustomobject]@{GoOwners=2;RustOwners=0;Controlled=$true}) -RestartTimes @()
Assert-Equal $d.Action 'HOLD' 'VPN outage must not abort Go'
Assert-Equal $d.Reason 'egress_degraded' 'VPN degraded reason'
$d=Get-CutoverGuardDecision -Now $now.AddMinutes(2) -DiagnosticEvents @() -TunnelEvents $vpn403 -Settled -QuietWindow -OwnerState ([pscustomobject]@{GoOwners=2;RustOwners=0;Controlled=$true}) -RestartTimes @()
Assert-Equal $d.Action 'CONTINUE' 'stale 403 must not permanently hold recovered Go'

$tmp = Join-Path $env:TEMP ('codexify-cutover-fixture-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
try {
    $rust = Join-Path $tmp 'rust'
    $tasks = Join-Path $tmp 'tasks'
    New-Item -ItemType Directory -Force -Path (Join-Path $rust 'bin'),$tasks | Out-Null
    [IO.File]::WriteAllBytes((Join-Path $rust 'bin\codexify.exe'),[byte[]](1,2,3))
    Set-Content -LiteralPath (Join-Path $rust 'codexify.config.json') -Value '{}' -Encoding ASCII
    Set-Content -LiteralPath (Join-Path $rust 'watchdog.ps1') -Value '# fixture' -Encoding ASCII
    Set-Content -LiteralPath (Join-Path $tasks 'Codexify.xml') -Value '<Task />' -Encoding ASCII
    Set-Content -LiteralPath (Join-Path $tasks 'Codexify Watchdog.xml') -Value '<Task />' -Encoding ASCII

    $files = @(
        'rust\bin\codexify.exe',
        'rust\codexify.config.json',
        'rust\watchdog.ps1',
        'tasks\Codexify.xml',
        'tasks\Codexify Watchdog.xml'
    )
    $manifestFiles = foreach($rel in $files) {
        $p = Join-Path $tmp $rel
        [pscustomobject]@{ path=$rel; sha256=(Get-FileHash -LiteralPath $p -Algorithm SHA256).Hash }
    }
    [pscustomobject]@{version=1;files=$manifestFiles} | ConvertTo-Json -Depth 5 |
        Set-Content -LiteralPath (Join-Path $tmp 'manifest.json') -Encoding UTF8

    $planText = & $rollbackPath -RecoveryDirectory $tmp -ExpectedGoExecutable 'C:\Fixture\codexify-go.exe' -DryRun
    $plan = $planText | ConvertFrom-Json
    Assert-Equal $plan.mode 'dry-run' 'rollback dry-run mode'
    Assert-Equal $plan.actions[0].name 'validate_snapshot' 'rollback first action'
    Assert-Equal $plan.actions[1].name 'validate_go_service_identity' 'rollback identity before mutation'
    Assert-Equal $plan.actions[2].name 'stop_disable_go' 'rollback stop Go before Rust'
    Assert-Equal $plan.actions[3].name 'verify_go_tree_gone' 'rollback verifies Go gone'
    Assert-Equal $plan.actions[4].name 'restore_rust_files' 'restore after exclusive stop'
    Assert-Equal $plan.actions[5].name 'restore_rust_tasks' 'restore startup state'
    Assert-Equal $plan.actions[6].name 'start_rust' 'Rust starts only after restore'
    Assert-Equal $plan.actions[7].name 'verify_rust_ready' 'readiness is final action'

    $snapshotValidation = & $snapshotPath -Destination $tmp -ValidateOnly
    $sv = $snapshotValidation | ConvertFrom-Json
    Assert-Equal $sv.status 'valid' 'snapshot validate-only'

    $diag = Join-Path $tmp 'diag.jsonl'
    $tunnel = Join-Path $tmp 'tunnel.jsonl'
    $owners = Join-Path $tmp 'owners.json'
    $fixedNow = [DateTimeOffset]::Parse('2026-10-01T16:30:00Z')
    1..10 | ForEach-Object {
        [pscustomobject]@{
            time=$fixedNow.AddSeconds(-($_ % 9)).ToString('o')
            phase='mcp_start'
            rpc_method='tools/call'
            operation_hash=("fixture-{0}" -f $_)
        } | ConvertTo-Json -Compress
    } | Set-Content -LiteralPath $diag -Encoding ASCII
    Set-Content -LiteralPath $tunnel -Value '' -Encoding ASCII
    [pscustomobject]@{GoOwners=1;RustOwners=0;Controlled=$true} | ConvertTo-Json |
        Set-Content -LiteralPath $owners -Encoding ASCII
    $watchResult = & $watchPath -EvaluateOnce -DiagnosticPath $diag -TunnelLogPath $tunnel -OwnerStatePath $owners -QuietWindow -Settled -Now $fixedNow.ToString('o') | ConvertFrom-Json
    Assert-Equal $watchResult.Action 'ABORT' 'watcher fixture must surface guard abort'
    Assert-Equal $watchResult.Reason 'mcp_calls_10s' 'watcher fixture reason'
} finally {
    $fixtureRoot = [IO.Path]::GetFullPath($tmp)
    $tempPrefix = [IO.Path]::GetFullPath($env:TEMP).TrimEnd('\') + '\'
    if (!$fixtureRoot.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path -Leaf $fixtureRoot) -notmatch '^codexify-cutover-fixture-[a-f0-9]{32}$') {
        throw 'Refusing to remove a path outside the allocated cutover fixture.'
    }
    Remove-Item -LiteralPath $tmp -Recurse -Force
}

Write-Output 'CUTOVER_TOOLING_FIXTURE=PASS'
