package d2raster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestDirectPatternPixelOffsets(t *testing.T) {
	tests := []struct {
		name         string
		tile         d2scene.Box
		transform    d2scene.Matrix
		width        int
		height       int
		wantX        int
		wantY        int
		wantEligible bool
	}{
		{name: "identity", tile: d2scene.Box{Width: 7, Height: 5}, transform: d2scene.Identity(), width: 7, height: 5, wantEligible: true},
		{name: "integer phase", tile: d2scene.Box{X: -9, Y: 12, Width: 7, Height: 5}, transform: d2scene.Translate(4, -6), width: 7, height: 5, wantX: 6, wantY: 2, wantEligible: true},
		{name: "multiple periods", tile: d2scene.Box{X: 14, Y: -10, Width: 7, Height: 5}, transform: d2scene.Translate(-21, 15), width: 7, height: 5, wantEligible: true},
		{name: "fractional origin", tile: d2scene.Box{X: .5, Width: 7, Height: 5}, transform: d2scene.Identity(), width: 7, height: 5},
		{name: "fractional translation", tile: d2scene.Box{Width: 7, Height: 5}, transform: d2scene.Translate(.5, 0), width: 7, height: 5},
		{name: "scale", tile: d2scene.Box{Width: 7, Height: 5}, transform: d2scene.Scale(2, 1), width: 7, height: 5},
		{name: "pixel dimensions", tile: d2scene.Box{Width: 7, Height: 5}, transform: d2scene.Identity(), width: 8, height: 5},
		{name: "non-finite", tile: d2scene.Box{Width: 7, Height: 5}, transform: d2scene.Matrix{A: 1, D: 1, E: math.Inf(1)}, width: 7, height: 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotX, gotY, gotEligible := directPatternPixelOffsets(test.tile, test.transform, test.width, test.height)
			if gotX != test.wantX || gotY != test.wantY || gotEligible != test.wantEligible {
				t.Fatalf("directPatternPixelOffsets() = (%d, %d, %v), want (%d, %d, %v)", gotX, gotY, gotEligible, test.wantX, test.wantY, test.wantEligible)
			}
		})
	}
}

func TestDrawDirectPatternMaskPixelsMatchesGeneralPath(t *testing.T) {
	const tileWidth, tileHeight = 7, 5
	tileImage := image.NewRGBA(image.Rect(0, 0, tileWidth, tileHeight))
	for y := 0; y < tileHeight; y++ {
		for x := 0; x < tileWidth; x++ {
			alpha := byte((x*43 + y*71) & 0xff)
			if (x+y)%5 == 0 {
				alpha = 0
			} else if (x+y)%4 == 0 {
				alpha = 0xff
			}
			offset := tileImage.PixOffset(x, y)
			tileImage.Pix[offset] = byte(uint16(byte(x*31+17)) * uint16(alpha) / 255)
			tileImage.Pix[offset+1] = byte(uint16(byte(y*47+29)) * uint16(alpha) / 255)
			tileImage.Pix[offset+2] = byte(uint16(byte(x*19+y*23+11)) * uint16(alpha) / 255)
			tileImage.Pix[offset+3] = alpha
		}
	}
	tile := &preparedPatternTile{width: tileWidth, height: tileHeight, image: tileImage}
	for caseIndex, phase := range []image.Point{{}, {X: 4, Y: -6}, {X: -17, Y: 13}} {
		const width, height = 9000, 4
		bounds := image.Rect(-11, -7, -11+width, -7+height)
		stride := width*4 + 13
		initial := make([]byte, stride*height+9)
		for index := range initial {
			initial[index] = byte(index*37 + caseIndex*19 + 5)
		}
		maskStride := width + 7
		mask := &image.Alpha{Pix: make([]byte, maskStride*height+3), Stride: maskStride, Rect: image.Rect(0, 0, width, height)}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				mask.Pix[y*maskStride+x] = [...]byte{0, 0xff, 97, 213}[(x/23+y)%4]
			}
		}
		pattern := &preparedPattern{
			tile:       d2scene.Box{X: -9, Y: 12, Width: tileWidth, Height: tileHeight},
			originModX: math.Mod(-9, tileWidth), originModY: math.Mod(12, tileHeight),
			deviceToPattern: d2scene.Translate(float64(phase.X), float64(phase.Y)),
			tileResource:    tile,
			directPixels:    true,
		}
		pattern.pixelOffsetX, pattern.pixelOffsetY, _ = directPatternPixelOffsets(pattern.tile, pattern.deviceToPattern, tileWidth, tileHeight)
		for _, cancelAt := range []int{0, 1, 2, 3, 7, 13} {
			got := &image.RGBA{Pix: bytes.Clone(initial), Stride: stride, Rect: bounds}
			want := &image.RGBA{Pix: bytes.Clone(initial), Stride: stride, Rect: bounds}
			var gotContext, wantContext context.Context = context.Background(), context.Background()
			var gotCounter, wantCounter *cancelAfterErrCallsContext
			if cancelAt != 0 {
				gotCounter = &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
				wantCounter = &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
				gotContext, wantContext = gotCounter, wantCounter
			}
			gotErr := drawDirectPatternMaskPixels(gotContext, got, bounds, mask, pattern)
			wantErr := drawPaintMaskPixels(wantContext, want, bounds, mask, &preparedPaint{kind: preparedPatternPaint, pattern: pattern})
			if errors.Is(gotErr, context.Canceled) != errors.Is(wantErr, context.Canceled) {
				t.Fatalf("case %d cancel %d errors differ: direct %v, general %v", caseIndex, cancelAt, gotErr, wantErr)
			}
			if gotCounter != nil && gotCounter.calls != wantCounter.calls {
				t.Fatalf("case %d cancel %d Err calls = %d, want %d", caseIndex, cancelAt, gotCounter.calls, wantCounter.calls)
			}
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("case %d cancel %d output differs", caseIndex, cancelAt)
			}
		}
	}
}

func TestPatternUserSpaceRepeatPhaseTransformAndOpacity(t *testing.T) {
	t.Run("repeat and tile phase have no transparent seams", func(t *testing.T) {
		pattern := stripedPattern(d2scene.UserSpaceOnUse, d2scene.Box{X: 1, Width: 2, Height: 1}, d2scene.Identity())
		frame, err := Render(context.Background(), patternDocument(8, 2, pattern), testOptions())
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 2; y++ {
			for x := 0; x < 8; x++ {
				want := color.NRGBA{R: 255, A: 255}
				if x%2 == 0 {
					want = color.NRGBA{B: 255, A: 255}
				}
				assertPixel(t, frame.NRGBAAt(x, y), want)
			}
		}
	})

	t.Run("paint rotation changes stripe axis", func(t *testing.T) {
		pattern := stripedPattern(d2scene.UserSpaceOnUse, d2scene.Box{Width: 2, Height: 1}, d2scene.Rotate(math.Pi/2))
		frame, err := Render(context.Background(), patternDocument(3, 4, pattern), testOptions())
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 4; y++ {
			want := color.NRGBA{R: 255, A: 255}
			if y%2 == 1 {
				want = color.NRGBA{B: 255, A: 255}
			}
			for x := 0; x < 3; x++ {
				assertPixel(t, frame.NRGBAAt(x, y), want)
			}
		}
	})

	t.Run("animated tile-root alpha composites through repeated samples", func(t *testing.T) {
		root := d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{Width: 1, Height: 1}, Fill: red,
		})
		root.Animations = []d2scene.Track{animationTrack(
			d2scene.AnimateOpacity, d2scene.NumberValue(0), d2scene.NumberValue(1),
		)}
		pattern := d2scene.PatternPaint{
			Tile: d2scene.Box{Width: 1, Height: 1}, Root: root,
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		}
		options := testOptions()
		options.Time = 500 * time.Millisecond
		frame, err := Render(context.Background(), patternDocument(4, 2, pattern), options)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 2; y++ {
			for x := 0; x < 4; x++ {
				assertPixel(t, frame.NRGBAAt(x, y), color.NRGBA{R: 255, A: 128})
			}
		}
	})
}

func TestPatternSampleMatchesNRGBAModel(t *testing.T) {
	t.Parallel()

	bounds := image.Rect(0, 0, 256, 256)
	tileImage := image.NewRGBA(bounds)
	for alpha := range 256 {
		for value := range 256 {
			offset := tileImage.PixOffset(value, alpha)
			tileImage.Pix[offset] = uint8(value)
			tileImage.Pix[offset+1] = uint8(255 - value)
			tileImage.Pix[offset+2] = uint8(value ^ alpha)
			tileImage.Pix[offset+3] = uint8(alpha)
		}
	}
	pattern := &preparedPattern{
		tile:            d2scene.Box{Width: 256, Height: 256},
		deviceToPattern: d2scene.Identity(),
		tileResource: &preparedPatternTile{
			width: 256, height: 256, bounds: bounds, image: tileImage,
		},
	}
	for alpha := range 256 {
		for value := range 256 {
			got, ok := pattern.colorAt(float64(value)+.5, float64(alpha)+.5)
			want := color.NRGBAModel.Convert(tileImage.RGBAAt(value, alpha)).(color.NRGBA)
			if got != want || ok != (want.A != 0) {
				t.Fatalf("sample value=%d alpha=%d = (%#v,%v), want (%#v,%v)", value, alpha, got, ok, want, want.A != 0)
			}
		}
	}
}

func TestWrappedPatternCoordinateMatchesDoubleModReference(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		value, origin, period float64
	}{
		{value: -5, origin: 5, period: 10}, // Exact negative-period difference produces -0.
		{value: 5, origin: -5, period: 10},
		{value: -15, origin: 5, period: 10},
		{value: 15, origin: -5, period: 10},
	} {
		got, gotOK := wrappedPatternCoordinateFromOriginMod(test.value, math.Mod(test.origin, test.period), test.period)
		want, wantOK := doubleModWrappedPatternCoordinate(test.value, test.origin, test.period)
		if gotOK != wantOK || gotOK && math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("wrapped(%g,%g,%g) = (%g,%v), want bit-exact (%g,%v)", test.value, test.origin, test.period, got, gotOK, want, wantOK)
		}
	}

	state := uint64(0x9e3779b97f4a7c15)
	next := func() uint64 {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		return state
	}
	for range 100_000 {
		value := math.Float64frombits(next())
		origin := math.Float64frombits(next())
		period := math.Float64frombits(next() &^ (uint64(1) << 63))
		got, gotOK := wrappedPatternCoordinateFromOriginMod(value, math.Mod(origin, period), period)
		want, wantOK := doubleModWrappedPatternCoordinate(value, origin, period)
		if gotOK != wantOK || gotOK && math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("wrapped(%g,%g,%g) = (%g,%v), want bit-exact (%g,%v)", value, origin, period, got, gotOK, want, wantOK)
		}
	}
}

func doubleModWrappedPatternCoordinate(value, origin, period float64) (float64, bool) {
	if !finite(value) || !finite(origin) || !finite(period) || period <= 0 {
		return 0, false
	}
	wrapped := math.Mod(value, period) - math.Mod(origin, period)
	wrapped = math.Mod(wrapped, period)
	if wrapped < 0 {
		wrapped += period
	}
	if !finite(wrapped) {
		return 0, false
	}
	return wrapped, true
}

func TestPatternObjectBoundingBoxAndObjectTransform(t *testing.T) {
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: .25, Height: 1}, Fill: red}),
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: .25, Width: .25, Height: 1}, Fill: blue}),
	}
	pattern := d2scene.PatternPaint{
		Tile: d2scene.Box{Width: .5, Height: 1}, Root: root,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(),
	}
	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 2, Y: 1, Width: 8, Height: 4}, Fill: pattern,
	})
	node.Transform = d2scene.Translate(3, 2)
	document := d2scene.NewDocument(d2scene.Box{Width: 15, Height: 8}, node)
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, frame.NRGBAAt(5, 4), color.NRGBA{R: 255, A: 255})
	assertPixel(t, frame.NRGBAAt(7, 4), color.NRGBA{B: 255, A: 255})
	assertPixel(t, frame.NRGBAAt(9, 4), color.NRGBA{R: 255, A: 255})
	assertPixel(t, frame.NRGBAAt(4, 4), color.NRGBA{})
}

func TestPatternStrokeAndEvenOddFill(t *testing.T) {
	pattern := stripedPattern(d2scene.UserSpaceOnUse, d2scene.Box{Width: 2, Height: 1}, d2scene.Identity())
	stroke := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{d2scene.MoveTo(0, 2), d2scene.LineTo(10, 2)},
		Stroke:   &d2scene.Stroke{Paint: &pattern, Width: 2, Join: d2scene.JoinRound},
	})
	strokeFrame, err := Render(
		context.Background(),
		d2scene.NewDocument(d2scene.Box{Width: 10, Height: 4}, stroke),
		testOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	for x := 1; x < 9; x++ {
		want := color.NRGBA{R: 255, A: 255}
		if x%2 == 1 {
			want = color.NRGBA{B: 255, A: 255}
		}
		assertPixel(t, strokeFrame.NRGBAAt(x, 2), want)
	}
	assertPixel(t, strokeFrame.NRGBAAt(5, 0), color.NRGBA{})

	evenOdd := nestedRectanglePath(
		d2scene.Box{Width: 8, Height: 8},
		d2scene.Box{X: 2, Y: 2, Width: 4, Height: 4},
		pattern,
	)
	fillFrame, err := Render(
		context.Background(),
		d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, d2scene.NewNode(evenOdd)),
		testOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, fillFrame.NRGBAAt(0, 0), color.NRGBA{R: 255, A: 255})
	assertPixel(t, fillFrame.NRGBAAt(1, 0), color.NRGBA{B: 255, A: 255})
	assertPixel(t, fillFrame.NRGBAAt(3, 3), color.NRGBA{})
}

func TestPatternNestedRasterAndRetainedVectorAssets(t *testing.T) {
	inner := d2scene.PatternPaint{
		Tile: d2scene.Box{Width: 1, Height: 1},
		Root: d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{Width: 1, Height: 1}, Fill: green,
		}),
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}
	vectorRoot := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 1, Height: 1}, Fill: inner,
	})
	outerRoot := d2scene.NewNode(nil)
	outerRoot.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Image{Asset: "pixel", Box: d2scene.Box{Width: 1, Height: 1}}),
		d2scene.NewNode(d2scene.Image{Asset: "vector", Box: d2scene.Box{X: 1, Width: 1, Height: 1}}),
	}
	outer := d2scene.PatternPaint{
		Tile: d2scene.Box{Width: 2, Height: 1}, Root: outerRoot,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}
	document := patternDocument(6, 1, outer)
	document.Assets["pixel"] = d2scene.RasterAsset{
		MIMEType:   "image/png",
		Data:       encodeRasterPNG(t, uniformNRGBA(1, 1, color.NRGBA{B: 255, A: 255})),
		PixelWidth: 1, PixelHeight: 1,
	}
	document.Assets["vector"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: vectorRoot,
	}
	prepared, err := prepare(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.patterns) != 2 {
		t.Fatalf("prepared pattern tiles = %d, want nested and outer tiles", len(prepared.patterns))
	}
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 6; x++ {
		want := color.NRGBA{B: 255, A: 255}
		if x%2 == 1 {
			want = color.NRGBA{G: 255, A: 255}
		}
		assertPixel(t, frame.NRGBAAt(x, 0), want)
	}
}

func TestPatternConcurrentRendersUseFrameLocalTiles(t *testing.T) {
	pattern := stripedPattern(d2scene.UserSpaceOnUse, d2scene.Box{Width: 2, Height: 1}, d2scene.Identity())
	document := patternDocument(32, 16, pattern)
	const workers = 8
	errorsByWorker := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			frame, err := Render(context.Background(), document, testOptions())
			if err == nil {
				if got := frame.NRGBAAt(11, 7); got != (color.NRGBA{B: 255, A: 255}) {
					err = errors.New("concurrent pattern render returned the wrong pixel")
				}
			}
			errorsByWorker <- err
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPatternSharedDefinitionUsesOneTileAndPerOccurrenceMapping(t *testing.T) {
	t.Run("one tile resource and one work charge", func(t *testing.T) {
		tilePath := nestedRectanglePath(
			d2scene.Box{Width: 8, Height: 8},
			d2scene.Box{X: 2, Y: 2, Width: 4, Height: 4},
			red,
		)
		tileRoot := d2scene.NewNode(tilePath)
		pattern := d2scene.PatternPaint{
			Tile: d2scene.Box{Width: 8, Height: 8}, Root: tileRoot,
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		}
		root := d2scene.NewNode(nil)
		root.Children = []*d2scene.Node{
			d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 8, Height: 8}, Fill: pattern}),
			d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 8, Width: 8, Height: 8}, Fill: pattern}),
		}
		document := d2scene.NewDocument(d2scene.Box{Width: 16, Height: 8}, root)
		options := testOptions()
		prepared, err := prepare(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		const wantWork = int64(8 * 8 * 8 * 4 * 4)
		if len(prepared.patterns) != 1 {
			t.Fatalf("prepared pattern tiles = %d, want one shared tile", len(prepared.patterns))
		}
		if prepared.resources.evenOddClipWork != wantWork {
			t.Fatalf("even-odd work = %d, want one shared tile's %d", prepared.resources.evenOddClipWork, wantWork)
		}
		options.MaxEvenOddClipWork = wantWork
		if _, err := Render(context.Background(), document, options); err != nil {
			t.Fatalf("inclusive one-tile work limit: %v", err)
		}
	})

	t.Run("sampling transforms remain occurrence local", func(t *testing.T) {
		pattern := stripedPattern(
			d2scene.UserSpaceOnUse,
			d2scene.Box{Width: 2, Height: 1},
			d2scene.Identity(),
		)
		left := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 1}, Fill: pattern})
		right := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 1}, Fill: pattern})
		right.Transform = d2scene.Translate(5, 0)
		root := d2scene.NewNode(nil)
		root.Children = []*d2scene.Node{left, right}
		document := d2scene.NewDocument(d2scene.Box{Width: 9, Height: 1}, root)

		prepared, err := prepare(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		if len(prepared.patterns) != 1 {
			t.Fatalf("prepared pattern tiles = %d, want one shared tile", len(prepared.patterns))
		}
		leftPattern := prepared.root.children[0].primitive.fill.pattern
		rightPattern := prepared.root.children[1].primitive.fill.pattern
		if leftPattern == nil || rightPattern == nil || leftPattern.tileResource != rightPattern.tileResource {
			t.Fatal("pattern occurrences do not reference the same prepared tile")
		}
		if leftPattern.deviceToPattern == rightPattern.deviceToPattern {
			t.Fatal("pattern occurrences unexpectedly share their sampling transform")
		}

		frame, err := Render(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		assertPixel(t, frame.NRGBAAt(0, 0), color.NRGBA{R: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(1, 0), color.NRGBA{B: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(4, 0), color.NRGBA{})
		assertPixel(t, frame.NRGBAAt(5, 0), color.NRGBA{R: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(6, 0), color.NRGBA{B: 255, A: 255})
	})
}

func TestPatternResourceLimitsAreExactAndInclusive(t *testing.T) {
	t.Run("tile and tile-root transient storage", func(t *testing.T) {
		tileRoot := d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{Width: 10, Height: 10}, Fill: red,
		})
		tileRoot.Opacity = .5
		pattern := d2scene.PatternPaint{
			Tile: d2scene.Box{Width: 10, Height: 10}, Root: tileRoot,
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		}
		document := patternDocument(1, 1, pattern)
		options := testOptions()
		prepared, err := prepare(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		// The peak is a 400-byte retained tile plus its 400-byte opacity
		// layer, together with retained scanline rows and two vertical edges.
		scanlineScratch, ok := scanline.RetainedBytes(10, 10, 2)
		if !ok {
			t.Fatal("scanline retained-byte calculation overflowed")
		}
		want := int64(800) + scanlineScratch
		if prepared.resources.peakOffscreenBytes != want {
			t.Fatalf("peak offscreen bytes = %d, want %d", prepared.resources.peakOffscreenBytes, want)
		}
		options.MaxOffscreenBytes = want - 1
		if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("peak offscreen pixel storage %d bytes exceeds limit %d", want, want-1)) {
			t.Fatalf("below-limit Render() error = %v", err)
		}
		options.MaxOffscreenBytes = want
		if _, err := Render(context.Background(), document, options); err != nil {
			t.Fatalf("inclusive limit Render() error = %v", err)
		}
	})

	t.Run("nested tiles are rendered and retained once", func(t *testing.T) {
		inner := d2scene.PatternPaint{
			Tile:  d2scene.Box{Width: 4, Height: 4},
			Root:  d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 4}, Fill: red}),
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		}
		outer := d2scene.PatternPaint{
			Tile:  d2scene.Box{Width: 4, Height: 4},
			Root:  d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 4}, Fill: inner}),
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		}
		document := patternDocument(4, 4, outer)
		options := testOptions()
		prepared, err := prepare(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		// Two 64-byte tiles persist. Coverage for the outer tile and final scene
		// is streamed, while the shared scanline rasterizer retains its rows and
		// two vertical edges.
		scanlineScratch, ok := scanline.RetainedBytes(4, 4, 2)
		if !ok {
			t.Fatal("scanline retained-byte calculation overflowed")
		}
		want := int64(128) + scanlineScratch
		if len(prepared.patterns) != 2 || prepared.resources.peakOffscreenBytes != want {
			t.Fatalf("nested pattern plan: tiles=%d peak=%d, want tiles=2 peak=%d", len(prepared.patterns), prepared.resources.peakOffscreenBytes, want)
		}
		options.MaxOffscreenBytes = want - 1
		if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("peak offscreen pixel storage %d bytes exceeds limit %d", want, want-1)) {
			t.Fatalf("below-nested-limit Render() error = %v", err)
		}
		options.MaxOffscreenBytes = want
		if _, err := Render(context.Background(), document, options); err != nil {
			t.Fatalf("inclusive nested limit Render() error = %v", err)
		}
	})

	t.Run("even-odd work inside one reused tile", func(t *testing.T) {
		tilePath := nestedRectanglePath(
			d2scene.Box{Width: 8, Height: 8},
			d2scene.Box{X: 2, Y: 2, Width: 4, Height: 4},
			red,
		)
		pattern := d2scene.PatternPaint{
			Tile: d2scene.Box{Width: 8, Height: 8}, Root: d2scene.NewNode(tilePath),
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		}
		document := patternDocument(16, 16, pattern)
		options := testOptions()
		prepared, err := prepare(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		const wantWork = int64(8 * 8 * 8 * 4 * 4)
		if prepared.resources.evenOddClipWork != wantWork {
			t.Fatalf("even-odd pattern work = %d, want one tile's %d", prepared.resources.evenOddClipWork, wantWork)
		}
		options.MaxEvenOddClipWork = wantWork - 1
		if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "even-odd clip work 8192 exceeds limit 8191") {
			t.Fatalf("below-work-limit Render() error = %v", err)
		}
		options.MaxEvenOddClipWork = wantWork
		if _, err := Render(context.Background(), document, options); err != nil {
			t.Fatalf("inclusive work limit Render() error = %v", err)
		}
	})
}

func TestPatternDepthNodeAndImportBoundaries(t *testing.T) {
	inner := d2scene.PatternPaint{
		Tile:  d2scene.Box{Width: 1, Height: 1},
		Root:  d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red}),
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}
	outer := d2scene.PatternPaint{
		Tile:  d2scene.Box{Width: 1, Height: 1},
		Root:  d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: inner}),
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}
	document := patternDocument(1, 1, outer)
	options := testOptions()
	options.MaxDepth = 1
	options.MaxImportDepth = 1
	options.MaxNodes = 2
	if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "node count exceeds limit 2") {
		t.Fatalf("below-node-limit Render() error = %v", err)
	}
	options.MaxNodes = 3
	if _, err := Render(context.Background(), document, options); err != nil {
		t.Fatalf("pattern roots should reset node depth and not consume vector-only import limit: %v", err)
	}

	assetPatternRoot := d2scene.NewNode(d2scene.Image{
		Asset: "vector", Box: d2scene.Box{Width: 1, Height: 1},
	})
	assetPattern := d2scene.PatternPaint{
		Tile: d2scene.Box{Width: 1, Height: 1}, Root: assetPatternRoot,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}
	assetDocument := patternDocument(1, 1, assetPattern)
	assetDocument.Assets["vector"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: 1, Height: 1},
		Root:    d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: green}),
	}
	options = testOptions()
	options.MaxImportDepth = 1
	if _, err := Render(context.Background(), assetDocument, options); err == nil || !strings.Contains(err.Error(), "import depth 2 exceeds limit 1") {
		t.Fatalf("pattern-to-vector import limit error = %v", err)
	}
	options.MaxImportDepth = 2
	if _, err := Render(context.Background(), assetDocument, options); err != nil {
		t.Fatalf("inclusive pattern-to-vector import limit: %v", err)
	}

	unused := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, d2scene.NewNode(nil))
	unused.Assets["pattern-only"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: 1, Height: 1},
		Root:    d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: outer}),
	}
	options = testOptions()
	options.MaxImportDepth = 1
	if _, err := prepare(context.Background(), unused, options); err != nil {
		t.Fatalf("unused retained pattern without a downstream vector consumed import depth: %v", err)
	}

	deepTileRoot := d2scene.NewNode(nil)
	deepTileRoot.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red}),
	}
	depthReset := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, d2scene.NewNode(nil))
	depthReset.Assets["deep-pattern"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: 1, Height: 1},
		Root: d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{Width: 1, Height: 1},
			Fill: d2scene.PatternPaint{
				Tile: d2scene.Box{Width: 1, Height: 1}, Root: deepTileRoot,
				Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
			},
		}),
	}
	options = testOptions()
	options.MaxDepth = 2
	if _, err := prepare(context.Background(), depthReset, options); err != nil {
		t.Fatalf("retained pattern root depth should reset independently of its host asset: %v", err)
	}
}

func TestPatternCyclesAndMalformedInputsFailPreflight(t *testing.T) {
	t.Run("visible root cycle", func(t *testing.T) {
		pattern := &d2scene.PatternPaint{
			Tile:  d2scene.Box{Width: 1, Height: 1},
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		}
		node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: pattern})
		pattern.Root = node
		_, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, node), testOptions())
		if err == nil || !strings.Contains(err.Error(), "node cycle") {
			t.Fatalf("cycle Render() error = %v", err)
		}
	})

	t.Run("unused retained root cycle", func(t *testing.T) {
		pattern := &d2scene.PatternPaint{
			Tile:  d2scene.Box{Width: 1, Height: 1},
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		}
		root := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: pattern})
		pattern.Root = root
		document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, d2scene.NewNode(nil))
		document.Assets["cycle"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: root}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), "node cycle") {
			t.Fatalf("retained cycle prepare() error = %v", err)
		}
	})

	t.Run("unused retained malformed pattern", func(t *testing.T) {
		document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, d2scene.NewNode(nil))
		document.Assets["bad-pattern"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root: d2scene.NewNode(d2scene.Rect{
				Box: d2scene.Box{Width: 1, Height: 1},
				Fill: d2scene.PatternPaint{
					Tile:  d2scene.Box{Height: 1},
					Root:  d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red}),
					Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
				},
			}),
		}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), "pattern tile has zero width or height") {
			t.Fatalf("retained malformed prepare() error = %v", err)
		}
	})

	validRoot := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
	tests := []struct {
		name    string
		paint   d2scene.Paint
		box     d2scene.Box
		mutate  func(*d2scene.Node)
		wantErr string
	}{
		{name: "nil pointer", paint: (*d2scene.PatternPaint)(nil), box: d2scene.Box{Width: 2, Height: 2}, wantErr: "nil pattern paint"},
		{name: "zero width", paint: d2scene.PatternPaint{Tile: d2scene.Box{Height: 1}, Root: validRoot, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity()}, box: d2scene.Box{Width: 2, Height: 2}, wantErr: "zero width or height"},
		{name: "negative height", paint: d2scene.PatternPaint{Tile: d2scene.Box{Width: 1, Height: -1}, Root: validRoot, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity()}, box: d2scene.Box{Width: 2, Height: 2}, wantErr: "negative box size"},
		{name: "endpoint overflow", paint: d2scene.PatternPaint{Tile: d2scene.Box{X: math.MaxFloat64, Width: math.MaxFloat64, Height: 1}, Root: validRoot, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity()}, box: d2scene.Box{Width: 2, Height: 2}, wantErr: "endpoints are non-finite"},
		{name: "nil root", paint: d2scene.PatternPaint{Tile: d2scene.Box{Width: 1, Height: 1}, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity()}, box: d2scene.Box{Width: 2, Height: 2}, wantErr: "no root node"},
		{name: "invalid units", paint: d2scene.PatternPaint{Tile: d2scene.Box{Width: 1, Height: 1}, Root: validRoot, Units: d2scene.PaintUnits(255), Transform: d2scene.Identity()}, box: d2scene.Box{Width: 2, Height: 2}, wantErr: "invalid paint units 255"},
		{name: "non-finite transform", paint: d2scene.PatternPaint{Tile: d2scene.Box{Width: 1, Height: 1}, Root: validRoot, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Matrix{A: math.NaN(), D: 1}}, box: d2scene.Box{Width: 2, Height: 2}, wantErr: "non-finite pattern transform"},
		{name: "singular transform", paint: d2scene.PatternPaint{Tile: d2scene.Box{Width: 1, Height: 1}, Root: validRoot, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Matrix{}}, box: d2scene.Box{Width: 2, Height: 2}, wantErr: "singular pattern transform"},
		{name: "zero object bounds", paint: d2scene.PatternPaint{Tile: d2scene.Box{Width: 1, Height: 1}, Root: validRoot, Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity()}, box: d2scene.Box{Height: 2}, wantErr: "object bounding box has zero width or height"},
		{name: "unrepresentable extent", paint: d2scene.PatternPaint{Tile: d2scene.Box{Width: 2, Height: 1}, Root: validRoot, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Scale(math.MaxFloat64, 1)}, box: d2scene.Box{Width: 2, Height: 2}, wantErr: "maps outside the finite pixel domain"},
		{name: "platform extent", paint: d2scene.PatternPaint{Tile: d2scene.Box{Width: float64(math.MaxInt), Height: 1}, Root: validRoot, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity()}, box: d2scene.Box{Width: 2, Height: 2}, wantErr: "platform integer domain"},
		{name: "singular ancestor", paint: d2scene.PatternPaint{Tile: d2scene.Box{Width: 1, Height: 1}, Root: validRoot, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity()}, box: d2scene.Box{Width: 2, Height: 2}, mutate: func(node *d2scene.Node) { node.Transform = d2scene.Scale(0, 1) }, wantErr: "singular pattern transform"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := d2scene.NewNode(d2scene.Rect{Box: test.box, Fill: test.paint})
			if test.mutate != nil {
				test.mutate(node)
			}
			_, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 2, Height: 2}, node), testOptions())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("prepare() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestPatternColorAnimationIsExplicitlyRejected(t *testing.T) {
	pattern := stripedPattern(d2scene.UserSpaceOnUse, d2scene.Box{Width: 2, Height: 1}, d2scene.Identity())
	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 2, Height: 1}, Fill: pattern})
	node.Animations = []d2scene.Track{animationTrack(
		d2scene.AnimateFillColor,
		d2scene.ColorValue(color.NRGBA{R: 255, A: 255}),
		d2scene.ColorValue(color.NRGBA{B: 255, A: 255}),
	)}
	_, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 2, Height: 1}, node), testOptions())
	if err == nil || !strings.Contains(err.Error(), "color animation targets pattern paint") {
		t.Fatalf("Render() error = %v", err)
	}
}

func TestPatternTileRenderCancellationReleasesReservation(t *testing.T) {
	root := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 512, Height: 512}, Fill: red,
	})
	pattern := d2scene.PatternPaint{
		Tile: d2scene.Box{Width: 512, Height: 512}, Root: root,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}
	prepared, err := prepare(context.Background(), patternDocument(1, 1, pattern), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.patterns) != 1 {
		t.Fatalf("prepared patterns = %d, want 1", len(prepared.patterns))
	}
	scratch := &rasterScratch{offscreen: offscreenBudget{limit: testOptions().MaxOffscreenBytes}}
	err = prepared.patterns[0].render(&cancelAfterContext{after: 1}, scratch)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pattern render error = %v, want context.Canceled", err)
	}
	if scratch.offscreen.live != 0 || scratch.patternBytes != 0 || prepared.patterns[0].image != nil {
		t.Fatalf("canceled pattern state: live=%d patternBytes=%d image=%v", scratch.offscreen.live, scratch.patternBytes, prepared.patterns[0].image != nil)
	}

	if err := prepared.patterns[0].render(context.Background(), scratch); err != nil {
		t.Fatal(err)
	}
	tileBytes := prepared.patterns[0].tileBytes
	err = drawPaintMask(
		&cancelAfterContext{after: 1},
		image.NewRGBA(image.Rect(0, 0, 512, 512)),
		image.Rect(0, 0, 512, 512),
		&preparedPaint{kind: preparedPatternPaint, pattern: &preparedPattern{
			tile:            pattern.Tile,
			deviceToPattern: d2scene.Identity(),
			tileResource:    prepared.patterns[0],
		}},
		scratch,
		"pattern cancellation Alpha mask",
		func(mask *image.Alpha) error {
			for index := range mask.Pix {
				mask.Pix[index] = 255
			}
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pattern sampling error = %v, want context.Canceled", err)
	}
	if scratch.offscreen.live != tileBytes || scratch.patternBytes != tileBytes {
		t.Fatalf("sampling cancellation leaked mask: live=%d patternBytes=%d, want retained tile %d", scratch.offscreen.live, scratch.patternBytes, tileBytes)
	}
	scratch.releasePatternTiles()
	if scratch.offscreen.live != 0 || prepared.patterns[0].image != nil {
		t.Fatalf("released pattern tile state: live=%d image=%v", scratch.offscreen.live, prepared.patterns[0].image != nil)
	}
}

func stripedPattern(units d2scene.PaintUnits, tile d2scene.Box, transform d2scene.Matrix) d2scene.PatternPaint {
	root := d2scene.NewNode(nil)
	half := tile.Width / 2
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: half, Height: tile.Height}, Fill: red}),
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: half, Width: half, Height: tile.Height}, Fill: blue}),
	}
	return d2scene.PatternPaint{Tile: tile, Root: root, Units: units, Transform: transform}
}

func patternDocument(width, height float64, paint d2scene.Paint) *d2scene.Document {
	return d2scene.NewDocument(
		d2scene.Box{Width: width, Height: height},
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: width, Height: height}, Fill: paint}),
	)
}

func BenchmarkRenderPatternPaint(b *testing.B) {
	document := patternDocument(512, 512, stripedPattern(
		d2scene.UserSpaceOnUse,
		d2scene.Box{Width: 16, Height: 16},
		d2scene.Identity(),
	))
	options := testOptions()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		frame, err := Render(ctx, document, options)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkFrame = frame
	}
}
