# Windows G11 — implementation and controlled handover (16 October 2026)

**Status: G11-03 LIVE HANDOVER VERIFIED / overall G11_READY_FOR_REVIEW, NOT ACCEPTED. G11-04 and G11-05 remain pending.**

This document includes an original preparation baseline and subsequent Gate 1/Gate 2 execution. The live SCM service, Go binary, tunnel, production config, Rust tasks and Project Groups were not switched. No G10 or overall G11 acceptance is implied. See [Gate 2 live handover record](WINDOWS_G11_GATE2_RECOVERY_HANDOVER_20261016.md).

## G11-00 — read-only baseline

- Active source checkout: C:\Users\FoxOS_User\codexify-go, managedWorktree=false. Main was clean and synced at d9d549293f71ede01805213415286b85b7102ad2.
- Windows CodexifyGo service Running/Automatic, supervisor PID 19324, one worker, one managed tunnel, zero Rust.
- Windows SCM failure actions: RESTART after 5, 15, 60 seconds; counter reset 86400 seconds.
- Local health endpoint is **http://127.0.0.1:3300/health** (live HTTP 200 from the Go worker). Port 3000 is not the current Windows worker listener; using it would produce a false HOLD. The monitor defaults to port 3300.
- Running Go SHA256: F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2.
- Current config SHA256: 9453FF3D13DCD97287112D13AB5A3E8ABDC64AAAC72C910AD238AA339D6D72F1. MCP diagnostics enabled, Project Groups disabled.
- Previous Go SHA256: 8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880.
- Frozen operator-v6 manifest SHA256: 2FBFE8E5EE8EBAEE206AF835D84C989F270C7E61C36375545D863C5FE79E0403.
- At the initial audit, G10 observer was one running 27-hour Watch-GoTrial.ps1, recovery-armed for the old Go. Watcher PID 20140 started at 2026-10-16 07:59:54 UTC, originally estimated to expire 2026-10-17 10:59:54 UTC (13:59:54 Moscow). Original Task XML exported-text UTF-16LE SHA256: 851BFB832107712BCC37AF1F2D6038FD95F5B6D93D54F0741BBD29ADA818D507. **After Gate 2 the task is Disabled and PID 20140 exited normally; G11 is the only recovery-armed observer.**
- Inspector LOCAL_OBSERVED, fresh CONTINUE/within_limits, two Go processes and no Rust. This is not strict 24-hour G10 acceptance for the new binary.
- Rust tasks Codexify and Codexify Watchdog remain Disabled. The single-use PG canary task was not rerun. Vacancy Hourly Watch was not touched.

Preserve private production artifacts under %USERPROFILE%\.codexify-go\staging\project-groups-v1-cd6b5bd59dea\. Never commit or export unredacted manifests, configurations, task XML, approval tokens or runtime logs.

## G11-01 — ownership and monitoring model

Windows SCM alone owns crashed-supervisor restart (5/15/60); Go supervisor owns worker/tunnel restart. G11 **must not start another service runner, worker, tunnel or Rust executable**.

New monitor is one Task Scheduler invocation per minute plus BootTrigger, IgnoreNew and a named cross-process mutex. Unlike G10's 27-hour resident observer it exits after one sample. The repeating trigger runs for ten years; schedule renewal before the long expiration. SYSTEM principal is independent of interactive user logon and the ChatGPT MCP worker. Task scripts, prior-Go-only recovery code, state and pinned hashes are installed under ACL-protected %ProgramData%\CodexifyGo\G11 (never execute SYSTEM code directly from a writable source checkout).

- CONTINUE: correct SHA-pinned current Go/config, 1 supervisor, 1 worker, 1 tunnel, healthy loopback endpoint, no Rust.
- HOLD: worker/tunnel self-healing, degraded local health, stale G10 watcher, SCM uncertainty or a confirmed outage that still needs operator recovery. Network 403/429/503/VPN failure is never a rollback input.
- BLOCKED: unexpected SCM image/hash, duplicate/orphaned Go or tunnel, Rust owner/task, foreign or competing watchdog. Never attack foreign processes or SCM owners.
- OBSERVATION_ERROR: local sample/evidence persistence failure, fail closed.
- RECOVER_GO: only after six minute-spaced samples and at least 300 seconds of supervisor absence, after SCM recovery grace, previous Go/hash verification, independent authorization, no armed G10 watcher or Rust, recorded one-shot attempt and a final fresh SCM/process check.
- Recovery compatibility after diagnostics-off: G11 maintains a **separate protected SHA-pinned v1 Go-only recovery manifest per explicitly approved SCM ImagePath**. The initial diagnostics-on profile and the later diagnostics-off normal profile each select their own verified manifest; both target the same preserved previous Go. The original frozen operator-v6 manifest and code are not modified. Duplicate or unknown recovery profile selection fails closed. Both forms are tested with the original Go-only executor in plan-only mode.
- GO_PREVIOUS_RECOVERED: old Go already healthy and pinned; prohibit repeat recovery and oscillation. Unhealthy previous Go remains HOLD.

Events are UTC, transition-only plus 15-minute heartbeat, bounded JSONL rotation (four chunks of 2 MiB), no secrets or command lines. Monitor state freshness and Task Scheduler LastRunTime must be checked by the operator; this design does not deliver external push alerts.

## G11-02 — prepared changes

- tools/windows/g11/G11SteadyState.psm1 — pure policy and single-owner protection.
- tools/windows/g11/Invoke-G11SteadyState.ps1 — bounded CIM/SCM/task/health sample, protected evidence and gated previous-Go-only recovery.
- tools/windows/g11/Manage-G11Task.ps1 — PlanInstall, InstallReadOnly, Inspect, ArmRecovery, ApproveNormalProfile and Remove. All mutations require both -Execute and -OperatorApproved; removal also requires -SuccessorVerified.
- tools/windows/g11/New-G11NormalConfig.ps1 — byte-for-byte preservation of every field except the diagnostics enabled token, writes only a private inactive candidate.
- tools/windows/g11/Test-G11SteadyState.ps1 — AST, pure policy, network, duplicate owners, stale watcher, cross-run recovery grace, loop prevention, error handling, offline execution and native Task Scheduler fixtures.
- .github/workflows/ci.yml — adds Windows native G11 fixtures to the existing Go tests, vet, race, native builds and cross-build gates.

Local G11 fixtures, existing Go-only policy and transaction fixtures, go test -count=1 ./... and go vet ./... passed. PR #37 commit 03aadb1 CI [38894134781](https://github.com/benice2me11/codexify-go/actions/runs/38894134781) completed successfully.

Inactive candidate normal config: %USERPROFILE%\.codexify-go\staging\g11-normal-candidate-20261016\normal.json. SHA256 76749955DC6E8D73B68C44E11474F15055910070AF0B67F739CB39A5E28794BD. Only mcp.diagnostics.enabled changes true to false; the output is one byte longer. Project Groups is still disabled. The current SCM image has not been altered.

## G11-03 — controlled two-stage handover (EXECUTED AND VERIFIED)

**Live update, 16 October 2026:** Gate 1 installed the read-only monitor and verified at least five scheduled runs. Gate 2 was explicitly approved and completed at 11:47:10 UTC: the original G10 task was disabled, `observer.stop` caused the only original watcher to exit, the prior Go backup and Task XML were privately snapshotted, and the G11 task was armed. Two subsequent automatically scheduled samples returned `CONTINUE / within_limits` with Task Scheduler result 0, no recovery attempt and unchanged Go/tunnel PIDs. See [operational evidence report](WINDOWS_G11_GATE2_RECOVERY_HANDOVER_20261016.md). The original procedure below remains the repeatable runbook, **not pending work**.

**Gate 1, after explicit operator approval:**

1. Capture fresh hashes, G10 task XML, latest guard/evidence timestamps, 1 watcher PID, SCM 5/15/60 failure policy, Go topology, old-Go recovery bytes and Rust-disabled state. Keep originals in private immutable evidence; do not invoke the completed PG canary transaction.
2. Run Manage-G11Task.ps1 with -Action PlanInstall first. Require no other G11 tasks and one healthy G10 watcher.
3. Separately authorize -Action InstallReadOnly -Execute -OperatorApproved. This installs only the independent SYSTEM read-only G11 task with secured code and one-minute repeat; G10 remains the **sole armed recovery owner**. It must not restart the live service, tunnel or worker.
4. Observe at least five consecutive task invocations: valid pinned profile, fresh state/evidence, one registered G11 task, no duplicate Go/tunnel owners, no spurious recovery, no growing error log. Verify Task Scheduler last run and exit code. Keep G10 in place throughout.
5. If G11 fails: disable/remove the new read-only G11 task using protected task XML and documented operator steps, leave G10 running and investigate offline. No Rust fallback.

**Gate 2, separately authorized recovery-authority transfer:**

1. Verify G11 reads remain healthy and save the exact current G10 task XML, all guard/event evidence and the previous-Go recovery snapshot.
2. Disable the G10 scheduled task, request the old watcher to exit using its private observer.stop marker, and verify the original watcher PID exited. Only if necessary and with the same authorization, stop the remaining G10 task process; do not kill Go or tunnel owners. Keep G11 read-only monitor running throughout; SCM continues 5/15/60 restarts.
3. Check **G10 task Disabled and zero Watch-GoTrial watchers**, zero Rust tasks/processes, zero foreign G11 task and verified protected installed code/assets. If any are false, STOP: new recovery must remain disarmed.
4. Run Manage-G11Task.ps1 -Action ArmRecovery -Execute -OperatorApproved; it saves prior G11 task XML, creates one approved recovery marker, and changes only the G11 task action. It does not change the Go service. Verify single recovery authority and monitor continuity.
5. Simulate only on disposable SCM fixtures, not the production service. Test reboot/logout with a separately approved maintenance window; compare task last-run freshness before/after. Do not introduce a forced production crash simply to test a watchdog.
6. Retain the disabled G10 task/observer, G10 evidence, previous Go files, and one-shot operator transaction as recoverable history; do not delete them. If handover cannot complete before the estimated G10 deadline, obtain a **separate** approval for safe G10 extension instead of silently re-arming or duplicating the watcher.

**G11-03 status is LIVE HANDOVER VERIFIED.** A future reboot/logout test still requires an independently approved window. Overall G11 stays `G11_READY_FOR_REVIEW`, not `G11_ACCEPTED`, until G11-04 final production profile restart, 30-minute smoke and G11-05 operator acceptance. The current-binary G10 did **not** reach a new strict 24-hour PASS before the handover.

## G11-04 — normal-profile switch and 30-minute smoke TECHNICAL PASS

**17 October 2026 update:** The independent SYSTEM one-shot transaction successfully switched to the SHA-pinned diagnostics-disabled profile at 07:53:39 UTC, reusing the same current Go executable (SHA256 `F5AF36CE...D3A0DB2`). Local supervisor/worker/tunnel readiness and loopback health succeeded, Task Scheduler returned 0, G11 remained `CONTINUE`, Project Groups stayed disabled and no Rust/previous-Go fallback occurred. A first attempt had failed *before any SCM modification* due to the reserved PowerShell `$PID` variable; the script and offline regression test were fixed before the successful fresh retry. A separate SYSTEM read-only smoke then recorded **31/31 PASS from 07:58:50 to 08:29:11 UTC**, duration **1820.790s**, maximum gap **61.552s**, with stable Go/worker/tunnel PIDs. A subsequent independent 16/16 evidence audit passed. Temporary one-shot tasks have been disabled; permanent G11 was not changed. See [actual transaction and smoke record](WINDOWS_G11_G04_NORMAL_PROFILE_20261017.md) and [G11-05 acceptance ledger](WINDOWS_G11_ACCEPTANCE_20261017.md). **Overall G11 remains READY_FOR_REVIEW / NOT ACCEPTED; explicit risk/exception disposition still pending.**

The following numbered steps are the original runbook, **not all pending work**:

1. Obtain separate operator authorization after G11 steady-state deployment. Confirm current SCM executable SHA and normal private config candidate SHA. Freeze the previous SCM ImagePath/config, G11 task/profile policy, current evidence and Go-only recovery files.
2. Explicitly authorize Manage-G11Task.ps1 -Action ApproveNormalProfile -ApprovedConfigPath [private staged normal.json] -ApprovedConfigSha256 76749955DC6E8D73B68C44E11474F15055910070AF0B67F739CB39A5E28794BD -Execute -OperatorApproved. The protected monitor then accepts both existing and diagnostics-off command lines **and selects a corresponding protected previous-Go recovery manifest for either path**, but **does not switch the service itself**. Verify both recovery selectors in offline fixtures and only approve this config during a separate authorized window.
3. Independently authorize one controlled Go-only SCM ImagePath/config switch and final restart, using the existing approved Go executable. Require exactly one service and tunnel owner, no Rust, no second watchdog and Project Groups disabled. On unexpected failure, use the preserved verified previous Go only under the approved recovery policy.
4. Observe the 30-minute interval with task samples each minute and no unexplained gaps greater than two intervals. Verify qfailure 5/15/60 remains configured, stable supervisor PID and worker/tunnel topology, exact approved binary/config SHA, no owner restarts, repeated local /health=200, no new MCP burst or duplicate calls.
5. Run ChatGPT Setup handshake with existing Setup v3 (not v4); ensure conversation binding, source checkout and other projects/worktrees remain intact. Read MCP sources, use only authorized safe upstream reads (hh_local get_my_resumes and habr_career habr_whoami), and confirm no vacancy submissions or scheduled vacancy watch.
6. Capture start/end UTC, exact sample count, missed intervals, task XML hash, process restarts, MCP observations, and operational rollback readiness. A failure cannot be declared PASS by retiming the interval or borrowing old G10 evidence.

## G11-05, PG and release decisions

- Current report is **G11_READY_FOR_REVIEW**, not G11_ACCEPTED. A documented operator approval plus live handover, normal profile restart, 30-minute smoke and all other applicable G11 checks are required.
- New binary G10 is still RESET_AND_RUNNING_NOT_ACCEPTED until its own actual 24-hour requirement or a new binary-specific risk decision. The old-Go acceptance exception does not transfer automatically.
- Retain Rust forensic/recovery assets for at least seven days and three successful normal sessions after GO_ACCEPTED; do not enable them as fallback or remove evidence automatically.
- Project Groups implementation is already in the current binary but disabled. Keep Setup v3 and existing source/managed worktrees. Test Setup v4 and PG-A00..PG-A08 through a **separate** authorized feature-flag change; do not combine it with diagnostics-off.
- Do not publish stable v0.9.0, enable Vacancy Hourly Watch, send applications, or modify unrelated job-search repositories during G11.
