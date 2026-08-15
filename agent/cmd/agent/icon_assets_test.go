package main

import (
	"image/png"
	"os"
	"testing"
)

func TestNotificationStatusIconsUseExpectedBrandColor(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantOnline bool
	}{
		{name: "online", path: "assets/genesisit-online.png", wantOnline: true},
		{name: "offline", path: "assets/genesisit-offline.png", wantOnline: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := os.Open(test.path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			image, err := png.Decode(file)
			if err != nil {
				t.Fatal(err)
			}
			bounds := image.Bounds()
			r, g, b, a := image.At(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2).RGBA()
			if a == 0 {
				t.Fatal("brand diamond center is transparent")
			}
			if test.wantOnline && !(g > r && g > b) {
				t.Fatalf("online icon center is not green: r=%d g=%d b=%d", r, g, b)
			}
			if !test.wantOnline && !(r > g && r > b) {
				t.Fatalf("offline icon center is not red: r=%d g=%d b=%d", r, g, b)
			}
		})
	}
}

func TestAgentIconAtlasIsCompleteAndTransparent(t *testing.T) {
	file, err := os.Open("assets/remoteit-agent-icons.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	atlas, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	const cell = 256
	iconKinds := []string{"monitor", "panel", "log", "folder", "settings", "info", "bolt", "pencil", "link", "copy", "pulse", "service", "shield", "clock", "list", "arrow", "check", "circle-check", "dot", "update", "cpu"}
	iconColors := []string{"green", "ink", "muted", "white", "orange", "red"}
	wantWidth := cell * len(iconKinds)
	wantHeight := cell * len(iconColors)
	if atlas.Bounds().Dx() != wantWidth || atlas.Bounds().Dy() != wantHeight {
		t.Fatalf("unexpected atlas size: got %dx%d, want %dx%d", atlas.Bounds().Dx(), atlas.Bounds().Dy(), wantWidth, wantHeight)
	}
	for row := range iconColors {
		for column := range iconKinds {
			opaque := 0
			for y := row*cell + 2; y < (row+1)*cell-2; y++ {
				for x := column*cell + 2; x < (column+1)*cell-2; x++ {
					_, _, _, alpha := atlas.At(x, y).RGBA()
					if alpha > 0 {
						opaque++
					}
				}
			}
			if opaque < 40 {
				t.Fatalf("atlas cell %s:%s is empty or too small", iconColors[row], iconKinds[column])
			}
		}
	}
}

func TestDeviceMonitorIllustrationKeepsHighResolutionAlpha(t *testing.T) {
	file, err := os.Open("assets/remoteit-device-monitor.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	illustration, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if illustration.Bounds().Dx() < 512 || illustration.Bounds().Dy() < 512 {
		t.Fatalf("device monitor illustration is too small: %dx%d", illustration.Bounds().Dx(), illustration.Bounds().Dy())
	}
	for _, point := range [][2]int{{0, 0}, {illustration.Bounds().Dx() - 1, 0}, {0, illustration.Bounds().Dy() - 1}, {illustration.Bounds().Dx() - 1, illustration.Bounds().Dy() - 1}} {
		_, _, _, alpha := illustration.At(point[0], point[1]).RGBA()
		if alpha != 0 {
			t.Fatalf("device monitor corner %v must be transparent", point)
		}
	}
}
