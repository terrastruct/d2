package d2raster

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

var benchmarkImageSample [4]uint32

func BenchmarkBilinearPremultiplied(b *testing.B) {
	benchmarkBilinearPremultipliedAt(b, .375, .625)
}

func BenchmarkBilinearPremultipliedPixelCenter(b *testing.B) {
	benchmarkBilinearPremultipliedAt(b, .5, .5)
}

func benchmarkBilinearPremultipliedAt(b *testing.B, offsetX, offsetY float64) {
	for name, source := range benchmarkRasterSources() {
		bounds := source.Bounds()
		asset := newPreparedRasterAsset(source)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var sample [4]uint32
			for i := 0; i < b.N; i++ {
				x := float64((i*37)%bounds.Dx()) + offsetX
				y := float64((i*67)%bounds.Dy()) + offsetY
				sample = bilinearPremultiplied(asset, x, y)
			}
			benchmarkImageSample = sample
		})
	}
}

func BenchmarkDrawPreparedImageNativeSize(b *testing.B) {
	for name, source := range benchmarkRasterSources() {
		asset := newPreparedRasterAsset(source)
		box := d2scene.Box{Width: float64(asset.width), Height: float64(asset.bounds.Dy())}
		prepared := &preparedImage{
			asset: asset, box: box, placement: box,
			inverse: inverseAffine{a: 1, d: 1}, bounds: image.Rect(0, 0, asset.width, asset.bounds.Dy()),
		}
		for implementation, drawFn := range map[string]func(context.Context, *image.RGBA, *preparedImage) error{
			"NativeSize": drawPreparedImage,
			"Generic": func(ctx context.Context, destination *image.RGBA, prepared *preparedImage) error {
				return drawNativeSizeGeneric(ctx, destination, prepared.asset, prepared.bounds, image.Point{})
			},
			"Sampled": func(ctx context.Context, destination *image.RGBA, prepared *preparedImage) error {
				return drawSampledPreparedImage(ctx, destination, prepared, prepared.bounds)
			},
		} {
			b.Run(name+"/"+implementation, func(b *testing.B) {
				destination := image.NewRGBA(prepared.bounds)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := drawFn(context.Background(), destination, prepared); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkDrawNativeSizePreparedImageAlphaWorkloads(b *testing.B) {
	bounds := image.Rect(-13, 17, 499, 529)
	origin := image.Pt(bounds.Min.X, bounds.Min.Y)
	workloads := []struct {
		name  string
		alpha func(x, y int) byte
	}{
		{name: "Opaque", alpha: func(_, _ int) byte { return 0xff }},
		{name: "Transparent", alpha: func(_, _ int) byte { return 0 }},
		{name: "SparseOpaque", alpha: func(x, y int) byte {
			if (x*17+y*31)&63 == 0 {
				return 0xff
			}
			return 0
		}},
		{name: "Alternating", alpha: func(x, y int) byte {
			if (x+y)&1 == 0 {
				return 0xff
			}
			return 0
		}},
		{name: "HighEntropy", alpha: func(x, y int) byte { return byte(x*47 + y*71) }},
	}
	for _, kind := range []string{"RGBA", "NRGBA"} {
		for _, workload := range workloads {
			stride := bounds.Dx()*4 + 19
			pixels := make([]byte, stride*bounds.Dy())
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					offset := (y-bounds.Min.Y)*stride + (x-bounds.Min.X)*4
					pixels[offset] = byte(x*29 + y*3)
					pixels[offset+1] = byte(x*7 + y*43)
					pixels[offset+2] = byte(x*53 + y*11)
					pixels[offset+3] = workload.alpha(x, y)
				}
			}
			var source image.Image
			if kind == "RGBA" {
				source = &image.RGBA{Pix: pixels, Stride: stride, Rect: bounds}
			} else {
				source = &image.NRGBA{Pix: pixels, Stride: stride, Rect: bounds}
			}
			asset := newPreparedRasterAsset(source)
			for _, implementation := range []struct {
				name string
				draw func(context.Context, *image.RGBA, *preparedRasterAsset, image.Rectangle, image.Point) error
			}{
				{name: "Concrete", draw: drawNativeSizePreparedImage},
				{name: "Generic", draw: drawNativeSizeGeneric},
			} {
				b.Run(kind+"/"+workload.name+"/"+implementation.name, func(b *testing.B) {
					destination := image.NewRGBA(bounds)
					b.ReportAllocs()
					b.SetBytes(int64(bounds.Dx() * bounds.Dy() * 4))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := implementation.draw(context.Background(), destination, asset, bounds, origin); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}

func BenchmarkCompositePremultipliedRGBA64(b *testing.B) {
	for name, source := range map[string][4]uint32{
		"Opaque":      {0x1234, 0x5678, 0x9abc, 0xffff},
		"Translucent": {0x1234, 0x2345, 0x3456, 0x789a},
	} {
		b.Run(name, func(b *testing.B) {
			destination := []byte{31, 47, 59, 127}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				compositePremultipliedRGBA64(destination, source)
			}
		})
	}
}

func benchmarkRasterSources() map[string]image.Image {
	bounds := image.Rect(11, 17, 267, 273)
	nrgba := image.NewNRGBA(bounds)
	rgba := image.NewRGBA(bounds)
	nrgba64 := image.NewNRGBA64(bounds)
	rgba64 := image.NewRGBA64(bounds)
	gray := image.NewGray(bounds)
	gray16 := image.NewGray16(bounds)
	alpha := image.NewAlpha(bounds)
	alpha16 := image.NewAlpha16(bounds)
	cmyk := image.NewCMYK(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			value := color.NRGBA{
				R: uint8(x*31 + y*7), G: uint8(x*11 + y*29),
				B: uint8(x*19 + y*13), A: uint8(1 + (x*5+y*3)%255),
			}
			nrgba.SetNRGBA(x, y, value)
			rgba.Set(x, y, value)
			nrgba64.Set(x, y, value)
			rgba64.Set(x, y, value)
			gray.Set(x, y, value)
			gray16.Set(x, y, value)
			alpha.Set(x, y, value)
			alpha16.Set(x, y, value)
			cmyk.Set(x, y, value)
		}
	}
	ycbcr := image.NewYCbCr(bounds, image.YCbCrSubsampleRatio420)
	for i := range ycbcr.Y {
		ycbcr.Y[i] = uint8(i*17 + 23)
	}
	for i := range ycbcr.Cb {
		ycbcr.Cb[i] = uint8(i*29 + 41)
		ycbcr.Cr[i] = uint8(i*37 + 59)
	}
	nycbcra := &image.NYCbCrA{
		YCbCr: *ycbcr, A: make([]uint8, bounds.Dx()*bounds.Dy()), AStride: bounds.Dx(),
	}
	for i := range nycbcra.A {
		nycbcra.A[i] = uint8(i*41 + 7)
	}
	palette := make(color.Palette, 256)
	for i := range palette {
		palette[i] = color.NRGBA{
			R: uint8(i), G: uint8(i * 31), B: uint8(i * 67), A: uint8(1 + i%255),
		}
	}
	paletted := image.NewPaletted(bounds, palette)
	for i := range paletted.Pix {
		paletted.Pix[i] = uint8(i * 43)
	}
	return map[string]image.Image{
		"NRGBA": nrgba, "RGBA": rgba, "NRGBA64": nrgba64, "RGBA64": rgba64,
		"Gray": gray, "Gray16": gray16, "Alpha": alpha, "Alpha16": alpha16,
		"CMYK": cmyk, "YCbCr420": ycbcr, "NYCbCrA420": nycbcra, "Paletted": paletted,
	}
}
