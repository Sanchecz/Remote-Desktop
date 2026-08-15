//go:build windows

package main

import (
	"math"
	"testing"
)

func TestCompactAgentWindowPixels(t *testing.T) {
	tests := []struct {
		name         string
		screenWidth  int
		screenHeight int
		maxWidth     int
		maxHeight    int
	}{
		{name: "full hd", screenWidth: 1920, screenHeight: 1080, maxWidth: 1536, maxHeight: 1024},
		{name: "small laptop", screenWidth: 1366, screenHeight: 768, maxWidth: 1068, maxHeight: 712},
		{name: "four k", screenWidth: 3840, screenHeight: 2160, maxWidth: 1536, maxHeight: 1024},
		{name: "reference workstation", screenWidth: 1840, screenHeight: 1162, maxWidth: 1536, maxHeight: 1024},
		{name: "invalid metrics fallback", screenWidth: 0, screenHeight: 0, maxWidth: 1536, maxHeight: 1024},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			size := compactAgentWindowPixels(test.screenWidth, test.screenHeight)
			if size.Width > test.maxWidth || size.Height > test.maxHeight {
				t.Fatalf("window %dx%d exceeds %dx%d", size.Width, size.Height, test.maxWidth, test.maxHeight)
			}
			ratioError := math.Abs(float64(size.Width)/float64(size.Height) - 1536.0/1024.0)
			if ratioError > 0.003 {
				t.Fatalf("window %dx%d does not preserve the 1536:1024 reference ratio", size.Width, size.Height)
			}
		})
	}
}

func TestCompactAgentWindowPixelsMatchesReferenceAndLaptopGeometry(t *testing.T) {
	reference := compactAgentWindowPixels(1920, 1080)
	if reference.Width != 1536 || reference.Height != 1024 {
		t.Fatalf("reference workstation=%dx%d, want 1536x1024", reference.Width, reference.Height)
	}
	laptop := compactAgentWindowPixels(1366, 768)
	if laptop.Width < 1060 || laptop.Height < 700 {
		t.Fatalf("laptop window %dx%d is smaller than the usable compact composition", laptop.Width, laptop.Height)
	}
}

func TestAgentUIScaleAppliesSingleDPIConversion(t *testing.T) {
	for _, test := range []struct{ dpi, want int }{{96, 100}, {120, 80}, {144, 67}, {192, 50}} {
		scale := newAgentUIScale(test.dpi)
		if got := scale.unit(100); got != test.want {
			t.Fatalf("dpi %d: logical reference unit=%d, want %d", test.dpi, got, test.want)
		}
		// One point size is preserved in physical terms after Windows applies DPI;
		// individual dashboard roles define their own deliberate hierarchy.
		physical := float64(scale.font(10)) * float64(test.dpi) / 96
		if physical < 9 || physical > 11 {
			t.Fatalf("dpi %d: physical type scale %.1f is outside the reference band", test.dpi, physical)
		}
	}
}

func TestAgentSeverityDistinguishesReconnectFromCriticalFailure(t *testing.T) {
	tests := []struct {
		name                      string
		connected, running, known bool
		want                      agentConnectionSeverity
	}{
		{name: "healthy", connected: true, running: true, known: true, want: agentStatusHealthy},
		{name: "vpn change reconnect", connected: false, running: true, known: true, want: agentStatusReconnecting},
		{name: "service stopped", connected: false, running: false, known: true, want: agentStatusCritical},
		{name: "state unknown", connected: false, running: false, known: false, want: agentStatusCritical},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := agentSeverityFor(test.connected, test.running, test.known); got != test.want {
				t.Fatalf("severity=%v, want %v", got, test.want)
			}
		})
	}
}
