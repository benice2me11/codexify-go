package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/mcpdiag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func diagnosticRuntime(t *testing.T, enabled bool) (*Runtime, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg := config.Default()
	cfg.MCP.WorkspaceRoot = t.TempDir()
	cfg.MCP.AuthEnabled = false
	cfg.MCP.Diagnostics.Enabled = enabled
	cfg.MCP.Diagnostics.Directory = filepath.Join(t.TempDir(), "capture")
	cfg.MCP.Diagnostics.MaxEvents = 1000
	cfg.ArtifactEgress.Dir = t.TempDir()
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:0/mcp"
	cfg.Tunnel.TunnelID = "isolated-diagnostics-test"
	cfg.Tunnel.APIKeyRef = "env:UNUSED_DIAGNOSTICS_TEST"
	r, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
		_ = r.listener.Close()
	})
	return r, cfg.MCP.Diagnostics.Directory
}

func TestRuntimeDiagnosticsDisabledHasNoArtifacts(t *testing.T) {
	r, dir := diagnosticRuntime(t, false)
	if r.diagnostics != nil {
		t.Fatal("diagnostics enabled by default")
	}
	_ = postJSON(t, r.Handler(), `{bad json`)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("disabled diagnostics created a path: %v", err)
	}
}

func TestRuntimeDiagnosticsModernAndLegacy(t *testing.T) {
	for _, mode := range []string{"modern", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			r, _ := diagnosticRuntime(t, true)
			if mode == "modern" {
				host := httptest.NewServer(r.Handler())
				defer host.Close()
				client := mcp.NewClient(&mcp.Implementation{Name: "diagnostics-fixture", Version: "1"}, nil)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: host.URL + "/mcp", DisableStandaloneSSE: true, MaxRetries: -1}, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := session.ListTools(ctx, nil); err != nil {
					t.Fatal(err)
				}
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Meta: mcp.Meta{"openai/session": "SECRET_RUNTIME_CONVERSATION"}, Name: "get_environment", Arguments: map[string]any{}})
				if err != nil || result.IsError {
					t.Fatalf("tool call failed: %v", err)
				}
				_ = session.Close()
				retained := 0
				for range r.server.Sessions() {
					retained++
				}
				if retained != 0 {
					t.Fatalf("modern session lifecycle changed: %d", retained)
				}
			} else {
				init := postJSON(t, r.Handler(), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"legacy-fixture","version":"1"}}}`)
				id := init.Header().Get("Mcp-Session-Id")
				if init.Code != 200 || id == "" {
					t.Fatalf("legacy init status=%d", init.Code)
				}
				_ = postJSON(t, r.Handler(), `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`, id)
				listed := postJSON(t, r.Handler(), `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, id)
				if listed.Code != 200 {
					t.Fatalf("legacy list status=%d", listed.Code)
				}
				called := postJSON(t, r.Handler(), `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_environment","arguments":{},"_meta":{"openai/session":"SECRET_RUNTIME_CONVERSATION"}}}`, id)
				if called.Code != 200 || strings.Contains(called.Body.String(), `"isError":true`) {
					t.Fatalf("legacy call status=%d", called.Code)
				}
				request := httptest.NewRequest(http.MethodDelete, "http://localhost/mcp", nil)
				request.Header.Set("Mcp-Session-Id", id)
				r.Handler().ServeHTTP(httptest.NewRecorder(), request)
			}
			_ = postJSON(t, r.Handler(), `{bad json`)
			if err := r.diagnostics.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(r.diagnostics.Path())
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "SECRET_RUNTIME_CONVERSATION") {
				t.Fatal("raw conversation leaked")
			}
			methods := map[string]int{}
			toolEvents := 0
			httpErrors := 0
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var event mcpdiag.Event
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					t.Fatal(err)
				}
				if event.Phase == "mcp_start" {
					methods[event.Method]++
					if event.HTTPID == 0 {
						t.Fatalf("%s HTTP correlation missing", mode)
					}
					if event.Tool == "get_environment" && event.ConversationHash != "" {
						toolEvents++
					}
				}
				if event.Phase == "http_end" && event.Status >= 400 {
					httpErrors++
				}
			}
			if methods["tools/list"] != 1 || methods["tools/call"] != 1 || toolEvents != 1 || httpErrors == 0 {
				t.Fatalf("methods=%v toolEvents=%d httpErrors=%d", methods, toolEvents, httpErrors)
			}
			if mode == "modern" && methods["server/discover"] != 1 {
				t.Fatalf("discovery not observed: %v", methods)
			}
			if mode == "legacy" && (methods["initialize"] != 1 || methods["notifications/initialized"] != 1) {
				t.Fatalf("legacy initialization not observed: %v", methods)
			}
			t.Logf("mode=%s decoded_methods=%v malformed_http_captured=%d", mode, methods, httpErrors)
		})
	}
}

func TestRuntimeDiagnosticsAuthRejection(t *testing.T) {
	r, _ := diagnosticRuntime(t, true)
	// Exercise the same configured auth wrapper without creating any tunnel.
	r.cfg.MCP.AuthEnabled = true
	r.token = "SECRET_EXPECTED_BEARER"
	h := r.diagnostics.Handler(r.auth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unauthorized request reached handler") })))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "http://localhost/mcp", strings.NewReader(`{"secret":"SECRET_AUTH_BODY"}`)))
	if response.Code != 401 {
		t.Fatalf("auth status=%d", response.Code)
	}
	_ = r.diagnostics.Close()
	data, err := os.ReadFile(r.diagnostics.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"http_status":401`) || strings.Contains(string(data), "SECRET_") {
		t.Fatal("unsafe or missing auth metadata")
	}
}
