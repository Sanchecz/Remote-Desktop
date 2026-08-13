package main

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (s *server) confirmAgentUninstall(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.Header.Get("X-Genesis-Device-Id"))
	authorization := r.Header.Get("Authorization")
	if deviceID == "" || !strings.HasPrefix(authorization, "Device ") {
		writeError(w, http.StatusUnauthorized, "Недействительные данные устройства")
		return
	}
	secret := strings.TrimSpace(strings.TrimPrefix(authorization, "Device "))
	var stored []byte
	var pendingRemoval bool
	if err := s.db.QueryRow(r.Context(), `SELECT secret_hash,pending_removal FROM devices WHERE id=$1`, deviceID).Scan(&stored, &pendingRemoval); err != nil || subtle.ConstantTimeCompare(tokenHash(secret), stored) != 1 {
		writeError(w, http.StatusUnauthorized, "Недействительные данные устройства")
		return
	}
	var input struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	if !input.Success {
		message := truncate(strings.TrimSpace(input.Error), 4096)
		if message == "" {
			message = "Локальный модуль удаления не смог полностью очистить Agent"
		}
		if pendingRemoval {
			_, _ = s.db.Exec(r.Context(), `UPDATE devices SET pending_removal=false,updated_at=now() WHERE id=$1`, deviceID)
			_, _ = s.db.Exec(r.Context(), `UPDATE agent_jobs SET status='failed',error_text=$1,updated_at=now(),completed_at=now() WHERE id=(SELECT id FROM agent_jobs WHERE device_id=$2 AND job_type='uninstall' ORDER BY created_at DESC LIMIT 1)`, message, deviceID)
		}
		s.audit(r.Context(), nil, &deviceID, "device.uninstall_failed", "device", deviceID, clientIP(r), map[string]any{"error": message})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.removeDeviceTransferFiles(deviceID)
	// The same signed cleanup helper is used by a queued panel removal and by
	// Windows "Installed apps". In the latter case there is no pending panel
	// job, but a successful local cleanup must still remove the stale device row.
	result, err := s.db.Exec(r.Context(), `DELETE FROM devices WHERE id=$1`, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось подтвердить удаление устройства")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "Устройство уже удалено")
		return
	}
	s.audit(r.Context(), nil, nil, "device.uninstalled", "device", deviceID, clientIP(r), map[string]any{"confirmedByCleanup": true, "requestedFromPanel": pendingRemoval})
	w.WriteHeader(http.StatusNoContent)
}
