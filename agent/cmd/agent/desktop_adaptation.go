package main

import "time"

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
		if uploadDuration < 65*time.Millisecond {
			cadence.stable++
		} else {
			cadence.stable = 0
		}
		if cadence.stable >= 20 {
			cadence.FPS = 30
			cadence.stable = 0
		}
	case 60:
		cadence.stable = 0
		if uploadDuration > 24*time.Millisecond {
			cadence.slow++
		} else {
			cadence.slow = 0
		}
		if cadence.slow >= 5 {
			cadence.FPS = 30
			cadence.slow = 0
		}
	default:
		cadence.FPS = 30
		if uploadDuration > 110*time.Millisecond {
			cadence.slow++
			cadence.stable = 0
		} else {
			cadence.slow = 0
			if uploadDuration < 14*time.Millisecond {
				cadence.stable++
			} else {
				cadence.stable = 0
			}
		}
		if cadence.slow >= 20 {
			cadence.FPS = 15
			cadence.slow = 0
		} else if cadence.stable >= 300 {
			cadence.FPS = 60
			cadence.stable = 0
		}
	}
}
