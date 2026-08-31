package main

import "time"

const (
	// Service time is normalized by the number of fresh upload lanes. These
	// thresholds therefore describe aggregate frame throughput rather than WAN
	// round-trip latency: <15 ms can sustain 60 FPS, >40 ms cannot sustain a
	// useful 30 FPS stream, and <28 ms has enough headroom to recover from 15.
	desktopAutoFastSample = 15 * time.Millisecond
	// Sixty consecutive fast uploads are two seconds of evidence at the safe
	// 30 FPS starting cadence. The previous 90-sample window made a healthy
	// local/Wi-Fi session look artificially capped for about three seconds.
	// Demotion still needs only eight slow 60 FPS samples, so a path that cannot
	// sustain the promoted cadence falls back before a latency queue can form.
	desktopAutoPromoteSamples   = 60
	desktopAutoCongestedSample  = 40 * time.Millisecond
	desktopAutoCongestedSamples = 20
	desktopAutoSixtySlowSample  = 22 * time.Millisecond
	desktopAutoSixtySlowSamples = 8
	desktopAutoRecoverySample   = 28 * time.Millisecond
	desktopAutoRecoverySamples  = 20
	desktopAutoVideoLaneCount   = 6
	// A dropped capture means every latest-only upload lane was already writing.
	// Treat several clustered drops as direct aggregate transport pressure, but
	// let isolated scheduling races decay without changing the visible profile.
	desktopAutoDropPressureStep    = 3
	desktopAutoDropPressureDecay   = 1
	desktopAutoSixtyDropPressure   = 6
	desktopAutoConstrainedPressure = 12
	// After a failed 60 FPS probe, remain at the sharp 30 FPS profile long
	// enough to avoid visible resolution/quality oscillation on a borderline
	// link. Explicit 60 FPS remains unaffected.
	desktopAutoPromotionCooldown = 10 * time.Second
	// Once Auto has selected its lighter 30 FPS profile, keep it long enough to
	// measure the recovered path rather than oscillating between a 4K keyframe
	// and the constrained profile every few dozen frames.
	desktopAutoConstraintHold = 5 * time.Second
	// A lane that completed a frame recently contributes real in-flight capacity.
	// This is deliberately longer than one 30 FPS round-robin cycle but short
	// enough to forget an auxiliary websocket soon after a proxy drops it.
	desktopAutoLaneFreshness = 2 * time.Second
)

// desktopCaptureInterval is the hard producer limit for the selected mode.
// Capture is paced from absolute deadlines, so capture/encode time does not get
// added to every period.  Do not schedule ahead of the advertised cadence:
// doing so generated about 80 JPEGs/s in the 60 FPS mode on a real 1080p host,
// wasting CPU and bandwidth while the viewer could only display the newest 60.
func desktopCaptureInterval(targetFPS int) time.Duration {
	switch targetFPS {
	case 60:
		return time.Second / 60
	case 30:
		return time.Second / 30
	case 15:
		return time.Second / 15
	default:
		return time.Second / 30
	}
}

// desktopAutoCadence adapts only when the transport is clearly outside the
// current cadence. Short HTTPS jitter must not force a healthy 20-30 FPS link
// into a permanent 15 FPS cap.
type desktopAutoCadence struct {
	FPS              int
	maximumFPS       int
	Constrained      bool
	stable           int
	slow             int
	promoteAllowedAt time.Time
	constraintUntil  time.Time
	laneSuccessfulAt [desktopAutoVideoLaneCount]time.Time
	dropPressure     int
}

func newDesktopAutoCadence() desktopAutoCadence {
	return desktopAutoCadence{FPS: 30, maximumFPS: 60}
}

func (cadence *desktopAutoCadence) Reset() {
	*cadence = newDesktopAutoCadence()
}

// SetMaximumFPS keeps Auto from trading native desktop geometry for cadence.
// A 2560/4K source promoted to the transport-bounded Full-HD 60 FPS profile
// looked visibly softer even on an excellent connection because the viewer had
// to upscale every frame. Users can still explicitly select 60 FPS when motion
// matters more than native detail; Auto keeps its sharp 30 FPS default on
// high-DPI/ultrawide desktops and may promote only native Full-HD sources.
func (cadence *desktopAutoCadence) SetMaximumFPS(maximumFPS int) {
	if maximumFPS != 30 && maximumFPS != 60 {
		maximumFPS = 30
	}
	cadence.maximumFPS = maximumFPS
	if cadence.FPS > maximumFPS {
		cadence.FPS = maximumFPS
		cadence.stable = 0
		cadence.slow = 0
	}
}

func desktopAutoMaximumFPS(screenWidth int) int {
	if screenWidth > 0 && screenWidth <= 1920 {
		return 60
	}
	return 30
}

// Observe records a completed upload lane separately from capture/encode work.
// RemoteIt deliberately uses several latest-only websocket lanes: six 45 ms
// writes can sustain substantially more than 60 frames/s in aggregate, so the
// old max(upload, processing) calculation incorrectly treated ordinary WAN RTT
// as a single-lane bottleneck and demoted Auto to 30 FPS. Dividing only the
// transport service time by the number of recently successful lanes models the
// actual pipeline capacity while still requiring capture/encode to fit inside
// the cadence on its own.
func (cadence *desktopAutoCadence) Observe(uploadDuration, processingDuration time.Duration, lane int, observedAt time.Time) {
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	if lane >= 0 && lane < len(cadence.laneSuccessfulAt) {
		cadence.laneSuccessfulAt[lane] = observedAt
	}
	activeLanes := 0
	for _, successfulAt := range cadence.laneSuccessfulAt {
		age := observedAt.Sub(successfulAt)
		// Upload lanes finish concurrently, so results may reach the coordinator
		// a few milliseconds out of completion order. A future timestamp must not
		// make that lane look fresh relative to an older result.
		if !successfulAt.IsZero() && age >= 0 && age <= desktopAutoLaneFreshness {
			activeLanes++
		}
	}
	if activeLanes < 1 {
		activeLanes = 1
	}
	cadence.dropPressure = max(0, cadence.dropPressure-desktopAutoDropPressureDecay)
	transportService := uploadDuration / time.Duration(activeLanes)
	serviceDuration := max(transportService, processingDuration)
	switch cadence.FPS {
	case 60:
		cadence.stable = 0
		if serviceDuration > desktopAutoSixtySlowSample {
			cadence.slow++
		} else {
			cadence.slow = 0
		}
		if cadence.slow >= desktopAutoSixtySlowSamples {
			cadence.FPS = 30
			cadence.slow = 0
			cadence.promoteAllowedAt = observedAt.Add(desktopAutoPromotionCooldown)
		}
	default:
		cadence.FPS = 30
		if cadence.Constrained {
			cadence.slow = 0
			if !observedAt.Before(cadence.constraintUntil) && serviceDuration < desktopAutoRecoverySample {
				cadence.stable++
			} else {
				cadence.stable = 0
			}
			if cadence.stable >= desktopAutoRecoverySamples {
				cadence.Constrained = false
				cadence.stable = 0
				cadence.promoteAllowedAt = observedAt.Add(desktopAutoPromotionCooldown)
			}
			return
		}
		if serviceDuration > desktopAutoCongestedSample {
			cadence.slow++
			cadence.stable = 0
		} else {
			cadence.slow = 0
			if !observedAt.Before(cadence.promoteAllowedAt) && serviceDuration < desktopAutoFastSample {
				cadence.stable++
			} else {
				cadence.stable = 0
			}
		}
		if cadence.slow >= desktopAutoCongestedSamples {
			// Reducing cadence to 15 used to select the sharpest 4K/q94 profile at
			// the exact moment the host or link was already overloaded. Slow VMware
			// capture could then collapse to 2-4 delivered FPS and never recover.
			// Keep the honest 30 FPS clock and temporarily use the existing
			// 2560/q84 motion profile, which reduces both encode and wire service
			// time without building a frame queue.
			cadence.Constrained = true
			cadence.slow = 0
			cadence.constraintUntil = observedAt.Add(desktopAutoConstraintHold)
		} else if cadence.maximumFPS >= 60 && cadence.stable >= desktopAutoPromoteSamples {
			cadence.FPS = 60
			cadence.stable = 0
		}
	}
}

// ObserveDropped records a capture that could not start on any of the six
// unbuffered transport lanes. It intentionally does not fabricate an upload
// duration or mark a lane healthy. A single dropped frame can be ordinary
// scheduler jitter and decays after successful writes; repeated drops are
// stronger evidence of saturation than a completed write divided by the
// apparent lane count.
func (cadence *desktopAutoCadence) ObserveDropped(processingDuration time.Duration, observedAt time.Time) {
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	cadence.stable = 0
	cadence.dropPressure = min(desktopAutoConstrainedPressure, cadence.dropPressure+desktopAutoDropPressureStep)

	if cadence.FPS >= 60 {
		if cadence.dropPressure >= desktopAutoSixtyDropPressure {
			cadence.FPS = 30
			cadence.slow = 0
			cadence.dropPressure = 0
			cadence.promoteAllowedAt = observedAt.Add(desktopAutoPromotionCooldown)
		}
		return
	}

	cadence.FPS = 30
	// A slow encoder plus fully occupied lanes is already a host-side backlog;
	// allow that combination to reach the bounded profile one drop sooner.
	if processingDuration > desktopAutoCongestedSample {
		cadence.dropPressure = min(desktopAutoConstrainedPressure, cadence.dropPressure+desktopAutoDropPressureStep)
	}
	if cadence.dropPressure >= desktopAutoConstrainedPressure {
		cadence.Constrained = true
		cadence.slow = 0
		cadence.dropPressure = 0
		cadence.constraintUntil = observedAt.Add(desktopAutoConstraintHold)
	}
}

// desktopNextFrameDeadline establishes the next real presentation slot from
// the frame that actually started. A stalled VDI/GDI capture must count as a
// dropped slot, not create a catch-up burst with a near-zero following gap.
func desktopNextFrameDeadline(frameStartedAt time.Time, interval time.Duration) time.Time {
	if frameStartedAt.IsZero() {
		frameStartedAt = time.Now()
	}
	if interval <= 0 {
		interval = time.Second / 30
	}
	return frameStartedAt.Add(interval)
}
