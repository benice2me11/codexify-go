# Windows Cutover v2 — G00 complete, G01 operator boundary (2026-10-01)

> Historical phase record, versioned after G03. Ownership, process identities
> and verification results below describe that phase, not current runtime health.

## Current state

Candidate: `20261001T160710Z-node-repl-compat`.

Private candidate run:

```text
C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat
```

Live owner was intentionally not changed in this continuation:

- Rust `codexify.exe` remains the active Windows connector.
- `CodexifyGo` remains Stopped / Disabled.
- No hosted Go tunnel was started.
- No scheduled task or live runtime configuration was changed.
- No commit or push was performed.

Gate status after this continuation:

| Gate | Status | Reason |
| --- | --- | --- |
| G00 | PASS | Exact frozen source/binary/tunnel/profiles plus fresh tests and actual worker metadata are recorded |
| G01 | BLOCKED | Real private Rust recovery snapshot and a coordinated quiet Rust reference still require operator participation |
| G02 | PENDING | No live Go switch has occurred |

## 1. Rust reference already established

The earlier Rust baseline is recorded in:

`docs/handoffs/WINDOWS_RUST_BASELINE_20261001T152635Z.md`.

That bounded workload completed 15/15 intentional calls with 15 forwarded records, one per operation, no RATE_LIMITED response and no lifecycle churn in the selected window. It is useful comparative evidence, but it does not satisfy the G01 requirement for a separately coordinated baseline with other Windows connector workloads paused.

## 2. Integration parity investigation

The active Rust connector exposes three private catalog sources:

| Source | Server | Tools |
| --- | --- | ---: |
| `habr_career` | `habr-career-mcp 0.1.0` | 5 |
| `hh_local` | `hh_scraper 3.2.0` | 13 |
| `node_repl` | `rmcp 1.5.0` | 4 |

The original Go configuration had no upstreams. Normal/capture candidate profiles were therefore derived from the actual Codex MCP configuration while keeping credentials/private environment values only in the private run directory.

HH and Habr connected through Go immediately. `node_repl` reproducibly failed with:

```text
connection closed: calling "initialize": client is closing: EOF
```

### Root cause

The Go bridge used `github.com/modelcontextprotocol/go-sdk v1.7.0`. Its client negotiates the current `2026-07-28` protocol by attempting `server/discover` first.

The local `node_repl` server (`rmcp 1.5.0`) closes its stdio connection on that discovery request instead of returning a negotiation response. That prevents the SDK's normal fallback from reaching legacy `initialize`.

A separate scratch client using go-sdk v1.8.0 and explicit:

```text
ProtocolVersion: 2025-11-25
```

connected successfully to a second `node_repl` instance while Rust remained live:

```text
LEGACY_CONNECT=PASS server=rmcp version=1.5.0 tools=4 protocol=2025-11-25
```

This isolates the problem to upstream protocol negotiation rather than node_repl single-instance/pipe ownership.

## 3. Test-first compatibility fix

A regression fixture was added to `internal/upstream/bridge_test.go`. The fixture exits when it receives `server/discover`, but accepts legacy `initialize`.

The RED run failed with the same EOF behavior observed from real `node_repl`.

The minimal production change then:

1. upgrades `github.com/modelcontextprotocol/go-sdk` from v1.7.0 to v1.8.0;
2. adds optional `mcp.upstreams[].protocolVersion`;
3. passes that value via exported `mcp.ClientSessionOptions.ProtocolVersion`;
4. leaves the field unset for normal/latest negotiation;
5. sets only `node_repl` to `2025-11-25` in the private candidate profile.

The targeted regression then passed.

This does **not** downgrade the hosted Go MCP server. The isolated candidate probe still uses the modern `2026-07-28` downstream protocol. The compatibility override is only for the one legacy upstream client connection.

## 4. New frozen candidate

Because the SDK and runtime source changed, the previous `20260930T184330-preflight` candidate is historical and is not reused for acceptance.

New candidate source snapshot:

```text
C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\source
```

Source identity:

```text
HEAD:                 1098803bb3861e3f5d1a2702e4f5fda42afe0546
Snapshot files:       139
source-files SHA256:  ECA7EF44DA3ED485564EF1D50983C48110133F5BF1C289D3EAB664421BCC48F6
go.mod SHA256:        9BE099BEB377FA0E4FC66EE0FCA9117C7666E018E64A1D50618DC7A9DCD89C79
go.sum SHA256:        8020DFD936EE1998F26270F226F7E1F174E479D5F901E44277213E7B0F8864A1
```

Candidate executable:

```text
SHA256:
61E2A086445A1B10E7537E720119B0855E17515A58A5B3F8BDCA914075B07DD6
```

Build:

- Go `go1.26.5 windows/amd64`;
- `-trimpath`;
- `-buildvcs=false`;
- go-sdk `v1.8.0`.

Pinned tunnel:

```text
v0.0.15
SHA256 A922D372D6BE0649156FBC1C8A040597F1F4BB5B4356890151D7875602593B1A
```

The tunnel artifact is staged inside the private candidate run and passed candidate `doctor`. It was not used to connect to the hosted control plane in this continuation.

## 5. Frozen profiles

Private profile hashes:

```text
normal:
75A086E6ED53124AD52B78CE35C83B8A04FC4D57A14D10C2C91645F5D692955D

capture:
BCDF7B1362047E957F21B55704D69EC3783BD189B9D79759980499613D9A197F

inspection:
034BC5B8CF3FCCBCD994B4731E8A5D976AFA18F288167FF87D4A631306700A96
```

Normal profile:

- diagnostics disabled;
- multi-project enabled;
- Chat disabled, matching the current declared deployment scope;
- three catalog upstreams configured;
- only node_repl has the legacy protocol override.

Capture profile differs only by enabling bounded MCP metadata diagnostics for focused acceptance windows.

Inspection profile uses isolated state/workspace and a fake tunnel identity. It launches only `worker run`, never the hosted tunnel.

## 6. Fresh verification from the frozen source

All required native commands were run from the exact new source snapshot:

| Check | Result |
| --- | --- |
| `go test ./... -count=1` | PASS |
| `go vet ./...` | PASS |
| UI regression, `-count=10` | PASS 10/10, no SKIP |
| isolated MCP amplification test, `-count=10` | PASS 10/10 |
| `go mod verify` | PASS |
| native Windows build | PASS |

Each isolated amplification run executed:

```text
sent=1024 executed=1024 http_pairs=1024 mcp_pairs=1024
distinct_trace_ids=1024 operation_hashes=1 retained_sessions=0
```

Race detector status remains explicitly:

```text
NOT_RUN
CGO_ENABLED=0
gcc=MISSING
clang=MISSING
```

Per the acceptance plan, this is a disclosed residual test gap, not a race PASS and not a requirement to alter the machine.

## 7. Actual candidate metadata

The exact frozen candidate was started as an isolated local worker and queried using modern MCP `2026-07-28`.

Observed:

```text
tools:     32
resources: 4

habr_career: 5
hh_local:    13
node_repl:    4
```

Tool-catalog response fingerprint:

```text
DA473A21C0DDE5C173014D8083C4F778EAA13CCAD11FE877DAB96FEECA6C3E0C
```

Resource content fingerprints:

```text
ui://codexify-go/diff/v1/mcp-app.html
24F40B215F3807AAB6D9FE30C2AC399D1B13203355AF5D876695FD24AE58FBC6

ui://codexify-go/markdown-chat/v2/mcp-app.html
5DE1550794C89789D608D22454AC15F59D53F91962E757B87DAEC0457EFBB25C

ui://codexify-go/self-update/v2/mcp-app.html
7B5D1DCBDD51CE3434235FF902E7B37685EEFB339A25ACFB690B0B42F30C7C0D

ui://codexify-go/setup/v2/mcp-app.html
E95DA94ECA6F86BFA269EA55A45615384F52C2D7CEC2EC650EBCBA052BE093E2
```

The isolated worker stopped after capture and the same Rust PIDs remained.

## 8. G01 operator tooling

Prepared under:

`tools/windows/cutover-v2/`

Files:

- `CutoverGuard.psm1`;
- `Prepare-RecoverySnapshot.ps1`;
- `Rollback-To-Rust.ps1`;
- `Watch-Cutover.ps1`;
- `Test-CutoverTooling.ps1`.

A frozen copy and SHA256 inventory are stored under the private candidate run in `operator-tooling/` and `evidence/operator-tooling-hashes.json`.

Fixture tests verify:

- 9 distinct calls in 10 seconds remain below the emergency threshold;
- 10 calls in 10 seconds produce ABORT;
- 5 repeats of one operation hash in 10 seconds produce ABORT;
- >100 forwarded records in 60 seconds during a declared quiet window produce ABORT;
- RATE_LIMITED produces HOLD, not automatic causal attribution;
- simultaneous Rust/Go ownership produces ABORT;
- two unexpected owned-process restarts in 5 minutes produce ABORT;
- recovery manifests are hash-validated;
- rollback dry-run ordering validates snapshot and Go identity before mutations, stops Go before restoring/starting Rust, then verifies Rust readiness;
- watcher fixture correctly surfaces the guard decision.

The observer does not invoke MCP. In armed mode an ABORT can invoke the local rollback script directly.

No live rollback, service stop/start or task restore was exercised here.

## 9. Why G01 is still BLOCKED

Two requirements cannot be honestly replaced by fixture evidence.

### A. Real private Rust recovery snapshot

A previous attempt to perform the combined backup operation through the tool environment was blocked before execution by platform safety classification. That operation was intentionally **not retried or routed around the block**.

A dedicated manual script is now prepared. The operator must run it locally once:

```powershell
& 'C:\Users\FoxOS_User\codexify-go\tools\windows\cutover-v2\Prepare-RecoverySnapshot.ps1' -Destination 'C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\rollback-private\rust-snapshot' -Capture
```

After that, ChatGPT can perform the read-only `-ValidateOnly` check and dry-run rollback plan against the resulting snapshot.

### B. Coordinated quiet Rust reference

Before final G01 PASS, the operator must pause other Windows connector conversations/automations and close stale Windows connector widgets. Then run the acceptance reference workload while those other workloads remain paused.

The earlier 15-call Rust baseline is retained but is not relabelled as this coordinated G01 reference.

## 10. Boundary before G02

Do not start `CodexifyGo` yet.

After the two operator items above are complete:

1. validate the recovery snapshot and frozen rollback tooling;
2. capture the coordinated quiet Rust reference;
3. mark G01 PASS only if both succeed;
4. then perform G02 as one guarded Go switch with the local watcher active;
5. rehearse one real Go -> Rust rollback locally;
6. only after successful rollback, re-enter the exact same Go candidate.

No old installed Go binary should be used for G02. The accepted trial candidate is identified by SHA256 `61E2A086445A1B10E7537E720119B0855E17515A58A5B3F8BDCA914075B07DD6`.

Private machine-readable evidence:

```text
C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\manifest.json
C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\gate-results.json
```
