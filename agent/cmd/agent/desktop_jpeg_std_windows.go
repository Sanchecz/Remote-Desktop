//go:build windows && !cgo

package main

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
)

// The non-CGO implementation keeps developer cross-builds working. Production
// Windows artifacts are built with libjpeg-turbo in the Docker release stage.
type desktopJPEGEncoder struct{}

func (*desktopJPEGEncoder) Close() {}

func (*desktopJPEGEncoder) EncodeBGRA(pixels []byte, width, height, quality int, _ desktopJPEGChroma) ([]byte, error) {
	if width <= 0 || height <= 0 || len(pixels) < width*height*4 {
		return nil, errors.New("invalid desktop frame buffer")
	}
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for index := 0; index < width*height*4; index += 4 {
		frame.Pix[index] = pixels[index+2]
		frame.Pix[index+1] = pixels[index+1]
		frame.Pix[index+2] = pixels[index]
		frame.Pix[index+3] = 255
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, frame, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}
