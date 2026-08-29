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

func TestDesktopPointerProjectionStressMatrix(t *testing.T) {
	coordinateFrames := [][2]int{{1, 1}, {1024, 768}, {1366, 768}, {1920, 1080}, {2256, 1504}, {2560, 1600}, {3440, 1440}, {3840, 2160}, {7680, 4320}, {1080, 1920}}
	virtualDesktops := []desktopCapture{
		{ScreenX: 0, ScreenY: 0, ScreenWidth: 1024, ScreenHeight: 768},
		{ScreenX: 0, ScreenY: 0, ScreenWidth: 3840, ScreenHeight: 2160},
		{ScreenX: -2560, ScreenY: -200, ScreenWidth: 6400, ScreenHeight: 2360},
		{ScreenX: 1920, ScreenY: 1080, ScreenWidth: 7680, ScreenHeight: 4320},
	}
	for _, capture := range virtualDesktops {
		for _, frame := range coordinateFrames {
			previousX := capture.ScreenX
			for step := -2; step <= 102; step++ {
				x := step * max(1, frame[0]-1) / 100
				y := (100 - step) * max(1, frame[1]-1) / 100
				screenX, screenY := desktopPointerScreenPoint(desktopInput{X: x, Y: y, CoordinateWidth: frame[0], CoordinateHeight: frame[1]}, capture)
				if screenX < capture.ScreenX || screenX > capture.ScreenX+capture.ScreenWidth-1 {
					t.Fatalf("%dx%d x=%d escaped horizontal desktop: %d", frame[0], frame[1], x, screenX)
				}
				if screenY < capture.ScreenY || screenY > capture.ScreenY+capture.ScreenHeight-1 {
					t.Fatalf("%dx%d y=%d escaped vertical desktop: %d", frame[0], frame[1], y, screenY)
				}
				if step >= 0 && step <= 100 && screenX < previousX {
					t.Fatalf("%dx%d projection moved backwards: %d < %d", frame[0], frame[1], screenX, previousX)
				}
				previousX = screenX
			}
			left, top := desktopPointerScreenPoint(desktopInput{X: 0, Y: 0, CoordinateWidth: frame[0], CoordinateHeight: frame[1]}, capture)
			right, bottom := desktopPointerScreenPoint(desktopInput{X: frame[0] - 1, Y: frame[1] - 1, CoordinateWidth: frame[0], CoordinateHeight: frame[1]}, capture)
			wantRight := capture.ScreenX + capture.ScreenWidth - 1
			wantBottom := capture.ScreenY + capture.ScreenHeight - 1
			if frame[0] <= 1 {
				wantRight = capture.ScreenX
			}
			if frame[1] <= 1 {
				wantBottom = capture.ScreenY
			}
			if left != capture.ScreenX || top != capture.ScreenY || right != wantRight || bottom != wantBottom {
				t.Fatalf("%dx%d endpoints mapped to (%d,%d)-(%d,%d)", frame[0], frame[1], left, top, right, bottom)
			}
		}
	}
}
