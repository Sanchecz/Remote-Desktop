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
		// Motion frames only live for a few dozen milliseconds. Keeping the same
		// desktop geometry makes input mapping stable, while a balanced 4:2:0 JPEG
		// stays readable and leaves enough wire budget for a real 30 FPS stream on
		// ordinary WAN links. The sharp 4:4:4 frame is restored immediately after
		// the interaction window expires.
		// The mode-specific surface and quality below keep motion inside its wire
		// budget. The following native 4:4:4 refresh restores fine coloured text as
		// soon as the user pauses, without leaving the stream in a pixelated state.
		if targetFPS >= 60 {
			// Six latest-only viewer lanes and a dedicated input socket leave enough
			// headroom for a 1600px motion surface without allowing stale frames to
			// queue. This is visibly sharper than the former 1280px/quality-82 mode
			// on 2K/4K monitors, while the native 4:4:4 refresh still follows as soon
			// as input stops.
			profile.quality = min(profile.quality, 86)
			profile.fullChroma = false
		} else if targetFPS <= 15 {
			// 15 FPS is the high-detail motion option. Chroma subsampling is still
			// required on a busy desktop to leave input packets headroom, but the
			// 1920px surface and quality 88 preserve substantially more detail than
			// the normal/smooth profiles.
			profile.quality = min(profile.quality, 88)
			profile.fullChroma = false
		} else {
			// Normal administration favours text clarity. A 1920px quality-88 frame
			// keeps small Windows labels readable during motion; adaptive Auto can
			// still fall back to 15 FPS on a persistently congested link.
			profile.quality = min(profile.quality, 88)
			profile.fullChroma = false
		}
	}
	return profile
}

func desktopInteractionWidth(targetFPS, profileWidth int) int {
	if targetFPS == 0 {
		targetFPS = 30
	}
	if targetFPS >= 60 {
		return min(profileWidth, 1600)
	}
	if targetFPS <= 15 {
		return min(profileWidth, 1920)
	}
	return min(profileWidth, 1920)
}
