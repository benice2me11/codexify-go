package upstream

import (
	"bufio"
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

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoInput struct {
	Text string `json:"text"`
}

func TestGatewayModeWritesSkillAndUsesSingleTool(t *testing.T) {
	upstreamServer := mcp.NewServer(&mcp.Implementation{Name: "gateway-fixture", Version: "1.0.0"}, nil)
	mcp.AddTool(upstreamServer, &mcp.Tool{Name: "echo", Description: "echo text"},
		func(_ context.Context, _ *mcp.CallToolRequest, in echoInput) (*mcp.CallToolResult, echoOutput, error) {
			return nil, echoOutput{Text: in.Text}, nil
		})
	mcp.AddTool(upstreamServer, &mcp.Tool{Name: "second", Description: "second function"},
		func(_ context.Context, _ *mcp.CallToolRequest, in echoInput) (*mcp.CallToolResult, echoOutput, error) {
			return nil, echoOutput{Text: "second:" + in.Text}, nil
		})
	upstreamHTTP := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return upstreamServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	))
	defer upstreamHTTP.Close()

	downstream := mcp.NewServer(&mcp.Implementation{Name: "downstream", Version: "1"}, nil)
	generated := t.TempDir()
	bridge, err := ConnectAndRegister(context.Background(), []config.UpstreamMCPConfig{
		{Name: "fixture_gateway", URL: upstreamHTTP.URL, Transport: "streamable_http", Mode: "gateway"},
	}, downstream, slog.New(slog.NewTextHandler(io.Discard, nil)), map[string]struct{}{}, 1<<20, generated)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	downstreamHTTP := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return downstream },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	))
	defer downstreamHTTP.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             downstreamHTTP.URL,
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
	var gatewayName string
	for _, tool := range listed.Tools {
		if strings.Contains(tool.Description, "Gateway to the") {
			gatewayName = tool.Name
		}
	}
	if gatewayName == "" {
		t.Fatalf("gateway tool missing: %+v", listed.Tools)
	}
	called, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: gatewayName,
		Arguments: map[string]any{
			"function":  "echo",
			"arguments": map[string]any{"text": "gateway works"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredText(t, called.StructuredContent, "gateway works")

	skillPath := filepath.Join(generated, "fixture_gateway", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{"name: fixture_gateway", "## echo", "## second", gatewayName} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated skill missing %q:\n%s", expected, text)
		}
	}
}

type echoOutput struct {
	Text string `json:"text"`
}

type emptyInput struct{}

type resourceOutput struct {
	OK bool `json:"ok"`
}

func TestBridgeCatalogAndDirectModes(t *testing.T) {
	upstreamServer := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1.0.0"}, nil)
	mcp.AddTool(upstreamServer, &mcp.Tool{Name: "echo", Description: "echo text"},
		func(_ context.Context, _ *mcp.CallToolRequest, in echoInput) (*mcp.CallToolResult, echoOutput, error) {
			return nil, echoOutput{Text: in.Text}, nil
		})
	upstreamServer.AddResource(&mcp.Resource{URI: "fixture://document/1", Name: "fixture-document", MIMEType: "text/plain"},
		func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
				URI: "fixture://document/1", MIMEType: "text/plain", Text: "bridged resource body",
			}}}, nil
		})
	mcp.AddTool(upstreamServer, &mcp.Tool{Name: "resource_link", Description: "return a resource link"},
		func(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, resourceOutput, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ResourceLink{
				URI: "fixture://document/1", Name: "fixture-document", MIMEType: "text/plain",
			}}}, resourceOutput{OK: true}, nil
		})
	upstreamHTTP := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return upstreamServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	))
	defer upstreamHTTP.Close()

	downstream := mcp.NewServer(&mcp.Implementation{Name: "downstream", Version: "1"}, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bridge, err := ConnectAndRegister(context.Background(), []config.UpstreamMCPConfig{
		{Name: "private", URL: upstreamHTTP.URL, Transport: "streamable_http", Mode: "catalog"},
		{Name: "direct", URL: upstreamHTTP.URL, Transport: "streamable_http", Mode: "direct"},
	}, downstream, logger, map[string]struct{}{}, 1<<20, "")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	downstreamHTTP := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return downstream },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	))
	defer downstreamHTTP.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             downstreamHTTP.URL,
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
	names := map[string]bool{}
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	for _, expected := range []string{"direct__echo", "mcp_list_sources", "mcp_search_tools", "mcp_get_tool", "mcp_call_tool"} {
		if !names[expected] {
			t.Fatalf("missing tool %q in %#v", expected, names)
		}
	}

	direct, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "direct__echo",
		Arguments: map[string]any{"text": "direct works"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if direct.IsError {
		t.Fatalf("direct call failed: %+v", direct.Content)
	}
	assertStructuredText(t, direct.StructuredContent, "direct works")

	catalog, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "mcp_call_tool",
		Arguments: map[string]any{
			"source":    "private",
			"name":      "echo",
			"arguments": map[string]any{"text": "catalog works"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.IsError {
		t.Fatalf("catalog call failed: %+v", catalog.Content)
	}
	assertStructuredText(t, catalog.StructuredContent, "catalog works")

	resourceCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "mcp_call_tool",
		Arguments: map[string]any{
			"source":    "private",
			"name":      "resource_link",
			"arguments": map[string]any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var link *mcp.ResourceLink
	for _, content := range resourceCall.Content {
		if candidate, ok := content.(*mcp.ResourceLink); ok {
			link = candidate
			break
		}
	}
	if link == nil || !strings.HasPrefix(link.URI, resourcePrefix) {
		t.Fatalf("resource link was not rewritten: %+v", resourceCall.Content)
	}
	read, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: link.URI})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Contents) != 1 || read.Contents[0].Text != "bridged resource body" || read.Contents[0].URI != link.URI {
		t.Fatalf("unexpected bridged resource: %+v", read.Contents)
	}
}

func TestOptionalUpstreamFailureIsReported(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "downstream", Version: "1"}, nil)
	bridge, err := ConnectAndRegister(context.Background(), []config.UpstreamMCPConfig{
		{Name: "offline", URL: "http://127.0.0.1:1/mcp", Transport: "streamable_http", Mode: "catalog", Required: false},
	}, server, slog.New(slog.NewTextHandler(io.Discard, nil)), map[string]struct{}{}, 1<<20, "")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	report := bridge.Report()
	if len(report) != 1 || !strings.Contains(report[0], "FAILED") {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestConnectSourceHonorsLegacyProtocolVersion(t *testing.T) {
	var spec config.UpstreamMCPConfig
	if err := json.Unmarshal([]byte(`{"protocolVersion":"2025-11-25"}`), &spec); err != nil {
		t.Fatal(err)
	}
	spec.Name = "legacy"
	spec.Transport = "stdio"
	spec.Mode = "catalog"
	spec.Command = os.Args[0]
	spec.Args = []string{"-test.run=^TestLegacyStdioHelper$"}
	spec.Env = map[string]string{"CODEXIFY_GO_LEGACY_STDIO_HELPER": "1"}

	src, err := connectSource(context.Background(), spec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("legacy stdio upstream failed to connect: %v", err)
	}
	defer src.session.Close()
	if src.info == nil || src.info.Name != "legacy-helper" {
		t.Fatalf("unexpected server info: %+v", src.info)
	}
	if len(src.tools) != 1 || src.tools["echo"] == nil {
		t.Fatalf("unexpected tools: %#v", src.tools)
	}
}

func TestLegacyStdioHelper(t *testing.T) {
	if os.Getenv("CODEXIFY_GO_LEGACY_STDIO_HELPER") != "1" {
		return
	}
	type request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			os.Exit(2)
		}
		switch req.Method {
		case "server/discover":
			// Simulate older stdio servers that close instead of returning
			// MethodNotFound for the 2026-07-28 discovery probe.
			os.Exit(17)
		case "initialize":
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil || params.ProtocolVersion != "2025-11-25" {
				os.Exit(18)
			}
			if err := encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": "2025-11-25",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "legacy-helper", "version": "1"},
				},
			}); err != nil {
				os.Exit(3)
			}
		case "notifications/initialized":
			continue
		case "tools/list":
			if err := encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"tools": []any{map[string]any{
						"name":        "echo",
						"description": "fixture",
						"inputSchema": map[string]any{"type": "object"},
					}},
				},
			}); err != nil {
				os.Exit(4)
			}
		default:
			if len(req.ID) > 0 && string(req.ID) != "null" {
				_ = encoder.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"error":   map[string]any{"code": -32601, "message": "method not found"},
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		os.Exit(5)
	}
}

func assertStructuredText(t *testing.T, value any, want string) {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("structured content type = %T, value=%#v", value, value)
	}
	if got, _ := m["text"].(string); got != want {
		t.Fatalf("text=%q want=%q", got, want)
	}
}
