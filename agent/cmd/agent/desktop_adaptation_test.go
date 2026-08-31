package main

import (
	"testing"
	"time"
)

func TestDesktopCaptureIntervalMatchesAdvertisedModeLimit(t *testing.T) {
	if got := desktopCaptureInterval(60); got != time.Second/60 {
		t.Fatalf("60 FPS producer interval = %s", got)
	}
	if got := desktopCaptureInterval(30); got != time.Second/30 {
		t.Fatalf("30 FPS producer interval = %s", got)
	}
	if got := desktopCaptureInterval(15); got != time.Second/15 {
		t.Fatalf("15 FPS producer interval = %s", got)
	}
}

func TestDesktopAutoCadenceKeepsThirtyAcrossNormalHTTPSJitter(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(1, 0)
	for index := 0; index < 200; index++ {
		cadence.Observe(45*time.Millisecond, 18*time.Millisecond, index%desktopAutoVideoLaneCount, now.Add(time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 30 {
		t.Fatalf("normal transport jitter selected %d FPS, want 30", cadence.FPS)
	}
}

func TestDesktopAutoCadenceConstrainsQualityOnlyOnSustainedCongestionAndRecovers(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(1, 0)
	for index := 0; index < 19; index++ {
		cadence.Observe(140*time.Millisecond, 8*time.Millisecond, 0, now.Add(time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 30 || cadence.Constrained {
		t.Fatalf("cadence constrained before sustained threshold: %#v", cadence)
	}
	cadence.Observe(140*time.Millisecond, 8*time.Millisecond, 0, now.Add(19*time.Second/30))
	if cadence.FPS != 30 || !cadence.Constrained {
		t.Fatalf("sustained congestion did not select bounded 30 FPS: %#v", cadence)
	}
	for index := 0; index < desktopAutoRecoverySamples+1; index++ {
		cadence.Observe(40*time.Millisecond, 18*time.Millisecond, index%desktopAutoVideoLaneCount, now.Add(7*time.Second+time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 30 || cadence.Constrained {
		t.Fatalf("recovered transport retained constrained profile: %#v", cadence)
	}
}

func TestDesktopAutoCadenceUsesSixtyOnlyForFastTransport(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(1, 0)
	for index := 0; index < desktopAutoPromoteSamples-1; index++ {
		cadence.Observe(10*time.Millisecond, 7*time.Millisecond, 0, now.Add(time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 30 {
		t.Fatalf("fast transport promoted before hysteresis threshold: %d", cadence.FPS)
	}
	cadence.Observe(10*time.Millisecond, 7*time.Millisecond, 0, now.Add(time.Duration(desktopAutoPromoteSamples)*time.Second/30))
	if cadence.FPS != 60 {
		t.Fatalf("fast transport selected %d FPS, want 60", cadence.FPS)
	}
	for index := 0; index < desktopAutoSixtySlowSamples; index++ {
		cadence.Observe(30*time.Millisecond, 8*time.Millisecond, 0, now.Add(3*time.Second+time.Duration(index)*time.Second/60))
	}
	if cadence.FPS != 30 {
		t.Fatalf("slower transport retained %d FPS, want 30", cadence.FPS)
	}
	for index := 0; index < desktopAutoPromoteSamples*2; index++ {
		cadence.Observe(10*time.Millisecond, 7*time.Millisecond, 0, now.Add(4*time.Second+time.Duration(index)*time.Second/60))
	}
	if cadence.FPS != 30 {
		t.Fatalf("failed 60 FPS probe repromoted during cooldown: %d", cadence.FPS)
	}
	for index := 0; index < desktopAutoPromoteSamples; index++ {
		cadence.Observe(10*time.Millisecond, 7*time.Millisecond, 0, now.Add(14*time.Second+time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 60 {
		t.Fatalf("healthy transport did not promote after cooldown: %d", cadence.FPS)
	}
}

func TestDesktopAutoCadenceKeepsNativeDetailAboveFullHD(t *testing.T) {
	cadence := newDesktopAutoCadence()
	cadence.SetMaximumFPS(desktopAutoMaximumFPS(2560))
	now := time.Unix(1, 0)
	for index := 0; index < desktopAutoPromoteSamples*2; index++ {
		cadence.Observe(8*time.Millisecond, 6*time.Millisecond, index%desktopAutoVideoLaneCount, now.Add(time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 30 {
		t.Fatalf("high-resolution Auto selected %d FPS and would downscale the source", cadence.FPS)
	}

	cadence.SetMaximumFPS(desktopAutoMaximumFPS(1920))
	for index := 0; index < desktopAutoPromoteSamples; index++ {
		cadence.Observe(8*time.Millisecond, 6*time.Millisecond, index%desktopAutoVideoLaneCount, now.Add(5*time.Second+time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 60 {
		t.Fatalf("Full-HD Auto selected %d FPS, want 60 after sustained headroom", cadence.FPS)
	}
}

func TestDesktopAutoCadenceDemotesWhenDisplayBecomesHighResolution(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(1, 0)
	for index := 0; index < desktopAutoPromoteSamples; index++ {
		cadence.Observe(8*time.Millisecond, 6*time.Millisecond, index%desktopAutoVideoLaneCount, now.Add(time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 60 {
		t.Fatalf("precondition: Full-HD cadence did not promote: %d", cadence.FPS)
	}
	cadence.SetMaximumFPS(desktopAutoMaximumFPS(3840))
	if cadence.FPS != 30 || cadence.stable != 0 || cadence.slow != 0 {
		t.Fatalf("high-resolution display did not atomically restore sharp Auto 30: %#v", cadence)
	}
}

func TestDesktopAutoMaximumFPSIsResolutionAware(t *testing.T) {
	if got := desktopAutoMaximumFPS(1920); got != 60 {
		t.Fatalf("1920 px maximum = %d, want 60", got)
	}
	if got := desktopAutoMaximumFPS(1921); got != 30 {
		t.Fatalf("1921 px maximum = %d, want 30", got)
	}
	if got := desktopAutoMaximumFPS(0); got != 30 {
		t.Fatalf("unknown display maximum = %d, want safe 30", got)
	}
}

func TestDesktopAutoCadenceDoesNotPromoteOnBorderlineTransport(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(1, 0)
	for index := 0; index < desktopAutoPromoteSamples*2; index++ {
		cadence.Observe(22*time.Millisecond, 8*time.Millisecond, 0, now.Add(time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 30 {
		t.Fatalf("borderline transport selected %d FPS, want stable 30", cadence.FPS)
	}
}

func TestDesktopAutoCadenceUsesAggregateMultiLaneCapacity(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(10, 0)
	// A 45 ms WAN write is slower than a 30 FPS frame interval in isolation,
	// but six healthy latest-only lanes provide ample aggregate capacity.
	for index := 0; index < desktopAutoPromoteSamples+desktopAutoVideoLaneCount; index++ {
		cadence.Observe(45*time.Millisecond, 8*time.Millisecond, index%desktopAutoVideoLaneCount, now.Add(time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 60 {
		t.Fatalf("multi-lane WAN capacity selected %d FPS, want 60", cadence.FPS)
	}
}

func TestDesktopAutoCadenceForgetsStaleAccelerationLanes(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(10, 0)
	for lane := 0; lane < desktopAutoVideoLaneCount; lane++ {
		cadence.Observe(45*time.Millisecond, 8*time.Millisecond, lane, now.Add(time.Duration(lane)*time.Millisecond))
	}
	// Once auxiliary lanes disappear, a 45 ms primary lane cannot sustain
	// 30 FPS. Stale concurrency must not be counted forever, so Auto selects the
	// bounded 30 FPS profile instead of generating oversized frames the link
	// cannot send.
	late := now.Add(desktopAutoLaneFreshness + time.Second)
	for index := 0; index < desktopAutoPromoteSamples*2; index++ {
		cadence.Observe(45*time.Millisecond, 8*time.Millisecond, 0, late.Add(time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 30 || !cadence.Constrained {
		t.Fatalf("stale lanes did not constrain Auto: %#v", cadence)
	}
}

func TestDesktopAutoCadenceDoesNotCountFutureLaneCompletions(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(10, 0)
	// Lane one finishes later but its result reaches the coordinator first.
	// Observing an older lane zero completion afterwards must not count the
	// future completion as capacity available at the older instant.
	cadence.Observe(45*time.Millisecond, 8*time.Millisecond, 1, now.Add(time.Second))
	for index := 0; index < desktopAutoPromoteSamples*2; index++ {
		cadence.Observe(45*time.Millisecond, 8*time.Millisecond, 0, now.Add(time.Duration(index)*time.Millisecond))
	}
	if cadence.FPS != 30 || !cadence.Constrained {
		t.Fatalf("out-of-order lane completion did not constrain Auto: %#v", cadence)
	}
}

func TestDesktopAutoCadenceHoldsConstrainedProfileBeforeRecovery(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(100, 0)
	for index := 0; index < desktopAutoCongestedSamples; index++ {
		cadence.Observe(180*time.Millisecond, 55*time.Millisecond, 0, now.Add(time.Duration(index)*time.Second/30))
	}
	if !cadence.Constrained {
		t.Fatal("expected sustained processing pressure to constrain Auto")
	}
	for index := 0; index < desktopAutoRecoverySamples*3; index++ {
		cadence.Observe(8*time.Millisecond, 8*time.Millisecond, index%desktopAutoVideoLaneCount, now.Add(time.Second+time.Duration(index)*time.Second/30))
	}
	if !cadence.Constrained {
		t.Fatal("Auto left its constrained profile before the hold elapsed")
	}
}

func TestDesktopAutoCadenceIgnoresAndDecaysIsolatedDroppedFrame(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(200, 0)
	cadence.ObserveDropped(8*time.Millisecond, now)
	if cadence.FPS != 30 || cadence.Constrained {
		t.Fatalf("isolated dropped frame changed Auto profile: %#v", cadence)
	}
	for index := 0; index < desktopAutoDropPressureStep; index++ {
		cadence.Observe(8*time.Millisecond, 8*time.Millisecond, index%desktopAutoVideoLaneCount, now.Add(time.Duration(index+1)*time.Second/30))
	}
	if cadence.dropPressure != 0 {
		t.Fatalf("healthy uploads did not decay isolated pressure: %#v", cadence)
	}
}

func TestDesktopAutoCadenceConstrainsAfterClusteredTransportDrops(t *testing.T) {
	cadence := newDesktopAutoCadence()
	now := time.Unix(300, 0)
	for index := 0; index < desktopAutoConstrainedPressure/desktopAutoDropPressureStep; index++ {
		cadence.ObserveDropped(8*time.Millisecond, now.Add(time.Duration(index)*time.Second/30))
	}
	if cadence.FPS != 30 || !cadence.Constrained {
		t.Fatalf("clustered transport drops did not select bounded Auto profile: %#v", cadence)
	}
}

func TestDesktopAutoCadenceDemotesSixtyAfterClusteredTransportDrops(t *testing.T) {
	cadence := newDesktopAutoCadence()
	cadence.FPS = 60
	now := time.Unix(400, 0)
	for index := 0; index < desktopAutoSixtyDropPressure/desktopAutoDropPressureStep; index++ {
		cadence.ObserveDropped(8*time.Millisecond, now.Add(time.Duration(index)*time.Second/60))
	}
	if cadence.FPS != 30 || cadence.Constrained {
		t.Fatalf("clustered 60 FPS drops did not demote to sharp 30 FPS: %#v", cadence)
	}
	if cadence.promoteAllowedAt.Before(now.Add(desktopAutoPromotionCooldown)) {
		t.Fatalf("failed 60 FPS cadence did not establish a promotion cooldown: %#v", cadence)
	}
}

func TestDesktopNextFrameDeadlineDropsMissedSlotWithoutCatchUp(t *testing.T) {
	started := time.Unix(500, 0)
	interval := time.Second / 60
	deadline := desktopNextFrameDeadline(started, interval)
	if want := started.Add(interval); !deadline.Equal(want) {
		t.Fatalf("deadline = %v, want %v", deadline, want)
	}
	// A slow frame that started much later establishes a fresh full interval;
	// it must not inherit a nearly-expired deadline from an earlier slot.
	lateStarted := started.Add(187 * time.Millisecond)
	lateDeadline := desktopNextFrameDeadline(lateStarted, interval)
	if !lateDeadline.Equal(lateStarted.Add(interval)) {
		t.Fatalf("late deadline = %v", lateDeadline)
	}
}
