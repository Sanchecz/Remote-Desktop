package main

import "testing"

func TestDesktopInteractiveProfileBalancesQualityAndWireSize(t *testing.T) {
	sharp := desktopProfileForInteraction(15, true)
	if sharp.quality != 92 || sharp.chroma != desktopJPEGChroma422 {
		t.Fatalf("15 FPS profile must prioritize sharp text: %#v", sharp)
	}
	for _, fps := range []int{0, 30} {
		profile := desktopProfileForInteraction(fps, true)
		if profile.quality != 90 || profile.chroma != desktopJPEGChroma422 {
			t.Fatalf("interactive quality is not balanced at %d FPS: %#v", fps, profile)
		}
	}
	smooth := desktopProfileForInteraction(60, true)
	if smooth.quality != 85 || smooth.chroma != desktopJPEGChroma420 {
		t.Fatalf("60 FPS motion profile must remain transport bounded: %#v", smooth)
	}
}

func TestDesktopRestProfileRemainsSharp(t *testing.T) {
	profile := desktopProfileForInteraction(30, false)
	if profile.maxWidth != 3840 || profile.quality != 92 || profile.chroma != desktopJPEGChroma444 {
		t.Fatalf("unexpected 30 FPS rest profile: %#v", profile)
	}
}

func TestDesktopInteractionWidth(t *testing.T) {
	if width := desktopInteractionWidth(15, 3840); width != 2560 {
		t.Fatalf("15 FPS interaction width = %d", width)
	}
	if width := desktopInteractionWidth(30, 3840); width != 1920 {
		t.Fatalf("30 FPS interaction width = %d", width)
	}
	if width := desktopInteractionWidth(60, 2560); width != 1920 {
		t.Fatalf("60 FPS interaction width = %d", width)
	}
}
