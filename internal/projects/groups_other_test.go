//go:build !windows

package projects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGroupRegistrySymlinkEscape(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	outside := t.TempDir()
	initGitRepo(t, outside)
	symlink := filepath.Join(root, "symlink-out")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	cfg.Projects[0].Path = symlink
	if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "outside allowed access root") {
		t.Fatalf("symlink escape was accepted: %v", err)
	}
}
