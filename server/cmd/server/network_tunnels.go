package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	networkTunnelLifetime          = 2 * time.Hour
	networkTunnelPerUserLimit      = 8
	networkTunnelDeviceOnlineQuery = `SELECT last_seen>now()-interval '75 seconds' AND NOT pending_removal FROM devices WHERE id=$1`
)

type networkTunnelRuntime struct {
	mu        sync.Mutex
	agent     *websocket.Conn
	client    *websocket.Conn
	clientTCP net.Conn
	started   bool
	done      chan struct{}
	closeOne  sync.Once
}

func newNetworkTunnelRuntime() *networkTunnelRuntime {
	return &networkTunnelRuntime{done: make(chan struct{})}
}

func (runtime *networkTunnelRuntime) close(code websocket.StatusCode, reason string) {
	runtime.closeOne.Do(func() {
		runtime.mu.Lock()
		agent, client, clientTCP := runtime.agent, runtime.client, runtime.clientTCP
		runtime.mu.Unlock()
		if agent != nil {
			_ = agent.Close(code, reason)
		}
		if client != nil {
			_ = client.Close(code, reason)
		}
		if clientTCP != nil {
			_ = clientTCP.Close()
		}
		close(runtime.done)
	})
}

func (s *server) tunnelRuntime(id string) *networkTunnelRuntime {
	candidate := newNetworkTunnelRuntime()
	actual, _ := s.networkTunnels.LoadOrStore(id, candidate)
	return actual.(*networkTunnelRuntime)
}

func (s *server) createNetworkTunnel(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "RDP и SSH доступны только владельцу и администраторам")
		return
	}
	var input struct {
		DeviceID string `json:"deviceId"`
		Protocol string `json:"protocol"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.DeviceID = strings.ToLower(strings.TrimSpace(input.DeviceID))
	input.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	input.Host = strings.TrimSpace(input.Host)
	input.Username = strings.TrimSpace(input.Username)
	if !validTransferID(input.DeviceID) || !oneOf(input.Protocol, "rdp", "ssh") || input.Port < 1 || input.Port > 65535 || !validNetworkTunnelUsername(input.Username) {
		writeError(w, http.StatusBadRequest, "Параметры подключения некорректны")
		return
	}
	target, targetAllowed := networkTunnelPrivateIPv4(input.Host)
	if !targetAllowed {
		writeError(w, http.StatusBadRequest, "Укажите внутренний IPv4-адрес из частной сети")
		return
	}
	if !s.requireDeviceAccess(w, r, input.DeviceID) {
		return
	}
	var online bool
	if err := s.db.QueryRow(r.Context(), networkTunnelDeviceOnlineQuery, input.DeviceID).Scan(&online); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Agent не найден")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить Agent")
		return
	} else if !online {
		writeError(w, http.StatusConflict, "Agent сейчас не в сети")
		return
	}

	clientToken := randomToken(32)
	payload, _ := json.Marshal(map[string]any{"host": target, "port": input.Port, "protocol": input.Protocol})
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать защищённый туннель")
		return
	}
	defer tx.Rollback(r.Context())
	var userLock, activeTunnels int
	if err = tx.QueryRow(r.Context(), `SELECT 1 FROM users WHERE id=$1 FOR UPDATE`, a.UserID).Scan(&userLock); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось подготовить защищённый туннель")
		return
	}
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM network_tunnels WHERE created_by=$1 AND status IN ('waiting','connected') AND expires_at>now()`, a.UserID).Scan(&activeTunnels); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить активные туннели")
		return
	}
	if activeTunnels >= networkTunnelPerUserLimit {
		writeError(w, http.StatusTooManyRequests, "Сначала завершите одно из активных RDP/SSH-подключений")
		return
	}
	var tunnelID, jobID string
	var expiresAt time.Time
	if err = tx.QueryRow(r.Context(), `INSERT INTO network_tunnels(device_id,created_by,protocol,target_host,target_port,client_token_hash,expires_at) VALUES($1,$2,$3,$4,$5,$6,now()+make_interval(secs=>$7)) RETURNING id,expires_at`, input.DeviceID, a.UserID, input.Protocol, target, input.Port, tokenHash(clientToken), int(networkTunnelLifetime/time.Second)).Scan(&tunnelID, &expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать защищённый туннель")
		return
	}
	var tunnelPayload map[string]any
	_ = json.Unmarshal(payload, &tunnelPayload)
	tunnelPayload["tunnelId"] = tunnelID
	tunnelPayload["expiresAt"] = expiresAt.UTC().Format(time.RFC3339)
	jobPayload, _ := json.Marshal(tunnelPayload)
	if err = tx.QueryRow(r.Context(), `INSERT INTO agent_jobs(device_id,created_by,job_type,payload,timeout_seconds,expires_at) VALUES($1,$2,'tunnel',$3,30,$4) RETURNING id`, input.DeviceID, a.UserID, jobPayload, expiresAt).Scan(&jobID); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось передать туннель Agent")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить защищённый туннель")
		return
	}
	launch := &url.URL{Scheme: "remoteit", Host: "connect"}
	query := launch.Query()
	query.Set("id", tunnelID)
	query.Set("token", clientToken)
	query.Set("protocol", input.Protocol)
	query.Set("username", input.Username)
	launch.RawQuery = query.Encode()
	s.audit(r.Context(), a, nil, "network_tunnel.created", "network_tunnel", tunnelID, clientIP(r), map[string]any{"deviceId": input.DeviceID, "protocol": input.Protocol, "targetHost": target, "targetPort": input.Port, "jobId": jobID})
	writeJSON(w, http.StatusCreated, map[string]any{"id": tunnelID, "status": "waiting", "launchUrl": launch.String(), "expiresAt": expiresAt})
}

func (s *server) networkTunnelStatus(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	id := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "id")))
	if !validTransferID(id) {
		writeError(w, http.StatusBadRequest, "Некорректный идентификатор туннеля")
		return
	}
	var status, protocol, host, errorText string
	var port int
	var expiresAt time.Time
	err := s.db.QueryRow(r.Context(), `SELECT status,protocol,target_host::text,target_port,expires_at,error_text FROM network_tunnels WHERE id=$1 AND created_by=$2`, id, a.UserID).Scan(&status, &protocol, &host, &port, &expiresAt, &errorText)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Туннель не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось получить состояние туннеля")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": status, "protocol": protocol, "host": host, "port": port, "expiresAt": expiresAt, "error": errorText})
}

func (s *server) networkTunnelAgent(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "id")))
	deviceID := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Genesis-Device-Id")))
	authorization := r.Header.Get("Authorization")
	if !validTransferID(id) || !validTransferID(deviceID) || !strings.HasPrefix(authorization, "Device ") {
		writeError(w, http.StatusUnauthorized, "Недействительные данные Agent")
		return
	}
	secret := strings.TrimSpace(strings.TrimPrefix(authorization, "Device "))
	var stored []byte
	var tunnelDeviceID string
	var expiresAt time.Time
	err := s.db.QueryRow(r.Context(), `SELECT d.secret_hash,t.device_id,t.expires_at FROM network_tunnels t JOIN devices d ON d.id=t.device_id WHERE t.id=$1 AND t.status IN ('waiting','connected')`, id).Scan(&stored, &tunnelDeviceID, &expiresAt)
	if err != nil || tunnelDeviceID != deviceID || time.Now().After(expiresAt) || subtle.ConstantTimeCompare(tokenHash(secret), stored) != 1 {
		writeError(w, http.StatusUnauthorized, "Туннель не принадлежит этому Agent")
		return
	}
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	if !s.attachNetworkTunnel(r.Context(), id, "agent", connection) {
		_ = connection.Close(websocket.StatusPolicyViolation, "Agent tunnel already connected")
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE network_tunnels SET agent_connected_at=now() WHERE id=$1`, id)
	s.waitNetworkTunnel(r.Context(), id)
}

func (s *server) networkTunnelClient(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "id")))
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Tunnel ") {
		writeError(w, http.StatusUnauthorized, "Недействительный ключ туннеля")
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Tunnel "))
	if !validTransferID(id) || len(token) < 24 || len(token) > 512 || strings.ContainsAny(token, "\r\n\x00") {
		writeError(w, http.StatusUnauthorized, "Недействительный ключ туннеля")
		return
	}
	var expiresAt time.Time
	err := s.db.QueryRow(r.Context(), `UPDATE network_tunnels SET client_connected_at=now() WHERE id=$1 AND status IN ('waiting','connected') AND client_connected_at IS NULL AND client_token_hash=$2 AND expires_at>now() RETURNING expires_at`, id, tokenHash(token)).Scan(&expiresAt)
	if err != nil || time.Now().After(expiresAt) {
		writeError(w, http.StatusUnauthorized, "Туннель истёк или недействителен")
		return
	}
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		_, _ = s.db.Exec(context.Background(), `UPDATE network_tunnels SET client_connected_at=NULL WHERE id=$1 AND status='waiting' AND connected_at IS NULL`, id)
		return
	}
	if !s.attachNetworkTunnel(r.Context(), id, "client", connection) {
		_ = connection.Close(websocket.StatusPolicyViolation, "Client tunnel already connected")
		return
	}
	s.waitNetworkTunnel(r.Context(), id)
}

func (s *server) attachNetworkTunnel(ctx context.Context, id, side string, connection *websocket.Conn) bool {
	runtime := s.tunnelRuntime(id)
	runtime.mu.Lock()
	if (side == "agent" && runtime.agent != nil) || (side == "client" && runtime.client != nil) {
		runtime.mu.Unlock()
		return false
	}
	if side == "agent" {
		runtime.agent = connection
	} else {
		runtime.client = connection
	}
	ready := runtime.agent != nil && (runtime.client != nil || runtime.clientTCP != nil) && !runtime.started
	if ready {
		runtime.started = true
	}
	runtime.mu.Unlock()
	if ready {
		go s.relayNetworkTunnel(id, runtime)
	}
	return true
}

func (s *server) attachNetworkTunnelTCP(id string, connection net.Conn) bool {
	runtime := s.tunnelRuntime(id)
	runtime.mu.Lock()
	if runtime.client != nil || runtime.clientTCP != nil {
		runtime.mu.Unlock()
		return false
	}
	runtime.clientTCP = connection
	ready := runtime.agent != nil && !runtime.started
	if ready {
		runtime.started = true
	}
	runtime.mu.Unlock()
	if ready {
		go s.relayNetworkTunnel(id, runtime)
	}
	return true
}

func (s *server) waitNetworkTunnel(ctx context.Context, id string) {
	runtime := s.tunnelRuntime(id)
	select {
	case <-runtime.done:
	case <-ctx.Done():
		runtime.close(websocket.StatusNormalClosure, "Tunnel connection closed")
		_, _ = s.db.Exec(context.Background(), `UPDATE network_tunnels SET status='ended',ended_at=now(),error_text=CASE WHEN connected_at IS NULL THEN 'Одна из сторон отключилась до запуска туннеля' ELSE error_text END WHERE id=$1 AND status='waiting'`, id)
		s.networkTunnels.Delete(id)
	}
}

func (s *server) relayNetworkTunnel(id string, runtime *networkTunnelRuntime) {
	_, _ = s.db.Exec(context.Background(), `UPDATE network_tunnels SET status='connected',connected_at=now() WHERE id=$1 AND status='waiting'`, id)
	runtime.mu.Lock()
	agentConnection, clientConnection, clientTCP := runtime.agent, runtime.client, runtime.clientTCP
	runtime.mu.Unlock()
	agent := websocket.NetConn(context.Background(), agentConnection, websocket.MessageBinary)
	var client net.Conn
	if clientTCP != nil {
		client = clientTCP
	} else {
		client = websocket.NetConn(context.Background(), clientConnection, websocket.MessageBinary)
	}
	completed := make(chan struct{}, 2)
	copySide := func(destination, source net.Conn) {
		_, _ = io.Copy(destination, source)
		completed <- struct{}{}
	}
	go copySide(agent, client)
	go copySide(client, agent)
	<-completed
	_ = agent.Close()
	_ = client.Close()
	runtime.close(websocket.StatusNormalClosure, "Tunnel ended")
	_, _ = s.db.Exec(context.Background(), `UPDATE network_tunnels SET status='ended',ended_at=now() WHERE id=$1 AND status IN ('waiting','connected')`, id)
	s.networkTunnels.Delete(id)
}

func networkTunnelPrivateIPv4(value string) (string, bool) {
	parsed := net.ParseIP(strings.TrimSpace(value))
	if parsed == nil || parsed.To4() == nil || !parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsUnspecified() || parsed.IsMulticast() {
		return "", false
	}
	return parsed.To4().String(), true
}

func validNetworkTunnelUsername(value string) bool {
	return len(value) <= 255 && !strings.HasPrefix(value, "-") && strings.IndexFunc(value, unicode.IsControl) < 0
}

func (s *server) removeExpiredNetworkTunnels() {
	rows, err := s.db.Query(context.Background(), `UPDATE network_tunnels SET status='expired',ended_at=COALESCE(ended_at,now()),error_text=CASE WHEN error_text='' THEN 'Срок защищённого туннеля истёк' ELSE error_text END WHERE expires_at<=now() AND status IN ('waiting','connected') RETURNING id`)
	if err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				if value, ok := s.networkTunnels.Load(id); ok {
					value.(*networkTunnelRuntime).close(websocket.StatusNormalClosure, "Tunnel expired")
				}
				s.networkTunnels.Delete(id)
			}
		}
		rows.Close()
	}
	_, _ = s.db.Exec(context.Background(), `DELETE FROM network_tunnels WHERE ended_at<now()-interval '7 days' AND status IN ('ended','failed','expired')`)
}
