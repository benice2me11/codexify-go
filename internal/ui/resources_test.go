package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

// Execute the shipped inline JavaScript, not a reimplementation of its logic.
// Node is test-only; the production Go binary has no JavaScript dependency.
func TestWidgetsDoNotCallToolsFromHostGlobals(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for embedded widget behavior tests")
	}
	payload, err := json.Marshal(map[string]string{"SetupHTML": SetupHTML, "ChatHTML": ChatHTML, "UpdateHTML": UpdateHTML, "DiffHTML": DiffHTML})
	if err != nil {
		t.Fatal(err)
	}
	// Windows CI runners may cold-start Node under parallel Go package tests.
	// Keep a finite timeout, but distinguish resource-starved launches from
	// an actual JavaScript assertion failure with an actionable error.
	const timeout = 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "widget_runtime_test.mjs")
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			t.Fatalf("widget runtime test timed out after %s (node=%q): %v\n%s", timeout, node, ctx.Err(), output)
		}
		t.Fatalf("widget runtime test failed: %v\n%s", err, output)
	}
	t.Log(string(output))
}
