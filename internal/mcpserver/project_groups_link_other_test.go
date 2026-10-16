//go:build !windows

package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectGroupSymlinkToSiblingRejected(t *testing.T) {
	f := newPGFixture(t)
	link := filepath.Join(f.paths["hh"], "linked")
	if err := os.Symlink(f.paths["habr"], link); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	runCrossMemberLinkChecks(t, f, "linked")
}

func TestProjectGroupInstructionsAndSkillsRejectSymlinkedSiblingContent(t *testing.T) {
	f := newPGFixture(t)
	if err := os.Remove(filepath.Join(f.paths["hh"], "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.paths["habr"], "AGENTS.md"),
		filepath.Join(f.paths["hh"], "AGENTS.md")); err != nil {
		t.Skipf("file symlink unavailable: %v", err)
	}
	remoteSkill := filepath.Join(f.paths["habr"], "private-skill.md")
	if err := os.WriteFile(remoteSkill, []byte("---\nname: private-habr\ndescription: should not appear in hh\n---\n\nHabr only.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	localSkill := filepath.Join(f.paths["hh"], ".agents", "skills", "leak")
	if err := os.MkdirAll(localSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(remoteSkill, filepath.Join(localSkill, "SKILL.md")); err != nil {
		t.Skipf("skill symlink unavailable: %v", err)
	}
	pgCall(t, f, "set_project_group", map[string]any{"name": "job-search"})
	doc := pgValue(t, pgCall(t, f, "get_project_doc", map[string]any{"repository": "hh"}))
	if strings.Contains(strings.ToLower(doc["content"].(string)), "habr") {
		t.Fatalf("AGENTS.md escaped hh checkout: %+v", doc)
	}
	brief := pgValue(t, pgCall(t, f, "get_agent_brief", map[string]any{"repository": "hh"}))
	if strings.Contains(brief["content"].(string), "instructions-for-habr") {
		t.Fatalf("agent brief imported sibling AGENTS instructions: %+v", brief)
	}
	skills := pgValue(t, pgCall(t, f, "skills_list", map[string]any{"repository": "hh"}))
	if strings.Contains(fmt.Sprint(skills), "private-habr") {
		t.Fatalf("skills catalog leaked sibling skill: %+v", skills)
	}
	pgExpectError(t, f, "skills_read", map[string]any{"repository": "hh", "name": "private-habr"})
}
