//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func remoteUserKnownDirectory(cfg *config, valueName string) (string, error) {
	if cfg == nil {
		return "", errors.New("не задан пользователь удалённого рабочего стола")
	}
	sid := normalizeWindowsUserSID(cfg.WindowsSessionUserSID)
	if sid == "" {
		return "", errors.New("Agent не привязан к Windows-пользователю")
	}
	profileKey, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`+sid, registry.QUERY_VALUE)
	if err != nil {
		return "", errors.New("не найден профиль закреплённого Windows-пользователя")
	}
	profile, _, readErr := profileKey.GetStringValue("ProfileImagePath")
	profileKey.Close()
	profile = strings.TrimSpace(profile)
	if readErr != nil || profile == "" {
		return "", errors.New("не удалось определить профиль закреплённого Windows-пользователя")
	}
	profile = expandWindowsProfilePath(profile, profile)
	defaults := map[string]string{"Desktop": "Desktop", "Downloads": "Downloads", "Personal": "Documents"}
	folderName, ok := defaults[valueName]
	if !ok {
		return "", errors.New("неподдерживаемая папка закреплённого пользователя")
	}
	destination := filepath.Join(profile, folderName)
	if shellKey, shellErr := registry.OpenKey(registry.USERS, sid+`\Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`, registry.QUERY_VALUE); shellErr == nil {
		if configured, _, valueErr := shellKey.GetStringValue(valueName); valueErr == nil && strings.TrimSpace(configured) != "" {
			destination = expandWindowsProfilePath(configured, profile)
		}
		shellKey.Close()
	}
	destination = filepath.Clean(destination)
	if !filepath.IsAbs(destination) {
		return "", errors.New("Windows вернул некорректный путь пользовательской папки")
	}
	info, statErr := os.Stat(destination)
	if statErr != nil || !info.IsDir() {
		return "", errors.New("папка закреплённого пользователя недоступна")
	}
	return destination, nil
}

func remoteUserDesktopDirectory(cfg *config) (string, error) {
	return remoteUserKnownDirectory(cfg, "Desktop")
}

func expandWindowsProfilePath(value, profile string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, `%USERPROFILE%`, profile)
	value = strings.ReplaceAll(value, `%userprofile%`, profile)
	return os.ExpandEnv(value)
}
