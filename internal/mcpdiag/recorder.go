// Package mcpdiag records bounded, opt-in MCP metadata without retaining payloads.
package mcpdiag

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxFingerprintBytes = 64 << 10

// Options applies to one runtime lifetime. A capture never re-arms itself.
type Options struct {
	Directory   string
	MaxEvents   int
	MaxDuration time.Duration
	KnownTools  map[string]struct{}
	Logger      *slog.Logger
}

// Event deliberately has no fields for bodies, arguments, results, or error text.
// TraceID is an unverified X-Request-Id header, retained only for UUID-shaped IDs.
// Hashes use a private per-capture key; they are not stable across captures.
type Event struct {
	Version          int       `json:"version"`
	Time             time.Time `json:"time"`
	CaptureID        string    `json:"capture_id"`
	Sequence         uint64    `json:"sequence"`
	Phase            string    `json:"phase"`
	HTTPID           uint64    `json:"http_id,omitempty"`
	CallID           uint64    `json:"call_id,omitempty"`
	HTTPMethod       string    `json:"http_method,omitempty"`
	Status           int       `json:"http_status,omitempty"`
	ResponseBytes    int64     `json:"response_bytes,omitempty"`
	DurationMS       float64   `json:"duration_ms,omitempty"`
	Method           string    `json:"rpc_method,omitempty"`
	MethodHash       string    `json:"method_hash,omitempty"`
	Tool             string    `json:"tool_name,omitempty"`
	ToolHash         string    `json:"tool_hash,omitempty"`
	OperationHash    string    `json:"operation_hash,omitempty"`
	FingerprintState string    `json:"fingerprint_state,omitempty"`
	ConversationHash string    `json:"conversation_hash,omitempty"`
	TransportHash    string    `json:"transport_hash,omitempty"`
	TraceID          string    `json:"control_plane_trace_id,omitempty"`
	TraceHash        string    `json:"trace_hash,omitempty"`
	ProtocolVersion  string    `json:"protocol_version,omitempty"`
	Notification     bool      `json:"notification,omitempty"`
	Outcome          string    `json:"outcome,omitempty"`
	RPCErrorCode     *int64    `json:"rpc_error_code,omitempty"`
	ToolIsError      *bool     `json:"tool_is_error,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	MaxEvents        int       `json:"max_events,omitempty"`
	MaxDurationMS    int64     `json:"max_duration_ms,omitempty"`
}

type Recorder struct {
	mu           sync.Mutex
	writer       io.WriteCloser
	path         string
	logger       *slog.Logger
	key          [32]byte
	captureID    string
	knownTools   map[string]struct{}
	maxEvents    uint64
	deadline     time.Time
	now          func() time.Time
	events       uint64
	stopped      bool
	closeErr     error
	httpSequence atomic.Uint64
	callSequence atomic.Uint64
}

func New(opts Options) (*Recorder, error) {
	if strings.TrimSpace(opts.Directory) == "" {
		return nil, errors.New("diagnostic directory is required")
	}
	if opts.MaxEvents < 4 || opts.MaxEvents > 100000 {
		return nil, errors.New("diagnostic maxEvents must be between 4 and 100000")
	}
	if opts.MaxDuration <= 0 || opts.MaxDuration > time.Hour {
		return nil, errors.New("diagnostic maxDuration must be > 0 and <= 1h")
	}
	r := &Recorder{logger: opts.Logger, knownTools: make(map[string]struct{}), maxEvents: uint64(opts.MaxEvents), now: time.Now}
	if r.logger == nil {
		r.logger = slog.Default()
	}
	if _, err := rand.Read(r.key[:]); err != nil {
		return nil, fmt.Errorf("diagnostic key: %w", err)
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, fmt.Errorf("diagnostic ID: %w", err)
	}
	r.captureID = hex.EncodeToString(id[:])
	for name := range opts.KnownTools {
		if len(name) <= 128 {
			r.knownTools[name] = struct{}{}
		}
	}
	if err := os.MkdirAll(opts.Directory, 0o700); err != nil {
		return nil, err
	}
	start := r.now()
	r.deadline = start.Add(opts.MaxDuration)
	r.path = filepath.Join(opts.Directory, "mcp-trace-"+start.UTC().Format("20060102T150405.000000000Z")+"-"+r.captureID+".jsonl")
	file, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	r.writer = file
	// Even an extremely short capture must begin with a valid framing record.
	if err := r.writeLocked(Event{Phase: "capture_start", MaxEvents: opts.MaxEvents, MaxDurationMS: opts.MaxDuration.Milliseconds()}); err != nil {
		return nil, errors.Join(errors.New("cannot initialize diagnostic capture"), err, file.Close())
	}
	return r, nil
}

func (r *Recorder) Path() string { return r.path }

func (r *Recorder) active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeLocked()
}

func (r *Recorder) activeLocked() bool {
	if r.stopped {
		return false
	}
	if !r.now().Before(r.deadline) {
		r.stopLocked("duration_limit")
		return false
	}
	if r.events >= r.maxEvents-1 {
		r.stopLocked("event_limit")
		return false
	}
	return true
}

func (r *Recorder) record(e Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.activeLocked() {
		return false
	}
	if err := r.writeLocked(e); err != nil {
		r.closeErr = errors.Join(err, r.writer.Close())
		r.stopped = true
		r.logger.Warn("MCP diagnostic capture stopped after write failure", "capture_id", r.captureID)
		return false
	}
	return true
}

func (r *Recorder) writeLocked(e Event) error {
	r.events++
	e.Version, e.CaptureID, e.Sequence, e.Time = 1, r.captureID, r.events, r.now().UTC()
	return json.NewEncoder(r.writer).Encode(e)
}

func (r *Recorder) stopLocked(reason string) {
	if r.stopped {
		return
	}
	r.stopped = true
	r.closeErr = errors.Join(r.writeLocked(Event{Phase: "capture_stop", Reason: reason}), r.writer.Close())
	if r.closeErr != nil {
		r.logger.Warn("MCP diagnostic capture could not be finalized", "capture_id", r.captureID)
	}
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked("shutdown")
	return r.closeErr
}

// hash is domain-separated; the key is never written to a log or config file.
func (r *Recorder) hash(domain string, data []byte) string {
	h := hmac.New(sha256.New, r.key[:])
	h.Write([]byte(domain))
	h.Write([]byte{0})
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

var traceIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}(?:/[a-zA-Z0-9]{4})?$`)
var versionPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

func (r *Recorder) headers(e *Event, headers http.Header) {
	if trace := headers.Get("X-Request-Id"); trace != "" {
		e.TraceHash = r.hash("trace", []byte(trace))
		if len(trace) <= 41 && traceIDPattern.MatchString(trace) {
			e.TraceID = trace
		}
	}
	if session := headers.Get("Mcp-Session-Id"); session != "" {
		e.TransportHash = r.hash("transport", []byte(session))
	}
	if version := headers.Get("Mcp-Protocol-Version"); len(version) == 10 && versionPattern.MatchString(version) {
		e.ProtocolVersion = version
	}
}

type httpContextKey struct{}

// Handler does not read or replace request bodies and does not add wire headers.
func (r *Recorder) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		e := Event{Phase: "http_start", HTTPID: r.httpSequence.Add(1), HTTPMethod: safeHTTPMethod(req.Method)}
		if !r.active() {
			next.ServeHTTP(w, req)
			return
		}
		r.headers(&e, req.Header)
		if !r.record(e) {
			next.ServeHTTP(w, req)
			return
		}
		start := time.Now()
		wrapped := &statusWriter{ResponseWriter: w}
		returned := false
		defer func() {
			e.Phase, e.DurationMS = "http_end", float64(time.Since(start))/float64(time.Millisecond)
			e.Status, e.ResponseBytes = wrapped.snapshot(returned)
			e.Outcome = "returned"
			if e.Status >= 400 {
				e.Outcome = "http_error"
			}
			if req.Context().Err() != nil {
				e.Outcome = contextOutcome(req.Context().Err())
			}
			if !returned {
				e.Outcome = "aborted"
			}
			r.record(e)
		}()
		next.ServeHTTP(wrapped, req.WithContext(context.WithValue(req.Context(), httpContextKey{}, e.HTTPID)))
		returned = true
	})
}

func safeHTTPMethod(method string) string {
	switch method {
	case "POST", "GET", "DELETE", "HEAD", "OPTIONS":
		return method
	}
	return "OTHER"
}

// Unwrap/FlushError preserve the SDK's ResponseController streaming behavior.
// No response bytes are retained; only the count and first final status are kept.
type statusWriter struct {
	http.ResponseWriter
	mu     sync.Mutex
	status int
	bytes  int64
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *statusWriter) WriteHeader(status int) {
	w.mu.Lock()
	if status >= 200 && w.status == 0 {
		w.status = status
	}
	w.mu.Unlock()
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.mu.Unlock()
	n, err := w.ResponseWriter.Write(p)
	w.mu.Lock()
	w.bytes += int64(n)
	w.mu.Unlock()
	return n, err
}
func (w *statusWriter) FlushError() error {
	w.mu.Lock()
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.mu.Unlock()
	return http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *statusWriter) Flush() { _ = w.FlushError() }
func (w *statusWriter) snapshot(returned bool) (int, int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	status := w.status
	if status == 0 && returned {
		status = http.StatusOK
	}
	return status, w.bytes
}

// Middleware observes the final receiving chain, including authentication/ticket
// rejections. It never calls next twice, retries, sends requests, or edits params.
func (r *Recorder) Middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (result mcp.Result, err error) {
			if !r.active() {
				return next(ctx, method, request)
			}
			e := r.metadata(method, request)
			e.HTTPID, _ = ctx.Value(httpContextKey{}).(uint64)
			e.CallID, e.Phase = r.callSequence.Add(1), "mcp_start"
			if !r.record(e) {
				return next(ctx, method, request)
			}
			start := time.Now()
			returned := false
			defer func() {
				e.Phase, e.DurationMS = "mcp_end", float64(time.Since(start))/float64(time.Millisecond)
				e.Outcome = "ok"
				if err != nil {
					e.Outcome = "error"
					var rpcErr *jsonrpc.Error
					if errors.As(err, &rpcErr) {
						code := int64(rpcErr.Code)
						e.RPCErrorCode, e.Outcome = &code, "rpc_error"
					}
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						e.Outcome = contextOutcome(err)
					}
				}
				if tool, ok := result.(*mcp.CallToolResult); ok && tool != nil {
					isError := tool.IsError
					e.ToolIsError = &isError
					if isError && err == nil {
						e.Outcome = "tool_error"
					}
				}
				if !returned {
					e.Outcome = "aborted"
				}
				r.record(e)
			}()
			result, err = next(ctx, method, request)
			returned = true
			return result, err
		}
	}
}

func contextOutcome(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	return "canceled"
}

func (r *Recorder) metadata(method string, request mcp.Request) Event {
	e := Event{Method: safeMethod(method), Notification: strings.HasPrefix(method, "notifications/")}
	if e.Method == "OTHER" {
		e.MethodHash = r.hash("method", []byte(method))
	}
	if request == nil {
		e.FingerprintState = "missing_params"
		return e
	}
	if extra := request.GetExtra(); extra != nil {
		r.headers(&e, extra.Header)
	}
	params := request.GetParams()
	if isNil(params) {
		e.OperationHash = r.hash("operation", []byte(method+"\x00{}"))
		return e
	}
	meta := params.GetMeta()
	if identity, ok := meta["openai/session"].(string); ok && strings.TrimSpace(identity) != "" {
		e.ConversationHash = r.hash("conversation", []byte(strings.TrimSpace(identity)))
	}
	if version, ok := meta[mcp.MetaKeyProtocolVersion].(string); ok && len(version) == 10 && versionPattern.MatchString(version) {
		e.ProtocolVersion = version
	}
	if call, ok := params.(*mcp.CallToolParamsRaw); ok && call != nil {
		e.ToolHash = r.hash("tool", []byte(call.Name))
		if _, known := r.knownTools[call.Name]; known {
			e.Tool = call.Name
		} else {
			e.Tool = "OTHER"
		}
		if len(call.Arguments) > maxFingerprintBytes {
			e.FingerprintState = "too_large"
			return e
		}
	}
	// Canonical JSON normalizes object key order without converting numbers to
	// float64. Volatile transport metadata is excluded, not used for deduplication.
	encoded, err := json.Marshal(params)
	if err != nil {
		e.FingerprintState = "encode_error"
		return e
	}
	if len(encoded) > maxFingerprintBytes {
		e.FingerprintState = "too_large"
		return e
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		e.FingerprintState = "decode_error"
		return e
	}
	delete(object, "_meta")
	canonical, err := json.Marshal(object)
	if err != nil {
		e.FingerprintState = "encode_error"
		return e
	}
	e.OperationHash = r.hash("operation", append([]byte(method+"\x00"), canonical...))
	return e
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	x := reflect.ValueOf(v)
	return x.Kind() == reflect.Pointer && x.IsNil()
}

func safeMethod(method string) string {
	switch method {
	case "initialize", "server/discover", "ping", "tools/list", "tools/call",
		"resources/list", "resources/templates/list", "resources/read", "resources/subscribe", "resources/unsubscribe",
		"subscriptions/listen", "prompts/list", "prompts/get", "completion/complete", "logging/setLevel",
		"notifications/initialized", "notifications/cancelled", "notifications/progress", "notifications/roots/list_changed",
		"tasks/get", "tasks/result", "tasks/list", "tasks/cancel":
		return method
	}
	return "OTHER"
}
