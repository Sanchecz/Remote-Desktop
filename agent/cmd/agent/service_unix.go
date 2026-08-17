//go:build !windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func backgroundCommandOutput(name string, arguments ...string) ([]byte, error) {
	return exec.Command(name, arguments...).Output()
}

func prepareBackgroundCommand(_ *exec.Cmd) {}

func runAsService(_ func(context.Context) error) (bool, error) {
	return false, nil
}

func useUserConfig() bool {
	return false
}

func agentExecutionMode() (string, bool) {
	if os.Geteuid() == 0 {
		return "system", true
	}
	return "user", false
}

func forceAgentUpdateCheckPlatform() error {
	return errors.New("ручная проверка обновлений из окна Agent доступна в Windows; Linux и macOS проверяют обновления каждым heartbeat")
}

func installPlatform() error {
	if os.Geteuid() != 0 {
		return errors.New("установку службы необходимо запустить с sudo")
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		target := "/opt/genesisit/agent/genesis-agent"
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyUnixFile(current, target); err != nil {
			return err
		}
		service := `[Unit]
Description=RemoteIt Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/genesisit/agent/genesis-agent run
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
		if err := os.WriteFile("/etc/systemd/system/genesisit-agent.service", []byte(service), 0o644); err != nil {
			return err
		}
		if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
			return err
		}
		if err := exec.Command("systemctl", "enable", "genesisit-agent.service").Run(); err != nil {
			return err
		}
		// A reinstall must switch the running process to the new binary.
		return exec.Command("systemctl", "restart", "genesisit-agent.service").Run()
	case "darwin":
		target := "/Library/Application Support/GenesisIt/genesis-agent"
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyUnixFile(current, target); err != nil {
			return err
		}
		plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>ru.supportgenesis.agent</string>
<key>ProgramArguments</key><array><string>/Library/Application Support/GenesisIt/genesis-agent</string><string>run</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
</dict></plist>`
		path := "/Library/LaunchDaemons/ru.supportgenesis.agent.plist"
		if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
			return err
		}
		_ = exec.Command("launchctl", "bootout", "system", path).Run()
		return exec.Command("launchctl", "bootstrap", "system", path).Run()
	default:
		return fmt.Errorf("установка службы пока не поддерживается для %s", runtime.GOOS)
	}
}

func scheduleRemoteUninstall() error {
	if os.Geteuid() != 0 {
		return errors.New("удалённое удаление системного агента требует root")
	}
	switch runtime.GOOS {
	case "linux":
		unit := fmt.Sprintf("genesisit-agent-remove-%d", os.Getpid())
		script := `systemctl disable --now genesisit-agent.service >/dev/null 2>&1 || true; rm -f /etc/systemd/system/genesisit-agent.service; systemctl daemon-reload >/dev/null 2>&1 || true; rm -rf /opt/genesisit/agent /var/lib/genesisit`
		if output, err := exec.Command("systemd-run", "--quiet", "--collect", "--unit="+unit, "--on-active=10s", "/bin/sh", "-c", script).CombinedOutput(); err != nil {
			return fmt.Errorf("не удалось запланировать удаление Linux-агента: %w (%s)", err, string(output))
		}
		return nil
	case "darwin":
		label := fmt.Sprintf("ru.supportgenesis.agent.remove.%d", os.Getpid())
		script := `sleep 10; launchctl bootout system '/Library/LaunchDaemons/ru.supportgenesis.agent.plist' >/dev/null 2>&1 || true; rm -f '/Library/LaunchDaemons/ru.supportgenesis.agent.plist'; rm -rf '/Library/Application Support/GenesisIt'`
		if output, err := exec.Command("launchctl", "submit", "-l", label, "--", "/bin/sh", "-c", script).CombinedOutput(); err != nil {
			return fmt.Errorf("не удалось запланировать удаление macOS-агента: %w (%s)", err, string(output))
		}
		return nil
	default:
		return fmt.Errorf("удалённое удаление пока не поддерживается для %s", runtime.GOOS)
	}
}

func uninstallPlatform() error {
	if os.Geteuid() != 0 {
		return errors.New("удаление службы необходимо запустить с sudo")
	}
	if runtime.GOOS == "linux" {
		_ = exec.Command("systemctl", "disable", "--now", "genesisit-agent.service").Run()
		return os.Remove("/etc/systemd/system/genesisit-agent.service")
	}
	if runtime.GOOS == "darwin" {
		path := "/Library/LaunchDaemons/ru.supportgenesis.agent.plist"
		_ = exec.Command("launchctl", "bootout", "system", path).Run()
		return os.Remove(path)
	}
	return nil
}

func cleanupPlatformCommand() error {
	return errors.New("внутренний модуль очистки доступен только в Windows")
}

func scheduleAgentUpdate(updatePath, expectedVersion string) error {
	target, err := installedAgentPath()
	if err != nil {
		return err
	}
	if !allowedUnixAgentTarget(target) || os.Geteuid() != 0 {
		return errors.New("автообновление разрешено только для системной установки RemoteIt Agent")
	}
	// Unix permits replacing an executable while the old process is still
	// running. Installing the already verified binary before returning avoids
	// a systemd/launchd race where a detached helper is killed together with
	// the service cgroup before it can replace the old file.
	output, err := exec.Command(updatePath, "version").Output()
	if err != nil {
		return fmt.Errorf("не удалось проверить версию обновления: %w", err)
	}
	if strings.TrimSpace(string(output)) != strings.TrimSpace(expectedVersion) {
		return errors.New("версия загруженного обновления не совпала с ожидаемой")
	}
	if err := copyUnixFile(updatePath, target); err != nil {
		return fmt.Errorf("не удалось атомарно установить обновление: %w", err)
	}
	_ = os.Remove(updatePath)
	return nil
}

func updatePlatformCommand() error {
	flags := flag.NewFlagSet("update-helper", flag.ContinueOnError)
	target := flags.String("target", "", "установленный файл Agent")
	waitPID := flags.Int("wait-pid", 0, "PID предыдущей версии")
	expectedVersion := flags.String("expected-version", "", "ожидаемая версия")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	*target = filepath.Clean(strings.TrimSpace(*target))
	if os.Geteuid() != 0 || *target == "." || *waitPID <= 0 || *expectedVersion != version || !allowedUnixAgentTarget(*target) {
		return errors.New("параметры модуля обновления не прошли проверку")
	}
	if err := waitForUnixProcessExit(*waitPID, 60*time.Second); err != nil {
		return err
	}
	if err := installPlatform(); err != nil {
		return fmt.Errorf("установка обновления: %w", err)
	}
	self, _ := os.Executable()
	_ = os.Remove(self)
	return nil
}

func allowedUnixAgentTarget(target string) bool {
	switch runtime.GOOS {
	case "linux":
		return filepath.Clean(target) == filepath.Clean("/opt/genesisit/agent/genesis-agent")
	case "darwin":
		return filepath.Clean(target) == filepath.Clean("/Library/Application Support/GenesisIt/genesis-agent")
	default:
		return false
	}
}

func waitForUnixProcessExit(processID int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		err := syscall.Kill(processID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("предыдущая версия Agent не завершилась за 60 секунд")
}

func copyUnixFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	temporary := target + ".new"
	_ = os.Remove(temporary)
	out, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(temporary)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(temporary)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Chmod(temporary, 0o755); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return nil
	}
	defer directory.Close()
	return directory.Sync()
}
