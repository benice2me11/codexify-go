# Windows Rust -> Go Cutover Report

## Host and builds

- Date: 2026-09-29
- Platform: Windows x86_64
- Rust baseline: Codexify 1.6.5
- Go source branch: `main`
- Go source commit at cutover start: `9cdbe90`
- Managed tunnel runtime: `v0.0.12`
- Connector strategy: existing real ChatGPT tunnel identity switched in place from Rust to Go
- Rust rollback: private local snapshot of the Rust executable, configuration, watchdog script, and Scheduled Task XML created before cutover; no credentials are committed here

## Current status

The real Windows connector is currently served by Codexify Go. The Rust `Codexify` and `Codexify Watchdog` Scheduled Tasks are disabled, and no `codexify.exe` Rust process is present.

The installed Go path uses:

- SCM service `CodexifyGo` with automatic startup;
- LocalSystem service supervisor;
- interactive-user `codexify-go worker run`;
- Go-owned managed `tunnel-client-runtime v0.0.12`;
- loopback MCP endpoint on port 3300.

The same ChatGPT conversation recovered across the hot transport replacement and across forced tunnel/service failures.

## Phase status

| Phase | Status | Evidence / notes |
| --- | --- | --- |
| 0 - Baseline and rollback | PASS | Rust 1.6.5 was healthy before cutover. `doctor` reported a healthy service, connector, and managed tunnel. A private rollback snapshot was created before changing the active path. |
| 1 - Go connector cutover | PASS | Go was built and tested natively on Windows, a separate `CodexifyGo` SCM service was installed, Rust tasks were disabled, Rust processes were terminated, and the same real tunnel identity reconnected through Go. |
| 2 - Cold boot | PENDING | Requires a real Windows reboot with Rust still disabled. |
| 3 - ChatGPT reconnect | PASS for hot reconnect | This conversation continued through Rust -> Go replacement and later tunnel/service crash recovery. An explicit connector Refresh is still recommended to replace stale Rust tool-schema metadata in the ChatGPT client. |
| 4 - Conversation persistence | PARTIAL | The active conversation stayed bound through hot cutover and service/tunnel recovery. Old/new conversation coverage after connector Refresh and reboot remains pending. |
| 5 - Multi-project | PENDING | Not yet exercised as part of this Windows run. |
| 6 - Worktree lifecycle | PENDING | Not yet exercised as part of this Windows run. |
| 7 - Real Git delivery | IN PROGRESS | The Windows-only supervisor fix from this cutover is ready to be committed/pushed after final verification. |
| 8 - Long exec/stdin | BLOCKED BY STALE CLIENT SCHEMA | Hot replacement left ChatGPT with part of the Rust tool schema. The Go server is active, but the host-side `write_stdin` schema must be refreshed before this phase is accepted. |
| 9 - Tunnel recovery | PASS | Killing the active Go-managed tunnel produced one replacement tunnel with a new PID and no Rust fallback. |
| 10 - Service recovery | PASS after fix | Initial forced SCM service death exposed an orphan tunnel. After the Windows Job Object fix, forced service death restarted the SCM service and the old tunnel was gone; exactly one replacement tunnel remained. |
| 11 - Self-update | PENDING | No update apply was exercised during this run. |
| 12 - Bindings/state | PARTIAL | Current conversation binding survived hot replacement and service/tunnel restart. Reboot/new/old conversation coverage remains pending. |
| 13 - Plugin skills | PENDING | Not yet exercised as part of this Windows run. |
| 14 - Upstream MCP | PENDING | Not yet exercised as part of this Windows run. |
| 15 - Attachments | PENDING | Not yet exercised as part of this Windows run. |
| 16 - Rust dependency elimination | PARTIAL / ACTIVE | Rust tasks are disabled and no Rust process is in the active path. Representative Go-only connector and recovery operations work. Reboot plus broader workflow coverage is still required for final acceptance. |

## Defect found and fixed

### WINDOWS-CUTOVER-001 - orphan managed tunnel after SCM service crash

**Severity:** P2 during controlled cutover; replacement-blocking if left unresolved for production service recovery.

Initial service-crash test:

- the active `CodexifyGo` service was killed with `Stop-Process -Force`;
- Windows SCM correctly restarted the service;
- the pre-crash `tunnel-client-runtime.exe` survived as an orphan;
- the restarted service also launched a new tunnel, leaving two Go tunnel processes for the same cutover installation.

Root cause:

- normal Windows supervisor shutdown used `taskkill /T`;
- the supervised tunnel process itself was not held in a Job Object;
- a hard service-process death bypassed Go cleanup code, so the kernel had no ownership primitive that required the child process tree to die.

Fix:

- every Windows `CommandFactory` supervised process is attached immediately after start to a dedicated Job Object;
- the Job Object uses `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`;
- the handle remains owned by the supervising Go process for the lifetime of the child;
- normal child exit releases the handle;
- if the supervising service dies unexpectedly, Windows closes the handle and terminates the supervised process tree;
- non-Windows process-group behavior is unchanged.

Regression coverage:

- `TestAttachProcessJobKillsProcessWhenReleased` verifies that closing the Job Object terminates a real Windows process;
- `go test -count=1 ./internal/supervisor` passes;
- full `go test ./...` passes;
- native Windows build passes.

Live verification after installing the fixed build:

- old service PID: `7332`;
- old tunnel PID: `6108`;
- service killed forcibly;
- replacement service PID: `2748`;
- old tunnel alive: `False`;
- Go tunnel count after recovery: `1`;
- replacement tunnel PID: `13988`;
- subsequent direct tunnel crash also recovered to exactly one tunnel, PID `15152`.

PID values are evidence for this run only.

## Rust-unavailable evidence

After cutover:

- Scheduled Task `Codexify`: disabled;
- Scheduled Task `Codexify Watchdog`: disabled;
- Rust `codexify.exe` process count: zero;
- SCM `CodexifyGo`: running and automatic;
- Go worker: running under the active user context;
- Go-managed tunnel: running from the private `.codexify-go` installation;
- this ChatGPT conversation continues to execute MCP operations through the Go connector.

The Rust executable/configuration remain on disk only for rollback and are not in the active execution path.

## Remaining Windows acceptance gates

1. Refresh the Codexify connector tool list in ChatGPT so the client receives the Go tool schema.
2. Re-run long command/stdin/cancellation.
3. Exercise new and existing conversations.
4. Exercise multiple projects and managed worktree create/switch/resume.
5. Perform a real Git commit/push through the installed Go connector.
6. Exercise installed plugin skills, upstream MCP, and real attachment import/export.
7. Reboot Windows with Rust tasks still disabled and verify automatic Go service/worker/tunnel recovery.
8. Continue normal Go-only dogfooding before declaring Windows replacement complete.
