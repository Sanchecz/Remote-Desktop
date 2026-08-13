package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxLargeTransfer   int64 = 10 * 1024 * 1024 * 1024
	largeTransferChunk int64 = 8 * 1024 * 1024
)

type largeFileTransfer struct {
	ID, Direction, Name, RemotePath string
	Size, Received                  int64
}

func runFileTransferLoop(ctx context.Context, cfg *config) {
	client := &http.Client{}
	for ctx.Err() == nil {
		transfer, active, err := fetchNextFileTransfer(ctx, client, cfg)
		if err == nil && active {
			err = executeLargeFileTransfer(ctx, client, cfg, transfer)
			if err != nil {
				logTransferFailure(ctx, client, cfg, transfer.ID, err)
			}
			continue
		}
		if !waitContext(ctx, 3*time.Second) {
			return
		}
	}
}

func transferRequest(ctx context.Context, client *http.Client, cfg *config, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(cfg.ServerURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Genesis-Device-Id", cfg.DeviceID)
	req.Header.Set("Authorization", "Device "+cfg.DeviceSecret)
	req.Header.Set("User-Agent", "RemoteIt-Transfer/"+version)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return client.Do(req)
}

func fetchNextFileTransfer(ctx context.Context, client *http.Client, cfg *config) (largeFileTransfer, bool, error) {
	response, err := transferRequest(ctx, client, cfg, http.MethodGet, "/api/agent/file-transfers/next", nil, "")
	if err != nil {
		return largeFileTransfer{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return largeFileTransfer{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return largeFileTransfer{}, false, fmt.Errorf("transfer offer: HTTP %d", response.StatusCode)
	}
	var transfer largeFileTransfer
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&transfer); err != nil {
		return transfer, false, err
	}
	return transfer, transfer.ID != "", nil
}

func executeLargeFileTransfer(ctx context.Context, client *http.Client, cfg *config, transfer largeFileTransfer) error {
	if transfer.Size < 0 || transfer.Size > maxLargeTransfer {
		return errors.New("размер передачи превышает 10 ГБ")
	}
	switch transfer.Direction {
	case "to_device":
		return receiveLargeFile(ctx, client, cfg, transfer)
	case "from_device":
		return sendLargeFile(ctx, client, cfg, transfer)
	default:
		return errors.New("неизвестное направление передачи")
	}
}

func receiveLargeFile(ctx context.Context, client *http.Client, cfg *config, transfer largeFileTransfer) error {
	directory := filepath.Clean(strings.TrimSpace(transfer.RemotePath))
	name := strings.TrimSpace(transfer.Name)
	if !filepath.IsAbs(directory) || name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return errors.New("некорректный путь назначения")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("путь назначения не является папкой")
		}
		return err
	}
	target := filepath.Join(directory, name)
	if _, err = os.Stat(target); err == nil {
		return errors.New("файл с таким именем уже существует")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary := target + ".remoteit-part-" + transfer.ID[:8]
	finished := false
	defer func() {
		if !finished {
			_ = os.Remove(temporary)
		}
	}()
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	current, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if current > transfer.Size {
		_ = file.Truncate(0)
		current = 0
	}
	for current < transfer.Size {
		chunkStart := current
		received := false
		var lastErr error
		for attempt := 0; attempt < 5 && !received; attempt++ {
			if _, seekErr := file.Seek(chunkStart, io.SeekStart); seekErr != nil {
				return seekErr
			}
			_ = file.Truncate(chunkStart)
			path := "/api/agent/file-transfers/" + transfer.ID + "/data?offset=" + strconv.FormatInt(chunkStart, 10)
			response, requestErr := transferRequest(ctx, client, cfg, http.MethodGet, path, nil, "")
			if requestErr == nil {
				if response.StatusCode == http.StatusOK {
					written, copyErr := io.Copy(file, io.LimitReader(response.Body, largeTransferChunk+1))
					response.Body.Close()
					if copyErr == nil && written > 0 && written <= largeTransferChunk && chunkStart+written <= transfer.Size {
						if syncErr := file.Sync(); syncErr != nil {
							return syncErr
						}
						current = chunkStart + written
						received = true
						break
					}
					requestErr = copyErr
					if requestErr == nil {
						requestErr = errors.New("сервер вернул некорректную часть файла")
					}
				} else {
					response.Body.Close()
					requestErr = fmt.Errorf("скачивание части: HTTP %d", response.StatusCode)
				}
			}
			lastErr = requestErr
			if !waitContext(ctx, time.Duration(attempt+1)*time.Second) {
				return ctx.Err()
			}
		}
		if !received {
			return lastErr
		}
	}
	if current != transfer.Size {
		return errors.New("размер полученного файла не совпадает")
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(temporary, target); err != nil {
		return err
	}
	finished = true
	return completeFileTransfer(ctx, client, cfg, transfer.ID)
}

func sendLargeFile(ctx context.Context, client *http.Client, cfg *config, transfer largeFileTransfer) error {

	source := filepath.Clean(strings.TrimSpace(transfer.RemotePath))
	if !filepath.IsAbs(source) {
		return errors.New("некорректный путь к файлу")
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("передавать можно только обычные файлы")
	}
	if info.Size() != transfer.Size || info.Size() > maxLargeTransfer {
		return errors.New("размер файла изменился или превышает 10 ГБ")
	}
	offset := transfer.Received
	if offset < 0 || offset > transfer.Size {
		return errors.New("некорректный прогресс сервера")
	}
	for offset < transfer.Size {
		chunkOffset := offset
		length := min(largeTransferChunk, transfer.Size-chunkOffset)
		uploaded := false
		var lastErr error
		for attempt := 0; attempt < 5 && !uploaded; attempt++ {
			body := io.NewSectionReader(file, chunkOffset, length)
			path := "/api/agent/file-transfers/" + transfer.ID + "/data?offset=" + url.QueryEscape(strconv.FormatInt(chunkOffset, 10))
			response, requestErr := transferRequest(ctx, client, cfg, http.MethodPut, path, body, "application/octet-stream")
			if requestErr == nil {
				payload, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
				response.Body.Close()
				if readErr == nil && response.StatusCode == http.StatusOK {
					var progress struct {
						Received int64 `json:"received"`
					}
					if json.Unmarshal(payload, &progress) == nil && progress.Received > chunkOffset {
						offset = progress.Received
						uploaded = true
						break
					}
				}
				requestErr = fmt.Errorf("загрузка части: HTTP %d", response.StatusCode)
			}
			lastErr = requestErr
			if checkpoint, checkpointErr := fetchFileTransferCheckpoint(ctx, client, cfg, transfer.ID); checkpointErr == nil && checkpoint > chunkOffset {
				offset = checkpoint
				uploaded = true
				break
			}
			if !waitContext(ctx, time.Duration(attempt+1)*time.Second) {
				return ctx.Err()
			}
		}
		if !uploaded {
			return lastErr
		}
	}
	return completeFileTransfer(ctx, client, cfg, transfer.ID)
}

func fetchFileTransferCheckpoint(ctx context.Context, client *http.Client, cfg *config, id string) (int64, error) {
	response, err := transferRequest(ctx, client, cfg, http.MethodGet, "/api/agent/file-transfers/"+id, nil, "")
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("прогресс передачи: HTTP %d", response.StatusCode)
	}
	var progress struct {
		Received int64 `json:"received"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&progress); err != nil {
		return 0, err
	}
	return progress.Received, nil
}

func completeFileTransfer(ctx context.Context, client *http.Client, cfg *config, id string) error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		response, err := transferRequest(ctx, client, cfg, http.MethodPost, "/api/agent/file-transfers/"+id+"/complete", bytes.NewReader([]byte("{}")), "application/json")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("завершение передачи: HTTP %d", response.StatusCode)
		}
		lastErr = err
		if !waitContext(ctx, time.Duration(1<<attempt)*time.Second) {
			return ctx.Err()
		}
	}
	return lastErr
}

func logTransferFailure(ctx context.Context, client *http.Client, cfg *config, id string, transferErr error) {
	log.Printf("передача файла %s завершилась ошибкой: %v", id, transferErr)
	data, _ := json.Marshal(map[string]string{"error": truncateText(transferErr.Error(), 1000)})
	response, err := transferRequest(ctx, client, cfg, http.MethodPost, "/api/agent/file-transfers/"+id+"/fail", bytes.NewReader(data), "application/json")
	if err == nil {
		response.Body.Close()
	}
}
