//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const windowsServiceName = "GenesisItAgent"

type serviceHandler struct {
	run func(context.Context) error
}

func (handler *serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- handler.run(ctx) }()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(15 * time.Second):
				}
				return false, 0
			}
		case <-done:
			cancel()
			return false, 1
		}
	}
}

func runAsService(run func(context.Context) error) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false, err
	}
	return true, svc.Run(windowsServiceName, &serviceHandler{run: run})
}

func useUserConfig() bool {
	if requestedUserInstall || os.Getenv("GENESIS_USER_MODE") == "1" {
		return true
	}
	current, err := os.Executable()
	if err != nil {
		return false
	}
	root := os.Getenv("LocalAppData")
	if root == "" {
		return false
	}
	current = strings.ToLower(filepath.Clean(current))
	remoteRoot := strings.ToLower(filepath.Clean(filepath.Join(root, "Programs", "RemoteIt"))) + string(os.PathSeparator)
	legacyRoot := strings.ToLower(filepath.Clean(filepath.Join(root, "Programs", "GenesisIt"))) + string(os.PathSeparator)
	return strings.HasPrefix(current, remoteRoot) || strings.HasPrefix(current, legacyRoot)
}

func agentExecutionMode() (string, bool) {
	if useUserConfig() {
		return "user", false
	}
	return "system", true
}

// forceAgentUpdateCheckPlatform is the explicit fallback behind the Agent
// settings button. The normal path remains the 15-second signed heartbeat
// update; restarting only the RemoteIt background component forces that check
// immediately without changing network, SSH or device configuration.
func forceAgentUpdateCheckPlatform() error {
	target, err := installedAgentPath()
	if err != nil {
		return err
	}
	if useUserConfig() {
		if !allowedWindowsAgentTarget(target, true) {
			return errors.New("ручная проверка разрешена только для установленного RemoteIt Agent")
		}
		if err := stopWindowsUserProcesses(target); err != nil {
			return err
		}
		for _, argument := range []string{"run", "tray"} {
			command := exec.Command(target, argument)
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
			if err := command.Start(); err != nil {
				return fmt.Errorf("не удалось перезапустить пользовательский Agent: %w", err)
			}
			_ = command.Process.Release()
		}
		return nil
	}
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("не удалось открыть диспетчер служб: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(windowsServiceName)
	if err != nil {
		return fmt.Errorf("служба RemoteIt не найдена: %w", err)
	}
	defer service.Close()
	if err := stopWindowsService(service); err != nil {
		return err
	}
	if err := service.Start(); err != nil {
		return fmt.Errorf("не удалось запустить службу RemoteIt: %w", err)
	}
	return nil
}

func installPlatform() error {
	if useUserConfig() {
		return installWindowsUser()
	}
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	installDir := filepath.Join(programFiles, "RemoteIt", "Agent")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("для установки в Program Files запустите команду от имени администратора: %w", err)
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	target := filepath.Join(installDir, "RemoteIt-Agent.exe")
	legacyTarget := filepath.Join(programFiles, "GenesisIt", "Agent", "genesis-agent.exe")
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("не удалось открыть диспетчер служб: %w", err)
	}
	defer manager.Disconnect()
	service, openErr := manager.OpenService(windowsServiceName)
	if openErr != nil && !errors.Is(openErr, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return fmt.Errorf("не удалось проверить службу: %w", openErr)
	}
	serviceWasRunning := false
	serviceRestarted := false
	if service != nil {
		defer service.Close()
		if status, statusErr := service.Query(); statusErr == nil {
			serviceWasRunning = status.State != svc.Stopped
		}
		defer func() {
			if serviceWasRunning && !serviceRestarted {
				_ = service.Start()
			}
		}()
		if err := stopWindowsService(service); err != nil {
			return err
		}
	}
	// Stop every companion whose executable is inside the two explicitly
	// allowed RemoteIt install roots. This removes stale tray/desktop processes
	// from previous upgrades while never touching downloaded files elsewhere.
	stopWindowsProcessesInDirs([]string{installDir, filepath.Dir(legacyTarget)})
	if !samePath(current, target) {
		if err := stopWindowsUserProcesses(target); err != nil {
			if service != nil {
				_ = service.Start()
			}
			return err
		}
		if err := stopWindowsUserProcesses(legacyTarget); err != nil {
			return err
		}
		if err := copyFile(current, target); err != nil {
			if service != nil {
				_ = service.Start()
			}
			return err
		}
	}
	cleanupStaleAgentFiles(installDir, target)
	configDir := filepath.Dir(defaultConfigPath())
	if output, err := windowsHiddenCommand("icacls", configDir, "/inheritance:r", "/grant:r", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F", "*S-1-5-32-545:(RX)").CombinedOutput(); err != nil {
		return fmt.Errorf("не удалось защитить каталог агента: %w (%s)", err, string(output))
	}
	if err := protectPrivateFile(defaultConfigPath()); err != nil {
		return err
	}
	if output, err := windowsHiddenCommand("icacls", deviceNamePath(), "/inheritance:r", "/grant:r", "*S-1-5-18:F", "*S-1-5-32-544:F", "*S-1-5-32-545:R").CombinedOutput(); err != nil {
		return fmt.Errorf("не удалось защитить название устройства: %w (%s)", err, string(output))
	}
	if output, err := windowsHiddenCommand("icacls", publicInfoPath(), "/inheritance:r", "/grant:r", "*S-1-5-18:F", "*S-1-5-32-544:F", "*S-1-5-32-545:R").CombinedOutput(); err != nil {
		return fmt.Errorf("не удалось открыть публичный статус агента: %w (%s)", err, string(output))
	}
	if _, err := os.Stat(desktopAccessPath()); err == nil {
		if output, aclErr := windowsHiddenCommand("icacls", desktopAccessPath(), "/inheritance:r", "/grant:r", "*S-1-5-18:F", "*S-1-5-32-544:F", "*S-1-5-32-545:R").CombinedOutput(); aclErr != nil {
			return fmt.Errorf("не удалось открыть параметры удалённого экрана: %w (%s)", aclErr, string(output))
		}
	}
	_ = windowsHiddenCommand("reg", "delete", `HKLM\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "GenesisItAgentTray", "/f").Run()
	// The service broker owns the tray/capture companion. Leaving an HKLM Run
	// copy would start a medium-integrity process first and prevent control of
	// elevated windows through Windows UIPI.
	_ = windowsHiddenCommand("reg", "delete", `HKLM\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "RemoteItAgentTray", "/f").Run()
	_ = windowsHiddenCommand("reg", "delete", `HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\GenesisItAgent`, "/f").Run()
	if err := registerWindowsUninstall(false, target, installDir); err != nil {
		return err
	}

	if service != nil {
		if output, configErr := windowsHiddenCommand("sc.exe", "config", windowsServiceName, "binPath=", fmt.Sprintf(`"%s" run`, target), "DisplayName=", "RemoteIt Agent", "start=", "auto").CombinedOutput(); configErr != nil {
			return fmt.Errorf("не удалось обновить службу RemoteIt: %w (%s)", configErr, strings.TrimSpace(string(output)))
		}
		if err := service.Start(); err != nil {
			return fmt.Errorf("новая версия агента установлена, но служба не запустилась: %w", err)
		}
		serviceRestarted = true
		cleanupLegacyWindowsInstall(legacyTarget)
		return nil
	}
	service, err = manager.CreateService(windowsServiceName, target, mgr.Config{DisplayName: "RemoteIt Agent", Description: "Защищённый агент удалённого администрирования RemoteIt", StartType: mgr.StartAutomatic}, "run")
	if err != nil {
		return fmt.Errorf("не удалось создать службу: %w", err)
	}
	defer service.Close()
	if err := service.Start(); err != nil {
		return fmt.Errorf("служба создана, но не запущена: %w", err)
	}
	cleanupLegacyWindowsInstall(legacyTarget)
	return nil
}

func stopWindowsService(service *mgr.Service) error {
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("не удалось проверить состояние службы: %w", err)
	}
	if status.State == svc.Stopped {
		return nil
	}
	if status.State != svc.StopPending {
		if _, err := service.Control(svc.Stop); err != nil {
			return fmt.Errorf("не удалось остановить старую версию службы: %w", err)
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		status, err = service.Query()
		if err == nil && status.State == svc.Stopped {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("служба RemoteIt не остановилась за 20 секунд")
}

func installWindowsUser() error {
	root := os.Getenv("LocalAppData")
	if root == "" {
		return errors.New("переменная LocalAppData недоступна")
	}
	installDir := filepath.Join(root, "Programs", "RemoteIt", "Agent")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		return fmt.Errorf("не удалось создать пользовательский каталог RemoteIt: %w", err)
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	target := filepath.Join(installDir, "RemoteIt-Agent.exe")
	legacyTarget := filepath.Join(root, "Programs", "GenesisIt", "Agent", "genesis-agent.exe")
	if !samePath(current, target) {
		if err := stopWindowsUserProcesses(target); err != nil {
			return err
		}
		if err := stopWindowsUserProcesses(legacyTarget); err != nil {
			return err
		}
		if err := copyFile(current, target); err != nil {
			return err
		}
	}
	backgroundCommand := fmt.Sprintf(`"%s" run`, target)
	trayCommand := fmt.Sprintf(`"%s" tray`, target)
	for _, legacyName := range []string{"GenesisItAgentUser", "GenesisItAgentTray"} {
		_ = windowsHiddenCommand("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", legacyName, "/f").Run()
	}
	_ = windowsHiddenCommand("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\GenesisItAgent`, "/f").Run()
	for _, entry := range []struct{ name, command string }{
		{"RemoteItAgentUser", backgroundCommand},
		{"RemoteItAgentTray", trayCommand},
	} {
		if output, err := windowsHiddenCommand("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", entry.name, "/t", "REG_SZ", "/d", entry.command, "/f").CombinedOutput(); err != nil {
			return fmt.Errorf("не удалось включить автозапуск %s: %w (%s)", entry.name, err, strings.TrimSpace(string(output)))
		}
	}
	if err := registerWindowsUninstall(true, target, installDir); err != nil {
		return err
	}
	for _, argument := range []string{"run", "tray"} {
		command := exec.Command(target, argument)
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
		if err := command.Start(); err != nil {
			return fmt.Errorf("не удалось запустить пользовательский агент: %w", err)
		}
		_ = command.Process.Release()
	}
	cleanupLegacyWindowsInstall(legacyTarget)
	return nil
}

func cleanupLegacyWindowsInstall(legacyTarget string) {
	legacyAgentDir := filepath.Dir(legacyTarget)
	_ = os.RemoveAll(legacyAgentDir)
	_ = os.Remove(filepath.Dir(legacyAgentDir))
}

func cleanupStaleAgentFiles(installDir, activeTarget string) {
	entries, err := os.ReadDir(installDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(installDir, entry.Name())
		if samePath(path, activeTarget) {
			continue
		}
		name := strings.ToLower(entry.Name())
		if (strings.HasPrefix(name, "remoteit-agent") || strings.HasPrefix(name, "genesis-agent")) &&
			(strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".old") || strings.HasSuffix(name, ".new") || strings.HasSuffix(name, ".tmp")) {
			_ = os.Remove(path)
		}
	}
}

func stopWindowsUserProcesses(target string) error {
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return fmt.Errorf("не удалось получить список процессов: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == uint32(os.Getpid()) {
			continue
		}
		process, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, entry.ProcessID)
		if openErr != nil {
			continue
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		queryErr := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size)
		if queryErr == nil && strings.EqualFold(filepath.Clean(windows.UTF16ToString(buffer[:size])), filepath.Clean(target)) {
			_ = windows.TerminateProcess(process, 0)
		}
		windows.CloseHandle(process)
	}
	time.Sleep(400 * time.Millisecond)
	return nil
}

func registerWindowsUninstall(userMode bool, target, installDir string) error {
	root := `HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteItAgent`
	if userMode {
		root = `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteItAgent`
	}
	values := []struct {
		name, value, kind string
	}{
		{"DisplayName", "RemoteIt Agent", "REG_SZ"},
		{"DisplayVersion", version, "REG_SZ"},
		{"Publisher", "RemoteIt", "REG_SZ"},
		{"InstallLocation", installDir, "REG_SZ"},
		{"DisplayIcon", target + ",0", "REG_SZ"},
		{"UninstallString", fmt.Sprintf(`"%s" uninstall`, target), "REG_SZ"},
		{"QuietUninstallString", fmt.Sprintf(`"%s" uninstall`, target), "REG_SZ"},
		{"NoModify", "1", "REG_DWORD"},
		{"NoRepair", "1", "REG_DWORD"},
	}
	for _, value := range values {
		if output, err := windowsHiddenCommand("reg", "add", root, "/v", value.name, "/t", value.kind, "/d", value.value, "/f").CombinedOutput(); err != nil {
			return fmt.Errorf("не удалось зарегистрировать RemoteIt в списке установленных приложений: %w (%s)", err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

type windowsCleanupPayload struct {
	Target       string `json:"target"`
	UserMode     bool   `json:"userMode"`
	ServerURL    string `json:"serverUrl"`
	DeviceID     string `json:"deviceId"`
	DeviceSecret string `json:"deviceSecret"`
}

func scheduleRemoteUninstall() error {
	target, err := os.Executable()
	if err != nil {
		return err
	}
	target = filepath.Clean(target)
	installDir := filepath.Dir(target)
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	localAppData := os.Getenv("LocalAppData")
	allowedInstallDirs := []string{
		filepath.Join(programFiles, "RemoteIt", "Agent"),
		filepath.Join(localAppData, "Programs", "RemoteIt", "Agent"),
	}
	allowed := false
	for _, allowedDir := range allowedInstallDirs {
		if allowedDir != "" && samePath(installDir, allowedDir) {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("полное удаление разрешено только для установленной копии RemoteIt Agent")
	}

	helperFile, err := os.CreateTemp("", "RemoteIt-Cleanup-*.exe")
	if err != nil {
		return fmt.Errorf("не удалось подготовить модуль удаления RemoteIt: %w", err)
	}
	helperPath := helperFile.Name()
	if err := helperFile.Close(); err != nil {
		return err
	}
	_ = os.Remove(helperPath)
	if err := copyFile(target, helperPath); err != nil {
		return fmt.Errorf("не удалось подготовить модуль удаления RemoteIt: %w", err)
	}

	payloadFile, err := os.CreateTemp("", "RemoteIt-Cleanup-*.json")
	if err != nil {
		_ = os.Remove(helperPath)
		return err
	}
	payloadPath := payloadFile.Name()
	cfg, configErr := loadConfig()
	if configErr != nil || cfg.DeviceID == "" || cfg.DeviceSecret == "" {
		payloadFile.Close()
		_ = os.Remove(helperPath)
		_ = os.Remove(payloadPath)
		return errors.New("не удалось загрузить защищённые данные устройства для подтверждения удаления")
	}
	encodeErr := json.NewEncoder(payloadFile).Encode(windowsCleanupPayload{Target: target, UserMode: useUserConfig(), ServerURL: cfg.ServerURL, DeviceID: cfg.DeviceID, DeviceSecret: cfg.DeviceSecret})
	closeErr := payloadFile.Close()
	if encodeErr != nil || closeErr != nil {
		_ = os.Remove(helperPath)
		_ = os.Remove(payloadPath)
		if encodeErr != nil {
			return encodeErr
		}
		return closeErr
	}
	if err := protectPrivateFile(payloadPath); err != nil {
		_ = os.Remove(helperPath)
		_ = os.Remove(payloadPath)
		return err
	}

	command := exec.Command(helperPath, "cleanup", "--cleanup-file", payloadPath)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP}
	if err := command.Start(); err != nil {
		_ = os.Remove(helperPath)
		_ = os.Remove(payloadPath)
		return fmt.Errorf("не удалось запланировать удаление Windows-агента: %w", err)
	}
	return command.Process.Release()
}

func cleanupPlatformCommand() error {
	if len(os.Args) != 3 || os.Args[1] != "--cleanup-file" {
		return errors.New("некорректные параметры модуля удаления")
	}
	payloadPath := filepath.Clean(os.Args[2])
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		return err
	}
	var payload windowsCleanupPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.ServerURL == "" || payload.DeviceID == "" || payload.DeviceSecret == "" {
		return errors.New("неполные данные подтверждения удаления")
	}
	payload.Target = filepath.Clean(payload.Target)
	installDir := filepath.Dir(payload.Target)
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	localAppData := os.Getenv("LocalAppData")
	allowedDir := filepath.Join(programFiles, "RemoteIt", "Agent")
	legacyDir := filepath.Join(programFiles, "GenesisIt", "Agent")
	registryRoot := "HKLM"
	if payload.UserMode {
		if localAppData == "" {
			return errors.New("LocalAppData недоступна для удаления пользовательского агента")
		}
		allowedDir = filepath.Join(localAppData, "Programs", "RemoteIt", "Agent")
		legacyDir = filepath.Join(localAppData, "Programs", "GenesisIt", "Agent")
		registryRoot = "HKCU"
	}
	if !samePath(installDir, allowedDir) {
		return errors.New("отказ от удаления: путь установленного агента не прошёл проверку")
	}

	// The heartbeat first confirms the queued removal to the server. The helper
	// waits independently and only then stops the agent and removes its files.
	time.Sleep(18 * time.Second)
	if !payload.UserMode {
		runWindowsHidden("sc.exe", "stop", windowsServiceName)
		runWindowsHidden("sc.exe", "delete", windowsServiceName)
	}
	for _, value := range []string{"RemoteItAgentUser", "RemoteItAgentTray", "GenesisItAgentUser", "GenesisItAgentTray"} {
		runWindowsHidden("reg.exe", "delete", registryRoot+`\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", value, "/f")
	}
	for _, key := range []string{"RemoteItAgent", "GenesisItAgent"} {
		runWindowsHidden("reg.exe", "delete", registryRoot+`\Software\Microsoft\Windows\CurrentVersion\Uninstall\`+key, "/f")
	}
	stopWindowsProcessesInDirs([]string{installDir, legacyDir})

	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	dataRoot := filepath.Join(programData, "GenesisIt")
	if payload.UserMode {
		dataRoot = filepath.Join(localAppData, "GenesisIt")
	}
	for attempt := 0; attempt < 20; attempt++ {
		_ = os.RemoveAll(installDir)
		_ = os.RemoveAll(legacyDir)
		if _, statErr := os.Stat(installDir); errors.Is(statErr, os.ErrNotExist) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	_ = os.RemoveAll(dataRoot)
	_ = os.Remove(filepath.Dir(installDir))
	_ = os.Remove(filepath.Dir(legacyDir))
	_, installError := os.Stat(installDir)
	cleanupSuccess := errors.Is(installError, os.ErrNotExist)
	cleanupError := ""
	if !cleanupSuccess {
		cleanupError = "папка установленного Agent осталась на компьютере"
	}
	_ = reportWindowsCleanup(payload, cleanupSuccess, cleanupError)
	self, _ := os.Executable()
	scheduleWindowsDelete(self)
	scheduleWindowsDelete(payloadPath)
	return nil
}

func scheduleAgentUpdate(updatePath, expectedVersion string) error {
	target, err := installedAgentPath()
	if err != nil {
		return err
	}
	userMode := useUserConfig()
	if !allowedWindowsAgentTarget(target, userMode) {
		return errors.New("автообновление разрешено только для установленной копии RemoteIt Agent")
	}
	arguments := []string{"update-helper", "--target", target, "--wait-pid", fmt.Sprint(os.Getpid()), "--expected-version", expectedVersion}
	if userMode {
		arguments = append(arguments, "--user-mode")
	}
	command := exec.Command(updatePath, arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP}
	if err := command.Start(); err != nil {
		return fmt.Errorf("не удалось запустить модуль обновления: %w", err)
	}
	return command.Process.Release()
}

func updatePlatformCommand() error {
	flags := flag.NewFlagSet("update-helper", flag.ContinueOnError)
	target := flags.String("target", "", "установленный файл Agent")
	waitPID := flags.Int("wait-pid", 0, "PID предыдущей версии")
	expectedVersion := flags.String("expected-version", "", "ожидаемая версия")
	userMode := flags.Bool("user-mode", false, "пользовательская установка")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	*target = filepath.Clean(strings.TrimSpace(*target))
	if *target == "." || *waitPID <= 0 || *expectedVersion != version || !allowedWindowsAgentTarget(*target, *userMode) {
		return errors.New("параметры модуля обновления не прошли проверку")
	}
	if err := waitForWindowsProcessExit(uint32(*waitPID), 60*time.Second); err != nil {
		return err
	}
	requestedUserInstall = *userMode
	if err := installPlatform(); err != nil {
		return fmt.Errorf("установка обновления: %w", err)
	}
	self, _ := os.Executable()
	if !*userMode {
		// The service is upgraded in session 0. Restart every interactive companion
		// so the tray UI and Remote Control engine switch to this version immediately.
		if err := terminateInteractiveCompanions(*target); err != nil {
			log.Printf("Agent обновлён, но не все старые окна трея остановлены: %v", err)
		}
		log.Printf("RemoteIt Agent обновлён до %s; интерактивный Agent будет перезапущен службой", version)
	}
	scheduleWindowsDelete(self)
	return nil
}

func allowedWindowsAgentTarget(target string, userMode bool) bool {
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	expected := filepath.Join(programFiles, "RemoteIt", "Agent", "RemoteIt-Agent.exe")
	if userMode {
		localAppData := os.Getenv("LocalAppData")
		if localAppData == "" {
			return false
		}
		expected = filepath.Join(localAppData, "Programs", "RemoteIt", "Agent", "RemoteIt-Agent.exe")
	}
	return samePath(target, expected)
}

func waitForWindowsProcessExit(processID uint32, timeout time.Duration) error {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, processID)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("не удалось дождаться завершения предыдущей версии: %w", err)
	}
	defer windows.CloseHandle(process)
	result, err := windows.WaitForSingleObject(process, uint32(timeout/time.Millisecond))
	if err != nil {
		return err
	}
	if result == uint32(windows.WAIT_TIMEOUT) {
		return errors.New("предыдущая версия Agent не завершилась за 60 секунд")
	}
	return nil
}

func reportWindowsCleanup(payload windowsCleanupPayload, success bool, cleanupError string) error {
	data, err := json.Marshal(map[string]any{"success": success, "error": cleanupError})
	if err != nil {
		return err
	}
	var lastError error
	// The Agent has already been removed when this callback is sent, therefore
	// the temporary helper owns the retry window. Keep it alive long enough to
	// survive a short Wi-Fi reconnect or a server restart; otherwise the panel
	// could remain in "pending removal" even though the PC is already clean.
	for attempt := 0; attempt < 30; attempt++ {
		request, requestErr := http.NewRequest(http.MethodPost, strings.TrimRight(payload.ServerURL, "/")+"/api/agent/uninstall-complete", bytes.NewReader(data))
		if requestErr != nil {
			return requestErr
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Genesis-Device-Id", payload.DeviceID)
		request.Header.Set("Authorization", "Device "+payload.DeviceSecret)
		request.Header.Set("User-Agent", "RemoteIt-Cleanup/"+version)
		response, requestErr := (&http.Client{Timeout: 8 * time.Second}).Do(request)
		if requestErr == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				return nil
			}
			requestErr = fmt.Errorf("подтверждение удаления: HTTP %d", response.StatusCode)
		}
		lastError = requestErr
		if attempt < 29 {
			time.Sleep(4 * time.Second)
		}
	}
	return lastError
}

func runWindowsHidden(name string, arguments ...string) {
	_ = windowsHiddenCommand(name, arguments...).Run()
}

func windowsHiddenCommand(name string, arguments ...string) *exec.Cmd {
	command := exec.Command(name, arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return command
}

func backgroundCommandOutput(name string, arguments ...string) ([]byte, error) {
	return windowsHiddenCommand(name, arguments...).Output()
}

func prepareBackgroundCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

func stopWindowsProcessesInDirs(directories []string) {
	normalized := make([]string, 0, len(directories))
	for _, directory := range directories {
		if directory != "" {
			normalized = append(normalized, strings.ToLower(filepath.Clean(directory))+string(os.PathSeparator))
		}
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == uint32(os.Getpid()) {
			continue
		}
		process, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, entry.ProcessID)
		if openErr != nil {
			continue
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		if windows.QueryFullProcessImageName(process, 0, &buffer[0], &size) == nil {
			path := strings.ToLower(filepath.Clean(windows.UTF16ToString(buffer[:size])))
			for _, directory := range normalized {
				if strings.HasPrefix(path, directory) {
					_ = windows.TerminateProcess(process, 0)
					break
				}
			}
		}
		windows.CloseHandle(process)
	}
	time.Sleep(600 * time.Millisecond)
}

func scheduleWindowsDelete(path string) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err == nil {
		_ = windows.MoveFileEx(pointer, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
}

func uninstallPlatform() error {
	if !useUserConfig() {
		if _, err := loadConfig(); err != nil && !containsArgument("--elevated") {
			executable, executableErr := os.Executable()
			if executableErr != nil {
				return executableErr
			}
			return runElevatedAgentCommand(executable, "uninstall --elevated")
		}
	}
	return scheduleRemoteUninstall()
}

func containsArgument(expected string) bool {
	for _, argument := range os.Args[1:] {
		if argument == expected {
			return true
		}
	}
	return false
}

func copyFile(source, target string) error {
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
	if baseSize, bound := boundExecutableBaseSize(source); bound {
		_, err = io.CopyN(out, in, baseSize)
	} else {
		_, err = io.Copy(out, in)
	}
	if err != nil {
		out.Close()
		_ = os.Remove(temporary)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	sourcePointer, err := windows.UTF16PtrFromString(temporary)
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	targetPointer, err := windows.UTF16PtrFromString(target)
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := windows.MoveFileEx(sourcePointer, targetPointer, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("не удалось атомарно заменить агент: %w", err)
	}
	return nil
}

func samePath(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return filepath.Clean(a) == filepath.Clean(b)
}

var _ = errors.New
