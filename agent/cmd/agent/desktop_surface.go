package main

import (
	"strings"
	"time"
)

const (
	desktopStaticFrameHeartbeat    = 5 * time.Second
	desktopVDIRecoveryRestartAfter = 4 * time.Second
)

func desktopRequiresSecureCapture(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && !strings.EqualFold(name, "default")
}

func desktopShouldPublishHeartbeat(lastPublished, now time.Time) bool {
	return lastPublished.IsZero() || !now.Before(lastPublished.Add(desktopStaticFrameHeartbeat))
}

// desktopVDIRecoveryDelay prevents a disconnected virtual display from
// spinning the capture loop every two milliseconds. The first retries remain
// quick enough for a transient RDP reconnect, then settle at one attempt per
// second until the interactive worker is replaced.
func desktopVDIRecoveryDelay(failures int) time.Duration {
	switch {
	case failures <= 1:
		return 100 * time.Millisecond
	case failures == 2:
		return 250 * time.Millisecond
	case failures == 3:
		return 500 * time.Millisecond
	default:
		return time.Second
	}
}

func desktopVDIRecoveryShouldRestart(startedAt, now time.Time) bool {
	return !startedAt.IsZero() && !now.Before(startedAt.Add(desktopVDIRecoveryRestartAfter))
}

func desktopVDIShouldUseUserTokenFallback(err error) bool {
	if err == nil {
		return false
	}
	// DXGI reports HRESULT 0x80070005 when the VDI display adapter rejects the
	// LocalSystem token after an RDP reconnect. A fresh process is still required,
	// but it must be launched with the exact SID-bound user's primary token.
	return strings.Contains(strings.ToLower(err.Error()), "80070005")
}
