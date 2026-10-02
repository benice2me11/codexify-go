# Windows G04: tools, exec and state integrity

G04 is PASS for candidate `20261001T160710Z-node-repl-compat`. The 36 hosted
Windows MCP calls made between 16:20:30Z and 16:29:29Z on 2026-10-02 each have
one matching server start and end. All selected operations returned the expected
result, including four deliberate tool errors. Overall deployment remains
`GO_TRIAL`; G05-G11 have not been passed.

The [machine-readable evidence](evidence/WINDOWS_G04_TOOL_STATE_20261002.json)
records operation labels, trace joins, timings, asset hashes and evidence paths.
Requirements come from the
[acceptance plan](WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md#g04---everyday-tools-exec-and-state-integrity).
The prior [G03 checkpoint](WINDOWS_G03_GIT_CHECKPOINT_20261002.md) proves the
candidate source and local Git history.

## Scope and ownership

The operator authorized continuation and confirmed that other Windows MCP
conversations/automations were paused and old widgets closed. Initial preflight
found the Manual Go service stopped after host shutdown/reboot. This run started
the same frozen candidate and later performed one planned same-profile SCM
restart; it did not validate automatic boot or fault recovery.

Repository changes use the separate worktree:

```text
C:\Users\FoxOS_User\codexify-go\.codexify-go\worktrees\g03-checkpoint-20261002
branch: codex/windows-cutover-g03
base: 3bbf9c7d6f5b8ef6e3ae24cd5a183a9eb3be27af
```

The hosted conversation is still bound to `C:\Users\FoxOS_User\codexify-go`.
File and command tools reached the fixture through the explicit worktree-relative
path. The dedicated `git_status` and staged `git_diff` tools read the bound root;
the fixture's actual Git status and nonempty diff were separately verified by a
hosted `exec_command` with the worktree as its working directory. This distinction
does not establish G05 worktree isolation.

No production source, executable, declared profile or frozen v3 operator script
changed. No push, merge, autostart change or Rust fallback was performed.

## Results

| Check | Observed result |
| --- | --- |
| Create/read/patch | Exact UTF-8 content, including Cyrillic text, matched before and after patch; `glob` and `grep` found the expected file and sentinel. |
| Git | Hosted structured status/diff returned the expected root state. Hosted worktree command showed only the disposable file and its exact added lines. |
| Mutation once | One append request produced exactly one marker line. It remained single after restart. No interrupted write was replayed. |
| Delayed exec | Returned a session ID; polling that ID returned `G04:READY`; stdin returned the exact nonce and exit code `7`. |
| Cancellation | The valid second fixture's parent disappeared within 0.809 s and child within 0.827 s of the client request, below the 10 s limit. Server-start bounds were 0.154 s and 0.171 s. |
| Expected tool errors | Finished exec session, missing fixture file, invalid patch context and duplicate memory key each returned one `INVALID_ARGUMENT` result. |
| Authentication error | One unauthenticated local MCP request returned HTTP 401. It is a local probe, separate from the 36 hosted tool calls. No credential value was read or logged by the probe. |
| Brief and installed skill | `get_agent_brief`, `skills_list` and a complete `computer-use:computer-use` skill read succeeded. |
| State after restart | Workspace binding and recalled note were identical. The Go memory file hash, patched file bytes and one-line marker survived the planned SCM restart. |
| Cleanup | Removed only the test note and two path/hash-validated fixture files; removed their temporary Git intent-to-add entry. The worktree was clean before report creation. |

The source checkout's existing dirty status was unchanged. The test did not
write project code there or modify the older managed worktree.

## Evidence and capture boundaries

Private evidence is retained under:

```text
C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\evidence\g04-live-20261002T155913Z
```

The first capture began at 16:20:24.593Z and contains 127 events. Its worker was
stopped by the planned SCM restart before the 10-minute budget; it has no terminal
capture event. The second began at 16:28:16.393Z and contains all seven
post-restart hosted calls, with the last test response at 16:29:29.117Z. Its
10-minute budget ends at 16:38:16.393Z. Snapshot hashes and the observed terminal
state are recorded in the JSON; idle time after that budget is not decoded
request coverage.

Across both captures, all 36 hosted calls have unique client-time/tool-name joins
and complete server call pairs. Four expected errors match both the client
response and server `tool_is_error` flag. No extra tool call is unexplained.
The largest observed 10-second window contains four calls; the largest repeated
operation-fingerprint count in such a window is one. Each capture contains one
hosted conversation hash; hashes are not compared across captures.

The independent local sampler observed zero Rust owners and at most the two
expected Go processes and one tunnel. Every guard decision through the recorded
cutoff was `CONTINUE`. The planned restart changed supervisor PID from `19852`
to `2432`; its worker `15048` and tunnel `21544` are siblings owned by that
supervisor. The final recorded service is Running/Manual, all eleven Codexify
scheduled tasks remain disabled, and the four frozen asset hashes match.

## Preparation and measurement corrections

A fresh 51-file Rust recovery snapshot preserves the current Rust state. Its two
task definitions intentionally use the previously verified enabled pretrial
definitions, while the actually captured disabled definitions are separately
retained. Snapshot validation and the eight-action recovery dry-run passed.
No real rollback was needed in G04.

The private G04 controller and rollback wrapper serialize recovery with a named
mutex. Review caught an absent-watcher cleanup failure, a potential overlapping
rollback, and insufficient tunnel ownership validation before execution. The
corrected helper validates the candidate's supervisor/worker/tunnel topology.
Readiness negative cases and actual wrapper mutex blocking/release were tested
without running a rollback. The frozen v3 scripts were not edited.

The first cancellation's local observer mishandled a PowerShell array and did
not produce a valid timing measurement. Both processes were subsequently found
absent. A fresh second fixture, with identities verified alive before observing,
provided the reported timing; the first attempt is not counted as timing proof.
Two local ledger-save failures were repaired from already returned responses,
without repeating their hosted tool calls. Original runtime traces, returned
results, observer error and retained fixture bytes remain outside Git.

## Acceptance limits

The existing Rust memory has zero durable notes and a six-step plan. The known
`update_plan` and legacy-plan visibility gap from G03 remains open before final
acceptance; this test does not equate a new Go note with migration of that plan.

G04 proves the selected everyday tools, fixture side effects and same-profile
restart persistence. It does not prove multi-project isolation, service fault
recovery, cold boot, long observation or final acceptance. Those remain G05-G11.
The capture profile and Manual startup mode are retained. The local guard remains
armed; its decoded traffic input is bounded by the capture window. The passive
sampler was started for at most 120 minutes, so this is not a 24-hour soak result.
