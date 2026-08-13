package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	maxTransferSize   int64 = 10 * 1024 * 1024 * 1024
	transferChunkSize int64 = 8 * 1024 * 1024
	maxStagedSize     int64 = 20 * 1024 * 1024 * 1024
)

type fileTransfer struct {
	ID, DeviceID, Direction, Name, RemotePath, Status, Error string
	Size, Received                                           int64
	CreatedAt, ExpiresAt                                     time.Time
}

func (s *server) transferDataPath(id string) string {
	return filepath.Join(s.transferRoot, id+".data")
}

func validTransferID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for _, character := range id {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && character != '-' {
			return false
		}
	}
	return true
}

func (s *server) authenticateTransferAgent(w http.ResponseWriter, r *http.Request) (string, bool) {
	deviceID := strings.TrimSpace(r.Header.Get("X-Genesis-Device-Id"))
	authz := r.Header.Get("Authorization")
	if deviceID == "" || !strings.HasPrefix(authz, "Device ") {
		writeError(w, http.StatusUnauthorized, "Недействительные данные агента")
		return "", false
	}
	var stored []byte
	secret := strings.TrimSpace(strings.TrimPrefix(authz, "Device "))
	if err := s.db.QueryRow(r.Context(), `SELECT secret_hash FROM devices WHERE id=$1`, deviceID).Scan(&stored); err != nil || subtle.ConstantTimeCompare(tokenHash(secret), stored) != 1 {
		writeError(w, http.StatusUnauthorized, "Недействительные данные агента")
		return "", false
	}
	return deviceID, true
}

func (s *server) createFileTransfer(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	deviceID := chi.URLParam(r, "id")
	if !s.requireDeviceAccess(w, r, deviceID) {
		return
	}
	var in struct {
		Direction  string `json:"direction"`
		Name       string `json:"name"`
		RemotePath string `json:"remotePath"`
		Size       int64  `json:"size"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	in.Direction = strings.TrimSpace(in.Direction)
	in.Name = strings.TrimSpace(in.Name)
	in.RemotePath = strings.TrimSpace(in.RemotePath)
	if (in.Direction != "to_device" && in.Direction != "from_device") || in.RemotePath == "" || len([]rune(in.RemotePath)) > 4096 {
		writeError(w, http.StatusBadRequest, "Некорректные параметры передачи")
		return
	}
	if in.Name == "" || in.Name == "." || in.Name == ".." || len([]rune(in.Name)) > 255 || strings.ContainsAny(in.Name, `/\\`) {
		writeError(w, http.StatusBadRequest, "Недопустимое имя файла")
		return
	}
	if in.Size < 0 || in.Size > maxTransferSize {
		writeError(w, http.StatusBadRequest, "Размер файла не должен превышать 10 ГБ")
		return
	}
	var online bool
	if err := s.db.QueryRow(r.Context(), `SELECT last_seen>now()-interval '90 seconds' FROM devices WHERE id=$1`, deviceID).Scan(&online); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	} else if err != nil || !online {
		writeError(w, http.StatusConflict, "Агент устройства сейчас не в сети")
		return
	}
	var staged int64
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(sum(size_bytes),0) FROM remote_file_transfers WHERE expires_at>now() AND ((direction='to_device' AND status IN ('uploading','queued','transferring')) OR (direction='from_device' AND status IN ('queued','transferring','ready','completed')))`).Scan(&staged)
	if staged+in.Size > maxStagedSize {
		writeError(w, http.StatusInsufficientStorage, "На сервере уже выполняются крупные передачи. Завершите их и повторите попытку")
		return
	}
	status := "queued"
	if in.Direction == "to_device" {
		status = "uploading"
	}
	var transfer fileTransfer
	err := s.db.QueryRow(r.Context(), `INSERT INTO remote_file_transfers(device_id,created_by,direction,file_name,remote_path,size_bytes,status) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,created_at,expires_at`, deviceID, a.UserID, in.Direction, in.Name, in.RemotePath, in.Size, status).Scan(&transfer.ID, &transfer.CreatedAt, &transfer.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать передачу")
		return
	}
	s.audit(r.Context(), a, nil, "file_transfer.created", "device", deviceID, clientIP(r), map[string]any{"transferId": transfer.ID, "direction": in.Direction, "size": in.Size})
	writeJSON(w, http.StatusCreated, map[string]any{"id": transfer.ID, "status": status, "size": in.Size, "received": 0, "expiresAt": transfer.ExpiresAt})
}

func (s *server) loadUserTransfer(r *http.Request) (fileTransfer, error) {
	a := currentAuth(r)
	privileged := a.Role == "owner" || a.Role == "admin"
	var t fileTransfer
	err := s.db.QueryRow(r.Context(), `SELECT id,device_id,direction,file_name,remote_path,size_bytes,received_bytes,status,error_text,created_at,expires_at FROM remote_file_transfers WHERE id=$1 AND (created_by=$2 OR $3)`, chi.URLParam(r, "id"), a.UserID, privileged).Scan(&t.ID, &t.DeviceID, &t.Direction, &t.Name, &t.RemotePath, &t.Size, &t.Received, &t.Status, &t.Error, &t.CreatedAt, &t.ExpiresAt)
	if err == nil {
		allowed, accessErr := s.deviceAccessAllowed(r, t.DeviceID)
		if accessErr != nil {
			return t, accessErr
		}
		if !allowed {
			return t, errDeviceAccessLocked
		}
	}
	return t, err
}

func writeUserTransferLoadError(w http.ResponseWriter, t fileTransfer, err error) {
	if isDeviceAccessLocked(err) {
		writeDeviceAccessLocked(w, t.DeviceID)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Передача не найдена")
		return
	}
	writeError(w, http.StatusInternalServerError, "Не удалось получить передачу")
}

func (s *server) fileTransferStatus(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadUserTransfer(r)
	if err != nil {
		writeUserTransferLoadError(w, t, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": t.ID, "deviceId": t.DeviceID, "direction": t.Direction, "name": t.Name, "remotePath": t.RemotePath, "size": t.Size, "received": t.Received, "status": t.Status, "error": t.Error, "createdAt": t.CreatedAt, "expiresAt": t.ExpiresAt})
}

func requestOffset(r *http.Request) (int64, error) {
	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil || offset < 0 {
		return 0, errors.New("invalid offset")
	}
	return offset, nil
}

func appendTransferChunk(w http.ResponseWriter, r *http.Request, path string, expectedOffset, total int64) (int64, error) {
	if expectedOffset > total {
		return expectedOffset, errors.New("смещение превышает размер файла")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return expectedOffset, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return expectedOffset, err
	}
	if info.Size() != expectedOffset {
		return info.Size(), fmt.Errorf("ожидалось смещение %d, получено %d", info.Size(), expectedOffset)
	}
	if _, err = file.Seek(expectedOffset, io.SeekStart); err != nil {
		return expectedOffset, err
	}
	limited := io.LimitReader(r.Body, transferChunkSize+1)
	written, err := io.Copy(file, limited)
	if err != nil {
		return expectedOffset, err
	}
	if written > transferChunkSize || expectedOffset+written > total {
		return expectedOffset, errors.New("часть файла слишком большая")
	}
	if err = file.Sync(); err != nil {
		return expectedOffset, err
	}
	return expectedOffset + written, nil
}

func (s *server) uploadTransferChunk(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadUserTransfer(r)
	if err != nil {
		writeUserTransferLoadError(w, t, err)
		return
	}
	if t.Direction != "to_device" || t.Status != "uploading" {
		writeError(w, http.StatusConflict, "Передача не принимает данные")
		return
	}
	offset, err := requestOffset(r)
	if err != nil || offset != t.Received {
		writeError(w, http.StatusConflict, "Некорректное смещение части")
		return
	}
	next, err := appendTransferChunk(w, r, s.transferDataPath(t.ID), offset, t.Size)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err = s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET received_bytes=$1,updated_at=now() WHERE id=$2 AND status='uploading'`, next, t.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить прогресс")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": next, "size": t.Size})
}

func (s *server) readyFileTransfer(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadUserTransfer(r)
	if err != nil {
		writeUserTransferLoadError(w, t, err)
		return
	}
	if t.Direction == "to_device" && t.Status == "uploading" && t.Size == 0 {
		file, createErr := os.OpenFile(s.transferDataPath(t.ID), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if createErr == nil {
			createErr = file.Close()
		}
		if createErr != nil {
			writeError(w, http.StatusInternalServerError, "Не удалось подготовить пустой файл")
			return
		}
	}
	info, statErr := os.Stat(s.transferDataPath(t.ID))
	if t.Direction != "to_device" || t.Status != "uploading" || statErr != nil || info.Size() != t.Size {
		writeError(w, http.StatusConflict, "Файл загружен не полностью")
		return
	}
	_, err = s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET status='queued',received_bytes=size_bytes,updated_at=now() WHERE id=$1`, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось отправить файл агенту")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) downloadFileTransfer(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadUserTransfer(r)
	if err != nil {
		writeUserTransferLoadError(w, t, err)
		return
	}
	if t.Direction != "from_device" || (t.Status != "ready" && t.Status != "completed") {
		writeError(w, http.StatusConflict, "Файл ещё не готов")
		return
	}
	file, err := os.Open(s.transferDataPath(t.ID))
	if err != nil {
		writeError(w, http.StatusGone, "Файл передачи отсутствует")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != t.Size {
		writeError(w, http.StatusConflict, "Файл передачи повреждён")
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET status='completed',completed_at=COALESCE(completed_at,now()),updated_at=now() WHERE id=$1`, t.ID)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": t.Name}))
	w.Header().Set("Content-Length", strconv.FormatInt(t.Size, 10))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, t.Name, info.ModTime(), file)
}

func (s *server) cancelFileTransfer(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadUserTransfer(r)
	if err != nil {
		if isDeviceAccessLocked(err) {
			writeDeviceAccessLocked(w, t.DeviceID)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET status='cancelled',completed_at=now(),updated_at=now() WHERE id=$1 AND status NOT IN ('completed','cancelled','expired')`, t.ID)
	_ = os.Remove(s.transferDataPath(t.ID))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) agentNextFileTransfer(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateTransferAgent(w, r)
	if !ok {
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET status='queued',updated_at=now() WHERE device_id=$1 AND status='transferring' AND updated_at<now()-interval '5 minutes'`, deviceID)
	var t fileTransfer
	err := s.db.QueryRow(r.Context(), `UPDATE remote_file_transfers SET status='transferring',started_at=COALESCE(started_at,now()),updated_at=now() WHERE id=(SELECT id FROM remote_file_transfers WHERE device_id=$1 AND status='queued' AND expires_at>now() ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id,direction,file_name,remote_path,size_bytes,received_bytes,status`, deviceID).Scan(&t.ID, &t.Direction, &t.Name, &t.RemotePath, &t.Size, &t.Received, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось получить передачу")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": t.ID, "direction": t.Direction, "name": t.Name, "remotePath": t.RemotePath, "size": t.Size, "received": t.Received})
}

func (s *server) loadAgentTransfer(r *http.Request, deviceID string) (fileTransfer, error) {
	var t fileTransfer
	err := s.db.QueryRow(r.Context(), `SELECT id,device_id,direction,file_name,remote_path,size_bytes,received_bytes,status,error_text,created_at,expires_at FROM remote_file_transfers WHERE id=$1 AND device_id=$2`, chi.URLParam(r, "id"), deviceID).Scan(&t.ID, &t.DeviceID, &t.Direction, &t.Name, &t.RemotePath, &t.Size, &t.Received, &t.Status, &t.Error, &t.CreatedAt, &t.ExpiresAt)
	return t, err
}

func (s *server) agentFileTransferStatus(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateTransferAgent(w, r)
	if !ok {
		return
	}
	t, err := s.loadAgentTransfer(r, deviceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Передача не найдена")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": t.ID, "status": t.Status, "size": t.Size, "received": t.Received, "error": t.Error})
}

func (s *server) agentDownloadTransferChunk(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateTransferAgent(w, r)
	if !ok {
		return
	}
	t, err := s.loadAgentTransfer(r, deviceID)
	if err != nil || t.Direction != "to_device" || t.Status != "transferring" {
		writeError(w, http.StatusConflict, "Передача недоступна")
		return
	}
	offset, err := requestOffset(r)
	if err != nil || offset > t.Size {
		writeError(w, http.StatusBadRequest, "Некорректное смещение")
		return
	}
	file, err := os.Open(s.transferDataPath(t.ID))
	if err != nil {
		writeError(w, http.StatusGone, "Файл отсутствует")
		return
	}
	defer file.Close()
	_, _ = file.Seek(offset, io.SeekStart)
	remaining := t.Size - offset
	length := min(transferChunkSize, remaining)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("X-RemoteIt-Total", strconv.FormatInt(t.Size, 10))
	w.Header().Set("X-RemoteIt-Offset", strconv.FormatInt(offset, 10))
	_, _ = s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET updated_at=now() WHERE id=$1`, t.ID)
	_, _ = io.CopyN(w, file, length)
}

func (s *server) agentUploadTransferChunk(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateTransferAgent(w, r)
	if !ok {
		return
	}
	t, err := s.loadAgentTransfer(r, deviceID)
	if err != nil || t.Direction != "from_device" || t.Status != "transferring" {
		writeError(w, http.StatusConflict, "Передача недоступна")
		return
	}
	offset, err := requestOffset(r)
	if err != nil || offset != t.Received {
		writeError(w, http.StatusConflict, "Некорректное смещение части")
		return
	}
	next, err := appendTransferChunk(w, r, s.transferDataPath(t.ID), offset, t.Size)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, err = s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET received_bytes=$1,updated_at=now() WHERE id=$2 AND status='transferring'`, next, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить прогресс")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": next, "size": t.Size})
}

func (s *server) agentCompleteFileTransfer(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateTransferAgent(w, r)
	if !ok {
		return
	}
	t, err := s.loadAgentTransfer(r, deviceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Передача не найдена")
		return
	}
	expectedStatus := "completed"
	if t.Direction == "from_device" {
		expectedStatus = "ready"
	}
	// Completion is idempotent: an agent can safely retry when the first HTTP
	// response was lost after the server committed the transfer.
	if t.Status == expectedStatus || (t.Direction == "from_device" && t.Status == "completed") {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": t.Status})
		return
	}
	if t.Status != "transferring" {
		writeError(w, http.StatusConflict, "Передача недоступна")
		return
	}
	if t.Direction == "from_device" {
		info, statErr := os.Stat(s.transferDataPath(t.ID))
		if statErr != nil || info.Size() != t.Size {
			writeError(w, http.StatusConflict, "Файл передан не полностью")
			return
		}
	}
	result, err := s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET status=$1,received_bytes=size_bytes,completed_at=CASE WHEN $1='completed' THEN now() ELSE completed_at END,updated_at=now() WHERE id=$2 AND status='transferring'`, expectedStatus, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось завершить передачу")
		return
	}
	if result.RowsAffected() == 0 {
		latest, loadErr := s.loadAgentTransfer(r, deviceID)
		if loadErr == nil && (latest.Status == expectedStatus || (latest.Direction == "from_device" && latest.Status == "completed")) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": latest.Status})
			return
		}
		writeError(w, http.StatusConflict, "Передача уже изменила состояние")
		return
	}
	if t.Direction == "to_device" {
		_ = os.Remove(s.transferDataPath(t.ID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": expectedStatus})
}

func (s *server) agentCompleteFileTransferLegacy(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateTransferAgent(w, r)
	if !ok {
		return
	}
	t, err := s.loadAgentTransfer(r, deviceID)
	if err != nil || t.Status != "transferring" {
		writeError(w, http.StatusConflict, "Передача недоступна")
		return
	}
	status := "completed"
	if t.Direction == "from_device" {
		info, statErr := os.Stat(s.transferDataPath(t.ID))
		if statErr != nil || info.Size() != t.Size {
			writeError(w, http.StatusConflict, "Файл передан не полностью")
			return
		}
		status = "ready"
	} else {
		_ = os.Remove(s.transferDataPath(t.ID))
	}
	_, err = s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET status=$1,received_bytes=size_bytes,completed_at=CASE WHEN $1='completed' THEN now() ELSE completed_at END,updated_at=now() WHERE id=$2`, status, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось завершить передачу")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
}

func (s *server) agentFailFileTransfer(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateTransferAgent(w, r)
	if !ok {
		return
	}
	var in struct {
		Error string `json:"error"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	result, err := s.db.Exec(r.Context(), `UPDATE remote_file_transfers SET status='failed',error_text=$1,completed_at=now(),updated_at=now() WHERE id=$2 AND device_id=$3 AND status IN ('queued','transferring')`, truncate(in.Error, 4096), chi.URLParam(r, "id"), deviceID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "Передача недоступна")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) removeExpiredTransferFiles() {
	rows, err := s.db.Query(context.Background(), `UPDATE remote_file_transfers SET status='expired',completed_at=now(),updated_at=now() WHERE expires_at<=now() AND status NOT IN ('completed','cancelled','expired') RETURNING id`)
	if err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil && validTransferID(id) {
				_ = os.Remove(s.transferDataPath(id))
			}
		}
		rows.Close()
	}
	rows, err = s.db.Query(context.Background(), `DELETE FROM remote_file_transfers WHERE completed_at<now()-interval '7 days' RETURNING id`)
	if err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil && validTransferID(id) {
				_ = os.Remove(s.transferDataPath(id))
			}
		}
		rows.Close()
	}
}

func (s *server) removeDeviceTransferFiles(deviceID string) {
	rows, err := s.db.Query(context.Background(), `SELECT id FROM remote_file_transfers WHERE device_id=$1`, deviceID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && validTransferID(id) {
			_ = os.Remove(s.transferDataPath(id))
		}
	}
}
