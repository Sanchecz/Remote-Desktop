//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	stdDraw "image/draw"
	"image/png"
	"math"

	"github.com/lxn/walk"
)

//go:embed assets/genesisit-online.png
var onlineIconPNG []byte

//go:embed assets/genesisit-offline.png
var offlineIconPNG []byte

//go:embed assets/remoteit-device-monitor.png
var deviceMonitorPNG []byte

//go:embed assets/remoteit-agent-icons.png
var agentIconAtlasPNG []byte

var agentIconKinds = []string{
	"monitor", "panel", "log", "folder", "settings", "info", "bolt", "pencil",
	"link", "copy", "pulse", "service", "shield", "clock", "list", "arrow",
	"check", "circle-check", "dot", "update", "cpu",
}

var agentIconColors = []string{"green", "ink", "muted", "white", "orange", "red"}

type agentIconSet struct {
	bitmaps map[string]*walk.Bitmap
	images  map[string]image.Image
}

func loadStatusIcon(data []byte) (*walk.Icon, error) {
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return walk.NewIconFromImage(image)
}

func loadEmbeddedBitmap(data []byte) (*walk.Bitmap, error) {
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return walk.NewBitmapFromImageForDPI(image, 96)
}

func loadAgentIconSet() (*agentIconSet, error) {
	const cell = 256
	atlas, err := png.Decode(bytes.NewReader(agentIconAtlasPNG))
	if err != nil {
		return nil, err
	}
	wantWidth := cell * len(agentIconKinds)
	wantHeight := cell * len(agentIconColors)
	if atlas.Bounds().Dx() != wantWidth || atlas.Bounds().Dy() != wantHeight {
		return nil, fmt.Errorf("unexpected Agent icon atlas size: got %dx%d, want %dx%d", atlas.Bounds().Dx(), atlas.Bounds().Dy(), wantWidth, wantHeight)
	}
	set := &agentIconSet{
		bitmaps: make(map[string]*walk.Bitmap, len(agentIconKinds)*len(agentIconColors)*4),
		images:  make(map[string]image.Image, len(agentIconKinds)*len(agentIconColors)),
	}
	for colorIndex, color := range agentIconColors {
		for kindIndex, kind := range agentIconKinds {
			source := image.Rect(kindIndex*cell, colorIndex*cell, (kindIndex+1)*cell, (colorIndex+1)*cell)
			icon := image.NewRGBA(image.Rect(0, 0, cell, cell))
			stdDraw.Draw(icon, icon.Bounds(), atlas, source.Min, stdDraw.Src)
			set.images[color+":"+kind] = icon
		}
	}
	return set, nil
}

func (set *agentIconSet) Bitmap(kind, color string) *walk.Bitmap {
	return set.BitmapSized(kind, color, 256, 256)
}

// BitmapSized pre-rasterizes the vector atlas into the exact physical target
// size using an area filter. GDI's default StretchBlt path aliases thin icon
// strokes when a 256 px tile is drawn directly at 15-34 px, especially at
// 125/150/200% DPI. Exact-size cached bitmaps keep edges, circles and rounded
// joins visibly smooth and consistent throughout the native Agent window.
func (set *agentIconSet) BitmapSized(kind, colorName string, width, height int) *walk.Bitmap {
	if set == nil {
		return nil
	}
	if width < 1 || height < 1 {
		return nil
	}
	baseKey := colorName + ":" + kind
	cacheKey := fmt.Sprintf("%s:%dx%d", baseKey, width, height)
	if bitmap := set.bitmaps[cacheKey]; bitmap != nil {
		return bitmap
	}
	source := set.images[baseKey]
	if source == nil {
		return nil
	}
	resized := resizeImageArea(source, width, height)
	bitmap, err := walk.NewBitmapFromImageForDPI(resized, 96)
	if err != nil {
		return nil
	}
	set.bitmaps[cacheKey] = bitmap
	return bitmap
}

func resizeImageArea(source image.Image, width, height int) *image.NRGBA {
	target := image.NewNRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	scaleX := float64(bounds.Dx()) / float64(width)
	scaleY := float64(bounds.Dy()) / float64(height)
	for y := 0; y < height; y++ {
		y0 := float64(y)*scaleY + float64(bounds.Min.Y)
		y1 := float64(y+1)*scaleY + float64(bounds.Min.Y)
		for x := 0; x < width; x++ {
			x0 := float64(x)*scaleX + float64(bounds.Min.X)
			x1 := float64(x+1)*scaleX + float64(bounds.Min.X)
			var red, green, blue, alpha, weight float64
			for sourceY := int(math.Floor(y0)); sourceY < int(math.Ceil(y1)); sourceY++ {
				yWeight := math.Min(y1, float64(sourceY+1)) - math.Max(y0, float64(sourceY))
				if yWeight <= 0 || sourceY < bounds.Min.Y || sourceY >= bounds.Max.Y {
					continue
				}
				for sourceX := int(math.Floor(x0)); sourceX < int(math.Ceil(x1)); sourceX++ {
					xWeight := math.Min(x1, float64(sourceX+1)) - math.Max(x0, float64(sourceX))
					if xWeight <= 0 || sourceX < bounds.Min.X || sourceX >= bounds.Max.X {
						continue
					}
					coverage := xWeight * yWeight
					pixel := color.NRGBAModel.Convert(source.At(sourceX, sourceY)).(color.NRGBA)
					pixelAlpha := float64(pixel.A) / 255
					red += float64(pixel.R) * pixelAlpha * coverage
					green += float64(pixel.G) * pixelAlpha * coverage
					blue += float64(pixel.B) * pixelAlpha * coverage
					alpha += float64(pixel.A) * coverage
					weight += coverage
				}
			}
			if weight == 0 || alpha == 0 {
				continue
			}
			resultAlpha := alpha / weight
			unpremultiply := 255 / resultAlpha
			target.SetNRGBA(x, y, color.NRGBA{
				R: uint8(math.Min(255, red/weight*unpremultiply)),
				G: uint8(math.Min(255, green/weight*unpremultiply)),
				B: uint8(math.Min(255, blue/weight*unpremultiply)),
				A: uint8(math.Min(255, resultAlpha)),
			})
		}
	}
	return target
}

func (set *agentIconSet) Dispose() {
	if set == nil {
		return
	}
	for key, bitmap := range set.bitmaps {
		if bitmap != nil {
			bitmap.Dispose()
		}
		delete(set.bitmaps, key)
	}
	clear(set.images)
}

// showRemoteItNotification keeps every Windows notification aligned with the
// live Agent state. Windows takes the small app glyph from the current notify
// icon and the large glyph from ShowCustom, so both must be updated together.
// Informational/success notifications always use the green RemoteIt favicon;
// the red variant is reserved for a confirmed connection failure.
func showRemoteItNotification(tray *walk.NotifyIcon, window *walk.MainWindow, title, message string, healthy bool, onlineIcon, offlineIcon *walk.Icon) {
	icon := onlineIcon
	if !healthy {
		icon = offlineIcon
	}
	if tray == nil || icon == nil {
		return
	}
	_ = tray.SetIcon(icon)
	if window != nil {
		_ = window.SetIcon(icon)
	}
	_ = tray.ShowCustom(title, message, icon)
}
