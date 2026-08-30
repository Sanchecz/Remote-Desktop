package main

import "time"

type desktopCaptureProfile struct {
	maxWidth int
	quality  int
	chroma   desktopJPEGChroma
}

type desktopJPEGChroma uint8

const (
	desktopJPEGChroma420 desktopJPEGChroma = iota
	desktopJPEGChroma422
	desktopJPEGChroma444
)

func desktopCanReuseBoundedJPEG(
	desired desktopCaptureProfile,
	encodedQuality int,
	encodedChroma desktopJPEGChroma,
	oversizeUntil time.Time,
	now time.Time,
) bool {
	if encodedQuality == desired.quality && encodedChroma == desired.chroma {
		return true
	}
	return !oversizeUntil.IsZero() && now.Before(oversizeUntil)
}

// desktopBoundedEncodingProfile returns the requested JPEG profile followed by
// progressively more transport-efficient fallbacks. The fallback is selected
// only when the encoded frame itself exceeds the bounded wire limit; ordinary
// desktop content therefore keeps the configured sharp 4:4:4 profile.
//
// preferFallback is used for a short period after an oversized frame. It avoids
// spending CPU on an encode that is already known not to fit while the same
// high-entropy animation or photo remains on screen.
func desktopBoundedEncodingProfile(profile desktopCaptureProfile, attempt int, preferFallback bool) (quality int, chroma desktopJPEGChroma, ok bool) {
	if !preferFallback {
		if attempt == 0 {
			return profile.quality, profile.chroma, true
		}
		attempt--
	}
	switch attempt {
	case 0:
		return max(72, profile.quality-6), desktopJPEGChroma422, true
	case 1:
		return max(68, profile.quality-12), desktopJPEGChroma420, true
	default:
		return 0, desktopJPEGChroma420, false
	}
}

func desktopProfileForFPS(targetFPS int) desktopCaptureProfile {
	switch {
	case targetFPS <= 15:
		// The low-cadence mode is also the sharpest mode. It is intended for
		// administration where small text matters more than motion, so preserve up
		// through a native 4K desktop and spend the saved frame budget on JPEG
		// quality instead of sending a small image that the viewer has to upscale.
		return desktopCaptureProfile{maxWidth: 3840, quality: 94, chroma: desktopJPEGChroma444}
	case targetFPS >= 60:
		// Sixty FPS must remain transport-bounded even when the remote desktop is
		// changing without administrator input (video, animation, an installer or
		// another user). Keeping the former 2560/q88/4:4:4 idle profile in that
		// situation could saturate the connection before an interaction window was
		// ever observed, so the viewer received fewer frames and input fell behind.
		// A representative administration desktop measured about 94 Mbit/s at
		// Full HD q79/4:2:2 and 60 FPS, including the complete JPEG payload. This
		// leaves transport and input headroom on a 100 Mbit/s path while retaining
		// the same reconstructed detail as the previous q85/4:2:0 motion profile
		// and visibly cleaner coloured text edges.
		return desktopCaptureProfile{maxWidth: 1920, quality: 79, chroma: desktopJPEGChroma422}
	default:
		// Auto and explicit 30 FPS are the normal high-quality profile. Preserve a
		// full 4K administration desktop and use a visually lossless JPEG
		// level: small Windows text must stay readable without switching to the
		// slower 15 FPS profile.
		return desktopCaptureProfile{maxWidth: 3840, quality: 92, chroma: desktopJPEGChroma444}
	}
}
