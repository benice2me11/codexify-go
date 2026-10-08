# codexify-go

Experimental Go implementation of the Codexify runtime architecture, with
native OS service lifecycle and supervision of OpenAI's official Secure MCP
Tunnel runtime.

The project is inspired by
[devnoname120/codexify](https://github.com/devnoname120/codexify), which is MIT
licensed. This repository is an independent implementation, not a line-by-line
port and not yet a drop-in replacement.

## Current status

Implemented:

- native Windows Service Control Manager (SCM) integration;
- native Linux per-user systemd service integration;
- automatic Windows service startup;
- SCM recovery actions at 5s, 15s and 60s;
- no Task Scheduler or PowerShell in the service lifecycle;
- supervision of the official `tunnel-client-runtime`;
- exponential child restart backoff;
- health monitoring through the tunnel runtime `health.url` + `/readyz`;
- stale health-file removal before every tunnel start;
- controlled process-tree shutdown with Windows fallback to forced termination;
- `CREATE_NO_WINDOW` for child processes;
- JSON structured logging;
- foreground mode for debugging;
- `doctor` checks;
- unit and Windows process integration tests;
- official MCP Go SDK (`modelcontextprotocol/go-sdk` v1.7.0);
- stateless Streamable HTTP MCP transport with current-protocol negotiation and
  backwards compatibility;
- localhost-only MCP endpoint validation and request-size limits;
- generated per-process bearer authentication between the tunnel runtime and
  local MCP server (the secret is not stored in config);
- `/health` endpoint;
- workspace-root confinement, including traversal and symlink-boundary checks;
- `get_environment`;
- `read_file` / `write_file`;
- recursive `glob` and regex `grep`;
- `exec_command` with long-running sessions and `write_stdin`;
- `git_status`, `git_diff`, and `git_log`;
- Windows user-context worker launched from the SCM service with
  `CreateProcessAsUser`;
- active WTS session discovery (console or RDP-compatible active session);
- worker Job Object with `KILL_ON_JOB_CLOSE`, so worker child processes do not
  survive service shutdown;
- independent worker and tunnel supervision/restart loops;
- MCP tools and shell commands execute as the logged-in user rather than
  `LocalSystem`;
- multi-project catalogue below one configured access root, including explicit
  project metadata and bounded filesystem discovery;
- durable ChatGPT conversation binding keyed by a SHA-256 digest of
  `_meta["openai/session"]`; the raw conversation id is never persisted;
- `list_projects`, `set_project_root`, and `list_worktrees`;
- immutable per-conversation project selection with idempotent repeated
  selection;
- managed Git worktrees with `auto`, `always`, and `never` policies; `auto`
  isolates the second active conversation that selects the same source project;
- upstream MCP aggregation for stdio and Streamable HTTP servers;
- upstream `catalog` mode through `mcp_list_sources`, `mcp_search_tools`,
  `mcp_get_tool`, and `mcp_call_tool`, plus `direct` mode using
  `<source>__<tool>` names;
- optional/required upstreams, environment/header injection, and bounded
  upstream call timeouts;
- safe HTTPS/SSH repository selection with local-checkout reuse and private
  cloning below the access root;
- exact GitHub HTTPS branch, pull-request, and full-commit target selection;
- explicit `setup_ui_switch_project`, persistent scratch workspaces, and
  validated `resumePath` continuation of previously saved workspaces;
- project-scoped durable memory through `remember`, `recall`,
  `update_memory_note`, and `forget_memory_note`;
- repo/user skill discovery through `skills_list` and package-confined
  `skills_read`, including `agents/openai.yaml` implicit-invocation policy;
- `export_host_file` with opaque capability URIs, immutable snapshots when they
  fit the configured budget, and bounded source-backed fallback references;
- opaque proxy resources for `ResourceLink` values returned by upstream MCP
  tools, with downstream `resources/read` forwarding and size limits;
- compact MCP App resources for workspace setup/status and `git_diff`, using
  standard `ui/resourceUri` and OpenAI output-template metadata;
- `get_agent_brief` / `get_project_doc` with environment, saved memory, skill
  catalogue, and `AGENTS.override.md` / `AGENTS.md` discovery from repository
  root to the active workspace; project instructions are rendered last;
- workspace-confined `apply_patch` using Codex patch grammar with full
  preflight/context validation before the first write;
- checkpointed `show_diff` with immutable `project_open` and incremental
  `last_diff` baselines, tracked/untracked/deleted state, bounded patch metadata,
  and a private diff cursor that does not modify the user's Git index;
- secure `import_host_file` for host-authorized ChatGPT/native file references:
  HTTPS-only downloads, host allowlist, redirect/DNS/private-IP checks, byte and
  timeout bounds, and no overwrite of existing workspace files;
- hybrid MCP transport: current `2026-07-28` clients use stateless requests,
  while legacy/stateful clients without `openai/session` receive an isolated
  transport-session workspace binding that is forgotten on disconnect;
- upstream MCP `gateway` mode: one compact gateway tool plus a generated
  `SKILL.md` containing the upstream function list and exact input schemas;
- tunnel-scoped connector schema markers surfaced by `setup_status`, including
  per-conversation stale-version detection for setup/reload UX;
- installed Codex plugin skill discovery from `CODEX_HOME`/`~/.codex/config.toml`
  and the enabled plugin cache, including plugin manifests, active-version
  selection, per-skill enable rules, and namespaced `<plugin>:<skill>` names;
- opt-in Claude Code skill/plugin discovery (`skills.claudePlugins=true`) using
  `.claude/skills` and the installed plugin registry with user/managed/project/
  local scope applicability;
- managed OpenAI tunnel runtime installation when `tunnel.executable` is omitted:
  pinned `tunnel-client-runtime v0.0.12`, per-platform archive SHA-256, private
  versioned install directory, binary integrity manifest, and compatibility
  probes before use;
- `codexify-go update check` and `update apply`: stable GitHub release discovery,
  checksummed release archives, staged extraction, and a Windows helper process
  for replacement after the running process exits/restarting the service;
- optional Markdown chat (`agentChat.enabled`) with append-only `CHAT.md`,
  persistent per-conversation read cursors, `chat_read`, `chat_write`, and
  bounded `chat_await`;
- optional experimental agent tickets (`experimental.agentTickets`) that add a
  serial one-time ticket chain to model-facing tools, persist conversation
  ownership across transports, reject stale/concurrent branches before tool
  execution, and allow bounded offline recovery.

Not implemented yet:

- pixel-level parity with the much larger upstream Rust ChatGPT widgets; the Go
  implementation intentionally uses compact self-contained MCP Apps over the
  same server contracts;
- full Linux production validation. Native user-systemd lifecycle and crash
  recovery have passed on Ubuntu 26.04.1 LTS x86_64 with systemd 259, and the
  managed OpenAI tunnel runtime installs/verifies natively. Cold login/reboot,
  a real managed-tunnel connector session, service-context Git/SSH, and
  self-update still remain before Linux can be called production-validated.

The MCP core has been exercised with both raw MCP requests and the official Go
MCP client. End-to-end Windows SCM smoke tests verify the user-context lifecycle
and the multi-project/aggregation path:

```text
Windows SCM
  -> SYSTEM supervisor
       |-> tunnel runtime
       |-> <domain>\<interactive-user> worker
             -> MCP server
             -> exec_command whoami == <domain>\<interactive-user>
             -> long-running shell child

worker kill  -> worker restarts as the same interactive user
tunnel kill  -> tunnel restarts and re-authenticates to MCP
service stop -> worker + tunnel + long-running shell child are all gone

conversation A -> source checkout
conversation B -> same project -> managed worktree (auto mode)
catalog upstream -> search echo tool -> call echo tool -> bridged result

GitHub commit URL -> private clone -> fetched exact commit -> managed worktree
project -> remember/export -> switch -> scratch -> skill read -> resume project
setup MCP App resource -> readable with text/html;profile=mcp-app

get_agent_brief -> gateway skill + AGENTS.md at highest project priority
apply_patch -> show_diff(project_open) -> apply_patch -> show_diff(last_diff)
host file reference -> HTTPS ingress -> new workspace file
legacy stateful MCP session -> transient project binding -> disconnect -> forgotten

installed Codex plugin registry -> active plugin version -> namespaced skills
Markdown CHAT.md -> chat_read/write/await -> persistent conversation cursor
first model-facing call -> new_codexify_ticket -> serial ticket chain -> stale call rejected
managed tunnel install -> pinned archive SHA -> binary verify -> compatibility probe
```

## Build

```text
go test ./...
go build -o bin/codexify-go ./cmd/codexify-go
```

Go 1.26+ is currently used for development.

## Configuration

Copy `config.example.json` to an ignored local file. Relative paths in the
example are resolved from the config file directory.

```text
cp config.example.json config.local.json
# PowerShell: Copy-Item config.example.json config.local.json
```

Set:

- `mcp.workspaceRoot` to the directory this instance is allowed to access. In
  multi-project mode this is the **access root**, not an individual project;
- omit `tunnel.executable` to use the pinned managed OpenAI runtime, or set it
  explicitly to a compatible local `tunnel-client-runtime` binary;
- `tunnel.tunnelId`;
- `tunnel.apiKeyRef` to an `env:NAME` or `file:<path>` reference;
- `tunnel.mcpServerUrl` to the local MCP endpoint.

`tunnel.mcpServerUrl` must be an explicit loopback HTTP URL such as
`http://127.0.0.1:3300/mcp/tunnel_<id>`. When `mcp.authEnabled` is true (the
default), `codexify-go` generates a random bearer token at process start and
passes it to the tunnel child through an environment reference. It is never
written to `config.local.json`.

Do not put the OpenAI tunnel API key itself in Git.

### Multi-project mode

Enable durable conversation-scoped project selection with:

```json
{
  "mcp": {
    "workspaceRoot": "<absolute-or-env-expanded-access-root>",
    "multiProject": true,
    "projectScanDepth": 2,
    "worktrees": { "mode": "auto" }
  }
}
```

`list_projects` returns relative selectors beneath the access root.
`set_project_root` accepts one of those selectors. Bindings survive MCP/service
restarts and are immutable for that ChatGPT conversation. The raw
`openai/session` value is hashed before it is used as a binding key or filename.

When `worktrees.mode` is `auto`, the first conversation uses the source checkout
and a later conversation selecting the same Git project gets an isolated managed
worktree. `always` isolates Git projects and `never` uses the source checkout.
An explicit `createWorktree=true/false` on `set_project_root` overrides the
configured mode for that selection.

`set_project_root.path` may also be a safe repository URL. GitHub HTTPS URLs
support repository roots plus `/tree/<branch>`, `/pull/<number>`, and
`/commit/<full-40-char-sha>`. Generic non-GitHub HTTPS/SSH URLs must end in
`.git`. Credential-bearing HTTPS URLs, `file://`, and other local/insecure
transport forms are rejected. Matching local checkouts are reused; otherwise a
clone is staged and published inside the configured private clone directory.
Targeted revisions never silently move an unrelated source checkout: when the
requested commit is not already checked out, an isolated worktree is required.

Workspace changes are explicit. `setup_ui_switch_project` archives the active
binding without deleting its files/worktree; `set_project_root` can then select
another project or `withoutProject=true` for persistent scratch. A later/new
conversation may pass an exact saved active workspace as `resumePath`; arbitrary
filesystem paths are not accepted as resumable state.

### Upstream MCP aggregation

Upstreams are configured under `mcp.upstreams`. Example:

```json
{
  "name": "source-name",
  "transport": "stdio",
  "mode": "catalog",
  "command": "example-mcp-server",
  "args": [],
  "required": false,
  "timeout": "15s"
}
```

For Streamable HTTP use `"transport": "streamable_http"`, `"url": "..."`,
and optional `headers`. `catalog` keeps the upstream tool set private behind the
four `mcp_*` discovery/call tools; `direct` exposes tools as
`<source>__<original-tool-name>`.

For legacy upstreams that close instead of rejecting the modern
`server/discover` probe, set `"protocolVersion": "2025-11-25"` on that
upstream. Leave it unset to negotiate the newest protocol supported by the SDK.

When an upstream tool returns a `ResourceLink`, `codexify-go` replaces its
original URI with an opaque `codexify-go://upstream-resource/...` capability.
Reading that capability proxies `resources/read` back to the originating MCP
server and enforces `artifactEgress.maxFileBytes`.

### Memory, skills, and file export

Memory is stored outside the repository and keyed by the active workspace. It
is intended for concise project decisions/facts rather than conversation logs;
the default aggregate note budget is 16 KiB.

Skills are progressively disclosed: `skills_list` reads only `SKILL.md`
frontmatter, and `skills_read` loads the selected body/resource. Public skill
descriptors use scope-relative package paths rather than absolute host paths.

`export_host_file` accepts an active-workspace-relative regular file and returns
an opaque downloadable MCP resource. By default files up to 100 MiB may be
exported, snapshots use a bounded durable store, and a non-snapshotted resource
may safely fall back to the latest source file for a bounded TTL. No local host
path appears in the resource URI or export receipt.

`import_host_file` is the inverse path for a host-authorized native file
reference. The tool advertises `openai/fileParams`, accepts the temporary
download URL/file metadata supplied by the host, validates every redirect and
resolved address, and writes only to a new workspace-relative destination.
Arbitrary local source paths and overwrite-by-import are not supported.

### Agent brief, patching, and diff checkpoints

After selecting or switching a workspace, call `get_agent_brief`. It combines
generic coding workflow guidance, current environment/workspace information,
saved project memory, the progressive skill catalogue, and repository
instructions. Project `AGENTS.override.md` / `AGENTS.md` content is appended
last so repository-specific instructions have the highest project-level
priority.

`apply_patch` implements the Codex patch grammar (`*** Begin Patch`, add/delete/
update/move actions and contextual hunks). Every path and update context is
validated before the first filesystem mutation; paths cannot escape the active
workspace.

`show_diff` captures the complete working-tree snapshot through a temporary Git
index (`GIT_INDEX_FILE` + `git add -A` + `git write-tree`), so untracked/deleted
files and mode changes are represented without touching the user's staging
area. `since=project_open` compares against the immutable selection checkpoint;
`since=last_diff` compares against the private incremental cursor. The bounded
patch is placed in component/widget metadata rather than model-visible
structured output.

### Generic MCP clients

ChatGPT conversations use their stable hashed `openai/session` identity and
persist bindings. Older/generic stateful MCP clients that lack that metadata
receive a transport-session identity instead. Such a binding is intentionally
transient: reconnecting creates a new unbound session, while any files already
written to the chosen workspace remain untouched.

### MCP Apps

The server exposes small self-contained setup and diff resources with MIME type
`text/html;profile=mcp-app`. The setup app calls the same server-side
`setup_status`, project-selection, scratch, and switch tools; it has no separate
workspace state. `show_diff` carries the diff-app/result metadata and remains
usable as a normal MCP tool in clients that ignore MCP Apps metadata.

`setup_status` also exposes a connector schema marker such as
`0.8.2-dev+markdown-chat-v5+tickets-v1+workspace-v1+artifact-ingress-v1+gateway-v1`. A conversation can
echo the marker it currently holds; the server records the first observed
conversation marker privately and reports `conversationStale` when the active
server schema has changed.

### Installed plugin skills

With the default skill roots, installed Codex plugins are discovered from the
effective Codex home (`CODEX_HOME` or `~/.codex`). Only plugins enabled by
`config.toml` are considered; cached but uninstalled/disabled plugin versions
are not blindly scanned. `local` wins version selection, otherwise semantic
versions are ordered before falling back to lexical ordering. Agent Plugin and
legacy plugin manifests define the namespace/skill roots, and Codex
`[[skills.config]]` enable rules are applied to the resulting qualified skills.

Claude plugin discovery is deliberately opt-in. When enabled, Codexify Go reads
`~/.claude/plugins/installed_plugins.json`, selects only an installation whose
scope applies to the active workspace, and does not fall back to stale cache
directories that are absent from the registry.

### Managed tunnel runtime and updates

`tunnel.executable` remains an explicit override. When omitted, the managed
runtime lives below `tunnel.managedDir` in a versioned directory. Install it or
verify it with:

```text
codexify-go tunnel install --config config.json
codexify-go tunnel status --config config.json
codexify-go tunnel verify --config config.json
```

The current runtime is pinned to OpenAI `tunnel-client-runtime v0.0.12`; the
archive hash table matches upstream Codexify and the installed binary is also
hashed into a private manifest. `--version` and `run --help` are executed before
the runtime is accepted, including checks for the MCP/auth/health flags used by
Codexify Go.

Self-update is explicit rather than automatic:

```text
codexify-go update check --force
codexify-go update apply --config config.json
```

`check` is read-only and cached unless forced. `apply` requires a newer stable
GitHub release containing the exact platform archive plus `checksums.txt`,
verifies SHA-256 before staging, and on Windows delegates replacement to a
detached helper that waits for the old PID to exit. No release means no change.

### Markdown chat and agent tickets

When Markdown chat is enabled, its `CHAT.md` lives in private user state keyed
by workspace and conversation rather than inside the Git repository. User text
is append-only; agent messages are written only through `chat_write` and are
filtered from subsequent `chat_read` results. Persistent conversations retain a
private cursor; transient transport identities keep it in memory only.

Agent tickets are an optional experimental concurrency guard. The first
model-facing tool call omits `codexify_ticket` and receives
`new_codexify_ticket`; every subsequent call must supply the latest value. A
successful call or accepted tool-error advances the ticket. Reuse of an older
ticket, concurrent ownership, or a duplicated agent branch is rejected before
the underlying tool executes. Persistent tickets use locked private files so
the chain survives reconnects/processes; anonymous transport tickets stay
memory-scoped.

### Connector migration and release publishing

Connector schema state keeps the plain compatibility marker used by older Go
versions and now also maintains a bounded JSONL transition history. An existing
plain marker is migrated lazily as `source=legacy_plain`; later runtime schema
changes append transition records without changing the first-observed
conversation-version rule.

GitHub Actions includes native Windows/Linux/macOS tests, `go vet`, a Linux race
detector job, all six cross-build targets, and a stable aggregate `required`
check intended for branch protection. A manual release-candidate workflow runs
the same native validation, packages all six targets, and executes the packaged
amd64 artifacts on Windows/Linux/macOS without publishing a GitHub release.

The stable tag release workflow re-runs native tests before packaging, injects
the exact tag version into the binary, verifies every archive/checksum, executes
the packaged amd64 artifacts on their native runners, and only then publishes.
Stable `vX.Y.Z` tags produce the exact archives consumed by the updater:

```text
codexify-go-vX.Y.Z-windows-amd64.zip
codexify-go-vX.Y.Z-windows-arm64.zip
codexify-go-vX.Y.Z-linux-amd64.tar.gz
codexify-go-vX.Y.Z-linux-arm64.tar.gz
codexify-go-vX.Y.Z-darwin-amd64.tar.gz
codexify-go-vX.Y.Z-darwin-arm64.tar.gz
checksums.txt
```

Windows uses ZIP; macOS and Linux use gzip-compressed tar archives so Unix
release artifacts follow the native CLI distribution convention. The updater
supports both formats and still verifies the archive SHA-256 before extraction.
Published releases are immutable: `v0.8.1` remains the historical all-ZIP
release. New Unix updaters prefer tar.gz, while releases also publish ZIP
compatibility assets for Darwin/Linux. Older updaters query only
`releases/latest` and request ZIP, so retaining those compatibility assets keeps
their self-update path valid even after newer releases become latest.

The release contract is regression-tested against the self-updater. Successful
cross-compilation is not treated as runtime validation: source-built native
tests and packaged-artifact smoke tests are separate gates. A manual
`Linux Service Smoke` workflow additionally exercises the real `systemd --user`
install/start/status/restart/stop/remove lifecycle with an isolated fake tunnel
fixture, so CI can validate the service manager boundary without requiring a
real connector or credentials. Windows/macOS Rust-to-Go cutover acceptance
remains a separate host-level exercise.

### macOS Apple Silicon service model

macOS uses a per-user LaunchAgent under `~/Library/LaunchAgents`, not a root
LaunchDaemon. The agent is installed with `RunAtLoad` plus restart-on-failure
`KeepAlive` semantics and managed through `launchctl bootstrap/bootout` in the
current `gui/<uid>` domain.

Because launchd already runs the service in the logged-in user's Aqua session,
macOS does not need the Windows `CreateProcessAsUser` worker split. The MCP
server, Git commands, Keychain/SSH context, plugin discovery, and tunnel
supervisor all execute directly as that user.

POSIX child processes are placed in their own process groups. Graceful/forced
supervisor shutdown and exec-session cancellation signal the full process group,
so grandchildren do not survive service stop. Linux uses the same process-tree
behavior under its per-user systemd cgroup.

On the Apple Silicon validation host the real LaunchAgent lifecycle passed
install/start/status, MCP health, forced tunnel-child death/restart, and clean
stop/remove without orphan processes. The managed OpenAI
`tunnel-client-runtime v0.0.12` was also downloaded, SHA-verified, compatibility
probed, and confirmed as a native Mach-O arm64 executable.

### Linux user-systemd service model

Linux installs a per-user unit under the effective user configuration directory
(`$XDG_CONFIG_HOME/systemd/user`, normally `~/.config/systemd/user`) and enables
it for `default.target`. Normal install/start/stop/restart/remove/status
operations use `systemctl --user` and require neither a root daemon nor an
automatic `loginctl enable-linger` side effect.

The unit executes Codexify Go directly as the logged-in user, so Linux does not
use the Windows user-worker split. It uses `Restart=on-failure`,
`KillMode=control-group`, bounded shutdown, and journal stdout/stderr. Unit names
are normalized and hash-suffixed, and generated `ExecStart` arguments are
shell-free and safe for absolute paths containing spaces, percent signs, and
dollar signs.

On Ubuntu 26.04.1 LTS x86_64 (kernel 7.0.0-34-generic, systemd 259), a dedicated
smoke unit passed install/start/status/restart/stop/remove, MCP health, forced
tunnel-child death/restart, forced service death/restart, and final cgroup
cleanup without orphan processes. The official managed
`tunnel-client-runtime v0.0.12` also passed download, pinned SHA-256,
compatibility probes, and native ELF x86_64 verification. This is intentionally
not yet described as full Linux production validation: cold login/reboot, an
end-to-end real managed-tunnel connector session, service-context Git/SSH/MCP
workflow, and Linux self-update are still pending.

Validate configuration without starting the tunnel:

```text
./bin/codexify-go doctor --config ./config.local.json
# PowerShell: .\bin\codexify-go.exe doctor --config .\config.local.json
```

Run in the foreground:

```text
./bin/codexify-go run --config ./config.local.json
# PowerShell: .\bin\codexify-go.exe run --config .\config.local.json
```

## Windows service

Service installation modifies SCM and therefore normally requires an elevated
terminal:

```powershell
.\bin\codexify-go.exe service install --config .\config.local.json
.\bin\codexify-go.exe service start   --config .\config.local.json
.\bin\codexify-go.exe service status  --config .\config.local.json
```

Removal:

```powershell
.\bin\codexify-go.exe service stop   --config .\config.local.json
.\bin\codexify-go.exe service remove --config .\config.local.json
```

The service is deliberately named `CodexifyGo` by default so it does not touch
or conflict with an installed Rust Codexify service.

## Windows identity model

The durable SCM service runs as `LocalSystem`, but developer-facing MCP tools do
not. The service discovers an active interactive WTS session, obtains that
session's user token, and starts `codexify-go worker run` with
`CreateProcessAsUser`. The worker receives the ephemeral MCP bearer through its
user environment and owns the MCP HTTP server, filesystem tools, Git and exec
sessions.

If no interactive user is logged in, the Windows service remains alive and the
worker supervisor retries with bounded backoff until an active session appears.

This removes the main LocalSystem limitation from Phase 2. Repository cloning,
target fetches, multi-project bindings, managed worktrees, skills, memory,
artifact export, and upstream MCP aggregation all run inside this user context,
so Git Credential Manager, SSH agents, PATH, and other user-scoped state remain
available to developer workflows.

See [ARCHITECTURE.md](ARCHITECTURE.md).
