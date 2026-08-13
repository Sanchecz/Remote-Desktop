//go:build windows

package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
)

const boundInstallerMagic = "GENITB01"

type boundInstallerPayload struct {
	Token     string `json:"token"`
	ServerURL string `json:"serverUrl"`
}

type setupInstallPayload struct {
	Token      string `json:"token"`
	Name       string `json:"name"`
	ServerURL  string `json:"serverUrl"`
	UserMode   bool   `json:"userMode"`
	ResultFile string `json:"resultFile,omitempty"`
}

type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            windows.Handle
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          windows.Handle
	IDList                            unsafe.Pointer
	Class                             *uint16
	ClassKey                          windows.Handle
	HotKey                            uint32
	Icon                              windows.Handle
	Process                           windows.Handle
}

var shellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

func setupCommand() error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	prefilledToken := flags.String("token", "", "токен регистрации RemoteIt")
	prefilledName := flags.String("name", "", "название компьютера")
	serverURL := flags.String("server", defaultServer, "адрес сервера RemoteIt")
	quiet := flags.Bool("quiet", false, "quiet installation without the setup window")
	userMode := flags.Bool("user-mode", false, "install only for the current user")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}

	tokenEmbedded := false
	if executable, err := os.Executable(); err == nil {
		if payload, _, ok := readBoundInstaller(executable); ok {
			tokenEmbedded = true
			if strings.TrimSpace(*prefilledToken) == "" {
				*prefilledToken = payload.Token
			}
			if strings.TrimSpace(payload.ServerURL) != "" {
				*serverURL = payload.ServerURL
			}
		}
	}
	if strings.TrimSpace(*prefilledName) == "" {
		*prefilledName, _ = os.Hostname()
	}
	if *quiet {
		token := strings.TrimSpace(*prefilledToken)
		name := strings.TrimSpace(*prefilledName)
		if token == "" {
			return errors.New("для тихой установки требуется --token или Agent со встроенным токеном")
		}
		if name == "" || len([]rune(name)) > 64 {
			return errors.New("для тихой установки требуется корректное --name длиной до 64 символов")
		}
		server, normalizeErr := normalizeServerURL(*serverURL)
		if normalizeErr != nil {
			return normalizeErr
		}
		return runGraphicalInstall(setupInstallPayload{Token: token, Name: name, ServerURL: server, UserMode: *userMode})
	}

	window, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	defer window.Dispose()
	window.SetTitle("RemoteIt Agent — установка")
	if windowIcon, iconErr := walk.NewIconFromResourceId(1); iconErr == nil {
		defer windowIcon.Dispose()
		_ = window.SetIcon(windowIcon)
	}
	window.SetSize(walk.Size{Width: 560, Height: 510})
	window.SetMinMaxSize(walk.Size{Width: 540, Height: 485}, walk.Size{Width: 720, Height: 620})
	if background, brushErr := walk.NewSolidColorBrush(walk.RGB(244, 248, 247)); brushErr == nil {
		defer background.Dispose()
		window.SetBackground(background)
	}
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 32, VNear: 28, HFar: 32, VFar: 28})
	layout.SetSpacing(9)
	if err := window.SetLayout(layout); err != nil {
		return err
	}

	eyebrow, _ := walk.NewLabel(window)
	eyebrow.SetText("REMOTEIT  •  БЕЗОПАСНОЕ ПОДКЛЮЧЕНИЕ")
	eyebrow.SetTextColor(walk.RGB(29, 165, 112))
	if eyebrowFont, fontErr := walk.NewFont("Segoe UI", 9, walk.FontBold); fontErr == nil {
		defer eyebrowFont.Dispose()
		eyebrow.SetFont(eyebrowFont)
	}
	title, _ := walk.NewLabel(window)
	title.SetText("RemoteIt Agent")
	if font, fontErr := walk.NewFont("Segoe UI", 23, walk.FontBold); fontErr == nil {
		defer font.Dispose()
		title.SetFont(font)
	}
	description, _ := walk.NewLabel(window)
	description.SetText("Подключение компьютера к вашей панели удалённого администрирования")

	description.SetTextColor(walk.RGB(88, 105, 98))

	tokenLabel, _ := walk.NewLabel(window)
	tokenLabel.SetText("Токен подключения")
	tokenEdit, _ := walk.NewLineEdit(window)
	tokenEdit.SetMinMaxSize(walk.Size{Height: 32}, walk.Size{Height: 32})
	tokenEdit.SetPasswordMode(true)
	tokenEdit.SetText(strings.TrimSpace(*prefilledToken))
	if tokenEmbedded {
		tokenLabel.SetVisible(false)
		tokenEdit.SetVisible(false)
	}

	nameLabel, _ := walk.NewLabel(window)
	nameLabel.SetText("Название компьютера")
	nameEdit, _ := walk.NewLineEdit(window)
	nameEdit.SetMinMaxSize(walk.Size{Height: 32}, walk.Size{Height: 32})
	nameEdit.SetText(strings.TrimSpace(*prefilledName))

	serverLabel, _ := walk.NewLabel(window)
	serverLabel.SetText("Сервер")
	serverEdit, _ := walk.NewLineEdit(window)
	serverEdit.SetMinMaxSize(walk.Size{Height: 32}, walk.Size{Height: 32})
	serverEdit.SetText(strings.TrimSpace(*serverURL))
	serverEdit.SetReadOnly(true)

	allUsers, _ := walk.NewCheckBox(window)
	allUsers.SetText("Установить для всех пользователей с системными правами")
	allUsers.SetChecked(true)

	progress, _ := walk.NewProgressBar(window)
	progress.SetRange(0, 100)
	progress.SetValue(0)
	progress.SetVisible(false)

	statusLabel, _ := walk.NewLabel(window)
	statusLabel.SetMinMaxSize(walk.Size{Height: 34}, walk.Size{Height: 44})
	if tokenEmbedded {
		statusLabel.SetText("Доступ уже настроен. Проверьте название компьютера и нажмите «Установить».")
	} else {
		statusLabel.SetText("Вставьте токен из панели RemoteIt и укажите имя компьютера.")
	}

	installButton, _ := walk.NewPushButton(window)
	installButton.SetText("Установить и подключить")
	installButton.SetMinMaxSize(walk.Size{Height: 42}, walk.Size{Height: 42})
	completed := false
	installButton.Clicked().Attach(func() {
		if completed {
			window.Close()
			return
		}
		token := strings.TrimSpace(tokenEdit.Text())
		name := strings.TrimSpace(nameEdit.Text())
		server := strings.TrimSpace(serverEdit.Text())
		if token == "" {
			walk.MsgBox(window, "RemoteIt Agent", "Укажите токен подключения из панели RemoteIt.", walk.MsgBoxIconWarning)
			return
		}
		if name == "" || len([]rune(name)) > 64 {
			walk.MsgBox(window, "RemoteIt Agent", "Название компьютера должно содержать от 1 до 64 символов.", walk.MsgBoxIconWarning)
			return
		}
		if _, normalizeErr := normalizeServerURL(server); normalizeErr != nil {
			walk.MsgBox(window, "RemoteIt Agent", normalizeErr.Error(), walk.MsgBoxIconWarning)
			return
		}

		installButton.SetEnabled(false)
		tokenEdit.SetEnabled(false)
		nameEdit.SetEnabled(false)
		allUsers.SetEnabled(false)
		progress.SetVisible(true)
		progress.SetValue(20)
		statusLabel.SetText("Устанавливаем фоновую службу и подключаем компьютер…")
		installSystemWide := allUsers.Checked()
		go func() {
			installErr := runGraphicalInstall(setupInstallPayload{
				Token: token, Name: name, ServerURL: server, UserMode: !installSystemWide,
			})
			window.Synchronize(func() {
				if installErr != nil {
					progress.SetValue(0)
					progress.SetVisible(false)
					statusLabel.SetText("Не удалось установить агент.")
					installButton.SetEnabled(true)
					tokenEdit.SetEnabled(true)
					nameEdit.SetEnabled(true)
					allUsers.SetEnabled(true)
					walk.MsgBox(window, "Ошибка установки", installErr.Error(), walk.MsgBoxIconError)
					return
				}
				progress.SetValue(100)
				statusLabel.SetText("Готово. Компьютер подключён к RemoteIt.")
				completed = true
				installButton.SetText("Готово")
				installButton.SetEnabled(false)
				time.AfterFunc(1400*time.Millisecond, func() { window.Synchronize(func() { window.Close() }) })
			})
		}()
	})

	window.Show()
	window.Run()
	return nil
}

func runGraphicalInstall(payload setupInstallPayload) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	resultFile, err := os.CreateTemp("", "RemoteIt-Setup-Result-*.txt")
	if err != nil {
		return fmt.Errorf("не удалось подготовить диагностику установки: %w", err)
	}
	resultPath := resultFile.Name()
	if err = resultFile.Close(); err != nil {
		_ = os.Remove(resultPath)
		return fmt.Errorf("не удалось подготовить диагностику установки: %w", err)
	}
	defer os.Remove(resultPath)
	payload.ResultFile = resultPath
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	setupFile, err := os.CreateTemp("", "RemoteIt-Setup-*.json")
	if err != nil {
		return err
	}
	setupPath := setupFile.Name()
	if _, err = setupFile.Write(data); err != nil {
		setupFile.Close()
		_ = os.Remove(setupPath)
		return err
	}
	if err = setupFile.Close(); err != nil {
		_ = os.Remove(setupPath)
		return err
	}
	defer os.Remove(setupPath)

	if payload.UserMode {
		command := exec.Command(executable, "install", "--setup-file", setupPath)
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
		if output, runErr := command.CombinedOutput(); runErr != nil {
			if reason := readInstallResult(resultPath); reason != "" {
				return errors.New(reason)
			}
			return fmt.Errorf("установка для текущего пользователя не выполнена: %w (%s)", runErr, strings.TrimSpace(string(output)))
		}
		return nil
	}

	if runErr := runElevatedInstaller(executable, setupPath); runErr != nil {
		if reason := readInstallResult(resultPath); reason != "" {
			return errors.New(reason)
		}
		return fmt.Errorf("установка с правами администратора отменена или завершилась ошибкой: %w", runErr)
	}

	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	installed := filepath.Join(programFiles, "RemoteIt", "Agent", "RemoteIt-Agent.exe")
	tray := exec.Command(installed, "tray")
	tray.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	if err := tray.Start(); err == nil {
		_ = tray.Process.Release()
	}
	return nil
}

func readInstallResult(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	result := strings.TrimSpace(string(data))
	if result == "OK" {
		return ""
	}
	return result
}

func recordInstallResult(path string, installErr error) {
	if strings.TrimSpace(path) == "" {
		return
	}
	result := "OK"
	if installErr != nil {
		result = installErr.Error()
	}
	_ = os.WriteFile(path, []byte(result), 0o600)
}

func appendInstallDiagnostic(installErr error) {
	programData := strings.TrimSpace(os.Getenv("ProgramData"))
	if requestedUserInstall {
		programData = strings.TrimSpace(os.Getenv("LocalAppData"))
	}
	if programData == "" {
		return
	}
	directory := filepath.Join(programData, "RemoteIt", "Agent")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return
	}
	status := "успешно"
	if installErr != nil {
		status = "ошибка: " + installErr.Error()
	}
	line := fmt.Sprintf("%s RemoteIt Agent %s: %s\r\n", time.Now().Format(time.RFC3339), version, status)
	file, err := os.OpenFile(filepath.Join(directory, "install.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(line)
}

func runElevatedInstaller(executable, setupPath string) error {
	return runElevatedAgentCommand(executable, `install --setup-file "`+setupPath+`"`)
}

func runElevatedAgentCommand(executable, parametersValue string) error {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(executable)
	parameters, _ := windows.UTF16PtrFromString(parametersValue)
	directory, _ := windows.UTF16PtrFromString(filepath.Dir(executable))
	info := shellExecuteInfo{Mask: 0x00000040, Verb: verb, File: file, Parameters: parameters, Directory: directory, Show: 0}
	info.Size = uint32(unsafe.Sizeof(info))
	result, _, callErr := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		return callErr
	}
	if info.Process == 0 {
		return errors.New("установщик не вернул дескриптор процесса")
	}
	defer windows.CloseHandle(info.Process)
	if _, err := windows.WaitForSingleObject(info.Process, windows.INFINITE); err != nil {
		return err
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(info.Process, &exitCode); err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("код завершения %d", exitCode)
	}
	return nil
}

func readBoundInstaller(path string) (boundInstallerPayload, int64, bool) {
	file, err := os.Open(path)
	if err != nil {
		return boundInstallerPayload{}, 0, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() < 12 {
		return boundInstallerPayload{}, 0, false
	}
	if _, err = file.Seek(-12, io.SeekEnd); err != nil {
		return boundInstallerPayload{}, 0, false
	}
	tail := make([]byte, 12)
	if _, err = io.ReadFull(file, tail); err != nil || string(tail[:8]) != boundInstallerMagic {
		return boundInstallerPayload{}, 0, false
	}
	payloadLength := int64(binary.LittleEndian.Uint32(tail[8:]))
	if payloadLength < 2 || payloadLength > 4096 || info.Size() < payloadLength+12 {
		return boundInstallerPayload{}, 0, false
	}
	baseSize := info.Size() - payloadLength - 12
	if _, err = file.Seek(baseSize, io.SeekStart); err != nil {
		return boundInstallerPayload{}, 0, false
	}
	payloadData := make([]byte, payloadLength)
	if _, err = io.ReadFull(file, payloadData); err != nil {
		return boundInstallerPayload{}, 0, false
	}
	var payload boundInstallerPayload
	if json.Unmarshal(payloadData, &payload) != nil {
		return boundInstallerPayload{}, 0, false
	}
	payload.Token = strings.TrimSpace(payload.Token)
	payload.ServerURL = strings.TrimSpace(payload.ServerURL)
	if payload.Token == "" || len(payload.Token) > 300 {
		return boundInstallerPayload{}, 0, false
	}
	return payload, baseSize, true
}

func boundExecutableBaseSize(path string) (int64, bool) {
	_, size, ok := readBoundInstaller(path)
	return size, ok
}

func loadSetupInstallPayload(path string) (setupInstallPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return setupInstallPayload{}, err
	}
	_ = os.Remove(path)
	var payload setupInstallPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return setupInstallPayload{}, err
	}
	if strings.TrimSpace(payload.Token) == "" || strings.TrimSpace(payload.Name) == "" {
		return setupInstallPayload{}, errors.New("файл установки не содержит токен или название компьютера")
	}
	return payload, nil
}
