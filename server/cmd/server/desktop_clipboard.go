package main

import (
	"bytes"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const desktopMaximumClipboardImageBytes = 12 << 20

type desktopClipboardImage struct {
	Sequence uint64
	Data     []byte
	At       time.Time
}

type desktopClipboardImageState struct {
	mu     sync.Mutex
	next   uint64
	viewer desktopClipboardImage
	agent  desktopClipboardImage
}

func (s *server) desktopClipboardImageState(sessionID string) *desktopClipboardImageState {
	candidate := &desktopClipboardImageState{}
	actual, _ := s.desktopClipboardImages.LoadOrStore(sessionID, candidate)
	return actual.(*desktopClipboardImageState)
}

func readDesktopClipboardPNG(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "image/png" {
		writeError(w, http.StatusUnsupportedMediaType, "Буфер поддерживает изображения PNG")
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, desktopMaximumClipboardImageBytes+1)
	data, err := io.ReadAll(r.Body)
	if err != nil || len(data) == 0 || len(data) > desktopMaximumClipboardImageBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Изображение в буфере больше 12 МБ")
		return nil, false
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 12000 || config.Height > 12000 || int64(config.Width)*int64(config.Height) > 32_000_000 {
		writeError(w, http.StatusBadRequest, "Некорректное или слишком большое изображение PNG")
		return nil, false
	}
	return data, true
}

func writeDesktopClipboardPNG(w http.ResponseWriter, image desktopClipboardImage) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("X-RemoteIt-Clipboard-Sequence", strconv.FormatUint(image.Sequence, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image.Data)
}

func (s *server) desktopSessionUploadClipboardImage(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "Недостаточно прав для удалённого управления")
		return
	}
	sessionID := chi.URLParam(r, "id")
	if !s.requireDesktopSessionAccess(w, r, sessionID) {
		return
	}
	runtime, active, err := s.validateDesktopRuntime(r.Context(), sessionID, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить удалённый сеанс")
		return
	}
	if !active {
		writeError(w, http.StatusNotFound, "Удалённый сеанс завершён или недоступен")
		return
	}
	if !runtime.Control {
		writeError(w, http.StatusConflict, "Управление в этом сеансе ещё не включено")
		return
	}
	data, ok := readDesktopClipboardPNG(w, r)
	if !ok {
		return
	}
	state := s.desktopClipboardImageState(sessionID)
	state.mu.Lock()
	state.next++
	image := desktopClipboardImage{Sequence: state.next, Data: append([]byte(nil), data...), At: time.Now().UTC()}
	state.viewer = image
	state.mu.Unlock()
	inputID := s.desktopQueue(sessionID).enqueue([]desktopInputEvent{{Type: "clipboard_image_write", Text: strconv.FormatUint(image.Sequence, 10)}})
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "sequence": image.Sequence, "inputId": inputID})
}

func (s *server) desktopSessionClipboardImage(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if !s.requireDesktopSessionAccess(w, r, sessionID) {
		return
	}
	after, _ := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("after")), 10, 64)
	state := s.desktopClipboardImageState(sessionID)
	state.mu.Lock()
	image := state.agent
	state.mu.Unlock()
	if image.Sequence == 0 || image.Sequence <= after || time.Since(image.At) > 30*time.Minute {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeDesktopClipboardPNG(w, image)
}

func (s *server) desktopAgentUploadClipboardImage(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateDesktopAgent(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "id")
	if _, active, err := s.validateDesktopRuntime(r.Context(), sessionID, deviceID); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить удалённый сеанс")
		return
	} else if !active {
		writeError(w, http.StatusConflict, "Удалённый сеанс уже завершён")
		return
	}
	data, valid := readDesktopClipboardPNG(w, r)
	if !valid {
		return
	}
	state := s.desktopClipboardImageState(sessionID)
	state.mu.Lock()
	state.next++
	state.agent = desktopClipboardImage{Sequence: state.next, Data: append([]byte(nil), data...), At: time.Now().UTC()}
	sequence := state.agent.Sequence
	state.mu.Unlock()
	w.Header().Set("X-RemoteIt-Clipboard-Sequence", strconv.FormatUint(sequence, 10))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) desktopAgentDownloadClipboardImage(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateDesktopAgent(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "id")
	if _, active, err := s.validateDesktopRuntime(r.Context(), sessionID, deviceID); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить удалённый сеанс")
		return
	} else if !active {
		writeError(w, http.StatusConflict, "Удалённый сеанс уже завершён")
		return
	}
	sequence, err := strconv.ParseUint(chi.URLParam(r, "sequence"), 10, 64)
	if err != nil || sequence == 0 {
		writeError(w, http.StatusBadRequest, "Некорректная версия буфера")
		return
	}
	state := s.desktopClipboardImageState(sessionID)
	state.mu.Lock()
	image := state.viewer
	state.mu.Unlock()
	if image.Sequence != sequence || time.Since(image.At) > 30*time.Minute {
		writeError(w, http.StatusNotFound, "Изображение буфера уже недоступно")
		return
	}
	writeDesktopClipboardPNG(w, image)
}
