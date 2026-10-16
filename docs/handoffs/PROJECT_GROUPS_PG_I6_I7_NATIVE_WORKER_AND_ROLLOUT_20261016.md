# Project Groups PG-I6 — native compiled-worker smoke and PG-I7 operator readiness

Date: 2026-10-16

## Candidate and boundaries

- Development parent: `fix/project-groups-v4-cross-platform-20261016` at `c86a8672dae89ab98c0713e93442041ce9320131`.
- Scope: compiled worker smoke and native CI coverage, read-only Go-only operator preflight, production rollout plan.
- Feature flag default remains `mcp.projectGroupsEnabled=false`.
- This candidate is **not deployed** to Windows SCM or the user tunnel.
- The preceding cross-platform checkout fix passed all GitHub CI jobs: run [38848479526](https://github.com/benice2me11/codexify-go/actions/runs/38848479526).

## PG-I6 compiled-worker acceptance

New fixture: `tools/forensics/project-groups-smoke/`. It builds no fake success response: it spawns the **actual compiled candidate** using `worker run --config` on a random 127.0.0.1 port, connects through the official MCP SDK, and repeats the full restart sequence with persisted file-backed state.

Observed on local Windows 11 / Go 1.26.5:

| Evidence | Status |
|---|---|
| Compiled binary, official MCP transport, real `/health` and Setup v4 resource | PASS |
| Three disposable Git repos, one v3 conversation, hh managed and work/habr source | PASS |
| Dirty hh managed file remains private to its checkout | PASS |
| Scoped group artifact: own conversation allowed; foreign conversation rejected | PASS |
| Active alias, AGENTS, primary `CHAT.md`, Git diff cursor | PASS |
| Real worker process restart #1: preserve binding, managed root, dirty data, last-diff, artifact | PASS |
| Real worker process restart #2: preserve group, active alias, dirty data and primary chat | PASS |
| Source–Managed roundtrip preserves dirty managed worktree, revokes old artifact | PASS |
| Feature disabled: group tools hidden, v3 binding fails closed | PASS |
| Reenable: same v3 state, active alias and managed worktree restored | PASS |
| Optional real previous-Go worker in offline fixture: legacy source read succeeds, saved v3 binding stays byte-for-byte identical, then new candidate resumes | PASS (Windows) |
| Staged worker binary never invokes tunnel or installs service/SCM | PASS |
| Native Windows/Linux/macOS CI compiled-worker smoke | PASS — GitHub Actions [38850369282](https://github.com/benice2me11/codexify-go/actions/runs/38850369282) |

The smoke launches five independent worker instances (four worker restarts) and does not need a tunnel, control-plane API, external source, or production directory. The fixture initializes Git commits in temporary directories only. Its result is **worker process lifecycle**, not native service-manager acceptance. For the optional previous-Go stage, the old binary runs as a standalone fixture worker, never as the Windows SCM service.

## Live Windows preflight (read-only)

Observed without changing production:

- Windows SCM service `CodexifyGo`: Running, Automatic.
- Go executable SHA256: `8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880`.
- Process counts: 2 `codexify-go`, 1 `tunnel-client-runtime`, 0 Rust `codexify.exe`.
- `Codexify` and `Codexify Watchdog` scheduled tasks: Disabled.
- `CodexifyGo-G10Observers` task: Running. Running task status **does not establish** G10 guard PASS or 24-hour soak.
- Offline safety fixtures: `Test-GoOnlyUpgrade.ps1` and `Test-G10RuntimeInspection.ps1` executed successfully; policy output `GO_ONLY_UPGRADE_POLICY=PASS`.
- User Git repositories and personal bindings were **not** modified or copied for this work.

### PG-I7 local readiness assessment — HOLD (2026-10-16)

Implemented `tools/windows/cutover-v2/Assess-ProjectGroupsCanaryReadiness.ps1` as a
**read-only** preflight over the existing G10 JSONL evidence and local
`Inspect-G10Runtime.ps1`. It requires a complete observation window, fresh
last guard, no unexpected gaps, no non-CONTINUE decisions, current Go-only
topology, and disabled Rust tasks. It never reports production rollout PASS:
the best possible local result is `LOCAL_READY_FOR_OPERATOR_REVIEW` after
the other conditions are satisfied.

Read-only command on the existing Windows run directory:

```powershell
$run = Join-Path $env:USERPROFILE '.codexify-go\cutover-v2\20261015T101000Z-go-only-upgrade'
./tools/windows/cutover-v2/Assess-ProjectGroupsCanaryReadiness.ps1 -RunDirectory $run
```

Observed at **2026-10-16T04:24:55Z** (sanitized counts only):

- **Decision: HOLD**, despite `LOCAL_OBSERVED`, one-second-fresh `CONTINUE`,
  2 Go processes, one owned tunnel, zero Rust processes and both Rust
  Scheduled Tasks disabled.
- G10 evidence covers only **17.972 hours**, not the mandatory 24-hour
  window. The largest observation gap is **31,499 seconds** (~8 h 45 min),
  with five gaps over 60 seconds.
- Two `HOLD` observations have safe reason code
  `tunnel_child_recovery`. They are not evidence of a Rust rollback.
- Windows last boot was **2026-10-16 05:48 local time**, accounting for the
  overnight gap. A rebooted machine does not supply uninterrupted soak.
- Hold reasons: `soak_window_short`, `g10_observation_gap`, and
  `non_continue_guard_action`.

The accompanying `Test-ProjectGroupsCanaryReadiness.ps1` passed offline
synthetic clean-window, short-window, gap, HOLD, and missing-file scenarios.
It runs in Windows CI. The fixture cannot grant production authorization.

The Linux and Mac machine connectors were unavailable (tunnel not seen for over 300 seconds) during this inspection. Native CI runs are a substitute for native worker smoke, **not** real launchd/systemd runtime or production user-machine restart acceptance.

## PG-I7 — exact go-only operator rollout procedure (not executed)

Do not mix this staged feature into the current `v0.9.0` release-candidate/stable HOLD. Merge dependency chain deliberately after independent review: PR #20 -> #22 -> #24 -> #25 -> #26 -> #27 -> PG-I6 PR. Keep `projectGroupsEnabled=false` in the production profile until a separate go-only canary is authorized.

1. Record full candidate commit and binary SHA256. Snapshot the currently running Go executable and redacted config/profile hashes, binding directory inventory, task XML and original SCM ImagePath in a protected private operator directory outside the repo. Never commit tokens, `.env`, user paths with personal content, or complete private config.
2. Recheck G10 watcher health, Rust disabled status, unique Go owner, managed tunnel and a working current connector. If any is ambiguous, hold.
3. Prepare an **independent** Go-to-Go task using the existing tested transactional upgrade scripts. Execute plan-only/preflight first and confirm that both the new candidate and the previous Go recovery binary are healthy and distinct. No in-worker self-restart, Rust fallback or second tunnel owner.
4. Stage a private config with the validated three-source `job-search` membership and per-member explicit `source`/managed selection, respecting Windows access-root path canonicalization. Verify actual projects exist and are permitted. Do not put unrelated home directories in a group member root.
5. Perform operator-controlled cutover only after the independent task has valid recoverability. On a failed local health/schema/ownership gate, revert to the **previous verified Go** binary/config and leave saved v3 bindings untouched. Transient upstream VPN/HTTP 403/429/503 alone is not a code-cutover rollback trigger.
6. With a *fresh connector schema* and one real ChatGPT conversation, select the configured group, read-only inspect roots/AGENTS and Git status for all three, then exercise explicit/implicit alias overrides and Source/Managed mode display. Require a successful trusted `resources/read` identity-bound artifact read. If the ChatGPT host fails to send trusted resource identity, fail closed and block rollout rather than loosening ownership.
7. Verify upstream `hh_local`/`habr_career` read-only discovery and harmless account/status endpoints without new connection bursts or rate-limit loops. Do **not** submit applications or enable Vacancy Hourly Watch.
8. Record baseline hashes, then have the independent operator restart the **Go worker/service twice**. After each restart/reconnect, verify the same v3 group, active alias, checkout revisions/paths, dirty fixture data, diff checkpoint and private artifact permissions. Stop if the control plane loses ownership or two Go service owners appear.
9. Rehearse feature-flag disable/re-enable and previous-Go rollback on a bounded canary. Confirm legacy single-project mode works, v3 data remains recoverable, active Go service is healthy and Rust tasks stayed disabled.
10. Capture redacted timeline, build hash, schema marker, CI evidence, G10 samples and gate results; only mark PG-A00..PG-A08 PASS when each exact acceptance scenario has executed.

## Gate ledger after isolated PG-I6

| Gate | Status | Outstanding item |
|---|---|---|
| PG-A00 | PARTIAL | No production config/binary rollback drill |
| PG-A01 | PARTIAL | Canonical registry and hostile-path test coverage; live catalog verification pending |
| PG-A02 | PARTIAL | **Real compiled worker process** restart tested multiple times; Windows SCM/ChatGPT-bound restarts not tested |
| PG-A03 | PARTIAL | Multi-repo MCP fixture PASS; real hosted conversation smoke pending |
| PG-A04 | PARTIAL | Per-repo Git/diff/context/artifacts fixture PASS; live host authorization pending |
| PG-A05 | PARTIAL | Negative path/session/resource cases PASS; adversarial OS-handle race protections not promised |
| PG-A06 | PARTIAL | Setup v4 harness and mock upstream PASS; hosted UI/live upstream/429 check pending |
| PG-A07 | PARTIAL | Full Go test/vet, race and native compiled-worker CI planned; macOS launchd/Linux systemd user lifecycle not run |
| PG-A08 | PARTIAL / LIVE NOT RUN | Previous-Go binary compatibility and v3 preservation passed offline; operator-approved production Go-only canary/restart/rollback and G10 evidence needed |

Do not report production Project Groups as enabled or claim a stable release until every mandatory gate has independent evidence.
