package xgif

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"reflect"
	"testing"
	"time"
)

var normalizeBenchmarkSink *image.Paletted

func TestNormalizePalettedImageMatchesScalarCanvas(t *testing.T) {
	for _, test := range []struct {
		name                              string
		bounds                            image.Rectangle
		stride, canvasWidth, canvasHeight int
		palette                           color.Palette
	}{
		{
			name:         "small_nonzero_background",
			bounds:       image.Rect(3, -5, 10, 4),
			stride:       11,
			canvasWidth:  12,
			canvasHeight: 14,
			palette:      color.Palette{color.Black, color.RGBA{R: 127, G: 31, B: 223, A: 255}},
		},
		{
			name:         "large_nonzero_background",
			bounds:       image.Rect(-17, 29, 206, 300),
			stride:       257,
			canvasWidth:  513,
			canvasHeight: 519,
			palette:      color.Palette{color.Black, color.RGBA{R: 127, G: 31, B: 223, A: 255}},
		},
		{
			name:         "zero_background",
			bounds:       image.Rect(101, -73, 364, 190),
			stride:       271,
			canvasWidth:  512,
			canvasHeight: 512,
			palette:      whiteFirstPalette(),
		},
		{
			name:         "full_palette_nonzero_background",
			bounds:       image.Rect(101, -73, 364, 190),
			stride:       271,
			canvasWidth:  512,
			canvasHeight: 512,
			palette:      whiteLastPalette(),
		},
		{
			name:         "full_width_contiguous",
			bounds:       image.Rect(-211, 307, 301, 818),
			stride:       512,
			canvasWidth:  512,
			canvasHeight: 513,
			palette:      color.Palette{color.Black, color.White},
		},
		{
			name:         "large_source_crossing_fill_chunks",
			bounds:       image.Rect(-211, 307, 1289, 1207),
			stride:       1537,
			canvasWidth:  2048,
			canvasHeight: 1025,
			palette:      color.Palette{color.Black, color.White},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := patternedPaletted(test.bounds, test.stride, test.palette)
			got, err := NormalizePalettedImage(context.Background(), source, test.canvasWidth, test.canvasHeight)
			if err != nil {
				t.Fatal(err)
			}
			want := scalarNormalizedPaletted(source, test.canvasWidth, test.canvasHeight)
			if got.Bounds() != want.Bounds() || got.Stride != want.Stride || !bytes.Equal(got.Pix, want.Pix) || !reflect.DeepEqual(got.Palette, want.Palette) {
				t.Fatal("normalized frame differs from scalar canvas")
			}
		})
	}
}

func TestCenteredEncoderMatchesMaterializedNormalization(t *testing.T) {
	for caseIndex := range 48 {
		canvasWidth := caseIndex%37 + 3
		canvasHeight := caseIndex%29 + 3
		frameCount := caseIndex%5 + 1
		frames := make([]*image.Paletted, frameCount)
		for frameIndex := range frames {
			width := (caseIndex*7+frameIndex*3)%canvasWidth + 1
			height := (caseIndex*11+frameIndex*5)%canvasHeight + 1
			minimum := image.Pt(caseIndex%9-4, frameIndex%7-3)
			bounds := image.Rectangle{Min: minimum, Max: minimum.Add(image.Pt(width, height))}
			paletteLength := []int{1, 2, 3, 4, 16, 255, 256}[(caseIndex+frameIndex)%7]
			palette := make(color.Palette, paletteLength)
			for index := range palette {
				palette[index] = color.RGBA{R: uint8(index * 17), G: uint8(index * 31), B: uint8(index * 47), A: 255}
			}
			frames[frameIndex] = patternedPaletted(bounds, width+frameIndex%3, palette)
		}

		materialized := make([]*image.Paletted, len(frames))
		for index, frame := range frames {
			clone := clonePaletted(frame)
			var err error
			materialized[index], err = NormalizePalettedImage(context.Background(), clone, canvasWidth, canvasHeight)
			if err != nil {
				t.Fatalf("case %d frame %d normalization: %v", caseIndex, index, err)
			}
		}
		want := encodeOpaquePalettedFramesForTest(t, materialized, 1000, 1<<30)
		got, err := AnimateCenteredOpaquePalettedImagesWithLimit(context.Background(), frames, canvasWidth, canvasHeight, 1000, 1<<30)
		if err != nil {
			t.Fatalf("case %d centered encoding: %v", caseIndex, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("case %d centered GIF differs from materialized normalization: %d versus %d bytes", caseIndex, len(got), len(want))
		}
	}
}

func TestCenteredOpaqueEncoderRejectsNonOpaquePalettes(t *testing.T) {
	for _, palette := range []color.Palette{
		{color.NRGBA{R: 17, G: 31, B: 47, A: 0}, color.White},
		{color.NRGBA{R: 17, G: 31, B: 47, A: 127}, color.White},
	} {
		frame := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		if encoded, err := AnimateCenteredOpaquePalettedImagesWithLimit(context.Background(), []*image.Paletted{frame}, 4, 4, 1000, 1<<20); err == nil || encoded != nil {
			t.Fatalf("centered opaque encoder accepted palette alpha %#v", palette[0])
		}
	}
}

func TestCenteredOpaqueEncoderDoesNotMutateInputPalettes(t *testing.T) {
	for _, paletteLength := range []int{255, 256} {
		palette := make(color.Palette, paletteLength)
		for index := range palette {
			palette[index] = color.RGBA{R: uint8(index), G: uint8(index * 3), B: uint8(index * 7), A: 0xff}
		}
		frame := image.NewPaletted(image.Rect(0, 0, 4, 4), palette)
		before := append(color.Palette(nil), frame.Palette...)
		if _, err := AnimateCenteredOpaquePalettedImagesWithLimit(context.Background(), []*image.Paletted{frame}, 4, 4, 1000, 1<<20); err != nil {
			t.Fatalf("palette length %d: %v", paletteLength, err)
		}
		if !reflect.DeepEqual(frame.Palette, before) {
			t.Fatalf("palette length %d was mutated", paletteLength)
		}
	}
}

func TestCenteredOpaqueEncoderPreservesCancellationAndOutputLimits(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	frames := []*image.Paletted{frame, frame}
	want, err := AnimateCenteredOpaquePalettedImagesWithLimit(context.Background(), frames, 4, 4, 1000, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	got, err := AnimateCenteredOpaquePalettedImagesWithLimit(context.Background(), frames, 4, 4, 1000, int64(len(want)))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("exact output limit = %d bytes/%v, want %d bytes/nil", len(got), err, len(want))
	}
	if encoded, err := AnimateCenteredOpaquePalettedImagesWithLimit(context.Background(), frames, 4, 4, 1000, int64(len(want)-1)); err == nil || encoded != nil {
		t.Fatalf("below-exact output limit = %d bytes/%v, want nil/error", len(encoded), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if encoded, err := AnimateCenteredOpaquePalettedImagesWithLimit(ctx, frames, 4, 4, 1000, int64(len(want))); !errors.Is(err, context.Canceled) || encoded != nil {
		t.Fatalf("canceled centered encoding = %d bytes/%v, want nil/context.Canceled", len(encoded), err)
	}
}

func clonePaletted(source *image.Paletted) *image.Paletted {
	clone := *source
	clone.Pix = bytes.Clone(source.Pix)
	clone.Palette = append(color.Palette(nil), source.Palette...)
	return &clone
}

func TestNormalizePalettedImageCancellationCheckCadence(t *testing.T) {
	const (
		canvasWidth  = 2048
		canvasHeight = 1025
		sourceHeight = 513
	)
	wantChecks := 1 + (sourceHeight+255)/256 + 1 + (canvasWidth*canvasHeight+(1<<20)-1)/(1<<20) + (sourceHeight+255)/256 + 1
	palette255 := make(color.Palette, 255)
	for index := range palette255 {
		palette255[index] = color.Black
	}
	for _, palette := range []color.Palette{{color.Black}, palette255, whiteFirstPalette()} {
		source := patternedPaletted(image.Rect(-7, 11, 1017, 11+sourceHeight), 1031, palette)
		success := &normalizationCheckContext{}
		if _, err := NormalizePalettedImage(success, source, canvasWidth, canvasHeight); err != nil {
			t.Fatal(err)
		}
		if success.checks != wantChecks {
			t.Fatalf("successful normalization with %d colors checked context %d times, want %d", len(palette), success.checks, wantChecks)
		}

		for failAt := 1; failAt <= wantChecks; failAt++ {
			ctx := &normalizationCheckContext{failAt: failAt}
			frame, err := NormalizePalettedImage(ctx, source, canvasWidth, canvasHeight)
			if frame != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("%d-color cancellation at check %d = (%v, %v), want (nil, context.Canceled)", len(palette), failAt, frame, err)
			}
			if ctx.checks != failAt {
				t.Fatalf("%d-color cancellation at check %d made %d checks", len(palette), failAt, ctx.checks)
			}
		}
	}
}

func TestValidatePalettedImageIndexBoundaries(t *testing.T) {
	for _, paletteLength := range []int{1, 2, 3, 4, 8, 16, 32, 64, 127, 128, 255} {
		palette := make(color.Palette, paletteLength)
		for index := range palette {
			palette[index] = color.Black
		}
		for _, width := range []int{1, 7, 8, 9, 255, 257} {
			bounds := image.Rect(-17, 23, -17+width, 25)
			frame := patternedPaletted(bounds, width+13, palette)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				rowOffset := frame.PixOffset(bounds.Min.X, y)
				for x := 0; x < width; x++ {
					frame.Pix[rowOffset+x] = byte(paletteLength - 1)
				}
			}
			// Storage outside the active rectangle is not part of the image and
			// may contain any byte, including an otherwise invalid palette index.
			for index := width; index < frame.Stride; index++ {
				frame.Pix[index] = 255
			}
			if err := validatePalettedImage(context.Background(), frame); err != nil {
				t.Fatalf("valid %d-color %d-wide frame: %v", paletteLength, width, err)
			}
			secondRow := frame.PixOffset(bounds.Min.X, bounds.Min.Y+1)
			for _, position := range []int{0, width / 2, width - 1, secondRow, secondRow + width - 1} {
				frame.Pix[position] = byte(paletteLength)
				if err := validatePalettedImage(context.Background(), frame); err == nil {
					t.Fatalf("accepted index %d at position %d in %d-wide %d-color frame", paletteLength, position, width, paletteLength)
				}
				frame.Pix[position] = byte(paletteLength - 1)
			}
		}
	}
}

func TestValidatePalettedImagePackedIndexCheckExhaustive(t *testing.T) {
	for paletteLength := 1; paletteLength < 256; paletteLength++ {
		palette := make(color.Palette, paletteLength)
		for index := range palette {
			palette[index] = color.Black
		}
		frame := image.NewPaletted(image.Rect(0, 0, 8, 1), palette)
		for value := 0; value < 256; value++ {
			for index := range frame.Pix {
				frame.Pix[index] = byte(value)
			}
			err := validatePalettedImage(context.Background(), frame)
			if (err != nil) != (value >= paletteLength) {
				t.Fatalf("palette length %d index %d validation error = %v", paletteLength, value, err)
			}
		}
	}
}

func TestFindWhiteIndexPreservesSelectionRules(t *testing.T) {
	if got := findWhiteIndex(rgbaPalette()); got != 255 {
		t.Fatalf("RGBA palette background index = %d, want 255", got)
	}
	if got := findWhiteIndex(color.Palette{
		color.RGBA64{R: 2_000, G: 2_000, B: 2_000, A: 0xffff},
		color.RGBA64{R: 255, G: 255, B: 255, A: 0xffff},
		color.RGBA64{R: 0xffff, G: 0xffff, B: 0xffff, A: 0xffff},
	}); got != 1 {
		t.Fatalf("exact 255-channel entry index = %d, want 1", got)
	}
	if got := findWhiteIndex(color.Palette{
		color.RGBA64{R: 700, G: 200, B: 100, A: 0xffff},
		color.RGBA64{R: 100, G: 200, B: 700, A: 0xffff},
		color.RGBA64{R: 1_001, A: 0xffff},
	}); got != 2 {
		t.Fatalf("highest channel-sum entry index = %d, want 2", got)
	}
	if got := findWhiteIndex(color.Palette{color.Black, color.Black}); got != 0 {
		t.Fatalf("tied entry index = %d, want first entry", got)
	}
}

func BenchmarkNormalizePalettedImage(b *testing.B) {
	benchmarks := []struct {
		name                              string
		bounds                            image.Rectangle
		stride, canvasWidth, canvasHeight int
		palette                           color.Palette
	}{
		{"tiny_background_0", image.Rect(7, -3, 8, -2), 1, 2, 1, whiteFirstPalette()},
		{"tiny_background_rgba_255", image.Rect(7, -3, 8, -2), 1, 2, 1, rgbaPalette()},
		{"tiny_background_nonzero", image.Rect(7, -3, 8, -2), 1, 2, 1, color.Palette{color.Black}},
		{"small_background_0", image.Rect(7, -3, 39, 29), 37, 64, 64, whiteFirstPalette()},
		{"small_background_nonzero", image.Rect(7, -3, 39, 29), 37, 64, 64, color.Palette{color.Black, color.RGBA{R: 127, A: 255}}},
		{"large_background_0", image.Rect(7, -3, 135, 125), 139, 2048, 2048, whiteFirstPalette()},
		{"large_background_255", image.Rect(7, -3, 135, 125), 139, 2048, 2048, whiteLastPalette()},
		{"large_background_nonzero", image.Rect(7, -3, 135, 125), 139, 2048, 2048, color.Palette{color.Black, color.RGBA{R: 127, A: 255}}},
		{"near_full_contiguous", image.Rect(-13, 19, 2035, 2065), 2048, 2048, 2048, color.Palette{color.Black, color.RGBA{R: 127, A: 255}}},
		{"near_full_padded", image.Rect(-13, 19, 1998, 2006), 2048, 2048, 2048, color.Palette{color.Black, color.RGBA{R: 127, A: 255}}},
		{"near_full_palette_256", image.Rect(-13, 19, 2035, 2065), 2048, 2048, 2048, whiteFirstPalette()},
		{"near_full_palette_256_nonzero", image.Rect(-13, 19, 2035, 2065), 2048, 2048, 2048, whiteLastPalette()},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			source := patternedPaletted(benchmark.bounds, benchmark.stride, benchmark.palette)
			b.ReportAllocs()
			b.SetBytes(int64(benchmark.canvasWidth * benchmark.canvasHeight))
			b.ResetTimer()
			for range b.N {
				var err error
				normalizeBenchmarkSink, err = NormalizePalettedImage(context.Background(), source, benchmark.canvasWidth, benchmark.canvasHeight)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAnimateCenteredPalettedImages(b *testing.B) {
	frames := make([]*image.Paletted, 12)
	palette := make(color.Palette, 16)
	for index := range palette {
		palette[index] = color.RGBA{R: uint8(index * 17), G: uint8(index * 31), B: uint8(index * 47), A: 255}
	}
	for index := range frames {
		frames[index] = patternedPaletted(image.Rect(-7, 11, 121, 139), 131, append(color.Palette(nil), palette...))
	}
	const canvasSize = 512
	b.Run("Materialized", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			normalized := make([]*image.Paletted, len(frames))
			for index, frame := range frames {
				var err error
				normalized[index], err = NormalizePalettedImage(context.Background(), frame, canvasSize, canvasSize)
				if err != nil {
					b.Fatal(err)
				}
			}
			encodedGIFBenchmarkSink = encodeOpaquePalettedFramesForTest(b, normalized, 1000, 1<<30)
		}
	})
	b.Run("Centered", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			var err error
			encodedGIFBenchmarkSink, err = AnimateCenteredOpaquePalettedImagesWithLimit(context.Background(), frames, canvasSize, canvasSize, 1000, 1<<30)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func patternedPaletted(bounds image.Rectangle, stride int, palette color.Palette) *image.Paletted {
	pixels := make([]byte, (bounds.Dy()-1)*stride+bounds.Dx())
	for y := 0; y < bounds.Dy(); y++ {
		row := pixels[y*stride : y*stride+bounds.Dx()]
		for x := range row {
			row[x] = uint8((x*17 + y*31) % len(palette))
		}
	}
	return &image.Paletted{Pix: pixels, Stride: stride, Rect: bounds, Palette: palette}
}

func whiteFirstPalette() color.Palette {
	palette := make(color.Palette, 256)
	palette[0] = color.White
	for index := 1; index < len(palette); index++ {
		palette[index] = color.Black
	}
	return palette
}

func whiteLastPalette() color.Palette {
	palette := make(color.Palette, 256)
	for index := 0; index < len(palette)-1; index++ {
		palette[index] = color.Black
	}
	palette[len(palette)-1] = color.White
	return palette
}

func rgbaPalette() color.Palette {
	palette := make(color.Palette, 256)
	for index := range palette {
		value := uint8(index)
		palette[index] = color.RGBA{R: value, G: value, B: value, A: 255}
	}
	return palette
}

func scalarNormalizedPaletted(source *image.Paletted, width, height int) *image.Paletted {
	palette := append(color.Palette(nil), source.Palette...)
	palette, backgroundIndex := paletteWithBackground(palette)
	frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
	for index := range frame.Pix {
		frame.Pix[index] = uint8(backgroundIndex)
	}
	bounds := source.Bounds()
	top := (height - bounds.Dy()) / 2
	left := (width - bounds.Dx()) / 2
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		sourceOffset := source.PixOffset(bounds.Min.X, y)
		destinationOffset := frame.PixOffset(left, top+y-bounds.Min.Y)
		copy(frame.Pix[destinationOffset:destinationOffset+bounds.Dx()], source.Pix[sourceOffset:sourceOffset+bounds.Dx()])
	}
	return frame
}

type normalizationCheckContext struct {
	checks int
	failAt int
}

func (*normalizationCheckContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*normalizationCheckContext) Done() <-chan struct{}       { return nil }
func (*normalizationCheckContext) Value(any) any               { return nil }
func (ctx *normalizationCheckContext) Err() error {
	ctx.checks++
	if ctx.failAt != 0 && ctx.checks >= ctx.failAt {
		return context.Canceled
	}
	return nil
}
