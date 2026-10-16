# Deterministic tests for the Go-only-to-Go-only operator policy. No SCM,
# services, tasks, tunnels or real production files are opened or modified.
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyTrialTransaction.psm1') -Force

# The executor stays uninvoked in CI, but its real PowerShell AST must parse
# cleanly on Windows (including its explicit, gated recovery branch).
foreach ($scriptName in @('GoOnlyTrialTransaction.psm1','Prepare-GoOnlyTrialUpgrade.ps1',
                         'Invoke-GoOnlyTrialUpgrade.ps1','New-GoOnlyTrialOperatorTask.ps1')) {
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

$checks = @{
    G10Decision = 'HOLD'
    GoCount = 3
    WorkerCount = 0
    TunnelCount = 0
    RustCount = 1
    RustTasksDisabled = $false
    WatcherCount = 0
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

Write-Output 'GO_ONLY_TRIAL_TRANSACTION_POLICY=PASS'
