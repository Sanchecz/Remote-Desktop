package main

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
)

const desktopMaximumClipboardImageBytes = 12 << 20

func encodeClipboardDIB(source image.Image) ([]byte, error) {
	if source == nil {
		return nil, errors.New("clipboard image is empty")
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 1 || height < 1 || width > 12000 || height > 12000 || int64(width)*int64(height) > 32_000_000 {
		return nil, errors.New("clipboard image dimensions are unsupported")
	}
	stride := width * 4
	data := make([]byte, 40+stride*height)
	binary.LittleEndian.PutUint32(data[0:4], 40)
	binary.LittleEndian.PutUint32(data[4:8], uint32(width))
	binary.LittleEndian.PutUint32(data[8:12], uint32(height)) // positive: bottom-up DIB
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], 32)
	binary.LittleEndian.PutUint32(data[20:24], uint32(stride*height))
	for y := 0; y < height; y++ {
		target := 40 + (height-1-y)*stride
		for x := 0; x < width; x++ {
			pixel := color.NRGBAModel.Convert(source.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			offset := target + x*4
			data[offset+0] = pixel.B
			data[offset+1] = pixel.G
			data[offset+2] = pixel.R
			data[offset+3] = pixel.A
		}
	}
	return data, nil
}

func decodeClipboardDIB(data []byte) (*image.NRGBA, error) {
	if len(data) < 40 {
		return nil, errors.New("clipboard DIB header is truncated")
	}
	headerSize := int(binary.LittleEndian.Uint32(data[0:4]))
	width := int(int32(binary.LittleEndian.Uint32(data[4:8])))
	rawHeight := int32(binary.LittleEndian.Uint32(data[8:12]))
	planes := binary.LittleEndian.Uint16(data[12:14])
	bits := int(binary.LittleEndian.Uint16(data[14:16]))
	compression := binary.LittleEndian.Uint32(data[16:20])
	if headerSize < 40 || headerSize > len(data) || width < 1 || rawHeight == 0 || planes != 1 || (bits != 24 && bits != 32) || (compression != 0 && !(compression == 3 && bits == 32)) {
		return nil, errors.New("clipboard DIB format is unsupported")
	}
	height := int(rawHeight)
	topDown := height < 0
	if topDown {
		height = -height
	}
	if width > 12000 || height > 12000 || int64(width)*int64(height) > 32_000_000 {
		return nil, errors.New("clipboard DIB dimensions are unsupported")
	}
	pixelOffset := headerSize
	if compression == 3 && headerSize == 40 {
		pixelOffset += 12 // RGB masks follow BITMAPINFOHEADER for BI_BITFIELDS.
	}
	stride := ((width*bits + 31) / 32) * 4
	if stride < 1 || pixelOffset+stride*height > len(data) {
		return nil, errors.New("clipboard DIB pixels are truncated")
	}
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	bytesPerPixel := bits / 8
	for y := 0; y < height; y++ {
		sourceY := y
		if !topDown {
			sourceY = height - 1 - y
		}
		row := pixelOffset + sourceY*stride
		for x := 0; x < width; x++ {
			offset := row + x*bytesPerPixel
			alpha := byte(255)
			if bits == 32 && data[offset+3] != 0 {
				alpha = data[offset+3]
			}
			result.SetNRGBA(x, y, color.NRGBA{R: data[offset+2], G: data[offset+1], B: data[offset], A: alpha})
		}
	}
	return result, nil
}
