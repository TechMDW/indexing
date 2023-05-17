//go:build windows
// +build windows

package attributes

import (
	"runtime"

	"golang.org/x/sys/windows"
)

type WindowsAttributes struct {
	ReadOnly          bool `json:"read_only"`
	Hidden            bool `json:"hidden"`
	System            bool `json:"system"`
	Directory         bool `json:"directory"`
	Archive           bool `json:"archive"`
	Normal            bool `json:"normal"`
	Temporary         bool `json:"temporary"`
	Offline           bool `json:"offline"`
	NotContentIndexed bool `json:"notContentIndexed"`
	Encrypted         bool `json:"encrypted"`
	OneDrive          bool `json:"oneDrive"`
}

func GetFileAttributes(path string) (WindowsAttributes, error) {
	if runtime.GOOS != "windows" {
		return WindowsAttributes{}, nil
	}

	var attrs WindowsAttributes

	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return attrs, err
	}

	attr, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return attrs, err
	}

	attrs.ReadOnly = attr&windows.FILE_ATTRIBUTE_READONLY != 0
	attrs.Hidden = attr&windows.FILE_ATTRIBUTE_HIDDEN != 0
	attrs.System = attr&windows.FILE_ATTRIBUTE_SYSTEM != 0
	attrs.Directory = attr&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	attrs.Archive = attr&windows.FILE_ATTRIBUTE_ARCHIVE != 0
	attrs.Normal = attr&windows.FILE_ATTRIBUTE_NORMAL != 0
	attrs.Temporary = attr&windows.FILE_ATTRIBUTE_TEMPORARY != 0
	attrs.Offline = attr&windows.FILE_ATTRIBUTE_OFFLINE != 0
	attrs.NotContentIndexed = attr&windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED != 0
	attrs.Encrypted = attr&windows.FILE_ATTRIBUTE_ENCRYPTED != 0
	attrs.OneDrive = attr&0x00400000 != 0

	return attrs, nil
}
