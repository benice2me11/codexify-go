# One bounded read-only sample per Task Scheduler invocation. No looping.
# Requires protected installation under ProgramData, independent of ChatGPT MCP.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$PolicyFile,
    [Parameter(Mandatory)][string]$DataDirectory,
    [string]$HealthUrl = 'http://127.0.0.1:3300/health',
    [switch]$RecoveryEnabled,
    [switch]$Offline,
    [string]$FixtureSnapshotPath
)
$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'G11SteadyState.psm1') -Force

function Assert-ProtectedPath([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path -ErrorAction Stop
    foreach($entry in $acl.Access) {
        if($entry.AccessControlType -ne 'Allow') {continue}
        $id=$entry.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        $dangerous=[Security.AccessControl.FileSystemRights]::Write -bor
            [Security.AccessControl.FileSystemRights]::Modify -bor
            [Security.AccessControl.FileSystemRights]::FullControl
        if(($entry.FileSystemRights -band $dangerous) -and
            $id -notin @('S-1-5-18','S-1-5-32-544','S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464')) {
            throw 'Monitor directory has a non-privileged writer'
        }
    }
}

function Get-Hash([string]$Path) {
    if(!(Test-Path -LiteralPath $Path -PathType Leaf)) {return ''}
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256 -ErrorAction Stop).Hash
}

function Read-G11Snapshot($Pins,[string]$Uri) {
    $service=Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'" -ErrorAction Stop
    if(!$service){throw 'Go SCM service not found'}
    $all=@(Get-CimInstance Win32_Process -ErrorAction Stop)
    $go=@($all | Where-Object {$_.Name -ieq 'codexify-go.exe'})
    $tunnels=@($all | Where-Object {$_.Name -ieq 'tunnel-client-runtime.exe'})
    $super=@($go | Where-Object {[int]$_.ProcessId -eq [int]$service.ProcessId -and [int]$service.ProcessId -gt 0})
    $workers=@($go | Where-Object {[int]$_.ParentProcessId -eq [int]$service.ProcessId -and [int]$service.ProcessId -gt 0})
    $ownedTunnel=@($tunnels | Where-Object {[int]$_.ParentProcessId -eq [int]$service.ProcessId -and [int]$service.ProcessId -gt 0})
    $watchdogProcs=@($all | Where-Object {
        $_.Name -in @('powershell.exe','pwsh.exe') -and
        $_.CommandLine -match 'Watch-GoTrial\.ps1'
    })
    $g10=@($watchdogProcs |Where-Object {
        $_.CommandLine -match [regex]::Escape([string]$Pins.transactionDirectory)
    })
    $rust=@($all | Where-Object {$_.Name -ieq 'codexify.exe'})
    $rustDisabled=$true
    foreach($name in @('Codexify','Codexify Watchdog')){
        $task=Get-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue
        if($task -and [string]$task.State -ne 'Disabled'){$rustDisabled=$false}
    }
    $g10Task=Get-ScheduledTask -TaskName 'CodexifyGo-G10Observers' -ErrorAction SilentlyContinue
    $g10Enabled=($null -ne $g10Task -and [string]$g10Task.State -ne 'Disabled')
    $otherG11=@(Get-ScheduledTask -ErrorAction Stop |Where-Object {
        $_.TaskName -like 'CodexifyGo-G11*' -and
        $_.TaskName -ne 'CodexifyGo-G11SteadyState' -and
        [string]$_.State -ne 'Disabled'
    })
    $g10Pinned=(!$g10Enabled -or (
        @($g10Task.Actions).Count -eq 1 -and
        [string]$g10Task.Actions[0].Arguments -match [regex]::Escape([string]$Pins.transactionDirectory) -and
        [string]$g10Task.Actions[0].Arguments -match 'ArmGoRecovery'
    ))
    $profile='foreign'
    $profiles=@([pscustomobject]@{
        image=[string]$Pins.currentImage
        config=[string]$Pins.currentConfig
        sha=[string]$Pins.currentConfigSha
    })
    if($Pins.PSObject.Properties['additionalCurrentProfiles']){
        $profiles+=@($Pins.additionalCurrentProfiles)
    }
    if((Get-Hash $Pins.currentExecutable) -eq [string]$Pins.currentSha){
        foreach($approved in $profiles){
            if([string]$service.PathName -eq [string]$approved.image -and
                (Get-Hash $approved.config) -eq [string]$approved.sha){
                $profile='current'
                break
            }
        }
    }
    if($profile -eq 'foreign' -and [string]$service.PathName -eq [string]$Pins.previousImage -and
        (Get-Hash $Pins.previousExecutable) -eq [string]$Pins.previousSha -and
        (Get-Hash $Pins.previousConfig) -eq [string]$Pins.previousConfigSha){
        $profile='previous'
    }
    $rightExe=if($profile -eq 'previous'){$Pins.previousExecutable}else{$Pins.currentExecutable}
    $unexpectedGo=@($go|Where-Object {
        ($_.ExecutablePath -ne $rightExe) -or
        ([int]$_.ProcessId -ne [int]$service.ProcessId -and [int]$_.ParentProcessId -ne [int]$service.ProcessId)
    })
    $health='unavailable'
    if($service.State -eq 'Running' -and $Uri -match '^http://(127\.0\.0\.1|localhost):[0-9]+/health$'){
        try{
            $response=Invoke-WebRequest -Uri $Uri -UseBasicParsing -TimeoutSec 3 -ErrorAction Stop
            $health=if([int]$response.StatusCode -eq 200){'healthy'}else{'unhealthy'}
        }catch{$health='unhealthy'}
    }
    $scmText=(& sc.exe qfailure CodexifyGo 2>&1|Out-String)
    $restarts=Test-G11SCMRecoveryPolicy -Text $scmText
    return [pscustomobject]@{
        ProbeError='';ServiceState=[string]$service.State;ImageKind=$profile
        RustCount=$rust.Count;RustTasksDisabled=$rustDisabled;GoCount=$go.Count
        UnexpectedGoCount=$unexpectedGo.Count;UnexpectedTunnelCount=$tunnels.Count-$ownedTunnel.Count
        SupervisorCount=$super.Count;WorkerCount=$workers.Count;TunnelCount=$ownedTunnel.Count
        G10TaskEnabled=$g10Enabled;G10WatcherCount=$g10.Count
        G10TaskUnpinned=(!$g10Pinned);UnexpectedWatchdogCount=$watchdogProcs.Count-$g10.Count+$otherG11.Count
        ScmRecoveryValid=($restarts -and $service.StartMode -eq 'Auto')
        RecoveryAssetsValid=((Get-Hash $Pins.previousExecutable) -eq [string]$Pins.previousSha)
        Health=$health
    }
}

function Write-G11Event([string]$File,[string]$Action,[string]$Reason) {
    # At most 8 MiB retained, no command line, token or payload material.
    if((Test-Path -LiteralPath $File) -and (Get-Item -LiteralPath $File).Length -ge 2097152){
        for($i=3;$i -ge 1;$i--){
            $from="$File.$i";$to="$File."+($i+1)
            if(Test-Path -LiteralPath $from){Move-Item -LiteralPath $from -Destination $to -Force}
        }
        Move-Item -LiteralPath $File -Destination "$File.1" -Force
        if(Test-Path -LiteralPath "$File.4"){Remove-Item -LiteralPath "$File.4" -Force}
    }
    $record=[pscustomobject]@{
        timeUtc=[DateTimeOffset]::UtcNow.ToString('o')
        action=$Action;reason=$Reason
    }
    $record|ConvertTo-Json -Compress|Add-Content -LiteralPath $File -Encoding UTF8 -ErrorAction Stop
}

function Write-G11State([string]$Path,$State) {
    $temp=$Path+'.'+[guid]::NewGuid().ToString('N')+'.tmp'
    try {
        $State|ConvertTo-Json -Depth 5|Set-Content -LiteralPath $temp -Encoding UTF8 -ErrorAction Stop
        Move-Item -LiteralPath $temp -Destination $Path -Force -ErrorAction Stop
    } finally {
        if(Test-Path -LiteralPath $temp){Remove-Item -LiteralPath $temp -Force}
    }
}

function Write-G11OperatorAlert([string]$Action,[string]$Reason) {
    # Optional native Windows Event Log warning on a new incident. Never fail
    # closed observation solely because Event Log submission is unavailable;
    # the protected JSONL state remains authoritative.
    if($Action -eq 'CONTINUE'){return}
    $id=if($Action -eq 'BLOCKED'){611}elseif($Action -eq 'GO_PREVIOUS_RECOVERED'){613}else{612}
    try{
        & eventcreate.exe /L Application /T WARNING /SO CodexifyGo-G11 /ID $id /D ('G11 '+$Action+': '+$Reason) | Out-Null
    }catch{}
}

# Offline fixture invocations must not contend with the live SYSTEM monitor's
# Global mutex (which can have a more restrictive security descriptor). Keep
# the one stable global name exclusively for real scheduled invocations.
$mutexName=if($Offline){'Local\CodexifyG11Offline-'+[guid]::NewGuid().ToString('N')}
           else{'Global\CodexifyGo.G11SteadyState'}
$mutex=Enter-G11Mutex -Name $mutexName
if($null -eq $mutex){
    [pscustomobject]@{action='DUPLICATE_SKIPPED';mutatedService=$false}|ConvertTo-Json -Compress
    return
}
try {
    $dir=[IO.Path]::GetFullPath($DataDirectory)
    $file=[IO.Path]::GetFullPath($PolicyFile)
    if(!(Test-Path -LiteralPath $dir -PathType Container)){throw 'Missing G11 monitor evidence directory'}
    if(!$Offline) {
        Assert-ProtectedPath $PSScriptRoot
        Assert-ProtectedPath $dir
        Assert-ProtectedPath (Split-Path -Parent $file)
    }
    $pins=Get-Content -LiteralPath $file -Raw -ErrorAction Stop|ConvertFrom-Json
    if([int]$pins.version -ne 1 -or [string]$pins.serviceName -ne 'CodexifyGo'){
        throw 'Unsupported or foreign G11 runtime policy'
    }
    if(!$Offline -and (Get-Hash $pins.transactionManifest) -ne [string]$pins.manifestSha){
        throw 'Immutable Go-only recovery manifest SHA mismatch'
    }
    if($HealthUrl -notmatch '^http://(localhost|127\.0\.0\.1):[0-9]+/health$') {
        throw 'Health URL must be a fixed HTTP loopback endpoint'
    }
    $stateFile=Join-Path $dir 'state.json'
    $eventsFile=Join-Path $dir 'events.jsonl'
    $old=$null
    if(Test-Path -LiteralPath $stateFile){
        $old=Get-Content -LiteralPath $stateFile -Raw|ConvertFrom-Json
    }
    if($Offline) {
        if(!$FixtureSnapshotPath){throw 'Offline mode requires a local fixture snapshot'}
        $snap=Get-Content -LiteralPath $FixtureSnapshotPath -Raw|ConvertFrom-Json
    } else {
        $snap=Read-G11Snapshot -Pins $pins -Uri $HealthUrl
    }
    $armedFile=Join-Path $dir 'recovery-armed.json'
    $armed=$false
    if($RecoveryEnabled -and !$Offline) {
        if(!(Test-Path -LiteralPath $armedFile -PathType Leaf)){throw 'Recovery not operator-armed'}
        $arm=Get-Content -LiteralPath $armedFile -Raw|ConvertFrom-Json
        if($arm.version -ne 1 -or $arm.manifestSha -ne $pins.manifestSha) {
            throw 'Recovery approval does not match pinned manifest'
        }
        $armed=$true
    }
    $decision=Get-G11Decision -Snapshot $snap -PreviousState $old -RecoveryEnabled:$armed
    $next=$decision.nextState
    # A recovery attempt is a one-shot operation, even across task restarts.
    $attemptUtc=$null
    if($old -and $old.PSObject.Properties['recoveryAttemptedUtc']){
        $attemptUtc=$old.recoveryAttemptedUtc
    }
    if($attemptUtc -and $decision.action -eq 'RECOVER_GO'){
        $decision.action='HOLD'
        $decision.reason='recovery_already_attempted_operator_required'
        $decision.recoveryAllowed=$false
        $next.lastAction=$decision.action
    }
    $next|Add-Member -NotePropertyName recoveryAttemptedUtc -NotePropertyValue $attemptUtc
    $previousAction=if($old){[string]$old.lastAction}else{''}
    $previousReason=if($old){[string]$old.reportedReason}else{''}
    $previousHeartbeat=if($old -and $old.PSObject.Properties['heartbeatUtc']){[string]$old.heartbeatUtc}else{''}
    $heartbeatDue=$true
    if($previousHeartbeat) {
        try {$heartbeatDue=([DateTimeOffset]::UtcNow-[DateTimeOffset]::Parse($previousHeartbeat)).TotalMinutes -ge 15}
        catch{$heartbeatDue=$true}
    }
    $next|Add-Member -NotePropertyName reportedReason -NotePropertyValue $decision.reason
    $next|Add-Member -NotePropertyName heartbeatUtc -NotePropertyValue $(if($heartbeatDue){[DateTimeOffset]::UtcNow.ToString('o')}else{$previousHeartbeat})
    if($previousAction -ne $decision.action -or $previousReason -ne $decision.reason -or $heartbeatDue){
        Write-G11Event -File $eventsFile -Action $decision.action -Reason $decision.reason
        if(!$Offline -and ($previousAction -ne $decision.action -or $previousReason -ne $decision.reason)){
            Write-G11OperatorAlert -Action $decision.action -Reason $decision.reason
        }
    }
    # Persistent state/evidence MUST be writable before any recovery action.
    Write-G11State -Path $stateFile -State $next
    $serviceMutated=$false
    if(!$Offline -and $decision.action -eq 'RECOVER_GO' -and $decision.recoveryAllowed){
        $handoverArgs=@{
            G10TaskDisabled=(!$snap.G10TaskEnabled)
            G10WatcherCount=[int]$snap.G10WatcherCount
            RustTasksDisabled=[bool]$snap.RustTasksDisabled
            OtherG11TaskCount=0
            AssetsVerified=([bool]$snap.RecoveryAssetsValid -and
                (Get-Hash $pins.recoveryScript) -eq [string]$pins.recoveryScriptSha -and
                (Get-Hash $pins.recoveryModule) -eq [string]$pins.recoveryModuleSha)
            ProgramDataProtected=$true
        }
        $handover=Test-G11RecoveryHandover @handoverArgs
        if(!$handover.ready){throw ('Recovery blocked at execution: '+($handover.reasons -join ','))}
        # Re-read SCM/processes after policy evaluation to avoid fighting an
        # SCM restart that completed between the two observations.
        $again=Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'" -ErrorAction Stop
        $goNow=@(Get-CimInstance Win32_Process |Where-Object {
            $_.Name -in @('codexify-go.exe','tunnel-client-runtime.exe','codexify.exe')
        })
        $newG10=Get-ScheduledTask -TaskName 'CodexifyGo-G10Observers' -ErrorAction SilentlyContinue
        # The frozen operator-v6 manifest only accepts its original
        # diagnostic-enabled ImagePath. Each explicitly approved G11 config
        # instead has its own protected, SHA-pinned Go-only v1 recovery
        # manifest. This prevents a diagnostics-off update from silently
        # invalidating rollback or modifying the historical v6 transaction.
        $recoveryProfile=Get-G11RecoveryProfile -Profiles $pins.recoveryProfiles -SCMImage ([string]$again.PathName)
        $recoveryManifestPath=Join-Path ([string]$recoveryProfile.directory) 'manifest.json'
        Assert-ProtectedPath ([string]$recoveryProfile.directory)
        if((Get-Hash $recoveryManifestPath) -ne [string]$recoveryProfile.manifestSha){
            throw 'Protected G11 recovery manifest SHA mismatch'
        }
        $recoveryManifest=Get-Content -LiteralPath $recoveryManifestPath -Raw|ConvertFrom-Json
        if([int]$recoveryManifest.version -ne 1 -or
            [string]$recoveryManifest.serviceName -ne 'CodexifyGo' -or
            [string]$recoveryManifest.newImage -cne [string]$again.PathName -or
            [string]$recoveryManifest.oldExe -cne [string]$pins.previousExecutable -or
            [string]$recoveryManifest.oldSha -cne [string]$pins.previousSha -or
            [string]$recoveryManifest.originalImage -cne [string]$pins.previousImage){
            throw 'G11 recovery manifest does not match approved previous-Go identity'
        }
        if(($again.State -eq 'Running' -and [int]$again.ProcessId -gt 0) -or
            $goNow.Count -gt 0 -or
            ($newG10 -and [string]$newG10.State -ne 'Disabled')){
            Write-G11Event -File $eventsFile -Action 'HOLD' -Reason 'recovery_cancelled_race_or_foreign_owner'
            $decision.action='HOLD'
            $decision.reason='recovery_cancelled_race_or_foreign_owner'
            $decision.recoveryAllowed=$false
            $next.lastAction=$decision.action
            $next.reportedReason=$decision.reason
            Write-G11State -Path $stateFile -State $next
        }else{
            # Persist a single-use attempt marker before any service mutation.
            # On failure, the next invocation must alert, not start a loop.
            $next.recoveryAttemptedUtc=[DateTimeOffset]::UtcNow.ToString('o')
            Write-G11Event -File $eventsFile -Action 'RECOVER_GO_STARTED' -Reason $decision.reason
            Write-G11State -Path $stateFile -State $next
            & $pins.recoveryScript -TransactionDirectory $recoveryProfile.directory -Reason 'go_supervisor_unavailable' -Execute | Out-Null
            if($LASTEXITCODE -and $LASTEXITCODE -ne 0){throw 'Go-only recovery process returned a nonzero exit code'}
            $serviceMutated=$true
            $decision.action='RECOVER_GO_INVOKED'
            Write-G11Event -File $eventsFile -Action $decision.action -Reason 'awaiting_previous_go_health'
        }
    }
    [pscustomobject]@{
        action=$decision.action;reason=$decision.reason
        consecutiveFailures=$decision.consecutiveFailures;recoveryAllowed=$decision.recoveryAllowed
        mutatedService=$serviceMutated
    }|ConvertTo-Json -Compress
} finally {
    Exit-G11Mutex $mutex
}
