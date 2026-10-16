# Windows Go-only trial → Go-only Project Groups transaction (2026-10-16)

**Status:** PREPARED (read-only operator snapshot), execution **HOLD**, production SCM unchanged.

**Source revision:** `main` at `cd6b5bd59dea0be35e3107446dfddce322f41ad2`. The staged candidate was compiled from this revision and previously passed Windows/Linux/macOS native CI plus compiled-worker and previous-Go fixture smoke.

**Operator scripts:** merged through PR #30 at `b7a39f08717dee6b3cfee1316694e3b553f827f4`; the frozen operator source is `00647a6e976105d2ea641d0eadb79cad5db80c5e`, which passed all 11 checks in [CI 38858554632](https://github.com/benice2me11/codexify-go/actions/runs/38858554632). Only the PowerShell/docs/CI files changed after the Go binary was built.

## Why a new transaction was required

The old `Prepare-GoToGoUpgrade.ps1` / `Invoke-GoToGoUpgrade.ps1` assumes the *previous* rollback-armed `Watch-Cutover.ps1 -ArmRollback` process. Production now has a **Go-only** `Watch-GoTrial.ps1 -ArmGoRecovery`, one matching watcher process and an Automatic `CodexifyGo` SCM service. The old read-only preparer correctly fails with `Expected exactly one old rollback-armed watcher, got 0`. Executing the old transaction against this topology is unsafe.

The new transaction never references a Rust executable as a recovery target and never enables `Codexify` or `Codexify Watchdog`.

## Frozen candidate and operator assets

Private protected stage:

`C:\Users\FoxOS_User\.codexify-go\staging\project-groups-v1-cd6b5bd59dea`

- Candidate Go binary SHA256: `F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2`.
- Previous verified live Go SHA256: `8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880`.
- Feature-disabled staged config SHA256: `9453FF3D13DCD97287112D13AB5A3E8ABDC64AAAC72C910AD238AA339D6D72F1`.
- Group `job-search`: explicit `work`, `hh`, `habr` source checkouts; `projectGroupsEnabled=false`. The existing upstream MCP configuration, credentials, auth, tunnel ID, access root and bindings are unchanged.
- Final operator transaction `operator-trial-v5` **was frozen after green CI**, with previous Go executable/profile, original G10 task XML, nine operator scripts and a hashed manifest. All nine frozen file checksums passed, as did the previous Go/config backup checks. All nine normalize to the corresponding source blobs already in `main`; three working-tree `.ps1` files have harmless LF/CRLF byte differences across Git checkout, so their frozen checksum (not the working-tree checksum) remains the authority at execution. ACLs inherit only from the protected stage; private JSON config and Task XML are **never pushed to Git**.
- Obsolete `operator-trial-v2`, `operator-trial-v3` and `operator-trial-v4` generated during validation were retired; each executor was renamed to `.SUPERSEDED_DO_NOT_EXECUTE` so its expected operator hash check fails. Their snapshots remain only as forensic evidence.

## New operator workflow

1. `Prepare-GoOnlyTrialUpgrade.ps1` defaults to **read-only `PLAN_ONLY`**. It validates the exact existing Go SCM executable/config, frozen old/new SHA256, Go-only `Watch-GoTrial` process/Task Scheduler BootTrigger, disabled Rust tasks, the `job-search` feature-off configuration and read-only G10 readiness. Beyond the explicitly added group fields, the **entire** staged production configuration must be identical to the current one (including supervisor, auth, tunnel and upstream sources). `-Prepare` writes a protected snapshot of immutable recovery assets and does **not** change SCM, tasks or tunnel. Refuses to overwrite an existing transaction.
2. `Invoke-GoOnlyTrialUpgrade.ps1` defaults to **read-only `PLAN_ONLY`**. It verifies the manifest and all frozen operator file hashes and calls the live preparer again. No service operation is attempted while G10 reports `HOLD`.
3. `New-GoOnlyTrialOperatorTask.ps1` defaults to **read-only PLAN_ONLY**. Only `-Register -OperatorApproved` when G10 is ready can create a *separate*, `Highest`, **on-demand-only** task named `CodexifyGo-PGCanary-cd6b5bd59dea`. Registration itself does not run the task, install a tunnel or touch SCM.
4. **Future independent execution is a separate operator action.** The on-demand task must be started intentionally after an explicit go/no-go decision, never as a child of the Go service that is being replaced. The executor checks the registered task is Running, commit-pinned, elevated, has no automatic trigger and is not descended from Go/Rust. It then repeats the full current-G10 acceptance and identity checks. The immutable old supervisor PID, **unique** Go-only watcher PID, SCM ImagePath, and the **complete exported G10 task definition** must still match the prepared record; otherwise reprepare, **do not guess or retry**.
5. On a guarded **Execute**: disable and stop the verified old **Go-only** observer, change the SCM ImagePath using `Win32_Service.Change`, restart **only** the Go SCM service, verify one supervisor + one worker + one tunnel at the same localhost `/health`, set a new `Watch-GoTrial -ArmGoRecovery` on the existing G10 BootTrigger, confirm a fresh controlled `CONTINUE`. Current job-search member repositories and bindings are not selected or modified by the binary upgrade. Feature flag stays OFF.
6. If a local health/schema/owner/task gate fails after mutation, restore **previous Go** and its SCM image/config, restore the original G10 task action and observer, and record the exact failure phase. Unexpected foreign owners/SCM changes fail closed and require operator review. Recovery policy explicitly distinguishes untouched SCM, candidate/previous Go ownership, and foreign ownership. There is no automatic Rust fallback. Network/VPN/control-plane HTTP 403, 429 or 503 alone is never a binary rollback trigger.
7. A successful cutover **resets** the new candidate's 24-hour G10 observation period. Afterwards separately verify actual ChatGPT connector schema, hosted Setup v4, conversation persistence, trusted `resources/read` metadata and live `hh_local`/`habr_career` read-only calls without MCP tool bursts. Enabling `projectGroupsEnabled=true` is an additional explicitly controlled configuration operation, not done by this binary upgrade.

### Operator commands (no live execution performed)

Read-only checks, safe while G10 is HOLD:

```powershell
$stage = Join-Path $env:USERPROFILE '.codexify-go\staging\project-groups-v1-cd6b5bd59dea'
$run = Join-Path $env:USERPROFILE '.codexify-go\cutover-v2\20261015T101000Z-go-only-upgrade'
$tx = Join-Path $stage 'operator-trial-v5'

./tools/windows/cutover-v2/Prepare-GoOnlyTrialUpgrade.ps1 -StagingDirectory $stage -CurrentG10RunDirectory $run -TransactionDirectory $tx
& (Join-Path $tx 'operator\Invoke-GoOnlyTrialUpgrade.ps1') -TransactionDirectory $tx
& (Join-Path $tx 'operator\New-GoOnlyTrialOperatorTask.ps1') -TransactionDirectory $tx
```

Only after a separate operator approval and a fresh `LOCAL_READY_FOR_OPERATOR_REVIEW`
result, `New-GoOnlyTrialOperatorTask.ps1 -Register -OperatorApproved`
can register `CodexifyGo-PGCanary-cd6b5bd59dea` as a *manual-on-demand,
Highest* Scheduled Task. Registration does **not** execute it. A separate
intentional `Start-ScheduledTask -TaskName 'CodexifyGo-PGCanary-cd6b5bd59dea'`
would execute the protected transaction independently of Codexify's Go
worker. **None of these mutating commands was run in this preparation.**
They must not be used from a stale snapshot, while G10 reports HOLD,
or as a workaround for a 403/429/503 network interruption.

## Local verification (not a live cutover)

- The new PowerShell AST parsed without errors, and `Test-GoOnlyTrialTransaction.ps1` passed its pure Go-only safety/recovery scenarios (healthy state, missing watcher, extra Go, Rust present, changed SHA, unsafe profile, G10 HOLD, disabled flag, tunnel ownership, foreign SCM owner fail-closed).
- Live `Prepare-GoOnlyTrialUpgrade.ps1` PLAN_ONLY returned `HOLD` only for `g10_observation_not_accepted`; correctly saw one Go-only watcher and zero Rust processes.
- Preliminary `-Prepare` runs produced private snapshots v2–v4 and were retired after safety fixes. The final `operator-trial-v5` was created from the exact 11/11-CI-tested scripts after CI completion; 9/9 operator hashes, previous-Go/config snapshots and the stored G10 XML passed. This final transaction remains **not armed**.
- Frozen `Invoke-GoOnlyTrialUpgrade.ps1` PLAN_ONLY returned `HOLD`, no service changes. Frozen task-builder PLAN_ONLY said `taskCanBeInstalled=false`, `executionArmed=false`, zero automatic triggers.
- Production: `CodexifyGo` Running, 2 Go, 1 managed tunnel, 0 Rust, Rust Scheduled Tasks disabled. Live production binary/config hashes unchanged.
- **No `-Execute`, no `-Register`, no `Start-ScheduledTask`, no SCM update or service restart was performed.** Offline policy and syntax testing do not prove an elevated production transaction. A real approval and canary remain required.

## When the G10 guard may be reevaluated

G10 requires an uninterrupted, fresh 24-hour evidence window (max 60 s sample gaps) with no unexpected HOLD. Earlier observations had a Windows reboot and two `tunnel_child_recovery` HOLD samples. The earliest nominal post-reboot continuous window could finish **2026-10-17 around 05:50 Moscow time**, provided the watcher continues recording and no new gap/HOLD occurs. This is not a scheduled or guaranteed approval time. Inspect using:

```powershell
$run = Join-Path $env:USERPROFILE '.codexify-go\cutover-v2\20261015T101000Z-go-only-upgrade'
./tools/windows/cutover-v2/Assess-ProjectGroupsCanaryReadiness.ps1 -RunDirectory $run
```

After G10 is actually accepted, prepare a **fresh** transaction if the original Go supervisor or watcher PID changed. Re-run the frozen operator PLAN_ONLY before any independent registration. Never reuse a stale approved preflight.

**Operational status:** Project Groups implementation merged in `main` (#29); release `0.9.0` remains on HOLD; no job applications were sent and Vacancy Hourly Watch remains unchanged.
