package main

import (
	"image"
	"image/color"
	"testing"
)

func TestClipboardDIBRoundTripPreservesGeometryAndColor(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	source.SetNRGBA(0, 0, color.NRGBA{R: 240, G: 10, B: 20, A: 255})
	source.SetNRGBA(2, 1, color.NRGBA{R: 30, G: 220, B: 40, A: 255})
	dib, err := encodeClipboardDIB(source)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeClipboardDIB(dib)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != source.Bounds() {
		t.Fatalf("bounds = %v, want %v", decoded.Bounds(), source.Bounds())
	}
	if got := decoded.NRGBAAt(0, 0); got.R != 240 || got.G != 10 || got.B != 20 {
		t.Fatalf("top pixel = %#v", got)
	}
	if got := decoded.NRGBAAt(2, 1); got.R != 30 || got.G != 220 || got.B != 40 {
		t.Fatalf("bottom pixel = %#v", got)
	}
}

func TestClipboardDIBRejectsTruncatedPixels(t *testing.T) {
	if _, err := decodeClipboardDIB(make([]byte, 40)); err == nil {
		t.Fatal("expected truncated DIB error")
	}
}
