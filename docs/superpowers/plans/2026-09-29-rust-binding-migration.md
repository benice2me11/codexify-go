# Rust Binding Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore an exact legacy Rust conversation workspace automatically when no Go-native binding exists.

**Architecture:** Derive both Go and legacy Rust identity hashes from the same `openai/session`. On Go binding miss, read only the exact legacy Rust record, validate access-root/path invariants, convert it to the current Go binding, and persist it without modifying Rust state.

**Tech Stack:** Go, SHA-256 identity derivation, JSON binding persistence, filesystem validation.

---

### Task 1: Legacy identity and record parsing

**Files:** `internal/projects/identity.go`, new `internal/projects/legacy_binding.go`, tests in `internal/projects/projects_test.go`.

- [ ] Add failing tests proving Rust namespace `codexify/openai-session/v1\0` derives the expected legacy hash and only the exact matching record is considered.
- [ ] Run focused tests and confirm RED.
- [ ] Add a legacy identity key alongside the Go identity and a strict decoder for Rust v2 project bindings.
- [ ] Run focused tests and confirm GREEN.

### Task 2: Safe binding import

**Files:** `internal/projects/bindings.go`, `internal/projects/manager.go`, `internal/projects/legacy_binding.go`, tests in `internal/projects/projects_test.go`.

- [ ] Add failing tests for direct checkout import, managed-worktree import, existing-Go-binding precedence, access-root mismatch, missing paths/path escape, and unchanged legacy bytes.
- [ ] Run focused tests and confirm RED.
- [ ] On persistent Go binding miss only, locate the exact legacy file, validate roots and Git/worktree metadata, convert to Binding v2, and write only the Go binding file.
- [ ] Run focused tests and confirm GREEN.

### Task 3: Verification and cutover report

**Files:** `docs/handoffs/RUST_TO_GO_CUTOVER_REPORT.md`.

- [ ] Run `gofmt` on touched Go files.
- [ ] Run `go test ./internal/projects -count=1` and `go test ./...`.
- [ ] Build `./cmd/codexify-go` to a temporary output path and remove it afterward.
- [ ] Update CUTOVER-002 with source-fix evidence and note that live migration acceptance requires a fresh Rust-bound conversation or controlled fixture during the next install/update cycle.
