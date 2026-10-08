# Windows Rust baseline - 2026-10-01

## Verdict

`RUST_BASELINE_SMOKE_PASS`: five cycles / fifteen intentional MCP operations
completed successfully. No forwarded-record amplification was observed in the
selected workload window. This is NOT a Go cutover acceptance or a root-cause
finding. Go remained stopped and disabled; Rust was not restarted.

Evidence directory, relative to the checkout:
`docs/handoffs/evidence/WINDOWS_RUST_BASELINE_20261001T152635Z/`.

## 1. Scope and timestamps

The existing checkout was resumed directly at
`C:\Users\FoxOS_User\codexify-go`; no worktree or clone was created.

- Selected log window: `[2026-10-01T15:26:40Z, 2026-10-01T15:28:59.6847897Z)`.
- The same window at UTC+03:00: 18:26:40 through 18:28:59.6847897 on October 1.
- Workload handler completion IDs: 905 through 919 inclusive.
- First workload completion: 15:26:49.807149Z; last: 15:28:22.405769Z.
- Setup completion 904 is present in the broader raw tool-event extract but is
  explicitly excluded from workload statistics.
- Other conversations / clients were not explicitly paused. The observed
  handler IDs 905-919 are consecutive and match the fifteen intended operations;
  that is not a general guarantee of control-plane isolation.

## 2. Runtime identity and process ownership

Before and after the workload, the persistent process tree was:

```text
codexify.exe service run          PID 11304  started 13:22:17 UTC+03:00
  codexify.exe                    PID  8452  started 13:38:40 UTC+03:00
    tunnel-client-runtime.exe    PID  8468  started 13:38:40 UTC+03:00
```

All three start times are on 2026-10-01. The process IDs and creation times did
not change. A diagnostic PowerShell child from this conversation had parent PID
8452; the loopback MCP listener `127.0.0.1:3000` was also owned by PID 8452.
These are direct ownership checks, not an inference from executable names alone.

Rust executable: `C:\Users\FoxOS_User\.codexify\bin\codexify.exe`.
The tunnel initialization record advertises server `codexify`, version `1.6.5`,
and negotiated MCP protocol `2025-06-18`. This identifies the advertised server
version; the Rust Git commit was not independently recovered.

Tunnel executable:
`C:\Users\FoxOS_User\.codexify\openai-tunnel\v0.0.12\tunnel-client-runtime.exe`.

`CodexifyGo`: `Stopped`, start mode `Disabled`, service PID `0` before and after.
The Rust scheduled task remained `Running`, `Enabled`, with last result
`267009 = 0x00041301 = SCHED_S_TASK_RUNNING`. This is the documented running
status, not a failed exit. The enabled Rust watchdog was left unchanged.
Existing Go one-shot install/recovery tasks were not changed or executed.

Binary SHA256 fingerprints:

```text
Rust:   66227C6CE9326D331CC3A7A31E76B1C1A21A29F64019777ED5DA214B4EAB4CA9
Tunnel: 9CB700F21CA31B05A2C255204123F95B34384C7186972314FE7D62667F27B4DE
Go:     E5D786D20D9F0F2771925E4BDC13B5B15F01CC5DCE29E20CF0BA529E6DF2E4E5
```

## 3. Actual workload results

Each cycle used separate real Windows MCP calls, not a loop of local commands:

1. `list_projects` with `limit=20`, without a query.
2. `read_file` for `README.md`, offset 0, limit 12.
3. `exec_command`: a cycle/UTC marker followed by
   `git --no-optional-locks status --porcelain=v1`.

| Operation | Calls | Successful | Rust handler duration |
| --- | ---: | ---: | ---: |
| list_projects | 5 | 5 | 1-2 ms |
| read_file | 5 | 5 | 69-75 ms |
| exec_command | 5 | 5 | 1297-1350 ms |
| Total | 15 | 15 | Not an end-to-end latency measurement |

Every command exited with code 0. No RATE_LIMITED response occurred in the
workload. The source checkout binding remained usable throughout all five cycles.

Unfiltered discovery reported `total: 41`, returning twenty entries because of
the requested limit. The earlier `query=codexify-go` lookup returned no matches,
but direct resume of the known checkout succeeded. This does not mean that the
connector was unavailable or that all project discovery was broken.

## 4. Tunnel evidence

The selected window contains exactly:

- 15 `dispatcher forwarded command to MCP server` records;
- 15 distinct request IDs and 15 distinct command request IDs;
- one observed client instance ID;
- a maximum of 1 forwarded record in an aligned one-second UTC bucket;
- zero WARN/ERROR or rate-limit/retry/timeout/reconnect keyword matches;
- zero session/connect/disconnect lifecycle log records.

At the analysis read, the active tunnel log had 946 parsed JSON records and zero
parse errors. It contained one `mcp session initialized` record, at line 13,
`2026-10-01T13:38:40.7796983+03:00`, and no additional such record in the workload
window. The log's earlier startup warning / poll-recovery entries are outside
this workload window; this report does not claim an error-free process lifetime.

The counts are consistent with one forwarded record for each tested operation,
with no observed amplification. They are a bounded observation, not proof that
all possible upstream retries or unlogged transport activity are absent.

The forwarded timestamp is a post-response-handling log timestamp, not an
arrival timestamp or an unconditional proof of response delivery. The source
analysis of that distinction is recorded in
`WINDOWS_MCP_BURST_FORENSICS_20260930.md`, section 3, lines 103-135.
The current workload's successful results are established separately by the
Rust handler records and the actual returned MCP tool results.

## 5. Installed Go binary is not the source checkout

The actual checkout HEAD is
`1098803bb3861e3f5d1a2702e4f5fda42afe0546`, with pre-existing uncommitted changes.

Read-only `go version -m` inspection of the installed Go executable reports:

```text
Go toolchain: go1.26.5
vcs.revision: a8e968cc613154b78d8589c04e7c6300bc70581e
vcs.modified: true
module: v0.8.2-0.20260929142645-a8e968cc6131+dirty
MCP Go SDK: v1.7.0
```

Therefore the installed candidate must not be identified as the current HEAD or
as a clean commit. No Go executable was started for this inspection.

The stopped Go historical log remains at
`C:\Users\FoxOS_User\.codexify-go\codexify-go.log`; its SHA256 is unchanged from
the forensic report:

```text
B093A2384E7E531898C6B6DC6505A81808245BC17A18501272240301823B3F51
```

## 6. Corrections and measurement limitations

The earlier statement that Task Scheduler result 267009 was suspicious was
incorrect. Microsoft's Task Scheduler error and success constants define it as
`SCHED_S_TASK_RUNNING`.

Initial directory metadata showed an old modification time / zero tunnel-log
length. Subsequent content reads established that both logs contained fresh
records. File metadata alone was not used to declare logging unavailable.

Preparation included an unsupported Rust `--version` invocation, a GET of the
health server's base URL that returned 404, and a missing file referenced by
stale saved state. These were diagnostic probe errors outside the workload,
not failed baseline operations. The base-URL 404 is not a health-endpoint result;
no health-route assertion is made. Advertised version came from the real tunnel
initialization log instead.

No same-load Go trial, fresh-conversation test, service restart, recovery test,
or prolonged soak test was performed here. Handler durations exclude the full
ChatGPT/control-plane round trip. Fifteen successful requests do not demonstrate
that rate limiting is impossible under other loads.

Rust's observed initialization behavior must not be treated as a universal MCP
requirement. Go lifecycle, SDK behavior, discovery, protocol negotiation and
upstream dispatch remain possible investigation areas, not proven causes.
No transport mode was forced or rewritten to imitate this observation.

## 7. Saved artifacts and next boundary

The evidence directory contains `before.json`, `after.json`,
`tool-events-before.txt`, `tool-events-window.txt`, `tool-events-window.json`,
`tunnel-window.json`, `tunnel-session-initialization.json`,
`runtime-identity.json`, `forwarded-per-second.json`, `metrics.json`,
`go-buildinfo.txt`, `task-after.json`, and final verification/checksum files.

Derived metric validation checks fifteen handler records, three groups of five,
all successful statuses, and the per-second bucket maximum. Raw event extracts
were not changed when correcting PowerShell array/bucket aggregation during
analysis. Evidence contains local paths and correlation identifiers; review
before external sharing. No credentials or raw request bodies were collected.

Only this report and new evidence files were written. Existing source changes,
installed binaries, runtime configuration, service settings and task settings
were left untouched. No commit, push, merge, installation or Go cutover occurred.

Next boundary: identify and freeze the intended Go candidate and its effective
configuration, meet the cutover/preflight gates, then perform a separately
bounded comparable workload. Do not mark GO_ACCEPTED or CUTOVER_CLOSED on the
basis of this Rust smoke baseline. No manual operator action is needed to finish
this recorded Rust baseline; leave the current runtime unchanged.
