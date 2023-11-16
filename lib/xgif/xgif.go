// xgif is a helper package to create GIF animations based on PNG images
// The resulting animations have the following properties:
// 1. All frames have the same size (max(pngs.width), max(pngs.height))
// 2. All PNGs are centered in the given frame
// 3. The frame background is plain white
// Note that to convert from a PNG to a GIF compatible image (Bitmap), the PNG image must be quantized (colors are aggregated in median buckets)
// so that it has at most 255 colors.
// This is required because GIFs support only 256 colors and we must keep 1 slot for the white background.
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
			return nil, fmt.Errorf("decoded gif image could not be cast as *image.Paletted")
		}

		// 2. make GIF frames of the same size, keeping images centered and with a white background
		bounds := pngImage.Bounds()
		top := (height - bounds.Dy()) / 2
		bottom := top + bounds.Dy()
		left := (width - bounds.Dx()) / 2
		right := left + bounds.Dx()

		var bgIndex int
		if len(palettedImg.Palette) == 256 {
			bgIndex = findWhiteIndex(palettedImg.Palette)
			palettedImg.Palette[bgIndex] = BG_COLOR
		} else {
			bgIndex = len(palettedImg.Palette)
			palettedImg.Palette = append(palettedImg.Palette, BG_COLOR)
		}
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palettedImg.Palette)
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				if x <= left || y <= top || x >= right || y >= bottom {
					frame.SetColorIndex(x, y, uint8(bgIndex))
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

func findWhiteIndex(palette color.Palette) int {
	nearestIndex := 0
	nearestScore := 0.
	for i, c := range palette {
		r, g, b, _ := c.RGBA()
		if r == 255 && g == 255 && b == 255 {
			return i
		}

		avg := float64(r+g+b) / 255.
		if avg > nearestScore {
			nearestScore = avg
			nearestIndex = i
		}
	}
	return nearestIndex
}

func Validate(gifBytes []byte, nFrames int, intervalMS int) error {
	anim, err := gif.DecodeAll(bytes.NewBuffer(gifBytes))
	if err != nil {
		return err
	}

	if nFrames > 1 && anim.LoopCount != INFINITE_LOOP {
		return fmt.Errorf("expected infinite loop, got=%d", anim.LoopCount)
	} else if nFrames == 1 && anim.LoopCount != -1 {
		return fmt.Errorf("wrong loop count for single frame gif, got=%d", anim.LoopCount)
	}

	if len(anim.Image) != nFrames {
		return fmt.Errorf("expected %d frames, got=%d", nFrames, len(anim.Image))
	}

	interval := intervalMS / 10
	width, height := anim.Config.Width, anim.Config.Height
	for i, frame := range anim.Image {
		w := frame.Bounds().Dx()
		if w != width {
			return fmt.Errorf("expected all frames to have the same width=%d, got=%d at frame=%d", width, w, i)
		}
		h := frame.Bounds().Dy()
		if h != height {
			return fmt.Errorf("expected all frames to have the same height=%d, got=%d at frame=%d", height, h, i)
		}
		if anim.Delay[i] != interval {
			return fmt.Errorf("expected interval between frames to be %d, got=%d at frame=%d", interval, anim.Delay[i], i)
		}
	}

	return nil
}
