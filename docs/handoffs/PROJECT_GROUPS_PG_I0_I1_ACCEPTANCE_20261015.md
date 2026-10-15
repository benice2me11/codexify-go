# Project Groups — PG-I0/I1 implementation ledger (15 Oct 2026)

**Scope:** baseline, opt-in configuration and canonical source repository registry. This is **not** acceptance of multi-repo tools, v3 bindings, UI or a production cutover.

## Baseline / operator boundary

- Repository: `benice2me11/codexify-go`
- Base: `main` at `cbb665aa4e5c2655227882fc6094cc350c847358`
- Work branch: `feat/project-groups-config-registry-20261015`
- Local source checkout: `C:\Users\FoxOS_User\codexify-go`; initially clean and aligned with `origin/main`
- Windows service: `CodexifyGo` **Running / Automatic**, Go worker/service PID 2596 and 18952, managed tunnel process observed; no service/worker restart, configuration replacement or deployment performed for this feature
- Running Go executable SHA256 (both processes): `8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880`
- Current runtime remains governed by its existing Go-only cutover / G10 watcher. No Rust fallback is to be used for transient HTTP 403/503.
- No live `work-search`, `hh-mcp-server`, `habr-career-mcp` files or worktrees were modified by tests. All new repository fixtures use temporary Git repositories.
- No Vacancy Hourly Watch change and no HH/Habr application submission.

## Implemented in PG-I1

- Additive `mcp.projectGroupsEnabled` (default **false**) and `mcp.projectGroups` declarations, referencing only explicitly configured `mcp.projects` by stable identifier.
- Strict validation: feature requires `multiProject`; no nested group; unique case-insensitive group and alias; valid primary alias; unique configured project reference; explicit `source`/`managed` checkout choice; reject unknown project, empty checkout and unsafe identifier.
- An opt-in, read-only `projects.GroupRegistry` created by `projects.New`. It never creates worktrees, writes conversation bindings or changes active project. When disabled it returns no visible groups and refuses source resolution.
- Canonical source Git-root registration with access-root containment and rejection of overlapping or duplicated member roots. `ResolveSource` revalidates the configured path and Git top level at call time, and checks checkout file identity against replacement or reparse-point swaps.
- Deterministic SHA-256 membership fingerprint over group name, primary alias, aliases, configured-project names, checkout kinds and canonical Git paths. Reordering member declarations does not alter the fingerprint; permission-bearing changes do.
- `ListProjectGroups` and `ResolveProjectGroupSource` are **internal manager methods**, not MCP tools. Runtime bindings still use legacy single-project v1/v2.

**Safety detail:** on Windows, Go's `os.FileInfo` lazily loads file IDs. The registry resolves/caches identity while the configured checkout is still present to reject a subsequent rename-and-replace at the same path.

## Test evidence (Windows/amd64, Go 1.26.5)

| Command / scenario | Result |
|---|---|
| Baseline `go test ./internal/config ./internal/projects ./internal/workspace -count=1` | PASS |
| Candidate `go test ./... -count=1` | PASS |
| Candidate `go vet ./...` | PASS |
| Config positive/negative aliases, flags, unknown projects, checkout kinds | PASS |
| Three independent fixture Git repositories; deterministic list and alias/source lookup | PASS |
| Same-root/overlap, access-root member, non-Git, nested Git, absent/outside path | PASS |
| Replace checkout at same canonical path; stale source must be denied | PASS |
| Windows `\\?\` extended path and junction escape | PASS |
| Windows 8.3 short-name fixture | SKIPPED — short-name generation disabled on volume; existing workspace path-identity tests PASS |
| Windows `go test -race` | BLOCKED — CGO disabled in current Windows Go environment |
| `GOOS=linux GOARCH=amd64 go build ./cmd/codexify-go` | PASS (cross-build, not Linux runtime evidence) |
| `GOOS=darwin GOARCH=arm64 go build ./cmd/codexify-go` | PASS (cross-build, not macOS runtime evidence) |

The supported repository's existing CI runners may provide native Linux/macOS and CGO-enabled race results after the PR is opened; these are not implied to have passed by the local cross-build.

## Acceptance status

| Gate | Status | Explanation |
|---|---|---|
| PG-A00 | PARTIAL | Exact code/runtime baseline captured; no rollout or full rollback rehearsal |
| PG-A01 | PARTIAL | Static catalog + canonical source registry implemented; fixture checks pass; native 8.3 short-name alias fixture skipped; v3 stale membership binding not yet available |
| PG-A02 | NOT RUN | Binding v3, persisted active member, source/managed per member, 2 live restarts absent |
| PG-A03 | NOT RUN | MCP file/search/patch/exec routing absent |
| PG-A04 | NOT RUN | Git/diff/context/member scope absent |
| PG-A05 | PARTIAL | Catalog, path containment and junction negatives pass; session/patch/import cross-member checks remain |
| PG-A06 | NOT RUN | Hosted UI/group selection/upstream burst acceptance absent |
| PG-A07 | PARTIAL | Windows tests/vet and cross-build pass; native Linux/macOS and race matrix pending |
| PG-A08 | NOT RUN | Production Go-only canary, restarts and rollback require separate operator authorization |

## Next stage: PG-I2

Extend bindings additively with tagged `version=3, mode=group`. Persist validated membership fingerprint, active alias and each member's source/active checkout. Existing single-project v1/v2 and scratch handling must stay byte-compatible. `set_active_repository` must update only the implicit default; per-call explicit repository selection must not mutate the group binding. Managed worktree creation, when explicitly requested, must be per-member and reversible on initial selection failure, never deleting pre-existing dirty work.

Do not enable the production feature flag or call the release accepted until routing, UI, runtime and all PG-A00–PG-A08 gates have been validated.
