# Windows Project Groups binary-only Go→Go cutover — 2026-10-16

**Result:** `GO_UPGRADED` — controlled Windows SCM Go→Go transition completed.

**Operation:** candidate binary activation only; **Project Groups remains OFF**.

**User authorization:** explicit 2026-10-16 approval for the exact candidate SHA256, previous-Go recovery only and no Rust fallback.
**Rollout classification:** production binary switch **PASS**; new candidate G10 observation **RUNNING, NOT YET ACCEPTED**; hosted Project Groups Setup v4 / PG-A gates **NOT YET RUN**. Stable `v0.9.0` release is still **HOLD**.

## Frozen identities and preserved recovery

| Identity | SHA256 |
|---|---|
| New Windows Go executable, compiled from `cd6b5bd59dea0be35e3107446dfddce322f41ad2` | `F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2` |
| Previous verified Windows Go executable | `8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880` |
| New feature-disabled production config | `9453FF3D13DCD97287112D13AB5A3E8ABDC64AAAC72C910AD238AA339D6D72F1` |
| Frozen previous production config | `47DF2D446BAF84D9DC9CE0243AFD79A22B803525884BFB94E6D3D79B6B03830D` |
| Previous-Go operator G10 risk-acceptance marker | `0BA09B150E44FD57C1C41005ECE2290775A1AA7EC559311B584ABD7C8D74E42B` |
| Time-bound, candidate-specific authorization record used for this operation | `2F474C940A7830BC48A43378881BE57B925767E39922C3C6B5B7DE1891EB5213` |

The staged candidate is still correct relative to current `main` source: between candidate commit `cd6b5bd` and the pre-cutover `main` `69648bc`, the only changed Go file was `internal/supervisor/command_windows_test.go`. No production Go source files changed.

The exact independent operator and Go-only recovery scripts were frozen in a private snapshot **`operator-trial-v6`** after native PR CI. All ten frozen operator files were SHA256-verified. The former verified Go executable/config are preserved in the snapshot, along with the original G10 scheduled-task XML. No previous Go or Rust recovery asset was deleted or overwritten. Private stage and local evidence are located under:

```text
%USERPROFILE%\.codexify-go\staging\project-groups-v1-cd6b5bd59dea\
  candidate\codexify-go.exe
  config\project-groups-disabled.json
  operator-trial-v6\
    manifest.json
    upgrade-result.json
    previous-go\codexify-go.exe
    previous-go\production.json
    g10-observers-before.xml
    operator\
    evidence\operator-events.jsonl
    evidence\probe-observations.jsonl
```

This is an evidence inventory, not a public archive. Do not commit `*.json` config/approval, task XML, tunnel tokens or raw G10 runtime data to Git.

## Go-only transaction timeline (UTC)

| UTC time, 2026-10-16 | Event | Result |
|---|---|---|
| 07:59:52.185 | `go_only_trial_independent_task_started`; accepted previous-Go G10 deviation + explicit candidate-specific approval | PASS |
| 07:59:52.488 | `old_go_only_watcher_retired` | PASS |
| 07:59:52.622 | `new_go_image_selected` through Win32 SCM | PASS |
| 07:59:54.187 | `candidate_go_only_owner_healthy` — one Go supervisor + worker + tunnel | PASS |
| 07:59:55.563 | `go_only_watcher_started`, armed for **Go-only** recovery | INFO |
| 07:59:56.220 | `go_only_trial_watcher_ready`, fresh controlled `CONTINUE` | PASS |
| 07:59:56.224 | `go_only_trial_upgrade_completed` | PASS |

Independent Task Scheduler job: `CodexifyGo-PGCanary-cd6b5bd59dea`, Interactive/Highest, **zero automatic triggers**, last task result `0`, task state `Ready` after completion. The one-shot transaction lock remains; repeated execution is not an accepted path. No Rust-capable watcher or rollback was started, and previous-Go recovery was **not required**.

Durable `operator-trial-v6/upgrade-result.json` contains `status=GO_UPGRADED`, exact candidate SHA256, `featureFlagEnabled=false`, `autoRustFallback=false`, and `g10Acceptance=RESET_AND_RUNNING_NOT_ACCEPTED`.

## Post-switch live verification

- Windows SCM `CodexifyGo`: **Running**, Automatic; new Go supervisor PID **19324**.
- The actual SCM executable SHA256 matches `F5AF36CE...`; exactly one supervisor and one worker from that executable, plus **one tunnel child owned by the supervisor**; **zero Rust** processes.
- `CodexifyGo-G10Observers`: Running, retains its boot trigger, and is now pinned to the current `operator-trial-v6` Go-only `Watch-GoTrial.ps1`. There is **one** owned watcher process.
- `Codexify` and `Codexify Watchdog` Rust scheduled tasks remain Disabled.
- New G10 local inspection returns `LOCAL_OBSERVED`, fresh controlled `CONTINUE/within_limits`, expected two Go owners, one tunnel, zero Rust, no inspector warnings. This is an active **new observation period**, not 24-hour acceptance.
- The new `Watch-GoTrial` was started with a **27-hour run duration** by the transactional operator. Before that observation window expires, choose an explicit G11/steady-state Go-only monitoring arrangement; do not assume this 27-hour observer runs indefinitely.
- Local loopback `/health`: **HTTP 200**.
- The same ChatGPT conversation retained `bindingScope=chatgpt_conversation`, `workspaceRoot=C:\Users\FoxOS_User\codexify-go` and `managedWorktree=false`; `git status` returned clean `main`.
- Real `list_projects`: 22 currently discovered repositories, including `work-search`, `hh-mcp-server`, and `habr-career-mcp`. No workspace selection changed.
- Upstream `hh_local.get_my_resumes` and `habr_career.habr_whoami`: read-only calls **responded without MCP errors**. No vacancy application sent; no Vacancy Hourly Watch activation.
- Real ChatGPT Setup handshake after using the returned `connectorVersion` reported `status=current`, `conversationStale=false`, `refreshRecommended=false`, selected source checkout preserved. **Advertised Setup schema remains `setup-v3` because Project Groups is disabled**; this does not establish real hosted Setup v4 acceptance.
- New production config preserves former upstream definitions, auth, tunnel endpoint and `projectScanDepth=2`, adding only explicit project catalog/group definitions with **`mcp.projectGroupsEnabled=false`**. The exact former config hash remains preserved for recovery.

## Hard boundaries and next step

1. **Do not re-run the old operator transaction** and do not start a second Go/tunnel owner. The existing one-shot lock and registered triggerless operator task are retained as evidence; do not treat the job as a periodic watchdog.
2. Do not restore or enable Rust. Temporary external VPN/control-plane 403/429/503 alone is not a rollback trigger. For a confirmed local Go supervisor failure, the new `Watch-GoTrial` can invoke **only previous-Go recovery** using frozen assets.
3. The new binary's G10 observation is in progress. The user's previous operator acceptance with deviations applied **only to the old Go** and was explicitly taken into account in authorizing this **single controlled binary switch**. It is not equivalent to technical G10 PASS for this new binary.
4. **Project Groups remains OFF.** A separately authorized configuration change and host-side test are still required to enable the `job-search` group and validate Setup v4, routing, conversation-binding persistence, repository isolation, artifacts and upstream MCP burst safety.
5. G11 and stable release `v0.9.0` remain independently gated.

**Conclusion:** the requested Go→Go binary cutover succeeded, hosted connector returned with unchanged conversation workspace identity, Go-only process ownership is healthy, and the previous verified Go recovery path has been preserved. Project Groups feature enablement was intentionally not attempted.
