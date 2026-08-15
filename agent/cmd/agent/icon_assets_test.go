package main

import (
	"bytes"
	"encoding/binary"
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

func TestWindowsExecutableIconUsesGreenBrandAtEverySize(t *testing.T) {
	data, err := os.ReadFile("../../assets/genesisit.ico")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 6 || binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		t.Fatal("invalid Windows ICO header")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count < 5 || len(data) < 6+count*16 {
		t.Fatalf("Windows ICO does not contain a complete multi-resolution icon set: count=%d", count)
	}
	for index := 0; index < count; index++ {
		entry := data[6+index*16 : 6+(index+1)*16]
		size := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if size <= 0 || offset < 0 || offset+size > len(data) {
			t.Fatalf("invalid ICO entry %d: offset=%d size=%d", index, offset, size)
		}
		frame, err := png.Decode(bytes.NewReader(data[offset : offset+size]))
		if err != nil {
			t.Fatalf("ICO entry %d is not the expected lossless PNG frame: %v", index, err)
		}
		bounds := frame.Bounds()
		r, g, b, a := frame.At(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2).RGBA()
		if a == 0 || !(g > r && g > b) {
			t.Fatalf("ICO entry %d center is not the green RemoteIt diamond: r=%d g=%d b=%d a=%d", index, r, g, b, a)
		}
		for _, point := range [][2]int{{bounds.Min.X, bounds.Min.Y}, {bounds.Max.X - 1, bounds.Min.Y}, {bounds.Min.X, bounds.Max.Y - 1}, {bounds.Max.X - 1, bounds.Max.Y - 1}} {
			cr, cg, cb, ca := frame.At(point[0], point[1]).RGBA()
			if ca > 0 && cr < 0x3000 && cg < 0x3000 && cb < 0x3000 {
				t.Fatalf("ICO entry %d has a black opaque corner at %v", index, point)
			}
		}
	}
}
