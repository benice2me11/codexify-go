$ErrorActionPreference = 'Stop'
$toolDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$watch = Join-Path $toolDir 'Watch-Cutover.ps1'
$fixture = [IO.Path]::GetFullPath((Join-Path $env:TEMP ('codexify-watch-fixture-' + [guid]::NewGuid().ToString('N'))))
New-Item -ItemType Directory -Path $fixture | Out-Null
$process = $null

function Start-FixtureWatcher([string]$Name, [switch]$Arm) {
    $args = @('-NoProfile','-File',('"'+$watch+'"'),
        '-CandidateExecutable','C:\CutoverFixture\unused-go.exe',
        '-RustExecutable','C:\CutoverFixture\unused-rust.exe',
        '-GoServiceName','CodexifyNonexistentFixture',
        '-DiagnosticPath',('"'+(Join-Path $fixture 'diagnostic.jsonl')+'"'),
        '-TunnelLogPath',('"'+(Join-Path $fixture 'tunnel.jsonl')+'"'),
        '-ObservationPath',('"'+(Join-Path $fixture ($Name+'.jsonl'))+'"'),
        '-QuietWindow','-SettlingSeconds','0','-PollSeconds','1')
    if ($Arm) {
        $args += @('-ArmRollback','-RecoveryDirectory',('"'+$fixture+'"'),
            '-RollbackScript',('"'+(Join-Path $fixture 'fixture-rollback.ps1')+'"'))
    }
    Start-Process powershell.exe -WindowStyle Hidden -PassThru -ArgumentList $args -RedirectStandardOutput (Join-Path $fixture ($Name+'.stdout')) -RedirectStandardError (Join-Path $fixture ($Name+'.stderr'))
}

try {
    Set-Content -LiteralPath (Join-Path $fixture 'diagnostic.jsonl') -Value '' -Encoding ASCII
    [pscustomobject]@{time=[DateTimeOffset]::UtcNow.ToString('o');msg='RATE_LIMITED fixture'} | ConvertTo-Json -Compress | Set-Content -LiteralPath (Join-Path $fixture 'tunnel.jsonl') -Encoding ASCII
    $process = Start-FixtureWatcher 'hold'
    $deadline = [DateTime]::UtcNow.AddSeconds(12)
    $observations = @()
    do {
        Start-Sleep -Milliseconds 250
        $process.Refresh()
        if ($process.HasExited) { throw 'HOLD exited the watcher and lost observation.' }
        $path = Join-Path $fixture 'hold.jsonl'
        if (Test-Path -LiteralPath $path) { $observations = @(Get-Content -LiteralPath $path | ForEach-Object { $_ | ConvertFrom-Json }) }
    } while ($observations.Count -lt 4 -and [DateTime]::UtcNow -lt $deadline)
    if ($observations.Count -lt 4 -or $observations[0].action -ne 'INFO' -or $observations[0].reason -ne 'guard_started' -or
        @($observations | Select-Object -Skip 1 | Where-Object action -ne 'HOLD').Count) { throw 'HOLD was not observed continuously after guard_started.' }
    Stop-Process -Id $process.Id -Force
    $process.WaitForExit()
    $process = $null
    Write-Output 'PASS: HOLD keeps local observation active'

    Set-Content -LiteralPath (Join-Path $fixture 'tunnel.jsonl') -Value '' -Encoding ASCII
    Set-Content -LiteralPath (Join-Path $fixture 'diagnostic.jsonl') -Value '{"time":"invalid","phase":"mcp_start","rpc_method":"tools/call"}' -Encoding ASCII
    @'
param([string]$RecoveryDirectory,[string]$ExpectedGoExecutable,[string]$GoServiceName,[switch]$Execute)
if (!$Execute -or $GoServiceName -ne 'CodexifyNonexistentFixture') { throw 'Invalid fixture rollback arguments' }
Set-Content -LiteralPath (Join-Path $RecoveryDirectory 'rollback-called.txt') -Value 'fixture only' -Encoding ASCII
exit 0
'@ | Set-Content -LiteralPath (Join-Path $fixture 'fixture-rollback.ps1') -Encoding ASCII
    $process = Start-FixtureWatcher 'failure' -Arm
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    $failureRows = @()
    do {
        Start-Sleep -Milliseconds 250
        $process.Refresh()
        if ($process.HasExited) { throw 'Guard exited on a telemetry parsing failure.' }
        $abortLog = Join-Path $fixture 'guard-abort.log'
        if (Test-Path -LiteralPath $abortLog) {
            $failureRows = @(Get-Content -LiteralPath $abortLog | Where-Object { $_ -match 'observation_failed#' })
        }
    } while ($failureRows.Count -lt 3 -and [DateTime]::UtcNow -lt $deadline)
    if ($failureRows.Count -lt 3) { throw 'Observation failure did not produce durable diagnostics.' }
    $last = Get-Content -LiteralPath (Join-Path $fixture 'failure.jsonl') -Tail 1 | ConvertFrom-Json
    if ($last.action -ne 'HOLD' -or $last.reason -ne 'observer_unhealthy') { throw 'Observer must degrade rather than roll back a live Go owner.' }
    if (Test-Path -LiteralPath (Join-Path $fixture 'rollback-called.txt')) { throw 'Observer fault triggered an unwanted Go-to-Rust rollback.' }
    if (!(Select-String -LiteralPath $abortLog -Pattern 'OBSERVER_DEGRADED' -Quiet)) { throw 'Missing observer degradation audit record.' }
    Stop-Process -Id $process.Id -Force
    $process.WaitForExit()
    $process = $null
    Write-Output 'PASS: observer failure records degraded G10 without invoking fixture rollback'
    Write-Output 'CUTOVER_WATCHER_LIFECYCLE=PASS'
} finally {
    if ($process -and !$process.HasExited) { Stop-Process -Id $process.Id -Force; $process.WaitForExit() }
    $tempPrefix = [IO.Path]::GetFullPath($env:TEMP).TrimEnd('\') + '\'
    if (!$fixture.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path -Leaf $fixture) -notmatch '^codexify-watch-fixture-[a-f0-9]{32}$') { throw 'Unsafe fixture cleanup path.' }
    Remove-Item -LiteralPath $fixture -Recurse -Force
}
