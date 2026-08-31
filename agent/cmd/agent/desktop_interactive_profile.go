package main

import "time"

// Motion frames are intentionally transport-bounded, but keeping that profile
// for too long after the last input makes text look soft after every click. Two
// to seven 30 FPS frames are enough to absorb the visual transition without
// wasting a sharp 4:4:4 encode on every pointer update.
const desktopInteractionQualityWindow = 220 * time.Millisecond

func desktopInteractionIsActive(controlEnabled bool, lastInputAt, now time.Time) bool {
	if !controlEnabled || lastInputAt.IsZero() || now.Before(lastInputAt) {
		return false
	}
	return now.Sub(lastInputAt) < desktopInteractionQualityWindow
}

// desktopProfileForInteraction keeps direct user interaction separate from an
// Auto-mode congestion fallback. A healthy 1080p/2256px desktop can afford
// sharp 4:4:4 chroma while the pointer moves; a genuinely constrained link must
// retain the smaller 4:2:2 profile until it recovers.
func desktopProfileForInteraction(targetFPS int, interactive, constrained bool, outputWidth int) desktopCaptureProfile {
	return desktopProfileForInteractionMode(targetFPS, interactive, constrained, outputWidth, false)
}

func desktopProfileForInteractionMode(targetFPS int, interactive, constrained bool, outputWidth int, preserveDetail bool) desktopCaptureProfile {
	// Auto is a 30 FPS mode until the adaptive controller has enough evidence
	// to select 15 or 60. Treating its API value (0) as "below 15" produced a
	// needless 4:4:4 bandwidth spike during the first interactive frames.
	if targetFPS == 0 {
		targetFPS = 30
	}
	profile := desktopProfileForCapture(targetFPS, preserveDetail)
	if constrained {
		// Congestion is sustained evidence from the cadence controller, not merely
		// a recent click. Keep the established recovery profile even on smaller
		// displays so Auto cannot oscillate between high and low wire budgets.
		if targetFPS < 60 {
			profile.quality = min(profile.quality, 84)
			profile.chroma = desktopJPEGChroma422
		}
	} else if interactive {
		// Motion frames only live for a few dozen milliseconds. The selected
		// surfaces below spend approximately the same wire budget in every mode:
		// 15 FPS can preserve a full 4K frame, 30 FPS preserves a 2K-class frame,
		// and 60 FPS preserves Full HD. This removes the former 1600px quality cliff
		// while still leaving room for input packets on a 100 Mbit/s WAN link. The
		// sharp 4:4:4 rest frame is restored immediately after input stops.
		if targetFPS >= 60 {
			// The 60 FPS base profile is deliberately bounded for autonomous motion
			// as well as direct input. Keep that profile unchanged here so entering or
			// leaving the interaction window cannot produce a bandwidth spike or a
			// visible chroma-quality switch.
		} else if targetFPS <= 15 {
			profile.quality = min(profile.quality, 92)
			profile.chroma = desktopJPEGChroma422
		} else if outputWidth > 0 && outputWidth <= 2304 {
			// Full HD and the 2256px 3:2 notebook class remain native. Preserving
			// chroma here makes ClearType and coloured terminal text materially
			// cleaner, while Auto can still enter the constrained profile when the
			// measured encode/upload cadence proves the path cannot sustain it.
			// q90 is the lowest TurboJPEG quality at which fine 125-150% DPI
			// ClearType text on the native 2256x1504 notebook surface remains
			// visually stable while dragging a window. The former q88 profile was
			// transport-safe but still showed block boundaries around dark glyphs.
			// Six latest-only lanes and the drop-pressure controller keep this small
			// quality increase from turning into a latency queue.
			profile.quality = min(profile.quality, 90)
			profile.chroma = desktopJPEGChroma444
		} else {
			// Keep native 2K-class geometry while the mouse moves. On the reference
			// administration desktop q86/4:2:2 at 2560 px measured about 83 Mbit/s
			// and 44.8 dB, versus 77.6 Mbit/s and 44.4 dB at q84. The small wire
			// increase is worthwhile for text and thin window borders, while still
			// leaving substantially more headroom than the rejected 2560 px 60 FPS
			// profile. The drop-pressure controller remains the safety valve for a
			// path that cannot sustain this quality.
			profile.quality = min(profile.quality, 86)
			profile.chroma = desktopJPEGChroma422
		}
	}
	return profile
}

func desktopInteractionWidth(targetFPS, profileWidth int) int {
	return desktopInteractionWidthMode(targetFPS, profileWidth, false)
}

func desktopInteractionWidthMode(targetFPS, profileWidth int, preserveDetail bool) int {
	if targetFPS == 0 {
		targetFPS = 30
	}
	if targetFPS >= 60 {
		if preserveDetail {
			return min(profileWidth, 2560)
		}
		return min(profileWidth, 1920)
	}
	if targetFPS <= 15 {
		return min(profileWidth, 2560)
	}
	return min(profileWidth, 2560)
}

// desktopOutputGeometry is the single source of truth for the frame geometry
// advertised to the viewer and used for pointer coordinates. Keeping this
// calculation outside the capture backends prevents a quality transition from
// changing one dimension independently or from alternating between the native
// notebook surface and a differently-strided cursor surface.
func desktopOutputGeometry(baseWidth, baseHeight, targetFPS int, interactive, constrained bool) (int, int) {
	return desktopOutputGeometryMode(baseWidth, baseHeight, targetFPS, interactive, constrained, false)
}

func desktopOutputGeometryMode(baseWidth, baseHeight, targetFPS int, interactive, constrained, preserveDetail bool) (int, int) {
	if baseWidth <= 0 || baseHeight <= 0 {
		return 0, 0
	}
	width := baseWidth
	interactionWidth := desktopInteractionWidthMode(targetFPS, baseWidth, preserveDetail)
	if (interactive || constrained) && width > interactionWidth {
		width = interactionWidth
	}
	return width, max(1, baseHeight*width/baseWidth)
}

// The realtime scaler deliberately trades sampling quality for the 16 ms
// budget of 60 FPS. Auto/30 FPS has enough time for the bilinear scaler; using
// the realtime path there made text and thin UI lines visibly pixelated during
// every mouse movement even on a healthy connection.
func desktopUseRealtimeScaler(targetFPS int, interactive bool) bool {
	return desktopUseRealtimeScalerMode(targetFPS, interactive, false)
}

func desktopUseRealtimeScalerMode(targetFPS int, interactive, preserveDetail bool) bool {
	return interactive && targetFPS >= 60 && !preserveDetail
}
