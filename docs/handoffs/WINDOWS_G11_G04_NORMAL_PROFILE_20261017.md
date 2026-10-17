# Windows G11-04 — normal production profile, 17 October 2026

**Operational status: NORMAL_PROFILE_ACTIVE; 30-minute smoke TECHNICAL PASS; overall G11_READY_FOR_REVIEW / NOT ACCEPTED.**

Continuation of WINDOWS_G11_GATE2_RECOVERY_HANDOVER_20261016.md. Records operator-approved normal-profile switch; does not authorize Project Groups, Rust fallback, a release or unrelated changes.

## Overnight baseline

- Windows boot: 09:26:14 Moscow (06:26:14 UTC). Go SCM supervisor restarted at 09:26:22 Moscow; the current managed tunnel was started 10:15:16 Moscow. The reboot cause is not confirmed; G11 did not record an automatic recovery attempt.
- Around 10:31: service Running / Automatic, SCM failure policy 5/15/60 seconds, loopback /health HTTP 200, 1 supervisor + 1 worker + 1 tunnel, 0 Rust.
- G10 Disabled, G11 recovery armed and reporting CONTINUE / within_limits.
- Production Go SHA256: F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2.
- Prior production config SHA256: 9453FF3D13DCD97287112D13AB5A3E8ABDC64AAAC72C910AD238AA339D6D72F1 (diagnostics=true).
- New config SHA256: 76749955DC6E8D73B68C44E11474F15055910070AF0B67F739CB39A5E28794BD (only diagnostics=false, Project Groups still disabled).
- Previous Go recovery SHA256: 8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880.

## Execution

1. Around 10:33 Moscow, Manage-G11Task.ps1 ApproveNormalProfile returned NORMAL_PROFILE_PINNED_NOT_ACTIVATED. Two protected exact-SCM-image Go-only recovery manifests now select the same preserved previous Go. Policy and config snapshots retained. Service PID 3296 unchanged. New pinned G11 policy SHA256: 03FBB74A20D4BAFD12F40D3353A075D9BC253C7E255A73EC143B6E035A405EB7.
2. Added the guarded independent one-shot operator Invoke-G11NormalProfileSwitch.ps1. SYSTEM, zero automatic task triggers, protected script, single-use lock, pinned hashes, healthy G11 and SCM-owner checks, rollback to the same Go on the old config if the new config fails. Offline fixtures on PowerShell 5.1/7 PASS. Commit 9721cf1; GitHub CI 39014812340 SUCCESS.
3. First one-shot operator task at 10:46:39 Moscow failed BEFORE changing SCM, due PowerShell SessionStateUnauthorizedAccessException from a function parameter named $PID (reserved automatic variable). Old Go PID 3296 and old config remained healthy. The failed task is Disabled; evidence and one-shot lock preserved.
4. Fixed reserved $PID parameter/assignment, added AST regression fixture. Commit e647ca7, GitHub CI 39015282835 SUCCESS. Created protected retry evidence at C:\ProgramData\CodexifyGo\G11\normal-switch-retry-20261017T075005Z, separate one-shot task, SYSTEM PLAN_ONLY PASS.
5. Retry operated independently at 10:53:37–10:53:39 Moscow and completed GO_NORMAL_PROFILE_VERIFIED, task result 0. SCM now selects diagnostics=false. The SAME Go executable runs: supervisor PID 14400, worker PID 13484, managed tunnel PID 11304. Local /health, process topology passed. G11 CONTINUE; no recovery attempt. Rust remains absent.
6. Source workspace and conversation binding preserved; Setup v3 (+setup-v3), 22 discoverable projects, previous worktrees. MCP catalog includes hh_local, habr_career and node_repl. Read-only habr_whoami PASS; hh_local.get_my_resumes blocked by external OpenAI-side safety checks, so HH integration is UNVERIFIED rather than failed.
7. One read-only smoke sample at 10:58:11 Moscow returned SINGLE_SAMPLE_PASS_NOT_ACCEPTANCE. Independent SYSTEM passive smoke task `Codexify Go 30min Smoke 20261017` began at 10:58:50 Moscow and finished at 11:29:11 Moscow. **31/31 samples passed**, spanning **1820.790 seconds**, maximum gap **61.552 seconds** (limit 140). All samples showed the same supervisor PID 14400, worker PID 13484 and tunnel PID 11304, exact executable/config SHA256, local health HTTP 200, healthy G11, no G10/Rust ownership. Task Scheduler finished with **result 0**, protected `smoke-30min-result.json` says `G11_30_MIN_SMOKE_PASS`.
8. Read-only smoke implementation/fixture committed as 8b6686a; [GitHub CI #39016064879](https://github.com/benice2me11/codexify-go/actions/runs/39016064879) **SUCCESS**. Local Windows PowerShell 5.1 offline fixtures, `go test -count=1 ./...` and `go vet ./...` all passed again during smoke.
9. The final independent smoke-evidence audit passed **16/16** checks and wrote protected `G11-G04-final-audit.json`. Its SHA256 is `8BEF4C7797D680F73B4F93C6C3289E15160CEA6F8FE7540715567A0EE7373986`. The standalone one-shot tasks (failed original operator, successful retry, finished smoke) are all **Disabled** but retained with task XML and forensic evidence. Permanent G11 remains running; no extra production restarts were performed.

## Safety and acceptance

- Immutable, ACL-protected private evidence contains before/after configs, task XML, manifests and both failed/successful operator transactions. Do not commit sensitive configurations, approval tokens or unredacted event logs.
- **G11-04 local technical gate PASSED:** 31 samples, >30 minutes, Task Scheduler 0, stable owners, no G11 recovery. This is not an operator-level waiver of the separately unverified MCP burst and HH hosted-read criteria.
- The early terminated strict new-binary G10 24-hour trial is NOT retroactively PASS. G11-05 still needs explicit operator acceptance/exception decision.
- Post-diagnostics-off reboot/logout is still pending a separately approved window.
- HH safe upstream read remains unverified because the request was blocked by a tool safety check; do not bypass or retry.
- Setup v3 is advertised and live Windows tools still work, but the bound conversation's previous connector schema was marked `conversationStale=true` (`refreshRecommended=false`); complete any intended hosted refresh separately before claiming full refreshed-schema acceptance.
- Project Groups remain disabled and Setup v4 / PG-A00..A08 still need separate canary. No Rust fallback, no Vacancy Hourly Watch, no job applications, no stable v0.9.0 release.
- The independent 30-minute monitor validates local ownership/health; it does not record request-by-request MCP burst telemetry after diagnostics were disabled. The stricter host-level duplicate-call requirement must remain a separately assessed evidence gap rather than an invented PASS.
