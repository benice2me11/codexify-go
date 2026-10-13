# Windows Rust→Go Cutover — Agent Handoff (2026-10-13)

Goal: replace the working Rust ChatGPT connector (`codexify.exe`) with `codexify-go`
on this Windows host, one tunnel owner at a time, Rust retained as verified
rollback asset. Original ask (RU): tunnel for the web ChatGPT version with
separate limits. Target is **web ChatGPT via MCP tunnel**, not Codex Desktop.

## Current state (verified 2026-10-13 ~05:38 local)

- **Owner: Rust** (codexify.exe supervisor + worker + tunnel-client-runtime).
- `CodexifyGo` SCM service: **Stopped / Disabled**, PathName already points at
  the v3 candidate (see below).
- G10 24h soak window: **BLOCKED** — must restart from a clean window.
- Go candidate: unchanged, still valid. No candidate-code fixes needed.

## Candidate identity (v3, frozen)

```
run dir:   C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261005T005800Z-g10-tunnel-selfheal-v3
exe:       <run>\candidate\codexify-go.exe   sha256 B908C8DE5BB5526BCFEFC69AAF631A9189963A8BBB1EC08AFD380495BF10F387
profile:   <run>\profiles\capture.json
head:      6387f542 (branch codex/windows-cutover-g03)
tunnel:    runtime\openai-tunnel\v0.0.15\tunnel-client-runtime.exe, tunnel_id tunnel_6ab7cf3674248191881e90cf863352bb
recovery:  C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\rollback-private\rust-snapshot-g04-20261002T155913Z
tooling:   C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\operator-tooling-v3
```

## Architecture quick map

- `CodexifyGo` SCM service spawns Go supervisor → worker + managed
  `tunnel-client-runtime` child (OpenAI control-plane tunnel).
- Rust side = scheduled tasks `Codexify` (boot `service run`) + `Codexify Watchdog`
  (every ~60s `watchdog.ps1` → `codexify.exe service restart` when unhealthy).
- `Watch-Cutover.ps1` (armed, `-ArmRollback`) evaluates every 2s; on ABORT it
  calls `Invoke-ProbeRollback.ps1 -Execute` → `Rollback-To-Rust.ps1 -Execute`.
- Observers: `Start-G10Observers.ps1` (run-dir `evidence\`) launches
  sampler (`Observe-G05Local.ps1`, 5s), metrics (`Collect-G10Metrics.ps1`, 60s),
  armed watcher; scheduled task **`CodexifyGo-G10Observers`** relaunches at boot.
- Evidence: `<run>\evidence\` → `probe-observations.jsonl` (guard decisions),
  `guard-abort.log` (every abort + observation_failed, written BEFORE rollback),
  `local-samples.jsonl`, `g10-metrics.jsonl`, `observer-launches.jsonl`,
  `go-trial-capture.log` (tunnel runtime log).

## Incident history — READ THIS FIRST

Two tooling-induced rollbacks (candidate never at fault):

1. **2026-10-06 06:51 local** — manually relaunched armed watcher aborted ~10s
   in (`observation_failed`, silent launch, no logs) → rollback. Go had run ~25h
   clean incl. proven self-heal.
2. **2026-10-09 08:33 local** — `observation_failed` x3 → abort → rollback.
   Root cause found in `guard-abort.log`: `ConvertTo-CutoverTime` is NOT
   exported by `CutoverGuard.psm1`; the 30s signature-merge code path threw
   NameNotFound on the first restart episode. **Fixed 2026-10-13**: local copy
   added to `Watch-Cutover.ps1`.

### Recurring Rust-side defect (not Go's)

`codexify.exe service restart` spawns a **new** `service run` supervisor without
killing old ones → duplicate supervisors → extra workers die on
`os error 10048` (:3000 busy) → watchdog sees unhealthy → restart every ~60s →
self-amplifying churn. Observed 2026-10-08..13; 250+ respawns.
**Remediation**: kill duplicate `service run` supervisors + wedged workers,
keep exactly one supervisor+worker pair (check `netstat -ano | findstr :3000`).

## Guard/tooling patches applied (post-freeze)

`Watch-Cutover.ps1` current sha256: **D4CEAA1DF77528387A441DA31B0EABB6258DA825B1E4B99D7E6D1B492118975E**
- Preflight `guard_started` marker — dies loudly at launch if observation path
  unwritable instead of arming blind.
- `observation_failed` needs 3 consecutive failures; logged per-failure.
- `guard-abort.log` durable record before any rollback.
- 30s signature-transition merge (one kill+respawn = one restart episode).
- Local `ConvertTo-CutoverTime` (fixes abort #2).

`Start-G10Observers.ps1`: dedup by CommandLine, timestamped stdout/stderr per
observer, `observer-launches.jsonl` audit. Entry script
`evidence\Invoke-SelfhealEntry.ps1` updated: tooling override hash,
12-task expectation (incl. `CodexifyGo-G10Observers`),
`workspace-last-used`/`memory.json` exempt from Rust integrity check
(runtime-mutable markers).

**IMPORTANT**: watcher hash changed again — update `$toolingOverrides` in
`Invoke-SelfhealEntry.ps1` to `D4CEAA1D...` before next entry (script rejects
unknown tooling hashes by design).

## Re-entry runbook (Rust → Go v3)

1. Verify Rust fallback healthy: 1 supervisor + 1 worker + 1 tunnel; kill
   duplicate `service run` PIDs if crash-looping (log: `os error 10048`).
2. Update tooling override hash (above), fresh `evidence\authorization.json`
   (all flags true, `confirmedAtUtc` ≤10 min), rename old
   `handoff-started.json`.
3. Dry-run: `Invoke-SelfhealEntry.ps1` → `READY_DRY_RUN`.
4. Execute: `Invoke-SelfhealEntry.ps1 -Execute` → stops Rust tasks+tree, starts
   service, sampler + armed patched watcher, verifies readyz 200 + capture.
5. `Set-Service CodexifyGo -StartupType Automatic`.
6. Normalize observers: kill the entry's 120-min sampler, run
   `Start-G10Observers.ps1` (starts 26h sampler+metrics, skips running watcher).
7. Record G10 RUNNING window in
   `cutover-v2\20261001T160710Z-node-repl-compat\gate-results.json`.

## Acceptance status (docs/handoffs/WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md)

- G00–G09: **PASS** (incl. reboot/logout recovery, tunnel self-heal proven live:
  403-poll storm → PollWatcher trip → restart; cooldown restarts at exact 6-min
  grid; auto-recovery when VPN returns).
- G10: BLOCKED twice by tooling aborts + observer coverage gaps
  (missing-coverage rule). Needs: ≥24h observed, ≥4h awake, 2 work sessions,
  ≥50 tool actions, 3× 15-min quiet windows, ≤3% idle CPU, no unexplained
  restarts.
- G11: pending.

## Hard rules

- Exactly one connector owner at a time. Never start Go+Rust together.
- Rollback triggers (armed guard): duplicate live owner, ≥2 owned-process
  restarts/5min, `local_control_lost` (service PathName ≠ candidate),
  observation failure ×3, MCP call bursts in quiet window.
- Tunnel client sticks to egress at spawn: after VPN flap it needs a restart —
  PollWatcher does this (3 consecutive poll failures → restart, 6-min cooldown).
  Known Rust parity: same binary, same behavior, Rust just never healed it.
- Rust is frozen recovery asset — do not modify `.codexify\bin` or its config.

## Useful commands

```powershell
# who owns
Get-CimInstance Win32_Process | ? {$_.ExecutablePath -match 'codexify|tunnel-client'}
Get-Service CodexifyGo
# guard + abort history
Get-Content <run>\evidence\probe-observations.jsonl -Tail 5
Get-Content <run>\evidence\guard-abort.log
# rust crash-loop check
Select-String 'os error 10048' C:\Users\FoxOS_User\.codexify\logs\codexify.log -List
```
