package artifacts

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/workspace"
)

func TestDurableExportKeepsImmutableSnapshot(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "report.txt"), []byte("original report\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.New(project)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(config.ArtifactEgressConfig{
		Enabled:              true,
		Dir:                  t.TempDir(),
		MaxFileBytes:         1024,
		SnapshotMaxFileBytes: 1024,
		MaxSnapshotBytes:     4096,
		FallbackToSource:     true,
		MaxReferences:        8,
		ReferenceTTL:         config.Duration(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	link, receipt, err := store.Export(root, "report.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Durable || receipt.Path != "report.txt" || receipt.ChatLink != link.URI {
		t.Fatalf("unexpected receipt: %+v link=%+v", receipt, link)
	}
	if strings.Contains(receipt.ChatLink, project) {
		t.Fatal("artifact URI leaked host path")
	}
	if err := os.WriteFile(filepath.Join(project, "report.txt"), []byte("changed later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read, err := store.Read(link.URI)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Contents) != 1 || !bytes.Equal(read.Contents[0].Blob, []byte("original report\n")) {
		t.Fatalf("snapshot changed: %+v", read.Contents)
	}
}

func TestFallbackResourceReadsLatestSafeSource(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "large.txt"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.New(project)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(config.ArtifactEgressConfig{
		Enabled:              true,
		Dir:                  t.TempDir(),
		MaxFileBytes:         1024,
		SnapshotMaxFileBytes: 0,
		MaxSnapshotBytes:     0,
		FallbackToSource:     true,
		MaxReferences:        8,
		ReferenceTTL:         config.Duration(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	link, receipt, err := store.Export(root, "large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Durable {
		t.Fatal("expected fallback-to-source reference")
	}
	if err := os.WriteFile(filepath.Join(project, "large.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read, err := store.Read(link.URI)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Contents) != 1 || string(read.Contents[0].Blob) != "second\n" {
		t.Fatalf("fallback did not read latest source: %+v", read.Contents)
	}
}

func TestExportRejectsWorkspaceEscape(t *testing.T) {
	project := t.TempDir()
	root, err := workspace.New(project)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(config.ArtifactEgressConfig{
		Enabled:              true,
		Dir:                  t.TempDir(),
		MaxFileBytes:         1024,
		SnapshotMaxFileBytes: 1024,
		MaxSnapshotBytes:     4096,
		FallbackToSource:     true,
		MaxReferences:        8,
		ReferenceTTL:         config.Duration(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Export(root, "../outside.txt"); err == nil {
		t.Fatal("expected workspace escape to be rejected")
	}
}

func TestExportRejectsNonSnapshotWhenFallbackDisabled(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "report.txt"), []byte("report\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.New(project)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(config.ArtifactEgressConfig{
		Enabled:              true,
		Dir:                  t.TempDir(),
		MaxFileBytes:         1024,
		SnapshotMaxFileBytes: 0,
		MaxSnapshotBytes:     0,
		FallbackToSource:     false,
		MaxReferences:        8,
		ReferenceTTL:         config.Duration(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Export(root, "report.txt"); err == nil {
		t.Fatal("expected export to fail when snapshot is unavailable and fallback is disabled")
	}
}

func TestGroupedArtifactRequiresMatchingOwnerAndExpires(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "private.txt"), []byte("group-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.New(project)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(config.ArtifactEgressConfig{
		Enabled: true, Dir: t.TempDir(),
		MaxFileBytes: 1024, SnapshotMaxFileBytes: 1024, MaxSnapshotBytes: 4096,
		FallbackToSource: true, MaxReferences: 8, ReferenceTTL: config.Duration(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{Conversation: "chat-one", Group: "job-search", Repository: "hh",
		ProjectRoot: root.Path(), Fingerprint: "snapshot", BindingEpoch: "epoch-1"}
	if _, _, err := store.ExportScoped(root, "private.txt", &Owner{Conversation: "chat-one"}); err == nil {
		t.Fatal("export accepted incomplete or mismatched owner")
	}
	link, receipt, err := store.ExportScoped(root, "private.txt", &owner)
	if err != nil || !receipt.Durable {
		t.Fatalf("grouped snapshot export failed: %+v %v", receipt, err)
	}
	if _, err := store.Read(link.URI); err == nil {
		t.Fatal("unscoped read of grouped snapshot succeeded")
	}
	for _, different := range []Owner{
		{Conversation: "chat-two", Group: owner.Group, Repository: owner.Repository, ProjectRoot: owner.ProjectRoot, Fingerprint: owner.Fingerprint, BindingEpoch: owner.BindingEpoch},
		{Conversation: owner.Conversation, Group: owner.Group, Repository: "habr", ProjectRoot: owner.ProjectRoot, Fingerprint: owner.Fingerprint, BindingEpoch: owner.BindingEpoch},
		{Conversation: owner.Conversation, Group: owner.Group, Repository: owner.Repository, ProjectRoot: owner.ProjectRoot, Fingerprint: owner.Fingerprint, BindingEpoch: "epoch-2"},
		{Conversation: owner.Conversation, Group: owner.Group, Repository: owner.Repository, ProjectRoot: owner.ProjectRoot, Fingerprint: "stale", BindingEpoch: owner.BindingEpoch},
	} {
		if _, err := store.ReadScoped(link.URI, &different); err == nil {
			t.Fatalf("group artifact leaked to owner %+v", different)
		}
	}
	allowed, err := store.ReadScoped(link.URI, &owner)
	if err != nil || string(allowed.Contents[0].Blob) != "group-secret\n" {
		t.Fatalf("original owner could not read: %+v %v", allowed, err)
	}
	data, err := os.ReadFile(store.recordPath(strings.TrimPrefix(link.URI, Prefix)))
	if err != nil {
		t.Fatal(err)
	}
	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Version != 2 || rec.Owner == nil {
		t.Fatalf("owner was not persisted: %+v", rec)
	}
	rec.CreatedAt = time.Now().Add(-3 * time.Minute).UTC().Format(time.RFC3339Nano)
	changed, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.recordPath(rec.Token), changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadScoped(link.URI, &owner); err == nil {
		t.Fatal("expired grouped durable snapshot remained accessible")
	}
	if _, err := os.Stat(store.recordPath(rec.Token)); !os.IsNotExist(err) {
		t.Fatalf("expired grouped artifact record was not purged: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.root, rec.Snapshot)); !os.IsNotExist(err) {
		t.Fatalf("expired grouped snapshot blob was not purged: %v", err)
	}
}

func TestGroupedFallbackRemainsMemberScoped(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "stream.txt"), []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.New(project)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(config.ArtifactEgressConfig{
		Enabled: true, Dir: t.TempDir(),
		MaxFileBytes: 1024, SnapshotMaxFileBytes: 0, MaxSnapshotBytes: 0,
		FallbackToSource: true, MaxReferences: 8, ReferenceTTL: config.Duration(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{Conversation: "chat-a", Group: "job-search", Repository: "work",
		ProjectRoot: root.Path(), Fingerprint: "v1", BindingEpoch: "e1"}
	link, receipt, err := store.ExportScoped(root, "stream.txt", &owner)
	if err != nil || receipt.Durable {
		t.Fatalf("expected scoped fallback export: %+v %v", receipt, err)
	}
	if _, err := store.Read(link.URI); err == nil {
		t.Fatal("fallback leaked to unscoped caller")
	}
	if err := os.WriteFile(filepath.Join(project, "stream.txt"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	read, err := store.ReadScoped(link.URI, &owner)
	if err != nil || string(read.Contents[0].Blob) != "second\n" {
		t.Fatalf("scoped fallback failed to preserve legacy latest-source semantics: %+v %v", read, err)
	}
}
