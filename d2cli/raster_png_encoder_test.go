package d2cli

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/util-go/cmdlog"
	"github.com/d2lang/util-go/go2"
	"github.com/d2lang/util-go/xmain"
	"github.com/d2lang/util-go/xos"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
)

func TestRasterPNGEncoderMatchesStatelessEncoder(t *testing.T) {
	opaque := image.NewNRGBA(image.Rect(0, 0, 19, 11))
	for index := 0; index < len(opaque.Pix); index += 4 {
		copy(opaque.Pix[index:index+4], []byte{0x31, 0x72, 0xb4, 0xff})
	}
	storage := image.NewNRGBA(image.Rect(-3, -2, 37, 29))
	for y := storage.Rect.Min.Y; y < storage.Rect.Max.Y; y++ {
		for x := storage.Rect.Min.X; x < storage.Rect.Max.X; x++ {
			storage.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x*31 + y*17), G: uint8(x*7 + y*13), B: uint8(x ^ y), A: uint8(64 + (x-y)&191),
			})
		}
	}
	inputs := []image.Image{
		image.NewNRGBA(image.Rect(0, 0, 1, 1)),
		opaque,
		storage,
		storage.SubImage(image.Rect(2, 3, 29, 23)),
		image.NewPaletted(image.Rect(0, 0, 13, 7), color.Palette{color.Black, color.White}),
		image.NewNRGBA(image.Rect(0, 0, 8, 3)),
	}
	var encoder rasterPNGEncoder
	defer encoder.close()
	for index, input := range inputs {
		want, err := d2raster.EncodePNG(context.Background(), input)
		if err != nil {
			t.Fatalf("input %d stateless encode: %v", index, err)
		}
		got, err := encoder.encode(context.Background(), input)
		if err != nil {
			t.Fatalf("input %d pooled encode: %v", index, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("input %d pooled PNG differs from stateless PNG", index)
		}
	}
}

func TestRasterPNGEncoderMatchesStdlibForOpaqueRandomImages(t *testing.T) {
	random := rand.New(rand.NewSource(0xD2))
	var encoder rasterPNGEncoder
	defer encoder.close()
	dimensions := [][2]int{{1, 1}, {2, 3}, {3, 2}, {7, 5}, {31, 17}, {64, 65}, {127, 33}, {257, 129}, {512, 257}}
	for caseIndex := 0; caseIndex < 40; caseIndex++ {
		dimensionsIndex := caseIndex % len(dimensions)
		width, height := dimensions[dimensionsIndex][0], dimensions[dimensionsIndex][1]
		outer := image.Rect(-7, -5, width+19, height+17)
		storage := image.NewNRGBA(outer)
		bounds := image.Rect(-3, 2, -3+width, 2+height)
		img := storage.SubImage(bounds).(*image.NRGBA)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				offset := img.PixOffset(x, y)
				value := random.Uint32()
				img.Pix[offset], img.Pix[offset+1], img.Pix[offset+2], img.Pix[offset+3] = byte(value), byte(value>>8), byte(value>>16), 0xff
			}
		}
		want, err := d2raster.EncodePNG(context.Background(), img)
		if err != nil {
			t.Fatalf("case %d stdlib encode: %v", caseIndex, err)
		}
		got, err := encoder.encode(context.Background(), img)
		if err != nil {
			t.Fatalf("case %d native encode: %v", caseIndex, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("case %d bounds %v native PNG differs from stdlib", caseIndex, bounds)
		}
	}
}

func TestRasterPNGAbs8(t *testing.T) {
	for value := 0; value < 256; value++ {
		want := value
		if want >= 128 {
			want = 256 - want
		}
		if got := rasterPNGAbs8(byte(value)); got != want {
			t.Fatalf("abs8(%d) = %d, want %d", value, got, want)
		}
	}
}

func TestRasterPNGEncoderFallsBackExactlyForTranslucentNRGBA(t *testing.T) {
	base := opaqueRasterPNGTestImage(image.Rect(-3, 2, 254, 131))
	for y := base.Rect.Min.Y; y < base.Rect.Max.Y; y++ {
		for x := base.Rect.Min.X; x < base.Rect.Max.X; x++ {
			offset := base.PixOffset(x, y)
			base.Pix[offset], base.Pix[offset+1], base.Pix[offset+2] = byte(x*31+y*17), byte(x*7+y*13), byte(x^y)
		}
	}
	for _, test := range []struct {
		name  string
		point image.Point
		alpha byte
	}{
		{name: "unrolled lane 0", point: image.Pt(-3, 2), alpha: 0xfe},
		{name: "unrolled lane 1", point: image.Pt(-2, 2), alpha: 0x7f},
		{name: "unrolled lane 2", point: image.Pt(-1, 2), alpha: 0x00},
		{name: "unrolled lane 3", point: image.Pt(0, 2), alpha: 0x01},
		{name: "later unrolled block", point: image.Pt(17, 19), alpha: 0x80},
		// This position is handled by the scalar tail after nearly the entire
		// specialized attempt has already been compressed.
		{name: "final scalar pixel", point: image.Pt(253, 130), alpha: 0x7f},
	} {
		t.Run(test.name, func(t *testing.T) {
			img := &image.NRGBA{Pix: bytes.Clone(base.Pix), Stride: base.Stride, Rect: base.Rect}
			img.Pix[img.PixOffset(test.point.X, test.point.Y)+3] = test.alpha
			want, err := d2raster.EncodePNG(context.Background(), img)
			if err != nil {
				t.Fatal(err)
			}
			var encoder rasterPNGEncoder
			got, err := encoder.encode(context.Background(), img)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("translucent fallback PNG differs from generic encoder")
			}
			if encoder.native.storage != nil || encoder.native.zw != nil || encoder.native.bw != nil || encoder.native.stream.output != nil {
				t.Fatal("translucent fallback retained abandoned specialized state")
			}
		})
	}
}

func TestRasterPNGEncoderLeavesOtherImageTypesOnGenericPath(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(-3, 2, 29, 21))
	for index := 0; index < len(rgba.Pix); index += 4 {
		rgba.Pix[index], rgba.Pix[index+1], rgba.Pix[index+2], rgba.Pix[index+3] = byte(index), byte(index>>8), byte(index>>16), 0xff
	}
	inputs := []image.Image{
		rgba,
		image.NewGray(image.Rect(0, 0, 23, 17)),
		image.NewPaletted(image.Rect(0, 0, 13, 7), color.Palette{color.Black, color.White}),
	}
	for index, img := range inputs {
		want, err := d2raster.EncodePNG(context.Background(), img)
		if err != nil {
			t.Fatalf("input %d generic encode: %v", index, err)
		}
		var encoder rasterPNGEncoder
		if _, err := encoder.encode(context.Background(), opaqueRasterPNGTestImage(image.Rect(0, 0, 2, 2))); err != nil {
			t.Fatal(err)
		}
		got, err := encoder.encode(context.Background(), img)
		if err != nil {
			t.Fatalf("input %d fallback encode: %v", index, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("input %d fallback PNG differs from generic encoder", index)
		}
		if encoder.native.storage != nil || encoder.native.zw != nil || encoder.native.bw != nil || encoder.native.stream.output != nil {
			t.Fatalf("input %d generic fallback retained specialized state", index)
		}
	}
}

func TestRasterPNGEncoderGenericPoolReuseCancellationAndClose(t *testing.T) {
	translucent := image.NewNRGBA(image.Rect(0, 0, 257, 129))
	for y := range translucent.Bounds().Dy() {
		for x := range translucent.Bounds().Dx() {
			translucent.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x*31 + y*17), G: uint8(x*7 + y*13), B: uint8(x ^ y), A: uint8(64 + (x-y)&191),
			})
		}
	}
	want, err := d2raster.EncodePNG(context.Background(), translucent)
	if err != nil {
		t.Fatal(err)
	}
	var encoder rasterPNGEncoder
	got, err := encoder.encode(context.Background(), translucent)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generic pooled PNG differs from stateless output")
	}
	firstBuffer := encoder.generic.buffer
	if firstBuffer == nil {
		t.Fatal("generic encode retained no reusable EncoderBuffer")
	}
	if encoder.genericWriter.ctx != nil || encoder.genericWriter.output != nil || encoder.genericImage.Pix != nil {
		t.Fatal("generic pool retained context, encoded output, or source frame")
	}

	got, err = encoder.encode(context.Background(), translucent)
	if err != nil {
		t.Fatal(err)
	}
	if encoder.generic.buffer != firstBuffer || !bytes.Equal(got, want) {
		t.Fatal("second generic encode did not reuse its exact-output buffer")
	}

	canceled := &rasterPNGCancelAfterChecksContext{cancelAt: 3}
	if got, err := encoder.encodeGeneric(canceled, translucent); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("canceled generic encode = %d bytes, %v", len(got), err)
	}
	if encoder.generic.buffer != firstBuffer || encoder.genericWriter.ctx != nil || encoder.genericWriter.output != nil || encoder.genericImage.Pix != nil {
		t.Fatal("canceled generic encode poisoned its pool or retained call state")
	}
	got, err = encoder.encode(context.Background(), translucent)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("generic encode after cancellation = %d bytes, %v", len(got), err)
	}

	encoder.close()
	if encoder.generic.buffer != nil || encoder.genericWriter.ctx != nil || encoder.genericWriter.output != nil || encoder.genericImage.Pix != nil {
		t.Fatal("close retained generic encoder state")
	}
}

func TestRasterPNGEncoderRetainsOnlyOperationWorkspace(t *testing.T) {
	var first, second rasterPNGEncoder
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for index := 3; index < len(img.Pix); index += 4 {
		img.Pix[index] = 0xff
	}
	if _, err := first.encode(context.Background(), img); err != nil {
		t.Fatal(err)
	}
	if first.native.storage == nil || first.native.zw == nil || first.native.bw == nil {
		t.Fatal("first encoder retained no reusable workspace")
	}
	storage := &first.native.storage[0]
	zw, bw := first.native.zw, first.native.bw
	if first.native.stream.output != nil {
		t.Fatal("encoder retained output state")
	}
	if _, err := first.encode(context.Background(), img); err != nil {
		t.Fatal(err)
	}
	if &first.native.storage[0] != storage || first.native.zw != zw || first.native.bw != bw {
		t.Fatal("encoder did not reuse its retained workspace")
	}

	if _, err := second.encode(context.Background(), img); err != nil {
		t.Fatal(err)
	}
	if second.native.storage == nil || &second.native.storage[0] == &first.native.storage[0] || second.native.zw == first.native.zw {
		t.Fatal("separate export encoders shared retained state")
	}
	first.close()
	if first.native.storage != nil || first.native.zw != nil || first.native.bw != nil {
		t.Fatal("close retained the first export workspace")
	}
	if second.native.storage == nil {
		t.Fatal("closing the first export changed the second export")
	}
	second.close()
}

func TestRasterPNGEncoderCancellationDoesNotPoisonReuse(t *testing.T) {
	var encoder rasterPNGEncoder
	if _, err := encoder.encode(context.Background(), opaqueRasterPNGTestImage(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if encoder.native.storage == nil {
		t.Fatal("setup encode retained no workspace")
	}
	ctx := &rasterPNGCancelAfterChecksContext{cancelAt: 3}
	encoded, err := encoder.encode(ctx, opaqueRasterPNGTestImage(image.Rect(0, 0, 4, 4)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled encode error = %v, want context.Canceled", err)
	}
	if encoded != nil {
		t.Fatalf("canceled encode returned %d bytes", len(encoded))
	}
	if encoder.native.storage != nil || encoder.native.zw != nil || encoder.native.bw != nil || encoder.native.stream.output != nil {
		t.Fatal("canceled encode retained poisoned state")
	}

	next := opaqueRasterPNGTestImage(image.Rect(0, 0, 7, 5))
	want, err := d2raster.EncodePNG(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	got, err := encoder.encode(context.Background(), next)
	if err != nil {
		t.Fatalf("encode after cancellation: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("encode after cancellation differs from stateless output")
	}
	encoder.close()
}

func TestRasterPNGEncoderErrorDoesNotPoisonReuse(t *testing.T) {
	var encoder rasterPNGEncoder
	if _, err := encoder.encode(context.Background(), opaqueRasterPNGTestImage(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if encoder.native.storage == nil {
		t.Fatal("setup encode retained no workspace")
	}
	encoded, err := encoder.encode(context.Background(), image.NewNRGBA(image.Rect(0, 0, 4, 0)))
	if err == nil || !strings.Contains(err.Error(), "invalid image size") {
		t.Fatalf("invalid image error = %v", err)
	}
	if encoded != nil {
		t.Fatalf("invalid image returned %d bytes", len(encoded))
	}
	if encoder.native.storage != nil || encoder.native.zw != nil || encoder.native.bw != nil || encoder.native.stream.output != nil {
		t.Fatal("failed encode retained poisoned state")
	}

	next := opaqueRasterPNGTestImage(image.Rect(0, 0, 7, 5))
	want, err := d2raster.EncodePNG(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	got, err := encoder.encode(context.Background(), next)
	if err != nil {
		t.Fatalf("encode after error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("encode after error differs from stateless output")
	}
	encoder.close()
}

func TestRasterPNGEncoderDoesNotRetainOversizeRows(t *testing.T) {
	var encoder rasterPNGEncoder
	if _, err := encoder.encode(context.Background(), opaqueRasterPNGTestImage(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if encoder.native.storage == nil {
		t.Fatal("setup encode retained no workspace")
	}
	oversize := image.NewNRGBA(image.Rect(0, 0, rasterMaxDimension+1, 1))
	want, err := d2raster.EncodePNG(context.Background(), oversize)
	if err != nil {
		t.Fatal(err)
	}
	got, err := encoder.encode(context.Background(), oversize)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("oversize unpooled output differs from stateless output")
	}
	if encoder.native.storage != nil || encoder.native.zw != nil || encoder.native.bw != nil {
		t.Fatal("encoder retained a row buffer beyond the paged dimension limit")
	}
}

func TestRenderPNGWithEncoderMatchesStateless(t *testing.T) {
	opts := d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)}
	diagrams := []*d2target.Diagram{
		simpleRasterDiagramWithLabel(),
		simpleRasterDiagramWithSize(47, 31),
	}
	var encoder rasterPNGEncoder
	defer encoder.close()
	for index, diagram := range diagrams {
		want, err := renderPNGWithEncoder(context.Background(), "-", false, diagram, opts, nil)
		if err != nil {
			t.Fatalf("diagram %d stateless render: %v", index, err)
		}
		got, err := renderPNGWithEncoder(context.Background(), "-", false, diagram, opts, &encoder)
		if err != nil {
			t.Fatalf("diagram %d operation-scoped render: %v", index, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("diagram %d operation-scoped PNG differs from stateless PNG", index)
		}
	}
	if encoder.native.storage == nil || encoder.native.zw == nil || encoder.native.bw == nil {
		t.Fatal("multi-board render path retained no reusable encoder workspace")
	}
	if encoder.native.stream.output != nil {
		t.Fatal("multi-board render path retained its last output")
	}
	encoder.close()
	if encoder.native.storage != nil || encoder.native.zw != nil || encoder.native.bw != nil || encoder.native.stream.output != nil {
		t.Fatal("completed multi-board render retained operation state")
	}
}

func TestFolderPNGExportMatchesStatelessOutput(t *testing.T) {
	newFolder := func() *d2target.Diagram {
		first := simpleRasterDiagramWithLabel()
		first.Name = "one"
		second := simpleRasterDiagramWithSize(47, 31)
		second.Name = "two"
		return &d2target.Diagram{IsFolderOnly: true, Layers: []*d2target.Diagram{first, second}}
	}
	renderFolder := func(directory string, encoder *rasterPNGEncoder, productionPath bool) {
		t.Helper()
		env := xos.NewEnv(nil)
		state := &xmain.State{Env: env, Log: cmdlog.NewTB(env, t), PWD: directory}
		ctx := context.Background()
		opts := d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)}
		var written bool
		var err error
		if productionPath {
			_, written, err = render(
				ctx, state, 0, nil, opts, "input.d2", filepath.Join(directory, "output.png"),
				false, false, nil, newFolder(), PNG, "", false,
			)
		} else {
			_, written, err = renderWithPNGEncoder(
				ctx, state, 0, nil, opts, "input.d2", filepath.Join(directory, "output.png"),
				false, false, nil, newFolder(), PNG, "", false, encoder,
			)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !written {
			t.Fatal("folder render reported no written board")
		}
	}

	pooledDirectory := t.TempDir()
	var encoder rasterPNGEncoder
	renderFolder(pooledDirectory, &encoder, false)
	if encoder.native.storage == nil || encoder.native.zw == nil || encoder.native.bw == nil {
		t.Fatal("folder render retained no reusable workspace between boards")
	}
	statelessDirectory := t.TempDir()
	renderFolder(statelessDirectory, nil, false)
	productionDirectory := t.TempDir()
	renderFolder(productionDirectory, nil, true)

	for _, name := range []string{"one.png", "two.png"} {
		pooled, err := os.ReadFile(filepath.Join(pooledDirectory, "output", name))
		if err != nil {
			t.Fatal(err)
		}
		stateless, err := os.ReadFile(filepath.Join(statelessDirectory, "output", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(pooled, stateless) {
			t.Fatalf("pooled folder output %s differs from stateless output", name)
		}
		production, err := os.ReadFile(filepath.Join(productionDirectory, "output", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(production, stateless) {
			t.Fatalf("production folder output %s differs from stateless output", name)
		}
	}
	encoder.close()
	if encoder.native.storage != nil || encoder.native.zw != nil || encoder.native.bw != nil || encoder.native.stream.output != nil {
		t.Fatal("completed folder export retained operation state")
	}
}

type rasterPNGCancelAfterChecksContext struct {
	checks   int
	cancelAt int
}

func opaqueRasterPNGTestImage(bounds image.Rectangle) *image.NRGBA {
	img := image.NewNRGBA(bounds)
	for index := 3; index < len(img.Pix); index += 4 {
		img.Pix[index] = 0xff
	}
	return img
}

func (*rasterPNGCancelAfterChecksContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*rasterPNGCancelAfterChecksContext) Done() <-chan struct{}       { return nil }
func (*rasterPNGCancelAfterChecksContext) Value(any) any               { return nil }

func (c *rasterPNGCancelAfterChecksContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

var (
	benchmarkRasterPNG      []byte
	benchmarkRasterPNGBytes int
)

func BenchmarkRasterPNGEncoder(b *testing.B) {
	for _, benchmark := range rasterPNGEncoderBenchmarks() {
		b.Run(benchmark.name+"/stdlib", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				encoded, err := d2raster.EncodePNG(context.Background(), benchmark.img)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkRasterPNG = encoded
			}
		})
		b.Run(benchmark.name+"/native", func(b *testing.B) {
			var encoder rasterPNGEncoder
			defer encoder.close()
			b.ReportAllocs()
			for range b.N {
				encoded, err := encoder.encode(context.Background(), benchmark.img)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkRasterPNG = encoded
			}
		})
	}
}

func BenchmarkRasterPNGEncoderGenericPool(b *testing.B) {
	img := rasterPNGEncoderBenchmarks()[4].img
	b.Run("stateless", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			encoded, err := d2raster.EncodePNG(context.Background(), img)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkRasterPNG = encoded
		}
	})
	b.Run("operation-scoped", func(b *testing.B) {
		var encoder rasterPNGEncoder
		defer encoder.close()
		b.ReportAllocs()
		for range b.N {
			encoded, err := encoder.encodeGeneric(context.Background(), img)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkRasterPNG = encoded
		}
	})
}

// BenchmarkRasterPNGEncoderSingleBoardOperation includes encoder construction
// and teardown in every iteration. It protects the common one-board CLI export
// from being traded off for the workspace reuse benefits measured above.
func BenchmarkRasterPNGEncoderSingleBoardOperation(b *testing.B) {
	for _, benchmark := range rasterPNGEncoderBenchmarks() {
		b.Run(benchmark.name+"/stdlib", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				encoded, err := d2raster.EncodePNG(context.Background(), benchmark.img)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkRasterPNG = encoded
			}
		})
		b.Run(benchmark.name+"/native", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				var encoder rasterPNGEncoder
				encoded, err := encoder.encode(context.Background(), benchmark.img)
				if err != nil {
					b.Fatal(err)
				}
				encoder.close()
				benchmarkRasterPNG = encoded
			}
		})
	}
}

type rasterPNGEncoderBenchmark struct {
	name string
	img  *image.NRGBA
}

func rasterPNGEncoderBenchmarks() []rasterPNGEncoderBenchmark {
	flat1 := opaqueRasterPNGTestImage(image.Rect(0, 0, 1, 1))
	flat512 := opaqueRasterPNGTestImage(image.Rect(0, 0, 512, 512))
	gradient512 := opaqueRasterPNGTestImage(image.Rect(0, 0, 512, 512))
	for y := range gradient512.Bounds().Dy() {
		for x := range gradient512.Bounds().Dx() {
			offset := gradient512.PixOffset(x, y)
			gradient512.Pix[offset], gradient512.Pix[offset+1], gradient512.Pix[offset+2] = byte(x), byte(y), byte(x+y)
		}
	}
	noisy512 := opaqueRasterPNGTestImage(image.Rect(0, 0, 512, 512))
	random := rand.New(rand.NewSource(0xD2))
	for offset := 0; offset < len(noisy512.Pix); offset += 4 {
		value := random.Uint32()
		noisy512.Pix[offset], noisy512.Pix[offset+1], noisy512.Pix[offset+2] = byte(value), byte(value>>8), byte(value>>16)
	}
	translucent512 := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := range translucent512.Bounds().Dy() {
		for x := range translucent512.Bounds().Dx() {
			offset := translucent512.PixOffset(x, y)
			translucent512.Pix[offset], translucent512.Pix[offset+1], translucent512.Pix[offset+2], translucent512.Pix[offset+3] =
				byte(x), byte(y), byte(x+y), byte(64+(x-y)&191)
		}
	}
	flat2048 := opaqueRasterPNGTestImage(image.Rect(0, 0, 2048, 2048))
	return []rasterPNGEncoderBenchmark{
		{name: "tiny-flat", img: flat1},
		{name: "medium-flat", img: flat512},
		{name: "medium-gradient", img: gradient512},
		{name: "medium-noisy", img: noisy512},
		{name: "medium-translucent", img: translucent512},
		{name: "large-flat", img: flat2048},
	}
}

func BenchmarkRasterPNGEncoderTwoBoardOperation(b *testing.B) {
	img := opaqueRasterPNGTestImage(image.Rect(0, 0, 512, 512))
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 0xff})
		}
	}
	b.Run("stateless", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			first, err := d2raster.EncodePNG(context.Background(), img)
			if err != nil {
				b.Fatal(err)
			}
			second, err := d2raster.EncodePNG(context.Background(), img)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkRasterPNGBytes = len(first) + len(second)
		}
	})
	b.Run("operation-scoped", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			var encoder rasterPNGEncoder
			first, err := encoder.encode(context.Background(), img)
			if err != nil {
				b.Fatal(err)
			}
			second, err := encoder.encode(context.Background(), img)
			if err != nil {
				b.Fatal(err)
			}
			encoder.close()
			benchmarkRasterPNGBytes = len(first) + len(second)
		}
	})
}
