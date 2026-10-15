package mcpserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pgFixture struct {
	root    string
	paths   map[string]string
	runtime *Runtime
	session *mcp.ClientSession
	meta    mcp.Meta
}

func newPGFixture(t *testing.T) *pgFixture {
	t.Helper()
	root := t.TempDir()
	paths := map[string]string{
		"work": filepath.Join(root, "work-search"),
		"hh":   filepath.Join(root, "hh-mcp-server"),
		"habr": filepath.Join(root, "habr-career-mcp"),
	}
	for alias, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		runTestGit(t, path, "init")
		for name, content := range map[string]string{
			"README.md":   "project-" + alias + "\n",
			"tracked.txt": "before-" + alias + "\n",
			"AGENTS.md":   "instructions-for-" + alias + "\n",
		} {
			if err := os.WriteFile(filepath.Join(path, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		runTestGit(t, path, "add", ".")
		runTestGit(t, path, "-c", "user.name=PG Fixture", "-c", "user.email=pg@example.invalid", "commit", "-m", "baseline")
	}
	cfg := config.Default()
	cfg.MCP.WorkspaceRoot = root
	cfg.MCP.AuthEnabled = false
	cfg.MCP.MultiProject = true
	cfg.MCP.ProjectGroupsEnabled = true
	cfg.MCP.BindingsDir = filepath.Join(root, ".state", "bindings")
	cfg.MCP.Worktrees = config.WorktreeConfig{Mode: "auto", Root: filepath.Join(root, ".state", "worktrees")}
	cfg.MCP.Projects = []config.ProjectSpec{
		{Name: "work-search", Path: paths["work"]},
		{Name: "hh-mcp-server", Path: paths["hh"]},
		{Name: "habr-career-mcp", Path: paths["habr"]},
	}
	cfg.MCP.ProjectGroups = []config.ProjectGroupSpec{{
		Name: "job-search", Primary: "work",
		Repositories: []config.ProjectGroupRepository{
			{Alias: "work", Project: "work-search", Checkout: "source"},
			{Alias: "hh", Project: "hh-mcp-server", Checkout: "source"},
			{Alias: "habr", Project: "habr-career-mcp", Checkout: "source"},
		},
	}}
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:0/mcp"
	cfg.Tunnel.Executable = filepath.Join(t.TempDir(), "unused")
	cfg.Tunnel.TunnelID = "pg-routing-fixture"
	cfg.Tunnel.APIKeyRef = "env:TEST"
	cfg.Memory.Dir = filepath.Join(t.TempDir(), "memory")
	cfg.Skills.IncludeUser = false
	cfg.ArtifactEgress.Dir = filepath.Join(t.TempDir(), "artifacts")
	cfg.ArtifactEgress.MaxSnapshotBytes = 4 << 20
	cfg.ArtifactEgress.SnapshotMaxFileBytes = 1 << 20
	r, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.listener.Close()
		r.exec.Close()
		if r.bridge != nil {
			r.bridge.Close()
		}
	})
	srv := httptest.NewServer(r.Handler())
	t.Cleanup(srv.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "pg-route-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL + "/mcp", DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return &pgFixture{root: root, paths: paths, runtime: r, session: session, meta: mcp.Meta{"openai/session": "pg-chat-1"}}
}

func pgCall(t *testing.T, f *pgFixture, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{
		Meta: f.meta, Name: name, Arguments: args,
	})
	if err != nil {
		t.Fatalf("%s transport: %v", name, err)
	}
	if result == nil || result.IsError {
		t.Fatalf("%s failed: %+v", name, result)
	}
	return result
}

func pgValue(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	value, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected structured output map, got %T", result.StructuredContent)
	}
	return value
}

func pgExpectError(t *testing.T, f *pgFixture, name string, args map[string]any) {
	t.Helper()
	result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Meta: f.meta, Name: name, Arguments: args})
	if err == nil && result != nil && !result.IsError {
		t.Fatalf("%s unexpectedly succeeded with %+v: %+v", name, args, result)
	}
}

func TestProjectGroupEndToEndRoutingAcrossThreeRepos(t *testing.T) {
	f := newPGFixture(t)
	listed, err := f.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]bool{}
	for _, tool := range listed.Tools {
		tools[tool.Name] = true
	}
	for _, required := range []string{"list_project_groups", "set_project_group", "get_project_group_status", "set_active_repository", "read_file", "write_file", "glob", "grep", "apply_patch", "exec_command", "write_stdin", "git_status", "git_diff", "git_log", "show_diff", "list_worktrees"} {
		if !tools[required] {
			t.Fatalf("missing required Project Groups MCP tool %s", required)
		}
	}
	list := pgValue(t, pgCall(t, f, "list_project_groups", nil))
	if !strings.Contains(fmt.Sprint(list), "job-search") {
		t.Fatalf("catalog not available: %+v", list)
	}
	created := pgValue(t, pgCall(t, f, "set_project_group", map[string]any{"name": "job-search"}))
	if created["groupName"] != "job-search" || created["activeRepository"] != "work" {
		t.Fatalf("bad v3 selection: %+v", created)
	}
	if got := pgValue(t, pgCall(t, f, "get_project_group_status", nil)); got["generation"] != float64(1) {
		t.Fatalf("bad initial generation: %+v", got)
	}

	// An explicit repository argument is atomic and does not switch defaults.
	for alias, marker := range map[string]string{"work": "project-work", "hh": "project-hh", "habr": "project-habr"} {
		read := pgValue(t, pgCall(t, f, "read_file", map[string]any{"repository": alias, "path": "README.md"}))
		if !strings.Contains(fmt.Sprint(read["content"]), marker) {
			t.Fatalf("read %s returned wrong member content: %+v", alias, read)
		}
		ctx, ok := read["repository"].(map[string]any)
		if !ok || ctx["alias"] != alias || ctx["checkout"] != "source" {
			t.Fatalf("read %s missing repository identity: %+v", alias, read)
		}
		if _, err := os.Stat(filepath.Join(f.paths[alias], "README.md")); err != nil {
			t.Fatal(err)
		}
	}
	if got := pgValue(t, pgCall(t, f, "get_project_group_status", nil)); got["activeRepository"] != "work" {
		t.Fatalf("explicit reads changed default repository: %+v", got)
	}

	for _, alias := range []string{"work", "hh", "habr"} {
		out := pgValue(t, pgCall(t, f, "write_file", map[string]any{
			"repository": alias, "path": "new.txt", "content": "unique-" + alias + "\n",
		}))
		ctx, _ := out["repository"].(map[string]any)
		if ctx["alias"] != alias {
			t.Fatalf("write %s selected wrong checkout: %+v", alias, out)
		}
		contents, err := os.ReadFile(filepath.Join(f.paths[alias], "new.txt"))
		if err != nil || string(contents) != "unique-"+alias+"\n" {
			t.Fatalf("file %s not written independently: %q, %v", alias, contents, err)
		}
		glob := pgValue(t, pgCall(t, f, "glob", map[string]any{"repository": alias, "pattern": "*.txt"}))
		if !strings.Contains(fmt.Sprint(glob), "new.txt") {
			t.Fatalf("glob %s missing its own file: %+v", alias, glob)
		}
		grep := pgValue(t, pgCall(t, f, "grep", map[string]any{"repository": alias, "path": "new.txt", "pattern": "unique-"}))
		if !strings.Contains(fmt.Sprint(grep), "unique-"+alias) {
			t.Fatalf("grep %s missing its own content: %+v", alias, grep)
		}
	}

	patch := "*** Begin Patch\n*** Update File: tracked.txt\n@@\n-before-hh\n+after-hh\n*** End Patch"
	patched := pgValue(t, pgCall(t, f, "apply_patch", map[string]any{"repository": "hh", "input": patch}))
	if ctx, _ := patched["repository"].(map[string]any); ctx["alias"] != "hh" {
		t.Fatalf("patch routed to wrong member: %+v", patched)
	}
	hhData, _ := os.ReadFile(filepath.Join(f.paths["hh"], "tracked.txt"))
	habrData, _ := os.ReadFile(filepath.Join(f.paths["habr"], "tracked.txt"))
	if string(hhData) != "after-hh\n" || string(habrData) != "before-habr\n" {
		t.Fatalf("patch crossed source repositories: hh=%q habr=%q", hhData, habrData)
	}
	pgExpectError(t, f, "read_file", map[string]any{"repository": "hh", "path": "../habr-career-mcp/README.md"})
	pgExpectError(t, f, "write_file", map[string]any{"repository": "missing", "path": "hello", "content": "bad"})
	pgExpectError(t, f, "apply_patch", map[string]any{"repository": "hh", "input": "*** Begin Patch\n*** Add File: ../habr-career-mcp/escaped.txt\n+bad\n*** End Patch"})

	for _, alias := range []string{"work", "hh", "habr"} {
		status := pgValue(t, pgCall(t, f, "git_status", map[string]any{"repository": alias}))
		ctx, _ := status["repository"].(map[string]any)
		if ctx["alias"] != alias || !strings.Contains(fmt.Sprint(status["output"]), "new.txt") {
			t.Fatalf("status in wrong repository: %+v", status)
		}
		log := pgValue(t, pgCall(t, f, "git_log", map[string]any{"repository": alias, "count": 1}))
		if !strings.Contains(fmt.Sprint(log["output"]), "baseline") {
			t.Fatalf("missing own baseline commit for %s: %+v", alias, log)
		}
		diff := pgValue(t, pgCall(t, f, "git_diff", map[string]any{"repository": alias}))
		if alias == "hh" && !strings.Contains(fmt.Sprint(diff["output"]), "after-hh") {
			t.Fatalf("hh diff missing committed-root-specific tracked changes: %+v", diff)
		}
		worktrees := pgValue(t, pgCall(t, f, "list_worktrees", map[string]any{"repository": alias}))
		if !strings.Contains(fmt.Sprint(worktrees), filepath.Base(f.paths[alias])) {
			t.Fatalf("list_worktrees wrong source root for %s: %+v", alias, worktrees)
		}
	}

	// Diff owner remains the same conversation, while checkout identities keep
	// independent last-diff and project-open checkpoints.
	for _, alias := range []string{"hh", "habr"} {
		shown := pgValue(t, pgCall(t, f, "show_diff", map[string]any{
			"repository": alias, "since": "project_open", "advance": true,
		}))
		if !strings.Contains(fmt.Sprint(shown), "new.txt") {
			t.Fatalf("first diff for %s missing own file: %+v", alias, shown)
		}
		if ctx, _ := shown["repository"].(map[string]any); ctx["alias"] != alias {
			t.Fatalf("show_diff has wrong repository context: %+v", shown)
		}
	}
	hhAfter := pgValue(t, pgCall(t, f, "show_diff", map[string]any{"repository": "hh", "since": "last_diff", "advance": false}))
	if !strings.Contains(fmt.Sprint(hhAfter), "No changes since last diff") {
		t.Fatalf("hh incremental cursor was not independent: %+v", hhAfter)
	}

	pgCall(t, f, "set_active_repository", map[string]any{"repository": "habr"})
	defaultRead := pgValue(t, pgCall(t, f, "read_file", map[string]any{"path": "README.md"}))
	if !strings.Contains(fmt.Sprint(defaultRead["content"]), "project-habr") {
		t.Fatalf("implicit read did not switch to habr: %+v", defaultRead)
	}
	explicitWork := pgValue(t, pgCall(t, f, "read_file", map[string]any{"repository": "work", "path": "README.md"}))
	if !strings.Contains(fmt.Sprint(explicitWork["content"]), "project-work") {
		t.Fatalf("explicit work override did not route: %+v", explicitWork)
	}
	env := pgValue(t, pgCall(t, f, "get_environment", map[string]any{"repository": "hh"}))
	if env["groupName"] != "job-search" || env["activeRepository"] != "habr" || env["effectiveRepository"] != "hh" {
		t.Fatalf("environment missing active/effective distinction: %+v", env)
	}
	brief := pgValue(t, pgCall(t, f, "get_agent_brief", map[string]any{"repository": "hh"}))
	if !strings.Contains(fmt.Sprint(brief), "instructions-for-hh") || strings.Contains(fmt.Sprint(brief), "instructions-for-habr") {
		t.Fatalf("per-repository instructions leaked: %+v", brief)
	}
	doc := pgValue(t, pgCall(t, f, "get_project_doc", map[string]any{"repository": "work"}))
	if !strings.Contains(fmt.Sprint(doc), "instructions-for-work") || strings.Contains(fmt.Sprint(doc), "instructions-for-hh") {
		t.Fatalf("per-repository AGENTS leaked: %+v", doc)
	}
	pgCall(t, f, "remember", map[string]any{"repository": "hh", "key": "pg-test", "value": "unique-hh-memory"})
	hhMemory := pgValue(t, pgCall(t, f, "recall", map[string]any{"repository": "hh"}))
	habrMemory := pgValue(t, pgCall(t, f, "recall", map[string]any{"repository": "habr"}))
	if !strings.Contains(fmt.Sprint(hhMemory), "unique-hh-memory") || strings.Contains(fmt.Sprint(habrMemory), "unique-hh-memory") {
		t.Fatalf("member memory scopes crossed: hh=%+v habr=%+v", hhMemory, habrMemory)
	}
	pgExpectError(t, f, "set_project_root", map[string]any{"path": "hh-mcp-server", "createWorktree": false})
}

func TestProjectGroupShellSessionCannotCrossConversationsOrMembers(t *testing.T) {
	f := newPGFixture(t)
	pgCall(t, f, "set_project_group", map[string]any{"name": "job-search"})
	command, shell := "echo beginning; sleep 5; echo finished", "sh"
	if runtime.GOOS == "windows" {
		command, shell = "echo beginning & ping -n 9 127.0.0.1 >nul & echo finished", "cmd"
	}
	started := pgValue(t, pgCall(t, f, "exec_command", map[string]any{
		"repository": "hh", "cmd": command, "shell": shell, "yield_time_ms": 40,
	}))
	id, ok := started["session_id"].(string)
	if !ok || id == "" || started["running"] != true {
		t.Fatalf("long exec session not started: %+v", started)
	}
	if ctx, _ := started["repository"].(map[string]any); ctx["alias"] != "hh" {
		t.Fatalf("exec_command selected unexpected checkout: %+v", started)
	}
	pgCall(t, f, "set_active_repository", map[string]any{"repository": "habr"})
	// Knowledge of an opaque session ID must never grant control to another
	// conversation, even when that caller can also access the same group.
	foreign, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{
		Meta: mcp.Meta{"openai/session": "pg-chat-foreign"},
		Name: "write_stdin", Arguments: map[string]any{"session_id": id, "chars": ""},
	})
	if err == nil && (foreign == nil || !foreign.IsError) {
		t.Fatalf("foreign conversation could access original shell session: %+v", foreign)
	}
	// Give the second conversation its own valid group membership; even this
	// must not authorize stdin to the first conversation's process.
	secondSelection, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{
		Meta: mcp.Meta{"openai/session": "pg-chat-foreign"},
		Name: "set_project_group", Arguments: map[string]any{"name": "job-search"},
	})
	if err != nil || secondSelection.IsError {
		t.Fatalf("second conversation could not select its own group: %+v %v", secondSelection, err)
	}
	foreign, err = f.session.CallTool(context.Background(), &mcp.CallToolParams{
		Meta: mcp.Meta{"openai/session": "pg-chat-foreign"},
		Name: "write_stdin", Arguments: map[string]any{"session_id": id, "chars": ""},
	})
	if err == nil && (foreign == nil || !foreign.IsError) {
		t.Fatalf("second group binding could access foreign shell session: %+v", foreign)
	}
	polled := pgValue(t, pgCall(t, f, "write_stdin", map[string]any{"session_id": id, "chars": "", "yield_time_ms": 20}))
	if ctx, _ := polled["repository"].(map[string]any); ctx["alias"] != "hh" {
		t.Fatalf("session was retargeted when active changed: %+v", polled)
	}
	// Rebinding the same group to the same checkout must revoke a session from
	// the previous binding epoch, even though aliases and roots match.
	if polled["running"] == true {
		pgCall(t, f, "setup_ui_switch_project", map[string]any{})
		pgCall(t, f, "set_project_group", map[string]any{"name": "job-search"})
		pgExpectError(t, f, "write_stdin", map[string]any{"session_id": id, "chars": ""})
	}
}

func TestProjectGroupDisabledDoesNotAdvertiseGroupTools(t *testing.T) {
	r := testRuntime(t, false)
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "pg-disabled", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL + "/mcp", DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range result.Tools {
		if tool.Name == "set_project_group" || tool.Name == "set_active_repository" || tool.Name == "list_project_groups" {
			t.Fatalf("feature-disabled server advertised %s", tool.Name)
		}
	}
	unsafe, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "write_file", Arguments: map[string]any{
			"repository": "hh", "path": "should-not-exist.txt", "content": "no",
		},
	})
	if err == nil && (unsafe == nil || !unsafe.IsError) {
		t.Fatalf("legacy single-project mode accepted an unrelated alias: %+v", unsafe)
	}
}
