package main

import "time"

const (
	desktopAutoFastSample = 20 * time.Millisecond
	// Sixty consecutive fast uploads are two seconds of evidence at the safe
	// 30 FPS starting cadence. The previous 90-sample window made a healthy
	// local/Wi-Fi session look artificially capped for about three seconds.
	// Demotion still needs only eight slow 60 FPS samples, so a path that cannot
	// sustain the promoted cadence falls back before a latency queue can form.
	desktopAutoPromoteSamples   = 60
	desktopAutoCongestedSample  = 110 * time.Millisecond
	desktopAutoCongestedSamples = 20
	desktopAutoSixtySlowSample  = 28 * time.Millisecond
	desktopAutoSixtySlowSamples = 8
	desktopAutoRecoverySample   = 65 * time.Millisecond
	desktopAutoRecoverySamples  = 20
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
	FPS    int
	stable int
	slow   int
}

func newDesktopAutoCadence() desktopAutoCadence {
	return desktopAutoCadence{FPS: 30}
}

func (cadence *desktopAutoCadence) Reset() {
	*cadence = newDesktopAutoCadence()
}

func (cadence *desktopAutoCadence) Observe(uploadDuration time.Duration) {
	switch cadence.FPS {
	case 15:
		cadence.slow = 0
		if uploadDuration < desktopAutoRecoverySample {
			cadence.stable++
		} else {
			cadence.stable = 0
		}
		if cadence.stable >= desktopAutoRecoverySamples {
			cadence.FPS = 30
			cadence.stable = 0
		}
	case 60:
		cadence.stable = 0
		if uploadDuration > desktopAutoSixtySlowSample {
			cadence.slow++
		} else {
			cadence.slow = 0
		}
		if cadence.slow >= desktopAutoSixtySlowSamples {
			cadence.FPS = 30
			cadence.slow = 0
		}
	default:
		cadence.FPS = 30
		if uploadDuration > desktopAutoCongestedSample {
			cadence.slow++
			cadence.stable = 0
		} else {
			cadence.slow = 0
			if uploadDuration < desktopAutoFastSample {
				cadence.stable++
			} else {
				cadence.stable = 0
			}
		}
		if cadence.slow >= desktopAutoCongestedSamples {
			cadence.FPS = 15
			cadence.slow = 0
		} else if cadence.stable >= desktopAutoPromoteSamples {
			cadence.FPS = 60
			cadence.stable = 0
		}
	}
}
