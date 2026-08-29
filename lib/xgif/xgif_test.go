package xgif

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

//go:embed test_output.gif
var test_output []byte

func TestValidateCompatibility(t *testing.T) {
	t.Parallel()

	anim, err := gif.DecodeAll(bytes.NewReader(test_output))
	assert.NoError(t, err)
	assert.NoError(t, Validate(test_output, len(anim.Image), anim.Delay[0]*10))
}

func TestAnimationFrameCountRoundsUpFractionalSeconds(t *testing.T) {
	tests := []struct {
		interval int
		want     int
	}{
		{interval: 1, want: 1},
		{interval: 500, want: 15},
		{interval: 999, want: 30},
		{interval: 1000, want: 30},
		{interval: 1001, want: 31},
		{interval: 1400, want: 42},
	}
	for _, test := range tests {
		got, err := animationFrameCount(test.interval)
		if err != nil || got != test.want {
			t.Fatalf("animationFrameCount(%d) = %d/%v, want %d/nil", test.interval, got, err, test.want)
		}
	}
	if _, err := animationFrameCount(0); err == nil {
		t.Fatal("animationFrameCount accepted zero interval")
	}
}

func TestExportedAnimationSamplingContract(t *testing.T) {
	count, err := AnimationFrameCount(1_001)
	if err != nil || count != 31 {
		t.Fatalf("AnimationFrameCount(1001) = %d/%v, want 31/nil", count, err)
	}
	for _, test := range []struct {
		index int
		want  time.Duration
	}{
		{index: 0, want: 0},
		{index: 1, want: time.Second / 30},
		{index: 29, want: 29 * time.Second / 30},
		{index: 30, want: time.Second},
	} {
		got, err := AnimationFrameTime(test.index)
		if err != nil || got != test.want {
			t.Fatalf("AnimationFrameTime(%d) = %s/%v, want %s/nil", test.index, got, err, test.want)
		}
	}
	if _, err := AnimationFrameTime(-1); err == nil {
		t.Fatal("AnimationFrameTime accepted a negative index")
	}
}

func TestGIFFrameDelaysPreserveQuantizedBoardDuration(t *testing.T) {
	for _, test := range []struct {
		frames, interval, boards, wantCentiseconds int
	}{
		{frames: 30, interval: 1000, boards: 1, wantCentiseconds: 100},
		{frames: 84, interval: 1400, boards: 2, wantCentiseconds: 280},
		{frames: 15, interval: 500, boards: 1, wantCentiseconds: 50},
		// A non-sampled public-API call remains one board.
		{frames: 2, interval: 1000, boards: 1, wantCentiseconds: 100},
	} {
		delays, err := gifFrameDelays(test.frames, test.interval)
		if err != nil {
			t.Fatal(err)
		}
		if len(delays) != test.frames || sumDelays(delays) != test.wantCentiseconds {
			t.Fatalf("delays(%d,%d) = %v sum %d, want %d frames sum %d", test.frames, test.interval, delays, sumDelays(delays), test.frames, test.wantCentiseconds)
		}
		framesPerBoard := test.frames / test.boards
		for board := 0; board < test.boards; board++ {
			start := board * framesPerBoard
			if got, want := sumDelays(delays[start:start+framesPerBoard]), test.wantCentiseconds/test.boards; got != want {
				t.Fatalf("board %d delay sum = %d, want %d", board, got, want)
			}
		}
		for _, delay := range delays {
			if delay < 0 {
				t.Fatalf("negative frame delay in %v", delays)
			}
		}
	}
	delays, err := gifFrameDelays(30, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range []int{3, 3, 4, 3, 3, 4} {
		if delays[index] != want {
			t.Fatalf("distributed delays start %v, want 3,3,4 repeating", delays[:6])
		}
	}
}

func TestAnimateImagesPreservesNonZeroSourceBounds(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}
	small := image.NewNRGBA(image.Rect(5, 7, 7, 9))
	fillNRGBA(small, red)
	large := image.NewNRGBA(image.Rect(-2, -3, 2, 1))
	fillNRGBA(large, blue)

	encoded, err := animateImagesWithConcurrency(context.Background(), []image.Image{small, large}, 1000, workers)
	if err != nil {
		t.Fatal(err)
	}
	anim, err := gif.DecodeAll(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(anim.Image[0].At(1, 1)).(color.NRGBA); got != red {
		t.Fatalf("centered non-zero-bounds source pixel = %#v, want %#v", got, red)
	}
	if got := color.NRGBAModel.Convert(anim.Image[1].At(0, 0)).(color.NRGBA); got != blue {
		t.Fatalf("full-size non-zero-bounds source pixel = %#v, want %#v", got, blue)
	}
}

func TestAnimateImagesExplicitConcurrencyPreservesOutput(t *testing.T) {
	red := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	blue := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	fillNRGBA(red, color.NRGBA{R: 255, A: 255})
	fillNRGBA(blue, color.NRGBA{B: 255, A: 255})
	frames := []image.Image{red, blue}
	defaultEncoded, err := animateImagesWithConcurrency(context.Background(), frames, 1000, workers)
	if err != nil {
		t.Fatal(err)
	}
	serialEncoded, err := animateImagesWithConcurrency(context.Background(), frames, 1000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(defaultEncoded, serialEncoded) {
		t.Fatal("explicit serial quantization changed deterministic GIF bytes")
	}
}

func TestIncrementalPalettedPipelineMatchesAnimateImages(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}
	small := image.NewNRGBA(image.Rect(5, 7, 7, 9))
	fillNRGBA(small, red)
	large := image.NewNRGBA(image.Rect(-2, -3, 2, 1))
	fillNRGBA(large, blue)
	images := []image.Image{small, large}

	want, err := animateImagesWithConcurrency(context.Background(), images, 1000, 1)
	if err != nil {
		t.Fatal(err)
	}
	frames := make([]*image.Paletted, len(images))
	for index, img := range images {
		quantized, err := QuantizeImage(context.Background(), img)
		if err != nil {
			t.Fatalf("quantize frame %d: %v", index, err)
		}
		frames[index], err = NormalizePalettedImage(context.Background(), quantized, 4, 4)
		if err != nil {
			t.Fatalf("normalize frame %d: %v", index, err)
		}
	}
	got, err := animatePalettedImages(context.Background(), frames, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("incremental paletted pipeline changed deterministic GIF bytes")
	}
}

func TestQuantizeImageValidationAndCancellation(t *testing.T) {
	if _, err := QuantizeImage(context.Background(), nil); err == nil {
		t.Fatal("QuantizeImage accepted a nil image")
	}
	var typedNil *image.NRGBA
	if _, err := QuantizeImage(context.Background(), typedNil); err == nil {
		t.Fatal("QuantizeImage accepted a typed nil image")
	}
	if _, err := QuantizeImage(context.Background(), image.NewNRGBA(image.Rectangle{})); err == nil {
		t.Fatal("QuantizeImage accepted an empty image")
	}
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	invalidBounds := []image.Rectangle{
		{Min: image.Pt(minInt, 0), Max: image.Pt(maxInt, 1)},
		image.Rect(0, 0, 1<<16, 1),
	}
	for index, bounds := range invalidBounds {
		if _, err := QuantizeImage(context.Background(), boundsOnlyImage{bounds: bounds}); err == nil {
			t.Fatalf("QuantizeImage accepted invalid bounds %d: %v", index, bounds)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := QuantizeImage(ctx, image.NewNRGBA(image.Rect(0, 0, 1, 1))); !errors.Is(err, context.Canceled) {
		t.Fatalf("QuantizeImage cancellation = %v, want context.Canceled", err)
	}
	if _, err := QuantizeImage(nil, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("QuantizeImage nil context = %v, want nil", err)
	}
}

func TestNormalizePalettedImageCentersAndOwnsExpandedFrame(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	source := &image.Paletted{
		Pix:     []uint8{0, 0, 0, 0},
		Stride:  2,
		Rect:    image.Rect(5, 7, 7, 9),
		Palette: color.Palette{red},
	}
	normalized, err := NormalizePalettedImage(context.Background(), source, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	if normalized == source {
		t.Fatal("expanded normalization reused the source frame")
	}
	if normalized.Bounds() != image.Rect(0, 0, 4, 4) {
		t.Fatalf("normalized bounds = %v, want (0,0)-(4,4)", normalized.Bounds())
	}
	if got := color.NRGBAModel.Convert(normalized.At(1, 1)).(color.NRGBA); got != red {
		t.Fatalf("centered source pixel = %#v, want %#v", got, red)
	}
	if got := color.NRGBAModel.Convert(normalized.At(0, 0)).(color.NRGBA); got != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("normalized background = %#v, want white", got)
	}
	if len(source.Palette) != 1 {
		t.Fatalf("expanded normalization mutated source palette length to %d", len(source.Palette))
	}
}

func TestNormalizePalettedImageReusesMatchingFrame(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black})
	pixel := &frame.Pix[0]
	normalized, err := NormalizePalettedImage(context.Background(), frame, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if normalized != frame || &normalized.Pix[0] != pixel {
		t.Fatal("matching normalization allocated a second frame")
	}
	if len(normalized.Palette) != 2 || normalized.Palette[1] != BG_COLOR {
		t.Fatalf("normalized palette = %#v, want source color plus background", normalized.Palette)
	}
}

func TestNormalizePalettedImageValidationAndCancellation(t *testing.T) {
	valid := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NormalizePalettedImage(ctx, valid, 1, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("NormalizePalettedImage cancellation = %v, want context.Canceled", err)
	}
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	invalid := []*image.Paletted{
		nil,
		image.NewPaletted(image.Rectangle{}, color.Palette{color.Black}),
		{Pix: []byte{0}, Stride: 1, Rect: image.Rectangle{Min: image.Pt(minInt, 0), Max: image.Pt(maxInt, 1)}, Palette: color.Palette{color.Black}},
		{Pix: []byte{0}, Stride: 1, Rect: image.Rect(0, 0, 1, 1)},
		{Pix: []byte{0}, Stride: 1, Rect: image.Rect(0, 0, 1, 1), Palette: color.Palette{nil}},
		{Pix: nil, Stride: 1, Rect: image.Rect(0, 0, 1, 1), Palette: color.Palette{color.Black}},
		{Pix: []byte{1}, Stride: 1, Rect: image.Rect(0, 0, 1, 1), Palette: color.Palette{color.Black}},
	}
	for index, frame := range invalid {
		if _, err := NormalizePalettedImage(context.Background(), frame, 1, 1); err == nil {
			t.Fatalf("NormalizePalettedImage accepted invalid frame %d", index)
		}
	}
	if _, err := NormalizePalettedImage(context.Background(), valid, 0, 1); err == nil {
		t.Fatal("NormalizePalettedImage accepted a zero-width canvas")
	}
	if _, err := NormalizePalettedImage(context.Background(), valid, 0xffff+1, 1); err == nil {
		t.Fatal("NormalizePalettedImage accepted a GIF canvas wider than 65535")
	}
	if _, err := NormalizePalettedImage(context.Background(), valid, 1, 0xffff+1); err == nil {
		t.Fatal("NormalizePalettedImage accepted a GIF canvas taller than 65535")
	}
	if _, err := NormalizePalettedImage(context.Background(), image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.Black}), 1, 1); err == nil {
		t.Fatal("NormalizePalettedImage accepted a source wider than its canvas")
	}
}

func TestAnimatePalettedImagesValidationAndCancellation(t *testing.T) {
	valid := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	encoded, err := animatePalettedImages(nil, []*image.Paletted{valid, valid}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	animation, err := gif.DecodeAll(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(animation.Image) != 2 || sumDelays(animation.Delay) != 100 || animation.LoopCount != INFINITE_LOOP {
		t.Fatalf("paletted animation frames/delays/loop = %d/%v/%d", len(animation.Image), animation.Delay, animation.LoopCount)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := animatePalettedImages(ctx, []*image.Paletted{valid}, 1000); !errors.Is(err, context.Canceled) {
		t.Fatalf("AnimatePalettedImages cancellation = %v, want context.Canceled", err)
	}
	if _, err := animatePalettedImages(context.Background(), nil, 1000); err == nil {
		t.Fatal("AnimatePalettedImages accepted no frames")
	}
	if _, err := animatePalettedImages(context.Background(), []*image.Paletted{valid}, 0); err == nil {
		t.Fatal("AnimatePalettedImages accepted a zero interval")
	}
	if _, err := animatePalettedImages(context.Background(), []*image.Paletted{nil}, 1000); err == nil {
		t.Fatal("AnimatePalettedImages accepted a nil frame")
	}
	nonZeroBounds := &image.Paletted{Pix: []byte{0}, Stride: 1, Rect: image.Rect(1, 1, 2, 2), Palette: color.Palette{color.Black}}
	if _, err := animatePalettedImages(context.Background(), []*image.Paletted{nonZeroBounds}, 1000); err == nil {
		t.Fatal("AnimatePalettedImages accepted non-zero frame bounds")
	}
	larger := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.Black})
	if _, err := animatePalettedImages(context.Background(), []*image.Paletted{valid, larger}, 1000); err == nil {
		t.Fatal("AnimatePalettedImages accepted mismatched frame dimensions")
	}
}

func TestAnimatePalettedImagesOutputLimit(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	want, err := animatePalettedImages(context.Background(), []*image.Paletted{frame, frame}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	exact, err := AnimatePalettedImagesWithLimit(context.Background(), []*image.Paletted{frame, frame}, 1000, int64(len(want)))
	if err != nil {
		t.Fatalf("exact output limit rejected: %v", err)
	}
	if !bytes.Equal(exact, want) {
		t.Fatal("bounded paletted encoder changed deterministic GIF bytes")
	}
	if _, err := AnimatePalettedImagesWithLimit(context.Background(), []*image.Paletted{frame, frame}, 1000, int64(len(want)-1)); err == nil {
		t.Fatal("paletted encoder exceeded its output limit")
	}
	for _, limit := range []int64{-1, 0} {
		if _, err := AnimatePalettedImagesWithLimit(context.Background(), []*image.Paletted{frame}, 1000, limit); err == nil {
			t.Fatalf("paletted encoder accepted output limit %d", limit)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := AnimatePalettedImagesWithLimit(ctx, []*image.Paletted{frame}, 1000, int64(len(want))); !errors.Is(err, context.Canceled) {
		t.Fatalf("bounded paletted encoder cancellation = %v, want context.Canceled", err)
	}
}

func TestLimitedBufferRejectsBeforeGrowth(t *testing.T) {
	buffer := limitedBuffer{maxBytes: 3}
	if written, err := buffer.Write([]byte("GIF")); written != 3 || err != nil {
		t.Fatalf("exact limited write = %d/%v, want 3/nil", written, err)
	}
	if written, err := buffer.Write([]byte("8")); written != 0 || err == nil {
		t.Fatalf("over-limit write = %d/%v, want 0/error", written, err)
	}
	if got := buffer.String(); got != "GIF" {
		t.Fatalf("limited buffer grew after rejection: %q", got)
	}
}

func TestAnimateImagesRejectsInvalidFramesAndCancellation(t *testing.T) {
	if _, err := animateImagesWithConcurrency(context.Background(), []image.Image{nil}, 1000, workers); err == nil {
		t.Fatal("AnimateImages accepted a nil frame")
	}
	var typedNil *image.NRGBA
	if _, err := animateImagesWithConcurrency(context.Background(), []image.Image{typedNil}, 1000, workers); err == nil {
		t.Fatal("AnimateImages accepted a typed nil frame")
	}
	if _, err := animateImagesWithConcurrency(context.Background(), []image.Image{image.NewNRGBA(image.Rectangle{})}, 1000, workers); err == nil {
		t.Fatal("AnimateImages accepted an empty frame")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := animateImagesWithConcurrency(ctx, []image.Image{image.NewNRGBA(image.Rect(0, 0, 1, 1))}, 1000, workers); !errors.Is(err, context.Canceled) {
		t.Fatalf("AnimateImages cancellation = %v, want context.Canceled", err)
	}
	if _, err := animateImagesWithConcurrency(context.Background(), []image.Image{image.NewNRGBA(image.Rect(0, 0, 1, 1))}, 1000, 0); err == nil {
		t.Fatal("animateImagesWithConcurrency accepted zero workers")
	}
	if _, err := animateImagesWithConcurrency(context.Background(), []image.Image{image.NewNRGBA(image.Rect(0, 0, 1, 1))}, 1000, workers+1); err == nil {
		t.Fatal("animateImagesWithConcurrency accepted too many workers")
	}
}

func TestContextWriterObservesCancellationDuringWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var output bytes.Buffer
	writer := contextWriter{
		ctx: ctx,
		output: cancelAfterWrite{
			output: &output,
			cancel: cancel,
		},
	}
	written, err := writer.Write([]byte("GIF89a"))
	if written != len("GIF89a") || !errors.Is(err, context.Canceled) {
		t.Fatalf("context write = %d/%v, want %d/context.Canceled", written, err, len("GIF89a"))
	}
}

type cancelAfterWrite struct {
	output *bytes.Buffer
	cancel context.CancelFunc
}

func (w cancelAfterWrite) Write(data []byte) (int, error) {
	written, err := w.output.Write(data)
	w.cancel()
	return written, err
}

type boundsOnlyImage struct {
	bounds image.Rectangle
}

func (img boundsOnlyImage) ColorModel() color.Model { return color.NRGBAModel }
func (img boundsOnlyImage) Bounds() image.Rectangle { return img.bounds }
func (img boundsOnlyImage) At(_, _ int) color.Color { return color.Black }

func fillNRGBA(img *image.NRGBA, fill color.NRGBA) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
}

func sumDelays(delays []int) int {
	total := 0
	for _, delay := range delays {
		total += delay
	}
	return total
}
