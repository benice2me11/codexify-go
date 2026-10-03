package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/projects"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func setupContextFixture(t *testing.T) (*Runtime, *mcp.ClientSession) {
	t.Helper()
	cfg := config.Default()
	cfg.MCP.WorkspaceRoot = t.TempDir()
	cfg.MCP.MultiProject = true
	cfg.MCP.BindingsDir = filepath.Join(t.TempDir(), "bindings")
	cfg.MCP.Worktrees = config.WorktreeConfig{Mode: "never", Root: filepath.Join(t.TempDir(), "worktrees")}
	cfg.MCP.AuthEnabled = false
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:0/mcp"
	cfg.Tunnel.Executable = filepath.Join(t.TempDir(), "unused.exe")
	cfg.Tunnel.TunnelID = "tunnel_test"
	cfg.Tunnel.APIKeyRef = "env:TEST"
	for _, name := range []string{"project-a", "project-b"} {
		if err := os.MkdirAll(filepath.Join(cfg.MCP.WorkspaceRoot, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	r, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(r.Handler())
	client := mcp.NewClient(&mcp.Implementation{Name: "setup-context-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close(); server.Close(); r.listener.Close(); r.exec.Close() })
	if session.InitializeResult().ProtocolVersion != "2026-07-28" {
		t.Fatal("test must exercise stateless MCP")
	}
	return r, session
}

func setupContextCall(t *testing.T, session *mcp.ClientSession, meta mcp.Meta, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Meta: meta, Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

func TestSetupContextStatelessIsolation(t *testing.T) {
	_, session := setupContextFixture(t)
	metaA, metaB := mcp.Meta{"openai/session": "anonymous-A"}, mcp.Meta{"openai/session": "anonymous-B"}
	selectProject := func(meta mcp.Meta, project string) string {
		t.Helper()
		result := setupContextCall(t, session, meta, "set_project_root", map[string]any{"path": project})
		if result.IsError {
			t.Fatalf("select failed: %+v", result)
		}
		data, _ := json.Marshal(result.StructuredContent)
		var out projects.WorkspaceInfo
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}
		return out.ProjectRoot
	}
	rootA, rootB := selectProject(metaA, "project-a"), selectProject(metaB, "project-b")
	issue := func(meta mcp.Meta) string {
		t.Helper()
		result := setupContextCall(t, session, meta, "list_projects", map[string]any{})
		if result.IsError {
			t.Fatalf("list failed: %+v", result)
		}
		token, _ := result.Meta["io.github.devnoname120/codexify/setup-context"].(string)
		if len(token) != 43 {
			t.Fatal("missing opaque widget-only setup context")
		}
		visible, _ := json.Marshal([]any{result.Content, result.StructuredContent})
		if strings.Contains(string(visible), token) {
			t.Fatal("setup context leaked to model output")
		}
		return token
	}
	tokenA, tokenB := issue(metaA), issue(metaB)
	if tokenA == tokenB {
		t.Fatal("conversations share a context")
	}
	assertWorkspace := func(meta mcp.Meta, want string) {
		t.Helper()
		result := setupContextCall(t, session, meta, "get_environment", map[string]any{})
		if result.IsError || !strings.Contains(fmt.Sprint(result.StructuredContent), want) {
			t.Fatalf("workspace changed: %+v; want %s", result, want)
		}
	}
	for _, tc := range []struct {
		name string
		meta mcp.Meta
		tool string
		args map[string]any
	}{
		{"cross conversation", metaB, "setup_ui_switch_project", map[string]any{"uiContext": tokenA, "expectedPath": rootB}},
		{"native UUID is not a context", mcp.Meta{"thread_id": "native-A", "threadId": "native-A"}, "setup_ui_switch_project", map[string]any{"expectedPath": rootA}},
		{"unknown context", nil, "setup_ui_switch_project", map[string]any{"uiContext": strings.Repeat("A", 43), "expectedPath": rootA}},
		{"malformed context", nil, "setup_ui_switch_project", map[string]any{"uiContext": 13, "expectedPath": rootA}},
		{"other tool", nil, "get_environment", map[string]any{"uiContext": tokenA}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := setupContextCall(t, session, tc.meta, tc.tool, tc.args)
			if !result.IsError {
				t.Fatalf("invalid context call ran: %+v", result)
			}
			assertWorkspace(metaA, rootA)
			assertWorkspace(metaB, rootB)
		})
	}
	switched := setupContextCall(t, session, nil, "setup_ui_switch_project", map[string]any{"uiContext": tokenA, "expectedPath": rootA})
	if switched.IsError {
		t.Fatalf("own UI switch failed: %+v", switched)
	}
	if !setupContextCall(t, session, metaA, "get_environment", map[string]any{}).IsError {
		t.Fatal("A remained selected after its UI switch")
	}
	assertWorkspace(metaB, rootB)
	selected := setupContextCall(t, session, nil, "set_project_root", map[string]any{"uiContext": tokenA, "path": "project-b"})
	if selected.IsError {
		t.Fatalf("own UI select failed: %+v", selected)
	}
	assertWorkspace(metaA, rootB)
	assertWorkspace(metaB, rootB)
	if !setupContextCall(t, session, nil, "get_environment", map[string]any{}).IsError {
		t.Fatal("UI request context leaked to a later anonymous call")
	}
}

func TestSetupContextLifetimeCapacityAndRestart(t *testing.T) {
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	store := setupContextStore{now: func() time.Time { return now }}
	meta := map[string]any{"openai/session": "A"}
	token, err := store.issue(meta)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := store.issue(meta); again != token {
		t.Fatal("same conversation acquired duplicate contexts")
	}
	now = now.Add(30*time.Minute - time.Nanosecond)
	if _, err := store.resolve(token, nil); err != nil {
		t.Fatal("context expired before its deadline")
	}
	now = now.Add(time.Nanosecond)
	if _, err := store.resolve(token, nil); err == nil {
		t.Fatal("context remained valid at expiry")
	}
	renewed, err := store.issue(meta)
	if err != nil || renewed == token {
		t.Fatal("new model call did not replace expired context")
	}
	restarted := setupContextStore{}
	if _, err := restarted.resolve(renewed, nil); err == nil {
		t.Fatal("a pre-restart context was accepted")
	}
	for i := 1; i < 1024; i++ {
		if _, err := store.issue(map[string]any{"openai/session": fmt.Sprintf("A-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.entries) != 1024 {
		t.Fatalf("unexpected active contexts: %d", len(store.entries))
	}
	if _, err := store.issue(map[string]any{"openai/session": "overflow"}); err == nil {
		t.Fatal("context limit was exceeded")
	}
	if _, err := store.resolve(renewed, nil); err != nil {
		t.Fatal("full store evicted an active card")
	}
	now = now.Add(30 * time.Minute)
	if _, err := store.issue(meta); err != nil || len(store.entries) != 1 {
		t.Fatal("expired contexts were not reclaimed")
	}
	if token, err := store.issue(map[string]any{"thread_id": "A"}); err != nil || token != "" {
		t.Fatal("native thread ID acquired a context")
	}
}

func TestSetupContextConcurrentIssue(t *testing.T) {
	var store setupContextStore
	tokens := make(chan string, 32)
	errors := make(chan error, 32)
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			token, err := store.issue(map[string]any{"openai/session": "one"})
			tokens <- token
			errors <- err
		})
	}
	group.Wait()
	close(tokens)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first string
	for token := range tokens {
		if first == "" {
			first = token
		}
		if token != first {
			t.Fatal("concurrent issue created multiple contexts")
		}
	}
	if len(store.entries) != 1 {
		t.Fatal("context store grew for the same identity")
	}
}
