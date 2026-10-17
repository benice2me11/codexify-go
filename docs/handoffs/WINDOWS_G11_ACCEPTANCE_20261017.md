# Windows G11 steady-state — acceptance ledger (17 October 2026)

**Overall status: G11_READY_FOR_REVIEW / NOT ACCEPTED. G11-04 30-minute smoke VERIFIED; G11-05 requires explicit operator disposition of residual requirements.**

Related reports:
- WINDOWS_G11_STEADY_STATE_IMPLEMENTATION_20261016.md
- WINDOWS_G11_GATE2_RECOVERY_HANDOVER_20261016.md
- WINDOWS_G11_G04_NORMAL_PROFILE_20261017.md

## Runtime identity

| Property | Verified value |
| --- | --- |
| Service | CodexifyGo, Windows SCM Running / Automatic |
| Current Go executable SHA256 | F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2 |
| Current production config SHA256 | 76749955DC6E8D73B68C44E11474F15055910070AF0B67F739CB39A5E28794BD |
| Current production flags | mcp.diagnostics.enabled=false, mcp.projectGroupsEnabled=false |
| Previous-Go-only recovery SHA256 | 8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880 |
| G11 pinned-policy SHA256 | 03FBB74A20D4BAFD12F40D3353A075D9BC253C7E255A73EC143B6E035A405EB7 |
| SCM failure policy | Restart after 5 / 15 / 60 seconds, counter reset 86400 |
| G10 task | Disabled; 0 Watch-GoTrial processes |
| G11 task | SYSTEM, boot + each minute, IgnoreNew, sole approved Go-only recovery |
| Rust tasks | Codexify Disabled; Codexify Watchdog Disabled |
| Stable release | v0.9.0 HOLD |

## Completed G11 stages

| Gate | Result | Evidence |
| --- | --- | --- |
| G11-00 read-only audit | VERIFIED | Service, SCM, Git, binary/config hashes, G10/G11 and private recovery snapshot |
| G11-01 Go-only monitoring architecture | VERIFIED | SCM owns supervisor recovery; supervisor owns worker/tunnel; G11 only detects sustained supervisor absence |
| G11-02 implementation/fixtures | VERIFIED | Windows PowerShell 5.1 and PowerShell 7 offline tests; Go test/vet/race and cross-build CI |
| G11-03 observe-only and recovery handover | VERIFIED | G10 disabled; normal exit; one recovery-armed G11; multiple post-handover CONTINUE and reboot survival |
| G11-04 normal config switch | VERIFIED (deployment only) | Operator GO_NORMAL_PROFILE_VERIFIED, Task Scheduler result 0, exact config, same Go SHA, health 200 |
| G11-04 30-minute smoke | TECHNICAL PASS | Independent SYSTEM smoke completed 31/31 healthy samples over 1820.790 seconds, no gap over 61.552 seconds, task result 0, same Go/worker/tunnel PIDs |
| G11-05 operator acceptance | PENDING | This report is not an acceptance decision |

Source development branch: feat/windows-g11-steady-state-20261016, PR #37. Offline 30-minute smoke fixture commit `8b6686a`, [CI #39016064879](https://github.com/benice2me11/codexify-go/actions/runs/39016064879) SUCCESS. Do not merge or publish release merely because CI is green.

## Independent transaction and recovery

- First guarded normal-profile one-shot failed **before SCM change** on a reserved PowerShell PID variable; this was documented and corrected with an offline regression test. The failed task is Disabled and its private evidence retained.
- A new, distinct one-shot transaction completed at 2026-10-17 07:53:39 UTC with GO_NORMAL_PROFILE_VERIFIED.
- Supervisor PID 14400, worker PID 13484, managed tunnel PID 11304 at post-cutover baseline. The same current Go executable SHA is retained.
- There is no Rust fallback and no second recovery-armed task. The original diagnostics-on config and protected previous-Go backup remain available. A normal-profile failure would have attempted exactly one rollback to the original config of the same Go, with G11 as the longer-term previous-Go-only recovery authority.
- Evidence lives in protected ProgramData, especially C:\ProgramData\CodexifyGo\G11\normal-switch-retry-20261017T075005Z; do not commit raw configs/tokens or protected logs.

## Hosted compatibility (read-only)

- Setup v3 is advertised and the same `chatgpt_conversation` source checkout binding survived the config-only restart. The connector still reports `conversationStale=true` against the previous conversation schema (`refreshRecommended=false`), so a post-refresh handshake is **not** verified.
- 22 projects and prior worktrees preserved.
- MCP source listing included hh_local and habr_career; habr_whoami succeeded. The hh_local.get_my_resumes call was blocked by an OpenAI safety check, so the HH end-to-end read is **unverified**; no applications were submitted.
- A read-only automation status check on 17 October confirmed **Vacancy Hourly Watch remains disabled**. It was not enabled or edited by this work.
- No Project Groups flag change. Setup v4 and PG-A00..PG-A08 remain a separate future rollout.

## Final 30-minute smoke — verified

A PASS must include at least 31 timed samples across >=30 minutes, no unexpected sample gaps (> 140s), exact same Go supervisor/worker/tunnel ownership, HTTP 200, healthy G11 monitor, no Rust owner/task, no original G10 observer, exact binary/config hashes, no local recovery and no unexpected restart. Read-only evidence should show Task Scheduler completion result 0 and smoke-30min-result.json status G11_30_MIN_SMOKE_PASS.

**Result: G11_G04_TECHNICAL_PASS, 16/16 independent audit checks.** The protected one-shot task completed with Task Scheduler result 0 and wrote `G11_30_MIN_SMOKE_PASS`. Exact evidence:

| Metric | Observed |
| --- | --- |
| Start | 2026-10-17 07:58:50.275426 UTC |
| Last sample | 2026-10-17 08:29:11.065261 UTC |
| Duration | 1820.790 seconds (30m 20.790s) |
| Samples | 31/31 passed, ordered 0..30 |
| Maximum sample gap | 61.552 seconds; limit 140 seconds |
| Owners stable throughout | Go supervisor 14400, Go worker 13484, managed tunnel 11304 |
| Expected image/config, local health, G11 status | PASS in every sample |
| Rust processes/tasks and G10 observer | 0 / Disabled |
| Previous-Go recovery | Preserved, no invocation |
| G11 passive smoke samples SHA256 | `61B537878D39864C037DE142FE6F956861A006E7AFE9E6981F92F5EBD1D90931` |
| G11 passive smoke result SHA256 | `D0AD792964CD6B279AA2B35698EBD3F62C8EDDB8094B6EA475AA76EA9A42A4A7` |
| Final protected technical audit | `G11-G04-final-audit.json`, SHA256 `8BEF4C7797D680F73B4F93C6C3289E15160CEA6F8FE7540715567A0EE7373986` |

Private source path: `C:\ProgramData\CodexifyGo\G11\normal-switch-retry-20261017T075005Z\`. Evidence and raw production configs remain outside Git.

After PASS, the failed original switch task, successful retry one-shot task and completed 30-minute smoke task were all verified **Disabled**; registered task definitions and protected before-disable XML are retained. Permanent `CodexifyGo-G11SteadyState` remains active, recovery-armed and healthy. No additional SCM, Go, or tunnel restart was used for cleanup.

## Residual risks / explicit decision required

1. The current Go binary did not finish a new strict 24-hour G10 window before the previously operator-approved handover. Earlier acceptance for another binary does not transfer automatically.
2. A reboot/logout validation specifically **after diagnostics-off** is pending a separate maintenance-window authorization. The 17 October morning reboot took place with diagnostics still enabled and only verifies G11 boot-monitor continuity under that older profile.
3. The blocked HH safe read is an external/tool-permission coverage limitation, not proof of malfunction.
   The existing conversation's stale connector schema is a separate compatibility follow-up, not evidence of a failed Go server.
4. G11 local Windows events and protected logs do not implement external proactive push alerts; operators must still review freshness.
5. No live Project Groups acceptance; no stable v0.9.0 release.
6. The passive 30-minute smoke establishes local service, process, task and HTTP health, but is **not** full per-request MCP burst instrumentation. With the temporary MCP diagnostic tracing disabled, an absence of duplicates/burst cannot be proven from the smoke samples alone. Do not convert this into an unqualified per-request PASS.

**Operator decision: PENDING.** Permissible later outcomes: G11_ACCEPTED only after every required criterion, G11_ACCEPTED_WITH_EXCEPTIONS only with explicit approval specifying waived criteria, or G11_HOLD otherwise.
