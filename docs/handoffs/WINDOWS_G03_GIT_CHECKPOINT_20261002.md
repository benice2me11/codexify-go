# Windows G03 Git checkpoint - 2026-10-02

The user authorized an isolated worktree and thematic local commits after G03.
This checkpoint versions the source and dated gate records already validated.
It does not execute G04-G11, restart a service, deploy a binary or publish a branch.

## Working location and commits

Continue repository work on `codex/windows-cutover-g03` at:

```text
C:\Users\FoxOS_User\codexify-go\.codexify-go\worktrees\g03-checkpoint-20261002
```

The base is `1098803bb3861e3f5d1a2702e4f5fda42afe0546`. The older
`cutover/windows-v2-minimal` worktree contains a different development history
and its own pending changes; it was not merged or modified.

| Commit | Scope |
| --- | --- |
| `eb29089f58cfa23020d76605cd4ec5be523f5632` | Exact source of the frozen Windows Go candidate; 31 changed/added files |
| `064664e92bc80a38307c55ad4e1665edfb2bb072` | Seven frozen operator-tooling-v3 scripts |
| `dc86f0b913a98935868a498192edce8ca98bee94` | G00/G01 criteria, candidate metadata, guard correction and completed preparation record |
| `58b85edb5d0dfe4a67ce971b37ca081c56aef2f5` | G02 live rollback and Go re-entry record |
| This checkpoint's commit | G03 client report and source-provenance manifest |

All commits are local. The original `main` checkout remains on its original
commit with its existing modified/untracked files. Both earlier checkouts retain
their captured file hashes, index, branch, HEAD and status. Copies were preserved
in place; nothing was stashed, reset, deleted or blindly staged.

Repository commands use this worktree explicitly. Existing hosted conversation
bindings were not changed by the Git operation. This local worktree creation is
not evidence that the hosted G05/G07 acceptance workflows have passed.

## Exact candidate provenance

Candidate: `20261001T160710Z-node-repl-compat`.

Frozen executable SHA256:

```text
61E2A086445A1B10E7537E720119B0855E17515A58A5B3F8BDCA914075B07DD6
```

All 139 files in the frozen source manifest were checked against their recorded
SHA256 and length. All 108 non-documentation files match the frozen snapshot in
both the worktree and the source commit's Git blobs, byte-for-byte. The seven
operator scripts likewise match the frozen v3 files and their committed blobs.
The existing `.gitattributes` may materialize `.ps1` files with CRLF on a future
checkout; use the committed blob bytes when checking these recorded hashes.

The [provenance manifest](evidence/WINDOWS_G03_GIT_CHECKPOINT_20261002.json)
records the file hashes, relevant commits, original source-manifest hash,
build settings and validation results. Private profiles, credentials, raw
traces, recovery snapshots and old baseline captures are not included in Git.

The original executable was built with `-buildvcs=false`; these new commit IDs
were not embedded in that earlier binary. A fresh build from this worktree with
Go `go1.26.5`, `windows/amd64`, `CGO_ENABLED=0`, `-trimpath` and
`-buildvcs=false` produced exactly the same executable SHA256. The rebuilt copy
was retained privately and was not installed or executed.

## Fresh checks for this checkpoint

- The clean base passed `go test ./... -count=1` before source import.
- The imported candidate passed `go test -json ./... -count=1`: 155 tests,
  27 packages, zero failures. `TestResolveRejectsSymlinkEscape` was skipped
  because Windows symlink privileges were unavailable; it is not counted as a pass.
- `TestWidgetsDoNotCallToolsFromHostGlobals` passed with Node `v24.19.0`.
- `go vet ./...` and `go mod verify` passed.
- Native build passed and was byte-identical to the frozen candidate.
- Windows PowerShell tooling, ten-case log compatibility and watcher lifecycle
  fixture suites passed. Their harmless fixtures do not demonstrate live rollback.
- Staged whitespace checks and repository Markdown link checks passed.
- An independent read-only review found no important or critical source
  checkpoint issue and confirmed exact scope and original-checkout preservation.

No new race-detector or hosted-runtime acceptance claim is made by these checks.

Private local verification evidence is retained under the candidate run at
`evidence/git-checkpoint-20261002T151449Z`. It includes before/after preservation
checks, source/tooling selections, command logs, build output and commit records.

## Gate status and runtime boundary

G00-G03 retain their historical PASS results. G04-G11 remain PENDING.

- [G00 candidate metadata](WINDOWS_CUTOVER_V2_G00_G01_20261001.md) describes
  the initial operator boundary; later preparation completed G01.
- [G01 preparation record](../superpowers/plans/2026-10-01-windows-g02-preparation.md)
  records the completed readiness work and its original authorization boundary.
- [G02 live record](WINDOWS_G02_LIVE_REPORT_20261002.md) records the real
  Rust-to-Go-to-Rust-to-Go rehearsal.
- [G03 client record](WINDOWS_G03_CLIENT_REPORT_20261002.md) records the two
  hosted conversations, current-schema evidence and operator-confirmed widget.

The dated reports preserve their original observations. In particular, the
earlier G00 resource-encoding error is corrected by the retained G03 erratum;
the `update_plan` capability difference remains open before final acceptance.

At the read-only check on **2026-10-02 at 15:21:19 UTC**, `CodexifyGo` was
already **Stopped / Manual / PID 0**, with zero Go processes and the configured
path still selecting the same frozen candidate and capture profile. This is
separate from the successful historical G03 runtime checks. The reason for the
later stopped state was not investigated by this Git task. No service start,
stop, restart, rollback, profile change or ownership change was performed.

Before resuming live acceptance, establish current ownership, recovery readiness
and observation coverage again. A historical G03 PASS or a clean Git worktree
does not establish present runtime readiness.
