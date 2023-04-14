package xgif

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"

	"github.com/ericpauley/go-quantize/quantize"
	"oss.terrastruct.com/util-go/go2"
)

const INFINITE_LOOP = 0
const BG_INDEX uint8 = 255

var BG_COLOR = color.White

func AnimatePNGs(pngs [][]byte, animIntervalMs int) ([]byte, error) {
	var width, height int
	pngImgs := make([]image.Image, len(pngs))
	for i, pngBytes := range pngs {
		img, err := png.Decode(bytes.NewBuffer(pngBytes))
		if err != nil {
			return nil, err
		}
		pngImgs[i] = img
		bounds := img.Bounds()
		width = go2.Max(width, bounds.Dx())
		height = go2.Max(height, bounds.Dy())
	}

	interval := animIntervalMs / 10 // gif animation interval is in 100ths of a second
	anim := &gif.GIF{
		LoopCount: INFINITE_LOOP,
		Config: image.Config{
			Width:  width,
			Height: height,
		},
	}

	for _, pngImage := range pngImgs {
		// 1. convert the PNG into a GIF compatible image (Bitmap) by quantizing it to 255 colors
		buf := bytes.NewBuffer(nil)
		err := gif.Encode(buf, pngImage, &gif.Options{
			NumColors: 255, // GIFs can have up to 256 colors, so keep 1 slot for white background
			Quantizer: quantize.MedianCutQuantizer{},
		})
		if err != nil {
			return nil, err
		}
		gifImg, err := gif.Decode(buf)
		if err != nil {
			return nil, err
		}
		palettedImg, ok := gifImg.(*image.Paletted)
		if !ok {
			return nil, fmt.Errorf("decoded git image could not be cast as *image.Paletted")
		}

		// 2. make GIF frames of the same size, keeping images centered and with a white background
		bounds := pngImage.Bounds()
		top := (height - bounds.Dy()) / 2
		bottom := top + bounds.Dy()
		left := (width - bounds.Dx()) / 2
		right := left + bounds.Dx()

		palettedImg.Palette[BG_INDEX] = BG_COLOR
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palettedImg.Palette)
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				if x <= left || y <= top || x >= right || y >= bottom {
					frame.SetColorIndex(x, y, BG_INDEX)
				} else {
					frame.SetColorIndex(x, y, palettedImg.ColorIndexAt(x-left, y-top))
				}
			}
		}

		anim.Image = append(anim.Image, frame)
		anim.Delay = append(anim.Delay, interval)
	}

	buf := bytes.NewBuffer(nil)
	err := gif.EncodeAll(buf, anim)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
