//go:build windows

package mcpserver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectGroupWindowsJunctionToSiblingRejected(t *testing.T) {
	f := newPGFixture(t)
	link := filepath.Join(f.paths["hh"], "linked")
	command := exec.Command("cmd", "/d", "/c", "mklink", "/J", link, f.paths["habr"])
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("junction fixture not supported: %v %s", err, output)
	}
	defer os.Remove(link)
	runCrossMemberLinkChecks(t, f, "linked")
}

func TestProjectGroupWindowsJunctionedSkillPackageRejected(t *testing.T) {
	f := newPGFixture(t)
	remote := filepath.Join(f.paths["habr"], ".agents", "skills", "private")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "SKILL.md"),
		[]byte("---\nname: private-habr-junction\ndescription: hidden from hh\n---\n\nHabr only.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	localParent := filepath.Join(f.paths["hh"], ".agents", "skills")
	if err := os.MkdirAll(localParent, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(localParent, "linked")
	command := exec.Command("cmd", "/d", "/c", "mklink", "/J", link, remote)
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("junction fixture not supported: %v %s", err, output)
	}
	defer os.Remove(link)
	pgCall(t, f, "set_project_group", map[string]any{"name": "job-search"})
	result := pgValue(t, pgCall(t, f, "skills_list", map[string]any{"repository": "hh"}))
	if strings.Contains(fmt.Sprint(result), "private-habr-junction") {
		t.Fatalf("sibling skill imported through junction: %+v", result)
	}
	pgExpectError(t, f, "skills_read", map[string]any{"repository": "hh", "name": "private-habr-junction"})
}
