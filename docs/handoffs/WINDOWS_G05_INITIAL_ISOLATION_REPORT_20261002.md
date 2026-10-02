# Windows G05: initial isolation and reconnect checks

Status: **G05 BLOCKED by widget selection**. Initial selection, separate workspaces and same-path
reconnect checks passed on the frozen candidate. The actual Switch UI check,
post-G08/G09 repetitions and verified fixture cleanup remain outstanding.

This continues the [G05 preparation](WINDOWS_G05_ISOLATION_PREPARATION_20261002.md)
after the operator explicitly requested two Codex test tasks and confirmed that
other Windows MCP conversations and automations were paused. Both tasks used
the real Codexify-Win connector with separate conversation identities. Their
local Codex output directories were not used as substitutes for MCP workspaces.

## Observed results

| Check | Result |
| --- | --- |
| Real project discovery | Both clients received the two prepared fixture repositories. |
| Client A | Created one managed worktree from fixture A; marker and actual shell directory matched A. |
| Client B | Selected fixture B without a worktree; marker and actual shell directory matched B. |
| Identity separation | A and B had different conversation hashes within each capture. Hashes were not compared across captures. |
| Worktree inspection | A retained baseline commit `20a9b7ba8e631105d1a61b3694c9bd56f094d080`; source and managed worktrees were clean. |
| Planned SCM restart | Old owners exited; one new supervisor, worker and owned tunnel became ready. |
| Exact-path resume | A resumed the same worktree using the `\\?\C:\...` spelling; `newlySelected=false`, `managedWorktree=true`. |
| State after reconnect | Both binding files were byte-identical; both clients read their original markers and executed in their original directories. |

Actual workspace A:

```text
C:\Users\FoxOS_User\.codexify-go\worktrees\project-a\6f2ac39290dc-401fc790
```

Actual workspace B:

```text
C:\Users\FoxOS_User\codexify-g05-fixtures-20261002T174437Z\project-b
```

The model workload produced 23 successful client responses, with no
tool errors. Twenty-two responses were joined to unique decoded request/end
pairs and checked against the actual returned paths, markers and saved bindings.
One stdin-completion call occurred after the first capture expired. Its actual
response and process exit are retained, but it is not decoded request evidence
and is not used to pass the Switch check. No fixture shell remains running.

Four additional decoded UI calls occurred as two `setup_status` + `list_projects`
pairs, each following a resource read. These are recorded separately from model
calls; no `setup_ui_switch_project` call occurred. This does not pass G06.

The two successful capture windows started at 18:27:13Z and 18:41:07Z on
2026-10-02, each with a 600-second limit. A's first post-restart response took
48.371 seconds from client invocation to completion; it was one call, without
retry. This report does not attribute that delay to server execution or the
hosted transport. Both capture deadlines have now passed; the next live checks
need a fresh controlled capture.

## Operator startup correction

An earlier start was rolled back automatically before either test task called
MCP. The controller was mistakenly invoked under PowerShell 7.5 rather than
the validated Windows PowerShell 5.1 host. `ConvertFrom-Json` converted the UTC
timestamp to a DateTime; casting it to string dropped its timezone and fraction.
The controller then treated a fresh capture as approximately three hours old.
The same retained header reproduced a -10800.3874518-second error under 7.5 and
zero error under 5.1. The trace itself was present and fresh.

Rust recovery completed successfully. All 51 recovery files validated and all
48 retained live Rust files still matched. Re-entry used a separate evidence
directory, Windows PowerShell 5.1 and the same candidate/profile. A read-only
review checked the narrowly scoped handoff helper and its recovery ownership.
The preparation test was adjusted to compare the actual pre/post service state
because successful recovery had set Go to Disabled; its earlier Manual-state
assumption and failing run were retained. Production code and frozen v3 tooling
were not changed.

At 18:52:45Z, Go was Running/Manual: supervisor 19672, worker 24324 and one
owned tunnel 24172, with readiness HTTP 200. Rust owners: zero. All eleven
Codexify scheduled tasks were disabled. The guard reported CONTINUE and all
four frozen asset hashes matched. These observations do not establish G08/G09.

## Remaining work

### UI attempt and diagnostic boundary, 2026-10-02

The operator reported that **Select** on fixture B produced no visible result.
The new UI capture covered 19:16:58Z-19:26:58Z; the ordinary tunnel log records
forwarded requests at 19:29:22Z-19:30:11Z, after that capture deadline. Those
ordinary records do not identify the tools or their returned MCP errors. No
Switch/Select request was decoded inside the capture, and both original
conversation binding files remained unchanged. The B stdin fixture was closed
at 19:32:05Z with exit code zero and its original directory/marker; this cleanup
was outside decoded coverage and does not pass the switching check.

An offline probe executed the unchanged embedded SetupHTML with a synthetic
MCP `isError=true` selection response. It reproduced an empty error area and
two follow-up status/catalog calls: the current widget ignores that error
response. This proves an error-reporting defect. The frozen candidate is unchanged.

A fresh capture started at 19:37:09Z. Its retained snapshot contains two
`set_project_root` failures at 19:39:39Z and 19:39:58Z, both with
`tool_is_error=true`, protocol `2026-07-28`, and neither a conversation hash nor
a transport-session hash. A model-origin `get_environment` at 19:40:23Z has a
conversation hash and succeeds; client A confirms its original managed
worktree. The operator reports animation after Select without a project change.
Both saved A/B binding hashes still match their pre-UI values.

Missing conversation identity explains the rejection in the unchanged candidate:
this protocol uses the stateless route without a transport-session header,
`requestMeta` therefore has no identity fallback, and `projects.Select` rejects
selection in multi-project mode. The diagnostic trace does not retain the
actual error message; this explanation follows the traced metadata and source
path. The point where the UI request loses its identity upstream is not yet
established. OpenAI's [reference](https://developers.openai.com/plugins/reference#_meta-fields-the-client-provides)
documents `openai/session` as the conversation identifier for tool calls.

The source-only widget fix now displays an MCP `isError` response, preserves
the displayed workspace, re-enables controls and skips follow-up reads after
failed selection. The regression first failed on hidden error text, then passed
for text blocks, string content and empty-content fallback; `go test ./... -count=1`
also passed. No binary was built or deployed for this fix. It does not supply
the missing conversation identity or pass the live switching requirement.

Private probe evidence is retained under the existing G05 retry evidence
directory's `ui-restart` phase; `ui-diagnosis` retains the fresh trace snapshot,
diagnosis and red/green/full-suite outputs. Restore real UI conversation context
before repeating the isolation scenario; do not repeat already passed gates.

### Outstanding acceptance

The next step needs the actual **Switch project** button in client A while a
new B stdin session remains live, followed by A's return to the recorded exact
worktree and verification of B's unchanged binding and session. The available
native window identified itself as ChatGPT.exe; the current Computer Use skill
prohibits automating that app UI. No automated UI input or direct model/HTTP
call to the app-only switch tool was used. Manual UI readiness is still needed.

Retain both fixture repositories and A's worktree for the required post-G08/G09
checks. Cleanup must follow the original fixture/path/cleanliness safeguards.
G00-G04 retain their historical PASS records. G05 is not PASS, and the overall
deployment is still GO_TRIAL.

The [evidence manifest](evidence/WINDOWS_G05_INITIAL_ISOLATION_20261002.json)
identifies the private results, capture snapshots and helper hashes. Raw tool
responses, bindings, traces and recovery assets remain outside Git. This report
is a local thematic checkpoint; no push is part of this step.
