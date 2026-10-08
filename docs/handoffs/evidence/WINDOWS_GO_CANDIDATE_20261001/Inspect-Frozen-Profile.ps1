param(
    [Parameter(Mandatory = $true)][string]$RunDirectory,
    [Parameter(Mandatory = $true)][string]$ProfilePath,
    [Parameter(Mandatory = $true)][string]$EvidenceDirectory,
    [Parameter(Mandatory = $true)][string]$ExpectedHash,
    [int]$ExpectedUpstreamCount = 3
)
$ErrorActionPreference = 'Stop'

$binary = Join-Path $RunDirectory 'candidate\codexify-go.exe'
if ((Get-FileHash -LiteralPath $binary).Hash -ne $ExpectedHash) { throw 'Candidate hash mismatch' }
if (!(Test-Path -LiteralPath $ProfilePath)) { throw 'Profile not found' }

$profile = Get-Content -LiteralPath $ProfilePath -Raw | ConvertFrom-Json
$url = [uri]$profile.tunnel.mcpServerUrl
if ($url.Host -notin @('127.0.0.1','localhost','::1')) { throw 'Probe MCP URL is not loopback' }
if (@($profile.mcp.upstreams).Count -ne $ExpectedUpstreamCount) {
    throw ('Unexpected upstream count: ' + @($profile.mcp.upstreams).Count)
}
New-Item -ItemType Directory -Force -Path $EvidenceDirectory | Out-Null
if (Get-ChildItem -LiteralPath $EvidenceDirectory -Force -ErrorAction SilentlyContinue) {
    throw 'Evidence directory is not empty'
}

function Get-Owners {
    Get-CimInstance Win32_Process |
        Where-Object { $_.Name -match '^codexify|^tunnel-client' } |
        Select-Object Name, ProcessId, ParentProcessId, CreationDate, ExecutablePath
}
$before = @(Get-Owners)

$requests = New-Object 'System.Collections.Generic.List[object]'
$script:sequence = 0
$script:endpoint = ''
$script:baseHeaders = @{}

function Invoke-ProbeRpc {
    param([string]$Method, [hashtable]$Params, [string]$File, [string]$Name = '')
    $script:sequence++
    $Params['_meta'] = @{
        'io.modelcontextprotocol/protocolVersion' = '2026-07-28'
        'io.modelcontextprotocol/clientInfo' = @{ name = 'frozen-profile-probe'; version = '1' }
        'io.modelcontextprotocol/clientCapabilities' = @{}
    }
    $headers = $script:baseHeaders.Clone()
    $headers['Mcp-Method'] = $Method
    if ($Name) { $headers['Mcp-Name'] = $Name }
    elseif ($Method -eq 'resources/read') { $headers['Mcp-Name'] = $Params.uri }

    $body = @{ jsonrpc = '2.0'; id = $script:sequence; method = $Method; params = $Params } |
        ConvertTo-Json -Depth 12 -Compress
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $reply = Invoke-WebRequest -Uri $script:endpoint -Method Post -ContentType 'application/json' -Headers $headers -Body $body -UseBasicParsing -TimeoutSec 15
    $watch.Stop()
    $content = [string]$reply.Content
    [IO.File]::WriteAllText((Join-Path $EvidenceDirectory $File), $content, [Text.UTF8Encoding]::new($false))
    $rpc = $content | ConvertFrom-Json
    $requests.Add([pscustomobject]@{
        id=$script:sequence; method=$Method; name=$Name; status=[int]$reply.StatusCode
        durationMs=$watch.ElapsedMilliseconds; responseFile=$File
    })
    if ($rpc.error) { throw ('RPC ' + $Method + ' returned code ' + $rpc.error.code + ': ' + $rpc.error.message) }
    if ($rpc.id -ne $script:sequence) { throw 'RPC response ID mismatch' }
    return $rpc.result
}

$secret = New-Object byte[] 32
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
try { $rng.GetBytes($secret) } finally { $rng.Dispose() }
$token = [Convert]::ToBase64String($secret)
$oldAuth = $env:CODEXIFY_GO_INTERNAL_MCP_AUTHORIZATION
$probe = $null
$capture = [ordered]@{
    startedAtUtc=[DateTime]::UtcNow.ToString('o')
    candidateSha256=$ExpectedHash
    profileSha256=(Get-FileHash -LiteralPath $ProfilePath).Hash
    hostedTunnelStarted=$false
    status='RUNNING'
}
try {
    try {
        $env:CODEXIFY_GO_INTERNAL_MCP_AUTHORIZATION = 'Bearer ' + $token
        $startArgs = @('worker','run','--config',('"' + $ProfilePath + '"'))
        $probe = Start-Process -FilePath $binary -ArgumentList $startArgs -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $EvidenceDirectory 'worker-stdout.txt') -RedirectStandardError (Join-Path $EvidenceDirectory 'worker-stderr.txt')
    } finally {
        $env:CODEXIFY_GO_INTERNAL_MCP_AUTHORIZATION = $oldAuth
    }
    $capture.probePid = $probe.Id

    $deadline = [DateTime]::UtcNow.AddSeconds(20)
    $listener = $null
    do {
        if ($probe.HasExited) { throw ('Probe exited ' + $probe.ExitCode) }
        $listener = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue |
            Where-Object { $_.OwningProcess -eq $probe.Id -and $_.LocalAddress -eq '127.0.0.1' } |
            Select-Object -First 1
        if (!$listener) { Start-Sleep -Milliseconds 200 }
    } while (!$listener -and [DateTime]::UtcNow -lt $deadline)
    if (!$listener) { throw 'No isolated MCP listener within 20 seconds' }

    $script:endpoint = 'http://127.0.0.1:' + $listener.LocalPort + $url.AbsolutePath
    $script:baseHeaders = @{
        Authorization='Bearer ' + $token
        Accept='application/json, text/event-stream'
        'MCP-Protocol-Version'='2026-07-28'
    }

    $discover = Invoke-ProbeRpc 'server/discover' @{} 'server-discover.json'
    $tools = Invoke-ProbeRpc 'tools/list' @{} 'tools-list.json'
    $resources = Invoke-ProbeRpc 'resources/list' @{} 'resources-list.json'
    $sources = Invoke-ProbeRpc 'tools/call' @{name='mcp_list_sources';arguments=@{}} 'sources.json' 'mcp_list_sources'

    $resourceFingerprints = @()
    $resourceIndex = 0
    foreach($resource in @($resources.resources)) {
        $resourceIndex++
        $responseFile = 'resource-' + $resourceIndex + '.json'
        $read = Invoke-ProbeRpc 'resources/read' @{uri=[string]$resource.uri} $responseFile
        foreach($content in @($read.contents)) {
            if($null -eq $content.text) { continue }
            $bytes = [Text.Encoding]::UTF8.GetBytes([string]$content.text)
            $sha = [Security.Cryptography.SHA256]::Create()
            try { $textHash = ([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-','') }
            finally { $sha.Dispose() }
            $resourceFingerprints += [pscustomobject]@{
                uri=[string]$content.uri
                mimeType=[string]$content.mimeType
                textBytes=$bytes.Length
                textSha256=$textHash
                responseFile=$responseFile
            }
        }
    }

    $toolNames = @($tools.tools | ForEach-Object {$_.name})
    foreach($required in @('mcp_list_sources','mcp_search_tools','mcp_get_tool','mcp_call_tool')) {
        if ($required -notin $toolNames) { throw ('Missing catalog tool ' + $required) }
    }
    if ($sources.isError) { throw 'mcp_list_sources returned an MCP tool error' }
    $sourcePayload = $sources.structuredContent
    if (!$sourcePayload) { $sourcePayload = $sources }
    if (@($sourcePayload.sources).Count -ne $ExpectedUpstreamCount) {
        throw ('Unexpected connected source count: ' + @($sourcePayload.sources).Count)
    }

    $capture.toolsCount=@($tools.tools).Count
    $capture.resourcesCount=@($resources.resources).Count
    $capture.toolCatalogResponseSha256=(Get-FileHash -LiteralPath (Join-Path $EvidenceDirectory 'tools-list.json') -Algorithm SHA256).Hash
    $capture.resourceFingerprints=$resourceFingerprints
    $capture.sources=@($sourcePayload.sources | ForEach-Object {
        [pscustomobject]@{id=$_.id; name=$_.name; toolCount=$_.toolCount; transport=$_.transport}
    })
    $capture.discoverySupportedVersions=@($discover.supportedVersions)
    $capture.status='PASS'
    Write-Output ('TOOLS=' + $capture.toolsCount)
    Write-Output ('RESOURCES=' + $capture.resourcesCount)
    $capture.sources | Format-Table -AutoSize
} catch {
    $capture.status='FAIL'
    $capture.failure=$_.Exception.Message
    Write-Output ('PROFILE_PROBE_FAILED=' + $_.Exception.Message)
} finally {
    if ($probe -and !$probe.HasExited) {
        $owned = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $probe.Id)
        if ($owned.ExecutablePath -ne $binary -or $owned.CommandLine -notlike '*worker run*') {
            throw 'Probe cleanup ownership mismatch'
        }
        Stop-Process -Id $probe.Id -ErrorAction Stop
        $probe.WaitForExit(5000) | Out-Null
    }
    $capture.probeStopped=($null -eq $probe -or $probe.HasExited)
    $capture.requests=$requests.ToArray()
    $capture.endedAtUtc=[DateTime]::UtcNow.ToString('o')
    $capture | ConvertTo-Json -Depth 20 |
        Set-Content -LiteralPath (Join-Path $EvidenceDirectory 'profile-probe.json') -Encoding UTF8
    Start-Sleep -Milliseconds 500
    $after=@(Get-Owners)
    [ordered]@{before=$before;after=$after} | ConvertTo-Json -Depth 5 |
        Set-Content -LiteralPath (Join-Path $EvidenceDirectory 'ownership.json') -Encoding UTF8
    $after | Format-Table Name,ProcessId,ParentProcessId,CreationDate -AutoSize
}
if ($capture.status -ne 'PASS' -or !$capture.probeStopped) { exit 1 }
Write-Output 'FROZEN_PROFILE_PROBE=PASS'
