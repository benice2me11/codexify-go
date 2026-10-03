# Setup Context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax.

**Goal:** Restore the actual setup widget's own conversation binding without fabricating host identity.
**Architecture:** Add a bounded in-memory context store and receiving middleware scoped to four setup tools. Resolve only server-issued contexts to previously observed conversation metadata, using a request-local lookup consumed by requestMeta. Deliver the handle through hidden result metadata and forward it from the existing setup widget.
**Tech Stack:** Go 1.26.5, existing MCP Go SDK 1.8.0, embedded JavaScript and existing Node VM harness; no new dependencies.
**Spec:** docs/superpowers/specs/2026-10-03-setup-context.md

## Global Constraints
- Tokens: 32 random bytes, URL-safe base64, 30 minute lifetime, at most 1024 in-memory entries.
- Persistent server-observed conversation context only; no desktop UUID substitution or binding edits.
- Scope: list_projects, setup_status, set_project_root, setup_ui_switch_project.
- Authentication, model flow, rollback assets and unrelated work remain protected.
- Live G05 evidence cannot be replaced by synthetic tests.

## Review Focus
- A context paired with conversation B must fail before mutation.
- Expired or pre-restart contexts must not regain a workspace.
- Missing metadata and delayed host globals must not create request loops.
- Contexts cannot be used for file or execution tools, or leak into model output.
- Server failure during a widget action must preserve the displayed prior selection.

### Task 1: Bind setup calls to verified conversation context
**Files:** internal/mcpserver/setup_context.go; internal/mcpserver/setup_context_test.go; internal/mcpserver/server.go.
**Interfaces:** Runtime.setupContextMiddleware() mcp.Middleware; Runtime.requestMeta consults request-local verified metadata. uiContext is an optional string in the four setup input schemas. Hidden output metadata key is io.github.devnoname120/codexify/setup-context.
- [ ] Write TestSetupContextStatelessIsolation: two real stateless MCP conversations, one token per conversation, UI switch affects only A, UI select restores only A, cross-conversation/malformed/unknown/other-tool attempts rejected with unchanged bindings, no raw token in content or structuredContent.
- [ ] Run go test ./internal/mcpserver -run TestSetupContextStatelessIsolation -count=1; expect missing hidden context failure.
- [ ] Implement zero-value bounded store and middleware. Preserve original metadata, apply verified context only for the current request, attach hidden metadata to setup results.
- [ ] Add expiry/restart/capacity tests and run go test ./internal/mcpserver -count=1; expect PASS.
- [ ] Commit only this task's source/tests after verification.

### Task 2: Carry the setup context through the widget
**Files:** internal/ui/resources.go; internal/ui/widget_runtime_test.mjs.
**Interfaces:** window.openai.toolResponseMetadata or globals event supplies hidden metadata; call results may carry _meta directly or in canonical envelopes. uiContext is sent only when present; fresh received context replaces stale context.
- [ ] Add harness tests for direct, call_tool_result and mcp_tool_result metadata, delayed metadata, context refresh, absent metadata compatibility, and no extra calls from host events.
- [ ] Run node internal/ui/widget_runtime_test.mjs --source internal/ui/resources.go; expect missing uiContext failure.
- [ ] Implement metadata extraction and argument forwarding in SetupHTML only. Bump SetupURI to v3 so old resource caches cannot conceal this change.
- [ ] Run go test ./... and the Node harness; expect PASS. Obtain one fresh whole-change review and fix material findings.
- [ ] Commit exact scoped files; freeze a new source snapshot and build/hash it with complete verified tunnel runtime.

### Task 3: Verify the actual host and record outcomes
**Files:** private candidate evidence; docs/handoffs/WINDOWS_G05_INITIAL_ISOLATION_REPORT_20261002.md; relevant private gate ledger.
**Interfaces:** The new frozen candidate from task 2; existing A and B test tasks and recovery assets.
- [ ] Guarded handoff; request a fresh model list_projects card in A; verify UI shows A's existing workspace.
- [ ] Perform the authorized A Switch project and select project-b once; confirm A's new workspace and B's unchanged binding and session.
- [ ] Preserve timestamped traces, exact runtime identity and binding evidence. Mark G05 PASS only if its complete live contract is met; otherwise record the concrete boundary.
- [ ] Proceed to the authorized G06 checks if dependencies permit; stop only at a real external blocker or separately required Windows reboot confirmation.

