# Windows G02 live trial - 2026-10-02

**G02 PASS. Current deployment state: GO_TRIAL.** The separately authorized
Rust -> Go -> Rust -> Go rehearsal completed with the same frozen candidate.
Go is the current MCP owner. GO_ACCEPTED and CUTOVER_CLOSED have not been reached.

## Identity and current ownership

- Candidate: 20261001T160710Z-node-repl-compat.
- Binary SHA256: 61E2A086445A1B10E7537E720119B0855E17515A58A5B3F8BDCA914075B07DD6.
- Tunnel v0.0.15 SHA256: A922D372D6BE0649156FBC1C8A040597F1F4BB5B4356890151D7875602593B1A.
- Capture profile SHA256: BCDF7B1362047E957F21B55704D69EC3783BD189B9D79759980499613D9A197F.
- CodexifyGo: Running, Manual startup. Supervisor PID 19372, SYSTEM/session 0.
- Interactive worker PID 18692, FOXOS\FoxOS_User/session 1; parent 19372.
- One owned tunnel PID 22028; parent 19372. No Rust runtime remains.
- Rust Codexify and Codexify Watchdog tasks are Disabled. The nine legacy Go
  maintenance/test tasks remain Disabled.
- Exact workspace from both real hosted Go reads: C:\Users\FoxOS_User\codexify-go.

These are timestamped observations, not permanent process identifiers. Reverify
PID, executable, command line and creation time before any later process action.

## Executed sequence and evidence

All timestamps below are UTC. The user's separate "делай" confirmation authorized
this rehearsal after preparation. Local elevated PowerShell performed ownership
changes and rollback independently of the connector.

| Phase | Result |
| --- | --- |
| First entry | Zero owners verified at 2026-10-01 23:57:16.361; a fresh armed v3 watcher wrote observations before Go start at 23:57:19.414. Exact supervisor 20228, worker 19228 and tunnel 21632 reached HTTP 200. |
| First hosted Go read | get_environment succeeded at 23:58:26.385; dispatch-to-result 1,112 ms; intended workspace returned. |
| Real rollback | Started locally at 23:59:19.269. The independent sampler observed Rust HTTP 200, one tunnel and Go Stopped/Disabled after 5.571 seconds. The rollback script returned RUST_RECOVERED after 7.921 seconds. Target: <=120 seconds. |
| Hosted Rust read | First attempted verification succeeded at 2026-10-02 00:00:15.062; call duration 968 ms; intended workspace preserved. This does not measure the earliest possible hosted reconnection time. |
| Re-entry | After disabling/stopping the restored Rust tasks/tree again, zero owners verified at 00:03:21.853. A new armed watcher was verified before Go start at 00:03:24.905. Same binary/profile/tunnel, new exact Go tree, HTTP 200. |
| Second hosted Go read | Succeeded at 00:04:27.239; call duration 591 ms; intended workspace returned. |

The fresh recovery snapshot contains 51 hash-validated files and two Rust task
definitions. No candidate rebuild, profile change, native restart test, reboot,
source commit or push was performed.

## Observation and limits

At the 00:06:25 verification, the independent local sampler contained 330
successful samples covering 23:54:54.528-00:06:24.258. No sample contained both
Rust and Go owners. Maximum sample spacing was 4.121 seconds, including local
health request time during the planned transitions. The zero-owner barriers
and exact process checks supplement this sampled evidence.

Both watcher files contain zero ABORT and zero HOLD decisions; logs show no
rate-limit event. Decoded Go traces contain exactly the two intended
get_environment calls and one unsupported update_plan attempt. No unexplained
tools/call was observed in this interval. This short run does not satisfy idle,
amplification, resource, recovery-fault or soak acceptance gates.

Two startup OAuth discovery warnings were retained. They report invalid
protected-resource metadata; both hosted reads nevertheless succeeded.
Cause and impact on other workflows remain for later verification.

## Compatibility item carried to G03

The existing Rust tool inventory includes update_plan. The frozen Go runtime
returned "unknown tool update_plan" when it was used to record progress.
No update was performed by that call. The progress ledger was completed locally;
no substitute success was claimed.

G03 must refresh/inspect the actual Go catalog and schemas, determine the
required plan workflow, and verify two real conversations. A fresh catalog
alone cannot prove that the former workflow is supported. The get_environment
result shape also differs between Rust and Go. Full compatibility is not claimed.

## Current local recovery and observation

Private run root:
C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat

Current trial evidence:
evidence\g02-live-20261001T235259Z

Current recovery snapshot:
rollback-private\rust-snapshot-g02-live-20261001T235259Z

The trial's handover.json records exact rollback paths, active observer
identities, bounded coverage and the current LOCAL_MANUAL_RECOVERY.md.
The rollback uses frozen operator-tooling-v3. Older preparation snapshots and
evidence remain intact.

At handover, the armed re-entry watcher and independent local sampler remain
running. The sampler is bounded to 120 minutes from its start; it is not a
24-hour collector. Detailed MCP capture is bounded to 10 minutes / 20,000 events
per worker start. Capture expiry does not establish continued decoded coverage;
the watcher can still examine ownership and coarse tunnel logs.

Automatic Go startup and the normal diagnostics profile are later acceptance
steps. Restore only specifically approved paused normal work at the planned
phase; do not bulk-enable the nine obsolete Go installer/test tasks.

## Next gates

G03-G11 remain pending. Prioritize current catalog/schema and two-conversation
verification, including the missing plan workflow and startup OAuth warning.
Then follow the unchanged
[acceptance plan](WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md).

The [readiness criteria](WINDOWS_G02_READINESS_CRITERIA_20261001.md) and
[preparation plan](../superpowers/plans/2026-10-01-windows-g02-preparation.md)
remain the preparation record. The mutable private gate ledger now records the
separate live result. No final migration acceptance is inferred from G02 PASS.

Subsequent execution: [G03 client report](WINDOWS_G03_CLIENT_REPORT_20261002.md)
records the later current-schema, two-conversation and operator widget checks.
