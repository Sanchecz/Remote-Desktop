package main

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	desktopMaxFrame         = 8 << 20
	desktopMaxFrameEnvelope = desktopMaxFrame + 80
	desktopVideoLaneCount   = 6
)

type desktopFrameState struct {
	Frame            []byte
	Width            int
	Height           int
	At               time.Time
	Sequence         uint64
	ProducerSequence uint64
	DeviceID         string
	DatabaseTouchAt  time.Time
}

type desktopFrameDiagnostics struct {
	CaptureMillis int    `json:"captureMillis"`
	CopyMillis    int    `json:"copyMillis"`
	ScaleMillis   int    `json:"scaleMillis"`
	EncodeMillis  int    `json:"encodeMillis"`
	Backend       string `json:"captureBackend"`
}

type parsedDesktopFrameMessage struct {
	Width            int
	Height           int
	Frame            []byte
	Diagnostics      desktopFrameDiagnostics
	HasDiagnostics   bool
	ProducerSequence uint64
}

func desktopViewerLaneMatches(frame desktopFrameState, lane int) bool {
	if lane == -2 { // dedicated input-only channel
		return false
	}
	if lane < 0 {
		return true
	}
	// Old Agents do not include a producer sequence. They remain compatible on
	// lane 0 while modern clients spread consecutive frames across both lanes.
	if frame.ProducerSequence == 0 {
		return lane == 0
	}
	return int(frame.ProducerSequence%desktopVideoLaneCount) == lane
}

func desktopViewerPayload(frame desktopFrameState, lane int) []byte {
	if lane < 0 {
		return frame.Frame
	}
	payload := make([]byte, 12+len(frame.Frame))
	copy(payload[:4], "RTV1")
	binary.BigEndian.PutUint64(payload[4:12], frame.ProducerSequence)
	copy(payload[12:], frame.Frame)
	return payload
}

func parseDesktopFrameMessage(payload []byte) (parsedDesktopFrameMessage, error) {
	if len(payload) < 112 || len(payload) > desktopMaxFrameEnvelope {
		return parsedDesktopFrameMessage{}, errors.New("invalid desktop frame envelope")
	}
	magic := string(payload[:4])
	if magic != "RIT1" && magic != "RIT2" && magic != "RIT3" {
		return parsedDesktopFrameMessage{}, errors.New("invalid desktop frame magic")
	}
	parsed := parsedDesktopFrameMessage{
		Width:  int(binary.BigEndian.Uint32(payload[4:8])),
		Height: int(binary.BigEndian.Uint32(payload[8:12])),
		Frame:  payload[12:],
	}
	if magic == "RIT2" || magic == "RIT3" {
		if len(payload) < 18 {
			return parsedDesktopFrameMessage{}, errors.New("invalid desktop frame diagnostics")
		}
		backendLength := int(payload[17])
		metadataOffset := 18
		if magic == "RIT3" {
			metadataOffset = 26
			if len(payload) < metadataOffset {
				return parsedDesktopFrameMessage{}, errors.New("invalid desktop frame sequence")
			}
			parsed.ProducerSequence = binary.BigEndian.Uint64(payload[18:26])
		}
		frameOffset := metadataOffset + backendLength
		if backendLength > 48 || len(payload) < frameOffset+100 {
			return parsedDesktopFrameMessage{}, errors.New("invalid desktop frame diagnostics")
		}
		parsed.Frame = payload[frameOffset:]
		parsed.Diagnostics = desktopFrameDiagnostics{
			CaptureMillis: int(payload[12]),
			CopyMillis:    int(payload[13]),
			ScaleMillis:   int(payload[14]),
			EncodeMillis:  int(payload[15]),
			Backend:       string(payload[metadataOffset:frameOffset]),
		}
		parsed.HasDiagnostics = true
	}
	return parsed, nil
}

var errDesktopSessionInactive = errors.New("desktop session is inactive")

type cachedDesktopCredential struct {
	Hash      []byte
	ExpiresAt time.Time
}

type cachedDesktopSessionAccess struct {
	DeviceID  string
	CheckedAt time.Time
}

type desktopSessionRuntimeState struct {
	DeviceID      string
	Control       bool
	TargetFPS     int
	CursorVisible bool
	ValidatedAt   time.Time
}

type queuedDesktopInput struct {
	ID    int64             `json:"id"`
	Event desktopInputEvent `json:"event"`
}

type desktopInputQueue struct {
	mu        sync.Mutex
	events    []queuedDesktopInput
	nextID    int64
	touchedAt time.Time
	notify    chan struct{}
}

func newDesktopInputQueue() *desktopInputQueue {
	return &desktopInputQueue{notify: make(chan struct{}, 1), touchedAt: time.Now().UTC()}
}

func (queue *desktopInputQueue) enqueue(events []desktopInputEvent) {
	queue.mu.Lock()
	for _, event := range events {
		if event.Type == "pointer" && event.Action == "move" {
			filtered := queue.events[:0]
			for _, queued := range queue.events {
				if queued.Event.Type != "pointer" || queued.Event.Action != "move" {
					filtered = append(filtered, queued)
				}
			}
			queue.events = filtered
		}
		queue.nextID++
		queue.events = append(queue.events, queuedDesktopInput{ID: queue.nextID, Event: event})
	}
	if len(queue.events) > 120 {
		queue.events = append([]queuedDesktopInput(nil), queue.events[len(queue.events)-120:]...)
	}
	queue.touchedAt = time.Now().UTC()
	queue.mu.Unlock()
	select {
	case queue.notify <- struct{}{}:
	default:
	}
}

func (queue *desktopInputQueue) drain(limit int) []queuedDesktopInput {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.events) == 0 {
		queue.touchedAt = time.Now().UTC()
		return []queuedDesktopInput{}
	}
	if limit > len(queue.events) {
		limit = len(queue.events)
	}
	result := append([]queuedDesktopInput(nil), queue.events[:limit]...)
	queue.events = append([]queuedDesktopInput(nil), queue.events[limit:]...)
	queue.touchedAt = time.Now().UTC()
	return result
}

func (queue *desktopInputQueue) hasEvents() bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return len(queue.events) > 0
}

func (s *server) desktopQueue(sessionID string) *desktopInputQueue {
	if value, ok := s.desktopInputQueues.Load(sessionID); ok {
		if queue, valid := value.(*desktopInputQueue); valid {
			return queue
		}
	}
	queue := newDesktopInputQueue()
	actual, _ := s.desktopInputQueues.LoadOrStore(sessionID, queue)
	return actual.(*desktopInputQueue)
}

func (s *server) loadDesktopFrame(sessionID string) (desktopFrameState, bool) {
	value, ok := s.desktopFrames.Load(sessionID)
	if !ok {
		return desktopFrameState{}, false
	}
	frame, ok := value.(desktopFrameState)
	return frame, ok
}

func (s *server) loadDesktopFrameLane(sessionID string, lane int) (desktopFrameState, bool) {
	value, ok := s.desktopFrameLanes.Load(sessionID + "\x00" + strconv.Itoa(lane))
	if !ok {
		return desktopFrameState{}, false
	}
	frame, ok := value.(desktopFrameState)
	return frame, ok
}

func (s *server) desktopFrameSignal(sessionID string) chan struct{} {
	if value, ok := s.desktopFrameSignals.Load(sessionID); ok {
		if signal, valid := value.(chan struct{}); valid {
			return signal
		}
	}
	signal := make(chan struct{}, 1)
	actual, _ := s.desktopFrameSignals.LoadOrStore(sessionID, signal)
	return actual.(chan struct{})
}

func (s *server) signalDesktopFrame(sessionID string) {
	keys := []string{sessionID}
	for lane := 0; lane < desktopVideoLaneCount; lane++ {
		keys = append(keys, sessionID+"\x00viewer"+strconv.Itoa(lane))
	}
	for _, key := range keys {
		select {
		case s.desktopFrameSignal(key) <- struct{}{}:
		default:
		}
	}
}

func (s *server) deleteDesktopFrame(sessionID string) {
	if value, ok := s.desktopSessionRuntime.Load(sessionID); ok {
		if runtime, valid := value.(desktopSessionRuntimeState); valid {
			if mapped, exists := s.desktopDeviceSessions.Load(runtime.DeviceID); exists && mapped == sessionID {
				s.desktopDeviceSessions.Delete(runtime.DeviceID)
			}
		}
	}
	s.desktopFrames.Delete(sessionID)
	for lane := 0; lane < desktopVideoLaneCount; lane++ {
		s.desktopFrameLanes.Delete(sessionID + "\x00" + strconv.Itoa(lane))
	}
	s.desktopFrameDiagnostics.Delete(sessionID)
	s.desktopFrameSignals.Delete(sessionID)
	for lane := 0; lane < desktopVideoLaneCount; lane++ {
		s.desktopFrameSignals.Delete(sessionID + "\x00viewer" + strconv.Itoa(lane))
	}
	s.desktopFrameSignals.Delete(sessionID + "\x00viewerinput")
	s.desktopAgentSeen.Delete(sessionID)
	s.desktopViewerTouches.Delete(sessionID)
	s.desktopSessionRuntime.Delete(sessionID)
	s.desktopInputQueues.Delete(sessionID)
	prefix := sessionID + "\x00"
	s.desktopSessionAccess.Range(func(key, _ any) bool {
		if value, ok := key.(string); ok && strings.HasPrefix(value, prefix) {
			s.desktopSessionAccess.Delete(key)
		}
		return true
	})
}

func (s *server) desktopFrameLock(sessionID string) *sync.Mutex {
	lock := &sync.Mutex{}
	actual, _ := s.desktopFrameLocks.LoadOrStore(sessionID, lock)
	return actual.(*sync.Mutex)
}

func (s *server) deleteDesktopFramesForDevice(deviceID string) {
	s.desktopSessionRuntime.Range(func(key, value any) bool {
		if runtime, ok := value.(desktopSessionRuntimeState); ok && runtime.DeviceID == deviceID {
			if sessionID, valid := key.(string); valid {
				s.deleteDesktopFrame(sessionID)
			}
		}
		return true
	})
}

func (s *server) shouldTouchDesktopViewer(sessionID string, now time.Time) bool {
	if previous, ok := s.desktopViewerTouches.Load(sessionID); ok {
		if touchedAt, valid := previous.(time.Time); valid && now.Sub(touchedAt) < time.Second {
			return false
		}
	}
	s.desktopViewerTouches.Store(sessionID, now)
	return true
}

func (s *server) pruneDesktopRuntimeState(cutoff time.Time) {
	s.desktopFrames.Range(func(key, value any) bool {
		frame, ok := value.(desktopFrameState)
		if !ok || frame.At.Before(cutoff) {
			s.desktopFrames.Delete(key)
		}
		return true
	})
	s.desktopFrameLanes.Range(func(key, value any) bool {
		frame, ok := value.(desktopFrameState)
		if !ok || frame.At.Before(cutoff) {
			s.desktopFrameLanes.Delete(key)
		}
		return true
	})
	for _, state := range []*sync.Map{&s.desktopAgentSeen, &s.desktopViewerTouches} {
		state.Range(func(key, value any) bool {
			touchedAt, ok := value.(time.Time)
			if !ok || touchedAt.Before(cutoff) {
				state.Delete(key)
			}
			return true
		})
	}
	s.desktopSessionRuntime.Range(func(key, value any) bool {
		runtime, ok := value.(desktopSessionRuntimeState)
		if !ok || runtime.ValidatedAt.Before(cutoff) {
			if sessionID, valid := key.(string); valid {
				s.deleteDesktopFrame(sessionID)
			}
		}
		return true
	})
	s.desktopSessionAccess.Range(func(key, value any) bool {
		access, ok := value.(cachedDesktopSessionAccess)
		if !ok || access.CheckedAt.Before(cutoff) {
			s.desktopSessionAccess.Delete(key)
		}
		return true
	})
	s.desktopInputQueues.Range(func(key, value any) bool {
		queue, ok := value.(*desktopInputQueue)
		if !ok {
			s.desktopInputQueues.Delete(key)
			return true
		}
		queue.mu.Lock()
		stale := queue.touchedAt.Before(cutoff)
		queue.mu.Unlock()
		if stale {
			s.desktopInputQueues.Delete(key)
		}
		return true
	})
	now := time.Now().UTC()
	s.desktopAgentCredentials.Range(func(key, value any) bool {
		credential, ok := value.(cachedDesktopCredential)
		if !ok || credential.ExpiresAt.Before(now) {
			s.desktopAgentCredentials.Delete(key)
		}
		return true
	})
	s.authSessions.Range(func(key, value any) bool {
		entry, ok := value.(cachedAuthState)
		if !ok || entry.ValidatedAt.Before(cutoff) {
			s.authSessions.Delete(key)
		}
		return true
	})
	s.csrfTokens.Range(func(key, value any) bool {
		entry, ok := value.(cachedCSRFToken)
		if !ok || entry.ExpiresAt.Before(now) {
			s.csrfTokens.Delete(key)
		}
		return true
	})
}

type desktopInputEvent struct {
	Type    string `json:"type"`
	Action  string `json:"action,omitempty"`
	Button  string `json:"button,omitempty"`
	Text    string `json:"text,omitempty"`
	X       int    `json:"x,omitempty"`
	Y       int    `json:"y,omitempty"`
	Delta   int    `json:"delta,omitempty"`
	KeyCode int    `json:"keyCode,omitempty"`
}

func (s *server) authenticateDesktopAgent(w http.ResponseWriter, r *http.Request) (string, bool) {
	deviceID := strings.TrimSpace(r.Header.Get("X-Genesis-Device-Id"))
	authz := r.Header.Get("Authorization")
	if deviceID == "" || !strings.HasPrefix(authz, "Desktop ") {
		writeError(w, http.StatusUnauthorized, "Недействительные данные настольного агента")
		return "", false
	}
	secret := strings.TrimSpace(strings.TrimPrefix(authz, "Desktop "))
	now := time.Now().UTC()
	var stored []byte
	if value, exists := s.desktopAgentCredentials.Load(deviceID); exists {
		if credential, valid := value.(cachedDesktopCredential); valid && credential.ExpiresAt.After(now) {
			stored = credential.Hash
		}
	}
	if len(stored) == 0 {
		if err := s.db.QueryRow(r.Context(), `SELECT desktop_secret_hash FROM devices WHERE id=$1`, deviceID).Scan(&stored); err != nil || len(stored) == 0 {
			s.desktopAgentCredentials.Delete(deviceID)
			writeError(w, http.StatusUnauthorized, "Недействительные данные настольного агента")
			return "", false
		}
		stored = append([]byte(nil), stored...)
		s.desktopAgentCredentials.Store(deviceID, cachedDesktopCredential{Hash: stored, ExpiresAt: now.Add(30 * time.Second)})
	}
	if subtle.ConstantTimeCompare(tokenHash(secret), stored) != 1 {
		writeError(w, http.StatusUnauthorized, "Недействительные данные настольного агента")
		return "", false
	}
	return deviceID, true
}

func (s *server) storeDesktopRuntime(sessionID string, runtime desktopSessionRuntimeState) {
	s.desktopSessionRuntime.Store(sessionID, runtime)
	s.desktopDeviceSessions.Store(runtime.DeviceID, sessionID)
}

func (s *server) validateDesktopRuntime(ctx context.Context, sessionID, expectedDeviceID string) (desktopSessionRuntimeState, bool, error) {
	now := time.Now().UTC()
	if value, ok := s.desktopSessionRuntime.Load(sessionID); ok {
		if runtime, valid := value.(desktopSessionRuntimeState); valid && now.Sub(runtime.ValidatedAt) < time.Second && (expectedDeviceID == "" || runtime.DeviceID == expectedDeviceID) {
			return runtime, true, nil
		}
	}
	var runtime desktopSessionRuntimeState
	err := s.db.QueryRow(ctx, `SELECT device_id,control_enabled,target_fps,cursor_visible FROM remote_desktop_sessions WHERE id=$1 AND status='active' AND expires_at>now() AND viewer_seen_at>now()-interval '45 seconds'`, sessionID).Scan(&runtime.DeviceID, &runtime.Control, &runtime.TargetFPS, &runtime.CursorVisible)
	if errors.Is(err, pgx.ErrNoRows) {
		s.deleteDesktopFrame(sessionID)
		return desktopSessionRuntimeState{}, false, nil
	}
	if err != nil {
		return desktopSessionRuntimeState{}, false, err
	}
	if expectedDeviceID != "" && runtime.DeviceID != expectedDeviceID {
		return desktopSessionRuntimeState{}, false, nil
	}
	runtime.ValidatedAt = now
	s.storeDesktopRuntime(sessionID, runtime)
	return runtime, true, nil
}

func (s *server) startDesktopSession(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "Недостаточно прав для удалённого управления")
		return
	}
	deviceID := chi.URLParam(r, "id")
	if !s.requireDeviceAccess(w, r, deviceID) {
		return
	}
	var input struct {
		ControlEnabled *bool `json:"controlEnabled"`
		TargetFPS      *int  `json:"targetFps"`
		CursorVisible  *bool `json:"cursorVisible"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	controlEnabled := true
	if input.ControlEnabled != nil {
		controlEnabled = *input.ControlEnabled
	}
	// Zero is Auto. The desktop Agent treats Auto as a 30 FPS target and
	// adapts down/up according to measured capture and upload cost.
	targetFPS := 0
	if input.TargetFPS != nil {
		targetFPS = *input.TargetFPS
	}
	if !validDesktopTargetFPS(targetFPS) {
		writeError(w, http.StatusBadRequest, "Частота кадров должна быть Авто, 15, 30 или 60 FPS")
		return
	}
	cursorVisible := false
	if input.CursorVisible != nil {
		cursorVisible = *input.CursorVisible
	}
	var name, osName string
	var online bool
	if err := s.db.QueryRow(r.Context(), `SELECT name,os,(last_seen>now()-interval '90 seconds') FROM devices WHERE id=$1`, deviceID).Scan(&name, &osName, &online); err != nil {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	if !online {
		writeError(w, http.StatusConflict, "Агент устройства сейчас не в сети")
		return
	}
	if !strings.Contains(strings.ToLower(osName), "windows") {
		writeError(w, http.StatusConflict, "Удалённый экран в этой версии доступен для Windows")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать удалённый сеанс")
		return
	}
	defer tx.Rollback(r.Context())
	_, _ = tx.Exec(r.Context(), `UPDATE remote_desktop_sessions SET status='ended',frame=NULL,ended_at=now() WHERE device_id=$1 AND status='active'`, deviceID)
	var sessionID string
	err = tx.QueryRow(r.Context(), `INSERT INTO remote_desktop_sessions(device_id,created_by,control_enabled,target_fps,cursor_visible) VALUES($1,$2,$3,$4,$5) RETURNING id`, deviceID, a.UserID, controlEnabled, targetFPS, cursorVisible).Scan(&sessionID)
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать удалённый сеанс")
		return
	}
	s.deleteDesktopFramesForDevice(deviceID)
	s.storeDesktopRuntime(sessionID, desktopSessionRuntimeState{DeviceID: deviceID, Control: controlEnabled, TargetFPS: targetFPS, CursorVisible: cursorVisible, ValidatedAt: time.Now().UTC()})
	s.audit(r.Context(), a, nil, "desktop.started", "device", deviceID, clientIP(r), map[string]any{"sessionId": sessionID, "deviceName": name, "control": controlEnabled, "targetFps": targetFPS, "cursorVisible": cursorVisible})
	writeJSON(w, http.StatusCreated, map[string]any{"id": sessionID, "deviceId": deviceID, "status": "active", "controlEnabled": controlEnabled, "targetFps": targetFPS, "cursorVisible": cursorVisible})
}

func (s *server) requireDesktopSessionAccess(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	a := currentAuth(r)
	cacheKey := sessionID + "\x00" + a.SessionID
	now := time.Now().UTC()
	if value, ok := s.desktopSessionAccess.Load(cacheKey); ok {
		if access, valid := value.(cachedDesktopSessionAccess); valid && now.Sub(access.CheckedAt) < time.Second {
			return true
		}
	}
	privileged := a.Role == "owner" || a.Role == "admin"
	var deviceID string
	err := s.db.QueryRow(r.Context(), `SELECT device_id FROM remote_desktop_sessions WHERE id=$1 AND (created_by=$2 OR $3)`, sessionID, a.UserID, privileged).Scan(&deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Удалённый сеанс завершён или недоступен")
		return false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить удалённый сеанс")
		return false
	}
	if !s.requireDeviceAccess(w, r, deviceID) {
		s.desktopSessionAccess.Delete(cacheKey)
		return false
	}
	s.desktopSessionAccess.Store(cacheKey, cachedDesktopSessionAccess{DeviceID: deviceID, CheckedAt: now})
	return true
}

func (s *server) desktopSessionStatus(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	sessionID := chi.URLParam(r, "id")
	if !s.requireDesktopSessionAccess(w, r, sessionID) {
		return
	}
	var deviceID, status, agentError string
	var control, cursorVisible bool
	var frameWidth, frameHeight, targetFPS int
	var frameSequence uint64
	var producerFrameSequence uint64
	var frameAt, agentSeen *time.Time
	privileged := a.Role == "owner" || a.Role == "admin"
	err := s.db.QueryRow(r.Context(), `UPDATE remote_desktop_sessions SET viewer_seen_at=now(),expires_at=now()+interval '30 minutes' WHERE id=$1 AND (created_by=$2 OR $3) AND status='active' RETURNING device_id,status,control_enabled,target_fps,cursor_visible,frame_width,frame_height,frame_at,agent_seen_at,agent_error`, sessionID, a.UserID, privileged).Scan(&deviceID, &status, &control, &targetFPS, &cursorVisible, &frameWidth, &frameHeight, &frameAt, &agentSeen, &agentError)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Удалённый сеанс завершён или недоступен")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось получить состояние удалённого сеанса")
		return
	}
	if live, ok := s.loadDesktopFrame(sessionID); ok {
		frameWidth = live.Width
		frameHeight = live.Height
		frameSequence = live.Sequence
		producerFrameSequence = live.ProducerSequence
		seenAt := live.At
		frameAt = &seenAt
	}
	if value, ok := s.desktopAgentSeen.Load(sessionID); ok {
		if seenAt, valid := value.(time.Time); valid && (agentSeen == nil || seenAt.After(*agentSeen)) {
			agentSeen = &seenAt
		}
	}
	agentConnected := agentSeen != nil && time.Since(*agentSeen) < 8*time.Second
	diagnostics := desktopFrameDiagnostics{}
	if value, ok := s.desktopFrameDiagnostics.Load(sessionID); ok {
		diagnostics, _ = value.(desktopFrameDiagnostics)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": sessionID, "deviceId": deviceID, "status": status, "controlEnabled": control, "targetFps": targetFPS, "cursorVisible": cursorVisible, "frameWidth": frameWidth, "frameHeight": frameHeight, "frameAt": frameAt, "frameSequence": frameSequence, "producerFrameSequence": producerFrameSequence, "agentConnected": agentConnected, "agentError": agentError, "captureDiagnostics": diagnostics})
}

func (s *server) desktopSessionFrame(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if !s.requireDesktopSessionAccess(w, r, sessionID) {
		return
	}
	after := time.Unix(0, 0).UTC()
	if raw := strings.TrimSpace(r.URL.Query().Get("after")); raw != "" {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, raw); parseErr == nil {
			after = parsed
		}
	}
	live, ok := s.loadDesktopFrame(sessionID)
	if !ok || !live.At.After(after) || time.Since(live.At) > 15*time.Second {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	now := time.Now().UTC()
	if s.shouldTouchDesktopViewer(sessionID, now) {
		a := currentAuth(r)
		privileged := a.Role == "owner" || a.Role == "admin"
		result, err := s.db.Exec(r.Context(), `UPDATE remote_desktop_sessions SET viewer_seen_at=now(),expires_at=now()+interval '30 minutes' WHERE id=$1 AND (created_by=$2 OR $3) AND status='active'`, sessionID, a.UserID, privileged)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Не удалось подтвердить удалённый сеанс")
			return
		}
		if result.RowsAffected() == 0 {
			s.deleteDesktopFrame(sessionID)
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("X-RemoteIt-Width", strconv.Itoa(live.Width))
	w.Header().Set("X-RemoteIt-Height", strconv.Itoa(live.Height))
	w.Header().Set("X-RemoteIt-Frame-At", live.At.UTC().Format(time.RFC3339Nano))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(live.Frame)
}

func (s *server) updateDesktopSession(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "Недостаточно прав для удалённого управления")
		return
	}
	sessionID := chi.URLParam(r, "id")
	if !s.requireDesktopSessionAccess(w, r, sessionID) {
		return
	}
	var input struct {
		ControlEnabled *bool `json:"controlEnabled"`
		TargetFPS      *int  `json:"targetFps"`
		CursorVisible  *bool `json:"cursorVisible"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	if input.ControlEnabled == nil && input.TargetFPS == nil && input.CursorVisible == nil {
		writeError(w, http.StatusBadRequest, "Не указаны параметры удалённого сеанса")
		return
	}
	if input.TargetFPS != nil && !validDesktopTargetFPS(*input.TargetFPS) {
		writeError(w, http.StatusBadRequest, "Частота кадров должна быть Авто, 15, 30 или 60 FPS")
		return
	}
	privileged := a.Role == "owner" || a.Role == "admin"
	var controlValue any
	if input.ControlEnabled != nil {
		controlValue = *input.ControlEnabled
	}
	var fpsValue any
	if input.TargetFPS != nil {
		fpsValue = *input.TargetFPS
	}
	var cursorValue any
	if input.CursorVisible != nil {
		cursorValue = *input.CursorVisible
	}
	var deviceID string
	var controlEnabled, cursorVisible bool
	var targetFPS int
	err := s.db.QueryRow(r.Context(), `UPDATE remote_desktop_sessions SET control_enabled=COALESCE($1,control_enabled),target_fps=COALESCE($2,target_fps),cursor_visible=COALESCE($3,cursor_visible),viewer_seen_at=now(),expires_at=now()+interval '30 minutes' WHERE id=$4 AND (created_by=$5 OR $6) AND status='active' RETURNING device_id,control_enabled,target_fps,cursor_visible`, controlValue, fpsValue, cursorValue, sessionID, a.UserID, privileged).Scan(&deviceID, &controlEnabled, &targetFPS, &cursorVisible)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Удалённый сеанс завершён или недоступен")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось изменить режим удалённого сеанса")
		return
	}
	s.storeDesktopRuntime(sessionID, desktopSessionRuntimeState{DeviceID: deviceID, Control: controlEnabled, TargetFPS: targetFPS, CursorVisible: cursorVisible, ValidatedAt: time.Now().UTC()})
	if input.ControlEnabled != nil {
		s.audit(r.Context(), a, nil, "desktop.control.changed", "device", deviceID, clientIP(r), map[string]any{"sessionId": sessionID, "control": controlEnabled})
	}
	if input.TargetFPS != nil {
		s.audit(r.Context(), a, nil, "desktop.fps.changed", "device", deviceID, clientIP(r), map[string]any{"sessionId": sessionID, "targetFps": targetFPS})
	}
	if input.CursorVisible != nil {
		s.audit(r.Context(), a, nil, "desktop.cursor.changed", "device", deviceID, clientIP(r), map[string]any{"sessionId": sessionID, "cursorVisible": cursorVisible})
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": sessionID, "deviceId": deviceID, "status": "active", "controlEnabled": controlEnabled, "targetFps": targetFPS, "cursorVisible": cursorVisible})
}

func validDesktopTargetFPS(value int) bool {
	return value == 0 || value == 15 || value == 30 || value == 60
}

func (s *server) desktopSessionInput(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "Недостаточно прав для удалённого управления")
		return
	}
	sessionID := chi.URLParam(r, "id")
	if !s.requireDesktopSessionAccess(w, r, sessionID) {
		return
	}
	var input struct {
		desktopInputEvent
		Events []desktopInputEvent `json:"events"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	events := input.Events
	if len(events) == 0 {
		events = []desktopInputEvent{input.desktopInputEvent}
	}
	if len(events) > 64 {
		writeError(w, http.StatusBadRequest, "Input batch is too large")
		return
	}
	for _, candidate := range events {
		if !validDesktopInput(candidate) {
			writeError(w, http.StatusBadRequest, "Invalid remote input event")
			return
		}
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
	s.desktopQueue(sessionID).enqueue(events)
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func validDesktopInput(event desktopInputEvent) bool {
	switch event.Type {
	case "pointer":
		if event.X < 0 || event.Y < 0 || event.X > 20000 || event.Y > 20000 {
			return false
		}
		if event.Action == "move" {
			return true
		}
		return (event.Action == "down" || event.Action == "up") && (event.Button == "left" || event.Button == "right" || event.Button == "middle")
	case "wheel":
		return event.Delta >= -2400 && event.Delta <= 2400
	case "key":
		return (event.Action == "down" || event.Action == "up") && event.KeyCode >= 1 && event.KeyCode <= 255
	case "text":
		return event.Text != "" && len([]rune(event.Text)) <= 128 && !strings.ContainsRune(event.Text, '\x00')
	case "sas":
		// Secure Attention Sequence is deliberately a separate privileged event;
		// accepting modifiers or coordinates here would make the wire contract
		// ambiguous and could accidentally replay stale input data.
		return event.Action == "" && event.Button == "" && event.Text == "" && event.X == 0 && event.Y == 0 && event.Delta == 0 && event.KeyCode == 0
	default:
		return false
	}
}

func desktopInputEventsFromJSON(payload []byte) ([]desktopInputEvent, error) {
	var input struct {
		desktopInputEvent
		Events []desktopInputEvent `json:"events"`
	}
	if len(payload) == 0 || len(payload) > 64<<10 || json.Unmarshal(payload, &input) != nil {
		return nil, errors.New("invalid remote input payload")
	}
	events := input.Events
	if len(events) == 0 {
		events = []desktopInputEvent{input.desktopInputEvent}
	}
	if len(events) == 0 || len(events) > 64 {
		return nil, errors.New("invalid remote input batch size")
	}
	for _, candidate := range events {
		if !validDesktopInput(candidate) {
			return nil, errors.New("invalid remote input event")
		}
	}
	return events, nil
}

func (s *server) endDesktopSession(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	sessionID := chi.URLParam(r, "id")
	if !s.requireDesktopSessionAccess(w, r, sessionID) {
		return
	}
	privileged := a.Role == "owner" || a.Role == "admin"
	var deviceID string
	err := s.db.QueryRow(r.Context(), `UPDATE remote_desktop_sessions SET status='ended',frame=NULL,ended_at=now() WHERE id=$1 AND (created_by=$2 OR $3) AND status='active' RETURNING device_id`, sessionID, a.UserID, privileged).Scan(&deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		s.deleteDesktopFrame(sessionID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось завершить удалённый сеанс")
		return
	}
	_, _ = s.db.Exec(r.Context(), `DELETE FROM remote_desktop_inputs WHERE session_id=$1`, sessionID)
	s.deleteDesktopFrame(sessionID)
	s.audit(r.Context(), a, nil, "desktop.ended", "device", deviceID, clientIP(r), map[string]any{"sessionId": sessionID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) desktopAgentSession(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateDesktopAgent(w, r)
	if !ok {
		return
	}
	var sessionID string
	var control, cursorVisible bool
	var targetFPS int
	now := time.Now().UTC()
	if mapped, exists := s.desktopDeviceSessions.Load(deviceID); exists {
		if candidate, valid := mapped.(string); valid {
			if value, found := s.desktopSessionRuntime.Load(candidate); found {
				if runtime, current := value.(desktopSessionRuntimeState); current && now.Sub(runtime.ValidatedAt) < 500*time.Millisecond {
					s.desktopAgentSeen.Store(candidate, now)
					writeJSON(w, http.StatusOK, map[string]any{"id": candidate, "controlEnabled": runtime.Control, "targetFps": runtime.TargetFPS, "cursorVisible": runtime.CursorVisible})
					return
				}
			}
		}
	}
	err := s.db.QueryRow(r.Context(), `SELECT id,control_enabled,target_fps,cursor_visible FROM remote_desktop_sessions WHERE device_id=$1 AND status='active' AND expires_at>now() AND viewer_seen_at>now()-interval '45 seconds' ORDER BY created_at DESC LIMIT 1`, deviceID).Scan(&sessionID, &control, &targetFPS, &cursorVisible)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = s.db.Exec(r.Context(), `UPDATE remote_desktop_sessions SET status='expired',frame=NULL,ended_at=now() WHERE device_id=$1 AND status='active' AND (expires_at<=now() OR viewer_seen_at<now()-interval '45 seconds')`, deviceID)
		s.deleteDesktopFramesForDevice(deviceID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось получить удалённый сеанс")
		return
	}
	s.storeDesktopRuntime(sessionID, desktopSessionRuntimeState{DeviceID: deviceID, Control: control, TargetFPS: targetFPS, CursorVisible: cursorVisible, ValidatedAt: now})
	s.desktopAgentSeen.Store(sessionID, now)
	writeJSON(w, http.StatusOK, map[string]any{"id": sessionID, "controlEnabled": control, "targetFps": targetFPS, "cursorVisible": cursorVisible})
}

func (s *server) storeDesktopFrameSequenced(ctx context.Context, sessionID, deviceID string, width, height int, frame []byte, producerSequence uint64) (bool, error) {
	if width < 1 || height < 1 || width > 12000 || height > 12000 || len(frame) < 100 || len(frame) > desktopMaxFrame {
		return false, errors.New("invalid desktop frame")
	}
	now := time.Now().UTC()
	runtime, active, err := s.validateDesktopRuntime(ctx, sessionID, deviceID)
	if err != nil {
		return false, err
	}
	if !active || runtime.DeviceID != deviceID {
		return false, errDesktopSessionInactive
	}
	lock := s.desktopFrameLock(sessionID)
	lock.Lock()
	defer lock.Unlock()
	previous, hasPrevious := s.loadDesktopFrame(sessionID)
	frameLane := 0
	if producerSequence > 0 {
		frameLane = int(producerSequence % desktopVideoLaneCount)
	}
	lanePrevious, hasLanePrevious := s.loadDesktopFrameLane(sessionID, frameLane)
	if producerSequence > 0 && hasLanePrevious && lanePrevious.ProducerSequence >= producerSequence {
		return false, nil
	}
	if producerSequence > 0 && hasPrevious && previous.ProducerSequence > 0 && producerSequence <= previous.ProducerSequence {
		// Upload lanes can complete out of order. A late frame must never replace
		// the global newest frame, but it may still be the newest unread frame for
		// its parity lane. Publish it only to that lane so the viewer can merge it
		// by producer sequence without introducing a stale global state.
		immutableFrame := append([]byte(nil), frame...)
		lateFrame := desktopFrameState{Frame: immutableFrame, Width: width, Height: height, At: now, Sequence: producerSequence, ProducerSequence: producerSequence, DeviceID: deviceID, DatabaseTouchAt: previous.DatabaseTouchAt}
		s.desktopFrameLanes.Store(sessionID+"\x00"+strconv.Itoa(frameLane), lateFrame)
		s.desktopAgentSeen.Store(sessionID, now)
		s.signalDesktopFrame(sessionID)
		return true, nil
	}
	databaseTouchAt := previous.DatabaseTouchAt
	if !hasPrevious || now.Sub(databaseTouchAt) >= time.Second {
		result, touchErr := s.db.Exec(ctx, `UPDATE remote_desktop_sessions SET frame=NULL,frame_width=$1,frame_height=$2,frame_at=$3,agent_seen_at=$3,agent_error='' WHERE id=$4 AND device_id=$5 AND status='active' AND expires_at>now() AND viewer_seen_at>now()-interval '45 seconds'`, width, height, now, sessionID, deviceID)
		if touchErr != nil {
			return false, touchErr
		}
		if result.RowsAffected() == 0 {
			s.deleteDesktopFrame(sessionID)
			return false, errDesktopSessionInactive
		}
		databaseTouchAt = now
	}
	immutableFrame := append([]byte(nil), frame...)
	sequence := uint64(1)
	if hasPrevious {
		sequence = previous.Sequence + 1
	}
	storedFrame := desktopFrameState{Frame: immutableFrame, Width: width, Height: height, At: now, Sequence: sequence, ProducerSequence: producerSequence, DeviceID: deviceID, DatabaseTouchAt: databaseTouchAt}
	s.desktopFrames.Store(sessionID, storedFrame)
	// Each viewer lane keeps its own newest frame. Sharing the immutable JPEG
	// slice costs only a small state object, while preventing an even frame from
	// overwriting an unread odd frame in the single latest-frame slot.
	laneFrame := storedFrame
	if producerSequence > 0 {
		laneFrame.Sequence = producerSequence
	}
	s.desktopFrameLanes.Store(sessionID+"\x00"+strconv.Itoa(frameLane), laneFrame)
	s.desktopAgentSeen.Store(sessionID, now)
	s.signalDesktopFrame(sessionID)
	return true, nil
}

func (s *server) storeDesktopFrame(ctx context.Context, sessionID, deviceID string, width, height int, frame []byte) error {
	_, err := s.storeDesktopFrameSequenced(ctx, sessionID, deviceID, width, height, frame, 0)
	return err
}

func (s *server) desktopAgentFrame(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateDesktopAgent(w, r)
	if !ok {
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "image/jpeg") {
		writeError(w, http.StatusUnsupportedMediaType, "Ожидается JPEG-кадр")
		return
	}
	width, errWidth := strconv.Atoi(r.Header.Get("X-RemoteIt-Width"))
	height, errHeight := strconv.Atoi(r.Header.Get("X-RemoteIt-Height"))
	if errWidth != nil || errHeight != nil || width < 1 || height < 1 || width > 12000 || height > 12000 {
		writeError(w, http.StatusBadRequest, "Некорректный размер кадра")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, desktopMaxFrame)
	frame, err := io.ReadAll(r.Body)
	if err != nil || len(frame) < 100 || len(frame) > desktopMaxFrame {
		writeError(w, http.StatusBadRequest, "Некорректный кадр удалённого экрана")
		return
	}
	if err := s.storeDesktopFrame(r.Context(), chi.URLParam(r, "id"), deviceID, width, height, frame); err != nil {
		if errors.Is(err, errDesktopSessionInactive) {
			writeError(w, http.StatusConflict, "Удалённый сеанс уже завершён")
			return
		}
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить кадр")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// desktopAgentFrameStream is the low-latency transport used by current desktop
// agents. The connection is duplex: frame messages travel Agent -> server and
// input batches travel server -> Agent. The legacy HTTP endpoints remain as a
// fallback for upgrades and restrictive proxies. Every binary frame message is:
// "RIT1" + uint32(width) + uint32(height) + JPEG bytes. Current agents append
// a compact diagnostics trailer after the fixed geometry fields so production
// measurements can distinguish DXGI, secure-desktop GDI, encoder and network
// bottlenecks without logging or persisting image content.
func (s *server) desktopAgentFrameStream(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateDesktopAgent(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "id")
	runtime, active, err := s.validateDesktopRuntime(r.Context(), sessionID, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить удалённый сеанс")
		return
	}
	if !active || runtime.DeviceID != deviceID {
		writeError(w, http.StatusConflict, "Удалённый сеанс уже завершён")
		return
	}
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	connection.SetReadLimit(desktopMaxFrameEnvelope)
	streamCtx, cancelStream := context.WithCancel(r.Context())
	defer cancelStream()
	queue := s.desktopQueue(sessionID)
	agentLane := 0
	if rawLane := strings.TrimSpace(r.URL.Query().Get("lane")); rawLane != "" {
		parsedLane, parseErr := strconv.Atoi(rawLane)
		if parseErr != nil || parsedLane < 0 || parsedLane >= desktopVideoLaneCount {
			_ = connection.Close(websocket.StatusPolicyViolation, "invalid stream lane")
			return
		}
		agentLane = parsedLane
	}
	// Input belongs to the primary duplex connection only. Earlier every video
	// lane raced to drain the same queue, so an auxiliary lane could consume a
	// mouse/keyboard batch that its Agent-side client intentionally did not
	// expose. Besides losing clicks, that made the Agent leave its interactive
	// capture profile and send oversized sharp frames in the middle of 60 FPS
	// motion. Legacy Agents omit the parameter and remain primary-lane clients.
	if agentLane == 0 {
		go func() {
			validationTicker := time.NewTicker(500 * time.Millisecond)
			defer validationTicker.Stop()
			for {
				select {
				case <-streamCtx.Done():
					return
				case <-queue.notify:
				case <-validationTicker.C:
				}
				runtime, current, validationErr := s.validateDesktopRuntime(streamCtx, sessionID, deviceID)
				if validationErr != nil || !current {
					_ = connection.Close(websocket.StatusPolicyViolation, "session ended")
					return
				}
				if !runtime.Control || !queue.hasEvents() {
					continue
				}
				items := queue.drain(64)
				payload, marshalErr := json.Marshal(map[string]any{"events": items})
				if marshalErr != nil {
					continue
				}
				writeCtx, cancel := context.WithTimeout(streamCtx, 3*time.Second)
				writeErr := connection.Write(writeCtx, websocket.MessageText, payload)
				cancel()
				if writeErr != nil {
					return
				}
				s.desktopAgentSeen.Store(sessionID, time.Now().UTC())
			}
		}()
	}
	for {
		messageType, payload, readErr := connection.Read(streamCtx)
		if readErr != nil {
			return
		}
		if messageType != websocket.MessageBinary {
			_ = connection.Close(websocket.StatusUnsupportedData, "invalid frame")
			return
		}
		parsed, parseErr := parseDesktopFrameMessage(payload)
		if parseErr != nil {
			_ = connection.Close(websocket.StatusUnsupportedData, "invalid frame")
			return
		}
		stored, storeErr := s.storeDesktopFrameSequenced(r.Context(), sessionID, deviceID, parsed.Width, parsed.Height, parsed.Frame, parsed.ProducerSequence)
		if storeErr != nil {
			if errors.Is(storeErr, errDesktopSessionInactive) {
				_ = connection.Close(websocket.StatusPolicyViolation, "session ended")
				return
			}
			_ = connection.Close(websocket.StatusInternalError, "frame store failed")
			return
		}
		if stored && parsed.HasDiagnostics {
			// Diagnostics stay in memory and are available only to authenticated viewers.
			s.desktopFrameDiagnostics.Store(sessionID, parsed.Diagnostics)
		}
	}
}

// desktopSessionStream pushes complete frames to the browser and accepts input
// batches back over the same authenticated connection. This removes both the
// per-frame HTTP round trip and the old Agent-side 150 ms input polling window.
func (s *server) desktopSessionStream(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if !s.requireDesktopSessionAccess(w, r, sessionID) {
		return
	}
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	connection.SetReadLimit(64 << 10)
	lane := -1
	if rawLane := strings.TrimSpace(r.URL.Query().Get("lane")); rawLane != "" && rawLane != "input" {
		parsedLane, parseErr := strconv.Atoi(rawLane)
		if parseErr == nil && parsedLane >= 0 && parsedLane < desktopVideoLaneCount {
			lane = parsedLane
		}
	} else if rawLane == "input" {
		// Keep control packets off both high-bandwidth video sockets. Otherwise a
		// large JPEG write can occupy the same TCP flow and delay pointer/keyboard
		// input even though capture itself is fast.
		lane = -2
	}
	ctx, cancelStream := context.WithCancel(r.Context())
	defer cancelStream()
	inputPayloads := make(chan []byte, 16)
	readErrors := make(chan error, 1)
	go func() {
		for {
			messageType, payload, readErr := connection.Read(ctx)
			if readErr != nil {
				select {
				case readErrors <- readErr:
				default:
				}
				return
			}
			if messageType != websocket.MessageText {
				continue
			}
			select {
			case inputPayloads <- payload:
			case <-ctx.Done():
				return
			}
		}
	}()
	signalKey := sessionID
	if lane >= 0 {
		signalKey += "\x00viewer" + strconv.Itoa(lane)
	} else if lane == -2 {
		signalKey += "\x00viewerinput"
	}
	signal := s.desktopFrameSignal(signalKey)
	viewerTicker := time.NewTicker(time.Second)
	defer viewerTicker.Stop()
	var afterSequence uint64
	for {
		live, ok := s.loadDesktopFrame(sessionID)
		if lane >= 0 {
			live, ok = s.loadDesktopFrameLane(sessionID, lane)
		}
		if ok && live.Sequence > afterSequence && time.Since(live.At) <= 15*time.Second {
			if !desktopViewerLaneMatches(live, lane) {
				afterSequence = live.Sequence
				continue
			}
			payload := desktopViewerPayload(live, lane)
			writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			writeErr := connection.Write(writeCtx, websocket.MessageBinary, payload)
			cancel()
			if writeErr != nil {
				return
			}
			afterSequence = live.Sequence
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-readErrors:
			return
		case payload := <-inputPayloads:
			a := currentAuth(r)
			if a.Role == "viewer" {
				_ = connection.Close(websocket.StatusPolicyViolation, "input is not permitted")
				return
			}
			events, parseErr := desktopInputEventsFromJSON(payload)
			if parseErr != nil {
				_ = connection.Close(websocket.StatusUnsupportedData, "invalid input")
				return
			}
			runtime, active, runtimeErr := s.validateDesktopRuntime(ctx, sessionID, "")
			if runtimeErr != nil || !active || !runtime.Control {
				continue
			}
			s.desktopQueue(sessionID).enqueue(events)
		case <-signal:
		case <-viewerTicker.C:
			// One browser opens six video lanes plus a dedicated input lane. Only
			// lane 0 owns the database keepalive; letting every lane execute the same
			// UPDATE created multiple synchronous PostgreSQL writes per second and a
			// visible 31-80 ms hitch in an otherwise 4-8 ms capture path. The other
			// lanes are still authenticated and close with lane 0, but never compete
			// with frame delivery for a database connection.
			if lane > 0 || lane == -2 {
				continue
			}
			a := currentAuth(r)
			privileged := a.Role == "owner" || a.Role == "admin"
			result, touchErr := s.db.Exec(ctx, `UPDATE remote_desktop_sessions SET viewer_seen_at=now(),expires_at=now()+interval '30 minutes' WHERE id=$1 AND (created_by=$2 OR $3) AND status='active'`, sessionID, a.UserID, privileged)
			if touchErr != nil || result.RowsAffected() == 0 {
				return
			}
		}
	}
}

func (s *server) desktopAgentStatus(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateDesktopAgent(w, r)
	if !ok {
		return
	}
	var input struct {
		Error string `json:"error"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.Error = strings.TrimSpace(input.Error)
	if len(input.Error) > 500 {
		input.Error = input.Error[:500]
	}
	sessionID := chi.URLParam(r, "id")
	result, err := s.db.Exec(r.Context(), `UPDATE remote_desktop_sessions SET agent_seen_at=now(),agent_error=$1 WHERE id=$2 AND device_id=$3 AND status='active' AND expires_at>now() AND viewer_seen_at>now()-interval '45 seconds'`, input.Error, sessionID, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить состояние удалённого экрана")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "Удалённый сеанс уже завершён")
		return
	}
	s.desktopAgentSeen.Store(sessionID, time.Now().UTC())
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) desktopAgentInputs(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authenticateDesktopAgent(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "id")
	runtime, active, err := s.validateDesktopRuntime(r.Context(), sessionID, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить очередь ввода")
		return
	}
	if !active || !runtime.Control {
		writeError(w, http.StatusConflict, "Удалённый сеанс уже завершён")
		return
	}
	queue := s.desktopQueue(sessionID)
	if !queue.hasEvents() {
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-queue.notify:
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
	}
	items := queue.drain(64)
	s.desktopAgentSeen.Store(sessionID, time.Now().UTC())
	writeJSON(w, http.StatusOK, map[string]any{"events": items})
}
