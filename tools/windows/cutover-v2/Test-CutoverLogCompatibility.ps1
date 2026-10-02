[CmdletBinding()]
param([string]$ModulePath)
$ErrorActionPreference = 'Stop'
if (!$ModulePath) { $ModulePath = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) 'CutoverGuard.psm1' }
Import-Module $ModulePath -Force

$now = [DateTimeOffset]::Parse('2026-10-01T18:00:00Z')
$owner = [pscustomobject]@{ GoOwners=2; RustOwners=0; Controlled=$true }
$failures = [Collections.Generic.List[string]]::new()
$cases = @(
    @{ Name='Go slog msg is readable'; Action='CONTINUE'; Reason='within_limits';
       Tunnel=@('{"time":"2026-10-01T17:59:59Z","level":"INFO","msg":"codexify-go starting"}' | ConvertFrom-Json) },
    @{ Name='Go slog rate limit holds'; Action='HOLD'; Reason='rate_limited';
       Tunnel=@('{"time":"2026-10-01T17:59:59Z","level":"WARN","msg":"RATE_LIMITED"}' | ConvertFrom-Json) },
    @{ Name='Legacy message rate limit holds'; Action='HOLD'; Reason='rate_limited';
       Tunnel=@('{"time":"2026-10-01T17:59:59Z","level":"WARN","message":"RATE_LIMITED"}' | ConvertFrom-Json) },
    @{ Name='Go slog forwarded burst aborts'; Action='ABORT'; Reason='forwarded_60s';
       Tunnel=@(1..101 | ForEach-Object { '{"time":"2026-10-01T17:59:59Z","msg":"dispatcher forwarded command to MCP server"}' | ConvertFrom-Json }) },
    @{ Name='Protocol discovery is not a tools call burst'; Action='CONTINUE'; Reason='within_limits';
       Diagnostic=@(1..10 | ForEach-Object { '{"time":"2026-10-01T17:59:59Z","phase":"mcp_start","rpc_method":"tools/list","operation_hash":"catalog"}' | ConvertFrom-Json }) },
    @{ Name='Protocol discovery is not a repeated tool operation'; Action='CONTINUE'; Reason='within_limits';
       Diagnostic=@(1..5 | ForEach-Object { '{"time":"2026-10-01T17:59:59Z","phase":"mcp_start","rpc_method":"resources/list","operation_hash":"resources"}' | ConvertFrom-Json }) },
    @{ Name='Tools call without optional fingerprint stays observable'; Action='CONTINUE'; Reason='within_limits';
       Diagnostic=@('{"time":"2026-10-01T17:59:59Z","phase":"mcp_start","rpc_method":"tools/call","fingerprint_state":"missing_params"}' | ConvertFrom-Json) },
    @{ Name='Unfingerprinted tool burst still aborts'; Action='ABORT'; Reason='mcp_calls_10s';
       Diagnostic=@(1..10 | ForEach-Object { '{"time":"2026-10-01T17:59:59Z","phase":"mcp_start","rpc_method":"tools/call","fingerprint_state":"missing_params"}' | ConvertFrom-Json }) },
    @{ Name='Repeated tools calls still abort'; Action='ABORT'; Reason='repeated_operation_10s';
       Diagnostic=@(1..5 | ForEach-Object { '{"time":"2026-10-01T17:59:59Z","phase":"mcp_start","rpc_method":"tools/call","operation_hash":"same-operation"}' | ConvertFrom-Json }) },
    @{ Name='Rate limit does not mask a confirmed tool burst'; Action='ABORT'; Reason='mcp_calls_10s';
       Tunnel=@('{"time":"2026-10-01T17:59:59Z","msg":"RATE_LIMITED"}' | ConvertFrom-Json);
       Diagnostic=@(1..10 | ForEach-Object { '{"time":"2026-10-01T17:59:59Z","phase":"mcp_start","rpc_method":"tools/call"}' | ConvertFrom-Json }) }
)

foreach ($case in $cases) {
    try {
        $tunnelEvents = @()
        $diagnosticEvents = @()
        if ($case.ContainsKey('Tunnel')) { $tunnelEvents = @($case.Tunnel) }
        if ($case.ContainsKey('Diagnostic')) { $diagnosticEvents = @($case.Diagnostic) }
        $decision = Get-CutoverGuardDecision -Now $now -OwnerState $owner -Settled -QuietWindow -TunnelEvents $tunnelEvents -DiagnosticEvents $diagnosticEvents
        if ($decision.Action -ne $case.Action -or $decision.Reason -ne $case.Reason) {
            throw "got $($decision.Action)/$($decision.Reason), expected $($case.Action)/$($case.Reason)"
        }
        Write-Output ('PASS: ' + $case.Name)
    } catch {
        $failures.Add($case.Name + ': ' + $_.Exception.Message)
        Write-Output ('FAIL: ' + $failures[$failures.Count - 1])
    }
}
if ($failures.Count) { throw "$($failures.Count) cutover log compatibility checks failed." }
Write-Output "CUTOVER_LOG_COMPATIBILITY=PASS ($($cases.Count) cases)"
