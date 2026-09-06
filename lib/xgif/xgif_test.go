package xgif

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"math"
	"math/rand"
	"reflect"
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
	got, err := AnimatePalettedImagesWithLimit(context.Background(), frames, 1000, math.MaxInt64)
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
	encoded, err := AnimatePalettedImagesWithLimit(nil, []*image.Paletted{valid, valid}, 1000, math.MaxInt64)
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
	if _, err := AnimatePalettedImagesWithLimit(ctx, []*image.Paletted{valid}, 1000, math.MaxInt64); !errors.Is(err, context.Canceled) {
		t.Fatalf("AnimatePalettedImages cancellation = %v, want context.Canceled", err)
	}
	if _, err := AnimatePalettedImagesWithLimit(context.Background(), nil, 1000, math.MaxInt64); err == nil {
		t.Fatal("AnimatePalettedImages accepted no frames")
	}
	if _, err := AnimatePalettedImagesWithLimit(context.Background(), []*image.Paletted{valid}, 0, math.MaxInt64); err == nil {
		t.Fatal("AnimatePalettedImages accepted a zero interval")
	}
	if _, err := AnimatePalettedImagesWithLimit(context.Background(), []*image.Paletted{nil}, 1000, math.MaxInt64); err == nil {
		t.Fatal("AnimatePalettedImages accepted a nil frame")
	}
	nonZeroBounds := &image.Paletted{Pix: []byte{0}, Stride: 1, Rect: image.Rect(1, 1, 2, 2), Palette: color.Palette{color.Black}}
	if _, err := AnimatePalettedImagesWithLimit(context.Background(), []*image.Paletted{nonZeroBounds}, 1000, math.MaxInt64); err == nil {
		t.Fatal("AnimatePalettedImages accepted non-zero frame bounds")
	}
	larger := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.Black})
	if _, err := AnimatePalettedImagesWithLimit(context.Background(), []*image.Paletted{valid, larger}, 1000, math.MaxInt64); err == nil {
		t.Fatal("AnimatePalettedImages accepted mismatched frame dimensions")
	}
}

func TestAnimatePalettedImagesOutputLimit(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	want, err := AnimatePalettedImagesWithLimit(context.Background(), []*image.Paletted{frame, frame}, 1000, math.MaxInt64)
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

func TestGIFEncoderMatchesStandardLibraryWithoutGlobalColorTable(t *testing.T) {
	random := rand.New(rand.NewSource(0x5eedc0de))
	for caseIndex := range 96 {
		frameCount := caseIndex%7 + 1
		width := caseIndex%19 + 1
		height := caseIndex%11 + 1
		paletteLength := []int{1, 2, 3, 4, 7, 16, 31, 255, 256}[caseIndex%9]
		basePalette := make(color.Palette, paletteLength)
		for index := range basePalette {
			basePalette[index] = color.RGBA{
				R: uint8(random.Intn(256)),
				G: uint8(random.Intn(256)),
				B: uint8(random.Intn(256)),
				A: 255,
			}
		}
		if caseIndex%4 == 0 {
			basePalette[caseIndex%paletteLength] = color.NRGBA{R: 19, G: 37, B: 73, A: 0}
		}
		frames := make([]*image.Paletted, frameCount)
		for frameIndex := range frames {
			framePalette := basePalette
			if caseIndex%3 != 0 && frameIndex != 0 {
				framePalette = append(color.Palette(nil), basePalette...)
				if frameIndex%2 != 0 && len(framePalette) > 1 {
					entry := frameIndex % len(framePalette)
					framePalette[entry] = color.RGBA{R: uint8(caseIndex), G: uint8(frameIndex * 29), B: 211, A: 255}
				}
			}
			stride := width + frameIndex%3
			pixels := make([]byte, (height-1)*stride+width)
			for y := range height {
				for x := range width {
					pixels[y*stride+x] = uint8(random.Intn(paletteLength))
				}
			}
			frames[frameIndex] = &image.Paletted{
				Pix: pixels, Stride: stride, Rect: image.Rect(0, 0, width, height), Palette: framePalette,
			}
		}
		delays := make([]int, frameCount)
		for index := range delays {
			delays[index] = random.Intn(101)
		}

		got, err := encodePalettedImages(context.Background(), frames, delays, 1<<30)
		if err != nil {
			t.Fatalf("case %d native encoder: %v", caseIndex, err)
		}
		var standard bytes.Buffer
		err = gif.EncodeAll(&standard, &gif.GIF{
			Image: frames, Delay: delays, LoopCount: INFINITE_LOOP,
			Config: image.Config{Width: width, Height: height},
		})
		if err != nil {
			t.Fatalf("case %d standard encoder: %v", caseIndex, err)
		}
		if !bytes.Equal(got, standard.Bytes()) {
			t.Fatalf("case %d differs from standard GIF encoder: %d versus %d bytes", caseIndex, len(got), standard.Len())
		}
		if got[10]&0x80 != 0 {
			t.Fatalf("case %d unexpectedly emitted a global color table", caseIndex)
		}
	}
}

func TestGIFEncoderMatchesStandardLibraryAcrossDictionaryResets(t *testing.T) {
	palette := make(color.Palette, 256)
	for index := range palette {
		palette[index] = color.RGBA{R: uint8(index * 37), G: uint8(index * 73), B: uint8(index * 109), A: 255}
	}
	frames := make([]*image.Paletted, 3)
	state := uint32(42)
	for frameIndex := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, 256, 256), append(color.Palette(nil), palette...))
		for index := range frame.Pix {
			state = state*1664525 + 1013904223
			frame.Pix[index] = uint8(state >> 24)
		}
		frames[frameIndex] = frame
	}
	delays := []int{3, 4, 3}
	got, err := encodePalettedImages(context.Background(), frames, delays, 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	var standard bytes.Buffer
	if err := gif.EncodeAll(&standard, &gif.GIF{
		Image: frames, Delay: delays, LoopCount: INFINITE_LOOP,
		Config: image.Config{Width: 256, Height: 256},
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, standard.Bytes()) {
		t.Fatalf("native encoder differs after dictionary reset: %d versus %d bytes", len(got), standard.Len())
	}
}

func TestOpaqueGlobalPaletteEncodingMatchesStandardLibraryAndIsSmaller(t *testing.T) {
	palette := color.Palette{
		color.RGBA{R: 255, A: 255},
		color.RGBA{R: 19, G: 37, B: 73, A: 255},
		color.RGBA{G: 255, A: 255},
		color.RGBA{B: 255, A: 255},
	}
	frames := make([]*image.Paletted, 8)
	for frameIndex := range frames {
		framePalette := append(color.Palette(nil), palette...)
		frame := image.NewPaletted(image.Rect(0, 0, 17, 13), framePalette)
		for index := range frame.Pix {
			frame.Pix[index] = uint8((index + frameIndex) % len(framePalette))
		}
		frames[frameIndex] = frame
	}
	delays, err := gifFrameDelays(len(frames), 1000)
	if err != nil {
		t.Fatal(err)
	}
	got := encodeOpaquePalettedFramesForTest(t, frames, 1000, 1<<20)
	var global bytes.Buffer
	if err := gif.EncodeAll(&global, &gif.GIF{
		Image: frames, Delay: delays, LoopCount: INFINITE_LOOP,
		Config: image.Config{ColorModel: frames[0].Palette, Width: 17, Height: 13},
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, global.Bytes()) {
		t.Fatalf("opaque global-table encoder differs from standard GIF encoder: %d versus %d bytes", len(got), global.Len())
	}
	var local bytes.Buffer
	if err := gif.EncodeAll(&local, &gif.GIF{
		Image: frames, Delay: delays, LoopCount: INFINITE_LOOP,
		Config: image.Config{Width: 17, Height: 13},
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) >= local.Len() {
		t.Fatalf("global palette GIF length = %d, want less than local-table length %d", len(got), local.Len())
	}
	if got[10]&0x80 == 0 {
		t.Fatal("opaque encoder did not emit a global color table")
	}
	gotAnimation, err := gif.DecodeAll(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	wantAnimation, err := gif.DecodeAll(bytes.NewReader(local.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(gotAnimation.Image) != len(wantAnimation.Image) ||
		!reflect.DeepEqual(gotAnimation.Delay, wantAnimation.Delay) ||
		!reflect.DeepEqual(gotAnimation.Disposal, wantAnimation.Disposal) ||
		gotAnimation.LoopCount != wantAnimation.LoopCount {
		t.Fatal("global palette changed animation timing, disposal, or loop behavior")
	}
	for frameIndex := range gotAnimation.Image {
		gotFrame := gotAnimation.Image[frameIndex]
		wantFrame := wantAnimation.Image[frameIndex]
		if gotFrame.Bounds() != wantFrame.Bounds() || !bytes.Equal(gotFrame.Pix, wantFrame.Pix) {
			t.Fatalf("frame %d indexed pixels differ", frameIndex)
		}
		for paletteIndex := range gotFrame.Palette {
			if gotFrame.Palette[paletteIndex] != wantFrame.Palette[paletteIndex] {
				t.Fatalf("frame %d palette index %d differs", frameIndex, paletteIndex)
			}
		}
	}
}

func TestLocalColorTablesPreserveTransparentConfigAndCompositing(t *testing.T) {
	frames := []*image.Paletted{
		{Pix: []byte{0, 1}, Stride: 2, Rect: image.Rect(0, 0, 2, 1), Palette: color.Palette{
			color.NRGBA{R: 19, G: 37, B: 73, A: 0},
			color.RGBA{R: 255, A: 255},
		}},
		{Pix: []byte{0, 1}, Stride: 2, Rect: image.Rect(0, 0, 2, 1), Palette: color.Palette{
			color.RGBA{G: 255, A: 255},
			color.NRGBA{R: 91, G: 53, B: 17, A: 0},
		}},
	}
	delays := []int{3, 4}
	encoded, err := encodePalettedImages(context.Background(), frames, delays, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	globalPalette, ok := decoded.Config.ColorModel.(color.Palette)
	if !ok || len(globalPalette) != 0 {
		t.Fatalf("decoded local-table GIF global color model = %#v, want empty palette", decoded.Config.ColorModel)
	}
	if encoded[10]&0x80 != 0 {
		t.Fatal("transparent animation unexpectedly emitted a global color table")
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{B: 255, A: 255}), image.Point{}, draw.Src)
	for _, frame := range decoded.Image {
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
	}
	if got := canvas.NRGBAAt(0, 0); got != (color.NRGBA{G: 255, A: 255}) {
		t.Fatalf("opaque second-frame pixel = %#v, want green", got)
	}
	if got := canvas.NRGBAAt(1, 0); got != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("transparent second-frame pixel = %#v, want preserved red", got)
	}
}

func encodeOpaquePalettedFramesForTest(t testing.TB, frames []*image.Paletted, intervalMs int, maxBytes int64) []byte {
	t.Helper()
	bounds := frames[0].Bounds()
	encoder, err := NewOpaquePalettedAnimationEncoder(context.Background(), bounds.Dx(), bounds.Dy(), len(frames), intervalMs, maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	for index, frame := range frames {
		if err := encoder.WriteFrame(frame); err != nil {
			t.Fatalf("frame %d: %v", index, err)
		}
	}
	encoded, err := encoder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestGIFEncoderObservesCancellationDuringCompression(t *testing.T) {
	frames := benchmarkPalettedFrames(3, 128, 96, 256, true)
	success := &normalizationCheckContext{}
	if _, err := AnimatePalettedImagesWithLimit(success, frames, 1000, 1<<20); err != nil {
		t.Fatal(err)
	}
	if success.checks < 8 {
		t.Fatalf("GIF encoding made only %d context checks", success.checks)
	}
	ctx := &normalizationCheckContext{failAt: success.checks / 2}
	encoded, err := AnimatePalettedImagesWithLimit(ctx, frames, 1000, 1<<20)
	if encoded != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-compression cancellation = (%d bytes, %v), want (nil, context.Canceled)", len(encoded), err)
	}
}

func TestOpaquePalettedAnimationEncoderMatchesBatchAndDoesNotRetainFrames(t *testing.T) {
	frames := benchmarkPalettedFrames(7, 43, 29, 16, false)
	want := encodeOpaquePalettedFramesForTest(t, frames, 1000, 1<<20)
	encoder, err := NewOpaquePalettedAnimationEncoder(context.Background(), 43, 29, len(frames), 1000, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for index, frame := range frames {
		if err := encoder.WriteFrame(frame); err != nil {
			t.Fatalf("frame %d: %v", index, err)
		}
		if index == 0 {
			// Header palette values have already been copied. A caller can release
			// or reuse its first frame immediately after WriteFrame returns.
			frame.Palette[0] = color.RGBA{R: 1, G: 2, B: 3, A: 255}
		}
	}
	got, err := encoder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("incremental encoder differs from batch encoder")
	}
}

func TestOpaquePalettedAnimationEncoderStateValidation(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	if _, err := NewOpaquePalettedAnimationEncoder(context.Background(), 2, 2, 1, 1000, 0); err == nil {
		t.Fatal("incremental encoder accepted zero output limit")
	}
	encoder, err := NewOpaquePalettedAnimationEncoder(context.Background(), 2, 2, 1, 1000, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Finish(); err == nil {
		t.Fatal("incremental encoder finished without its frame")
	}
	if err := encoder.WriteFrame(frame); err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := encoder.WriteFrame(frame); err == nil {
		t.Fatal("incremental encoder accepted a frame after finishing")
	}
	if _, err := encoder.Finish(); err == nil {
		t.Fatal("incremental encoder finished twice")
	}

	wrongSize, err := NewOpaquePalettedAnimationEncoder(context.Background(), 2, 2, 1, 1000, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := wrongSize.WriteFrame(image.NewPaletted(image.Rect(0, 0, 1, 2), frame.Palette)); err == nil {
		t.Fatal("incremental encoder accepted wrong-size frame")
	}
	if err := wrongSize.WriteFrame(frame); err == nil {
		t.Fatal("incremental encoder did not retain its first error")
	}

	for _, palette := range []color.Palette{
		{color.NRGBA{R: 17, G: 31, B: 47, A: 0}, color.White},
		{color.NRGBA{R: 17, G: 31, B: 47, A: 127}, color.White},
	} {
		opacity, err := NewOpaquePalettedAnimationEncoder(context.Background(), 2, 2, 1, 1000, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		if err := opacity.WriteFrame(image.NewPaletted(image.Rect(0, 0, 2, 2), palette)); err == nil {
			t.Fatalf("opaque encoder accepted palette alpha %#v", palette[0])
		}
		if _, err := opacity.Finish(); err == nil {
			t.Fatal("opaque encoder did not retain its palette-opacity error")
		}
	}

	laterFrame, err := NewOpaquePalettedAnimationEncoder(context.Background(), 2, 2, 2, 1000, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := laterFrame.WriteFrame(frame); err != nil {
		t.Fatal(err)
	}
	transparent := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Transparent, color.White})
	if err := laterFrame.WriteFrame(transparent); err == nil {
		t.Fatal("opaque encoder accepted transparency after writing its global table")
	}
	if _, err := laterFrame.Finish(); err == nil {
		t.Fatal("opaque encoder returned partial output after a later transparency error")
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
