package main

import (
	"net"
	"reflect"
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

func TestParseLANScanTargetsSupportsMultipleCIDRsAndCrossSubnetRange(t *testing.T) {
	ranges, addresses, err := parseLANScanTargets("192.168.10.0/30, 10.20.0.5; 192.168.1.254-192.168.2.2")
	if err != nil {
		t.Fatal(err)
	}
	wantRanges := []string{"192.168.10.0/30", "10.20.0.5", "192.168.1.254-192.168.2.2"}
	if !reflect.DeepEqual(ranges, wantRanges) {
		t.Fatalf("unexpected normalized ranges: got=%v want=%v", ranges, wantRanges)
	}
	got := make([]string, 0, len(addresses))
	for _, address := range addresses {
		got = append(got, address.String())
	}
	want := []string{"10.20.0.5", "192.168.1.254", "192.168.1.255", "192.168.2.0", "192.168.2.1", "192.168.2.2", "192.168.10.1", "192.168.10.2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected targets: got=%v want=%v", got, want)
	}
}

func TestParseLANScanTargetsRejectsPublicReversedAndOversized(t *testing.T) {
	for _, candidate := range []string{
		"8.8.8.8",
		"192.168.2.5-192.168.1.5",
		"10.0.0.0/21",
		"192.168.0.0/22,192.168.4.0/22",
	} {
		if _, _, err := parseLANScanTargets(candidate); err == nil {
			t.Fatalf("unsafe scan selection accepted: %s", candidate)
		}
	}
}

func TestLANInterfaceScorePrefersPhysicalLAN(t *testing.T) {
	physical := lanInterfaceScore("Ethernet", net.ParseIP("192.168.1.10"), 24)
	virtual := lanInterfaceScore("WireGuard Tunnel", net.ParseIP("10.6.7.1"), 24)
	if physical <= virtual {
		t.Fatalf("physical LAN must outrank a tunnel: physical=%d virtual=%d", physical, virtual)
	}
}

func TestParseLANNeighborTableIsLanguageIndependentAndPrivate(t *testing.T) {
	input := []byte("Interface: 192.168.1.10 --- 0x9\n" +
		"  Internet Address      Physical Address      Type\n" +
		"  192.168.1.1           aa-bb-cc-dd-ee-ff     dynamic\n" +
		"  192.168.1.20          11-22-33-44-55-66     динамический\n" +
		"  192.168.1.1           aa-bb-cc-dd-ee-ff     dynamic\n" +
		"  8.8.8.8               00-00-00-00-00-00     static\n" +
		"  224.0.0.22            01-00-5e-00-00-16     static\n")
	neighbors := parseLANNeighborTable(input)
	got := make([]string, 0, len(neighbors))
	for _, neighbor := range neighbors {
		got = append(got, neighbor.String())
	}
	want := []string{"192.168.1.1", "192.168.1.20"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected neighbor table: got=%v want=%v", got, want)
	}
}
