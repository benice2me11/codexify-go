//go:build !windows

package projects

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

func directoryIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("filesystem identity requires directory: %q", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("filesystem identity unavailable for %q", path)
	}
	return fmt.Sprintf("%s:%x:%x", runtime.GOOS, stat.Dev, stat.Ino), nil
}
