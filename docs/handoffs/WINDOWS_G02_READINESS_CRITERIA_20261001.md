# Windows G02 readiness criteria

> Historical phase record, versioned after G03. Ownership, process identities
> and verification results below describe that phase, not current runtime health.

This document separates preparation, authorization, a successful G02 trial and
final Go acceptance. It implements the existing
[acceptance plan](WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md) and the
[current guard handoff](WINDOWS_G02_BINDING_AND_GUARD_20261001.md).
The step-by-step execution record is in the
[preparation plan](../superpowers/plans/2026-10-01-windows-g02-preparation.md).

## Authorization boundary

The user authorized preparation, verification, documentation, and pausing
attributable competing Windows MCP work. The user explicitly prohibited a live
switch without separate confirmation. "Begin preparation" does not remove that
boundary. Do not run the Go executable, start its service, stop the live Rust
owner, execute a real rollback, or arm a live rollback watcher during preparation.

G00/G01 evidence can establish READY_FOR_CUTOVER. G02 PASS requires an actual
authorized Rust -> Go -> Rust -> Go rehearsal. GO_ACCEPTED additionally requires
G03 through G10; CUTOVER_CLOSED requires G11. A running Go service is insufficient.

## Fixed candidate

- Workspace: `C:\Users\FoxOS_User\codexify-go`.
- Run root: `C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat`.
- Binary: `candidate\codexify-go.exe` under the run root.
- Binary SHA256: `61E2A086445A1B10E7537E720119B0855E17515A58A5B3F8BDCA914075B07DD6`.
- Tunnel: `runtime\openai-tunnel\v0.0.15\tunnel-client-runtime.exe`.
- Tunnel SHA256: `A922D372D6BE0649156FBC1C8A040597F1F4BB5B4356890151D7875602593B1A`.
- Trial profile: `profiles\capture.json`, SHA256 `BCDF7B1362047E957F21B55704D69EC3783BD189B9D79759980499613D9A197F`.
- Normal profile: `profiles\normal.json`, SHA256 `75A086E6ED53124AD52B78CE35C83B8A04FC4D57A14D10C2C91645F5D692955D`.
- Operator scripts: `operator-tooling-v3`; the original `operator-tooling` and
  intermediate `operator-tooling-v2` are historical evidence, not the trial tools.
- Rust executable: `C:\Users\FoxOS_User\.codexify\bin\codexify.exe`.

Do not rebuild or change frozen source, binaries, profiles or upstream routing
as part of preparation. Preserve the existing dirty checkout and old evidence.

## Entry criteria before G02

| ID | Required evidence | Completion rule |
| --- | --- | --- |
| R1 | Current MCP workspace | Exact resumePath binding; hosted get_environment returns the populated checkout. No substitute empty checkout or new worktree. |
| R2 | Frozen identity and tooling | Both executable hashes and profile hashes match. Workspace tools match v3. Guard, log-compatibility and watcher-lifecycle fixture suites pass. These fixtures do not execute Go or affect live services. |
| R3 | Exclusive Rust owner | Exact Rust path and parent chain, one Rust-owned tunnel, Rust task Running; Go service Stopped/Disabled, PID 0, zero Go runtime processes. Repeat after preparation. |
| R4 | Prepared Go service | Save original SCM configuration. Set only the stopped/disabled service ImagePath to the exact frozen binary and capture profile; read it back. Do not start Go. |
| R5 | Independent recovery | Fresh private Rust snapshot includes this conversation binding and both Rust task definitions. All manifest hashes validate. Dry-run has the eight ordered actions below. Local administrator control is available without MCP. Actual recovery remains unproven until G02. |
| R6 | Competing work controlled | Inventory accessible chats, schedules and clients; save and disable identified old Go maintenance/test tasks. Keep Rust startup/watchdog active. Obtain evidence that other Windows MCP jobs are paused and old widgets are closed, including inaccessible clients. Idle chat status does not prove a widget is closed or a cloud schedule is paused. |
| R7 | Coordinated Rust reference | Only after R6: timestamp process/health/log snapshots and at least 10 hosted get_environment calls, sequentially with at least 5 seconds between calls. Record dispatch-to-result latency, success and coverage. No extra connector activity during the interval. |
| R8 | Concrete handover package | Exact candidate/profile/recovery/script paths, observation paths, startup policy and manual fallback are recorded. Stage the watcher command, but leave it unarmed and unstarted during preparation. Verify fresh observation at every authorized entry. |
| A1 | Separate live authorization | Explicit user authorization for Rust -> Go -> real Rust rollback -> Go re-entry, immediately before executing that trial. Preparation approval and quiet-window confirmation are not A1. |

If R1-R8 are incomplete, record the missing evidence and keep G01/G02 BLOCKED.
Do not replace missing operator/UI evidence with quiet logs, fixture results or
historical success. If R1-R8 pass, record G01 PASS and READY_FOR_CUTOVER; A1 still
controls whether execution can begin.

Rollback dry-run order: `validate_snapshot`, `validate_go_service_identity`,
`stop_disable_go`, `verify_go_tree_gone`, `restore_rust_files`,
`restore_rust_tasks`, `start_rust`, `verify_rust_ready`.

## Recorded preparation result (2026-10-01)

R1-R8 are complete for this frozen candidate: G00/G01 PASS,
READY_FOR_CUTOVER. A1 is still absent, so G02 remains PENDING. Rust is the live
owner; Go is Stopped/Disabled. Revalidate time-sensitive ownership, quiet-window
and observation conditions immediately before any separately authorized trial.

The dated evidence directory is `evidence\preparation-20261001T185029Z` under
the fixed run root. Its `cutover-readiness.json`, `handover.json`,
`STAGED_G02_COMMANDS.md` and `LOCAL_MANUAL_RECOVERY.md` contain the current
ledger and local operating instructions. The preparation plan records actual
measurements and limitations. Historical candidate manifests are preserved.

The subsequent separately authorized rehearsal completed on 2026-10-02:
G02 PASS, current state GO_TRIAL. See the
[live G02 report](WINDOWS_G02_LIVE_REPORT_20261002.md) for current ownership,
measured rollback and remaining compatibility work. The preparation result
above describes the earlier state before that authorization.

## G02 success criteria after authorization

1. Before each Go start, Rust is stopped and its scheduled starts/watchdog cannot
   reintroduce it. Independent local observation is active. Start and verify the
   armed v3 watcher after exclusive Rust shutdown and before Go start.
2. SCM supervisor, interactive-user worker and exactly one owned tunnel have
   the expected executable paths and candidate hashes; no Rust runtime serves MCP.
3. Local readiness and one real hosted read succeed on the candidate.
4. A real rollback initiated locally, outside MCP, restores Rust readiness
   within 120 seconds and leaves no Go-owned tree. Record hosted reconnection
   separately from local readiness and user refresh time.
5. Re-entry uses the same candidate/profile and a fresh verified watcher;
   ownership, readiness and a hosted read succeed again.
6. No safety abort, unexplained traffic, duplicate side effect or observation gap
   is hidden. A failed real rollback ends the trial on a recovery path.

The capture profile lasts at most 10 minutes / 20,000 events. Verify coverage
before every phase. An expired trace or stopped watcher is not current evidence.

## Stop conditions

The existing acceptance thresholds remain unchanged: duplicate owners, extra
mutation, workspace leakage, exposed secrets or lost local control require an
immediate stop/rollback during the authorized trial. In a quiet window after
30 seconds settling, >=10 unexplained tools/call in 10 seconds, >=5 repeated
operation hashes in 10 seconds, or >100 forwarded events in 60 seconds abort.
Any rate limit pauses new stimuli. Confirmed amplification aborts; a demonstrated
external rate limit holds the trial. Two unexpected owned-process restarts in
5 minutes or lost observation also abort. Low-rate persistent feedback still
fails acceptance even below these emergency ceilings.

During preparation, a failed check stops dependent work and preserves Rust.
It does not authorize a live rollback.

## After G02

| Gates | Required outcome |
| --- | --- |
| G03-G07 | Fresh client schemas, two real conversations, normal tools/exec/state, project isolation, hosted widgets without amplification, required upstreams and artifact/Git workflows. |
| G08-G09 | Verified process recovery and actual OS/session persistence; reboot/logout require separate user coordination. |
| G10 | Same-candidate 24-hour elapsed observation, >=4 awake connected hours, two work sessions totalling >=2 hours, >=50 intentional actions, three 15-minute idle windows, continuous coverage and the original latency/resource targets. |
| G11 | Traceable deployed source, verified normal profile, final restart and 30-minute observation, explicit acceptance and restoration of approved paused work. Retain Rust recovery for >=7 days and three successful work sessions after GO_ACCEPTED. |

Every PASS record identifies candidate/profile, start/end timestamps, expected
and observed results, evidence paths and checker. Store raw traces, credentials,
SCM/task backups and recovery state privately. Commit only reviewed safe reports
if separately requested; no push is part of preparation.
