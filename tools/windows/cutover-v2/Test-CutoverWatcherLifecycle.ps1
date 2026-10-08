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
    } while ($observations.Count -lt 3 -and [DateTime]::UtcNow -lt $deadline)
    if ($observations.Count -lt 3 -or @($observations | Where-Object action -ne 'HOLD').Count) { throw 'HOLD was not observed continuously.' }
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
    if (!$process.WaitForExit(12000)) { throw 'Observation failure did not stop the watcher.' }
    if (!(Test-Path -LiteralPath (Join-Path $fixture 'rollback-called.txt'))) { throw 'Observation failure did not invoke the fixture rollback.' }
    $last = Get-Content -LiteralPath (Join-Path $fixture 'failure.jsonl') -Tail 1 | ConvertFrom-Json
    if ($last.action -ne 'ABORT' -or $last.reason -ne 'observation_failed') { throw 'Missing observation failure ABORT evidence.' }
    Write-Output 'PASS: observation failure invokes local fixture rollback'
    Write-Output 'CUTOVER_WATCHER_LIFECYCLE=PASS'
} finally {
    if ($process -and !$process.HasExited) { Stop-Process -Id $process.Id -Force; $process.WaitForExit() }
    $tempPrefix = [IO.Path]::GetFullPath($env:TEMP).TrimEnd('\') + '\'
    if (!$fixture.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path -Leaf $fixture) -notmatch '^codexify-watch-fixture-[a-f0-9]{32}$') { throw 'Unsafe fixture cleanup path.' }
    Remove-Item -LiteralPath $fixture -Recurse -Force
}
