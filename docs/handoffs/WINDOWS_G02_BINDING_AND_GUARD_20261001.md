# Windows G02 continuation: binding restored, guard corrected

## Current state

The Windows MCP conversation is bound to the existing `%USERPROFILE%\codexify-go`
checkout. Rust remains the live owner. `CodexifyGo` remains Stopped / Disabled.
No hosted Go switch, real rollback, re-entry, commit or push occurred in this
continuation. The current quiet-window confirmation is still pending.

Candidate ID remains `20261001T160710Z-node-repl-compat`, binary SHA256
`61E2A086445A1B10E7537E720119B0855E17515A58A5B3F8BDCA914075B07DD6`.
The candidate binary, frozen source and normal/capture profiles were not changed.

## Workspace binding

The full hosted Rust `list_projects(limit=100)` returned 42 projects, without
warnings. Its `codexify-go` entry was `Documents/ChatGPT/codexify-go`, an empty
repository with no commits. It was not the populated source checkout.

The catalog advertises `codex_config` and `explicit_metadata` as its sources;
all returned entries came from `codex_config`. The Codex configuration registers
the empty directory but does not register the source checkout. A project memory
directory does not by itself establish catalog membership. No index corruption
was demonstrated, and no configuration or path normalization was changed.

The exact call `set_project_root({resumePath: <existing source checkout>})`
succeeded. It reported a persistent conversation binding, `managed_worktree=false`
and `cloned=false`. `get_agent_brief`, `recall` and hosted `get_environment` then
succeeded; the last confirmed the extended-length source path. The earlier
resume failure did not reproduce, so its historical cause remains unproven.

## Recovery verification and newly found guard defects

The supplied handoff reported G01 PASS, while the older saved plan and private
gate ledger still said BLOCKED. The existing real recovery snapshot validated:
49 files. A new pre-G02 snapshot captures this conversation's binding as well:
50 files, two task definitions, all manifest hashes valid. Both rollback dry-runs
returned the intended ordered actions. A dry-run does not prove actual SCM
permissions, process cleanup or restore readiness.

Inspection against actual Go log/diagnostic formats exposed these defects in
the original frozen operator tooling:

- Strict property access threw on Go slog `msg` records and on a diagnostic
  without the optional `operation_hash`.
- Discovery methods were counted against the emergency `tools/call` thresholds.
- HOLD exited the watcher; a rate-limit event could leave Go unobserved.
- A simultaneous rate limit masked a confirmed traffic ABORT condition.
- A monitoring exception exited without invoking the armed rollback.

The corrected tooling reads optional fields safely, applies call thresholds only
to `tools/call`, preserves observation during HOLD, gives demonstrated ABORT
conditions precedence, and invokes the local rollback on observation exceptions.
HOLD still means the operator must stop new stimuli and investigate; it does
not attribute the external rate limit to Go or grant permission to continue work.

Changes are confined to `tools/windows/cutover-v2/`. Original frozen tooling was
retained. The final frozen operator revision is **operator-tooling-v3**;
operator-tooling-v2 is an intermediate compatibility-only revision and must not
be used for the live trial.

## Verification

- Original guard: six of nine compatibility cases failed before the fix.
- HOLD, observation-failure and rate-limit/traffic precedence regressions each
  failed before their corresponding fixes.
- Final Windows PowerShell compatibility suite: 10/10 PASS.
- Existing guard/snapshot/rollback fixture suite: PASS.
- Watcher process fixture: sustained HOLD remains alive; a monitoring exception
  invokes a harmless local rollback fixture. PASS.
- All three suites rerun from frozen operator-tooling-v3: exit 0.
- `go test ./... -count=1`: PASS on the existing working tree.
- `git diff --check`: PASS. No new race-detector claim is made.

An independent read-only reviewer verified the compatibility patch and both
initial suites. Its pre-existing HOLD-exit finding was addressed in one fix pass
with failing-then-passing lifecycle tests. Deployment, real rollback and the
quiet-window prerequisite were not claimed by that review.

## Evidence and continuation

Private run root:
`%USERPROFILE%\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat`.

- `evidence/g02-binding-guard-20261001/`: compatibility failure/pass logs,
  final tooling SHA256 inventory, dated suite results and readiness snapshot.
- `operator-tooling-v3/`: final scripts for the next guarded run.
- `rollback-private/rust-snapshot-before-g02/`: current validated recovery copy.
- Original `manifest.json`, `operator-tooling/` and recovery copy are retained.

G00 candidate identity remains verified. Current G01 operational readiness and
G02 remain BLOCKED pending the operator's answer that other Windows MCP chats
and automations are paused and old widgets are closed. This is acceptance-plan
G01's coordination requirement, not a new cutover approval request.

After that answer, continue the existing acceptance plan: capture its spaced
Rust reference, update the stopped Go service to the exact candidate/profile,
arm the final local watcher, switch exclusively to Go, verify local identity and
one hosted read, perform the actual local Rust rollback, then re-enter the same
candidate. Verify watcher coverage on every entry; the capture profile is bounded
to 10 minutes. Do not reuse a stopped watcher or expired trace as live evidence.

The discovery workaround does not require editing `memory.json`, rewriting
extended-length paths, creating another worktree or copying the source project.
