//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const consoleURL = "https://supportgenesis.ru"
const consoleBuildVersion = "1.0.39"

func main() {
	installedPath, relaunch, installErr := ensureConsoleInstalled()
	if relaunch {
		arguments := append([]string{}, os.Args[1:]...)
		if err := exec.Command(installedPath, arguments...).Start(); err == nil {
			return
		}
	}
	if installedPath == "" {
		installedPath, _ = os.Executable()
	}
	if installErr == nil {
		removeOldConsoleVersions(installedPath)
	}
	_ = registerRemoteItProtocol(installedPath)
	if len(os.Args) > 1 && strings.HasPrefix(strings.ToLower(strings.TrimSpace(os.Args[1])), "remoteit://") {
		if err := runTunnelLaunch(os.Args[1]); err != nil {
			walk.MsgBox(nil, "RemoteIt — подключение", err.Error(), walk.MsgBoxIconError)
		}
		return
	}
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

func registerRemoteItProtocol(executable string) error {
	if strings.TrimSpace(executable) == "" {
		return errors.New("путь RemoteIt Console не определён")
	}
	root, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\remoteit`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	if err = root.SetStringValue("", "URL:RemoteIt secure tunnel"); err == nil {
		err = root.SetStringValue("URL Protocol", "")
	}
	root.Close()
	if err != nil {
		return err
	}
	command, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\remoteit\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer command.Close()
	return command.SetStringValue("", `"`+executable+`" "%1"`)
}

func ensureConsoleInstalled() (string, bool, error) {
	current, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	current, err = filepath.Abs(current)
	if err != nil {
		return "", false, err
	}
	localAppData := strings.TrimSpace(os.Getenv("LocalAppData"))
	if localAppData == "" {
		return current, false, errors.New("папка LocalAppData недоступна")
	}
	directory := filepath.Join(localAppData, "RemoteIt", "Console")
	target := filepath.Join(directory, "RemoteIt-Console-"+consoleBuildVersion+".exe")
	if strings.EqualFold(filepath.Clean(current), filepath.Clean(target)) {
		return target, false, nil
	}
	if err = os.MkdirAll(directory, 0o700); err != nil {
		return current, false, err
	}
	equal, compareErr := filesHaveSameSHA256(current, target)
	if compareErr == nil && equal {
		return target, true, nil
	}
	source, err := os.Open(current)
	if err != nil {
		return current, false, err
	}
	defer source.Close()
	temporary, err := os.CreateTemp(directory, ".RemoteIt-Console-*.tmp")
	if err != nil {
		return current, false, err
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err = io.Copy(temporary, source); err != nil {
		temporary.Close()
		return current, false, err
	}
	if err = temporary.Sync(); err != nil {
		temporary.Close()
		return current, false, err
	}
	if err = temporary.Close(); err != nil {
		return current, false, err
	}
	_ = os.Remove(target)
	if err = os.Rename(temporaryPath, target); err != nil {
		return current, false, err
	}
	cleanup = false
	return target, true, nil
}

func filesHaveSameSHA256(first, second string) (bool, error) {
	firstDigest, err := fileSHA256(first)
	if err != nil {
		return false, err
	}
	secondDigest, err := fileSHA256(second)
	if err != nil {
		return false, err
	}
	return firstDigest == secondDigest, nil
}

func fileSHA256(path string) ([sha256.Size]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return [sha256.Size]byte{}, err
	}
	var result [sha256.Size]byte
	copy(result[:], digest.Sum(nil))
	return result, nil
}

func removeOldConsoleVersions(current string) {
	directory := filepath.Dir(current)
	matches, err := filepath.Glob(filepath.Join(directory, "RemoteIt-Console-*.exe"))
	if err != nil {
		return
	}
	for _, candidate := range matches {
		if !strings.EqualFold(filepath.Clean(candidate), filepath.Clean(current)) {
			_ = os.Remove(candidate)
		}
	}
}

func runTunnelLaunch(raw string) error {
	request, err := parseRemoteItTunnelURL(raw)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("не удалось открыть локальный порт: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	endpoint, err := consoleTunnelEndpoint(request.ID)
	if err != nil {
		return err
	}
	headers := http.Header{"Authorization": []string{"Tunnel " + request.Token}}
	connection, response, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{HTTPClient: &http.Client{Timeout: 30 * time.Second}, HTTPHeader: headers, CompressionMode: websocket.CompressionDisabled})
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("сервер не принял туннель: %w", err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "Console tunnel ended")
	cleanup, err := launchNativeTunnelClient(request, port)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	if tcpListener, ok := listener.(*net.TCPListener); ok {
		_ = tcpListener.SetDeadline(time.Now().Add(3 * time.Minute))
	}
	local, err := listener.Accept()
	if err != nil {
		return fmt.Errorf("клиент %s не подключился к локальному туннелю: %w", strings.ToUpper(request.Protocol), err)
	}
	defer local.Close()
	remote := websocket.NetConn(ctx, connection, websocket.MessageBinary)
	defer remote.Close()
	completed := make(chan struct{}, 2)
	copySide := func(destination, source net.Conn) {
		_, _ = io.Copy(destination, source)
		completed <- struct{}{}
	}
	go copySide(local, remote)
	go copySide(remote, local)
	select {
	case <-completed:
		return nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("срок защищённого туннеля истёк")
		}
		return ctx.Err()
	}
}

func consoleTunnelEndpoint(id string) (string, error) {
	base, err := url.Parse(consoleURL)
	if err != nil || base.Scheme != "https" || !strings.EqualFold(base.Hostname(), "supportgenesis.ru") {
		return "", errors.New("адрес сервера RemoteIt не прошёл проверку")
	}
	base.Scheme = "wss"
	base.Path = "/api/network-tunnels/" + url.PathEscape(id) + "/client"
	return base.String(), nil
}

func launchNativeTunnelClient(request remoteItTunnelRequest, port int) (func(), error) {
	if request.Protocol == "rdp" {
		file, err := os.CreateTemp("", "remoteit-*.rdp")
		if err != nil {
			return nil, fmt.Errorf("не удалось подготовить RDP: %w", err)
		}
		path := file.Name()
		content := "full address:s:127.0.0.1:" + strconv.Itoa(port) + "\r\nprompt for credentials:i:1\r\nauthentication level:i:2\r\nenablecredsspsupport:i:1\r\nredirectclipboard:i:1\r\n"
		if request.Username != "" {
			content += "username:s:" + request.Username + "\r\n"
		}
		if _, err = file.WriteString(content); err != nil {
			file.Close()
			os.Remove(path)
			return nil, err
		}
		if err = file.Close(); err != nil {
			os.Remove(path)
			return nil, err
		}
		command := exec.Command("mstsc.exe", path)
		if err = command.Start(); err != nil {
			os.Remove(path)
			return nil, fmt.Errorf("не удалось запустить RDP: %w", err)
		}
		return func() { _ = os.Remove(path) }, nil
	}
	target := "127.0.0.1"
	if request.Username != "" {
		target = request.Username + "@127.0.0.1"
	}
	// Launch OpenSSH directly in its own interactive console. Avoid cmd.exe so
	// a supplied account name can never be interpreted as shell syntax.
	command := exec.Command("ssh.exe", "-p", strconv.Itoa(port), target)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NEW_CONSOLE}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("не удалось запустить SSH: %w", err)
	}
	return nil, nil
}
