# Windows G05 isolation preparation

Status: PREPARED, not a G05 PASS. The operator authorized G05 after the
[G04 result](WINDOWS_G04_TOOL_STATE_REPORT_20261002.md). Live execution needs
two real conversation identities, an available workspace-switch UI, and a quiet
test window. Client choice and renewed quiet-window confirmation are pending.

The candidate is still `20261001T160710Z-node-repl-compat`, executable SHA-256
`61E2A086445A1B10E7537E720119B0855E17515A58A5B3F8BDCA914075B07DD6`.
Preflight found Go Stopped/Manual, no connector processes, and all eleven
Codexify scheduled tasks disabled. Windows records a user power-off at
17:02:55Z and boot at 17:19:25Z on 2026-10-02. No G05 service start, binding
change, hosted call, managed-worktree creation or cleanup has happened yet.

## Prepared fixtures and evidence

Two new repositories contain only `README.md`, `marker.txt` and a fixture-specific
`only-a.txt` or `only-b.txt`. Both are clean, have no remote, and use a local
`codex/g05-fixture` baseline branch with `core.autocrlf=false`.

| Fixture | Canonical source path | Baseline commit |
| --- | --- | --- |
| A | `C:\Users\FoxOS_User\codexify-g05-fixtures-20261002T174437Z\project-a` | `20a9b7ba8e631105d1a61b3694c9bd56f094d080` |
| B | `C:\Users\FoxOS_User\codexify-g05-fixtures-20261002T174437Z\project-b` | `fb10622fa4d8eae42d39d644d5e150fdd8cd5992` |

The marker contents differ only in the fixture identity:

```text
fixture=A
run=G05_20261002T174437Z
```

Fixture B says `fixture=B`. The normal configured project scanner can discover
both repositories at depth two without changing the frozen profile.

Private evidence is retained in:

```text
C:\Users\FoxOS_User\.codexify-go\cutover-v2\20261001T160710Z-node-repl-compat\evidence\g05-live-20261002T174437Z
```

`fixture-baseline.json` records paths, extended-length path spellings, commit IDs
and marker hashes. `preflight.json` records SCM/process state, host events and
four frozen asset hashes. `go-state-before.json` and `go-state-before/` retain
26 Go binding/schema/project-state files with verified copy hashes.

The G04 Rust recovery snapshot is reusable: all 51 snapshot files validate and
all 48 retained live Rust files still match. Its established task-restoration
policy remains documented in the G04 report. No recovery copy was overwritten.

Private G05 copies of the validated controller, serialized rollback wrapper and
passive sampler use a distinct mutex and output paths. Their preparation checks
pass: four ownership/readiness cases, absent-watcher cleanup, and actual wrapper
blocking/release in dry-run mode. The candidate, profiles and frozen v3 scripts
were not edited. The controller refuses execution while quiet-window
confirmation in `request.json` is false.

## Execution sequence

Activate one client at a time for each step and space hosted tool calls by at
least three seconds. Do not automatically retry a failed mutation or overlap
the two clients' setup sequences. Record explicit UI actions separately from
the model's tool requests and host metadata traffic.

1. Verify current ownership, recovery and fixture hashes again; record the
   selected two client conversations and quiet-window confirmation. Start the
   frozen candidate with fresh local observation and a bounded capture only
   when both clients are ready.
2. In each conversation, call `list_projects` with query
   `codexify-g05-fixtures-20261002T174437Z`. Preserve the real returned catalog.
   Existing conversation bindings must be reopened through the actual workspace
   switch UI before a different project is selected.
3. In A, select fixture A with `createWorktree=true`. Record the returned
   source path, exact new managed-worktree path, branch and binding scope.
   Do not infer that path from its naming convention. In B, select fixture B
   with `createWorktree=false`.
4. Both clients call `get_agent_brief`, `get_environment`, `read_file` on
   `marker.txt`, and a read-only command returning their actual working
   directory. Check A/B markers and canonical paths. Exercise the observed
   absolute path using its `\\?\C:\...` spelling where supported, and verify
   identity equivalence rather than string equality alone.
5. In B, start a disposable exec that reports its directory/marker, waits for
   one stdin line, and yields a session ID. Through the actual UI, switch A to
   fixture B and then back to A's recorded exact worktree with `resumePath`.
   Verify B's binding file/path/marker is unchanged; send its returned session
   ID the planned input and verify exact output from the same working directory.
6. Inspect A's managed worktree through `list_worktrees`, Git and marker hashes.
   Complete all exec fixtures before a planned same-profile restart. After the
   resulting transport reconnect, resume A's exact saved worktree and repeat
   both clients' binding/marker checks. Keep all successful calls inside fresh
   diagnostic windows; expired capture is not request evidence.
7. Reconcile client requests with decoded traces per capture and conversation.
   Any incorrect workspace or unaccounted mutation fails the run and triggers
   the accepted local recovery policy. Synthetic metadata identities or local
   loopback calls cannot substitute for the two real client conversations.
8. Retain the fixtures and exact worktree while the acceptance plan's mandatory
   post-G08/G09 binding checks remain outstanding. Afterwards, verify the path,
   repository ownership, baseline hashes and absence of user changes before
   removing only this fixture worktree and its test data. Preserve evidence.

The application intentionally exposes `setup_ui_switch_project` to its widget,
not the model tool catalog. Do not edit saved binding files or impersonate a
conversation to bypass that UI boundary. A client strategy that cannot operate
the actual switch UI cannot pass the switching check.

## Completion boundary

The [G05 requirements](WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md#g05---multi-project-and-worktree-isolation)
include repeated binding checks after G08/G09 recovery. Record the initial live
checks separately; do not mark full G05 PASS while those repetitions or safe
cleanup are still outstanding. G00-G04 retain their historical PASS results;
G05-G11 remain PENDING and the deployment is not accepted.

Repository records continue in the separate `codex/windows-cutover-g03`
worktree, beginning this preparation at `426b9e3`. The new fixture repositories
are disposable test inputs under the configured access root; user repositories
and their existing worktrees are preserved. No push is part of this preparation.
