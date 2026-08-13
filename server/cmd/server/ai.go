package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type aiDeviceContext struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Hostname        string    `json:"hostname"`
	OS              string    `json:"os"`
	OSVersion       string    `json:"osVersion"`
	AgentVersion    string    `json:"agentVersion"`
	PublicIP        string    `json:"publicIp"`
	LocalIPs        []string  `json:"localIps"`
	CPUModel        string    `json:"cpuModel"`
	CPULoadPercent  float64   `json:"cpuLoadPercent"`
	MemoryBytes     int64     `json:"memoryBytes"`
	MemoryUsedBytes int64     `json:"memoryUsedBytes"`
	DiskTotalBytes  int64     `json:"diskTotalBytes"`
	DiskFreeBytes   int64     `json:"diskFreeBytes"`
	UptimeSeconds   int64     `json:"uptimeSeconds"`
	Online          bool      `json:"online"`
	LastSeen        time.Time `json:"lastSeen"`
}

type aiCommandResult struct {
	ID       string `json:"id"`
	Command  string `json:"command"`
	Shell    string `json:"shell"`
	Output   string `json:"output"`
	Error    string `json:"error"`
	ExitCode *int   `json:"exitCode"`
}

type aiFinding struct {
	Level   string `json:"level"`
	Title   string `json:"title"`
	Details string `json:"details"`
}

type aiCommand struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Explanation          string `json:"explanation"`
	Shell                string `json:"shell"`
	Command              string `json:"command"`
	Risk                 string `json:"risk"`
	RequiresConfirmation bool   `json:"requiresConfirmation"`
}

type aiAnalysis struct {
	Mode            string            `json:"mode"`
	ModelConfigured bool              `json:"modelConfigured"`
	Summary         string            `json:"summary"`
	Findings        []aiFinding       `json:"findings"`
	Commands        []aiCommand       `json:"commands"`
	Evidence        []aiCommandResult `json:"evidence"`
	PrivacyNote     string            `json:"privacyNote"`
}

func (s *server) aiAnalyze(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "AI-диагностика недоступна пользователю только для просмотра")
		return
	}
	var in struct {
		DeviceID string            `json:"deviceId"`
		Question string            `json:"question"`
		Results  []aiCommandResult `json:"results"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.Question = strings.TrimSpace(in.Question)
	if in.DeviceID == "" || in.Question == "" {
		writeError(w, http.StatusBadRequest, "Выберите устройство и опишите задачу")
		return
	}
	if len([]rune(in.Question)) > 2000 || len(in.Results) > 16 {
		writeError(w, http.StatusBadRequest, "Запрос диагностики слишком большой")
		return
	}
	if !s.requireDeviceAccess(w, r, in.DeviceID) {
		return
	}
	device, err := s.loadAIDeviceContext(r.Context(), in.DeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось собрать контекст устройства")
		return
	}
	for index := range in.Results {
		in.Results[index].Output = truncate(in.Results[index].Output, 24000)
		in.Results[index].Error = truncate(in.Results[index].Error, 2000)
		in.Results[index].Command = truncate(in.Results[index].Command, 2048)
	}
	analysis := buildLocalAIAnalysis(device, in.Question, in.Results)
	if key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); key != "" {
		if enhanced, enhanceErr := requestOpenAIAnalysis(r.Context(), key, device, in.Question, analysis); enhanceErr == nil && strings.TrimSpace(enhanced) != "" {
			analysis.Mode = "openai"
			analysis.ModelConfigured = true
			analysis.Summary = strings.TrimSpace(enhanced)
			analysis.PrivacyNote = "Контекст выбранного устройства и результаты диагностики обработаны серверной AI-моделью. Команды выполняются только после подтверждения администратора."
		} else {
			logAIError(enhanceErr)
			analysis.ModelConfigured = true
		}
	}
	s.audit(r.Context(), a, nil, "ai.analysis_requested", "device", in.DeviceID, clientIP(r), map[string]any{"mode": analysis.Mode, "results": len(in.Results)})
	writeJSON(w, http.StatusOK, analysis)
}

func (s *server) loadAIDeviceContext(ctx context.Context, deviceID string) (aiDeviceContext, error) {
	var device aiDeviceContext
	err := s.db.QueryRow(ctx, `SELECT id,name,hostname,os,os_version,agent_version,COALESCE(host(public_ip),''),local_ips,cpu_model,cpu_load_percent,memory_bytes,memory_used_bytes,disk_total_bytes,disk_free_bytes,uptime_seconds,(last_seen>now()-interval '90 seconds'),last_seen FROM devices WHERE id=$1`, deviceID).Scan(
		&device.ID, &device.Name, &device.Hostname, &device.OS, &device.OSVersion, &device.AgentVersion, &device.PublicIP, &device.LocalIPs, &device.CPUModel, &device.CPULoadPercent, &device.MemoryBytes, &device.MemoryUsedBytes, &device.DiskTotalBytes, &device.DiskFreeBytes, &device.UptimeSeconds, &device.Online, &device.LastSeen,
	)
	return device, err
}

func buildLocalAIAnalysis(device aiDeviceContext, question string, results []aiCommandResult) aiAnalysis {
	analysis := aiAnalysis{
		Mode:        "local",
		Summary:     fmt.Sprintf("Контекст устройства %s собран. Запустите предложенную диагностику — RemoteIt сопоставит результаты и выделит вероятную причину.", device.Name),
		Findings:    make([]aiFinding, 0),
		Commands:    diagnosticCommands(device.OS, question),
		Evidence:    results,
		PrivacyNote: "Работает встроенная диагностика RemoteIt: данные не передаются внешней AI-модели. Для расширенного объяснения задайте OPENAI_API_KEY только на сервере.",
	}
	if !device.Online {
		analysis.Findings = append(analysis.Findings, aiFinding{Level: "error", Title: "Агент не в сети", Details: fmt.Sprintf("Последняя связь: %s. Удалённые команды будут поставлены в очередь, но не выполнятся до восстановления соединения.", device.LastSeen.Format("02.01.2006 15:04:05"))})
	}
	if device.CPULoadPercent >= 85 {
		analysis.Findings = append(analysis.Findings, aiFinding{Level: "warning", Title: "Высокая загрузка процессора", Details: fmt.Sprintf("Текущая загрузка CPU %.0f%%. Нужен список процессов, чтобы определить источник.", device.CPULoadPercent)})
	}
	if device.MemoryBytes > 0 {
		used := float64(device.MemoryUsedBytes) / float64(device.MemoryBytes) * 100
		if used >= 85 {
			analysis.Findings = append(analysis.Findings, aiFinding{Level: "warning", Title: "Высокое использование памяти", Details: fmt.Sprintf("Используется %.0f%% оперативной памяти (%s из %s).", used, humanBytes(device.MemoryUsedBytes), humanBytes(device.MemoryBytes))})
		}
	}
	if device.DiskTotalBytes > 0 {
		used := float64(device.DiskTotalBytes-device.DiskFreeBytes) / float64(device.DiskTotalBytes) * 100
		if used >= 90 {
			analysis.Findings = append(analysis.Findings, aiFinding{Level: "error", Title: "Заканчивается место на диске", Details: fmt.Sprintf("Диск заполнен на %.0f%%, свободно %s.", used, humanBytes(device.DiskFreeBytes))})
		}
	}
	if len(analysis.Findings) == 0 {
		analysis.Findings = append(analysis.Findings, aiFinding{Level: "ok", Title: "Критичных отклонений в телеметрии нет", Details: fmt.Sprintf("CPU %.0f%% · RAM %s/%s · свободно на диске %s · uptime %s.", device.CPULoadPercent, humanBytes(device.MemoryUsedBytes), humanBytes(device.MemoryBytes), humanBytes(device.DiskFreeBytes), humanDuration(device.UptimeSeconds))})
	}
	if len(results) > 0 {
		failed := 0
		for _, result := range results {
			if result.Error != "" || (result.ExitCode != nil && *result.ExitCode != 0) {
				failed++
				message := strings.TrimSpace(result.Error)
				if message == "" {
					message = excerpt(result.Output, 360)
				}
				analysis.Findings = append(analysis.Findings, aiFinding{Level: "warning", Title: "Одна из проверок завершилась с ошибкой", Details: message})
			}
		}
		if failed == 0 {
			analysis.Summary = fmt.Sprintf("Диагностика %s завершена: выполнено %d проверок без ошибок. Изучите собранные факты ниже; изменяющие действия всё равно требуют отдельного подтверждения.", device.Name, len(results))
		} else {
			analysis.Summary = fmt.Sprintf("Диагностика %s завершена: %d из %d проверок требуют внимания. Ошибки и вывод команд переданы в анализ.", device.Name, failed, len(results))
		}
	}
	return analysis
}

func diagnosticCommands(osName, question string) []aiCommand {
	osLower := strings.ToLower(osName)
	q := strings.ToLower(question)
	commands := make([]aiCommand, 0, 5)
	add := func(title, explanation, shell, command string) {
		commands = append(commands, aiCommand{ID: fmt.Sprintf("diag-%d", len(commands)+1), Title: title, Explanation: explanation, Shell: shell, Command: command, Risk: "read", RequiresConfirmation: false})
	}
	addChange := func(title, explanation, shell, command string) {
		commands = append(commands, aiCommand{ID: fmt.Sprintf("action-%d", len(commands)+1), Title: title, Explanation: explanation, Shell: shell, Command: command, Risk: "change", RequiresConfirmation: true})
	}
	windows := strings.Contains(osLower, "windows")
	mac := strings.Contains(osLower, "darwin") || strings.Contains(osLower, "mac")
	if windows {
		add("Состояние системы", "CPU, память, диски и время работы без изменения настроек.", "powershell", `$os=Get-CimInstance Win32_OperatingSystem; $cpu=Get-CimInstance Win32_Processor | Measure-Object LoadPercentage -Average; [pscustomobject]@{CPUPercent=[math]::Round($cpu.Average); RAMUsedGB=[math]::Round(($os.TotalVisibleMemorySize-$os.FreePhysicalMemory)/1MB,2); RAMTotalGB=[math]::Round($os.TotalVisibleMemorySize/1MB,2); Uptime=(Get-Date)-$os.LastBootUpTime}; Get-PSDrive -PSProvider FileSystem | Select-Object Name,@{n='UsedGB';e={[math]::Round($_.Used/1GB,2)}},@{n='FreeGB';e={[math]::Round($_.Free/1GB,2)}} | Format-Table -AutoSize`)
		if containsAny(q, "процесс", "cpu", "цп", "тормоз", "памят", "ram", "озу") {
			add("Самые ресурсоёмкие процессы", "Показывает лидеров по CPU и памяти.", "powershell", `Get-Process | Sort-Object CPU -Descending | Select-Object -First 15 Name,Id,@{n='CPU';e={[math]::Round($_.CPU,1)}},@{n='RAM_MB';e={[math]::Round($_.WorkingSet64/1MB,1)}} | Format-Table -AutoSize`)
		}
		if containsAny(q, "интернет", "сет", "dns", "шлюз", "маршрут", "порт") {
			add("Сеть, DNS и маршруты", "Проверяет конфигурацию, шлюз, DNS и доступ к интернету.", "powershell", `Get-NetIPConfiguration | Format-List InterfaceAlias,IPv4Address,IPv4DefaultGateway,DNSServer; Test-NetConnection 1.1.1.1 -InformationLevel Detailed; Resolve-DnsName supportgenesis.ru -ErrorAction Continue; route print`)
		}
		if containsAny(q, "rdp", "удален", "рабочий стол") {
			add("Готовность RDP", "Проверяет службу, порт и правила брандмауэра RDP.", "powershell", `Get-Service TermService; Get-NetTCPConnection -LocalPort 3389 -State Listen -ErrorAction SilentlyContinue; Get-NetFirewallRule -DisplayGroup 'Remote Desktop' -ErrorAction SilentlyContinue | Select-Object DisplayName,Enabled,Action`)
		}
		if containsAny(q, "ошиб", "журнал", "падает", "сбой") {
			add("Последние системные ошибки", "Читает последние критические события Windows.", "powershell", `Get-WinEvent -FilterHashtable @{LogName='System'; Level=1,2} -MaxEvents 25 -ErrorAction SilentlyContinue | Select-Object TimeCreated,ProviderName,Id,LevelDisplayName,Message | Format-List`)
		}
		if containsAny(q, "печать", "принтер", "spool") && containsAny(q, "перезап", "исправ", "запуст") {
			addChange("Перезапустить службу печати", "Остановит и снова запустит Windows Print Spooler.", "powershell", `Restart-Service Spooler -Force; Get-Service Spooler`)
		}
		if containsAny(q, "dns") && containsAny(q, "очист", "сброс", "исправ") {
			addChange("Очистить DNS-кэш", "Сбрасывает только локальный кэш DNS Windows.", "powershell", `Clear-DnsClientCache; Get-DnsClientCache | Select-Object -First 20`)
		}
		if containsAny(q, "temp", "временн") && containsAny(q, "очист", "удал") {
			addChange("Очистить временные файлы пользователя", "Удаляет доступные файлы только из профиля TEMP; занятые файлы пропускаются.", "powershell", `Get-ChildItem -LiteralPath $env:TEMP -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue`)
		}
	} else if mac {
		add("Состояние системы", "CPU, память, диски и время работы.", "zsh", `uptime; memory_pressure; df -h; ps -Ao pid,comm,%cpu,%mem -r | head -n 16`)
		if containsAny(q, "интернет", "сет", "dns", "шлюз", "маршрут", "порт") {
			add("Сеть, DNS и маршруты", "Проверяет интерфейсы, маршрут, DNS и соединения.", "zsh", `ifconfig; route -n get default; scutil --dns | head -n 120; ping -c 3 1.1.1.1; netstat -an | head -n 80`)
		}
		if containsAny(q, "ошиб", "журнал", "падает", "сбой") {
			add("Последние системные ошибки", "Читает ошибки unified log за последний час.", "zsh", `log show --last 1h --style compact --predicate 'messageType == error' | tail -n 80`)
		}
		if containsAny(q, "dns") && containsAny(q, "очист", "сброс", "исправ") {
			addChange("Очистить DNS-кэш", "Перезапускает локальный DNS-кэш macOS.", "zsh", `dscacheutil -flushcache; killall -HUP mDNSResponder`)
		}
	} else {
		add("Состояние системы", "CPU, память, диски, нагрузка и время работы.", "bash", `uptime; free -h; df -h; ps -eo pid,comm,%cpu,%mem --sort=-%cpu | head -n 16`)
		if containsAny(q, "интернет", "сет", "dns", "шлюз", "маршрут", "порт") {
			add("Сеть, DNS и маршруты", "Проверяет адреса, маршруты, DNS, ping и открытые порты.", "bash", `ip address; ip route; getent hosts supportgenesis.ru; ping -c 3 1.1.1.1; ss -tulpn | head -n 100`)
		}
		if containsAny(q, "nginx") {
			add("Диагностика nginx", "Проверяет конфигурацию, службу и последние сообщения журнала.", "bash", `nginx -t 2>&1; systemctl status nginx --no-pager; journalctl -u nginx -n 80 --no-pager`)
		}
		if containsAny(q, "ошиб", "журнал", "падает", "сбой") {
			add("Системные ошибки", "Читает ошибки текущей загрузки.", "bash", `journalctl -p err -b -n 100 --no-pager`)
		}
		if containsAny(q, "nginx") && containsAny(q, "перезап", "исправ", "запуст") {
			addChange("Перезапустить nginx", "Перезапустит службу после отдельного подтверждения.", "bash", `nginx -t && systemctl restart nginx && systemctl status nginx --no-pager`)
		}
		if containsAny(q, "dns") && containsAny(q, "очист", "сброс", "исправ") {
			addChange("Очистить DNS-кэш", "Очищает кэш systemd-resolved, если служба используется.", "bash", `resolvectl flush-caches; resolvectl statistics`)
		}
	}
	if len(commands) > 6 {
		commands = commands[:6]
	}
	return commands
}

func requestOpenAIAnalysis(ctx context.Context, key string, device aiDeviceContext, question string, local aiAnalysis) (string, error) {
	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = "gpt-5.6-terra"
	}
	contextJSON, _ := json.Marshal(map[string]any{"device": device, "question": question, "findings": local.Findings, "results": local.Evidence})
	body, _ := json.Marshal(map[string]any{
		"model":        model,
		"store":        false,
		"instructions": "Ты AI-администратор RemoteIt. Отвечай по-русски, кратко и по фактам. Используй только переданные метрики и результаты команд. Не придумывай значения. Сначала назови наиболее вероятную причину, затем доказательства и безопасный следующий шаг. Не выводи команды: RemoteIt показывает только проверенные команды из своей библиотеки. Никогда не утверждай, что действие уже выполнено.",
		"input":        string(contextJSON),
	})
	requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 25 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("OpenAI API HTTP %d: %s", response.StatusCode, excerpt(string(payload), 320))
	}
	var parsed struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", err
	}
	for _, item := range parsed.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return truncate(strings.TrimSpace(content.Text), 6000), nil
			}
		}
	}
	return "", errors.New("OpenAI API returned no output text")
}

func logAIError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "RemoteIt AI enhancement unavailable: %v\n", err)
	}
}

func containsAny(value string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}

func humanBytes(value int64) string {
	if value <= 0 {
		return "0 Б"
	}
	units := []string{"Б", "КБ", "МБ", "ГБ", "ТБ"}
	number := float64(value)
	index := 0
	for number >= 1024 && index < len(units)-1 {
		number /= 1024
		index++
	}
	return fmt.Sprintf("%.1f %s", number, units[index])
}

func humanDuration(seconds int64) string {
	if seconds <= 0 {
		return "неизвестно"
	}
	days := seconds / 86400
	hours := seconds % 86400 / 3600
	if days > 0 {
		return fmt.Sprintf("%d дн. %d ч.", days, hours)
	}
	return fmt.Sprintf("%d ч. %d мин.", hours, seconds%3600/60)
}

func excerpt(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Команда не вернула подробностей"
	}
	if len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit]) + "…"
}
