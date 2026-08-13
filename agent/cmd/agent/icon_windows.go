//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"image/png"

	"github.com/lxn/walk"
)

//go:embed assets/genesisit-online.png
var onlineIconPNG []byte

//go:embed assets/genesisit-offline.png
var offlineIconPNG []byte

func loadStatusIcon(data []byte) (*walk.Icon, error) {
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return walk.NewIconFromImage(image)
}
