package xgif

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/png"

	"github.com/ericpauley/go-quantize/quantize"
)

const INFINITE_LOOP = 0

func AnimatePNGs(pngs [][]byte, gifWidth, gifHeight int, animIntervalMs int) ([]byte, error) {
	interval := animIntervalMs / 10 // gif animation interval is in 100ths of a second
	anim := &gif.GIF{
		LoopCount: INFINITE_LOOP,
		Config: image.Config{
			Width:  gifWidth,
			Height: gifHeight,
		},
	}

	for _, pngBytes := range pngs {
		pngImage, err := png.Decode(bytes.NewBuffer(pngBytes))
		if err != nil {
			return nil, err
		}
		buf := bytes.NewBuffer(nil)
		err = gif.Encode(buf, pngImage, &gif.Options{
			NumColors: 256, // GIFs can have up to 256 colors
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
		anim.Image = append(anim.Image, palettedImg)
		anim.Delay = append(anim.Delay, interval)
	}

	buf := bytes.NewBuffer(nil)
	err := gif.EncodeAll(buf, anim)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
