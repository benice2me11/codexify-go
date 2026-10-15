# Read-only, bounded G10 inspection. No service/task/process mutations and no
# fallback to Rust. Safe to run locally even when the MCP tunnel is offline.
[CmdletBinding()]
param(
    [string]$ServiceName = 'CodexifyGo',
    [string]$RunDirectory = '',
    [string]$CandidateExecutable = '',
    [ValidateRange(1,1440)][int]$RecentMinutes = 60,
    [ValidateRange(5,3600)][int]$GuardFreshSeconds = 45,
    [switch]$Offline
)

$ErrorActionPreference = 'Stop'
$now = [DateTimeOffset]::UtcNow
$since = $now.AddMinutes(-$RecentMinutes)

function Read-BoundedJsonLines {
    param([string]$Path, [int]$MaxLines = 2500)
    if (!$Path -or !(Test-Path -LiteralPath $Path -PathType Leaf)) { return @() }
    $parsed = [System.Collections.Generic.List[object]]::new()
    foreach ($line in @(Get-Content -LiteralPath $Path -Tail $MaxLines -ErrorAction Stop)) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        try { $parsed.Add(($line | ConvertFrom-Json -ErrorAction Stop)) }
        catch { continue } # Incomplete trailing JSONL line must not abort inspection.
    }
    return $parsed.ToArray()
}

function Event-Time {
    param($Event)
    $prop = $Event.PSObject.Properties['time']
    if (!$prop) { return $null }
    try { return [DateTimeOffset]::Parse([string]$prop.Value) }
    catch { return $null }
}

function Get-ServiceExecutable {
    param([string]$PathName)
    if ([string]::IsNullOrWhiteSpace($PathName)) { return '' }
    $value = $PathName.Trim()
    if ($value.StartsWith('"')) {
        $end = $value.IndexOf('"',1)
        if ($end -gt 1) { return $value.Substring(1,$end - 1) }
        return ''
    }
    return ($value -split '\s+',2)[0]
}

# Offline is for tests and incident post-mortems. In this mode no SCM, tasks,
# process enumeration, network requests, or service-related commands are made.
$svc = $null
$serviceState = 'not_inspected'
$servicePid = 0
$serviceExe = ''
$goTotal = $null
$goSameBinary = $null
$rustCount = $null
$tunnelDirectChildren = $null
$rustTasks = [ordered]@{}
$scmTransitions = @()
$readErrors = [System.Collections.Generic.List[string]]::new()

if (!$Offline) {
    try {
        $safeName = $ServiceName.Replace("'","''")
        $svc = Get-CimInstance Win32_Service -Filter ("Name='" + $safeName + "'") -ErrorAction Stop
        if ($svc) {
            $serviceState = [string]$svc.State
            $servicePid = [int]$svc.ProcessId
            $serviceExe = Get-ServiceExecutable ([string]$svc.PathName)
        } else { $serviceState = 'not_installed' }
    } catch {
        $serviceState = 'inspection_failed'
        $readErrors.Add('service_query_failed')
    }
    try {
        $proc = @(Get-CimInstance Win32_Process -ErrorAction Stop)
        $go = @($proc | Where-Object { $_.Name -ieq 'codexify-go.exe' })
        $goTotal = $go.Count
        $rustCount = @($proc | Where-Object { $_.Name -ieq 'codexify.exe' }).Count
        if ($serviceExe) {
            $goSameBinary = @($go | Where-Object {
                $_.ExecutablePath -and [string]::Equals([string]$_.ExecutablePath,$serviceExe,[StringComparison]::OrdinalIgnoreCase)
            }).Count
        }
        if ($servicePid -gt 0) {
            $tunnelDirectChildren = @($proc | Where-Object {
                $_.Name -ieq 'tunnel-client-runtime.exe' -and [int]$_.ParentProcessId -eq $servicePid
            }).Count
        }
    } catch { $readErrors.Add('process_query_failed') }
    foreach ($name in @('Codexify','Codexify Watchdog','CodexifyGo-G10Observers')) {
        try {
            $task = Get-ScheduledTask -TaskName $name -ErrorAction Stop
            $rustTasks[$name] = [string]$task.State
        } catch { $rustTasks[$name] = 'not_found_or_unreadable' }
    }
    try {
        $ids = @(7031,7034,7035,7036,7023,7024)
        $scmTransitions = @(Get-WinEvent -FilterHashtable @{
            LogName='System';StartTime=$since.LocalDateTime;Id=$ids
        } -ErrorAction Stop | Where-Object {
            $_.ProviderName -like '*Service Control Manager*' -and
            $_.Message -match [regex]::Escape($ServiceName)
        } | Select-Object -First 100 | ForEach-Object {
            [pscustomobject]@{
                time=$_.TimeCreated.ToUniversalTime().ToString('o')
                eventId=[int]$_.Id
            }
        })
    } catch { $scmTransitions = @() } # No matching events can be normal.
}

if (!$RunDirectory -and $serviceExe) {
    # The established Windows run layout is <run>/candidate/codexify-go.exe.
    $candidateDir = Split-Path -Parent $serviceExe
    if ((Split-Path -Leaf $candidateDir) -ieq 'candidate') {
        $RunDirectory = Split-Path -Parent $candidateDir
    }
}
if (!$RunDirectory) {
    throw 'Cannot resolve run directory. Supply -RunDirectory; no service was changed.'
}
$runPath = [IO.Path]::GetFullPath($RunDirectory)
$evidence = Join-Path $runPath 'evidence'

$guardStatus = 'not_found'
$guardTime = $null
$guardAgeSeconds = $null
$guardRun = $null
$guardOwners = $null
if (Test-Path -LiteralPath $evidence -PathType Container) {
    $guards = @(Get-ChildItem -LiteralPath $evidence -Recurse -File -Filter 'probe-observations.jsonl' -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTimeUtc -Descending)
    foreach ($file in $guards) {
        $rows = @(Read-BoundedJsonLines -Path $file.FullName -MaxLines 12)
        if ($rows.Count -eq 0) { continue }
        $last = $rows[-1]
        $t = Event-Time $last
        if ($null -eq $t) { continue }
        $guardTime = $t.ToString('o')
        $guardAgeSeconds = [int][math]::Max(0,[math]::Round(($now - $t).TotalSeconds))
        $guardStatus = [string]$last.action
        $guardRun = [string]$file.Directory.Name
        $guardOwners = [ordered]@{
            go = $last.goOwners
            rust = $last.rustOwners
            controlled = $last.controlled
        }
        break
    }
}
$guardFresh = $null -ne $guardAgeSeconds -and $guardAgeSeconds -le $GuardFreshSeconds
if (!$guardFresh -and $guardStatus -ne 'not_found') {
    $readErrors.Add('guard_observation_stale')
}

$logs = [ordered]@{
    scannedRecentMinutes = $RecentMinutes
    parsedLines = 0
    cleanGoStops = 0
    erroredGoStops = 0
    goStarts = 0
    tunnelHealthFailures = 0
    controlPlanePollFailures = 0
    http403Mentions = 0
    http429Mentions = 0
    lastGoStopUtc = $null
    lastGoStartUtc = $null
}
$logPath = Join-Path $evidence 'go-trial-capture.log'
foreach ($record in @(Read-BoundedJsonLines -Path $logPath -MaxLines 5000)) {
    $t = Event-Time $record
    if ($null -eq $t -or $t -lt $since -or $t -gt $now.AddMinutes(1)) { continue }
    $logs.parsedLines++
    $msg = [string]$record.msg
    $detail = $msg + ' ' + [string]$record.error
    if ($msg -eq 'codexify-go stopped') {
        $logs.cleanGoStops++
        $logs.lastGoStopUtc = $t.ToString('o')
    }
    if ($msg -eq 'codexify-go stopped with error') {
        $logs.erroredGoStops++
        $logs.lastGoStopUtc = $t.ToString('o')
    }
    if ($msg -eq 'codexify-go starting') {
        $logs.goStarts++
        $logs.lastGoStartUtc = $t.ToString('o')
    }
    if ($msg -eq 'child health check failed') { $logs.tunnelHealthFailures++ }
    if ($msg -like 'poll failed*') { $logs.controlPlanePollFailures++ }
    if ($detail -match '(^|[^0-9])403([^0-9]|$)|Forbidden') { $logs.http403Mentions++ }
    if ($detail -match '(^|[^0-9])429([^0-9]|$)|RATE_LIMITED') { $logs.http429Mentions++ }
}

$actualBinaryHash = $null
if ($serviceExe -and (Test-Path -LiteralPath $serviceExe -PathType Leaf)) {
    try { $actualBinaryHash = (Get-FileHash -LiteralPath $serviceExe -Algorithm SHA256).Hash }
    catch { $readErrors.Add('running_binary_hash_unavailable') }
}
$candidateHash = $null
$candidateMatchesRunning = $null
if ($CandidateExecutable) {
    if (Test-Path -LiteralPath $CandidateExecutable -PathType Leaf) {
        try {
            $candidateHash = (Get-FileHash -LiteralPath $CandidateExecutable -Algorithm SHA256).Hash
            if ($actualBinaryHash) {
                $candidateMatchesRunning = ($candidateHash -eq $actualBinaryHash)
            }
        } catch { $readErrors.Add('candidate_hash_unavailable') }
    } else { $readErrors.Add('candidate_missing') }
}

$localAssessment = 'INSPECTION_ONLY'
if (!$Offline) {
    $localAssessment = 'LOCAL_DEGRADED'
    if ($serviceState -ieq 'Running' -and $goTotal -ge 2 -and
        $tunnelDirectChildren -ge 1 -and $rustCount -eq 0 -and $guardFresh -and
        $guardStatus -eq 'CONTINUE') {
        $localAssessment = 'LOCAL_OBSERVED'
    }
}
[pscustomobject]@{
    schemaVersion = 1
    checkedAtUtc = $now.ToString('o')
    mode = $(if ($Offline) {'offline'} else {'live_read_only'})
    localAssessment = $localAssessment
    acceptance = 'NOT_EVALUATED' # Never infer G10 PASS from a snapshot.
    service = [pscustomobject]@{
        name=$ServiceName
        state=$serviceState
        pid=$servicePid
        binarySha256=$actualBinaryHash
        candidateSha256=$candidateHash
        candidateMatchesRunning=$candidateMatchesRunning
    }
    processes = [pscustomobject]@{
        goCount=$goTotal
        goFromServiceBinary=$goSameBinary
        rustCount=$rustCount
        tunnelDirectChildren=$tunnelDirectChildren
    }
    scheduledTasks = $rustTasks
    guard = [pscustomobject]@{
        action=$guardStatus
        time=$guardTime
        ageSeconds=$guardAgeSeconds
        fresh=[bool]$guardFresh
        evidenceSubdirectory=$guardRun
        owners=$guardOwners
    }
    scmEvents = $scmTransitions
    logSummary = $logs
    inspectionWarnings = $readErrors.ToArray()
    note = 'This read-only snapshot cannot establish remote ChatGPT tunnel reachability or G10 acceptance, and never triggers Rust rollback.'
} | ConvertTo-Json -Depth 8
