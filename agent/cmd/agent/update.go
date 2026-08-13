package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const maxAgentUpdateSize = 128 * 1024 * 1024

type agentUpdate struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size,omitempty"`
}

func downloadAndScheduleAgentUpdate(ctx context.Context, cfg *config, update agentUpdate) error {
	if err := validateAgentUpdate(cfg.ServerURL, update); err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, update.URL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "RemoteIt-Agent-Updater/"+version)
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Do(request)
	if err != nil {
		return fmt.Errorf("загрузка обновления: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("загрузка обновления: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxAgentUpdateSize {
		return errors.New("файл обновления превышает допустимый размер")
	}

	extension := ""
	if runtime.GOOS == "windows" {
		extension = ".exe"
	}
	file, err := os.CreateTemp("", "RemoteIt-Update-*"+extension)
	if err != nil {
		return fmt.Errorf("подготовка обновления: %w", err)
	}
	path := file.Name()
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()

	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxAgentUpdateSize+1))
	if copyErr != nil {
		return fmt.Errorf("сохранение обновления: %w", copyErr)
	}
	if written > maxAgentUpdateSize {
		return errors.New("файл обновления превышает допустимый размер")
	}
	if update.Size > 0 && written != update.Size {
		return fmt.Errorf("размер обновления не совпал: получено %d, ожидалось %d", written, update.Size)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), update.SHA256) {
		return errors.New("контрольная сумма обновления не совпала")
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	if err := scheduleAgentUpdate(path, update.Version); err != nil {
		return err
	}
	remove = false
	return nil
}

func validateAgentUpdate(serverURL string, update agentUpdate) error {
	if strings.TrimSpace(update.Version) == "" || agentVersionAtLeast(version, update.Version) {
		return errors.New("обновление не новее установленной версии")
	}
	expectedHash, err := hex.DecodeString(strings.TrimSpace(update.SHA256))
	if err != nil || len(expectedHash) != sha256.Size {
		return errors.New("сервер передал некорректную контрольную сумму обновления")
	}
	base, err := url.Parse(serverURL)
	if err != nil {
		return errors.New("некорректный адрес сервера")
	}
	candidate, err := url.Parse(update.URL)
	if err != nil || candidate.Scheme != "https" || candidate.User != nil || candidate.RawQuery != "" || candidate.Fragment != "" {
		return errors.New("некорректный HTTPS-адрес обновления")
	}
	if !strings.EqualFold(candidate.Host, base.Host) || !strings.HasPrefix(candidate.EscapedPath(), "/downloads/") {
		return errors.New("обновление разрешено только с настроенного сервера RemoteIt")
	}
	if update.Size < 0 || update.Size > maxAgentUpdateSize {
		return errors.New("сервер передал некорректный размер обновления")
	}
	return nil
}

func agentVersionAtLeast(actual, required string) bool {
	left := parseAgentVersion(actual)
	right := parseAgentVersion(required)
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for index := 0; index < length; index++ {
		leftValue, rightValue := 0, 0
		if index < len(left) {
			leftValue = left[index]
		}
		if index < len(right) {
			rightValue = right[index]
		}
		if leftValue != rightValue {
			return leftValue > rightValue
		}
	}
	return true
}

func parseAgentVersion(value string) []int {
	parts := strings.Split(strings.TrimSpace(value), ".")
	result := make([]int, len(parts))
	for index, part := range parts {
		part = strings.TrimLeftFunc(part, func(character rune) bool { return character < '0' || character > '9' })
		digits := strings.TrimRightFunc(part, func(character rune) bool { return character < '0' || character > '9' })
		result[index], _ = strconv.Atoi(digits)
	}
	return result
}

func installedAgentPath() (string, error) {
	current, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Clean(current), nil
}
