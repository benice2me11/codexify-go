package connectorschema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benice2me11/codexify-go/internal/config"
)

func TestVersionReflectsSchemaFeatures(t *testing.T) {
	cfg := config.Default()
	cfg.MCP.MultiProject = false
	cfg.ArtifactIngress.Enabled = false
	cfg.MCP.Upstreams = nil
	if got := Version(cfg); got != BaseVersion {
		t.Fatalf("version=%q", got)
	}

	cfg.MCP.MultiProject = true
	cfg.ArtifactIngress.Enabled = true
	cfg.MCP.Upstreams = []config.UpstreamMCPConfig{{Name: "gw", Mode: "gateway"}}
	got := Version(cfg)
	for _, want := range []string{BaseVersion, "+workspace-v1", "+setup-v3", "+artifact-ingress-v1", "+gateway-v1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("version %q missing %q", got, want)
		}
	}
	if len(got) > 64 {
		t.Fatalf("version too long: %d", len(got))
	}
}

func TestProjectGroupsSchemaVersionIsDistinctAndOptIn(t *testing.T) {
	cfg := config.Default()
	cfg.MCP.MultiProject = true
	cfg.ArtifactIngress.Enabled = false
	legacy := Version(cfg)
	if strings.Contains(legacy, "project-groups") {
		t.Fatalf("legacy version unexpectedly advertised groups: %q", legacy)
	}
	cfg.MCP.ProjectGroupsEnabled = true
	groupVersion := Version(cfg)
	if groupVersion == legacy || !strings.Contains(groupVersion, "+project-groups-v1") {
		t.Fatalf("group schema needs a distinct marker: legacy=%q group=%q", legacy, groupVersion)
	}
	cfg.ArtifactIngress.Enabled = true
	if got := Version(cfg); got == legacy || len(got) > 64 {
		t.Fatalf("group feature marker must survive version shortening: %q", got)
	}
}

func TestStorePersistsConnectorAndFirstConversationVersion(t *testing.T) {
	dir := t.TempDir()
	store := &Store{dir: dir}
	if err := store.RecordConnector("0.6.0-dev+workspace-v1"); err != nil {
		t.Fatal(err)
	}
	if err := store.RememberConversationVersion("abc", "0.5.0-dev"); err != nil {
		t.Fatal(err)
	}
	if err := store.RememberConversationVersion("abc", "0.6.0-dev"); err != nil {
		t.Fatal(err)
	}
	reloaded := &Store{dir: dir}
	if got := reloaded.ConnectorVersion(); got != "0.6.0-dev+workspace-v1" {
		t.Fatalf("connector=%q", got)
	}
	if got := reloaded.ConversationVersion("abc"); got != "0.5.0-dev" {
		t.Fatalf("conversation=%q", got)
	}
	info, err := os.Stat(filepath.Join(dir, "connector"))
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		t.Fatal("connector record is directory")
	}
}

func TestConnectorHistoryMigratesLegacyPlainStateAndRecordsTransitions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "connector"), []byte("0.6.0-dev"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &Store{dir: dir}
	if err := store.RecordConnector("0.7.0-dev"); err != nil {
		t.Fatal(err)
	}
	history := store.History("connector")
	if len(history) != 2 {
		t.Fatalf("history=%#v", history)
	}
	if history[0].Version != "0.6.0-dev" || history[0].Source != "legacy_plain" {
		t.Fatalf("legacy entry=%#v", history[0])
	}
	if history[1].Version != "0.7.0-dev" || history[1].Source != "runtime" {
		t.Fatalf("runtime entry=%#v", history[1])
	}
	if got := store.ConnectorVersion(); got != "0.7.0-dev" {
		t.Fatalf("connector=%q", got)
	}
}

func TestConversationHistoryPreservesFirstObservedVersion(t *testing.T) {
	store := &Store{dir: t.TempDir()}
	if err := store.RememberConversationVersion("abc", "0.6.0-dev"); err != nil {
		t.Fatal(err)
	}
	if err := store.RememberConversationVersion("abc", "0.7.0-dev"); err != nil {
		t.Fatal(err)
	}
	history := store.History("conversation-abc")
	if len(history) != 1 || history[0].Version != "0.6.0-dev" {
		t.Fatalf("history=%#v", history)
	}
}
