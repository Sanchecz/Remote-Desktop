package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadDesktopClipboardPNGValidatesRealPNG(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	source.SetNRGBA(1, 1, color.NRGBA{R: 20, G: 180, B: 90, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/clipboard-image", bytes.NewReader(encoded.Bytes()))
	request.Header.Set("Content-Type", "image/png")
	recorder := httptest.NewRecorder()
	data, ok := readDesktopClipboardPNG(recorder, request)
	if !ok || !bytes.Equal(data, encoded.Bytes()) {
		t.Fatalf("valid PNG rejected: status=%d bytes=%d", recorder.Code, len(data))
	}
}

func TestReadDesktopClipboardPNGRejectsWrongMediaAndPayload(t *testing.T) {
	for _, candidate := range []struct {
		contentType string
		body        string
	}{
		{contentType: "text/plain", body: "not-png"},
		{contentType: "image/png", body: "not-png"},
	} {
		request := httptest.NewRequest("POST", "/clipboard-image", strings.NewReader(candidate.body))
		request.Header.Set("Content-Type", candidate.contentType)
		recorder := httptest.NewRecorder()
		if _, ok := readDesktopClipboardPNG(recorder, request); ok {
			t.Fatalf("invalid payload accepted: %#v", candidate)
		}
		if recorder.Code < 400 {
			t.Fatalf("invalid payload status = %d", recorder.Code)
		}
	}
}
