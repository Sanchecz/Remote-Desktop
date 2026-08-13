//go:build windows && cgo

package main

/*
#cgo windows LDFLAGS: -lturbojpeg
#include <turbojpeg.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// desktopJPEGEncoder owns one TurboJPEG compressor for the lifetime of the
// capture surface. Reusing it avoids allocator and codec initialization cost
// on every frame.
type desktopJPEGEncoder struct {
	handle   C.tjhandle
	buffer   *C.uchar
	capacity C.ulong
}

func (encoder *desktopJPEGEncoder) Close() {
	if encoder.buffer != nil {
		C.tjFree(encoder.buffer)
		encoder.buffer = nil
		encoder.capacity = 0
	}
	if encoder.handle != nil {
		C.tjDestroy(encoder.handle)
		encoder.handle = nil
	}
}

func (encoder *desktopJPEGEncoder) EncodeBGRA(pixels []byte, width, height, quality int, fullChroma bool) ([]byte, error) {
	if width <= 0 || height <= 0 || len(pixels) < width*height*4 {
		return nil, errors.New("invalid desktop frame buffer")
	}
	if encoder.handle == nil {
		encoder.handle = C.tjInitCompress()
		if encoder.handle == nil {
			return nil, errors.New("libjpeg-turbo compressor initialization failed")
		}
	}
	subsampling := C.int(C.TJSAMP_420)
	if fullChroma {
		// Desktop text and thin UI glyphs lose a surprising amount of detail with
		// 4:2:0 chroma subsampling. The 15/30 FPS profiles use 4:4:4 so coloured
		// text stays crisp; the 60 FPS profile also keeps 4:4:4 while controlling
		// motion bandwidth through its 1920 px output profile.
		subsampling = C.int(C.TJSAMP_444)
	}
	required := C.tjBufSize(C.int(width), C.int(height), subsampling)
	if required == 0 {
		return nil, errors.New("libjpeg-turbo returned an invalid buffer size")
	}
	if encoder.buffer == nil || encoder.capacity < required {
		if encoder.buffer != nil {
			C.tjFree(encoder.buffer)
		}
		encoder.buffer = C.tjAlloc(C.int(required))
		encoder.capacity = required
		if encoder.buffer == nil {
			encoder.capacity = 0
			return nil, errors.New("libjpeg-turbo frame buffer allocation failed")
		}
	}
	destination := encoder.buffer
	destinationSize := encoder.capacity
	status := C.tjCompress2(
		encoder.handle,
		(*C.uchar)(unsafe.Pointer(&pixels[0])),
		C.int(width),
		C.int(width*4),
		C.int(height),
		C.TJPF_BGRA,
		&destination,
		&destinationSize,
		subsampling,
		C.int(quality),
		C.TJFLAG_FASTDCT|C.TJFLAG_NOREALLOC,
	)
	if status != 0 {
		message := C.GoString(C.tjGetErrorStr2(encoder.handle))
		if message == "" {
			message = "unknown error"
		}
		return nil, fmt.Errorf("libjpeg-turbo: %s", message)
	}
	if destination == nil || destination != encoder.buffer || destinationSize == 0 || uint64(destinationSize) > desktopMaximumFrameBytes {
		return nil, errors.New("libjpeg-turbo returned an invalid frame")
	}
	return C.GoBytes(unsafe.Pointer(destination), C.int(destinationSize)), nil
}
