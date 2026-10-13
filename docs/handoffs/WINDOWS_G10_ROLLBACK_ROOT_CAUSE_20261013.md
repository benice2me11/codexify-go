# G10 Go -> Rust rollback root-cause and guard hardening (2026-10-13)

Scope: Windows Go v3 candidate at 6387f542. This work changes **operator tooling and tests only** in a separate Git worktree. The frozen v3 executable, live Rust process tree, scheduled tasks, service configuration, and recovery snapshot are not modified.

## Evidence timeline (local time, UTC+03:00)

| Local time | Evidence | Root cause |
| --- | --- | --- |
| 2026-10-05 05:37 | `evidence/go-trial-capture.log`, `evidence/local-samples.jsonl`, `evidence/probe-observations.jsonl` | Control-plane poll received repeated 403 `unsupported_country_region_territory`. PollWatcher restarted only the tunnel child (17708 -> 4476). Supervisor 23692 and worker 9556 remained. Old guard counted tunnel exit and respawn as two PID signature transitions and emitted `ABORT owned_process_restarts count=2` at 05:37:32. |
| 2026-10-06 06:51 | `WINDOWS_G10_SOAK_HANDOFF_20261013.md` | An armed watcher was relaunched manually, encountered `observation_failed` and rolled back with insufficient diagnostics. Precise exception cannot be reconstructed from surviving evidence. |
| 2026-10-09 08:33 | `evidence/guard-abort.log`, `evidence/probe-observations.jsonl`, `evidence/watcher.stdout.log` | 3 consecutive PowerShell NameNotFound exceptions: watcher invoked private module function `ConvertTo-CutoverTime` on a restart-signature transition. `goOwners=2`, `rustOwners=0`, `controlled=true`; then an observer fault caused `RUST_RECOVERED`. |

In the observed October 5 and October 9 cases, the guard initiated rollback without evidence that the Go supervisor or worker had crashed. Tunnel self-heal is not equivalent to a Go service failure. A 403 from the control plane is a network/egress constraint and cannot be fixed by restarting the Go binary.

## Fixes in this branch

- Retain the on-host improvements: startup observation marker, three-attempt diagnostic tolerance, durable guard-abort log before confirmed rollback.
- Move restart-episode coalescing into exported, unit-tested module function `Test-CutoverRestartEpisode`. This avoids the private-function scope bug entirely and coalesces child exit+respawn within 30 seconds.
- An observer exception alone cannot trigger Go -> Rust automatic failover. It records `observation_failed`, after 3 failures records `OBSERVER_DEGRADED` and `HOLD observer_unhealthy`, retries, and invalidates G10. A separate, successfully observed owner conflict or loss of local control still causes an armed rollback.
- Preserve the existing hard stop for 2 genuinely independent owned-process restart episodes in 5 minutes and the quiet-window MCP burst policies.
- Regression fixtures cover the October 5 PID transition pair, a second independent restart episode, and the October 9 class of malformed/invalid observations.

## Acceptance before next Go trial

1. Run `Test-CutoverTooling.ps1`, `Test-CutoverWatcherLifecycle.ps1`, and `Test-CutoverLogCompatibility.ps1` with no production service or scheduled-task mutations.
2. Verify the exact candidate hash stays `B908C8DE5BB5526BCFEFC69AAF631A9189963A8BBB1EC08AFD380495BF10F387`.
3. Stage this **new** tooling in a separate, hash-pinned operator-tooling directory. Do not edit the frozen v3 binary or silently replace the active watcher. Update the entry script's hash manifest and observer launch paths consistently, and verify the launched watcher comes from the intended build.
4. Confirm a healthy Rust fallback only as a rollback asset; do not spend time redesigning Rust. Perform a dry-run with refreshed operator authorization and zero duplicate owners before any live switch.
5. Re-enter G10 and measure an uninterrupted 24-hour observation window. Every `OBSERVER_DEGRADED`, missing observer coverage, or required safety rollback invalidates that window. Keep G11 pending until G10 is accepted.

This branch is an offline tooling fix, **not** a claim of a successful new cutover or completed soak.

## VPN outage policy and G10 reentry preparation (2026-10-13)

VPN-induced control-plane `403 unsupported_country_region_territory` and `poll transport failed (network_error)` are `HOLD egress_degraded`, not automatic Rust rollback. Go remains the sole connector owner, while the bounded PollWatcher retries tunnel recovery when network connectivity returns. A sustained external 403 might require connectivity changes; restarting Rust does not cure it.

Only Go supervisor and Go worker PIDs count toward the `owned_process_restarts` guard threshold. Tunnel runtime PID replacement is not a Go service restart. A duplicate Go process topology, repeated Go supervisor/worker restart episodes, duplicate Rust+Go ownership, or confirmed loss of local service control still aborts.

The old sampler exited 2026-10-13 05:35 with an IOException writing `local-samples.jsonl` while another process was using the file. The new 24-hour soak uses a dedicated G10 evidence directory and a bounded, shared-file append retry writer. Observer failures mark G10 degraded without automatic rollback.

**Frozen Go v3 binary SHA256:** B908C8DE5BB5526BCFEFC69AAF631A9189963A8BBB1EC08AFD380495BF10F387.
**Staged tooling:** C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\operator-tooling-g10-20261013.
**Entry script:** `evidence/Invoke-GoReentry-G10-20261013.ps1` (on Windows host, not Git checkout).
**Boot observers:** `evidence/Start-G10Observers-G10-20261013.ps1`.
**Dedicated evidence:** `evidence/g10-soak-20261013/`.

Acceptance before live relaunch: all three staged PowerShell suites PASS, including 12 log compatibility cases; pinned tunnel runtime v0.0.15 and 51-file rollback snapshot validate; new sampler produced four good fixture rows; fresh entry dry-run returned `READY_DRY_RUN` without service mutation; WMI-created detached operator process was verified elevated and parented by `WmiPrvSE.exe`, not by the Rust connector tree.

Do not mark G10 PASS until a fresh uninterrupted 24-hour soak is observed. Rust remains an unchanged recovery asset; no Rust watchdog development is required.
