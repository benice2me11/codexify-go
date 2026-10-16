# Windows G11 Gate 2 — Go-only recovery authority handover (16 October 2026)

**Operational result: G11-03 HANDOVER VERIFIED. Overall G11: NOT ACCEPTED (G11-04/G11-05 pending).**

This is a record of the operator-approved Gate 2 operation, not permission to restart the service, enable Project Groups, or publish `v0.9.0`.

## Identity and protected evidence

- Source repository: `benice2me11/codexify-go`; G11 code commit `03aadb166b8dd648f7247d9851a2e8824c9663f0`, branch `feat/windows-g11-steady-state-20261016`. CI run [38894134781](https://github.com/benice2me11/codexify-go/actions/runs/38894134781): **success**.
- Production Go SHA256: `F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2`.
- Previous-Go recovery SHA256: `8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880`.
- Current config SHA256: `9453FF3D13DCD97287112D13AB5A3E8ABDC64AAAC72C910AD238AA339D6D72F1`; `mcp.diagnostics.enabled=true`, `mcp.projectGroupsEnabled=false`.
- Private, ACL-restricted evidence: `C:\ProgramData\CodexifyGo\G11\handover-20261016T114427Z\` (22 files, including both Task XML states, frozen recovery binaries/configuration, original G10 logs, observer status, task hash inventory and `G11-Gate2-verification.json`). **Do not commit or share these private contents.**
- Original G10 task XML SHA256: `6FEF852C28824AA4B1A662C02A5991E9BA445DBC51400F7335F9C3360DB4BCC5`.
- G11 read-only task XML SHA256: `E338D27984F8C26945509D170F5D54958E5566D312A24C0DBAF6C44C80F7CA14`.
- G11 recovery-armed task XML SHA256: `4A4338A3806C3299E6F5EED3FA513B84F13FC08D357D4E99DF62D25591D35D6F`.

## Executed sequence (UTC)

1. Before G10 shutdown, verified SCM Running/Automatic, recovery 5/15/60 seconds, PID 19324 (Go supervisor), PID 16868 (Go worker), PID 3028 (managed tunnel), zero Rust, and a fresh G11 `CONTINUE / within_limits`. Verified production and previous-Go SHA256. G11 remained observe-only.
2. Created the protected evidence snapshot at approximately **11:44:27 UTC**, including Go-only recovery backups and before-task XML. Original artifacts were preserved.
3. Disabled `CodexifyGo-G10Observers`. Windows Task Scheduler continued to show `State=Running` while the running observer was alive even though its `Settings.Enabled=false`; the first safety check halted before stop-marker creation. Verified the registered task XML was disabled, then wrote the observer's supported `observer.stop` marker at **11:45:59 UTC**. PID 20140 exited normally. Final G10 state: **Disabled; 0 Watch-GoTrial processes**. No Go/tunnel process was terminated.
4. Ran a 16-check pre-arming safety gate: G10 disabled/no watcher; G11 recent/healthy/read-only; source and protected G11 scripts identical; exact Go/config/previous-Go hashes; one Go supervisor/worker/tunnel; no Rust or foreign G11 task; saved evidence; local `http://127.0.0.1:3300/health` HTTP 200. Result: **GATE2_PREFLIGHT_PASS**.
5. At **11:47:10 UTC**, ran `Manage-G11Task.ps1 -Action ArmRecovery -Execute -OperatorApproved`. Result: **GO_ONLY_RECOVERY_ARMED**, with an installed approval marker, original G11 Task XML backup, and only the G11 task action changed to include `-RecoveryEnabled`.
6. Observed two distinct automatic G11 runs at **11:47:33** and **11:48:33 UTC**: both Task Scheduler result `0`, `CONTINUE / within_limits`, no recovery attempt, unchanged Go PIDs 19324/16868. A subsequent run at **11:50:33 UTC** also returned result `0`. No competing G10 watcher.
7. Tested the protected previous-Go recovery executor **without `-Execute`**: `PLAN_ONLY`, `target=previous_go`, `wouldMutate=false`. The diagnostic-off config candidate was re-hashed, but **not activated**.

**Post-handover offline fixture isolation:** An offline test invoked the source runner while the live SYSTEM task owned its production global mutex, yielding an access-denied exception in the *test invocation*. Changed the source runner so `-Offline` takes a unique local mutex; production runs retain the same shared global mutex. The offline suite then returned `G11_STEADY_STATE_OFFLINE=PASS` while the live armed task continued reporting result `0` and `CONTINUE`. This code-only fix has **not** been hot-copied to the protected production installation and does not change the deployed monitor.

## Resulting steady state

| Component | Live state |
| --- | --- |
| `CodexifyGo` | Running, same Go supervisor and managed tunnel |
| Windows SCM failure actions | RESTART 5 / 15 / 60 seconds, reset 86400 seconds |
| `CodexifyGo-G10Observers` | Disabled, no observer process; `observer.stop` preserved |
| `CodexifyGo-G11SteadyState` | Every minute + boot trigger, SYSTEM, IgnoreNew, recovery armed |
| G11 permitted recovery | Previous **Go** only; 6 consecutive minute-spaced observations AND ≥300s confirmed supervisor absence after SCM grace; never Rust, never single VPN/403/429/503 fault |
| Rust tasks/processes | Both scheduled tasks Disabled; no Rust process |
| Production configuration | Unchanged; temporary MCP diagnostics still enabled; Project Groups disabled |
| Workspaces/bindings/worktrees | Untouched |
| Vacancy Hourly Watch and applications | Untouched; not enabled/not submitted |
| Stable `0.9.0` | HOLD |

## Residual gates

**This is not G11_ACCEPTED, and it is not a new strict 24-hour G10 PASS.** The new-binary G10 observer was retired well before completing 24 hours, by explicit operator-approved transition. The earlier acceptance exception for a different Go binary does not transfer automatically.

- **G11-04 (separate approval):** candidate `%USERPROFILE%\.codexify-go\staging\g11-normal-candidate-20261016\normal.json`, SHA256 `76749955DC6E8D73B68C44E11474F15055910070AF0B67F739CB39A5E28794BD`, differs only by turning `mcp.diagnostics.enabled` from true to false. First approve the corresponding protected recovery profile, then separately authorize a controlled Go-only SCM config switch/restart. Preserve current binary/SCM and previous-Go recovery throughout. **Do not perform this as part of Gate 2.**
- **G11 30-minute acceptance smoke:** sample every minute with no unexpected gaps, stable supervisor PID, exact binary/config hashes, local /health=200, no duplicate owner, tunnel restart loop, MCP burst or Rust fallback; Setup v3 and existing bindings, source checkout, and safe upstream reads. Record an explicit operator decision afterward.
- **Reboot/logout validation:** pending a separately approved maintenance window; not simulated by crashing the live service.
- **Project Groups:** still feature-disabled; Setup v4 and PG-A00–PG-A08 must be tested in a separate rollout.
- **CI for this record:** documentation-only update needs its own PR branch check; do not conflate CI with live operational acceptance.

**Only one recovery-armed Task Scheduler observer was active at the end of this operation.** Do not re-enable G10 unless a separate rollback/handover procedure explicitly removes G11 recovery authority first.
