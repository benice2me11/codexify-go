# Setup v3 compiled-binary smoke

This Windows integration fixture launches the built `codexify-go.exe worker run` on an ephemeral loopback port. It does **not** start a tunnel or touch Windows SCM, Task Scheduler, Rust, live connectors, or real projects.

The fixture creates one temporary Git repository and separate workspace binding/state directories. It verifies the real MCP `setup` tool and v3 UI resource, initial Source checkout selection, managed worktree creation, same-chat Source → Managed → Source switching, worktree preservation, and rejection of a file path that escapes the selected repository.

Run from the repository root after building a candidate:

~~~powershell
go build -trimpath -o codexify-go-smoke.exe ./cmd/codexify-go
go run ./tools/forensics/setup-smoke .\codexify-go-smoke.exe
~~~

Expected result: `SETUP_V3_ISOLATED_BINARY_SMOKE=PASS`. Temporary files/processes are cleaned up after the run, including on test failure. GitHub Windows CI runs the fixture against the binary built during its native build job.

This test does **not** establish hosted ChatGPT UI rendering, real connector identity after refresh, upstream MCP availability, tunnel self-healing, or G10 soak acceptance. Those require separate live gates.
