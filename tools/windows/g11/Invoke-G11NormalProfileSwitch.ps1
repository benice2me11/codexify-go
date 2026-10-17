# Independent, single-use Windows Task Scheduler operator for G11 diagnostics-off.
# Defaults to PLAN_ONLY. No Rust fallback, no second Go owner.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$PolicyFile,
    [Parameter(Mandatory)][string]$EvidenceDirectory,
    [Parameter(Mandatory)][string]$ExpectedPolicySha256,
    [int]$ExpectedServicePid=0,
    [switch]$Execute,
    [switch]$OperatorApproved
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest

function HashFile([string]$Path) {
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256 -ErrorAction Stop).Hash
}
function AssertProtected([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path -ErrorAction Stop
    foreach($entry in $acl.Access) {
        if($entry.AccessControlType -ne 'Allow'){continue}
        $sid=$entry.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        $danger=[Security.AccessControl.FileSystemRights]::Write -bor
            [Security.AccessControl.FileSystemRights]::Modify -bor
            [Security.AccessControl.FileSystemRights]::FullControl
        if(($entry.FileSystemRights -band $danger) -and
           $sid -notin @('S-1-5-18','S-1-5-32-544')) {
            throw 'Untrusted writer in G11 operator path'
        }
    }
}
function CheckHash([string]$Path,[string]$Expected,[string]$Description) {
    if(!(Test-Path -LiteralPath $Path -PathType Leaf) -or
       (HashFile $Path) -ne $Expected.ToUpperInvariant()) {
        throw ('SHA256 mismatch: '+$Description)
    }
}
function AuditTopology([int]$ScmPid,[string]$ExpectedExe) {
    $all=@(Get-CimInstance Win32_Process -ErrorAction Stop)
    $go=@($all|Where-Object{$_.Name -ieq 'codexify-go.exe'})
    $tunnel=@($all|Where-Object{$_.Name -ieq 'tunnel-client-runtime.exe'})
    $rust=@($all|Where-Object{$_.Name -ieq 'codexify.exe'})
    $super=@($go|Where-Object{[int]$_.ProcessId -eq $ScmPid -and
        [string]::Equals([string]$_.ExecutablePath,$ExpectedExe,[StringComparison]::OrdinalIgnoreCase)})
    $workers=@($go|Where-Object{[int]$_.ParentProcessId -eq $ScmPid})
    $ownedTunnel=@($tunnel|Where-Object{[int]$_.ParentProcessId -eq $ScmPid})
    return ($rust.Count -eq 0 -and $go.Count -eq 2 -and $tunnel.Count -eq 1 -and
        $super.Count -eq 1 -and $workers.Count -eq 1 -and $ownedTunnel.Count -eq 1)
}
function HealthOK {
    try {
        return ((Invoke-WebRequest -Uri 'http://127.0.0.1:3300/health' -UseBasicParsing -TimeoutSec 3 -ErrorAction Stop).StatusCode -eq 200)
    }catch{return $false}
}
function CheckReadiness([string]$Image,[string]$Executable,[int]$NotPid,[int]$Seconds=90) {
    for($tick=0;$tick -lt $Seconds;$tick++) {
        Start-Sleep -Seconds 1
        $svc=Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'" -ErrorAction SilentlyContinue
        if(!$svc -or $svc.State -ne 'Running' -or [int]$svc.ProcessId -le 0 -or
           [int]$svc.ProcessId -eq $NotPid -or [string]$svc.PathName -cne $Image){continue}
        if((AuditTopology -ScmPid ([int]$svc.ProcessId) -ExpectedExe $Executable) -and (HealthOK)){
            return [int]$svc.ProcessId
        }
    }
    return 0
}
function WriteEvent([string]$Event,[string]$Outcome) {
    [pscustomobject]@{timeUtc=[DateTimeOffset]::UtcNow.ToString('o');event=$Event;outcome=$Outcome}|
        ConvertTo-Json -Compress|Add-Content -LiteralPath $events -Encoding UTF8 -ErrorAction Stop
}
function WriteStatus([string]$Status,[int]$Pid=0) {
    $s=[pscustomobject]@{
        timeUtc=[DateTimeOffset]::UtcNow.ToString('o')
        status=$Status;serviceName='CodexifyGo'
        previousPid=$initialPid;currentPid=$Pid
        rustFallback=$false;projectGroupsEnabled=$false
    }
    $file=Join-Path $EvidenceDirectory 'normal-switch-result.json'
    $tmp=$file+'.tmp'
    try {
        $s|ConvertTo-Json -Depth 4|Set-Content -LiteralPath $tmp -Encoding UTF8 -ErrorAction Stop
        Move-Item -LiteralPath $tmp -Destination $file -Force -ErrorAction Stop
    } finally {
        if(Test-Path -LiteralPath $tmp){Remove-Item -LiteralPath $tmp -Force}
    }
}

if($Execute -and !$OperatorApproved){throw 'Execute requires separate operator authorization'}
if($ExpectedPolicySha256 -notmatch '^[A-Fa-f0-9]{64}$'){throw 'Expected policy SHA256 required'}
$policy=[IO.Path]::GetFullPath($PolicyFile)
$dir=[IO.Path]::GetFullPath($EvidenceDirectory)
if($Execute){
    $base=([IO.Path]::GetFullPath((Join-Path $env:ProgramData 'CodexifyGo\G11'))).TrimEnd('\')+'\'
    if(!$policy.StartsWith($base,[StringComparison]::OrdinalIgnoreCase) -or
       !$dir.StartsWith($base,[StringComparison]::OrdinalIgnoreCase) -or
       !$PSScriptRoot.StartsWith($base,[StringComparison]::OrdinalIgnoreCase)){
        throw 'Execute requires protected ProgramData operator deployment'
    }
    foreach($protected in @($PSScriptRoot,(Split-Path -Parent $policy),$dir)){AssertProtected $protected}
    $who=[Security.Principal.WindowsIdentity]::GetCurrent()
    $principal=New-Object Security.Principal.WindowsPrincipal($who)
    if(!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){
        throw 'Execute requires elevated SYSTEM or administrator'
    }
}
CheckHash $policy $ExpectedPolicySha256 'G11 runtime pins'
$pins=Get-Content -LiteralPath $policy -Raw -ErrorAction Stop|ConvertFrom-Json -ErrorAction Stop
if([int]$pins.version -ne 1 -or [string]$pins.serviceName -ne 'CodexifyGo'){throw 'Unrecognized G11 policy'}
if(@($pins.additionalCurrentProfiles).Count -ne 1 -or @($pins.recoveryProfiles).Count -ne 2){
    throw 'Expected exactly one additional current profile and two pinned recovery profiles'
}
$normal=$pins.additionalCurrentProfiles[0]
if([string]$normal.image -ceq [string]$pins.currentImage){throw 'Normal and diagnostic images are identical'}
if([string]$normal.config -ceq [string]$pins.currentConfig){throw 'Normal and diagnostic config files are identical'}
if(@($pins.recoveryProfiles|Where-Object{[string]$_.image -ceq [string]$normal.image}).Count -ne 1){
    throw 'Missing uniquely pinned normal recovery'
}
CheckHash ([string]$pins.currentExecutable) ([string]$pins.currentSha) 'current Go binary'
CheckHash ([string]$pins.previousExecutable) ([string]$pins.previousSha) 'previous Go binary'
CheckHash ([string]$pins.currentConfig) ([string]$pins.currentConfigSha) 'diagnostics-on config'
CheckHash ([string]$normal.config) ([string]$normal.sha) 'diagnostics-off config'
CheckHash ([string]$pins.recoveryScript) ([string]$pins.recoveryScriptSha) 'protected Go-only recovery script'
CheckHash ([string]$pins.recoveryModule) ([string]$pins.recoveryModuleSha) 'protected Go-only recovery module'
foreach($p in $pins.recoveryProfiles){
    CheckHash (Join-Path ([string]$p.directory) 'manifest.json') ([string]$p.manifestSha) 'protected recovery profile'
}
$before=Get-Content -LiteralPath $pins.currentConfig -Raw|ConvertFrom-Json
$after=Get-Content -LiteralPath $normal.config -Raw|ConvertFrom-Json
if($before.mcp.diagnostics.enabled -ne $true -or $before.mcp.projectGroupsEnabled -ne $false -or
   $after.mcp.diagnostics.enabled -ne $false -or $after.mcp.projectGroupsEnabled -ne $false){
    throw 'Unexpected diagnostics or Project Groups flags'
}
$before.mcp.diagnostics.enabled=$false
if(($before|ConvertTo-Json -Compress -Depth 100) -cne ($after|ConvertTo-Json -Compress -Depth 100)){
    throw 'Additional settings differ between profiles'
}
$scm=Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'" -ErrorAction Stop
if(!$scm -or $scm.State -ne 'Running' -or [int]$scm.ProcessId -le 0 -or
    [string]$scm.PathName -cne [string]$pins.currentImage){
    throw 'Expected current Go SCM ImagePath and Running service'
}
$initialPid=[int]$scm.ProcessId
if($ExpectedServicePid -gt 0 -and $initialPid -ne $ExpectedServicePid){
    throw 'Service PID changed since operator preflight'
}
if(!(AuditTopology -ScmPid $initialPid -ExpectedExe ([string]$pins.currentExecutable))){
    throw 'Unexpected Go, tunnel, or Rust process ownership'
}
foreach($name in @('Codexify','Codexify Watchdog')){
    $rustTask=Get-ScheduledTask -TaskName $name -ErrorAction Stop
    if([string]$rustTask.State -ne 'Disabled'){throw 'Rust fallback task is enabled'}
}
$g10=Get-ScheduledTask -TaskName 'CodexifyGo-G10Observers' -ErrorAction Stop
if([string]$g10.State -ne 'Disabled' -or [bool]$g10.Settings.Enabled){throw 'G10 recovery owner still active'}
$watchers=@(Get-CimInstance Win32_Process|Where-Object{
    $_.Name -in @('powershell.exe','pwsh.exe') -and $_.CommandLine -match 'Watch-GoTrial\.ps1'
})
if($watchers.Count -ne 0){throw 'Unexpected G10 watcher process'}
$g11=Get-ScheduledTask -TaskName 'CodexifyGo-G11SteadyState' -ErrorAction Stop
if([string]$g11.State -notin @('Ready','Running') -or
   [string]$g11.Principal.UserId -notin @('SYSTEM','S-1-5-18') -or
   @($g11.Actions).Count -ne 1 -or
   [string]$g11.Actions[0].Arguments -notmatch ' -RecoveryEnabled'){
    throw 'G11 is not sole recovery-armed task'
}
$other=@(Get-ScheduledTask|Where-Object{$_.TaskName -like 'CodexifyGo-G11*' -and $_.TaskName -ne 'CodexifyGo-G11SteadyState'})
if($other.Count -ne 0){throw 'Foreign G11 watchdog task'}
$armed=Join-Path (Split-Path -Parent $policy) 'evidence\recovery-armed.json'
if(!(Test-Path -LiteralPath $armed)){throw 'G11 approval marker missing'}
$marker=Get-Content -LiteralPath $armed -Raw|ConvertFrom-Json
if([int]$marker.version -ne 1 -or [string]$marker.manifestSha -ne [string]$pins.manifestSha){
    throw 'G11 recovery approval marker changed'
}
$taskInfo=Get-ScheduledTaskInfo -TaskName 'CodexifyGo-G11SteadyState'
$stateFile=Join-Path (Split-Path -Parent $policy) 'evidence\state.json'
$state=Get-Content -LiteralPath $stateFile -Raw|ConvertFrom-Json
if([int]$taskInfo.LastTaskResult -ne 0 -or $state.lastAction -ne 'CONTINUE' -or
   [DateTimeOffset]::Parse([string]$state.lastCheckUtc) -lt [DateTimeOffset]::UtcNow.AddMinutes(-3)){
    throw 'G11 monitor not currently healthy'
}
$scmPolicy=(& sc.exe qfailure CodexifyGo 2>&1|Out-String)
$delays=@([regex]::Matches($scmPolicy,'(?im)RESTART\s+--\s+Delay\s*=\s*(\d+)\s+milliseconds')|
    ForEach-Object{[int]$_.Groups[1].Value})
if($delays.Count -lt 3 -or $delays[0] -ne 5000 -or $delays[1] -ne 15000 -or
   $delays[2] -ne 60000 -or $scmPolicy -notmatch '(?im)RESET_PERIOD\s*\(in seconds\)\s*:\s*86400\b'){
    throw 'Existing 5/15/60 SCM recovery policy changed'
}
if(!(HealthOK)){throw 'Local worker health degraded'}
$plan=[pscustomobject]@{
    status=$(if($Execute){'READY_TO_SWITCH'}else{'PLAN_ONLY'})
    currentGoSha=[string]$pins.currentSha
    normalConfigSha=[string]$normal.sha
    previousGoSha=[string]$pins.previousSha
    servicePid=$initialPid
    normalProfileApproved=$true
    projectGroupsEnabled=$false
    g11RecoveryArmed=$true
    rustFallback=$false
    serviceMutation=$false
}
if(!$Execute){$plan|ConvertTo-Json -Depth 3;return}
$events=Join-Path $dir 'operator-events.jsonl'
if(!(Test-Path -LiteralPath $dir -PathType Container)){throw 'Operator evidence directory missing'}
$attempt=Join-Path $dir 'normal-switch-once.lock'
$guard=[IO.File]::Open($attempt,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
$guard.Dispose()
$switched=$false
try{
    WriteEvent 'preflight' 'PASS'
    WriteStatus 'STARTING' $initialPid
    # Last pre-mutation recheck prevents fighting an independently changed SCM.
    $fresh=Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'" -ErrorAction Stop
    if($fresh.State -ne 'Running' -or [int]$fresh.ProcessId -ne $initialPid -or
       [string]$fresh.PathName -cne [string]$pins.currentImage){throw 'SCM owner changed before switch'}
    Import-Module -Name ([string]$pins.recoveryModule) -Force -ErrorAction Stop
    $switched=$true # also rollback if ChangeServiceConfig succeeds but subsequent validation throws
    Set-GoServiceImage -ServiceName CodexifyGo -ImagePath ([string]$normal.image)
    WriteEvent 'scm_normal_image_pinned' 'PASS'
    WriteStatus 'SCM_NORMAL_CONFIG_SET' $initialPid
    Stop-Service -Name CodexifyGo -ErrorAction Stop
    (Get-Service -Name CodexifyGo).WaitForStatus('Stopped',[timespan]::FromSeconds(30))
    WriteEvent 'go_service_stopped' 'PASS'
    Start-Service -Name CodexifyGo -ErrorAction Stop
    $pid=CheckReadiness -Image ([string]$normal.image) -Executable ([string]$pins.currentExecutable) -NotPid $initialPid -Seconds 90
    if($pid -le 0){throw 'Normal-profile local readiness did not pass within 90 seconds'}
    WriteEvent 'new_profile_go_supervisor_worker_tunnel_healthy' 'PASS'
    WriteStatus 'GO_NORMAL_PROFILE_VERIFIED' $pid
    $switched=$false
    [pscustomobject]@{
        status='GO_NORMAL_PROFILE_VERIFIED';servicePid=$pid
        previousPid=$initialPid;goSha=[string]$pins.currentSha
        normalConfigSha=[string]$normal.sha
        g11RecoveryArmed=$true;projectGroupsEnabled=$false
        rustFallback=$false
    }|ConvertTo-Json -Depth 4
} catch {
    $kind=$_.Exception.GetType().Name
    try{WriteEvent 'normal_profile_switch_failed' $kind}catch{}
    if($switched){
        try{
            Import-Module -Name ([string]$pins.recoveryModule) -Force -ErrorAction Stop
            Set-GoServiceImage -ServiceName CodexifyGo -ImagePath ([string]$pins.currentImage)
            $current=Get-Service CodexifyGo
            if($current.Status -ne 'Stopped'){
                Stop-Service -Name CodexifyGo -ErrorAction Stop
                (Get-Service CodexifyGo).WaitForStatus('Stopped',[timespan]::FromSeconds(30))
            }
            Start-Service -Name CodexifyGo -ErrorAction Stop
            $rollbackPid=CheckReadiness -Image ([string]$pins.currentImage) -Executable ([string]$pins.currentExecutable) -NotPid $initialPid -Seconds 90
            if($rollbackPid -le 0){throw 'Original Go config did not recover local health'}
            WriteEvent 'original_go_profile_restored' 'PASS'
            WriteStatus 'NORMAL_PROFILE_FAILED_ORIGINAL_GO_RESTORED' $rollbackPid
            throw 'G11 normal-profile switch failed; original Go config restored'
        } catch {
            if($_.Exception.Message -eq 'G11 normal-profile switch failed; original Go config restored'){throw}
            try{WriteEvent 'original_go_profile_restore_failed' $_.Exception.GetType().Name}catch{}
            try{WriteStatus 'NORMAL_PROFILE_FAILED_OPERATOR_INTERVENTION_REQUIRED'}catch{}
            throw 'G11 normal-profile switch and original-Go-config restoration both failed; operator required'
        }
    }
    try{WriteStatus 'FAILED_BEFORE_SCM_CHANGE'}catch{}
    throw
}