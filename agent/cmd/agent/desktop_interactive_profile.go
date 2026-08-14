package main

// desktopProfileForInteraction preserves the selected output dimensions but
// uses a transport-friendly JPEG while input is actively changing the screen.
// A normal profile replaces it after the short interaction window, so small
// text remains sharp at rest without forcing every transient motion frame to
// carry full 4:4:4 chroma.
func desktopProfileForInteraction(targetFPS int, interactive bool) desktopCaptureProfile {
	// Auto is a 30 FPS mode until the adaptive controller has enough evidence
	// to select 15 or 60. Treating its API value (0) as "below 15" produced a
	// needless 4:4:4 bandwidth spike during the first interactive frames.
	if targetFPS == 0 {
		targetFPS = 30
	}
	profile := desktopProfileForFPS(targetFPS)
	if interactive {
		// Motion frames only live for a few dozen milliseconds. The selected
		// surfaces below spend approximately the same wire budget in every mode:
		// 15 FPS can preserve a full 4K frame, 30 FPS preserves a 2K-class frame,
		// and 60 FPS preserves Full HD. This removes the former 1600px quality cliff
		// while still leaving room for input packets on a 100 Mbit/s WAN link. The
		// sharp 4:4:4 rest frame is restored immediately after input stops.
		if targetFPS >= 60 {
			profile.quality = min(profile.quality, 88)
			profile.chroma = desktopJPEGChroma420
		} else if targetFPS <= 15 {
			profile.quality = min(profile.quality, 92)
			profile.chroma = desktopJPEGChroma422
		} else {
			profile.quality = min(profile.quality, 90)
			profile.chroma = desktopJPEGChroma422
		}
	}
	return profile
}

func desktopInteractionWidth(targetFPS, profileWidth int) int {
	if targetFPS == 0 {
		targetFPS = 30
	}
	if targetFPS >= 60 {
		return min(profileWidth, 1920)
	}
	if targetFPS <= 15 {
		return min(profileWidth, 2560)
	}
	return min(profileWidth, 1920)
}
