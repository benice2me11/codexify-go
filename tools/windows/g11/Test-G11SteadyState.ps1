# Offline G11 safety tests. Never touch SCM, production tasks, or real G10.
$ErrorActionPreference='Stop'
$dir=$PSScriptRoot
Import-Module (Join-Path $dir 'G11SteadyState.psm1') -Force
function Assert([bool]$OK,[string]$What){if(!$OK){throw ('G11 fixture FAIL: '+$What)}}
foreach($scriptName in @('G11SteadyState.psm1','Invoke-G11SteadyState.ps1','Manage-G11Task.ps1','New-G11NormalConfig.ps1')){
    $path=Join-Path $dir $scriptName
    if(!(Test-Path -LiteralPath $path)){throw "G11 required script missing: $scriptName"}
    $tokens=$null;$errors=$null
    [void][Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$errors)
    Assert (@($errors).Count -eq 0) "PowerShell AST: $scriptName"
}
$healthy=[pscustomobject]@{
    ProbeError='';ServiceState='Running';ImageKind='current'
    RustCount=0;RustTasksDisabled=$true;GoCount=2
    UnexpectedGoCount=0;UnexpectedTunnelCount=0
    SupervisorCount=1;WorkerCount=1;TunnelCount=1
    G10TaskEnabled=$false;G10WatcherCount=0;G10TaskUnpinned=$false;UnexpectedWatchdogCount=0
    ScmRecoveryValid=$true;RecoveryAssetsValid=$true;Health='healthy'
    Http403=0;Http429=0;Http503=0;VpnDown=$false
}
$now=[DateTimeOffset]::Parse('2026-10-16T09:00:00Z')
function Decide($S){Get-G11Decision -Snapshot $S -Now $now}
Assert ((Decide $healthy).action -eq 'CONTINUE') 'normal Go'
$network=$healthy.PSObject.Copy()
$network.Http403=100;$network.Http429=100;$network.Http503=100;$network.VpnDown=$true
Assert ((Decide $network).action -eq 'CONTINUE') 'VPN/403/429/503 must not recover'
$worker=$healthy.PSObject.Copy();$worker.WorkerCount=0;$worker.GoCount=1
Assert ((Decide $worker).reason -eq 'worker_self_healing') 'worker auto-managed'
$tunnel=$healthy.PSObject.Copy();$tunnel.TunnelCount=0
Assert ((Decide $tunnel).reason -eq 'tunnel_self_healing') 'tunnel auto-managed'
$health=$healthy.PSObject.Copy();$health.Health='unhealthy'
Assert ((Decide $health).action -eq 'HOLD') '/health alone must not roll back'
$rust=$healthy.PSObject.Copy();$rust.RustCount=1
Assert ((Decide $rust).action -eq 'BLOCKED') 'Rust owner'
$rust=$healthy.PSObject.Copy();$rust.RustTasksDisabled=$false
Assert ((Decide $rust).action -eq 'BLOCKED') 'Rust task'
$foreign=$healthy.PSObject.Copy();$foreign.ImageKind='foreign'
Assert ((Decide $foreign).action -eq 'BLOCKED') 'foreign SCM image'
$duplicate=$healthy.PSObject.Copy();$duplicate.GoCount=3
Assert ((Decide $duplicate).reason -eq 'duplicate_or_foreign_owner') 'duplicate Go'
$duplicate=$healthy.PSObject.Copy();$duplicate.UnexpectedTunnelCount=1
Assert ((Decide $duplicate).action -eq 'BLOCKED') 'duplicate tunnel'
$stale=$healthy.PSObject.Copy();$stale.G10TaskEnabled=$true;$stale.G10WatcherCount=0
Assert ((Decide $stale).reason -eq 'g10_watcher_missing_or_stale') 'stale G10 watcher'
$watchdog=$healthy.PSObject.Copy();$watchdog.UnexpectedWatchdogCount=1
Assert ((Decide $watchdog).action -eq 'BLOCKED') 'foreign watchdog process'
$watchdog=$healthy.PSObject.Copy();$watchdog.G10TaskUnpinned=$true
Assert ((Decide $watchdog).action -eq 'BLOCKED') 'unpinned G10 task'
$compete=$healthy.PSObject.Copy();$compete.G10TaskEnabled=$true;$compete.G10WatcherCount=1
Assert ((Get-G11Decision -Snapshot $compete -RecoveryEnabled).reason -eq 'competing_g10_recovery_authority') 'double armed watcher'
Assert ((Decide $compete).action -eq 'CONTINUE') 'parallel read-only observation allowed'
$previous=$healthy.PSObject.Copy();$previous.ImageKind='previous'
Assert ((Decide $previous).action -eq 'GO_PREVIOUS_RECOVERED') 'previous Go must not oscillate'
$previous.ServiceState='Stopped';$previous.GoCount=0;$previous.SupervisorCount=0;$previous.WorkerCount=0;$previous.TunnelCount=0
Assert ((Decide $previous).reason -eq 'previous_go_degraded_manual_recovery') 'previous Go offline is not healthy'
$orphan=$healthy.PSObject.Copy();$orphan.ServiceState='Stopped';$orphan.SupervisorCount=0
Assert ((Decide $orphan).reason -eq 'orphaned_owner_when_supervisor_unavailable') 'orphaned Go'
$down=$healthy.PSObject.Copy();$down.ServiceState='Stopped';$down.SupervisorCount=0
$down.WorkerCount=0;$down.GoCount=0;$down.TunnelCount=0
$state=$null
for($i=0;$i -lt 6;$i++){
    $d=Get-G11Decision -Snapshot $down -PreviousState $state -Now $now.AddMinutes($i) -RecoveryEnabled
    if($i -lt 5){Assert ($d.action -eq 'HOLD') 'SCM grace period'}
    $state=$d.nextState
}
Assert ($d.action -eq 'RECOVER_GO' -and $d.recoveryAllowed) 'only six checks and five minutes may recover'
$once=$state.PSObject.Copy()
$once|Add-Member -NotePropertyName recoveryAttemptedUtc -NotePropertyValue $now.ToString('o')
$d=Get-G11Decision -Snapshot $down -PreviousState $once -Now $now.AddMinutes(6) -RecoveryEnabled
Assert ($d.action -eq 'HOLD' -and $d.reason -eq 'recovery_already_attempted_operator_required') 'no recovery loop'
$staleState=Get-G11Decision -Snapshot $down -PreviousState $state -Now $now.AddHours(2) -RecoveryEnabled
Assert ($staleState.consecutiveFailures -eq 1) 'reboot or missed polls reset streak'
$scm=$down.PSObject.Copy();$scm.ScmRecoveryValid=$false
$d=Get-G11Decision -Snapshot $scm -PreviousState $state -Now $now.AddMinutes(6) -RecoveryEnabled
Assert ($d.action -eq 'HOLD' -and $d.reason -eq 'scm_recovery_not_verified') 'SCM recovery must be verified'
$asset=$down.PSObject.Copy();$asset.RecoveryAssetsValid=$false
$d=Get-G11Decision -Snapshot $asset -PreviousState $state -Now $now.AddMinutes(6) -RecoveryEnabled
Assert ($d.action -eq 'HOLD') 'previous Go asset hash mismatch'
$handover=Test-G11RecoveryHandover -G10TaskDisabled $true -G10WatcherCount 0 -RustTasksDisabled $true -OtherG11TaskCount 0 -AssetsVerified $true -ProgramDataProtected $true
Assert $handover.ready 'valid one-owner handover'
foreach($bad in @(
    @{G10TaskDisabled=$false},
    @{G10WatcherCount=1},
    @{RustTasksDisabled=$false},
    @{OtherG11TaskCount=1},
    @{AssetsVerified=$false},
    @{ProgramDataProtected=$false}
)){
    $p=@{G10TaskDisabled=$true;G10WatcherCount=0;RustTasksDisabled=$true;OtherG11TaskCount=0;AssetsVerified=$true;ProgramDataProtected=$true}
    foreach($k in $bad.Keys){$p[$k]=$bad[$k]}
    Assert (!(Test-G11RecoveryHandover @p).ready) 'unsafe handover accepted'
}

# A second process cannot take the incident/recovery mutex concurrently.
$mutexName='Local\CodexifyG11Fixture-'+[guid]::NewGuid().ToString('N')
$job=Start-Job -ArgumentList (Join-Path $dir 'G11SteadyState.psm1'),$mutexName -ScriptBlock {
    param($module,$name)
    Import-Module $module -Force
    $lock=Enter-G11Mutex -Name $name
    if(!$lock){throw 'fixture worker could not acquire mutex'}
    'ACQUIRED'
    try{Start-Sleep -Seconds 8}finally{Exit-G11Mutex $lock}
}
try {
    $acquired=$false
    $deadline=(Get-Date).AddSeconds(12)
    do {
        Start-Sleep -Milliseconds 250
        if(@(Receive-Job -Job $job -Keep -ErrorAction SilentlyContinue) -contains 'ACQUIRED'){$acquired=$true;break}
    }while((Get-Date) -lt $deadline)
    Assert $acquired 'mutex-holder fixture started'
    $second=Enter-G11Mutex -Name $mutexName
    Assert ($null -eq $second) 'cross-process duplicate lock rejected'
}finally{
    if($second){Exit-G11Mutex $second}
    Stop-Job -Job $job -ErrorAction SilentlyContinue
    Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
}

$root=Join-Path $env:TEMP ('codexify-g11-fixture-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root|Out-Null
try{
    $policy=Join-Path $root 'pins.json'
    $sample=Join-Path $root 'snapshot.json'
    $healthy|ConvertTo-Json|Set-Content -LiteralPath $sample -Encoding UTF8
    '{"version":1,"serviceName":"CodexifyGo"}'|Set-Content -LiteralPath $policy -Encoding ASCII
    $run=Join-Path $dir 'Invoke-G11SteadyState.ps1'
    $actual=& $run -PolicyFile $policy -DataDirectory $root -Offline -FixtureSnapshotPath $sample|ConvertFrom-Json
    Assert ($actual.action -eq 'CONTINUE' -and !$actual.mutatedService) 'isolated fixture run'
    Assert (Test-Path -LiteralPath (Join-Path $root 'state.json')) 'durable state created'
    $event=Join-Path $root 'events.jsonl'
    Remove-Item -LiteralPath $event -Force
    New-Item -ItemType Directory -Path $event|Out-Null
    $blocked=$false
    try{
        $foreign|ConvertTo-Json|Set-Content -LiteralPath $sample -Encoding UTF8
        & $run -PolicyFile $policy -DataDirectory $root -Offline -FixtureSnapshotPath $sample|Out-Null
    } catch {$blocked=$true}
    Assert $blocked 'log write failure must fail closed'

    # Exercise the actual installer in PLAN_ONLY against disposable files.
    # No live task/service is registered, restarted, or modified.
    $oldExe=Join-Path $root 'old-go.exe'
    $newExe=Join-Path $root 'new-go.exe'
    $oldCfg=Join-Path $root 'old-config.json'
    $newCfg=Join-Path $root 'new-config.json'
    'fixture previous Go'|Set-Content -LiteralPath $oldExe -Encoding ASCII
    'fixture current Go'|Set-Content -LiteralPath $newExe -Encoding ASCII
    '{}'|Set-Content -LiteralPath $oldCfg -Encoding ASCII
    '{}'|Set-Content -LiteralPath $newCfg -Encoding ASCII
    $manifest=[pscustomobject]@{
        version=1;transactionKind='go_only_watch_go_trial_to_go_only'
        serviceName='CodexifyGo';g10TaskName='CodexifyGo-G10Observers'
        oldExe=$oldExe;oldSha=(Get-FileHash $oldExe -Algorithm SHA256).Hash
        newExe=$newExe;newSha=(Get-FileHash $newExe -Algorithm SHA256).Hash
        oldConfigPath=$oldCfg;oldConfigSha=(Get-FileHash $oldCfg -Algorithm SHA256).Hash
        configPath=$newCfg;configSha=(Get-FileHash $newCfg -Algorithm SHA256).Hash
        originalImage='"C:\fixtures\old-go.exe" service run'
        newImage='"C:\fixtures\new-go.exe" service run'
        autoRustFallback=$false
    }
    $manifest|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $root 'manifest.json') -Encoding UTF8
    $installer=Join-Path $dir 'Manage-G11Task.ps1'
    $planned=& $installer -TransactionDirectory $root -Action InstallReadOnly|ConvertFrom-Json
    Assert ($planned.status -eq 'PLAN_ONLY' -and !$planned.serviceMutation -and !$planned.rustFallback) 'installer must default to inert plan'
    $notArmed=& $installer -TransactionDirectory $root -Action ArmRecovery|ConvertFrom-Json
    Assert ($notArmed.status -eq 'PLAN_ONLY') 'recovery plan must not arm a task'
    $t=New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 1) -RepetitionDuration (New-TimeSpan -Days 3650)
    $boot=New-ScheduledTaskTrigger -AtStartup
    $settings=New-ScheduledTaskSettingsSet -MultipleInstances IgnoreNew -StartWhenAvailable
    $system=New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
    Assert ($t.Repetition.Interval -eq 'PT1M' -and $boot.CimClass.CimClassName -eq 'MSFT_TaskBootTrigger' -and
        [string]$settings.MultipleInstances -eq 'IgnoreNew' -and
        [string]$system.LogonType -eq 'ServiceAccount') 'Windows native durable no-overlap task spec'
}finally{
    $prefix=[IO.Path]::GetFullPath($env:TEMP).TrimEnd('\')+'\'
    if(!$root.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path -Leaf $root) -notmatch '^codexify-g11-fixture-[a-f0-9]{32}$'){throw 'Unsafe fixture cleanup'}
    Remove-Item -LiteralPath $root -Recurse -Force
}
'G11_STEADY_STATE_OFFLINE=PASS'
