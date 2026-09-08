package d2cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	stdpng "image/png"
	"io"
	"math"
	"math/rand"
	"testing"

	d2png "github.com/d2lang/d2/lib/png"
)

func TestRasterPNGBandsMatchImageEncoder(t *testing.T) {
	for _, opaque := range []bool{true, false} {
		for _, width := range []int{1, 2, 7, 257} {
			t.Run(fmt.Sprintf("opaque=%t/width=%d", opaque, width), func(t *testing.T) {
				const height = 37
				// Cropping a wider image exercises stride padding and nonzero
				// storage offsets independently of the bands' absolute Y values.
				storage := image.NewNRGBA(image.Rect(-3, -2, width+5, height+4))
				full := storage.SubImage(image.Rect(0, 0, width, height)).(*image.NRGBA)
				random := rand.New(rand.NewSource(21))
				for y := 0; y < height; y++ {
					for x := 0; x < width; x++ {
						value := random.Uint32()
						alpha := byte(value >> 24)
						if opaque {
							alpha = 255
						} else if y == height-1 {
							// Keep RGB values even when alpha is zero.
							alpha = 0
						}
						full.SetNRGBA(x, y, color.NRGBA{byte(value), byte(value >> 8), byte(value >> 16), alpha})
					}
				}
				var reference bytes.Buffer
				encoder := stdpng.Encoder{CompressionLevel: stdpng.BestSpeed}
				if err := encoder.Encode(&reference, full); err != nil {
					t.Fatal(err)
				}
				want, err := d2png.AddExif(reference.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				var stream rasterPNGBandEncoder
				defer stream.close()
				if err := stream.start(context.Background(), &out, width, height, opaque); err != nil {
					t.Fatal(err)
				}
				for y := 0; y < height; {
					end := min(height, y+1+random.Intn(7))
					band := full.SubImage(image.Rect(0, y, width, end)).(*image.NRGBA)
					if err := stream.append(band); err != nil {
						t.Fatal(err)
					}
					y = end
				}
				if err := stream.finish(); err != nil {
					t.Fatal(err)
				}
				decoded, err := stdpng.Decode(bytes.NewReader(out.Bytes()))
				if err != nil {
					t.Fatal(err)
				}
				if decoded.Bounds() != full.Bounds() {
					t.Fatalf("decoded bounds %v, want %v", decoded.Bounds(), full.Bounds())
				}
				for y := 0; y < height; y++ {
					for x := 0; x < width; x++ {
						if got := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA); got != full.NRGBAAt(x, y) {
							t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, full.NRGBAAt(x, y))
						}
					}
				}
				if !bytes.Equal(out.Bytes(), want) {
					t.Fatal("streamed bands differ from the standard image encoder with D2 EXIF")
				}
			})
		}
	}
}

func TestRasterPNGBandsValidateSequenceAndStorage(t *testing.T) {
	tests := []struct {
		name string
		band *image.NRGBA
	}{
		{"nil", nil},
		{"gap", image.NewNRGBA(image.Rect(0, 1, 4, 2))},
		{"offset X", image.NewNRGBA(image.Rect(1, 0, 5, 1))},
		{"wrong width", image.NewNRGBA(image.Rect(0, 0, 3, 1))},
		{"past height", image.NewNRGBA(image.Rect(0, 0, 4, 4))},
		{"empty", image.NewNRGBA(image.Rect(0, 0, 4, 0))},
		{"short pixels", &image.NRGBA{Rect: image.Rect(0, 0, 4, 1), Stride: 16, Pix: make([]byte, 15)}},
		{"short last row", &image.NRGBA{Rect: image.Rect(0, 0, 4, 2), Stride: 16, Pix: make([]byte, 31)}},
		{"small stride", &image.NRGBA{Rect: image.Rect(0, 0, 4, 1), Stride: 15, Pix: make([]byte, 16)}},
		{"negative stride", &image.NRGBA{Rect: image.Rect(0, 0, 4, 1), Stride: -1, Pix: make([]byte, 16)}},
		{"overflow stride", &image.NRGBA{Rect: image.Rect(0, 0, 4, 3), Stride: math.MaxInt, Pix: make([]byte, 16)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stream rasterPNGBandEncoder
			defer stream.close()
			if err := stream.start(context.Background(), io.Discard, 4, 3, false); err != nil {
				t.Fatal(err)
			}
			err := stream.append(test.band)
			if err == nil {
				t.Fatal("invalid band accepted")
			}
			if got := stream.finish(); got != err {
				t.Fatalf("finish lost band error: %v, want %v", got, err)
			}
		})
	}
	t.Run("duplicate rows", func(t *testing.T) {
		var stream rasterPNGBandEncoder
		defer stream.close()
		if err := stream.start(nil, io.Discard, 4, 3, false); err != nil {
			t.Fatal(err)
		}
		band := image.NewNRGBA(image.Rect(0, 0, 4, 1))
		if err := stream.append(band); err != nil {
			t.Fatal(err)
		}
		if err := stream.append(band); err == nil {
			t.Fatal("duplicate rows accepted")
		}
	})
	t.Run("incomplete", func(t *testing.T) {
		var stream rasterPNGBandEncoder
		defer stream.close()
		if err := stream.start(nil, io.Discard, 4, 3, false); err != nil {
			t.Fatal(err)
		}
		if err := stream.finish(); err == nil {
			t.Fatal("incomplete PNG accepted")
		}
	})
}

func TestRasterPNGBandsValidateStartAndOpaquePromise(t *testing.T) {
	invalidSizes := [][2]int{{0, 1}, {1, 0}, {-1, 1}, {1, -1}, {math.MaxInt, 1}}
	if uint64(math.MaxInt) > math.MaxInt32 {
		invalidSizes = append(invalidSizes, [2]int{1, math.MaxInt})
	}
	for _, size := range invalidSizes {
		var stream rasterPNGBandEncoder
		var out bytes.Buffer
		if err := stream.start(nil, &out, size[0], size[1], false); err == nil {
			t.Fatalf("invalid size %v accepted", size)
		}
		if out.Len() != 0 {
			t.Fatal("invalid size wrote output")
		}
	}
	var stream rasterPNGBandEncoder
	defer stream.close()
	if err := stream.start(nil, nil, 1, 1, false); err == nil {
		t.Fatal("nil writer accepted")
	}
	if err := stream.append(image.NewNRGBA(image.Rect(0, 0, 1, 1))); err == nil {
		t.Fatal("append without start accepted")
	}
	if err := stream.finish(); err == nil {
		t.Fatal("finish without start accepted")
	}
	if err := stream.start(nil, io.Discard, 1, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := stream.start(nil, io.Discard, 1, 1, true); err == nil {
		t.Fatal("start on active stream accepted")
	}
	if err := stream.append(image.NewNRGBA(image.Rect(0, 0, 1, 1))); !errors.Is(err, errRasterPNGTranslucent) {
		t.Fatalf("opaque promise violation = %v", err)
	}
}

func TestRasterPNGBandsWriterErrorsAndReuse(t *testing.T) {
	img := opaqueRasterPNGTestImage(image.Rect(0, 0, 31, 7))
	var stream rasterPNGBandEncoder
	defer stream.close()
	encode := func(output io.Writer) error {
		defer stream.close()
		if err := stream.start(nil, output, 31, 7, true); err != nil {
			return err
		}
		if err := stream.append(img); err != nil {
			return err
		}
		return stream.finish()
	}
	var expected bytes.Buffer
	if err := encode(&expected); err != nil {
		t.Fatal(err)
	}
	storage := &stream.native.storage[0]
	compressor := stream.native.zw
	writerErr := errors.New("test output failed")
	for _, failure := range []error{writerErr, nil} {
		for _, limit := range []int{0, 8, 12, 30, 33, 60, expected.Len() - 12, expected.Len() - 2} {
			writer := &rasterPNGLimitedWriter{remaining: limit, err: failure}
			want := failure
			if want == nil {
				want = io.ErrShortWrite
			}
			if err := encode(writer); !errors.Is(err, want) {
				t.Fatalf("limit %d: %v, want %v", limit, err, want)
			}
			if stream.writer.ctx != nil || stream.writer.output != nil || stream.native.stream.output != nil {
				t.Fatal("closed stream retained context or output")
			}
			var out bytes.Buffer
			if err := encode(&out); err != nil {
				t.Fatalf("reuse after limit %d: %v", limit, err)
			}
			if !bytes.Equal(out.Bytes(), expected.Bytes()) {
				t.Fatalf("reuse after limit %d corrupted PNG", limit)
			}
			if storage != &stream.native.storage[0] || compressor != stream.native.zw {
				t.Fatal("stream failed to reuse its working memory")
			}
		}
	}
}

func TestRasterPNGBandsCancellation(t *testing.T) {
	for _, cancelBefore := range []string{"start", "append", "finish"} {
		t.Run(cancelBefore, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var stream rasterPNGBandEncoder
			defer stream.close()
			steps := []struct {
				name string
				run  func() error
			}{
				{"start", func() error { return stream.start(ctx, io.Discard, 3, 2, false) }},
				{"append", func() error { return stream.append(image.NewNRGBA(image.Rect(0, 0, 3, 2))) }},
				{"finish", stream.finish},
			}
			for _, step := range steps {
				if step.name == cancelBefore {
					cancel()
				}
				err := step.run()
				if step.name == cancelBefore {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("%s = %v, want cancellation", step.name, err)
					}
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	t.Run("during band", func(t *testing.T) {
		ctx := &rasterPNGCancelAfterChecksContext{cancelAt: math.MaxInt}
		var stream rasterPNGBandEncoder
		defer stream.close()
		if err := stream.start(ctx, io.Discard, 3, 100, false); err != nil {
			t.Fatal(err)
		}
		ctx.cancelAt = ctx.checks + 5
		if err := stream.append(image.NewNRGBA(image.Rect(0, 0, 3, 100))); !errors.Is(err, context.Canceled) {
			t.Fatalf("append = %v, want cancellation", err)
		}
		if stream.nextY == 0 || stream.nextY >= 100 {
			t.Fatalf("cancellation stopped at row %d, want partial progress", stream.nextY)
		}
	})
}

func TestRasterPNGBandsAllowPixelBufferReuse(t *testing.T) {
	const width, height, bandHeight = 7, 100, 3
	band := image.NewNRGBA(image.Rect(0, 0, width, bandHeight))
	var out bytes.Buffer
	var stream rasterPNGBandEncoder
	defer stream.close()
	if err := stream.start(nil, &out, width, height, false); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < height; y += bandHeight {
		band.Rect = image.Rect(0, y, width, min(height, y+bandHeight))
		for row := band.Rect.Min.Y; row < band.Rect.Max.Y; row++ {
			for x := 0; x < width; x++ {
				band.SetNRGBA(x, row, color.NRGBA{uint8(x), uint8(row), 255, 128})
			}
		}
		if err := stream.append(band); err != nil {
			t.Fatal(err)
		}
		clear(band.Pix)
	}
	if err := stream.finish(); err != nil {
		t.Fatal(err)
	}
	decoded, err := stdpng.Decode(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			want := color.NRGBA{uint8(x), uint8(y), 255, 128}
			if got := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func BenchmarkRasterPNGBands(b *testing.B) {
	const width, bandHeight = 1024, 256
	for _, height := range []int{256, 4096, 8192} {
		b.Run(fmt.Sprintf("%dx%d", width, height), func(b *testing.B) {
			band := opaqueRasterPNGTestImage(image.Rect(0, 0, width, bandHeight))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				var stream rasterPNGBandEncoder
				if err := stream.start(nil, io.Discard, width, height, true); err != nil {
					b.Fatal(err)
				}
				for y := 0; y < height; y += bandHeight {
					band.Rect = image.Rect(0, y, width, min(height, y+bandHeight))
					if err := stream.append(band); err != nil {
						b.Fatal(err)
					}
				}
				if err := stream.finish(); err != nil {
					b.Fatal(err)
				}
				stream.close()
			}
		})
	}
}

type rasterPNGLimitedWriter struct {
	remaining int
	err       error
}

func (w *rasterPNGLimitedWriter) Write(data []byte) (int, error) {
	if len(data) > w.remaining {
		n := w.remaining
		w.remaining = 0
		return n, w.err
	}
	w.remaining -= len(data)
	return len(data), nil
}
