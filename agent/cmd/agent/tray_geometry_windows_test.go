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
		{name: "full hd", screenWidth: 1920, screenHeight: 1080, maxWidth: 1366, maxHeight: 1022},
		{name: "small laptop", screenWidth: 1366, screenHeight: 768, maxWidth: 965, maxHeight: 722},
		{name: "four k", screenWidth: 3840, screenHeight: 2160, maxWidth: 1450, maxHeight: 1085},
		{name: "reference workstation", screenWidth: 1840, screenHeight: 1162, maxWidth: 1450, maxHeight: 1085},
		{name: "invalid metrics fallback", screenWidth: 0, screenHeight: 0, maxWidth: 1366, maxHeight: 1022},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			size := compactAgentWindowPixels(test.screenWidth, test.screenHeight)
			if size.Width > test.maxWidth || size.Height > test.maxHeight {
				t.Fatalf("window %dx%d exceeds %dx%d", size.Width, size.Height, test.maxWidth, test.maxHeight)
			}
			ratioError := math.Abs(float64(size.Width)/float64(size.Height) - 1450.0/1085.0)
			if ratioError > 0.003 {
				t.Fatalf("window %dx%d does not preserve the 1450:1085 reference ratio", size.Width, size.Height)
			}
		})
	}
}

func TestCompactAgentWindowPixelsMatchesReferenceAndLaptopGeometry(t *testing.T) {
	reference := compactAgentWindowPixels(1840, 1162)
	if reference.Width != 1450 || reference.Height != 1085 {
		t.Fatalf("reference workstation=%dx%d, want 1450x1085", reference.Width, reference.Height)
	}
	laptop := compactAgentWindowPixels(1366, 768)
	if laptop.Width < 940 || laptop.Height < 700 {
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
