//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	windowsSASRequestKind = "Request"
	windowsSASSuccessKind = "Success"
	windowsSASFailureKind = "Failure"
)

var windowsSASPolicyMu sync.Mutex

// runWindowsSASBroker is executed by the real SCM service in session 0. The
// capture worker deliberately runs as a separate LocalSystem process in the
// interactive session, but Windows does not regard that child as an NT service
// for SendSAS authorization. Named kernel events keep the privileged boundary
// explicit: the worker requests SAS for its own session and the service performs
// the documented SendSAS call while impersonating that session's user token.
func runWindowsSASBroker(ctx context.Context) {
	// Software SAS is a machine policy, not a per-call capability. Keeping the
	// service bit enabled for the lifetime of the installed service matches the
	// Windows policy contract and avoids a race where Winlogon observes the old
	// value after a short-lived registry change has already been rolled back.
	// The ease-of-access bit, if configured by the administrator, is preserved.
	if err := ensureWindowsSoftwareSASPolicy(); err != nil {
		log.Printf("не удалось включить системную политику Ctrl+Alt+Delete: %v", err)
	}
	watchers := make(map[uint32]context.CancelFunc)
	syncSessions := func() {
		sessions, err := activeWindowsSessions()
		if err != nil {
			log.Printf("не удалось подготовить Ctrl+Alt+Delete: %v", err)
			return
		}
		active := make(map[uint32]bool, len(sessions))
		for _, sessionID := range sessions {
			active[sessionID] = true
			if _, exists := watchers[sessionID]; exists {
				continue
			}
			watcherCtx, cancel := context.WithCancel(ctx)
			watchers[sessionID] = cancel
			go serveWindowsSASRequests(watcherCtx, sessionID)
		}
		for sessionID, cancel := range watchers {
			if active[sessionID] {
				continue
			}
			cancel()
			delete(watchers, sessionID)
		}
	}

	syncSessions()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer func() {
		for _, cancel := range watchers {
			cancel()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncSessions()
		}
	}
}

func createWindowsSASEvent(kind string, sessionID uint32) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(windowsSASEventName(kind, sessionID))
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return 0, err
	}
	if handle == 0 {
		return 0, errors.New("Windows returned an empty SAS event handle")
	}
	return handle, nil
}

func openWindowsSASEvent(kind string, sessionID uint32) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(windowsSASEventName(kind, sessionID))
	if err != nil {
		return 0, err
	}
	return windows.OpenEvent(windows.EVENT_MODIFY_STATE|windows.SYNCHRONIZE, false, name)
}

func serveWindowsSASRequests(ctx context.Context, sessionID uint32) {
	request, err := createWindowsSASEvent(windowsSASRequestKind, sessionID)
	if err != nil {
		log.Printf("не удалось создать канал Ctrl+Alt+Delete для сеанса %d: %v", sessionID, err)
		return
	}
	defer windows.CloseHandle(request)
	success, err := createWindowsSASEvent(windowsSASSuccessKind, sessionID)
	if err != nil {
		log.Printf("не удалось создать подтверждение Ctrl+Alt+Delete для сеанса %d: %v", sessionID, err)
		return
	}
	defer windows.CloseHandle(success)
	failure, err := createWindowsSASEvent(windowsSASFailureKind, sessionID)
	if err != nil {
		log.Printf("не удалось создать ошибку Ctrl+Alt+Delete для сеанса %d: %v", sessionID, err)
		return
	}
	defer windows.CloseHandle(failure)

	for ctx.Err() == nil {
		result, waitErr := windows.WaitForSingleObject(request, 250)
		if waitErr != nil {
			log.Printf("ошибка канала Ctrl+Alt+Delete для сеанса %d: %v", sessionID, waitErr)
			return
		}
		if result != windows.WAIT_OBJECT_0 {
			continue
		}
		if sendErr := sendWindowsSASFromService(sessionID); sendErr != nil {
			log.Printf("Ctrl+Alt+Delete не выполнен в сеансе %d: %v", sessionID, sendErr)
			_ = windows.SetEvent(failure)
			continue
		}
		log.Printf("Ctrl+Alt+Delete передан в защищённый экран Windows, сеанс %d", sessionID)
		_ = windows.SetEvent(success)
	}
}

func ensureWindowsSoftwareSASPolicy() error {
	windowsSASPolicyMu.Lock()
	defer windowsSASPolicyMu.Unlock()

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, registry.QUERY_VALUE|registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return fmt.Errorf("open SoftwareSASGeneration policy: %w", err)
	}
	defer key.Close()

	current, _, readErr := key.GetIntegerValue("SoftwareSASGeneration")
	exists := readErr == nil
	if readErr != nil && !errors.Is(readErr, registry.ErrNotExist) {
		return fmt.Errorf("read SoftwareSASGeneration policy: %w", readErr)
	}
	target, changed, err := desiredSoftwareSASGeneration(current, exists)
	if err != nil {
		return err
	}
	if changed {
		if err := key.SetDWordValue("SoftwareSASGeneration", target); err != nil {
			return fmt.Errorf("allow service Secure Attention Sequence: %w", err)
		}
	}
	return nil
}

func sendWindowsSASFromService(sessionID uint32) error {
	if err := ensureWindowsSoftwareSASPolicy(); err != nil {
		return err
	}
	if err := sasDesktop.Load(); err != nil {
		return fmt.Errorf("Windows Secure Attention Sequence is unavailable: %w", err)
	}
	if err := procSendSAS.Find(); err != nil {
		return fmt.Errorf("Windows SendSAS export is unavailable: %w", err)
	}
	if !windowsSASShouldImpersonate(sessionID, windows.WTSGetActiveConsoleSessionId()) {
		// Microsoft documents FALSE for a genuine LocalSystem SCM service. This
		// is the only branch authorized to reach Winlogon on the physical console;
		// impersonating the logged-on user here can return without showing SAS.
		procSendSAS.Call(windowsSASAsUserArgument(false))
		return nil
	}

	// SendSAS does not accept a session ID. For a non-console RDS/VDI target,
	// impersonate that exact interactive token rather than affecting the console
	// or another user's desktop.
	var primary windows.Token
	if err := windows.WTSQueryUserToken(sessionID, &primary); err != nil {
		return fmt.Errorf("query user token for Windows session %d: %w", sessionID, err)
	}
	defer primary.Close()

	var impersonation windows.Token
	if err := windows.DuplicateTokenEx(primary, windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &impersonation); err != nil {
		return fmt.Errorf("duplicate user token for Windows session %d: %w", sessionID, err)
	}
	defer impersonation.Close()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.SetThreadToken(nil, impersonation); err != nil {
		return fmt.Errorf("impersonate Windows session %d: %w", sessionID, err)
	}
	defer windows.RevertToSelf()
	// The service thread now runs in the target interactive user's security
	// context. SendSAS has no return value, so selecting the documented AsUser
	// branch is essential: FALSE may return normally while addressing no visible
	// interactive session at all.
	procSendSAS.Call(windowsSASAsUserArgument(true))
	return nil
}

func requestWindowsServiceSAS() error {
	if useUserConfig() {
		return errors.New("Ctrl+Alt+Delete доступен только для системной установки RemoteIt Agent")
	}
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		return fmt.Errorf("определить сеанс Windows для Ctrl+Alt+Delete: %w", err)
	}
	if sessionID == 0 {
		return errors.New("Ctrl+Alt+Delete нельзя направить из служебного сеанса Windows 0")
	}

	deadline := time.Now().Add(2 * time.Second)
	var request, success, failure windows.Handle
	var err error
	for time.Now().Before(deadline) {
		request, err = openWindowsSASEvent(windowsSASRequestKind, sessionID)
		if err == nil {
			success, err = openWindowsSASEvent(windowsSASSuccessKind, sessionID)
		}
		if err == nil {
			failure, err = openWindowsSASEvent(windowsSASFailureKind, sessionID)
		}
		if err == nil {
			break
		}
		if request != 0 {
			windows.CloseHandle(request)
			request = 0
		}
		if success != 0 {
			windows.CloseHandle(success)
			success = 0
		}
		if failure != 0 {
			windows.CloseHandle(failure)
			failure = 0
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || request == 0 || success == 0 || failure == 0 {
		return fmt.Errorf("системная служба Ctrl+Alt+Delete не готова: %w", err)
	}
	defer windows.CloseHandle(request)
	defer windows.CloseHandle(success)
	defer windows.CloseHandle(failure)

	_ = windows.ResetEvent(success)
	_ = windows.ResetEvent(failure)
	if err := windows.SetEvent(request); err != nil {
		return fmt.Errorf("передать Ctrl+Alt+Delete системной службе: %w", err)
	}
	waitDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(waitDeadline) {
		if result, waitErr := windows.WaitForSingleObject(success, 0); waitErr != nil {
			return fmt.Errorf("проверить выполнение Ctrl+Alt+Delete: %w", waitErr)
		} else if result == windows.WAIT_OBJECT_0 {
			return nil
		}
		if result, waitErr := windows.WaitForSingleObject(failure, 0); waitErr != nil {
			return fmt.Errorf("проверить ошибку Ctrl+Alt+Delete: %w", waitErr)
		} else if result == windows.WAIT_OBJECT_0 {
			return errors.New("Windows отклонила Ctrl+Alt+Delete; проверьте журнал системной службы RemoteIt")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("системная служба RemoteIt не подтвердила Ctrl+Alt+Delete за 3 секунды")
}
