# Project Groups compiled-worker native smoke

This fixture exercises a **compiled Codexify Go worker process** using the official MCP SDK over local HTTP. It is intended for PG-I6 and runs in GitHub CI on Windows, Linux, and macOS after building the native binary.

Run from the repository root:

```powershell
go build -trimpath -o "$env:TEMP\codexify-go-pg-smoke.exe" ./cmd/codexify-go
go run ./tools/forensics/project-groups-smoke "$env:TEMP\codexify-go-pg-smoke.exe"
```

On Linux/macOS, build without `.exe` and use that path as the second argument.

## Test sequence

The harness creates three **disposable Git repositories**, a private configuration, memory/artifact/chat/binding directories, and an ephemeral loopback HTTP listener for each worker instance. It launches the exact compiled candidate with `worker run --config <fixture>`, waits for health, and connects through a fresh MCP transport for each process.

- First worker: verifies Setup v4 and legacy v3 resources; selects `job-search`, with `hh` managed and `work`/`habr` source. Writes a dirty fixture file, selects a new active alias, verifies AGENTS.md scopes, exports and reads a scoped artifact, rejects foreign artifact ownership and cross-member traversal, and advances the hh diff cursor.
- After restart 1: resumes the **same conversation identity**, checks the persisted managed root, dirty content, active alias and last-diff checkpoint, reads the still-authorized artifact, switches hh to source and back, ensures the old artifact has been revoked and no worktree data was lost.
- After restart 2: validates the active alias, managed checkout, dirty file and primary `CHAT.md` remain stable; confirms the old resource token has not revived.
- Feature-off worker: group tools disappear and the saved v3 binding fails closed.
- Feature-on worker: verifies the same v3 binding, alias and dirty managed checkout can be resumed without migration or data loss.

The five worker launches include **four worker restarts**, of which the first two directly satisfy the process-restart fixture goal. All operations run under the test process and temporary Git trees; cleanup removes the spawned process and fixture tree. No real tunnel, Windows SCM, launchd or systemd unit is created.

Expected output:

```text
PG_SMOKE_STAGE_INITIAL=PASS
PG_SMOKE_STAGE_RESTART_1=PASS
PG_SMOKE_STAGE_RESTART_2=PASS
PG_SMOKE_STAGE_FLAG_OFF=PASS
PG_SMOKE_STAGE_REENABLE=PASS
PG_SMOKE_RESTARTS=4;FEATURE_DISABLE_REENABLE=PASS;REAL_TUNNEL=OFF;SCM=UNTOUCHED
PROJECT_GROUPS_COMPILED_WORKER_SMOKE=PASS
```

## Explicit limits

This proves native compiled **worker lifecycle** behavior, not production Windows SCM/reboot persistence, macOS launchd/systemd service restart, or ChatGPT-hosted UI interaction. The MCP client supplies a controlled test conversation identity, so it does not prove that the real host provides the required trusted `resources/read` identity metadata. Live upstream `hh_local`/`habr_career` 429 and session-burst checks remain a separate read-only canary.

The configuration uses inert fake tunnel values and disables upstream connections. It never sends job applications, enables Vacancy Hourly Watch, installs a service, or invokes Rust.
