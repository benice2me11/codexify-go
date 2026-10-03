package mcpdiag

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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testRecorder(t *testing.T, max int) *Recorder {
	t.Helper()
	r, err := New(Options{Directory: t.TempDir(), MaxEvents: max, MaxDuration: time.Minute,
		KnownTools: map[string]struct{}{"fixture": {}}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func readEvents(t *testing.T, r *Recorder) ([]Event, string) {
	t.Helper()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(r.Path())
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var e Event
		err := decoder.Decode(&e)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if e.Version != 1 || e.Sequence != uint64(len(events)+1) || e.CaptureID != r.captureID {
			t.Fatalf("invalid event framing: %+v", e)
		}
		events = append(events, e)
	}
	return events, string(data)
}

func callRequest(arguments, identity, trace string) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"openai/session": identity, "volatile": "SECRET_META"}, Name: "fixture", Arguments: json.RawMessage(arguments)},
		Extra: &mcp.RequestExtra{Header: http.Header{
			"X-Request-Id": []string{trace}, "Authorization": []string{"Bearer SECRET_BEARER"},
			"Cookie": []string{"SECRET_COOKIE"}, "Mcp-Session-Id": []string{"SECRET_SESSION"},
		}},
	}
}

func TestFingerprintsAndMetadataPrivacy(t *testing.T) {
	r := testRecorder(t, 100)
	var invocations int
	next := r.Middleware()(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		invocations++
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "SECRET_RESULT"}}}, nil
	})
	requests := []*mcp.CallToolRequest{
		callRequest(`{"a":1,"secret":"SECRET_ARGUMENT","b":2}`, "SECRET_CONVERSATION", "101ef551-e5ce-4979-a53c-b89f99a364a9/abcd"),
		callRequest(`{"b":2,"secret":"SECRET_ARGUMENT","a":1}`, "SECRET_CONVERSATION", "101ef551-e5ce-4979-a53c-b89f99a364a9/efgh"),
		callRequest(`{"a":1,"secret":"SECRET_ARGUMENT","b":2}`, "OTHER_CONVERSATION", "SECRET_NON_UUID_TRACE"),
		callRequest(`{"number":9007199254740992}`, "SECRET_CONVERSATION", ""),
		callRequest(`{"number":9007199254740993}`, "SECRET_CONVERSATION", ""),
	}
	for _, req := range requests {
		before, _ := json.Marshal(req.Params)
		if _, err := next(context.Background(), "tools/call", req); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(req.Params)
		if !bytes.Equal(before, after) {
			t.Fatal("middleware modified params")
		}
	}
	if invocations != len(requests) {
		t.Fatal("middleware amplified or suppressed requests")
	}
	events, raw := readEvents(t, r)
	var starts []Event
	for _, e := range events {
		if e.Phase == "mcp_start" {
			starts = append(starts, e)
		}
	}
	if len(starts) != 5 {
		t.Fatalf("starts=%d", len(starts))
	}
	if starts[0].OperationHash == "" || starts[0].OperationHash != starts[1].OperationHash || starts[1].OperationHash != starts[2].OperationHash {
		t.Fatal("canonical operation hash changed with trace ID, metadata, or object key order")
	}
	if starts[0].ConversationHash != starts[1].ConversationHash || starts[0].ConversationHash == starts[2].ConversationHash {
		t.Fatal("conversation hashes are incorrect")
	}
	if starts[3].OperationHash == starts[4].OperationHash {
		t.Fatal("large JSON numbers lost precision")
	}
	if starts[0].TraceID == "" || starts[1].TraceID == starts[0].TraceID || starts[2].TraceID != "" || starts[2].TraceHash == "" {
		t.Fatal("trace allowlist failed")
	}
	for _, sentinel := range []string{"SECRET_", "OTHER_CONVERSATION", hexKey(r.key[:])} {
		if strings.Contains(raw, sentinel) {
			t.Fatalf("private value leaked: %s", sentinel)
		}
	}
	r2 := testRecorder(t, 100)
	if r2.metadata("tools/call", requests[0]).OperationHash == starts[0].OperationHash {
		t.Fatal("capture keys were reused")
	}
}

func hexKey(key []byte) string { return fmt.Sprintf("%x", key) }

func TestIdentityMetadataCanCompareUIAndModelWithoutLeakingValues(t *testing.T) {
	r := testRecorder(t, 30)
	model := callRequest(`{}`, "SECRET_ANONYMOUS_A", "")
	model.Params.Meta = mcp.Meta{
		"openai/session": "SECRET_ANONYMOUS_A",
		"thread_id":      "SECRET_NATIVE_A", "threadId": "SECRET_NATIVE_A",
		"conversation_id": 12345, "conversationId": nil,
		"SECRET_UNKNOWN_KEY": "SECRET_UNKNOWN_VALUE",
	}
	widget := callRequest(`{}`, "", "")
	widget.Params.Meta = mcp.Meta{"thread_id": "SECRET_NATIVE_A", "threadId": "SECRET_NATIVE_A"}
	next := r.Middleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})
	for _, req := range []*mcp.CallToolRequest{model, widget} {
		before, _ := json.Marshal(req.Params)
		if _, err := next(context.Background(), "tools/call", req); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(req.Params)
		if !bytes.Equal(before, after) {
			t.Fatal("identity diagnostics changed the request")
		}
	}
	_, raw := readEvents(t, r)
	if strings.Contains(raw, "SECRET_") || strings.Contains(raw, ":12345") || strings.Contains(raw, hexKey(r.key[:])) {
		t.Fatal("identity diagnostics leaked a value, unknown key or capture secret")
	}
	var starts []map[string]json.RawMessage
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		var row map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		if string(row["phase"]) == `"mcp_start"` {
			starts = append(starts, row)
		}
	}
	if len(starts) != 2 {
		t.Fatalf("expected two real recorded requests, got %d", len(starts))
	}
	type field struct {
		Type string `json:"type"`
		Hash string `json:"hash"`
	}
	var modelFields, widgetFields map[string]field
	if len(starts[0]["identity_metadata"]) == 0 {
		t.Fatal("capture cannot distinguish native thread identity from missing conversation identity")
	}
	if err := json.Unmarshal(starts[0]["identity_metadata"], &modelFields); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(starts[1]["identity_metadata"], &widgetFields); err != nil {
		t.Fatal(err)
	}
	native := modelFields["thread_id"].Hash
	if len(native) != 64 || modelFields["thread_id"].Type != "string" || native != modelFields["threadId"].Hash || native != widgetFields["thread_id"].Hash {
		t.Fatal("same native identity cannot be correlated across field aliases and call origins")
	}
	if modelFields["openai/session"].Hash == "" || modelFields["openai/session"].Hash == native {
		t.Fatal("anonymous and native identities were conflated")
	}
	if _, found := widgetFields["openai/session"]; found {
		t.Fatal("capture invented an absent conversation identity")
	}
	if len(modelFields) != 5 || len(widgetFields) != 2 || modelFields["conversation_id"].Type != "number" || modelFields["conversationId"].Type != "null" || modelFields["conversation_id"].Hash != "" {
		t.Fatal("capture lost presence/type information or hashed an invalid identity")
	}
	if string(starts[0]["other_metadata_count"]) != "1" {
		t.Fatal("unrecognized metadata must be counted without recording its keys")
	}
	r2 := testRecorder(t, 10)
	second, err := json.Marshal(r2.metadata("tools/call", model))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(second), native) {
		t.Fatal("identity hashes are linkable across captures")
	}
}

func TestHTTPAbortDoesNotInventSuccess(t *testing.T) {
	r := testRecorder(t, 20)
	func() {
		defer func() {
			if recover() != "SECRET_ABORT" {
				t.Error("panic changed")
			}
		}()
		r.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("SECRET_ABORT") })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "http://localhost/mcp", nil))
	}()
	events, raw := readEvents(t, r)
	for _, e := range events {
		if e.Phase == "http_end" && (e.Outcome != "aborted" || e.Status != 0) {
			t.Fatalf("invented response status: %+v", e)
		}
	}
	if strings.Contains(raw, "SECRET_ABORT") {
		t.Fatal("panic leaked")
	}
}

func TestCaptureCreationBoundaries(t *testing.T) {
	for _, opts := range []Options{
		{Directory: "", MaxEvents: 20, MaxDuration: time.Minute},
		{Directory: t.TempDir(), MaxEvents: 3, MaxDuration: time.Minute},
		{Directory: t.TempDir(), MaxEvents: 100001, MaxDuration: time.Minute},
		{Directory: t.TempDir(), MaxEvents: 20, MaxDuration: 2 * time.Hour},
	} {
		if r, err := New(opts); err == nil {
			_ = r.Close()
			t.Fatal("unbounded/invalid capture options accepted")
		}
	}
	dir := t.TempDir()
	r1, err := New(Options{Directory: dir, MaxEvents: 4, MaxDuration: time.Nanosecond})
	if err != nil {
		t.Fatal(err)
	}
	_ = r1.active()
	_, raw := readEvents(t, r1)
	if _, err := Analyze(strings.NewReader(raw), 1); err != nil {
		t.Fatalf("tiny duration corrupted framing: %v", err)
	}
	r2, err := New(Options{Directory: dir, MaxEvents: 4, MaxDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	if r1.Path() == r2.Path() {
		t.Fatal("new capture reused an existing file")
	}
}

func TestOutcomesAndPanicPreservation(t *testing.T) {
	r := testRecorder(t, 100)
	req := callRequest(`{}`, "SECRET_CONVERSATION", "")
	for _, tc := range []struct {
		name   string
		result mcp.Result
		err    error
	}{
		{"ok", &mcp.CallToolResult{}, nil},
		{"tool_error", &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "SECRET_ERROR_RESULT"}}}, nil},
		{"rpc_error", nil, &jsonrpc.Error{Code: -32602, Message: "SECRET_RPC_ERROR"}},
		{"error", nil, errors.New("SECRET_GENERIC_ERROR")},
		{"canceled", nil, context.Canceled},
		{"deadline", nil, context.DeadlineExceeded},
	} {
		next := r.Middleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) { return tc.result, tc.err })
		result, err := next(context.Background(), "tools/call", req)
		if result != tc.result || err != tc.err {
			t.Fatal("result/error changed")
		}
	}
	func() {
		defer func() {
			if recover() != "SECRET_PANIC" {
				t.Error("panic was swallowed or changed")
			}
		}()
		_, _ = r.Middleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) { panic("SECRET_PANIC") })(context.Background(), "tools/call", req)
	}()
	events, raw := readEvents(t, r)
	got := map[string]int{}
	for _, e := range events {
		if e.Phase == "mcp_end" {
			got[e.Outcome]++
			if e.Outcome == "rpc_error" && (e.RPCErrorCode == nil || *e.RPCErrorCode != -32602) {
				t.Fatal("missing numeric RPC error")
			}
			if e.Outcome == "tool_error" && (e.ToolIsError == nil || !*e.ToolIsError) {
				t.Fatal("missing tool isError")
			}
		}
	}
	for _, outcome := range []string{"ok", "tool_error", "rpc_error", "error", "canceled", "deadline", "aborted"} {
		if got[outcome] != 1 {
			t.Fatalf("outcome %s=%d", outcome, got[outcome])
		}
	}
	if strings.Contains(raw, "SECRET_") {
		t.Fatal("error or result content leaked")
	}
}

func TestCaptureLimitsAndWriteFailureFailOpen(t *testing.T) {
	for _, reason := range []string{"event_limit", "duration_limit"} {
		t.Run(reason, func(t *testing.T) {
			r := testRecorder(t, 6)
			if reason == "duration_limit" {
				r.now = func() time.Time { return r.deadline.Add(time.Second) }
			}
			calls := 0
			h := r.Middleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) {
				calls++
				return &mcp.CallToolResult{}, nil
			})
			for i := 0; i < 40; i++ {
				_, _ = h(context.Background(), "tools/call", callRequest(`{}`, "", ""))
			}
			if calls != 40 {
				t.Fatal("capture limit affected execution")
			}
			events, _ := readEvents(t, r)
			if len(events) > 6 || events[len(events)-1].Reason != reason {
				t.Fatalf("capture not bounded: %+v", events)
			}
		})
	}
	r := testRecorder(t, 100)
	_ = r.writer.Close()
	r.writer = failingWriter{}
	var log bytes.Buffer
	r.logger = slog.New(slog.NewJSONHandler(&log, nil))
	calls := 0
	h := r.Middleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		calls++
		return &mcp.CallToolResult{}, nil
	})
	for i := 0; i < 3; i++ {
		if _, err := h(context.Background(), "tools/call", callRequest(`{}`, "", "")); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 || r.Close() == nil || strings.Count(log.String(), "write failure") != 1 || strings.Contains(log.String(), "SECRET_DISK_ERROR") {
		t.Fatal("write failure did not fail open with one safe warning")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("SECRET_DISK_ERROR") }
func (failingWriter) Close() error              { return nil }

func TestLargeUnknownAndMissingMetadata(t *testing.T) {
	r := testRecorder(t, 100)
	request := callRequest(`{"data":"`+strings.Repeat("x", maxFingerprintBytes)+`"}`, "", "")
	request.Params.Name = "SECRET_UNKNOWN_TOOL"
	e := r.metadata("tools/call", request)
	if e.FingerprintState != "too_large" || e.OperationHash != "" || e.Tool != "OTHER" || e.ToolHash == "" {
		t.Fatalf("bad bounded metadata: %+v", e)
	}
	e = r.metadata("SECRET_UNKNOWN_METHOD", &mcp.ListToolsRequest{})
	encoded, _ := json.Marshal(e)
	if e.Method != "OTHER" || e.MethodHash == "" || strings.Contains(string(encoded), "SECRET_") {
		t.Fatal("unknown method leaked")
	}
	e = r.metadata("notifications/initialized", &mcp.InitializedRequest{})
	if !e.Notification {
		t.Fatal("notification classification missing")
	}
}

func TestHTTPTransparencyAndFlush(t *testing.T) {
	r := testRecorder(t, 100)
	body := `{"secret":"SECRET_HTTP_BODY"}`
	h := r.Handler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		data, err := io.ReadAll(req.Body)
		if err != nil || string(data) != body {
			t.Error("request body changed")
		}
		if req.Header.Get("Authorization") != "Bearer SECRET_AUTH" {
			t.Error("header changed")
		}
		w.Header().Set("X-Fixture", "preserved")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "SECRET_HTTP_RESPONSE")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
		}
	}))
	req := httptest.NewRequest(http.MethodPost, "http://localhost/mcp?secret=SECRET_QUERY", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer SECRET_AUTH")
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted || recorder.Body.String() != "SECRET_HTTP_RESPONSE" || !recorder.Flushed || recorder.Header().Get("X-Fixture") != "preserved" {
		t.Fatal("HTTP/streaming response changed")
	}
	events, raw := readEvents(t, r)
	if len(events) != 4 || events[2].Phase != "http_end" || events[2].Status != 202 || events[2].ResponseBytes != int64(len("SECRET_HTTP_RESPONSE")) {
		t.Fatalf("invalid HTTP events: %+v", events)
	}
	if strings.Contains(raw, "SECRET_") {
		t.Fatal("HTTP content leaked")
	}
}

// This is an isolated SDK endpoint with a synthetic tool, not the hosted tunnel.
func TestIsolatedConcurrentCallsNoAmplification(t *testing.T) {
	const count = 1024
	const workers = 16
	r := testRecorder(t, 10000)
	server := mcp.NewServer(&mcp.Implementation{Name: "isolated-fixture", Version: "1"}, &mcp.ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	var executed atomic.Int64
	server.AddTool(&mcp.Tool{Name: "fixture", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		executed.Add(1)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "SECRET_FIXTURE_RESULT"}}}, nil
	})
	server.AddReceivingMiddleware(r.Middleware())
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	host := httptest.NewServer(r.Handler(handler))
	defer host.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	var wg sync.WaitGroup
	errorsFound := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := worker; i < count; i += workers {
				meta := map[string]any{
					mcp.MetaKeyProtocolVersion:    "2026-07-28",
					mcp.MetaKeyClientCapabilities: map[string]any{},
					mcp.MetaKeyClientInfo:         map[string]any{"name": "isolated-fixture", "version": "1"},
					"openai/session":              "SECRET_LOAD_CONVERSATION",
				}
				payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "tools/call", "params": map[string]any{"_meta": meta, "name": "fixture", "arguments": map[string]any{"value": "SECRET_LOAD_ARGUMENT"}}})
				req, _ := http.NewRequest(http.MethodPost, host.URL, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json, text/event-stream")
				req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
				req.Header.Set("Mcp-Method", "tools/call")
				req.Header.Set("Mcp-Name", "fixture")
				req.Header.Set("X-Request-Id", fmt.Sprintf("%08x-e5ce-4979-a53c-b89f99a364a9/test", i))
				response, err := client.Do(req)
				if err != nil {
					errorsFound <- err
					return
				}
				data, readErr := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if readErr != nil || response.StatusCode != 200 || !bytes.Contains(data, []byte("SECRET_FIXTURE_RESULT")) {
					errorsFound <- fmt.Errorf("fixture response status=%d read=%v body=%s", response.StatusCode, readErr, data)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	if t.Failed() {
		return
	}
	if executed.Load() != count {
		t.Fatalf("executed %d, sent %d", executed.Load(), count)
	}
	events, raw := readEvents(t, r)
	phases := map[string]int{}
	operations, traces := map[string]bool{}, map[string]bool{}
	calls, httpRequests := map[uint64]int{}, map[uint64]int{}
	for _, e := range events {
		phases[e.Phase]++
		if e.Phase == "mcp_start" {
			if e.HTTPID == 0 {
				t.Fatal("SDK did not propagate the HTTP correlation context")
			}
			if e.Method != "tools/call" || e.Tool != "fixture" || e.ConversationHash == "" {
				t.Fatal("decoded metadata missing")
			}
			operations[e.OperationHash], traces[e.TraceID] = true, true
			calls[e.CallID]++
		}
		if e.Phase == "mcp_end" {
			if e.Outcome != "ok" {
				t.Fatal("unexpected MCP outcome")
			}
			calls[e.CallID]++
		}
		if e.Phase == "http_start" || e.Phase == "http_end" {
			httpRequests[e.HTTPID]++
		}
	}
	for _, phase := range []string{"http_start", "http_end", "mcp_start", "mcp_end"} {
		if phases[phase] != count {
			t.Fatalf("%s=%d, want %d", phase, phases[phase], count)
		}
	}
	if len(operations) != 1 || len(traces) != count || operations[""] || traces[""] {
		t.Fatalf("operations=%d traces=%d", len(operations), len(traces))
	}
	for _, pairs := range []map[uint64]int{calls, httpRequests} {
		for _, n := range pairs {
			if n != 2 {
				t.Fatal("unpaired concurrent events")
			}
		}
	}
	if strings.Contains(raw, "SECRET_") {
		t.Fatal("load-test payload leaked")
	}
	retained := 0
	for range server.Sessions() {
		retained++
	}
	if retained != 0 {
		t.Fatalf("stateless lifecycle changed: retained=%d", retained)
	}
	t.Logf("sent=%d executed=%d http_pairs=%d mcp_pairs=%d distinct_trace_ids=%d operation_hashes=%d retained_sessions=%d", count, executed.Load(), len(httpRequests), len(calls), len(traces), len(operations), retained)
}
