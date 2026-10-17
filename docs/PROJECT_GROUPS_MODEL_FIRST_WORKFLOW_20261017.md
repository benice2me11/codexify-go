# Project Groups: model-first routing and safe Git workflow

Date: 2026-10-17. Status: implementation candidate on
`feat/project-groups-model-first-20261017`; **not deployed to the Windows service**.

## Goal

A Project Group is a persistent working context for the model, **not** a
manual "Make active" repository switcher. A group has one primary/default alias,
but operations route explicitly by their `repository` argument.

Target Setup card when `job-search` is selected:

```text
Workspace                                       Project group

job-search
3 repositories · Primary: work

work-search                                    Source · main
hh-mcp-server                                  Source · master
habr-career-mcp                                Source · master

Advanced settings >
```

Branch labels reflect **actual local Git state**, not configured labels. The
compact card must not expose absolute paths, checkout overrides, or active
default buttons. Advanced holds these controls. Before group selection, the
picker supports Single repository / Project group / Scratch.

## Routing and task preparation

1. Select the group once, e.g. `set_project_group(name="job-search")`.
2. Inspect the affected members independently using their aliases
   `work`, `hh`, `habr`. Use `git_status(repository=...)`,
   `get_project_group_status()` and, where necessary,
   `exec_command(repository=..., cmd="git ...")`.
3. For a new feature, check the source and upstream, then **fetch the chosen
   remote** with an explicit `repository` argument. Fetch updates remote refs,
   but never resets or checks out the source working tree.
4. Call `prepare_repository_task(repository="hh", branch="fix/hh-api",
   baseRef="origin/master")`. The base must exist as a remote-tracking ref.
   The tool resolves its exact commit, creates a new feature branch in a fresh
   managed worktree, persists only the chosen member's active checkout, and
   returns source dirty/behind information. `activeRepository` is unchanged.
5. Edit/test/commit in the managed checkout via the same explicit alias. Each
   member gets independent feature branches, commits and PRs.
6. Review tests and PR integration individually. The tool does **not** perform
   an automatic merge, rebase, PR creation or force-push.
7. To finish after an ordinary Git merge, explicitly select that member's
   `source` checkout via `set_repository_checkout`. Fetch the intended
   integration branch, then call `finish_repository_task(repository="hh",
   baseRef="origin/master")`. Only a clean preserved worktree whose HEAD is an
   ancestor of that remote-tracking ref can be released and removed. The
   feature branch is retained. Squash/rebase merges and dirty tasks require
   manual review; they are never removed automatically.

## Safety guarantees

- `prepare_repository_task` never resets, switches, stashes, pulls, merges
  or writes source checkout content, regardless of dirt or upstream lag.
- Source `main`/`master` is not a valid feature branch name. A task must
  name a **new** branch and an **explicit remote-tracking base**; local HEAD
  cannot silently serve as a stale base.
- Existing managed checkouts are preserved and not replaced. A member can
  hold one pending managed task in this revision; attempting another before
  explicit finish is rejected. The other group members continue normally.
- Generation and checkout revision advance when one member changes checkout,
  maintaining existing ownership checks for artifact/exec sessions.
- The cleanup protocol persists the source binding before removing a worktree;
  a filesystem removal failure leaves an orphan visible to
  `list_worktrees`, never a binding pointing at a deleted checkout.
- Cleanup uses `git worktree remove` **without `--force`**, rejects ignored
  as well as uncommitted files, and does not delete the feature branch.
- All new tools are registered only with `projectGroupsEnabled=true`.
  Legacy single-repository workspace selection and its v3 Setup resource
  remain available.
- Arbitrary `exec_command` operations retain OS-user permissions and are
  **not sandboxed**. Model guidance and purpose-built tools reduce risk,
  but cannot guarantee that an arbitrary shell command is safe.

## Validation and deployment

Run `go test ./internal/projects ./internal/mcpserver ./internal/ui
./internal/connectorschema`, then `go test ./...` and `go vet ./...`.

The new group Setup resource is `ui://codexify-go/setup/v5/mcp-app.html`;
v4 remains readable for already-mounted cards, and the connector schema
version changes. A connector refresh/reconnect may be required after a later
approved binary cutover.

**No live service restart, tunnel modification, config switch or work-search
checkout update is part of this implementation.**

## Follow-up gaps

- Confirm on the actual `job-search` group in a controlled canary. The user
  reported dirty `work-search` source checkout, 9 commits behind upstream;
  re-check Git live, never assume its current state.
- Add multiple concurrently resumable managed tasks **within one** repository
  instead of the current one-preserved-task guard.
- Design an explicit reviewed integration flow for squash/fast-forward/normal
  PR merges, with GitHub PR verification and authorized cleanup.
- Run interactive Windows, macOS and Linux UI/connector smoke testing before
  replacing the running service; assess setup widget rendering at narrow widths.
