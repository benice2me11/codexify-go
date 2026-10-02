# Offline MCP burst log analysis

Read an existing Codexify Go JSONL log without starting a connector, contacting
a control plane, or modifying the input. Only standard-library Go packages are
used. Output is JSON on stdout; errors go to stderr with a nonzero exit status.

From the repository root on Windows:

```powershell
go run ./tools/forensics/burstlog -log 'C:\Users\FoxOS_User\.codexify-go\codexify-go.log'

go run ./tools/forensics/burstlog -log 'C:\Users\FoxOS_User\.codexify-go\codexify-go.log' -from '2026-09-29T19:57:00+03:00' -to '2026-09-29T20:08:00+03:00'

go test ./tools/forensics/burstlog -count=1
```

`-from` is inclusive and `-to` exclusive. Both accept RFC3339 timestamps and are
optional. Minute buckets use a fixed UTC offset, default +180 minutes; set
`-offset-minutes 0` for UTC. SHA256 always covers the entire original file,
including records outside the selected window. Analyze a stopped/snapshotted
log rather than a file that is being appended to during the analysis.

The report counts malformed JSON, missing timestamps, missing identifiers,
methods, and tool names explicitly. It preserves numeric versus string RPC IDs.
Overlong lines and read failures abort instead of returning partial statistics
that look complete. The maximum line size is 4 MiB.

The analyzer omits raw request/response bodies, arguments, headers, and result
data. The report still contains local paths, event messages, and correlation
identifiers. Review it before sharing outside the project; it is not a general
purpose secret scrubber for arbitrary free-form log messages.

## Interpretation limits

- For tunnel-client v0.0.12, `dispatcher forwarded command to MCP server` is
  emitted after response handling, not when the command arrives. Its timestamp
  is not an enqueue/poll timestamp or unconditional proof of response delivery.
- Different envelope IDs or prefixes before `/` in `cmd_request_id` do not prove
  distinct logical operations. Retries can receive new upstream trace IDs.
- A JSON-RPC ID is not a method name, tool name, conversation ID, or turn ID.
- Missing payload metadata cannot be reconstructed by this tool. The result
  reports the gap rather than classifying commands from session lifecycle lines.

See `docs/handoffs/WINDOWS_MCP_BURST_FORENSICS_20260930.md` for the investigation.
