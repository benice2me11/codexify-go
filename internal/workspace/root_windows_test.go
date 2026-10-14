//go:build windows

package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsShortAndLongNamesAreOneWorkspace(t *testing.T) {
	long := os.Getenv("ProgramFiles")
	if long == "" {
		t.Skip("no ProgramFiles path available")
	}
	ptr, err := windows.UTF16PtrFromString(long)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 32768)
	n, err := windows.GetShortPathName(ptr, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || n >= uint32(len(buf)) {
		t.Skipf("no Windows 8.3 alias available: %v", err)
	}
	short := windows.UTF16ToString(buf[:n])
	if strings.EqualFold(short, long) {
		t.Skip("Windows 8.3 aliases unavailable")
	}
	fromShort, err := New(short)
	if err != nil {
		t.Fatal(err)
	}
	fromLong, err := New(long)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(fromShort.Path(), fromLong.Path()) {
		t.Fatalf("short name %q and long name %q disagree: %q vs %q", short, long, fromShort.Path(), fromLong.Path())
	}
	if got, err := fromShort.Relative(long); err != nil || got != "." {
		t.Fatalf("long root should be inside short root, got %q, %v", got, err)
	}
	entries, err := os.ReadDir(long)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		target := filepath.Join(long, entry.Name())
		if _, err := fromShort.Relative(target); err == nil {
			return
		}
	}
	t.Fatal("none of the ProgramFiles entries could be accessed via the canonical short-name root")
}

func TestWindowsJunctionEscapeIsDenied(t *testing.T) {
	rootDir := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(rootDir, "outside-junction")
	out, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, outside).CombinedOutput()
	if err != nil {
		t.Skipf("cannot create junction without privilege: %v (%s)", err, out)
	}
	root, err := New(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.Resolve("outside-junction", false); err == nil {
		t.Fatal("junction to outside directory was accepted")
	}
	if _, err := root.Resolve(filepath.Join("outside-junction", "new-file.txt"), true); err == nil {
		t.Fatal("missing child under outside junction was accepted")
	}
	if _, err := root.Relative(link); err == nil {
		t.Fatal("Relative accepted a junction outside the root")
	}
}
