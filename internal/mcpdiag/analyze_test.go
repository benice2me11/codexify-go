package mcpdiag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAnalyzeCaptureAndRepeatGroups(t *testing.T) {
	r := testRecorder(t, 100)
	h := r.Middleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) { return &mcp.CallToolResult{}, nil })
	for i := 0; i < 3; i++ {
		_, _ = h(context.Background(), "tools/call", callRequest(`{"secret":"SECRET_INPUT"}`, "SECRET_CONVERSATION", fmt.Sprintf("%08x-e5ce-4979-a53c-b89f99a364a9/test", i)))
	}
	_, raw := readEvents(t, r)
	a, err := Analyze(strings.NewReader(raw), 20)
	if err != nil {
		t.Fatal(err)
	}
	if a.MCPStarts != 3 || a.MCPEnds != 3 || a.UnlinkedMCP != 3 || a.IncompleteMCP != 0 || a.UniqueOperationGroups != 1 || a.RepeatedOperationGroups != 1 || a.StopReason != "shutdown" {
		t.Fatalf("bad analysis: %+v", a)
	}
	if a.TopOperations[0].Count != 3 || a.TopOperations[0].DistinctTraces != 3 || len(a.TopOperations[0].TraceSamples) != 3 {
		t.Fatalf("bad operation group: %+v", a.TopOperations)
	}
	if a.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) {
		t.Fatal("wrong capture fingerprint")
	}
	out, _ := json.Marshal(a)
	if strings.Contains(string(out), "SECRET_") {
		t.Fatal("analysis leaked input")
	}
}

func encodedEvents(events ...Event) string {
	var out bytes.Buffer
	for i, e := range events {
		e.Version, e.CaptureID, e.Sequence = 1, "capture-fixture", uint64(i+1)
		e.Time = time.Date(2026, 9, 30, 12, 0, i, 0, time.UTC)
		_ = json.NewEncoder(&out).Encode(e)
	}
	return out.String()
}

func TestAnalyzeIncompleteAndHTTPOnly(t *testing.T) {
	input := encodedEvents(Event{Phase: "capture_start"}, Event{Phase: "http_start", HTTPID: 1}, Event{Phase: "http_end", HTTPID: 1, Status: 401}, Event{Phase: "http_start", HTTPID: 2}, Event{Phase: "mcp_start", HTTPID: 2, CallID: 1, Method: "tools/call"})
	a, err := Analyze(strings.NewReader(input), 20)
	if err != nil {
		t.Fatal(err)
	}
	if a.StopReason != "missing_stop" || a.IncompleteHTTP != 1 || a.IncompleteMCP != 1 || a.HTTPWithoutMCP != 1 || a.HTTPErrorsWithoutMCP != 1 || a.MissingFingerprints != 1 {
		t.Fatalf("incomplete capture looked complete: %+v", a)
	}
}

func TestAnalyzeRejectsCorruption(t *testing.T) {
	valid := encodedEvents(Event{Phase: "capture_start"}, Event{Phase: "capture_stop", Reason: "shutdown"})
	for name, input := range map[string]string{
		"empty": "", "bad_json": "{oops", "truncated": valid + "{", "blank": valid + "\n",
		"sequence":          strings.Replace(valid, `"sequence":2`, `"sequence":3`, 1),
		"extra_field":       strings.Replace(valid, `"version":1`, `"payload":"SECRET","version":1`, 1),
		"duplicate_call":    encodedEvents(Event{Phase: "capture_start"}, Event{Phase: "mcp_start", CallID: 1}, Event{Phase: "mcp_start", CallID: 1}),
		"end_without_start": encodedEvents(Event{Phase: "capture_start"}, Event{Phase: "mcp_end", CallID: 1}),
		"unknown_http_link": encodedEvents(Event{Phase: "capture_start"}, Event{Phase: "mcp_start", CallID: 1, HTTPID: 99}),
		"after_stop":        valid + encodedEvents(Event{Phase: "capture_start"}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Analyze(strings.NewReader(input), 20); err == nil {
				t.Fatal("corrupt capture accepted")
			}
		})
	}
	if _, err := Analyze(strings.NewReader(valid), 0); err == nil {
		t.Fatal("invalid limit accepted")
	}
	if _, err := Analyze(errorReader{}, 20); err == nil {
		t.Fatal("read failure ignored")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
