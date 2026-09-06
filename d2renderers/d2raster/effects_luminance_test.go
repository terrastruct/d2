package d2raster

import (
	"bytes"
	"context"
	"errors"
	"image"
	"testing"
)

func multiplyLayerByRGBALuminanceScalar(ctx context.Context, layer, mask *image.RGBA) error {
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
			i := maskOffset + x*4
			coverage := uint8((2126*uint32(mask.Pix[i]) +
				7152*uint32(mask.Pix[i+1]) +
				722*uint32(mask.Pix[i+2]) + 5000) / 10000)
			j := layerOffset + x*4
			scalePremultiplied(layer.Pix[j:j+4], coverage)
		}
	}
	return ctx.Err()
}

func TestMultiplyLayerByRGBALuminanceMatchesScalar(t *testing.T) {
	for _, width := range []int{0, 1, 7, 63, 64, 65, 4095, 4096, 4097, 8193} {
		const height = 3
		layerStride := width*4 + 13
		maskStride := width*4 + 21
		layerBounds := image.Rect(-7, 11, -7+width, 11+height)
		maskBounds := image.Rect(19, -3, 19+width, -3+height)
		layerPixels := make([]byte, layerStride*height+7)
		maskPixels := make([]byte, maskStride*height+9)
		for index := range layerPixels {
			layerPixels[index] = byte(index*73 + 41)
		}
		for index := range maskPixels {
			maskPixels[index] = byte(index*19 + 7)
		}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				i := y*maskStride + x*4
				switch (x/17 + y) % 5 {
				case 0:
					maskPixels[i], maskPixels[i+1], maskPixels[i+2] = 0, 0, 0
				case 1:
					maskPixels[i], maskPixels[i+1], maskPixels[i+2] = 0xff, 0xff, 0xff
				default:
					maskPixels[i], maskPixels[i+1], maskPixels[i+2] = byte(x*29+y*61+1), byte(x*17+y*43+2), byte(x*7+y*31+3)
				}
			}
		}
		got := &image.RGBA{Pix: bytes.Clone(layerPixels), Stride: layerStride, Rect: layerBounds}
		want := &image.RGBA{Pix: bytes.Clone(layerPixels), Stride: layerStride, Rect: layerBounds}
		mask := &image.RGBA{Pix: maskPixels, Stride: maskStride, Rect: maskBounds}
		if err := multiplyLayerByRGBALuminance(context.Background(), got, mask); err != nil {
			t.Fatalf("width %d optimized: %v", width, err)
		}
		if err := multiplyLayerByRGBALuminanceScalar(context.Background(), want, mask); err != nil {
			t.Fatalf("width %d scalar: %v", width, err)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("width %d output differs", width)
		}
	}
}

func TestMultiplyLayerByRGBALuminanceChannelEdges(t *testing.T) {
	values := [...]byte{0, 1, 2, 126, 127, 128, 129, 253, 254, 255}
	width := len(values) * len(values) * len(values)
	layerBounds := image.Rect(-5, 7, -5+width, 9)
	maskBounds := image.Rect(13, -11, 13+width, -9)
	layerStride := width*4 + 15
	maskStride := width*4 + 23
	layerPixels := make([]byte, layerStride*2+11)
	maskPixels := make([]byte, maskStride*2+17)
	for index := range layerPixels {
		layerPixels[index] = byte(index*47 + 29)
	}
	for y := 0; y < 2; y++ {
		x := 0
		for _, red := range values {
			for _, green := range values {
				for _, blue := range values {
					i := y*maskStride + x*4
					maskPixels[i], maskPixels[i+1], maskPixels[i+2], maskPixels[i+3] = red, green, blue, byte(x*31+y*67)
					x++
				}
			}
		}
	}
	got := &image.RGBA{Pix: bytes.Clone(layerPixels), Stride: layerStride, Rect: layerBounds}
	want := &image.RGBA{Pix: bytes.Clone(layerPixels), Stride: layerStride, Rect: layerBounds}
	mask := &image.RGBA{Pix: maskPixels, Stride: maskStride, Rect: maskBounds}
	if err := multiplyLayerByRGBALuminance(context.Background(), got, mask); err != nil {
		t.Fatal(err)
	}
	if err := multiplyLayerByRGBALuminanceScalar(context.Background(), want, mask); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("channel-edge output differs")
	}
}

func TestMultiplyLayerByRGBALuminanceCancellationMatchesScalar(t *testing.T) {
	const width, height = 9000, 65
	layerBounds := image.Rect(-2, 4, -2+width, 4+height)
	maskBounds := image.Rect(7, -9, 7+width, -9+height)
	layerStride := width*4 + 12
	maskStride := width*4 + 20
	layerPixels := make([]byte, layerStride*height+7)
	maskPixels := make([]byte, maskStride*height+9)
	for index := range layerPixels {
		layerPixels[index] = byte(index*37 + 13)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*maskStride + x*4
			switch (x/257 + y) % 3 {
			case 0:
				maskPixels[i], maskPixels[i+1], maskPixels[i+2] = 0, 0, 0
			case 1:
				maskPixels[i], maskPixels[i+1], maskPixels[i+2] = 0xff, 0xff, 0xff
			default:
				maskPixels[i], maskPixels[i+1], maskPixels[i+2] = 17, 113, 229
			}
		}
	}
	mask := &image.RGBA{Pix: maskPixels, Stride: maskStride, Rect: maskBounds}
	for _, cancelAt := range []int{1, 2, 3, 4, 7, 31, 63, 97, 131, 197} {
		got := &image.RGBA{Pix: bytes.Clone(layerPixels), Stride: layerStride, Rect: layerBounds}
		want := &image.RGBA{Pix: bytes.Clone(layerPixels), Stride: layerStride, Rect: layerBounds}
		gotContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
		wantContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
		gotErr := multiplyLayerByRGBALuminance(gotContext, got, mask)
		wantErr := multiplyLayerByRGBALuminanceScalar(wantContext, want, mask)
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

func BenchmarkMultiplyLayerByRGBALuminance(b *testing.B) {
	const width, height = 2048, 512
	patterns := []struct {
		name string
		fill func([]byte)
	}{
		{name: "black", fill: func(mask []byte) {
			for index := 0; index < len(mask); index += 4 {
				mask[index+3] = 0xff
			}
		}},
		{name: "white", fill: func(mask []byte) {
			for index := 0; index < len(mask); index += 4 {
				mask[index], mask[index+1], mask[index+2], mask[index+3] = 0xff, 0xff, 0xff, 0xff
			}
		}},
		{name: "alternating_binary", fill: func(mask []byte) {
			for index := 0; index < len(mask); index += 4 {
				if index/4&1 != 0 {
					mask[index], mask[index+1], mask[index+2] = 0xff, 0xff, 0xff
				}
				mask[index+3] = 0xff
			}
		}},
		{name: "mixed", fill: func(mask []byte) {
			for index := 0; index < len(mask); index += 4 {
				pixel := index / 4
				switch pixel % 5 {
				case 0:
					mask[index], mask[index+1], mask[index+2] = 0, 0, 0
				case 1:
					mask[index], mask[index+1], mask[index+2] = 0xff, 0xff, 0xff
				default:
					mask[index], mask[index+1], mask[index+2] = byte(pixel*29+1), byte(pixel*17+2), byte(pixel*7+3)
				}
				mask[index+3] = byte(pixel*11 + 5)
			}
		}},
		{name: "partial", fill: func(mask []byte) {
			for index := 0; index < len(mask); index += 4 {
				pixel := index / 4
				mask[index], mask[index+1], mask[index+2], mask[index+3] = byte(pixel%253+1), byte((pixel*3)%253+1), byte((pixel*7)%253+1), byte(pixel*11+5)
			}
		}},
		{name: "high_entropy", fill: func(mask []byte) {
			state := uint64(0x4d595df4d0f33173)
			for index := range mask {
				state ^= state << 13
				state ^= state >> 7
				state ^= state << 17
				mask[index] = byte(state >> 56)
			}
			mask[0], mask[1], mask[2] = 1, 2, 3
		}},
	}
	for _, pattern := range patterns {
		mask := image.NewRGBA(image.Rect(0, 0, width, height))
		pattern.fill(mask.Pix)
		for _, implementation := range []struct {
			name string
			fn   func(context.Context, *image.RGBA, *image.RGBA) error
		}{
			{name: "optimized", fn: multiplyLayerByRGBALuminance},
			{name: "scalar", fn: multiplyLayerByRGBALuminanceScalar},
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
