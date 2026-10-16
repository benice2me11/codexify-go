# Install/inspect/remove one scheduled G11 monitor. Default is PLAN_ONLY.
# Code is copied to an ACL-restricted ProgramData directory before SYSTEM
# may run it; never schedule PowerShell source from a user-writable checkout.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TransactionDirectory,
    [ValidateSet('PlanInstall','InstallReadOnly','Inspect','ArmRecovery','ApproveNormalProfile','Remove')]
    [string]$Action='PlanInstall',
    [string]$InstallDirectory='',
    [string]$ApprovedConfigPath='',
    [string]$ApprovedConfigSha256='',
    [switch]$Execute,
    [switch]$OperatorApproved,
    [switch]$SuccessorVerified
)
$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'G11SteadyState.psm1') -Force
$root=[IO.Path]::GetFullPath($TransactionDirectory)
$manifestFile=Join-Path $root 'manifest.json'
if(!(Test-Path -LiteralPath $manifestFile -PathType Leaf)){throw 'Frozen v6 manifest missing'}
$m=Get-Content -LiteralPath $manifestFile -Raw|ConvertFrom-Json -ErrorAction Stop
if($m.version -ne 1 -or $m.transactionKind -ne 'go_only_watch_go_trial_to_go_only' -or
    $m.serviceName -ne 'CodexifyGo' -or [bool]$m.autoRustFallback){
    throw 'Expected Go-only operator v6 manifest, no Rust fallback'
}
if(!$InstallDirectory){$InstallDirectory=Join-Path $env:ProgramData 'CodexifyGo\G11'}
$dest=[IO.Path]::GetFullPath($InstallDirectory)
$programData=([IO.Path]::GetFullPath($env:ProgramData)).TrimEnd('\')+'\'
if(!$dest.StartsWith($programData,[StringComparison]::OrdinalIgnoreCase)){
    throw 'Install location must be under ProgramData'
}
$taskName='CodexifyGo-G11SteadyState'
$bin=Join-Path $dest 'bin'
$events=Join-Path $dest 'evidence'
$recoveryBase=Join-Path $dest 'recovery'
$pinsFile=Join-Path $dest 'runtime-pins.json'
$manifestSha=(Get-FileHash -LiteralPath $manifestFile -Algorithm SHA256).Hash
$oldSha=(Get-FileHash -LiteralPath $m.oldExe -Algorithm SHA256 -ErrorAction Stop).Hash
$newSha=(Get-FileHash -LiteralPath $m.newExe -Algorithm SHA256 -ErrorAction Stop).Hash
$oldConfigSha=(Get-FileHash -LiteralPath $m.oldConfigPath -Algorithm SHA256 -ErrorAction Stop).Hash
$newConfigSha=(Get-FileHash -LiteralPath $m.configPath -Algorithm SHA256 -ErrorAction Stop).Hash
if($oldSha -ne $m.oldSha -or $newSha -ne $m.newSha -or
    $oldConfigSha -ne $m.oldConfigSha -or $newConfigSha -ne $m.configSha){
    throw 'Frozen recovery or current Go binary/config SHA mismatch'
}
$existing=Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
$other=@(Get-ScheduledTask -ErrorAction Stop | Where-Object {
    $_.TaskName -like 'CodexifyGo-G11*' -and $_.TaskName -ne $taskName
})
$g10=Get-ScheduledTask -TaskName $m.g10TaskName -ErrorAction SilentlyContinue
$go10Watchers=@(Get-CimInstance Win32_Process |Where-Object {
    $_.Name -in @('powershell.exe','pwsh.exe') -and $_.CommandLine -match 'Watch-GoTrial\.ps1'
})
$rustDisabled=$true
foreach($name in @('Codexify','Codexify Watchdog')){
    $t=Get-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue
    if($t -and [string]$t.State -ne 'Disabled'){$rustDisabled=$false}
}
$spec=[ordered]@{
    status='PLAN_ONLY';action=$Action;taskName=$taskName
    taskState=$(if($existing){[string]$existing.State}else{'not_installed'})
    repeatMinutes=1;bootTrigger=$true;multipleInstances='IgnoreNew'
    serviceMutation=$false;rustFallback=$false
    manifestSha256=$manifestSha;newGoSha256=$newSha;previousGoSha256=$oldSha
    g10Enabled=($null -ne $g10 -and [string]$g10.State -ne 'Disabled')
    g10Watchers=$go10Watchers.Count;otherG11Tasks=$other.Count
    operatorApprovalRequired=$true
}
if($Action -eq 'Inspect'){
    $spec.status='INSPECT_ONLY'
    $spec.recoveryArmed=(Test-Path -LiteralPath (Join-Path $events 'recovery-armed.json'))
    $spec.lastStatePresent=(Test-Path -LiteralPath (Join-Path $events 'state.json'))
    $spec|ConvertTo-Json -Depth 5
    return
}
if(!$Execute -or $Action -eq 'PlanInstall'){
    $spec|ConvertTo-Json -Depth 5
    return
}
$installMutex=Enter-G11Mutex -Name 'Global\CodexifyGo.G11Installer'
if($null -eq $installMutex){throw 'Another G11 install/handover operation is already active'}
try {
if(!$OperatorApproved){throw 'Mutation requires -Execute and -OperatorApproved'}
$principal=[Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
if(!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){
    throw 'Installer requires elevated Windows administrator'
}
if($other.Count -ne 0 -or !$rustDisabled){throw 'Conflicting G11 task or Rust scheduled task'}
$taskPowerShell=Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
$runner=Join-Path $bin 'Invoke-G11SteadyState.ps1'
$argsBase='-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$runner+
    '" -PolicyFile "'+$pinsFile+'" -DataDirectory "'+$events+'"'

function Assert-ProtectedDirectory([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path -ErrorAction Stop
    foreach($entry in $acl.Access){
        if($entry.AccessControlType -ne 'Allow'){continue}
        $sid=$entry.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        $danger=[Security.AccessControl.FileSystemRights]::Write -bor
            [Security.AccessControl.FileSystemRights]::Modify -bor
            [Security.AccessControl.FileSystemRights]::FullControl
        if(($entry.FileSystemRights -band $danger) -and $sid -notin @('S-1-5-18','S-1-5-32-544')){
            throw 'Unsafe writable monitor executable or evidence directory'
        }
    }
}

if($Action -eq 'InstallReadOnly'){
    if($existing){throw 'Refusing to overwrite existing G11 task'}
    if(Test-Path -LiteralPath $dest){throw 'Refusing to reuse existing G11 deployment directory'}
    if(!$spec.g10Enabled -or $go10Watchers.Count -ne 1){
        throw 'Observe-only deployment requires a single still-active G10 owner'
    }
    $currentRecovery=Join-Path $recoveryBase 'current'
    $currentRecoveryEvents=Join-Path $currentRecovery 'evidence'
    New-Item -ItemType Directory -Path $bin,$events,$currentRecoveryEvents -Force -ErrorAction Stop|Out-Null
    # A SYSTEM task must never execute scripts left writable by standard users.
    foreach($folder in @($dest,$bin,$events,$recoveryBase,$currentRecovery,$currentRecoveryEvents)){
        & icacls.exe $folder '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
        if($LASTEXITCODE -ne 0){throw 'Failed to protect G11 directory ACL'}
        Assert-ProtectedDirectory $folder
    }
    foreach($filename in @('G11SteadyState.psm1','Invoke-G11SteadyState.ps1')){
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $filename) -Destination (Join-Path $bin $filename) -ErrorAction Stop
    }
    # Freeze previous-Go-only recovery code into the protected installation.
    # The future SYSTEM task must not execute mutable repository/staging code.
    foreach($filename in @('Invoke-GoOnlyRecovery.ps1','GoOnlyUpgrade.psm1')){
        $source=Join-Path $root ('operator\'+$filename)
        $target=Join-Path $bin $filename
        $expectedHash=(Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash
        Copy-Item -LiteralPath $source -Destination $target -ErrorAction Stop
        if((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ne $expectedHash){
            throw 'Protected recovery script copy failed hash verification'
        }
    }
    # A dedicated minimal v1 manifest pins the same old Go as operator-v6 but
    # permits G11 to select the matching currently approved config. Never
    # rewrite the frozen v6 transaction, its evidence or its original manifest.
    $currentRecoveryManifest=Join-Path $currentRecovery 'manifest.json'
    [ordered]@{
        version=1;serviceName='CodexifyGo';g10TaskName=[string]$m.g10TaskName
        oldExe=[string]$m.oldExe;oldSha=$oldSha
        newExe=[string]$m.newExe;newImage=[string]$m.newImage
        originalImage=[string]$m.originalImage
    }|ConvertTo-Json -Depth 5|Set-Content -LiteralPath $currentRecoveryManifest -Encoding UTF8 -ErrorAction Stop
    $currentRecoverySha=(Get-FileHash -LiteralPath $currentRecoveryManifest -Algorithm SHA256).Hash
    $pins=[ordered]@{
        version=1;serviceName='CodexifyGo'
        transactionManifest=$manifestFile;manifestSha=$manifestSha
        transactionDirectory=$root
        currentExecutable=[string]$m.newExe;currentSha=$newSha
        currentImage=[string]$m.newImage
        currentConfig=[string]$m.configPath;currentConfigSha=$newConfigSha
        previousExecutable=[string]$m.oldExe;previousSha=$oldSha
        previousImage=[string]$m.originalImage
        previousConfig=[string]$m.oldConfigPath;previousConfigSha=$oldConfigSha
        recoveryScript=Join-Path $bin 'Invoke-GoOnlyRecovery.ps1'
        recoveryScriptSha=(Get-FileHash -LiteralPath (Join-Path $root 'operator\Invoke-GoOnlyRecovery.ps1') -Algorithm SHA256).Hash
        recoveryModule=Join-Path $bin 'GoOnlyUpgrade.psm1'
        recoveryModuleSha=(Get-FileHash -LiteralPath (Join-Path $root 'operator\GoOnlyUpgrade.psm1') -Algorithm SHA256).Hash
        recoveryProfiles=@([ordered]@{
            image=[string]$m.newImage
            directory=$currentRecovery
            manifestSha=$currentRecoverySha
        })
    }
    $pins|ConvertTo-Json -Depth 6|Set-Content -LiteralPath $pinsFile -Encoding UTF8 -ErrorAction Stop
    $triggers=@(
        (New-ScheduledTaskTrigger -AtStartup),
        (New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 1) -RepetitionDuration (New-TimeSpan -Days 3650))
    )
    $actionObj=New-ScheduledTaskAction -Execute $taskPowerShell -Argument $argsBase
    $taskPrincipal=New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
    $settings=New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Minutes 4) -MultipleInstances IgnoreNew -StartWhenAvailable
    $definition=New-ScheduledTask -Action $actionObj -Trigger $triggers -Principal $taskPrincipal -Settings $settings
    $created=$false
    try{
        Register-ScheduledTask -TaskName $taskName -InputObject $definition -ErrorAction Stop|Out-Null
        $created=$true
        $registered=Get-ScheduledTask -TaskName $taskName -ErrorAction Stop
        if(@($registered.Triggers).Count -ne 2 -or
            [string]$registered.Settings.MultipleInstances -ne 'IgnoreNew' -or
            [string]$registered.Principal.UserId -notin @('SYSTEM','S-1-5-18')){
            throw 'Registered G11 task differs from the audited plan'
        }
    } catch {
        if($created -and (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue)) {
            Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction Stop
        }
        throw
    }
    $spec.status='INSTALLED_OBSERVE_ONLY'
    $spec|ConvertTo-Json -Depth 5
    return
}

if(!$existing){throw 'Cannot mutate a G11 task that is not installed'}
if([string]$existing.State -eq 'Running'){throw 'Refusing to mutate task while it is running'}
Assert-ProtectedDirectory $dest
Assert-ProtectedDirectory $bin
Assert-ProtectedDirectory $events
if(!(Test-Path -LiteralPath $pinsFile)){throw 'Missing protected G11 policy'}
$installedPins=Get-Content -LiteralPath $pinsFile -Raw|ConvertFrom-Json
if([string]$installedPins.manifestSha -ne $manifestSha){throw 'Installed policy no longer matches immutable v6 manifest'}

if($Action -eq 'ApproveNormalProfile'){
    if(!$ApprovedConfigPath -or $ApprovedConfigSha256 -notmatch '^[a-fA-F0-9]{64}$'){
        throw 'Approved normal config path and pinned SHA256 are required'
    }
    $candidate=[IO.Path]::GetFullPath($ApprovedConfigPath)
    $private=([IO.Path]::GetFullPath((Join-Path $env:USERPROFILE '.codexify-go\staging'))).TrimEnd('\')+'\'
    if(!$candidate.StartsWith($private,[StringComparison]::OrdinalIgnoreCase)){
        throw 'Additional normal config must be a private versioned staged file'
    }
    if((Get-FileHash -LiteralPath $candidate -Algorithm SHA256).Hash -ne $ApprovedConfigSha256.ToUpperInvariant()){
        throw 'Unapproved normal config SHA256'
    }
    $before=[IO.File]::ReadAllBytes([string]$installedPins.currentConfig)
    $after=[IO.File]::ReadAllBytes($candidate)
    $encoding=New-Object Text.UTF8Encoding($false,$true)
    $raw=$encoding.GetString($before)
    $match=[regex]::Matches($raw,'(?<prefix>"diagnostics"\s*:\s*\{\s*"enabled"\s*:\s*)true\b')
    if($match.Count -ne 1){throw 'Ambiguous current diagnostics token'}
    $start=$match[0].Index+$match[0].Groups['prefix'].Length
    $byteStart=$encoding.GetByteCount($raw.Substring(0,$start))
    $expected=New-Object byte[] ($before.Length+1)
    [Array]::Copy($before,0,$expected,0,$byteStart)
    [Array]::Copy($encoding.GetBytes('false'),0,$expected,$byteStart,5)
    [Array]::Copy($before,$byteStart+4,$expected,$byteStart+5,$before.Length-$byteStart-4)
    $digest=[Security.Cryptography.SHA256]::Create()
    try{$exactHash=[BitConverter]::ToString($digest.ComputeHash($expected)).Replace('-','')}
    finally{$digest.Dispose()}
    if($after.Length -ne $expected.Length -or $exactHash -ne $ApprovedConfigSha256.ToUpperInvariant()){
        throw 'Normal profile must differ by exactly the diagnostics boolean'
    }
    $baseImage=[string]$installedPins.currentImage
    $from=[string]$installedPins.currentConfig
    if($baseImage.Split([string[]]@($from),[StringSplitOptions]::None).Count -ne 2){
        throw 'Ambiguous SCM image config parameter'
    }
    $newImage=$baseImage.Replace($from,$candidate)
    if(!$installedPins.PSObject.Properties['additionalCurrentProfiles']){
        $installedPins|Add-Member -NotePropertyName additionalCurrentProfiles -NotePropertyValue @()
    }
    if(!$installedPins.PSObject.Properties['recoveryProfiles'] -or
        @($installedPins.recoveryProfiles).Count -ne 1){
        throw 'Missing initial G11 pinned recovery profile'
    }
    if(@($installedPins.additionalCurrentProfiles).Count -ne 0){
        throw 'Additional profile already installed; explicit separate revision required'
    }
    $backup=Join-Path $events 'runtime-pins-before-normal-profile.json'
    if(Test-Path -LiteralPath $backup){throw 'Profile rollback copy already frozen'}
    $sourceRecovery=$installedPins.recoveryProfiles[0]
    $sourceRecoveryManifest=Join-Path $sourceRecovery.directory 'manifest.json'
    if((Get-FileHash -LiteralPath $sourceRecoveryManifest -Algorithm SHA256).Hash -ne [string]$sourceRecovery.manifestSha){
        throw 'Existing protected Go-only recovery manifest was modified'
    }
    $normalRecovery=Join-Path $recoveryBase 'normal'
    if(Test-Path -LiteralPath $normalRecovery){throw 'Normal recovery profile already staged'}
    $normalRecoveryEvents=Join-Path $normalRecovery 'evidence'
    New-Item -ItemType Directory -Path $normalRecoveryEvents -Force -ErrorAction Stop|Out-Null
    foreach($folder in @($normalRecovery,$normalRecoveryEvents)){
        & icacls.exe $folder '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
        if($LASTEXITCODE -ne 0){throw 'Failed to protect normal recovery profile'}
        Assert-ProtectedDirectory $folder
    }
    $normalManifest=Get-Content -LiteralPath $sourceRecoveryManifest -Raw|ConvertFrom-Json
    if([string]$normalManifest.newImage -ne $baseImage){throw 'Recovery source image did not match current pinned image'}
    $normalManifest.newImage=$newImage
    $normalManifestPath=Join-Path $normalRecovery 'manifest.json'
    $normalManifest|ConvertTo-Json -Depth 5|Set-Content -LiteralPath $normalManifestPath -Encoding UTF8 -ErrorAction Stop
    $normalManifestSha=(Get-FileHash -LiteralPath $normalManifestPath -Algorithm SHA256).Hash
    Copy-Item -LiteralPath $pinsFile -Destination $backup -ErrorAction Stop
    $installedPins.additionalCurrentProfiles=@([pscustomobject]@{
        image=$newImage;config=$candidate;sha=$ApprovedConfigSha256.ToUpperInvariant()
    })
    $installedPins.recoveryProfiles=@($installedPins.recoveryProfiles)+@([pscustomobject]@{
        image=$newImage;directory=$normalRecovery;manifestSha=$normalManifestSha
    })
    $temp=$pinsFile+'.tmp'
    try{
        $installedPins|ConvertTo-Json -Depth 12|Set-Content -LiteralPath $temp -Encoding UTF8
        Move-Item -LiteralPath $temp -Destination $pinsFile -Force -ErrorAction Stop
    }finally{
        if(Test-Path -LiteralPath $temp){Remove-Item -LiteralPath $temp -Force}
    }
    $spec.status='NORMAL_PROFILE_PINNED_NOT_ACTIVATED'
    $spec|ConvertTo-Json -Depth 5
    return
}

if($Action -eq 'ArmRecovery'){
    $handoverArgs=@{
        G10TaskDisabled=(!$spec.g10Enabled)
        G10WatcherCount=$go10Watchers.Count
        RustTasksDisabled=$rustDisabled
        OtherG11TaskCount=$other.Count
        AssetsVerified=($oldSha -eq $installedPins.previousSha)
        ProgramDataProtected=$true
    }
    $handover=Test-G11RecoveryHandover @handoverArgs
    if(!$handover.ready){throw ('Recovery handover refused: '+($handover.reasons -join ','))}
    if(Test-Path -LiteralPath (Join-Path $events 'recovery-armed.json')){
        throw 'Recovery has already been armed'
    }
    $taskBackup=Join-Path $events 'pre-recovery-task.xml'
    if(Test-Path -LiteralPath $taskBackup){throw 'Original G11 task XML already frozen'}
    Export-ScheduledTask -TaskName $taskName|Set-Content -LiteralPath $taskBackup -Encoding UTF8
    $marker=Join-Path $events 'recovery-armed.json'
    [pscustomobject]@{version=1;manifestSha=$manifestSha;armedAtUtc=[DateTimeOffset]::UtcNow.ToString('o')}|
        ConvertTo-Json|Set-Content -LiteralPath $marker -Encoding UTF8
    try {
        $armedAction=New-ScheduledTaskAction -Execute $taskPowerShell -Argument ($argsBase+' -RecoveryEnabled')
        Set-ScheduledTask -TaskName $taskName -Action $armedAction -ErrorAction Stop|Out-Null
    } catch {
        Remove-Item -LiteralPath $marker -Force -ErrorAction SilentlyContinue
        try {
            Register-ScheduledTask -TaskName $taskName -Xml (Get-Content -LiteralPath $taskBackup -Raw) -Force -ErrorAction Stop|Out-Null
        } catch {
            throw 'G11 arm failed AND Task XML restore failed; immediate operator inspection required'
        }
        throw 'Failed to arm G11 task; original Task XML restored'
    }
    $spec.status='GO_ONLY_RECOVERY_ARMED'
    $spec|ConvertTo-Json -Depth 5
    return
}

if($Action -eq 'Remove'){
    if(!$SuccessorVerified){throw 'Removing only monitoring requires a verified successor and explicit approval'}
    $backup=Join-Path $events ('task-before-removal-'+(Get-Date -Format 'yyyyMMdd-HHmmss')+'.xml')
    Export-ScheduledTask -TaskName $taskName|Set-Content -LiteralPath $backup -Encoding UTF8
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction Stop
    $spec.status='TASK_REMOVED_EVIDENCE_RETAINED'
    $spec|ConvertTo-Json -Depth 5
    return
}
throw 'Unsupported task action'
} finally {
    Exit-G11Mutex $installMutex
}
