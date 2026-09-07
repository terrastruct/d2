package d2isometricimg

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"math"
	"time"

	"github.com/ericpauley/go-quantize/quantize"
)

// A single palette is sampled across four traffic phases. Quantization then
// uses an immutable RGB lookup, without error diffusion: changing one particle
// cannot change the indexed color of otherwise identical neighboring pixels.
func renderGIF(s *capture) ([]byte, error) {
	frames, cycle := nativeGIFCycle(s.scene)
	if frames != FrameCount {
		scene, err := nativeGIFLoopScene(s.ctx, s.scene, time.Duration(cycle*float64(time.Second)))
		if err != nil {
			return nil, err
		}
		owned := *s
		owned.scene = scene
		s = &owned
	}
	frameAt := func(seconds float64) (*image.RGBA, error) {
		return s.frameImageAt(seconds, seconds*CycleSeconds/cycle, true)
	}
	samples := image.NewRGBA(image.Rect(0, 0, 256, 1024))
	for i := 0; i < 4; i++ {
		seconds := float64(i) * cycle / 4
		if frames != FrameCount {
			// Whole-second boundaries would sample the same authored pulse.
			seconds += float64(i) / 4
		}
		frame, err := frameAt(seconds)
		if err != nil {
			return nil, err
		}
		for y := 0; y < 256; y++ {
			for x := 0; x < 256; x++ {
				samples.Set(x, i*256+y, frame.At(x*s.opts.Width/256, y*s.opts.Height/256))
			}
		}
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	palette := quantize.MedianCutQuantizer{}.Quantize(make(color.Palette, 0, 256), samples)
	lookup, err := paletteLookup(s.ctx, palette)
	if err != nil {
		return nil, err
	}
	animation := &gif.GIF{LoopCount: 0, Config: image.Config{ColorModel: palette, Width: s.opts.Width, Height: s.opts.Height}}
	var previous *image.Paletted
	for i := 0; i < frames; i++ {
		// Do not repeat phase 1: the next loop's phase 0 is its next sample.
		frame, err := frameAt(float64(i) * cycle / float64(frames))
		if err != nil {
			return nil, err
		}
		indexed, err := indexFrame(s.ctx, frame, palette, lookup)
		if err != nil {
			return nil, err
		}
		// Delta rectangles retain the previous opaque canvas, saving bytes and
		// memory while still erasing each particle's old position correctly.
		cropped := indexed
		if previous != nil {
			cropped = frameDelta(previous, indexed)
		}
		animation.Image = append(animation.Image, cropped)
		animation.Delay = append(animation.Delay, cycleFrameDelay(i, frames, int(math.Round(cycle*100))))
		animation.Disposal = append(animation.Disposal, gif.DisposalNone)
		previous = indexed
	}
	var output bytes.Buffer
	w := &boundedWriter{ctx: s.ctx, w: &output, remaining: MaxOutputBytes}
	if err := gif.EncodeAll(w, animation); err != nil {
		return nil, err
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func nativeGIFCycle(scene *nativeScene) (frames int, seconds float64) {
	if len(scene.animatedNodes) > 0 {
		return 96, 8
	}
	for _, panel := range scene.panels {
		if panel.animated {
			return 96, 8
		}
	}
	return FrameCount, CycleSeconds
}

func frameDelay(i int) int {
	// GIF measures time in centiseconds. Round cumulative boundaries so the
	// complete 8.333...s cycle is represented by 833cs, not 800 or 900cs.
	return cycleFrameDelay(i, FrameCount, 833)
}

func cycleFrameDelay(i, frames, centiseconds int) int {
	return int(math.Round(float64(i+1)*float64(centiseconds)/float64(frames)) - math.Round(float64(i)*float64(centiseconds)/float64(frames)))
}

func paletteLookup(ctx context.Context, p color.Palette) ([]uint8, error) {
	lookup := make([]uint8, 1<<18)
	for r := 0; r < 64; r++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for g := 0; g < 64; g++ {
			for b := 0; b < 64; b++ {
				lookup[r<<12|g<<6|b] = uint8(p.Index(color.RGBA{uint8(r * 255 / 63), uint8(g * 255 / 63), uint8(b * 255 / 63), 255}))
			}
		}
	}
	return lookup, nil
}

func indexFrame(ctx context.Context, frame image.Image, p color.Palette, lookup []uint8) (*image.Paletted, error) {
	b := frame.Bounds()
	out := image.NewPaletted(b, p)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if y%32 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, _ := frame.At(x, y).RGBA()
			out.SetColorIndex(x, y, lookup[int(r>>10)<<12|int(g>>10)<<6|int(b>>10)])
		}
	}
	return out, nil
}

func frameDelta(before, after *image.Paletted) *image.Paletted {
	b := after.Bounds()
	x0, y0, x1, y1 := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if before.ColorIndexAt(x, y) != after.ColorIndexAt(x, y) {
				x0 = min(x0, x)
				y0 = min(y0, y)
				x1 = max(x1, x+1)
				y1 = max(y1, y+1)
			}
		}
	}
	if x0 >= x1 {
		x0, y0, x1, y1 = 0, 0, 1, 1
	}
	r := image.Rect(x0, y0, x1, y1)
	out := image.NewPaletted(r, after.Palette)
	for y := y0; y < y1; y++ {
		copy(out.Pix[(y-y0)*out.Stride:], after.Pix[after.PixOffset(x0, y):after.PixOffset(x1, y)])
	}
	return out
}

type boundedWriter struct {
	ctx       context.Context
	w         io.Writer
	remaining int64
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(p)) > w.remaining {
		return 0, fmt.Errorf("isometric image exceeds encoded byte limit")
	}
	n, err := w.w.Write(p)
	w.remaining -= int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}
