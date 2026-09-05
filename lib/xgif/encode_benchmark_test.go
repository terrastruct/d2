package xgif

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

var encodedGIFBenchmarkSink []byte

func BenchmarkEncodePalettedImages(b *testing.B) {
	for _, benchmark := range []struct {
		name      string
		frames    int
		width     int
		height    int
		shared    bool
		fewColors int
	}{
		{name: "one_16x16_shared_16", frames: 1, width: 16, height: 16, shared: true, fewColors: 16},
		{name: "one_32x32_shared_256", frames: 1, width: 32, height: 32, shared: true, fewColors: 256},
		{name: "one_64x64_shared_256", frames: 1, width: 64, height: 64, shared: true, fewColors: 256},
		{name: "one_128x128_shared_256", frames: 1, width: 128, height: 128, shared: true, fewColors: 256},
		{name: "one_256x256_shared_256", frames: 1, width: 256, height: 256, shared: true, fewColors: 256},
		{name: "one_512x512_shared_256", frames: 1, width: 512, height: 512, shared: true, fewColors: 256},
		{name: "one_512x512_shared_16", frames: 1, width: 512, height: 512, shared: true, fewColors: 16},
		{name: "thirty_256x256_shared_256", frames: 30, width: 256, height: 256, shared: true, fewColors: 256},
		{name: "thirty_256x256_distinct_256", frames: 30, width: 256, height: 256, fewColors: 256},
		{name: "thirty_512x512_shared_16", frames: 30, width: 512, height: 512, shared: true, fewColors: 16},
		{name: "thirty_512x512_shared_17", frames: 30, width: 512, height: 512, shared: true, fewColors: 17},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			frames := benchmarkPalettedFrames(benchmark.frames, benchmark.width, benchmark.height, benchmark.fewColors, benchmark.shared)
			encoded, err := AnimatePalettedImagesWithLimit(context.Background(), frames, 1000, 1<<30)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(benchmark.frames * benchmark.width * benchmark.height))
			b.ResetTimer()
			for range b.N {
				encodedGIFBenchmarkSink, err = AnimatePalettedImagesWithLimit(context.Background(), frames, 1000, 1<<30)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(encoded)), "encoded-B")
		})
	}
}

func BenchmarkOpaquePalettedAnimationEncoder(b *testing.B) {
	frames := benchmarkPalettedFrames(30, 256, 256, 256, true)
	b.ReportAllocs()
	b.SetBytes(int64(len(frames) * 256 * 256))
	for range b.N {
		encoder, err := NewOpaquePalettedAnimationEncoder(context.Background(), 256, 256, len(frames), 1000, 1<<30)
		if err != nil {
			b.Fatal(err)
		}
		for _, frame := range frames {
			if err := encoder.WriteFrame(frame); err != nil {
				b.Fatal(err)
			}
		}
		encodedGIFBenchmarkSink, err = encoder.Finish()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPalettedEncoderCompatibility(b *testing.B) {
	frames := benchmarkPalettedFrames(30, 256, 256, 256, true)
	delays, err := gifFrameDelays(len(frames), 1000)
	if err != nil {
		b.Fatal(err)
	}
	animation := &gif.GIF{
		Image: frames, Delay: delays, LoopCount: INFINITE_LOOP,
		Config: image.Config{Width: 256, Height: 256},
	}
	var standard bytes.Buffer
	if err := gif.EncodeAll(&standard, animation); err != nil {
		b.Fatal(err)
	}
	custom, err := AnimatePalettedImagesWithLimit(context.Background(), frames, 1000, 1<<30)
	if err != nil {
		b.Fatal(err)
	}
	if !bytes.Equal(custom, standard.Bytes()) {
		b.Fatal("paired encoders produced different GIF bytes")
	}

	b.Run("StandardLibrary", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(frames) * 256 * 256))
		b.ReportMetric(float64(standard.Len()), "encoded-B")
		for range b.N {
			var output bytes.Buffer
			if err := gif.EncodeAll(&output, animation); err != nil {
				b.Fatal(err)
			}
			encodedGIFBenchmarkSink = output.Bytes()
		}
	})
	b.Run("ReusableStandardLZW", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(frames) * 256 * 256))
		b.ReportMetric(float64(len(custom)), "encoded-B")
		for range b.N {
			encodedGIFBenchmarkSink, err = AnimatePalettedImagesWithLimit(context.Background(), frames, 1000, 1<<30)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	opaque := encodeOpaquePalettedFramesForTest(b, frames, 1000, 1<<30)
	b.Run("ValidatedOpaqueGlobalTable", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(frames) * 256 * 256))
		b.ReportMetric(float64(len(opaque)), "encoded-B")
		for range b.N {
			encodedGIFBenchmarkSink = encodeOpaquePalettedFramesForTest(b, frames, 1000, 1<<30)
		}
	})
}

func benchmarkPalettedFrames(frameCount, width, height, colorCount int, sharedPalette bool) []*image.Paletted {
	palette := make(color.Palette, colorCount)
	for index := range palette {
		palette[index] = color.RGBA{R: uint8(index * 37), G: uint8(index * 73), B: uint8(index * 109), A: 255}
	}
	frames := make([]*image.Paletted, frameCount)
	for frameIndex := range frames {
		framePalette := palette
		if !sharedPalette {
			framePalette = append(color.Palette(nil), palette...)
		}
		frame := image.NewPaletted(image.Rect(0, 0, width, height), framePalette)
		for y := range height {
			row := frame.Pix[y*frame.Stride : y*frame.Stride+width]
			for x := range row {
				row[x] = uint8((x*31 + y*17 + frameIndex*7) % colorCount)
			}
		}
		frames[frameIndex] = frame
	}
	return frames
}
