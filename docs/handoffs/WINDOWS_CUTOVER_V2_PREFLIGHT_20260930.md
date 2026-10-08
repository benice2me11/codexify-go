# Windows Cutover v2 - Candidate preparation checkpoint

Date: 2026-09-30. Acceptance protocol: `WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md`.
Run/candidate ID: `20260930T184330-preflight`.

## Current result

**NOT_READY. Rust remains the live connector. No Go cutover was performed.**

A content-hashed source snapshot and native Windows binary have been prepared and
fresh local checks have passed. This is partial G00 evidence, not G00 PASS.
G01 is BLOCKED: the backup preparation command was blocked by the tool environment
before execution. No backup, rollback entry point, or independent observer was
created. No unattended observation is running.

## Source and candidate identity

Private run directory:

```text
C:\Users\FoxOS_User\.codexify-go\cutover-v2\20260930T184330-preflight\
  source\
  candidate\codexify-go.exe
  evidence\
  manifest.json
  gate-results.json
  evidence-index.json
  profiles\             # empty; not yet frozen
  rollback-private\     # empty; no successful backup
  private-traces\       # empty; no capture started
```

The private run directory has a protected ACL granting access to the executing
user, LocalSystem and Administrators. It is outside the Git checkout.

Source checkout: `C:\Users\FoxOS_User\codexify-go`.
Source HEAD: `1098803bb3861e3f5d1a2702e4f5fda42afe0546`, branch `main`, dirty.
The existing changes were preserved; they were not staged, reverted or committed.

The snapshot contains 120 tracked/nonignored untracked files. A per-file SHA256
manifest and a binary-capable diff against HEAD preserve the exact trial source,
including uncommitted additions in the snapshot. Source files were checked against
both the original checkout and the snapshot after the native checks; all 120
matched. These hashes are tamper-evident records, not a claim of write-once storage.

```text
source-files.json SHA256:
58E0971D6956E036ACEF29B893A29BA098BCF6F6CED55BD2DC375A07559F0AC1

candidate/codexify-go.exe SHA256:
B0B421A0E3045B7F71ACD2ADF1C8079EAC2D87358091590A9BD467E36E798999
```

Build: Go `go1.26.5`, `windows/amd64`, `CGO_ENABLED=0`, `-trimpath`,
`-buildvcs=false`. The snapshot has no `.git`; source provenance comes from its
recorded HEAD, patch and per-file hashes, not from a misleading embedded clean VCS
revision. `go version -m` output is stored in `evidence/binary-buildinfo.txt`.
The binary was not installed or started.

## Fresh verification on this snapshot

All five required native commands below completed with exit code 0. Each has a
full output file and a `.result.json` record containing the command, start/end
host timestamps and exit code under `evidence/`.

| Check | Result | Evidence basename |
| --- | --- | --- |
| `go test ./... -count=1` | PASS | `test-all` |
| `go vet ./...` | PASS | `vet` |
| `go test ./internal/ui -count=10 -v` | 10 PASS, 0 SKIP | `ui-repeat` |
| `go test ./internal/mcpdiag -run '^TestIsolatedConcurrentCallsNoAmplification$' -count=10 -v` | 10 PASS | `isolated-mcp-repeat` |
| `go build -trimpath -buildvcs=false -o <candidate> ./cmd/codexify-go` | PASS | `native-build` |

Node `v24.19.0` was available. The shipped-widget JavaScript test actually ran in
all ten repetitions; it did not pass by skipping a missing Node dependency.

Every isolated MCP repetition reported:

```text
sent=1024 executed=1024 http_pairs=1024 mcp_pairs=1024
 distinct_trace_ids=1024 operation_hashes=1 retained_sessions=0
```

That is 10,240 synthetic loopback requests in total, not hosted traffic, live
widget acceptance, or historical incident attribution.

Additional check: `go mod verify` returned `all modules verified`.
Race detection was NOT RUN: CGO is disabled and neither gcc nor clang was found
in PATH. No compiler was installed. This remains a disclosed residual check gap,
not a passed race test and not a reason by itself to modify the host installation.

## What still prevents G00 PASS

- The source pins tunnel runtime `0.0.15`, while the inspected Go runtime cache
  contains only `v0.0.12`. A v0.0.15 artifact has not been staged and fingerprinted
  for this run. Do not use an existing v0.0.12 binary as though it matched the pin.
- Capture-enabled and normal configuration profiles have not been saved, reviewed
  and frozen. Only the existing Go configuration hash was recorded privately.
- Actual candidate tool-schema and served-resource fingerprints remain pending.
  Reading `/v2/` source constants or refreshing the active Rust connector does not
  provide this evidence.
- The required integration/optional-feature scope remains undecided. The inspected
  current Go configuration has Chat disabled; no live Chat coverage is claimed.

The source review in this continuation covered the relevant tracked diff and the
existing fix report, followed by native verification. It is not an independent
whole-branch security review of every untracked addition.

## G01 blocker and old local scripts

The combined backup/preparation command was rejected with:

```text
This tool call was blocked by OpenAI because we couldn't determine
 the safety status of the request.
```

It did not execute. The affected backup operation was not retried or routed
through another mechanism. Local build/test commands on the already-created
snapshot were separate operations and subsequently succeeded.

A later read verified `rollback-private` and `profiles` each contained zero files.
Do not mistake the presence of those directories for a successful backup. The
reason for the safety-classification failure is unknown. This response is not
`RATE_LIMITED`, a Go application error, or evidence of a new burst.

Old private scripts were inspected but NOT run:

- `.codexify-go/cutover-once.ps1` continues after errors and stops Rust processes
  by the broad process name `codexify`, rather than an established ownership tree.
- `.codexify-go/install-latest-cutover.ps1` copies from the mutable checkout's
  `bin/codexify-go.exe`, continues after failures and starts the service. It is
  not the v2 frozen-candidate deployment path.

Nine old `Codexify Go ...` scheduled tasks were enabled/Ready. All nine had zero
triggers; no automatic execution from those tasks was established. They remain
manually invocable and reference old workflows. No task was disabled or deleted.
The Rust watchdog remains enabled with its existing one-minute trigger.

## Live boundary

The initial live inspection found:

```text
CodexifyGo: Stopped / Disabled / PID 0
Rust supervisor: PID 11404
Rust worker: PID 12692
Rust-owned tunnel v0.0.12: PID 14788, parent 12692
```

These are observations, not durable PID targets for future termination. Resolve
ownership and process creation times again before any authorized fault test.

No service/task/configuration/startup-policy change, installed-binary replacement,
live fault injection, reboot, logout, integration mutation, commit or push was
performed in this continuation. The only new checkout file is this report.

## Gate checkpoint and continuation

| Gate | Status | Meaning |
| --- | --- | --- |
| G00 | PENDING | Snapshot/native checks completed; required artifact/profile/metadata work remains |
| G01 | BLOCKED | Backup command rejected; recovery/observation readiness not established |
| G02-G11 | PENDING | No live v2 switch or acceptance scenario performed |

Resume using the exact source checkout from the acceptance protocol. Inspect
`manifest.json` and the evidence rather than rebuilding from an evolving working
tree and reusing the same candidate ID.

Next preparation work: resolve the blocked authorized backup operation; prepare
and fixture-test a narrowly scoped local rollback/observer; complete tunnel and
profile/schema fingerprints; coordinate the quiet Rust baseline and other active
conversations/automations. Do not enable Go until G00 and G01 genuinely pass.
