# Independently scheduled Go-only canary operator task.
# Default: print an inert task specification. -Register requires explicit
# approval plus fresh G10 readiness and only installs an on-demand task.
# This script never starts the task or changes CodexifyGo SCM/worker/tunnel.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TransactionDirectory,
    [switch]$Register,
    [switch]$OperatorApproved
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
$name='CodexifyGo-PGCanary-'+$m.stageSourceCommit.Substring(0,12)
$executor=Join-Path $root 'operator\Invoke-GoOnlyTrialUpgrade.ps1'
if (!(Test-Path -LiteralPath $executor -PathType Leaf)){throw 'Prepared independent executor not found'}
$plan=& $executor -TransactionDirectory $root|ConvertFrom-Json -ErrorAction Stop
$argsText='-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$executor+
    '" -TransactionDirectory "'+$root+'" -ExpectedSourceCommit "'+$m.stageSourceCommit+
    '" -OperatorTaskName "'+$name+'" -Execute'
$spec=[ordered]@{
    checkedAtUtc=[DateTimeOffset]::UtcNow.ToString('o')
    taskName=$name
    mode=$(if($Register){'REGISTER_REQUESTED'}else{'PLAN_ONLY'})
    g10Decision=$plan.decision
    taskCanBeInstalled=($plan.decision -eq 'READY_FOR_INDEPENDENT_OPERATOR')
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
if($plan.decision -ne 'READY_FOR_INDEPENDENT_OPERATOR'){
    throw 'G10 readiness is HOLD; refusing to register an executable operator task'
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
