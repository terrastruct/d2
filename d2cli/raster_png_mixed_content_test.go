package d2cli

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"math/rand"
	"testing"
)

// These fixtures exercise unchanged prefixes followed by varied content.
// Fixture generation is outside timed runs.
func rasterPNGMixedContentImages() []struct {
	name string
	img  *image.NRGBA
} {
	var cases []struct {
		name string
		img  *image.NRGBA
	}
	for _, kind := range []string{"noise", "gradient"} {
		for _, margin := range []int{8, 64} {
			img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
			random := rand.New(rand.NewSource(0xD2))
			for y := 0; y < 512; y++ {
				for x := 0; x < 512; x++ {
					offset := img.PixOffset(x, y)
					// Consume one value for every pixel so both margin widths
					// have the same underlying noise at matching coordinates.
					value := random.Uint32()
					red, green, blue := byte(value), byte(value>>8), byte(value>>16)
					if kind == "gradient" {
						// Match the existing encoder benchmark's diagonal
						// gradient: colors vary both within and between rows.
						red, green, blue = byte(x), byte(y), byte(x+y)
					}
					if x < margin {
						red, green, blue = 0xff, 0xff, 0xff
					}
					img.Pix[offset], img.Pix[offset+1], img.Pix[offset+2], img.Pix[offset+3] = red, green, blue, 0xff
				}
			}
			cases = append(cases, struct {
				name string
				img  *image.NRGBA
			}{name: fmt.Sprintf("%s/margin-%d", kind, margin), img: img})
		}
	}
	return cases
}

var benchmarkRasterPNGMixedBytes []byte

// BenchmarkRasterPNGMixedContent uses the native subbenchmark lifecycle from
// BenchmarkRasterPNGEncoder: one encoder per subbenchmark, retained across
// iterations and closed afterward. Each encode allocates its output normally.
func BenchmarkRasterPNGMixedContent(b *testing.B) {
	for _, benchmark := range rasterPNGMixedContentImages() {
		b.Run(benchmark.name+"/native", func(b *testing.B) {
			var encoder rasterPNGEncoder
			defer encoder.close()
			b.ReportAllocs()
			for range b.N {
				encoded, err := encoder.encode(context.Background(), benchmark.img)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkRasterPNGMixedBytes = encoded
			}
		})
	}
}

func TestRasterPNGMixedContentMatchesStdlib(t *testing.T) {
	var encoder rasterPNGEncoder
	defer encoder.close()
	for _, test := range rasterPNGMixedContentImages() {
		t.Run(test.name, func(t *testing.T) {
			before := bytes.Clone(test.img.Pix)
			var expected bytes.Buffer
			stdlib := png.Encoder{CompressionLevel: png.BestSpeed}
			if err := stdlib.Encode(&expected, test.img); err != nil {
				t.Fatal(err)
			}
			// Both inputs are opaque NRGBA, so native encoding must emit
			// the same truecolor PNG bytes and filter choices as image/png.
			// Repeat to check retained encoder state as well as first use.
			for pass := 0; pass < 2; pass++ {
				got, err := encoder.encode(context.Background(), test.img)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, expected.Bytes()) {
					t.Fatalf("pass %d: native PNG differs from image/png BestSpeed", pass)
				}
				if !bytes.Equal(test.img.Pix, before) {
					t.Fatalf("pass %d: encoding mutated source pixels", pass)
				}
			}
		})
	}
}
