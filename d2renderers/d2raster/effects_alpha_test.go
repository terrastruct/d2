package d2raster

import (
	"bytes"
	"context"
	"errors"
	"image"
	"testing"
)

func multiplyLayerByAlphaScalar(ctx context.Context, layer *image.RGBA, mask *image.Alpha) error {
	width, height := layer.Bounds().Dx(), layer.Bounds().Dy()
	for y := 0; y < height; y++ {
		if y&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		layerOffset := layer.PixOffset(layer.Bounds().Min.X, layer.Bounds().Min.Y+y)
		maskOffset := mask.PixOffset(mask.Bounds().Min.X, mask.Bounds().Min.Y+y)
		for x := 0; x < width; x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			scalePremultiplied(layer.Pix[layerOffset+x*4:layerOffset+x*4+4], mask.Pix[maskOffset+x])
		}
	}
	return ctx.Err()
}

func multiplyLayerByRGBAAlphaScalar(ctx context.Context, layer, mask *image.RGBA) error {
	width, height := layer.Bounds().Dx(), layer.Bounds().Dy()
	for y := 0; y < height; y++ {
		if y&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		layerOffset := layer.PixOffset(layer.Bounds().Min.X, layer.Bounds().Min.Y+y)
		maskOffset := mask.PixOffset(mask.Bounds().Min.X, mask.Bounds().Min.Y+y)
		for x := 0; x < width; x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			scalePremultiplied(layer.Pix[layerOffset+x*4:layerOffset+x*4+4], mask.Pix[maskOffset+x*4+3])
		}
	}
	return ctx.Err()
}

func TestMultiplyLayerByAlphaMatchesScalar(t *testing.T) {
	for _, width := range []int{0, 1, 7, 4095, 4096, 4097, 8193} {
		const height = 3
		layerStride := width*4 + 11
		maskStride := width + 5
		layerBounds := image.Rect(-7, 11, -7+width, 11+height)
		maskBounds := image.Rect(19, -3, 19+width, -3+height)
		layerPixels := make([]byte, layerStride*height+7)
		maskPixels := make([]byte, maskStride*height+3)
		for index := range layerPixels {
			layerPixels[index] = byte(index*73 + 41)
		}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				switch (x/17 + y) % 5 {
				case 0:
					maskPixels[y*maskStride+x] = 0
				case 1:
					maskPixels[y*maskStride+x] = 0xff
				default:
					maskPixels[y*maskStride+x] = byte(x*29 + y*61 + 1)
				}
			}
		}
		gotPixels := append([]byte(nil), layerPixels...)
		wantPixels := append([]byte(nil), layerPixels...)
		got := &image.RGBA{Pix: gotPixels, Stride: layerStride, Rect: layerBounds}
		want := &image.RGBA{Pix: wantPixels, Stride: layerStride, Rect: layerBounds}
		mask := &image.Alpha{Pix: maskPixels, Stride: maskStride, Rect: maskBounds}
		if err := multiplyLayerByAlpha(context.Background(), got, mask); err != nil {
			t.Fatalf("width %d optimized: %v", width, err)
		}
		if err := multiplyLayerByAlphaScalar(context.Background(), want, mask); err != nil {
			t.Fatalf("width %d scalar: %v", width, err)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("width %d output differs", width)
		}
	}
}

func TestMultiplyLayerByAlphaCancellationMatchesScalar(t *testing.T) {
	const width, height = 9000, 65
	layerBounds := image.Rect(-2, 4, -2+width, 4+height)
	maskBounds := image.Rect(7, -9, 7+width, -9+height)
	layerPixels := make([]byte, width*height*4)
	maskPixels := make([]byte, width*height)
	for index := range layerPixels {
		layerPixels[index] = byte(index*37 + 13)
	}
	for index := range maskPixels {
		maskPixels[index] = [...]byte{0, 0xff, 97}[index%3]
	}
	mask := &image.Alpha{Pix: maskPixels, Stride: width, Rect: maskBounds}
	for _, cancelAt := range []int{1, 2, 3, 4, 7, 31, 63, 97, 131, 197} {
		gotPixels := append([]byte(nil), layerPixels...)
		wantPixels := append([]byte(nil), layerPixels...)
		got := &image.RGBA{Pix: gotPixels, Stride: width * 4, Rect: layerBounds}
		want := &image.RGBA{Pix: wantPixels, Stride: width * 4, Rect: layerBounds}
		gotContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
		wantContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
		gotErr := multiplyLayerByAlpha(gotContext, got, mask)
		wantErr := multiplyLayerByAlphaScalar(wantContext, want, mask)
		if errors.Is(gotErr, context.Canceled) != errors.Is(wantErr, context.Canceled) {
			t.Fatalf("cancel call %d errors differ: optimized %v, scalar %v", cancelAt, gotErr, wantErr)
		}
		if gotContext.calls != wantContext.calls {
			t.Fatalf("cancel call %d Err calls = %d, want %d", cancelAt, gotContext.calls, wantContext.calls)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("cancel call %d left a different output prefix", cancelAt)
		}
	}
}

func TestMultiplyLayerByRGBAAlphaMatchesScalar(t *testing.T) {
	const width, height = 8193, 3
	layerStride := width*4 + 11
	maskStride := width*4 + 17
	layerBounds := image.Rect(-7, 11, -7+width, 11+height)
	maskBounds := image.Rect(19, -3, 19+width, -3+height)
	layerPixels := make([]byte, layerStride*height+7)
	maskPixels := make([]byte, maskStride*height+3)
	for index := range layerPixels {
		layerPixels[index] = byte(index*73 + 41)
	}
	for index := range maskPixels {
		maskPixels[index] = byte(index*19 + 7)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			switch (x/17 + y) % 5 {
			case 0:
				maskPixels[y*maskStride+x*4+3] = 0
			case 1:
				maskPixels[y*maskStride+x*4+3] = 0xff
			default:
				maskPixels[y*maskStride+x*4+3] = byte(x*29 + y*61 + 1)
			}
		}
	}
	mask := &image.RGBA{Pix: maskPixels, Stride: maskStride, Rect: maskBounds}
	for _, cancelAt := range []int{0, 1, 2, 3, 4, 7, 17} {
		got := &image.RGBA{Pix: bytes.Clone(layerPixels), Stride: layerStride, Rect: layerBounds}
		want := &image.RGBA{Pix: bytes.Clone(layerPixels), Stride: layerStride, Rect: layerBounds}
		var gotContext, wantContext context.Context = context.Background(), context.Background()
		var gotCounter, wantCounter *cancelAfterErrCallsContext
		if cancelAt != 0 {
			gotCounter = &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
			wantCounter = &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
			gotContext, wantContext = gotCounter, wantCounter
		}
		gotErr := multiplyLayerByRGBAAlpha(gotContext, got, mask)
		wantErr := multiplyLayerByRGBAAlphaScalar(wantContext, want, mask)
		if errors.Is(gotErr, context.Canceled) != errors.Is(wantErr, context.Canceled) {
			t.Fatalf("cancel call %d errors differ: optimized %v, scalar %v", cancelAt, gotErr, wantErr)
		}
		if gotCounter != nil && gotCounter.calls != wantCounter.calls {
			t.Fatalf("cancel call %d Err calls = %d, want %d", cancelAt, gotCounter.calls, wantCounter.calls)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("cancel call %d output differs", cancelAt)
		}
	}
}

func BenchmarkMultiplyLayerByAlpha(b *testing.B) {
	const width, height = 2048, 512
	patterns := []struct {
		name string
		fill func(mask []byte)
	}{
		{name: "opaque", fill: func(mask []byte) {
			for index := range mask {
				mask[index] = 0xff
			}
		}},
		{name: "transparent", fill: func(mask []byte) { clear(mask) }},
		{name: "long_runs", fill: func(mask []byte) {
			for index := range mask {
				mask[index] = [...]byte{0, 0xff, 113, 0xff}[(index/256)%4]
			}
		}},
		{name: "alternating", fill: func(mask []byte) {
			for index := range mask {
				mask[index] = [...]byte{0, 0xff, 113}[index%3]
			}
		}},
		{name: "partial", fill: func(mask []byte) {
			for index := range mask {
				mask[index] = byte(index%253 + 1)
			}
		}},
	}
	for _, pattern := range patterns {
		mask := image.NewAlpha(image.Rect(0, 0, width, height))
		pattern.fill(mask.Pix)
		for _, implementation := range []struct {
			name string
			fn   func(context.Context, *image.RGBA, *image.Alpha) error
		}{
			{name: "optimized", fn: multiplyLayerByAlpha},
			{name: "scalar", fn: multiplyLayerByAlphaScalar},
		} {
			b.Run(pattern.name+"/"+implementation.name, func(b *testing.B) {
				layer := image.NewRGBA(image.Rect(0, 0, width, height))
				for index := range layer.Pix {
					layer.Pix[index] = byte(index*31 + 17)
				}
				ctx := context.Background()
				b.ReportAllocs()
				b.SetBytes(width * height)
				b.ResetTimer()
				for range b.N {
					if err := implementation.fn(ctx, layer, mask); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkMultiplyLayerByRGBAAlpha(b *testing.B) {
	const width, height = 2048, 512
	patterns := []struct {
		name string
		fill func(mask []byte)
	}{
		{name: "opaque", fill: func(mask []byte) {
			for index := 3; index < len(mask); index += 4 {
				mask[index] = 0xff
			}
		}},
		{name: "transparent", fill: func(mask []byte) { clear(mask) }},
		{name: "alternating", fill: func(mask []byte) {
			for index := 3; index < len(mask); index += 4 {
				mask[index] = [...]byte{0, 0xff, 113}[(index/4)%3]
			}
		}},
		{name: "partial", fill: func(mask []byte) {
			for index := 3; index < len(mask); index += 4 {
				mask[index] = byte((index/4)%253 + 1)
			}
		}},
	}
	for _, pattern := range patterns {
		mask := image.NewRGBA(image.Rect(0, 0, width, height))
		pattern.fill(mask.Pix)
		for _, implementation := range []struct {
			name string
			fn   func(context.Context, *image.RGBA, *image.RGBA) error
		}{
			{name: "optimized", fn: multiplyLayerByRGBAAlpha},
			{name: "scalar", fn: multiplyLayerByRGBAAlphaScalar},
		} {
			b.Run(pattern.name+"/"+implementation.name, func(b *testing.B) {
				layer := image.NewRGBA(image.Rect(0, 0, width, height))
				for index := range layer.Pix {
					layer.Pix[index] = byte(index*31 + 17)
				}
				ctx := context.Background()
				b.ReportAllocs()
				b.SetBytes(width * height)
				b.ResetTimer()
				for range b.N {
					if err := implementation.fn(ctx, layer, mask); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
