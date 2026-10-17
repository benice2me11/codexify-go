# One-shot, Go-only Project Groups canary. Run PLAN_ONLY first.
# In Execute mode this file must be copied to protected ProgramData and run
# as an independent elevated operator (never from the MCP worker).
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$PolicyFile,
    [Parameter(Mandatory)][string]$CandidateConfig,
    [Parameter(Mandatory)][string]$ExpectedCandidateSha256,
    [Parameter(Mandatory)][string]$ExpectedPolicySha256,
    [Parameter(Mandatory)][string]$EvidenceDirectory,
    [int]$ExpectedServicePid = 0,
    [switch]$Execute,
    [switch]$OperatorApproved
)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'

function FileHash([string]$Path) {
    (Get-FileHash -LiteralPath $Path -Algorithm SHA256 -ErrorAction Stop).Hash
}
function CheckHash([string]$Path,[string]$Expected,[string]$Kind) {
    if($Expected -notmatch '^[a-fA-F0-9]{64}$' -or
       !(Test-Path -LiteralPath $Path -PathType Leaf) -or
       (FileHash $Path) -ne $Expected.ToUpperInvariant()){
        throw "SHA256 mismatch: $Kind"
    }
}
function AssertProtected([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path -ErrorAction Stop
    foreach($entry in $acl.Access){
        if($entry.AccessControlType -ne 'Allow'){continue}
        $sid=$entry.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        $danger=[Security.AccessControl.FileSystemRights]::Write -bor
            [Security.AccessControl.FileSystemRights]::Modify -bor
            [Security.AccessControl.FileSystemRights]::FullControl
        if(($entry.FileSystemRights -band $danger) -and
           $sid -notin @('S-1-5-18','S-1-5-32-544')){
            throw 'Canary operator has an untrusted writer'
        }
    }
}
function AssertGoTopology([int]$SupervisorPid,[string]$Exe) {
    $all=@(Get-CimInstance Win32_Process -ErrorAction Stop)
    $go=@($all|Where-Object{$_.Name -ieq 'codexify-go.exe'})
    $tunnel=@($all|Where-Object{$_.Name -ieq 'tunnel-client-runtime.exe'})
    $rust=@($all|Where-Object{$_.Name -ieq 'codexify.exe'})
    $super=@($go|Where-Object{
        [int]$_.ProcessId -eq $SupervisorPid -and
        [string]::Equals([string]$_.ExecutablePath,$Exe,[StringComparison]::OrdinalIgnoreCase)
    })
    $worker=@($go|Where-Object{[int]$_.ParentProcessId -eq $SupervisorPid})
    $owned=@($tunnel|Where-Object{[int]$_.ParentProcessId -eq $SupervisorPid})
    return ($go.Count -eq 2 -and $tunnel.Count -eq 1 -and $rust.Count -eq 0 -and
        $super.Count -eq 1 -and $worker.Count -eq 1 -and $owned.Count -eq 1)
}
function HealthOK {
    try{(Invoke-WebRequest -Uri 'http://127.0.0.1:3300/health' -UseBasicParsing -TimeoutSec 3).StatusCode -eq 200}
    catch{return $false}
}
function WaitForNewGo([string]$Image,[string]$Exe,[int]$OldPid,[int]$Seconds=90){
    for($i=0;$i -lt $Seconds;$i++){
        Start-Sleep -Seconds 1
        $s=Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'" -ErrorAction SilentlyContinue
        if($s -and $s.State -eq 'Running' -and [int]$s.ProcessId -gt 0 -and
           [int]$s.ProcessId -ne $OldPid -and [string]$s.PathName -ceq $Image -and
           (AssertGoTopology -SupervisorPid ([int]$s.ProcessId) -Exe $Exe) -and (HealthOK)){
            return [int]$s.ProcessId
        }
    }
    return 0
}
function WriteResult([string]$Status,[int]$ServicePid=0){
    $result=[pscustomobject]@{
        status=$Status;utc=[DateTimeOffset]::UtcNow.ToString('o')
        previousServicePid=$initialPid;servicePid=$ServicePid
        currentGoSha=$pins.currentSha;originalConfigSha=$normal.sha
        projectGroupsConfigSha=$ExpectedCandidateSha256.ToUpperInvariant()
        projectGroupsEnabled=($Status -eq 'PROJECT_GROUPS_ENABLED')
        rustFallback=$false
    }
    $dst=Join-Path $EvidenceDirectory 'canary-result.json'
    $tmp=$dst+'.tmp'
    $result|ConvertTo-Json -Depth 5|Set-Content -LiteralPath $tmp -Encoding UTF8
    Move-Item -LiteralPath $tmp -Destination $dst -Force
}
function RecordEvent([string]$Event,[string]$Result){
    [pscustomobject]@{utc=[DateTimeOffset]::UtcNow.ToString('o');event=$Event;result=$Result}|
        ConvertTo-Json -Compress|Add-Content -LiteralPath (Join-Path $EvidenceDirectory 'canary-events.jsonl') -Encoding UTF8
}

$policy=[IO.Path]::GetFullPath($PolicyFile)
$source=[IO.Path]::GetFullPath($CandidateConfig)
$evidence=[IO.Path]::GetFullPath($EvidenceDirectory)
$root=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'CodexifyGo\G11')).TrimEnd('\')+'\'
if($Execute){
    if(!$OperatorApproved){throw 'Execute requires explicit operator approval'}
    $principal=New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    if(!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Elevated operator required'}
    foreach($item in @($policy,$evidence,$PSScriptRoot)){
        if(!$item.StartsWith($root,[StringComparison]::OrdinalIgnoreCase)){
            throw 'Execute permitted only from protected ProgramData'
        }
    }
    foreach($item in @($PSScriptRoot,$evidence,(Split-Path -Parent $policy))){
        AssertProtected $item
    }
}
CheckHash $policy $ExpectedPolicySha256 'G11 approved policy'
CheckHash $source $ExpectedCandidateSha256 'disabled-to-enabled candidate'
$pins=Get-Content -LiteralPath $policy -Raw | ConvertFrom-Json -ErrorAction Stop
if([int]$pins.version -ne 1 -or [string]$pins.serviceName -ne 'CodexifyGo' -or
   @($pins.additionalCurrentProfiles).Count -ne 1 -or @($pins.recoveryProfiles).Count -ne 2){
    throw 'Unexpected G11 pin registry: refuse overlapping deployment'
}
$normal=$pins.additionalCurrentProfiles[0]
if([string]$normal.image -ceq [string]$pins.currentImage){throw 'Normal profile is not additional'}
CheckHash ([string]$pins.currentExecutable) ([string]$pins.currentSha) 'deployed Go'
CheckHash ([string]$pins.previousExecutable) ([string]$pins.previousSha) 'previous-Go recovery'
CheckHash ([string]$normal.config) ([string]$normal.sha) 'current normal config'
CheckHash ([string]$pins.currentConfig) ([string]$pins.currentConfigSha) 'diagnostic config'
CheckHash ([string]$pins.recoveryScript) ([string]$pins.recoveryScriptSha) 'protected recovery executor'
CheckHash ([string]$pins.recoveryModule) ([string]$pins.recoveryModuleSha) 'protected recovery module'
foreach($profile in $pins.recoveryProfiles){
    CheckHash (Join-Path ([string]$profile.directory) 'manifest.json') ([string]$profile.manifestSha) 'recovery profile'
}
$normalManifestProfile=@($pins.recoveryProfiles|Where-Object{[string]$_.image -ceq [string]$normal.image})
if($normalManifestProfile.Count -ne 1){throw 'Normal profile has no unique protected previous-Go recovery'}
$normalManifest=(Get-Content -LiteralPath (Join-Path $normalManifestProfile[0].directory 'manifest.json') -Raw|ConvertFrom-Json)
if([string]$normalManifest.newImage -cne [string]$normal.image -or
   [string]$normalManifest.oldExe -cne [string]$pins.previousExecutable -or
   [string]$normalManifest.oldSha -cne [string]$pins.previousSha -or
   [string]$normalManifest.originalImage -cne [string]$pins.previousImage){
    throw 'Normal recovery manifest identity differs'
}
# Byte-exact check: permit ONLY the feature boolean false -> true.
$enc=New-Object Text.UTF8Encoding($false,$true)
$raw=$enc.GetString([IO.File]::ReadAllBytes([string]$normal.config))
$token=[regex]::Matches($raw,'"projectGroupsEnabled"\s*:\s*false\b')
if($token.Count -ne 1){throw 'Ambiguous Project Groups token in current normal config'}
$from=$token[0].Value
$expectedRaw=$raw.Substring(0,$token[0].Index)+$from.Substring(0,$from.Length-5)+'true'+
    $raw.Substring($token[0].Index+$token[0].Length)
$expectedBytes=$enc.GetBytes($expectedRaw)
$candidateBytes=[IO.File]::ReadAllBytes($source)
if($expectedBytes.Length -ne $candidateBytes.Length -or
   ![Linq.Enumerable]::SequenceEqual([byte[]]$expectedBytes,[byte[]]$candidateBytes)){
    throw 'Candidate differs by more than the explicit feature flag'
}
$cfg=$expectedRaw|ConvertFrom-Json -ErrorAction Stop
if(!$cfg.mcp.multiProject -or !$cfg.mcp.projectGroupsEnabled -or
   $cfg.mcp.diagnostics.enabled -ne $false -or @($cfg.mcp.projectGroups).Count -ne 1 -or
   [string]$cfg.mcp.projectGroups[0].name -ne 'job-search' -or
   [string]$cfg.mcp.projectGroups[0].primary -ne 'work' -or
   @($cfg.mcp.projectGroups[0].repositories).Count -ne 3){
    throw 'Not the approved job-search source group'
}
foreach($repo in $cfg.mcp.projectGroups[0].repositories){
    if([string]$repo.checkout -ne 'source'){throw 'Canary may not generate managed worktrees'}
}
$scm=Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'" -ErrorAction Stop
if(!$scm -or $scm.State -ne 'Running' -or [int]$scm.ProcessId -le 0 -or
   [string]$scm.PathName -cne [string]$normal.image){throw 'SCM is not on approved normal Go'}
$initialPid=[int]$scm.ProcessId
if($ExpectedServicePid -gt 0 -and $initialPid -ne $ExpectedServicePid){throw 'Go supervisor PID changed'}
if(!(AssertGoTopology -SupervisorPid $initialPid -Exe ([string]$pins.currentExecutable)) -or !(HealthOK)){
    throw 'Go topology or local health not ready'
}
foreach($name in @('Codexify','Codexify Watchdog')){
    $t=Get-ScheduledTask -TaskName $name -ErrorAction Stop
    if([string]$t.State -ne 'Disabled'){throw 'Rust task not disabled'}
}
$g10=Get-ScheduledTask -TaskName 'CodexifyGo-G10Observers' -ErrorAction Stop
if([string]$g10.State -ne 'Disabled'){throw 'G10 watcher task not disabled'}
if(@(Get-CimInstance Win32_Process |Where-Object{
    $_.Name -in @('powershell.exe','pwsh.exe') -and $_.CommandLine -match 'Watch-GoTrial\.ps1'
}).Count -gt 0){throw 'G10 watcher still active'}
$g11=Get-ScheduledTask -TaskName 'CodexifyGo-G11SteadyState' -ErrorAction Stop
if([string]$g11.State -notin @('Ready','Running') -or
   [string]$g11.Principal.UserId -notin @('SYSTEM','S-1-5-18') -or
   @($g11.Actions).Count -ne 1 -or
   [string]$g11.Actions[0].Arguments -notmatch ' -RecoveryEnabled'){
    throw 'G11 recovery-armed monitor not present'
}
$marker=Get-Content -Raw -LiteralPath (Join-Path $root 'evidence\recovery-armed.json')|ConvertFrom-Json
if([string]$marker.manifestSha -ne [string]$pins.manifestSha){throw 'G11 recovery authorization mismatch'}
$state=Get-Content -Raw -LiteralPath (Join-Path $root 'evidence\state.json')|ConvertFrom-Json
$info=Get-ScheduledTaskInfo -TaskName 'CodexifyGo-G11SteadyState'
if([int]$info.LastTaskResult -ne 0 -or $state.lastAction -ne 'CONTINUE' -or
   [DateTimeOffset]::Parse([string]$state.lastCheckUtc) -lt [DateTimeOffset]::UtcNow.AddMinutes(-3)){
    throw 'G11 steady-state monitor unhealthy or stale'
}
$failures=(& sc.exe qfailure CodexifyGo 2>&1 | Out-String)
$delays=@([regex]::Matches($failures,'(?im)RESTART\s+--\s+Delay\s*=\s*(\d+)\s+milliseconds')|
    ForEach-Object{[int]$_.Groups[1].Value})
if($delays.Count -lt 3 -or $delays[0] -ne 5000 -or $delays[1] -ne 15000 -or $delays[2] -ne 60000 -or
   $failures -notmatch '(?im)RESET_PERIOD\s*\(in seconds\)\s*:\s*86400\b'){
    throw 'SCM failure policy changed'
}
if(!$Execute){
    [pscustomobject]@{
        status='PLAN_ONLY_CANARY_READY';servicePid=$initialPid
        goSha=$pins.currentSha;normalSha=$normal.sha;enabledSha=$ExpectedCandidateSha256.ToUpperInvariant()
        recoveryProfiles=2;g11Action=$state.lastAction;rustFallback=$false;serviceMutation=$false
    }|ConvertTo-Json -Depth 4
    return
}
# The remainder must run in a Task Scheduler SYSTEM process, independent of MCP.
$mutex=New-Object System.Threading.Mutex($false,'Global\CodexifyGo.G11ProjectGroupsEnable')
$acquired=$false
try {
    try{$acquired=$mutex.WaitOne(0)}catch [System.Threading.AbandonedMutexException]{$acquired=$true}
    if(!$acquired){throw 'Another Project Groups deployment is active'}
    $lock=Join-Path $evidence 'canary-once.lock'
    $handle=[IO.File]::Open($lock,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    $handle.Dispose()
    $newImage=''
    $imageChanged=$false
    $newServicePid=0
    try {
        # Protect config, rollback evidence and recovery manifest BEFORE changing SCM.
        $enabled=Join-Path $evidence 'enabled.json'
        if(Test-Path -LiteralPath $enabled){throw 'Canary config destination already exists'}
        [IO.File]::WriteAllBytes($enabled,$candidateBytes)
        CheckHash $enabled $ExpectedCandidateSha256 'protected enabled profile'
        $fromImage=[string]$normal.image
        $fromConfig=[string]$normal.config
        if($fromImage.Split([string[]]@($fromConfig),[StringSplitOptions]::None).Count -ne 2){
            throw 'Ambiguous SCM profile replacement'
        }
        $newImage=$fromImage.Replace($fromConfig,$enabled)
        $recoveryDir=Join-Path $root 'recovery\project-groups-20261017'
        if(Test-Path -LiteralPath $recoveryDir){throw 'Project Groups recovery profile already exists'}
        New-Item -ItemType Directory -Path (Join-Path $recoveryDir 'evidence') -Force | Out-Null
        foreach($d in @($recoveryDir,(Join-Path $recoveryDir 'evidence'))){
            & icacls.exe $d '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
            if($LASTEXITCODE -ne 0){throw 'Cannot protect Project Groups recovery'}
            AssertProtected $d
        }
        $newManifest=Get-Content -Raw -LiteralPath (Join-Path $normalManifestProfile[0].directory 'manifest.json')|ConvertFrom-Json
        $newManifest.newImage=$newImage
        $manifestPath=Join-Path $recoveryDir 'manifest.json'
        $newManifest|ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $manifestPath -Encoding UTF8
        $manifestHash=FileHash $manifestPath
        $backup=Join-Path $evidence 'runtime-pins-before.json'
        if(Test-Path -LiteralPath $backup){throw 'Cannot overwrite original G11 pins'}
        Copy-Item -LiteralPath $policy -Destination $backup -ErrorAction Stop
        $pins.additionalCurrentProfiles=@($pins.additionalCurrentProfiles)+@([pscustomobject]@{
            image=$newImage;config=$enabled;sha=$ExpectedCandidateSha256.ToUpperInvariant()
        })
        $pins.recoveryProfiles=@($pins.recoveryProfiles)+@([pscustomobject]@{
            image=$newImage;directory=$recoveryDir;manifestSha=$manifestHash
        })
        $policyTmp=$policy+'.pg.tmp'
        try{
            $pins|ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $policyTmp -Encoding UTF8
            Move-Item -LiteralPath $policyTmp -Destination $policy -Force
        }finally{if(Test-Path -LiteralPath $policyTmp){Remove-Item $policyTmp -Force}}
        RecordEvent 'canary_profile_pinned' 'PASS'
        WriteResult 'CANARY_PROFILE_PINNED_NOT_SWITCHED' $initialPid
        # One last ownership check before touching SCM.
        $fresh=Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'" -ErrorAction Stop
        if($fresh.State -ne 'Running' -or [int]$fresh.ProcessId -ne $initialPid -or
           [string]$fresh.PathName -cne $fromImage){throw 'Service owner changed before SCM switch'}
        CheckHash $enabled $ExpectedCandidateSha256 'profile at cutover'
        Import-Module ([string]$pins.recoveryModule) -Force -ErrorAction Stop
        $imageChanged=$true
        Set-GoServiceImage -ServiceName 'CodexifyGo' -ImagePath $newImage
        RecordEvent 'canary_scm_image_pinned' 'PASS'
        Stop-Service -Name 'CodexifyGo' -ErrorAction Stop
        (Get-Service -Name 'CodexifyGo').WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
        Start-Service -Name 'CodexifyGo' -ErrorAction Stop
        $newServicePid=WaitForNewGo -Image $newImage -Exe ([string]$pins.currentExecutable) -OldPid $initialPid -Seconds 90
        if($newServicePid -le 0){throw 'Project Groups local service readiness failed'}
        RecordEvent 'project_groups_service_ready' 'PASS'
        WriteResult 'PROJECT_GROUPS_ENABLED' $newServicePid
        $imageChanged=$false
        [pscustomobject]@{
            status='PROJECT_GROUPS_ENABLED';servicePid=$newServicePid
            goSha=$pins.currentSha;configSha=$ExpectedCandidateSha256.ToUpperInvariant()
            g11RecoveryArmed=$true;rustFallback=$false
        }|ConvertTo-Json -Depth 4
    }catch{
        try{RecordEvent 'canary_switch_failed' $_.Exception.GetType().Name}catch{}
        if($imageChanged){
            try {
                Import-Module ([string]$pins.recoveryModule) -Force -ErrorAction Stop
                Set-GoServiceImage -ServiceName 'CodexifyGo' -ImagePath ([string]$normal.image)
                $current=Get-Service -Name 'CodexifyGo'
                if($current.Status -ne 'Stopped'){
                    Stop-Service -Name 'CodexifyGo' -ErrorAction Stop
                    (Get-Service -Name 'CodexifyGo').WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
                }
                Start-Service -Name 'CodexifyGo' -ErrorAction Stop
                $restored=WaitForNewGo -Image ([string]$normal.image) -Exe ([string]$pins.currentExecutable) -OldPid $initialPid -Seconds 90
                if($restored -le 0){throw 'Normal Go could not be restored'}
                RecordEvent 'same_go_normal_profile_restored' 'PASS'
                WriteResult 'CANARY_FAILED_NORMAL_GO_RESTORED' $restored
                throw 'Project Groups canary failed; same Go normal profile restored'
            } catch {
                if($_.Exception.Message -eq 'Project Groups canary failed; same Go normal profile restored'){throw}
                try{RecordEvent 'same_go_restore_failed' $_.Exception.GetType().Name}catch{}
                try{WriteResult 'CANARY_FAILED_OPERATOR_INTERVENTION_REQUIRED'}catch{}
                throw 'Canary and same-Go rollback both failed; operator intervention required'
            }
        }
        try{WriteResult 'FAILED_BEFORE_SCM_CHANGE' $initialPid}catch{}
        throw
    }
} finally {
    if($acquired){$mutex.ReleaseMutex()}
    $mutex.Dispose()
}
