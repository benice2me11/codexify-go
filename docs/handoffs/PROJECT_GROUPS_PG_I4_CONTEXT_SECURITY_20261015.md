# Project Groups — PG-I4 Context, Artifact Ownership & Security Ledger

**Date:** 2026-10-15
**Stack:** PG-I1 PR #20 → PG-I2 PR #22 → PG-I3 PR #24 → this PG-I4 branch.
**Branch:** `feat/project-groups-context-safety-20261015`
**Rollout:** DEVELOPMENT / FEATURE FLAG OFF / NO WINDOWS SERVICE CHANGE

## Scope contract

| State / operation | Scope | Enforcement |
|---|---|---|
| Group membership | Config catalog; per conversation snapshot | Canonical roots, stable aliases, physical directory identity in membership fingerprint |
| Active alias | Conversation | Explicit `set_active_repository`, never mutated by per-call overrides |
| Files, search, patch, Git and diff | Effective repository checkout | Per-call `workspace.Root`; independent Git roots/checkpoints |
| `AGENTS.md` and override | Effective repository only | `projectdoc.LoadWithin` rejects file/ancestor symlink escapes |
| Repo-local skills and resources | Effective repository only | `skills.ListWithin/ReadWithin` reject external symlinks and package traversal |
| User/global/plugin skills | Explicit shared host configuration | Still visible from any repository; marked by scope, **not** confused with repo-local instructions |
| Project memory | Effective checkout root | Existing per-project memory state key; member-specific `WorkspaceFor` |
| `CHAT.md` | Conversation + **primary** group checkout | Same path across active-alias switches; agent briefs link to primary chat |
| Artifact export and resource reads | Conversation + group + effective member + checkout epoch | Scoped persisted owner and request-time binding validation; legacy unscoped exports unchanged |
| Upstream MCP sources/sessions | Worker-level bridge | Constructed once on Runtime init; alias switches do not reconnect |
| Long-running commands | Immutable conversation/member/root/epoch | PG-I3 owner-scoped stdin/stop; **no OS shell sandbox** |

## Implemented hardening

1. **Group artifact ownership.** A grouped export persists owner fields (conversation identity hash, group, alias, active root, membership fingerprint, binding epoch). Resource access through the native MCP handler checks the caller identity from request metadata or its original transport session, revalidates the group's current checkout, and denies a foreign/stale owner. It rejects a bare token without matching owner. Legacy resources preserve their existing bearer-token capability contract and immutable snapshot behavior. Owner data stays in local artifact records, not in resource URI.
2. **TTL.** Group snapshots and source fallbacks expire at `artifactEgress.referenceTtl`. Expired metadata and blobs are removed on owner-authorized expired reads or during subsequent artifact pruning/export. This is *lazy cleanup*, not a scheduled deletion guarantee.
3. **Root and instructions confinement.** New `projectdoc.LoadWithin` and `skills.ListWithin/ReadWithin` are used for group members. Symlinked `AGENTS.md`, `SKILL.md`, nested resources and checkout-escaping junctions do not gain access to a sibling repository. Existing standalone single-project instruction/skill semantics are unchanged.
4. **Point-of-use revalidation.** `write_file` revalidates its canonical destination after creating parent directories. The patch engine revalidates the planned source and move destinations before each mutation and after parent creation, and compares file identity for verified update/delete sources. These checks reduce time-of-check/time-of-use exposure but do **not** guarantee race-free OS-level confinement against a hostile concurrent filesystem writer.
5. **Persistent checkout identity.** The private membership hash now includes each source Git directory's OS-level identity: volume serial and file ID on Windows, device+inode on Linux/macOS. Changing the underlying checkout while keeping the same canonical path invalidates persisted v3 group bindings after a worker restart. Identifiers are hashed and not shown in the public group catalog.
6. **Upstream connection behavior.** A fake read-only MCP server recorded 9 intentional echo requests after 9 active alias switches and **no additional HTTP requests or new protocol initialization**, while source discovery/search continued to work. This fixture test does not prove live hh_local/habr_career uptime or 429 behavior.

## Windows fixture test evidence

- `go test ./internal/artifacts ./internal/projectdoc ./internal/skills ./internal/projects ./internal/mcpserver -count=1`: PASS.
- Scoped resource URI: own conversation/epoch allowed, token-only/foreign conversation/stale epoch denied; active alias change allowed; TTL expiry enforced and locally pruned.
- Parent/sibling junction escape: MCP file/read/search/patch/export/workdir operations denied.
- Windows junction into a sibling skill directory rejected. Linux/macOS file-symlink negative tests are included for native CI runners.
- Per-member skills: hh and habr discover/read only their own repo-local `SKILL.md`.
- Group Markdown chat remains stable on the primary checkout after member switch.
- Replacing a Git checkout at the same path across two Manager instances changes fingerprint and denies old group binding.
- Upstream mock: 9 source switches, no extra HTTP requests or connection burst.

## Unresolved acceptance and trust limitations

- **Resource metadata compatibility:** grouped resource reads require the caller's authenticated conversation identity in MCP `resources/read` metadata (or the same stateful transport identity). The official SDK test confirms this protocol shape; the real ChatGPT host must be verified in an approved canary. If the host omits this identity, grouped artifact reads deliberately fail closed, and the release is BLOCKED pending compatible host support—not silently downgraded to bearer-token access.
- The current trusted-host model treats `openai/session` identity metadata as authoritative. This is not a defense against a malicious client able to forge conversation metadata on an authenticated transport; additional transport claims would be required for that threat model.
- Arbitrary shell commands have full OS account privileges. File tools use containment checks but do not provide race-free kernel-level sandboxing. A stronger OS handle-relative `openat`/Windows-handle design is separate future work.
- Durable cross-process compare-and-swap for group binding generation remains pending; existing mutex serializes only one worker process.
- Live upstream hh_local/habr_career session burst/429 monitoring, UI group picker/checkout mode changes, two actual Go worker restarts, Go-only canary/rollback, and PG-A00..PG-A08 final closure remain NOT RUN.

| Gate | Result after PG-I4 |
|---|---|
| PG-A00 | PARTIAL — source and Go-only rollback baseline documented, rollout not rehearsed |
| PG-A01 | PARTIAL — stronger checkout fingerprint and cross-restart fixture |
| PG-A02 | PARTIAL — Manager recreation, not live service restart |
| PG-A03 | PARTIAL — prior 3-repo routing fixtures pass; no live operator canary |
| PG-A04 | PARTIAL — AGENTS/skills/memory/chat/artifact scopes tested |
| PG-A05 | PARTIAL — more junction/symlink and ownership negative tests; OS-race caveat |
| PG-A06 | PARTIAL — upstream mock shows zero excess connections; hosted UI/live upstream not tested |
| PG-A07 | PARTIAL — local Windows tests; CI/native matrix must be verified on PR |
| PG-A08 | NOT RUN — production feature flag remains disabled |

**Next:** PG-I5 Setup Group picker + Source/Managed member controls, schema refresh path and accessibility; then PG-I6/PG-I7 isolated/native runtime and controlled Go-only canary. Do not merge into a v0.9.0 stable release while its existing HOLD applies.

**Operations explicitly excluded:** production Go service restart/replacement, changing tunnel or watcher, Rust fallback, live checkout mutation in work-search/hh-mcp-server/habr-career-mcp, Vacancy Hourly Watch enablement, and job applications.
