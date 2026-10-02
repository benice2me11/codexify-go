package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticsOptInDefaultsAndValidation(t *testing.T) {
	cfg := Default()
	cfg.Tunnel.Executable = "unused-test-binary"
	cfg.Tunnel.TunnelID = "isolated-test"
	cfg.Tunnel.APIKeyRef = "env:UNUSED"
	cfg.Tunnel.MCPServerURL = "http://127.0.0.1:43210/mcp"
	if cfg.MCP.Diagnostics.Enabled {
		t.Fatal("diagnostics must be opt-in")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"mcp":{"diagnostics":{"enabled":true,"directory":"traces"}}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.MCP.Diagnostics.MaxEvents != 20000 || cfg.MCP.Diagnostics.MaxDuration.Duration() != 10*time.Minute {
		t.Fatal("partial configuration lost diagnostic defaults")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	cfg.expand(base)
	if cfg.MCP.Diagnostics.Directory != filepath.Join(base, "traces") {
		t.Fatal("diagnostic path must resolve relative to config")
	}
	for _, tc := range []struct {
		name   string
		change func(*MCPDiagnosticsConfig)
	}{
		{"directory", func(d *MCPDiagnosticsConfig) { d.Directory = "" }},
		{"maxEvents", func(d *MCPDiagnosticsConfig) { d.MaxEvents = 0 }},
		{"maxEvents", func(d *MCPDiagnosticsConfig) { d.MaxEvents = 100001 }},
		{"maxDuration", func(d *MCPDiagnosticsConfig) { d.MaxDuration = 0 }},
		{"maxDuration", func(d *MCPDiagnosticsConfig) { d.MaxDuration = Duration(2 * time.Hour) }},
	} {
		copy := cfg
		tc.change(&copy.MCP.Diagnostics)
		err := copy.Validate()
		if err == nil || !strings.Contains(err.Error(), "mcp.diagnostics."+tc.name) {
			t.Fatalf("%s validation error=%v", tc.name, err)
		}
	}
}
