# Windows G10 Go-to-Go transactional upgrade protocol — 2026-10-15

**Status:** **GO_UPGRADED / G10_RUNNING_NOT_ACCEPTED**. Independent live SCM switch completed 2026-10-15 at 13:26 Moscow time.
**Current owner:** Setup v3 Go Windows SCM service; previous Go is preserved for Go-only recovery; Rust service and watchdog remain disabled.
**Original frozen live binary:** SHA-256 B908C8DE5BB5526BCFEFC69AAF631A9189963A8BBB1EC08AFD380495BF10F387.
**New Setup v3 candidate:** commit cdf8c98, SHA-256 8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880.

PRs #7–#10 brought Windows path identity, explicit checkout UI, G03 tunnel self-heal and verified Setup context into main. PRs #11–#12 added offline diagnostics and compiled-binary smoke. The later commits add diagnostics/tests, not binary-affecting source.

## Live result — 2026-10-15

- Operator source: PR #14, merged into main at `1536b7737f69cebe3bd2013a20f0ada784512ffb`. Windows CI (Go test/vet, PowerShell fixtures, native build, compiled Setup v3 smoke, race and cross-build) PASS.
- Independent on-demand Task Scheduler job: `CodexifyGo-GoUpgrade-20261015`, Interactive/Highest, first PLAN_ONLY result 0, then separately armed and started with Execute. This task was not owned by the Go worker.
- Operator start: 2026-10-15T10:26:30Z. Sequence in operator-events.jsonl: independent_task_started, old_rust_capable_watcher_disabled, scm_image_switched, new_go_supervisor_worker_tunnel_ready, go_only_watcher_ready, old_observers_retired, go_upgrade_transaction_completed — **all PASS**.
- Durable transaction result: **GO_UPGRADED** in `upgrade-result.json`. Previous-Go recovery was not required. Old Go executable/config and old G10 task XML backup remain available in their previous paths and the transaction folder.
- Live Go supervisor PID initially **18952**, started at 13:26:30 Moscow; one Go worker child and one tunnel child, zero Rust processes. Running service executable SHA-256 equals the staged Setup v3 candidate exactly. Original profile SHA-256 remains `47DF2D446BAF84D9DC9CE0243AFD79A22B803525884BFB94E6D3D79B6B03830D`.
- Old Rust-capable rollback-watcher count **0**. `CodexifyGo-G10Observers` now runs the new `Watch-GoTrial.ps1` with its original BootTrigger and **without ArmRollback/Watch-Cutover**. Go-only watcher count **1**; both Rust Scheduled Tasks disabled.
- New G10 evidence at private `C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261015T101000Z-go-only-upgrade\evidence`. At 13:30:31 Moscow: **24/24 guard samples CONTINUE/within_limits**, 24 local samples, 4 metric samples, guard age 2 seconds, CPU sample ~4%, 2 Go processes, 1 tunnel, 0 Rust.
- After reconnection, this same ChatGPT conversation retained `workspaceRoot=C:\Users\FoxOS_User\codexify-go`, `managedWorktree=false`. A fresh setup call reported schema **0.8.2-dev+workspace-v1+setup-v3+artifact-ingress-v1**, status **current**, `conversationStale=false`.
- Read-only upstream smoke: `mcp_list_sources` returned `hh_local`, `habr_career` and `node_repl`; `hh_local.get_my_resumes` and `habr_career.habr_whoami` returned without upstream MCP errors. No vacancy application or HH mutation was sent.

**Still pending:** user-side visual check of the rendered Setup v3 picker, later boot/reconnect acceptance, full 24-hour G10 coverage and the original workload/quiet-window thresholds, then G11. Do not mark G10 PASS merely from the short successful live smoke; record any later unexplained restart or monitoring gap as a blocker.

## Topology and safety

The old G10 watcher is launched through a boot-time Task Scheduler task and is armed to roll back to Rust. Changing SCM ImagePath while it observes the previous Go binary risks local_control_lost followed by Rust fallback. Upgrading must be a **single independently scheduled transaction**, not a child of the Go user-worker.

New operator scripts:

- Prepare-GoToGoUpgrade.ps1 validates single Go owner, disabled Rust tasks, old/new/config hashes, active watcher count and boot trigger; writes a private manifest, original task XML and operator copies. No live SCM/task mutation.
- GoOnlyUpgrade.psm1 and Test-GoOnlyUpgrade.ps1 provide pure policy and hash verification. External HTTP 403/429/503 and temporary missing tunnel child do not trigger service fallback.
- Invoke-GoToGoUpgrade.ps1 final preflight, disables old boot observer and terminates old rollback-armed watcher **before** changing SCM ImagePath through Win32_Service.Change. Restarts CodexifyGo only, checks local worker health and unique Go supervisor/worker/tunnel, updates the observer boot task to Go-only watcher and validates a fresh CONTINUE; then retires only old G10 observer subprocesses.
- Watch-GoTrial.ps1 watches service and process topology for 27 hours independent of MCP, records 10-second local samples, 60-second metrics, and Go-only guard decisions. A confirmed absent Go supervisor for six consecutive samples can trigger recovery to *previous Go*, never Rust. Rust owners or foreign SCM changes block acceptance and are not silently modified.
- Invoke-GoOnlyRecovery.ps1 restores only the frozen previous Go executable through the SCM API and checks its health. It disables the new observer task while leaving all Rust tasks untouched and disabled.

Rust fallback binaries remain frozen on disk as recovery/forensic assets, but these scripts do not invoke them. Existing Git files, workspace bindings, upstream HH/Habr and vacancy automation remain unchanged.

## Prepared private Windows transaction

Operator root:

C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261015T101000Z-go-only-upgrade

Contents include manifest.json, g10-observers-before.xml, operator-file-hashes.json, operator scripts, evidence/operator-events.jsonl and operator-preflight.json.

An earlier incomplete directory ending 20261015T100000Z-go-only-upgrade was marked INCOMPLETE_DO_NOT_RUN.txt and must not be executed.

The independent on-demand Task Scheduler task:

CodexifyGo-GoUpgrade-20261015

Its first run was PLAN_ONLY, returned 0, and recorded READY_GO_TO_GO. It reuses the existing Interactive/Highest task principal but has **no periodic trigger**. This dry-run did not change the running service.

The SCM Change method was separately tested on a new, stopped disposable Windows service fixture. After the ImagePath change was verified, the fixture service was deleted. Local Go-only policy tests passed.

## Mandatory execution gates

1. Pass CI against exact operator commit: Go tests, race, Windows PowerShell fixtures, cross-platform builds.
2. Recheck live service and binary, profile hashes, old watcher and task, disabled Rust tasks, latest G10 guard.
3. Verify the independent Task Scheduler PLAN_ONLY result and fresh operator-preflight.json output.
4. Amend only that manual upgrade task action to add the Execute flag, without changing principal or installing second tunnel. Start the task; it must continue after ChatGPT disconnects.
5. Recover evidence after reconnect from upgrade-result.json and evidence/operator-events.jsonl. For failures verify go-recovery-result.json and previous Go service; do not trigger Rust fallback.
6. Verify ChatGPT Setup v3 resource and +setup-v3 connector schema after refresh, plus Source/Managed switch, hh_local and habr_career read-only tools, conversation binding persistence, and absence of MCP tool bursts.
7. Restart G10 observation for new candidate and verify ≥24h coverage, required work sessions, action counts, quiet windows, CPU and restart criteria. G11 is separate.

No G10 or G11 PASS is permitted based only on CI, a local readiness probe or a short live smoke.

**Important:** temporary VPN or tunnel HTTP 403/429/503 must not activate Rust restoration. Go owns its transient child recovery; recovery of the previous Go binary is reserved for actual local safety faults.

**Job search constraints:** Vacancy Hourly Watch remains off, no job applications, no hh_local upgrade in this transaction.
