# Windows G03 client metadata and conversation checks

> Historical phase record, versioned after G03. Ownership, process identities
> and verification results below describe that phase, not current runtime health.

Status: **G03 PASS - schemas, two ChatGPT conversations and visible widget confirmed**.
This report is not full cutover acceptance. G00/G01/G02 remain PASS; G04-G11
have not been executed by this phase.

## Runtime and observation

- Candidate: 20261001T160710Z-node-repl-compat, unchanged from
  [G02](WINDOWS_G02_LIVE_REPORT_20261002.md).
- Go executable SHA256:
  61E2A086445A1B10E7537E720119B0855E17515A58A5B3F8BDCA914075B07DD6.
- Capture profile SHA256:
  BCDF7B1362047E957F21B55704D69EC3783BD189B9D79759980499613D9A197F.
- A planned local restart of this same candidate renewed the bounded trace.
  The old watcher was identity-checked and stopped; all old service children
  exited; a new armed watcher observed zero owners before Go started.
- New Go supervisor 16800, interactive worker 13704 (FOXOS\FoxOS_User,
  session 1), tunnel 4268. Rust absent; its two scheduled tasks remain disabled.
  CodexifyGo remains Running / Manual. New armed watcher: 21692.
- Decoded capture started 2026-10-02T02:13:46.2130124Z, with a 10-minute limit
  ending at 02:23:46.2130124Z. Capture cannot certify subsequent idle time.
- Read-only local sampling restarted at 02:01:20Z for at most 120 minutes.
  The previous sampler's last sample was 01:54:52.8935968Z; the intervening
  sampling gap is retained explicitly. The independent old watcher continued
  across that gap until the planned replacement.

Private evidence directory:

C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\evidence\g03-metadata-20261002T020120Z

## Actual served metadata

Six authenticated loopback reads after the planned restart returned HTTP 200:
tools/list, resources/list, and four resources/read calls. These are metadata
evidence, not substitutes for hosted conversations. No local server/discover
was used by the metadata collector to mark the connector refreshed.

The live catalog contains 32 tools and exactly matches the frozen G00 catalog.
Canonical JSON SHA256:
086B18C31031F7DA12CF9AEAB956C03B30178122CB263773781FB76965F2C3D5.

Observed schemas include:

| Tool | Input fields | Key result fields |
| --- | --- | --- |
| get_environment | none | workspaceRoot, accessRoot, bindingScope, platform, defaultShell, username, managedWorktree |
| apply_patch | input | content |
| exec_command | cmd, shell, workdir, yield_time_ms | output, running, session_id, exit_code |
| read_file | path, offset, limit | content |
| set_project_root | path, withoutProject, createWorktree, resumePath | workspace information |
| show_diff | since, advance, include_patch | content, summary, files, checkpointAdvanced |

All four served resources equal their raw Go string literals in the frozen
source snapshot:

| Resource | UTF-8 bytes | SHA256 |
| --- | ---: | --- |
| ui://codexify-go/diff/v1/mcp-app.html | 769 | 24F40B215F3807AAB6D9FE30C2AC399D1B13203355AF5D876695FD24AE58FBC6 |
| ui://codexify-go/markdown-chat/v2/mcp-app.html | 2824 | 5DE1550794C89789D608D22454AC15F59D53F91962E757B87DAEC0457EFBB25C |
| ui://codexify-go/self-update/v2/mcp-app.html | 2345 | ED7C534E412BBF475994C9724441F0433FAA4EE722EB764356FD856B3C302C4C |
| ui://codexify-go/setup/v2/mcp-app.html | 4302 | E95DA94ECA6F86BFA269EA55A45615384F52C2D7CEC2EC650EBCBA052BE093E2 |

G00 evidence correction: the old self-update response decoded UTF-8 ellipses
as Latin-1 before re-encoding, producing 2351 bytes and a different hash.
Reversing that exact encoding transformation recovers the live text, which
also equals the frozen source. The original manifest and response remain
untouched; g00-resource-encoding-erratum.json records the correction.
This corrects evidence and does not change the deployed candidate.

## Hosted conversations

The operator reported Refresh completed in their own browser. Browser-side
inspection was unavailable: the in-app browser needed login, and Computer Use
stopped Firefox access because browser URL policy enforcement was unsupported.
The operator chose to perform the two ChatGPT tests manually.

Server evidence through 02:20:04Z distinguishes three conversation hashes:

| Conversation | Observed tools/call results |
| --- | --- |
| Current Codex task, excluded from the two-chat requirement | 2 successful operator checks |
| First manual ChatGPT test | get_environment, get_agent_brief, read_file: 3 successful calls |
| Second manual ChatGPT test | list_projects, set_project_root, get_agent_brief, get_environment, read_file, git_status: 6 successful calls; 4 additional successful widget setup/list calls |

The test conversation hashes differ. All 24 decoded MCP calls in this interval
have matching completion records. There were no tool errors, guard HOLD/ABORT
decisions, overlapping Rust/Go owners, or failed local samples at this cutoff.
The operator subsequently supplied both real ChatGPT outputs. Both report the
exact intended workspace, the expected module line, no tool/binding errors,
and the current Go schemas: apply_patch uses input; exec_command exposes cmd,
shell, workdir and yield_time_ms. The existing conversation reports no binding
change. The new conversation used selector codexify-go returned by its own
list_projects call with createWorktree=false. The similarly named Documents
checkout was not selected. These observations match the successful call traces.

Both brief results reported two plugin-discovery warnings about missing usable
skill roots in codex-app-tools and unified-computer-use. The required reads still
completed. This phase does not alter those unrelated plugin manifests.

The new-chat model explicitly said it could not see the ChatGPT UI. Its statement
that no separate widget was returned in the tool payload is not proof that a
widget was absent on screen. The operator later reported that there appeared
to be two widgets. This matches both list_projects and set_project_root
advertising the setup resource, and two recorded resource loads. Correct
on-screen project contents and absence of visible errors were subsequently
confirmed by the operator. This is operator visual evidence; detailed UX and
feedback-loop checks remain G06.

One HTTP 401 was the deliberate unauthenticated local MCP probe. Startup HTTP
415 and 400 responses are separately retained without a hosted conversation
identity; they are not counted as ordinary-call failures. Their exact request
payloads were not captured. No claim that every HTTP response was successful
is made.

## Compatibility and authentication

The Go catalog does not implement update_plan. Refresh removes an obsolete
advertisement only when the client actually obtains the current catalog; it
does not add the missing tool. This phase uses repository Markdown and a private
JSON execution ledger. Equivalence with the former persisted Rust plan workflow
has not been accepted or proven. The operator did not make a compatibility
waiver when asked about the unfamiliar internal tool. G03 itself uses the
existing repository plan; the capability difference remains open for final
acceptance and does not replace a mandatory G03 check.

Compared with this Codex task's pre-refresh declarations, the live catalog also
omits clock_curr_time, clock_sleep, git_commit, git_push, list_directory,
self_update, tree and view_image, and adds git_diff, list_worktrees,
self_update_status, setup_status and setup_ui_switch_project. These are tool
inventory differences; they do not by themselves prove that a user workflow is
impossible. Any required capability or agreed exclusion must be settled before
final acceptance under the [acceptance plan](WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md).

The frozen server uses an internal Bearer credential injected by its supervisor.
Both protected-resource metadata paths return plain-text HTTP 404; the MCP
endpoint without authorization returns 401; authenticated metadata and hosted
tool calls succeed. This accounts for the startup OAuth metadata parse warning
on this profile. No OAuth user-login flow is implemented or validated here.
The warning remains recorded; authentication settings were not changed.

## Collector corrections and remaining evidence

The first manual metadata requests lacked required 2026-07-28 metadata/headers
and were rejected; the raw errors remain preserved. A later Windows PowerShell
collector saved all six responses but consumed CPU/memory during deep schema
serialization. Its exact process was stopped, not the service. Collector v2
completed successfully and delegates schema comparison to Python. The initial
responses, failed attempt ledgers and process-stop record are retained.

Before final cutover acceptance, settle the planning capability gap without
silently treating a different workflow as equivalent.

G03 decision: PASS. Actual served metadata matches the frozen source/catalog;
both operator-run ChatGPT conversations completed at least three ordinary calls
using their own bindings/selector; both model outputs report current Go schemas
and the correct workspace; the operator confirmed the visible widget.
Final local verification at 02:26:53Z confirmed unchanged binary/profile/tunnel
hashes, two expected Go processes, one tunnel, zero Rust processes, HTTP 200,
disabled Rust tasks and fresh independent observation.

The next gate is G04: everyday tools, delayed exec/stdin/cancellation, state
integrity and controlled fixture mutations. Overall deployment remains GO_TRIAL.

No production source was edited, rebuilt, committed or pushed by G03.

Official refresh procedure:
[Connect and test your plugin](https://developers.openai.com/plugins/deploy/connect-chatgpt).
