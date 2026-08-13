//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

func diskSpace() (total, free int64) {
	root, err := windows.UTF16PtrFromString(`C:\`)
	if err != nil {
		return 0, 0
	}
	var available, totalBytes, freeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(root, &available, &totalBytes, &freeBytes); err != nil {
		return 0, 0
	}
	return int64(totalBytes), int64(freeBytes)
}
