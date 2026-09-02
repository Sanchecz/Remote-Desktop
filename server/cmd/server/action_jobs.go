package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const integrationTokenPrefix = "rmt_mcp_"

var safeServiceName = regexp.MustCompile(`^[A-Za-z0-9_. -]{1,128}$`)
var safePackageID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+:-]{0,127}$`)
var safeLocalPrincipal = regexp.MustCompile(`^[\p{L}\p{N}._@ -]{1,128}$`)
var safeWindowsPrincipal = regexp.MustCompile(`^[\p{L}\p{N}._@\\ -]{1,128}$`)
var safeDownloadName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}._() -]{0,127}$`)
var sha256Text = regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)
var vpnName = regexp.MustCompile(`^[\p{L}\p{N}._ -]{1,64}$`)
var dnsName = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+)$`)
var safePrinterName = regexp.MustCompile(`^[^\x00-\x1f]{1,256}$`)
var safeWindowsPath = regexp.MustCompile(`(?i)^[a-z]:\\[^<>:"|?*\x00-\x1f]{1,220}$`)
var safeShareName = regexp.MustCompile(`^[\p{L}\p{N}._ -]{1,64}$`)

type actionSigner struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func newActionSignerFromEnv() (*actionSigner, error) {
	secret := strings.TrimSpace(os.Getenv("REMOTEIT_ACTION_SIGNING_SECRET"))
	if secret == "" {
		return nil, nil
	}
	if len(secret) < 32 {
		return nil, errors.New("REMOTEIT_ACTION_SIGNING_SECRET must contain at least 32 characters")
	}
	seed := sha256.Sum256([]byte(secret))
	private := ed25519.NewKeyFromSeed(seed[:])
	public := private.Public().(ed25519.PublicKey)
	return &actionSigner{private: private, public: public}, nil
}

func (signer *actionSigner) publicKey() string {
	if signer == nil {
		return ""
	}
	return base64.RawStdEncoding.EncodeToString(signer.public)
}

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

func (signer *actionSigner) sign(envelope signedActionEnvelope) (string, error) {
	if signer == nil {
		return "", errors.New("контур подписанных действий не настроен")
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(ed25519.Sign(signer.private, payload)), nil
}

func (signer *actionSigner) marshalAndSign(envelope signedActionEnvelope) (string, string, error) {
	if signer == nil {
		return "", "", errors.New("контур подписанных действий не настроен")
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return "", "", err
	}
	signature := ed25519.Sign(signer.private, payload)
	return base64.RawStdEncoding.EncodeToString(payload), base64.RawStdEncoding.EncodeToString(signature), nil
}

type actionDefinition struct {
	Type             string
	Title            string
	Description      string
	Risk             string
	ApprovalRequired bool
	TimeoutSeconds   int
	SupportedOS      string
	Rollback         string
}

var actionDefinitions = map[string]actionDefinition{
	"diagnostic.system": {
		Type: "diagnostic.system", Title: "Диагностика системы", Description: "Собирает версию ОС, загрузку, память, диски и время работы без изменения компьютера.", Risk: "read", TimeoutSeconds: 30,
	},
	"diagnostic.network": {
		Type: "diagnostic.network", Title: "Диагностика сети", Description: "Показывает интерфейсы, IP, маршруты и DNS без изменения сетевых параметров.", Risk: "read", TimeoutSeconds: 30,
	},
	"diagnostic.services": {
		Type: "diagnostic.services", Title: "Диагностика служб", Description: "Показывает службы с ошибками и остановленные важные службы без изменения системы.", Risk: "read", TimeoutSeconds: 30,
	},
	"diagnostic.lan_scan": {
		Type: "diagnostic.lan_scan", Title: "Сканирование локальной сети", Description: "Ищет доступные внутренние узлы и типовые службы в нескольких заданных диапазонах через выбранный Agent, не изменяя сеть пользователя.", Risk: "read", TimeoutSeconds: 180,
	},
	"diagnostic.tcp_probe": {
		Type: "diagnostic.tcp_probe", Title: "Проверка узла мониторинга", Description: "Проверяет доступность до 16 явно заданных TCP-портов внутреннего узла через выбранный Agent.", Risk: "read", TimeoutSeconds: 30,
	},
	"windows.printers.list": {
		Type: "windows.printers.list", Title: "Список принтеров", Description: "Показывает установленные принтеры, порты, драйверы, очередь и принтер по умолчанию без изменений.", Risk: "read", TimeoutSeconds: 30, SupportedOS: "windows",
	},
	"windows.printers.discover": {
		Type: "windows.printers.discover", Title: "Найти принтеры в локальной сети", Description: "Проверяет только стандартные службы печати в одной внутренней подсети через выбранный Agent и объединяет результат с установленными принтерами.", Risk: "read", TimeoutSeconds: 60, SupportedOS: "windows",
	},
	"windows.printers.open_settings": {
		Type: "windows.printers.open_settings", Title: "Открыть принтеры и сканеры", Description: "Открывает штатную страницу Windows «Принтеры и сканеры» в закреплённом пользовательском сеансе.", Risk: "high", TimeoutSeconds: 30, SupportedOS: "windows",
	},
	"windows.printer.open_web": {
		Type: "windows.printer.open_web", Title: "Открыть веб-интерфейс принтера", Description: "Открывает локальный HTTP/HTTPS-интерфейс выбранного принтера в закреплённом Windows-сеансе Agent.", Risk: "high", TimeoutSeconds: 30, SupportedOS: "windows",
	},
	"windows.printer.set_default": {
		Type: "windows.printer.set_default", Title: "Назначить принтер по умолчанию", Description: "Назначает один точно выбранный установленный принтер принтером по умолчанию.", Risk: "high", ApprovalRequired: true, TimeoutSeconds: 30, SupportedOS: "windows", Rollback: "Вернуть прежний принтер по умолчанию в параметрах Windows.",
	},
	"windows.scan_folder.configure": {
		Type: "windows.scan_folder.configure", Title: "Настроить папку сканов", Description: "Создаёт указанную папку и SMB-ресурс только для выбранной существующей учётной записи Windows. Пароль не хранится в RemoteIt.", Risk: "high", ApprovalRequired: true, TimeoutSeconds: 60, SupportedOS: "windows", Rollback: "Удалить SMB-ресурс командой Remove-SmbShare; сама папка и уже полученные сканы сохранятся.",
	},
	"service.restart": {
		Type: "service.restart", Title: "Перезапуск службы", Description: "Перезапускает одну явно указанную системную службу.", Risk: "high", ApprovalRequired: true, TimeoutSeconds: 60, Rollback: "Проверить состояние службы; при необходимости запустить её повторно.",
	},
	"process.terminate": {
		Type: "process.terminate", Title: "Завершение процесса", Description: "Принудительно завершает один процесс по PID.", Risk: "high", ApprovalRequired: true, TimeoutSeconds: 30, Rollback: "Запустить приложение или службу повторно, если завершение было ошибочным.",
	},
	"file.download": {
		Type: "file.download", Title: "Безопасное скачивание файла", Description: "Скачивает HTTPS-файл в изолированный каталог RemoteIt и обязательно проверяет заранее известный SHA-256. Файл не запускается.", Risk: "high", ApprovalRequired: true, TimeoutSeconds: 600, Rollback: "Удалить проверенный файл из каталога staging RemoteIt.",
	},
	"package.install": {
		Type: "package.install", Title: "Установка пакета", Description: "Устанавливает один пакет по точному ID через системный менеджер пакетов без произвольной командной строки.", Risk: "high", ApprovalRequired: true, TimeoutSeconds: 600, Rollback: "Удалить пакет штатным менеджером пакетов после проверки зависимостей.",
	},
	"local.group.add_member": {
		Type: "local.group.add_member", Title: "Добавление в локальную группу", Description: "Добавляет одну существующую учётную запись в одну явно указанную локальную группу.", Risk: "critical", ApprovalRequired: true, TimeoutSeconds: 30, Rollback: "Удалить указанную учётную запись из локальной группы.",
	},
	"windows.vpn.upsert": {
		Type: "windows.vpn.upsert", Title: "Настройка встроенного VPN Windows", Description: "Создаёт или обновляет системный VPN-профиль Windows без сохранения пароля пользователя или общего секрета.", Risk: "high", ApprovalRequired: true, TimeoutSeconds: 60, SupportedOS: "windows", Rollback: "Удалить созданный VPN-профиль в параметрах Windows или командой Remove-VpnConnection.",
	},
	"system.reboot": {
		Type: "system.reboot", Title: "Перезагрузка компьютера", Description: "Планирует штатную перезагрузку с минутной задержкой, чтобы Agent успел отправить результат.", Risk: "critical", ApprovalRequired: true, TimeoutSeconds: 30, Rollback: "Отменить запланированную перезагрузку в течение одной минуты штатной командой ОС.",
	},
	"script.execute": {
		Type: "script.execute", Title: "Выполнение проверенного сценария", Description: "Выполняет точный текст сценария, полностью показанный владельцу до подтверждения. Это универсальный путь для задач, которых ещё нет в типовом каталоге; секреты и пароли в сценарий передавать нельзя.", Risk: "critical", ApprovalRequired: true, TimeoutSeconds: 600, Rollback: "Откат зависит от показанного сценария и должен быть проверен владельцем до запуска.",
	},
}

type actionPlan struct {
	Action           string         `json:"action"`
	Title            string         `json:"title"`
	Description      string         `json:"description"`
	Risk             string         `json:"risk"`
	ApprovalRequired bool           `json:"approvalRequired"`
	TimeoutSeconds   int            `json:"timeoutSeconds"`
	DeviceID         string         `json:"deviceId"`
	DeviceName       string         `json:"deviceName"`
	DeviceOS         string         `json:"deviceOs"`
	Parameters       map[string]any `json:"parameters"`
	Steps            []string       `json:"steps"`
	Rollback         string         `json:"rollback"`
	ExpiresInSeconds int            `json:"expiresInSeconds"`
}

func normalizeActionParameters(action string, input map[string]any) (map[string]any, error) {
	if input == nil {
		input = map[string]any{}
	}
	switch action {
	case "diagnostic.system", "diagnostic.network", "diagnostic.services", "windows.printers.list", "windows.printers.open_settings", "system.reboot":
		if len(input) != 0 {
			return nil, errors.New("это действие не принимает параметры")
		}
		return map[string]any{}, nil
	case "diagnostic.lan_scan":
		if len(input) > 1 {
			return nil, errors.New("для сканирования допускается только параметр subnet")
		}
		subnet, _ := input["subnet"].(string)
		subnet = strings.TrimSpace(subnet)
		if subnet != "" {
			normalized, err := normalizeLANScanRanges(subnet)
			if err != nil {
				return nil, err
			}
			subnet = normalized
		}
		return map[string]any{"subnet": subnet}, nil
	case "windows.printers.discover":
		if len(input) > 1 {
			return nil, errors.New("для поиска принтеров допускается только параметр subnet")
		}
		subnet, _ := input["subnet"].(string)
		subnet = strings.TrimSpace(subnet)
		if subnet != "" {
			ip, network, err := net.ParseCIDR(subnet)
			if err != nil || ip.To4() == nil || !ip.IsPrivate() {
				return nil, errors.New("подсеть должна быть внутренней IPv4-подсетью в формате CIDR")
			}
			ones, bits := network.Mask.Size()
			if bits != 32 || ones < 24 || ones > 32 {
				return nil, errors.New("для поиска принтеров допускается не более 256 внутренних IPv4-адресов (/24…/32)")
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
			value, err := numericActionParameter(rawPort)
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
		if len(input) != 2 {
			return nil, errors.New("для веб-интерфейса принтера требуются только host и scheme")
		}
		host, _ := input["host"].(string)
		scheme, _ := input["scheme"].(string)
		host, scheme = strings.TrimSpace(host), strings.ToLower(strings.TrimSpace(scheme))
		address := net.ParseIP(host)
		if address == nil || (!address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast()) {
			return nil, errors.New("веб-интерфейс принтера должен иметь внутренний IP-адрес")
		}
		if scheme != "http" && scheme != "https" {
			return nil, errors.New("для принтера разрешены только HTTP и HTTPS")
		}
		return map[string]any{"host": address.String(), "scheme": scheme}, nil
	case "windows.printer.set_default":
		if len(input) != 1 {
			return nil, errors.New("требуется только точное имя принтера")
		}
		name, _ := input["name"].(string)
		name = strings.TrimSpace(name)
		if !safePrinterName.MatchString(name) {
			return nil, errors.New("имя принтера недопустимо")
		}
		return map[string]any{"name": name}, nil
	case "windows.scan_folder.configure":
		if len(input) != 3 {
			return nil, errors.New("для папки сканов требуются только path, shareName и principal")
		}
		path, _ := input["path"].(string)
		shareName, _ := input["shareName"].(string)
		principal, _ := input["principal"].(string)
		path, shareName, principal = strings.TrimSpace(path), strings.TrimSpace(shareName), strings.TrimSpace(principal)
		if !safeWindowsPath.MatchString(path) || strings.Contains(path, `..`) {
			return nil, errors.New("папка сканов должна быть абсолютным безопасным путём Windows")
		}
		if !safeShareName.MatchString(shareName) || strings.HasSuffix(shareName, "$") || !safeWindowsPrincipal.MatchString(principal) {
			return nil, errors.New("имя общего ресурса или учётной записи недопустимо")
		}
		return map[string]any{"path": path, "shareName": shareName, "principal": principal}, nil
	case "service.restart":
		if len(input) != 1 {
			return nil, errors.New("для перезапуска службы требуется только параметр name")
		}
		name, _ := input["name"].(string)
		name = strings.TrimSpace(name)
		if !safeServiceName.MatchString(name) {
			return nil, errors.New("имя службы должно содержать от 1 до 128 безопасных символов")
		}
		return map[string]any{"name": name}, nil
	case "process.terminate":
		if len(input) != 1 {
			return nil, errors.New("для завершения процесса требуется только параметр pid")
		}
		var pid int64
		switch value := input["pid"].(type) {
		case float64:
			if math.Trunc(value) != value {
				return nil, errors.New("PID должен быть целым числом")
			}
			pid = int64(value)
		case int:
			pid = int64(value)
		case int64:
			pid = value
		case json.Number:
			pid, _ = value.Int64()
		case string:
			pid, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		}
		if pid < 1 || pid > 1<<31-1 {
			return nil, errors.New("PID должен быть положительным целым числом")
		}
		return map[string]any{"pid": pid}, nil
	case "file.download":
		if len(input) != 3 {
			return nil, errors.New("для скачивания файла требуются только url, sha256 и fileName")
		}
		rawURL, _ := input["url"].(string)
		checksum, _ := input["sha256"].(string)
		fileName, _ := input["fileName"].(string)
		rawURL, checksum, fileName = strings.TrimSpace(rawURL), strings.ToLower(strings.TrimSpace(checksum)), strings.TrimSpace(fileName)
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
			return nil, errors.New("URL файла должен быть корректным HTTPS-адресом без логина и фрагмента")
		}
		if !sha256Text.MatchString(checksum) {
			return nil, errors.New("для скачивания требуется полный SHA-256 из 64 шестнадцатеричных символов")
		}
		if !safeDownloadName.MatchString(fileName) || filepath.Base(fileName) != fileName {
			return nil, errors.New("имя файла недопустимо")
		}
		return map[string]any{"url": parsed.String(), "sha256": checksum, "fileName": fileName}, nil
	case "package.install":
		if len(input) != 1 {
			return nil, errors.New("для установки пакета требуется только параметр packageId")
		}
		packageID, _ := input["packageId"].(string)
		packageID = strings.TrimSpace(packageID)
		if !safePackageID.MatchString(packageID) {
			return nil, errors.New("ID пакета должен содержать только буквы, цифры, точки, подчёркивания, плюсы, двоеточия или дефисы")
		}
		return map[string]any{"packageId": packageID}, nil
	case "local.group.add_member":
		if len(input) != 2 {
			return nil, errors.New("для изменения группы требуются только member и group")
		}
		member, _ := input["member"].(string)
		group, _ := input["group"].(string)
		member, group = strings.TrimSpace(member), strings.TrimSpace(group)
		if !safeLocalPrincipal.MatchString(member) || !safeLocalPrincipal.MatchString(group) {
			return nil, errors.New("имя пользователя или группы содержит недопустимые символы")
		}
		return map[string]any{"member": member, "group": group}, nil
	case "windows.vpn.upsert":
		if len(input) != 4 {
			return nil, errors.New("для VPN-профиля требуются только name, serverAddress, tunnelType и authenticationMethod")
		}
		name, _ := input["name"].(string)
		serverAddress, _ := input["serverAddress"].(string)
		tunnelType, _ := input["tunnelType"].(string)
		authMethod, _ := input["authenticationMethod"].(string)
		name, serverAddress = strings.TrimSpace(name), strings.TrimSpace(serverAddress)
		if !vpnName.MatchString(name) {
			return nil, errors.New("название VPN-профиля недопустимо")
		}
		if net.ParseIP(serverAddress) == nil && !dnsName.MatchString(serverAddress) {
			return nil, errors.New("адрес VPN-сервера должен быть IP или полным DNS-именем")
		}
		if !oneOf(tunnelType, "Automatic", "Ikev2", "L2tp", "Pptp", "Sstp") {
			return nil, errors.New("неподдерживаемый тип VPN-туннеля")
		}
		if !oneOf(authMethod, "Eap", "Pap", "Chap", "MSChapv2") {
			return nil, errors.New("неподдерживаемый метод аутентификации VPN")
		}
		return map[string]any{"name": name, "serverAddress": serverAddress, "tunnelType": tunnelType, "authenticationMethod": authMethod}, nil
	case "script.execute":
		if len(input) != 2 {
			return nil, errors.New("для сценария требуются только shell и script")
		}
		shell, _ := input["shell"].(string)
		script, _ := input["script"].(string)
		shell = strings.ToLower(strings.TrimSpace(shell))
		if !oneOf(shell, "powershell", "cmd", "bash", "sh", "zsh") {
			return nil, errors.New("оболочка сценария не поддерживается")
		}
		if strings.TrimSpace(script) == "" || len(script) > 16*1024 || strings.ContainsRune(script, '\x00') {
			return nil, errors.New("сценарий должен содержать от 1 до 16384 байт и не содержать NUL")
		}
		return map[string]any{"shell": shell, "script": script}, nil
	default:
		return nil, errors.New("неизвестный тип действия")
	}
}

func normalizeLANScanRanges(requested string) (string, error) {
	parts := strings.FieldsFunc(requested, func(value rune) bool {
		return value == ',' || value == ';' || value == '\n' || value == '\r'
	})
	if len(parts) == 0 || len(parts) > 8 {
		return "", errors.New("укажите от одного до восьми внутренних диапазонов")
	}
	seen := make(map[uint32]struct{}, 512)
	normalized := make([]string, 0, len(parts))
	appendAddress := func(ip net.IP) error {
		ip4 := ip.To4()
		if ip4 == nil || !ip4.IsPrivate() || ip4.IsUnspecified() || ip4.IsMulticast() {
			return errors.New("сканировать можно только внутренние IPv4-адреса")
		}
		value := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
		seen[value] = struct{}{}
		if len(seen) > 1024 {
			return errors.New("за один запуск допускается не более 1024 внутренних IPv4-адресов")
		}
		return nil
	}
	for _, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		if strings.Contains(part, "/") {
			ip, network, err := net.ParseCIDR(part)
			if err != nil || ip.To4() == nil || !ip.IsPrivate() {
				return "", errors.New("CIDR должен быть внутренней IPv4-подсетью")
			}
			ones, bits := network.Mask.Size()
			if bits != 32 || ones < 22 || ones > 32 {
				return "", errors.New("одна CIDR-подсеть должна быть в пределах /22…/32")
			}
			base := network.IP.To4()
			count := uint32(1 << (32 - ones))
			start, end := uint32(0), count
			if count > 2 {
				start, end = 1, count-1
			}
			baseValue := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
			for offset := start; offset < end; offset++ {
				value := baseValue + offset
				if err := appendAddress(net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value))); err != nil {
					return "", err
				}
			}
			normalized = append(normalized, network.String())
			continue
		}
		if separator := strings.Index(part, "-"); separator >= 0 {
			first := net.ParseIP(strings.TrimSpace(part[:separator])).To4()
			last := net.ParseIP(strings.TrimSpace(part[separator+1:])).To4()
			if first == nil || last == nil || !first.IsPrivate() || !last.IsPrivate() {
				return "", errors.New("диапазон должен содержать два внутренних IPv4-адреса")
			}
			start := uint32(first[0])<<24 | uint32(first[1])<<16 | uint32(first[2])<<8 | uint32(first[3])
			end := uint32(last[0])<<24 | uint32(last[1])<<16 | uint32(last[2])<<8 | uint32(last[3])
			if start > end || uint64(end)-uint64(start)+1 > 1024 {
				return "", errors.New("диапазон должен идти по возрастанию и содержать не более 1024 адресов")
			}
			for value := start; ; value++ {
				if err := appendAddress(net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value))); err != nil {
					return "", err
				}
				if value == end {
					break
				}
			}
			normalized = append(normalized, first.String()+"-"+last.String())
			continue
		}
		ip := net.ParseIP(part).To4()
		if ip == nil {
			return "", errors.New("используйте CIDR, одиночный IP или диапазон IP-IP")
		}
		if err := appendAddress(ip); err != nil {
			return "", err
		}
		normalized = append(normalized, ip.String())
	}
	return strings.Join(normalized, ", "), nil
}

func numericActionParameter(value any) (int64, error) {
	var number int64
	switch typed := value.(type) {
	case float64:
		if math.Trunc(typed) != typed {
			return 0, errors.New("not an integer")
		}
		number = int64(typed)
	case int:
		number = int64(typed)
	case int64:
		number = typed
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, err
		}
		number = parsed
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, err
		}
		number = parsed
	default:
		return 0, errors.New("not a number")
	}
	return number, nil
}

func actionSteps(definition actionDefinition, parameters map[string]any) []string {
	switch definition.Type {
	case "diagnostic.system":
		return []string{"Проверить актуальность Agent", "Собрать системные показатели", "Вернуть структурированный результат в журнал RemoteIt"}
	case "diagnostic.network":
		return []string{"Собрать интерфейсы и IP", "Прочитать маршруты и DNS", "Вернуть результат без изменения сети"}
	case "diagnostic.services":
		return []string{"Прочитать состояние служб", "Выделить остановленные и ошибочные службы", "Вернуть результат без перезапуска"}
	case "diagnostic.lan_scan":
		return []string{"Проверить и объединить заданные внутренние подсети и диапазоны", "Проверить не более 1024 уникальных адресов с ограничением параллельности", "Вернуть найденные RDP, SSH, файловые, веб- и печатные службы"}
	case "diagnostic.tcp_probe":
		return []string{"Проверить внутренний IP выбранного узла", "Подключиться только к явно заданным TCP-портам", "Сохранить задержку и состояние в истории мониторинга"}
	case "windows.printers.discover":
		return []string{"Определить одну внутреннюю подсеть выбранного Agent", "Проверить только стандартные IPP, JetDirect, LPD и веб-порты принтеров", "Объединить сетевые устройства с уже установленными принтерами"}
	case "windows.printers.list":
		return []string{"Прочитать установленные принтеры и порты", "Определить состояние очередей и принтер по умолчанию", "Вернуть структурированный список без изменений"}
	case "windows.printers.open_settings":
		return []string{"Найти закреплённый Windows-сеанс", "Открыть штатную страницу принтеров и сканеров", "Оставить ввод реквизитов внутри Windows"}
	case "windows.printer.open_web":
		return []string{fmt.Sprintf("Повторно проверить внутренний адрес принтера %s", parameters["host"]), "Открыть его HTTP/HTTPS-интерфейс штатным браузером закреплённого Windows-сеанса"}
	case "windows.printer.set_default":
		return []string{fmt.Sprintf("Проверить установленный принтер %s", parameters["name"]), "Назначить его принтером по умолчанию", "Повторно прочитать итоговое состояние"}
	case "windows.scan_folder.configure":
		return []string{fmt.Sprintf("Создать папку %s при отсутствии", parameters["path"]), fmt.Sprintf("Создать или проверить SMB-ресурс %s", parameters["shareName"]), fmt.Sprintf("Выдать изменение только учётной записи %s", parameters["principal"]), "Вернуть UNC-путь для настройки МФУ"}
	case "service.restart":
		return []string{fmt.Sprintf("Проверить службу %s", parameters["name"]), "Перезапустить только выбранную службу", "Проверить итоговое состояние"}
	case "process.terminate":
		return []string{fmt.Sprintf("Проверить процесс PID %v", parameters["pid"]), "Завершить только этот PID", "Зафиксировать код результата"}
	case "file.download":
		return []string{fmt.Sprintf("Скачать %s по HTTPS в изолированный staging-каталог", parameters["fileName"]), "Ограничить сценарий размером 2 ГБ; для файлов до 50 ГБ использовать потоковый канал передачи RemoteIt", "Сверить SHA-256 до публикации файла и не запускать его"}
	case "package.install":
		return []string{fmt.Sprintf("Найти точный пакет %s в системном менеджере", parameters["packageId"]), "Установить без произвольной командной строки", "Вернуть код и журнал менеджера пакетов"}
	case "local.group.add_member":
		return []string{fmt.Sprintf("Проверить локальную группу %s", parameters["group"]), fmt.Sprintf("Добавить только учётную запись %s", parameters["member"]), "Проверить итоговое членство"}
	case "windows.vpn.upsert":
		return []string{fmt.Sprintf("Проверить VPN-профиль %s", parameters["name"]), "Создать или заменить профиль встроенного VPN Windows", "Не сохранять пароль или общий секрет", "Вернуть итоговые параметры профиля"}
	case "system.reboot":
		return []string{"Проверить связь с Agent", "Запланировать штатную перезагрузку через одну минуту", "Отправить результат до разрыва соединения"}
	case "script.execute":
		script, _ := parameters["script"].(string)
		sum := sha256.Sum256([]byte(script))
		return []string{fmt.Sprintf("Показать владельцу полный сценарий %s и его SHA-256 %s", parameters["shell"], hex.EncodeToString(sum[:])), "После отдельного подтверждения выполнить ровно один раз", "Сохранить ограниченный вывод, код завершения, автора и время в журнале"}
	default:
		return nil
	}
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func actionRequestHash(deviceID, action string, parameters map[string]any) string {
	payload, _ := json.Marshal(struct {
		DeviceID   string         `json:"deviceId"`
		Action     string         `json:"action"`
		Parameters map[string]any `json:"parameters"`
	}{deviceID, action, parameters})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

type actionActor struct {
	UserID   string
	Username string
	Role     string
	Via      string
}

type integrationState struct {
	TokenID  string
	UserID   string
	Username string
	Role     string
	Scopes   map[string]bool
}

const integrationAuthKey contextKey = "integration-auth"

func currentIntegration(r *http.Request) *integrationState {
	state, _ := r.Context().Value(integrationAuthKey).(*integrationState)
	return state
}

func canUseCodexIntegration(role string) bool {
	return role == "owner" || role == "admin"
}

func (s *server) requireIntegrationToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "Не указан токен интеграции RemoteIt")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if !strings.HasPrefix(token, integrationTokenPrefix) || len(token) < len(integrationTokenPrefix)+32 {
			writeError(w, http.StatusUnauthorized, "Недействительный токен интеграции")
			return
		}
		var state integrationState
		var stored []byte
		var scopes []string
		err := s.db.QueryRow(r.Context(), `SELECT t.id,t.user_id,u.username,u.role,t.token_hash,t.scopes FROM integration_tokens t JOIN users u ON u.id=t.user_id WHERE t.token_hash=$1 AND t.revoked_at IS NULL AND t.expires_at>now() AND u.disabled=false`, tokenHash(token)).Scan(&state.TokenID, &state.UserID, &state.Username, &state.Role, &stored, &scopes)
		if err != nil || subtle.ConstantTimeCompare(tokenHash(token), stored) != 1 {
			writeError(w, http.StatusUnauthorized, "Токен интеграции истёк или отозван")
			return
		}
		// Re-check the live account role on every request. If an administrator is
		// demoted, disabled or removed, an old MCP token must not preserve the
		// elevated RemoteIt capabilities it had when it was issued.
		if !canUseCodexIntegration(state.Role) {
			writeError(w, http.StatusForbidden, "Интеграция Codex доступна только владельцу и администраторам")
			return
		}
		state.Scopes = make(map[string]bool, len(scopes))
		for _, scope := range scopes {
			state.Scopes[scope] = true
		}
		_, _ = s.db.Exec(r.Context(), `UPDATE integration_tokens SET last_used_at=now() WHERE id=$1 AND (last_used_at IS NULL OR last_used_at<now()-interval '1 minute')`, state.TokenID)
		ctx := context.WithValue(r.Context(), integrationAuthKey, &state)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireIntegrationScope(w http.ResponseWriter, state *integrationState, scope string) bool {
	if state == nil || !state.Scopes[scope] {
		writeError(w, http.StatusForbidden, "Токен интеграции не имеет права "+scope)
		return false
	}
	return true
}

func (s *server) listIntegrationTokens(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if !canUseCodexIntegration(a.Role) {
		writeError(w, http.StatusForbidden, "Интеграциями Codex управляют только владелец и администраторы")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT id,name,scopes,expires_at,created_at,last_used_at,revoked_at FROM integration_tokens WHERE user_id=$1 ORDER BY created_at DESC`, a.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить интеграции")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name string
		var scopes []string
		var expires, created time.Time
		var lastUsed, revoked *time.Time
		if rows.Scan(&id, &name, &scopes, &expires, &created, &lastUsed, &revoked) == nil {
			items = append(items, map[string]any{"id": id, "name": name, "scopes": scopes, "expiresAt": expires, "createdAt": created, "lastUsedAt": lastUsed, "revokedAt": revoked})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": items})
}

func (s *server) createIntegrationToken(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if !canUseCodexIntegration(a.Role) {
		writeError(w, http.StatusForbidden, "Интеграцию Codex могут создавать только владелец и администраторы")
		return
	}
	var input struct {
		Name        string `json:"name"`
		ExpiresDays int    `json:"expiresDays"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 100 {
		writeError(w, http.StatusBadRequest, "Название интеграции должно содержать от 1 до 100 символов")
		return
	}
	if input.ExpiresDays == 0 {
		input.ExpiresDays = 90
	}
	if input.ExpiresDays < 1 || input.ExpiresDays > 365 {
		writeError(w, http.StatusBadRequest, "Срок токена должен быть от 1 до 365 дней")
		return
	}
	token := integrationTokenPrefix + randomToken(32)
	scopes := []string{"devices:read", "actions:plan", "actions:create", "actions:read", "actions:cancel"}
	var id string
	var expires time.Time
	err := s.db.QueryRow(r.Context(), `INSERT INTO integration_tokens(user_id,name,token_hash,scopes,expires_at) VALUES($1,$2,$3,$4,now()+make_interval(days=>$5)) RETURNING id,expires_at`, a.UserID, input.Name, tokenHash(token), scopes, input.ExpiresDays).Scan(&id, &expires)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать токен интеграции")
		return
	}
	s.audit(r.Context(), a, nil, "integration_token.created", "integration_token", id, clientIP(r), map[string]any{"name": input.Name, "expiresAt": expires, "scopes": scopes})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": input.Name, "token": token, "scopes": scopes, "expiresAt": expires, "shownOnce": true})
}

func (s *server) revokeIntegrationToken(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if !canUseCodexIntegration(a.Role) {
		writeError(w, http.StatusForbidden, "Интеграциями Codex управляют только владелец и администраторы")
		return
	}
	id := chi.URLParam(r, "id")
	result, err := s.db.Exec(r.Context(), `UPDATE integration_tokens SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, id, a.UserID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Активная интеграция не найдена")
		return
	}
	s.audit(r.Context(), a, nil, "integration_token.revoked", "integration_token", id, clientIP(r), map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

type integrationDevice struct {
	ID              string    `json:"id"`
	ConnectionCode  string    `json:"remoteId"`
	Name            string    `json:"name"`
	Hostname        string    `json:"hostname"`
	OS              string    `json:"os"`
	OSVersion       string    `json:"osVersion"`
	Arch            string    `json:"arch"`
	AgentVersion    string    `json:"agentVersion"`
	PublicIP        string    `json:"publicIp"`
	LocalIPs        []string  `json:"localIps"`
	CurrentUser     string    `json:"currentUser"`
	CPULoadPercent  float64   `json:"cpuLoadPercent"`
	MemoryBytes     int64     `json:"memoryBytes"`
	MemoryUsedBytes int64     `json:"memoryUsedBytes"`
	DiskTotalBytes  int64     `json:"diskTotalBytes"`
	DiskFreeBytes   int64     `json:"diskFreeBytes"`
	UptimeSeconds   int64     `json:"uptimeSeconds"`
	Privileged      bool      `json:"privileged"`
	Online          bool      `json:"online"`
	LastSeen        time.Time `json:"lastSeen"`
}

func (s *server) getIntegrationDevice(ctx context.Context, reference string) (integrationDevice, error) {
	var device integrationDevice
	reference = strings.TrimSpace(reference)
	err := s.db.QueryRow(ctx, `SELECT id,connection_code,name,hostname,os,os_version,arch,agent_version,COALESCE(host(public_ip),''),local_ips,logged_in_user,cpu_load_percent,memory_bytes,memory_used_bytes,disk_total_bytes,disk_free_bytes,uptime_seconds,privileged,last_seen>now()-interval '75 seconds',last_seen FROM devices WHERE id::text=$1 OR connection_code=$1`, reference).Scan(
		&device.ID, &device.ConnectionCode, &device.Name, &device.Hostname, &device.OS, &device.OSVersion, &device.Arch, &device.AgentVersion, &device.PublicIP, &device.LocalIPs, &device.CurrentUser, &device.CPULoadPercent, &device.MemoryBytes, &device.MemoryUsedBytes, &device.DiskTotalBytes, &device.DiskFreeBytes, &device.UptimeSeconds, &device.Privileged, &device.Online, &device.LastSeen,
	)
	return device, err
}

func (s *server) integrationDeviceAccessAllowed(ctx context.Context, deviceID, role string) (bool, error) {
	var allowed bool
	err := s.db.QueryRow(ctx, `SELECT ($2='owner' OR access_password_hash='') FROM devices WHERE id=$1`, deviceID, role).Scan(&allowed)
	return allowed, err
}

func (s *server) requireIntegrationDeviceAccess(w http.ResponseWriter, r *http.Request, state *integrationState, deviceID string) bool {
	allowed, err := s.integrationDeviceAccessAllowed(r.Context(), deviceID, state.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить доступ к устройству")
		return false
	}
	if !allowed {
		writeJSON(w, http.StatusLocked, map[string]any{
			"error":    "Устройство защищено владельцем RemoteIt и недоступно этой интеграции Codex.",
			"code":     "DEVICE_LOCKED",
			"deviceId": deviceID,
		})
		return false
	}
	return true
}

func (s *server) integrationListDevices(w http.ResponseWriter, r *http.Request) {
	state := currentIntegration(r)
	if !requireIntegrationScope(w, state, "devices:read") {
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT id FROM devices WHERE ($1='owner' OR access_password_hash='') ORDER BY lower(name),connection_code`, state.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить устройства")
		return
	}
	defer rows.Close()
	devices := make([]integrationDevice, 0)
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			if device, getErr := s.getIntegrationDevice(r.Context(), id); getErr == nil {
				devices = append(devices, device)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (s *server) integrationGetDevice(w http.ResponseWriter, r *http.Request) {
	state := currentIntegration(r)
	if !requireIntegrationScope(w, state, "devices:read") {
		return
	}
	device, err := s.getIntegrationDevice(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить устройство")
		return
	}
	if !s.requireIntegrationDeviceAccess(w, r, state, device.ID) {
		return
	}
	writeJSON(w, http.StatusOK, device)
}

func (s *server) buildActionPlan(ctx context.Context, deviceReference, action string, input map[string]any) (actionPlan, error) {
	definition, ok := actionDefinitions[strings.TrimSpace(action)]
	if !ok {
		return actionPlan{}, errors.New("неизвестный тип действия")
	}
	device, err := s.getIntegrationDevice(ctx, deviceReference)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return actionPlan{}, errors.New("устройство не найдено")
		}
		return actionPlan{}, err
	}
	parameters, err := normalizeActionParameters(definition.Type, input)
	if err != nil {
		return actionPlan{}, err
	}
	if definition.SupportedOS != "" && !strings.Contains(strings.ToLower(device.OS), definition.SupportedOS) {
		return actionPlan{}, fmt.Errorf("действие недоступно для %s", device.OS)
	}
	if definition.Type == "script.execute" {
		shell, _ := parameters["shell"].(string)
		deviceOS := strings.ToLower(device.OS)
		if strings.Contains(deviceOS, "windows") && !oneOf(shell, "powershell", "cmd") {
			return actionPlan{}, fmt.Errorf("оболочка %s недоступна для %s", shell, device.OS)
		}
		if strings.Contains(deviceOS, "mac") && !oneOf(shell, "bash", "sh", "zsh") {
			return actionPlan{}, fmt.Errorf("оболочка %s недоступна для %s", shell, device.OS)
		}
		if !strings.Contains(deviceOS, "windows") && !strings.Contains(deviceOS, "mac") && !oneOf(shell, "bash", "sh") {
			return actionPlan{}, fmt.Errorf("оболочка %s недоступна для %s", shell, device.OS)
		}
	}
	expires := 120
	if definition.ApprovalRequired {
		expires = 600
	}
	return actionPlan{
		Action: definition.Type, Title: definition.Title, Description: definition.Description, Risk: definition.Risk,
		ApprovalRequired: definition.ApprovalRequired, TimeoutSeconds: definition.TimeoutSeconds, DeviceID: device.ID,
		DeviceName: device.Name, DeviceOS: device.OS, Parameters: parameters, Steps: actionSteps(definition, parameters),
		Rollback: definition.Rollback, ExpiresInSeconds: expires,
	}, nil
}

type actionCreateInput struct {
	DeviceID       string         `json:"deviceId"`
	Action         string         `json:"action"`
	Parameters     map[string]any `json:"parameters"`
	IdempotencyKey string         `json:"idempotencyKey"`
}

func (s *server) createAction(ctx context.Context, actor actionActor, input actionCreateInput) (map[string]any, error) {
	if s.actionSigner == nil {
		return nil, errors.New("контур подписанных действий временно отключён администратором сервера")
	}
	if !canUseCodexIntegration(actor.Role) {
		return nil, errors.New("действия Codex доступны только владельцу и администраторам")
	}
	plan, err := s.buildActionPlan(ctx, input.DeviceID, input.Action, input.Parameters)
	if err != nil {
		return nil, err
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if len(input.IdempotencyKey) > 128 {
		return nil, errors.New("ключ идемпотентности не должен превышать 128 символов")
	}
	parametersJSON, _ := json.Marshal(plan.Parameters)
	planJSON, _ := json.Marshal(plan)
	rollbackJSON, _ := json.Marshal(map[string]any{"description": plan.Rollback})
	requestHash := actionRequestHash(plan.DeviceID, plan.Action, plan.Parameters)
	nonce := randomToken(24)
	status := "awaiting_approval"
	if !plan.ApprovalRequired {
		status = "queued"
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var id string
	var created, expires time.Time
	err = tx.QueryRow(ctx, `INSERT INTO action_jobs(device_id,requested_by,requested_via,action_type,parameters,risk_level,status,approval_required,plan,rollback_plan,idempotency_key,request_hash,nonce,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now()+make_interval(secs=>$14)) RETURNING id,created_at,expires_at`,
		plan.DeviceID, actor.UserID, actor.Via, plan.Action, parametersJSON, plan.Risk, status, plan.ApprovalRequired, planJSON, rollbackJSON, input.IdempotencyKey, requestHash, nonce, plan.ExpiresInSeconds).Scan(&id, &created, &expires)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "action_jobs_idempotency_idx") {
			return nil, errors.New("действие с таким ключом идемпотентности уже существует")
		}
		return nil, err
	}
	if !plan.ApprovalRequired {
		if err = s.queueActionExecution(ctx, tx, id, plan.DeviceID, plan.Action, parametersJSON, requestHash, nonce, created, expires, actor.UserID, plan.TimeoutSeconds); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "deviceId": plan.DeviceID, "deviceName": plan.DeviceName, "action": plan.Action, "risk": plan.Risk, "status": status, "approvalRequired": plan.ApprovalRequired, "plan": plan, "createdAt": created, "expiresAt": expires}, nil
}

func (s *server) queueActionExecution(ctx context.Context, tx pgx.Tx, actionJobID, deviceID, action string, parameters json.RawMessage, requestHash, nonce string, issuedAt, expiresAt time.Time, actorID string, timeout int) error {
	envelope := signedActionEnvelope{Version: 1, ActionJobID: actionJobID, DeviceID: deviceID, Action: action, Parameters: parameters, IssuedAt: issuedAt.Unix(), ExpiresAt: expiresAt.Unix(), Nonce: nonce, RequestHash: requestHash}
	signedEnvelope, signature, err := s.actionSigner.marshalAndSign(envelope)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"actionJobId": actionJobID, "signedEnvelope": signedEnvelope, "signature": signature})
	if err != nil {
		return err
	}
	var executionJobID string
	err = tx.QueryRow(ctx, `INSERT INTO agent_jobs(device_id,created_by,job_type,payload,timeout_seconds,expires_at) VALUES($1,$2,'action',$3,$4,$5) RETURNING id`, deviceID, actorID, payload, timeout, expiresAt).Scan(&executionJobID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE action_jobs SET status='queued',execution_job_id=$1,signature=$2,updated_at=now() WHERE id=$3`, executionJobID, signature, actionJobID)
	return err
}

func actionActorFromAuth(a *authState) actionActor {
	return actionActor{UserID: a.UserID, Username: a.Username, Role: a.Role, Via: "web"}
}

func actionActorFromIntegration(state *integrationState) actionActor {
	return actionActor{UserID: state.UserID, Username: state.Username, Role: state.Role, Via: "mcp"}
}

func decodeActionCreate(w http.ResponseWriter, r *http.Request) (actionCreateInput, bool) {
	var input actionCreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		return input, false
	}
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.Action = strings.TrimSpace(input.Action)
	if input.DeviceID == "" || input.Action == "" {
		writeError(w, http.StatusBadRequest, "Не указаны устройство и действие")
		return input, false
	}
	return input, true
}

func writeActionError(w http.ResponseWriter, err error) {
	message := err.Error()
	status := http.StatusBadRequest
	if strings.Contains(message, "не найдено") {
		status = http.StatusNotFound
	} else if strings.Contains(message, "роль") || strings.Contains(message, "доступны только") {
		status = http.StatusForbidden
	} else if strings.Contains(message, "временно отключён") {
		status = http.StatusServiceUnavailable
	}
	writeError(w, status, message)
}

func (s *server) planActionJob(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if !canUseCodexIntegration(a.Role) {
		writeError(w, http.StatusForbidden, "Действия Codex доступны только владельцу и администраторам")
		return
	}
	input, ok := decodeActionCreate(w, r)
	if !ok {
		return
	}
	device, err := s.getIntegrationDevice(r.Context(), input.DeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить устройство")
		return
	}
	if !s.requireDeviceAccess(w, r, device.ID) {
		return
	}
	plan, err := s.buildActionPlan(r.Context(), device.ID, input.Action, input.Parameters)
	if err != nil {
		writeActionError(w, err)
		return
	}
	s.audit(r.Context(), a, nil, "action_job.planned", "device", plan.DeviceID, clientIP(r), map[string]any{"action": plan.Action, "risk": plan.Risk})
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan})
}

func (s *server) createActionJobFromWeb(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if !canUseCodexIntegration(a.Role) {
		writeError(w, http.StatusForbidden, "Действия Codex доступны только владельцу и администраторам")
		return
	}
	input, ok := decodeActionCreate(w, r)
	if !ok {
		return
	}
	device, err := s.getIntegrationDevice(r.Context(), input.DeviceID)
	if err != nil {
		writeActionError(w, errors.New("устройство не найдено"))
		return
	}
	if !s.requireDeviceAccess(w, r, device.ID) {
		return
	}
	result, err := s.createAction(r.Context(), actionActorFromAuth(a), input)
	if err != nil {
		writeActionError(w, err)
		return
	}
	s.audit(r.Context(), a, nil, "action_job.created", "action_job", result["id"].(string), clientIP(r), map[string]any{"deviceId": result["deviceId"], "action": result["action"], "risk": result["risk"], "status": result["status"], "via": "web"})
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) integrationPlanAction(w http.ResponseWriter, r *http.Request) {
	state := currentIntegration(r)
	if !requireIntegrationScope(w, state, "actions:plan") {
		return
	}
	input, ok := decodeActionCreate(w, r)
	if !ok {
		return
	}
	device, err := s.getIntegrationDevice(r.Context(), input.DeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить устройство")
		return
	}
	if !s.requireIntegrationDeviceAccess(w, r, state, device.ID) {
		return
	}
	plan, err := s.buildActionPlan(r.Context(), device.ID, input.Action, input.Parameters)
	if err != nil {
		writeActionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan})
}

func (s *server) integrationCreateAction(w http.ResponseWriter, r *http.Request) {
	state := currentIntegration(r)
	if !requireIntegrationScope(w, state, "actions:create") {
		return
	}
	input, ok := decodeActionCreate(w, r)
	if !ok {
		return
	}
	device, err := s.getIntegrationDevice(r.Context(), input.DeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить устройство")
		return
	}
	if !s.requireIntegrationDeviceAccess(w, r, state, device.ID) {
		return
	}
	input.DeviceID = device.ID
	result, err := s.createAction(r.Context(), actionActorFromIntegration(state), input)
	if err != nil {
		writeActionError(w, err)
		return
	}
	s.audit(r.Context(), nil, nil, "action_job.created", "action_job", result["id"].(string), clientIP(r), map[string]any{"deviceId": result["deviceId"], "action": result["action"], "risk": result["risk"], "status": result["status"], "via": "mcp", "integrationTokenId": state.TokenID, "actorUserId": state.UserID})
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) approveActionJob(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Подтверждать действия могут только владелец и администраторы")
		return
	}
	id := chi.URLParam(r, "id")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось подтвердить действие")
		return
	}
	defer tx.Rollback(r.Context())
	var deviceID, action, risk, requestHash, nonce, status string
	var parameters []byte
	var created, expires time.Time
	var timeout int
	err = tx.QueryRow(r.Context(), `SELECT a.device_id,a.action_type,a.parameters,a.risk_level,a.status,a.request_hash,a.nonce,a.created_at,a.expires_at,COALESCE((a.plan->>'timeoutSeconds')::integer,30) FROM action_jobs a WHERE a.id=$1 FOR UPDATE`, id).Scan(&deviceID, &action, &parameters, &risk, &status, &requestHash, &nonce, &created, &expires, &timeout)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Действие не найдено")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить действие")
		return
	}
	if status != "awaiting_approval" || time.Now().After(expires) {
		writeError(w, http.StatusConflict, "Действие уже подтверждено, завершено или истекло")
		return
	}
	if risk == "critical" && a.Role != "owner" {
		writeError(w, http.StatusForbidden, "Критическое действие может подтвердить только владелец")
		return
	}
	if !s.requireDeviceAccess(w, r, deviceID) {
		return
	}
	issuedAt := time.Now()
	executionExpires := issuedAt.Add(time.Duration(timeout+120) * time.Second)
	if err = s.queueActionExecution(r.Context(), tx, id, deviceID, action, parameters, requestHash, nonce, issuedAt, executionExpires, a.UserID, timeout); err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE action_jobs SET approved_by=$1,approved_at=now(),expires_at=$3,updated_at=now() WHERE id=$2`, a.UserID, id, executionExpires)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось поставить подтверждённое действие в очередь")
		return
	}
	s.audit(r.Context(), a, nil, "action_job.approved", "action_job", id, clientIP(r), map[string]any{"deviceId": deviceID, "action": action, "risk": risk})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "status": "queued"})
}

func (s *server) actionJobResponse(ctx context.Context, id, actorID string) (map[string]any, error) {
	_, _ = s.db.Exec(ctx, `UPDATE action_jobs SET status='expired',error_text='Срок действия запроса истёк',completed_at=now(),updated_at=now() WHERE id=$1 AND status IN ('awaiting_approval','queued') AND expires_at<=now()`, id)
	var deviceID, deviceName, remoteID, action, risk, status, via, output, errorText, requestHash string
	var parameters, plan []byte
	var approvalRequired bool
	var exitCode *int
	var created, expires time.Time
	var started, completed, approved *time.Time
	query, arguments := actionJobResponseLookup(id, actorID)
	err := s.db.QueryRow(ctx, query, arguments...).Scan(&deviceID, &deviceName, &remoteID, &action, &parameters, &risk, &status, &approvalRequired, &via, &plan, &output, &errorText, &exitCode, &requestHash, &created, &expires, &started, &completed, &approved)
	if err != nil {
		return nil, err
	}
	var parsedParameters, parsedPlan map[string]any
	_ = json.Unmarshal(parameters, &parsedParameters)
	_ = json.Unmarshal(plan, &parsedPlan)
	return map[string]any{"id": id, "deviceId": deviceID, "deviceName": deviceName, "remoteId": remoteID, "action": action, "parameters": parsedParameters, "risk": risk, "status": status, "approvalRequired": approvalRequired, "requestedVia": via, "plan": parsedPlan, "output": output, "error": errorText, "exitCode": exitCode, "requestHash": requestHash, "createdAt": created, "expiresAt": expires, "startedAt": started, "completedAt": completed, "approvedAt": approved}, nil
}

func actionJobResponseLookup(id, actorID string) (string, []any) {
	query := `SELECT a.device_id,d.name,d.connection_code,a.action_type,a.parameters,a.risk_level,a.status,a.approval_required,a.requested_via,a.plan,a.output,a.error_text,a.exit_code,a.request_hash,a.created_at,a.expires_at,a.started_at,a.completed_at,a.approved_at FROM action_jobs a JOIN devices d ON d.id=a.device_id WHERE a.id=$1`
	arguments := []any{id}
	if actorID != "" {
		// requested_by is UUID. Never bind an empty string to this predicate:
		// PostgreSQL correctly rejects it as an invalid UUID before evaluating OR.
		query += ` AND a.requested_by=$2`
		arguments = append(arguments, actorID)
	}
	return query, arguments
}

func (s *server) getActionJob(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if !canUseCodexIntegration(a.Role) {
		writeError(w, http.StatusForbidden, "Действия Codex доступны только владельцу и администраторам")
		return
	}
	result, err := s.actionJobResponse(r.Context(), chi.URLParam(r, "id"), "")
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Действие не найдено")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить действие")
		return
	}
	deviceID, _ := result["deviceId"].(string)
	if !s.requireDeviceAccess(w, r, deviceID) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) integrationGetAction(w http.ResponseWriter, r *http.Request) {
	state := currentIntegration(r)
	if !requireIntegrationScope(w, state, "actions:read") {
		return
	}
	result, err := s.actionJobResponse(r.Context(), chi.URLParam(r, "id"), state.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Действие не найдено")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить действие")
		return
	}
	deviceID, _ := result["deviceId"].(string)
	if !s.requireIntegrationDeviceAccess(w, r, state, deviceID) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) listActionJobs(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if !canUseCodexIntegration(a.Role) {
		writeError(w, http.StatusForbidden, "Действия Codex доступны только владельцу и администраторам")
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE action_jobs SET status='expired',error_text='Срок действия запроса истёк',completed_at=now(),updated_at=now() WHERE status IN ('awaiting_approval','queued') AND expires_at<=now()`)
	rows, err := s.db.Query(r.Context(), `SELECT id FROM action_jobs ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить действия")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			if item, itemErr := s.actionJobResponse(r.Context(), id, ""); itemErr == nil {
				deviceID, _ := item["deviceId"].(string)
				allowed, accessErr := s.deviceAccessAllowed(r, deviceID)
				if accessErr == nil && allowed {
					items = append(items, item)
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": items})
}

func (s *server) cancelAction(ctx context.Context, id, actorID string, owner bool) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var deviceID, status string
	var executionJobID *string
	query := `SELECT device_id,status,execution_job_id FROM action_jobs WHERE id=$1 AND ($2 OR requested_by=$3) FOR UPDATE`
	err = tx.QueryRow(ctx, query, id, owner, actorID).Scan(&deviceID, &status, &executionJobID)
	if err != nil {
		return "", err
	}
	if status != "awaiting_approval" && status != "queued" {
		return "", errors.New("можно отменить только ожидающее подтверждения или queued-действие")
	}
	if executionJobID != nil {
		_, _ = tx.Exec(ctx, `UPDATE agent_jobs SET status='cancelled',error_text='Отменено через Action Jobs',completed_at=now(),updated_at=now() WHERE id=$1 AND status='queued'`, *executionJobID)
	}
	_, err = tx.Exec(ctx, `UPDATE action_jobs SET status='cancelled',error_text='Отменено оператором',completed_at=now(),updated_at=now() WHERE id=$1`, id)
	if err != nil {
		return "", err
	}
	return deviceID, tx.Commit(ctx)
}

func (s *server) cancelActionJob(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if !canUseCodexIntegration(a.Role) {
		writeError(w, http.StatusForbidden, "Действия Codex доступны только владельцу и администраторам")
		return
	}
	deviceID, err := s.cancelAction(r.Context(), chi.URLParam(r, "id"), a.UserID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Действие не найдено")
		return
	}
	if err != nil {
		writeActionError(w, err)
		return
	}
	s.audit(r.Context(), a, nil, "action_job.cancelled", "action_job", chi.URLParam(r, "id"), clientIP(r), map[string]any{"deviceId": deviceID})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) integrationCancelAction(w http.ResponseWriter, r *http.Request) {
	state := currentIntegration(r)
	if !requireIntegrationScope(w, state, "actions:cancel") {
		return
	}
	deviceID, err := s.cancelAction(r.Context(), chi.URLParam(r, "id"), state.UserID, false)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Действие не найдено")
		return
	}
	if err != nil {
		writeActionError(w, err)
		return
	}
	s.audit(r.Context(), nil, nil, "action_job.cancelled", "action_job", chi.URLParam(r, "id"), clientIP(r), map[string]any{"deviceId": deviceID, "via": "mcp", "integrationTokenId": state.TokenID, "actorUserId": state.UserID})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
