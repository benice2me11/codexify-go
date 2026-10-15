# Branch reconciliation ledger — v0.9.0 preparation (2026-10-15)

**Scope:** `benice2me11/codexify-go`. **Snapshot:** 2026-10-15 11:27 UTC (14:27 Moscow). **Audit baseline:** GitHub/Windows `main=cbb665aa4e5c2655227882fc6094cc350c847358` (the original handoff's `7c66828` has since advanced via #17 and #16). **Reviewer:** source/CI/GitHub checks by continuation agent; human release and deletion approval **not granted**.

**Safety:** No branch has `DELETE_APPROVED`; no tags, releases, worktrees, Windows services, G10 guard tasks or rollback assets were modified for this audit. `MERGED_EQUIVALENT` is evidence of past merge equivalence, **not permission to delete**. A future deletion must revalidate SHA, PR/worktree/conversation dependencies and explicitly record approval.

## Evidence and verification method

- All 14 remote branches listed through GitHub `GET /branches?per_page=100` and Windows `git for-each-ref refs/remotes/origin`. Source checkout at `C:\Users\FoxOS_User\codexify-go` on clean `main`; `git status --short --branch` returned `## main...origin/main`. Local Go version `go1.26.5 windows/amd64`.
- For **each of 10 squash-merged PRs**, retrieved actual GitHub merged status, head SHA and squash/merge SHA, proved `git merge-base --is-ancestor <merge_sha> HEAD`, obtained exactly the paths touched by its squash commit with `git diff-tree --no-commit-id --name-only -r <merge_sha>`, then proved **zero blob differences** on those paths between that merge commit and the frozen remote branch head using `git diff --name-only <merge_sha> origin/<branch> -- <paths>`. This is patch/content parity at merge time; it avoids irrelevant divergence in old histories.
- Subsequent changes to those files in main are accounted for: #5 (README, UI resources, widget harness); #9 (README, server.go, server_test.go, UI resources); #11/#12 (CI workflow); #14 (transaction notes). They reflect later PRs, **not missing earlier patches**. Re-run native tests following any new integration.
- Windows `go test ./... -count=1`, `go vet ./...`, and `node internal/ui/widget_runtime_test.mjs --source internal/ui/resources.go`: **PASS** on `cbb665a`. GitHub main CI [run 38726760724](https://github.com/benice2me11/codexify-go/actions/runs/38726760724) completed **success** on exact SHA. Historical merged PRs #4, #5, #7–#12, #14, #15; subsequent #16 and #17 are also merged.

## Remote branch ledger (14 refs)

| Ref | Exact remote HEAD | Status | Evidence / decision |
| --- | --- | --- | --- |
| `main` | `cbb665aa4e5c2655227882fc6094cc350c847358` | PROTECTED_BASE | Clean, CI success. Never delete. |
| `codex/linux-setup-parity-20261009` | `df0180d0582ad59f49b47218cf4325f48f24afc0` | UNMERGED_REQUIRED / REVIEW | Linux Setup v5/legacy v1 candidate; main has v3 + v2 but **no v1 URI**. A minimal compatibility PR is required, not full branch merge. Preserve source/provenance. |
| `codex/windows-cutover-g03` | `6387f542fcc7c2bc3a236c065927264254611613` | FORENSIC_HOLD | Live-tested rollback/checkpoint; PollWatcher ported via #8, but branch/worktree must remain until G10/G11 accepted and recovery evidence archived. |
| `codex/windows-g10-guard-hardening-20261013` | `e4e36a7da59c13de7afaa162c83cca7bbc9c89d5` | FORENSIC_HOLD / REVIEW | +4 commits vs G03: legacy Rust-capable watcher log bounds, observer-failure handling, restart episodes and egress classification. Do **not** merge old rollback wiring. Audit selective Go-only parity separately. |
| `docs/g10-go-only-live-result-20261015` | `8d45f69c76e90993de72c578457ce7ebd735009a` | MERGED_EQUIVALENT / HOLD | #15 → `7c66828b`; 1 file, branch content identical to squash commit. |
| `docs/project-groups-plan-20261014` | `ed2f278bccd9e0189fd4f68689999e89f52709f2` | MERGED_EQUIVALENT / HOLD | #4 → `ae783fda`; 1 file, exact content parity. Project Groups is NOT implemented in v0.9.0. |
| `feat/g10-go-only-upgrade-20261015` | `2d236bf19560d63b110292206b6ba01254672a90` | MERGED_EQUIVALENT / HOLD | #14 → `1536b773`; 8 files, exact content parity at merge. Go-only recovery must remain independent of Rust. |
| `feat/g10-offline-live-inspector-20261015` | `ec72e4d2f88807bd7cfba8810a54c0c2a5a8c065` | MERGED_EQUIVALENT / HOLD | #11 → `9fa575c5`; 4 files, exact content parity. |
| `fix/g03-setup-context-parity-20261014` | `c9c37687393c560e4854e8834239495c910295c4` | MERGED_EQUIVALENT / HOLD | #9 → `8af149ce`; 8 files, exact content parity; subsequent v3 URI change in #10. |
| `fix/g03-tunnel-selfheal-main-20261014` | `7e92527f528fbb0378f7d0734d50b995c8835f3f` | MERGED_EQUIVALENT / HOLD | #8 → `7f456193`; 6 files, exact content parity. |
| `fix/setup-ui-version-20261015` | `14f19451e98a579ad62049c4628374bc14e7ee4d` | MERGED_EQUIVALENT / HOLD | #10 → `cdf8c98d`; 6 files, exact content parity. |
| `fix/windows-path-identity-20261014` | `170291dc51357d8d9b02e02d7ae931e4022875dc` | MERGED_EQUIVALENT / HOLD | #7 → `cbb49ac0`; 4 files, exact content parity. |
| `fix/workspace-mode-selection-20261014` | `716d117b0fb6e17fe486982719a03bd9b2a05011` | MERGED_EQUIVALENT / HOLD | #5 → `abb46f02`; 5 files, exact content parity; v3 contextual improvements later added. |
| `test/setup-v3-binary-smoke-20261015` | `9455910ffd1371b63e83df0259147bde28d9ea62` | MERGED_EQUIVALENT / HOLD | #12 → `637441ab`; 3 files, exact content parity. |

**PR inventory:** [#3](https://github.com/benice2me11/codexify-go/pull/3) is the **only open PR**, draft, head `1AAk/codexify-go:codex/linux-setup-main-integration-20261009` at `9cc7c302e65cbe6ce4212d8b4ed0cb910b506863`, divergent old base `e3e9ca1`, conflicting. The fork branch is **not** an origin ref and must not be deleted. #16 (handoffs, `cbb665a`) and #17 (Windows CI Node harness timeout, `4b2e5ba`) are now merged and already reflected in the baseline.

## Linux Setup PR #3: targeted delta

Current main has the contextual `setup` tool, server-side 30-minute verified `uiContext`, conversation isolation, Setup URI **v3**, compatibility resource URI **v2**, Source/Managed picker, no passive host-event tool calls, busy-click suppression, readable tool errors and Go/Node regression tests.

PR #3 additionally registers legacy `ui://codexify-go/setup/v1/mcp-app.html` (from Linux Setup v5) and tests that the v1 card serves current HTML. The relevant current code points are `internal/ui/resources.go` and `internal/mcpserver/server.go`/`server_test.go`. Port **only** v1 compatibility and an exact regression test from fresh main, preserving current v3 and v2 behavior. Remaining semantic parity must be reverified; full Linux/native host checks are pending. PR #3 remains HOLD until the minimal PR and acceptance are reviewed.

## Windows G10 branch: selective comparison

G10 hardening branch adds to **old** `Watch-Cutover.ps1`/`CutoverGuard.psm1`: bounded JSONL scans, egress-degradation HOLD, explicit observation-failure handling, Go process signatures, 30-second restart episode coalescing, tests, forensic handoff.

Current production Go-only `Watch-GoTrial.ps1` uses a separate `GoOnlyUpgrade.psm1` policy: Rust owners or unexpected image → BLOCKED; missing supervisor six polls → recover **previous Go**; missing worker or tunnel → HOLD; external HTTP 403/429/503 is **not a rollback condition**. Observer exception is already a warning + retry, not recovery. The new `Inspect-G10Runtime.ps1` reads bounded JSONL (tail 2500). Old process-signature episode logic and Rust rollback plumbing are not applicable to the Go-only observer; preserve historic branches until G10/G11. A follow-up may add explicit observer log-write preflight and fixture tests to Go-only code, separately, without touching the live task.

## Local refs and worktree protection

Windows has **18 local branch heads** (vs 14 remote), including local-only `backup/pre-github-reconcile-20260928` (`6dd4c4f`), `cutover/windows-v2-minimal` (`3fe1896`), two `codexify-go/*` conversation refs (both `1098803`), `local/g10-offline-inspector-20261015` (`ec72e4d`) and `feat/project-groups-config-registry-20261015` at baseline `cbb665a`. Keep all without a separate review and preserved conversation bindings.

Three additional Windows worktrees:
- `cutover-v2-clean-20260930` on `cutover/windows-v2-minimal` — **DIRTY**: four tracked script modifications and six untracked startup/handoff/test files. **DO NOT DELETE, CLEAN, RESET OR PRUNE.**
- `g03-checkpoint-20261002` on `codex/windows-cutover-g03` — clean, FORENSIC_HOLD.
- `g10-guard-hardening-20261013` on `codex/windows-g10-guard-hardening-20261013` — clean, FORENSIC_HOLD.

macOS and Linux local inventories: **BLOCKED** (both connectors returned `Tunnel-client has not been seen for 300 seconds`); cannot claim all-host reconciliation.

## Runtime and release gates

At 2026-10-15 11:27:37 UTC, read-only `Inspect-G10Runtime.ps1` with the **explicit** Go-only transaction evidence directory reported `LOCAL_OBSERVED`, running CodexifyGo PID 18952, expected binary SHA-256 `8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880`, 2 Go processes, one direct tunnel child, zero Rust processes, fresh `CONTINUE` guard (3 sec old). `CodexifyGo-G10Observers` task was Running; both Rust tasks Disabled. The inspector deliberately reports `acceptance=NOT_EVALUATED`. Without the transaction directory it reports guard not found, which is a diagnostic invocation issue, **not evidence of G10 failure**.

- **G10:** 24-hour observation starts 2026-10-15 10:26 UTC (13:26 Moscow); cannot be accepted before 2026-10-16 10:26 UTC; outages/observer gaps must be classified separately.
- **G11:** not accepted; Linux systemd --user and macOS launchd native host acceptance unavailable this session. CI does not substitute for these gates.
- **RC:** `.github/workflows/release-candidate.yml` supports `workflow_dispatch(ref,version)`; must explicitly use `version=0.9.0-rc.1`, verified main/PR SHA, package six GOOS/GOARCH binaries and validate archive hashes, smoke and retention. Not run.
- **Stable:** `.github/workflows/release.yml` publishes on a `vX.Y.Z` tag; `v0.9.0` **MUST NOT** be tagged without separate GO approval. Last releases `v0.8.1` and `v0.8.0` (2026-09-28). Stable should have **11 assets** (two Windows ZIP + four Unix TAR.GZ + four Unix legacy ZIP + checksums.txt).
- **Project Groups:** excluded from v0.9.0 scope.

## Gate status at this snapshot

| Gate | Current state | Next step |
| --- | --- | --- |
| R-A00 | PARTIAL | Windows and GitHub recorded; obtain native macOS/Linux refs. |
| R-A01 | PASS for the ten historical squash PRs | Validate after subsequent integrations; deletion still blocked. |
| R-A02 | REVIEW / UNMERGED_REQUIRED | Port and test v1 alias; examine #3; native Linux Setup acceptance. |
| R-A03 | REVIEW / FORENSIC_HOLD | Preserve G10 branch; assess selective Go-only observer safeguards. |
| R-A04 | BASELINE_PASS, FINAL_PENDING | Latest main Go/Node/CI green; rerun after new PRs. |
| R-A05 | G10_RUNNING, G11_PENDING | Do not release stable early. |
| R-A06 | NOT_RUN | Prepare candidate workflow when targeted PRs and CI accepted. |
| R-A07 | NOT_RUN | Requires separate release approval, tag, 11 verified assets. |
| R-A08 | BLOCKED | No DELETE_APPROVED refs; preserve local dirty worktree and checkpoints. |
