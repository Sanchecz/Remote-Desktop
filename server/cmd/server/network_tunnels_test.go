package main

import "testing"

func TestNetworkTunnelPrivateIPv4(t *testing.T) {
	for _, input := range []string{"10.0.0.25", "172.16.4.3", "192.168.1.105"} {
		if normalized, ok := networkTunnelPrivateIPv4(input); !ok || normalized != input {
			t.Fatalf("private address %q rejected as %q", input, normalized)
		}
	}
	for _, input := range []string{"", "127.0.0.1", "0.0.0.0", "8.8.8.8", "224.0.0.1", "::1", "example.org", "192.168.1.1:3389"} {
		if _, ok := networkTunnelPrivateIPv4(input); ok {
			t.Fatalf("unsafe tunnel target accepted: %q", input)
		}
	}
}

func TestValidNetworkTunnelUsername(t *testing.T) {
	for _, input := range []string{"", `DOMAIN\user`, "user@example.local", "Иван Петров"} {
		if !validNetworkTunnelUsername(input) {
			t.Fatalf("valid username rejected: %q", input)
		}
	}
	for _, input := range []string{"-oProxyCommand=bad", "user\r\nbad", "user\x00bad"} {
		if validNetworkTunnelUsername(input) {
			t.Fatalf("unsafe username accepted: %q", input)
		}
	}
}
