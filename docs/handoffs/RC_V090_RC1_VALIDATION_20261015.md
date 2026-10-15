# v0.9.0-rc.1 — build and archive verification (2026-10-15)

**Repository:** `benice2me11/codexify-go`  
**Pinned release-candidate source:** `1ec1bfc0ca1d291dc13df975437636d6f1c3a81c`  
**RC branch:** `rc/0.9.0-rc.1`  
**Workflow:** [Release Candidate run 38729767398](https://github.com/benice2me11/codexify-go/actions/runs/38729767398)  
**Scope:** BUILD_AND_ARCHIVE_PASS; G10/G11 and host runtime acceptance **PENDING**.

## CI and package gates

- Workflow version validation, native Windows/Linux/macOS Go tests and vet: **PASS**.
- Builds for `windows/linux/darwin × amd64/arm64`: **6/6 PASS**.
- Packaged amd64 smoke on Windows, Ubuntu and macOS: **3/3 PASS**.
- Final `required` gate: **PASS**.
- Six GitHub Actions `rc-<os>-<arch>` ZIP artifacts retrieved and independently opened. Each outer ZIP contains exactly one inner OS-specific release archive and its SHA256 checksum file. Artifacts expire on 2027-01-13 per GitHub.
- Verified that all six inner archives match their provided `.sha256` data exactly. Extracted content is exactly one named binary; Windows binaries have PE headers, Linux ELF, Darwin Mach-O. Additionally executed isolated Linux amd64 binary; `codexify-go version` returned `0.9.0-rc.1` with exit status 0.

## Independently verified inner archive SHA-256

| OS/ARCH | Archive | SHA-256 |
| --- | --- | --- |
| windows/amd64 | `codexify-go-v0.9.0-rc.1-windows-amd64.zip` | `995e4688830aafe681142e9501bff9d395da4f7e4f2cf0788cba5ac0d2fe281f` |
| windows/arm64 | `codexify-go-v0.9.0-rc.1-windows-arm64.zip` | `7d9f6f2c9b35c19e3c141aec64f6f2c85e4fd12a90dbc16d3ea0923517cb6ae8` |
| linux/amd64 | `codexify-go-v0.9.0-rc.1-linux-amd64.tar.gz` | `d63db1595a72f9ce0cc0f7e21fd1b3d80dc624ffc8611f1cc8f61da6d63b784a` |
| linux/arm64 | `codexify-go-v0.9.0-rc.1-linux-arm64.tar.gz` | `fe62e679667b3a8c42251814427bf273f978d1d0bf0fd18bd0a33694b3ab206f` |
| darwin/amd64 | `codexify-go-v0.9.0-rc.1-darwin-amd64.tar.gz` | `1a1004b4fdd35b51759300a581163833e7bf34fe2106734ef20da22689d2a77d` |
| darwin/arm64 | `codexify-go-v0.9.0-rc.1-darwin-arm64.tar.gz` | `f9bb9a8663bab54300aadb4836882c1aebfe3c578671d190b9d2b0f98ba69c15` |

These are SHA-256 of the **inner archive**, not the GitHub artifact wrapper ZIP. Checks on arm64 confirmed file format and checksum; arm64 binaries were not executed. RC publishes Actions artifacts, **not a stable GitHub Release**.

## Repository consolidation completed during R00–R04

- Historical squash-merged PR branches: **10/10** changed-file parity verified against merge commits at exact heads; documented in [BRANCH_RECONCILIATION_LEDGER_20261015.md](BRANCH_RECONCILIATION_LEDGER_20261015.md).
- [#18](https://github.com/benice2me11/codexify-go/pull/18) reconciled branch/worktree inventory (squash `65c2f41`).
- [#19](https://github.com/benice2me11/codexify-go/pull/19) added legacy Linux Setup v1 URI support alongside v2/v3 and error-regression tests (squash `d83964d`; green CI).
- [#21](https://github.com/benice2me11/codexify-go/pull/21) selectively forward-ported duplicate Go-process detection into **Go-only** watcher, without re-enabling Rust fallback (squash `1ec1bfc`; green CI).
- Historical divergent [#3](https://github.com/benice2me11/codexify-go/pull/3) was **closed without merge** as source integration superseded, with explanation and preserved fork branch. Real Linux host acceptance still pending.
- All 14 **initial** origin refs preserved; four new integration/RC refs were created, alongside a concurrent, unrelated Project Groups branch. A fresh inventory at end of RC work found 19 remote branches including `main`. No branches or old worktrees deleted.
- The Windows source checkout was independently modified for concurrent Project Groups work; reconciliation commits were made through isolated branches/worktrees and GitHub. That in-progress work was not included in pinned RC.

## Operations and release HOLD

- The Windows Go-only G10 observation started **2026-10-15 10:26 UTC** and cannot meet 24h before **2026-10-16 10:26 UTC**. An early read-only sample window had 455/455 `CONTINUE` observations and a maximum 11.5 s sampling gap, **not a G10 PASS**.
- G11 is not accepted. Real Linux `systemd --user` and macOS `launchd` host lifecycle tests cannot be claimed from CI. Linux/macOS connectors were unreachable during this audit.
- Full cached Linux v1/v2/v3 Setup cards and independent-chat host acceptance are still pending.
- Historical Windows cutover worktrees and rollback checkpoints are retained; one historical cutover worktree is DIRTY. Do not clean/prune it.
- Project Groups is **not included** in RC v0.9.0 scope; separate PR #20 has independent lifecycle.
- Stable `v0.9.0` release needs explicit approval and evidence for remaining gates. `release.yml` publishes automatically on the stable tag; **DO NOT CREATE `v0.9.0` TAG** on the strength of this RC alone.
- Production Windows service was not updated to the RC binary; live G10 watcher/SCM/tunnel were not restarted as part of this work.

**Decision:** RC archive acceptance: **PASS**. Stable release approval, operational acceptance, and remote branch deletion: **HOLD**.
