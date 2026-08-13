//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
)

const consoleURL = "https://supportgenesis.ru"

func main() {
	window, err := walk.NewMainWindow()
	if err != nil {
		return
	}
	defer window.Dispose()
	window.SetTitle("RemoteIt Console")
	icon, iconErr := walk.NewIconFromResourceId(1)
	if iconErr == nil {
		defer icon.Dispose()
		_ = window.SetIcon(icon)
	}

	tray, err := walk.NewNotifyIcon(window)
	if err != nil {
		return
	}
	defer tray.Dispose()
	if iconErr == nil {
		_ = tray.SetIcon(icon)
	}
	_ = tray.SetToolTip("RemoteIt Console — открыть панель")

	var processMu sync.Mutex
	var panelProcess *exec.Cmd
	openPanel := func() {
		processMu.Lock()
		if panelProcess != nil && panelProcess.ProcessState == nil {
			processMu.Unlock()
			return
		}
		command := newPanelCommand()
		if command == nil || command.Start() != nil {
			processMu.Unlock()
			return
		}
		panelProcess = command
		processMu.Unlock()
		go func(current *exec.Cmd) {
			_ = current.Wait()
			processMu.Lock()
			if panelProcess == current {
				panelProcess = nil
			}
			processMu.Unlock()
		}(command)
	}

	tray.MouseDown().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			openPanel()
		}
	})
	openAction := walk.NewAction()
	_ = openAction.SetText("Открыть RemoteIt Console")
	openAction.Triggered().Attach(openPanel)
	_ = tray.ContextMenu().Actions().Add(openAction)

	settingsAction := walk.NewAction()
	_ = settingsAction.SetText("Настройки")
	settingsAction.Triggered().Attach(func() {
		command := newPanelCommandWithURL(consoleURL + "/?section=settings")
		if command != nil {
			_ = command.Start()
		}
	})
	_ = tray.ContextMenu().Actions().Add(settingsAction)

	exitAction := walk.NewAction()
	_ = exitAction.SetText("Выйти из RemoteIt Console")
	exitAction.Triggered().Attach(func() { walk.App().Exit(0) })
	_ = tray.ContextMenu().Actions().Add(exitAction)

	if err := tray.SetVisible(true); err != nil {
		return
	}
	openPanel()
	window.Run()
}

func newPanelCommand() *exec.Cmd {
	return newPanelCommandWithURL(consoleURL)
}

func newPanelCommandWithURL(url string) *exec.Cmd {
	profileDir := filepath.Join(os.Getenv("LocalAppData"), "RemoteIt", "Console")
	_ = os.MkdirAll(profileDir, 0o700)
	paths := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			command := exec.Command(path, "--app="+url, "--start-maximized", "--no-first-run", "--user-data-dir="+profileDir)
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
			return command
		}
	}
	command := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	return command
}
