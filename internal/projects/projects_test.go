package projects

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/workspace"
)

func TestWorkspaceImportsExactLegacyRustBindingOnGoBindingMiss(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project-a")
	initGitRepo(t, project)
	manager := testManager(t, root, "never")
	meta := map[string]any{"openai/session": "legacy-chat"}

	sum := sha256.Sum256([]byte("codexify/openai-session/v1\x00legacy-chat"))
	legacyKey := hex.EncodeToString(sum[:])
	legacyDir := filepath.Join(root, ".codexify", "conversation-projects", "instance")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"version": 2, "accessRoot": root, "projectRoot": project,
		"sourceProjectRoot": project, "repositoryUrl": nil,
		"managedWorktree": false, "worktreeGitRoot": nil, "worktreesRoot": nil,
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyDir, legacyKey+".json")
	if err := os.WriteFile(legacyPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}

	resolved, info, err := manager.Workspace(meta)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Path() != project || info.ProjectRoot != project || info.BindingScope != "chatgpt_conversation" {
		t.Fatalf("unexpected migrated workspace: %q %+v", resolved.Path(), info)
	}
	if _, err := os.Stat(manager.bindingPath(IdentityFromMeta(meta))); err != nil {
		t.Fatalf("Go binding was not persisted: %v", err)
	}
	after, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("legacy Rust binding was modified")
	}
}

func TestWorkspaceDoesNotImportUnsafeLegacyRustBinding(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project-a")
	initGitRepo(t, project)
	manager := testManager(t, root, "never")
	meta := map[string]any{"openai/session": "unsafe-legacy-chat"}
	identity := IdentityFromMeta(meta)
	legacyDir := filepath.Join(root, ".codexify", "conversation-projects", "instance")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"version": 2, "accessRoot": filepath.Join(root, "other"),
		"projectRoot": project, "sourceProjectRoot": project,
		"managedWorktree": false,
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, identity.LegacyKey+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Workspace(meta); err == nil || !strings.Contains(err.Error(), "no project selected") {
		t.Fatalf("unsafe legacy binding should not import, got %v", err)
	}
}

func TestGoBindingWinsOverLegacyRustBinding(t *testing.T) {
	root := t.TempDir()
	goProject := filepath.Join(root, "go-project")
	legacyProject := filepath.Join(root, "legacy-project")
	initGitRepo(t, goProject)
	initGitRepo(t, legacyProject)
	manager := testManager(t, root, "never")
	meta := map[string]any{"openai/session": "existing-go-chat"}
	if _, err := manager.Select(meta, "go-project", nil); err != nil {
		t.Fatal(err)
	}
	identity := IdentityFromMeta(meta)
	legacyDir := filepath.Join(root, ".codexify", "conversation-projects", "instance")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"version": 2, "accessRoot": root, "projectRoot": legacyProject,
		"sourceProjectRoot": legacyProject, "managedWorktree": false,
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, identity.LegacyKey+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, _, err := manager.Workspace(meta)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Path() != goProject {
		t.Fatalf("legacy binding overrode Go binding: %q", resolved.Path())
	}
}

func TestConversationBindingPersistsWithoutRawSession(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project-a")
	initGitRepo(t, project)
	state := filepath.Join(root, ".state")

	manager, err := New(config.MCPConfig{
		WorkspaceRoot:    root,
		MultiProject:     true,
		BindingsDir:      filepath.Join(state, "bindings"),
		ProjectScanDepth: 2,
		Worktrees: config.WorktreeConfig{
			Mode: "never",
			Root: filepath.Join(state, "worktrees"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	meta := map[string]any{"openai/session": "raw-secret-conversation-id"}
	selection, err := manager.Select(meta, "project-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !selection.NewlySelected || selection.ManagedWorktree {
		t.Fatalf("unexpected selection: %+v", selection)
	}
	resolved, info, err := manager.Workspace(meta)
	if err != nil {
		t.Fatal(err)
	}
	projectCanonical, err := workspace.Canonical(project)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Path() != projectCanonical || info.ProjectRoot != projectCanonical {
		t.Fatalf("workspace mismatch: %s %+v", resolved.Path(), info)
	}

	identity := IdentityFromMeta(meta)
	data, err := os.ReadFile(manager.bindingPath(identity))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "raw-secret-conversation-id") {
		t.Fatal("raw conversation id leaked into persisted binding")
	}

	manager2, err := New(manager.cfg)
	if err != nil {
		t.Fatal(err)
	}
	resolved2, _, err := manager2.Workspace(meta)
	if err != nil {
		t.Fatal(err)
	}
	if resolved2.Path() != projectCanonical {
		t.Fatalf("persisted binding resolved to %q", resolved2.Path())
	}
}

func TestBindingIsImmutable(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, filepath.Join(root, "one"))
	initGitRepo(t, filepath.Join(root, "two"))
	manager := testManager(t, root, "never")
	meta := map[string]any{"openai/session": "chat-one"}

	if _, err := manager.Select(meta, "one", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Select(meta, "two", nil); err == nil || !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("expected immutable binding error, got %v", err)
	}
}

func TestAutoModeCreatesWorktreeWhenProjectAlreadyInUse(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "repo")
	initGitRepo(t, source)
	manager := testManager(t, root, "auto")

	firstMeta := map[string]any{"openai/session": "chat-one"}
	first, err := manager.Select(firstMeta, "repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.ManagedWorktree {
		t.Fatal("first binding should use source checkout in auto mode")
	}

	secondMeta := map[string]any{"openai/session": "chat-two"}
	second, err := manager.Select(secondMeta, "repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !second.ManagedWorktree {
		t.Fatalf("second binding should be isolated: %+v", second)
	}
	if second.ProjectRoot == source || second.WorktreeGitRoot == "" {
		t.Fatalf("invalid managed worktree selection: %+v", second)
	}
	if _, err := os.Stat(filepath.Join(second.ProjectRoot, "README.md")); err != nil {
		t.Fatalf("managed worktree missing repository content: %v", err)
	}

	listed, err := manager.ListWorktrees(secondMeta)
	if err != nil {
		t.Fatal(err)
	}
	foundManaged := false
	secondWorktreeCanonical, err := workspace.Canonical(second.WorktreeGitRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, wt := range listed.Worktrees {
		wtCanonical, canonicalErr := workspace.Canonical(wt.Path)
		if canonicalErr == nil && wtCanonical == secondWorktreeCanonical && wt.Managed {
			foundManaged = true
		}
	}
	if !foundManaged {
		t.Fatalf("managed worktree not listed: %+v", listed.Worktrees)
	}
}

func TestProjectCatalogFiltersAndSkipsNestedRepositoryContents(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, filepath.Join(root, "alpha"))
	beta := filepath.Join(root, "group", "beta")
	if err := os.MkdirAll(beta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beta, "go.mod"), []byte("module example/beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := testManager(t, root, "never")

	out, err := manager.List("beta", 10)
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 1 || len(out.Projects) != 1 || out.Projects[0].Selector != "group/beta" {
		t.Fatalf("unexpected catalog: %+v", out)
	}
}

func TestScratchSwitchAndResumeLifecycle(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project-a")
	initGitRepo(t, project)
	manager := testManager(t, root, "auto")

	meta := map[string]any{"openai/session": "chat-lifecycle"}
	selected, err := manager.Select(meta, "project-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	change, err := manager.Switch(meta, selected.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !change.AwaitingSelection || change.PreviousRoot != selected.ProjectRoot {
		t.Fatalf("unexpected switch: %+v", change)
	}
	if _, _, err := manager.Workspace(meta); err == nil {
		t.Fatal("workspace should be unselected after switch")
	}

	scratch, err := manager.SelectScratch(meta)
	if err != nil {
		t.Fatal(err)
	}
	if scratch.Mode != "scratch" || scratch.SourceProjectRoot != "" {
		t.Fatalf("unexpected scratch selection: %+v", scratch)
	}
	if _, err := os.Stat(scratch.ProjectRoot); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Switch(meta, scratch.ProjectRoot); err != nil {
		t.Fatal(err)
	}
	resumed, err := manager.Resume(meta, selected.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Mode != "project" || filepath.Clean(resumed.ProjectRoot) != filepath.Clean(selected.ProjectRoot) {
		t.Fatalf("unexpected resumed selection: %+v", resumed)
	}
}

func TestNewConversationCanResumeSavedWorkspace(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project-a")
	initGitRepo(t, project)
	manager := testManager(t, root, "never")

	first, err := manager.Select(map[string]any{"openai/session": "chat-one"}, "project-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := manager.Resume(map[string]any{"openai/session": "chat-two"}, first.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(resumed.ProjectRoot) != filepath.Clean(first.ProjectRoot) {
		t.Fatalf("resume mismatch: %+v", resumed)
	}
}

func TestRepositoryReferenceParsing(t *testing.T) {
	tests := []struct {
		input    string
		kind     CheckoutKind
		checkout string
		identity string
	}{
		{"https://github.com/owner/repo", CheckoutDefault, "", "owner/repo"},
		{"https://github.com/owner/repo/tree/feature/test", CheckoutBranch, "feature/test", "owner/repo"},
		{"https://github.com/owner/repo/pull/42", CheckoutPR, "42", "owner/repo"},
		{"https://github.com/owner/repo/commit/0123456789abcdef0123456789abcdef01234567", CheckoutCommit, "0123456789abcdef0123456789abcdef01234567", "owner/repo"},
		{"git@github.com:owner/repo.git", CheckoutDefault, "", "owner/repo"},
		{"https://gitlab.example/group/repo.git", CheckoutDefault, "", "gitlab.example/group/repo"},
		{"git@gitlab.example:group/repo.git", CheckoutDefault, "", "gitlab.example/group/repo"},
	}
	for _, tt := range tests {
		ref, err := ParseRepositoryReference(tt.input)
		if err != nil {
			t.Fatalf("parse %q: %v", tt.input, err)
		}
		if ref.CheckoutKind != tt.kind || ref.Checkout != tt.checkout || ref.Identity != tt.identity {
			t.Fatalf("parse %q = %+v", tt.input, ref)
		}
	}

	invalid := []string{
		"file:///tmp/repo.git",
		"https://user:secret@github.com/owner/repo.git",
		"https://gitlab.example/group/repo",
		"https://github.com/owner/repo/commit/deadbeef",
		"https://github.com/owner/repo?token=secret",
	}
	for _, input := range invalid {
		if _, err := ParseRepositoryReference(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}

func TestRepositoryURLReusesMatchingCheckout(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "existing")
	initGitRepo(t, repo)
	runGit(t, repo, "remote", "add", "origin", "https://github.com/example/reusable.git")
	manager := testManager(t, root, "never")

	selected, err := manager.Select(
		map[string]any{"openai/session": "repo-url-reuse"},
		"https://github.com/example/reusable",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Cloned {
		t.Fatal("matching local checkout should be reused rather than cloned")
	}
	repoCanonical, err := workspace.Canonical(repo)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(selected.SourceProjectRoot) != filepath.Clean(repoCanonical) {
		t.Fatalf("source = %q want %q", selected.SourceProjectRoot, repo)
	}
	if selected.RepositoryURL != "https://github.com/example/reusable" {
		t.Fatalf("repository URL = %q", selected.RepositoryURL)
	}
}

func testManager(t *testing.T, root, mode string) *Manager {
	t.Helper()
	manager, err := New(config.MCPConfig{
		WorkspaceRoot:    root,
		MultiProject:     true,
		BindingsDir:      filepath.Join(root, ".state", "bindings"),
		ProjectScanDepth: 3,
		Worktrees: config.WorktreeConfig{
			Mode: mode,
			Root: filepath.Join(root, ".state", "worktrees"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func initGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "init")
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "add", "README.md")
	runGit(t, path, "-c", "user.name=Codexify Test", "-c", "user.email=test@example.invalid", "commit", "-m", "init")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
