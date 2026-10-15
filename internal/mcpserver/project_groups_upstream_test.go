package mcpserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type groupUpstreamEchoInput struct {
	Text string `json:"text"`
}
type groupUpstreamEchoOutput struct {
	Text string `json:"text"`
}

func TestProjectGroupMemberSwitchDoesNotReconnectUpstream(t *testing.T) {
	var initializations atomic.Int64
	var requests atomic.Int64
	var echoes atomic.Int64
	upstream := mcp.NewServer(&mcp.Implementation{Name: "pg-readonly-upstream", Version: "1.0.0"}, nil)
	mcp.AddTool(upstream, &mcp.Tool{Name: "echo", Description: "safe read-only fixture echo"},
		func(_ context.Context, _ *mcp.CallToolRequest, in groupUpstreamEchoInput) (*mcp.CallToolResult, groupUpstreamEchoOutput, error) {
			echoes.Add(1)
			return nil, groupUpstreamEchoOutput{Text: in.Text}, nil
		})
	base := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return upstream },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, "bad fixture body", http.StatusBadRequest)
			return
		}
		if bytes.Contains(payload, []byte(`"method":"initialize"`)) ||
			bytes.Contains(payload, []byte(`"method":"server/discover"`)) {
			initializations.Add(1)
		}
		req.Body = io.NopCloser(bytes.NewReader(payload))
		base.ServeHTTP(w, req)
	}))
	defer srv.Close()
	f := newPGFixture(t, config.UpstreamMCPConfig{
		Name: "fixture_echo", URL: srv.URL, Transport: "streamable_http",
		Mode: "catalog", Required: true,
	})
	pgCall(t, f, "set_project_group", map[string]any{"name": "job-search"})
	before := initializations.Load()
	requestBaseline := requests.Load()
	if requestBaseline == 0 {
		t.Fatal("upstream fixture did not receive any initial connection request")
	}
	for i, alias := range []string{"work", "hh", "habr", "work", "habr", "hh", "work", "hh", "habr"} {
		pgCall(t, f, "set_active_repository", map[string]any{"repository": alias})
		pgCall(t, f, "mcp_list_sources", nil)
		pgCall(t, f, "mcp_search_tools", map[string]any{"source": "fixture_echo", "query": "echo"})
		result := pgCall(t, f, "mcp_call_tool", map[string]any{
			"source": "fixture_echo", "name": "echo",
			"arguments": map[string]any{"text": fmt.Sprintf("call-%d-%s", i, alias)},
		})
		if !bytes.Contains([]byte(fmt.Sprint(result.StructuredContent)), []byte(fmt.Sprintf("call-%d-%s", i, alias))) {
			t.Fatalf("upstream call was disrupted by member switching: %+v", result)
		}
	}
	if got := initializations.Load(); got != before {
		t.Fatalf("group switching created new upstream initialize calls: before=%d after=%d", before, got)
	}
	if got := echoes.Load(); got != 9 {
		t.Fatalf("expected exactly 9 explicit upstream echo calls, got %d", got)
	}
	if delta := requests.Load() - requestBaseline; delta != 9 {
		t.Fatalf("group switching caused extra upstream HTTP requests: got %d, expected 9 explicit echo calls", delta)
	}
	// One bridge is created with the Runtime, rather than on member selection.
	if f.runtime.bridge == nil {
		t.Fatal("group's read-only upstream bridge was unexpectedly closed")
	}
}
