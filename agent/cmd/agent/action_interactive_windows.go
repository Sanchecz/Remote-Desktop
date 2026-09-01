//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func executeInteractiveNetworkClient(ctx context.Context, protocol, host string, port int, username string) remoteJobResult {
	if err := validatePrivateLANHost(ctx, host); err != nil {
		return failedAction(err.Error())
	}
	if port < 1 || port > 65535 {
		return failedAction("порт внутреннего подключения недопустим")
	}
	var executable string
	var arguments []string
	switch protocol {
	case "rdp":
		executable = filepath.Join(os.Getenv("WINDIR"), "System32", "mstsc.exe")
		arguments = []string{"/v:" + net.JoinHostPort(host, strconv.Itoa(port))}
		if username != "" {
			arguments = append(arguments, "/prompt")
		}
	case "ssh":
		ssh := filepath.Join(os.Getenv("WINDIR"), "System32", "OpenSSH", "ssh.exe")
		if _, err := os.Stat(ssh); err != nil {
			return failedAction("встроенный OpenSSH Client не установлен в Windows")
		}
		target := host
		if username != "" {
			target = username + "@" + host
		}
		executable = filepath.Join(os.Getenv("WINDIR"), "System32", "cmd.exe")
		arguments = []string{"/D", "/K", ssh, "-p", strconv.Itoa(port), target}
	default:
		return failedAction("неподдерживаемый протокол внутреннего подключения")
	}
	if err := launchInBoundWindowsSession(executable, arguments...); err != nil {
		return failedAction(err.Error())
	}
	return remoteJobResult{Success: true, Output: fmt.Sprintf("%s-клиент открыт в закреплённом Windows-сеансе для %s:%d. Пароль RemoteIt не получал.", strings.ToUpper(protocol), host, port), ExitCode: 0}
}

func executeWindowsPrinterList(ctx context.Context) remoteJobResult {
	const script = `$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[Text.Encoding]::UTF8; $default=(Get-CimInstance Win32_Printer | Where-Object Default | Select-Object -First 1 -ExpandProperty Name); Get-Printer | Sort-Object Name | Select-Object Name,Type,DriverName,PortName,PrinterStatus,WorkOffline,@{Name='Default';Expression={$_.Name -eq $default}} | ConvertTo-Json -Depth 3`
	return runFixedActionCommand(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
}

func executeWindowsPrinterSettings(context.Context) remoteJobResult {
	explorer := filepath.Join(os.Getenv("WINDIR"), "explorer.exe")
	if err := launchInBoundWindowsSession(explorer, "ms-settings:printers"); err != nil {
		return failedAction(err.Error())
	}
	return remoteJobResult{Success: true, Output: "Параметры «Принтеры и сканеры» открыты в закреплённом Windows-сеансе.", ExitCode: 0}
}

func executeWindowsPrinterSetDefault(ctx context.Context, name string) remoteJobResult {
	if !safeAgentPrinterName.MatchString(strings.TrimSpace(name)) {
		return failedAction("имя принтера недопустимо")
	}
	const verifyScript = `$ErrorActionPreference='Stop'; & { param($n) Get-Printer -Name $n -ErrorAction Stop | Out-Null }`
	verified := runFixedActionCommand(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", verifyScript, name)
	if !verified.Success {
		return verified
	}
	rundll32 := filepath.Join(os.Getenv("WINDIR"), "System32", "rundll32.exe")
	if err := launchInBoundWindowsSession(rundll32, "printui.dll,PrintUIEntry", "/y", "/n", name); err != nil {
		return failedAction(err.Error())
	}
	return remoteJobResult{Success: true, Output: fmt.Sprintf("Windows получил команду назначить принтер «%s» по умолчанию в закреплённом пользовательском сеансе.", name), ExitCode: 0}
}

func executeWindowsScanFolderConfigure(ctx context.Context, path, shareName, principal string) remoteJobResult {
	if !safeAgentWindowsPath.MatchString(path) || strings.Contains(path, `..`) || !safeAgentShareName.MatchString(shareName) || strings.HasSuffix(shareName, "$") || !safeAgentWindowsPrincipal.MatchString(principal) {
		return failedAction("параметры папки сканов недопустимы")
	}
	const script = `$ErrorActionPreference='Stop'; & { param($p,$s,$u) $account=New-Object Security.Principal.NTAccount($u); $null=$account.Translate([Security.Principal.SecurityIdentifier]); New-Item -ItemType Directory -Path $p -Force | Out-Null; $existing=Get-SmbShare -Name $s -ErrorAction SilentlyContinue; if ($null -ne $existing -and $existing.Path -ne $p) { throw "Общий ресурс с таким именем уже указывает на другой путь" }; if ($null -eq $existing) { New-SmbShare -Name $s -Path $p -ChangeAccess $u -FolderEnumerationMode AccessBased | Out-Null } else { Grant-SmbShareAccess -Name $s -AccountName $u -AccessRight Change -Force | Out-Null }; $acl=Get-Acl $p; $rule=New-Object Security.AccessControl.FileSystemAccessRule($u,'Modify','ContainerInherit,ObjectInherit','None','Allow'); $acl.SetAccessRule($rule); Set-Acl -Path $p -AclObject $acl; [pscustomobject]@{Path=$p;ShareName=$s;UNC=('\\'+$env:COMPUTERNAME+'\\'+$s);Principal=$u} | ConvertTo-Json }`
	return runFixedActionCommand(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script, path, shareName, principal)
}

func validatePrivateLANHost(ctx context.Context, host string) error {
	addresses := []net.IP{}
	if parsed := net.ParseIP(host); parsed != nil {
		addresses = append(addresses, parsed)
	} else {
		if !safeAgentLANHost.MatchString(host) {
			return errors.New("адрес внутреннего узла недопустим")
		}
		resolved, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil || len(resolved) == 0 {
			return errors.New("внутреннее имя узла не разрешается через DNS Agent")
		}
		addresses = resolved
	}
	for _, address := range addresses {
		if address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
			continue
		}
		return errors.New("RDP/SSH через Agent разрешён только к внутренним адресам")
	}
	return nil
}

func launchInBoundWindowsSession(executable string, arguments ...string) error {
	sessions, err := activeWindowsSessions()
	if err != nil {
		return fmt.Errorf("закреплённый Windows-сеанс недоступен: %w", err)
	}
	if len(sessions) != 1 {
		return errors.New("нет единственного безопасно закреплённого Windows-сеанса")
	}
	var token windows.Token
	if err := windows.WTSQueryUserToken(sessions[0], &token); err != nil {
		return fmt.Errorf("токен закреплённого пользователя: %w", err)
	}
	defer token.Close()
	var environment *uint16
	if err := windows.CreateEnvironmentBlock(&environment, token, false); err != nil {
		return fmt.Errorf("окружение закреплённого пользователя: %w", err)
	}
	defer windows.DestroyEnvironmentBlock(environment)
	application, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{executable}, arguments...)))
	if err != nil {
		return err
	}
	desktop, _ := windows.UTF16PtrFromString(`winsta0\default`)
	workingDirectory, _ := windows.UTF16PtrFromString(filepath.Dir(executable))
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})), Desktop: desktop, Flags: windows.STARTF_USESHOWWINDOW, ShowWindow: windows.SW_SHOWNORMAL}
	var process windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NEW_PROCESS_GROUP)
	if err := windows.CreateProcessAsUser(token, application, commandLine, nil, nil, false, flags, environment, workingDirectory, &startup, &process); err != nil {
		return fmt.Errorf("запуск в закреплённом Windows-сеансе: %w", err)
	}
	windows.CloseHandle(process.Thread)
	windows.CloseHandle(process.Process)
	return nil
}
