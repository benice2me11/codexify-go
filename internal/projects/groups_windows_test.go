//go:build windows

package projects

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benice2me11/codexify-go/internal/workspace"
	"golang.org/x/sys/windows"
)

func TestGroupRegistryWindowsExtendedPathIdentity(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	cfg.Projects[0].Path = `\\?\` + filepath.Join(root, "work-search")
	manager, err := New(cfg)
	if err != nil {
		t.Fatalf("valid extended-path spelling rejected: %v", err)
	}
	resolved, err := manager.ResolveProjectGroupSource("job-search", "work")
	want, canonicalErr := workspace.Canonical(filepath.Join(root, "work-search"))
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if err != nil || !sameGroupPath(resolved.Member.SourceProjectRoot, want) {
		t.Fatalf("extended path resolved incorrectly: %+v, %v", resolved.Member, err)
	}
}

func TestGroupRegistryWindowsShortNameAlias(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	long := filepath.Join(root, "work-search")
	p, err := windows.UTF16PtrFromString(long)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, 32768)
	n, err := windows.GetShortPathName(p, &buffer[0], uint32(len(buffer)))
	if err != nil || n == 0 || n >= uint32(len(buffer)) {
		t.Skipf("8.3 aliases unavailable on this volume: n=%d err=%v", n, err)
	}
	short := windows.UTF16ToString(buffer[:n])
	if strings.EqualFold(short, long) {
		t.Skip("8.3 short-name generation is disabled on this filesystem")
	}
	cfg.Projects[0].Path = short
	manager, err := New(cfg)
	if err != nil {
		t.Fatalf("valid short-name alias should canonicalize: %v", err)
	}
	resolved, err := manager.ResolveProjectGroupSource("job-search", "work")
	want, canonicalErr := workspace.Canonical(long)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if err != nil || !sameGroupPath(resolved.Member.SourceProjectRoot, want) {
		t.Fatalf("8.3 source identity mismatch: %+v, %v", resolved.Member, err)
	}
}

func TestGroupRegistryWindowsJunctionEscape(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	outside := t.TempDir()
	initGitRepo(t, outside)
	junction := filepath.Join(root, "junction-out")
	cmd := exec.Command("cmd", "/c", "mklink", "/J", junction, outside)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("junction fixtures unavailable: %v: %s", err, output)
	}
	defer os.Remove(junction)
	cfg.Projects[0].Path = junction
	if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "outside allowed access root") {
		t.Fatalf("junction escape was accepted: %v", err)
	}
}
