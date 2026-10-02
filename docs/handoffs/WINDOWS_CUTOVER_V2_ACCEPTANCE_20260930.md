# Windows Cutover v2 - Acceptance and Execution Plan

> Historical acceptance criteria. The original checkbox/table state below is
> not the current gate ledger. This Git checkpoint does not execute another gate.

> For agentic workers: use superpowers:executing-plans for an authorized execution. Work gate by gate; do not turn a documentation task into an unannounced service replacement or reboot.

**Goal:** Accept a reproducible Go-only Windows deployment for the user's normal workflows, without requiring retrospective proof of every September 29 burst.

**Architecture:** One frozen candidate, one owner of the existing tunnel identity, staged live tests, and a local rollback path independent of MCP. Retain Rust as an inactive recovery asset. Treat unit tests, live checks and longer observation as different evidence classes.

**Tech stack:** Windows SCM, Rust fallback, Go user worker/supervisor, managed tunnel runtime, MCP, embedded JavaScript widgets, local PowerShell evidence collection.

**Spec:** User-approved direction in this conversation on 2026-09-30: restart the Windows cutover in a controlled way, preserve the existing Go fixes/diagnostics, and add explicit completion criteria. References below describe evidence and limitations, not automatic acceptance of the new deployment.

**Document status:** Acceptance criteria proposed for the next run. No live v2 gate is marked PASS by creating this file. Current deployment status: NOT_READY / Rust active.

## 1. Global constraints

- Scope is Windows. Do not change or re-test Linux/macOS installations as a prerequisite.
- No speculative MCP lifecycle changes, protocol downgrade, or additional unrelated fixes during acceptance.
- Retain and separately review the existing Go widget fix, log-level fix, bounded diagnostics, and pending tunnel v0.0.15 pin. A passing combination does not prove which change fixed the old incident.
- Preserve dirty source changes, bindings, original logs, secrets, and the Rust recovery assets. Do not blindly stage the working tree or delete state for a clean-looking run.
- No simultaneous Rust/Go owners of the same tunnel. Scope process checks by executable path, parentage and installation; unrelated tunnel processes for other projects are not this deployment.
- No load generator against the hosted connector. The 1,024-request harness remains loopback-only. Planned manual live checks are sequential except the explicitly bounded two-conversation test.
- This plan allows a bounded cutover only after G00/G01 pass. Outside that run the earlier Rust-only safety boundary still applies. Historical handoffs remain unchanged and are not current live-state reports.
- Closing client widgets, user sign-in, a second ChatGPT conversation, or reboot need actual completion evidence. Do not fabricate these from a tool call in the current conversation.
- All numeric limits below are project acceptance targets and safety triggers proposed here, not vendor SLAs or measurements already achieved. Freeze them before the run; do not relax them after a failure to obtain PASS.

### Execution discipline approved on 2026-10-02

Reduce coordination, repeated checks and intermediate reporting while retaining
the acceptance criteria and safety thresholds below:

- Work only on outstanding requirements. Reuse recorded PASS evidence for the
  unchanged candidate; rerun affected checks after changes or failures and keep
  every repetition explicitly required by this plan, including after G08/G09.
- Define the expected result, required evidence and stopping condition before
  each scenario. Stop once those criteria are met; add checks only to resolve
  a concrete failure or remaining uncertainty.
- Before live actions, verify current ownership, readiness, observation and
  recovery prerequisites. Historical PASS does not establish current health.
- Batch independent reads and validate retained evidence with a collector.
  Keep dependent mutations sequential, preserve the required call spacing,
  and never retry an uncertain mutation automatically.
- Reuse the existing G05 client A/B tasks and fixtures. Add another agent or
  review only when it addresses a specific implementation or recovery risk.
- Prepare the scenario and confirm operator UI readiness before starting a
  bounded capture. Use the validated Windows PowerShell 5.1 host for the
  existing cutover controllers. An expired capture is not request evidence.
- Keep raw output in private evidence files and report concise findings,
  deviations and the next required action. Produce one report and selective
  local commit per completed gate, or a checkpoint for an actual blocker.
- Preserve real client results, binding/session isolation, rollback evidence
  and actual UI confirmation. This efficiency policy does not waive a gate,
  turn PENDING into PASS or authorize publishing or pushing changes.

## 2. Review focus

1. Stale schemas or mounted old widgets: fresh metadata and actual served /v2/ resources must match the candidate; restarting a server alone is insufficient.
2. Loss of observation: an exhausted bounded trace is not evidence of quiet operation. Check coverage before declaring a phase passed.
3. Replayed mutations: cancellation, reconnect and retries must not repeat a completed write. A command may fail visibly during a fault; its side effect must not be silently duplicated.
4. Identity isolation: two conversations must keep separate workspace/exec state before and after reconnect and OS recovery.
5. Orphan processes/rollback dependency: failure of the Go service must not leave two tunnels, and rollback must work without asking the broken connector to execute it.

## 3. Current snapshot, not a new PASS

Inspected 2026-09-30T18:32:22.9895883+03:00 (host timestamp):

- Checkout: C:\Users\FoxOS_User\codexify-go.
- HEAD: 1098803bb3861e3f5d1a2702e4f5fda42afe0546, with tracked and untracked changes.
- Rust codexify processes observed: 11404 and 12692.
- CodexifyGo: Stopped / Disabled.
- Working-tree source pins tunnel runtime 0.0.15 and Setup/Chat/Update resource URIs /v2/.
- Prior full tests, isolated load tests and widget tests are recorded in the fix report. They must be tied to the frozen candidate and rerun for G00, not copied as live deployment evidence.

`WINDOWS_CUTOVER_REPORT.md` is the September 29 report, including its historical statement that Go was active. It is not the current operating state. Its reboot, new-conversation, worktree and integration gaps remain requirements for this new run.

## 4. State machine and definition of done

```text
NOT_READY
  -- G00 + G01 PASS --> READY_FOR_CUTOVER
  -- G02 PASS ------> GO_TRIAL
  -- G03..G10 PASS -> GO_ACCEPTED
  -- G11 PASS ------> CUTOVER_CLOSED

Any safety abort -> STOPPING -> RUST_RECOVERED / ROLLBACK_FAILED
Any unresolved observation/test gap -> HOLD / BLOCKED, never PASS
```

**Operational transition is accepted only at GO_ACCEPTED.**
**The migration task is finished only at CUTOVER_CLOSED.**

Gate statuses: PENDING, RUNNING, PASS, FAIL, BLOCKED, or N/A_SCOPE. A mandatory gate cannot use N/A_SCOPE. Optional subfeatures may use it only with a stated reason and an explicit agreed scope decision before final acceptance; never silently relabel a failed required test.

A PASS record contains: candidate ID, configuration-profile hash, exact start/end timestamps with offset, action, expected/observed result, evidence path, and checker. Reboot/new-conversation evidence also identifies the user action. A summary without underlying evidence is not sufficient.

Material binary, dependency, routing, auth, upstream, or active feature changes create a new candidate: rerun G00 and every impacted gate, and restart G10. Predeclared capture-enabled/disabled profiles and planned recovery events do not change the candidate. Documentation-only edits do not reset runtime acceptance.

## 5. Required gates and tests

### G00 - Frozen candidate and local verification

- [ ] Review the diff and separate relevant changes from unrelated work. Record an immutable source revision or an exact source snapshot plus patch/hash for the trial. By closure the deployed source must have a traceable commit.
- [ ] Record candidate ID, Go version/GOOS/GOARCH, binary SHA256, tunnel version and SHA256, go.mod/go.sum hashes, tool-schema fingerprint, and /v2/ resource fingerprints. Do not rely on a generic 0.x-dev string.
- [ ] Freeze test and final configuration profiles, startup/recovery policy and enabled integrations. Full configurations/credentials stay in private host storage; version control receives only safe metadata.
- [ ] Run `go test ./... -count=1`, `go vet ./...`, and native Windows build for the candidate. Capture output and exit codes.
- [ ] Run `go test ./internal/ui -count=10` with Node present; an accidental SKIP is not PASS. Run `go test ./internal/mcpdiag -run '^TestIsolatedConcurrentCallsNoAmplification$' -count=10` locally, not through the hosted connector.
- [ ] Record race-detector status separately. A missing C toolchain is a disclosed residual test gap, not a passed race check or a requirement to alter the Windows installation for this cutover.

PASS: the deployable binary can be tied to exactly the tested source and dependency set, with all above required checks passing.

### G01 - Independent recovery and evidence readiness

- [ ] Snapshot working Rust executable, configuration, scheduled-task definitions, startup settings and relevant binding state. Verify hashes, access and actual restore paths without publishing secrets.
- [ ] Prepare a local rollback entry point usable outside ChatGPT: stop/disable the candidate and its owned tree, verify no Go-owned tunnel remains, restore the captured Rust startup state, then verify Rust locally. It must not call MCP or depend on a running Go worker.
- [ ] Validate rollback targeting and ordering without killing unrelated processes. Prepare the manual fallback and an operator who can use it if the connector disconnects.
- [ ] Prepare independent local process/log/health observation and the safety thresholds in section 6. Test the observer/abort action on a disposable fixture first, not by creating a live burst.
- [ ] Record a Rust reference snapshot with no other connector jobs running: process tree, health, log activity and at least 10 spaced get_environment calls. Use it only as a reference, not as proof of Go behavior.
- [ ] Pause other Windows automations/conversations and close old widgets before the controlled baseline. Record what must be restored after acceptance.

PASS: rollback and observability are available locally before changing the live owner. A rollback script that has not been reviewed or a trace with no coverage plan blocks the run.

### G02 - Exclusive ownership and real rollback rehearsal

- [ ] Switch once to the frozen Go candidate with the independent local observer active; retain the same authorized tunnel identity.
- [ ] Verify one Go SCM supervisor, the expected interactive-user worker and one owned tunnel; Rust scheduled starts/watchdog cannot restart it, and no Rust runtime participates in serving requests.
- [ ] Confirm local endpoint readiness and one successful hosted read. Health alone is not the hosted-call check.
- [ ] Rehearse the real local Go -> Rust rollback once, outside MCP. Target: local Rust readiness within 120 seconds of rollback initiation, and exactly one owner. Record hosted reconnection separately; exclude time the user takes to refresh client UI.
- [ ] Re-enter Go with the same candidate after a successful rehearsal and verify ownership again.

PASS: actual rollback and re-entry work, no orphan/duplicate tunnel remains, and the Go identity remains stable. A failed rollback ends the trial and must be resolved on Rust before retrying.

### G03 - Client metadata, existing and new conversations

- [ ] Refresh the connection, verify that metadata actually changed, and start a fresh test conversation. Record the observed input/output schemas and served UI URIs, not merely a statement that Refresh was clicked.
- [ ] Existing conversation retains its intended binding after transport reconnect; a newly opened conversation selects its own intended workspace.
- [ ] Complete at least three ordinary calls in each conversation, using IDs returned by that conversation. Confirm no stale Rust-only argument/schema behavior, unexpected re-selection, or silent fallback.
- [ ] Confirm model-readable results as well as widget output. A pretty widget cannot mask a broken structured result.

PASS: two real ChatGPT conversations work on the current schema and correct bindings. A synthetic client cannot substitute for opening the second hosted conversation.

### G04 - Everyday tools, exec and state integrity

- [ ] In a dedicated fixture directory, create/read/patch a file; verify exact content; run search and Git status/diff; remove only that known disposable fixture after checking its path.
- [ ] Run a delayed command that yields an exec session, poll it using the returned session ID, send stdin, and confirm exact output and exit code. Run one cancellation and verify its child process ends within 10 seconds.
- [ ] Use a fixture mutation marker to confirm one requested operation executes once. Do not replay interrupted writes automatically merely because the response was lost.
- [ ] Verify return of expected validation/auth/tool errors without a retry loop; negative tests use disposable data and never log real credentials.
- [ ] Exercise get_agent_brief, recall/remember where enabled, and one installed skill read. Confirm state survives a planned service restart rather than writing to a different profile.

PASS: all selected operations have the expected result and one intended side effect; no unaccounted duplicate write, hanging exec, or lost state.

### G05 - Multi-project and worktree isolation

- [ ] List real configured projects and select two known fixtures through two conversations. Record canonical paths; include the Windows extended-length path form in validation where relevant.
- [ ] Verify each conversation sees its own marker and working directory. A switch in one conversation must not change the other conversation's binding or exec session.
- [ ] Create a disposable managed worktree, inspect it, resume the same exact path after reconnect, and clean it up only after verifying the target is the fixture and has no user changes.
- [ ] Repeat binding checks after G08/G09 recovery. Compare saved state to the private baseline where migration is involved.

PASS: correct identity and path behavior, no cross-conversation leakage or deletion of unrelated work. Cross-build/unit-test success alone does not pass this gate.

### G06 - Widgets and absence of feedback amplification

- [ ] Reconfirm shipped JavaScript tests: 100 theme/layout globals and synchronous/asynchronous tool-result globals do not initiate new tool calls; explicit buttons still work.
- [ ] Open one fresh Go widget at a time. Record one user action and the resulting actual method/tool sequence; separately account for the parent model tool call and host discovery/metadata requests.
- [ ] Expected initial calls from the widget bridge: Setup = setup_status + list_projects; Chat = chat_read; Update = self_update_status; Diff = none. These are NOT limits on all host traffic.
- [ ] Check Refresh, project selection/scratch and repeated clicks on fixture data. After each action completes, observe at least 5 minutes without further input: no unplanned tools/call sequence continues.
- [ ] Test Chat only when enabled in the declared deployment profile; otherwise mark that subfeature out of scope explicitly. Do not enable it mid-soak or claim disabled-feature live coverage.
- [ ] Verify UI controls, visible output and console errors in the actual supported host. Stored synthetic browser results supplement, not replace, a fresh hosted widget test.

PASS: no self-sustaining app/tool feedback, no duplicate mutation from repeated input, all observed calls accounted for. Unknown traffic is investigated or BLOCKED, never waved away as normal SDK sessions.

### G07 - Required integrations and artifact/Git workflow

- [ ] Inventory enabled upstreams from the chosen profile. For each required upstream, perform discovery plus one authorized read; record exact function, result and error behavior. HH/Habr checks are read-only; no vacancy applications are sent as a test.
- [ ] Import/export a small non-sensitive attachment through the actual host path; compare file bytes/SHA256. A sandbox-only or loopback substitute is not a hosted round-trip.
- [ ] In an explicitly selected test branch/repository owned by the user, make a fixture commit and push it without force; verify the remote commit. Do not publish unrelated changes or private logs.
- [ ] Verify read-only update status where available. Applying a self-update is a separate optional subfeature: defer explicitly when no distinct approved release exists. Manual deployment/rollback remains mandatory.

PASS: required integrations and real file/Git workflows operate through Go. An external upstream outage makes that subtest BLOCKED pending a rerun, not an automatic connector-code failure or PASS.

### G08 - Service, worker and tunnel recovery

- [ ] With no real user mutation in flight, run one ordinary SCM restart, then separately test one tunnel termination, one worker termination and one supervisor termination. Use precisely identified fixture/deployment PIDs; do not mass-kill process names.
- [ ] After each fault, the old owned process tree exits; no orphan persists longer than 10 seconds. Exactly one replacement owner reaches local readiness within 60 seconds, measured with network and interactive session already available.
- [ ] Confirm three successful hosted reads and both bindings after each recovery. No extra manual service start or repeated connector Refresh is allowed to hide a recovery failure; initial client schema refresh is a separate phase.
- [ ] An interrupted exec may end with an explicit error and require a new command; it need not survive process death. It must not be duplicated, silently rerun or reported as successful without evidence.

PASS: all four scenarios recover once without a restart loop, silent Rust fallback or data loss. External hosted downtime is recorded separately; never infer local recovery from a delayed forwarded-log timestamp.

### G09 - Windows OS/session persistence

- [ ] Save user work and obtain confirmation immediately before a real Windows reboot. Compare boot identity/timestamps before and after, not merely service uptime.
- [ ] Verify automatic Go service operation and expected worker availability under the interactive user. Target: local readiness within 120 seconds after user sign-in and network readiness, with Rust still disabled.
- [ ] With no interactive user, the service may wait according to its documented design; do not require running the user worker as LocalSystem to fake readiness.
- [ ] Run a separate logout/login test and a supported sleep/resume test, with user coordination; verify one tunnel, correct user context, binding and successful hosted reads afterward. Unsupported OS power states may be N/A_SCOPE with evidence, but reboot may not.

PASS: actual cold boot and session persistence are demonstrated. Reboot remains PENDING until the user actually performs/approves it; a unit test cannot replace it.

### G10 - Go-only observation and performance

- [ ] After earlier gates pass, observe the same candidate over at least 24 hours of elapsed time, including at least 4 hours of awake connected runtime and two separate normal work sessions totaling at least 2 hours.
- [ ] Complete at least 50 intentional ordinary tool actions across those sessions; do not run artificial rapid calls just to reach a count. Initially keep other jobs paused, then reintroduce the required normal background workload once, recording its expected cadence.
- [ ] Cover three 15-minute no-input intervals (after warm-up, between work sessions, and near the end). Zero unexplained repeated tools/call; protocol housekeeping and declared background jobs are classified separately.
- [ ] No unexplained service/worker/tunnel restarts, manual rescue, duplicate actions, binding loss, secret exposure or cutover-caused rate limit. A verified external outage pauses affected observations; rerun interrupted gates rather than silently counting the outage as stable uptime.
- [ ] Process/health/log aggregate observation covers the full connected run. Missing coverage is BLOCKED for that interval; regenerate enough observation before acceptance. Do not promise unattended collection unless a verified local collector is actually running.
- [ ] Compare 20 spaced trivial hosted reads with the G01 Rust reference: target p95 <= max(2x Rust p95, 5 seconds), measured dispatch-to-result, excluding model think time. Disclose network differences and rerun under comparable conditions if they invalidate the comparison.
- [ ] At matched idle points, measure only the owned supervisor/worker/tunnel tree. Target: average CPU <= 3% of total machine capacity over each idle window; private bytes after the workload <= warm idle baseline + max(25% of baseline, 256 MiB). Persistent growth beyond this is a review blocker, not proof of a leak from one sample. Track handles/threads and investigate monotonic growth across the three comparable idle samples.

PASS: the observation requirement and workload coverage both pass. Twenty-four hours mostly powered off, a quiet exhausted trace, or a few successful commands do not pass. A new runtime-affecting fix restarts this gate.

### G11 - Closure and recovery retention

- [ ] Confirm the accepted binary/configuration hashes, default automatic Go startup, inactive Rust tasks/watchdog and zero dependency on Rust executables at runtime. Files retained only for rollback are allowed.
- [ ] Put the deployed source in traceable commits and the user's chosen fork/release path after reviewing the exact diff. No blanket commit -a and no force-push. Record destination and commit; an upstream PR merge is not required for local acceptance.
- [ ] Switch to the predeclared normal diagnostics-disabled configuration, then perform a final restart, 30 minutes of observed runtime and the core/identity smoke checks. Diagnostic policy toggling is an expected profile transition, not a silent change in other settings.
- [ ] Remove/disable only temporary test jobs and guards after Go acceptance and manual rollback verification; restore the specifically paused normal jobs. Do not remove the Rust recovery copy or historical evidence as cleanup.
- [ ] Keep Rust recovery assets for at least 7 days AND three successful normal work sessions after GO_ACCEPTED. Deletion is a later explicit action, not an automatic effect of closure.
- [ ] Complete a final report: gate ledger, tested hashes, actual metrics, accepted scope, residual risks, evidence locations, rollback instructions and explicit user/maintainer acceptance. Do not call the migration closed solely because the service is Running.

PASS: all required G00..G10 are PASS, only explicitly agreed optional scope exclusions remain, the final normal profile works, and ownership of the accepted deployment is clear.

## 6. Safety stop and rollback rules

Freeze these conservative triggers with the workload plan before the live run:

- Immediate stop/rollback: duplicate live owners of the same tunnel, confirmed extra mutation, cross-workspace data access, secret disclosure, or inability to control the Go-owned processes locally.
- In a declared idle window after 30 seconds of settling, >=10 unexplained tools/call in 10 seconds OR >=5 unexplained repeats of the same operation fingerprint in 10 seconds triggers an abort. These are safety ceilings, not a permitted amount of self-generated traffic and not a claim that equal hashes prove retries.
- In any quiet window, >100 post-response forwarded-log events in 60 seconds with no declared workload also triggers a conservative abort even when decoded tracing has expired. This coarse alarm is not an incoming-request counter or a method attribution.
- A persistent low-rate self-sustaining sequence fails G06/G10 even below the emergency thresholds. Do not wait for RATE_LIMITED to recognize amplification.
- Any RATE_LIMITED pauses new stimuli immediately. Attribute it using captured traffic; local amplification leads to rollback, independently confirmed unrelated hosted limits lead to HOLD and a later rerun. Do not assert that every 429/503 is a Go defect.
- Two unexpected owned-process restarts inside 5 minutes, readiness beyond G08/G09 limits, or lost local observation/control during the guarded trial trigger stop/rollback. Intentional fault tests are labelled and do not count as unexplained restarts.
- If local rollback does not reach Rust readiness within 120 seconds, declare ROLLBACK_FAILED, keep the Go auto-start path stopped, and use the documented local manual recovery. Never start both owners as a workaround.

The independent abort mechanism must operate locally rather than by repeated MCP polling. During preparation it must be tested on fixtures and given narrowly scoped authority. This document does not install or arm such a mechanism.

## 7. Evidence coverage and bounded diagnostics

A recorder maximum of one hour cannot certify a 24-hour trial. The current recorder also has no automatic rotation or re-arm. Its event budget counts events, normally four per request, not calls.

Before each focused live gate, verify a fresh capture and enough duration/event budget. Use declared capture-enabled profiles for focused windows; controlled worker restarts may open new captures. Record each interval and its stop reason. Hashes are comparable within a capture only because HMAC keys change.

For the longer observation, use separately prepared local aggregate process/log/health collection that does not invoke MCP, store raw payloads or expose credentials. Record its actual coverage and failures. Coarse log counters can raise an alarm, but cannot retrospectively supply a missing tool name. If an unexplained sequence appears, fail/hold the gate and obtain a new bounded capture rather than declaring success from incomplete data.

Private evidence directory to create during execution (not created or armed by this document):

```text
C:\Users\FoxOS_User\.codexify-go\cutover-v2\<run-id>\
  manifest.json                 # safe candidate/profile hashes, no secrets
  actions.jsonl                 # timestamped test actions and expected budgets
  capture-index.json            # capture ranges, stop reasons and hashes
  process-health-summary.jsonl  # local aggregate observation
  gate-results.json             # per-gate result and evidence pointers
  private-traces\              # metadata traces with restricted access
  rollback-private\            # sensitive configuration/task snapshots
```

Do not commit the private directory. Public/repository reports contain sanitized summaries and hashes only. Exact command lines, configuration contents and raw application logs require review before export.

## 8. Acceptance ledger for the new run

| Gate | Requirement | Current status | Required evidence |
| --- | --- | --- | --- |
| G00 | Frozen candidate/local checks | PENDING | candidate manifest + fresh test/build outputs |
| G01 | Recovery/observation preflight | PENDING | private snapshot + reviewed local scripts + fixture proof |
| G02 | Exclusive live owner/rollback rehearsal | PENDING | process tree + actual rollback/re-entry times |
| G03 | Fresh schema/two conversations | PENDING | observed schemas/URIs + conversation binding checks |
| G04 | Tools/exec/state | PENDING | fixture results + exec/cancel/mutation checks |
| G05 | Multi-project/worktrees | PENDING | canonical paths + isolated markers + resume evidence |
| G06 | Hosted widgets/no feedback | PENDING | action/call budgets + quiet windows + UI checks |
| G07 | Integrations/artifacts/Git | PENDING | per-upstream reads + file hashes + remote fixture commit |
| G08 | Process recovery | PENDING | fault/action times + PID/ownership + successful reads |
| G09 | Reboot/session persistence | PENDING | boot/logon/resume evidence + post-boot smoke |
| G10 | Go-only observation | PENDING | real duration/workload/coverage + latency/resource summaries |
| G11 | Closure | PENDING | final profile + source delivery + acceptance/rollback record |

Optional scope ledger initially UNDECIDED: Chat when disabled; self-update apply; unsupported sleep state. Required active upstreams must be inventoried before G01 PASS. No optional feature is declared waived by this file.

## 9. What is explicitly not needed to close this transition

- Retrospective attribution of every old command UUID or proof of the exact historical burst source.
- Fixing the inactive Rust Chat widget, deleting Rust binaries, or upgrading unrelated machines.
- Rewriting a working stateless handler or changing the protocol for cosmetic log reduction.
- Full parity with every unused Rust feature; closure covers the explicitly accepted feature inventory only.
- Zero transient external service incidents for all time, or a mathematical proof that no bug remains.

An unresolved historical root cause may remain in the risk register. An unexplained new amplification, duplicate mutation, binding defect, mandatory PENDING/BLOCKED gate, or failed rollback may not.

## 10. References and source boundaries

Repository evidence read for this plan:

- `docs/handoffs/WINDOWS_CUTOVER_REPORT.md`: September 29 phase list, actual orphan-process defect and unfinished gates.
- `docs/handoffs/WINDOWS_MCP_BURST_FIX_AND_DIAGNOSTICS_20260930.md`: isolated widget/diagnostics validation and hosted-attribution limits.
- `tools/forensics/mcptrace/README.md`, `internal/mcpdiag/recorder.go`: current capture limits, stop behavior and privacy limits.
- `internal/tunnel/runtime.go`, `internal/ui/resources.go`: current working-tree candidate version/resource references, not proof of installed Go behavior.

Official client guidance consulted 2026-09-30:
https://developers.openai.com/plugins/deploy/connect-chatgpt

It prescribes refreshing changed metadata, checking that the advertised metadata changed, and repeating affected tests in a new conversation. Numeric acceptance targets, state names, gate grouping and rollback triggers above are this project's proposed policy, not quotations from that guidance.
