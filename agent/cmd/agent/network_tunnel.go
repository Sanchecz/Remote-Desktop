package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

var networkTunnelIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func startNetworkTunnel(parent context.Context, cfg *config, job *remoteJob) remoteJobResult {
	if cfg == nil || job == nil {
		return remoteJobResult{Success: false, Error: "туннель не содержит конфигурацию Agent", ExitCode: -1}
	}
	tunnelID := strings.ToLower(strings.TrimSpace(networkTunnelString(job.Payload["tunnelId"])))
	host := strings.TrimSpace(networkTunnelString(job.Payload["host"]))
	protocol := strings.ToLower(strings.TrimSpace(networkTunnelString(job.Payload["protocol"])))
	port, ok := agentTunnelPort(job.Payload["port"])
	targetIP := net.ParseIP(host)
	if !networkTunnelIDPattern.MatchString(tunnelID) || !ok || targetIP == nil || targetIP.To4() == nil || !targetIP.IsPrivate() || targetIP.IsLoopback() || targetIP.IsUnspecified() || targetIP.IsMulticast() || (protocol != "rdp" && protocol != "ssh") {
		return remoteJobResult{Success: false, Error: "сервер передал небезопасные параметры туннеля", ExitCode: -1}
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(networkTunnelString(job.Payload["expiresAt"])))
	if err != nil || expiresAt.Before(time.Now().Add(30*time.Second)) || expiresAt.After(time.Now().Add(2*time.Hour+5*time.Minute)) {
		return remoteJobResult{Success: false, Error: "срок действия туннеля некорректен", ExitCode: -1}
	}
	target, err := net.DialTimeout("tcp", net.JoinHostPort(targetIP.String(), strconv.Itoa(port)), 12*time.Second)
	if err != nil {
		return remoteJobResult{Success: false, Error: "внутренний адрес недоступен через Agent: " + err.Error(), ExitCode: -1}
	}
	endpoint, err := networkTunnelWebSocketURL(cfg.ServerURL, "/api/network-tunnels/"+url.PathEscape(tunnelID)+"/agent")
	if err != nil {
		target.Close()
		return remoteJobResult{Success: false, Error: err.Error(), ExitCode: -1}
	}
	connectContext, connectCancel := context.WithTimeout(parent, 20*time.Second)
	headers := http.Header{"X-Genesis-Device-Id": []string{cfg.DeviceID}, "Authorization": []string{"Device " + cfg.DeviceSecret}}
	connection, response, err := websocket.Dial(connectContext, endpoint, &websocket.DialOptions{HTTPHeader: headers, CompressionMode: websocket.CompressionDisabled})
	connectCancel()
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		target.Close()
		return remoteJobResult{Success: false, Error: "сервер не принял защищённый туннель: " + err.Error(), ExitCode: -1}
	}
	go relayAgentNetworkTunnel(parent, expiresAt, tunnelID, protocol, target, connection)
	return remoteJobResult{Success: true, Output: fmt.Sprintf("%s-туннель %s к %s:%d готов", strings.ToUpper(protocol), tunnelID, targetIP.String(), port), ExitCode: 0}
}

func relayAgentNetworkTunnel(parent context.Context, expiresAt time.Time, tunnelID, protocol string, target net.Conn, connection *websocket.Conn) {
	ctx, cancel := context.WithDeadline(parent, expiresAt)
	defer cancel()
	defer target.Close()
	defer connection.Close(websocket.StatusNormalClosure, "Agent tunnel ended")
	remote := websocket.NetConn(ctx, connection, websocket.MessageBinary)
	defer remote.Close()
	completed := make(chan struct{}, 2)
	copySide := func(destination, source net.Conn) {
		_, _ = io.Copy(destination, source)
		completed <- struct{}{}
	}
	go copySide(target, remote)
	go copySide(remote, target)
	select {
	case <-completed:
	case <-ctx.Done():
	}
	appendPublicAgentEvent("info", "network", strings.ToUpper(protocol)+"-туннель завершён", truncateText(tunnelID, 36))
}

func networkTunnelWebSocketURL(serverURL, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(serverURL), "/") + path)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("адрес сервера туннеля некорректен")
	}
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else {
		parsed.Scheme = "ws"
	}
	return parsed.String(), nil
}

func agentTunnelPort(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		port := int(typed)
		return port, typed == float64(port) && port >= 1 && port <= 65535
	case int:
		return typed, typed >= 1 && typed <= 65535
	case string:
		port, err := strconv.Atoi(typed)
		return port, err == nil && port >= 1 && port <= 65535
	default:
		return 0, false
	}
}

func networkTunnelString(value any) string {
	text, _ := value.(string)
	return text
}
