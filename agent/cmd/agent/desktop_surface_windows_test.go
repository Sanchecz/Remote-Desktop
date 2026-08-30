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

func TestRestoreDesktopCursorPatchUsesActualFrameStride(t *testing.T) {
	const width, height = 7, 5
	frame := make([]byte, width*height*4)
	for index := range frame {
		frame[index] = byte((index*17 + 9) % 251)
	}
	original := append([]byte(nil), frame...)
	patch := desktopCursorPatch{left: 2, top: 1, width: 3, height: 2}
	rowBytes := patch.width * 4
	saved := make([]byte, rowBytes*patch.height)
	for row := 0; row < patch.height; row++ {
		start := (patch.top+row)*width*4 + patch.left*4
		copy(saved[row*rowBytes:(row+1)*rowBytes], frame[start:start+rowBytes])
		for index := start; index < start+rowBytes; index++ {
			frame[index] = 0xff
		}
	}
	restoreDesktopCursorPatch(frame, width, patch, saved)
	if string(frame) != string(original) {
		t.Fatal("cursor patch restore changed pixels outside the cursor or used the wrong stride")
	}
}

func TestRestoreDesktopCursorPatchRejectsShortBackup(t *testing.T) {
	frame := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	original := append([]byte(nil), frame...)
	restoreDesktopCursorPatch(frame, 2, desktopCursorPatch{left: 0, top: 0, width: 2, height: 1}, []byte{9})
	if string(frame) != string(original) {
		t.Fatal("an incomplete cursor backup must not mutate the frame")
	}
}
