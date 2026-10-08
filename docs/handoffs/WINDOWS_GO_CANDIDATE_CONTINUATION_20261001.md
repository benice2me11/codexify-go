# Windows Go candidate preparation after Rust baseline - 2026-10-01

## Result and operating boundary

Candidate preparation advanced; the live Windows cutover remains NOT_READY.
G00 is still PENDING and G01 is still BLOCKED / readiness not demonstrated.
No hosted Go trial, service replacement, production configuration change,
installation into the active runtime, commit or push was performed.

An isolated Go worker was briefly started from the frozen candidate to read its
real MCP metadata. It used a synthetic tunnel identity, a loopback ephemeral
port, an isolated workspace/home, a fresh local bearer, no real tunnel key and
zero upstreams. No Go tunnel was started. The worker was stopped after both the
failed initial probe and the successful corrected probe.

The last successful ownership check, immediately after the successful metadata
probe, still showed Rust supervisor 11304, Rust worker 8452 and Rust-owned tunnel
8468, with unchanged creation times. The preceding service checks showed
CodexifyGo Stopped / Disabled / PID 0. A later consolidated evidence-read command
was blocked before execution; this report does not claim that command verified
anything.

## Source of truth and stale saved state

Actual checkout: C:\Users\FoxOS_User\codexify-go.
HEAD: 1098803bb3861e3f5d1a2702e4f5fda42afe0546, with existing dirty changes.

The saved project-memory note mentioned a later startup controller, commits
3fe1896 / 5e66db9 and WINDOWS_CUTOVER_V2_STARTUP_PREPARATION_20261001.md.
The named handoff does not exist in this checkout, and the actual tools tree
contains the forensic tools, not that controller. That note was not treated as
proof of implemented or tested recovery tooling. Its unknown test session was
not polled or rerun.

The actual preparation checkpoint was read from:
- docs/handoffs/WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md
- docs/handoffs/WINDOWS_CUTOVER_V2_PREFLIGHT_20260930.md
- The existing candidate's private manifest and gate-results files.

## Frozen candidate reused, not silently replaced

Candidate ID: 20260930T184330-preflight.
Private run directory:
C:\Users\FoxOS_User\.codexify-go\cutover-v2\20260930T184330-preflight

The existing snapshot and binary were retained. At the fresh source comparison,
all 120 manifest-listed files matched both the snapshot and the live checkout.
There were zero snapshot mismatches, zero checkout mismatches and no uncaptured
non-documentation source files. The sixteen then-new files were documentation
and earlier baseline evidence. The diagnostic script added in this continuation
is also under docs/handoffs/evidence, not part of the frozen runtime source.

Source-file manifest SHA256:
58E0971D6956E036ACEF29B893A29BA098BCF6F6CED55BD2DC375A07559F0AC1

Candidate executable SHA256:
B0B421A0E3045B7F71ACD2ADF1C8079EAC2D87358091590A9BD467E36E798999

The snapshot was rebuilt with Go go1.26.5, windows/amd64, CGO_ENABLED=0,
-trimpath and -buildvcs=false. The new executable was BYTE-IDENTICAL to the
existing frozen candidate. It was not copied over the installed executable.
Provenance is the exact source manifest and patch, not a fabricated clean VCS
revision embedded in the binary.

Evidence base for this continuation, relative to the private run directory:
evidence/continuation-20261001-rust-baseline/

## Fresh local checks

All six commands completed with exit code 0 in the frozen snapshot:

| Check | Result | Evidence basename |
| --- | --- | --- |
| go test ./... -count=1 | PASS | test-all |
| go vet ./... | PASS | vet |
| go test ./internal/ui -count=10 -v | 10 passes, no SKIP | ui-repeat |
| go test ./internal/mcpdiag -run ^TestIsolatedConcurrentCallsNoAmplification$ -count=10 -v | 10 passes | isolated-mcp-repeat |
| go mod verify | PASS | mod-verify |
| go build -trimpath -buildvcs=false -o <separate output> ./cmd/codexify-go | PASS; identical SHA256 | rebuild |

Each command has its output and a .result.json file with command, start/end UTC
and exit code. Node v24.19.0 was present, so widget tests actually ran.
Each loopback repetition reported:
sent=1024 executed=1024 http_pairs=1024 mcp_pairs=1024
 distinct_trace_ids=1024 operation_hashes=1 retained_sessions=0

The 10,240 synthetic requests were loopback-only, not calls through ChatGPT or
the hosted tunnel. These checks do not prove a historical burst root cause.
Race detection remains NOT RUN: CGO was disabled and gcc/clang were not found in
PATH. No C compiler was installed.

## Missing tunnel artifact now staged and verified

The frozen source already pinned tunnel-client-runtime 0.0.15. This continuation
did not introduce a new version pin.

The candidate's managed installer fetched and verified the official release
artifact into the continuation's separate staged-tunnel directory. Its built-in
checks validated the pinned archive hash, reported version and required CLI
flags. The installer ran only version/help checks, not a hosted tunnel process.
The official release listing was also checked at:
https://github.com/openai/tunnel-client/releases

Staged artifact: tunnel-client-runtime-v0.0.15-windows-amd64.zip
Archive SHA256:
aa5ddb14dddd602fa59f3e6f4401aa8a79a218e341466226b7434127dff65dbc
Binary SHA256:
a922d372d6be0649156fbc1c8a040597f1f4bb5b4356890151d7875602593b1a

Private executable location, relative to the evidence base:
staged-tunnel/v0.0.15/tunnel-client-runtime.exe

The managed install manifest and tunnel-stage.txt / tunnel-stage.result.json
record the result. Rust's active v0.0.12 executable and Go's existing active-cache
locations were not replaced. The staged path is not yet a frozen deployment
configuration reference.

## Actual native candidate metadata obtained

The reproducible probe script is:
docs/handoffs/evidence/WINDOWS_GO_CANDIDATE_20261001/Inspect-Candidate.ps1

The successful probe used the candidate executable's worker run entry point and
the modern 2026-07-28 protocol. It made server/discover, tools/list,
resources/list and four resources/read requests. All completed without a
JSON-RPC error; the probe exited 0 and confirmed its worker had stopped.

Observed built-in tool count: 28. Observed resource count: 4.
Actual served resource URIs:
- ui://codexify-go/diff/v1/mcp-app.html
- ui://codexify-go/markdown-chat/v2/mcp-app.html
- ui://codexify-go/self-update/v2/mcp-app.html
- ui://codexify-go/setup/v2/mcp-app.html

The probe saved actual JSON responses, a tool-response SHA256 and per-resource
UTF-8 text SHA256 values under:
evidence/continuation-20261001-rust-baseline/metadata-modern-v2/

Key files: candidate-metadata.json, server-discover.json, tools-list.json,
resources-list.json, resource-1.json through resource-4.json, probe-ownership.json.
The isolated profile is loopback-fixture/offline-profile.json under the evidence
base. Its hash is recorded in candidate-metadata.json.

These are fingerprints of THIS isolated profile and real binary, not a final
production-profile fingerprint. The profile has no upstreams, Chat disabled and
no user/plugin skill discovery. Serving the static Chat HTML does not establish
live Chat feature coverage. A profile with integrations must be fingerprinted
again; do not treat these 28 tools as the intended complete production catalog.

## Probe errors and verification boundary

The first one-line probe command had a PowerShell parse error and did not run.
The first parsed probe used an incomplete modern request and received HTTP 400;
its worker was stopped and its failed candidate-metadata.json was retained in
the evidence base.

Inspection of the installed go-sdk v1.7.0 showed the missing client contract:
per-request io.modelcontextprotocol/protocolVersion and clientCapabilities in
_meta, matching MCP-Protocol-Version, Mcp-Method, and Mcp-Name for resources/read.
The diagnostic probe was corrected to match those SDK definitions. Production
code, the frozen snapshot and protocol routing were not changed. The corrected
probe passed using the modern protocol, not a fallback/downgrade.

After that success, a combined read of private metadata, tool names and legacy
preparation directory counts was rejected by the tool environment:
"This tool call was blocked by OpenAI because we couldn't determine the safety
status of the request."
It did not execute and was not retried via another route. This is not
RATE_LIMITED, a candidate error or evidence of burst. Thus a new consolidated
post-probe evidence audit was not completed. The successes above come from the
actual prior command outputs, not from the blocked read.

## Configuration and readiness gaps

Read-only inspection of the installed Go configuration still showed:
- multiProject true;
- agentChat.enabled false;
- zero configured upstreams;
- unchanged configuration SHA256
  9D440BA16ACC991157831B9286F46248A16A9C9F00EB384CFE8AE5C9B701A8FD.

The active Rust connector exposes HH/Habr and node_repl transitive tools.
Integration parity is not established by the empty Go upstream configuration.
This continuation did not migrate integration definitions or copy credentials.

| Gate | Status after this continuation | Remaining requirements |
| --- | --- | --- |
| G00 | PENDING, advanced | Final capture/normal deployment profiles, startup/recovery policy, required integration inventory and profile-specific metadata; staged artifact location must be bound to the final profile |
| G01 | BLOCKED / not demonstrated | Private restore snapshot, reviewed local rollback, fixture-tested independent observer/abort, quiet-workload coordination |
| G02-G11 | PENDING | No hosted Go switch, rollback rehearsal or later acceptance scenario |

The previous backup-operation block was not bypassed or replayed. No new backup,
local rollback controller or unattended observer was created in this turn.
The successful fifteen-call Rust smoke baseline remains useful reference data,
but does not substitute for G01's explicitly coordinated baseline with other
workloads paused, health evidence and its specified get_environment sequence.

A Rust v0.0.12 versus Go plus tunnel v0.0.15 comparison changes more than one
variable. It can accept the combined candidate, but cannot isolate the historical
cause to the Go lifecycle. No such causal conclusion is made here.

## Next action boundary

Complete the deployment/integration profiles and independent local recovery
readiness before enabling CodexifyGo. Keep the current Rust owner running.
User-only actions such as pausing other sessions, closing stale widgets, opening
a second hosted conversation, refresh and reboot must have actual operator
evidence when their gates are reached; none was fabricated here.

No immediate manual service command is required by this checkpoint. In
particular, do not start the old installed Go binary just because the prepared
candidate passed local tests.
