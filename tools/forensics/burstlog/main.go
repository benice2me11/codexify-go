// burstlog summarizes existing JSONL logs. It never starts a connector or sends
// requests. Only identifiers, event names, and counts enter the output; request
// bodies, arguments, headers, credentials, and results are deliberately omitted.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

const forwardedMessage = "dispatcher forwarded command to MCP server"

type sample struct {
	Line         int    `json:"line"`
	Time         string `json:"time"`
	RequestID    string `json:"request_id"`
	CmdRequestID string `json:"cmd_request_id"`
}

type report struct {
	Source                string         `json:"source"`
	SHA256                string         `json:"sha256"`
	From                  string         `json:"from_inclusive,omitempty"`
	To                    string         `json:"to_exclusive,omitempty"`
	OffsetMinutes         int            `json:"histogram_utc_offset_minutes"`
	Lines                 int            `json:"file_lines"`
	Parsed                int            `json:"file_json_objects"`
	Blank                 int            `json:"file_blank_lines"`
	InvalidJSON           int            `json:"file_invalid_json_lines"`
	InvalidTime           int            `json:"file_missing_or_invalid_timestamps"`
	Scoped                int            `json:"scoped_records"`
	Messages              map[string]int `json:"messages"`
	Fields                []string       `json:"field_names"`
	Forwarded             int            `json:"forwarded_log_records"`
	First                 string         `json:"first_forwarded_log_time,omitempty"`
	Last                  string         `json:"last_forwarded_log_time,omitempty"`
	Minutes               map[string]int `json:"forwarded_by_minute"`
	UniqueEnvelopeIDs     int            `json:"unique_nonempty_request_ids"`
	UniqueCommandIDs      int            `json:"unique_nonempty_cmd_request_ids"`
	UniqueCommandPrefixes int            `json:"unique_nonempty_cmd_id_prefixes_before_slash"`
	MissingEnvelopeIDs    int            `json:"missing_request_ids"`
	MissingCommandIDs     int            `json:"missing_cmd_request_ids"`
	Instances             map[string]int `json:"forwarded_client_instances"`
	RPCIDs                map[string]int `json:"forwarded_rpc_ids_json_encoded"`
	Methods               map[string]int `json:"forwarded_methods"`
	Tools                 map[string]int `json:"forwarded_tool_names"`
	MissingMethods        int            `json:"forwarded_missing_method"`
	MissingTools          int            `json:"forwarded_missing_tool_name"`
	Samples               []sample       `json:"first_three_forwarded_identifiers"`
	Interpretation        string         `json:"interpretation_limit"`
}

func text(row map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(row[key], &value)
	return value
}

func analyze(input io.Reader, from, to time.Time, offsetMinutes int) (report, error) {
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return report{}, fmt.Errorf("from must precede to")
	}
	if offsetMinutes < -14*60 || offsetMinutes > 14*60 {
		return report{}, fmt.Errorf("UTC offset must be within -840..840 minutes")
	}
	out := report{
		OffsetMinutes: offsetMinutes,
		Messages:      make(map[string]int), Minutes: make(map[string]int),
		Instances: make(map[string]int), RPCIDs: make(map[string]int),
		Methods: make(map[string]int), Tools: make(map[string]int),
		Fields: []string{}, Samples: []sample{},
		Interpretation: "Forwarded lines are post-response-handling log events, not arrival timestamps or proof of successful delivery. Distinct IDs do not establish distinct operations or exclude retries with regenerated IDs. Missing methods cannot be inferred from RPC IDs.",
	}
	if !from.IsZero() {
		out.From = from.Format(time.RFC3339Nano)
	}
	if !to.IsZero() {
		out.To = to.Format(time.RFC3339Nano)
	}
	zone := time.FixedZone("report-offset", offsetMinutes*60)
	envelopes, commands, prefixes, fields := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	var first, last time.Time
	hash := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(input, hash))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		out.Lines++
		line := scanner.Bytes()
		if strings.TrimSpace(string(line)) == "" {
			out.Blank++
			continue
		}
		var row map[string]json.RawMessage
		if err := json.Unmarshal(line, &row); err != nil || row == nil {
			out.InvalidJSON++
			continue
		}
		out.Parsed++
		timestamp, err := time.Parse(time.RFC3339Nano, text(row, "time"))
		if err != nil {
			out.InvalidTime++
			continue
		}
		if (!from.IsZero() && timestamp.Before(from)) || (!to.IsZero() && !timestamp.Before(to)) {
			continue
		}
		out.Scoped++
		for key := range row {
			fields[key] = true
		}
		message := text(row, "msg")
		out.Messages[message]++
		if message != forwardedMessage {
			continue
		}
		out.Forwarded++
		if first.IsZero() || timestamp.Before(first) {
			first = timestamp
		}
		if last.IsZero() || timestamp.After(last) {
			last = timestamp
		}
		out.Minutes[timestamp.In(zone).Format("2006-01-02T15:04-07:00")]++
		requestID, commandID := text(row, "request_id"), text(row, "cmd_request_id")
		if requestID == "" {
			out.MissingEnvelopeIDs++
		} else {
			envelopes[requestID] = true
		}
		if commandID == "" {
			out.MissingCommandIDs++
		} else {
			commands[commandID] = true
			prefix, _, _ := strings.Cut(commandID, "/")
			if prefix != "" {
				prefixes[prefix] = true
			}
		}
		instance := text(row, "client_instance_id")
		if instance != "" {
			out.Instances[instance]++
		}
		if id, ok := row["rpc_request_id"]; ok {
			out.RPCIDs[string(id)]++
		} else {
			out.RPCIDs["<missing>"]++
		}
		method := text(row, "rpc_method")
		if method == "" {
			method = text(row, "method")
		}
		if method == "" {
			out.MissingMethods++
		} else {
			out.Methods[method]++
		}
		tool := text(row, "tool_name")
		if tool == "" {
			tool = text(row, "tool")
		}
		if tool == "" {
			out.MissingTools++
		} else {
			out.Tools[tool]++
		}
		if len(out.Samples) < 3 {
			out.Samples = append(out.Samples, sample{out.Lines, timestamp.Format(time.RFC3339Nano), requestID, commandID})
		}
	}
	if err := scanner.Err(); err != nil {
		return report{}, fmt.Errorf("read log near line %d: %w", out.Lines+1, err)
	}
	out.SHA256 = fmt.Sprintf("%x", hash.Sum(nil))
	out.UniqueEnvelopeIDs, out.UniqueCommandIDs, out.UniqueCommandPrefixes = len(envelopes), len(commands), len(prefixes)
	for field := range fields {
		out.Fields = append(out.Fields, field)
	}
	sort.Strings(out.Fields)
	if !first.IsZero() {
		out.First, out.Last = first.Format(time.RFC3339Nano), last.Format(time.RFC3339Nano)
	}
	return out, nil
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("burstlog", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("log", "", "existing JSONL log to read")
	fromText := flags.String("from", "", "inclusive RFC3339 start (optional)")
	toText := flags.String("to", "", "exclusive RFC3339 end (optional)")
	offset := flags.Int("offset-minutes", 180, "fixed UTC offset for minute buckets")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" || flags.NArg() != 0 {
		return fmt.Errorf("usage: burstlog -log <path> [-from RFC3339] [-to RFC3339] [-offset-minutes 180]")
	}
	var from, to time.Time
	if *fromText != "" {
		parsed, err := time.Parse(time.RFC3339Nano, *fromText)
		if err != nil {
			return fmt.Errorf("invalid -from timestamp: %w", err)
		}
		from = parsed
	}
	if *toText != "" {
		parsed, err := time.Parse(time.RFC3339Nano, *toText)
		if err != nil {
			return fmt.Errorf("invalid -to timestamp: %w", err)
		}
		to = parsed
	}
	file, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer file.Close()
	result, err := analyze(file, from, to, *offset)
	if err != nil {
		return err
	}
	result.Source = *path
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "burstlog:", err)
		os.Exit(1)
	}
}
