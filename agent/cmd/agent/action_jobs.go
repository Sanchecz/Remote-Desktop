package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const maxRememberedActionNonces = 128

var safeAgentServiceName = regexp.MustCompile(`^[A-Za-z0-9_. -]{1,128}$`)
var safeAgentPackageID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+:-]{0,127}$`)
var safeAgentPrincipal = regexp.MustCompile(`^[\p{L}\p{N}._@ -]{1,128}$`)
var safeAgentLANUsername = regexp.MustCompile(`^[\p{L}\p{N}._@\\ -]{1,128}$`)
var safeAgentWindowsPrincipal = regexp.MustCompile(`^[\p{L}\p{N}._@\\ -]{1,128}$`)
var safeAgentDownloadName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}._() -]{0,127}$`)
var agentSHA256Text = regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)
var safeAgentVPNName = regexp.MustCompile(`^[\p{L}\p{N}._ -]{1,64}$`)
var safeAgentDNSName = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+)$`)
var safeAgentLANHost = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*)$`)
var safeAgentPrinterName = regexp.MustCompile(`^[^\x00-\x1f]{1,256}$`)
var safeAgentWindowsPath = regexp.MustCompile(`(?i)^[a-z]:\\[^<>:"|?*\x00-\x1f]{1,220}$`)
var safeAgentShareName = regexp.MustCompile(`^[\p{L}\p{N}._ -]{1,64}$`)

const maxActionDownloadBytes int64 = 2 * 1024 * 1024 * 1024

type signedActionEnvelope struct {
	Version     int             `json:"version"`
	ActionJobID string          `json:"actionJobId"`
	DeviceID    string          `json:"deviceId"`
	Action      string          `json:"action"`
	Parameters  json.RawMessage `json:"parameters"`
	IssuedAt    int64           `json:"issuedAt"`
	ExpiresAt   int64           `json:"expiresAt"`
	Nonce       string          `json:"nonce"`
	RequestHash string          `json:"requestHash"`
}

func applyActionSigningKey(cfg *config, encoded string) (bool, error) {
	if cfg == nil {
		return false, errors.New("конфигурация Agent недоступна")
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return false, nil
	}
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return false, errors.New("сервер передал некорректный публичный ключ")
	}
	if cfg.ActionSigningPublicKey == "" {
		cfg.ActionSigningPublicKey = encoded
		return true, nil
	}
	if cfg.ActionSigningPublicKey != encoded {
		return false, errors.New("публичный ключ сервера изменился после первоначального закрепления")
	}
	return false, nil
}

func executeSignedActionJob(ctx context.Context, cfg *config, job *remoteJob) remoteJobResult {
	envelope, parameters, err := verifySignedActionJob(cfg, job)
	if err != nil {
		return failedAction(err.Error())
	}
	if actionNonceSeen(cfg, envelope.Nonce) {
		return failedAction("повторное выполнение подписанного действия заблокировано")
	}
	if err := rememberActionNonce(cfg, envelope.Nonce); err != nil {
		return failedAction("не удалось безопасно сохранить защиту от повторного выполнения: " + err.Error())
	}
	appendPublicAgentEvent("info", "action", "Получено подтверждённое действие", envelope.Action+" · "+envelope.ActionJobID)
	result := executeTypedAction(ctx, cfg, envelope.Action, parameters)
	if result.Success {
		appendPublicAgentEvent("success", "action", "Действие выполнено", envelope.Action+" · результат отправлен в панель")
	} else {
		appendPublicAgentEvent("warning", "action", "Действие не выполнено", truncateText(result.Error, 180))
	}
	return result
}

func verifySignedActionJob(cfg *config, job *remoteJob) (signedActionEnvelope, map[string]any, error) {
	var envelope signedActionEnvelope
	if cfg == nil || strings.TrimSpace(cfg.ActionSigningPublicKey) == "" {
		return envelope, nil, errors.New("публичный ключ подписанных действий ещё не закреплён")
	}
	if job == nil {
		return envelope, nil, errors.New("подписанное действие отсутствует")
	}
	signedEnvelope, _ := job.Payload["signedEnvelope"].(string)
	signatureText, _ := job.Payload["signature"].(string)
	payload, payloadErr := base64.RawStdEncoding.DecodeString(strings.TrimSpace(signedEnvelope))
	signature, signatureErr := base64.RawStdEncoding.DecodeString(strings.TrimSpace(signatureText))
	publicKey, keyErr := base64.RawStdEncoding.DecodeString(strings.TrimSpace(cfg.ActionSigningPublicKey))
	if payloadErr != nil || signatureErr != nil || keyErr != nil || len(signature) != ed25519.SignatureSize || len(publicKey) != ed25519.PublicKeySize {
		return envelope, nil, errors.New("подписанное действие имеет некорректный формат")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return envelope, nil, errors.New("подпись действия не прошла криптографическую проверку")
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return envelope, nil, errors.New("подписанный конверт действия повреждён")
	}
	parameters, err := validateSignedActionEnvelope(cfg, job, envelope)
	if err != nil {
		return envelope, nil, err
	}
	return envelope, parameters, nil
}

func validateSignedActionEnvelope(cfg *config, job *remoteJob, envelope signedActionEnvelope) (map[string]any, error) {
	now := time.Now()
	if envelope.Version != 1 || strings.TrimSpace(envelope.ActionJobID) == "" || envelope.ActionJobID != job.ID {
		return nil, errors.New("идентификатор подписанного действия не совпадает с заданием")
	}
	if envelope.DeviceID != cfg.DeviceID {
		return nil, errors.New("подписанное действие предназначено для другого устройства")
	}
	if envelope.IssuedAt <= 0 || envelope.ExpiresAt <= envelope.IssuedAt || envelope.ExpiresAt-envelope.IssuedAt > 15*60 {
		return nil, errors.New("срок действия подписанного задания некорректен")
	}
	if now.Unix() > envelope.ExpiresAt || envelope.IssuedAt > now.Add(90*time.Second).Unix() {
		return nil, errors.New("срок действия подписанного задания истёк или ещё не наступил")
	}
	if len(envelope.Nonce) < 24 || len(envelope.Nonce) > 128 {
		return nil, errors.New("одноразовый идентификатор действия некорректен")
	}
	parameters, err := decodeAndNormalizeActionParameters(envelope.Action, envelope.Parameters)
	if err != nil {
		return nil, err
	}
	if requestHash := agentActionRequestHash(envelope.DeviceID, envelope.Action, parameters); requestHash != envelope.RequestHash {
		return nil, errors.New("контрольная сумма параметров действия не совпадает")
	}
	return parameters, nil
}

func decodeAndNormalizeActionParameters(action string, raw json.RawMessage) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var input map[string]any
	if err := decoder.Decode(&input); err != nil {
		return nil, errors.New("параметры подписанного действия повреждены")
	}
	if input == nil {
		input = map[string]any{}
	}
	switch action {
	case "diagnostic.system", "diagnostic.network", "diagnostic.services", "windows.printers.list", "windows.printers.open_settings", "system.reboot":
		if len(input) != 0 {
			return nil, errors.New("диагностическое действие не принимает параметры")
		}
		return map[string]any{}, nil
	case "diagnostic.lan_scan", "windows.printers.discover":
		if len(input) > 1 {
			return nil, errors.New("параметры сканирования локальной сети недопустимы")
		}
		subnet, _ := input["subnet"].(string)
		subnet = strings.TrimSpace(subnet)
		if subnet != "" {
			ip, network, err := net.ParseCIDR(subnet)
			if err != nil || ip.To4() == nil || !ip.IsPrivate() {
				return nil, errors.New("сканировать можно только внутреннюю IPv4-подсеть")
			}
			ones, bits := network.Mask.Size()
			if bits != 32 || ones < 24 || ones > 32 {
				return nil, errors.New("за один запуск допускается не более 256 адресов")
			}
			subnet = network.String()
		}
		return map[string]any{"subnet": subnet}, nil
	case "diagnostic.tcp_probe":
		if len(input) != 2 {
			return nil, errors.New("для мониторинга требуются только host и ports")
		}
		host, _ := input["host"].(string)
		host = strings.TrimSpace(host)
		address := net.ParseIP(host)
		if address == nil || (!address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast()) {
			return nil, errors.New("мониторинг разрешён только для внутреннего IP-адреса")
		}
		rawPorts, ok := input["ports"].([]any)
		if !ok || len(rawPorts) < 1 || len(rawPorts) > 16 {
			return nil, errors.New("для мониторинга требуется от 1 до 16 портов")
		}
		ports := make([]int, 0, len(rawPorts))
		seen := make(map[int]bool)
		for _, rawPort := range rawPorts {
			value, err := agentIntegerParameter(rawPort)
			if err != nil || value < 1 || value > 65535 {
				return nil, errors.New("порт мониторинга недопустим")
			}
			if !seen[int(value)] {
				seen[int(value)] = true
				ports = append(ports, int(value))
			}
		}
		return map[string]any{"host": address.String(), "ports": ports}, nil
	case "windows.printer.open_web":
		if runtime.GOOS != "windows" || len(input) != 2 {
			return nil, errors.New("параметры веб-интерфейса принтера недопустимы")
		}
		host, _ := input["host"].(string)
		scheme, _ := input["scheme"].(string)
		host, scheme = strings.TrimSpace(host), strings.ToLower(strings.TrimSpace(scheme))
		address := net.ParseIP(host)
		if address == nil || (!address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast()) || !agentOneOf(scheme, "http", "https") {
			return nil, errors.New("адрес веб-интерфейса принтера не прошёл локальную проверку Agent")
		}
		return map[string]any{"host": address.String(), "scheme": scheme}, nil
	case "network.rdp.open", "network.ssh.open":
		if runtime.GOOS != "windows" || len(input) != 3 {
			return nil, errors.New("параметры внутреннего подключения недопустимы")
		}
		host, _ := input["host"].(string)
		username, _ := input["username"].(string)
		host, username = strings.TrimSpace(host), strings.TrimSpace(username)
		parsedHost := net.ParseIP(host)
		if parsedHost == nil && !safeAgentLANHost.MatchString(host) {
			return nil, errors.New("адрес внутреннего узла недопустим")
		}
		if parsedHost != nil && !parsedHost.IsPrivate() && !parsedHost.IsLoopback() && !parsedHost.IsLinkLocalUnicast() {
			return nil, errors.New("адрес внутреннего узла недопустим")
		}
		port, err := agentIntegerParameter(input["port"])
		if err != nil || port < 1 || port > 65535 {
			return nil, errors.New("порт внутреннего узла недопустим")
		}
		if username != "" && !safeAgentLANUsername.MatchString(username) {
			return nil, errors.New("имя пользователя внутреннего подключения недопустимо")
		}
		return map[string]any{"host": host, "port": port, "username": username}, nil
	case "windows.printer.set_default":
		if runtime.GOOS != "windows" || len(input) != 1 {
			return nil, errors.New("параметры принтера недопустимы")
		}
		name, _ := input["name"].(string)
		name = strings.TrimSpace(name)
		if !safeAgentPrinterName.MatchString(name) {
			return nil, errors.New("имя принтера недопустимо")
		}
		return map[string]any{"name": name}, nil
	case "windows.scan_folder.configure":
		if runtime.GOOS != "windows" || len(input) != 3 {
			return nil, errors.New("параметры папки сканов недопустимы")
		}
		path, _ := input["path"].(string)
		shareName, _ := input["shareName"].(string)
		principal, _ := input["principal"].(string)
		path, shareName, principal = strings.TrimSpace(path), strings.TrimSpace(shareName), strings.TrimSpace(principal)
		if !safeAgentWindowsPath.MatchString(path) || strings.Contains(path, `..`) || !safeAgentShareName.MatchString(shareName) || strings.HasSuffix(shareName, "$") || !safeAgentWindowsPrincipal.MatchString(principal) {
			return nil, errors.New("параметры папки сканов не прошли локальную проверку Agent")
		}
		return map[string]any{"path": path, "shareName": shareName, "principal": principal}, nil
	case "service.restart":
		name, _ := input["name"].(string)
		name = strings.TrimSpace(name)
		if !safeAgentServiceName.MatchString(name) || len(input) != 1 {
			return nil, errors.New("имя службы в подписанном действии недопустимо")
		}
		return map[string]any{"name": name}, nil
	case "process.terminate":
		if len(input) != 1 {
			return nil, errors.New("параметры завершения процесса недопустимы")
		}
		var pid int64
		switch value := input["pid"].(type) {
		case json.Number:
			pid, _ = value.Int64()
		case float64:
			pid = int64(value)
		case string:
			pid, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		}
		if pid <= 4 || pid > 1<<31-1 || int(pid) == os.Getpid() {
			return nil, errors.New("защищённый или некорректный PID нельзя завершить")
		}
		return map[string]any{"pid": pid}, nil
	case "file.download":
		if len(input) != 3 {
			return nil, errors.New("параметры скачивания файла недопустимы")
		}
		rawURL, _ := input["url"].(string)
		checksum, _ := input["sha256"].(string)
		fileName, _ := input["fileName"].(string)
		rawURL, checksum, fileName = strings.TrimSpace(rawURL), strings.ToLower(strings.TrimSpace(checksum)), strings.TrimSpace(fileName)
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
			return nil, errors.New("HTTPS-адрес файла в подписанном действии недопустим")
		}
		if !agentSHA256Text.MatchString(checksum) || !safeAgentDownloadName.MatchString(fileName) || filepath.Base(fileName) != fileName {
			return nil, errors.New("имя или SHA-256 файла в подписанном действии недопустимы")
		}
		return map[string]any{"url": parsed.String(), "sha256": checksum, "fileName": fileName}, nil
	case "package.install":
		if len(input) != 1 {
			return nil, errors.New("параметры установки пакета недопустимы")
		}
		packageID, _ := input["packageId"].(string)
		packageID = strings.TrimSpace(packageID)
		if !safeAgentPackageID.MatchString(packageID) {
			return nil, errors.New("ID пакета в подписанном действии недопустим")
		}
		return map[string]any{"packageId": packageID}, nil
	case "local.group.add_member":
		if len(input) != 2 {
			return nil, errors.New("параметры локальной группы недопустимы")
		}
		member, _ := input["member"].(string)
		group, _ := input["group"].(string)
		member, group = strings.TrimSpace(member), strings.TrimSpace(group)
		if !safeAgentPrincipal.MatchString(member) || !safeAgentPrincipal.MatchString(group) {
			return nil, errors.New("имя пользователя или группы в подписанном действии недопустимо")
		}
		return map[string]any{"member": member, "group": group}, nil
	case "windows.vpn.upsert":
		if runtime.GOOS != "windows" || len(input) != 4 {
			return nil, errors.New("параметры VPN-профиля недопустимы для этого устройства")
		}
		name, _ := input["name"].(string)
		serverAddress, _ := input["serverAddress"].(string)
		tunnelType, _ := input["tunnelType"].(string)
		authMethod, _ := input["authenticationMethod"].(string)
		name, serverAddress = strings.TrimSpace(name), strings.TrimSpace(serverAddress)
		if !safeAgentVPNName.MatchString(name) || (net.ParseIP(serverAddress) == nil && !safeAgentDNSName.MatchString(serverAddress)) || !agentOneOf(tunnelType, "Automatic", "Ikev2", "L2tp", "Pptp", "Sstp") || !agentOneOf(authMethod, "Eap", "Pap", "Chap", "MSChapv2") {
			return nil, errors.New("параметры VPN-профиля не прошли локальную проверку Agent")
		}
		return map[string]any{"name": name, "serverAddress": serverAddress, "tunnelType": tunnelType, "authenticationMethod": authMethod}, nil
	case "script.execute":
		if len(input) != 2 {
			return nil, errors.New("параметры сценария недопустимы")
		}
		shell, _ := input["shell"].(string)
		script, _ := input["script"].(string)
		shell = strings.ToLower(strings.TrimSpace(shell))
		if strings.TrimSpace(script) == "" || len(script) > 16*1024 || strings.ContainsRune(script, '\x00') {
			return nil, errors.New("текст сценария не прошёл локальную проверку Agent")
		}
		if runtime.GOOS == "windows" && !agentOneOf(shell, "powershell", "cmd") {
			return nil, errors.New("оболочка сценария недоступна в Windows")
		}
		if runtime.GOOS == "darwin" && !agentOneOf(shell, "bash", "sh", "zsh") {
			return nil, errors.New("оболочка сценария недоступна в macOS")
		}
		if runtime.GOOS != "windows" && runtime.GOOS != "darwin" && !agentOneOf(shell, "bash", "sh") {
			return nil, errors.New("оболочка сценария недоступна в Linux")
		}
		return map[string]any{"shell": shell, "script": script}, nil
	default:
		return nil, errors.New("тип подписанного действия не поддерживается этой версией Agent")
	}
}

func agentIntegerParameter(value any) (int64, error) {
	switch typed := value.(type) {
	case json.Number:
		return typed.Int64()
	case float64:
		if typed != float64(int64(typed)) {
			return 0, errors.New("not an integer")
		}
		return int64(typed), nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
	case int64:
		return typed, nil
	default:
		return 0, errors.New("not a number")
	}
}

func agentActionRequestHash(deviceID, action string, parameters map[string]any) string {
	payload, _ := json.Marshal(struct {
		DeviceID   string         `json:"deviceId"`
		Action     string         `json:"action"`
		Parameters map[string]any `json:"parameters"`
	}{deviceID, action, parameters})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func actionNonceSeen(cfg *config, nonce string) bool {
	for _, remembered := range cfg.ActionNonces {
		if remembered == nonce {
			return true
		}
	}
	return false
}

func rememberActionNonce(cfg *config, nonce string) error {
	cfg.ActionNonces = append(cfg.ActionNonces, nonce)
	if len(cfg.ActionNonces) > maxRememberedActionNonces {
		cfg.ActionNonces = append([]string(nil), cfg.ActionNonces[len(cfg.ActionNonces)-maxRememberedActionNonces:]...)
	}
	return saveConfig(cfg)
}

func executeTypedAction(ctx context.Context, cfg *config, action string, parameters map[string]any) remoteJobResult {
	switch action {
	case "diagnostic.system":
		data, err := json.MarshalIndent(collectInventory(effectiveDeviceName(cfg)), "", "  ")
		if err != nil {
			return failedAction(err.Error())
		}
		return remoteJobResult{Success: true, Output: string(data), ExitCode: 0}
	case "diagnostic.network":
		return executeNetworkDiagnostic(ctx)
	case "diagnostic.services":
		return executeServicesDiagnostic(ctx)
	case "diagnostic.lan_scan":
		subnet, _ := parameters["subnet"].(string)
		return executeLANScan(ctx, subnet)
	case "diagnostic.tcp_probe":
		host, _ := parameters["host"].(string)
		ports, _ := parameters["ports"].([]int)
		return executeTCPProbe(ctx, host, ports)
	case "windows.printers.discover":
		subnet, _ := parameters["subnet"].(string)
		return executeWindowsPrinterDiscover(ctx, subnet)
	case "network.rdp.open", "network.ssh.open":
		host, _ := parameters["host"].(string)
		username, _ := parameters["username"].(string)
		port, _ := parameters["port"].(int64)
		protocol := strings.TrimSuffix(strings.TrimPrefix(action, "network."), ".open")
		return executeInteractiveNetworkClient(ctx, protocol, host, int(port), username)
	case "windows.printers.list":
		return executeWindowsPrinterList(ctx)
	case "windows.printers.open_settings":
		return executeWindowsPrinterSettings(ctx)
	case "windows.printer.open_web":
		host, _ := parameters["host"].(string)
		scheme, _ := parameters["scheme"].(string)
		return executeWindowsPrinterWeb(ctx, host, scheme)
	case "windows.printer.set_default":
		name, _ := parameters["name"].(string)
		return executeWindowsPrinterSetDefault(ctx, name)
	case "windows.scan_folder.configure":
		path, _ := parameters["path"].(string)
		shareName, _ := parameters["shareName"].(string)
		principal, _ := parameters["principal"].(string)
		return executeWindowsScanFolderConfigure(ctx, path, shareName, principal)
	case "service.restart":
		name, _ := parameters["name"].(string)
		return executeServiceRestart(ctx, name)
	case "process.terminate":
		pid, _ := parameters["pid"].(int64)
		process, err := os.FindProcess(int(pid))
		if err != nil {
			return failedAction(err.Error())
		}
		if err := process.Kill(); err != nil {
			return failedAction(err.Error())
		}
		return remoteJobResult{Success: true, Output: fmt.Sprintf("Процесс PID %d завершён", pid), ExitCode: 0}
	case "file.download":
		return executeApprovedDownload(ctx, parameters)
	case "package.install":
		packageID, _ := parameters["packageId"].(string)
		return executePackageInstall(ctx, packageID)
	case "local.group.add_member":
		member, _ := parameters["member"].(string)
		group, _ := parameters["group"].(string)
		return executeLocalGroupAdd(ctx, member, group)
	case "windows.vpn.upsert":
		return executeWindowsVPNUpsert(ctx, parameters)
	case "system.reboot":
		return executeScheduledReboot(ctx)
	case "script.execute":
		shell, _ := parameters["shell"].(string)
		script, _ := parameters["script"].(string)
		return executeApprovedScript(ctx, shell, script)
	default:
		return failedAction("действие не поддерживается")
	}
}

func agentOneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func executeApprovedDownload(ctx context.Context, parameters map[string]any) remoteJobResult {
	rawURL, _ := parameters["url"].(string)
	expectedSHA, _ := parameters["sha256"].(string)
	fileName, _ := parameters["fileName"].(string)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || !agentSHA256Text.MatchString(expectedSHA) || !safeAgentDownloadName.MatchString(fileName) || filepath.Base(fileName) != fileName {
		return failedAction("параметры скачивания не прошли повторную проверку")
	}
	stagingDir := filepath.Join(filepath.Dir(defaultConfigPath()), "staging")
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return failedAction("не удалось создать staging-каталог: " + err.Error())
	}
	target := filepath.Join(stagingDir, fileName)
	if _, err := os.Stat(target); err == nil {
		return failedAction("файл с таким именем уже существует в staging-каталоге")
	} else if !errors.Is(err, os.ErrNotExist) {
		return failedAction("не удалось проверить staging-каталог: " + err.Error())
	}
	temporary, err := os.CreateTemp(stagingDir, ".remoteit-download-*.part")
	if err != nil {
		return failedAction("не удалось создать временный файл: " + err.Error())
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	_ = temporary.Chmod(0o600)
	client := &http.Client{
		Timeout: 0,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 || request.URL.Scheme != "https" || request.URL.Hostname() == "" || request.URL.User != nil {
				return errors.New("небезопасное перенаправление загрузки")
			}
			return nil
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		temporary.Close()
		return failedAction(err.Error())
	}
	request.Header.Set("User-Agent", "RemoteIt-Agent/"+version)
	response, err := client.Do(request)
	if err != nil {
		temporary.Close()
		return failedAction("HTTPS-загрузка не выполнена: " + err.Error())
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		temporary.Close()
		return failedAction(fmt.Sprintf("сервер загрузки вернул HTTP %d", response.StatusCode))
	}
	if response.ContentLength > maxActionDownloadBytes {
		temporary.Close()
		return failedAction("файл превышает лимит 2 ГБ для сценария скачивания")
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, maxActionDownloadBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil {
		return failedAction("загрузка файла прервана")
	}
	if written > maxActionDownloadBytes {
		return failedAction("файл превышает лимит 2 ГБ для сценария скачивания")
	}
	actualSHA := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actualSHA, expectedSHA) {
		return failedAction("SHA-256 скачанного файла не совпадает; файл удалён")
	}
	if err := os.Link(temporaryPath, target); err != nil {
		return failedAction("не удалось безопасно опубликовать проверенный файл: " + err.Error())
	}
	_ = protectPrivateFile(target)
	return remoteJobResult{Success: true, Output: fmt.Sprintf("Файл сохранён: %s\nРазмер: %d байт\nSHA-256: %s\nФайл не запускался.", target, written, actualSHA), ExitCode: 0}
}

func executePackageInstall(ctx context.Context, packageID string) remoteJobResult {
	if !safeAgentPackageID.MatchString(packageID) {
		return failedAction("ID пакета недопустим")
	}
	switch runtime.GOOS {
	case "windows":
		return runFixedActionCommand(ctx, "winget.exe", "install", "--id", packageID, "--exact", "--silent", "--disable-interactivity", "--accept-package-agreements", "--accept-source-agreements")
	case "darwin":
		for _, candidate := range []string{"brew", "/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
			if path, err := exec.LookPath(candidate); err == nil {
				return runFixedActionCommand(ctx, path, "install", packageID)
			}
		}
		return failedAction("Homebrew не найден; установка пакета на macOS не выполнена")
	default:
		for _, candidate := range []struct {
			name string
			args []string
		}{
			{"apt-get", []string{"install", "-y", "--no-install-recommends", packageID}},
			{"dnf", []string{"install", "-y", packageID}},
			{"yum", []string{"install", "-y", packageID}},
			{"zypper", []string{"--non-interactive", "install", packageID}},
		} {
			if path, err := exec.LookPath(candidate.name); err == nil {
				return runFixedActionCommand(ctx, path, candidate.args...)
			}
		}
		return failedAction("поддерживаемый системный менеджер пакетов не найден")
	}
}

func executeLocalGroupAdd(ctx context.Context, member, group string) remoteJobResult {
	if !safeAgentPrincipal.MatchString(member) || !safeAgentPrincipal.MatchString(group) {
		return failedAction("имя пользователя или группы недопустимо")
	}
	switch runtime.GOOS {
	case "windows":
		result := runFixedActionCommand(ctx, "net.exe", "localgroup", group, member, "/add")
		if !result.Success {
			return result
		}
		return runFixedActionCommand(ctx, "net.exe", "localgroup", group)
	case "darwin":
		result := runFixedActionCommand(ctx, "/usr/sbin/dseditgroup", "-o", "edit", "-a", member, "-t", "user", group)
		if !result.Success {
			return result
		}
		return runFixedActionCommand(ctx, "/usr/sbin/dseditgroup", "-o", "checkmember", "-m", member, group)
	default:
		result := runFixedActionCommand(ctx, "usermod", "-a", "-G", group, member)
		if !result.Success {
			return result
		}
		return runFixedActionCommand(ctx, "id", member)
	}
}

func executeWindowsVPNUpsert(ctx context.Context, parameters map[string]any) remoteJobResult {
	if runtime.GOOS != "windows" {
		return failedAction("встроенный VPN-профиль поддерживается только Windows")
	}
	name, _ := parameters["name"].(string)
	serverAddress, _ := parameters["serverAddress"].(string)
	tunnelType, _ := parameters["tunnelType"].(string)
	authMethod, _ := parameters["authenticationMethod"].(string)
	if !safeAgentVPNName.MatchString(name) || (net.ParseIP(serverAddress) == nil && !safeAgentDNSName.MatchString(serverAddress)) || !agentOneOf(tunnelType, "Automatic", "Ikev2", "L2tp", "Pptp", "Sstp") || !agentOneOf(authMethod, "Eap", "Pap", "Chap", "MSChapv2") {
		return failedAction("параметры VPN-профиля недопустимы")
	}
	const fixedScript = `$ErrorActionPreference='Stop'; & { param($n,$s,$t,$a) $existing=Get-VpnConnection -Name $n -AllUserConnection -ErrorAction SilentlyContinue; if ($null -eq $existing) { Add-VpnConnection -Name $n -ServerAddress $s -TunnelType $t -AuthenticationMethod $a -EncryptionLevel Required -AllUserConnection -RememberCredential:$false -Force } else { Set-VpnConnection -Name $n -ServerAddress $s -TunnelType $t -AuthenticationMethod $a -EncryptionLevel Required -AllUserConnection -RememberCredential:$false -Force }; Get-VpnConnection -Name $n -AllUserConnection | Select-Object Name,ServerAddress,TunnelType,AuthenticationMethod,EncryptionLevel | Format-List }`
	return runFixedActionCommand(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", fixedScript, name, serverAddress, tunnelType, authMethod)
}

func executeScheduledReboot(ctx context.Context) remoteJobResult {
	switch runtime.GOOS {
	case "windows":
		return runFixedActionCommand(ctx, "shutdown.exe", "/r", "/t", "60", "/d", "p:4:1", "/c", "RemoteIt: подтверждённая перезагрузка")
	case "darwin":
		return runFixedActionCommand(ctx, "/sbin/shutdown", "-r", "+1", "RemoteIt approved reboot")
	default:
		return runFixedActionCommand(ctx, "shutdown", "-r", "+1", "RemoteIt approved reboot")
	}
}

func executeApprovedScript(ctx context.Context, shell, script string) remoteJobResult {
	if strings.TrimSpace(script) == "" || len(script) > 16*1024 || strings.ContainsRune(script, '\x00') {
		return failedAction("сценарий не прошёл повторную проверку")
	}
	var executable, extension string
	var args []string
	switch runtime.GOOS {
	case "windows":
		switch shell {
		case "powershell":
			executable, extension = "powershell.exe", ".ps1"
			args = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File"}
		case "cmd":
			executable, extension = "cmd.exe", ".cmd"
			args = []string{"/D", "/Q", "/C"}
		default:
			return failedAction("оболочка сценария недоступна в Windows")
		}
	case "darwin":
		if !agentOneOf(shell, "bash", "sh", "zsh") {
			return failedAction("оболочка сценария недоступна в macOS")
		}
		path, err := exec.LookPath(shell)
		if err != nil {
			return failedAction("оболочка сценария не найдена: " + shell)
		}
		executable, extension = path, ".sh"
	default:
		if !agentOneOf(shell, "bash", "sh") {
			return failedAction("оболочка сценария недоступна в Linux")
		}
		path, err := exec.LookPath(shell)
		if err != nil {
			return failedAction("оболочка сценария не найдена: " + shell)
		}
		executable, extension = path, ".sh"
	}
	directory := filepath.Join(filepath.Dir(defaultConfigPath()), "action-scripts")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return failedAction("не удалось создать защищённый каталог сценария: " + err.Error())
	}
	file, err := os.CreateTemp(directory, ".remoteit-action-*"+extension)
	if err != nil {
		return failedAction("не удалось создать временный сценарий: " + err.Error())
	}
	path := file.Name()
	defer os.Remove(path)
	_ = file.Chmod(0o600)
	payload := []byte(script)
	if runtime.GOOS == "windows" && shell == "powershell" {
		payload = append([]byte{0xEF, 0xBB, 0xBF}, payload...)
	}
	if _, err = file.Write(payload); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return failedAction("не удалось записать временный сценарий: " + err.Error())
	}
	args = append(args, path)
	return runFixedActionCommand(ctx, executable, args...)
}

func executeNetworkDiagnostic(ctx context.Context) remoteJobResult {
	switch runtime.GOOS {
	case "windows":
		return runFixedActionCommand(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", `[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-NetIPConfiguration -Detailed | Format-List; Get-DnsClientServerAddress | Format-Table -AutoSize; Get-NetRoute | Sort-Object RouteMetric | Select-Object -First 40 | Format-Table -AutoSize`)
	case "darwin":
		return runFixedActionCommand(ctx, "/bin/sh", "-c", `ifconfig; printf '\n--- routes ---\n'; netstat -rn; printf '\n--- dns ---\n'; scutil --dns`)
	default:
		return runFixedActionCommand(ctx, "/bin/sh", "-c", `ip -brief address 2>/dev/null || ifconfig; printf '\n--- routes ---\n'; ip route 2>/dev/null || route -n; printf '\n--- dns ---\n'; cat /etc/resolv.conf`)
	}
}

func executeServicesDiagnostic(ctx context.Context) remoteJobResult {
	switch runtime.GOOS {
	case "windows":
		return runFixedActionCommand(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", `[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Service | Where-Object {$_.Status -ne 'Running'} | Sort-Object Name | Select-Object -First 100 Name,DisplayName,Status,StartType | Format-Table -AutoSize`)
	case "darwin":
		return runFixedActionCommand(ctx, "/bin/launchctl", "list")
	default:
		return runFixedActionCommand(ctx, "systemctl", "--failed", "--no-pager", "--plain")
	}
}

func executeServiceRestart(ctx context.Context, name string) remoteJobResult {
	if !safeAgentServiceName.MatchString(name) {
		return failedAction("имя службы недопустимо")
	}
	switch runtime.GOOS {
	case "windows":
		stop := runFixedActionCommand(ctx, "sc.exe", "stop", name)
		if !stop.Success && !strings.Contains(strings.ToLower(stop.Output+stop.Error), "1062") {
			return stop
		}
		if !waitContext(ctx, 1200*time.Millisecond) {
			return failedAction("перезапуск службы прерван")
		}
		start := runFixedActionCommand(ctx, "sc.exe", "start", name)
		if !start.Success {
			return start
		}
		check := runFixedActionCommand(ctx, "sc.exe", "query", name)
		check.Output = strings.TrimSpace(stop.Output + "\n" + start.Output + "\n" + check.Output)
		return check
	case "darwin":
		return runFixedActionCommand(ctx, "/bin/launchctl", "kickstart", "-k", "system/"+name)
	default:
		restart := runFixedActionCommand(ctx, "systemctl", "restart", name)
		if !restart.Success {
			return restart
		}
		return runFixedActionCommand(ctx, "systemctl", "is-active", name)
	}
}

func runFixedActionCommand(ctx context.Context, name string, args ...string) remoteJobResult {
	command := exec.CommandContext(ctx, name, args...)
	prepareBackgroundCommand(command)
	output := &cappedBuffer{max: 256 * 1024}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
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
		errorText = "действие превысило разрешённое время выполнения"
		exitCode = -1
	}
	return remoteJobResult{Success: err == nil, Output: output.String(), Error: errorText, ExitCode: exitCode}
}

func failedAction(message string) remoteJobResult {
	return remoteJobResult{Success: false, Error: message, ExitCode: -1}
}
