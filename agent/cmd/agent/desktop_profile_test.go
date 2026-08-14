package main

import "testing"

func TestDesktopProfileForFPS(t *testing.T) {
	tests := []struct {
		fps     int
		width   int
		quality int
		chroma  desktopJPEGChroma
	}{
		{fps: 15, width: 3840, quality: 94, chroma: desktopJPEGChroma444},
		{fps: 30, width: 3840, quality: 92, chroma: desktopJPEGChroma444},
		{fps: 60, width: 2560, quality: 88, chroma: desktopJPEGChroma444},
	}
	for _, test := range tests {
		profile := desktopProfileForFPS(test.fps)
		if profile.maxWidth != test.width || profile.quality != test.quality || profile.chroma != test.chroma {
			t.Fatalf("FPS %d: got %#v, want width=%d quality=%d chroma=%v", test.fps, profile, test.width, test.quality, test.chroma)
		}
	}
}

func TestDesktopProfileForInteractionKeepsGeometryAndRestoresSharpness(t *testing.T) {
	sharp := desktopProfileForInteraction(30, false)
	motion := desktopProfileForInteraction(30, true)
	if sharp.maxWidth != motion.maxWidth {
		t.Fatalf("interaction changed geometry: sharp=%#v motion=%#v", sharp, motion)
	}
	if sharp.chroma != desktopJPEGChroma444 || sharp.quality != 92 {
		t.Fatalf("resting profile lost sharpness: %#v", sharp)
	}
	if motion.chroma != desktopJPEGChroma422 || motion.quality != 90 {
		t.Fatalf("interaction profile does not preserve readable text: %#v", motion)
	}
}
