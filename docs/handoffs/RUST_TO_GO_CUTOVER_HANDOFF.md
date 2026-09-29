# Rust Codexify -> Codexify Go Cutover Handoff

## Purpose

This handoff defines the final dogfooding and cutover validation required before Codexify Go can be considered a complete operational replacement for the original Rust Codexify.

This document is a **future test specification**. Do not perform the cutover merely because this file was opened. Start only when the user explicitly asks to begin the Rust -> Go replacement test.

## Active cutover run

The first real Linux cutover run started on 2026-09-29 after explicit user approval. Live evidence and phase status are recorded in `docs/handoffs/RUST_TO_GO_CUTOVER_REPORT.md`.

The run uses the real ChatGPT connector that previously terminated in Rust Codexify 1.6.6. It is being switched in place to the Go implementation so reconnect, conversation binding, schema migration, and Rust-unavailable behavior are exercised rather than simulated.

The test must use at least one **real Codexify connector and real day-to-day development work**. Synthetic smoke tests alone are insufficient.

## Primary acceptance criterion

The replacement is proven only when the original Rust Codexify can be completely stopped and removed from the active execution path while Codexify Go continues to support real ChatGPT development sessions independently.

The decisive final condition is:

> Stop/remove Rust Codexify. Keep it unavailable. Codexify Go must continue to connect, recover, execute real project work, persist state, and survive service/tunnel restarts without using the Rust implementation.

A green unit/CI matrix, successful cross-build, or one synthetic MCP smoke does **not** satisfy this criterion.

## Scope

Validate the full operational chain:

```text
ChatGPT
  -> connector / tunnel
  -> codexify-go service
  -> MCP server
  -> workspace/project binding
  -> Git/worktrees/files/exec
  -> skills/plugins/upstream MCP
  -> persistent state
```

The test should begin with one real connector. After that connector is stable on Go, use it for several normal working sessions before declaring the migration complete.

## Preconditions

Before beginning:

- Codexify Go release candidate is committed, pushed, and has a green CI matrix.
- The target OS has already passed its platform lifecycle validation.
- The Go service and managed tunnel runtime can be installed independently of Rust Codexify.
- Current Rust connector/service configuration is documented well enough to roll back.
- Relevant Codexify state/configuration is backed up before destructive migration steps.
- Git working trees involved in the test are clean or their existing changes are explicitly classified.
- No secrets, API keys, absolute personal machine paths, session credentials, or private connector tokens are committed to Git.
- The Rust implementation remains available for rollback during the early phases, but it must not remain in the active path during final acceptance.
- Record the exact Rust version/commit and Go version/commit used for the cutover.

## Evidence requirements

For every phase, record enough evidence to distinguish a real pass from an assumed pass.

Capture, where relevant:

- OS / architecture;
- Codexify Go version and Git commit;
- Rust Codexify version/commit;
- connector/tunnel identity without exposing secrets;
- service status and PID changes;
- tunnel status and PID changes;
- timestamps;
- selected project/worktree;
- relevant MCP result;
- Git status before/after;
- expected vs actual persistence after reconnect/restart;
- failure logs;
- recovery behavior;
- whether Rust was running, stopped, or unavailable.

Do not record secrets or personal absolute paths in committed evidence. Normalize/redact machine-specific paths before committing test reports.

## Test phases

### Phase 0 - Baseline and rollback preparation

Establish the known-good Rust baseline before changing the active connector.

Verify that the existing Rust connector currently works for a real project. Record the current connector configuration, active service/tunnel state, relevant state locations, and rollback procedure.

Prepare a rollback that can restore the Rust path without guessing. Do not delete Rust yet.

**Pass:** baseline is known-good and rollback is explicit.

**Fail:** current state cannot be reproduced or restored reliably.

### Phase 1 - Go connector cutover

Switch one real Codexify connector completely to Codexify Go.

The connector must reach the Go MCP server/tunnel directly. Do not leave Rust as an invisible proxy, fallback, helper process, or dependency.

Verify connector setup/status and perform a small real project operation.

**Pass:** ChatGPT is demonstrably talking to Go.

### Phase 2 - Cold boot

Reboot the OS.

After login/startup, do not manually launch Codexify components unless that is part of the documented product workflow.

Verify:

- service autostart;
- tunnel startup;
- MCP health;
- connector reconnection;
- persisted project state;
- no unexpected terminal/pop-up windows.

**Pass:** Codexify Go returns to an operational state through its installed lifecycle.

### Phase 3 - ChatGPT reconnect

Disconnect/reconnect ChatGPT or otherwise force a connector transport reconnection.

Verify that the connector recovers without reinstalling or manually reconstructing state.

Test both:

- same conversation reconnect;
- new transport/session where applicable.

### Phase 4 - Conversation persistence

Test both a **new conversation** and an **existing/older conversation**.

Verify:

- workspace binding behavior;
- conversation-scoped state;
- connector schema/stale-version handling;
- no accidental inheritance between unrelated conversations;
- expected persistent state survives reconnects.

### Phase 5 - Multi-project operation

Use multiple real projects.

Verify:

- project discovery;
- project selection/switching;
- no state leakage between projects;
- correct workspace confinement;
- independent Git state;
- conversation/project binding persistence.

At least one session should switch between projects more than once.

### Phase 6 - Worktree lifecycle

Exercise the complete managed worktree flow:

1. create/select a project with worktree behavior enabled;
2. create/use a managed worktree;
3. modify files;
4. inspect status/diff;
5. switch away;
6. switch back;
7. reconnect/resume;
8. verify the same worktree/state is recovered.

Verify create/switch/resume semantics, managed-worktree detection, cleanup behavior, and that the user's Git index is not corrupted.

### Phase 7 - Real Git delivery

Perform a real development change through Codexify Go.

Required flow:

```text
read/inspect
-> edit/apply_patch
-> tests
-> show_diff/git status
-> commit
-> push
```

Verify SSH/Git credential access in the installed service context.

**Pass:** a real repository change reaches its remote using only the Go connector path.

### Phase 8 - Long-running commands and stdin

Exercise `exec_command` / `write_stdin` with:

- quick command;
- long-running command;
- incremental output;
- stdin;
- cancellation;
- command producing child/grandchild processes.

Verify output is not lost between polls and cancellation/service stop leaves no orphan process tree.

### Phase 9 - Tunnel failure and recovery

While a real conversation is connected:

1. identify the Go-managed tunnel process;
2. terminate it unexpectedly;
3. observe supervisor behavior;
4. wait for recovery;
5. continue using the same connector.

Verify restart/backoff/health behavior and absence of manual repair.

**Pass:** a new tunnel process becomes healthy and the connector recovers.

### Phase 10 - Service failure and recovery

Terminate or otherwise crash the Codexify Go service process using a controlled test.

Verify the OS service manager restarts it according to the supported platform lifecycle.

After recovery verify:

- MCP health;
- tunnel;
- existing project state;
- conversation binding;
- real command execution.

### Phase 11 - Go self-update

Use a real published Go release/update path.

Verify:

1. update discovery;
2. correct platform archive selection;
3. checksum verification;
4. extraction/staging;
5. binary replacement;
6. service restart;
7. connector recovery;
8. version change;
9. state preservation.

Windows should exercise ZIP. macOS/Linux should exercise the preferred tar.gz path. Unix ZIP compatibility assets exist for older updaters but are not the preferred path for current clients.

### Phase 12 - Bindings and state persistence

Across service restart, tunnel restart, ChatGPT reconnect, and preferably OS reboot, verify persistence of:

- project bindings;
- conversation bindings;
- worktree/resume state;
- connector schema bookkeeping;
- Markdown chat cursor/state if enabled;
- agent-ticket behavior if enabled;
- saved project memory.

Also verify transient sessions do **not** incorrectly become persistent.

### Phase 13 - Plugin skills

Use real installed plugin skills rather than only fixtures.

Verify:

- Codex plugin discovery;
- enabled/disabled plugin behavior;
- namespaced skills;
- skill read/invocation;
- agent brief inclusion;
- no stale uninstalled cache discovery.

If Claude plugin discovery is enabled for the target installation, test its scoped registry behavior separately.

### Phase 14 - Upstream MCP gateway

Configure at least one real upstream MCP server through Codexify Go.

Verify:

- upstream discovery;
- gateway skill generation;
- schema exposure;
- gateway call;
- upstream result/error propagation;
- reconnect behavior;
- no collision with native Codexify tools.

Where useful, also verify catalog/direct mode behavior.

### Phase 15 - Attachments and host file transfer

Exercise both directions:

**Ingress**

ChatGPT/native attachment -> `import_host_file` -> active workspace.

Verify HTTPS/host policy, size bounds, destination confinement, and no overwrite.

**Egress**

Workspace file -> `export_host_file` -> host-readable artifact.

Verify capability expiry/limits and absence of leaked absolute filesystem paths.

Use at least one real attachment rather than only a generated fixture.

### Phase 16 - Rust dependency elimination

This is the decisive phase.

Stop the original Rust Codexify completely.

Confirm that no Rust Codexify service, process, tunnel, proxy, helper, or hidden fallback remains in the active path.

Where safe and reversible, temporarily rename/move or uninstall the Rust executable/service so accidental fallback cannot succeed.

Then repeat a representative subset of the real workflow:

- reconnect ChatGPT;
- open an existing conversation;
- start a new conversation;
- select/switch projects;
- read/edit files;
- run a long command;
- create/use/resume a worktree;
- commit/push;
- use a plugin skill;
- call an upstream MCP gateway;
- import/export an attachment;
- restart Go service/tunnel and recover.

Finally reboot or perform an equivalent cold-start test with Rust still unavailable.

**PASS:** all required workflows continue through Codexify Go with Rust unavailable.

**FAIL:** any required workflow needs the Rust binary, Rust service, Rust-owned tunnel, Rust-only state, or manual Rust fallback.

## Dogfooding period

Do not declare replacement immediately after Phase 16.

Use the Go connector for several normal working sessions and preferably across multiple days/restarts.

During dogfooding:

- develop a real project;
- ideally use Codexify Go to continue development of `codexify-go` itself;
- record crashes/reconnects/state anomalies;
- do not silently fall back to Rust when something breaks;
- classify defects and either fix them in Go or explicitly fail the cutover.

The strongest proof of self-hosting is:

> Continue developing Codexify Go through Codexify Go while the Rust implementation is unavailable.

## Defect severity during cutover

Use the following operational severity:

### P0 - Cutover blocker / catastrophic

Examples:

- data loss or repository corruption;
- credential/security boundary compromise;
- Go cannot start/connect at all after cutover;
- update destroys the active installation without a viable rollback.

Any P0 immediately stops the cutover.

### P1 - Replacement blocker

Examples:

- required workflow still depends on Rust;
- workspace confinement failure;
- persistent state is lost or assigned to the wrong conversation/project;
- service/tunnel cannot recover without manual reconstruction;
- Git delivery cannot work from the installed Go service context.

P1 means Codexify Go is **not yet a complete replacement**.

### P2 - Major but potentially non-blocking

Examples:

- recovery works but is unreliable/slow;
- optional subsystem fails while the core replacement remains operational;
- orphan process or lifecycle inconsistency that does not corrupt data;
- meaningful UX/runtime regression with a workaround.

Resolve or explicitly accept before v1.0.

### P3 - Minor / polish

Examples:

- cosmetic widget differences;
- non-critical logging/documentation issue;
- flaky test with no production behavior impact.

P3 does not by itself block cutover.

## Failure handling

When a phase fails:

1. preserve logs/evidence before restarting anything;
2. classify the defect P0-P3;
3. identify whether the failure is Go, infrastructure, connector, ChatGPT host behavior, or the test itself;
4. do not mask a Go defect by silently re-enabling Rust;
5. fix in a focused commit;
6. rerun the failed phase;
7. rerun any earlier phase whose invariant could have been affected;
8. keep the overall cutover status as failed/incomplete until the required regression passes.

## Rollback

Rollback exists to restore usability, not to turn a failed test into a pass.

If rollback is required:

- stop the Go connector/service path cleanly;
- preserve Go logs/state for diagnosis;
- restore the documented Rust connector/service configuration;
- verify the Rust baseline again;
- record exactly why rollback was required.

A rollback means the replacement acceptance criterion has **not** been met.

## Required final evidence/report

Create a cutover report alongside this handoff or under the project's verification documentation.

The report should include:

- Go release/version/commit tested;
- Rust version/commit retired;
- OS/architecture tested;
- connector used;
- phase-by-phase PASS/FAIL;
- evidence references/log excerpts without secrets;
- defects found and fixing commits;
- rollback events, if any;
- dogfooding duration/sessions;
- explicit confirmation that Rust was unavailable during final acceptance;
- final verdict.

## Final acceptance checklist

Do not mark the migration complete until all of the following are true:

- [ ] One real connector runs entirely through Codexify Go.
- [ ] Cold boot succeeds.
- [ ] ChatGPT reconnect succeeds.
- [ ] New conversation succeeds.
- [ ] Existing conversation succeeds.
- [ ] Multi-project switching succeeds.
- [ ] Worktree create/switch/resume succeeds.
- [ ] Real commit/push succeeds.
- [ ] Long command/stdin/cancellation succeeds without orphan processes.
- [ ] Tunnel crash recovery succeeds.
- [ ] Service crash recovery succeeds.
- [ ] Self-update and service restart succeed.
- [ ] Bindings/state persist correctly.
- [ ] Real plugin skills work.
- [ ] Upstream MCP gateway works.
- [ ] Real attachment import/export works.
- [ ] Rust Codexify is stopped/removed from the active path.
- [ ] Representative workflows still pass while Rust is unavailable.
- [ ] Cold-start/reboot still passes while Rust is unavailable.
- [ ] Several normal dogfooding sessions complete without Rust fallback.
- [ ] No unresolved P0/P1 defects remain.
- [ ] Final cutover report explicitly concludes that Codexify Go is independent of Rust Codexify.

## v1.0 implication

Passing this handoff is the operational criterion for calling Codexify Go a complete replacement for the original Rust Codexify.

Feature parity alone is insufficient.

A reasonable release progression is:

```text
platform parity
-> real Go connector dogfooding
-> Rust unavailable cutover
-> fix cutover defects
-> sustained Go-only operation
-> v1.0.0
```
