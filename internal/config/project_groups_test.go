package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleGroupMCPConfig() MCPConfig {
	return MCPConfig{
		MultiProject:         true,
		ProjectGroupsEnabled: true,
		Projects: []ProjectSpec{
			{Name: "work-search", Path: "work-search"},
			{Name: "hh-mcp-server", Path: "hh-mcp-server"},
			{Name: "habr-career-mcp", Path: "habr-career-mcp"},
		},
		ProjectGroups: []ProjectGroupSpec{{
			Name: "job-search", Primary: "work",
			Repositories: []ProjectGroupRepository{
				{Alias: "work", Project: "work-search", Checkout: "source"},
				{Alias: "hh", Project: "hh-mcp-server", Checkout: "source"},
				{Alias: "habr", Project: "habr-career-mcp", Checkout: "managed"},
			},
		}},
	}
}

func TestValidateProjectGroups(t *testing.T) {
	cfg := sampleGroupMCPConfig()
	if err := ValidateProjectGroups(cfg); err != nil {
		t.Fatalf("valid group: %v", err)
	}
	cfg.ProjectGroups[0].Repositories[1].Alias = "HH"
	if err := ValidateProjectGroups(cfg); err != nil {
		t.Fatalf("mixed-case unique alias should validate: %v", err)
	}

	tests := []struct {
		name    string
		change  func(*MCPConfig)
		wantErr string
	}{
		{"requires multiProject", func(m *MCPConfig) { m.MultiProject = false }, "multiProject"},
		{"flag needs groups", func(m *MCPConfig) { m.ProjectGroups = nil }, "requires at least one"},
		{"duplicate alias ignores case", func(m *MCPConfig) { m.ProjectGroups[0].Repositories[1].Alias = "WORK" }, "duplicate alias"},
		{"duplicate project", func(m *MCPConfig) { m.ProjectGroups[0].Repositories[1].Project = "work-search" }, "repeats configured project"},
		{"unknown project", func(m *MCPConfig) { m.ProjectGroups[0].Repositories[1].Project = "other" }, "unknown configured project"},
		{"missing primary", func(m *MCPConfig) { m.ProjectGroups[0].Primary = "missing" }, "not a member"},
		{"empty repos", func(m *MCPConfig) { m.ProjectGroups[0].Repositories = nil }, "must contain repositories"},
		{"untrusted alias", func(m *MCPConfig) { m.ProjectGroups[0].Repositories[0].Alias = "../work" }, "invalid alias"},
		{"unicode lookalike", func(m *MCPConfig) { m.ProjectGroups[0].Repositories[0].Alias = "wоrk" }, "invalid alias"},
		{"unknown checkout", func(m *MCPConfig) { m.ProjectGroups[0].Repositories[0].Checkout = "auto" }, "source or managed"},
		{"duplicate group", func(m *MCPConfig) {
			m.ProjectGroups = append(m.ProjectGroups, ProjectGroupSpec{Name: "JOB-SEARCH", Primary: "work", Repositories: []ProjectGroupRepository{{Alias: "work", Project: "work-search", Checkout: "source"}}})
		}, "duplicate project group"},
		{"duplicate project name", func(m *MCPConfig) { m.Projects[1].Name = "WORK-SEARCH" }, "duplicate configured project name"},
		{"unknown checkout empty", func(m *MCPConfig) { m.ProjectGroups[0].Repositories[0].Checkout = "" }, "source or managed"},
		{"no groups with multiProject disabled", func(m *MCPConfig) { m.ProjectGroupsEnabled = false; m.MultiProject = false }, "multiProject"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sampleGroupMCPConfig()
			tt.change(&m)
			err := ValidateProjectGroups(m)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestProjectGroupsDefaultOffAndDormantCatalog(t *testing.T) {
	defaultMCP := Default().MCP
	if defaultMCP.ProjectGroupsEnabled || len(defaultMCP.ProjectGroups) != 0 {
		t.Fatal("new installations must not enable project groups")
	}
	if err := ValidateProjectGroups(defaultMCP); err != nil {
		t.Fatalf("legacy config must remain valid: %v", err)
	}

	m := sampleGroupMCPConfig()
	m.ProjectGroupsEnabled = false
	if err := ValidateProjectGroups(m); err != nil {
		t.Fatalf("dormant group catalog should validate: %v", err)
	}
}

func TestLoadProjectGroupsFromJSON(t *testing.T) {
	dir := t.TempDir()
	fixture := sampleGroupMCPConfig()
	data, err := json.Marshal(map[string]any{
		"version": 1,
		"mcp": map[string]any{
			"workspaceRoot": ".", "multiProject": true, "projectGroupsEnabled": true,
			"projects": fixture.Projects, "projectGroups": fixture.ProjectGroups,
		},
		"tunnel": map[string]any{
			"executable": "tunnel.exe", "tunnelId": "fixture",
			"apiKeyRef": "env:TEST_ONLY", "mcpServerUrl": "http://127.0.0.1:3000/mcp",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.MCP.ProjectGroupsEnabled || len(loaded.MCP.ProjectGroups) != 1 ||
		loaded.MCP.ProjectGroups[0].Repositories[0].Alias != "work" ||
		loaded.MCP.Projects[0].Path != filepath.Join(dir, "work-search") {
		t.Fatalf("JSON group config not expanded: %+v", loaded.MCP)
	}
	loaded.MCP.MultiProject = false
	if err := loaded.Validate(); err == nil || !strings.Contains(err.Error(), "requires mcp.multiProject") {
		t.Fatalf("multi-project gate not enforced after JSON load: %v", err)
	}
}
