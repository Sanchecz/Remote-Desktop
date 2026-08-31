//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/png"
	"sync"
	"time"
	"unsafe"
)

var windowsClipboardImageState struct {
	sync.Mutex
	digest [sha256.Size]byte
	valid  bool
}

func windowsClipboardPNGChanged(imagePNG []byte) bool {
	digest := sha256.Sum256(imagePNG)
	windowsClipboardImageState.Lock()
	defer windowsClipboardImageState.Unlock()
	return !windowsClipboardImageState.valid || windowsClipboardImageState.digest != digest
}

func rememberWindowsClipboardPNG(imagePNG []byte) {
	digest := sha256.Sum256(imagePNG)
	windowsClipboardImageState.Lock()
	windowsClipboardImageState.digest = digest
	windowsClipboardImageState.valid = true
	windowsClipboardImageState.Unlock()
}

func openWindowsClipboard() error {
	for attempt := 0; attempt < 6; attempt++ {
		if opened, _, _ := procOpenClipboard.Call(0); opened != 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("буфер Windows временно занят другим приложением")
}

func readWindowsClipboardPNG() ([]byte, bool, error) {
	const (
		clipboardDIB   = uintptr(8)
		clipboardDIBV5 = uintptr(17)
	)
	format := uintptr(0)
	if available, _, _ := procIsClipboardFormat.Call(clipboardDIBV5); available != 0 {
		format = clipboardDIBV5
	} else if available, _, _ := procIsClipboardFormat.Call(clipboardDIB); available != 0 {
		format = clipboardDIB
	} else {
		return nil, false, nil
	}
	if err := openWindowsClipboard(); err != nil {
		return nil, true, err
	}
	defer procCloseClipboard.Call()
	handle, _, _ := procGetClipboardData.Call(format)
	if handle == 0 {
		return nil, true, errors.New("Windows не вернула изображение из буфера")
	}
	size, _, _ := procGlobalSize.Call(handle)
	if size == 0 || size > desktopMaximumClipboardImageBytes*8 {
		return nil, true, errors.New("изображение в буфере пустое или слишком большое")
	}
	pointer, _, _ := procGlobalLock.Call(handle)
	if pointer == 0 {
		return nil, true, errors.New("не удалось прочитать память изображения буфера")
	}
	data := make([]byte, int(size))
	procMoveMemory.Call(uintptr(unsafe.Pointer(&data[0])), pointer, size)
	procGlobalUnlock.Call(handle)
	image, err := decodeClipboardDIB(data)
	if err != nil {
		return nil, true, err
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image); err != nil {
		return nil, true, err
	}
	if encoded.Len() > desktopMaximumClipboardImageBytes {
		return nil, true, errors.New("PNG удалённого буфера больше 12 МБ")
	}
	return encoded.Bytes(), true, nil
}

func writeWindowsClipboardPNG(imagePNG []byte) error {
	decoded, err := png.Decode(bytes.NewReader(imagePNG))
	if err != nil {
		return errors.New("получено некорректное изображение PNG")
	}
	dib, err := encodeClipboardDIB(decoded)
	if err != nil {
		return err
	}
	const globalMoveable = uintptr(0x0002)
	handle, _, _ := procGlobalAlloc.Call(globalMoveable, uintptr(len(dib)))
	if handle == 0 {
		return errors.New("Windows не выделила память для изображения буфера")
	}
	ownedByClipboard := false
	defer func() {
		if !ownedByClipboard {
			procGlobalFree.Call(handle)
		}
	}()
	pointer, _, _ := procGlobalLock.Call(handle)
	if pointer == 0 {
		return errors.New("не удалось открыть память изображения буфера")
	}
	procMoveMemory.Call(pointer, uintptr(unsafe.Pointer(&dib[0])), uintptr(len(dib)))
	procGlobalUnlock.Call(handle)
	if err := openWindowsClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	if emptied, _, _ := procEmptyClipboard.Call(); emptied == 0 {
		return errors.New("Windows не очистила буфер перед записью")
	}
	const clipboardDIB = uintptr(8)
	if stored, _, _ := procSetClipboardData.Call(clipboardDIB, handle); stored == 0 {
		return fmt.Errorf("Windows не приняла изображение в буфер")
	}
	ownedByClipboard = true
	rememberWindowsClipboardPNG(imagePNG)
	return nil
}
