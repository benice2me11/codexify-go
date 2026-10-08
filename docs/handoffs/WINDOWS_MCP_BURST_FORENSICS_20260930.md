# Windows MCP burst: forensic continuation, 2026-09-30

## Outcome and operational boundary

The investigation found FOUR dense windows, not only the two described in the
incoming handoff. Their total is 16,343 `dispatcher forwarded command to MCP
server` log records. The saved file has 16,512 such records overall.

No burst root cause or production fix has been established. In particular,
distinct upstream trace IDs do not prove distinct logical tool operations, and
the forwarded-log timestamp is not a command-arrival timestamp.

Windows stayed on Rust throughout this investigation. Repeated live checks found
`CodexifyGo` Stopped / Disabled, with Rust `codexify.exe` PIDs 11404 and 12692.
No service, scheduled task, connector configuration, MCP lifecycle, dispatcher,
or installed runtime binary was changed. Only offline analysis code and reports
were added. No commit, push, or cutover was performed.

## 1. Inputs and reproducibility

Active checkout: `C:\Users\FoxOS_User\codexify-go`.
Baseline HEAD: `1098803` (`fix: support Ctrl-C exec session cancellation`).

Primary saved log:

```text
C:\Users\FoxOS_User\.codexify-go\codexify-go.log
13,148,591 bytes
83,047 JSONL records; zero malformed JSON lines
SHA256 b093a2384e7e531898c6b6dc6505a81808245bc17a18501272240301823b3f51
```

The hash was checked before running the analyzer. The log belongs to the stopped
Go runtime, not the currently active Rust connector.

Added reproducible analyzer: `tools/forensics/burstlog/`.
Generated statistics: `docs/handoffs/evidence/WINDOWS_MCP_BURST_20260929.json`.
The JSON contains full-file statistics, all four selected windows, field names,
missing-metadata counts, instance IDs, and three sample command IDs per window.
It does not contain request/response bodies or arguments. It still contains
local paths and correlation identifiers; review it before sharing externally.

```powershell
go run ./tools/forensics/burstlog -log 'C:\Users\FoxOS_User\.codexify-go\codexify-go.log'
go run ./tools/forensics/burstlog -log 'C:\Users\FoxOS_User\.codexify-go\codexify-go.log' -from '2026-09-29T19:57:00+03:00' -to '2026-09-29T20:08:00+03:00'
```

The analyzer uses only the standard library, opens the source read-only, and does
not start a connector or send network requests. It distinguishes numeric and
string RPC IDs, counts malformed/missing data, rejects reversed windows and
scanner/read failures, and hashes the entire source even for a selected window.

## 2. Four windows, not two

All times below are on 2026-09-29 at UTC+03:00. Bounds are inclusive/exclusive.
These are selected minute windows; their first log records are not necessarily
the precise onset of abnormal behavior.

| Window | Forwarded records | Unique envelope IDs | Unique full cmd IDs | Unique cmd prefixes before `/` |
| --- | ---: | ---: | ---: | ---: |
| [17:42, 17:51) | 5,297 | 5,297 | 5,297 | 5,285 |
| [19:44, 19:49) | 2,980 | 2,980 | 2,980 | 2,975 |
| [19:57, 20:08) | 6,438 | 6,438 | 6,438 | 6,418 |
| [21:11, 21:14) | 1,628 | 1,628 | 1,628 | 1,615 |

The four windows contain 16,343 forwarded records; 169 additional forwarded
records lie outside them. Uniqueness values are per window, not a claim of
cross-window logical-operation uniqueness.

First/last forwarded log timestamps in each selected window:

```text
17:42:00.464636  .. 17:50:52.2005715
19:44:01.5506979 .. 19:48:30.5819459
19:57:18.3143336 .. 20:07:40.2639194
21:11:32.6895841 .. 21:13:26.2880027
```

The first window was absent from the incoming handoff and predates the recorded
19:44:11 author timestamp of HEAD 1098803. Do not attribute onset to that commit
without establishing the deployed binary/build timeline.

The first window uses one client instance:
`2598230f1e9beb7bef25d0f48f807dd7`.
Its first/last occurrences in the saved log are 17:22:31.6508157 and
19:28:33.5586894. The other three windows all use
`ca2c7100bcab71597d64cd70c1fed34b`, first/last seen at 19:42:33.784162 and
21:13:26.2880027. These are observed log ranges, not independent OS process
lifetime measurements. No instance-ID churn appears among the forwarded records
inside an individual window.

The familiar 600/min section is confirmed:

```text
20:01 600
20:02 601
20:03 599
20:04 601
20:05 599
20:06 600
```

## 3. Correct the meaning of the forwarded line

Historical source was inspected from the official `openai/tunnel-client` tag
`v0.0.12`, peeled commit `881c9a8fed7cccbe6607cd419863bbca506b8215`.
An isolated static checkout was created at:

```text
C:\Users\FoxOS_User\AppData\Local\Temp\codexify-burst-static-e991c82ab3ca4be18c1b007919db2a74
```

No tunnel executable from that source was launched.

In `pkg/dispatcher/internal/processor.go:636-663`, notifications have a separate
acknowledgement branch and return before the ordinary forwarded line. For a
request, `forwardResponses(...)` is called at line 656; the INFO forwarded line
is emitted afterward at line 660.

Consequences:

- The histogram measures POST-RESPONSE-HANDLING LOG CADENCE, not enqueue, poll,
  arrival, or start-of-execution cadence.
- The line does not unconditionally prove successful response delivery: the
  caller only checks a particular response-deadline condition before emitting it.
- All 16,343 counted records in these four windows carry numeric RPC ID 0 and
  follow the non-notification log path. They are JSON-RPC requests, not thousands
  of standalone `notifications/*` messages.
- ID 0 does not identify the method, tool, conversation, or logical operation.
- The data still cannot distinguish `tools/list`, `tools/call`, `server/discover`,
  or another request method.

More detailed response information is already available on DEBUG paths in this
historical version: `processor.go:912` and `953-979`, including request/response
correlation attributes and `rpc_method`. The INFO line does not retain them.
The observed log does not contain those DEBUG records.

Source: https://github.com/openai/tunnel-client/blob/v0.0.12/pkg/dispatcher/internal/processor.go

## 4. Unique IDs are trace evidence, not logical-operation proof

`pkg/types/types.go:21-50` defines the control-plane request ID as an
`X-Request-Id` tracing identifier on the plugin-service/connectors -> tunnel-service
hop. `processor.go:308-317` imports it from command headers. The inspected source
does not define the UUID prefix before `/` as a stable logical-operation ID.

The 6,438/6,418 result is reproducible, but the correct claim is:

> Many distinct envelope IDs and upstream trace prefixes reached the dispatcher.

It excludes a simple explanation consisting only of repeated log lines with the
same identifiers. It does NOT exclude an upstream retry/reissue/discovery loop
that allocates fresh IDs. It also does not establish that ChatGPT generated
thousands of distinct user-facing tool calls. The original operation and turn
must be correlated separately.

Source: https://github.com/openai/tunnel-client/blob/v0.0.12/pkg/types/types.go

## 5. A local 100 ms constant exists, but is not a diagnosis

`pkg/controlplane/internal/poller.go:20-24` sets a 100 ms queue-full wait and a
maximum poll batch of 25. Lines 128-146 wait when available queue slots are zero;
normal polling and batch enqueueing proceed through a different path. Lines
260-263 log commands polled/enqueued at DEBUG level.

This is not a universal one-command-per-100-ms scheduler. The saved INFO log
contains no queue-occupancy or per-command arrival measurements proving that
this backpressure path caused the observed 10/sec terminal-log section. Do not
turn the matching constant into a root-cause claim.

## 6. What could and could not be recovered

The main log contains no method/tool fields on forwarded records in any of the
four windows. Missing method and tool counts equal the forwarded counts in each
window. Its field inventory contains no raw request payloads that could be
retrospectively decoded into tool names.

There are four earlier `rpc_method=tools/call` / HTTP 401 records at 17:00-17:01.
They do not establish the method of later burst requests. Three response-deadline
records occur at 19:48:24.7121341, 20:02:29.2343033, and 20:02:29.8220297; they
likewise contain no method/tool data.

Bounded auxiliary inspection covered:

- `.codexify-go` runtime/log metadata and saved install/recovery logs;
- binding JSONs, agent-ticket state, and connector-schema artifacts;
- `.codexify/logs` and the 11 discovered temporary Rust tunnel log locations;
- selected incident request/trace/instance IDs in those relevant artifacts.

No additional incident payload record was recovered. The inspected temporary
Rust tunnel logs cover other times, including the pre-cutover interval ending
16:56 and post-rollback interval starting 21:18, rather than the Go burst windows.

The binding schema stores a hashed identity and creation time, not a per-request
conversation/turn transcript. Exact conversation attribution remains unresolved.
The earlier two-turn hypothesis now also needs to explain the newly discovered
17:42 and 21:11 windows. No specific conversation is identified as the culprit.

`pkg/adminui/log_buffer.go:15-17,42-74` provides a default 2,000-event IN-MEMORY ring.
`pkg/adminui/fxmodule.go:125-144` exposes log/export endpoints when that module is
included and enabled. Such a ring is not a historical disk archive; restarting
an exited process cannot recover its former contents. No saved incident export
was found in the locations inspected. This was not an exhaustive search of all
personal files or every possible external archive.

Raw HTTP logging is explicitly marked unsafe by the upstream source and may
expose credentials and request contents. It was NOT enabled.

## 7. Separate Windows observability defect: log level is ignored

`internal/worker/worker.go:32` builds `slog.NewJSONHandler(out, nil)`, without
applying `cfg.Log.Level`. That logger is passed into the MCP runtime at line 49.
In contrast, `internal/app/app.go:37-48` maps the configured level to a LevelVar.

Thus changing the config to debug would not, by itself, enable debug logging in
the Windows user worker. This is a concrete diagnostic/configuration defect,
not an established amplification cause. The historical config inspected here
actually selected INFO, so do not claim an already-requested DEBUG setting was
lost during the incident. No production code fix was made for this in this pass.

## 8. Linux and version-upgrade conclusions remain bounded

The incoming handoff reports a successful Linux cutover with the same Go SDK /
tunnel stack and no analogous burst. That Linux workload was not replayed during
this Windows forensic pass. It argues against the stack combination being a
sufficient cause by itself; it does not rule out platform, configuration,
concurrency, or hosted-orchestration interactions specific to this incident.

The preexisting `v0.0.12 -> v0.0.15` change remains uncommitted and was neither
installed nor labeled a burst fix. No evidence here justifies changing stateless
MCP into legacy stateful behavior, reverting Job Object handling, or patching
`watchTransportSession` as the presumed source of command generation.

## 9. Next evidence needed before a burst fix

Use the exact four time windows and sample `request_id` / `cmd_request_id` values
in the JSON evidence to join hosted traces, when access to those traces is
available. Do not assume this connector has privileged access to hosted logs.
The needed join is:

```text
conversation/turn or originating operation
 -> upstream trace / retry attempt
 -> command envelope
 -> JSON-RPC method and tools/call name
 -> enqueue / poll / local dispatch / terminal response timestamps
 -> response status, RPC error code, and tool-result isError
```

A subsequent local diagnostic change should be explicit and metadata-only:
record actual decoded methods, tool names, allowlisted correlation IDs, stage
times, and response outcome. Do not log arguments, prompts, bearer headers,
cookies, raw results, or entire payloads. A keyed fingerprint of an operation or
conversation may help detect regenerated-ID retries without retaining raw data;
its key must stay out of exported reports. Exact hosted turn attribution still
requires the appropriate trace join.

First exercise this in an isolated harness with synthetic requests, duplicate
payloads under different IDs, notifications, errors, and concurrent calls. Keep
live Windows on Rust until an explicit decision to test a bounded Go cutover.
Neither a log-level change nor additional telemetry alone is a burst fix.

## 10. Changes and verification in this pass

Added only:

```text
tools/forensics/burstlog/main.go
tools/forensics/burstlog/main_test.go
tools/forensics/burstlog/README.md
docs/handoffs/evidence/WINDOWS_MCP_BURST_20260929.json
docs/handoffs/WINDOWS_MCP_BURST_FORENSICS_20260930.md
```

Existing dirty files left untouched:

```text
docs/handoffs/RUST_TO_GO_CUTOVER_HANDOFF.md
internal/mcpserver/server_test.go
internal/projects/manager.go
internal/projects/projects_test.go
internal/tunnel/runtime.go
docs/handoffs/WINDOWS_CUTOVER_REPORT.md
```

Executed successfully:

```text
gofmt on the two new Go files
go test ./tools/forensics/burstlog -count=1 -v
  TestWindowIdentityAndNoPayloadLeak PASS
  TestMalformedRecordsAndMissingIdentifiers PASS
  TestInvalidRangeAndReaderError PASS
go build of the standalone offline burstlog analyzer to a new temporary path
go test ./internal/mcpserver -run '^TestCurrentProtocolRequestsUseEphemeralSessions$' -count=1 -v
  PASS (existing pending test; isolated httptest server, no tunnel)
```

The full repository test suite was not rerun in this pass. The characterization
test confirms the observed stateless lifecycle; it does not reproduce or resolve
the hosted burst. Historical log statistics are reproducible; method/turn
attribution and the root cause remain unproven.
