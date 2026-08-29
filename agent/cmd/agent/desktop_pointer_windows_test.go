package main

import "testing"

func TestDesktopPointerUsesPacketCoordinateSpace(t *testing.T) {
	capture := desktopCapture{FrameWidth: 1600, FrameHeight: 900, ScreenX: 0, ScreenY: 0, ScreenWidth: 3840, ScreenHeight: 2160}
	event := desktopInput{Type: "pointer", X: 960, Y: 540, CoordinateWidth: 1920, CoordinateHeight: 1080}
	x, y := desktopPointerScreenPoint(event, capture)
	if x != 1920 || y != 1080 {
		t.Fatalf("packet midpoint projected to %d,%d; want 1920,1080", x, y)
	}
}

func TestDesktopPointerSupportsVirtualDesktopOffsetsAndNonstandardFrames(t *testing.T) {
	capture := desktopCapture{FrameWidth: 1920, FrameHeight: 804, ScreenX: -1920, ScreenY: -200, ScreenWidth: 5360, ScreenHeight: 1640}
	event := desktopInput{Type: "pointer", X: 1720, Y: 720, CoordinateWidth: 3440, CoordinateHeight: 1440}
	x, y := desktopPointerScreenPoint(event, capture)
	if x != 759 || y != 620 {
		t.Fatalf("virtual desktop point projected to %d,%d; want 759,620", x, y)
	}
}

func TestDesktopPointerKeepsLegacyCaptureBasis(t *testing.T) {
	capture := desktopCapture{FrameWidth: 1600, FrameHeight: 900, ScreenWidth: 2560, ScreenHeight: 1440}
	x, y := desktopPointerScreenPoint(desktopInput{Type: "pointer", X: 800, Y: 450}, capture)
	if x != 1280 || y != 720 {
		t.Fatalf("legacy midpoint projected to %d,%d; want 1280,720", x, y)
	}
}

func TestDesktopPointerReachesEdgesAndClampsStalePackets(t *testing.T) {
	capture := desktopCapture{ScreenX: -2560, ScreenY: 0, ScreenWidth: 6400, ScreenHeight: 2160}
	left, top := desktopPointerScreenPoint(desktopInput{X: -50, Y: -1, CoordinateWidth: 2256, CoordinateHeight: 1504}, capture)
	right, bottom := desktopPointerScreenPoint(desktopInput{X: 9999, Y: 9999, CoordinateWidth: 2256, CoordinateHeight: 1504}, capture)
	if left != -2560 || top != 0 {
		t.Fatalf("negative packet projected to %d,%d; want virtual top-left", left, top)
	}
	if right != 3839 || bottom != 2159 {
		t.Fatalf("oversized packet projected to %d,%d; want virtual bottom-right", right, bottom)
	}
}
