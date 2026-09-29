# Rust to Go Cutover Report

## Run identity

- Started: 2026-09-29
- Host: Ubuntu 26.04.1 LTS, Linux x86_64
- Go toolchain: go1.26.5 linux/amd64
- Go source commit under test: `9cf3514f2f3ea9f529e6ff45c95691865db3f372`
- Go version: `0.8.2-dev`
- Rust baseline: Codexify `1.6.6`
- Managed tunnel runtime: `v0.0.12`
- Connector: the existing real ChatGPT connector, switched in place from Rust to Go
- Rust rollback: private local snapshot of the Rust binary, config, and systemd user unit created before cutover; no credentials are committed here

## Current status

Cutover is **in progress**. Platform parity is not being reimplemented during this run unless a cutover test exposes a concrete defect.

This report records the **Linux cutover only**. A PASS in this report means the behavior has been verified on the Ubuntu/Linux host described above; it must not be interpreted as equivalent Rust -> Go replacement evidence for Windows or macOS. Windows and macOS require their own platform-specific cutover and acceptance runs before cross-platform replacement can be declared complete.

The real connector is currently served by Codexify Go. The Rust user service is stopped. This conversation continued through the transport switch and successfully executed Go MCP calls after the Rust service became inactive.

## Phase status

| Phase | Status | Evidence / notes |
| --- | --- | --- |
| 0 - Baseline and rollback | PASS | Rust 1.6.6 was healthy before cutover. Its binary was confirmed non-Go and identified itself as `Codexify MCP bridge (Rust)`. Private binary/config/unit rollback snapshot created before stopping it. |
| 1 - Go connector cutover | PASS with defects | Go 0.8.2-dev built from `9cf3514`, installed a separate systemd user unit and separate managed tunnel runtime, then took over the same real tunnel identity after Rust stopped. Current Go service and tunnel are healthy enough to serve this conversation. |
| 2 - Cold boot | PASS (Linux) | A second clean reboot was performed after removing the temporary required test upstream and disabling the preserved Rust unit. Without any manual Codexify/tunnel start, systemd started Codexify Go automatically; the service was `active/enabled` with `NRestarts=0`, while Rust remained `inactive/disabled`. This existing conversation automatically recovered its exact managed worktree and project binding. |
| 3 - ChatGPT reconnect | PASS (Linux) | Hot transport replacement recovered automatically, and a later explicit ChatGPT connector disconnect/reconnect also succeeded. The existing conversation retained its `codexify-go` binding and exact managed worktree, normal Go MCP calls resumed, `cutover_gateway.echo` returned `post-reconnect go gateway ok`, the generated gateway skill remained available, Rust stayed inactive, and the Go service remained active without a service restart. |
| 4 - Conversation persistence | PASS (Linux) | Go-native bindings survived service crash/restart, explicit connector disconnect/reconnect, multi-conversation activity, worktree leave/return, and a clean OS reboot. After the migration fix was installed, a separate legacy Rust-bound conversation with no manual project selection automatically restored `/home/whtvr/codexify-go` and persisted a new Go binding, confirming Rust -> Go conversation migration. |
| 5 - Multi-project | PASS (Linux) | Go enumerates the Linux access-root project catalogue correctly (15 selectable projects observed). A separate conversation was bound to another project, then this conversation was rechecked and still resolved to `codexify-go`, confirming that independent conversation/project bindings do not overwrite each other. |
| 6 - Worktree lifecycle | PASS (Linux) | The Rust side had this conversation in a managed worktree. After Go cutover and rebinding, Go initially selected the source project rather than restoring the Rust worktree. Native Go now lists the source tree plus managed/additional worktrees, successfully resumes this conversation's exact managed worktree via `resumePath`, and after leaving/returning to the conversation resolves the same workspace `/home/whtvr/.codexify-go/worktrees/codexify-go/7193a295d5aa-cbe21f21` and branch `codexify-go/7193a295d5aa-cbe21f21`. |
| 7 - Real Git delivery | PASS | This report and handoff update were committed and pushed to the user's fork through Go-only `exec_command` using the installed service's Git/SSH context. The final rebased delivery commit is `4602610`. |
| 8 - Long exec/stdin | PASS after schema refresh | After refreshing the connector schema, a long-running command returned a string session id and `write_stdin` accepted that id, delivered `go-only-stdin`, and observed normal completion. |
| 9 - Tunnel recovery | PASS | The Go-managed tunnel process was killed unexpectedly. The in-flight tool call disconnected/timed out as expected; the supervisor created a new tunnel process and the same connector resumed serving calls without Rust or manual repair. |
| 10 - Service recovery | PASS | The Go service was killed with SIGKILL. systemd restarted it with a new PID and incremented `NRestarts`; the tunnel and this conversation recovered while Rust stayed inactive. |
| 11 - Self-update | PARTIAL / BLOCKED BY RELEASE STATE | The installed Linux Go binary successfully exercised the live release-check path with `update check --force`: current `0.8.2-dev`, latest published `0.8.1`, status `ahead_of_latest`, source `github_api`. There is no newer release to apply, so a real binary replacement/restart update cannot be truthfully acceptance-tested yet. |
| 12 - Bindings/state | PASS (Linux) | Native Go conversation/project bindings survived service restart, connector reconnect, independent multi-conversation bindings, and clean reboot. The installed migration build also imported a real legacy Rust conversation binding automatically: a new Go binding was created for `/home/whtvr/codexify-go` at 2026-09-29 15:40 local time after the old conversation was opened without manual workspace selection, while the legacy Rust binding records remained unchanged. |
| 13 - Plugin skills | PASS | Go discovered the installed plugin registry and successfully read a real plugin skill. With the gateway enabled it also generated and discovered the `cutover_gateway` skill. |
| 14 - Upstream MCP | PASS (Linux) | A separately supervised standalone Streamable HTTP MCP server was independently probed, then connected as a required Go gateway. A direct authenticated MCP client to the Codexify Go endpoint listed `cutover_gateway` among 29 tools and successfully called its `echo` function. Go also generated/discovered the gateway skill. A new ChatGPT conversation after connector Refresh exposed `cutover_gateway` in the model-visible tool surface and successfully called `echo`, confirming end-to-end upstream MCP operation through Codexify Go on Linux. Windows and macOS remain separately unverified. |
| 15 - Attachments | PASS | Real egress succeeded through `export_host_file`; the resulting native OpenAI file was then imported back through `import_host_file`. The imported 36-byte file retained the same SHA-256 and content. |
| 16 - Rust dependency elimination | PASS (Linux) | Rust is `inactive/disabled`. A clean Linux reboot automatically started only the enabled Go cutover service, which restored this conversation and exact managed worktree without manual startup. Post-reboot multi-conversation soak kept independent conversation bindings isolated, Git/SSH and normal MCP operations remained healthy, and the Go service stayed stable without restarts. The Go service owns its own binary, config, credential copy, and managed tunnel runtime; Rust files remain only for rollback. |

## Live process evidence

Immediately after cutover:

- Rust user service: `inactive`
- Go user service: `active`
- Go service executable: private cutover installation of the binary built from `9cf3514`
- Go service PID observed: `55004`
- Go-managed tunnel PID observed: `55035`
- tunnel runtime: Go-owned copy of `v0.0.12`
- MCP endpoint: loopback port `3300`
- the same ChatGPT conversation successfully invoked Go MCP after the switch

PIDs are observations for this run only and are expected to change after recovery tests.

## Defects found

### CUTOVER-001 - Rust config is not directly consumable by Go

**Severity:** P2 for the current controlled cutover; would become P1 if an in-place migration is expected to be automatic.

Rust 1.6.6 uses the current Rust configuration shape including `openaiTunnel`. Go 0.8.2-dev expects its own `tunnel` configuration shape and requires an explicit loopback `mcpServerUrl`.

For this run a separate Go config was created instead of mutating the Rust config. No infrastructure code was added because the cutover can proceed safely with an explicit migration step.

### CUTOVER-002 - Existing conversation/project binding is not migrated Rust -> Go

**Status:** FIXED AND LIVE-VERIFIED (Linux).

After Go took over the connector, the existing conversation remained connected but Go returned that no project was selected. Selecting `codexify-go` again restored project access.

This is important for the final acceptance requirement that an existing conversation continues correctly after Rust becomes unavailable.

Go now derives the exact legacy Rust conversation identity from the same `openai/session` metadata using Rust's `codexify/openai-session/v1` namespace when no Go-native binding exists. It reads only the exact matching legacy Rust v2 binding, validates the access root and referenced paths, persists a new Go-native binding under the Go identity, and leaves the Rust record unchanged. Existing Go bindings always take precedence; unsafe, stale, mismatched, or ambiguous legacy records are ignored rather than guessed. Focused tests cover successful import, unchanged Rust state, unsafe-root rejection, and Go-binding precedence. Live acceptance then succeeded with a real pre-existing Rust-bound conversation: opening it after installing the fixed Go build, without manually selecting a workspace, automatically restored `/home/whtvr/codexify-go` and created a new Go binding at 2026-09-29 15:40 local time; the legacy Rust records retained their earlier mtimes.

### CUTOVER-003 - Hot cutover exposes connector tool-schema drift

**Severity:** P1 candidate until explicit reconnect/schema refresh is tested.

The ChatGPT side initially retained tool definitions from the Rust connector. Two concrete mismatches were observed after Go takeover:

- an Rust-era optional `exec_command` argument was rejected by Go;
- `list_directory`, present in the Rust-era tool surface, returned `unknown tool` from Go.
- `git_commit`, present in the Rust-era tool surface, returned `unknown tool` from Go during the first real delivery attempt.

The Go `apply_patch` schema also differed from the Rust-era invocation shape. This may be resolved by an explicit connector schema refresh/reconnect, so no code change should be made until Phase 3/4 distinguishes stale client schema from a real Go parity gap.

The initial Phase 8 stdin check made this a concrete blocker before connector refresh: Go `exec_command` returned a string session id, while the stale ChatGPT-loaded `write_stdin` schema validated `session_id` as an integer before the call could reach Go.

After the user refreshed the connector tools, the Go schema loaded correctly and the same long-exec/stdin flow passed. This reclassifies the session-id mismatch as stale client schema during in-place implementation replacement rather than a Go exec/stdin runtime defect.

### CUTOVER-004 - Required upstream failure can cause an unbounded service restart loop

**Status:** FIXED AND LIVE-VERIFIED (Linux).

Two deliberately required upstream configurations were unavailable during startup. Go correctly failed startup because the upstream was marked required, but `Restart=on-failure` then retried indefinitely. The observed restart counter exceeded 100 attempts.

The first upstream, ChatGPT's bundled `node_repl`, was not a valid standalone test in the supplied environment. The second initial HTTP attempt also became unavailable because its temporary server was a child of the connector exec session. Neither is evidence that the Go gateway implementation itself is broken.

The corrected gateway test runs the standalone MCP server under a separate systemd user transient service. It was independently probed successfully before Codexify Go was restarted, and Codexify Go then connected to it successfully.

The first real Linux cold-boot attempt reproduced this defect under realistic startup ordering. The temporary `codexify-cutover-upstream.service` did not survive reboot, while the Go cutover config still marked `cutover_gateway` as required. Codexify Go therefore failed startup on connection refusal to `127.0.0.1:39091` and systemd repeatedly restarted it; `NRestarts` reached 25 during investigation. The test-only upstream was then removed from the cutover boot config, after which the Go service remained active with a stable PID and `NRestarts=0` and project discovery recovered.

The same boot also revealed that the preserved Rust `codexify.service` was still enabled and therefore started automatically after reboot. It was stopped and disabled again; the Go cutover service remains enabled. A second reboot is required for a clean Go-only cold-boot acceptance run.

The Linux service unit renderer was subsequently hardened without weakening `required` upstream semantics. Managed units now set `StartLimitIntervalSec=60s` and `StartLimitBurst=5` while retaining `Restart=on-failure` and `RestartSec=5s`. An unavailable required upstream therefore still makes startup fail, but systemd rate-limits repeated failures instead of restarting indefinitely. The regression test was observed failing before the change, then the focused service tests, full `go test ./...`, and `go build ./cmd/codexify-go` passed. The fixed build was installed into the live Go-only cutover service and the connector recovered with the same exact managed worktree and `NRestarts=0`. A separate temporary systemd probe using the same start-limit policy failed repeatedly and stopped at `NRestarts=5` with `Start request repeated too quickly`, confirming the installed Linux systemd policy prevents an unbounded restart loop without disrupting the live connector.

### CUTOVER-005 - Existing conversation retained a stale gateway tool snapshot

**Status:** RESOLVED / client conversation snapshot limitation.

With a valid independently supervised Streamable HTTP upstream, Codexify Go starts successfully, connects the required upstream, and generates the `cutover_gateway` skill. Skill discovery increases from 38 to 39 entries and `skills_read` returns the generated gateway contract.

After an explicit ChatGPT connector Refresh, the connector tool surface still does not contain `cutover_gateway`. Therefore the generated skill describes a gateway function that ChatGPT cannot invoke through the connector.

This localizes the remaining Phase 14 failure to dynamic gateway tool exposure / connector schema publication rather than upstream transport or gateway discovery.

A direct authenticated MCP probe against the running Codexify Go endpoint confirmed that the server itself exposes 29 tools including `cutover_gateway`. Calling `cutover_gateway` with upstream function `echo` returned `gateway works through codexify-go`. The runtime gateway path therefore works end-to-end below ChatGPT connector publication.

The same direct tool list contains three intentional app/private tools that are also absent from the model-visible ChatGPT surface. Excluding those expected private tools, `cutover_gateway` is the only server-side tool missing from the current conversation's model-visible tool list.

The required retest was completed in a **new ChatGPT conversation after connector Refresh**. The new conversation exposed `cutover_gateway`, discovered/read its generated skill, and successfully invoked `echo` with the response `gateway works through codexify-go` while continuing through the Go connector. Rust was not started for this retest.

This confirms that the earlier missing gateway tool was caused by the existing conversation retaining a stale tool/schema snapshot. Dynamic gateway publication and invocation work through Codexify Go for a newly established conversation, so no Go code change is required for CUTOVER-005.

This resolution is **Linux-specific evidence**. The equivalent upstream MCP gateway publication/invocation flow still requires independent verification on Windows and macOS.

## Recovery evidence

### Tunnel crash recovery

The original Go-managed tunnel PID observed after cutover was `55035`. It was terminated unexpectedly during the live connector session. The active tool call lost transport and timed out, then the Go supervisor started a replacement tunnel process (PID `55619` in this run). A subsequent MCP command succeeded through the same ChatGPT connector while the Rust service remained inactive.

This satisfies the core Phase 9 recovery behavior. PID values are evidence for this run only.

## Future UX follow-up

### Scratch-contained repositories across conversations

Post-reboot multi-conversation soak confirmed that separate scratch bindings remain isolated and persistent. One conversation owned scratch root `/home/whtvr/.codexify-go/scratch/af4903e8079a`, containing the Git repository `mafia-party`, while another conversation was independently bound to `/home/whtvr/.codexify-go/scratch/bfd82b1af946`.

The repository was intact, but discovering or resuming a Git repository nested inside another conversation's scratch root from a different/new conversation is awkward: it is not naturally surfaced as an ordinary selectable project, and the user-facing recovery guidance can misleadingly suggest simply selecting the nested repository in a new context. Consider a future project-discovery/resume UX that can safely surface nested Git repositories from preserved scratch workspaces, or provide an explicit cross-conversation "resume existing workspace/repository" flow without weakening conversation binding isolation.

This is a usability follow-up, not a Linux Rust -> Go replacement blocker; the underlying scratch data and independent persistent bindings were preserved correctly.

## Rollback

The rollback remains intentionally local and private. The pre-cutover snapshot contains the Rust 1.6.6 executable, its configuration, and its systemd user unit. To roll back operationally, stop the Go cutover service and start the preserved Rust `codexify.service`; use the snapshot only if the installed Rust files themselves have been changed.

Do not commit the snapshot, tunnel credential, connector token, or machine-specific secret material.

## Next gates

1. Perform explicit connector disconnect/reconnect and verify old/new conversation behavior.
2. Exercise simultaneous independent conversation bindings to multiple projects and create/switch/resume managed worktrees through the interactive project-switch flow.
3. Exercise a real self-update apply when a release newer than the installed build exists; the current `0.8.2-dev` build is ahead of latest published `0.8.1`.
4. Reboot with Rust still inactive and verify automatic Go service/tunnel recovery plus binding persistence.
5. Continue several normal sessions with Rust unavailable before declaring Linux replacement complete.
