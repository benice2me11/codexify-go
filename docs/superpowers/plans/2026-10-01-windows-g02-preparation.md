# Windows G02 Preparation Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans for the authorized
> preparation tasks. Do not execute the live trial without the separate user
> authorization specified below.

**Goal:** Make the exact frozen Windows Go candidate reviewable and ready for an
explicitly authorized G02 trial while Rust remains the live MCP owner.

**Architecture:** Use the existing checkout and frozen private run directory.
Keep immutable candidate assets, independently executable local recovery and a
dated evidence ledger. Separate preparation evidence from actual live acceptance.

**Tech Stack:** Windows PowerShell, SCM, Task Scheduler, hosted Windows MCP.

**Spec:** [Readiness criteria](../../handoffs/WINDOWS_G02_READINESS_CRITERIA_20261001.md)
and [acceptance plan](../../handoffs/WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md).

## Global constraints

- No Go executable invocation, Go service start, Rust shutdown, live watcher
  arming, real rollback or reboot during this preparation.
- Preserve frozen candidate/profile hashes, Rust availability, dirty source and
  all existing private evidence. Use only operator-tooling-v3 for the trial.
- The user authorized inspecting and pausing competing Windows MCP work.
  Pause only positively identified jobs; preserve definitions and prior state.
- Do not infer cloud-schedule status or closed widgets from idle conversations.
- Do not mark G01 PASS without coordinated reference evidence; do not mark G02
  PASS without the separately authorized real rehearsal.

## Review focus

1. Wrong checkout despite the same project name: require the exact hosted cwd.
2. An old SCM ImagePath selecting a different binary: save and verify both states.
3. Legacy enabled installer/test tasks interfering with ownership: identify and
   disable those tasks without disabling the live Rust tasks.
4. A recovery snapshot predating the new binding: capture a new private snapshot
   and validate every hash; keep earlier snapshots.
5. Unobservable external widgets or automations: leave coordinated baseline
   blocked and provide concrete manual actions rather than inventing completion.

## Task 1: Record scope and criteria

**Files:** This plan; the separate readiness criteria; a dated private execution
ledger under the frozen run root's `evidence` directory.

- [x] Read the current gate ledger, acceptance plan and v3 guard handoff.
- [x] Write distinct entry, authorization, G02 success and later acceptance rules.
- [x] Record current preparation evidence and unresolved gates.

Expected: a reader can identify exactly what is authorized now and what evidence
is still missing without reading the chat history.

## Task 2: Restore binding and control competing work

**Inputs:** Exact workspace path and the user's permission to pause attributable
Windows MCP jobs. **Outputs:** binding evidence, inventory, task backups and
paused-task list with restoration instructions.

- [x] Resume `C:\Users\FoxOS_User\codexify-go`; read the hosted operating brief.
- [x] Verify hosted get_environment reports that exact populated checkout.
- [x] Inspect accessible task/chat status, local schedules and browser surfaces.
- [x] Export the nine identified `Codexify Go ...` task definitions and states,
  then disable those exact tasks. Keep `Codexify` and `Codexify Watchdog` enabled.
- [x] Record inaccessible client/cloud-schedule checks and exact manual actions.

Expected: no attributable legacy Go task can run, Rust ownership is unchanged,
and unknown external-client state is explicit.

## Task 3: Verify identity, fixtures and fresh recovery

**Inputs:** Frozen manifest, operator-tooling-v3, restored binding.
**Outputs:** hash comparison, fixture logs, fresh recovery path and rollback plan.

- [x] Verify binary, tunnel, capture/normal profile and v3 script hashes.
- [x] Run `Test-CutoverTooling.ps1`, `Test-CutoverLogCompatibility.ps1` and
  `Test-CutoverWatcherLifecycle.ps1` using Windows PowerShell and frozen v3.
- [x] Capture a fresh Rust snapshot into a new private directory after binding.
- [x] Run `Prepare-RecoverySnapshot.ps1 -ValidateOnly` on that snapshot.
- [x] Run `Rollback-To-Rust.ps1 -DryRun` with the frozen candidate path; verify
  all eight action names and order against the criteria.

Expected: all hashes match, all three suites exit 0, snapshot validates, dry-run
mode is explicit, and no live process has been stopped or started.

## Task 4: Prepare SCM without starting Go

**Inputs:** Task 3 evidence and a Stopped/Disabled `CodexifyGo` service.
**Outputs:** original SCM backup, verified new ImagePath, concrete launch metadata.

- [x] Save original service configuration privately.
- [x] Require service Stopped/Disabled, PID 0 and zero Go processes.
- [x] Set only ImagePath to the frozen binary's `service run --config` command
  with `profiles\capture.json`; explicitly retain Disabled startup.
- [x] Read back the exact ImagePath, status and startup type; recheck Rust PIDs.
- [x] Record future watcher/recovery/profile paths without starting the watcher.

Expected: exact frozen command is configured; Go remains Stopped/Disabled and
the same Rust process tree continues serving Windows MCP.

## Task 5: Establish the coordinated Rust reference

**Dependency:** Task 2's external-client/automation quiet-window checks must be
complete. User confirmation is sufficient for surfaces inaccessible to tools.

- [x] Close old Windows MCP widgets and pause other Windows MCP jobs on all
  clients; record the action or the operator's current confirmation.
- [x] Record process tree, local health and current log coverage.
- [x] Perform 10 sequential hosted get_environment calls with >=5-second gaps;
  record each dispatch/result time, latency, success and canonical cwd.
- [x] Record end state and distinguish planned calls from other activity.

Expected: complete, coordinated Rust reference evidence. If client state remains
unknown, do not run or relabel an uncoordinated sample as this reference.

## Task 6: Reconcile readiness and hand over

- [x] Re-read changed documents, check links/whitespace and final live ownership.
- [x] Update the dated evidence ledger and gate status with exact completed and
  blocked criteria; preserve historical manifests and evidence.
- [x] Report what was paused, how to restore it, and the remaining user actions.
- [x] Stop before G02 execution. Request separate live authorization only after
  the prerequisite package and coordinated reference are complete.

Expected: Rust is still the sole live owner; no Go runtime was invoked.

## Future G02 sequence requiring separate authorization

1. Verify fresh candidate/recovery identity and independent local observation.
2. Disable Rust scheduled starts/watchdog, stop only the verified Rust-owned tree
   and establish zero Rust/Go owners before proceeding.
3. Set the verified Go service to Manual startup. Start a fresh armed v3 watcher
   with the exact candidate/recovery paths; verify it is running and writing
   current observations before starting Go. Use the staged local commands in
   the private handover package.
4. Start the exact Go candidate, verify supervisor/worker/tunnel identity and one
   local readiness check plus one hosted read.
5. Execute local Go -> Rust rollback, time readiness (target <=120 seconds), and
   record hosted reconnection independently.
6. After successful rollback, stop the previous watcher by its recorded and
   reverified process identity. Rollback restores/enables the Rust tasks and
   leaves Go Disabled: repeat the Rust task/watchdog disable and verified Rust
   tree shutdown, establish zero owners, and set Go to Manual again. Start a new
   watcher with a separate observation file and verify fresh coverage before
   re-entering the same candidate. Repeat ownership/readiness/hosted-read checks.

## Execution record

Preparation evidence is saved under the fixed run root in
`evidence\preparation-20261001T185029Z`. Tasks 1-6 are complete. G00 remains
PASS, G01 is PASS, and preparation is READY_FOR_CUTOVER. G02 is PENDING separate
live authorization; no real cutover, rollback or Go re-entry has been performed.
The mutable `gate-results.json` and dated `cutover-readiness.json` record this
result; older manifests and gate evidence remain unchanged historical records.

- Exact current MCP binding is verified. Nine legacy Go maintenance/test tasks
  are Disabled; original XML/state and SCM configuration are retained in
  `rollback-private\preparation-20261001T185029Z`.
- The user explicitly confirmed that other Windows MCP schedules were paused
  and old widgets closed. The accessible browser required sign-in, so this
  portion is operator-confirmed, not independently observed UI evidence.
- Frozen binary/tunnel/profile hashes and seven v3 script identities match.
  All three frozen v3 fixture suites passed, including 10 compatibility cases.
- New recovery: `rollback-private\rust-snapshot-prepared-20261001T185029Z`,
  51 validated files and two Rust task definitions. Eight-action rollback
  dry-run passed. No real rollback was performed.
- The stopped/disabled Go service now selects the frozen binary and capture
  profile. No Go executable was invoked.
- Coordinated reference: 2026-10-01 18:58:07.798-18:59:03.210 UTC, ten successful
  hosted get_environment calls, >=5-second inter-call gaps, p95 1,447 ms
  dispatch-to-result. These measurements exclude model thinking time.
- Logs contain exactly ten successful handlers (IDs 1099-1108), ten forwarded
  records, one observed client, no additional tunnel events or warnings/rate
  limits in the captured window. Handler duration was 75-80 ms.
- Local observation covered 18:57:26.968-18:59:59.915 UTC: 74 samples, health
  HTTP 200 throughout, the persistent Rust tree unchanged, Go Stopped/Disabled
  throughout, no Go process and exactly one tunnel in every sample.
- The Rust watchdog remains enabled. Three short additional Rust processes
  appeared at its one-minute cadence; a subsequent targeted observation directly
  confirmed a watchdog child running read-only `service status`. The persistent
  supervisor/worker/tunnel identities did not change.
- `handover.json` records exact watcher, recovery, profile and observation paths,
  plus a future Manual trial start policy. The watcher is not running or armed.
  SCM recovery delays remain 5/15/60 seconds with a 24-hour reset period.
- The independent review found two documentation gaps, both addressed: concrete
  manual recovery and explicit full shutdown/preparation on Go re-entry.
  `STAGED_G02_COMMANDS.md` and `LOCAL_MANUAL_RECOVERY.md` in the private evidence
  directory contain the exact local commands. Ten PowerShell code blocks parse
  in Windows PowerShell 5.1; both disabled task definitions were validated in
  memory without registration. This does not demonstrate real recovery.
- Final verification preserved Rust PIDs 11304 -> 8452 -> 8468, one owned
  tunnel and local HTTP 200. Go is Stopped/Disabled/PID 0, all four asset hashes
  and seven script hashes match, and all nine legacy Go tasks remain Disabled.

The first JSON-string identity comparison returned false because parsing
timestamps as DateTime changed their serialization. Per-field comparisons,
including UTC ticks, pass. The original metric file is retained; the authoritative
result is `rust-reference-verification.json`. This was a measurement-comparison
correction, not a runtime restart or a code change.

## Paused work and restoration

The nine paused tasks are Cutover Once; Install Cutover Fix; Install Fixed Build;
Install Latest Cutover; Service Recovery JobFix Test; Service Recovery Retest;
Service Recovery Test; Tunnel Recovery Retest; Tunnel Recovery Test, each with
the exact `Codexify Go ` prefix. Their before-state file lists every full name
and XML checksum. They are obsolete maintenance/test entry points without timed
triggers; keep them disabled unless the operator intentionally chooses one.
Do not bulk-start them or auto-restore them as normal workload.

The operator resumes only the cloud schedules they paused for this trial at the
approved later phase; their names were not available to these tools. Current
source: [Scheduled task management](https://learn.chatgpt.com/docs/automations).
Rust `Codexify` and `Codexify Watchdog` remain enabled during preparation.

Ruling: this is an operational preparation/documentation task against an existing
frozen candidate. Work directly in the requested checkout, preserve unrelated
changes, use existing fixture suites and keep the execution ledger privately.
No production-code implementation, rebuild, new worktree or commit is required.

## Subsequent authorized execution

On 2026-10-02, the user separately authorized the live G02 rehearsal. It completed
with G02 PASS and state GO_TRIAL; Go is now the live owner. See the
[live report](../../handoffs/WINDOWS_G02_LIVE_REPORT_20261002.md) for the actual
process identities, measured rollback, fresh recovery path and compatibility
limitations. The preparation-only record above remains historical evidence.
