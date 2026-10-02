package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWindowIdentityAndNoPayloadLeak(t *testing.T) {
	input := `{"time":"2026-09-29T19:57:00+03:00","msg":"dispatcher forwarded command to MCP server","request_id":"a","cmd_request_id":"root/one","rpc_request_id":0,"params":{"password":"SECRET_NOT_TO_EXPORT"}}
{"time":"2026-09-29T16:57:01Z","msg":"dispatcher forwarded command to MCP server","request_id":"b","cmd_request_id":"root/two","rpc_request_id":"0","rpc_method":"tools/list","tool_name":"fixture"}
{"time":"2026-09-29T19:57:02+03:00","msg":"dispatcher forwarded command to MCP server","request_id":"b","cmd_request_id":"root/two"}
{"time":"2026-09-29T19:58:00+03:00","msg":"dispatcher forwarded command to MCP server","request_id":"excluded","cmd_request_id":"other/one"}
`
	from, _ := time.Parse(time.RFC3339, "2026-09-29T19:57:00+03:00")
	to := from.Add(time.Minute)
	r, err := analyze(strings.NewReader(input), from, to, 180)
	if err != nil {
		t.Fatal(err)
	}
	if r.Lines != 4 || r.Parsed != 4 || r.Forwarded != 3 || r.UniqueEnvelopeIDs != 2 || r.UniqueCommandIDs != 2 || r.UniqueCommandPrefixes != 1 {
		t.Fatalf("incorrect counts: %+v", r)
	}
	if r.Minutes["2026-09-29T19:57+03:00"] != 3 || r.RPCIDs["0"] != 1 || r.RPCIDs[`"0"`] != 1 || r.RPCIDs["<missing>"] != 1 {
		t.Fatalf("time zone / RPC ID normalization is incorrect: %+v", r)
	}
	if r.MissingMethods != 2 || r.Methods["tools/list"] != 1 || r.MissingTools != 2 {
		t.Fatalf("missing metadata must not be inferred: %+v", r)
	}
	if r.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(input))) {
		t.Fatal("fingerprint must cover the whole original file, not only the window")
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "SECRET_NOT_TO_EXPORT") {
		t.Fatal("raw request contents leaked")
	}
}

func TestMalformedRecordsAndMissingIdentifiers(t *testing.T) {
	input := "\nnot-json\nnull\n{}\n" + `{"time":"2026-09-29T17:00:00Z","msg":"dispatcher forwarded command to MCP server"}`
	r, err := analyze(strings.NewReader(input), time.Time{}, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Lines != 5 || r.Blank != 1 || r.InvalidJSON != 2 || r.InvalidTime != 1 || r.Forwarded != 1 || r.UniqueEnvelopeIDs != 0 || r.MissingEnvelopeIDs != 1 || r.MissingCommandIDs != 1 {
		t.Fatalf("malformed/missing data misrepresented: %+v", r)
	}
}

func TestInvalidRangeAndReaderError(t *testing.T) {
	now := time.Now()
	if _, err := analyze(strings.NewReader(""), now, now, 0); err == nil {
		t.Fatal("empty or reversed window must fail")
	}
	if _, err := analyze(strings.NewReader(""), time.Time{}, time.Time{}, 841); err == nil {
		t.Fatal("invalid UTC offset must fail")
	}
	if _, err := analyze(brokenReader{}, time.Time{}, time.Time{}, 0); err == nil {
		t.Fatal("read error must not produce a complete-looking report")
	}
	if err := run([]string{}, io.Discard, io.Discard); err == nil {
		t.Fatal("missing log path must fail")
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
