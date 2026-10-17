# G11 production smoke observer. Read-only, independent of the MCP worker.
# Records 31 samples over >=30 minutes; no service, task, tunnel or config writes.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$PolicyFile,
    [Parameter(Mandatory)][string]$EvidenceDirectory,
    [Parameter(Mandatory)][int]$ExpectedSupervisorPid,
    [ValidateRange(20,180)][int]$IntervalSeconds=60,
    [switch]$Once
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$pins=Get-Content -LiteralPath $PolicyFile -Raw|ConvertFrom-Json
if([int]$pins.version -ne 1 -or [string]$pins.serviceName -ne 'CodexifyGo' -or
   @($pins.additionalCurrentProfiles).Count -ne 1){throw 'Unexpected G11 policy'}
$normal=$pins.additionalCurrentProfiles[0]
$folder=[IO.Path]::GetFullPath($EvidenceDirectory)
if(!(Test-Path -LiteralPath $folder -PathType Container)){throw 'Evidence directory must already exist'}
$events=Join-Path $folder $(if($Once){'smoke-single-sample.jsonl'}else{'smoke-samples.jsonl'})
$resultFile=Join-Path $folder $(if($Once){'smoke-single-result.json'}else{'smoke-30min-result.json'})
if(Test-Path -LiteralPath $events){throw 'Refusing to overwrite prior smoke samples'}
if(Test-Path -LiteralPath $resultFile){throw 'Refusing to overwrite prior smoke outcome'}
$started=[DateTimeOffset]::UtcNow
$lastSample=$null
$expectedWorker=0
$expectedTunnel=0
$errors=New-Object 'System.Collections.Generic.List[string]'
$sampleCount=if($Once){1}else{31}
$failReason=''
function Get-Sample([int]$Sequence){
    $now=[DateTimeOffset]::UtcNow
    $svc=Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'" -ErrorAction Stop
    $procs=@(Get-CimInstance Win32_Process -ErrorAction Stop)
    $go=@($procs|Where-Object {$_.Name -ieq 'codexify-go.exe'})
    $tunnels=@($procs|Where-Object {$_.Name -ieq 'tunnel-client-runtime.exe'})
    $rust=@($procs|Where-Object {$_.Name -ieq 'codexify.exe'})
    $workers=@($go|Where-Object {[int]$_.ParentProcessId -eq $ExpectedSupervisorPid})
    $ownedTunnel=@($tunnels|Where-Object {[int]$_.ParentProcessId -eq $ExpectedSupervisorPid})
    $g10=Get-ScheduledTask -TaskName CodexifyGo-G10Observers -ErrorAction Stop
    $g11=Get-ScheduledTask -TaskName CodexifyGo-G11SteadyState -ErrorAction Stop
    $g11Info=Get-ScheduledTaskInfo -TaskName CodexifyGo-G11SteadyState -ErrorAction Stop
    $state=Get-Content -LiteralPath (Join-Path (Split-Path -Parent $PolicyFile) 'evidence\state.json') -Raw|ConvertFrom-Json
    $health=$false
    try{$health=((Invoke-WebRequest -Uri 'http://127.0.0.1:3300/health' -UseBasicParsing -TimeoutSec 3 -ErrorAction Stop).StatusCode -eq 200)}catch{}
    $rustTasksOK=$true
    foreach($name in @('Codexify','Codexify Watchdog')){
        if((Get-ScheduledTask -TaskName $name -ErrorAction Stop).State -ne 'Disabled'){$rustTasksOK=$false}
    }
    $imageOK=($svc.State -eq 'Running' -and $svc.StartMode -eq 'Auto' -and
        [int]$svc.ProcessId -eq $ExpectedSupervisorPid -and
        [string]$svc.PathName -ceq [string]$normal.image)
    $topologyOK=($go.Count -eq 2 -and $workers.Count -eq 1 -and
        $tunnels.Count -eq 1 -and $ownedTunnel.Count -eq 1 -and $rust.Count -eq 0)
    $g11OK=($g10.State -eq 'Disabled' -and $rustTasksOK -and
        [string]$g11.State -in @('Ready','Running') -and
        $g11.Actions[0].Arguments -match ' -RecoveryEnabled' -and
        [string]$state.lastAction -eq 'CONTINUE' -and
        !$state.recoveryAttemptedUtc -and
        [DateTimeOffset]::Parse([string]$state.lastCheckUtc) -ge $now.AddMinutes(-3) -and
        ($g11Info.LastTaskResult -eq 0 -or
            ($g11.State -eq 'Running' -and $g11Info.LastTaskResult -eq 267009)))
    $hashOK=$true
    if($Sequence -eq 0 -or $Sequence -eq $sampleCount-1 -or $Sequence%5 -eq 0){
        $hashOK=((Get-FileHash -LiteralPath $pins.currentExecutable -Algorithm SHA256).Hash -eq [string]$pins.currentSha -and
            (Get-FileHash -LiteralPath $normal.config -Algorithm SHA256).Hash -eq [string]$normal.sha -and
            (Get-FileHash -LiteralPath $pins.previousExecutable -Algorithm SHA256).Hash -eq [string]$pins.previousSha)
    }
    $workerPid=if($workers.Count -eq 1){[int]$workers[0].ProcessId}else{0}
    $tunnelPid=if($ownedTunnel.Count -eq 1){[int]$ownedTunnel[0].ProcessId}else{0}
    $durationSec=if($null -eq $script:lastSample){0}else{[int]($now-$script:lastSample).TotalSeconds}
    $stableOwners=($workerPid -gt 0 -and $tunnelPid -gt 0)
    if($script:expectedWorker -ne 0 -and $workerPid -ne $script:expectedWorker){$stableOwners=$false}
    if($script:expectedTunnel -ne 0 -and $tunnelPid -ne $script:expectedTunnel){$stableOwners=$false}
    if($durationSec -gt ($IntervalSeconds*2+20)){$stableOwners=$false}
    if($script:expectedWorker -eq 0 -and $workerPid -gt 0){$script:expectedWorker=$workerPid}
    if($script:expectedTunnel -eq 0 -and $tunnelPid -gt 0){$script:expectedTunnel=$tunnelPid}
    $script:lastSample=$now
    $pass=($imageOK -and $topologyOK -and $g11OK -and $hashOK -and $health -and $stableOwners)
    return [pscustomobject]@{
        timeUtc=$now.ToString('o');sample=$Sequence;pass=$pass
        scmPid=[int]$svc.ProcessId;workerPid=$workerPid;tunnelPid=$tunnelPid
        localHealth200=$health;scmPinned=$imageOK;topology=$topologyOK
        g11Continue=$g11OK;hashPinned=$hashOK;stableOwners=$stableOwners
        g11TaskResult=[int]$g11Info.LastTaskResult;gapSeconds=$durationSec
    }
}
try{
    for($n=0;$n -lt $sampleCount;$n++){
        $s=Get-Sample -Sequence $n
        $s|ConvertTo-Json -Compress|Add-Content -LiteralPath $events -Encoding UTF8 -ErrorAction Stop
        if(!$s.pass){$failReason='sample_failed_'+$n;break}
        if($n -lt $sampleCount-1){Start-Sleep -Seconds $IntervalSeconds}
    }
    if(!$failReason -and !$Once){
        $actualSamples=@(Get-Content -LiteralPath $events).Count
        $actualMinutes=([DateTimeOffset]::UtcNow-$started).TotalMinutes
        if($actualSamples -ne 31 -or $actualMinutes -lt 30){$failReason='insufficient_samples_or_duration'}
    }
}catch{
    $failReason='observer_exception_'+$_.Exception.GetType().Name
}
$end=[DateTimeOffset]::UtcNow
$out=[pscustomobject]@{
    status=$(if($failReason){'FAIL'}elseif($Once){'SINGLE_SAMPLE_PASS_NOT_ACCEPTANCE'}else{'G11_30_MIN_SMOKE_PASS'})
    errorType=$failReason
    firstUtc=$started.ToString('o');lastUtc=$end.ToString('o')
    durationSeconds=[int]($end-$started).TotalSeconds
    samples=$(if(Test-Path -LiteralPath $events){@(Get-Content -LiteralPath $events).Count}else{0})
    expectedSupervisorPid=$ExpectedSupervisorPid
    expectedWorkerPid=$expectedWorker
    expectedTunnelPid=$expectedTunnel
    projectGroupsEnabled=$false
    serviceMutations=0;rustFallback=$false
}
$out|ConvertTo-Json -Depth 4|Set-Content -LiteralPath $resultFile -Encoding UTF8 -ErrorAction Stop
$out|ConvertTo-Json -Depth 4
if($failReason){exit 1}