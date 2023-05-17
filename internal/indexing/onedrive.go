//go:build windows
// +build windows

package indexing

import (
	"runtime"

	"golang.org/x/sys/windows"
)

// isOneDrivePlaceholder returns true if the file is a OneDrive placeholder file.
func isOneDrivePlaceholder(path string) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, nil
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}

	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return false, err
	}
	return attrs&0x00400000 != 0, nil
}
