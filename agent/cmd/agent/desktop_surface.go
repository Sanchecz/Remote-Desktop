package main

import (
	"strings"
	"time"
)

const desktopStaticFrameHeartbeat = 5 * time.Second

func desktopRequiresSecureCapture(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && !strings.EqualFold(name, "default")
}

func desktopShouldPublishHeartbeat(lastPublished, now time.Time) bool {
	return lastPublished.IsZero() || !now.Before(lastPublished.Add(desktopStaticFrameHeartbeat))
}
