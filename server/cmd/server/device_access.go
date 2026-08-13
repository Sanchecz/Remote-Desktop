package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

var errDeviceAccessLocked = errors.New("device access is locked")

const deviceUnlockLifetime = 4 * time.Hour

func (s *server) deviceAccessAllowed(r *http.Request, deviceID string) (bool, error) {
	a := currentAuth(r)
	var allowed bool
	err := s.db.QueryRow(r.Context(), `SELECT ($2='owner' OR access_password_hash='' OR EXISTS(SELECT 1 FROM device_access_unlocks u WHERE u.device_id=devices.id AND u.session_id=$3 AND u.expires_at>now())) FROM devices WHERE id=$1`, deviceID, a.Role, a.SessionID).Scan(&allowed)
	return allowed, err
}

func writeDeviceAccessLocked(w http.ResponseWriter, deviceID string) {
	writeJSON(w, http.StatusLocked, map[string]any{
		"error":    "Устройство защищено владельцем RemoteIt. Введите пароль доступа в панели.",
		"code":     "DEVICE_LOCKED",
		"deviceId": deviceID,
	})
}

func (s *server) requireDeviceAccess(w http.ResponseWriter, r *http.Request, deviceID string) bool {
	allowed, err := s.deviceAccessAllowed(r, deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить доступ к устройству")
		return false
	}
	if !allowed {
		writeDeviceAccessLocked(w, deviceID)
		return false
	}
	return true
}

func validDeviceAccessPassword(password string) bool {
	length := len([]rune(password))
	return length >= 8 && length <= 128
}

func deviceUnlockAttemptKey(r *http.Request, a *authState, deviceID string) string {
	return fmt.Sprintf("device-unlock:%s:%s:%s", clientIP(r), a.UserID, deviceID)
}

func (s *server) setDeviceAccessProtection(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" {
		writeError(w, http.StatusForbidden, "Защиту устройств может менять только главный владелец RemoteIt")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	if !validDeviceAccessPassword(input.Password) {
		writeError(w, http.StatusBadRequest, "Пароль устройства должен содержать от 8 до 128 символов")
		return
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось защитить пароль устройства")
		return
	}
	deviceID := chi.URLParam(r, "id")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось включить защиту устройства")
		return
	}
	defer tx.Rollback(r.Context())
	var wasProtected bool
	if err = tx.QueryRow(r.Context(), `SELECT access_password_hash<>'' FROM devices WHERE id=$1 FOR UPDATE`, deviceID).Scan(&wasProtected); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить устройство")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE devices SET access_password_hash=$1,access_protected_at=now(),pending_removal=false,updated_at=now() WHERE id=$2`, hash, deviceID); err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM device_access_unlocks WHERE device_id=$1`, deviceID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE remote_desktop_sessions d SET status='ended',frame=NULL,ended_at=now() FROM users u WHERE d.created_by=u.id AND d.device_id=$1 AND d.status='active' AND u.role<>'owner'`, deviceID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM remote_desktop_inputs WHERE session_id IN (SELECT id FROM remote_desktop_sessions WHERE device_id=$1)`, deviceID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE agent_jobs SET status='cancelled',error_text='Отменено при изменении защиты устройства',completed_at=now(),updated_at=now() WHERE device_id=$1 AND status IN ('queued','running')`, deviceID)
	}
	var cancelledTransfers []string
	if err == nil {
		rows, updateErr := tx.Query(r.Context(), `UPDATE remote_file_transfers SET status='cancelled',error_text='Отменено при изменении защиты устройства',completed_at=now(),updated_at=now() WHERE device_id=$1 AND status IN ('uploading','queued','transferring','ready') RETURNING id`, deviceID)
		if updateErr != nil {
			err = updateErr
		} else {
			for rows.Next() {
				var id string
				if scanErr := rows.Scan(&id); scanErr != nil {
					err = scanErr
					break
				}
				cancelledTransfers = append(cancelledTransfers, id)
			}
			if rowsErr := rows.Err(); err == nil && rowsErr != nil {
				err = rowsErr
			}
			rows.Close()
		}
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось включить защиту устройства")
		return
	}
	for _, transferID := range cancelledTransfers {
		_ = os.Remove(s.transferDataPath(transferID))
	}
	s.deleteDesktopFramesForDevice(deviceID)
	eventType := "device.access_protection.enabled"
	if wasProtected {
		eventType = "device.access_protection.password_changed"
	}
	s.audit(r.Context(), a, nil, eventType, "device", deviceID, clientIP(r), map[string]any{"revokedUnlocks": true, "endedNonOwnerDesktopSessions": true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "accessProtected": true, "accessGranted": true})
}

func (s *server) removeDeviceAccessProtection(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" {
		writeError(w, http.StatusForbidden, "Защиту устройств может менять только главный владелец RemoteIt")
		return
	}
	deviceID := chi.URLParam(r, "id")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось снять защиту устройства")
		return
	}
	defer tx.Rollback(r.Context())
	result, err := tx.Exec(r.Context(), `UPDATE devices SET access_password_hash='',access_protected_at=NULL,updated_at=now() WHERE id=$1`, deviceID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM device_access_unlocks WHERE device_id=$1`, deviceID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось снять защиту устройства")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось снять защиту устройства")
		return
	}
	s.audit(r.Context(), a, nil, "device.access_protection.disabled", "device", deviceID, clientIP(r), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "accessProtected": false, "accessGranted": true})
}

func (s *server) unlockDeviceAccess(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "Недостаточно прав для работы с устройством")
		return
	}
	deviceID := chi.URLParam(r, "id")
	if a.Role == "owner" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "accessGranted": true, "ownerBypass": true})
		return
	}
	attemptKey := deviceUnlockAttemptKey(r, a, deviceID)
	if s.loginBlocked(attemptKey) {
		writeError(w, http.StatusTooManyRequests, "Слишком много попыток. Повторите через 15 минут.")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	var passwordHash string
	if err := s.db.QueryRow(r.Context(), `SELECT access_password_hash FROM devices WHERE id=$1`, deviceID).Scan(&passwordHash); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить пароль устройства")
		return
	}
	if passwordHash == "" {
		s.clearLoginFailures(attemptKey)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "accessGranted": true})
		return
	}
	if !verifyPassword(input.Password, passwordHash) {
		s.recordLoginFailure(attemptKey)
		s.audit(r.Context(), a, nil, "device.access_unlock.failed", "device", deviceID, clientIP(r), map[string]any{})
		writeError(w, http.StatusUnauthorized, "Неверный пароль устройства")
		return
	}
	s.clearLoginFailures(attemptKey)
	expiresAt := time.Now().Add(deviceUnlockLifetime)
	_, err := s.db.Exec(r.Context(), `INSERT INTO device_access_unlocks(device_id,session_id,expires_at) VALUES($1,$2,$3) ON CONFLICT(device_id,session_id) DO UPDATE SET unlocked_at=now(),expires_at=EXCLUDED.expires_at`, deviceID, a.SessionID, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось разблокировать устройство")
		return
	}
	s.audit(r.Context(), a, nil, "device.access_unlock.succeeded", "device", deviceID, clientIP(r), map[string]any{"expiresAt": expiresAt})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "accessGranted": true, "expiresAt": expiresAt})
}

func (s *server) lockDeviceAccess(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	deviceID := chi.URLParam(r, "id")
	if a.Role == "owner" {
		writeError(w, http.StatusBadRequest, "Главный владелец RemoteIt всегда имеет доступ к защищённым устройствам")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось заблокировать устройство")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `DELETE FROM device_access_unlocks WHERE device_id=$1 AND session_id=$2`, deviceID, a.SessionID); err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE remote_desktop_sessions SET status='ended',frame=NULL,ended_at=now() WHERE device_id=$1 AND created_by=$2 AND status='active'`, deviceID, a.UserID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM remote_desktop_inputs WHERE session_id IN (SELECT id FROM remote_desktop_sessions WHERE device_id=$1 AND created_by=$2)`, deviceID, a.UserID)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось заблокировать устройство")
		return
	}
	s.deleteDesktopFramesForDevice(deviceID)
	s.audit(r.Context(), a, nil, "device.access_locked", "device", deviceID, clientIP(r), map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

func isDeviceAccessLocked(err error) bool {
	return errors.Is(err, errDeviceAccessLocked) || strings.Contains(err.Error(), errDeviceAccessLocked.Error())
}
