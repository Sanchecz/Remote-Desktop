package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
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

const boundWindowsAgentMagic = "GENITB01"

type publicInstallToken struct {
	ID        string
	Name      string
	Group     string
	Token     string
	Uses      int
	MaxUses   int
	ExpiresAt time.Time
}

func (s *server) activePublicInstallToken(ctx context.Context, code string) (publicInstallToken, error) {
	code = strings.TrimSpace(code)
	if len(code) < 24 || len(code) > 512 || strings.ContainsAny(code, "\r\n\x00") {
		return publicInstallToken{}, pgx.ErrNoRows
	}
	var token publicInstallToken
	var disabled bool
	err := s.db.QueryRow(ctx, `SELECT id,name,device_group,uses,max_uses,expires_at,disabled FROM enrollment_tokens WHERE token_hash=$1`, tokenHash(code)).Scan(&token.ID, &token.Name, &token.Group, &token.Uses, &token.MaxUses, &token.ExpiresAt, &disabled)
	if err != nil {
		return publicInstallToken{}, err
	}
	if disabled || time.Now().After(token.ExpiresAt) || token.Uses >= token.MaxUses {
		return publicInstallToken{}, errors.New("install code is inactive")
	}
	token.Token = code
	return token, nil
}

func decodePublicInstallCode(w http.ResponseWriter, r *http.Request) (string, bool) {
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return "", false
	}
	return strings.TrimSpace(input.Code), true
}

func (s *server) resolvePublicInstallCode(w http.ResponseWriter, r *http.Request) {
	code, ok := decodePublicInstallCode(w, r)
	if !ok {
		return
	}
	token, err := s.activePublicInstallToken(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusNotFound, "Код установки недействителен, отозван или исчерпал лимит")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": token.Name, "group": token.Group, "remaining": token.MaxUses - token.Uses, "expiresAt": token.ExpiresAt})
}

func (s *server) downloadPublicWindowsAgent(w http.ResponseWriter, r *http.Request) {
	code, ok := decodePublicInstallCode(w, r)
	if !ok {
		return
	}
	token, err := s.activePublicInstallToken(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusNotFound, "Код установки недействителен, отозван или исчерпал лимит")
		return
	}
	if s.streamBoundWindowsAgent(w, r, token.ID, token.Token) {
		s.audit(r.Context(), nil, nil, "enrollment.public_agent_downloaded", "enrollment_token", token.ID, clientIP(r), map[string]any{"platform": "windows"})
	}
}

func (s *server) downloadPublicUnixAgent(w http.ResponseWriter, r *http.Request) {
	code, ok := decodePublicInstallCode(w, r)
	if !ok {
		return
	}
	token, err := s.activePublicInstallToken(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusNotFound, "Код установки недействителен, отозван или исчерпал лимит")
		return
	}
	if s.streamBoundUnixAgent(w, token.Token) {
		s.audit(r.Context(), nil, nil, "enrollment.public_agent_downloaded", "enrollment_token", token.ID, clientIP(r), map[string]any{"platform": "unix"})
	}
}

func (s *server) downloadPublicMacOSAgent(w http.ResponseWriter, r *http.Request) {
	code, ok := decodePublicInstallCode(w, r)
	if !ok {
		return
	}
	token, err := s.activePublicInstallToken(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusNotFound, "Код установки недействителен, отозван или исчерпал лимит")
		return
	}
	if s.streamBoundMacOSAgent(w, token.Token) {
		s.audit(r.Context(), nil, nil, "enrollment.public_agent_downloaded", "enrollment_token", token.ID, clientIP(r), map[string]any{"platform": "macos"})
	}
}

func (s *server) downloadBoundUnixAgent(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	tokenID := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "id")))
	if !validTransferID(tokenID) {
		writeError(w, http.StatusBadRequest, "Некорректный идентификатор токена")
		return
	}
	var token string
	var disabled bool
	var expiresAt time.Time
	var uses, maxUses int
	err := s.db.QueryRow(r.Context(), `SELECT COALESCE(token_value,''),disabled,expires_at,uses,max_uses FROM enrollment_tokens WHERE id=$1`, tokenID).Scan(&token, &disabled, &expiresAt, &uses, &maxUses)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Токен не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось подготовить Unix Agent")
		return
	}
	if token == "" {
		writeError(w, http.StatusConflict, "Для старого токена невозможно собрать готовый Agent")
		return
	}
	if disabled || time.Now().After(expiresAt) || uses >= maxUses {
		writeError(w, http.StatusConflict, "Токен выключен, истёк или уже использован полностью")
		return
	}
	if s.streamBoundUnixAgent(w, token) {
		s.audit(r.Context(), a, nil, "enrollment.agent_downloaded", "enrollment_token", tokenID, clientIP(r), map[string]any{"platform": "unix"})
	}
}

func (s *server) downloadBoundMacOSAgent(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	tokenID := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "id")))
	if !validTransferID(tokenID) {
		writeError(w, http.StatusBadRequest, "Некорректный идентификатор токена")
		return
	}
	var token string
	var disabled bool
	var expiresAt time.Time
	var uses, maxUses int
	err := s.db.QueryRow(r.Context(), `SELECT COALESCE(token_value,''),disabled,expires_at,uses,max_uses FROM enrollment_tokens WHERE id=$1`, tokenID).Scan(&token, &disabled, &expiresAt, &uses, &maxUses)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Токен не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось подготовить macOS Agent")
		return
	}
	if token == "" {
		writeError(w, http.StatusConflict, "Для старого токена невозможно собрать готовый Agent")
		return
	}
	if disabled || time.Now().After(expiresAt) || uses >= maxUses {
		writeError(w, http.StatusConflict, "Токен выключен, истёк или уже использован полностью")
		return
	}
	if s.streamBoundMacOSAgent(w, token) {
		s.audit(r.Context(), a, nil, "enrollment.agent_downloaded", "enrollment_token", tokenID, clientIP(r), map[string]any{"platform": "macos"})
	}
}

func (s *server) streamBoundMacOSAgent(w http.ResponseWriter, token string) bool {
	type artifact struct {
		path string
		file *os.File
		info os.FileInfo
	}
	artifacts := []artifact{
		{path: "RemoteIt Agent.app/Contents/Resources/remoteit-agent-macos-arm64"},
		{path: "RemoteIt Agent.app/Contents/Resources/remoteit-agent-macos-amd64"},
	}
	defer func() {
		for _, item := range artifacts {
			if item.file != nil {
				item.file.Close()
			}
		}
	}()
	for index := range artifacts {
		name := filepath.Base(artifacts[index].path)
		file, err := os.Open(filepath.Join(s.webRoot, "downloads", name))
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "Сборка macOS Agent временно недоступна")
			return false
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() < 1024 {
			file.Close()
			writeError(w, http.StatusServiceUnavailable, "Сборка macOS Agent повреждена")
			return false
		}
		artifacts[index].file = file
		artifacts[index].info = info
	}

	launcher := boundMacOSLauncher(token, s.publicURL)
	if len(launcher) < 512 || len(launcher) > 32<<10 {
		writeError(w, http.StatusInternalServerError, "Не удалось подготовить macOS Agent")
		return false
	}
	infoPlist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleDisplayName</key><string>RemoteIt Agent</string>
<key>CFBundleExecutable</key><string>RemoteIt Agent</string>
<key>CFBundleIdentifier</key><string>ru.supportgenesis.remoteit.agent</string>
<key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
<key>CFBundleName</key><string>RemoteIt Agent</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>1.0.42</string>
<key>CFBundleVersion</key><string>1</string>
<key>LSMinimumSystemVersion</key><string>11.0</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>`
	readme := "RemoteIt Agent для macOS\n\n1. Распакуйте архив.\n2. Перетащите RemoteIt Agent.app в папку Applications.\n3. Первый раз откройте приложение через правый клик -> Open.\n4. Введите имя компьютера и подтвердите пароль администратора macOS.\n\nТокен уже встроен и не требует копирования. Для установки без предупреждения Gatekeeper будущая сборка должна быть подписана Apple Developer ID и нотарифицирована.\n"

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "RemoteIt-Agent-macOS.zip"}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	archive := zip.NewWriter(w)
	writeBytes := func(name, value string, mode os.FileMode) error {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(mode)
		destination, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.WriteString(destination, value)
		return err
	}
	if err := writeBytes("RemoteIt Agent.app/Contents/Info.plist", infoPlist, 0o644); err != nil {
		return false
	}
	if err := writeBytes("RemoteIt Agent.app/Contents/MacOS/RemoteIt Agent", launcher, 0o755); err != nil {
		return false
	}
	if err := writeBytes("УСТАНОВКА.txt", readme, 0o644); err != nil {
		return false
	}
	for _, item := range artifacts {
		header := &zip.FileHeader{Name: item.path, Method: zip.Deflate}
		header.SetMode(0o755)
		header.UncompressedSize64 = uint64(item.info.Size())
		destination, err := archive.CreateHeader(header)
		if err != nil {
			return false
		}
		if _, err = io.Copy(destination, item.file); err != nil {
			return false
		}
	}
	return archive.Close() == nil
}

func boundMacOSLauncher(token, serverURL string) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
APP_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
case "$(uname -m)" in
  arm64|aarch64) AGENT="$APP_ROOT/Resources/remoteit-agent-macos-arm64" ;;
  x86_64|amd64) AGENT="$APP_ROOT/Resources/remoteit-agent-macos-amd64" ;;
  *) osascript -e 'display alert "RemoteIt Agent" message "Архитектура этого Mac пока не поддерживается." as critical'; exit 3 ;;
esac
DEFAULT_NAME="$(scutil --get ComputerName 2>/dev/null || hostname 2>/dev/null || printf 'Mac')"
/usr/bin/osascript - "$AGENT" %s "$DEFAULT_NAME" %s <<'APPLESCRIPT'
on run argv
  set agentPath to item 1 of argv
  set enrollmentToken to item 2 of argv
  set defaultName to item 3 of argv
  set serverURL to item 4 of argv
  set deviceName to text returned of (display dialog "Как назвать этот Mac в RemoteIt?" default answer defaultName buttons {"Отмена", "Установить"} default button "Установить" cancel button "Отмена" with title "RemoteIt Agent")
  if deviceName is "" then error number -128
  set installCommand to "chmod 700 " & quoted form of agentPath & " && " & quoted form of agentPath & " install --token " & quoted form of enrollmentToken & " --name " & quoted form of deviceName & " --server " & quoted form of serverURL
  do shell script installCommand with administrator privileges
  display alert "RemoteIt Agent установлен" message "Mac появится в панели через несколько секунд. Приложение можно закрыть." as informational
end run
APPLESCRIPT
`, shellSingleQuote(token), shellSingleQuote(strings.TrimRight(serverURL, "/")))
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (s *server) streamBoundUnixAgent(w http.ResponseWriter, token string) bool {
	installer, err := os.ReadFile(filepath.Join(s.webRoot, "downloads", "install-remoteit.sh"))
	if err != nil || len(installer) < 1024 || len(installer) > 256<<10 {
		writeError(w, http.StatusServiceUnavailable, "Unix-установщик временно недоступен")
		return false
	}
	quoted := "'" + strings.ReplaceAll(token, "'", "'\"'\"'") + "'"
	bound := bytes.Replace(installer, []byte("TOKEN=\"\""), []byte("TOKEN="+quoted), 1)
	if bytes.Equal(bound, installer) {
		writeError(w, http.StatusServiceUnavailable, "Не удалось подготовить Unix-установщик")
		return false
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "RemoteIt-Agent-Setup.sh"}))
	w.Header().Set("Content-Length", strconv.Itoa(len(bound)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(bound)
	return err == nil
}

func (s *server) downloadBoundWindowsAgent(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	tokenID := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "id")))
	if !validTransferID(tokenID) {
		writeError(w, http.StatusBadRequest, "Некорректный идентификатор токена")
		return
	}
	var token string
	var disabled bool
	var expiresAt time.Time
	var uses, maxUses int
	err := s.db.QueryRow(r.Context(), `SELECT COALESCE(token_value,''),disabled,expires_at,uses,max_uses FROM enrollment_tokens WHERE id=$1`, tokenID).Scan(&token, &disabled, &expiresAt, &uses, &maxUses)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Токен не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось подготовить Windows Agent")
		return
	}
	if token == "" {
		writeError(w, http.StatusConflict, "Для старого токена невозможно собрать готовый Agent")
		return
	}
	if disabled || time.Now().After(expiresAt) || uses >= maxUses {
		writeError(w, http.StatusConflict, "Токен выключен, истёк или уже использован полностью")
		return
	}
	if s.streamBoundWindowsAgent(w, r, tokenID, token) {
		s.audit(r.Context(), a, nil, "enrollment.agent_downloaded", "enrollment_token", tokenID, clientIP(r), map[string]any{"platform": "windows"})
	}
}

func (s *server) streamBoundWindowsAgent(w http.ResponseWriter, r *http.Request, tokenID, token string) bool {
	payload, err := json.Marshal(map[string]string{"token": token, "serverUrl": s.publicURL})
	if err != nil || len(payload) < 2 || len(payload) > 4096 {
		writeError(w, http.StatusInternalServerError, "Не удалось подготовить параметры Agent")
		return false
	}
	basePath := filepath.Join(s.webRoot, "downloads", "RemoteIt-Agent-Setup.exe")
	base, err := os.Open(basePath)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Сборка Windows Agent временно недоступна")
		return false
	}
	defer base.Close()
	info, err := base.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1024 {
		writeError(w, http.StatusServiceUnavailable, "Сборка Windows Agent повреждена")
		return false
	}
	lengthBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lengthBytes, uint32(len(payload)))
	totalSize := info.Size() + int64(len(payload)+len(boundWindowsAgentMagic)+len(lengthBytes))
	w.Header().Set("Content-Type", "application/vnd.microsoft.portable-executable")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "RemoteIt-Agent.exe"}))
	w.Header().Set("Content-Length", strconv.FormatInt(totalSize, 10))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err = io.Copy(w, base); err != nil {
		return false
	}
	if _, err = w.Write(payload); err != nil {
		return false
	}
	if _, err = io.WriteString(w, boundWindowsAgentMagic); err != nil {
		return false
	}
	if _, err = w.Write(lengthBytes); err != nil {
		return false
	}
	return true
}
