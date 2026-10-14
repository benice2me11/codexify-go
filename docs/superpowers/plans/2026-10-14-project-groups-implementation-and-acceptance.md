# Project Groups — Implementation Plan and Acceptance Protocol

**Date:** 2026-10-14  
**Status:** PROPOSED / NOT IMPLEMENTED / ALL ACCEPTANCE GATES NOT RUN  
**Repository:** benice2me11/codexify-go  
**Reference baseline:** main at f0d5d22fb9f2bfbaa6aec4293bc29bc5d6c8647d  
**Intended first use case:** one ChatGPT conversation working with work-search, hh-mcp-server and habr-career-mcp on Windows.

## 1. Goal and boundaries

One conversation must bind to a logical group of independent Git repositories, choose an active repository without unbinding the group, and explicitly target any group member in file, patch, command, Git, diff, memory and project-instruction operations. Every member retains its own canonical source checkout, active source/worktree choice, Git context, instructions and lifecycle.

**Preserve:** single-project bindings, scratch projects, existing MCP upstreams, source checkouts, existing managed worktrees, Git history, and all uncommitted changes.

**Out of scope:** OS-level command sandboxing, combining repositories into one Git repository, arbitrary filesystem access beyond configured group membership, automated job applications, enabling Vacancy Hourly Watch, and touching Windows production cutover as part of documentation/design work. Completing job-search G01–G06 is independent of Project Groups. G01 should not wait for this feature.

**Rollout:** opt-in behind a default-disabled capability flag; staged implementation and tests first. No Windows service replacement, restart, rollback or tunnel reconfiguration without separately agreed operational approval.

## 2. Source-code findings / baseline

- internal/projects/manager.go: Manager.Workspace() returns one workspace.Root per binding; Select() persists one ProjectRoot.
- internal/projects/bindings.go: version 2 binding contains a single SourceProjectRoot/ProjectRoot.
- internal/workspace/root.go: workspace-relative path resolution and canonical confinement.
- internal/agenttools/files.go and git.go: each instance holds one workspace.Root.
- internal/mcpserver/server.go: file, patch, exec, Git and show_diff handlers derive root from workspaceFor(); ShowDiff is checkpointed by root.
- internal/execsession/manager.go: shell sessions are currently keyed by session id; per-conversation ownership should be added before multi-repo use.
- internal/config/config.go: project catalogue and independently configured MCP upstreams exist, but not Project Groups.

A valid workdir is **not** an OS sandbox. exec_command starts a normal child shell using the user's process permissions and environment. Project Groups must not claim filesystem isolation for arbitrary shell commands.

## 3. Proposed configuration contract (not implemented)

~~~json
{
  "mcp": {
    "multiProject": true,
    "projectGroupsEnabled": false,
    "projects": [
      {"name": "work-search", "path": "C:/Users/FoxOS_User/work-search"},
      {"name": "hh-mcp-server", "path": "C:/Users/FoxOS_User/hh-mcp-server"},
      {"name": "habr-career-mcp", "path": "C:/Users/FoxOS_User/habr-career-mcp"}
    ],
    "projectGroups": [
      {
        "name": "job-search",
        "primary": "work",
        "repositories": [
          {"alias": "work", "project": "work-search", "checkout": "source"},
          {"alias": "hh", "project": "hh-mcp-server", "checkout": "source"},
          {"alias": "habr", "project": "habr-career-mcp", "checkout": "source"}
        ]
      }
    ]
  }
}
~~~

**Contract decisions:**

1. Group names and member aliases are unique, stable, case-insensitive identifiers. Primary must reference an existing member. No nested groups in v1.
2. Each member resolves from a configured, positively identified project; a group does **not** expand its authority to the whole parent/home directory.
3. Canonicalize and validate roots at selection and access time. Reject duplicate, overlapping or missing roots, symlink/junction escapes and unexpectedly changed canonical targets. Treat Windows extended-length paths, case differences and UNC identity consistently.
4. Checkout is selected per member and clearly exposed in UI and API: source checkout versus managed worktree. The initial Windows configuration above uses source explicitly; do not silently convert a running service checkout into a managed worktree.
5. A group may be configured only when multiProject is enabled. The feature flag defaults to off. Invalid groups fail validation rather than partly enabling themselves.
6. A change to group membership/path cannot silently grant a conversation access to a new location: persist a validated membership snapshot/fingerprint and require explicit review/rebind if it changes.

## 4. Runtime and API design

### 4.1 Binding model

Add version 3 as an additive, tagged group binding, not as a replacement for existing v1/v2 bindings.

~~~json
{
  "version": 3,
  "mode": "group",
  "groupName": "job-search",
  "primaryRepository": "work",
  "activeRepository": "work",
  "membershipFingerprint": "<canonical-member-set-hash>",
  "repositories": [
    {
      "alias": "work",
      "sourceProjectRoot": "C:/Users/FoxOS_User/work-search",
      "projectRoot": "C:/Users/FoxOS_User/work-search",
      "checkout": "source",
      "managedWorktree": false
    }
  ]
}
~~~

The example omits existing identity/scope/timestamp and other per-worktree metadata, which must be preserved. Do not infer persistent trust from a user-supplied alias alone. Keep v1/v2 read/selection/resume behavior unchanged, including previously saved source/worktree bindings. Switching groups or disabling the flag must never delete worktrees or dirty files. A stale v3 binding with the feature disabled must fail closed with an actionable selection/resume message.

### 4.2 Selection and routing

Proposed tools (names subject to schema review): list_project_groups; set_project_group; set_active_repository. set_project_root retains its existing meaning for single projects. get_environment/setup_status reveal group, primary alias, active alias, member aliases and each member's source/active/worktree identity.

Add an optional repository field to repository-scoped operations. If omitted in group mode, resolve the current active alias; an explicit repository always overrides it. For older single-project bindings, behavior remains unchanged. Never change active alias merely because a tool used an explicit repository argument.

Example desired behavior:

~~~text
set_project_group(name="job-search")
read_file(repository="hh", path="pyproject.toml")
exec_command(repository="habr", cmd="git status --short")
git_status(repository="work")
set_active_repository(repository="hh")
git_diff()      # resolves hh, until the active selection changes
~~~

Root selection must be a single validated operation at the beginning of each call. Include effective repository alias, project root and checkout mode in status/diagnostic responses so results cannot be mistaken for another repository.

### 4.3 Tool coverage (no partial multi-repo support)

- File/search: read_file, write_file, glob, grep, apply_patch, import_host_file, export_host_file. A single apply_patch operates in exactly one selected member; cross-member patches require separately validated calls.
- Shell: exec_command takes repository; relative workdir resolves inside that member. write_stdin/stop/poll are bound to the originating conversation + repository + immutable session root, not whichever member is active later.
- Git/diff: git_status, git_diff, git_log, show_diff and list_worktrees target exactly one selected repository. Diff checkpoints use independent canonical checkout identities and must not advance a different member's cursor.
- Context: get_project_doc/get_agent_brief load the active/selected member's AGENTS.md rules; group-level orchestration guidance can be sourced from the primary member but must not override stricter member-specific instructions. Audit scoped memory, skills, chat and artifacts; define explicit group-vs-member scope and avoid accidental cross-member writes.
- MCP bridge: upstream hh_local and habr_career continue to work independently of group selection; avoid duplicate discovery, session churn and stale MCP tool-schema assumptions.

### 4.4 Isolation and trust

Files/patches/imports must use per-member canonical containment checks at point of use, including symlink/reparse-point scenarios and directory creation. Do not use the parent workspace as the file-tools root. A shell command is **not** restricted to the member by its workdir; expose this as a Trusted Workspace behavior. An optional future Sandboxed Workspace may use OS/container permissions. No security claim that is unsupported by the runtime.

## 5. Implementation sequence (separate reviewable PRs)

**PG-I0 — Baseline and contract (docs/test fixtures).**
- Inventory current running Windows service, connector schema, effective checkout, worktree mappings, bindings and dirty files, using redacted evidence. Do not modify them.
- Freeze tool inventory and legacy behavior. Define explicit API inputs/outputs, risk model and upgrade/rollback contract.

**PG-I1 — Config/catalog and canonical repository registry.**
- Add group configuration and flag, validate uniqueness/membership/canonical identity, disallow unexpected alias overlap.
- Introduce a group-aware resolver returning immutable member alias, source root, active root and checkout metadata.
- Cover Windows path identity, symlinks and junctions in unit tests.

**PG-I2 — Binding v3 and worktree lifecycle.**
- Persist group identity, membership fingerprint, active alias and per-member checkout mode.
- Preserve legacy decoding, switch/resume/scratch and existing worktrees. Support explicit source/managed selection per member; on failure roll back partial group setup only, leaving pre-existing repositories untouched.
- Test crash/restart and concurrent reads/selections.

**PG-I3 — Tool routing and isolation.**
- Add repository-aware wrappers for file, search, patch, artifact ingress/egress, shell and Git handlers.
- Bind each exec session to caller identity and member; enforce ownership on write_stdin and stop/poll. Ensure member switching cannot retarget a running process.
- Maintain distinct Git/show_diff checkpoints; validate per-member AGENTS.md and scopes for memory/skills/brief.
- Provide clear errors for unknown alias, changed root and out-of-root path.

**PG-I4 — UI and connector discovery.**
- Add group picker; member switcher; explicit Source checkout vs Managed worktree indicator/choice; one-click return to group overview.
- Display selected repo alias for file, shell and Git operations where useful.
- Update tool schemas, schema-version/refresh guidance, help/README/ARCHITECTURE.md.

**PG-I5 — Cross-platform tests and staged rollout.**
- Run unit, integration, race and build checks. Test native Windows/SCM, Linux/systemd --user and macOS/launchd runtime behavior on the actual OS (cross-compilation is not runtime validation).
- Run guarded Windows canary only after explicit approval; verify one ChatGPT conversation and three repositories. Revert to the previous Go build/flag if acceptance fails; no automatic Rust fallback for transient VPN/tunnel HTTP errors.
- Produce sanitized gate ledger and final release decision.

No release cutover or G01/G02–G06 job-search automation actions are authorized by this plan.

## 6. Acceptance gates (all currently NOT RUN)

A gate is PASS only with reproducible test output, sanitized logs/config hashes and named commit/build identifiers. "Tool exists" or "Go starts" is insufficient.

### PG-A00 — Baseline / reversibility

- [ ] Exact initial branch/commit/build, service binary hash, config, existing bindings/worktrees and Git dirty-state recorded without secrets.
- [ ] Feature disabled by default; legacy behavior matches baseline before any group selection.
- [ ] Tested rollback/revert procedure exists. No pre-existing dirty file, binding or worktree was altered by staging.

### PG-A01 — Configuration / membership

- [ ] Valid three-member job-search group loads and enumerates with deterministic aliases and primary.
- [ ] Duplicate aliases/canonical roots, unknown member, missing primary, outside-access-root paths and overlapping roots fail predictably.
- [ ] Windows case, extended-length path spelling, symlink/junction and Unicode/UNC edge cases handled or explicitly rejected safely.
- [ ] Changing group membership after binding requires explicit user rebind; no silent privilege expansion.

### PG-A02 — Persistent binding / checkout

- [ ] Legacy v1/v2 single-project conversations, scratch, switch and resume still work without data migration or unexpected new worktrees.
- [ ] Group binding v3 survives two controlled service/worker restarts, with identical member roots, active alias and per-member checkout identities.
- [ ] Source checkout/managed worktree shown explicitly; createWorktree=false behavior respected and no deployment checkout selected accidentally.
- [ ] Concurrent conversations do not leak group bindings; removing/reselecting a group does not delete repositories, dirty state or unrelated worktrees.

### PG-A03 — Complete tool routing

- [ ] From one conversation read, write, glob, grep and patch fixture files in each of three repositories with repository parameter and with active alias default.
- [ ] Switching active repo changes subsequent implicit calls; explicit repository always overrides active; no call silently switches group or alias.
- [ ] Relative ../ traversal from any member is denied; file and patch tools do not access another member without selecting that member.
- [ ] exec_command executes from correct directory for every member; an existing write_stdin session remains bound to its original member after active selection changes.
- [ ] Cross-conversation access to a foreign exec session is denied. Stale/unknown alias returns clear error.

### PG-A04 — Git / diff / instructions

- [ ] git_status, git_log, git_diff, show_diff and list_worktrees resolve the correct checkout for each member.
- [ ] Make independent fixture changes in hh and habr: showing/advancing hh diff does not advance habr diff; work remains distinct.
- [ ] Commit/push in disposable test repositories affects only the requested Git root and branch; existing local user changes remain intact.
- [ ] Per-member AGENTS.md, brief, memory/skills and artifact behavior is scoped and documented; no unrelated member content is treated as its instruction by accident.
- [ ] Temporary group/switch state is safely cleaned up; persisted project-open diff baselines remain stable after restart.

### PG-A05 — Negative security cases

- [ ] Reject absolute path, traversal, symlink escape, Windows junction/reparse escape, path alias normalization bypass and forbidden writes to a nonmember root in file/patch/import operations.
- [ ] Revalidate checkout identity after path replacement/deletion. Fail closed rather than resolving a missing member to the parent/home directory.
- [ ] A group is never equivalent to giving file tools the entire C:/Users/FoxOS_User root.
- [ ] Logs and API results do not leak secrets from .env, tunnel configuration or unrelated projects.
- [ ] Documentation and UI explicitly state shell access is Trusted Workspace, not an OS sandbox. If OS sandboxing is later promised, it requires a separate security gate.

### PG-A06 — UX / MCP compatibility / performance

- [ ] Group and member selection operate in one live ChatGPT conversation without disconnecting, creating a new chat or manually switching the global project binding.
- [ ] UI shows group, member, Source checkout / Managed worktree and actual root; no confusion between managed worktree and source checkout.
- [ ] Stale schema refresh is detected and actionable; after refresh all repository arguments are advertised correctly.
- [ ] Existing upstream hh_local and habr_career still discover and execute independently of selected repo.
- [ ] No duplicate or bursty MCP sessions/tools, no unexpected rate-limit loop, and no unbounded reconnection on switching; demonstrate with timestamped diagnostics for a repeatable short smoke run.

### PG-A07 — CI and native runtime matrix

- [ ] go test ./... -count=1 and go vet ./... pass on tested candidate.
- [ ] go test -race passes for projects/workspace/agenttools/mcpserver/execsession/diff on supported runners.
- [ ] Windows, Linux and macOS binaries build; native integration smoke passes on each OS, not just cross-build.
- [ ] Windows SCM health, restart and conversation-binding persistence pass without concurrent Rust ownership; Linux systemd --user and macOS launchd startup/restart pass in native environments.
- [ ] Existing single-project tests, worktree lifecycle, artifact flow, session tests and upstream MCP bridge regressions pass.

### PG-A08 — Guarded canary and closure

- [ ] Operator-approved Windows canary checks: one job-search group, work/hh/habr root detection, all per-repo operations, two restarts, unchanged hh_local/habr_career functionality.
- [ ] After restart, run independent read/patch/test/status/diff sequences in disposable fixture branches (no vacancy application, no watcher enablement).
- [ ] Old build + config rollback rehearsed and verified; transient VPN or tunnel HTTP 403/503 alone does not count as a Project Groups failure and must not trigger legacy Rust restoration.
- [ ] Record PASS/FAIL/BLOCKED with evidence links, candidate SHA, operator, date and rollback conditions. Only then mark Project Groups accepted.

## 7. Concrete end-to-end scenario

1. From existing chat choose group job-search, primary=work, source checkouts; verify get_environment returns all three allowed roots and exact checkout modes.
2. Read work-search README, hh-mcp-server pyproject.toml and habr-career-mcp manifest with explicit member selection.
3. Create unique fixture files (in dedicated temporary branches or disposable clones) in two members and run member-specific tests concurrently. Keep original dirty trees untouched.
4. Compare both Git statuses and independent show_diff checkpoints; commit to their respective fixture branches. Verify other member stays unchanged.
5. Switch active member hh -> habr -> work with no setup_ui_switch_project, connector refresh or chat restart.
6. Start a long-running harmless fixture command in hh, switch to habr, poll hh session safely. Try polling it from an unrelated conversation: denied.
7. Attempt traversal into sibling and parent, Windows junction escape and unknown member: each fails closed for file/patch tools.
8. Restart worker/service twice; group binding, member roots, active alias and diff checkpoints are unchanged. Upstreams still respond.
9. Disable feature flag and exercise existing single-project flow. No repo/worktree data loss; reenable and explicitly resume group.
10. Capture sanitized evidence and decide GO/NO-GO. Do not touch live vacancy submissions or Vacancy Hourly Watch.

## 8. Evidence and rollback

Create a separate dated acceptance ledger under docs/handoffs for release work. Every PG-Axx gate records status (NOT RUN / PASS / FAIL / BLOCKED), test commands, machine/OS, source commit, build hash, logs, relevant screenshots and any remaining risk. Avoid committing keys, tokens, private user files or full machine environment dumps.

Rollback sequence: disable feature flag and restore the previously verified Go binary/config, preserving group v3 data for later inspection; validate legacy project binding and upstream MCP operation. Never silently substitute the entire home directory, switch project roots, remove managed worktrees, or activate Rust on a transient VPN/tunnel error.

**Definition of done:** a single chat can safely coordinate independent edits/tests/Git operations across the three known repositories, source/worktree identities are explicit, per-member isolation holds for filesystem tools, legacy bindings survive, native OS acceptance passes, and the deployed Go runtime remains recoverable.

**Current result:** design only. No gate executed; no changes to live services, existing local worktrees, user job-search automations or applications.
