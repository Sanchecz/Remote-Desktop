//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func remoteUserDesktopDirectory(cfg *config) (string, error) {
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
	desktop := filepath.Join(profile, "Desktop")
	if shellKey, shellErr := registry.OpenKey(registry.USERS, sid+`\Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`, registry.QUERY_VALUE); shellErr == nil {
		if configured, _, valueErr := shellKey.GetStringValue("Desktop"); valueErr == nil && strings.TrimSpace(configured) != "" {
			desktop = expandWindowsProfilePath(configured, profile)
		}
		shellKey.Close()
	}
	desktop = filepath.Clean(desktop)
	if !filepath.IsAbs(desktop) {
		return "", errors.New("Windows вернул некорректный путь рабочего стола")
	}
	info, statErr := os.Stat(desktop)
	if statErr != nil || !info.IsDir() {
		return "", errors.New("рабочий стол закреплённого пользователя недоступен")
	}
	return desktop, nil
}

func expandWindowsProfilePath(value, profile string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, `%USERPROFILE%`, profile)
	value = strings.ReplaceAll(value, `%userprofile%`, profile)
	return os.ExpandEnv(value)
}
