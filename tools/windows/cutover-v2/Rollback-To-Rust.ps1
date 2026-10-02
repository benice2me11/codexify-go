[CmdletBinding(DefaultParameterSetName='DryRun')]
param(
    [Parameter(Mandatory=$true)][string]$RecoveryDirectory,
    [Parameter(Mandatory=$true)][string]$ExpectedGoExecutable,
    [string]$GoServiceName = 'CodexifyGo',
    [Parameter(ParameterSetName='DryRun')][switch]$DryRun,
    [Parameter(ParameterSetName='Execute')][switch]$Execute
)
$ErrorActionPreference = 'Stop'
$toolDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Import-Module (Join-Path $toolDir 'CutoverGuard.psm1') -Force

$recovery = [IO.Path]::GetFullPath($RecoveryDirectory)
$goExe = [IO.Path]::GetFullPath($ExpectedGoExecutable)
$manifest = Test-RecoverySnapshot -RecoveryDirectory $recovery

$actions = @(
    [pscustomobject]@{name='validate_snapshot';detail='Verify every recovery file against manifest SHA256.'},
    [pscustomobject]@{name='validate_go_service_identity';detail='Require CodexifyGo ImagePath to match the expected candidate executable.'},
    [pscustomobject]@{name='stop_disable_go';detail='Stop and disable only the named Go SCM service.'},
    [pscustomobject]@{name='verify_go_tree_gone';detail='Verify/terminate only previously identified candidate-owned processes.'},
    [pscustomobject]@{name='restore_rust_files';detail='Restore captured Rust runtime/config/state to its recorded root.'},
    [pscustomobject]@{name='restore_rust_tasks';detail='Restore the two captured Rust scheduled-task definitions and enabled states.'},
    [pscustomobject]@{name='start_rust';detail='Start only the restored Codexify scheduled task.'},
    [pscustomobject]@{name='verify_rust_ready';detail='Require Rust service status, exact executable and Rust-owned tunnel within 120 seconds.'}
)

if ($DryRun) {
    [pscustomobject]@{
        mode='dry-run'
        recoveryDirectory=$recovery
        expectedGoExecutable=$goExe
        serviceName=$GoServiceName
        actions=$actions
    } | ConvertTo-Json -Depth 6
    exit 0
}
if (!$Execute) {
    throw 'Use -Execute explicitly for a real rollback, or -DryRun to inspect the action plan.'
}
if (-not $manifest.liveRustRoot) {
    throw 'Recovery manifest lacks liveRustRoot; a fixture snapshot cannot be used for live rollback.'
}

function Get-ExecutableFromServicePath([string]$PathName) {
    $trimmed = $PathName.Trim()
    if ($trimmed.StartsWith('"')) {
        $end = $trimmed.IndexOf('"',1)
        if ($end -lt 2) { throw 'Malformed quoted service ImagePath' }
        return $trimmed.Substring(1,$end-1)
    }
    $space = $trimmed.IndexOf(' ')
    if ($space -lt 0) { return $trimmed }
    return $trimmed.Substring(0,$space)
}

function Get-ProcessSnapshot {
    @(Get-CimInstance Win32_Process | Select-Object Name,ProcessId,ParentProcessId,ExecutablePath,CommandLine)
}

$svc = Get-CimInstance Win32_Service -Filter ("Name='" + $GoServiceName.Replace("'","''") + "'") -ErrorAction Stop
if (!$svc) { throw "Go service not found: $GoServiceName" }
$serviceExe = [IO.Path]::GetFullPath((Get-ExecutableFromServicePath ([string]$svc.PathName)))
if (-not $serviceExe.Equals($goExe,[StringComparison]::OrdinalIgnoreCase)) {
    throw "Go service executable mismatch: $serviceExe"
}

$before = Get-ProcessSnapshot
$goRoots = @($before | Where-Object {
    $_.ExecutablePath -and ([IO.Path]::GetFullPath($_.ExecutablePath)).Equals($goExe,[StringComparison]::OrdinalIgnoreCase)
})
$owned = New-Object 'System.Collections.Generic.HashSet[int]'
foreach($p in $goRoots){ [void]$owned.Add([int]$p.ProcessId) }
$changed=$true
while($changed) {
    $changed=$false
    foreach($p in $before) {
        if($owned.Contains([int]$p.ParentProcessId) -and !$owned.Contains([int]$p.ProcessId)) {
            [void]$owned.Add([int]$p.ProcessId)
            $changed=$true
        }
    }
}
$runRoot = Split-Path -Parent (Split-Path -Parent $goExe)

if ($svc.State -ne 'Stopped') {
    Stop-Service -Name $GoServiceName -Force -ErrorAction Stop
}
Set-Service -Name $GoServiceName -StartupType Disabled -ErrorAction Stop

$deadline = [DateTime]::UtcNow.AddSeconds(10)
do {
    $remaining = @(Get-ProcessSnapshot | Where-Object { $owned.Contains([int]$_.ProcessId) })
    if($remaining.Count -eq 0){ break }
    Start-Sleep -Milliseconds 250
} while([DateTime]::UtcNow -lt $deadline)

foreach($p in @(Get-ProcessSnapshot | Where-Object { $owned.Contains([int]$_.ProcessId) })) {
    if(!$p.ExecutablePath){ throw "Owned PID $($p.ProcessId) has no executable path; refusing force termination." }
    $path=[IO.Path]::GetFullPath($p.ExecutablePath)
    $underRun=$path.StartsWith($runRoot.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase)
    if(!$underRun -and -not $path.Equals($goExe,[StringComparison]::OrdinalIgnoreCase)) {
        throw "Owned PID path escaped candidate root: $path"
    }
    Stop-Process -Id $p.ProcessId -Force -ErrorAction Stop
}
Start-Sleep -Milliseconds 500

$afterGo = Get-ProcessSnapshot
$goStill = @($afterGo | Where-Object {
    $_.ExecutablePath -and (
        ([IO.Path]::GetFullPath($_.ExecutablePath)).Equals($goExe,[StringComparison]::OrdinalIgnoreCase) -or
        ([IO.Path]::GetFullPath($_.ExecutablePath)).StartsWith($runRoot.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase)
    )
})
if($goStill.Count -gt 0) {
    throw "Candidate-owned processes remain after stop: $($goStill.ProcessId -join ',')"
}

$rustRoot=[IO.Path]::GetFullPath([string]$manifest.liveRustRoot)
$rustExe=Join-Path $rustRoot 'bin\codexify.exe'
$rustRunning=@(Get-ProcessSnapshot | Where-Object {
    $_.ExecutablePath -and ([IO.Path]::GetFullPath($_.ExecutablePath)).Equals($rustExe,[StringComparison]::OrdinalIgnoreCase)
})
if($rustRunning.Count -gt 0) {
    throw 'Rust is already running before restore; refusing to overwrite a live recovery runtime.'
}

$snapshotRust=Join-Path $recovery 'rust'
Get-ChildItem -LiteralPath $snapshotRust -File -Recurse |
    Where-Object { $_.FullName -notlike (Join-Path $snapshotRust 'private\*') } |
    ForEach-Object {
        $rel=$_.FullName.Substring($snapshotRust.Length).TrimStart('\')
        $dst=Join-Path $rustRoot $rel
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $dst) | Out-Null
        Copy-Item -LiteralPath $_.FullName -Destination $dst -Force
    }
if($manifest.apiKeyOriginalPath) {
    $keyBackup=Join-Path $snapshotRust 'private\api-key'
    if(!(Test-Path -LiteralPath $keyBackup -PathType Leaf)){ throw 'Captured API key file is missing.' }
    $keyTarget=[IO.Path]::GetFullPath([string]$manifest.apiKeyOriginalPath)
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $keyTarget) | Out-Null
    Copy-Item -LiteralPath $keyBackup -Destination $keyTarget -Force
}

foreach($taskState in @($manifest.taskStates)) {
    $name=[string]$taskState.name
    if($name -notin @('Codexify','Codexify Watchdog')) { throw "Unexpected recovery task: $name" }
    $xml=Join-Path $recovery ('tasks\'+$name+'.xml')
    & schtasks.exe /Create /TN ('\'+$name) /XML $xml /F | Out-Null
    if($LASTEXITCODE -ne 0){ throw "Failed to restore scheduled task $name" }
    if([bool]$taskState.enabled){ Enable-ScheduledTask -TaskPath '\' -TaskName $name | Out-Null }
    else { Disable-ScheduledTask -TaskPath '\' -TaskName $name | Out-Null }
}
Start-ScheduledTask -TaskPath '\' -TaskName 'Codexify'

$readyDeadline=[DateTime]::UtcNow.AddSeconds(120)
$ready=$false
do {
    Start-Sleep -Seconds 1
    $procs=Get-ProcessSnapshot
    $rust=@($procs | Where-Object {
        $_.ExecutablePath -and ([IO.Path]::GetFullPath($_.ExecutablePath)).Equals($rustExe,[StringComparison]::OrdinalIgnoreCase)
    })
    $tunnel=@($procs | Where-Object {
        $_.Name -eq 'tunnel-client-runtime.exe' -and $_.ExecutablePath -and
        ([IO.Path]::GetFullPath($_.ExecutablePath)).StartsWith($rustRoot.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase)
    })
    $statusText=& $rustExe service status 2>&1 | Out-String
    if($rust.Count -ge 1 -and $tunnel.Count -eq 1 -and $statusText -match 'Running:\s*yes') {
        $ready=$true
        break
    }
} while([DateTime]::UtcNow -lt $readyDeadline)
if(!$ready){ throw 'Rust did not reach verified local readiness within 120 seconds.' }

[pscustomobject]@{
    mode='executed'
    status='RUST_RECOVERED'
    completedAtUtc=[DateTime]::UtcNow.ToString('o')
    rustExecutable=$rustExe
} | ConvertTo-Json -Compress
