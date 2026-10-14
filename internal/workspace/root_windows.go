//go:build windows

package workspace

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// canonicalExistingPath uses the opened file's normalized Windows name, rather
// than only string cleanup, to resolve aliases such as RUNNER~1 and \?\C:\...
// The handle follows directory junctions/reparse points to their actual target.
func canonicalExistingPath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	size := uint32(512)
	for {
		buffer := make([]uint16, size)
		n, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &buffer[0], size, 0)
		if err != nil {
			return "", fmt.Errorf("resolve final Windows path %q: %w", path, err)
		}
		if n < size {
			return NormalizePathIdentity(windows.UTF16ToString(buffer[:n])), nil
		}
		if n >= 32768 {
			return "", fmt.Errorf("final Windows path exceeds supported length: %q", path)
		}
		size = n + 1
	}
}
