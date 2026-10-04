package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/benice2me11/codexify-go/internal/artifacts"
	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/projects"
	"github.com/benice2me11/codexify-go/internal/ui"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testRuntime(t *testing.T, auth bool) *Runtime {
	t.Helper()
	cfg := config.Default()
	cfg.MCP.WorkspaceRoot = t.TempDir()
	cfg.MCP.AuthEnabled = auth
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:0/mcp"
	cfg.Tunnel.Executable = filepath.Join(t.TempDir(), "unused.exe")
	cfg.Tunnel.TunnelID = "tunnel_test"
	cfg.Tunnel.APIKeyRef = "env:TEST"
	r, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.listener.Close()
		r.exec.Close()
	})
	return r
}

func TestAuthMiddleware(t *testing.T) {
	r := testRuntime(t, true)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without auth status=%d", rec.Code)
	}
	_, env := r.TunnelEnvironment()
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", bytes.NewBufferString("{}"))
	req.Header.Set("Authorization", env[InternalAuthEnv])
	rec = httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("generated tunnel authorization was rejected")
	}
}

func TestMCPInitializeListAndFileTools(t *testing.T) {
	r := testRuntime(t, false)

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	initResp := postJSON(t, r.Handler(), initBody)
	if initResp.Code != http.StatusOK {
		t.Fatalf("initialize status=%d body=%s", initResp.Code, initResp.Body.String())
	}
	sessionID := initResp.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("stateful initialize did not return Mcp-Session-Id")
	}
	initializedBody := `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`
	_ = postJSON(t, r.Handler(), initializedBody, sessionID)

	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	listResp := postJSON(t, r.Handler(), listBody, sessionID)
	if listResp.Code != http.StatusOK {
		t.Fatalf("tools/list status=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var listed struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	foundRead := false
	foundExec := false
	for _, tool := range listed.Result.Tools {
		foundRead = foundRead || tool.Name == "read_file"
		foundExec = foundExec || tool.Name == "exec_command"
	}
	if !foundRead || !foundExec {
		t.Fatalf("expected tools missing: %+v", listed.Result.Tools)
	}

	writeBody := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"hello.txt","content":"hello\n"}}}`
	writeResp := postJSON(t, r.Handler(), writeBody, sessionID)
	if writeResp.Code != http.StatusOK {
		t.Fatalf("write_file status=%d body=%s", writeResp.Code, writeResp.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(r.root.Path(), "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("written data=%q", data)
	}

	readBody := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"hello.txt"}}}`
	readResp := postJSON(t, r.Handler(), readBody, sessionID)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read_file status=%d body=%s", readResp.Code, readResp.Body.String())
	}
	if !bytes.Contains(readResp.Body.Bytes(), []byte("1\\thello")) {
		t.Fatalf("read response=%s", readResp.Body.String())
	}
}

func TestOfficialClientNegotiatesCurrentProtocol(t *testing.T) {
	r := testRuntime(t, false)
	httpServer := httptest.NewServer(r.Handler())
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "codexify-go-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if got := session.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Fatalf("negotiated protocol version = %q, want 2026-07-28", got)
	}

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) < 8 {
		t.Fatalf("unexpected tool count: %d", len(listed.Tools))
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "write_file",
		Arguments: map[string]any{"path": "sdk.txt", "content": "sdk works\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(r.root.Path(), "sdk.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sdk works\n" {
		t.Fatalf("written data=%q", data)
	}
}

func TestLegacyTransportSessionBindingIsTransient(t *testing.T) {
	accessRoot := t.TempDir()
	projectRoot := filepath.Join(accessRoot, "project-a")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module example/project-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "hello.txt"), []byte("transport bound\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.MCP.WorkspaceRoot = accessRoot
	cfg.MCP.MultiProject = true
	cfg.MCP.BindingsDir = filepath.Join(accessRoot, ".state", "bindings")
	cfg.MCP.Worktrees = config.WorktreeConfig{Mode: "never", Root: filepath.Join(accessRoot, ".state", "worktrees")}
	cfg.MCP.AuthEnabled = false
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:0/mcp"
	cfg.Tunnel.Executable = filepath.Join(t.TempDir(), "unused.exe")
	cfg.Tunnel.TunnelID = "tunnel_test"
	cfg.Tunnel.APIKeyRef = "env:TEST"
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

	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"legacy-test","version":"1"}}}`
	initResp := postJSON(t, r.Handler(), initialize)
	if initResp.Code != http.StatusOK {
		t.Fatalf("initialize: %d %s", initResp.Code, initResp.Body.String())
	}
	sessionID := initResp.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("missing stateful session id")
	}
	_ = postJSON(t, r.Handler(), `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`, sessionID)

	selected := postJSON(t, r.Handler(), `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"set_project_root","arguments":{"path":"project-a"}}}`, sessionID)
	if selected.Code != http.StatusOK || !strings.Contains(selected.Body.String(), `"bindingScope":"transport_session"`) {
		t.Fatalf("transport selection failed: %d %s", selected.Code, selected.Body.String())
	}
	read := postJSON(t, r.Handler(), `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"hello.txt"}}}`, sessionID)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "transport bound") {
		t.Fatalf("transport-bound read failed: %d %s", read.Code, read.Body.String())
	}
	if entries, err := os.ReadDir(cfg.MCP.BindingsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				t.Fatalf("transient binding persisted to disk: %s", entry.Name())
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/mcp", nil)
	deleteReq.Header.Set("Mcp-Session-Id", sessionID)
	deleteReq.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	deleteRec := httptest.NewRecorder()
	r.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code < 200 || deleteRec.Code >= 300 {
		t.Fatalf("session delete failed: %d %s", deleteRec.Code, deleteRec.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := r.sessions.Load(sessionID); !ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := r.sessions.Load(sessionID); ok {
		t.Fatal("transport session cleanup watcher did not finish")
	}

	secondInit := postJSON(t, r.Handler(), initialize)
	secondID := secondInit.Header().Get("Mcp-Session-Id")
	if secondID == "" || secondID == sessionID {
		t.Fatalf("invalid second session id %q", secondID)
	}
	_ = postJSON(t, r.Handler(), `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`, secondID)
	unbound := postJSON(t, r.Handler(), `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"hello.txt"}}}`, secondID)
	if unbound.Code != http.StatusOK || !strings.Contains(unbound.Body.String(), "no project selected") {
		t.Fatalf("new transport session inherited old binding: %d %s", unbound.Code, unbound.Body.String())
	}
}

func TestMultiProjectConversationBindingOverMCP(t *testing.T) {
	accessRoot := t.TempDir()
	projectRoot := filepath.Join(accessRoot, "project-a")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module example/project-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "hello.txt"), []byte("bound workspace\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.MCP.WorkspaceRoot = accessRoot
	cfg.MCP.MultiProject = true
	cfg.MCP.ProjectScanDepth = 2
	cfg.MCP.BindingsDir = filepath.Join(accessRoot, ".state", "bindings")
	cfg.MCP.Worktrees = config.WorktreeConfig{Mode: "never", Root: filepath.Join(accessRoot, ".state", "worktrees")}
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:0/mcp"
	cfg.Tunnel.Executable = filepath.Join(t.TempDir(), "unused.exe")
	cfg.Tunnel.TunnelID = "tunnel_test"
	cfg.Tunnel.APIKeyRef = "env:TEST"
	cfg.MCP.AuthEnabled = false

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
	httpServer := httptest.NewServer(r.Handler())
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "binding-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	listed, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_projects",
		Arguments: map[string]any{"query": "project-a"},
	})
	if err != nil || listed.IsError {
		t.Fatalf("list_projects failed: err=%v result=%+v", err, listed)
	}

	metaA := mcp.Meta{"openai/session": "conversation-a"}
	selected, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Meta:      metaA,
		Name:      "set_project_root",
		Arguments: map[string]any{"path": "project-a", "createWorktree": false},
	})
	if err != nil || selected.IsError {
		t.Fatalf("set_project_root failed: err=%v result=%+v", err, selected)
	}

	read, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Meta:      metaA,
		Name:      "read_file",
		Arguments: map[string]any{"path": "hello.txt"},
	})
	if err != nil || read.IsError {
		t.Fatalf("bound read_file failed: err=%v result=%+v", err, read)
	}
	if !strings.Contains(fmt.Sprint(read.StructuredContent), "bound workspace") {
		t.Fatalf("unexpected read result: %#v", read.StructuredContent)
	}

	metaB := mcp.Meta{"openai/session": "conversation-b"}
	unbound, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Meta:      metaB,
		Name:      "read_file",
		Arguments: map[string]any{"path": "hello.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !unbound.IsError {
		t.Fatalf("unbound conversation unexpectedly inherited binding: %+v", unbound)
	}
}

func TestUIResourcesAndToolMetadata(t *testing.T) {
	r := testRuntime(t, false)
	httpServer := httptest.NewServer(r.Handler())
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "ui-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	metaByName := map[string]mcp.Meta{}
	for _, tool := range listed.Tools {
		metaByName[tool.Name] = tool.Meta
	}
	if got := metaByName["setup"]["ui/resourceUri"]; got != ui.SetupURI {
		t.Fatalf("setup UI resource = %#v", got)
	}
	if got, ok := metaByName["list_projects"]["ui/resourceUri"]; ok {
		t.Fatalf("list_projects must stay app-callable only, UI resource = %#v", got)
	}
	if got := metaByName["show_diff"]["ui/resourceUri"]; got != ui.DiffURI {
		t.Fatalf("show_diff UI resource = %#v", got)
	}
	if got := metaByName["self_update_status"]["ui/resourceUri"]; got != ui.UpdateURI {
		t.Fatalf("self_update_status UI resource = %#v", got)
	}

	setup, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: ui.SetupURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(setup.Contents) != 1 || setup.Contents[0].MIMEType != ui.MIMEType || !strings.Contains(setup.Contents[0].Text, "setup_status") {
		t.Fatalf("unexpected setup UI resource: %+v", setup.Contents)
	}
	legacy, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "ui://codexify-go/setup/v2/mcp-app.html"})
	if err != nil || len(legacy.Contents) != 1 || legacy.Contents[0].Text != ui.SetupHTML {
		t.Fatalf("cached setup v2 descriptor cannot read the current widget: %v", err)
	}
	diff, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: ui.DiffURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Contents) != 1 || !strings.Contains(diff.Contents[0].Text, "toolOutput") {
		t.Fatalf("unexpected diff UI resource: %+v", diff.Contents)
	}
	chat, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: ui.ChatURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.Contents) != 1 || !strings.Contains(chat.Contents[0].Text, "chat_write") {
		t.Fatalf("unexpected chat UI resource: %+v", chat.Contents)
	}
	update, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: ui.UpdateURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(update.Contents) != 1 || !strings.Contains(update.Contents[0].Text, "self_update_status") || strings.Contains(update.Contents[0].Text, "fetch(") {
		t.Fatalf("unexpected update UI resource: %+v", update.Contents)
	}
}

func TestShowDiffCheckpointsOverMCP(t *testing.T) {
	workspaceRoot := t.TempDir()
	runTestGit(t, workspaceRoot, "init")
	if err := os.WriteFile(filepath.Join(workspaceRoot, "tracked.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, workspaceRoot, "add", "tracked.txt")
	runTestGit(t, workspaceRoot, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "init")

	cfg := config.Default()
	cfg.MCP.WorkspaceRoot = workspaceRoot
	cfg.MCP.AuthEnabled = false
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:0/mcp"
	cfg.Tunnel.Executable = filepath.Join(t.TempDir(), "unused.exe")
	cfg.Tunnel.TunnelID = "tunnel_test"
	cfg.Tunnel.APIKeyRef = "env:TEST"
	cfg.ArtifactEgress.Dir = filepath.Join(t.TempDir(), "artifacts")
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
	httpServer := httptest.NewServer(r.Handler())
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "diff-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	write, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "write_file",
		Arguments: map[string]any{
			"path":    "tracked.txt",
			"content": "changed\n",
		},
	})
	if err != nil || write.IsError {
		t.Fatalf("write_file failed: err=%v result=%+v", err, write)
	}

	shown, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "show_diff",
		Arguments: map[string]any{
			"since":   "project_open",
			"advance": true,
		},
	})
	if err != nil || shown.IsError {
		t.Fatalf("show_diff failed: err=%v result=%+v", err, shown)
	}
	structured, ok := shown.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("show_diff structured type=%T", shown.StructuredContent)
	}
	if strings.Contains(fmt.Sprint(structured), "diff --git") {
		t.Fatalf("model-visible structured output leaked patch: %#v", structured)
	}
	if advanced, _ := structured["checkpointAdvanced"].(bool); !advanced {
		t.Fatalf("checkpoint did not advance: %#v", structured)
	}
	metaPayload, ok := shown.Meta["io.github.devnoname120/codexify/diff"].(map[string]any)
	if !ok {
		t.Fatalf("missing diff metadata: %#v", shown.Meta)
	}
	if patchText, _ := metaPayload["patch"].(string); !strings.Contains(patchText, "tracked.txt") {
		t.Fatalf("diff metadata missing patch: %#v", metaPayload)
	}

	incremental, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "show_diff",
		Arguments: map[string]any{"since": "last_diff", "advance": false},
	})
	if err != nil || incremental.IsError {
		t.Fatalf("incremental show_diff failed: err=%v result=%+v", err, incremental)
	}
	if !strings.Contains(fmt.Sprint(incremental.StructuredContent), "No changes since last diff") {
		t.Fatalf("expected empty incremental diff: %#v", incremental.StructuredContent)
	}
}

func TestArtifactMemoryAndSkillsOverMCP(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "report.txt"), []byte("immutable report\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(workspaceRoot, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Use for demo work\n---\n\nDo demo work.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.MCP.WorkspaceRoot = workspaceRoot
	cfg.MCP.AuthEnabled = false
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:0/mcp"
	cfg.Tunnel.Executable = filepath.Join(t.TempDir(), "unused.exe")
	cfg.Tunnel.TunnelID = "tunnel_test"
	cfg.Tunnel.APIKeyRef = "env:TEST"
	cfg.Memory.Dir = filepath.Join(t.TempDir(), "memory")
	cfg.Skills.IncludeUser = false
	cfg.ArtifactEgress.Dir = filepath.Join(t.TempDir(), "artifacts")
	cfg.ArtifactEgress.MaxFileBytes = 1 << 20
	cfg.ArtifactEgress.SnapshotMaxFileBytes = 1 << 20
	cfg.ArtifactEgress.MaxSnapshotBytes = 4 << 20
	cfg.ArtifactEgress.MaxReferences = 8

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
	httpServer := httptest.NewServer(r.Handler())
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "feature-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	remembered, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "remember",
		Arguments: map[string]any{"key": "approach", "value": "use the SDK"},
	})
	if err != nil || remembered.IsError {
		t.Fatalf("remember failed: err=%v result=%+v", err, remembered)
	}
	recalled, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "recall", Arguments: map[string]any{}})
	if err != nil || recalled.IsError || !strings.Contains(fmt.Sprint(recalled.StructuredContent), "use the SDK") {
		t.Fatalf("recall failed: err=%v result=%+v", err, recalled)
	}

	skillsResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "skills_list", Arguments: map[string]any{}})
	if err != nil || skillsResult.IsError || !strings.Contains(fmt.Sprint(skillsResult.StructuredContent), "demo") {
		t.Fatalf("skills_list failed: err=%v result=%+v", err, skillsResult)
	}
	readSkill, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "skills_read", Arguments: map[string]any{"name": "demo"}})
	if err != nil || readSkill.IsError || !strings.Contains(fmt.Sprint(readSkill.StructuredContent), "Do demo work") {
		t.Fatalf("skills_read failed: err=%v result=%+v", err, readSkill)
	}

	exported, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "export_host_file",
		Arguments: map[string]any{"path": "report.txt"},
	})
	if err != nil || exported.IsError {
		t.Fatalf("export_host_file failed: err=%v result=%+v", err, exported)
	}
	var link *mcp.ResourceLink
	for _, content := range exported.Content {
		if candidate, ok := content.(*mcp.ResourceLink); ok {
			link = candidate
			break
		}
	}
	if link == nil || !strings.HasPrefix(link.URI, artifacts.Prefix) {
		t.Fatalf("missing artifact resource link: %+v", exported.Content)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "report.txt"), []byte("changed after export\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resource, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: link.URI})
	if err != nil {
		t.Fatal(err)
	}
	if len(resource.Contents) != 1 || string(resource.Contents[0].Blob) != "immutable report\n" {
		t.Fatalf("artifact snapshot changed: %+v", resource.Contents)
	}
}

func TestMarkdownChatOverMCP(t *testing.T) {
	accessRoot := t.TempDir()
	projectRoot := filepath.Join(accessRoot, "project-a")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module example/chat\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.MCP.WorkspaceRoot = accessRoot
	cfg.MCP.MultiProject = true
	cfg.MCP.BindingsDir = filepath.Join(accessRoot, ".state", "bindings")
	cfg.MCP.Worktrees = config.WorktreeConfig{Mode: "never", Root: filepath.Join(accessRoot, ".state", "worktrees")}
	cfg.MCP.AuthEnabled = false
	cfg.AgentChat.Enabled = true
	cfg.AgentChat.Dir = filepath.Join(t.TempDir(), "chats")
	cfg.AgentChat.MaxWaitMS = 1500
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:0/mcp"
	cfg.Tunnel.Executable = filepath.Join(t.TempDir(), "unused.exe")
	cfg.Tunnel.TunnelID = "tunnel_chat_test"
	cfg.Tunnel.APIKeyRef = "env:TEST"

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
	httpServer := httptest.NewServer(r.Handler())
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "chat-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	chatMetaOK := false
	for _, tool := range listed.Tools {
		if tool.Name == "chat_read" && tool.Meta["ui/resourceUri"] == ui.ChatURI {
			chatMetaOK = true
		}
	}
	if !chatMetaOK {
		t.Fatal("chat_read does not advertise the Markdown chat MCP App")
	}

	meta := mcp.Meta{"openai/session": "markdown-chat-conversation"}
	selected, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Meta:      meta,
		Name:      "set_project_root",
		Arguments: map[string]any{"path": "project-a", "createWorktree": false},
	})
	if err != nil || selected.IsError {
		t.Fatalf("selection failed: err=%v result=%+v", err, selected)
	}

	brief, err := session.CallTool(context.Background(), &mcp.CallToolParams{Meta: meta, Name: "get_agent_brief", Arguments: map[string]any{}})
	if err != nil || brief.IsError || !strings.Contains(fmt.Sprint(brief.StructuredContent), "Markdown chat") {
		t.Fatalf("brief missing chat section: err=%v result=%+v", err, brief)
	}
	identity := projects.IdentityFromMeta(map[string]any{"openai/session": "markdown-chat-conversation"})
	selectedRoot, _, err := r.projects.Workspace(map[string]any{"openai/session": "markdown-chat-conversation"})
	if err != nil {
		t.Fatal(err)
	}
	chatPath, err := r.chat.Path(selectedRoot.Path(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(chatPath); err != nil {
		t.Fatalf("CHAT.md was not created by get_agent_brief: %v", err)
	}

	appendChat := func(text string) {
		t.Helper()
		file, err := os.OpenFile(chatPath, os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString(text); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	appendChat("user message one\n")
	read, err := session.CallTool(context.Background(), &mcp.CallToolParams{Meta: meta, Name: "chat_read", Arguments: map[string]any{}})
	if err != nil || read.IsError || !strings.Contains(fmt.Sprint(read.StructuredContent), "user message one") {
		t.Fatalf("chat_read failed: err=%v result=%+v", err, read)
	}
	written, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Meta:      meta,
		Name:      "chat_write",
		Arguments: map[string]any{"message": "agent answer one"},
	})
	if err != nil || written.IsError || !strings.Contains(fmt.Sprint(written.StructuredContent), "written") {
		t.Fatalf("chat_write failed: err=%v result=%+v", err, written)
	}

	go func() {
		time.Sleep(150 * time.Millisecond)
		file, openErr := os.OpenFile(chatPath, os.O_WRONLY|os.O_APPEND, 0o600)
		if openErr != nil {
			return
		}
		_, _ = file.WriteString("async user message\n")
		_ = file.Close()
	}()
	awaitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	awaited, err := session.CallTool(awaitCtx, &mcp.CallToolParams{Meta: meta, Name: "chat_await", Arguments: map[string]any{}})
	if err != nil || awaited.IsError || !strings.Contains(fmt.Sprint(awaited.StructuredContent), "async user message") {
		t.Fatalf("chat_await failed: err=%v result=%+v", err, awaited)
	}

	data, err := os.ReadFile(chatPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## Agent\n\nagent answer one") {
		t.Fatalf("CHAT.md missing agent block: %s", data)
	}
}

func postJSON(t *testing.T, handler http.Handler, body string, sessionID ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	if len(sessionID) > 0 && sessionID[0] != "" {
		req.Header.Set("Mcp-Session-Id", sessionID[0])
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestCurrentProtocolRequestsUseEphemeralSessions(t *testing.T) {
	r := testRuntime(t, false)
	httpServer := httptest.NewServer(r.Handler())
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "lifecycle-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if got := session.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Fatalf("negotiated protocol version = %q, want 2026-07-28", got)
	}

	// The current protocol is routed to the SDK's stateless handler. That handler
	// creates a temporary ServerSession for each HTTP request and closes it before
	// returning, so the server must have no retained transport session between calls.
	for i := 0; i < 5; i++ {
		if _, err := session.ListTools(context.Background(), nil); err != nil {
			t.Fatalf("ListTools #%d: %v", i+1, err)
		}
		count := 0
		for range r.server.Sessions() {
			count++
		}
		if count != 0 {
			t.Fatalf("after ListTools #%d retained server sessions = %d, want 0 for current stateless path", i+1, count)
		}
	}
}
