# Linux Codexify Go: Setup parity candidate (2026-10-09)

## Scope and status

Candidate branch: `codex/linux-setup-parity-20261009`, based on
`codex/windows-cutover-g03`. No production binary, systemd service, tunnel,
Rust fallback, or existing local worktree was modified by creating this branch.

**Status: LINUX UNIT/REGRESSION VERIFIED; NOT DEPLOYED OR LIVE-ACCEPTED.**
The candidate was fetched via the verified `upstream` remote into the isolated
Linux worktree `.codexify-go/worktrees/linux-setup-parity-test-20261009` at
commit `b37ec0e` (branch `codex/linux-setup-parity-test-20261009`). Evidence
returned by the Linux tool in this chat:

- `node internal/ui/widget_runtime_test.mjs --source internal/ui/resources.go`: PASS.
  SetupHTML had zero automatic initial calls, host notifications caused zero
  new calls, and setup context/error test cases passed.
- `go test -count=1 ./internal/mcpserver ./internal/ui ./internal/projects`: PASS.
- `go test -count=1 ./...`: PASS across all packages.
- `go vet ./...`: PASS.
- `go test -race -count=1 ./internal/mcpserver ./internal/projects ./internal/ui`: PASS.
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath ...`: completed;
  candidate identifies itself as `0.8.2-dev`, SHA256
  `baf8daa4f2541bd394120c32ec6a582e210bceaac0b0b9709998ac42bf9d96fa`.

The **live** connector/card, systemd service restart, binary swap, host UI
flicker and independent-conversation isolation have NOT been acceptance-tested.
Linux MCP intermittently returned `RATE_LIMITED`; do not treat that as a unit
regression or as evidence of hosted UI success.

## Observed discrepancy

- On the actual Linux checkout, `origin` is `git@github.com:1AAk/codexify-go.git` and `upstream` is `https://github.com/benice2me11/codexify-go.git`; this candidate must be fetched from `upstream`, not `origin`.
- The earlier Linux checkout uses Setup resource `v1`, while the GitHub
  `main` resource is `v2` and the Windows cutover is `v5`.
- Linux source checkout `main` was observed at `10e437e`, 21 commits
  behind GitHub `main` (`e3e9ca1`) in the verified comparison.
- Windows cutover adds a model-visible `setup` tool, app-callable project
  selectors, robust error display, in-flight request guards, a host-event loop
  regression harness, and verified `uiContext` delegation for widget calls.
- The old Setup catches errors via `String(e)`; an object error can surface
  as `[object Object]`. Windows code extracts MCP `isError` content and
  structured error messages.
- Windows G05 history documented failed hosted UI selection when
  `openai/session` was missing from widget calls. The source now has an
  opaque, bounded, server-issued setup context; live Linux acceptance remains
  mandatory before calling this fixed.
- Previously mounted Linux cards may still request
  `ui://codexify-go/setup/v1/mcp-app.html`. The Windows candidate served
  only the cached `v2` alias. This Linux candidate adds the `v1` alias and a
  regression test, while new cards use `v5`.

## What was changed on the candidate

- `internal/mcpserver/server_test.go`: verify the `v1` URI reads the current
  Setup widget.
- `internal/mcpserver/server.go`: serve the `v1` compatibility resource.

All remaining Windows Setup/context/host-event tests come from the candidate's
Windows-cutover base; no change to MCP authentication or identity keys.

## Linux validation procedure (do not deploy as part of these commands)

Use the actual Linux Codexify repository and a **new isolated Git worktree**.
Do not merge or `git pull` into the running source checkout merely to test
this candidate. Before creating the worktree, confirm the intended directory
is ignored and no same-name branch/worktree exists.

```bash
cd /home/whtvr/codexify-go
git status --short --branch
git worktree list --porcelain
git check-ignore -v .codexify-go/worktrees
git fetch --no-tags upstream \
  +refs/heads/codex/linux-setup-parity-20261009:refs/remotes/upstream/codex/linux-setup-parity-20261009
# Only when path and branch are available and safely isolated:
git worktree add -b codex/linux-setup-parity-test \
  .codexify-go/worktrees/linux-setup-parity-test \
  refs/remotes/upstream/codex/linux-setup-parity-20261009
cd .codexify-go/worktrees/linux-setup-parity-test
go version
node --version
go test -count=1 ./internal/mcpserver ./internal/ui ./internal/projects
node internal/ui/widget_runtime_test.mjs --source internal/ui/resources.go
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./internal/mcpserver ./internal/projects
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -o /tmp/codexify-go-setup-parity ./cmd/codexify-go
```

If the GitHub remote branch is absent, fetch fails, or tests fail, **stop**;
do not switch the installed service to the candidate. Avoid copying the
Windows SCM, PowerShell, task scheduler, or Windows path-normalization
procedures into Linux service management. Windows branch and current `main`
have diverged; reconcile that separately before a mainline merge.

## Hosted Linux acceptance

1. In a new conversation, the model can call `setup` and the latest Setup
   card renders without a request loop. The visible card uses Setup `v5`.
2. Existing cached `v1` and `v2` resource URIs both resolve to the current
   card with the expected MCP App MIME type.
3. Clicking `Switch project` once makes at most one mutation call and
   displays a model-visible (not raw Object) diagnostic on any rejected
   response; failures do not trigger a second mutation.
4. A selects `mikrotik` via Setup UI; its workspace resolves to
   `/home/whtvr/mikrotik`. Another conversation B keeps its original
   workspace. A cannot submit B's setup context. No binding is overwritten
   across conversations.
5. `get_environment`, `list_projects`, `list_worktrees`, `git_status`,
   and a bounded read-only `exec_command` work before and after connector
   refresh/reconnect; tools are not missing or listed with stale argument
   schemas. Record any intermittent `RATE_LIMITED` separately from UI bugs.
6. New and previously open cards neither flicker nor produce `[object Object]`
   during a controlled failing MCP call, a successful selection, and a
   reconnect. Inspect the actual client UI; synthetic Node tests alone do not
   establish this acceptance.

## Deployment / rollback gate

The installed Go service and tunnel must remain untouched until tests and a
controlled acceptance window are available. Before swapping anything:
record `systemctl --user cat`, `systemctl --user show`,
`readlink -f /proc/<MAIN_PID>/exe`, binary/config SHA-256 hashes,
the current worktree and project binding, the independent Rust fallback,
and the tunnel owner's PID/parent chain. Preserve exact bytes in a
separate private backup. Validate that the new binary can launch with
the current Go config without exposing credentials.

Only then use a controlled service restart, verify one Go owner/one tunnel,
MCP readiness and conversation bindings, and roll back to the saved binary
if any gate fails. A binary built in `/tmp` is not an installed update.
Never claim Windows G05 is a live Linux PASS without the Linux hosted tests.
