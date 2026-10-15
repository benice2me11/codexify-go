# Windows G10 runtime — read-only inspection after tunnel loss

**Date:** 2026-10-15  
**Status:** offline diagnostics available; live Windows deployment/acceptance **not run**.  
**Code baseline:** codexify-go main @ cdf8c98 (Setup v3, G03 PollWatcher).  
**Previously staged candidate:** C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261014T212234Z-setup-v3-g03-preserved\candidate\codexify-go.exe, SHA-256 8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880.  
**Previously live:** frozen G10 candidate in 20261005T005800Z-g10-tunnel-selfheal-v3, SHA-256 B908C8DE5BB5526BCFEFC69AAF631A9189963A8BBB1EC08AFD380495BF10F387.

## Why this is necessary

At the October 15 continuation, the Windows MCP tools returned: "Tunnel-client has not been seen for 300 seconds." This says the ChatGPT-managed tunnel is **currently unreachable**, not that the Go service, worker, SCM configuration or VPN is conclusively failed. No safe remote service operation can be performed through that connector until it returns.

Historical live logs on October 14/15 showed multiple "codexify-go stopped" / "codexify-go starting" episodes while the previous Windows boot continued. A clean "stopped" log is compatible with an SCM-initiated stop or cancelled parent context; it is **not** enough to identify the actor requesting a restart. The Go service_windows.go handler can return cleanly on a Service Control Manager Stop/Shutdown request. No cause has been proven.

An earlier interpretation incorrectly treated root run/evidence metrics as the active G10 observer. The **active** files were under run/evidence/g10-soak-20261013b and later reported CONTINUE/within_limits. Always locate the newest probe-observations.jsonl by timestamp before claiming coverage has stopped.

## Non-mutating inspection

From the Windows source checkout:

~~~powershell
cd C:\Users\FoxOS_User\codexify-go
$run = 'C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261005T005800Z-g10-tunnel-selfheal-v3'
$next = 'C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261014T212234Z-setup-v3-g03-preserved\candidate\codexify-go.exe'

# No Stop-Service, Start-Service, SCM update, task modification, Rust invocation or tunnel restart.
powershell.exe -NoProfile -File .\tools\windows\cutover-v2\Inspect-G10Runtime.ps1 -RunDirectory $run -CandidateExecutable $next -RecentMinutes 60
~~~

Output is redacted summary JSON containing: SCM state and PID; process counts for Go/Rust and a directly parented tunnel; Rust and G10 observer task states; recent SCM transition event IDs/timestamps (no raw event descriptions); latest guard action, freshness and selected soak evidence subdirectory; recent clean/error Go stops and restarts, poll failures, 403/429 mentions; running and candidate binary hashes. It does **not** print tunnel IDs, tokens, cookie values, raw log lines, service command-line secrets, or private file contents.

For historical evidence from copied files, use -Offline with a copy of the run directory; offline mode does not query running SCM, process tables, tasks, events, network or tunnel resources. The PowerShell-only fixture test is Test-G10RuntimeInspection.ps1, run by Windows CI. The read-only inspector performs no writes.

**Interpretation:** LOCAL_OBSERVED means only that local service/process counts and a fresh CONTINUE guard were seen together. It is **not** proof that the remote ChatGPT tunnel is connected, and never a G10 PASS. Missing/stale guard, absent owned tunnel, Rust owner, absent worker, or unavailable SCM observations should block an upgrade or acceptance pending investigation.

## Required restart investigation

If Go is running but ChatGPT sees a tunnel timeout, check the newly observed Go service PID/start timestamp, owned tunnel child, freshest watcher evidence, recent poll-failed records, and external network/VPN state. Do not assume HTTP 403 implies a process crash. An outage may require **only** a bounded Go-owned tunnel-child restart, not an SCM restart and never an automatic Rust fallback.

For unexplained *Go service* restarts, correlate (1) SCM event IDs 7031/7034/7035/7036 near "codexify-go stopped", (2) G10 watcher/observer logs and scheduled task launches, (3) actual Windows boot timestamp, and (4) external automation/service-management tools. The diagnostic script reports event times/IDs but deliberately makes no attribution without causal evidence.

## Upgrade gates (not yet cleared)

1. Confirm the current live Go service and Rust tasks; inspect a fresh local runtime summary after Windows connectivity returns.
2. Preserve a verifiable old Go SCM ImagePath/config/binary recovery and separate frozen Rust snapshot. Back up guard files; compare active watcher arguments with actual candidate path.
3. Confirm G10 observer control of candidate identity **before** any SCM binary-path change. The old guard considers unexpected ImagePath local_control_lost and may invoke Rust rollback.
4. Avoid changing the live service while a rollback-armed old-candidate watcher remains active. Stop/replace observers only within an approved maintenance transaction with bounded monitoring and a verified recovery plan.
5. Verify new candidate SHA, source commit, exact config/profile, schema +setup-v3, resource ui://codexify-go/setup/v3/mcp-app.html and compatibility with G03 PollWatcher.
6. After controlled switch: verify one Go supervisor/worker/tunnel, Rust disabled, setup mounted once, Auto/Source/Managed selection, same-chat switch/resume, hh_local and habr_career, no MCP session burst, conversation persistence and fresh watcher decisions.
7. Re-start G10 acceptance observation window for the new binary. Record evidence for at least 24 hours and the specific thresholds in WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md; do not call G10/G11 PASS based on CI, a local snapshot, or a short smoke test.

**Policy:** temporary VPN/tunnel HTTP 403/429/503 must not alone cause fallback to Rust. Preserve Go ownership, perform bounded child-level recovery, and reserve any higher-impact intervention for confirmed local safety violations with valid evidence.

**No production operations** are performed by this document or inspector. In particular, no vacancy submissions, Vacancy Hourly Watch activation or hh_local deployment is authorized.
