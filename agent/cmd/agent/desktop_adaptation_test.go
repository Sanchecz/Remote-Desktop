package main

import (
	"testing"
	"time"
)

func TestDesktopCaptureIntervalLeadsDWMWithoutChangingModeLimit(t *testing.T) {
	if got := desktopCaptureInterval(60); got != 12*time.Millisecond {
		t.Fatalf("60 FPS producer interval = %s", got)
	}
	if got := desktopCaptureInterval(30); got != 28*time.Millisecond {
		t.Fatalf("30 FPS producer interval = %s", got)
	}
	if got := desktopCaptureInterval(15); got != 64*time.Millisecond {
		t.Fatalf("15 FPS producer interval = %s", got)
	}
}

func TestDesktopAutoCadenceKeepsThirtyAcrossNormalHTTPSJitter(t *testing.T) {
	cadence := newDesktopAutoCadence()
	for index := 0; index < 200; index++ {
		cadence.Observe(50 * time.Millisecond)
	}
	if cadence.FPS != 30 {
		t.Fatalf("normal transport jitter selected %d FPS, want 30", cadence.FPS)
	}
}

func TestDesktopAutoCadenceDropsOnlyOnSustainedCongestionAndRecovers(t *testing.T) {
	cadence := newDesktopAutoCadence()
	for index := 0; index < 19; index++ {
		cadence.Observe(140 * time.Millisecond)
	}
	if cadence.FPS != 30 {
		t.Fatalf("cadence dropped before sustained threshold: %d", cadence.FPS)
	}
	cadence.Observe(140 * time.Millisecond)
	if cadence.FPS != 15 {
		t.Fatalf("sustained congestion selected %d FPS, want 15", cadence.FPS)
	}
	for index := 0; index < 20; index++ {
		cadence.Observe(40 * time.Millisecond)
	}
	if cadence.FPS != 30 {
		t.Fatalf("recovered transport selected %d FPS, want 30", cadence.FPS)
	}
}

func TestDesktopAutoCadenceUsesSixtyOnlyForFastTransport(t *testing.T) {
	cadence := newDesktopAutoCadence()
	for index := 0; index < 300; index++ {
		cadence.Observe(10 * time.Millisecond)
	}
	if cadence.FPS != 60 {
		t.Fatalf("fast transport selected %d FPS, want 60", cadence.FPS)
	}
	for index := 0; index < 5; index++ {
		cadence.Observe(30 * time.Millisecond)
	}
	if cadence.FPS != 30 {
		t.Fatalf("slower transport retained %d FPS, want 30", cadence.FPS)
	}
}
