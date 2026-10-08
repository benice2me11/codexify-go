# Windows MCP burst: widget feedback fix and bounded diagnostics

Date: 2026-09-30. Checkout: `C:\Users\FoxOS_User\codexify-go`.
Baseline commit: `1098803bb3861e3f5d1a2702e4f5fda42afe0546`.
Continuation of `WINDOWS_MCP_BURST_FORENSICS_20260930.md`.

## Result: one reproducible amplification mechanism fixed

The embedded Go widgets had a concrete feedback hazard in
`internal/ui/resources.go`:

```text
openai:set_globals
  -> refresh()/check()
  -> window.openai.callTool(...)
  -> host updates widget globals
  -> openai:set_globals
  -> more tool calls
```

The Setup listener unconditionally called `refresh()`, which called both
`setup_status` and `list_projects`. It had no single-flight guard. Chat and
Update listeners likewise called `chat_read` and `self_update_status` on every
globals event. Even unrelated theme/layout globals initiated tool requests.
Diff used a passive renderer and did not have this request-generation path.

This is a SOURCE-LEVEL BUG WITH AN ISOLATED REPRODUCTION, not yet a proven
attribution of every historical request on September 29. The saved incident log
still lacks method/tool/turn fields. A successful host-level reproduction or
trace join is still needed to label this the historical incident's root cause.

Official UI reference, consulted 2026-09-30:
https://developers.openai.com/plugins/reference

It identifies `window.openai.callTool` as a widget-originated MCP call and
`openai:set_globals` as a host-state subscription mechanism. The reference does
not establish which notifications occurred for the user's historical calls.
The test host explicitly simulates result-triggered globals, before and after
promise settlement. No actual ChatGPT call or hosted control plane was used.

## Reproduction and fix

The durable Node harness runs the SHIPPED inline JavaScript, not a rewritten
version of its business logic:

```text
internal/ui/widget_runtime_test.mjs
internal/ui/resources_test.go
```

A synthetic tool-call budget of 64 prevents a runaway test. In the original
source, all three active widgets consumed the entire budget without another
user action. Setup reached 17 concurrent calls in the Node run and 22 in the
Chromium run; these peaks depend on scheduling and are not asserted constants.

| Widget | Original automatic calls under feedback | Fixed initial calls | Fixed calls from 100 unrelated globals |
| --- | ---: | ---: | ---: |
| Setup | 64; stopped by test budget | 2 | 0 |
| Chat | 64; stopped by test budget | 1 | 0 |
| Update | 64; stopped by test budget | 1 | 0 |
| Diff control | 0 | 0 | 0 |

The fix separates passive rendering from commands. Host globals only render
provided tool output. Calls happen during a single initial load or an explicit
button action. No polling timer, retry, trailing refresh, or queued refresh was
introduced. A shared in-flight guard per widget prevents overlapping actions;
buttons are disabled while an action is running and restored on success/error.
Project selection/scratch/switch calls execute once, followed by two finite
status/list reads. Chat send does not erase text edited during the pending send.

Setup, Chat and Update resource URIs were advanced to `/v2/`; Diff stayed `/v1/`.
No protocol downgrade, stateless lifecycle, dispatcher, or Job Object change was
made. Existing installed binaries and configuration were not replaced.

The tests exercise both synchronous and asynchronously delivered host events,
unrelated globals, passive payload rendering, error/no-retry behavior, repeated
Refresh/send/scratch clicks, update force flags, and continued UI usability.
The Node test is test-only and skips explicitly when Node is absent; production
Go builds do not require Node. An effective CI gate must provide Node.

## Chromium interaction validation

Browser plugin was not available in the session. Used the already installed
Python Playwright from the hh-mcp-server virtual environment and existing
Chromium. No dependency installation or user browser profile was required.

All routes were mocked or blocked at `https://widget-fixture.invalid/`.
The flow was widget load -> mock tool results/globals -> Refresh or explicit
mutation -> rendered status/message. Original and fixed HTML were checked.
Desktop viewport: 900x650 for all widgets; Setup also at 390x844.

Confirmed: nonempty DOM; zero page errors; initial/feedback request counts;
Refresh; selecting a project; chat message submission; cleared sent input;
rendered workspace/update status. The host was synthetic, not ChatGPT.
Screenshots were captured, but the connector image viewer rejected their
outside-workspace temporary paths, so no screenshot-based visual review is
claimed. Browser interaction checks and DOM assertions completed successfully.

Temporary browser script, results and screenshots:

```text
C:\Users\FoxOS_User\AppData\Local\Temp\codexify-widget-qa-cfc2f90bbf8e43bfb587589b0b21e8ef\
  check_widgets.py
  results.json
  SetupHTML-900.png
  SetupHTML-390.png
  ChatHTML-900.png
  UpdateHTML-900.png
  DiffHTML-900.png
```

The source widgets existed before the incident: history inspected includes
`d7eeaf4` (2026-09-28 23:03:43 +03, UI migration) and `79fe4b9` (2026-09-28
19:55:01 +03). This establishes chronology, not deployed-binary identity or
which widgets were open. AgentChat was disabled in the inspected incident
config, so the Chat-specific bug is not attributed to that incident.

## Separate Windows logging defect fixed

`internal/worker/worker.go` previously constructed a JSON logger without applying
`cfg.Log.Level`. The supervisor/in-process app honored it. Both now use the same
`internal/logging.NewJSON` helper, with tests for debug/info/warn/warning/error,
case/whitespace, unknown/default values, filtering and valid JSON output.

This fixes a real observability/configuration defect, not by itself command
amplification. The historical config had INFO selected; no historical missing
DEBUG request is alleged.

## Opt-in bounded metadata diagnostics

New package `internal/mcpdiag` adds a dedicated metadata-only capture, disabled
by default and independent of the normal runtime log level. Integration in
`internal/mcpserver/server.go` wraps HTTP admission and decoded SDK receiving
middleware without changing body bytes, response bytes, protocol routing,
returned results/errors, panic propagation, or tool execution count.

It records local HTTP/MCP start/end, local call IDs, actual decoded method and
allowlisted tool name, status/size/duration, numeric RPC error, tool isError,
and per-capture HMACs for trace/session/conversation/operation correlation.
Only strictly UUID-shaped X-Request-Id values are also retained for a possible
hosted trace join. Arguments, bodies, results, error messages, bearer tokens,
cookies and the random HMAC key are not written.

Operation hashes exclude top-level `_meta` and retain numeric precision. Equal
hashes are repeat candidates, NOT proof of retries or grounds for deduplication.
The key is per-capture and is never persisted. A JSON-RPC ID is not guessed from
local sequence numbers. Early HTTP rejections remain HTTP-only observations.

Capture limits: default 20,000 EVENTS and 10 minutes, hard configurable bounds
4..100,000 events and >0..1 hour. Four records normally represent one request.
Stop reasons/unmatched pairs are explicit. Limits stop capture, not tool calls.
Capture initialization errors reject an explicitly misconfigured test startup;
later write failures stop recording with one safe warning, not an RPC failure.
Synchronous local writes have overhead; no zero-overhead assertion is made.

Usage and all privacy/interpretation limits:
`tools/forensics/mcptrace/README.md`.

```powershell
go run ./tools/forensics/mcptrace -log '<stopped metadata capture.jsonl>'
```

The offline analyzer separates arrivals/completions, missing pairs, methods,
tools, statuses, RPC errors and repeated operation/conversation groups. It
rejects corrupted/mixed/truncated JSON rather than manufacturing complete-looking
counts. Original INFO logs are analyzed separately by `burstlog`; missing old
payload metadata cannot be reconstructed by the new recorder.

## Isolated MCP tests and validation

Synthetic SDK test: 1,024 HTTP calls, 16 concurrent workers, regenerated trace
IDs, identical tool inputs. Observed:

```text
sent=1024
executed=1024
http_pairs=1024
mcp_pairs=1024
distinct_trace_ids=1024
operation_hashes=1
retained_sessions=0
```

Ten repeated runs passed (10,240 synthetic requests per ten-run gate). This
shows the tested local path does not itself multiply calls; it is not a replay
of the hosted control plane or proof against every possible concurrency defect.

Actual isolated Codexify runtime tests cover modern `server/discover`, tools/list,
tools/call, legacy initialize/notifications/retained-session lifecycle, malformed
JSON, unauthorized HTTP requests, optional diagnostics disabled, and HTTP/MCP
correlation. Additional tests cover secrets, large parameters, precision,
canonical key order, unknown metadata, errors, cancellation classifications,
panics, capture limits, disk errors, EOF/corruption and the offline CLI.

Passed in this continuation:

```text
go test ./... -count=1
go vet ./...
go test ./internal/ui -count=10
go test ./internal/mcpdiag -run '^TestIsolatedConcurrentCallsNoAmplification$' -count=10
git diff --check
```

Compile-only `go build ./...` passed for windows/amd64, windows/arm64,
linux/amd64, linux/arm64, darwin/amd64 and darwin/arm64. This does not establish
native service behavior on other OSes. The race detector was not run: CGO was
disabled and no gcc/clang toolchain was found in PATH or checked conventional
locations. No compiler was installed solely for this pass.

## Operational boundary and next justified deployment check

Rust remained live; `CodexifyGo` remained Stopped/Disabled. No real tunnel was
launched, no hosted trace request was sent, and no original source log was
modified. The pending v0.0.15 runtime upgrade and preexisting project/path/test
changes were left separate. No commit, push, service/task mutation or cutover.

Before any later explicitly approved Go test: ensure old Go widget iframes are
closed, deploy a separately verified build, refresh connector tools to obtain
v2 resource references, and open a fresh widget. An already mounted old iframe
keeps its old JavaScript even after server code changes. Test one controlled
widget action with bounded capture, then join method/tool/operation fingerprints
and actual trace IDs. Do not claim that stale widgets are repaired by changing
server source alone.

The four historical windows still require exact method/turn attribution. Existing
unique envelope IDs and stable tunnel instances are consistent with UI-originated
calls as well as other upstream request sources. They never established that
hosted orchestration, rather than a local widget, generated the amplification.
The current strongest new result is the reproduced and fixed widget feedback
mechanism; the historical incident attribution remains a separate evidence gap.
