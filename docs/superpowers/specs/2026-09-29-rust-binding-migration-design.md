# Rust binding migration design

## Goal

Make an in-place Rust -> Go connector cutover restore an existing ChatGPT conversation workspace automatically when Go has no native binding yet.

## Design

On a persistent ChatGPT conversation identity, Go first resolves its normal binding. If none exists, it derives the legacy Rust identity from the same raw `openai/session` metadata using Rust's namespace `codexify/openai-session/v1\0`. It then looks only for that exact legacy binding under the legacy state tree rooted at the configured access root's user state.

The legacy record is treated as read-only input. Import is allowed only when its recorded access root matches the active Go access root and all referenced project/worktree paths still exist and remain inside that access root or the validated legacy managed-worktree location. The imported Go binding is written under Go's own identity hash and current binding format. Existing Go bindings always win and are never overwritten by migration.

Migration never copies credentials, connector tokens, tunnel state, arbitrary Rust configuration, or unrelated conversation records. Invalid, ambiguous, stale, or unsafe legacy records are ignored with a diagnostic and normal workspace selection remains available.

## Testing

Tests must prove: exact Rust identity derivation; successful import for a valid direct project and managed worktree; no import when a Go binding exists; rejection of mismatched access roots, missing paths, and path escapes; Rust legacy files remain byte-for-byte unchanged. The focused project tests and full `go test ./...` must pass.
