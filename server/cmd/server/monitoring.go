package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const monitoredAgentVersion = "1.0.40"

type monitoredDevice struct {
	ID, Name, OS, AgentVersion, InstallMode, RemoteError  string
	CPULoad, MemoryBytes, MemoryUsed, DiskTotal, DiskFree float64
	Privileged                                            bool
	LastSeen                                              time.Time
}

type deviceHealthSnapshot struct {
	DeviceID, DeviceName, Status                string
	CPULoad, MemoryUsedPercent, DiskFreePercent float64
	ProblemCodes, Problems                      []string
	CheckedAt                                   time.Time
}

func deviceHealthSeverityRank(status string) int {
	switch status {
	case "down":
		return 2
	case "warning":
		return 1
	default:
		return 0
	}
}

func evaluateMonitoredDevice(device monitoredDevice, now time.Time) deviceHealthSnapshot {
	snapshot := deviceHealthSnapshot{
		DeviceID: device.ID, DeviceName: device.Name, Status: "ok", CPULoad: math.Round(device.CPULoad*10) / 10,
		DiskFreePercent: 100, ProblemCodes: make([]string, 0), Problems: make([]string, 0), CheckedAt: now.UTC(),
	}
	addProblem := func(status, code, text string) {
		if deviceHealthSeverityRank(status) > deviceHealthSeverityRank(snapshot.Status) {
			snapshot.Status = status
		}
		snapshot.ProblemCodes = append(snapshot.ProblemCodes, code)
		snapshot.Problems = append(snapshot.Problems, text)
	}
	if now.Sub(device.LastSeen) > 90*time.Second {
		addProblem("down", "agent_offline", "Agent не отвечает больше 90 секунд")
		return snapshot
	}
	if device.MemoryBytes > 0 {
		snapshot.MemoryUsedPercent = math.Round(device.MemoryUsed/device.MemoryBytes*1000) / 10
	}
	if device.DiskTotal > 0 {
		snapshot.DiskFreePercent = math.Round(device.DiskFree/device.DiskTotal*1000) / 10
	}
	if device.CPULoad >= 95 {
		addProblem("down", "cpu_critical", fmt.Sprintf("CPU %.0f%%", device.CPULoad))
	} else if device.CPULoad >= 80 {
		addProblem("warning", "cpu_high", fmt.Sprintf("CPU %.0f%%", device.CPULoad))
	}
	if snapshot.MemoryUsedPercent >= 95 {
		addProblem("down", "memory_critical", fmt.Sprintf("RAM %.0f%%", snapshot.MemoryUsedPercent))
	} else if snapshot.MemoryUsedPercent >= 85 {
		addProblem("warning", "memory_high", fmt.Sprintf("RAM %.0f%%", snapshot.MemoryUsedPercent))
	}
	if device.DiskTotal > 0 && snapshot.DiskFreePercent <= 5 {
		addProblem("down", "disk_critical", fmt.Sprintf("Свободно %.0f%% диска", snapshot.DiskFreePercent))
	} else if device.DiskTotal > 0 && snapshot.DiskFreePercent <= 12 {
		addProblem("warning", "disk_low", fmt.Sprintf("Свободно %.0f%% диска", snapshot.DiskFreePercent))
	}
	if !semanticVersionAtLeast(device.AgentVersion, monitoredAgentVersion) {
		version := strings.TrimSpace(device.AgentVersion)
		if version == "" {
			version = "не определена"
		}
		addProblem("warning", "agent_outdated", "Версия Agent "+version+", требуется "+monitoredAgentVersion)
	}
	osName := strings.ToLower(device.OS)
	if !strings.Contains(osName, "android") && (!device.Privileged || strings.EqualFold(device.InstallMode, "user")) {
		addProblem("warning", "agent_unprivileged", "Agent работает без системных прав")
	}
	if message := strings.TrimSpace(device.RemoteError); message != "" {
		addProblem("down", "remote_screen_error", "Удалённый экран: "+truncate(message, 180))
	}
	return snapshot
}

func (s *server) loadMonitoredDevices(ctx context.Context) ([]monitoredDevice, error) {
	rows, err := s.db.Query(ctx, `SELECT d.id,d.name,d.os,d.agent_version,d.cpu_load_percent,d.memory_bytes,d.memory_used_bytes,d.disk_total_bytes,d.disk_free_bytes,d.install_mode,d.privileged,d.last_seen,COALESCE((SELECT r.agent_error FROM remote_desktop_sessions r WHERE r.device_id=d.id AND r.created_at>now()-interval '1 day' ORDER BY COALESCE(r.frame_at,r.created_at) DESC LIMIT 1),'') FROM devices d WHERE NOT d.pending_removal ORDER BY lower(d.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := make([]monitoredDevice, 0)
	for rows.Next() {
		var device monitoredDevice
		if err := rows.Scan(&device.ID, &device.Name, &device.OS, &device.AgentVersion, &device.CPULoad, &device.MemoryBytes, &device.MemoryUsed, &device.DiskTotal, &device.DiskFree, &device.InstallMode, &device.Privileged, &device.LastSeen, &device.RemoteError); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

type monitorTargetInput struct {
	Name            string `json:"name"`
	GatewayDeviceID string `json:"gatewayDeviceId"`
	Host            string `json:"host"`
	Ports           []int  `json:"ports"`
	SuccessPolicy   string `json:"successPolicy"`
	IntervalSeconds int    `json:"intervalSeconds"`
	Enabled         *bool  `json:"enabled,omitempty"`
}

func requireMonitoringAdmin(w http.ResponseWriter, r *http.Request) *authState {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Мониторинг доступен только владельцу и администраторам")
		return nil
	}
	return a
}

func normalizeMonitorTarget(in monitorTargetInput) (monitorTargetInput, error) {
	in.Name, in.GatewayDeviceID, in.Host = strings.TrimSpace(in.Name), strings.TrimSpace(in.GatewayDeviceID), strings.TrimSpace(in.Host)
	if len([]rune(in.Name)) < 1 || len([]rune(in.Name)) > 80 || in.GatewayDeviceID == "" {
		return in, errors.New("укажите название и Agent-шлюз")
	}
	address := net.ParseIP(in.Host)
	if address == nil || (!address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast()) {
		return in, errors.New("мониторить можно только внутренний IP-адрес")
	}
	in.Host = address.String()
	if len(in.Ports) < 1 || len(in.Ports) > 16 {
		return in, errors.New("укажите от 1 до 16 TCP-портов")
	}
	seen := make(map[int]bool)
	ports := make([]int, 0, len(in.Ports))
	for _, port := range in.Ports {
		if port < 1 || port > 65535 {
			return in, errors.New("порт должен быть от 1 до 65535")
		}
		if !seen[port] {
			seen[port] = true
			ports = append(ports, port)
		}
	}
	in.Ports = ports
	in.SuccessPolicy = strings.ToLower(strings.TrimSpace(in.SuccessPolicy))
	if in.SuccessPolicy == "" {
		in.SuccessPolicy = "all"
	}
	if in.SuccessPolicy != "any" && in.SuccessPolicy != "all" {
		return in, errors.New("политика портов должна быть any или all")
	}
	if in.IntervalSeconds == 0 {
		in.IntervalSeconds = 300
	}
	if in.IntervalSeconds < 60 || in.IntervalSeconds > 86400 {
		return in, errors.New("интервал должен быть от 1 минуты до 24 часов")
	}
	return in, nil
}

func (s *server) listMonitoring(w http.ResponseWriter, r *http.Request) {
	if requireMonitoringAdmin(w, r) == nil {
		return
	}
	devices, err := s.loadMonitoredDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить состояние компьютеров")
		return
	}
	deviceHealth := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		snapshot := evaluateMonitoredDevice(device, time.Now())
		deviceHealth = append(deviceHealth, map[string]any{
			"deviceId": snapshot.DeviceID, "deviceName": snapshot.DeviceName, "status": snapshot.Status,
			"cpuLoadPercent": snapshot.CPULoad, "memoryUsedPercent": snapshot.MemoryUsedPercent, "diskFreePercent": snapshot.DiskFreePercent,
			"problemCodes": snapshot.ProblemCodes, "problems": snapshot.Problems, "checkedAt": snapshot.CheckedAt,
		})
	}
	rows, err := s.db.Query(r.Context(), `SELECT t.id,t.name,t.gateway_device_id,d.name,host(t.host),t.ports,t.success_policy,t.interval_seconds,t.enabled,t.status,t.last_latency_ms,t.last_error,t.last_checked_at,t.next_probe_at,t.created_at FROM monitor_targets t JOIN devices d ON d.id=t.gateway_device_id ORDER BY lower(t.name)`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить цели мониторинга")
		return
	}
	defer rows.Close()
	targets := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, gatewayID, gatewayName, host, successPolicy, status, lastError string
		var ports []int32
		var interval int
		var enabled bool
		var latency int64
		var lastChecked *time.Time
		var nextProbe, created time.Time
		if rows.Scan(&id, &name, &gatewayID, &gatewayName, &host, &ports, &successPolicy, &interval, &enabled, &status, &latency, &lastError, &lastChecked, &nextProbe, &created) == nil {
			targets = append(targets, map[string]any{"id": id, "name": name, "gatewayDeviceId": gatewayID, "gatewayName": gatewayName, "host": host, "ports": ports, "successPolicy": successPolicy, "intervalSeconds": interval, "enabled": enabled, "status": status, "lastLatencyMs": latency, "lastError": lastError, "lastCheckedAt": lastChecked, "nextProbeAt": nextProbe, "createdAt": created})
		}
	}
	sampleRows, err := s.db.Query(r.Context(), `SELECT s.id,s.target_id,t.name,s.status,s.latency_ms,s.open_ports,s.error_text,s.checked_at FROM monitor_samples s JOIN monitor_targets t ON t.id=s.target_id ORDER BY s.checked_at DESC LIMIT 200`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить историю мониторинга")
		return
	}
	defer sampleRows.Close()
	samples := make([]map[string]any, 0)
	for sampleRows.Next() {
		var id, latency int64
		var targetID, targetName, status, errorText string
		var openPorts []int32
		var checked time.Time
		if sampleRows.Scan(&id, &targetID, &targetName, &status, &latency, &openPorts, &errorText, &checked) == nil {
			samples = append(samples, map[string]any{"id": id, "targetId": targetID, "targetName": targetName, "status": status, "latencyMs": latency, "openPorts": openPorts, "error": errorText, "checkedAt": checked})
		}
	}
	deviceSampleRows, err := s.db.Query(r.Context(), `SELECT s.id,s.device_id,d.name,s.status,s.cpu_load_percent,s.memory_used_percent,s.disk_free_percent,s.problem_codes,s.problem_text,s.checked_at FROM monitor_device_samples s JOIN devices d ON d.id=s.device_id ORDER BY s.checked_at DESC LIMIT 200`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить историю компьютеров")
		return
	}
	defer deviceSampleRows.Close()
	deviceSamples := make([]map[string]any, 0)
	for deviceSampleRows.Next() {
		var id int64
		var deviceID, deviceName, status string
		var cpuLoad, memoryUsed, diskFree float64
		var problemCodes, problems []string
		var checked time.Time
		if deviceSampleRows.Scan(&id, &deviceID, &deviceName, &status, &cpuLoad, &memoryUsed, &diskFree, &problemCodes, &problems, &checked) == nil {
			deviceSamples = append(deviceSamples, map[string]any{
				"id": id, "deviceId": deviceID, "deviceName": deviceName, "status": status, "cpuLoadPercent": cpuLoad,
				"memoryUsedPercent": memoryUsed, "diskFreePercent": diskFree, "problemCodes": problemCodes, "problems": problems, "checkedAt": checked,
			})
		}
	}
	var retentionDays int
	var sampleCount, alertCount, storageBytes int64
	_ = s.db.QueryRow(r.Context(), `SELECT retention_days FROM monitor_settings WHERE id=1`).Scan(&retentionDays)
	_ = s.db.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM monitor_samples)+(SELECT count(*) FROM monitor_device_samples),(SELECT count(*) FROM monitor_samples WHERE status<>'ok')+(SELECT count(*) FROM monitor_device_samples WHERE status<>'ok')`).Scan(&sampleCount, &alertCount)
	_ = s.db.QueryRow(r.Context(), `SELECT pg_total_relation_size('monitor_samples')+pg_total_relation_size('monitor_device_samples')`).Scan(&storageBytes)
	writeJSON(w, http.StatusOK, map[string]any{
		"deviceHealth": deviceHealth, "deviceSamples": deviceSamples, "targets": targets, "samples": samples,
		"settings": map[string]any{"retentionDays": retentionDays}, "storage": map[string]any{"sampleCount": sampleCount, "alertCount": alertCount, "bytes": storageBytes},
	})
}

func (s *server) createMonitorTarget(w http.ResponseWriter, r *http.Request) {
	a := requireMonitoringAdmin(w, r)
	if a == nil {
		return
	}
	var in monitorTargetInput
	if decodeJSON(w, r, &in) != nil {
		return
	}
	in, err := normalizeMonitorTarget(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.requireDeviceAccess(w, r, in.GatewayDeviceID) {
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	var id string
	err = s.db.QueryRow(r.Context(), `INSERT INTO monitor_targets(name,gateway_device_id,host,ports,success_policy,interval_seconds,enabled,created_by) SELECT $1,$2,$3::inet,$4,$5,$6,$7,$8 WHERE EXISTS(SELECT 1 FROM devices WHERE id=$2 AND NOT pending_removal) RETURNING id`, in.Name, in.GatewayDeviceID, in.Host, in.Ports, in.SuccessPolicy, in.IntervalSeconds, enabled, a.UserID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Agent-шлюз не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось добавить цель мониторинга")
		return
	}
	s.audit(r.Context(), a, nil, "monitor.target_created", "monitor_target", id, clientIP(r), map[string]any{"gatewayDeviceId": in.GatewayDeviceID, "host": in.Host, "ports": in.Ports, "successPolicy": in.SuccessPolicy})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *server) updateMonitorTarget(w http.ResponseWriter, r *http.Request) {
	a := requireMonitoringAdmin(w, r)
	if a == nil {
		return
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if in.Enabled == nil {
		writeError(w, http.StatusBadRequest, "Не указано состояние мониторинга")
		return
	}
	id := chi.URLParam(r, "id")
	result, err := s.db.Exec(r.Context(), `UPDATE monitor_targets SET enabled=$1,next_probe_at=CASE WHEN $1 THEN now() ELSE next_probe_at END,updated_at=now() WHERE id=$2`, *in.Enabled, id)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Цель мониторинга не найдена")
		return
	}
	s.audit(r.Context(), a, nil, "monitor.target_updated", "monitor_target", id, clientIP(r), map[string]any{"enabled": *in.Enabled})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) deleteMonitorTarget(w http.ResponseWriter, r *http.Request) {
	a := requireMonitoringAdmin(w, r)
	if a == nil {
		return
	}
	id := chi.URLParam(r, "id")
	result, err := s.db.Exec(r.Context(), `DELETE FROM monitor_targets WHERE id=$1`, id)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Цель мониторинга не найдена")
		return
	}
	s.audit(r.Context(), a, nil, "monitor.target_deleted", "monitor_target", id, clientIP(r), map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) probeMonitorTarget(w http.ResponseWriter, r *http.Request) {
	a := requireMonitoringAdmin(w, r)
	if a == nil {
		return
	}
	id := chi.URLParam(r, "id")
	result, err := s.db.Exec(r.Context(), `UPDATE monitor_targets SET next_probe_at=now(),updated_at=now() WHERE id=$1`, id)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Цель мониторинга не найдена")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}

func (s *server) updateMonitoringSettings(w http.ResponseWriter, r *http.Request) {
	a := requireMonitoringAdmin(w, r)
	if a == nil {
		return
	}
	var in struct {
		RetentionDays int `json:"retentionDays"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if in.RetentionDays != 0 && in.RetentionDays != 1 && in.RetentionDays != 7 && in.RetentionDays != 30 && in.RetentionDays != 90 {
		writeError(w, http.StatusBadRequest, "Недопустимый срок хранения")
		return
	}
	_, err := s.db.Exec(r.Context(), `UPDATE monitor_settings SET retention_days=$1,updated_at=now() WHERE id=1`, in.RetentionDays)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить срок хранения")
		return
	}
	s.pruneMonitoringHistory(r.Context())
	s.audit(r.Context(), a, nil, "monitor.retention_updated", "monitoring", "history", clientIP(r), map[string]any{"retentionDays": in.RetentionDays})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) clearMonitoringHistory(w http.ResponseWriter, r *http.Request) {
	a := requireMonitoringAdmin(w, r)
	if a == nil {
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось очистить историю мониторинга")
		return
	}
	defer tx.Rollback(r.Context())
	networkResult, err := tx.Exec(r.Context(), `DELETE FROM monitor_samples`)
	if err == nil {
		deviceResult, deviceErr := tx.Exec(r.Context(), `DELETE FROM monitor_device_samples`)
		if deviceErr == nil {
			deleted := networkResult.RowsAffected() + deviceResult.RowsAffected()
			if err = tx.Commit(r.Context()); err == nil {
				s.audit(r.Context(), a, nil, "monitor.history_cleared", "monitoring", "history", clientIP(r), map[string]any{"deletedSamples": deleted})
				writeJSON(w, http.StatusOK, map[string]any{"deletedSamples": deleted})
				return
			}
		}
	}
	writeError(w, http.StatusInternalServerError, "Не удалось очистить историю мониторинга")
}

func (s *server) pruneMonitoringHistory(ctx context.Context) {
	var retentionDays int
	if s.db.QueryRow(ctx, `SELECT retention_days FROM monitor_settings WHERE id=1`).Scan(&retentionDays) != nil || retentionDays == 0 {
		return
	}
	_, _ = s.db.Exec(ctx, `DELETE FROM monitor_samples WHERE checked_at<now()-make_interval(days=>$1)`, retentionDays)
	_, _ = s.db.Exec(ctx, `DELETE FROM monitor_device_samples WHERE checked_at<now()-make_interval(days=>$1)`, retentionDays)
}

func (s *server) runMonitoring(ctx context.Context) {
	lastDeviceSample := time.Time{}
	run := func() {
		s.collectMonitoringResults(ctx)
		s.queueDueMonitoringProbes(ctx)
		if lastDeviceSample.IsZero() || time.Since(lastDeviceSample) >= time.Minute {
			s.collectDeviceHealthSamples(ctx)
			lastDeviceSample = time.Now()
		}
	}
	run()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (s *server) collectDeviceHealthSamples(ctx context.Context) {
	devices, err := s.loadMonitoredDevices(ctx)
	if err != nil {
		return
	}
	type previousSample struct {
		status, codes string
		checkedAt     time.Time
	}
	previous := make(map[string]previousSample)
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (device_id) device_id,status,array_to_string(problem_codes,'|'),checked_at FROM monitor_device_samples ORDER BY device_id,checked_at DESC`)
	if err == nil {
		for rows.Next() {
			var deviceID string
			var sample previousSample
			if rows.Scan(&deviceID, &sample.status, &sample.codes, &sample.checkedAt) == nil {
				previous[deviceID] = sample
			}
		}
		rows.Close()
	}
	now := time.Now()
	for _, device := range devices {
		snapshot := evaluateMonitoredDevice(device, now)
		last, exists := previous[device.ID]
		codes := strings.Join(snapshot.ProblemCodes, "|")
		if exists && last.status == snapshot.Status && last.codes == codes && now.Sub(last.checkedAt) < 30*time.Minute {
			continue
		}
		_, _ = s.db.Exec(ctx, `INSERT INTO monitor_device_samples(device_id,status,cpu_load_percent,memory_used_percent,disk_free_percent,problem_codes,problem_text,checked_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			snapshot.DeviceID, snapshot.Status, snapshot.CPULoad, snapshot.MemoryUsedPercent, snapshot.DiskFreePercent, snapshot.ProblemCodes, snapshot.Problems, snapshot.CheckedAt)
	}
}

func (s *server) queueDueMonitoringProbes(ctx context.Context) {
	var ownerID, ownerName string
	if s.db.QueryRow(ctx, `SELECT id,username FROM users WHERE role='owner' AND NOT disabled ORDER BY created_at LIMIT 1`).Scan(&ownerID, &ownerName) != nil {
		return
	}
	rows, err := s.db.Query(ctx, `SELECT t.id,t.gateway_device_id,host(t.host),t.ports,t.interval_seconds FROM monitor_targets t JOIN devices d ON d.id=t.gateway_device_id WHERE t.enabled AND t.next_probe_at<=now() AND d.last_seen>now()-interval '90 seconds' AND NOT d.pending_removal AND NOT EXISTS(SELECT 1 FROM action_jobs a WHERE a.id=t.last_action_job_id AND a.status IN ('awaiting_approval','queued','running')) ORDER BY t.next_probe_at LIMIT 20`)
	if err != nil {
		return
	}
	defer rows.Close()
	type dueTarget struct {
		id, deviceID, host string
		ports              []int32
		interval           int
	}
	due := make([]dueTarget, 0)
	for rows.Next() {
		var target dueTarget
		if rows.Scan(&target.id, &target.deviceID, &target.host, &target.ports, &target.interval) == nil {
			due = append(due, target)
		}
	}
	for _, target := range due {
		ports := make([]any, 0, len(target.ports))
		for _, port := range target.ports {
			ports = append(ports, int(port))
		}
		result, createErr := s.createAction(ctx, actionActor{UserID: ownerID, Username: ownerName, Role: "owner", Via: "web"}, actionCreateInput{DeviceID: target.deviceID, Action: "diagnostic.tcp_probe", Parameters: map[string]any{"host": target.host, "ports": ports}, IdempotencyKey: fmt.Sprintf("monitor-%s-%d", target.id, time.Now().Unix()/20)})
		if createErr != nil {
			_, _ = s.db.Exec(ctx, `UPDATE monitor_targets SET status='unknown',last_error=$1,next_probe_at=now()+make_interval(secs=>$2),updated_at=now() WHERE id=$3`, createErr.Error(), target.interval, target.id)
			continue
		}
		_, _ = s.db.Exec(ctx, `UPDATE monitor_targets SET last_action_job_id=$1,next_probe_at=now()+make_interval(secs=>$2),last_error='',updated_at=now() WHERE id=$3`, result["id"], target.interval, target.id)
	}
}

func (s *server) collectMonitoringResults(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT t.id,t.success_policy,cardinality(t.ports),a.id,a.status,a.output,a.error_text FROM monitor_targets t JOIN action_jobs a ON a.id=t.last_action_job_id WHERE t.processed_action_job_id IS DISTINCT FROM t.last_action_job_id AND a.status IN ('succeeded','failed','cancelled','expired') LIMIT 100`)
	if err != nil {
		return
	}
	defer rows.Close()
	type completed struct {
		targetID, policy, actionID, status, output, errorText string
		expectedPorts                                         int
	}
	items := make([]completed, 0)
	for rows.Next() {
		var item completed
		if rows.Scan(&item.targetID, &item.policy, &item.expectedPorts, &item.actionID, &item.status, &item.output, &item.errorText) == nil {
			items = append(items, item)
		}
	}
	for _, item := range items {
		status, errorText, latency := "down", item.errorText, int64(0)
		openPorts := make([]int, 0)
		if item.status == "succeeded" {
			var payload struct {
				DurationMS int64 `json:"durationMs"`
				Ports      []struct {
					Port      int   `json:"port"`
					Open      bool  `json:"open"`
					LatencyMS int64 `json:"latencyMs"`
				} `json:"ports"`
			}
			if json.Unmarshal([]byte(item.output), &payload) == nil {
				latency, errorText = payload.DurationMS, ""
				minimumOpenLatency := int64(0)
				for _, port := range payload.Ports {
					if port.Open {
						openPorts = append(openPorts, port.Port)
						if minimumOpenLatency == 0 || port.LatencyMS < minimumOpenLatency {
							minimumOpenLatency = port.LatencyMS
						}
					}
				}
				if minimumOpenLatency > 0 {
					latency = minimumOpenLatency
				}
				switch {
				case len(openPorts) == 0:
					status, errorText = "down", "Узел или выбранные службы не отвечают"
				case item.policy == "any":
					status = "ok"
				case len(openPorts) == item.expectedPorts:
					status = "ok"
				default:
					status, errorText = "warning", "Часть обязательных портов недоступна"
				}
			} else {
				errorText = "Agent вернул повреждённый результат"
			}
		}
		if status != "ok" && status != "warning" && status != "down" {
			status = "down"
		}
		_, _ = s.db.Exec(ctx, `INSERT INTO monitor_samples(target_id,status,latency_ms,open_ports,error_text) VALUES($1,$2,$3,$4,$5)`, item.targetID, status, latency, openPorts, errorText)
		_, _ = s.db.Exec(ctx, `UPDATE monitor_targets SET processed_action_job_id=$1,status=$2,last_latency_ms=$3,last_error=$4,last_checked_at=now(),updated_at=now() WHERE id=$5`, item.actionID, status, latency, errorText, item.targetID)
	}
}
