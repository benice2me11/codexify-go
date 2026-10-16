# Go-only to Go-only preparation: PLAN_ONLY reads state; -Prepare creates a
# protected, inert transaction. Never edits SCM, tasks, watcher or tunnel.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$StagingDirectory,
    [Parameter(Mandatory)][string]$CurrentG10RunDirectory,
    [string]$TransactionDirectory = '',
    [string]$ServiceName = 'CodexifyGo',
    [string]$G10TaskName = 'CodexifyGo-G10Observers',
    [switch]$Prepare
)
$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyUpgrade.psm1') -Force
Import-Module (Join-Path $PSScriptRoot 'GoOnlyTrialTransaction.psm1') -Force
function Read-ProfilePath([string]$line) {
    if ($line -match '--config\s+"([^"]+)"') { return $Matches[1] }
    if ($line -match '--config\s+(\S+)') { return $Matches[1] }
    throw 'Running Go service has no explicit config argument'
}
$stage=[IO.Path]::GetFullPath($StagingDirectory)
$run=[IO.Path]::GetFullPath($CurrentG10RunDirectory)
$stageManifestFile=Join-Path $stage 'manifest.json'
if (!(Test-Path -LiteralPath $stageManifestFile -PathType Leaf)) {throw 'Missing staged candidate manifest'}
$staged=Get-Content -LiteralPath $stageManifestFile -Raw|ConvertFrom-Json
if ($staged.state -ne 'STAGED_ONLY_NOT_DEPLOYED' -or
    $staged.sourceCommit -notmatch '^[a-f0-9]{40}$' -or
    [bool]$staged.flags.projectGroupsEnabled) {
    throw 'Staged manifest is not a feature-disabled immutable candidate'
}
if (!(Get-Acl -LiteralPath $stage).AreAccessRulesProtected) {throw 'Staging ACL inherits unsafe permissions'}
$newExe=[IO.Path]::GetFullPath((Join-Path $stage $staged.stagePaths.candidateBinary))
$newCfgPath=[IO.Path]::GetFullPath((Join-Path $stage $staged.stagePaths.stagedConfig))
$frozenExe=[IO.Path]::GetFullPath((Join-Path $stage $staged.stagePaths.previousGo))
$frozenCfg=[IO.Path]::GetFullPath((Join-Path $stage $staged.stagePaths.previousConfig))
foreach($path in @($newExe,$newCfgPath,$frozenExe,$frozenCfg)) {
    if (!(Test-Path -LiteralPath $path -PathType Leaf)){throw 'One staged immutable asset is missing'}
}
$newSHA=(Get-FileHash -LiteralPath $newExe -Algorithm SHA256).Hash
$newCfgSHA=(Get-FileHash -LiteralPath $newCfgPath -Algorithm SHA256).Hash
if ($newSHA -ne $staged.candidateBinarySha256 -or
    $newCfgSHA -ne $staged.stagedGroupConfigSha256) {throw 'Staged candidate or config hash mismatch'}
$svc=Get-CimInstance Win32_Service -Filter ("Name='"+$ServiceName.Replace("'","''")+"'") -ErrorAction Stop
if (!$svc -or $svc.State -ne 'Running' -or [int]$svc.ProcessId -lt 1) {throw 'Go SCM is not running'}
$live=Get-CimInstance Win32_Process -Filter ('ProcessId='+[int]$svc.ProcessId) -ErrorAction Stop
$oldExe=Get-GoServiceExecutable ([string]$svc.PathName)
if (!$live -or !(Compare-GoPath $live.ExecutablePath $oldExe)) {throw 'SCM Go owner path differs from process'}
$oldSHA=(Get-FileHash -LiteralPath $oldExe -Algorithm SHA256).Hash
$oldCfgPath=[IO.Path]::GetFullPath((Read-ProfilePath ([string]$live.CommandLine)))
$oldCfgSHA=(Get-FileHash -LiteralPath $oldCfgPath -Algorithm SHA256).Hash
if ($oldSHA -ne [string]$staged.previousVerifiedGoSha256 -or
    $oldSHA -ne (Get-FileHash -LiteralPath $frozenExe -Algorithm SHA256).Hash -or
    $oldCfgSHA -ne [string]$staged.originalProductionConfigSha256 -or
    $oldCfgSHA -ne (Get-FileHash -LiteralPath $frozenCfg -Algorithm SHA256).Hash) {
    throw 'Live previous Go or config differs from frozen verified state'
}
[void](Assert-GoUpgradeAssets -OldExecutable $oldExe -NewExecutable $newExe -ConfigPath $newCfgPath -OldSHA256 $oldSHA -NewSHA256 $newSHA -ConfigSHA256 $newCfgSHA)
$oldConfig=Get-Content -LiteralPath $oldCfgPath -Raw|ConvertFrom-Json
$newConfig=Get-Content -LiteralPath $newCfgPath -Raw|ConvertFrom-Json
[void](Assert-GoOnlyTrialProfile -Previous $oldConfig -Candidate $newConfig)
$oldRunManifestFile=Join-Path $run 'manifest.json'
if (!(Test-Path -LiteralPath $oldRunManifestFile -PathType Leaf)){throw 'Go-only observer manifest missing'}
$oldRunManifest=Get-Content -LiteralPath $oldRunManifestFile -Raw|ConvertFrom-Json
if ([string]$oldRunManifest.newSha -ne $oldSHA -or
    !(Compare-GoPath ([string]$oldRunManifest.newExe) $oldExe) -or
    !(Compare-GoPath ([string]$oldRunManifest.configPath) $oldCfgPath)) {
    throw 'Go-only observer manifest is not bound to running Go'
}
$all=@(Get-CimInstance Win32_Process -ErrorAction Stop)
$go=@($all|Where-Object{$_.Name -ieq 'codexify-go.exe'})
$rust=@($all|Where-Object{$_.Name -ieq 'codexify.exe'})
$workers=@($go|Where-Object{[int]$_.ParentProcessId -eq [int]$svc.ProcessId})
$tunnel=@($all|Where-Object{$_.Name -ieq 'tunnel-client-runtime.exe' -and [int]$_.ParentProcessId -eq [int]$svc.ProcessId})
$watchers=@($all|Where-Object{
    $_.Name -ieq 'powershell.exe' -and
    [string]$_.CommandLine -match '(?i)-File\s+"?[^"]*Watch-GoTrial[.]ps1(?:"|\s|$)' -and
    [string]$_.CommandLine -match '(?i)-TransactionDirectory\s+' -and
    [string]$_.CommandLine.IndexOf($run,[StringComparison]::OrdinalIgnoreCase) -ge 0
})
$rustWatchers=@($all|Where-Object{
    $_.Name -ieq 'powershell.exe' -and $_.CommandLine -match '(?i)-File\s+"?[^"]*Watch-Cutover[.]ps1' -and
    $_.CommandLine -match '(?i)-ArmRollback'
})
if ($rustWatchers.Count -ne 0) {throw 'Rust-capable watcher still active'}
$task=Get-ScheduledTask -TaskName $G10TaskName -ErrorAction Stop
$actions=@($task.Actions)
$boot=@($task.Triggers|Where-Object{$_.CimClass.CimClassName -eq 'MSFT_TaskBootTrigger'})
$taskPinned=$false
if ($actions.Count -eq 1) {
    $taskPinned=([string]$actions[0].Arguments).IndexOf($run,[StringComparison]::OrdinalIgnoreCase) -ge 0 -and
        ([string]$actions[0].Arguments) -match 'Watch-GoTrial[.]ps1'
}
$rustTasksDisabled=$true
foreach($name in @('Codexify','Codexify Watchdog')){
    $rt=Get-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue
    if($rt -and $rt.State -ne 'Disabled'){$rustTasksDisabled=$false}
}
$reg=Get-Item ('HKLM:\SYSTEM\CurrentControlSet\Services\'+$ServiceName) -ErrorAction Stop
$originalImage=[string]$reg.GetValue('ImagePath')
if (!(Compare-GoPath (Get-GoServiceExecutable $originalImage) $oldExe)){throw 'SCM registry path mismatch'}
$g10=& (Join-Path $PSScriptRoot 'Assess-ProjectGroupsCanaryReadiness.ps1') -RunDirectory $run | ConvertFrom-Json -ErrorAction Stop
$facts=[pscustomobject]@{
    ServiceRunning=($svc.State -eq 'Running')
    GoCount=$go.Count;WorkerCount=$workers.Count;TunnelCount=$tunnel.Count
    RustCount=$rust.Count;RustTasksDisabled=$rustTasksDisabled
    WatcherCount=$watchers.Count
    WatcherMatchesLiveGo=($watchers.Count -eq 1 -and [string]$watchers[0].CommandLine -match '(?i)-ArmGoRecovery')
    WatcherTaskPinned=$taskPinned;BootTriggerPresent=($boot.Count -gt 0)
    WatcherHighest=($task.Principal.RunLevel -eq 'Highest')
    WatcherRecoveryArmed=($actions.Count -eq 1 -and [string]$actions[0].Arguments -match '(?i)-ArmGoRecovery')
    OldHashValid=$true;NewHashValid=$true;ConfigHashValid=$true
    SnapshotHashValid=$true;IndependentOperatorReady=$true
    FeatureFlagDisabled=!([bool]$newConfig.mcp.projectGroupsEnabled)
    ConfigScopePreserved=$true
    G10Decision=$g10.decision
}
$policy=Get-GoOnlyTrialPreflightDecision -Facts $facts
$transaction=if($TransactionDirectory){[IO.Path]::GetFullPath($TransactionDirectory)}else{Join-Path $stage 'operator-trial-v2'}
if (!$transaction.StartsWith($stage.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase)){
    throw 'Operator transaction must remain inside protected staging root'
}
$preflight=[ordered]@{
    checkedAtUtc=[DateTimeOffset]::UtcNow.ToString('o')
    phase=$(if($Prepare){'PREPARE_SNAPSHOT_ONLY'}else{'PLAN_ONLY'})
    decision=$policy.decision;blockReasons=$policy.reasons
    oldGoOnlyWatcherCount=$watchers.Count;rustOwners=$rust.Count
    oldGoHash=$oldSHA;candidateGoHash=$newSHA;candidateProfileHash=$newCfgSHA
    candidateSourceCommit=$staged.sourceCommit;g10Decision=$g10.decision
    proposedTransaction=$transaction
    liveServiceRestarted=$false;liveWatcherModified=$false
    featureFlagEnabled=$false;rustFallback=$false
}
if (!$Prepare){$preflight | ConvertTo-Json -Depth 8;return}
if (Test-Path -LiteralPath $transaction){throw 'Transaction directory exists; refusing to overwrite'}
[void](New-Item -ItemType Directory -Path (Join-Path $transaction 'operator'),(Join-Path $transaction 'evidence'),(Join-Path $transaction 'previous-go') -Force)
$oldCopy=Join-Path $transaction 'previous-go\codexify-go.exe'
$oldConfigCopy=Join-Path $transaction 'previous-go\production.json'
Copy-Item -LiteralPath $oldExe -Destination $oldCopy -ErrorAction Stop
Copy-Item -LiteralPath $oldCfgPath -Destination $oldConfigCopy -ErrorAction Stop
if ((Get-FileHash -LiteralPath $oldCopy -Algorithm SHA256).Hash -ne $oldSHA -or
    (Get-FileHash -LiteralPath $oldConfigCopy -Algorithm SHA256).Hash -ne $oldCfgSHA){
    throw 'Previous-Go snapshot hashes differ'
}
$xmlPath=Join-Path $transaction 'g10-observers-before.xml'
[IO.File]::WriteAllText($xmlPath,(Export-ScheduledTask -TaskName $G10TaskName -ErrorAction Stop),[Text.UTF8Encoding]::new($false))
$operatorFiles=@('GoOnlyUpgrade.psm1','GoOnlyTrialTransaction.psm1','Watch-GoTrial.ps1',
    'Invoke-GoOnlyRecovery.ps1','Invoke-GoOnlyTrialUpgrade.ps1','Prepare-GoOnlyTrialUpgrade.ps1',
    'New-GoOnlyTrialOperatorTask.ps1',
    'Assess-ProjectGroupsCanaryReadiness.ps1','Inspect-G10Runtime.ps1')
$hashes=[ordered]@{}
foreach($name in $operatorFiles) {
    $from=Join-Path $PSScriptRoot $name
    if (!(Test-Path -LiteralPath $from -PathType Leaf)) {throw "Missing operator script $name"}
    $to=Join-Path (Join-Path $transaction 'operator') $name
    Copy-Item -LiteralPath $from -Destination $to -ErrorAction Stop
    $hashes[$name]=(Get-FileHash -LiteralPath $to -Algorithm SHA256).Hash
}
$newImage='"'+$newExe+'" service run --config "'+$newCfgPath+'"'
$manifest=[ordered]@{
    version=1;transactionKind='go_only_watch_go_trial_to_go_only'
    stageSourceCommit=$staged.sourceCommit;createdAtUtc=[DateTimeOffset]::UtcNow.ToString('o')
    serviceName=$ServiceName;g10TaskName=$G10TaskName
    oldRunDirectory=$run;oldExe=$oldExe;oldSha=$oldSHA
    oldConfigPath=$oldCfgPath;oldConfigSha=$oldCfgSHA
    oldSnapshotSha=$oldSHA;oldConfigSnapshotSha=$oldCfgSHA
    newExe=$newExe;newSha=$newSHA;configPath=$newCfgPath;configSha=$newCfgSHA
    originalImage=$originalImage;newImage=$newImage
    originalImageKind=[string]$reg.GetValueKind('ImagePath')
    oldWatcherPID=$(if($watchers.Count -eq 1){[int]$watchers[0].ProcessId}else{0})
    oldServicePID=[int]$svc.ProcessId
    taskXmlSHA=(Get-FileHash -LiteralPath $xmlPath -Algorithm SHA256).Hash
    originalTaskActionArguments=$(if($actions.Count -eq 1){[string]$actions[0].Arguments}else{''})
    oldGoOnlyWatcherRunHash=(Get-FileHash -LiteralPath $oldRunManifestFile -Algorithm SHA256).Hash
    operatorFiles=$hashes
    preparedG10Decision=$g10.decision
    preparedExecutionEligible=($policy.decision -eq 'READY_FOR_INDEPENDENT_OPERATOR')
    autoRustFallback=$false
}
[IO.File]::WriteAllText((Join-Path $transaction 'manifest.json'),($manifest|ConvertTo-Json -Depth 14),[Text.UTF8Encoding]::new($false))
$preflight.phase='PREPARED_READ_ONLY_SNAPSHOT'
$preflight.transactionPrepared=$true
$preflight.executionArmed=$false
$preflight|ConvertTo-Json -Depth 8
