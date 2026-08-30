package main

import (
	"testing"
	"time"
)

func TestDesktopInteractionWindowRestoresSharpFramePromptly(t *testing.T) {
	inputAt := time.Unix(100, 0)
	if !desktopInteractionIsActive(true, inputAt, inputAt.Add(desktopInteractionQualityWindow-time.Millisecond)) {
		t.Fatal("recent input must keep the bounded motion profile")
	}
	if desktopInteractionIsActive(true, inputAt, inputAt.Add(desktopInteractionQualityWindow)) {
		t.Fatal("the sharp rest profile must return at the interaction deadline")
	}
	if desktopInteractionIsActive(false, inputAt, inputAt.Add(time.Millisecond)) {
		t.Fatal("view-only sessions must not use the motion profile")
	}
	if desktopInteractionIsActive(true, time.Time{}, inputAt) {
		t.Fatal("a session without input must start with the sharp profile")
	}
	if desktopInteractionIsActive(true, inputAt, inputAt.Add(-time.Millisecond)) {
		t.Fatal("a clock rollback must not keep motion mode active")
	}
}

func TestDesktopInteractiveProfileBalancesQualityAndWireSize(t *testing.T) {
	sharp := desktopProfileForInteraction(15, true, false, 2560)
	if sharp.quality != 92 || sharp.chroma != desktopJPEGChroma422 {
		t.Fatalf("15 FPS profile must prioritize sharp text: %#v", sharp)
	}
	for _, fps := range []int{0, 30} {
		profile := desktopProfileForInteraction(fps, true, false, 2560)
		if profile.quality != 86 || profile.chroma != desktopJPEGChroma422 {
			t.Fatalf("interactive quality is not balanced at %d FPS: %#v", fps, profile)
		}
	}
	smooth := desktopProfileForInteraction(60, true, false, 1920)
	if smooth.maxWidth != 1920 || smooth.quality != 79 || smooth.chroma != desktopJPEGChroma422 {
		t.Fatalf("60 FPS motion profile must remain transport bounded: %#v", smooth)
	}
	restingSmooth := desktopProfileForInteraction(60, false, false, 1920)
	if restingSmooth != smooth {
		t.Fatalf("60 FPS profile must not spike when interaction stops: rest=%#v motion=%#v", restingSmooth, smooth)
	}
}

func TestDesktopRestProfileRemainsSharp(t *testing.T) {
	profile := desktopProfileForInteraction(30, false, false, 2256)
	if profile.maxWidth != 3840 || profile.quality != 92 || profile.chroma != desktopJPEGChroma444 {
		t.Fatalf("unexpected 30 FPS rest profile: %#v", profile)
	}
}

func TestDesktopNotebookInteractionKeepsSharpChroma(t *testing.T) {
	profile := desktopProfileForInteraction(30, true, false, 2256)
	if profile.quality != 90 || profile.chroma != desktopJPEGChroma444 {
		t.Fatalf("2256px notebook motion must keep readable coloured text: %#v", profile)
	}
	constrained := desktopProfileForInteraction(30, true, true, 2256)
	if constrained.quality != 84 || constrained.chroma != desktopJPEGChroma422 {
		t.Fatalf("constrained Auto must retain its recovery budget: %#v", constrained)
	}
}

func TestDesktopInteractionWidth(t *testing.T) {
	if width := desktopInteractionWidth(15, 3840); width != 2560 {
		t.Fatalf("15 FPS interaction width = %d", width)
	}
	if width := desktopInteractionWidth(30, 3840); width != 2560 {
		t.Fatalf("30 FPS interaction width = %d", width)
	}
	if width := desktopInteractionWidth(60, 1920); width != 1920 {
		t.Fatalf("60 FPS interaction width = %d", width)
	}
}

func TestDesktopOutputGeometryPreservesNotebookAndBoundsHighResolutionMotion(t *testing.T) {
	tests := []struct {
		name                     string
		baseWidth, baseHeight    int
		fps                      int
		interactive, constrained bool
		wantWidth, wantHeight    int
	}{
		{name: "notebook-rest", baseWidth: 2256, baseHeight: 1504, fps: 30, wantWidth: 2256, wantHeight: 1504},
		{name: "notebook-motion", baseWidth: 2256, baseHeight: 1504, fps: 30, interactive: true, wantWidth: 2256, wantHeight: 1504},
		{name: "4k-rest", baseWidth: 3840, baseHeight: 2160, fps: 30, wantWidth: 3840, wantHeight: 2160},
		{name: "4k-motion", baseWidth: 3840, baseHeight: 2160, fps: 30, interactive: true, wantWidth: 2560, wantHeight: 1440},
		{name: "4k-constrained", baseWidth: 3840, baseHeight: 2160, fps: 30, constrained: true, wantWidth: 2560, wantHeight: 1440},
		{name: "ultrawide-motion", baseWidth: 3440, baseHeight: 1440, fps: 30, interactive: true, wantWidth: 2560, wantHeight: 1071},
		{name: "60-fps", baseWidth: 1920, baseHeight: 1080, fps: 60, interactive: true, wantWidth: 1920, wantHeight: 1080},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			width, height := desktopOutputGeometry(test.baseWidth, test.baseHeight, test.fps, test.interactive, test.constrained)
			if width != test.wantWidth || height != test.wantHeight {
				t.Fatalf("geometry = %dx%d, want %dx%d", width, height, test.wantWidth, test.wantHeight)
			}
		})
	}
	if width, height := desktopOutputGeometry(0, 1080, 30, true, false); width != 0 || height != 0 {
		t.Fatalf("invalid geometry = %dx%d", width, height)
	}
}

func TestDesktopRealtimeScalerIsReservedForSixtyFPSMotion(t *testing.T) {
	for _, fps := range []int{0, 15, 30} {
		if desktopUseRealtimeScaler(fps, true) {
			t.Fatalf("%d FPS must use the higher-quality scaler", fps)
		}
	}
	if !desktopUseRealtimeScaler(60, true) {
		t.Fatal("60 FPS interaction must retain the low-latency scaler")
	}
	if desktopUseRealtimeScaler(60, false) {
		t.Fatal("idle frames must always use the higher-quality scaler")
	}
}
