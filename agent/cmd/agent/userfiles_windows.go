//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var runtimeStatusACLOnce sync.Once
var publicEventsACLOnce sync.Once
var publicEventsMu sync.Mutex
var replaceFileWindows = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

const maxPublicAgentEvents = 80

type publicAgentEvent struct {
	At     time.Time `json:"at"`
	Level  string    `json:"level"`
	Kind   string    `json:"kind"`
	Title  string    `json:"title"`
	Detail string    `json:"detail"`
}

type publicAgentInfo struct {
	ServerURL      string    `json:"serverUrl"`
	DeviceName     string    `json:"deviceName"`
	ConnectionCode string    `json:"connectionCode"`
	Version        string    `json:"version"`
	LocalIPs       []string  `json:"localIps"`
	Running        bool      `json:"running"`
	Connected      bool      `json:"connected"`
	LastHeartbeat  time.Time `json:"lastHeartbeat,omitempty"`
	LastError      string    `json:"lastError,omitempty"`
}

func publishPublicRuntimeStatus(status runtimeStatus) {
	info, err := loadPublicAgentInfo()
	if err != nil {
		info = publicAgentInfo{ServerURL: defaultServer, Version: version, LocalIPs: localIPs()}
	}
	info.Running = status.Running
	info.Connected = status.Connected
	info.LocalIPs = localIPs()
	info.LastHeartbeat = status.LastHeartbeat
	info.LastError = status.LastError
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return
	}
	path := publicInfoPath()
	temporary := path + ".runtime"
	if os.WriteFile(temporary, data, 0o644) == nil {
		_ = replaceAgentStatusFile(temporary, path)
	}
}

func replaceAgentStatusFile(temporary, target string) error {
	temporaryPointer, err := windows.UTF16PtrFromString(temporary)
	if err != nil {
		return err
	}
	targetPointer, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		result, _, callErr := replaceFileWindows.Call(uintptr(unsafe.Pointer(targetPointer)), uintptr(unsafe.Pointer(temporaryPointer)), 0, 0x00000001, 0, 0)
		if result != 0 {
			return nil
		}
		return callErr
	}
	return windows.MoveFileEx(temporaryPointer, targetPointer, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

type desktopAgentAccess struct {
	ServerURL     string `json:"serverUrl"`
	DeviceID      string `json:"deviceId"`
	DesktopSecret string `json:"desktopSecret"`
}

func agentDataDir() string {
	return filepath.Dir(defaultConfigPath())
}

func deviceNamePath() string {
	return filepath.Join(agentDataDir(), "device-name.txt")
}

func publicInfoPath() string {
	return filepath.Join(agentDataDir(), "public.json")
}

func desktopAccessPath() string {
	return filepath.Join(agentDataDir(), "desktop-access.json")
}

func publicEventsPath() string {
	return filepath.Join(agentDataDir(), "public-events.json")
}

// appendPublicAgentEvent writes a deliberately small, secret-free activity
// journal for the interactive tray. The private service log and protected
// configuration remain accessible only to LocalSystem/administrators.
func appendPublicAgentEvent(level, kind, title, detail string) {
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case "success", "info", "warning", "error":
	default:
		level = "info"
	}
	kind = truncateText(strings.ReplaceAll(strings.ReplaceAll(kind, "\r", " "), "\n", " "), 32)
	title = truncateText(strings.ReplaceAll(strings.ReplaceAll(title, "\r", " "), "\n", " "), 120)
	detail = truncateText(strings.ReplaceAll(strings.ReplaceAll(detail, "\r", " "), "\n", " "), 240)
	if title == "" {
		return
	}

	publicEventsMu.Lock()
	defer publicEventsMu.Unlock()

	events, _ := loadPublicAgentEventsUnlocked()
	now := time.Now().UTC()
	if len(events) > 0 {
		last := &events[len(events)-1]
		if last.Level == level && last.Kind == kind && last.Title == title && last.Detail == detail && now.Sub(last.At) < 2*time.Minute {
			last.At = now
		} else {
			events = append(events, publicAgentEvent{At: now, Level: level, Kind: kind, Title: title, Detail: detail})
		}
	} else {
		events = append(events, publicAgentEvent{At: now, Level: level, Kind: kind, Title: title, Detail: detail})
	}
	if len(events) > maxPublicAgentEvents {
		events = append([]publicAgentEvent(nil), events[len(events)-maxPublicAgentEvents:]...)
	}
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return
	}
	path := publicEventsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return
	}
	if err := replaceAgentStatusFile(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return
	}
	publicEventsACLOnce.Do(func() {
		_ = windowsHiddenCommand("icacls", path, "/inheritance:r", "/grant:r", "*S-1-5-18:F", "*S-1-5-32-544:F", "*S-1-5-32-545:R").Run()
	})
}

func loadPublicAgentEvents() ([]publicAgentEvent, error) {
	publicEventsMu.Lock()
	defer publicEventsMu.Unlock()
	return loadPublicAgentEventsUnlocked()
}

func loadPublicAgentEventsUnlocked() ([]publicAgentEvent, error) {
	data, err := os.ReadFile(publicEventsPath())
	if err != nil {
		return nil, err
	}
	var events []publicAgentEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func effectiveDeviceName(cfg *config) string {
	data, err := os.ReadFile(deviceNamePath())
	if err == nil {
		if name := strings.TrimSpace(string(data)); name != "" && len([]rune(name)) <= 64 {
			return name
		}
	}
	return cfg.DeviceName
}

func persistDeviceName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return fmt.Errorf("название должно содержать от 1 до 64 символов")
	}
	return os.WriteFile(deviceNamePath(), []byte(name+"\n"), 0o644)
}

func setupAgentUserFiles(cfg *config) error {
	if err := os.MkdirAll(agentDataDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(deviceNamePath(), []byte(cfg.DeviceName+"\n"), 0o644); err != nil {
		return err
	}
	status, _ := loadRuntimeStatus()
	data, err := json.MarshalIndent(publicAgentInfo{
		ServerURL:      cfg.ServerURL,
		DeviceName:     cfg.DeviceName,
		ConnectionCode: cfg.ConnectionCode,
		Version:        version,
		LocalIPs:       localIPs(),
		Running:        status.Running,
		Connected:      status.Connected,
		LastHeartbeat:  status.LastHeartbeat,
		LastError:      status.LastError,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(publicInfoPath(), data, 0o644); err != nil {
		return err
	}
	if cfg.DeviceID == "" || cfg.DesktopSecret == "" {
		return nil
	}
	desktopData, err := json.Marshal(desktopAgentAccess{ServerURL: cfg.ServerURL, DeviceID: cfg.DeviceID, DesktopSecret: cfg.DesktopSecret})
	if err != nil {
		return err
	}
	if err := os.WriteFile(desktopAccessPath(), desktopData, 0o644); err != nil {
		return err
	}
	if !useUserConfig() {
		if output, aclErr := windowsHiddenCommand("icacls", desktopAccessPath(), "/inheritance:r", "/grant:r", "*S-1-5-18:F", "*S-1-5-32-544:F", "*S-1-5-32-545:R").CombinedOutput(); aclErr != nil {
			return fmt.Errorf("не удалось открыть параметры удалённого экрана: %w (%s)", aclErr, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func loadDesktopAgentAccess() (desktopAgentAccess, error) {
	data, err := os.ReadFile(desktopAccessPath())
	if err != nil {
		return desktopAgentAccess{}, err
	}
	var access desktopAgentAccess
	if err := json.Unmarshal(data, &access); err != nil {
		return desktopAgentAccess{}, err
	}
	access.ServerURL = strings.TrimRight(strings.TrimSpace(access.ServerURL), "/")
	access.DeviceID = strings.TrimSpace(access.DeviceID)
	access.DesktopSecret = strings.TrimSpace(access.DesktopSecret)
	if access.ServerURL == "" || access.DeviceID == "" || access.DesktopSecret == "" {
		return desktopAgentAccess{}, fmt.Errorf("неполные параметры удалённого экрана")
	}
	return access, nil
}

func loadPublicAgentInfo() (publicAgentInfo, error) {
	data, err := os.ReadFile(publicInfoPath())
	if err != nil {
		return publicAgentInfo{}, err
	}
	var info publicAgentInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return publicAgentInfo{}, err
	}
	info.DeviceName = strings.TrimSpace(info.DeviceName)
	if data, err := os.ReadFile(deviceNamePath()); err == nil {
		if name := strings.TrimSpace(string(data)); name != "" {
			info.DeviceName = name
		}
	}
	return info, nil
}

func protectPrivateFile(path string) error {
	grants := []string{"*S-1-5-18:F", "*S-1-5-32-544:F"}
	if useUserConfig() {
		account, err := user.Current()
		if err != nil || strings.TrimSpace(account.Username) == "" {
			return fmt.Errorf("не удалось определить текущего пользователя для защиты конфигурации")
		}
		grants = []string{account.Username + ":F"}
	}
	arguments := []string{path, "/inheritance:r", "/grant:r"}
	arguments = append(arguments, grants...)
	command := windowsHiddenCommand("icacls", arguments...)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("не удалось защитить конфигурацию агента: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func makeRuntimeStatusReadable(path string) {
	runtimeStatusACLOnce.Do(func() {
		// The tray runs in the interactive user session while the heartbeat runs
		// as LocalSystem. Publish only this secret-free status file to local users.
		_ = windowsHiddenCommand("icacls", path, "/inheritance:r", "/grant:r", "*S-1-5-18:F", "*S-1-5-32-544:F", "*S-1-5-32-545:R").Run()
	})
}
