[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TransactionDirectory,
    [ValidateSet('go_supervisor_unavailable','deploy_validation_failed','new_service_failed','manual_go_only_recovery')][string]$Reason='deploy_validation_failed',
    [switch]$Execute
)
$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyUpgrade.psm1') -Force
$root=[IO.Path]::GetFullPath($TransactionDirectory)
$m=Get-Content -LiteralPath (Join-Path $root 'manifest.json') -Raw|ConvertFrom-Json
if([int]$m.version -ne 1){throw 'Invalid recovery manifest'}
$evidence=Join-Path $root 'evidence'
$events=Join-Path $evidence 'operator-events.jsonl'
$serviceName=[string]$m.serviceName
$taskName=[string]$m.g10TaskName

# Never recover to Rust: only the exact previous Go executable is permitted.
if(!(Test-Path -LiteralPath $m.oldExe -PathType Leaf)){throw 'Old Go binary missing'}
if((Get-FileHash -LiteralPath $m.oldExe -Algorithm SHA256).Hash -ne [string]$m.oldSha) {
    throw 'Old Go recovery asset hash mismatch'
}
if(!$Execute){
    [pscustomobject]@{decision='PLAN_ONLY';service=$serviceName;reason=$Reason;target='previous_go';wouldMutate=$false} | ConvertTo-Json
    return
}
Write-GoUpgradeEvent -Path $events -Phase 'go_only_recovery_start' -Result 'WARN' -Reason $Reason

try {
    $task=Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if($task){ Disable-ScheduledTask -TaskName $taskName -ErrorAction Stop | Out-Null }
    Set-Content -LiteralPath (Join-Path $root 'observer.stop') -Value 'Go-only recovery initiated' -Encoding UTF8

    $live=Get-CimInstance Win32_Service -Filter ("Name='"+$serviceName.Replace("'","''")+"'") -ErrorAction Stop
    $image=[string]$live.PathName
    $oldImage=[string]$m.originalImage
    $newImage=[string]$m.newImage
    $currentExe=Get-GoServiceExecutable $image
    if(!(Compare-GoPath $currentExe $m.oldExe) -and !(Compare-GoPath $currentExe $m.newExe)){
        throw 'Refusing recovery: SCM ImagePath changed by another actor'
    }
    if(!(Compare-GoPath $currentExe $m.oldExe)){
        Set-GoServiceImage -ServiceName $serviceName -ImagePath $oldImage
    }
    $service=Get-Service -Name $serviceName -ErrorAction Stop
    if($service.Status -ne 'Stopped'){
        Stop-Service -Name $serviceName -ErrorAction Stop
        $service.WaitForStatus('Stopped',[timespan]::FromSeconds(25))
    }
    Start-Service -Name $serviceName -ErrorAction Stop
    $up=$false
    for($i=0;$i -lt 45;$i++){
        Start-Sleep -Seconds 1
        $live=Get-CimInstance Win32_Service -Filter ("Name='"+$serviceName+"'")
        if(!$live -or $live.State -ne 'Running' -or [int]$live.ProcessId -lt 1){continue}
        $p=Get-CimInstance Win32_Process -Filter ("ProcessId="+$live.ProcessId) -ErrorAction SilentlyContinue
        if($p -and (Compare-GoPath $p.ExecutablePath $m.oldExe)){$up=$true;break}
    }
    if(!$up){throw 'Previous Go service did not return to Running with the correct executable'}
    Write-GoUpgradeEvent -Path $events -Phase 'go_only_recovered' -Result 'PASS' -Reason $Reason
    [pscustomobject]@{status='GO_RECOVERED';owner='go';oldGoHash=$m.oldSha;taskDisabled=$true} |
        ConvertTo-Json | Set-Content -LiteralPath (Join-Path $root 'go-recovery-result.json') -Encoding UTF8
} catch {
    Write-GoUpgradeEvent -Path $events -Phase 'go_only_recovery_failed' -Result 'ERROR' -Reason $_.Exception.GetType().Name
    throw
}
