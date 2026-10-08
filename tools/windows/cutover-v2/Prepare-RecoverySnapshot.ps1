[CmdletBinding(DefaultParameterSetName='Validate')]
param(
    [Parameter(Mandatory=$true)][string]$Destination,
    [Parameter(ParameterSetName='Validate')][switch]$ValidateOnly,
    [Parameter(ParameterSetName='Capture')][switch]$Capture
)
$ErrorActionPreference = 'Stop'
$toolDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Import-Module (Join-Path $toolDir 'CutoverGuard.psm1') -Force

if ($ValidateOnly) {
    $manifest = Test-RecoverySnapshot -RecoveryDirectory $Destination
    [pscustomobject]@{
        status='valid'
        version=[int]$manifest.version
        files=@($manifest.files).Count
    } | ConvertTo-Json -Compress
    exit 0
}
if (!$Capture) {
    throw 'Use -Capture explicitly to create a recovery snapshot, or -ValidateOnly to verify one.'
}

$dest = [IO.Path]::GetFullPath($Destination)
if (Test-Path -LiteralPath $dest) {
    if (Get-ChildItem -LiteralPath $dest -Force -ErrorAction SilentlyContinue) {
        throw "Recovery destination must be empty: $dest"
    }
} else {
    New-Item -ItemType Directory -Force -Path $dest | Out-Null
}

$rustRoot = [IO.Path]::GetFullPath((Join-Path $env:USERPROFILE '.codexify'))
$required = @(
    'bin\codexify.exe',
    'codexify.config.json',
    'watchdog.ps1'
)
foreach ($rel in $required) {
    if (!(Test-Path -LiteralPath (Join-Path $rustRoot $rel) -PathType Leaf)) {
        throw "Required Rust recovery file is missing: $rel"
    }
}

$rustDest = Join-Path $dest 'rust'
New-Item -ItemType Directory -Force -Path $rustDest | Out-Null
$copyItems = @(
    'bin',
    'connector-schemas',
    'conversation-projects',
    'openai-tunnel',
    'projects',
    'codexify.config.json',
    'watchdog.ps1'
)
foreach ($rel in $copyItems) {
    $src = Join-Path $rustRoot $rel
    if (!(Test-Path -LiteralPath $src)) { continue }
    $dst = Join-Path $rustDest $rel
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $dst) | Out-Null
    Copy-Item -LiteralPath $src -Destination $dst -Recurse
}

$apiKeyOriginalPath = $null
$config = Get-Content -LiteralPath (Join-Path $rustRoot 'codexify.config.json') -Raw | ConvertFrom-Json
$apiKeyRef = $null
if ($config.openaiTunnel -and $config.openaiTunnel.apiKeyRef) {
    $apiKeyRef = [string]$config.openaiTunnel.apiKeyRef
}
if ($apiKeyRef -and $apiKeyRef.StartsWith('file:',[StringComparison]::OrdinalIgnoreCase)) {
    $raw = $apiKeyRef.Substring(5)
    if ([IO.Path]::IsPathRooted($raw)) { $apiKeyOriginalPath = [IO.Path]::GetFullPath($raw) }
    else { $apiKeyOriginalPath = [IO.Path]::GetFullPath((Join-Path $rustRoot $raw)) }
    if (!(Test-Path -LiteralPath $apiKeyOriginalPath -PathType Leaf)) {
        throw 'Rust API-key file reference exists but the referenced file is unavailable.'
    }
    $privateDir = Join-Path $rustDest 'private'
    New-Item -ItemType Directory -Force -Path $privateDir | Out-Null
    Copy-Item -LiteralPath $apiKeyOriginalPath -Destination (Join-Path $privateDir 'api-key')
}

$tasksDir = Join-Path $dest 'tasks'
New-Item -ItemType Directory -Force -Path $tasksDir | Out-Null
$taskStates = @()
foreach ($name in @('Codexify','Codexify Watchdog')) {
    $task = Get-ScheduledTask -TaskPath '\' -TaskName $name -ErrorAction Stop
    $xml = Export-ScheduledTask -TaskPath '\' -TaskName $name
    [IO.File]::WriteAllText((Join-Path $tasksDir ($name + '.xml')),$xml,[Text.UTF8Encoding]::new($false))
    $info = Get-ScheduledTaskInfo -InputObject $task
    $taskStates += [pscustomobject]@{
        name=$name
        enabled=($task.State -ne 'Disabled')
        state=[string]$task.State
        lastRunTime=$info.LastRunTime.ToString('o')
        lastTaskResult=$info.LastTaskResult
    }
}

$entries = @()
Get-ChildItem -LiteralPath $dest -File -Recurse |
    Where-Object { $_.Name -ne 'manifest.json' } |
    Sort-Object FullName |
    ForEach-Object {
        $relative = $_.FullName.Substring($dest.Length).TrimStart('\')
        $entries += [pscustomobject]@{
            path=$relative
            sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash
            length=$_.Length
        }
    }

$manifest = [ordered]@{
    version=1
    capturedAtUtc=[DateTime]::UtcNow.ToString('o')
    liveRustRoot=$rustRoot
    rustExecutableSha256=(Get-FileHash -LiteralPath (Join-Path $rustRoot 'bin\codexify.exe') -Algorithm SHA256).Hash
    apiKeyOriginalPath=$apiKeyOriginalPath
    taskStates=$taskStates
    files=$entries
}
$manifest | ConvertTo-Json -Depth 8 |
    Set-Content -LiteralPath (Join-Path $dest 'manifest.json') -Encoding UTF8

$null = Test-RecoverySnapshot -RecoveryDirectory $dest
[pscustomobject]@{
    status='captured'
    destination=$dest
    files=$entries.Count
    rustExecutableSha256=$manifest.rustExecutableSha256
    taskCount=$taskStates.Count
} | ConvertTo-Json -Compress
