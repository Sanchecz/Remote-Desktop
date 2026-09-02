package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type browserConnectionInput struct {
	DeviceID string `json:"deviceId"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain"`
}

func (s *server) createBrowserConnection(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "RDP и SSH доступны только владельцу и администраторам")
		return
	}
	secret, err := guacamoleJSONSecret()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Браузерный шлюз временно не настроен")
		return
	}
	var in browserConnectionInput
	if decodeJSON(w, r, &in) != nil {
		return
	}
	in.DeviceID = strings.ToLower(strings.TrimSpace(in.DeviceID))
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	in.Host, in.Username, in.Domain = strings.TrimSpace(in.Host), strings.TrimSpace(in.Username), strings.TrimSpace(in.Domain)
	if !validTransferID(in.DeviceID) || !oneOf(in.Protocol, "rdp", "ssh") || in.Port < 1 || in.Port > 65535 || !validNetworkTunnelUsername(in.Username) || len(in.Domain) > 255 || strings.IndexFunc(in.Domain, func(r rune) bool { return r < 32 }) >= 0 || len(in.Password) > 1024 || strings.ContainsRune(in.Password, '\x00') {
		writeError(w, http.StatusBadRequest, "Параметры подключения некорректны")
		return
	}
	target, allowed := networkTunnelPrivateIPv4(in.Host)
	if !allowed {
		writeError(w, http.StatusBadRequest, "Укажите внутренний IPv4-адрес из частной сети")
		return
	}
	if !s.requireDeviceAccess(w, r, in.DeviceID) {
		return
	}
	var online bool
	if err := s.db.QueryRow(r.Context(), networkTunnelDeviceOnlineQuery, in.DeviceID).Scan(&online); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Agent не найден")
		return
	} else if err != nil || !online {
		writeError(w, http.StatusConflict, "Agent сейчас не в сети")
		return
	}

	listener, err := net.Listen("tcp4", ":0")
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Не удалось открыть внутренний браузерный мост")
		return
	}
	closeListener := true
	defer func() {
		if closeListener {
			_ = listener.Close()
		}
	}()
	guacdHost := envOr("REMOTEIT_GUACD_HOST", "guacd")
	guacdIPs, err := net.LookupIP(guacdHost)
	if err != nil || len(guacdIPs) == 0 {
		writeError(w, http.StatusServiceUnavailable, "HTML5-шлюз RDP/SSH недоступен")
		return
	}

	clientToken := randomToken(32)
	transaction, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось подготовить браузерное подключение")
		return
	}
	defer transaction.Rollback(r.Context())
	var lock, active int
	if err = transaction.QueryRow(r.Context(), `SELECT 1 FROM users WHERE id=$1 FOR UPDATE`, a.UserID).Scan(&lock); err == nil {
		err = transaction.QueryRow(r.Context(), `SELECT count(*) FROM network_tunnels WHERE created_by=$1 AND status IN ('waiting','connected') AND expires_at>now()`, a.UserID).Scan(&active)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить активные подключения")
		return
	}
	if active >= networkTunnelPerUserLimit {
		writeError(w, http.StatusTooManyRequests, "Сначала завершите одно из активных подключений")
		return
	}
	var tunnelID, jobID string
	var expiresAt time.Time
	if err = transaction.QueryRow(r.Context(), `INSERT INTO network_tunnels(device_id,created_by,protocol,target_host,target_port,client_token_hash,expires_at) VALUES($1,$2,$3,$4,$5,$6,now()+make_interval(secs=>$7)) RETURNING id,expires_at`, in.DeviceID, a.UserID, in.Protocol, target, in.Port, tokenHash(clientToken), int(networkTunnelLifetime/time.Second)).Scan(&tunnelID, &expiresAt); err == nil {
		jobPayload, _ := json.Marshal(map[string]any{"host": target, "port": in.Port, "protocol": in.Protocol, "tunnelId": tunnelID, "expiresAt": expiresAt.UTC().Format(time.RFC3339)})
		err = transaction.QueryRow(r.Context(), `INSERT INTO agent_jobs(device_id,created_by,job_type,payload,timeout_seconds,expires_at) VALUES($1,$2,'tunnel',$3,30,$4) RETURNING id`, in.DeviceID, a.UserID, jobPayload, expiresAt).Scan(&jobID)
	}
	if err != nil || transaction.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать браузерное подключение")
		return
	}
	closeListener = false
	go s.acceptBrowserTunnel(tunnelID, listener, guacdIPs, expiresAt)

	bridgePort := listener.Addr().(*net.TCPAddr).Port
	tunnelHost := envOr("REMOTEIT_GUACAMOLE_TUNNEL_HOST", "app")
	parameters := map[string]string{"hostname": tunnelHost, "port": strconv.Itoa(bridgePort)}
	if in.Username != "" {
		parameters["username"] = in.Username
	}
	if in.Password != "" {
		parameters["password"] = in.Password
	}
	if in.Protocol == "rdp" {
		if in.Domain != "" {
			parameters["domain"] = in.Domain
		}
		parameters["security"] = "any"
		parameters["ignore-cert"] = "true"
		parameters["resize-method"] = "display-update"
		parameters["enable-font-smoothing"] = "true"
		parameters["enable-desktop-composition"] = "true"
	} else {
		parameters["enable-sftp"] = "true"
	}
	connectionName := fmt.Sprintf("RemoteIt %s · %s", strings.ToUpper(in.Protocol), target)
	authPayload := map[string]any{
		"username":    "remoteit-" + a.UserID,
		"expires":     time.Now().Add(2 * time.Minute).UnixMilli(),
		"connections": map[string]any{connectionName: map[string]any{"id": tunnelID, "protocol": in.Protocol, "parameters": parameters}},
	}
	encrypted, err := encryptGuacamoleJSON(secret, authPayload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось подписать браузерное подключение")
		return
	}
	launchURL := "/guacamole/?" + url.Values{"data": []string{encrypted}}.Encode()
	s.audit(r.Context(), a, nil, "browser_connection.created", "network_tunnel", tunnelID, clientIP(r), map[string]any{"deviceId": in.DeviceID, "protocol": in.Protocol, "targetHost": target, "targetPort": in.Port, "jobId": jobID})
	writeJSON(w, http.StatusCreated, map[string]any{"id": tunnelID, "launchUrl": launchURL, "expiresAt": expiresAt})
}

func guacamoleJSONSecret() ([]byte, error) {
	value := strings.TrimSpace(os.Getenv("REMOTEIT_GUACAMOLE_JSON_SECRET"))
	secret, err := hex.DecodeString(value)
	if err != nil || len(secret) != 16 {
		return nil, errors.New("REMOTEIT_GUACAMOLE_JSON_SECRET must be 32 hexadecimal characters")
	}
	return secret, nil
}

func encryptGuacamoleJSON(secret []byte, payload any) (string, error) {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	signature := hmac.New(sha256.New, secret)
	_, _ = signature.Write(plaintext)
	message := append(signature.Sum(nil), plaintext...)
	padding := aes.BlockSize - len(message)%aes.BlockSize
	message = append(message, make([]byte, padding)...)
	for index := len(message) - padding; index < len(message); index++ {
		message[index] = byte(padding)
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return "", err
	}
	encrypted := make([]byte, len(message))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(encrypted, message)
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func (s *server) acceptBrowserTunnel(id string, listener net.Listener, allowed []net.IP, expiresAt time.Time) {
	defer listener.Close()
	if tcp, ok := listener.(*net.TCPListener); ok {
		_ = tcp.SetDeadline(expiresAt)
	}
	for {
		connection, err := listener.Accept()
		if err != nil {
			_, _ = s.db.Exec(context.Background(), `UPDATE network_tunnels SET status='failed',ended_at=now(),error_text='HTML5-шлюз не подключился к внутреннему мосту' WHERE id=$1 AND status='waiting'`, id)
			if value, ok := s.networkTunnels.Load(id); ok {
				value.(*networkTunnelRuntime).close(1000, "Browser gateway timeout")
			}
			s.networkTunnels.Delete(id)
			return
		}
		remoteIP := connection.RemoteAddr().(*net.TCPAddr).IP
		trusted := false
		for _, candidate := range allowed {
			if candidate.Equal(remoteIP) {
				trusted = true
				break
			}
		}
		if !trusted {
			_ = connection.Close()
			continue
		}
		if !s.attachNetworkTunnelTCP(id, connection) {
			_ = connection.Close()
			return
		}
		_, _ = s.db.Exec(context.Background(), `UPDATE network_tunnels SET client_connected_at=now() WHERE id=$1`, id)
		return
	}
}
