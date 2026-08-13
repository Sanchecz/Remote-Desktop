//go:build windows && !cgo

package main

type desktopFastCapturer struct{}

func (capturer *desktopFastCapturer) Close() {}

func (capturer *desktopFastCapturer) CaptureBGRA(_ []byte, _, _ int) int { return -1 }

func (capturer *desktopFastCapturer) BackendDetail() string { return "gdi-dxgi-unavailable" }

func scaleDesktopBGRAFast(source []byte, sourceWidth, sourceHeight int, target []byte, targetWidth, targetHeight int, scaleX, scaleWeight []int32) bool {
	scaleDesktopBGRA(source, sourceWidth, sourceHeight, target, targetWidth, targetHeight, scaleX, scaleWeight)
	return true
}

func scaleDesktopBGRARealtime(source []byte, sourceWidth, sourceHeight int, target []byte, targetWidth, targetHeight int, scaleX, scaleWeight []int32) bool {
	// Non-CGO developer builds keep the reference scaler. Production Windows
	// artifacts use the optimized C implementation above.
	return scaleDesktopBGRAFast(source, sourceWidth, sourceHeight, target, targetWidth, targetHeight, scaleX, scaleWeight)
}
