//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Session switches on an RDS/VDI host replace the usable display generation
// immediately. Reconcile the bound user's companion quickly enough that a
// browser session can recover in-place instead of spending up to eight seconds
// attached to the worker from the previous WinStation.
const interactiveCompanionInterval = 2 * time.Second

var windowsSessionBindingMu sync.Mutex

// runInteractiveCompanionBroker keeps the visible tray and desktop-capture
// companion in exactly one SID-bound interactive Windows session. Windows services run
// in session 0, where screen capture and tray icons are not available. The
// broker therefore launches the same signed Agent executable in the logged-in
// user's session, including immediately after an automatic Agent upgrade.
func runInteractiveCompanionBroker(ctx context.Context) {
	if useUserConfig() {
		return
	}
	// SendSAS must run inside the actual SCM service. The interactive capture
	// worker requests it over a session-scoped kernel event and receives an
	// explicit success/failure acknowledgement.
	go runWindowsSASBroker(ctx)
	target, err := os.Executable()
	if err != nil {
		log.Printf("не удалось определить файл интерактивного RemoteIt Agent: %v", err)
		return
	}
	target = filepath.Clean(target)
	lastError := ""
	for {
		started, ensureErr := ensureInteractiveCompanions(target)
		workerStarted, workerErr := ensureInteractiveDesktopCompanions(target)
		if workerErr != nil {
			if ensureErr != nil {
				ensureErr = fmt.Errorf("%v; системный модуль экрана: %w", ensureErr, workerErr)
			} else {
				ensureErr = fmt.Errorf("системный модуль экрана: %w", workerErr)
			}
		}
		if ensureErr != nil {
			message := ensureErr.Error()
			if message != lastError {
				log.Printf("интерактивный RemoteIt Agent пока не запущен: %v", ensureErr)
				lastError = message
			}
		} else {
			lastError = ""
			for _, sessionID := range started {
				log.Printf("интерактивный RemoteIt Agent запущен в сеансе Windows %d", sessionID)
			}
			for _, sessionID := range workerStarted {
				log.Printf("системный модуль Remote Control запущен в сеансе Windows %d", sessionID)
			}
		}
		if !waitContext(ctx, interactiveCompanionInterval) {
			return
		}
	}
}

func ensureInteractiveCompanions(target string) ([]uint32, error) {
	sessions, err := activeWindowsSessions()
	if err != nil {
		// Fail closed on a legacy multi-user VDI. Old versions may have left one
		// tray process in every logged-on session; keeping those processes alive
		// would preserve the very cross-user race the SID binding prevents.
		cleanupErr := terminateWindowsAgentSessionsExcept(target, nil)
		return nil, errors.Join(err, cleanupErr)
	}
	if err := terminateWindowsAgentSessionsExcept(target, sessions); err != nil {
		return nil, err
	}
	running, err := windowsAgentSessions(target)
	if err != nil {
		return nil, err
	}
	started := make([]uint32, 0, len(sessions))
	var failures []string
	for _, sessionID := range sessions {
		if running[sessionID] {
			continue
		}
		if err := launchInteractiveCompanion(target, sessionID); err != nil {
			failures = append(failures, fmt.Sprintf("сеанс %d: %v", sessionID, err))
			continue
		}
		started = append(started, sessionID)
	}
	if len(failures) > 0 {
		return started, errors.New(strings.Join(failures, "; "))
	}
	return started, nil
}

func activeWindowsSessions() ([]uint32, error) {
	windowsSessionBindingMu.Lock()
	defer windowsSessionBindingMu.Unlock()
	candidates, err := windowsSessionCandidates()
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("привязка сеанса Windows: %w", err)
	}
	selection := selectWindowsSession(candidates, cfg.WindowsSessionUserSID)
	if selection.AutoBound && selection.UserSID != "" {
		cfg.WindowsSessionUserSID = selection.UserSID
		cfg.WindowsSessionUserName = selection.UserName
		if err := saveConfig(cfg); err != nil {
			return nil, fmt.Errorf("сохранение безопасной привязки Windows: %w", err)
		}
		_ = setupAgentUserFiles(cfg)
		appendPublicAgentEvent("success", "session", "Windows-сеанс закреплён", "Удалённый экран привязан к "+windowsSessionDisplayName(cfg))
		log.Printf("RemoteIt закрепил удалённый экран за Windows-пользователем %s (%s)", windowsSessionDisplayName(cfg), selection.UserSID)
	}
	if selection.SessionID == 0 {
		if selection.Ambiguous {
			return nil, errors.New("обнаружено несколько пользователей VDI: безопасная привязка не задана; повторно запустите установщик из нужного Windows-сеанса")
		}
		return nil, nil
	}
	return []uint32{selection.SessionID}, nil
}

func windowsSessionCandidates() ([]windowsSessionCandidate, error) {
	var first *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &first, &count); err != nil {
		return nil, fmt.Errorf("список сеансов Windows: %w", err)
	}
	if first == nil || count == 0 {
		return nil, nil
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(first)))
	items := unsafe.Slice(first, int(count))
	sessions := make([]windowsSessionCandidate, 0, len(items))
	for _, item := range items {
		if item.SessionID == 0 || (item.State != windows.WTSActive && item.State != windows.WTSConnected && item.State != windows.WTSDisconnected) {
			continue
		}
		var token windows.Token
		if err := windows.WTSQueryUserToken(item.SessionID, &token); err != nil {
			// The generic sign-in WinStation has no logged-on user token. It must
			// never become a fallback capture source for a user-bound device.
			continue
		}
		tokenUser, userErr := token.GetTokenUser()
		token.Close()
		if userErr != nil || tokenUser == nil || tokenUser.User.Sid == nil {
			continue
		}
		sid := normalizeWindowsUserSID(tokenUser.User.Sid.String())
		if sid == "" {
			continue
		}
		account, domain, _, lookupErr := tokenUser.User.Sid.LookupAccount("")
		name := strings.TrimSpace(account)
		if lookupErr == nil && strings.TrimSpace(domain) != "" && name != "" {
			name = strings.TrimSpace(domain) + `\` + name
		}
		state := windowsSessionStateConnected
		if item.State == windows.WTSActive {
			state = windowsSessionStateActive
		} else if item.State == windows.WTSDisconnected {
			state = windowsSessionStateDisconnected
		}
		sessions = append(sessions, windowsSessionCandidate{ID: item.SessionID, UserSID: sid, UserName: name, State: state})
	}
	return sessions, nil
}

func windowsAgentSessions(target string) (map[uint32]bool, error) {
	running := make(map[uint32]bool)
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("список процессов Windows: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		process, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if openErr != nil {
			continue
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		queryErr := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size)
		windows.CloseHandle(process)
		if queryErr != nil || !strings.EqualFold(filepath.Clean(windows.UTF16ToString(buffer[:size])), target) {
			continue
		}
		var sessionID uint32
		if windows.ProcessIdToSessionId(entry.ProcessID, &sessionID) == nil && sessionID != 0 {
			running[sessionID] = true
		}
	}
	if err != nil && !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("перебор процессов Windows: %w", err)
	}
	return running, nil
}

func terminateWindowsAgentSessionsExcept(target string, keepSessions []uint32) error {
	keep := make(map[uint32]bool, len(keepSessions))
	for _, sessionID := range keepSessions {
		keep[sessionID] = true
	}
	target = filepath.Clean(target)
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return fmt.Errorf("список процессов Windows: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var failures []string
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		process, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if openErr != nil {
			continue
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		queryErr := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size)
		windows.CloseHandle(process)
		if queryErr != nil || !strings.EqualFold(filepath.Clean(windows.UTF16ToString(buffer[:size])), target) {
			continue
		}
		var sessionID uint32
		if windows.ProcessIdToSessionId(entry.ProcessID, &sessionID) != nil || sessionID == 0 || keep[sessionID] {
			continue
		}
		process, openErr = windows.OpenProcess(windows.PROCESS_TERMINATE, false, entry.ProcessID)
		if openErr != nil {
			failures = append(failures, fmt.Sprintf("PID %d: %v", entry.ProcessID, openErr))
			continue
		}
		terminateErr := windows.TerminateProcess(process, 0)
		windows.CloseHandle(process)
		if terminateErr != nil {
			failures = append(failures, fmt.Sprintf("PID %d: %v", entry.ProcessID, terminateErr))
		}
	}
	if err != nil && !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func terminateInteractiveCompanions(target string) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return fmt.Errorf("список интерактивных процессов Agent: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	target = filepath.Clean(target)
	targets := map[string]bool{
		strings.ToLower(target):                    true,
		strings.ToLower(desktopWorkerPath(target)): true,
	}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var failures []string
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		process, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if openErr != nil {
			continue
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		queryErr := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size)
		windows.CloseHandle(process)
		processPath := strings.ToLower(filepath.Clean(windows.UTF16ToString(buffer[:size])))
		if queryErr != nil || !targets[processPath] {
			continue
		}
		var sessionID uint32
		if windows.ProcessIdToSessionId(entry.ProcessID, &sessionID) != nil || sessionID == 0 {
			continue
		}
		process, openErr = windows.OpenProcess(windows.PROCESS_TERMINATE, false, entry.ProcessID)
		if openErr != nil {
			failures = append(failures, fmt.Sprintf("PID %d: %v", entry.ProcessID, openErr))
			continue
		}
		terminateErr := windows.TerminateProcess(process, 0)
		windows.CloseHandle(process)
		if terminateErr != nil {
			failures = append(failures, fmt.Sprintf("PID %d: %v", entry.ProcessID, terminateErr))
		}
	}
	if err != nil && !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func launchInteractiveCompanion(target string, sessionID uint32) error {
	var token windows.Token
	if err := windows.WTSQueryUserToken(sessionID, &token); err != nil {
		return fmt.Errorf("токен пользователя: %w", err)
	}
	defer token.Close()

	var environment *uint16
	if err := windows.CreateEnvironmentBlock(&environment, token, false); err != nil {
		return fmt.Errorf("окружение пользователя: %w", err)
	}
	defer windows.DestroyEnvironmentBlock(environment)

	application, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	commandLine, err := windows.UTF16PtrFromString(fmt.Sprintf(`"%s" tray`, target))
	if err != nil {
		return err
	}
	desktop, _ := windows.UTF16PtrFromString(`winsta0\default`)
	workingDirectory, _ := windows.UTF16PtrFromString(filepath.Dir(target))
	startup := windows.StartupInfo{
		Cb:         uint32(unsafe.Sizeof(windows.StartupInfo{})),
		Desktop:    desktop,
		Flags:      windows.STARTF_USESHOWWINDOW,
		ShowWindow: windows.SW_HIDE,
	}
	var process windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NEW_PROCESS_GROUP)
	if err := windows.CreateProcessAsUser(token, application, commandLine, nil, nil, false, flags, environment, workingDirectory, &startup, &process); err != nil {
		return fmt.Errorf("запуск в пользовательском сеансе: %w", err)
	}
	windows.CloseHandle(process.Thread)
	windows.CloseHandle(process.Process)
	return nil
}

func desktopWorkerPath(target string) string {
	return filepath.Join(filepath.Dir(target), "RemoteIt-Desktop.exe")
}

func desktopWorkerVersionPath(target string) string {
	return desktopWorkerPath(target) + ".version"
}

func ensureInteractiveDesktopCompanions(target string) ([]uint32, error) {
	sessions, err := activeWindowsSessions()
	if err != nil {
		// The capture worker is privacy-sensitive. If the target SID is still
		// ambiguous, terminate every stale interactive worker rather than publish
		// a frame from whichever VDI user happened to become active last.
		cleanupErr := terminateWindowsAgentSessionsExcept(desktopWorkerPath(target), nil)
		return nil, errors.Join(err, cleanupErr)
	}
	worker := desktopWorkerPath(target)
	workerVersion, versionErr := os.ReadFile(desktopWorkerVersionPath(target))
	_, workerErr := os.Stat(worker)
	refreshWorker := errors.Is(workerErr, os.ErrNotExist) || versionErr != nil || strings.TrimSpace(string(workerVersion)) != version
	if workerErr != nil && !errors.Is(workerErr, os.ErrNotExist) {
		return nil, workerErr
	}
	if refreshWorker {
		// The desktop worker must run with service graphics authority inside only the
		// bound user's interactive session. Stop every old companion before replacing it so an automatic
		// Agent update cannot leave capture and input on the previous release.
		if err := terminateInteractiveCompanions(target); err != nil {
			return nil, fmt.Errorf("stop the previous RemoteIt desktop worker: %w", err)
		}
		if err := os.Remove(worker); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove the previous RemoteIt desktop worker: %w", err)
		}
	}
	if _, err := os.Stat(worker); errors.Is(err, os.ErrNotExist) {
		if err := copyFile(target, worker); err != nil {
			return nil, fmt.Errorf("подготовка системного модуля экрана: %w", err)
		}
	} else if err != nil {
		return nil, err
	}
	if refreshWorker {
		if err := os.WriteFile(desktopWorkerVersionPath(target), []byte(version+"\n"), 0o644); err != nil {
			return nil, fmt.Errorf("write the RemoteIt desktop worker version marker: %w", err)
		}
	}
	if err := terminateWindowsAgentSessionsExcept(worker, sessions); err != nil {
		return nil, err
	}
	running, err := windowsAgentSessions(worker)
	if err != nil {
		return nil, err
	}
	started := make([]uint32, 0, len(sessions))
	var failures []string
	for _, sessionID := range sessions {
		if running[sessionID] {
			continue
		}
		if err := launchInteractiveDesktopCompanion(worker, sessionID); err != nil {
			failures = append(failures, fmt.Sprintf("сеанс %d: %v", sessionID, err))
			continue
		}
		started = append(started, sessionID)
	}
	if len(failures) > 0 {
		return started, errors.New(strings.Join(failures, "; "))
	}
	return started, nil
}

func launchInteractiveDesktopCompanion(target string, sessionID uint32) error {
	var serviceToken windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE|windows.TOKEN_ADJUST_PRIVILEGES, &serviceToken); err != nil {
		return fmt.Errorf("токен службы: %w", err)
	}
	defer serviceToken.Close()
	// DXGI adapter enumeration on Windows Server RDS can return E_ACCESSDENIED
	// under the user's primary token even inside the correct WinStation. Keep the
	// service authority required by graphics drivers, then bind the duplicated
	// token to only the privacy-selected Windows session.
	if err := enableWindowsTokenPrivilege(serviceToken, "SeTcbPrivilege"); err != nil {
		return fmt.Errorf("привилегия системного модуля: %w", err)
	}
	var token windows.Token
	if err := windows.DuplicateTokenEx(serviceToken, windows.TOKEN_ALL_ACCESS, nil, windows.SecurityImpersonation, windows.TokenPrimary, &token); err != nil {
		return fmt.Errorf("интерактивный токен службы: %w", err)
	}
	defer token.Close()
	if err := windows.SetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&sessionID)), uint32(unsafe.Sizeof(sessionID))); err != nil {
		return fmt.Errorf("сеанс интерактивного токена: %w", err)
	}
	var environment *uint16
	if err := windows.CreateEnvironmentBlock(&environment, token, false); err != nil {
		return fmt.Errorf("окружение системного модуля: %w", err)
	}
	defer windows.DestroyEnvironmentBlock(environment)
	application, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	commandLine, err := windows.UTF16PtrFromString(fmt.Sprintf(`"%s" desktop-worker`, target))
	if err != nil {
		return err
	}
	desktop, _ := windows.UTF16PtrFromString(`winsta0\default`)
	workingDirectory, _ := windows.UTF16PtrFromString(filepath.Dir(target))
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})), Desktop: desktop, Flags: windows.STARTF_USESHOWWINDOW, ShowWindow: windows.SW_HIDE}
	var process windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP)
	if err := windows.CreateProcessAsUser(token, application, commandLine, nil, nil, false, flags, environment, workingDirectory, &startup, &process); err != nil {
		return fmt.Errorf("запуск системного модуля в интерактивном сеансе: %w", err)
	}
	windows.CloseHandle(process.Thread)
	windows.CloseHandle(process.Process)
	return nil
}

func enableWindowsTokenPrivilege(token windows.Token, name string) error {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, namePtr, &luid); err != nil {
		return err
	}
	state := windows.Tokenprivileges{PrivilegeCount: 1}
	state.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	return windows.AdjustTokenPrivileges(token, false, &state, 0, nil, nil)
}

func desktopWorkerCommand() error {
	setupLogging()
	done := make(chan struct{})
	runDesktopAgentLoop(done)
	return nil
}
