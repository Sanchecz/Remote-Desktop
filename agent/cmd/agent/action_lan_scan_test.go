package main

import (
	"net"
	"testing"
)

func TestUsableIPv4AddressesBounded(t *testing.T) {
	_, network, err := net.ParseCIDR("192.168.42.0/24")
	if err != nil {
		t.Fatal(err)
	}
	addresses := usableIPv4Addresses(network)
	if len(addresses) != 254 || addresses[0].String() != "192.168.42.1" || addresses[len(addresses)-1].String() != "192.168.42.254" {
		t.Fatalf("unexpected /24 host range: count=%d first=%v last=%v", len(addresses), addresses[0], addresses[len(addresses)-1])
	}
	_, tooLarge, _ := net.ParseCIDR("10.0.0.0/16")
	if got := usableIPv4Addresses(tooLarge); got != nil {
		t.Fatalf("oversized network produced %d targets", len(got))
	}
}

func TestResolveLANScanNetworkRejectsPublicAndOversized(t *testing.T) {
	for _, candidate := range []string{"8.8.8.0/24", "10.0.0.0/16", "invalid"} {
		if _, _, err := resolveLANScanNetwork(candidate); err == nil {
			t.Fatalf("unsafe scan network accepted: %s", candidate)
		}
	}
}
