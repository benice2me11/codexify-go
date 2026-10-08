package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/benice2me11/codexify-go/internal/mcpdiag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOfflineCLI(t *testing.T) {
	r, err := mcpdiag.New(mcpdiag.Options{Directory: t.TempDir(), MaxEvents: 20, MaxDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	h := r.Middleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) { return &mcp.ListToolsResult{}, nil })
	_, _ = h(context.Background(), "tools/list", &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"-log", r.Path()}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var report mcpdiag.Analysis
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.MCPStarts != 1 || report.MCPEnds != 1 || report.StopReason != "shutdown" {
		t.Fatalf("invalid CLI report: %+v", report)
	}
	for _, args := range [][]string{nil, {"-invalid"}, {"-log", r.Path(), "-max-groups", "0"}, {"-log", r.Path(), "unexpected"}} {
		if err := run(args, io.Discard, io.Discard); err == nil {
			t.Fatalf("invalid arguments accepted: %v", args)
		}
	}
}
