package mcpdiag

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"
)

// OperationGroup describes matching non-meta parameters, NOT a proven retry or
// a deduplication key. Different operations may intentionally have equal inputs.
type OperationGroup struct {
	OperationHash    string   `json:"operation_hash"`
	ConversationHash string   `json:"conversation_hash,omitempty"`
	Method           string   `json:"rpc_method"`
	Tool             string   `json:"tool_name,omitempty"`
	Count            int      `json:"count"`
	DistinctTraces   int      `json:"distinct_trace_hashes"`
	TraceSamples     []string `json:"trace_id_samples,omitempty"`
}

type Analysis struct {
	Version                 int              `json:"version"`
	CaptureID               string           `json:"capture_id"`
	SHA256                  string           `json:"source_sha256"`
	Events                  int              `json:"events"`
	StartedAt               time.Time        `json:"started_at"`
	StoppedAt               *time.Time       `json:"stopped_at,omitempty"`
	StopReason              string           `json:"stop_reason"`
	HTTPStarts              int              `json:"http_starts"`
	HTTPEnds                int              `json:"http_ends"`
	MCPStarts               int              `json:"mcp_starts"`
	MCPEnds                 int              `json:"mcp_ends"`
	IncompleteHTTP          int              `json:"incomplete_http"`
	IncompleteMCP           int              `json:"incomplete_mcp"`
	UnlinkedMCP             int              `json:"mcp_starts_without_http_link"`
	HTTPWithoutMCP          int              `json:"http_requests_without_mcp_start"`
	HTTPErrorsWithoutMCP    int              `json:"http_errors_without_mcp_start"`
	Methods                 map[string]int   `json:"methods_at_start"`
	Tools                   map[string]int   `json:"tools_at_start"`
	Outcomes                map[string]int   `json:"mcp_outcomes_at_end"`
	HTTPStatuses            map[string]int   `json:"http_statuses_at_end"`
	RPCErrorCodes           map[string]int   `json:"rpc_error_codes_at_end"`
	MissingFingerprints     int              `json:"mcp_starts_without_operation_hash"`
	UniqueOperationGroups   int              `json:"unique_operation_conversation_groups"`
	RepeatedOperationGroups int              `json:"repeated_operation_conversation_groups"`
	TopOperations           []OperationGroup `json:"top_operation_groups"`
	Interpretation          string           `json:"interpretation_limit"`
}

type operationAccumulator struct {
	group  OperationGroup
	traces map[string]bool
}

// Analyze accepts one complete or crash-interrupted capture. Malformed JSON,
// duplicate event IDs, mixed captures, or sequence gaps fail rather than being
// silently skipped. A missing stop or unmatched start remains explicit.
func Analyze(input io.Reader, maxGroups int) (Analysis, error) {
	if maxGroups < 1 || maxGroups > 100 {
		return Analysis{}, fmt.Errorf("max-groups must be between 1 and 100")
	}
	a := Analysis{Version: 1, StopReason: "missing_stop", Methods: map[string]int{}, Tools: map[string]int{}, Outcomes: map[string]int{}, HTTPStatuses: map[string]int{}, RPCErrorCodes: map[string]int{}, TopOperations: []OperationGroup{},
		Interpretation: "Counts describe this local capture only. Equal method/non-meta-parameter HMACs are repeat candidates, not proven retries or distinct user operations. No hosted enqueue/poll timing or conversation/turn mapping is inferred. Early HTTP failures can occur before decoded MCP middleware. Truncated/limited captures may have unmatched starts; fingerprints are comparable only inside one capture."}
	httpStarted, httpEnded, callsStarted, callsEnded := map[uint64]bool{}, map[uint64]int{}, map[uint64]bool{}, map[uint64]bool{}
	httpWithMCP := map[uint64]bool{}
	groups := map[string]*operationAccumulator{}
	hash := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(input, hash))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	stopped := false
	line := 0
	for scanner.Scan() {
		line++
		if line > 100000 {
			return Analysis{}, fmt.Errorf("capture exceeds 100000-event bound")
		}
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			return Analysis{}, fmt.Errorf("blank record at line %d", line)
		}
		var e Event
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&e); err != nil {
			return Analysis{}, fmt.Errorf("invalid diagnostic record at line %d", line)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return Analysis{}, fmt.Errorf("trailing data at line %d", line)
		}
		if e.Version != 1 || e.CaptureID == "" || e.Time.IsZero() || e.Sequence != uint64(line) || stopped {
			return Analysis{}, fmt.Errorf("invalid event framing at line %d", line)
		}
		if line == 1 {
			if e.Phase != "capture_start" {
				return Analysis{}, fmt.Errorf("capture_start is required first")
			}
			a.CaptureID, a.StartedAt = e.CaptureID, e.Time
		}
		if e.CaptureID != a.CaptureID {
			return Analysis{}, fmt.Errorf("mixed capture IDs at line %d", line)
		}
		a.Events++
		switch e.Phase {
		case "capture_start":
			if line != 1 {
				return Analysis{}, fmt.Errorf("duplicate capture_start")
			}
		case "capture_stop":
			if e.Reason != "shutdown" && e.Reason != "duration_limit" && e.Reason != "event_limit" {
				return Analysis{}, fmt.Errorf("unknown stop reason at line %d", line)
			}
			stopped, a.StoppedAt, a.StopReason = true, &e.Time, e.Reason
		case "http_start":
			if e.HTTPID == 0 || httpStarted[e.HTTPID] {
				return Analysis{}, fmt.Errorf("duplicate/missing http_id at line %d", line)
			}
			httpStarted[e.HTTPID] = true
			a.HTTPStarts++
		case "http_end":
			if !httpStarted[e.HTTPID] {
				return Analysis{}, fmt.Errorf("http_end without start at line %d", line)
			}
			if _, seen := httpEnded[e.HTTPID]; seen {
				return Analysis{}, fmt.Errorf("duplicate http_end at line %d", line)
			}
			httpEnded[e.HTTPID] = e.Status
			a.HTTPEnds++
			a.HTTPStatuses[strconv.Itoa(e.Status)]++
		case "mcp_start":
			if e.CallID == 0 || callsStarted[e.CallID] {
				return Analysis{}, fmt.Errorf("duplicate/missing call_id at line %d", line)
			}
			callsStarted[e.CallID] = true
			a.MCPStarts++
			if e.HTTPID == 0 {
				a.UnlinkedMCP++
			} else {
				if !httpStarted[e.HTTPID] {
					return Analysis{}, fmt.Errorf("MCP links unknown http_id at line %d", line)
				}
				httpWithMCP[e.HTTPID] = true
			}
			a.Methods[e.Method]++
			if e.Tool != "" {
				a.Tools[e.Tool]++
			}
			if e.OperationHash == "" {
				a.MissingFingerprints++
				continue
			}
			key := e.OperationHash + "/" + e.ConversationHash
			acc := groups[key]
			if acc == nil {
				acc = &operationAccumulator{group: OperationGroup{OperationHash: e.OperationHash, ConversationHash: e.ConversationHash, Method: e.Method, Tool: e.Tool}, traces: map[string]bool{}}
				groups[key] = acc
			}
			acc.group.Count++
			if e.TraceHash != "" && !acc.traces[e.TraceHash] {
				acc.traces[e.TraceHash] = true
				if len(acc.group.TraceSamples) < 3 && len(e.TraceID) <= 41 && traceIDPattern.MatchString(e.TraceID) {
					acc.group.TraceSamples = append(acc.group.TraceSamples, e.TraceID)
				}
			}
		case "mcp_end":
			if !callsStarted[e.CallID] || callsEnded[e.CallID] {
				return Analysis{}, fmt.Errorf("unmatched/duplicate mcp_end at line %d", line)
			}
			callsEnded[e.CallID] = true
			a.MCPEnds++
			a.Outcomes[e.Outcome]++
			if e.RPCErrorCode != nil {
				a.RPCErrorCodes[strconv.FormatInt(*e.RPCErrorCode, 10)]++
			}
		default:
			return Analysis{}, fmt.Errorf("unknown event phase at line %d", line)
		}
	}
	if err := scanner.Err(); err != nil {
		return Analysis{}, fmt.Errorf("read capture after line %d: %w", line, err)
	}
	if a.Events == 0 {
		return Analysis{}, fmt.Errorf("empty capture")
	}
	a.SHA256 = hex.EncodeToString(hash.Sum(nil))
	a.IncompleteHTTP, a.IncompleteMCP = a.HTTPStarts-a.HTTPEnds, a.MCPStarts-a.MCPEnds
	for id := range httpStarted {
		if !httpWithMCP[id] {
			a.HTTPWithoutMCP++
			if httpEnded[id] >= 400 {
				a.HTTPErrorsWithoutMCP++
			}
		}
	}
	a.UniqueOperationGroups = len(groups)
	for _, acc := range groups {
		acc.group.DistinctTraces = len(acc.traces)
		if acc.group.Count > 1 {
			a.RepeatedOperationGroups++
		}
		a.TopOperations = append(a.TopOperations, acc.group)
	}
	sort.Slice(a.TopOperations, func(i, j int) bool {
		if a.TopOperations[i].Count != a.TopOperations[j].Count {
			return a.TopOperations[i].Count > a.TopOperations[j].Count
		}
		if a.TopOperations[i].OperationHash != a.TopOperations[j].OperationHash {
			return a.TopOperations[i].OperationHash < a.TopOperations[j].OperationHash
		}
		return a.TopOperations[i].ConversationHash < a.TopOperations[j].ConversationHash
	})
	if len(a.TopOperations) > maxGroups {
		a.TopOperations = a.TopOperations[:maxGroups]
	}
	return a, nil
}
