# Project Groups: consolidated integration and Go-only staging — 2026-10-16

**Integration branch:** `integration/project-groups-v1-20261016` based on upstream `main` `709dbbf`.

**Required production boundary:** `mcp.projectGroupsEnabled=false`. Nothing here authorizes replacement/restart of the running Windows service, enabling job-search watchers, applications, changing VPN or tunnel, or restoring Rust.

## Consolidated code set

One merge commit carries exactly the stacked, reviewable Project Groups history:

| Stacked PR | Scope |
|---|---|
| #20 | PG-I1 config gate, canonical group catalog/registry |
| #22 | PG-I2 persisted binding v3, per-member checkout |
| #24 | PG-I3 MCP alias-routing, owned shell sessions, independent Git/diff |
| #25 | PG-I4 artifacts and context security, filesystem confinement |
| #26 | PG-I5 Setup v4 group/member mode UI |
| #27 | Windows/macOS managed-worktree path canonicalization |
| #28 | PG-I6 native compiled-worker smoke and PG-I7 Go-only readiness gate |

The integration merge starts from the **latest main**, retaining G10 duplicate-Go owner fixes and release-candidate HOLD documentation. This is not a source-tree overwrite of `main`, a release-candidate merge, or an OS cutover.

## Integration-only defect found and fixed

During local `go test ./... -count=1`, concurrent calls to `read_file` / `git_status` and `set_active_repository` intermittently hit Windows `ERROR_SHARING_VIOLATION` when a persistent binding read overlapped an `os.Rename` replacement. A repeated 40-round MCP stress test reproduced the exact `open <binding>.json: The process cannot access the file because it is being used by another process` error.

The fix adds an independent `sync.RWMutex` in `projects.Manager` around persistent binding read/write and workspace-switch record read/replace/delete. This complements, but does **not** replace, the existing mutation mutex and does not change JSON binding schema, member roots or Go-only behavior. It is explicitly **in-process**, not durable multi-process compare-and-swap. The concurrent MCP test now has stronger read/write overlap and includes source error text in failures. 35 repeated iterations on the first fix passed; final broader tests and GitHub CI are recorded in the PR.

## Minimum acceptance before main merge

- `git merge-base --is-ancestor origin/main <integration>` and `git merge-base --is-ancestor origin/test/project-groups-native-worker-20261016 <integration>`.
- No unexpected modifications to the existing `supervisor`, `tunnel`, `G10` service scripts, `config.example.json`, release workflows or `rc/0.9.0-rc.1`.
- Windows `go test ./... -count=1`, `go vet ./...` and concurrent Group MCP stress run pass.
- Native GitHub CI on **this integration head**: Windows/Linux/macOS tests including compiled-worker smoke, race, six cross-builds, required aggregate.
- All original PRs remain reviewable in Git history. Merge the consolidated PR into `main` only after these conditions pass; then record original PR #20/#22/#24/#25/#26/#27/#28 as superseded/integrated.
- Preserve `0.9.0` RC HOLD and keep production feature flag disabled.

## Preparation of a production Go-only candidate

A tested Go binary and SHA256 manifest may be staged in a separate private versioned path under `%USERPROFILE%\.codexify-go\`. It must **not** overwrite the current Go executable, change SCM ImagePath, modify scheduled tasks, touch the tunnel, start a second production worker or change any user checkout.

For the later approved Go-to-Go operator transaction:
1. Freeze original Go service binary/profile/config hashes and original SCM/task manifests.
2. Verify an independent Go-only previous-binary recovery procedure before the cutover.
3. Keep the new feature flag OFF in the initial staged config. Do not select a Project Group in the real user conversation until schema-refresh acceptance.
4. Require native compiled smoke, actual host identity for scoped resources, no HH/Habr MCP burst, and a verified 24h uninterrupted G10 soak.
5. 403/429/503 from VPN/control-plane alone is NOT authorization for Rust rollback.

## Gate ledger

PG-I1–PG-I6 implementation and isolated native worker verification are complete pending final integrated-main CI. PG-A00–PG-A08 remain **PARTIAL/NOT RUN** for operator production acceptance; native smoke must not be mislabeled as SCM/launchd/systemd or actual ChatGPT-host acceptance.

**G10 decision:** inspect the current run's `probe-observations.jsonl` at the time of operator review. A fresh `CONTINUE` is not sufficient if the 24-hour window has a reboot/gap or any unresolved `HOLD`. `Assess-ProjectGroupsCanaryReadiness.ps1` is the non-mutating gate.
