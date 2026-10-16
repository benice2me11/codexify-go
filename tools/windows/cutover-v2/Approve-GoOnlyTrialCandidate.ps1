# An explicit, candidate-specific authorization for a Go-only canary based on
# the separately accepted *previous Go* G10 exception.
# Defaults to PLAN_ONLY. -Authorize only writes a time-bound private record;
# it NEVER registers a task, changes SCM, or starts a tunnel/worker.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TransactionDirectory,
    [switch]$Authorize,
    [switch]$OperatorApproved,
    [string]$ExpectedCandidateSHA256 = ''
)
$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyTrialTransaction.psm1') -Force
$root=[IO.Path]::GetFullPath($TransactionDirectory)
$file=Join-Path $root 'manifest.json'
if(!(Test-Path -LiteralPath $file -PathType Leaf)){throw 'Protected Go-only transaction is missing'}
$m=Get-Content -LiteralPath $file -Raw | ConvertFrom-Json -ErrorAction Stop
if([string]$m.transactionKind -ne 'go_only_watch_go_trial_to_go_only' -or
   [bool]$m.autoRustFallback -or
   [string]$m.stageSourceCommit -notmatch '^[a-f0-9]{40}$' -or
   [string]$m.g10OperatorAcceptanceSHA256 -notmatch '^[A-Fa-f0-9]{64}$'){
    throw 'Transaction has no pinned prior-Go G10 operator acceptance'
}
$stage=Split-Path -Parent $root
if(!(Get-Acl -LiteralPath $stage).AreAccessRulesProtected){
    throw 'Candidate approval is not below a protected operator staging root'
}
$priorFile=Join-Path $stage 'g10-operator-acceptance-20261016.json'
$actualPriorSHA=(Get-FileHash -LiteralPath $priorFile -Algorithm SHA256 -ErrorAction Stop).Hash
if($actualPriorSHA -ne [string]$m.g10OperatorAcceptanceSHA256){
    throw 'Prior Go operator acceptance record was modified after transaction preparation'
}
if((Get-FileHash -LiteralPath $m.newExe -Algorithm SHA256).Hash -ne [string]$m.newSha -or
   (Get-FileHash -LiteralPath $m.configPath -Algorithm SHA256).Hash -ne [string]$m.configSha){
    throw 'Candidate executable or feature-disabled config changed since staging'
}
$accepted=Get-Content -LiteralPath $priorFile -Raw|ConvertFrom-Json -ErrorAction Stop
$raw=& (Join-Path $PSScriptRoot 'Assess-ProjectGroupsCanaryReadiness.ps1') -RunDirectory $m.oldRunDirectory
$g10=$raw|ConvertFrom-Json -ErrorAction Stop
$risk=Get-GoOnlyTrialAcceptedRiskEvidence -Acceptance $accepted -G10 $g10 -PreviousGoSHA256 $m.oldSha
if(!$risk.validForPreviousGoOnly){
    throw ('Current G10 safety state differs from the accepted old-Go exceptions: '+($risk.reasons -join ','))
}
$approvalPath=Join-Path $root 'candidate-approval.json'
$existing=Test-Path -LiteralPath $approvalPath -PathType Leaf
$summary=[ordered]@{
    mode=$(if($Authorize){'AUTHORIZE_REQUESTED'}else{'PLAN_ONLY'})
    oldGoOperatorRiskAccepted=$risk.validForPreviousGoOnly
    strictTechnicalG10=$g10.decision
    candidateAuthorized=$existing
    candidateSourceCommit=$m.stageSourceCommit
    previousGoSHA256=$m.oldSha
    candidateGoSHA256=$m.newSha
    candidateConfigSHA256=$m.configSha
    explicitCandidateApprovalRequired=$true
    actionRegistered=$false
    serviceOrTunnelChanged=$false
    rustFallback=$false
}
if(!$Authorize){$summary|ConvertTo-Json -Depth 5;return}
if(!$OperatorApproved){throw 'Candidate authorization requires explicit independent go/no-go approval'}
if($ExpectedCandidateSHA256 -ne [string]$m.newSha){
    throw 'Candidate authorization must name exact 64-digit SHA256 of the staged Go binary'
}
if($existing){throw 'Candidate approval record already exists: refusing overwrite/replay'}
$now=[DateTimeOffset]::UtcNow
$approval=[ordered]@{
    schemaVersion=1
    intent='GO_ONLY_PROJECT_GROUPS_CANARY_WITH_G10_OPERATOR_RISK_EXCEPTION'
    explicitOperatorApproval=$true
    createdAtUtc=$now.ToString('o')
    expiresAtUtc=$now.AddMinutes(30).ToString('o')
    previousGoSHA256=$m.oldSha
    candidateGoSHA256=$m.newSha
    candidateConfigSHA256=$m.configSha
    candidateSourceCommit=$m.stageSourceCommit
    acceptedRiskMarkerSHA256=$actualPriorSHA
    strictTechnicalG10AtApproval=$g10.decision
    allowRustFallback=$false
}
$args=@{
    Approval=[pscustomobject]$approval
    PreviousGoSHA256=$m.oldSha
    CandidateSHA256=$m.newSha
    CandidateConfigSHA256=$m.configSha
    SourceCommit=$m.stageSourceCommit
    AcceptedRiskMarkerSHA256=$actualPriorSHA
    Now=$now
}
$check=Get-GoOnlyTrialCandidateApprovalEvidence @args
if(!$check.valid){throw "Candidate approval policy rejected proposal: $($check.reason)"}
$bytes=[Text.UTF8Encoding]::new($false).GetBytes(($approval|ConvertTo-Json -Depth 6))
$handle=[IO.File]::Open($approvalPath,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
try{$handle.Write($bytes,0,$bytes.Length)}finally{$handle.Dispose()}
$summary.mode='CANDIDATE_APPROVAL_CREATED_NOT_DEPLOYED'
$summary.candidateAuthorized=$true
$summary.expiresAtUtc=$approval.expiresAtUtc
$summary|ConvertTo-Json -Depth 5
