# Go-only to Go-only upgrade executor. The default mode is strictly
# read-only. Execution requires an explicit independently launched, elevated
# Scheduled Task plus live 24h G10 readiness and frozen asset verification.
# Never starts Rust or performs an automatic Rust fallback.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TransactionDirectory,
    [switch]$Execute,
    [string]$ExpectedSourceCommit = '',
    [string]$OperatorTaskName = ''
)
$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyUpgrade.psm1') -Force
Import-Module (Join-Path $PSScriptRoot 'GoOnlyTrialTransaction.psm1') -Force
$root=[IO.Path]::GetFullPath($TransactionDirectory)
$file=Join-Path $root 'manifest.json'
if (!(Test-Path -LiteralPath $file -PathType Leaf)) {throw 'No protected prepared Go-only transaction found'}
if (!(Get-Acl -LiteralPath $root).AreAccessRulesProtected) {
    # A transaction can inherit a protected root while having its own ACL
    # inheritance enabled. Accept only explicit or inherited protected ACLs
    # from a verified parent.
    $parent=Split-Path -Parent $root
    if (!(Get-Acl -LiteralPath $parent).AreAccessRulesProtected) {
        throw 'Prepared operator transaction is not below a protected root'
    }
}
$m=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json -ErrorAction Stop
if ([int]$m.version -ne 1 -or [string]$m.transactionKind -ne 'go_only_watch_go_trial_to_go_only' -or
    [bool]$m.autoRustFallback -or [string]$m.stageSourceCommit -notmatch '^[a-f0-9]{40}$') {
    throw 'Unexpected or unsafe Go-only transaction manifest'
}
if ($ExpectedSourceCommit -ne '' -and $ExpectedSourceCommit -ne [string]$m.stageSourceCommit) {
    throw 'Explicit expected candidate commit differs from prepared manifest'
}
$operatorRoot=Join-Path $root 'operator'
foreach($entry in @($m.operatorFiles.PSObject.Properties)){
    $scriptPath=Join-Path $operatorRoot $entry.Name
    if (!(Test-Path -LiteralPath $scriptPath -PathType Leaf) -or
        (Get-FileHash -LiteralPath $scriptPath -Algorithm SHA256).Hash -ne [string]$entry.Value) {
        throw 'Frozen transaction operator script hash mismatch'
    }
}
$oldCopy=Join-Path $root 'previous-go\codexify-go.exe'
$oldCfgCopy=Join-Path $root 'previous-go\production.json'
if ((Get-FileHash -LiteralPath $oldCopy -Algorithm SHA256).Hash -ne [string]$m.oldSnapshotSha -or
    (Get-FileHash -LiteralPath $oldCfgCopy -Algorithm SHA256).Hash -ne [string]$m.oldConfigSnapshotSha) {
    throw 'Frozen previous Go recovery snapshot changed'
}
[void](Assert-GoUpgradeAssets -OldExecutable $m.oldExe -NewExecutable $m.newExe -ConfigPath $m.configPath -OldSHA256 $m.oldSha -NewSHA256 $m.newSha -ConfigSHA256 $m.configSha)
if ((Get-FileHash -LiteralPath $m.oldConfigPath -Algorithm SHA256).Hash -ne [string]$m.oldConfigSha) {
    throw 'Production configuration changed since Go-only preparation'
}
$xml=Join-Path $root 'g10-observers-before.xml'
if ((Get-FileHash -LiteralPath $xml -Algorithm SHA256).Hash -ne [string]$m.taskXmlSHA) {
    throw 'Frozen original G10 task XML changed'
}
$stage=Split-Path -Parent $root
$planRaw=& (Join-Path $PSScriptRoot 'Prepare-GoOnlyTrialUpgrade.ps1') -StagingDirectory $stage -CurrentG10RunDirectory $m.oldRunDirectory -TransactionDirectory $root -ServiceName $m.serviceName -G10TaskName $m.g10TaskName
$plan=$planRaw | ConvertFrom-Json -ErrorAction Stop
$report=[ordered]@{
    checkedAtUtc=[DateTimeOffset]::UtcNow.ToString('o')
    mode=$(if($Execute){'EXECUTE_REQUESTED'}else{'PLAN_ONLY'})
    decision=$plan.decision
    blockingReasons=$plan.blockReasons
    previousGoHash=$m.oldSha
    candidateGoHash=$m.newSha
    sourceCommit=$m.stageSourceCommit
    watcherKind='Watch-GoTrial-GoOnly'
    rustFallback=$false
    productionChanged=$false
}
if (!$Execute) { $report|ConvertTo-Json -Depth 7;return }
if ($plan.decision -ne 'READY_FOR_INDEPENDENT_OPERATOR') {
    throw 'Live G10/Go-only preflight is HOLD; execution forbidden'
}
if ($ExpectedSourceCommit -ne [string]$m.stageSourceCommit) {
    throw 'Execute requires the exact prepared source commit'
}

# The operator task must be a distinct explicit on-demand Highest task.
# Never accept a direct MCP command or a call by the worker it will replace.
$expectedTask='CodexifyGo-PGCanary-'+$m.stageSourceCommit.Substring(0,12)
if ($OperatorTaskName -ne $expectedTask) {
    throw 'Only the independently scheduled, commit-pinned operator task can execute'
}
$independent=Get-ScheduledTask -TaskName $expectedTask -ErrorAction Stop
$actions=@($independent.Actions)
if ($independent.State -ne 'Running' -or $independent.Principal.RunLevel -ne 'Highest' -or
    @($independent.Triggers | Where-Object { $null -ne $_ }).Count -ne 0 -or $actions.Count -ne 1 -or
    [string]$actions[0].Arguments -notmatch 'Invoke-GoOnlyTrialUpgrade[.]ps1' -or
    [string]$actions[0].Arguments -notmatch '[-]Execute' -or
    ([string]$actions[0].Arguments).IndexOf($root,[StringComparison]::OrdinalIgnoreCase) -lt 0) {
    throw 'Independent Highest on-demand operator task not verified'
}
$principal=[Security.Principal.WindowsPrincipal]([Security.Principal.WindowsIdentity]::GetCurrent())
if (!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'An elevated independent operator process is required'
}
$ancestors=@(Get-CimInstance Win32_Process -ErrorAction Stop)
$currentPID=$PID
for($depth=0;$depth -lt 16;$depth++) {
    $self=@($ancestors|Where-Object{[int]$_.ProcessId -eq [int]$currentPID})
    if($self.Count -ne 1){break}
    if($self[0].Name -ieq 'codexify-go.exe' -or
       $self[0].Name -ieq 'tunnel-client-runtime.exe' -or
       $self[0].Name -ieq 'codexify.exe'){
        throw 'Operator execution is a child of Codexify; it cannot survive an SCM restart'
    }
    $currentPID=[int]$self[0].ParentProcessId
    if($currentPID -le 0){break}
}

# Preflight was read-only. Only after all guards pass does the task enter
# the mutation section below.
$events=Join-Path $root 'evidence\operator-events.jsonl'
$lockPath=Join-Path $root 'go-only-trial.running.lock'
if (Test-Path -LiteralPath $lockPath) {throw 'Go-only trial transaction was already attempted; do not retry implicitly'}
$lock=[IO.File]::Open($lockPath,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
$lock.Close()
$taskDisabled=$false
$oldWatcherStopped=$false
$imageChanged=$false
$newTaskActionSet=$false
$oldService=Get-CimInstance Win32_Service -Filter ("Name='"+[string]$m.serviceName+"'")
if (!$oldService -or [int]$oldService.ProcessId -ne [int]$m.oldServicePID) {
    throw 'Original Go SCM supervisor PID changed; prepare a new transaction before operator execution'
}
$oldPID=[int]$oldService.ProcessId
$oldTask=Get-ScheduledTask -TaskName $m.g10TaskName -ErrorAction Stop
$oldAction=@($oldTask.Actions)[0]
if ([string]$oldAction.Arguments -ne [string]$m.originalTaskActionArguments) {
    throw 'Existing G10 task action differs from the protected snapshot'
}
$originalWatcher=Get-CimInstance Win32_Process -Filter ('ProcessId='+[int]$m.oldWatcherPID) -ErrorAction Stop
if (!$originalWatcher -or $originalWatcher.Name -ine 'powershell.exe' -or
    [string]$originalWatcher.CommandLine -notmatch 'Watch-GoTrial[.]ps1' -or
    ([string]$originalWatcher.CommandLine).IndexOf([string]$m.oldRunDirectory,[StringComparison]::OrdinalIgnoreCase) -lt 0) {
    throw 'Previous Go-only watcher process was restarted since preparation; reprepare the transaction'
}
$newHealthURI=[uri]([string](Get-Content -LiteralPath $m.configPath -Raw|ConvertFrom-Json).tunnel.mcpServerUrl)
if($newHealthURI.Scheme -ne 'http' -or $newHealthURI.Host -ne '127.0.0.1') {
    throw 'Go-only trial must use the previously verified loopback MCP endpoint'
}
$health='http://127.0.0.1:'+$newHealthURI.Port+'/health'
function Test-LiveGoOwner([string]$Image, [string]$HealthURL, [int]$TimeoutSeconds) {
    for($i=0;$i -lt $TimeoutSeconds;$i++){
        Start-Sleep -Seconds 1
        $sv=Get-CimInstance Win32_Service -Filter ("Name='"+[string]$m.serviceName+"'") -ErrorAction SilentlyContinue
        if(!$sv -or $sv.State -ne 'Running' -or [int]$sv.ProcessId -lt 1){continue}
        $processes=@(Get-CimInstance Win32_Process -ErrorAction SilentlyContinue)
        $go=@($processes|Where-Object{$_.Name -ieq 'codexify-go.exe'})
        $rust=@($processes|Where-Object{$_.Name -ieq 'codexify.exe'})
        $super=@($go|Where-Object{[int]$_.ProcessId -eq [int]$sv.ProcessId -and (Compare-GoPath $_.ExecutablePath $Image)})
        $workers=@($go|Where-Object{[int]$_.ParentProcessId -eq [int]$sv.ProcessId -and (Compare-GoPath $_.ExecutablePath $Image)})
        $tunnels=@($processes|Where-Object{$_.Name -ieq 'tunnel-client-runtime.exe' -and [int]$_.ParentProcessId -eq [int]$sv.ProcessId})
        if($go.Count -ne 2 -or $rust.Count -ne 0 -or $super.Count -ne 1 -or $workers.Count -ne 1 -or $tunnels.Count -ne 1){continue}
        try{
            if((Invoke-WebRequest -Uri $HealthURL -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop).StatusCode -eq 200){return $true}
        }catch{}
    }
    return $false
}
function Restore-OriginalWatcher {
    $restoreAction=New-ScheduledTaskAction -Execute $oldAction.Execute -Argument $oldAction.Arguments
    Set-ScheduledTask -TaskName $m.g10TaskName -Action $restoreAction -ErrorAction Stop|Out-Null
    Enable-ScheduledTask -TaskName $m.g10TaskName -ErrorAction Stop|Out-Null
    if($oldWatcherStopped) {
        Start-ScheduledTask -TaskName $m.g10TaskName -ErrorAction Stop
    }
}
try {
    Write-GoUpgradeEvent -Path $events -Phase 'go_only_trial_independent_task_started' -Result 'PASS'
    Disable-ScheduledTask -TaskName $m.g10TaskName -ErrorAction Stop | Out-Null
    $taskDisabled=$true
    $watcherID=[int]$m.oldWatcherPID
    $watcher=Get-CimInstance Win32_Process -Filter ('ProcessId='+$watcherID) -ErrorAction Stop
    if(!$watcher -or $watcher.Name -ine 'powershell.exe' -or
        [string]$watcher.CommandLine -notmatch 'Watch-GoTrial[.]ps1' -or
        ([string]$watcher.CommandLine).IndexOf([string]$m.oldRunDirectory,[StringComparison]::OrdinalIgnoreCase) -lt 0) {
        throw 'Go-only watcher identity changed since preparation'
    }
    Stop-Process -Id $watcherID -Force -ErrorAction Stop
    for($i=0;$i -lt 20;$i++){
        if(!(Get-Process -Id $watcherID -ErrorAction SilentlyContinue)){break}
        Start-Sleep -Milliseconds 250
    }
    if(Get-Process -Id $watcherID -ErrorAction SilentlyContinue){throw 'Old Go-only watcher did not terminate'}
    $oldWatcherStopped=$true
    Write-GoUpgradeEvent -Path $events -Phase 'old_go_only_watcher_retired' -Result 'PASS'

    # Win32_Service.Change, never direct registry overwrite or Rust takeover.
    # If Win32_Service.Change applies the new ImagePath but its post-change
    # verification fails, the catch handler must still attempt previous-Go
    # restoration. Never leave the SCM pointing at an unverified candidate.
    $imageChanged=$true
    Set-GoServiceImage -ServiceName $m.serviceName -ImagePath $m.newImage
    Write-GoUpgradeEvent -Path $events -Phase 'new_go_image_selected' -Result 'PASS'
    Stop-Service -Name $m.serviceName -ErrorAction Stop
    (Get-Service -Name $m.serviceName).WaitForStatus('Stopped',[timespan]::FromSeconds(30))
    Start-Service -Name $m.serviceName -ErrorAction Stop
    if(!(Test-LiveGoOwner -Image $m.newExe -HealthURL $health -TimeoutSeconds 75)) {
        throw 'Candidate Go supervisor/worker/tunnel failed to reach unique local health'
    }
    Write-GoUpgradeEvent -Path $events -Phase 'candidate_go_only_owner_healthy' -Result 'PASS'
    $watchPath=Join-Path $operatorRoot 'Watch-GoTrial.ps1'
    $newArgs='-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$watchPath+'" -TransactionDirectory "'+$root+'" -ArmGoRecovery -IntervalSeconds 10 -DurationHours 27'
    $newAction=New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $newArgs
    Set-ScheduledTask -TaskName $m.g10TaskName -Action $newAction -ErrorAction Stop|Out-Null
    $newTaskActionSet=$true
    Enable-ScheduledTask -TaskName $m.g10TaskName -ErrorAction Stop|Out-Null
    Start-ScheduledTask -TaskName $m.g10TaskName -ErrorAction Stop
    $newGuardFile=Join-Path $root 'evidence\probe-observations.jsonl'
    $ready=$false
    for($i=0;$i -lt 40;$i++){
        Start-Sleep -Seconds 1
        if(!(Test-Path -LiteralPath $newGuardFile -PathType Leaf)){continue}
        try {
            $entry=Get-Content -LiteralPath $newGuardFile -Tail 1|ConvertFrom-Json
            $age=([DateTimeOffset]::UtcNow-[DateTimeOffset]::Parse([string]$entry.time)).TotalSeconds
            if($age -lt 15 -and $entry.action -eq 'CONTINUE' -and
                $entry.goOwners -eq 2 -and $entry.rustOwners -eq 0 -and
                $entry.controlled -eq $true){$ready=$true;break}
        }catch{}
    }
    if(!$ready){throw 'New Go-only G10 watcher did not produce a fresh controlled CONTINUE'}
    Write-GoUpgradeEvent -Path $events -Phase 'go_only_trial_watcher_ready' -Result 'PASS'
    [pscustomobject]@{
        status='GO_UPGRADED';owner='go'
        candidateSHA=$m.newSha
        g10Acceptance='RESET_AND_RUNNING_NOT_ACCEPTED'
        featureFlagEnabled=$false
        hostedChatGPTUI='NOT_YET_VERIFIED'
        autoRustFallback=$false
        atUtc=[DateTimeOffset]::UtcNow.ToString('o')
    }|ConvertTo-Json -Depth 5|Set-Content -LiteralPath (Join-Path $root 'upgrade-result.json') -Encoding UTF8
    Write-GoUpgradeEvent -Path $events -Phase 'go_only_trial_upgrade_completed' -Result 'PASS'
}catch{
    $reason=$_.Exception.GetType().Name
    try{Write-GoUpgradeEvent -Path $events -Phase 'go_only_trial_upgrade_failed' -Result 'ERROR' -Reason $reason}catch{}
    $recovered=$false
    try{
        $current=Get-CimInstance Win32_Service -Filter ("Name='"+[string]$m.serviceName+"'") -ErrorAction Stop
        $activeImage=Get-GoServiceExecutable ([string]$current.PathName)
        $imageOwner='foreign'
        if(Compare-GoPath $activeImage $m.oldExe){$imageOwner='previous_go'}
        elseif(Compare-GoPath $activeImage $m.newExe){$imageOwner='candidate_go'}
        $recoveryDecision=Get-GoOnlyTrialRecoveryDecision -ServiceImageMayHaveChanged $imageChanged -ImageOwner $imageOwner -OldObserverWasDisabled $taskDisabled
        if($recoveryDecision.action -eq 'BLOCKED_FOREIGN_OR_UNKNOWN_OWNER'){
            throw 'Foreign SCM ownership detected; operator recovery required'
        }
        if($recoveryDecision.mayRestartService){
            if(!(Compare-GoPath $activeImage $m.oldExe)){
                Set-GoServiceImage -ServiceName $m.serviceName -ImagePath $m.originalImage
            }
            $running=Get-Service -Name $m.serviceName -ErrorAction Stop
            if($running.Status -ne 'Stopped'){
                Stop-Service -Name $m.serviceName -ErrorAction Stop
                $running.WaitForStatus('Stopped',[timespan]::FromSeconds(30))
            }
            Start-Service -Name $m.serviceName -ErrorAction Stop
            if(!(Test-LiveGoOwner -Image $m.oldExe -HealthURL $health -TimeoutSeconds 60)){
                throw 'Previous verified Go did not return to unique healthy ownership'
            }
        }
        if($recoveryDecision.mayRestoreOldWatcher){Restore-OriginalWatcher}
        $recovered=$true
        Write-GoUpgradeEvent -Path $events -Phase 'previous_go_and_go_only_watcher_restored' -Result 'PASS'
    }catch{
        try{Write-GoUpgradeEvent -Path $events -Phase 'go_only_recovery_requires_operator' -Result 'ERROR' -Reason $_.Exception.GetType().Name}catch{}
    }
    [pscustomobject]@{
        status='GO_UPGRADE_FAILED'
        previousGoRecoverySucceeded=$recovered
        rustFallback=$false
        manualOperatorReviewRequired=!$recovered
    }|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $root 'upgrade-result.json') -Encoding UTF8
    throw 'Go-only trial upgrade failed. Read protected operator-events/upgrade-result; never start Rust.'
}
