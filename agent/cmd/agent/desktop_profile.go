package main

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

func desktopProfileForFPS(targetFPS int) desktopCaptureProfile {
	switch {
	case targetFPS <= 15:
		// The low-cadence mode is also the sharpest mode. It is intended for
		// administration where small text matters more than motion, so preserve up
		// through a native 4K desktop and spend the saved frame budget on JPEG
		// quality instead of sending a small image that the viewer has to upscale.
		return desktopCaptureProfile{maxWidth: 3840, quality: 94, chroma: desktopJPEGChroma444}
	case targetFPS >= 60:
		// The smooth profile keeps enough resolution for readable text while
		// bounding bandwidth. Full chroma avoids the coloured halos that 4:2:0
		// creates around Windows fonts and icons.
		return desktopCaptureProfile{maxWidth: 2560, quality: 88, chroma: desktopJPEGChroma444}
	default:
		// Auto and explicit 30 FPS are the normal high-quality profile. Preserve a
		// full 4K administration desktop and use a visually lossless JPEG
		// level: small Windows text must stay readable without switching to the
		// slower 15 FPS profile.
		return desktopCaptureProfile{maxWidth: 3840, quality: 92, chroma: desktopJPEGChroma444}
	}
}
