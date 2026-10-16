# Project Groups — PG-I2 Binding v3 development ledger (15 Oct 2026)

**Status:** implemented and fixture-tested as an internal backend API, not available to production MCP clients. Stacked on [PG-I1 PR #20](https://github.com/benice2me11/codexify-go/pull/20).

## Scope

- Tagged `version=3, mode=group` persisted binding, additive to legacy versions 1 and 2. Legacy writes continue using v2; no migration or rewrite of old binding files.
- Group name, stable primary alias, persisted active alias, SHA-256 membership fingerprint, generation counter and per-member source/active root + explicit checkout kind.
- `Manager.SelectGroup` creates one group binding after validating each catalog member. When `checkout=managed`, it creates only that member's disposable managed worktree; source checkouts are not changed. Partial creation is rolled back if initial selection/persistence fails.
- `Manager.SetActiveRepository` updates the implicit default under the process mutex and increments generation. A repository-specific `Manager.WorkspaceFor(meta, alias)` resolves an immutable, validated root for that call without modifying active alias.
- Group bindings fail closed when feature flag is disabled or membership changes, while retaining their v3 JSON on disk for later re-enable.
- Validations reject duplicate member records, unknown aliases, a foreign managed Git worktree, active root changes, out-of-access-root paths and invalid group fingerprints.
- Legacy `Resume(path)` cannot silently reinterpret group v3 as v2. Explicit group switches archive the existing binding; old worktrees and user files remain untouched. A legacy `set_project_root` cannot bypass an existing group binding.

**Not yet exposed:** MCP tools `list_project_groups`, `set_project_group`, `set_active_repository`; repository params for file/patch/exec/git/diff; Setup UI/schema, member memory/context and session ownership. Those must be introduced with the routing stages.

## Evidence and test boundary

New tests use only disposable Git repositories under `t.TempDir()`. They cover:
- Three independent source repositories and persisted group binding v3.
- Explicit alias override independent of active alias, active switch persistence after **two Manager recreations** (not live worker restarts).
- v3 feature-flag disable/re-enable and v2 legacy preservation.
- Stale group fingerprint and unknown alias denial.
- One explicitly managed checkout with other members source-only; auto source-in-use compatibility for another legacy conversation.
- Aborted initial group selection cleans up newly created managed worktree when binding persistence fails.
- Concurrent explicit lookups and active switches in one Manager.
- Malformed v3 duplicate member record, foreign managed worktree and global selection bypass denial.
- Legacy `Resume` does not treat group root as a single-project permission grant.

## Acceptance delta

| Gate | Status | Notes |
|---|---|---|
| PG-A00 | PARTIAL | Running Windows Go-only runtime preserved; no operator cutover or rollback rehearsal |
| PG-A01 | PARTIAL | Catalog/registry tests from PG-I1, v3 stale fingerprint guard |
| PG-A02 | PARTIAL | Source/managed v3 persistence and two Manager recreations; **real worker/service restarts NOT RUN** |
| PG-A03 | PARTIAL | Internal per-call `WorkspaceFor` contract tested; MCP tool routing NOT IMPLEMENTED |
| PG-A04 | NOT RUN | Independent Git/diff/context/memory tools not yet routed |
| PG-A05 | PARTIAL | Root and binding negatives pass; file/patch/execsession/ingress checks pending |
| PG-A06 | NOT RUN | Hosted one-conversation UX and upstream burst tests pending |
| PG-A07 | PARTIAL | Native Windows Go tests and vet; CI native Linux/macOS and race pending |
| PG-A08 | NOT RUN | No production feature enablement, restart, canary or rollback |

## Follow-up and limitations

1. Implement PG-I3 per-call routing in MCP handlers and connector schemas; do not advertise group APIs until all relevant operations can select the correct member.
2. Add session ownership tuple (conversation identity + group + alias + immutable checkout root), member-scoped diff checkpoint/artifact/context/memory semantics and negative cross-chat tests.
3. Handle multi-process writers to the same v3 binding with a durable compare-and-swap/lock if concurrent MCP workers become supported. Current generation is monotonic under one Manager's process mutex; it is **not** a cross-process CAS.
4. Add Setup group picker and schema-version bump in a separate UI PR. Only then run Windows Go-only operator-approved canary and PG-A00–PG-A08 completion tests.
5. Never allow the group source resolver to stand in for arbitrary OS shell sandboxing. An exec working directory is not an OS-level isolation boundary.

**Operations not performed:** service restart, worker replacement, tunnel change, Rust fallback, feature-flag enablement in production, vacancy submissions, Vacancy Hourly Watch activation.
