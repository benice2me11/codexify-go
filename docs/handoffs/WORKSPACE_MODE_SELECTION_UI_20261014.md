# Codexify Go — explicit Source checkout / Managed worktree selection

**Date:** 2026-10-14 (Europe/Moscow)  
**Status:** READY_FOR_IMPLEMENTATION — research and implementation handoff only. **Feature is not implemented or deployed.**  
**Repository:** https://github.com/benice2me11/codexify-go  
**Source baseline inspected:** main @ eb784777f229140fabd92e44576f317bdb0d9572  
**Priority:** P1 usability / operational correctness. Independently unblock HH deployment G01 as a P0 job-search prerequisite.

## 1. User-facing issue and reproduced case

The user chooses the Windows project **hh-mcp-server** in Codexify Go Setup UI, expecting the actual checkout at:

~~~text
C:\Users\FoxOS_User\hh-mcp-server
~~~

The connector instead binds the ChatGPT conversation to an isolated managed worktree:

~~~text
C:\Users\FoxOS_User\.codexify-go\worktrees\hh-mcp-server\17b58d069d8f-bc4d1844
~~~

A previous selection similarly created ...\17b58d069d8f-18d8fcdc. A repeated project selection creates another worktree under the configured **worktrees.mode=auto** policy when the source project is already in use. This is not a Git merge problem.

The *running* Windows HH MCP uses the source checkout and its virtual environment:

~~~text
C:\Users\FoxOS_User\hh-mcp-server\.venv\Scripts\python.exe -m hh_mcp_server
~~~

Thus, editing/updating only a managed worktree does not update the loaded HH MCP code. Re-selecting the project with the old UI cannot reliably choose the source checkout.

## 2. Verified root cause — exact existing implementation

1. **internal/ui/resources.go**, embedded **SetupHTML**:
   - renderProjects() displays a project and a generic **Select** button.
   - selectProject(path) calls **set_project_root({path})**, omitting **createWorktree**.
   - Current status only adds **(worktree)** when managedWorktree is true; no explicit choice before selection.
   - Existing **Switch project** button invokes **setup_ui_switch_project({expectedPath})** and reloads, but the next Select still omits the preference.

2. **internal/projects/manager.go**, **Manager.Select(meta, selector, createWorktree *bool)**:
   - Explicit bool is already supported. **false** means source checkout; **true** requests a managed worktree.
   - When the argument is nil and config mode is **auto**, the selection uses **hasGit && m.sourceInUse(source, identity)**.
   - When the conversation already has a binding for the same source, Select returns that immutable binding *before evaluating createWorktree*. Calling set_project_root with false **after selection** therefore cannot change an existing worktree binding.

3. **internal/mcpserver/server.go**:
   - **SetProjectRootInput.CreateWorktree *bool** and handler already forward to Manager.Select.
   - **setup_ui_switch_project** is **app-only**; the model-facing catalog does not expose it as a normal tool.
   - **setup_status** provides current workspace; **list_projects** provides candidates.
   - Existing backend switching archives the old binding without deleting workspace files/worktrees.

4. **internal/projects/bindings.go**, **sourceInUse** reads other bindings to select isolation automatically. Explicitly forcing a source checkout may allow multiple conversations to share a checkout; this is a deliberate concurrency risk that must be surfaced in UI.

5. The README already documents **auto / always / never** and **createWorktree true/false**. The missing piece is the Setup UI's ability to choose the parameter **before** creating a binding, and to switch explicitly from an existing mode.

**Conclusion:** First deliver a focused Setup UI fix, not a global change to **mcp.worktrees.mode=never**, not a Git reset, and not another HH patch. Backend enhancements should be minimal and guard correctness.

## 3. Desired UX and API contract

Within the existing Setup UI (MCP App), expose a compact selection control before choosing a project:

| Label | Meaning | set_project_root input |
| --- | --- | --- |
| **Auto (default)** | Preserve configured auto/always/never policy | **{path: selector}** — omit createWorktree |
| **Source checkout** | Use the actual discovered repository directory | **{path: selector, createWorktree: false}** |
| **Managed worktree** | Create an isolated Git worktree | **{path: selector, createWorktree: true}** |

The two explicit choices must be visually discoverable. Keep **Auto** as the backwards-compatible default; do not silently alter existing users' behavior or global configuration.

### Initial selection

- Show the project name, selector, Git/non-Git distinction if available, and current selection preference.
- On clicking **Select**, pass the *boolean* true/false, not the strings "true"/"false". Omit the property in Auto mode.
- After the tool returns, verify/display **sourceProjectRoot**, **projectRoot**, **managedWorktree**, and the effective status. **worktreeMode** denotes the configured policy; **managedWorktree** denotes the actual bound outcome.
- If an explicit preference is not honored, show an error or actionable mismatch instead of misleading success.

### Already-selected conversation / switching mode

- Current bindings are intentionally immutable. Selecting the same project again must not pretend to change the mode.
- Offer **Change workspace / Change workspace type**, available even if the target is the *same* project.
- On explicit user action, call **setup_ui_switch_project({expectedPath: current.projectRoot})**. Then render the chooser and issue a new **set_project_root({path, createWorktree})** on the user's subsequent selection.
- Show that the *previous* workspace remains on disk, including its uncommitted changes; **do not automatically migrate, stash, reset, delete, merge, or clean** its files.
- If reselection fails after switching, show how to restore the previous **saved workspace** through the supported resume flow; never fabricate a resumePath or silently bind to a different checkout.
- After a successful change, display the actual path prominently and instruct the agent to re-read **get_agent_brief** for the new workspace.

### Safety and special cases

- **Source checkout** should visibly warn: "Shared checkout; other conversations or local services may use or modify these files. Concurrent writes can conflict." Require an explicit action, not an automatic switch.
- **Managed worktree** requires a Git repository. For non-Git projects disable this option with a clear explanation or propagate the supported backend validation error.
- If a URL targets a branch/commit not checked out in the source repository, existing backend rules may require an isolated worktree. **createWorktree=false must fail explicitly**; never move/reset the source checkout to satisfy a URL request.
- **Scratch** does not take a worktree preference. **resumePath** cannot be combined with a preference, per existing handler validation.
- No initial Git checkout, pull, update, Python process kill or upstream MCP restart should be triggered *by merely selecting a workspace type*.
- No cross-project file access or global configuration rewrite; honor workspace confinement.
- Do not expose other conversation identities, secrets, token paths or the contents of their bindings. A simple general collision warning is enough for MVP.

## 4. Implementation tasks (smallest viable diff)

### UI — P0 for this feature
**File:** internal/ui/resources.go (SetupHTML inline HTML/CSS/JS).

1. Add a small **Workspace type** radio/select: Auto / Source checkout / Managed worktree.
2. Track choice as local UI state (e.g. 'auto' | 'source' | 'managed').
3. Modify **selectProject(path)** so its call arguments are assembled as:

~~~javascript
const args = { path };
if (selectionMode === 'source') args.createWorktree = false;
if (selectionMode === 'managed') args.createWorktree = true;
await call('set_project_root', args);
~~~

4. Do not add the property in Auto mode; the backend must receive nil, not false.
5. Render post-selection identity using actual **managedWorktree** and both source and active paths. Disable selection while busy.
6. Support explicit **Switch project / Change workspace type** in the same chat, including returning to the same selector with a different mode. Preserve the user's choice through the switch flow where appropriate.
7. Preserve the existing host-globals hygiene: passive **openai:set_globals** events must never cause tool mutations or repeated loads.

### Backend — optional, only if the UI needs stronger guarantees
**Files:** internal/projects/manager.go and tests.

- Consider making a **re-selection with a conflicting explicit createWorktree** return an actionable immutable-binding error, instead of silently returning the previous mode. Preserve idempotent re-selection when the mode matches.
- Consider a read-only project selection preview or **sourceInUse** hint, but **not required for MVP**. Do not expose other chat identities.
- Keep the existing isolation and targeted-commit checkout rules intact.
- Do not globally flip **worktrees.mode**, delete existing worktrees, or make workspace rebinding automatic.

### Documentation
**Files:** README.md and, if applicable, setup UI user guidance.
- Document Auto vs Source checkout vs Managed worktree, concurrency warning, difference between current project and running upstream process, switching without losing files.
- Clarify that **createWorktree=false passed only after the conversation is already bound does not change its mode**.

## 5. Tests and acceptance

Update/reuse:
- **internal/ui/widget_runtime_test.mjs** — actual SetupHTML extracted and run in the offline host-event harness.
- **internal/ui/resources_test.go** — Go test that runs Node widget harness.
- **internal/projects/projects_test.go** — Manager.Select mode and lifecycle tests.
- **internal/mcpserver/server_test.go** — integration/API contract where valuable.

Test matrix:

| ID | Scenario | Required result |
| --- | --- | --- |
| T01 | Auto, source unused | Source checkout, no new worktree |
| T02 | Auto, source used elsewhere | Managed worktree (current behavior preserved) |
| T03 | Source checkout, source used elsewhere | **createWorktree=false** passed, source path selected, warning shown |
| T04 | Managed worktree, source unused | **createWorktree=true** passed, new isolated checkout |
| T05 | Switch same project worktree → source | Explicit switch, new binding uses source; old worktree retained |
| T06 | Switch same project source → worktree | New managed worktree; source unchanged |
| T07 | Re-select with conflicting mode but no switch | No silent misrepresentation; mode unchanged, actionable guidance |
| T08 | Non-Git + Managed | Disabled or clear validation error; no arbitrary directory created |
| T09 | Pinned Git ref mismatch + Source | Fail closed, no source branch reset |
| T10 | Existing uncommitted changes + switch | Files unchanged in previous workspace |
| T11 | Rapid clicks / repeated host global events | No duplicate mutations, no call amplification |
| T12 | Failed selection / reconnect / restored binding | Accurate status, recoverability, no data loss |
| T13 | Windows path canonicalization | Handles C:\ paths and extended-length path identities correctly |

Run:
~~~text
go test ./internal/projects ./internal/ui ./internal/mcpserver
go test ./...
go vet ./...
node internal/ui/widget_runtime_test.mjs --source internal/ui/resources.go
~~~

Note: the Node harness may need a small fixture update to simulate mode input, app-only switching, and same-project re-selection. Do not make tests pass by mocking the key argument checks away.

**Live Windows acceptance (mandatory after unit tests):**

1. Explicitly select **hh-mcp-server → Source checkout** in a fresh conversation; verify **get_environment.workspaceRoot = C:\Users\FoxOS_User\hh-mcp-server**, **managedWorktree=false** and no new worktree.
2. In the same conversation use **Change workspace type → Managed worktree**; verify new managed root and that source files remain unchanged.
3. Switch back to **Source checkout**; verify exact original path, no need to create another chat.
4. Preserve session/conversation identity and test reconnect/restart. No duplicate MCP transport bursts, background operations or side-effecting calls.
5. Verify **list_worktrees**, **get_agent_brief**, and repo-scoped command confinement after every switch.
6. Show a real end-to-end result, not only a Go test fixture. If runtime cannot be updated safely, record BLOCKED instead of PASS.

## 6. Safe development and rollout procedure

1. Select the separate **codexify-go** repository, read its applicable project instructions, inspect Git status, remotes, branches and working changes. Preserve existing work.
2. Work in an isolated feature branch; do **not** modify the live Windows supervisor/tunnel merely to exercise UI code.
3. Implement targeted UI change, then any minimal backend guard; keep unrelated cutover/runtime work out of scope.
4. Run the test matrix, show diff, stage only scoped files, commit and push. Prefer reviewable PR into main if code changes are substantial.
5. Roll out using the existing controlled Windows candidate/cutover process. Verify the running connector actually serves the updated SetupHTML and the previously existing bindings continue working.
6. If regression occurs, use the prior tested connector binary/profile and leave all workspace directories unchanged. Rollback must not delete managed worktrees.

**Do not claim deployment based on GitHub main HEAD alone.** Compare running connector version + rendered UI + live selected workspace.

## 7. Independent P0: finish Windows HH MCP G01 (do not confuse with this feature)

The UI improvement is a durable fix for future workspace selection; **HH MCP deployment can proceed independently once the proper original checkout is bound through an authorized supported method**.

Last verified state from the related chat:
- Canonical HH GitHub: **benice2me11/head-hunter-mcp**, **master @ 05e7b63dca68b98f4d8d1ac3263a5ee5bc3a1a3b**.
- Source Windows checkout **C:\Users\FoxOS_User\hh-mcp-server** was at **a5f9912** (4 commits behind), with its runtime still importing the pre-fix **hh_mcp_server/scraping/responses.py**.
- Earlier managed HH worktree **...\17b58d069d8f-18d8fcdc** was fast-forwarded to 05e7b63 and passed **125/125** tests in a dedicated Python 3.12 .venv.
- HH history **get_responses(include_deleted=true, limit=300)** previously failed on waiting for button **number-pages-2[aria-current=true]**; the merged fix replaces that with content/ID-change verification and fail-closed completeness metadata.
- Switching the Windows project via the UI again created managed **...\17b58d069d8f-bc4d1844**, originally at a5f9912, because SetupHTML omitted createWorktree.

After correct binding, update **only if the source checkout is clean and branch/remote checked**:

~~~powershell
# Illustrative commands; execute only when bound to the actual source checkout
git status --short --branch
git fetch origin
git pull --ff-only origin master
.\.venv\Scripts\python.exe -m unittest discover -s tests -v
~~~

Then verify the actual importing Python path/commit, controlled reload of **only hh_local** (do not casually restart unrelated upstreams or supervisor), and authenticated:

~~~text
hh_local.get_responses(include_deleted=true, limit=300)
~~~

**G01 live PASS requires** real active+deleted rows, exact vacancy IDs, an honest **history_complete=true**, reconciled tab counts, no stale paginated selector timeout, and recorded evidence. A partial return or false completeness is **not** PASS.

After G01 accepted, resume **work-search Search Coverage Remediation G02–G06** according to its newer handoff. G02 structured Habr, G03 durable queue, G04 non-HH sources, G05 state reconciliation, G06 query recall/E2E. This workspace-mode UI handoff does not authorize those unrelated code changes.

**Hard user constraints for the entire continuation:** **Vacancy Hourly Watch stays disabled**; no new scheduler, no application submissions without separate exact consent; preserve local modifications, personal browser sessions, cookies, tokens and drafts; do not conflate discovery with full screening.

## 8. Explicit definition of done

- [ ] Setup UI exposes Auto / Source checkout / Managed worktree before selection.
- [ ] Chosen mode is faithfully conveyed to set_project_root.
- [ ] Same project can be deliberately reselected with another mode after explicit switch without a new chat.
- [ ] Users see the true active/source paths and concurrency warning.
- [ ] Existing files/bindings/worktrees survive switching; non-Git and pinned refs are safe.
- [ ] Offline UI, backend and full Go tests pass; no host-feedback tool-call amplification.
- [ ] Controlled Windows live test demonstrates both directions and reconnect stability.
- [ ] Docs, scoped commit(s), and push/PR completed; no unrelated service or automation changes.
- [ ] Separately, HH G01 and later work-search gates recorded as PASS only on real acceptance evidence.

**Suggested next agent instruction:** Implement this handoff in codexify-go as a focused feature, with minimal backend changes and tests; separately complete the HH source-checkout deployment. Do not merely advise the user to repeat UI switching or start a new chat.
