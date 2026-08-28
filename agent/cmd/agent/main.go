package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	version       = "1.0.12"
	defaultServer = "https://supportgenesis.ru"
)

var requestedUserInstall bool
var configFileMu sync.Mutex

type config struct {
	ServerURL              string   `json:"serverUrl"`
	EnrollmentToken        string   `json:"enrollmentToken,omitempty"`
	DeviceName             string   `json:"deviceName"`
	DeviceID               string   `json:"deviceId,omitempty"`
	DeviceSecret           string   `json:"deviceSecret,omitempty"`
	DesktopSecret          string   `json:"desktopSecret,omitempty"`
	ConnectionCode         string   `json:"connectionCode,omitempty"`
	ActionSigningPublicKey string   `json:"actionSigningPublicKey,omitempty"`
	ActionNonces           []string `json:"actionNonces,omitempty"`
	WindowsSessionUserSID  string   `json:"windowsSessionUserSid,omitempty"`
	WindowsSessionUserName string   `json:"windowsSessionUserName,omitempty"`
}

type inventory struct {
	Name            string   `json:"name"`
	Hostname        string   `json:"hostname"`
	OS              string   `json:"os"`
	OSVersion       string   `json:"osVersion"`
	Arch            string   `json:"arch"`
	AgentVersion    string   `json:"agentVersion"`
	LocalIPs        []string `json:"localIps"`
	CurrentUser     string   `json:"currentUser"`
	CPUModel        string   `json:"cpuModel"`
	CPULoadPercent  float64  `json:"cpuLoadPercent"`
	MemoryBytes     int64    `json:"memoryBytes"`
	MemoryUsedBytes int64    `json:"memoryUsedBytes"`
	DiskTotalBytes  int64    `json:"diskTotalBytes"`
	DiskFreeBytes   int64    `json:"diskFreeBytes"`
	UptimeSeconds   int64    `json:"uptimeSeconds"`
	InstallMode     string   `json:"installMode"`
	Privileged      bool     `json:"privileged"`
}

type apiClient struct {
	baseURL   string
	http      *http.Client
	transport *http.Transport
}

type remoteJob struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	Payload        map[string]any `json:"payload"`
	TimeoutSeconds int            `json:"timeoutSeconds"`
}

type heartbeatResponse struct {
	DesiredName            string       `json:"desiredName"`
	ConnectionCode         string       `json:"connectionCode"`
	DesktopSecret          string       `json:"desktopSecret"`
	NextHeartbeatSeconds   int          `json:"nextHeartbeatSeconds"`
	Job                    *remoteJob   `json:"job"`
	AgentUpdate            *agentUpdate `json:"agentUpdate"`
	ActionSigningPublicKey string       `json:"actionSigningPublicKey"`
}

type remoteJobResult struct {
	Success  bool   `json:"success"`
	Output   string `json:"output"`
	Error    string `json:"error"`
	ExitCode int    `json:"exitCode"`
}

type runtimeStatus struct {
	Running       bool      `json:"running"`
	Connected     bool      `json:"connected"`
	LastHeartbeat time.Time `json:"lastHeartbeat,omitempty"`
	LastError     string    `json:"lastError,omitempty"`
	PID           int       `json:"pid"`
}

func main() {
	if err := realMain(); err != nil {
		log.Printf("RemoteIt Agent: %v", err)
		os.Exit(1)
	}
}

func realMain() error {
	command := "run"
	if runtime.GOOS == "windows" && len(os.Args) == 1 {
		command = "setup"
	}
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		command = os.Args[1]
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	switch command {
	case "install":
		return installCommand()
	case "uninstall":
		return uninstallPlatform()
	case "cleanup":
		return cleanupPlatformCommand()
	case "update-helper":
		return updatePlatformCommand()
	case "force-update-check":
		return forceAgentUpdateCheckPlatform()
	case "status":
		return statusCommand()
	case "tray":
		return trayCommand()
	case "desktop-worker":
		return desktopWorkerCommand()
	case "setup":
		return setupCommand()
	case "run":
		return runCommand()
	case "version":
		fmt.Println(version)
		return nil
	default:
		return fmt.Errorf("неизвестная команда %q", command)
	}
}

func installCommand() (resultErr error) {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	token := flags.String("token", "", "токен регистрации RemoteIt")
	name := flags.String("name", "", "название компьютера")
	server := flags.String("server", defaultServer, "адрес сервера RemoteIt")
	userMode := flags.Bool("user-mode", false, "установить агент только для текущего пользователя")
	setupFile := flags.String("setup-file", "", "защищённый файл параметров GUI-установки")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	windowsSessionUserSID := ""
	windowsSessionUserName := ""
	if strings.TrimSpace(*setupFile) != "" {
		payload, err := loadSetupInstallPayload(*setupFile)
		if err != nil {
			return fmt.Errorf("не удалось прочитать параметры установки: %w", err)
		}
		defer func() {
			recordInstallResult(payload.ResultFile, resultErr)
			appendInstallDiagnostic(resultErr)
		}()
		*token = payload.Token
		*name = payload.Name
		*server = payload.ServerURL
		*userMode = payload.UserMode
		windowsSessionUserSID = payload.WindowsSessionUserSID
		windowsSessionUserName = payload.WindowsSessionUserName
	}
	requestedUserInstall = *userMode
	if runtime.GOOS == "windows" && strings.TrimSpace(windowsSessionUserSID) == "" {
		windowsSessionUserSID, windowsSessionUserName, _ = currentInstallSessionOwner()
	}
	if strings.TrimSpace(*token) == "" {
		return errors.New("не указан обязательный параметр --token")
	}
	if strings.TrimSpace(*name) == "" {
		hostname, _ := os.Hostname()
		fmt.Printf("Название компьютера [%s]: ", hostname)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		*name = strings.TrimSpace(line)
		if *name == "" {
			*name = hostname
		}
	}
	if len([]rune(strings.TrimSpace(*name))) > 64 {
		return errors.New("название компьютера не должно превышать 64 символа")
	}
	serverURL, err := normalizeServerURL(*server)
	if err != nil {
		return err
	}
	cfg := &config{
		ServerURL:              serverURL,
		EnrollmentToken:        *token,
		DeviceName:             strings.TrimSpace(*name),
		WindowsSessionUserSID:  normalizeWindowsUserSID(windowsSessionUserSID),
		WindowsSessionUserName: strings.TrimSpace(windowsSessionUserName),
	}
	reusedEnrollment := false
	if existing, loadErr := loadConfig(); loadErr == nil && existing.DeviceID != "" && existing.DeviceSecret != "" && strings.EqualFold(existing.ServerURL, serverURL) {
		// A normal upgrade keeps the Remote ID and the admin-controlled name. If
		// an older panel removed only its database row, however, the local Agent
		// may still hold orphaned credentials. Verify them before reusing them so
		// installing 0.6 over such an endpoint enrolls it again with this token.
		verifyContext, cancelVerify := context.WithTimeout(context.Background(), 20*time.Second)
		verifyErr := newAPIClient(serverURL).verifyRegistration(verifyContext, existing)
		cancelVerify()
		if verifyErr == nil {
			cfg = existing
			cfg.ServerURL = serverURL
			// A graphical reinstall is also the explicit way to rebind an
			// existing machine-wide Agent to the Windows user who launched the
			// installer. Preserve the Remote ID, but replace the VDI owner only
			// when the non-elevated setup process supplied a verified SID.
			if sid := normalizeWindowsUserSID(windowsSessionUserSID); sid != "" {
				cfg.WindowsSessionUserSID = sid
				cfg.WindowsSessionUserName = strings.TrimSpace(windowsSessionUserName)
			}
			reusedEnrollment = true
		} else {
			var statusError *apiStatusError
			if !errors.As(verifyErr, &statusError) || (statusError.StatusCode != http.StatusUnauthorized && statusError.StatusCode != http.StatusNotFound) {
				return fmt.Errorf("не удалось проверить существующую регистрацию перед обновлением: %w", verifyErr)
			}
		}
	}
	if !reusedEnrollment {
		client := newAPIClient(cfg.ServerURL)
		if err := client.enroll(context.Background(), cfg); err != nil {
			return fmt.Errorf("регистрация не выполнена: %w", err)
		}
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	if err := setupAgentUserFiles(cfg); err != nil {
		return err
	}
	if err := installPlatform(); err != nil {
		return err
	}
	fmt.Printf("RemoteIt Agent установлен. ID: %s\n", cfg.ConnectionCode)
	return nil
}

func runCommand() error {
	handled, err := runAsService(runLoop)
	if handled {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runLoop(ctx)
}

func runLoop(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("не удалось загрузить конфигурацию: %w", err)
	}
	setupLogging()
	writeRuntimeStatus(false, nil, true)
	defer writeRuntimeStatus(false, nil, false)
	client := newAPIClient(cfg.ServerURL)
	if cfg.DeviceID == "" || cfg.DeviceSecret == "" {
		if cfg.EnrollmentToken == "" {
			return errors.New("агент не зарегистрирован и токен отсутствует")
		}
		if err := client.enroll(ctx, cfg); err != nil {
			return err
		}
		if err := saveConfig(cfg); err != nil {
			return err
		}
	}
	if changed, bindErr := prepareWindowsSessionBinding(cfg); bindErr != nil {
		log.Printf("безопасная привязка Windows-сеанса пока не выполнена: %v", bindErr)
		appendPublicAgentEvent("warning", "session", "Windows-сеанс не закреплён", "Экран другого пользователя не будет опубликован; повторно запустите установщик из нужного Windows-сеанса")
	} else if changed {
		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("не удалось сохранить привязку Windows-сеанса: %w", err)
		}
		log.Printf("удалённый экран закреплён за Windows-пользователем %s (%s)", windowsSessionDisplayName(cfg), cfg.WindowsSessionUserSID)
		appendPublicAgentEvent("success", "session", "Windows-сеанс закреплён", "Удалённый экран привязан к "+windowsSessionDisplayName(cfg))
	}
	// Upgrades replace the service binary without re-running enrollment. Always
	// republish the secret-free user-facing files so the tray immediately shows
	// the current version and any existing Remote ID. Preserve an admin-renamed
	// device name that was already written to device-name.txt.
	cfg.DeviceName = effectiveDeviceName(cfg)
	if err := setupAgentUserFiles(cfg); err != nil {
		log.Printf("не удалось обновить общедоступное состояние Agent: %v", err)
	}

	log.Printf("агент запущен, Remote ID %s", cfg.ConnectionCode)
	appendPublicAgentEvent("success", "service", "Служба Agent запущена", "Фоновая служба готова к защищённому подключению")
	defer appendPublicAgentEvent("info", "service", "Служба Agent остановлена", "Agent корректно завершил текущий рабочий цикл")
	go runInteractiveCompanionBroker(ctx)
	go runFileTransferLoop(ctx, cfg)
	backoff := 5 * time.Second
	lastUpdateVersion := ""
	lastUpdateAttempt := time.Time{}
	connectionKnown := false
	connectionOnline := false
	for {
		networkBeforeHeartbeat := networkSignature()
		deviceName := effectiveDeviceName(cfg)
		inv := collectInventory(deviceName)
		response, err := client.heartbeat(ctx, cfg, inv)
		if err != nil {
			log.Printf("нет связи с сервером: %v", err)
			if !connectionKnown || connectionOnline {
				appendPublicAgentEvent("warning", "link", "Соединение с сервером потеряно", "Agent автоматически повторяет подключение; вмешательство пользователя не требуется")
			}
			connectionKnown = true
			connectionOnline = false
			writeRuntimeStatus(false, err, true)
			client.closeIdleConnections()
			keepRunning, networkChanged := waitForNetworkChange(ctx, backoff, networkBeforeHeartbeat)
			if !keepRunning {
				return nil
			}
			if networkChanged {
				log.Printf("обнаружено изменение сети, повторное подключение выполняется сразу")
				appendPublicAgentEvent("info", "network", "Изменение сети обнаружено", "IP, VPN или активный маршрут изменились; подключение выполняется заново")
				backoff = 5 * time.Second
				continue
			}
			if backoff < 2*time.Minute {
				backoff *= 2
				if backoff > 2*time.Minute {
					backoff = 2 * time.Minute
				}
			}
			continue
		}
		if !connectionKnown || !connectionOnline {
			appendPublicAgentEvent("success", "link", "Соединение с сервером установлено", "Защищённый канал supportgenesis.ru работает")
		}
		connectionKnown = true
		connectionOnline = true
		writeRuntimeStatus(true, nil, true)
		backoff = 5 * time.Second
		configurationChanged := false
		if response.DesiredName != "" && response.DesiredName != deviceName {
			if err := persistDeviceName(response.DesiredName); err != nil {
				log.Printf("не удалось применить новое название: %v", err)
			} else {
				cfg.DeviceName = response.DesiredName
				configurationChanged = true
				log.Printf("название устройства изменено на %q", response.DesiredName)
				appendPublicAgentEvent("info", "settings", "Название устройства обновлено", truncateText(response.DesiredName, 64))
			}
		}
		identityChanged, remoteIDChanged := applyHeartbeatIdentity(cfg, response)
		actionKeyChanged, actionKeyErr := applyActionSigningKey(cfg, response.ActionSigningPublicKey)
		if actionKeyErr != nil {
			log.Printf("проверка ключа Action Jobs отклонена: %v", actionKeyErr)
			appendPublicAgentEvent("warning", "security", "Ключ подписанных действий отклонён", "Agent сохранил ранее доверенный ключ и не принимает неподписанные изменения")
		}
		if remoteIDChanged {
			log.Printf("Remote ID восстановлен из подтверждённой регистрации")
			appendPublicAgentEvent("success", "identity", "Remote ID синхронизирован", "Сервер подтвердил идентификатор этого устройства")
		}
		configurationChanged = configurationChanged || identityChanged || actionKeyChanged
		if configurationChanged {
			if err := saveConfig(cfg); err != nil {
				log.Printf("не удалось сохранить обновлённую конфигурацию: %v", err)
			} else if err := setupAgentUserFiles(cfg); err != nil {
				log.Printf("не удалось опубликовать обновлённые параметры Agent: %v", err)
			}
		}
		if response.Job != nil {
			if response.Job.Type == "uninstall" {
				err := scheduleRemoteUninstall()
				result := remoteJobResult{Success: err == nil, Output: "Удаление агента запланировано", ExitCode: 0}
				if err != nil {
					result.Output = ""
					result.Error = err.Error()
					result.ExitCode = -1
				}
				if reportErr := reportJobResultWithRetry(ctx, client, cfg, response.Job.ID, result); reportErr != nil {
					log.Printf("не удалось отправить результат удаления %s: %v", response.Job.ID, reportErr)
				}
				if err == nil {
					return nil
				}
				continue
			}
			result := executeRemoteJob(ctx, cfg, response.Job)
			if err := reportJobResultWithRetry(ctx, client, cfg, response.Job.ID, result); err != nil {
				log.Printf("не удалось отправить результат задания %s: %v", response.Job.ID, err)
			}
			continue
		}
		if response.AgentUpdate != nil && (response.AgentUpdate.Version != lastUpdateVersion || time.Since(lastUpdateAttempt) >= 6*time.Hour) {
			lastUpdateVersion = response.AgentUpdate.Version
			lastUpdateAttempt = time.Now()
			log.Printf("доступно обновление RemoteIt Agent %s", response.AgentUpdate.Version)
			appendPublicAgentEvent("info", "update", "Обнаружено обновление Agent", "Проверяется версия "+truncateText(response.AgentUpdate.Version, 32))
			if err := downloadAndScheduleAgentUpdate(ctx, cfg, *response.AgentUpdate); err != nil {
				log.Printf("автоматическое обновление не выполнено: %v", err)
				appendPublicAgentEvent("warning", "update", "Обновление временно не применено", "Agent повторит безопасную проверку автоматически")
			} else {
				log.Printf("обновление %s проверено и запланировано", response.AgentUpdate.Version)
				appendPublicAgentEvent("success", "update", "Обновление проверено и запланировано", "Новая версия будет применена служебным механизмом")
				return nil
			}
		}
		interval := response.NextHeartbeatSeconds
		if interval < 15 || interval > 300 {
			interval = 30
		}
		jitter := rand.IntN(7) - 3
		keepRunning, networkChanged := waitForNetworkChange(ctx, time.Duration(interval+jitter)*time.Second, networkSignature())
		if !keepRunning {
			return nil
		}
		if networkChanged {
			client.closeIdleConnections()
			log.Printf("сетевые параметры изменились, обновляем IP и маршрут подключения")
			appendPublicAgentEvent("info", "network", "Сетевые параметры обновлены", "Agent перечитывает IP и маршрут после смены сети или VPN")
		}
	}
}

func applyHeartbeatIdentity(cfg *config, response heartbeatResponse) (changed, remoteIDChanged bool) {
	if cfg == nil {
		return false, false
	}
	connectionCode := strings.TrimSpace(response.ConnectionCode)
	if connectionCode != "" && connectionCode != cfg.ConnectionCode {
		cfg.ConnectionCode = connectionCode
		changed = true
		remoteIDChanged = true
	}
	if response.DesktopSecret != "" && response.DesktopSecret != cfg.DesktopSecret {
		cfg.DesktopSecret = response.DesktopSecret
		changed = true
	}
	return changed, remoteIDChanged
}

func reportJobResultWithRetry(ctx context.Context, client *apiClient, cfg *config, jobID string, result remoteJobResult) error {
	var lastError error
	for attempt := 0; attempt < 4; attempt++ {
		if err := client.reportJobResult(ctx, cfg, jobID, result); err == nil {
			return nil
		} else {
			lastError = err
		}
		if !waitContext(ctx, time.Duration(3*(1<<attempt))*time.Second) {
			return ctx.Err()
		}
	}
	return lastError
}

func statusCommand() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	output := map[string]any{"serverUrl": cfg.ServerURL, "deviceName": effectiveDeviceName(cfg), "deviceId": cfg.DeviceID, "connectionCode": cfg.ConnectionCode, "registered": cfg.DeviceID != "" && cfg.DeviceSecret != "", "version": version}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func newAPIClient(baseURL string) *apiClient {
	// Heartbeats deliberately do not reuse a TCP/TLS connection. A persistent
	// connection can keep the old VPN source address and route after TUN is
	// disabled or the VPN location changes. A fresh connection makes every
	// heartbeat reflect the currently active Windows/macOS/Linux route.
	dialer := &net.Dialer{Timeout: 6 * time.Second, KeepAlive: 15 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.DisableKeepAlives = true
	transport.TLSHandshakeTimeout = 6 * time.Second
	transport.ResponseHeaderTimeout = 8 * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.IdleConnTimeout = 5 * time.Second
	return &apiClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		http:      &http.Client{Timeout: 12 * time.Second, Transport: transport},
		transport: transport,
	}
}

func (c *apiClient) closeIdleConnections() {
	if c != nil && c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

func (c *apiClient) enroll(ctx context.Context, cfg *config) error {
	payload := collectInventory(cfg.DeviceName)
	body := map[string]any{"token": cfg.EnrollmentToken, "name": payload.Name, "hostname": payload.Hostname, "os": payload.OS, "osVersion": payload.OSVersion, "arch": payload.Arch, "agentVersion": payload.AgentVersion, "localIps": payload.LocalIPs, "installMode": payload.InstallMode, "privileged": payload.Privileged}
	var response struct {
		DeviceID       string `json:"deviceId"`
		DeviceSecret   string `json:"deviceSecret"`
		DesktopSecret  string `json:"desktopSecret"`
		ConnectionCode string `json:"connectionCode"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/agent/enroll", body, nil, &response); err != nil {
		return err
	}
	if response.DeviceID == "" || response.DeviceSecret == "" || response.ConnectionCode == "" {
		return errors.New("сервер вернул неполные регистрационные данные")
	}
	cfg.DeviceID = response.DeviceID
	cfg.DeviceSecret = response.DeviceSecret
	cfg.DesktopSecret = response.DesktopSecret
	cfg.ConnectionCode = response.ConnectionCode
	cfg.EnrollmentToken = ""
	return nil
}

func (c *apiClient) heartbeat(ctx context.Context, cfg *config, payload inventory) (heartbeatResponse, error) {
	headers := map[string]string{"X-Genesis-Device-Id": cfg.DeviceID, "Authorization": "Device " + cfg.DeviceSecret}
	var response heartbeatResponse
	err := c.request(ctx, http.MethodPost, "/api/agent/heartbeat", payload, headers, &response)
	return response, err
}

func (c *apiClient) verifyRegistration(ctx context.Context, cfg *config) error {
	headers := map[string]string{"X-Genesis-Device-Id": cfg.DeviceID, "Authorization": "Device " + cfg.DeviceSecret}
	return c.request(ctx, http.MethodPost, "/api/agent/verify", map[string]bool{"verify": true}, headers, nil)
}

func (c *apiClient) reportJobResult(ctx context.Context, cfg *config, jobID string, result remoteJobResult) error {
	headers := map[string]string{"X-Genesis-Device-Id": cfg.DeviceID, "Authorization": "Device " + cfg.DeviceSecret}
	return c.request(ctx, http.MethodPost, "/api/agent/jobs/"+jobID+"/result", result, headers, nil)
}

type cappedBuffer struct {
	buffer bytes.Buffer
	max    int
}

func (buffer *cappedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	remaining := buffer.max - buffer.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = buffer.buffer.Write(data)
	}
	return originalLength, nil
}

func (buffer *cappedBuffer) String() string {
	return buffer.buffer.String()
}

func executeRemoteJob(parent context.Context, cfg *config, job *remoteJob) remoteJobResult {
	timeout := job.TimeoutSeconds
	maximumTimeout := 60
	if job.Type == "action" {
		maximumTimeout = 900
	}
	if timeout < 5 || timeout > maximumTimeout {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeout)*time.Second)
	defer cancel()

	if job.Type == "inventory" {
		data, err := json.MarshalIndent(collectInventory(effectiveDeviceName(cfg)), "", "  ")
		if err != nil {
			return remoteJobResult{Success: false, Error: err.Error(), ExitCode: -1}
		}
		return remoteJobResult{Success: true, Output: string(data), ExitCode: 0}
	}
	if job.Type == "files_list" {
		return executeFilesListJob(job)
	}
	if job.Type == "files_read" {
		return executeFilesReadJob(job)
	}
	if job.Type == "files_write" {
		return executeFilesWriteJob(job)
	}
	if job.Type == "action" {
		return executeSignedActionJob(ctx, cfg, job)
	}
	if job.Type != "shell" {
		return remoteJobResult{Success: false, Error: "неподдерживаемый тип задания", ExitCode: -1}
	}
	command, ok := job.Payload["command"].(string)
	command = strings.TrimSpace(command)
	if !ok || command == "" || len([]rune(command)) > 8192 {
		return remoteJobResult{Success: false, Error: "сервер передал некорректную команду", ExitCode: -1}
	}

	var process *exec.Cmd
	if runtime.GOOS == "windows" {
		shell, _ := job.Payload["shell"].(string)
		if strings.EqualFold(strings.TrimSpace(shell), "cmd") {
			process = exec.CommandContext(ctx, "cmd.exe", "/D", "/S", "/C", command)
		} else {
			prefix := `[Console]::OutputEncoding=[System.Text.Encoding]::UTF8;$OutputEncoding=[System.Text.Encoding]::UTF8;`
			process = exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", prefix+command)
		}
	} else {
		shell, _ := job.Payload["shell"].(string)
		executable := "/bin/sh"
		if candidate := "/bin/" + strings.ToLower(strings.TrimSpace(shell)); shell == "bash" || shell == "zsh" {
			if _, err := os.Stat(candidate); err == nil {
				executable = candidate
			}
		}
		process = exec.CommandContext(ctx, executable, "-c", command)
	}
	prepareBackgroundCommand(process)
	output := &cappedBuffer{max: 128 * 1024}
	process.Stdout = output
	process.Stderr = output
	err := process.Run()
	exitCode := 0
	errorText := ""
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
		errorText = err.Error()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		errorText = fmt.Sprintf("команда превысила тайм-аут %d секунд", timeout)
		exitCode = -1
	}
	return remoteJobResult{Success: err == nil, Output: output.String(), Error: errorText, ExitCode: exitCode}
}

type remoteFileEntry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Directory  bool      `json:"directory"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

func executeFilesListJob(job *remoteJob) remoteJobResult {
	requestedPath, _ := job.Payload["path"].(string)
	requestedPath = strings.TrimSpace(requestedPath)
	if requestedPath == "" && runtime.GOOS == "windows" {
		entries := make([]remoteFileEntry, 0, 8)
		for letter := 'A'; letter <= 'Z'; letter++ {
			root := fmt.Sprintf("%c:\\", letter)
			if info, err := os.Stat(root); err == nil && info.IsDir() {
				entries = append(entries, remoteFileEntry{Name: root, Path: root, Directory: true, ModifiedAt: info.ModTime().UTC()})
			}
		}
		return fileJobJSON(map[string]any{"path": "", "parent": "", "entries": entries})
	}
	if requestedPath == "" {
		requestedPath = string(os.PathSeparator)
	}
	cleanPath := filepath.Clean(requestedPath)
	if !filepath.IsAbs(cleanPath) {
		return remoteJobResult{Success: false, Error: "сервер передал не абсолютный путь", ExitCode: -1}
	}
	items, err := os.ReadDir(cleanPath)
	if err != nil {
		return remoteJobResult{Success: false, Error: err.Error(), ExitCode: -1}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir() != items[j].IsDir() {
			return items[i].IsDir()
		}
		return strings.ToLower(items[i].Name()) < strings.ToLower(items[j].Name())
	})
	if len(items) > 500 {
		items = items[:500]
	}
	entries := make([]remoteFileEntry, 0, len(items))
	for _, item := range items {
		info, infoErr := item.Info()
		if infoErr != nil {
			continue
		}
		entries = append(entries, remoteFileEntry{
			Name: item.Name(), Path: filepath.Join(cleanPath, item.Name()), Directory: item.IsDir(),
			Size: info.Size(), ModifiedAt: info.ModTime().UTC(),
		})
	}
	parent := filepath.Dir(cleanPath)
	if parent == cleanPath {
		parent = ""
	}
	return fileJobJSON(map[string]any{"path": cleanPath, "parent": parent, "entries": entries})
}

func executeFilesReadJob(job *remoteJob) remoteJobResult {
	requestedPath, _ := job.Payload["path"].(string)
	cleanPath := filepath.Clean(strings.TrimSpace(requestedPath))
	if requestedPath == "" || !filepath.IsAbs(cleanPath) {
		return remoteJobResult{Success: false, Error: "сервер передал некорректный путь к файлу", ExitCode: -1}
	}
	// Follow the final symlink: standard system files such as /etc/os-release are
	// commonly links, while Stat still guarantees that the resolved target is a
	// regular file before we read it.
	info, err := os.Stat(cleanPath)
	if err != nil {
		return remoteJobResult{Success: false, Error: err.Error(), ExitCode: -1}
	}
	if !info.Mode().IsRegular() {
		return remoteJobResult{Success: false, Error: "разрешено скачивание только обычных файлов", ExitCode: -1}
	}
	const maxRemoteFileSize = 512 * 1024
	if info.Size() > maxRemoteFileSize {
		return remoteJobResult{Success: false, Error: "в этой версии размер скачиваемого файла ограничен 512 КБ", ExitCode: -1}
	}
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return remoteJobResult{Success: false, Error: err.Error(), ExitCode: -1}
	}
	digest := sha256.Sum256(data)
	return fileJobJSON(map[string]any{
		"name": filepath.Base(cleanPath), "path": cleanPath, "size": len(data),
		"modifiedAt": info.ModTime().UTC(), "sha256": fmt.Sprintf("%x", digest[:]),
		"dataBase64": base64.StdEncoding.EncodeToString(data),
	})
}

func executeFilesWriteJob(job *remoteJob) remoteJobResult {
	directory, _ := job.Payload["path"].(string)
	name, _ := job.Payload["name"].(string)
	encoded, _ := job.Payload["dataBase64"].(string)
	directory = filepath.Clean(strings.TrimSpace(directory))
	name = strings.TrimSpace(name)
	if !filepath.IsAbs(directory) || name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return remoteJobResult{Success: false, Error: "сервер передал некорректное место назначения", ExitCode: -1}
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("путь назначения не является папкой")
		}
		return remoteJobResult{Success: false, Error: err.Error(), ExitCode: -1}
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) > 512*1024 {
		return remoteJobResult{Success: false, Error: "файл повреждён или превышает 512 КБ", ExitCode: -1}
	}
	target := filepath.Join(directory, name)
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return remoteJobResult{Success: false, Error: "файл с таким именем уже существует", ExitCode: -1}
		}
		return remoteJobResult{Success: false, Error: err.Error(), ExitCode: -1}
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(target)
		return remoteJobResult{Success: false, Error: err.Error(), ExitCode: -1}
	}
	digest := sha256.Sum256(data)
	return fileJobJSON(map[string]any{"name": name, "path": target, "size": len(data), "sha256": fmt.Sprintf("%x", digest[:])})
}

func fileJobJSON(value any) remoteJobResult {
	data, err := json.Marshal(value)
	if err != nil {
		return remoteJobResult{Success: false, Error: err.Error(), ExitCode: -1}
	}
	return remoteJobResult{Success: true, Output: string(data), ExitCode: 0}
}

type apiStatusError struct {
	StatusCode int
	Message    string
}

func (err *apiStatusError) Error() string {
	if err.Message != "" {
		return "сервер: " + err.Message
	}
	return fmt.Sprintf("сервер вернул HTTP %d", err.StatusCode)
}

func (c *apiClient) request(ctx context.Context, method, path string, payload any, headers map[string]string, response any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "RemoteIt-Agent/"+version)
	req.Close = true
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var apiError struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &apiError)
		return &apiStatusError{StatusCode: resp.StatusCode, Message: apiError.Error}
	}
	if response != nil {
		return json.NewDecoder(resp.Body).Decode(response)
	}
	return nil
}

func collectInventory(name string) inventory {
	hostname, _ := os.Hostname()
	currentUser := ""
	if account, err := user.Current(); err == nil {
		currentUser = account.Username
	}
	osName, osVersion := platformVersion()
	memory, memoryUsed, diskTotal, diskFree, cpu, cpuLoad, uptime := platformMetrics()
	installMode, privileged := agentExecutionMode()
	return inventory{Name: name, Hostname: hostname, OS: osName, OSVersion: osVersion, Arch: runtime.GOARCH, AgentVersion: version, LocalIPs: localIPs(), CurrentUser: currentUser, CPUModel: cpu, CPULoadPercent: cpuLoad, MemoryBytes: memory, MemoryUsedBytes: memoryUsed, DiskTotalBytes: diskTotal, DiskFreeBytes: diskFree, UptimeSeconds: uptime, InstallMode: installMode, Privileged: privileged}
}

func platformVersion() (string, string) {
	switch runtime.GOOS {
	case "windows":
		output, _ := backgroundCommandOutput("cmd", "/c", "ver")
		return "Windows", strings.TrimSpace(string(output))
	case "darwin":
		product, _ := backgroundCommandOutput("sw_vers", "-productVersion")
		return "macOS", strings.TrimSpace(string(product))
	case "linux":
		data, err := os.ReadFile("/etc/os-release")
		if err != nil {
			return "Linux", ""
		}
		values := parseKeyValues(string(data))
		name := strings.Trim(values["NAME"], `"`)
		version := strings.Trim(values["VERSION"], `"`)
		if name == "" {
			name = "Linux"
		}
		return name, version
	default:
		return runtime.GOOS, ""
	}
}

func platformMetrics() (memory, memoryUsed, diskTotal, diskFree int64, cpu string, cpuLoad float64, uptime int64) {
	if runtime.GOOS == "linux" {
		var memoryAvailable int64
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "MemTotal:") || strings.HasPrefix(line, "MemAvailable:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						value, _ := strconv.ParseInt(fields[1], 10, 64)
						if strings.HasPrefix(line, "MemTotal:") {
							memory = value * 1024
						} else {
							memoryAvailable = value * 1024
						}
					}
				}
			}
		}
		memoryUsed = maxInt64(0, memory-memoryAvailable)
		if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(strings.ToLower(line), "model name") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						cpu = strings.TrimSpace(parts[1])
						break
					}
				}
			}
		}
		if data, err := os.ReadFile("/proc/uptime"); err == nil {
			fields := strings.Fields(string(data))
			if len(fields) > 0 {
				value, _ := strconv.ParseFloat(fields[0], 64)
				uptime = int64(value)
			}
		}
		cpuLoad = linuxCPULoad()
	} else if runtime.GOOS == "windows" {
		script := `$os=Get-CimInstance Win32_OperatingSystem;$cpus=Get-CimInstance Win32_Processor;$load=($cpus|Measure-Object LoadPercentage -Average).Average;$name=($cpus|Select-Object -First 1 -ExpandProperty Name);'{0}|{1}|{2}|{3}|{4}' -f $os.TotalVisibleMemorySize,$os.FreePhysicalMemory,$load,[int64]((Get-Date)-$os.LastBootUpTime).TotalSeconds,$name`
		if output, err := backgroundCommandOutput("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script); err == nil {
			parts := strings.SplitN(strings.TrimSpace(string(output)), "|", 5)
			if len(parts) == 5 {
				totalKB, _ := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
				freeKB, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
				memory = totalKB * 1024
				memoryUsed = maxInt64(0, (totalKB-freeKB)*1024)
				cpuLoad, _ = strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(parts[2]), ",", "."), 64)
				uptime, _ = strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
				cpu = strings.TrimSpace(parts[4])
			}
		}
	} else if runtime.GOOS == "darwin" {
		if output, err := backgroundCommandOutput("sysctl", "-n", "hw.memsize"); err == nil {
			memory, _ = strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
		}
		if output, err := backgroundCommandOutput("sysctl", "-n", "machdep.cpu.brand_string"); err == nil {
			cpu = strings.TrimSpace(string(output))
		}
		if output, err := backgroundCommandOutput("sh", "-c", "vm_stat | awk '/page size of/{gsub(/[^0-9]/,\"\",$8);p=$8}/Pages free|Pages inactive|Pages speculative/{gsub(/\\./,\"\",$3);a+=$3}END{print p*a}'"); err == nil {
			available, _ := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
			memoryUsed = maxInt64(0, memory-available)
		}
		if output, err := backgroundCommandOutput("sh", "-c", "ps -A -o %cpu= | awk '{s+=$1} END {print s}'"); err == nil {
			cpuLoad, _ = strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
		}
		if output, err := backgroundCommandOutput("sysctl", "-n", "kern.boottime"); err == nil {
			text := string(output)
			if marker := strings.Index(text, "sec ="); marker >= 0 {
				fields := strings.Fields(text[marker+5:])
				if len(fields) > 0 {
					boot, _ := strconv.ParseInt(strings.TrimRight(fields[0], ","), 10, 64)
					uptime = time.Now().Unix() - boot
				}
			}
		}
	}
	if cpuLoad < 0 {
		cpuLoad = 0
	}
	if cpuLoad > 100 {
		cpuLoad = 100
	}
	diskTotal, diskFree = diskSpace()
	return
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func linuxCPULoad() float64 {
	read := func() (idle, total int64, ok bool) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0, false
		}
		line := strings.SplitN(string(data), "\n", 2)[0]
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			return 0, 0, false
		}
		values := make([]int64, 0, len(fields)-1)
		for _, field := range fields[1:] {
			value, _ := strconv.ParseInt(field, 10, 64)
			values = append(values, value)
			total += value
		}
		idle = values[3]
		if len(values) > 4 {
			idle += values[4]
		}
		return idle, total, true
	}
	idle1, total1, ok := read()
	if !ok {
		return 0
	}
	time.Sleep(120 * time.Millisecond)
	idle2, total2, ok := read()
	if !ok || total2 <= total1 {
		return 0
	}
	return 100 * (1 - float64(idle2-idle1)/float64(total2-total1))
}

func localIPs() []string {
	interfaces, _ := net.Interfaces()
	unique := make(map[string]struct{}, 16)
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := networkInterface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
				continue
			}
			unique[ip.String()] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for ip := range unique {
		result = append(result, ip)
	}
	sort.Slice(result, func(left, right int) bool {
		leftIP, rightIP := net.ParseIP(result[left]), net.ParseIP(result[right])
		if leftIP.To4() != nil && rightIP.To4() == nil {
			return true
		}
		if leftIP.To4() == nil && rightIP.To4() != nil {
			return false
		}
		return result[left] < result[right]
	})
	if len(result) > 16 {
		result = result[:16]
	}
	return result
}

func networkSignature() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "unavailable"
	}
	parts := make([]string, 0, len(interfaces)*2)
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d:%s:%d:%d", networkInterface.Index, networkInterface.Name, networkInterface.MTU, networkInterface.Flags))
		if addresses, addressErr := networkInterface.Addrs(); addressErr == nil {
			for _, address := range addresses {
				parts = append(parts, fmt.Sprintf("%d=%s", networkInterface.Index, address.String()))
			}
		}
	}
	sort.Strings(parts)
	digest := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", digest[:8])
}

func parseKeyValues(input string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(input, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			values[key] = value
		}
	}
	return values
}

func defaultConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		if useUserConfig() {
			root := os.Getenv("LocalAppData")
			if root == "" {
				root = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
			}
			return filepath.Join(root, "GenesisIt", "config.json")
		}
		root := os.Getenv("ProgramData")
		if root == "" {
			root = `C:\ProgramData`
		}
		return filepath.Join(root, "GenesisIt", "config.json")
	case "darwin":
		return "/Library/Application Support/GenesisIt/config.json"
	default:
		return "/var/lib/genesisit/config.json"
	}
}

func loadConfig() (*config, error) {
	data, err := os.ReadFile(defaultConfigPath())
	if err != nil {
		return nil, err
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = defaultServer
	}
	normalized, err := normalizeServerURL(cfg.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("недопустимый адрес сервера в конфигурации: %w", err)
	}
	cfg.ServerURL = normalized
	return &cfg, nil
}

func normalizeServerURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", errors.New("сервер RemoteIt должен быть корректным HTTPS-адресом")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("адрес сервера не должен содержать логин, путь, параметры или фрагмент")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func saveConfig(cfg *config) error {
	configFileMu.Lock()
	defer configFileMu.Unlock()
	path := defaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("не удалось создать каталог конфигурации: %w", err)
	}
	// The service heartbeat and the Windows session broker keep independent
	// in-memory config snapshots. Once a legacy endpoint has been safely bound
	// to a Windows SID, an older heartbeat snapshot must never erase that privacy
	// boundary. A non-empty value (for example, an explicit reinstall by another
	// administrator) still replaces the previous binding.
	if runtime.GOOS == "windows" && normalizeWindowsUserSID(cfg.WindowsSessionUserSID) == "" {
		if existingData, readErr := os.ReadFile(path); readErr == nil {
			var existing config
			if json.Unmarshal(existingData, &existing) == nil {
				mergeWindowsSessionBinding(cfg, &existing)
			}
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := protectPrivateFile(temp); err != nil {
		file.Close()
		_ = os.Remove(temp)
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	return protectPrivateFile(path)
}

func setupLogging() {
	path := filepath.Join(filepath.Dir(defaultConfigPath()), "agent.log")
	if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, file))
	}
}

func runtimeStatusPath() string {
	return filepath.Join(filepath.Dir(defaultConfigPath()), "runtime-status.json")
}

func loadRuntimeStatus() (runtimeStatus, error) {
	data, err := os.ReadFile(runtimeStatusPath())
	if err != nil {
		return runtimeStatus{}, err
	}
	var status runtimeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return runtimeStatus{}, err
	}
	return status, nil
}

func writeRuntimeStatus(heartbeat bool, connectionError error, running bool) {
	status, _ := loadRuntimeStatus()
	status.Running = running
	status.Connected = heartbeat && connectionError == nil
	status.PID = os.Getpid()
	if heartbeat {
		status.LastHeartbeat = time.Now().UTC()
		status.LastError = ""
	} else if connectionError != nil {
		status.LastError = truncateText(connectionError.Error(), 300)
	}
	data, err := json.Marshal(status)
	if err != nil {
		return
	}
	path := runtimeStatusPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err == nil {
		if replaceAgentStatusFile(temporary, path) == nil {
			makeRuntimeStatusReadable(path)
		}
	}
	publishPublicRuntimeStatus(status)
}

func truncateText(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func waitForNetworkChange(ctx context.Context, duration time.Duration, baseline string) (bool, bool) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false, false
		case <-timer.C:
			return true, false
		case <-ticker.C:
			current := networkSignature()
			if current != baseline {
				return true, true
			}
		}
	}
}
