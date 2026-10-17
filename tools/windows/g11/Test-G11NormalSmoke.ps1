# Offline single-sample safety fixture for the passive G11 30-minute smoke.
$ErrorActionPreference='Stop'
$scriptPath=Join-Path $PSScriptRoot 'Invoke-G11NormalSmoke.ps1'
$tokens=$null;$errors=$null
[void][Management.Automation.Language.Parser]::ParseFile($scriptPath,[ref]$tokens,[ref]$errors)
if(@($errors).Count -ne 0){throw 'G11 smoke observer failed PowerShell AST'}
$source=Get-Content -LiteralPath $scriptPath -Raw
if($source -match '(?im)\b(Stop-Service|Start-Service|Restart-Service|Set-Service|Set-GoServiceImage|Set-ScheduledTask|Start-ScheduledTask)\b'){
    throw 'Smoke observer unexpectedly contains live service/task mutation'
}
$root=Join-Path $env:TEMP ('g11-smoke-fixture-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root -ErrorAction Stop|Out-Null
try {
    $currentExe=Join-Path $root 'codexify-go.exe'
    $previousExe=Join-Path $root 'old-go.exe'
    $normal=Join-Path $root 'normal.json'
    foreach($f in @($currentExe,$previousExe)) {
        'test-binary'|Set-Content -LiteralPath $f -Encoding UTF8
    }
    '{"mcp":{"diagnostics":{"enabled":false},"projectGroupsEnabled":false},"test":1}'|
        Set-Content -LiteralPath $normal -Encoding UTF8
    $img='"'+$currentExe+'" service run --config "'+$normal+'"'
    $policy=Join-Path $root 'runtime-pins.json'
    [pscustomobject]@{
        version=1;serviceName='CodexifyGo'
        currentExecutable=$currentExe;currentSha=(Get-FileHash $currentExe -Algorithm SHA256).Hash
        previousExecutable=$previousExe;previousSha=(Get-FileHash $previousExe -Algorithm SHA256).Hash
        additionalCurrentProfiles=@([pscustomobject]@{
            image=$img;config=$normal;sha=(Get-FileHash $normal -Algorithm SHA256).Hash
        })
    }|ConvertTo-Json -Depth 7|Set-Content -LiteralPath $policy -Encoding UTF8
    $stateDir=Join-Path $root 'evidence'
    $sampleDir=Join-Path $root 'samples'
    New-Item -ItemType Directory -Path $stateDir,$sampleDir -ErrorAction Stop|Out-Null
    [pscustomobject]@{
        lastAction='CONTINUE';lastCheckUtc=[DateTimeOffset]::UtcNow.ToString('o')
        recoveryAttemptedUtc=$null
    }|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $stateDir 'state.json') -Encoding UTF8
    $global:smokeFixtureSvc=[pscustomobject]@{State='Running';StartMode='Auto';ProcessId=42;PathName=$img}
    $global:smokeFixtureProcs=@(
        [pscustomobject]@{Name='codexify-go.exe';ProcessId=42;ParentProcessId=5},
        [pscustomobject]@{Name='codexify-go.exe';ProcessId=43;ParentProcessId=42},
        [pscustomobject]@{Name='tunnel-client-runtime.exe';ProcessId=44;ParentProcessId=42}
    )
    function Get-CimInstance {
        param([string]$ClassName,[string]$Filter)
        if($ClassName -eq 'Win32_Service'){return $global:smokeFixtureSvc}
        if($ClassName -eq 'Win32_Process'){return $global:smokeFixtureProcs}
        throw 'Unrecognized smoke fixture CIM query'
    }
    function Get-ScheduledTask {
        param([string]$TaskName)
        switch($TaskName) {
            'CodexifyGo-G10Observers' {return [pscustomobject]@{State='Disabled'}}
            'CodexifyGo-G11SteadyState' {
                return [pscustomobject]@{
                    State='Ready';Actions=@([pscustomobject]@{Arguments='-File watcher.ps1 -RecoveryEnabled'})
                }
            }
            'Codexify' {return [pscustomobject]@{State='Disabled'}}
            'Codexify Watchdog' {return [pscustomobject]@{State='Disabled'}}
            default {throw 'Unrecognized smoke fixture scheduled task'}
        }
    }
    function Get-ScheduledTaskInfo {param([string]$TaskName);return [pscustomobject]@{LastTaskResult=0}}
    function Invoke-WebRequest {
        param([string]$Uri,[switch]$UseBasicParsing,[int]$TimeoutSec)
        return [pscustomobject]@{StatusCode=200}
    }
    $result=& $scriptPath -PolicyFile $policy -EvidenceDirectory $sampleDir -ExpectedSupervisorPid 42 -Once | ConvertFrom-Json
    if($result.status -ne 'SINGLE_SAMPLE_PASS_NOT_ACCEPTANCE' -or
       $result.samples -ne 1 -or $result.serviceMutations -ne 0 -or $result.rustFallback){
        throw 'G11 smoke mock single sample failed'
    }
    $sample=Get-Content -LiteralPath (Join-Path $sampleDir 'smoke-single-sample.jsonl') -Raw|ConvertFrom-Json
    if(!$sample.pass -or !$sample.scmPinned -or !$sample.topology -or !$sample.g11Continue -or
       !$sample.hashPinned -or !$sample.localHealth200 -or
       $sample.workerPid -ne 43 -or $sample.tunnelPid -ne 44){
        throw 'G11 smoke mock did not validate ownership and recovery'
    }
    'G11_NORMAL_SMOKE_OFFLINE=PASS'
}finally{
    $tmp=[IO.Path]::GetFullPath($env:TEMP).TrimEnd('\','/')
    $parent=[IO.Path]::GetFullPath((Split-Path -Parent $root)).TrimEnd('\','/')
    if(![string]::Equals($parent,$tmp,[StringComparison]::OrdinalIgnoreCase) -or
       [IO.Path]::GetFileName($root) -notmatch '^g11-smoke-fixture-[a-f0-9]{32}$'){
        throw 'Unsafe smoke-fixture cleanup'
    }
    Remove-Item -LiteralPath $root -Recurse -Force
}