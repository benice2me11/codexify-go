# Bounded MCP metadata capture and offline analysis

The recorder is disabled by default. It does not enable a tunnel, start a
connector, change MCP protocol routing, retry tools, or deduplicate operations.
Keep the Windows live connector on Rust during the burst investigation.

## Configuration for an explicitly authorized isolated run

Merge this into a separate test configuration, not the installed live config:

```json
{
  "mcp": {
    "diagnostics": {
      "enabled": true,
      "directory": "./private-test-traces",
      "maxEvents": 20000,
      "maxDuration": "10m"
    }
  }
}
```

Paths loaded through the config loader are relative to the config file. When
constructing a Runtime directly in Go tests, pass an explicit temporary path.
The directory must be writable and private to the test user. Unix creation modes
are directory 0700/file 0600; Windows inherits directory ACLs, so numeric modes
alone do not establish a Windows access-control boundary.

Each runtime capture gets a new exclusive file and a random in-memory HMAC key.
No key is exported. A normal request contributes four events: HTTP start, decoded
MCP start, MCP end, HTTP end. Capture start/stop also consume the event budget.
Thus 20,000 events cover fewer than 5,000 such requests, not 20,000 calls.
The allowed event limit is 4..100,000 and duration must be >0 and <=1 hour.

The recorder stops permanently for that runtime when the event/time limit is
observed. Duration is checked on activity rather than by a background timer.
Limits can leave an unmatched start; this is explicit in analysis. There is no
automatic rotation, re-arm, retention cleanup, or cross-restart aggregate budget.
Existing captures are not overwritten. Explicitly disable capture after an
approved experiment and apply the project's retention policy to the files.

## What is recorded

- Local HTTP and decoded-MCP start/end timestamps, local sequence IDs, duration,
  HTTP status and response byte count. These are NOT hosted enqueue/poll times
  or proof of response delivery to the hosted control plane.
- Decoded method, allowlisted registered tool name, tool isError, outcome class,
  and numeric JSON-RPC error code when the SDK returns one to the middleware.
- A strictly UUID-shaped X-Request-Id header for trace joining, plus HMACs of trace,
  transport session, conversation identity, and operation inputs. Header values
  are unverified correlation data, not authenticated operation identities.

Request/response bodies, arguments, prompts, results, raw errors, bearer headers,
cookies, and the HMAC key are NOT written. Unknown method/tool names become
OTHER plus a keyed hash. Only the dedicated diagnostic JSONL file has this
metadata-only contract; ordinary runtime/third-party debug logs are separate.

An operation fingerprint is HMAC(method + canonical JSON parameters excluding
_top-level_ `_meta`). JSON object key order is normalized and numbers are not
rounded through float64. It is not full semantic equivalence: numeric spellings,
array order, omitted defaults, nested metadata, or ticket parameters can differ.
Parameters over 64 KiB omit the operation hash and record a missing-fingerprint
reason. Hashes can be compared only within the same capture. A repeated hash is
a repeat candidate, NOT proof of a retry and never a deduplication instruction.

HTTP validation/auth failures can occur before decoded receiving middleware;
they have HTTP-only records. Notifications are marked separately. Raw JSON-RPC
IDs are not available through the receiving interface and are not reconstructed.
A local call_id/http_id is not the original command envelope ID or hosted turn ID.
No hosted conversation/turn mapping is guessed.

Capture initialization errors fail startup of the explicitly configured test
runtime. Later diagnostic write errors stop capture and produce one safe warning,
but do not turn successful tool execution into an error or retry. Writes are
synchronous and consume local I/O; no claim of zero latency overhead is made.

## Analyze a stopped capture

```powershell
go run ./tools/forensics/mcptrace -log 'C:\private-test-traces\mcp-trace-....jsonl'
go run ./tools/forensics/mcptrace -log 'C:\private-test-traces\mcp-trace-....jsonl' -max-groups 20
```

No service or network is contacted. JSON is printed to stdout. The report groups
methods, tools, outcomes and operation/conversation hashes, distinguishes starts
from ends, lists up to three trace-ID samples per operation group, and includes
the SHA256 of the input. max-groups accepts 1..100.

Malformed JSON, unknown fields/phases, mixed capture IDs, sequence gaps,
duplicated IDs, and overlong input fail rather than being silently skipped.
A crash-interrupted capture with valid complete lines and no stop event is
reported as missing_stop. Unmatched starts are counted rather than invented as
successful completions. A physically truncated JSON line is a parse failure.
The analyzer expects recorder-generated captures, not arbitrary untrusted logs;
it is not a general-purpose secret scrubber or cryptographic authenticity check.

## Isolated verification

```powershell
go test ./internal/mcpdiag ./internal/mcpserver ./internal/config ./internal/logging ./tools/forensics/mcptrace -count=1
go test ./internal/mcpdiag -run '^TestIsolatedConcurrentCallsNoAmplification$' -count=10
go test ./internal/ui -run '^TestWidgetsDoNotCallToolsFromHostGlobals$' -count=1 -v
```

The load test sends 1,024 requests through a synthetic loopback SDK endpoint,
with 16 workers and regenerated trace IDs. It checks one execution per request,
matched HTTP/MCP event pairs, one operation hash for equal inputs, no retained
stateless sessions, and no fixture secrets in the capture. It does not launch
tunnel-client or contact the hosted control plane.

The embedded widget test runs the actual shipped inline JS using Node's built-in
vm with a bounded synthetic host. Node is a test-only dependency; without Node
that Go test reports SKIP. CI should explicitly provide Node when using this gate.

The original log analyzer remains separate at `tools/forensics/burstlog`: old
INFO logs cannot be retrospectively converted to metadata captures.
