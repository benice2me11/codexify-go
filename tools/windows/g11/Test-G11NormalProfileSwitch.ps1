# Offline G11 normal-profile switch safety fixture. Never mutates production SCM.
$ErrorActionPreference='Stop'
$scriptPath=Join-Path $PSScriptRoot 'Invoke-G11NormalProfileSwitch.ps1'
$tokens=$null;$errors=$null
[void]($ast=[Management.Automation.Language.Parser]::ParseFile($scriptPath,[ref]$tokens,[ref]$errors))
if(@($errors).Count -ne 0){throw 'G11 normal operator failed PowerShell AST'}
$reservedPidWrites=@($ast.FindAll({
    param($node)
    if($node -is [Management.Automation.Language.ParameterAst]){
        return $node.Name.VariablePath.UserPath -ieq 'PID'
    }
    if($node -is [Management.Automation.Language.AssignmentStatementAst] -and
       $node.Left -is [Management.Automation.Language.VariableExpressionAst]){
        return $node.Left.VariablePath.UserPath -ieq 'PID'
    }
    return $false
},$true))
if($reservedPidWrites.Count -gt 0){
    throw 'Normal-profile operator must not assign or bind reserved PowerShell $PID'
}
$root=Join-Path $env:TEMP ('g11-normal-fixture-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root -ErrorAction Stop|Out-Null
function Assert([bool]$Cond,[string]$Reason){if(!$Cond){throw ('G11 normal switch fixture failed: '+$Reason)}}
function Reject([scriptblock]$Invoke,[string]$Reason) {
    $denied=$false
    try{& $Invoke|Out-Null}catch{$denied=$true}
    Assert $denied $Reason
}
try {
    $a=Join-Path $root 'previous-go.exe';$b=Join-Path $root 'current-go.exe'
    $cfgA=Join-Path $root 'diagnostics-on.json';$cfgB=Join-Path $root 'diagnostics-off.json'
    $mod=Join-Path $root 'module.psm1';$executor=Join-Path $root 'recovery.ps1'
    $policy=Join-Path $root 'runtime-pins.json'
    foreach($file in @($a,$b,$mod,$executor)){
        ('fixture-'+[IO.Path]::GetFileName($file))|Set-Content -LiteralPath $file -Encoding ASCII
    }
    '{"mcp":{"diagnostics":{"enabled":true},"projectGroupsEnabled":false},"test":{"keep":11}}'|
        Set-Content -LiteralPath $cfgA -Encoding UTF8
    '{"mcp":{"diagnostics":{"enabled":false},"projectGroupsEnabled":false},"test":{"keep":11}}'|
        Set-Content -LiteralPath $cfgB -Encoding UTF8
    $recA=Join-Path $root 'recovery-current';$recB=Join-Path $root 'recovery-normal'
    New-Item -ItemType Directory -Path $recA,$recB -ErrorAction Stop|Out-Null
    foreach($directory in @($recA,$recB)){
        '{"version":1}'|Set-Content -LiteralPath (Join-Path $directory 'manifest.json') -Encoding UTF8
    }
    $imageA='"'+$b+'" service run --config "'+$cfgA+'"'
    $imageB='"'+$b+'" service run --config "'+$cfgB+'"'
    $manifestSha=('A'*64)
    $pins=[ordered]@{
        version=1;serviceName='CodexifyGo'
        currentExecutable=$b;currentSha=(Get-FileHash $b -Algorithm SHA256).Hash
        previousExecutable=$a;previousSha=(Get-FileHash $a -Algorithm SHA256).Hash
        currentConfig=$cfgA;currentConfigSha=(Get-FileHash $cfgA -Algorithm SHA256).Hash
        currentImage=$imageA
        manifestSha=$manifestSha
        recoveryModule=$mod;recoveryModuleSha=(Get-FileHash $mod -Algorithm SHA256).Hash
        recoveryScript=$executor;recoveryScriptSha=(Get-FileHash $executor -Algorithm SHA256).Hash
        additionalCurrentProfiles=@([pscustomobject]@{
            image=$imageB;config=$cfgB;sha=(Get-FileHash $cfgB -Algorithm SHA256).Hash
        })
        recoveryProfiles=@(
            [pscustomobject]@{image=$imageA;directory=$recA;manifestSha=(Get-FileHash (Join-Path $recA 'manifest.json') -Algorithm SHA256).Hash},
            [pscustomobject]@{image=$imageB;directory=$recB;manifestSha=(Get-FileHash (Join-Path $recB 'manifest.json') -Algorithm SHA256).Hash}
        )
    }
    $pins|ConvertTo-Json -Depth 10|Set-Content -LiteralPath $policy -Encoding UTF8
    $hash=(Get-FileHash $policy -Algorithm SHA256).Hash
    $ev=Join-Path $root 'evidence'
    New-Item -ItemType Directory -Path $ev -ErrorAction Stop|Out-Null
    [pscustomobject]@{version=1;manifestSha=$manifestSha}|
        ConvertTo-Json|Set-Content -LiteralPath (Join-Path $ev 'recovery-armed.json') -Encoding UTF8
    [pscustomobject]@{lastAction='CONTINUE';lastCheckUtc=[DateTimeOffset]::UtcNow.ToString('o')}|
        ConvertTo-Json|Set-Content -LiteralPath (Join-Path $ev 'state.json') -Encoding UTF8

    $global:fixtureService=[pscustomobject]@{State='Running';ProcessId=42;PathName=$imageA}
    $global:fixtureG10=[pscustomobject]@{TaskName='CodexifyGo-G10Observers';State='Disabled';Settings=[pscustomobject]@{Enabled=$false}}
    $global:fixtureG11=[pscustomobject]@{TaskName='CodexifyGo-G11SteadyState';State='Ready'
        Principal=[pscustomobject]@{UserId='SYSTEM'}
        Actions=@([pscustomobject]@{Arguments='-File monitor.ps1 -RecoveryEnabled'})}
    $global:fixtureRust1=[pscustomobject]@{TaskName='Codexify';State='Disabled'}
    $global:fixtureRust2=[pscustomobject]@{TaskName='Codexify Watchdog';State='Disabled'}
    $global:fixtureProcs=@(
        [pscustomobject]@{Name='codexify-go.exe';ProcessId=42;ParentProcessId=5;ExecutablePath=$b},
        [pscustomobject]@{Name='codexify-go.exe';ProcessId=43;ParentProcessId=42;ExecutablePath=$b},
        [pscustomobject]@{Name='tunnel-client-runtime.exe';ProcessId=44;ParentProcessId=42;ExecutablePath='tunnel.exe'}
    )
    function Get-CimInstance {
        param([string]$ClassName,[string]$Filter)
        if($ClassName -eq 'Win32_Service'){return $global:fixtureService}
        if($ClassName -eq 'Win32_Process'){return $global:fixtureProcs}
        throw 'fixture unexpected CIM class'
    }
    function Get-ScheduledTask {
        param([string]$TaskName)
        if(!$TaskName){return @($global:fixtureG10,$global:fixtureG11,$global:fixtureRust1,$global:fixtureRust2)}
        switch($TaskName){
            'CodexifyGo-G10Observers' {return $global:fixtureG10}
            'CodexifyGo-G11SteadyState' {return $global:fixtureG11}
            'Codexify' {return $global:fixtureRust1}
            'Codexify Watchdog' {return $global:fixtureRust2}
            default {throw 'fixture unexpected task'}
        }
    }
    function Get-ScheduledTaskInfo {param([string]$TaskName);return [pscustomobject]@{LastTaskResult=0}}
    function Invoke-WebRequest {param([string]$Uri,[switch]$UseBasicParsing,[int]$TimeoutSec);return [pscustomobject]@{StatusCode=200}}
    function sc.exe {
        return @(
            'SERVICE_NAME: CodexifyGo',
            'RESET_PERIOD (in seconds)    : 86400',
            'FAILURE_ACTIONS : RESTART -- Delay = 5000 milliseconds.',
            '                  RESTART -- Delay = 15000 milliseconds.',
            '                  RESTART -- Delay = 60000 milliseconds.'
        )
    }
    $params=@{PolicyFile=$policy;EvidenceDirectory=$ev;ExpectedPolicySha256=$hash;ExpectedServicePid=42}
    $r=(& $scriptPath @params)|ConvertFrom-Json
    Assert ($r.status -eq 'PLAN_ONLY' -and !$r.serviceMutation -and $r.normalProfileApproved -and !$r.projectGroupsEnabled -and !$r.rustFallback) 'valid plan'
    Assert (!(Test-Path (Join-Path $ev 'normal-switch-once.lock'))) 'plan cannot create lock or mutate'
    Reject {& $scriptPath @params -Execute} 'execute without operator approval must fail'
    Reject {& $scriptPath @params -Execute -OperatorApproved} 'fixture cannot run elevated production switch'
    Reject {& $scriptPath -PolicyFile $policy -EvidenceDirectory $ev -ExpectedPolicySha256 ('B'*64)} 'wrong pinned policy SHA must fail'
    Reject {& $scriptPath @params -ExpectedServicePid 99} 'SCM PID change must fail'
    $global:fixtureService.PathName='foreign'
    Reject {& $scriptPath @params} 'foreign SCM ImagePath must fail closed'
    $global:fixtureService.PathName=$imageA
    $global:fixtureRust1.State='Ready'
    Reject {& $scriptPath @params} 'enabled Rust fallback task rejected'
    $global:fixtureRust1.State='Disabled'
    $global:fixtureProcs+=([pscustomobject]@{Name='codexify-go.exe';ProcessId=45;ParentProcessId=99;ExecutablePath=$b})
    Reject {& $scriptPath @params} 'duplicate Go owner rejected'
    $global:fixtureProcs=@($global:fixtureProcs|Where-Object ProcessId -ne 45)
    $global:fixtureG10.Settings.Enabled=$true
    Reject {& $scriptPath @params} 'armed G10 rejected'
    $global:fixtureG10.Settings.Enabled=$false
    $global:fixtureG11.Actions[0].Arguments='-File monitor.ps1'
    Reject {& $scriptPath @params} 'unarmed G11 rejected'
    $global:fixtureG11.Actions[0].Arguments='-File monitor.ps1 -RecoveryEnabled'
    $s=Get-Content (Join-Path $ev 'state.json') -Raw|ConvertFrom-Json
    $s.lastCheckUtc=[DateTimeOffset]::UtcNow.AddMinutes(-10).ToString('o')
    $s|ConvertTo-Json|Set-Content (Join-Path $ev 'state.json')
    Reject {& $scriptPath @params} 'stale G11 status rejected'
    Assert (!(Test-Path (Join-Path $ev 'normal-switch-once.lock'))) 'all rejected plans remain nonmutating'
    'G11_NORMAL_PROFILE_SWITCH_OFFLINE=PASS'
} finally {
    $tmp=[IO.Path]::GetFullPath($env:TEMP).TrimEnd('\','/')
    $parent=[IO.Path]::GetFullPath((Split-Path -Parent $root)).TrimEnd('\','/')
    if(![string]::Equals($parent,$tmp,[StringComparison]::OrdinalIgnoreCase) -or
       [IO.Path]::GetFileName($root) -notmatch '^g11-normal-fixture-[a-f0-9]{32}$'){
        throw 'Unsafe G11 fixture cleanup'
    }
    Remove-Item -LiteralPath $root -Recurse -Force
}
