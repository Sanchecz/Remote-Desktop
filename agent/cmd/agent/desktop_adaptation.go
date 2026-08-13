package main

import "time"

// desktopCaptureInterval is the producer deadline, not the advertised output
// limit. Windows/DWM, DXGI acquisition and the Go scheduler add a small fixed
// delay between two deadlines; scheduling at exactly 16.67/33.33 ms therefore
// produced only 52/28 observable surface frames. A modest acquisition lead
// lets DXGI meet the next DWM presentation without queuing or duplicating old
// images. The desktop itself still caps genuinely new frames at its refresh
// rate, and all transport queues remain latest-only.
func desktopCaptureInterval(targetFPS int) time.Duration {
	switch targetFPS {
	case 60:
		return 12 * time.Millisecond
	case 30:
		return 28 * time.Millisecond
	case 15:
		return 64 * time.Millisecond
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
