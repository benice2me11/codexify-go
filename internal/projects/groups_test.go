package projects

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/workspace"
)

func projectGroupFixture(t *testing.T) (string, config.MCPConfig) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"work-search", "hh-mcp-server", "habr-career-mcp"} {
		initGitRepo(t, filepath.Join(root, name))
	}
	cfg := config.MCPConfig{
		WorkspaceRoot:        root,
		MultiProject:         true,
		ProjectGroupsEnabled: true,
		BindingsDir:          filepath.Join(root, ".state", "bindings"),
		Worktrees:            config.WorktreeConfig{Mode: "auto"},
		Projects: []config.ProjectSpec{
			{Name: "work-search", Path: filepath.Join(root, "work-search")},
			{Name: "hh-mcp-server", Path: filepath.Join(root, "hh-mcp-server")},
			{Name: "habr-career-mcp", Path: filepath.Join(root, "habr-career-mcp")},
		},
		ProjectGroups: []config.ProjectGroupSpec{{
			Name: "job-search", Primary: "work",
			Repositories: []config.ProjectGroupRepository{
				{Alias: "work", Project: "work-search", Checkout: "source"},
				{Alias: "hh", Project: "hh-mcp-server", Checkout: "source"},
				{Alias: "habr", Project: "habr-career-mcp", Checkout: "source"},
			},
		}},
	}
	return root, cfg
}

func TestGroupRegistryThreeIndependentSourceRepos(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	manager, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	groups := manager.ListProjectGroups()
	if len(groups) != 1 || groups[0].Primary != "work" || len(groups[0].Repositories) != 3 {
		t.Fatalf("unexpected group catalog: %+v", groups)
	}
	if len(groups[0].MembershipFingerprint) != 64 {
		t.Fatalf("expected stable SHA-256 membership fingerprint: %+v", groups[0])
	}
	if got := []string{
		groups[0].Repositories[0].Alias,
		groups[0].Repositories[1].Alias,
		groups[0].Repositories[2].Alias,
	}; !slices.Equal(got, []string{"habr", "hh", "work"}) {
		t.Fatalf("repository aliases must be deterministic: %v", got)
	}

	for _, alias := range []string{"work", "hh", "habr"} {
		resolved, err := manager.ResolveProjectGroupSource("JOB-SEARCH", strings.ToUpper(alias))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(root, map[string]string{"work": "work-search", "hh": "hh-mcp-server", "habr": "habr-career-mcp"}[alias])
		want, _ = workspace.Canonical(want)
		if !sameGroupPath(resolved.Root.Path(), want) || resolved.Member.Alias != alias || resolved.Member.Checkout != "source" {
			t.Fatalf("resolved %s to %+v", alias, resolved.Member)
		}
		if _, err := resolved.Root.Resolve("README.md", false); err != nil {
			t.Fatalf("read own repo %s: %v", alias, err)
		}
		if _, err := resolved.Root.Resolve(filepath.Join("..", "hh-mcp-server", "README.md"), false); err == nil {
			t.Fatalf("alias %s escaped through sibling repo", alias)
		}
		if resolved.MembershipFingerprint != groups[0].MembershipFingerprint {
			t.Fatalf("fingerprint mismatch for alias %s", alias)
		}
	}
	primary, err := manager.ResolveProjectGroupSource("job-search", "")
	if err != nil || primary.Member.Alias != "work" {
		t.Fatalf("primary implicit alias: %+v, %v", primary.Member, err)
	}
	for _, alias := range []string{"missing", "../work", "habr/../work"} {
		if _, err := manager.ResolveProjectGroupSource("job-search", alias); err == nil {
			t.Fatalf("unknown alias %q was accepted", alias)
		}
	}

	groups[0].Repositories[0].Alias = "corrupted"
	again := manager.ListProjectGroups()
	if again[0].Repositories[0].Alias != "habr" {
		t.Fatal("returned catalog was not detached from internal storage")
	}
	// Input ordering and casing cannot change permission identity.
	cfg.ProjectGroups[0].Repositories[0], cfg.ProjectGroups[0].Repositories[2] =
		cfg.ProjectGroups[0].Repositories[2], cfg.ProjectGroups[0].Repositories[0]
	cfg.ProjectGroups[0].Name = "JOB-SEARCH"
	againManager, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if againManager.ListProjectGroups()[0].MembershipFingerprint != again[0].MembershipFingerprint {
		t.Fatal("fingerprint depends on input ordering or identifier case")
	}
}

func TestGroupRegistryRejectsUnsafeRoots(t *testing.T) {
	tests := []struct {
		name    string
		change  func(t *testing.T, root string, cfg *config.MCPConfig)
		wantErr string
	}{
		{"duplicate physical root", func(t *testing.T, root string, cfg *config.MCPConfig) {
			cfg.Projects[1].Path = cfg.Projects[0].Path
		}, "overlapping"},
		{"access root itself", func(t *testing.T, root string, cfg *config.MCPConfig) {
			cfg.Projects[0].Path = root
		}, "entire workspace access root"},
		{"unrelated existing root", func(t *testing.T, root string, cfg *config.MCPConfig) {
			outside := t.TempDir()
			initGitRepo(t, outside)
			cfg.Projects[0].Path = outside
		}, "outside allowed access root"},
		{"missing repo", func(t *testing.T, root string, cfg *config.MCPConfig) {
			cfg.Projects[0].Path = filepath.Join(root, "not-a-checkout")
		}, "unavailable"},
		{"non-Git repo", func(t *testing.T, root string, cfg *config.MCPConfig) {
			plain := filepath.Join(root, "plain")
			if err := os.MkdirAll(plain, 0o700); err != nil {
				t.Fatal(err)
			}
			cfg.Projects[0].Path = plain
		}, "independent Git checkout root"},
		{"subfolder inherits parent Git", func(t *testing.T, root string, cfg *config.MCPConfig) {
			sub := filepath.Join(root, "work-search", "docs")
			if err := os.MkdirAll(sub, 0o700); err != nil {
				t.Fatal(err)
			}
			cfg.Projects[0].Path = sub
		}, "independent Git checkout root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, cfg := projectGroupFixture(t)
			tt.change(t, root, &cfg)
			_, err := New(cfg)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("wanted %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestGroupRegistryFailClosedWhenFeatureDisabled(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	cfg.ProjectGroupsEnabled = false
	manager, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(manager.ListProjectGroups()) != 0 {
		t.Fatal("feature-disabled registry exposed dormant group")
	}
	if _, err := manager.ResolveProjectGroupSource("job-search", "work"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled feature should fail closed: %v", err)
	}
	meta := map[string]any{"openai/session": "legacy-single-project"}
	off := false
	info, err := manager.Select(meta, "work-search", &off)
	if err != nil {
		t.Fatalf("legacy project Select regressed: %v", err)
	}
	wantRoot, err := workspace.Canonical(filepath.Join(root, "work-search"))
	if err != nil {
		t.Fatal(err)
	}
	if info.ManagedWorktree || info.Mode != "project" || !sameGroupPath(info.ProjectRoot, wantRoot) {
		t.Fatalf("legacy project behavior changed: %+v", info)
	}
}

func TestGroupRegistryRejectsCheckoutReplacement(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	manager, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, "hh-mcp-server")
	backup := filepath.Join(root, "hh-old")
	if err := os.Rename(old, backup); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, old)
	if _, err := manager.ResolveProjectGroupSource("job-search", "hh"); err == nil || !strings.Contains(err.Error(), "replaced") {
		t.Fatalf("replaced checkout was accepted: %v", err)
	}
	if _, err := manager.ResolveProjectGroupSource("job-search", "work"); err != nil {
		t.Fatalf("unaffected group member should work: %v", err)
	}
}

func TestGroupRegistryMembershipChangeChangesFingerprint(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	first, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := first.ListProjectGroups()[0].MembershipFingerprint
	cfg.ProjectGroups[0].Repositories[1].Checkout = "managed"
	second, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b := second.ListProjectGroups()[0].MembershipFingerprint
	if a == b {
		t.Fatal("changing checkout mode did not invalidate group fingerprint")
	}
}
