param(
    [Parameter(Mandatory = $true)][string]$RunDirectory
)
$ErrorActionPreference = 'Stop'
$baseEvidence = Join-Path $RunDirectory 'evidence\continuation-20261001-rust-baseline'
$ev = Join-Path $baseEvidence 'metadata-modern-v2'
$fixture = Join-Path $baseEvidence 'loopback-fixture'
$profilePath = Join-Path $fixture 'offline-profile.json'
$binary = Join-Path $RunDirectory 'candidate\codexify-go.exe'
$expectedHash = 'B0B421A0E3045B7F71ACD2ADF1C8079EAC2D87358091590A9BD467E36E798999'
if ((Get-FileHash -LiteralPath $binary).Hash -ne $expectedHash) { throw 'Candidate hash mismatch' }
$profile = Get-Content -LiteralPath $profilePath -Raw | ConvertFrom-Json
if ($profile.tunnel.tunnelId -ne 'tunnel_offline_candidate_probe_20261001' -or
    $profile.tunnel.mcpServerUrl -ne 'http://127.0.0.1:0/mcp/offline-candidate-probe' -or
    @($profile.mcp.upstreams).Count -ne 0) { throw 'Not the isolated profile' }
if (Test-Path -LiteralPath $ev) { throw 'Probe evidence already exists' }
New-Item -ItemType Directory -Path $ev | Out-Null

function Get-Owners {
    Get-CimInstance Win32_Process |
        Where-Object { $_.Name -match '^codexify|^tunnel-client' } |
        Select-Object Name, ProcessId, ParentProcessId, CreationDate, ExecutablePath
}
$before = @(Get-Owners)
$requests = New-Object 'System.Collections.Generic.List[object]'
$script:sequence = 0
$script:endpoint = ''
$script:headers = @{}

function Invoke-ProbeRpc {
    param([string]$Method, [hashtable]$Params, [string]$File)
    $script:sequence++
    # Match the inspected v1.7.0 SDK's per-request modern protocol contract.
    $Params['_meta'] = @{
        'io.modelcontextprotocol/protocolVersion' = '2026-07-28'
        'io.modelcontextprotocol/clientInfo' = @{ name = 'offline-candidate-probe'; version = '1' }
        'io.modelcontextprotocol/clientCapabilities' = @{}
    }
    $requestHeaders = $script:headers.Clone()
    $requestHeaders['Mcp-Method'] = $Method
    if ($Method -eq 'resources/read') { $requestHeaders['Mcp-Name'] = $Params.uri }
    $body = @{ jsonrpc = '2.0'; id = $script:sequence; method = $Method; params = $Params } |
        ConvertTo-Json -Depth 10 -Compress
    $watch = [Diagnostics.Stopwatch]::StartNew()
    try {
        $reply = Invoke-WebRequest -Uri $script:endpoint -Method Post -ContentType 'application/json' `
            -Headers $requestHeaders -Body $body -UseBasicParsing -TimeoutSec 10
    } catch {
        if ($_.Exception.Response) {
            $reader = [IO.StreamReader]::new($_.Exception.Response.GetResponseStream())
            try { $errorBody = $reader.ReadToEnd() } finally { $reader.Dispose() }
            [IO.File]::WriteAllText((Join-Path $ev ($File + '.http-error.txt')), $errorBody)
        }
        throw
    }
    $watch.Stop()
    $content = [string]$reply.Content
    $rpc = $content | ConvertFrom-Json
    [IO.File]::WriteAllText((Join-Path $ev $File), $content, [Text.UTF8Encoding]::new($false))
    $requests.Add([pscustomobject]@{
        id = $script:sequence; method = $Method; status = [int]$reply.StatusCode
        durationMs = $watch.ElapsedMilliseconds; responseFile = $File
    })
    if ($rpc.error) { throw ('RPC ' + $Method + ' returned code ' + $rpc.error.code) }
    if ($rpc.id -ne $script:sequence) { throw 'RPC response ID mismatch' }
    return $rpc.result
}

$secret = New-Object byte[] 32
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
try { $rng.GetBytes($secret) } finally { $rng.Dispose() }
$token = [Convert]::ToBase64String($secret)
$oldHome = $env:USERPROFILE
$oldAuth = $env:CODEXIFY_GO_INTERNAL_MCP_AUTHORIZATION
$probe = $null
$capture = [ordered]@{
    startedAtUtc = [DateTime]::UtcNow.ToString('o')
    candidateSha256 = $expectedHash
    profileSha256 = (Get-FileHash -LiteralPath $profilePath).Hash
    hostedTunnelStarted = $false
    upstreamsConnected = 0
    productionConfigurationUsed = $false
    status = 'RUNNING'
}
try {
    # The worker entry point starts only MCP, not the tunnel or an SCM service.
    try {
        $env:USERPROFILE = Join-Path $fixture 'home'
        $env:CODEXIFY_GO_INTERNAL_MCP_AUTHORIZATION = 'Bearer ' + $token
        $probe = Start-Process -FilePath $binary -ArgumentList @(
            'worker', 'run', '--config', ('"' + $profilePath + '"')
        ) -WindowStyle Hidden -PassThru `
          -RedirectStandardOutput (Join-Path $ev 'worker-stdout.txt') `
          -RedirectStandardError (Join-Path $ev 'worker-stderr.txt')
    } finally {
        $env:USERPROFILE = $oldHome
        $env:CODEXIFY_GO_INTERNAL_MCP_AUTHORIZATION = $oldAuth
    }
    $capture.probePid = $probe.Id
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    $listener = $null
    do {
        if ($probe.HasExited) { throw ('Probe exited ' + $probe.ExitCode) }
        $listener = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue |
            Where-Object { $_.OwningProcess -eq $probe.Id -and $_.LocalAddress -eq '127.0.0.1' } |
            Select-Object -First 1
        if (!$listener) { Start-Sleep -Milliseconds 200 }
    } while (!$listener -and [DateTime]::UtcNow -lt $deadline)
    if (!$listener) { throw 'No isolated listener within 15 seconds' }
    $script:endpoint = 'http://127.0.0.1:' + $listener.LocalPort + '/mcp/offline-candidate-probe'
    $capture.endpoint = $script:endpoint
    $script:headers = @{
        Authorization = 'Bearer ' + $token
        Accept = 'application/json, text/event-stream'
        'MCP-Protocol-Version' = '2026-07-28'
    }
    $discovery = Invoke-ProbeRpc 'server/discover' @{} 'server-discover.json'
    $tools = Invoke-ProbeRpc 'tools/list' @{} 'tools-list.json'
    $resources = Invoke-ProbeRpc 'resources/list' @{} 'resources-list.json'
    if (@($tools.tools).Count -lt 10) { throw 'Unexpectedly small tool catalog' }
    if ($tools.nextCursor -or $resources.nextCursor) { throw 'Pagination requires explicit continuation' }
    $hashes = @()
    $index = 0
    foreach ($resource in $resources.resources) {
        $index++
        $file = 'resource-' + $index + '.json'
        $content = Invoke-ProbeRpc 'resources/read' @{ uri = $resource.uri } $file
        foreach ($entry in $content.contents) {
            if ($entry.text) {
                $hash = [Security.Cryptography.SHA256]::Create()
                try {
                    $bytes = [Text.Encoding]::UTF8.GetBytes([string]$entry.text)
                    $h = ([BitConverter]::ToString($hash.ComputeHash($bytes))).Replace('-', '')
                } finally { $hash.Dispose() }
                $hashes += [pscustomobject]@{
                    uri = $entry.uri; mimeType = $entry.mimeType
                    textSha256 = $h; responseFile = $file; textBytes = $bytes.Length
                }
            }
        }
    }
    foreach ($wanted in @(
        'ui://codexify-go/setup/v2/mcp-app.html',
        'ui://codexify-go/self-update/v2/mcp-app.html'
    )) {
        if ($wanted -notin $hashes.uri) { throw ('Missing shipped resource ' + $wanted) }
    }
    $capture.toolsCount = @($tools.tools).Count
    $capture.resourcesCount = @($resources.resources).Count
    $capture.toolsResponseSha256 = (Get-FileHash -LiteralPath (Join-Path $ev 'tools-list.json')).Hash
    $capture.discovery = $discovery
    $capture.resources = $hashes
    $capture.status = 'PASS'
    Write-Output ('CAPTURED_TOOLS=' + $capture.toolsCount)
    Write-Output ('CAPTURED_RESOURCES=' + $capture.resourcesCount)
    $hashes | Format-Table -AutoSize
} catch {
    $capture.status = 'FAIL'
    $capture.failureType = $_.Exception.GetType().FullName
    $capture.failure = $_.Exception.Message
    Write-Output ('PROBE_FAILED=' + $_.Exception.Message)
} finally {
    if ($probe -and !$probe.HasExited) {
        $owned = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $probe.Id)
        if ($owned.ExecutablePath -ne $binary -or
            $owned.CommandLine -notlike '*worker run*' -or
            $owned.CommandLine -notlike '*offline-profile.json*') {
            throw 'Probe cleanup ownership mismatch'
        }
        Stop-Process -Id $probe.Id -ErrorAction Stop
        $probe.WaitForExit(5000) | Out-Null
    }
    $capture.probeStopped = ($null -eq $probe -or $probe.HasExited)
    $capture.requests = $requests.ToArray()
    $capture.endedAtUtc = [DateTime]::UtcNow.ToString('o')
    $capture | ConvertTo-Json -Depth 25 |
        Set-Content -LiteralPath (Join-Path $ev 'candidate-metadata.json') -Encoding UTF8
    $after = @(Get-Owners)
    [ordered]@{ before = $before; after = $after } | ConvertTo-Json -Depth 5 |
        Set-Content -LiteralPath (Join-Path $ev 'probe-ownership.json') -Encoding UTF8
    Write-Output ('PROBE_STOPPED=' + $capture.probeStopped)
    $after | Format-Table Name, ProcessId, ParentProcessId, CreationDate -AutoSize
}
if ($capture.status -ne 'PASS' -or !$capture.probeStopped) { exit 1 }
Write-Output 'NATIVE_BINARY_METADATA_CAPTURE=PASS'
