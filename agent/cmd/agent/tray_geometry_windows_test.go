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
		{name: "full hd", screenWidth: 1920, screenHeight: 1080, maxWidth: 1220, maxHeight: 886},
		{name: "small laptop", screenWidth: 1366, screenHeight: 768, maxWidth: 1147, maxHeight: 629},
		{name: "four k", screenWidth: 3840, screenHeight: 2160, maxWidth: 1220, maxHeight: 913},
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
