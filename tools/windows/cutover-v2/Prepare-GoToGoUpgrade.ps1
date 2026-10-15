[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TransactionDirectory,
    [Parameter(Mandatory)][string]$NewExecutable,
    [Parameter(Mandatory)][string]$ExpectedNewSHA256,
    [Parameter(Mandatory)][string]$ExpectedOldSHA256,
    [Parameter(Mandatory)][string]$ConfigPath,
    [string]$ServiceName='CodexifyGo',
    [string]$G10TaskName='CodexifyGo-G10Observers',
    [switch]$Prepare
)
$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyUpgrade.psm1') -Force

$svc=Get-CimInstance Win32_Service -Filter ("Name='"+$ServiceName.Replace("'","''")+"'")
if(!$svc -or $svc.State -ne 'Running' -or [int]$svc.ProcessId -le 0) {
    throw 'Go service is not running: cannot prepare a guarded live upgrade'
}
$oldExe=Get-GoServiceExecutable ([string]$svc.PathName)
$oldHash=(Get-FileHash -LiteralPath $oldExe -Algorithm SHA256).Hash
if($oldHash -ne $ExpectedOldSHA256.ToUpperInvariant()){throw 'Live old Go executable hash mismatch'}
$newExe=[IO.Path]::GetFullPath($NewExecutable)
$newHash=(Get-FileHash -LiteralPath $newExe -Algorithm SHA256).Hash
if($newHash -ne $ExpectedNewSHA256.ToUpperInvariant()){throw 'New Go candidate hash mismatch'}
$config=[IO.Path]::GetFullPath($ConfigPath)
$configHash=(Get-FileHash -LiteralPath $config -Algorithm SHA256).Hash
[void](Assert-GoUpgradeAssets -OldExecutable $oldExe -NewExecutable $newExe -ConfigPath $config -OldSHA256 $oldHash -NewSHA256 $newHash -ConfigSHA256 $configHash)
$cfg=Get-Content -LiteralPath $config -Raw | ConvertFrom-Json
if($cfg.service.name -ne $ServiceName){throw 'Config service name mismatch'}
if([string]$cfg.tunnel.mcpServerUrl -notmatch '^http://127\.0\.0\.1:[0-9]+/'){throw 'MCP upstream loopback URL invalid'}
if($null -ne $cfg.tunnel.PSObject.Properties['pollFailureThreshold'] -and
   [int]$cfg.tunnel.pollFailureThreshold -lt 1){
    throw 'G03 poll watcher is explicitly disabled in this profile'
}

$live=Get-CimInstance Win32_Process -Filter ('ProcessId='+[int]$svc.ProcessId)
if(!$live -or !(Compare-GoPath $live.ExecutablePath $oldExe)){throw 'SCM PID is not the expected Go supervisor'}
$all=@(Get-CimInstance Win32_Process)
$rust=@($all|Where-Object{$_.Name -ieq 'codexify.exe'})
$go=@($all|Where-Object{$_.Name -ieq 'codexify-go.exe'})
$tunnel=@($all|Where-Object{$_.Name -ieq 'tunnel-client-runtime.exe' -and [int]$_.ParentProcessId -eq [int]$svc.ProcessId})
if($rust.Count -ne 0 -or $go.Count -ne 2 -or $tunnel.Count -ne 1){
    throw 'Existing Go-only owner topology is not healthy/unique'
}
foreach($name in @('Codexify','Codexify Watchdog')) {
    $rustTask=Get-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue
    if($rustTask -and $rustTask.State -ne 'Disabled'){throw "Rust task $name not disabled"}
}
$boot=Get-ScheduledTask -TaskName $G10TaskName -ErrorAction Stop
if(@($boot.Triggers | Where-Object {$_.CimClass.CimClassName -eq 'MSFT_TaskBootTrigger'}).Count -lt 1){
    throw 'Existing G10 observer task has no boot trigger'
}
if($boot.Principal.RunLevel -ne 'Highest'){throw 'G10 task is not running with highest privileges'}
$watchers=@($all|Where-Object {
    $_.Name -ieq 'powershell.exe' -and $_.CommandLine -match '(?i)-File\s+"?[^"]*Watch-Cutover\.ps1' -and
    $_.CommandLine -match '(?i)-ArmRollback' -and $_.CommandLine.Contains($oldExe)
})
if($watchers.Count -ne 1){throw "Expected exactly one old rollback-armed watcher, got $($watchers.Count)"}
$reg=Get-Item ('HKLM:\SYSTEM\CurrentControlSet\Services\'+$ServiceName) -ErrorAction Stop
$regImage=[string]$reg.GetValue('ImagePath')
if(!(Compare-GoPath (Get-GoServiceExecutable $regImage) $oldExe)){throw 'SCM registry executable disagrees with running supervisor'}

$transaction=[IO.Path]::GetFullPath($TransactionDirectory)
$nextImage='"'+$newExe+'" service run --config "'+$config+'"'
$planned=[pscustomobject]@{
    status='READY_FOR_PREPARATION';service=$ServiceName
    oldHash=$oldHash;newHash=$newHash;configHash=$configHash
    transaction=$transaction;oldServicePID=$svc.ProcessId;oldWatcherPID=$watchers[0].ProcessId
    restartsServiceDuringExecute=$true;planOnly=(!$Prepare);automaticRustRollback=$false
}
if(!$Prepare){$planned|ConvertTo-Json -Depth 4;return}

if(Test-Path -LiteralPath $transaction){throw 'Transaction directory exists; refusing overwrite'}
New-Item -ItemType Directory -Force -Path (Join-Path $transaction 'operator'),(Join-Path $transaction 'evidence')|Out-Null
$files=@('GoOnlyUpgrade.psm1','Invoke-GoOnlyRecovery.ps1','Watch-GoTrial.ps1','Invoke-GoToGoUpgrade.ps1')
foreach($name in $files){
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path (Join-Path $transaction 'operator') $name) -ErrorAction Stop
}
$taskXml=Export-ScheduledTask -TaskName $G10TaskName -ErrorAction Stop
Set-Content -LiteralPath (Join-Path $transaction 'g10-observers-before.xml') -Value $taskXml -Encoding UTF8
$manifest=[ordered]@{
    version=1;createdAtUtc=[DateTimeOffset]::UtcNow.ToString('o')
    serviceName=$ServiceName;g10TaskName=$G10TaskName
    oldExe=$oldExe;newExe=$newExe;configPath=$config
    oldSha=$oldHash;newSha=$newHash;configSha=$configHash
    originalImage=$regImage;newImage=$nextImage
    originalImageKind=[string]$reg.GetValueKind('ImagePath')
    oldWatcherPID=[int]$watchers[0].ProcessId
    oldServicePID=[int]$svc.ProcessId
    taskXmlSHA=(Get-FileHash -LiteralPath (Join-Path $transaction 'g10-observers-before.xml') -Algorithm SHA256).Hash
    autoRustFallback=$false
}
$manifest | ConvertTo-Json -Depth 7 | Set-Content -LiteralPath (Join-Path $transaction 'manifest.json') -Encoding UTF8
Write-GoUpgradeEvent -Path (Join-Path $transaction 'evidence\operator-events.jsonl') -Phase 'go_upgrade_prepared' -Result 'PASS'
$planned.status='PREPARED_FOR_INDEPENDENT_TASK'
$planned|ConvertTo-Json -Depth 4
