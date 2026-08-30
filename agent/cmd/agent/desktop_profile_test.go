package main

import (
	"testing"
	"time"
)

func TestDesktopCanReuseBoundedJPEGOnlyDuringBackoff(t *testing.T) {
	desired := desktopCaptureProfile{quality: 92, chroma: desktopJPEGChroma444}
	now := time.Unix(100, 0)
	if !desktopCanReuseBoundedJPEG(desired, 92, desktopJPEGChroma444, time.Time{}, now) {
		t.Fatal("the exact requested encoding must remain reusable")
	}
	if !desktopCanReuseBoundedJPEG(desired, 84, desktopJPEGChroma422, now.Add(time.Second), now) {
		t.Fatal("a bounded fallback should be reused during its CPU-protection backoff")
	}
	if desktopCanReuseBoundedJPEG(desired, 84, desktopJPEGChroma422, now, now) {
		t.Fatal("an expired fallback must retry the requested quality on an unchanged desktop")
	}
}

func TestDesktopProfileForFPS(t *testing.T) {
	tests := []struct {
		fps     int
		width   int
		quality int
		chroma  desktopJPEGChroma
	}{
		{fps: 15, width: 3840, quality: 94, chroma: desktopJPEGChroma444},
		{fps: 30, width: 3840, quality: 92, chroma: desktopJPEGChroma444},
		{fps: 60, width: 1920, quality: 79, chroma: desktopJPEGChroma422},
	}
	for _, test := range tests {
		profile := desktopProfileForFPS(test.fps)
		if profile.maxWidth != test.width || profile.quality != test.quality || profile.chroma != test.chroma {
			t.Fatalf("FPS %d: got %#v, want width=%d quality=%d chroma=%v", test.fps, profile, test.width, test.quality, test.chroma)
		}
	}
}

func TestDesktopProfileForInteractionKeepsGeometryAndRestoresSharpness(t *testing.T) {
	sharp := desktopProfileForInteraction(30, false, false, 2560)
	motion := desktopProfileForInteraction(30, true, false, 2560)
	if sharp.maxWidth != motion.maxWidth {
		t.Fatalf("interaction changed geometry: sharp=%#v motion=%#v", sharp, motion)
	}
	if sharp.chroma != desktopJPEGChroma444 || sharp.quality != 92 {
		t.Fatalf("resting profile lost sharpness: %#v", sharp)
	}
	if motion.chroma != desktopJPEGChroma422 || motion.quality != 86 {
		t.Fatalf("interaction profile does not preserve readable text: %#v", motion)
	}
}

func TestDesktopBoundedEncodingProfilesPreserveRequestedQualityBeforeFallback(t *testing.T) {
	profile := desktopCaptureProfile{quality: 92, chroma: desktopJPEGChroma444}
	tests := []struct {
		attempt int
		quality int
		chroma  desktopJPEGChroma
		ok      bool
	}{
		{attempt: 0, quality: 92, chroma: desktopJPEGChroma444, ok: true},
		{attempt: 1, quality: 86, chroma: desktopJPEGChroma422, ok: true},
		{attempt: 2, quality: 80, chroma: desktopJPEGChroma420, ok: true},
		{attempt: 3, ok: false},
	}
	for _, test := range tests {
		quality, chroma, ok := desktopBoundedEncodingProfile(profile, test.attempt, false)
		if quality != test.quality || chroma != test.chroma || ok != test.ok {
			t.Fatalf("attempt %d: got quality=%d chroma=%v ok=%v, want quality=%d chroma=%v ok=%v", test.attempt, quality, chroma, ok, test.quality, test.chroma, test.ok)
		}
	}
}

func TestDesktopBoundedEncodingProfilesSkipKnownOversizedProfile(t *testing.T) {
	profile := desktopCaptureProfile{quality: 94, chroma: desktopJPEGChroma444}
	quality, chroma, ok := desktopBoundedEncodingProfile(profile, 0, true)
	if !ok || quality != 88 || chroma != desktopJPEGChroma422 {
		t.Fatalf("unexpected first fallback: quality=%d chroma=%v ok=%v", quality, chroma, ok)
	}
	quality, chroma, ok = desktopBoundedEncodingProfile(profile, 1, true)
	if !ok || quality != 82 || chroma != desktopJPEGChroma420 {
		t.Fatalf("unexpected second fallback: quality=%d chroma=%v ok=%v", quality, chroma, ok)
	}
	if _, _, ok = desktopBoundedEncodingProfile(profile, 2, true); ok {
		t.Fatal("known oversized profile exposed an unbounded third fallback")
	}
}

func TestDesktopBoundedEncodingProfilesClampLowQuality(t *testing.T) {
	profile := desktopCaptureProfile{quality: 70, chroma: desktopJPEGChroma422}
	quality, chroma, ok := desktopBoundedEncodingProfile(profile, 1, false)
	if !ok || quality != 72 || chroma != desktopJPEGChroma422 {
		t.Fatalf("unexpected first clamped fallback: quality=%d chroma=%v ok=%v", quality, chroma, ok)
	}
	quality, chroma, ok = desktopBoundedEncodingProfile(profile, 2, false)
	if !ok || quality != 68 || chroma != desktopJPEGChroma420 {
		t.Fatalf("unexpected second clamped fallback: quality=%d chroma=%v ok=%v", quality, chroma, ok)
	}
}
