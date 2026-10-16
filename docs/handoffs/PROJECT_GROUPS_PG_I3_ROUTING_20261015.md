# Project Groups — PG-I3 routing and session-ownership handoff / acceptance ledger

**Date:** 2026-10-15
**Scope:** MCP-facing, repository-aware routing of the PG-I2 v3 group binding.
**Branch:** `feat/project-groups-mcp-routing-20261015` (stacked on PG-I2 PR #22, itself stacked on PG-I1 PR #20).
**Production rollout:** NOT PERFORMED.

## Implementation

1. When `mcp.projectGroupsEnabled=true`, advertise `list_project_groups`, `set_project_group`, `get_project_group_status` and `set_active_repository`. With the flag disabled, do **not** advertise these tools. Existing single-project selection remains available and refuses nonempty repository aliases.
2. Introduce an optional `repository` argument, defaulting to the active group member, across `read_file`, `write_file`, `glob`, `grep`, `apply_patch`, `exec_command`, Git status/diff/log, `show_diff`, `list_worktrees`, project doc/brief, get_environment, memory, skills and artifact import/export. Every call uses one immutable resolved member root; explicit arguments do not persistently change active alias.
3. Structured file/Git/exec/diff outputs include optional repository context: effective alias, canonical source/active roots, and checkout type. Existing legacy single-project responses omit it. The repository context is also attached in MCP tool metadata for project documents, skills and artifact operations.
4. Group diff baselines initialize per checkout, with existing independent diff checkpoint ownership. Two fixture changes in hh/habr do not share a Git tree or advance one another's last-diff cursor.
5. Exec session owner includes conversation identity, group, repository, immutable checkout root, membership fingerprint and binding epoch. `write_stdin` verifies identity, validates the original member independently of active alias and rejects foreign or stale sessions. The ownership check is also enforced by `execsession.Manager.WriteOwned` and `StopOwned`; old unscoped methods cannot operate on owned sessions. Explicit switch/rebind revokes old sessions even if aliases and paths are unchanged.
6. Group Markdown chat remains anchored to the primary checkout when the active member changes. Agent brief loads the **effective** member's AGENTS.md and states clearly that shell commands are Trusted Workspace commands, not an OS sandbox.
7. A glob match now passes canonical workspace containment before it is returned. Relative parent traversal and cross-member patch paths remain rejected by their existing workspace/patch guards.
8. Connector schema marker changes only for enabled groups (`+project-groups-v1`, or existing compact hash if the version exceeds 64 characters), so old cards can show stale-schema guidance. A full group picker/card redesign remains PG-I5.

## Automated disposable-fixture coverage

- Official MCP SDK over HTTP, 3 independent temporary Git repos in one conversation: group enumeration/selection, per-member read/write/glob/grep/patch, source identities, Git status/diff/log/worktrees and independent show_diff checkpoints.
- Active alias switch with implicit default and explicit override, without global set_project_root; get_environment correctly distinguishes active and effective aliases.
- Per-repo AGENTS.md/brief and memory isolation; no cross-member content observed.
- Traversal/unknown alias/cross-member patch negative calls, and attempts to use group-level tooling in legacy mode.
- Long-running exec in hh while active changes to habr: polling succeeds for the same conversation, fails for a second conversation even after it binds the same group, and fails after explicit group rebind.
- Concurrent explicit read/git calls while active alias changes never change effective root or mix fixture markers.
- Git add/commit/push through member-scoped exec_command into **two disposable bare remotes**. Verified each remote's pushed blob; the primary third repo retained its original HEAD. No real job-search checkout touched.
- Standalone execsession owner tests deny foreign conversation, repository, root, fingerprint, and epoch, including unscoped stdin/stop access.

## Risk / trust boundaries

- **Shell access is not sandboxed.** Workdir selects initial directory but commands run as the OS user; a command could explicitly navigate or write elsewhere with those privileges. For directory- and path-confined operations, Root must be used directly.
- The persistent group binding's generation is serialized with an in-process mutex, not a multi-worker durable CAS. Enablement should remain opt-in until the operator validates actual worker topology. File identity rechecks and binding epochs are separate safeguards.
- Root-symlink/path-swap races and artifact resource ownership need further adversarial tests. Output metadata identifies the repository but does not create OS-level permission boundaries.
- Native Windows/macOS/Linux CI and race are required before merging. Cross-compilation alone is not native runtime acceptance.
- A successful three-repo fixture does not prove live Setup cards, persistent Go worker restarts, systemd/launchd runtime, or production MCP burst behavior.

## Acceptance status (pre-operator)

| Gate | State | Evidence |
|---|---|---|
| PG-A00 | PARTIAL | Source baseline and go-only rollback protocol documented; production rollback not rehearsed |
| PG-A01 | PARTIAL | PG-I1 canonical registry and fingerprint checks passed, stale v3 detection; native 8.3 alias test may skip where unavailable |
| PG-A02 | PARTIAL | v3 persisted across two Manager recreations; **not** two actual service restarts |
| PG-A03 | PARTIAL | MCP SDK all three fixture roots + explicit/implicit routing + concurrent calls passed; no live production test |
| PG-A04 | PARTIAL | Fixture independent Git/diff, AGENTS/memory and disposable Git push passed; artifact resource owner/long-lived state still under review |
| PG-A05 | PARTIAL | Traversal, unknown alias, patch escape, foreign/stale session cases passed; broader symlink/junction/TOCTOU matrix pending |
| PG-A06 | NOT RUN | Hosted UI/production one-chat canary and upstream MCP session-burst acceptance remain |
| PG-A07 | PARTIAL | Go tests/vet locally and CI matrix (fill actual run evidence after completion); Windows/Linux/macOS service runtime acceptance not run |
| PG-A08 | NOT RUN | Operator-approved Go-only canary, worker restarts and rollback rehearsal not run |

## Safe continuation

- Review/merge PR #20, then retarget/merge #22, then retarget/merge this PG-I3 PR. Check that no v0.9.0 release branch or G10 repair is accidentally included.
- Continue PG-I4: artifact reference owner enforcement, complete per-member skills/projectdoc/memory/CHAT state contracts, upstream session/burst metrics and negative symlink/reparse TOCTOU tests.
- Implement PG-I5 Group Setup picker and member Source/Managed controls, schema-staleness recovery and accessibility tests.
- Run native CI + approved operator canary PG-A00..PG-A08 with Go-only rollback. **Do not** activate Rust fallback on transient VPN/HTTP 403/503; do not enable Vacancy Hourly Watch or send job applications without separate authorization.

**Production guard:** No Windows SCM service restart, tunnel change, Rust ownership, live job-search repository mutation, watcher toggle, or HH/Habr submission was performed in this development stage.
