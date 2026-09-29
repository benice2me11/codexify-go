package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNormalizePathIdentityWindowsAliases(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path identity semantics")
	}
	tests := map[string]string{
		`\\?\C:\repo`:               `C:\repo`,
		`\\.\C:\repo`:               `C:\repo`,
		`\\?\UNC\server\share\repo`: `\\server\share\repo`,
		`\\.\UNC\server\share\repo`: `\\server\share\repo`,
	}
	for input, want := range tests {
		if got := NormalizePathIdentity(input); got != want {
			t.Fatalf("NormalizePathIdentity(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWindowsExtendedAndOrdinaryPathsShareIdentityAndContainment(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path identity semantics")
	}
	dir := t.TempDir()
	child := filepath.Join(dir, "sub")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	extended := `\\?\` + dir
	root, err := New(extended)
	if err != nil {
		t.Fatal(err)
	}
	ordinaryCanonical, err := Canonical(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(root.Path(), ordinaryCanonical) {
		t.Fatalf("extended root %q and ordinary root %q differ", root.Path(), ordinaryCanonical)
	}
	if _, err := root.Relative(child); err != nil {
		t.Fatalf("ordinary child rejected under extended root: %v", err)
	}
	extendedChild := `\\?\` + child
	if _, err := root.Relative(extendedChild); err != nil {
		t.Fatalf("extended child rejected under normalized root: %v", err)
	}
	sibling := dir + "-other"
	if _, err := root.Relative(sibling); err == nil {
		t.Fatal("sibling path was incorrectly accepted as a child")
	}
}

func TestResolveRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	r, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve("../escape", true); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestResolveMissingChild(t *testing.T) {
	root := t.TempDir()
	r, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Resolve(filepath.Join("a", "b", "file.txt"), true)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Canonical(filepath.Join(root, "a", "b", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires Windows Developer Mode or elevation")
	}
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	r, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(filepath.Join("link", "file.txt"), true); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}
