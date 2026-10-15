package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

// runCrossMemberLinkChecks uses an alias under hh pointing at the sibling
// habr checkout. The access root contains both, but a member's file tools
// must not inherit access to sibling checkout content.
func runCrossMemberLinkChecks(t *testing.T, f *pgFixture, aliasPath string) {
	t.Helper()
	pgCall(t, f, "set_project_group", map[string]any{"name": "job-search"})
	for _, test := range []struct {
		tool string
		args map[string]any
	}{
		{"read_file", map[string]any{"repository": "hh", "path": filepath.ToSlash(filepath.Join(aliasPath, "README.md"))}},
		{"grep", map[string]any{"repository": "hh", "path": aliasPath, "pattern": "project-habr"}},
		{"write_file", map[string]any{"repository": "hh", "path": filepath.ToSlash(filepath.Join(aliasPath, "intrusion.txt")), "content": "bad"}},
		{"apply_patch", map[string]any{"repository": "hh", "input": "*** Begin Patch\n*** Add File: " + filepath.ToSlash(filepath.Join(aliasPath, "intrusion-patch.txt")) + "\n+bad\n*** End Patch"}},
		{"export_host_file", map[string]any{"repository": "hh", "path": filepath.ToSlash(filepath.Join(aliasPath, "README.md"))}},
		{"exec_command", map[string]any{"repository": "hh", "workdir": aliasPath, "cmd": "echo test", "yield_time_ms": 1000}},
	} {
		pgExpectError(t, f, test.tool, test.args)
	}
	for _, name := range []string{"intrusion.txt", "intrusion-patch.txt"} {
		if _, err := os.Stat(filepath.Join(f.paths["habr"], name)); !os.IsNotExist(err) {
			t.Fatalf("cross-member %s was created outside hh: %v", name, err)
		}
	}
}
