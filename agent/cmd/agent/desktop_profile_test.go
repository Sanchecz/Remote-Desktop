package main

import "testing"

func TestDesktopProfileForFPS(t *testing.T) {
	tests := []struct {
		fps        int
		width      int
		quality    int
		fullChroma bool
	}{
		{fps: 15, width: 3840, quality: 94, fullChroma: true},
		{fps: 30, width: 3840, quality: 92, fullChroma: true},
		{fps: 60, width: 2560, quality: 88, fullChroma: true},
	}
	for _, test := range tests {
		profile := desktopProfileForFPS(test.fps)
		if profile.maxWidth != test.width || profile.quality != test.quality || profile.fullChroma != test.fullChroma {
			t.Fatalf("FPS %d: got %#v, want width=%d quality=%d fullChroma=%v", test.fps, profile, test.width, test.quality, test.fullChroma)
		}
	}
}

func TestDesktopProfileForInteractionKeepsGeometryAndRestoresSharpness(t *testing.T) {
	sharp := desktopProfileForInteraction(30, false)
	motion := desktopProfileForInteraction(30, true)
	if sharp.maxWidth != motion.maxWidth {
		t.Fatalf("interaction changed geometry: sharp=%#v motion=%#v", sharp, motion)
	}
	if !sharp.fullChroma || sharp.quality != 92 {
		t.Fatalf("resting profile lost sharpness: %#v", sharp)
	}
	if motion.fullChroma || motion.quality != 88 {
		t.Fatalf("interaction profile does not preserve readable text: %#v", motion)
	}
}
