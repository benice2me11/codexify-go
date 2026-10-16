Set-StrictMode -Version Latest

# Pure, data-only policy functions shared by the independent Go-to-Go
# operator transaction and its offline test suite. They never invoke SCM,
# Task Scheduler, HTTP, or arbitrary external commands.

function Get-GoOnlyTrialPreflightDecision {
    [CmdletBinding()]
    param([Parameter(Mandatory)]$Facts)

    $reasons = [System.Collections.Generic.List[string]]::new()
    if (!$Facts.ServiceRunning) { $reasons.Add('go_service_not_running') }
    if ([int]$Facts.GoCount -ne 2 -or [int]$Facts.WorkerCount -ne 1 -or [int]$Facts.TunnelCount -ne 1) {
        $reasons.Add('go_owner_topology_not_unique')
    }
    if ([int]$Facts.RustCount -ne 0 -or !$Facts.RustTasksDisabled) {
        $reasons.Add('rust_owner_or_task_present')
    }
    if ([int]$Facts.WatcherCount -ne 1 -or !$Facts.WatcherMatchesLiveGo -or
        !$Facts.WatcherTaskPinned -or !$Facts.BootTriggerPresent -or !$Facts.WatcherHighest -or
        !$Facts.WatcherRecoveryArmed) {
        $reasons.Add('go_only_watcher_not_uniquely_owned')
    }
    if (!$Facts.OldHashValid -or !$Facts.NewHashValid -or !$Facts.ConfigHashValid -or
        !$Facts.SnapshotHashValid -or !$Facts.IndependentOperatorReady) {
        $reasons.Add('candidate_or_go_recovery_assets_invalid')
    }
    if (!$Facts.FeatureFlagDisabled -or !$Facts.ConfigScopePreserved) {
        $reasons.Add('unsafe_project_groups_staged_configuration')
    }
    if ([string]$Facts.G10Decision -ne 'LOCAL_READY_FOR_OPERATOR_REVIEW') {
        $reasons.Add('g10_observation_not_accepted')
    }
    $decision = 'READY_FOR_INDEPENDENT_OPERATOR'
    if ($reasons.Count -ne 0) { $decision = 'HOLD' }
    return [pscustomobject]@{
        decision = $decision
        reasons = @($reasons.ToArray())
        oldGoOnly = $true
        rustFallback = $false
        mayMutateSCM = ($decision -eq 'READY_FOR_INDEPENDENT_OPERATOR')
    }
}

function Get-GoOnlyTrialRecoveryDecision {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][bool]$ServiceImageMayHaveChanged,
        [Parameter(Mandatory)][ValidateSet('previous_go','candidate_go','foreign','unavailable')][string]$ImageOwner,
        [Parameter(Mandatory)][bool]$OldObserverWasDisabled
    )
    $decision = 'NO_MUTATION_NEEDED'
    if ($ImageOwner -in @('foreign','unavailable') -or
        (!$ServiceImageMayHaveChanged -and $ImageOwner -ne 'previous_go')) {
        $decision = 'BLOCKED_FOREIGN_OR_UNKNOWN_OWNER'
    } elseif ($ServiceImageMayHaveChanged) {
        $decision = 'RESTORE_PREVIOUS_GO_AND_OBSERVER'
    } elseif ($OldObserverWasDisabled) {
        $decision = 'RESTORE_PREVIOUS_GO_OBSERVER_ONLY'
    }
    return [pscustomobject]@{
        action = $decision
        autoRustFallback = $false
        mayRestartService = ($decision -eq 'RESTORE_PREVIOUS_GO_AND_OBSERVER')
        mayRestoreOldWatcher = ($decision -in @('RESTORE_PREVIOUS_GO_AND_OBSERVER','RESTORE_PREVIOUS_GO_OBSERVER_ONLY'))
    }
}

function Get-GoOnlyTrialOptionalProperty {
    param([Parameter(Mandatory)]$Item,[Parameter(Mandatory)][string]$Name)
    if ($null -eq $Item) { return '' }
    $property = $Item.PSObject.Properties[$Name]
    if ($null -eq $property -or $null -eq $property.Value) { return '' }
    return [string]$property.Value
}

function Assert-GoOnlyTrialProfile {
    [CmdletBinding()]
    param([Parameter(Mandatory)]$Previous, [Parameter(Mandatory)]$Candidate)

    if ([string]$Previous.service.name -ne 'CodexifyGo' -or
        [string]$Candidate.service.name -ne [string]$Previous.service.name) {
        throw 'candidate service name differs from previous Go profile'
    }
    if (!$Previous.mcp.multiProject -or !$Candidate.mcp.multiProject) {
        throw 'multiProject must remain enabled'
    }
    if ([bool]$Candidate.mcp.projectGroupsEnabled) {
        throw 'projectGroupsEnabled must remain false during binary cutover'
    }
    foreach ($name in @('workspaceRoot','authEnabled','bindingsDir','cloneDir','scratchDir')) {
        if ((Get-GoOnlyTrialOptionalProperty $Previous.mcp $name) -ne
            (Get-GoOnlyTrialOptionalProperty $Candidate.mcp $name)) {
            throw "candidate mcp.$name changes the existing workspace scope"
        }
    }
    foreach ($name in @('tunnelId','apiKeyRef','mcpServerUrl','organizationId',
                         'mcpAuthorizationRef','healthUrlFile','managedDir','executable')) {
        if ((Get-GoOnlyTrialOptionalProperty $Previous.tunnel $name) -ne
            (Get-GoOnlyTrialOptionalProperty $Candidate.tunnel $name)) {
            throw "candidate tunnel.$name differs from the current profile"
        }
    }
    # Source schemas must be unchanged. JSON serialization here compares
    # configuration state without printing any API keys or embedded headers.
    $previousUpstreams = $Previous.mcp.upstreams | ConvertTo-Json -Depth 45 -Compress
    $candidateUpstreams = $Candidate.mcp.upstreams | ConvertTo-Json -Depth 45 -Compress
    if ($previousUpstreams -ne $candidateUpstreams) {
        throw 'candidate changes existing upstream MCP definitions'
    }
    if (@($Candidate.mcp.projects | Where-Object { $null -ne $_ }).Count -lt 3) {
        throw 'staged Project Groups require an explicit three-repository catalog'
    }
    $groups = @($Candidate.mcp.projectGroups | Where-Object { $null -ne $_ })
    if ($groups.Count -ne 1 -or [string]$groups[0].name -ne 'job-search' -or
        [string]$groups[0].primary -ne 'work') {
        throw 'candidate group catalog differs from the expected job-search shape'
    }
    $rows = @($groups[0].repositories | Where-Object { $null -ne $_ })
    if ($rows.Count -ne 3) { throw 'job-search must have exactly three members' }
    $aliases = @('work','hh','habr')
    for ($i=0; $i -lt $rows.Count; $i++) {
        if ([string]$rows[$i].alias -ne $aliases[$i] -or
            [string]$rows[$i].checkout -ne 'source') {
            throw 'candidate group member differs from expected read-only source rollout'
        }
    }

    # Guard the *whole* production profile against accidental non-group
    # mutations, not just the tunnel id and bindings root. The staged profile
    # is produced by copying the previous production profile and adding the
    # three explicit group catalog fields. A silent change to service
    # supervision, tunnel polling, auth, skills or artifact settings is
    # forbidden during this binary-only Go-to-Go transition.
    $before = $Previous | ConvertTo-Json -Depth 70 | ConvertFrom-Json
    $after = $Candidate | ConvertTo-Json -Depth 70 | ConvertFrom-Json
    foreach ($field in @('projects','projectGroups','projectGroupsEnabled')) {
        [void]$before.mcp.PSObject.Properties.Remove($field)
        [void]$after.mcp.PSObject.Properties.Remove($field)
    }
    if (($before | ConvertTo-Json -Depth 70 -Compress) -cne
        ($after | ConvertTo-Json -Depth 70 -Compress)) {
        throw 'candidate changed production profile outside explicit Project Groups catalog'
    }
    return $true
}

Export-ModuleMember -Function Get-GoOnlyTrialPreflightDecision,Get-GoOnlyTrialRecoveryDecision,Assert-GoOnlyTrialProfile
