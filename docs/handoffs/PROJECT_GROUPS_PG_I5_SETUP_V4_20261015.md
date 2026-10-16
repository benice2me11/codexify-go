# Project Groups — PG-I5 Setup v4, per-member checkout modes, acceptance ledger

**Date:** 2026-10-15
**Status:** IMPLEMENTED IN DEVELOPMENT BRANCH / PRODUCTION FEATURE FLAG REMAINS OFF
**Branch:** `feat/project-groups-setup-v4-20261015` (stacked on PG-I4 PR #25, then #24 → #22 → #20)

## Goal and capability boundary

Provide a real Setup Group UI in the *same ChatGPT conversation* with selectable group and independently chosen Source checkout / Managed worktree per member. Persist an active default repository alias while allowing explicit per-call repository override. Preserve existing worktrees and dirty changes on checkout-mode switching. Maintain Setup v1/v2/v3 compatibility and avoid any group-specific UI/API exposure with `mcp.projectGroupsEnabled=false`.

No release/cutover or production configuration mutation is authorized by this stage. The only Git operations targeting repositories outside `codexify-go` are tests operating on disposable fixtures.

## Implemented

1. **Schema and setup resource.** `ui.GroupSetupURI = ui://codexify-go/setup/v4/mcp-app.html`; tool metadata selects v4 only if groups are enabled. Existing v1/v2/v3 setup resource URIs remain registered for compatibility. An additive `+setup-v4` feature marker is included alongside `+project-groups-v1` (hashed by the existing connector version shortening rule when needed).
2. **App-callable group tools.** `list_project_groups`, `set_project_group`, `get_project_group_status`, `set_active_repository`, `set_repository_checkout` support the conversation-scoped `uiContext` and are accepted by setup-context middleware. `setup_status` includes a configured group catalog and `projectGroupsEnabled`; no group mutation is invoked during UI mount or host globals updates.
3. **Initial member modes.** `set_project_group(name, checkouts)` accepts an optional case-insensitive alias map with explicit `source|managed` values. The group catalog remains the membership authority. Invalid/duplicate aliases, unknown members and `auto` in per-member checkout selection fail closed. Existing binding selection remains idempotent and refuses silent mode changes.
4. **Checkout switching.** `set_repository_checkout(repository, checkout)` changes only one member, preserving the group binding, active default, unrelated members and dirty files. Switching Managed→Source retains the original managed Git worktree and records a preserved managed path. Source→Managed reuses that preserved checkout or creates a new managed worktree only if none exists. A failed initial creation/persistence rolls back only the newly created checkout.
5. **Owner revocation.** Per-member `checkoutRevision` increments on checkout-kind changes, including return to the same physical root. Immutable exec-session owner validation includes the checkout revision, so an old process cannot be polled through a newly selected checkout. Grouped artifact owner records also include it: an old resource URI cannot revive after Source→Managed→Source. Legacy PG-I2 v3 records with missing revision normalize to revision 1 without rewriting the saved binding; first mode change advances to 2.
6. **Stable group chat.** `CHAT.md` always belongs to the primary member's *source* checkout, even when that member changes its active checkout to a managed worktree. Changes to active alias or primary member mode do not migrate the conversation transcript. The same behavior applies to `chat_await` and per-member agent brief references.
7. **Group Setup UI.** Shows configured groups, member aliases, primary/active alias, canonical source and active paths, per-member mode choices and preserved managed path. A dedicated member action switches the default alias; another switches checkout kind. Existing standalone Auto/Source/Managed controls remain for legacy single-project flow. Group switching uses the existing archived-binding flow explicitly; no implicit project rebind or destructive cleanup. After a failed mutation response the UI refreshes server state instead of retaining a possibly stale checkout indicator.
8. **Stale schema, focus and host-loop guard.** Setup prints actionable refresh/reconnect guidance and blocks workspace/group mutations if connector/conversation schemas disagree. All handlers are driven only by explicit user actions; host globals are display-only and cannot initiate tool requests. Native button/select/label semantics and focus restoration on updated member controls support keyboard use; manual hosted focus/screen-reader acceptance remains for PG-A06.

## Automated evidence

- Windows native Go `go test ./... -count=1`, `go vet ./...`.
- Linux amd64 and macOS arm64 cross-build. Cross-build is not native runtime acceptance.
- Group model tests: explicit initial modes, invalid/duplicate alias rejection; active alias and checkout mode persistence across 2 Manager recreations; primary member checkout switch; dirty managed worktree preserved when switching away and back; unrelated members unchanged.
- Old v3 binding schema without member revision: loaded with revision=1, first mode switch persists revision=2.
- Group primary checkout switches Source→Managed→Source without changing the conversation's `CHAT.md` path.
- Official MCP SDK over HTTP: `set_project_group` with initial modes, `set_repository_checkout`, `get_environment`, full checkout roundtrip; a foreign conversation cannot use another chat's `uiContext`; the v4 resource exists and v3 remains readable.
- Scoped artifact export and long-running exec session created before checkout roundtrip cannot be used afterward, even if a checkout path returns to its original string.
- Real embedded SetupHTML execution in offline Node host-event harness: group card mount does not call tools; initial checkout choices sent correctly; exactly one mutation for rapid clicks; active member and checkout type switches do not call global `set_project_root`; stale schema blocks group selection; 100 passive host globals cause no additional calls. Existing standalone Setup v3, Chat, Diff and Update widget smoke remain unchanged.

## Explicitly NOT accepted yet

- Hosted ChatGPT Setup v4 interaction and accessibility/keyboard/focus smoke with a real authorized connector, not just an offline DOM harness.
- Full native Windows/macOS/Linux service/launchd/systemd lifecycle and two real Windows worker/service restarts while retaining v3 binding.
- Live upstream hh_local/habr_career burst/429 regression smoke after feature-enable; mock upstream PG-I4 evidence is not equivalent.
- Cross-process binding generation CAS. Current Manager mutex serializes one Go worker process; separate writer processes require a durable concurrency protocol before multi-worker use.
- Filesystem operations are in Trusted Workspace mode and **not OS-sandboxed**. Existing canonical path rechecks reduce escape risk; they do not eliminate concurrent reparse-point swaps by an adversarial local process.
- The host must preserve trusted `resources/read` conversation identity metadata for grouped artifact resources; otherwise reads fail closed until supported.
- Production Go-only canary, downgrade/rollback rehearsal, release decision and PG-A00..PG-A08 closure.

## Gates after PG-I5

| Gate | Status | Reason |
|---|---|---|
| PG-A00 | PARTIAL | Source/build baseline recorded; live rollback rehearsal pending |
| PG-A01 | PARTIAL | Config/root validation and stale physical identities tested |
| PG-A02 | PARTIAL | Source/Managed per-member modes and Manager recreations tested, real restarts pending |
| PG-A03 | PARTIAL | Three-repo MCP fixture routing tested, not live |
| PG-A04 | PARTIAL | Independent Git/diff/context/artifacts tested on fixtures |
| PG-A05 | PARTIAL | Symlink/junction, invalid membership and foreign-session cases; not full OS sandbox |
| PG-A06 | PARTIAL | Setup v4 offline UI plus mock upstream no-burst, actual hosted canary pending |
| PG-A07 | PARTIAL | Windows full tests/vet and cross-build; GitHub native CI must finish |
| PG-A08 | NOT RUN | Controlled Go-only production canary/restart/rollback requires separate operator approval |

**Merge order:** PG-I1 #20 → PG-I2 #22 → PG-I3 #24 → PG-I4 #25 → PG-I5 (this PR). Do not mix with v0.9.0 release branches or G10 cutover changes.

**Preserve:** production Windows `CodexifyGo` SCM service, tunnel, Rust-disabled Go-only ownership, local source checkouts and managed worktrees. Do not enable Vacancy Hourly Watch or submit any applications without separate explicit authorization.
