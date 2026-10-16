# Deterministic tests for the Go-only-to-Go-only operator policy. No SCM,
# services, tasks, tunnels or real production files are opened or modified.
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyTrialTransaction.psm1') -Force

# The executor stays uninvoked in CI, but its real PowerShell AST must parse
# cleanly on Windows (including its explicit, gated recovery branch).
foreach ($scriptName in @('GoOnlyTrialTransaction.psm1','Prepare-GoOnlyTrialUpgrade.ps1',
                         'Invoke-GoOnlyTrialUpgrade.ps1','New-GoOnlyTrialOperatorTask.ps1',
                         'Approve-GoOnlyTrialCandidate.ps1')) {
    $tokens = $null
    $problems = $null
    $scriptPath = Join-Path $PSScriptRoot $scriptName
    [void][System.Management.Automation.Language.Parser]::ParseFile($scriptPath,[ref]$tokens,[ref]$problems)
    if (@($problems).Count -ne 0) {
        throw "PowerShell parser rejected independent transaction script: $scriptName"
    }
}

function Assert([bool]$condition, [string]$description) {
    if (!$condition) { throw $description }
}
$healthy = [pscustomobject]@{
    ServiceRunning = $true; GoCount = 2; WorkerCount = 1; TunnelCount = 1
    RustCount = 0; RustTasksDisabled = $true; WatcherCount = 1
    WatcherMatchesLiveGo = $true; WatcherTaskPinned = $true
    BootTriggerPresent = $true; WatcherHighest = $true
    WatcherRecoveryArmed = $true; OldHashValid = $true
    NewHashValid = $true; ConfigHashValid = $true
    SnapshotHashValid = $true; IndependentOperatorReady = $true
    FeatureFlagDisabled = $true; ConfigScopePreserved = $true
    G10Decision = 'LOCAL_READY_FOR_OPERATOR_REVIEW'
}
$decision = Get-GoOnlyTrialPreflightDecision -Facts $healthy
Assert ($decision.decision -eq 'READY_FOR_INDEPENDENT_OPERATOR' -and
    $decision.mayMutateSCM -and !$decision.rustFallback) 'healthy Go-only readiness was rejected'

$waived = $healthy.PSObject.Copy()
$waived.G10Decision='HOLD'
$waived | Add-Member -NotePropertyName G10OperatorEvidenceValid -NotePropertyValue $true -Force
$waived | Add-Member -NotePropertyName CandidateSpecificApprovalValid -NotePropertyValue $false -Force
$awaitingApproval=Get-GoOnlyTrialPreflightDecision -Facts $waived
Assert ($awaitingApproval.decision -eq 'HOLD' -and !$awaitingApproval.mayMutateSCM -and
    $awaitingApproval.reasons -contains 'candidate_specific_approval_required') 'previous Go risk acceptance authorized a new candidate without approval'
$waived.CandidateSpecificApprovalValid=$true
$permitted=Get-GoOnlyTrialPreflightDecision -Facts $waived
Assert ($permitted.decision -eq 'READY_FOR_INDEPENDENT_OPERATOR_WITH_ACCEPTED_RISK' -and
    $permitted.usedAcceptedRiskException -and $permitted.mayMutateSCM -and
    $permitted.technicalG10Decision -eq 'HOLD') 'fully approved waiver was not distinguished from strict G10 readiness'
$waived.RustCount=1
$blockedRisk=Get-GoOnlyTrialPreflightDecision -Facts $waived
Assert ($blockedRisk.decision -eq 'HOLD' -and !$blockedRisk.mayMutateSCM) 'accepted risk waived a Rust-owner safety gate'
$waived.RustCount=0
$waived.G10Decision='BLOCKED'
$blockedRisk=Get-GoOnlyTrialPreflightDecision -Facts $waived
Assert ($blockedRisk.decision -eq 'HOLD' -and !$blockedRisk.mayMutateSCM) 'accepted risk waived a new G10 BLOCKED decision'

$oldHash='8'*64
$newHash='F'*64
$cfgHash='A'*64
$commit='b'*40
$technicalReasons=@('g10_observation_gap','non_continue_guard_action','soak_window_short')
$acceptance=[pscustomobject]@{
    decision='G10_OPERATOR_ACCEPTED_WITH_DEVIATIONS'
    scope='already_running_windows_CodexifyGo_sha256_only'
    goSha256=$oldHash
    originalStrictTechnicalDecision='HOLD'
    originalStrictHoldReasons=$technicalReasons
    goToGoCandidateUpgradeApproved=$false
    rustFallbackAllowed=$false
}
$g10=[pscustomobject]@{
    decision='HOLD'
    mode='live_read_only'
    localAssessment='LOCAL_OBSERVED'
    holdReasons=$technicalReasons
    topology=[pscustomobject]@{latestGuard='CONTINUE';goCount=2;tunnelDirectChildren=1;rustCount=0}
    soak=[pscustomobject]@{lastSampleAgeSeconds=2.5;nonContinueCount=2}
    inspectionWarnings=@()
}
$evidence=Get-GoOnlyTrialAcceptedRiskEvidence -Acceptance $acceptance -G10 $g10 -PreviousGoSHA256 $oldHash
Assert ($evidence.validForPreviousGoOnly -and !$evidence.candidateApproved -and
    $evidence.technicalG10Decision -eq 'HOLD') 'healthy scoped old-Go acceptance was rejected'
$tampered=$acceptance.PSObject.Copy()
$tampered.goSha256=$newHash
Assert (!(Get-GoOnlyTrialAcceptedRiskEvidence -Acceptance $tampered -G10 $g10 -PreviousGoSHA256 $oldHash).validForPreviousGoOnly) 'old-Go marker accepted a different binary'
$badG10=$g10.PSObject.Copy()
$badG10.holdReasons=@($technicalReasons)+@('duplicate_go_processes')
Assert (!(Get-GoOnlyTrialAcceptedRiskEvidence -Acceptance $acceptance -G10 $badG10 -PreviousGoSHA256 $oldHash).validForPreviousGoOnly) 'unexpected technical G10 hold was waived'
$badG10=$g10.PSObject.Copy()
$badG10.soak=[pscustomobject]@{lastSampleAgeSeconds=52}
Assert (!(Get-GoOnlyTrialAcceptedRiskEvidence -Acceptance $acceptance -G10 $badG10 -PreviousGoSHA256 $oldHash).validForPreviousGoOnly) 'stale controlled guard was waived'
$badG10=$g10.PSObject.Copy()
$badG10.topology=[pscustomobject]@{latestGuard='CONTINUE';goCount=3;tunnelDirectChildren=1;rustCount=0}
Assert (!(Get-GoOnlyTrialAcceptedRiskEvidence -Acceptance $acceptance -G10 $badG10 -PreviousGoSHA256 $oldHash).validForPreviousGoOnly) 'duplicate live Go owner was waived'
$badG10=$g10.PSObject.Copy()
$badG10.topology=[pscustomobject]@{latestGuard='BLOCKED';goCount=2;tunnelDirectChildren=1;rustCount=0}
Assert (!(Get-GoOnlyTrialAcceptedRiskEvidence -Acceptance $acceptance -G10 $badG10 -PreviousGoSHA256 $oldHash).validForPreviousGoOnly) 'blocked guard was waived'
$badG10=$g10.PSObject.Copy()
$badG10.soak=[pscustomobject]@{lastSampleAgeSeconds=1;nonContinueCount=3}
Assert (!(Get-GoOnlyTrialAcceptedRiskEvidence -Acceptance $acceptance -G10 $badG10 -PreviousGoSHA256 $oldHash).validForPreviousGoOnly) 'new unaccepted tunnel recovery HOLD was waived'

$now=[DateTimeOffset]::Parse('2026-10-16T08:30:00Z')
$markerHash='D'*64
$approval=[pscustomobject]@{
    schemaVersion=1
    intent='GO_ONLY_PROJECT_GROUPS_CANARY_WITH_G10_OPERATOR_RISK_EXCEPTION'
    explicitOperatorApproval=$true
    previousGoSHA256=$oldHash
    candidateGoSHA256=$newHash
    candidateConfigSHA256=$cfgHash
    acceptedRiskMarkerSHA256=$markerHash
    candidateSourceCommit=$commit
    createdAtUtc=$now.AddMinutes(-4).ToString('o')
    expiresAtUtc=$now.AddMinutes(26).ToString('o')
    allowRustFallback=$false
}
$approvalArgs=@{PreviousGoSHA256=$oldHash;CandidateSHA256=$newHash;CandidateConfigSHA256=$cfgHash;SourceCommit=$commit;AcceptedRiskMarkerSHA256=$markerHash;Now=$now}
$valid=Get-GoOnlyTrialCandidateApprovalEvidence -Approval $approval @approvalArgs
Assert $valid.valid 'specific approval with a 30-minute TTL was rejected'
$wrong=$approval.PSObject.Copy()
$wrong.candidateGoSHA256=$oldHash
Assert (!(Get-GoOnlyTrialCandidateApprovalEvidence -Approval $wrong @approvalArgs).valid) 'approval for wrong candidate was accepted'
$wrong=$approval.PSObject.Copy()
$wrong.explicitOperatorApproval=$false
Assert (!(Get-GoOnlyTrialCandidateApprovalEvidence -Approval $wrong @approvalArgs).valid) 'approval without an explicit operator acknowledgement was accepted'
$wrong=$approval.PSObject.Copy()
$wrong.expiresAtUtc=$now.AddMinutes(-1).ToString('o')
Assert (!(Get-GoOnlyTrialCandidateApprovalEvidence -Approval $wrong @approvalArgs).valid) 'expired candidate approval was accepted'
$wrong=$approval.PSObject.Copy()
$wrong.expiresAtUtc=$now.AddMinutes(90).ToString('o')
Assert (!(Get-GoOnlyTrialCandidateApprovalEvidence -Approval $wrong @approvalArgs).valid) 'overlong candidate approval was accepted'
$wrong=$approval.PSObject.Copy()
$wrong.acceptedRiskMarkerSHA256='E'*64
Assert (!(Get-GoOnlyTrialCandidateApprovalEvidence -Approval $wrong @approvalArgs).valid) 'approval bound to another accepted G10 record was accepted'
$wrong=$approval.PSObject.Copy()
$wrong.allowRustFallback=$true
Assert (!(Get-GoOnlyTrialCandidateApprovalEvidence -Approval $wrong @approvalArgs).valid) 'authorization allowing Rust fallback was accepted'

$checks = @{
    G10Decision = 'HOLD'
    GoCount = 3
    WorkerCount = 0
    TunnelCount = 0
    RustCount = 1
    RustTasksDisabled = $false
    WatcherCount = 0
    # Two simultaneous Go-only Watch-GoTrial watchers must not pass.
    WatcherMatchesLiveGo = $false
    WatcherTaskPinned = $false
    BootTriggerPresent = $false
    WatcherHighest = $false
    WatcherRecoveryArmed = $false
    OldHashValid = $false
    NewHashValid = $false
    ConfigHashValid = $false
    SnapshotHashValid = $false
    IndependentOperatorReady = $false
    FeatureFlagDisabled = $false
    ConfigScopePreserved = $false
    ServiceRunning = $false
}
foreach ($name in @($checks.Keys)) {
    $broken = $healthy.PSObject.Copy()
    $broken.$name = $checks[$name]
    $blocked = Get-GoOnlyTrialPreflightDecision -Facts $broken
    Assert ($blocked.decision -eq 'HOLD' -and !$blocked.mayMutateSCM -and
        $blocked.reasons.Count -gt 0 -and !$blocked.rustFallback) "unsafe scenario $name passed"
}
$duplicated = $healthy.PSObject.Copy()
$duplicated.WatcherCount = 2
$duplicateDecision=Get-GoOnlyTrialPreflightDecision -Facts $duplicated
Assert ($duplicateDecision.decision -eq 'HOLD' -and
    $duplicateDecision.reasons -contains 'go_only_watcher_not_uniquely_owned') 'two Go-only watchers were accepted'

$recoveryCases = @(
    @{Changed=$false; Owner='previous_go'; Disabled=$false; Expected='NO_MUTATION_NEEDED'},
    @{Changed=$false; Owner='previous_go'; Disabled=$true; Expected='RESTORE_PREVIOUS_GO_OBSERVER_ONLY'},
    @{Changed=$true; Owner='candidate_go'; Disabled=$true; Expected='RESTORE_PREVIOUS_GO_AND_OBSERVER'},
    @{Changed=$false; Owner='candidate_go'; Disabled=$true; Expected='BLOCKED_FOREIGN_OR_UNKNOWN_OWNER'},
    @{Changed=$true; Owner='previous_go'; Disabled=$true; Expected='RESTORE_PREVIOUS_GO_AND_OBSERVER'},
    @{Changed=$true; Owner='foreign'; Disabled=$true; Expected='BLOCKED_FOREIGN_OR_UNKNOWN_OWNER'},
    @{Changed=$true; Owner='unavailable'; Disabled=$true; Expected='BLOCKED_FOREIGN_OR_UNKNOWN_OWNER'}
)
foreach ($case in $recoveryCases) {
    $result=Get-GoOnlyTrialRecoveryDecision -ServiceImageMayHaveChanged $case.Changed -ImageOwner $case.Owner -OldObserverWasDisabled $case.Disabled
    Assert ($result.action -eq $case.Expected -and !$result.autoRustFallback) "unexpected Go-only recovery action for $($case.Owner)"
    if ($case.Owner -eq 'foreign') {
        Assert (!$result.mayRestartService -and !$result.mayRestoreOldWatcher) 'foreign SCM owner was modified'
    }
}

$profile = @{
    service = @{name='CodexifyGo'}
    mcp = @{
        multiProject=$true; projectGroupsEnabled=$false
        workspaceRoot='C:\Fixture';authEnabled=$true
        bindingsDir='C:\Fixture\bindings';cloneDir='C:\Fixture\clones'
        scratchDir='C:\Fixture\scratch';upstreams=@(@{name='hh_local';command='read-only'})
        projects=@(
            @{name='work-search';path='C:\Fixture\work-search'},
            @{name='hh-mcp-server';path='C:\Fixture\hh-mcp-server'},
            @{name='habr-career-mcp';path='C:\Fixture\habr-career-mcp'}
        )
        projectGroups=@(@{
            name='job-search';primary='work'
            repositories=@(
                @{alias='work';project='work-search';checkout='source'},
                @{alias='hh';project='hh-mcp-server';checkout='source'},
                @{alias='habr';project='habr-career-mcp';checkout='source'}
            )
        })
    }
    tunnel = @{
        tunnelId='fixture';apiKeyRef='env:FIXTURE'
        mcpServerUrl='http://127.0.0.1:31002/mcp'
        organizationId='';mcpAuthorizationRef=''
        healthUrlFile='C:\Fixture\health.url'
        managedDir='C:\Fixture\tunnel';executable='C:\Fixture\tunnel.exe'
    }
}
$baseline = ($profile | ConvertTo-Json -Depth 50 | ConvertFrom-Json)
$candidate = ($profile | ConvertTo-Json -Depth 50 | ConvertFrom-Json)
Assert (Assert-GoOnlyTrialProfile -Previous $baseline -Candidate $candidate) 'same source scope rejected'
$candidate.mcp.projectGroupsEnabled = $true
$rejected = $false
try { Assert-GoOnlyTrialProfile -Previous $baseline -Candidate $candidate | Out-Null }catch{$rejected=$true}
Assert $rejected 'enabled feature flag was accepted'
$candidate.mcp.projectGroupsEnabled = $false
$candidate.tunnel.tunnelId = 'foreign-tunnel'
$rejected = $false
try { Assert-GoOnlyTrialProfile -Previous $baseline -Candidate $candidate | Out-Null }catch{$rejected=$true}
Assert $rejected 'different tunnel ownership was accepted'
$candidate.tunnel.tunnelId = 'fixture'
$candidate.mcp.bindingsDir = 'C:\Other'
$rejected = $false
try { Assert-GoOnlyTrialProfile -Previous $baseline -Candidate $candidate | Out-Null }catch{$rejected=$true}
Assert $rejected 'different binding root was accepted'
$candidate.mcp.bindingsDir = 'C:\Fixture\bindings'
$candidate.tunnel.apiKeyRef = 'env:DIFFERENT'
$rejected = $false
try { Assert-GoOnlyTrialProfile -Previous $baseline -Candidate $candidate | Out-Null }catch{$rejected=$true}
Assert $rejected 'different tunnel API-key reference was accepted'
$candidate.tunnel.apiKeyRef = 'env:FIXTURE'
$candidate.mcp.upstreams[0].command = 'changed'
$rejected = $false
try { Assert-GoOnlyTrialProfile -Previous $baseline -Candidate $candidate | Out-Null }catch{$rejected=$true}
Assert $rejected 'different upstream definition was accepted'
$candidate.mcp.upstreams[0].command = 'read-only'
$candidate | Add-Member -NotePropertyName supervisor -NotePropertyValue @{healthInterval='1ms'} -Force
$rejected = $false
try { Assert-GoOnlyTrialProfile -Previous $baseline -Candidate $candidate | Out-Null }catch{$rejected=$true}
Assert $rejected 'unrelated supervisor change was accepted'

Write-Output 'GO_ONLY_TRIAL_TRANSACTION_POLICY=PASS'
