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
	maxLargeTransfer          int64 = 10 * 1024 * 1024 * 1024
	largeTransferChunk        int64 = 64 * 1024 * 1024
	transferSourceWaitTimeout       = 5 * time.Minute
	transferIdleTimeout             = 45 * time.Second
	transferDownloadAttempts        = 5
	transferCompleteAttempts        = 6
)

type largeFileTransfer struct {
	ID, Direction, Name, RemotePath string
	Size, Received                  int64
}

type transferActivityReader struct {
	reader io.Reader
	touch  func()
}

func (reader transferActivityReader) Read(buffer []byte) (int, error) {
	read, err := reader.reader.Read(buffer)
	if read > 0 && reader.touch != nil {
		reader.touch()
	}
	return read, err
}

// newTransferIdleWatchdog cancels only a stalled request, not a merely slow
// transfer. Every successful body read refreshes the deadline, so a 10 GiB
// file can run for hours on a slow link while a dead Wi-Fi/VPN TCP flow is
// released quickly enough for the checkpoint retry to take over.
func newTransferIdleWatchdog(parent context.Context, idle time.Duration) (context.Context, context.CancelFunc, func()) {
	ctx, cancel := context.WithCancel(parent)
	activity := make(chan struct{}, 1)
	touch := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}
	go func() {
		timer := time.NewTimer(idle)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-activity:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(idle)
			case <-timer.C:
				cancel()
				return
			}
		}
	}()
	touch()
	return ctx, cancel, touch
}

func newFileTransferHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 8
	transport.MaxIdleConnsPerHost = 4
	transport.MaxConnsPerHost = 4
	transport.IdleConnTimeout = 60 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	// The streaming server long-poll waits for at most 20 seconds. A bounded
	// header wait prevents a vanished route from pinning the transfer loop, but
	// does not impose an absolute deadline on a large body that is progressing.
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: transport}
}

func runFileTransferLoop(ctx context.Context, cfg *config) {
	client := newFileTransferHTTPClient()
	defer client.CloseIdleConnections()
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
	if !validLargeTransferID(transfer.ID) {
		return errors.New("сервер вернул некорректный идентификатор передачи")
	}
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

func validLargeTransferID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for index, character := range id {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
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
	sourceWaitDeadline := time.Now().Add(transferSourceWaitTimeout)
	for current < transfer.Size {
		chunkStart := current
		received := false
		var lastErr error
		failures := 0
		for !received {
			if _, seekErr := file.Seek(chunkStart, io.SeekStart); seekErr != nil {
				return seekErr
			}
			_ = file.Truncate(chunkStart)
			path := "/api/agent/file-transfers/" + transfer.ID + "/data?offset=" + strconv.FormatInt(chunkStart, 10)
			requestCtx, cancelRequest, touch := newTransferIdleWatchdog(ctx, transferIdleTimeout)
			response, requestErr := transferRequest(requestCtx, client, cfg, http.MethodGet, path, nil, "")
			status := 0
			if requestErr == nil {
				status = response.StatusCode
				if response.StatusCode == http.StatusOK {
					written, copyErr := io.Copy(file, io.LimitReader(transferActivityReader{reader: response.Body, touch: touch}, largeTransferChunk+1))
					response.Body.Close()
					cancelRequest()
					if copyErr == nil && written > 0 && written <= largeTransferChunk && chunkStart+written <= transfer.Size {
						if syncErr := file.Sync(); syncErr != nil {
							return syncErr
						}
						current = chunkStart + written
						sourceWaitDeadline = time.Now().Add(transferSourceWaitTimeout)
						received = true
						break
					}
					requestErr = copyErr
					if requestErr == nil {
						requestErr = errors.New("сервер вернул некорректную часть файла")
					}
				} else {
					response.Body.Close()
					cancelRequest()
					requestErr = fmt.Errorf("скачивание части: HTTP %d", response.StatusCode)
				}
			} else {
				cancelRequest()
			}
			lastErr = requestErr
			retry, delay, waitsForSource := fileTransferDownloadRetry(status, failures)
			if !retry {
				return lastErr
			}
			if waitsForSource {
				if time.Now().After(sourceWaitDeadline) {
					return errors.New("источник не передал следующий блок файла за 5 минут")
				}
			} else {
				failures++
				if failures >= transferDownloadAttempts {
					return lastErr
				}
			}
			if !waitContext(ctx, delay) {
				return ctx.Err()
			}
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
			requestCtx, cancelRequest, touch := newTransferIdleWatchdog(ctx, transferIdleTimeout)
			body := transferActivityReader{reader: io.NewSectionReader(file, chunkOffset, length), touch: touch}
			path := "/api/agent/file-transfers/" + transfer.ID + "/data?offset=" + url.QueryEscape(strconv.FormatInt(chunkOffset, 10))
			response, requestErr := transferRequest(requestCtx, client, cfg, http.MethodPut, path, body, "application/octet-stream")
			if requestErr == nil {
				payload, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
				response.Body.Close()
				cancelRequest()
				if readErr == nil && response.StatusCode == http.StatusOK {
					var progress struct {
						Received int64 `json:"received"`
					}
					if json.Unmarshal(payload, &progress) == nil && validTransferCheckpoint(progress.Received, chunkOffset, length, transfer.Size) {
						offset = progress.Received
						uploaded = true
						break
					}
				}
				requestErr = fmt.Errorf("загрузка части: HTTP %d", response.StatusCode)
			} else {
				cancelRequest()
			}
			lastErr = requestErr
			if checkpoint, checkpointErr := fetchFileTransferCheckpoint(ctx, client, cfg, transfer.ID); checkpointErr == nil && validTransferCheckpoint(checkpoint, chunkOffset, length, transfer.Size) {
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

func validTransferCheckpoint(received, offset, chunkLength, total int64) bool {
	return offset >= 0 && chunkLength >= 0 && total >= 0 && offset <= total && chunkLength <= total-offset && received == offset+chunkLength
}

// fileTransferDownloadRetry keeps a pipelined browser-to-device transfer
// alive while the browser is still committing the next chunk. HTTP 425 is an
// expected flow-control response and therefore must not consume the ordinary
// network-error budget. Permanent authentication/path failures fail fast.
func fileTransferDownloadRetry(status, failures int) (retry bool, delay time.Duration, waitsForSource bool) {
	if status == http.StatusTooEarly {
		return true, 250 * time.Millisecond, true
	}
	if status == 0 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 {
		seconds := failures + 1
		if seconds > 5 {
			seconds = 5
		}
		return true, time.Duration(seconds) * time.Second, false
	}
	return false, 0, false
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
	for attempt := 0; attempt < transferCompleteAttempts; attempt++ {
		response, err := transferRequest(ctx, client, cfg, http.MethodPost, "/api/agent/file-transfers/"+id+"/complete", bytes.NewReader([]byte("{}")), "application/json")
		status := 0
		if err == nil {
			status = response.StatusCode
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("завершение передачи: HTTP %d", response.StatusCode)
		}
		lastErr = err
		retry, delay := fileTransferCompletionRetry(status, attempt)
		if !retry || attempt == transferCompleteAttempts-1 {
			return lastErr
		}
		if !waitContext(ctx, delay) {
			return ctx.Err()
		}
	}
	return lastErr
}

func fileTransferCompletionRetry(status, attempt int) (bool, time.Duration) {
	if status != 0 && status != http.StatusConflict && status != http.StatusTooEarly && status != http.StatusRequestTimeout && status != http.StatusTooManyRequests && status < 500 {
		return false, 0
	}
	delay := time.Second << min(attempt, 3)
	return true, delay
}

func logTransferFailure(ctx context.Context, client *http.Client, cfg *config, id string, transferErr error) {
	log.Printf("передача файла %s завершилась ошибкой: %v", id, transferErr)
	data, _ := json.Marshal(map[string]string{"error": truncateText(transferErr.Error(), 1000)})
	response, err := transferRequest(ctx, client, cfg, http.MethodPost, "/api/agent/file-transfers/"+id+"/fail", bytes.NewReader(data), "application/json")
	if err == nil {
		response.Body.Close()
	}
}
