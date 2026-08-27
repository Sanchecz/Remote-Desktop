//go:build !windows

package main

func currentInstallSessionOwner() (string, string, error) {
	return "", "", nil
}

func prepareWindowsSessionBinding(*config) (bool, error) {
	return false, nil
}
