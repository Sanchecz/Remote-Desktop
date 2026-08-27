package main

import (
	"testing"
	"time"
)

func TestDesktopRequiresSecureCapture(t *testing.T) {
	tests := []struct {
		name   string
		secure bool
	}{
		{name: "default", secure: false},
		{name: "DEFAULT", secure: false},
		{name: "winlogon", secure: true},
		{name: "CredUI", secure: true},
		{name: "disconnect", secure: true},
		{name: "", secure: false},
	}
	for _, test := range tests {
		if actual := desktopRequiresSecureCapture(test.name); actual != test.secure {
			t.Fatalf("desktopRequiresSecureCapture(%q) = %v, want %v", test.name, actual, test.secure)
		}
	}
}

func TestDesktopStaticFrameHeartbeat(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	if !desktopShouldPublishHeartbeat(time.Time{}, now) {
		t.Fatal("the first frame must be published")
	}
	if desktopShouldPublishHeartbeat(now.Add(-desktopStaticFrameHeartbeat+time.Millisecond), now) {
		t.Fatal("a static frame must not be republished before the heartbeat deadline")
	}
	if !desktopShouldPublishHeartbeat(now.Add(-desktopStaticFrameHeartbeat), now) {
		t.Fatal("a static frame must be republished at the heartbeat deadline")
	}
}
