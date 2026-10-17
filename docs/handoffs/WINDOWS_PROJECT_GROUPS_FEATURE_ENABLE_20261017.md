# Windows Project Groups: controlled feature-flag canary (2026-10-17)

**Result:** `PROJECT_GROUPS_ENABLED` on the existing Windows Go binary. This is **feature activation and initial hosted Setup v4 smoke**, not PG-A00..PG-A08 acceptance, G11-05 acceptance, or a stable v0.9.0 release.

## Authorized operation

On 2026-10-17 the user explicitly requested enabling Project Groups for testing. The Windows source checkout was selected with `createWorktree=false`; it remained the bound workspace.

- Go binary SHA256 remained `F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2` (compiled from `cd6b5bd59dea...`).
- Original G11 normal config SHA256: `76749955DC6E8D73B68C44E11474F15055910070AF0B67F739CB39A5E28794BD`.
- New Project Groups config SHA256: `694B2406C2AECDDC787FF41D84F1FB900D16E03BC686B2ACEF118F73FC39B0A6`.
- Byte-for-byte delta: the single `mcp.projectGroupsEnabled` value `false` → `true` (one byte shorter). `mcp.diagnostics.enabled=false` is unchanged. All upstream, tunnel, authorization, group membership and workspace configuration are unchanged.
- Configured group: `job-search`, primary `work`, with `work=work-search`, `hh=hh-mcp-server`, `habr=habr-career-mcp`. All three have `checkout=source`. Filesystem roots were checked to exist.
- Existing previous-Go-only recovery executable SHA256: `8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880`, unchanged. There is no Rust fallback.

## Operator execution

The one-shot operator code is tracked in `tools/windows/g11/Invoke-G11ProjectGroupsCanary.ps1`. This code is intentionally not run as SYSTEM from the mutable Git workspace: it was copied and hash-verified in an ACL-restricted ProgramData directory, together with a separate immutable canary input profile. The staged candidate itself and raw production configs were never committed.

Preflight:
- Go supervisor + worker + managed tunnel ownership: 2 Go / 1 tunnel / 0 Rust.
- Windows service Running/Automatic, approved Go and normal config hashes, `/health=200`.
- SCM restart after 5/15/60s; G10 disabled; G11 the sole recovery-armed monitor at `CONTINUE`; preserved previous-Go recovery assets.
- Approved G11 runtime pins SHA256 `03FBB74A20D4BAFD12F40D3353A075D9BC253C7E255A73EC143B6E035A405EB7`.
- Original plan-only operation returned `PLAN_ONLY_CANARY_READY`. Go tests passed: `go test ./internal/config ./internal/projects ./internal/mcpserver -count=1`; PowerShell AST parsing returned zero errors.

The independent on-demand SYSTEM task `CodexifyGo-PGEnable-20261017` had no automatic triggers, was single-use, and completed at **2026-10-17 09:10:08 UTC** with Task Scheduler result `0`. It pinned the enabled config and a corresponding previous-Go-only recovery profile **before** updating SCM. It performed a controlled same-Go stop/start, then required the new owner, one worker, one owned tunnel and loopback health. On local failure the script would restore the exact previous **same-Go normal** config.

Observed:
- Result `PROJECT_GROUPS_ENABLED`, original supervisor PID `14400`, new supervisor PID `15028`, worker PID `17100`, tunnel PID `5772`.
- 2 Go / 1 tunnel / 0 Rust, `/health=200`.
- G11 now has three pinned recovery profiles (original diagnostics, normal diagnostics-off, Project Groups enabled), and G11 reports `CONTINUE / within_limits` with last scheduled result 0. New pins SHA256: `2C293731898801DA9C6BBFC473F257279B366511BF9C390AD0A9CF46623481BF`.
- One-shot task was subsequently **Disabled**, retaining its protected evidence and registration. Rust/G10 tasks remained Disabled. No second supervisor/tunnel/watchdog was started.
- Production state and rollback evidence are preserved privately under `C:\ProgramData\CodexifyGo\G11\project-groups-canary-20261017\`. Never commit or share raw config, approvals, logs with credentials, or full binding records.

## Hosted Setup v4

Real `Windows.setup` returned:
- `projectGroupsEnabled=true`, `groups=[job-search]`, primary `work`, and canonical project roots for the three source checkouts.
- Advertised/observed connector version `0.8.2-dev+a1aad6a44063`; after supplying that exact version, `connectorSchema.status=current`, `conversationStale=false`, `refreshRecommended=false`.
- The existing `codexify-go` conversation stayed bound to its exact original source checkout; no automatic move to `job-search` or worktree creation occurred.

Read-only Git inspection: `hh-mcp-server` and `habr-career-mcp` were clean on `master`; `work-search` was on `main` with **17 existing dirty files**. Do not reset or edit these files as part of PG testing.

## Remaining acceptance

1. Open a **new or refreshed ChatGPT conversation** using the latest Windows connector schema; select `job-search` explicitly in the project group picker. Do not repurpose the currently bound source checkout without deliberate switching.
2. In the group, exercise alias selection and read-only `git_status`, `get_agent_brief`, context for `work`/`hh`/`habr`; confirm all operations stay in their own repo.
3. Validate source/managed mode switches only in approved/disposable worktrees and with explicit safeguards for dirty files. Confirm group binding persistence through further normal restarts **in a separately approved maintenance window**. Test scoped artifacts and deny foreign conversation access.
4. Check authorized read-only upstream calls for HH/Habr and watch for MCP duplicate bursts. Do not send applications or enable Vacancy Hourly Watch.
5. Retain `PG-A00..PG-A08` as PARTIAL/NOT FULLY ACCEPTED and keep G11-05 / stable v0.9.0 separate. No strict new Go 24-hour G10 acceptance is implied.

The test can be rolled back to the previously verified **same-Go** normal config if needed; the disabled one-shot task is not a rollback button and must not be rerun. No Rust fallback.
