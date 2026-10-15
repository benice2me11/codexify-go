//go:build windows

package projects

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// directoryIdentity captures the filesystem object identity of an existing
// directory. Unlike a canonical path string, a volume + file ID changes when
// a checkout is removed and replaced at the same name between worker starts.
func directoryIdentity(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return "", err
	}
	if info.VolumeSerialNumber == 0 && info.FileIndexHigh == 0 && info.FileIndexLow == 0 {
		return "", fmt.Errorf("filesystem identity unavailable for %q", path)
	}
	return fmt.Sprintf("win:%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
