//go:build !windows

package main

import (
	"os"
)

func effectiveDeviceName(cfg *config) string {
	return cfg.DeviceName
}

func persistDeviceName(name string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	cfg.DeviceName = name
	return saveConfig(cfg)
}

func setupAgentUserFiles(_ *config) error {
	return nil
}

func protectPrivateFile(path string) error {
	return os.Chmod(path, 0o600)
}

func makeRuntimeStatusReadable(path string) {
	_ = os.Chmod(path, 0o644)
}

func publishPublicRuntimeStatus(_ runtimeStatus) {}

func replaceAgentStatusFile(temporary, target string) error {
	return os.Rename(temporary, target)
}
