package main

import (
	"context"
	"encoding/json"
	"net"
	"sort"
	"strconv"
	"time"
)

type tcpProbePort struct {
	Port      int   `json:"port"`
	Open      bool  `json:"open"`
	LatencyMS int64 `json:"latencyMs"`
}

func executeTCPProbe(ctx context.Context, host string, ports []int) remoteJobResult {
	address := net.ParseIP(host)
	if address == nil || (!address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast()) {
		return failedAction("мониторинг разрешён только для внутреннего IP-адреса")
	}
	started := time.Now()
	results := make([]tcpProbePort, 0, len(ports))
	openCount := 0
	for _, port := range ports {
		probeStarted := time.Now()
		dialer := net.Dialer{Timeout: 900 * time.Millisecond}
		connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address.String(), strconv.Itoa(port)))
		result := tcpProbePort{Port: port, Open: err == nil, LatencyMS: max(time.Since(probeStarted).Milliseconds(), 1)}
		if err == nil {
			openCount++
			_ = connection.Close()
		}
		results = append(results, result)
	}
	sort.Slice(results, func(left, right int) bool { return results[left].Port < results[right].Port })
	status := "down"
	if openCount == len(results) && openCount > 0 {
		status = "ok"
	} else if openCount > 0 {
		status = "warning"
	}
	payload, err := json.Marshal(map[string]any{
		"host": host, "status": status, "openCount": openCount, "ports": results, "durationMs": time.Since(started).Milliseconds(), "checkedAt": time.Now().UTC(),
	})
	if err != nil {
		return failedAction("не удалось сформировать результат мониторинга")
	}
	return remoteJobResult{Success: true, Output: string(payload), ExitCode: 0}
}
