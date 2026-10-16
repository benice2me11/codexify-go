# Independently scheduled Go-only canary operator task.
# Default: print an inert task specification. -Register requires explicit
# approval plus fresh G10 readiness and only installs an on-demand task.
# This script never starts the task or changes CodexifyGo SCM/worker/tunnel.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TransactionDirectory,
    [switch]$Register,
    [switch]$OperatorApproved,
    [switch]$UseAcceptedG10
)
$ErrorActionPreference='Stop'
$root=[IO.Path]::GetFullPath($TransactionDirectory)
$file=Join-Path $root 'manifest.json'
if (!(Test-Path -LiteralPath $file -PathType Leaf)){throw 'Operator transaction is not prepared'}
$m=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json -ErrorAction Stop
if ([string]$m.transactionKind -ne 'go_only_watch_go_trial_to_go_only' -or
    [string]$m.stageSourceCommit -notmatch '^[a-f0-9]{40}$') {
    throw 'Prepared transaction is not the expected Go-only type'
}
if([bool]$m.useAcceptedG10 -ne [bool]$UseAcceptedG10){
    throw 'Task approval policy does not match the frozen operator transaction'
}
$name='CodexifyGo-PGCanary-'+$m.stageSourceCommit.Substring(0,12)
$executor=Join-Path $root 'operator\Invoke-GoOnlyTrialUpgrade.ps1'
if (!(Test-Path -LiteralPath $executor -PathType Leaf)){throw 'Prepared independent executor not found'}
$plan=& $executor -TransactionDirectory $root -UseAcceptedG10:$UseAcceptedG10|ConvertFrom-Json -ErrorAction Stop
$eligible=if($UseAcceptedG10){
    ($plan.decision -eq 'READY_FOR_INDEPENDENT_OPERATOR_WITH_ACCEPTED_RISK' -and
     [bool]$plan.candidateSpecificApprovalValid)
}else{
    $plan.decision -eq 'READY_FOR_INDEPENDENT_OPERATOR'
}
$approvalSHA=''
if($UseAcceptedG10 -and [bool]$plan.candidateSpecificApprovalValid){
    $approvalFile=Join-Path $root 'candidate-approval.json'
    $approvalSHA=(Get-FileHash -LiteralPath $approvalFile -Algorithm SHA256 -ErrorAction Stop).Hash
}
$argsText='-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$executor+
    '" -TransactionDirectory "'+$root+'" -ExpectedSourceCommit "'+$m.stageSourceCommit+
    '" -OperatorTaskName "'+$name+'" -Execute'
if($UseAcceptedG10){
    $argsText+=' -UseAcceptedG10'
    if($approvalSHA){$argsText+=' -ExpectedCandidateApprovalSHA256 "'+$approvalSHA+'"'}
}
$spec=[ordered]@{
    checkedAtUtc=[DateTimeOffset]::UtcNow.ToString('o')
    taskName=$name
    mode=$(if($Register){'REGISTER_REQUESTED'}else{'PLAN_ONLY'})
    g10Decision=$plan.decision
    taskCanBeInstalled=$eligible
    strictTechnicalG10=$plan.strictTechnicalG10
    useAcceptedG10=[bool]$UseAcceptedG10
    candidateSpecificApprovalValid=$plan.candidateSpecificApprovalValid
    candidateApprovalPinnedSHA256=$(if($approvalSHA){$approvalSHA}else{$null})
    exactCandidateHash=$m.newSha
    operatorRequiresElevation=$true
    operatorRequiresSeparateApproval=$true
    independentOnDemand=$true
    taskHasAutomaticTriggers=$false
    executionArmed=$false
    serviceMutated=$false
    rustFallback=$false
}
if(!$Register){
    $spec|ConvertTo-Json -Depth 6
    return
}
if(!$OperatorApproved){throw 'Register requires an explicit independent-operator approval'}
if(!$eligible){
    throw 'G10 safety/candidate approval is HOLD; refusing to register an executable operator task'
}
$principal=[Security.Principal.WindowsPrincipal]([Security.Principal.WindowsIdentity]::GetCurrent())
if(!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){
    throw 'Registration requires an elevated operator context'
}
if(Get-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue){
    throw 'Task name already exists: refusing overwrite'
}
$action=New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $argsText
$identity=[Security.Principal.WindowsIdentity]::GetCurrent()
$taskPrincipal=New-ScheduledTaskPrincipal -UserId $identity.Name -LogonType Interactive -RunLevel Highest
$settings=New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Minutes 12) -MultipleInstances IgnoreNew
$definition=New-ScheduledTask -Action $action -Principal $taskPrincipal -Settings $settings
Register-ScheduledTask -TaskName $name -InputObject $definition -ErrorAction Stop | Out-Null
$registered=Get-ScheduledTask -TaskName $name -ErrorAction Stop
if(@($registered.Triggers | Where-Object { $null -ne $_ }).Count -ne 0 -or
    @($registered.Actions).Count -ne 1 -or
    $registered.Principal.RunLevel -ne 'Highest'){
    throw 'On-demand operator task registration did not retain safety constraints'
}
$spec.mode='REGISTERED_ON_DEMAND_NOT_STARTED'
$spec.executionArmed=$true
$spec|ConvertTo-Json -Depth 6
