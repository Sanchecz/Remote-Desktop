package main

import "testing"

func TestNetworkTunnelWebSocketURL(t *testing.T) {
	for input, expected := range map[string]string{
		"https://supportgenesis.ru": "wss://supportgenesis.ru/api/network-tunnels/test/agent",
		"http://127.0.0.1:8080/":    "ws://127.0.0.1:8080/api/network-tunnels/test/agent",
	} {
		actual, err := networkTunnelWebSocketURL(input, "/api/network-tunnels/test/agent")
		if err != nil || actual != expected {
			t.Fatalf("networkTunnelWebSocketURL(%q) = %q, %v", input, actual, err)
		}
	}
	for _, input := range []string{"", "ftp://supportgenesis.ru", "://broken"} {
		if _, err := networkTunnelWebSocketURL(input, "/api/network-tunnels/test/agent"); err == nil {
			t.Fatalf("unsafe server URL accepted: %q", input)
		}
	}
}

func TestAgentTunnelPort(t *testing.T) {
	for _, value := range []any{float64(3389), 22, "443"} {
		if _, ok := agentTunnelPort(value); !ok {
			t.Fatalf("valid port rejected: %#v", value)
		}
	}
	for _, value := range []any{float64(22.5), 0, 65536, "bad", nil} {
		if _, ok := agentTunnelPort(value); ok {
			t.Fatalf("invalid port accepted: %#v", value)
		}
	}
}
