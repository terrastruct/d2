package d2raster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func bandEquivalenceDocument() *d2scene.Document {
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 3.125, Y: 4.375, Width: 85.25, Height: 106.75}, Fill: red}),
		d2scene.NewNode(d2scene.Ellipse{Center: d2scene.Point{X: 48.25, Y: 63.5}, RadiusX: 33.75, RadiusY: 54.125, Fill: d2scene.SolidPaint{Color: color.NRGBA{B: 255, A: 137}}}),
		d2scene.NewNode(d2scene.Path{Fill: green, Commands: []d2scene.PathCommand{
			d2scene.MoveTo(5.25, 109.5), d2scene.CubicTo(85.25, 7.125, 9.25, 100.125, 90.75, 4.375), d2scene.LineTo(73.75, 117.5), d2scene.ClosePath(),
		}}),
	}
	return d2scene.NewDocument(d2scene.Box{Width: 96, Height: 127}, root)
}

func TestRenderBandsMatchesFullFrame(t *testing.T) {
	for _, background := range []color.Color{nil, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, color.NRGBA{R: 20, G: 60, B: 100, A: 137}} {
		for _, scale := range []float64{1, 1.25} {
			options := testOptions()
			options.Background, options.Scale = background, scale
			document := bandEquivalenceDocument()
			want, err := Render(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			for _, height := range []int{1, 7, 32, 127, 500} {
				t.Run(fmt.Sprintf("background-%v/scale-%g/height-%d", background, scale, height), func(t *testing.T) {
					got := image.NewNRGBA(want.Bounds())
					nextY := 0
					var firstPixel *uint8
					err := RenderBands(context.Background(), document, options, height, func(band *image.NRGBA) error {
						if band.Bounds() != image.Rect(0, nextY, want.Bounds().Dx(), min(nextY+height, want.Bounds().Dy())) {
							t.Fatalf("band bounds = %v at row %d", band.Bounds(), nextY)
						}
						if firstPixel == nil {
							firstPixel = &band.Pix[0]
						} else if firstPixel != &band.Pix[0] {
							t.Fatal("band canvas storage was not reused")
						}
						draw.Draw(got, band.Bounds(), band, band.Bounds().Min, draw.Src)
						nextY = band.Bounds().Max.Y
						return nil
					})
					if err != nil {
						t.Fatal(err)
					}
					if nextY != want.Bounds().Dy() {
						t.Fatalf("received %d rows, want %d", nextY, want.Bounds().Dy())
					}
					assertBandPixels(t, got, want)
				})
			}
		}
	}
}

func TestRenderBandsEffectsMatchFullFrame(t *testing.T) {
	for _, mode := range []string{"blur", "shadow", "chain", "clip-mask", "even-odd", "gradient"} {
		t.Run(mode, func(t *testing.T) {
			document := bandEquivalenceDocument()
			node := document.Root
			switch mode {
			case "blur":
				node.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 2.5, SigmaY: 3.5}}
			case "shadow":
				node.Filters = []d2scene.Filter{d2scene.DropShadow{OffsetX: 2.5, OffsetY: -7.25, SigmaX: 2, SigmaY: 4, Color: color.NRGBA{A: 173}}}
			case "chain":
				node.Filters = []d2scene.Filter{d2scene.DropShadow{OffsetX: -2.5, OffsetY: 6.75, SigmaX: 1, SigmaY: 3, Color: color.NRGBA{A: 173}}, d2scene.GaussianBlur{SigmaX: 1.5, SigmaY: 2.5}}
			case "clip-mask":
				node.Opacity = .7
				node.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Transform: d2scene.Identity(), Root: d2scene.NewNode(d2scene.Ellipse{Center: d2scene.Point{X: 48, Y: 64}, RadiusX: 40, RadiusY: 56, Fill: red})}
				node.Clip = &d2scene.Clip{Transform: d2scene.Identity(), Path: d2scene.Path{Commands: []d2scene.PathCommand{d2scene.MoveTo(5, 9), d2scene.LineTo(90, 9), d2scene.LineTo(85, 110), d2scene.ClosePath()}}}
			case "even-odd":
				node.Children[2].Primitive = d2scene.Path{Fill: blue, FillRule: d2scene.EvenOdd, Commands: []d2scene.PathCommand{d2scene.MoveTo(3, 3), d2scene.LineTo(91, 5), d2scene.LineTo(88, 123), d2scene.LineTo(9, 125), d2scene.ClosePath(), d2scene.MoveTo(21, 21), d2scene.LineTo(77, 18), d2scene.LineTo(73, 111), d2scene.LineTo(23, 112), d2scene.ClosePath()}}
			case "gradient":
				node.Children[0].Primitive = d2scene.Rect{Box: d2scene.Box{X: 3, Y: 4, Width: 86, Height: 109}, Fill: d2scene.LinearGradient{Transform: d2scene.Identity(), Start: d2scene.Point{}, End: d2scene.Point{X: 96, Y: 127}, Stops: gradientStops}}
			}
			options := testOptions()
			want, err := Render(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			for _, height := range []int{1, 7, 32} {
				got := image.NewNRGBA(want.Bounds())
				if err := RenderBands(context.Background(), document, options, height, func(band *image.NRGBA) error {
					draw.Draw(got, band.Bounds(), band, band.Bounds().Min, draw.Src)
					return nil
				}); err != nil {
					t.Fatalf("height %d: %v", height, err)
				}
				assertBandPixels(t, got, want)
			}
		})
	}
}

func assertBandPixels(t *testing.T, got, want *image.NRGBA) {
	t.Helper()
	if !bytes.Equal(got.Pix, want.Pix) {
		for i := range got.Pix {
			if got.Pix[i] != want.Pix[i] {
				x, y := i/4%got.Bounds().Dx(), i/got.Stride
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got.NRGBAAt(x, y), want.NRGBAAt(x, y))
			}
		}
		t.Fatal("band pixels differ")
	}
}

func TestRenderBandsStopsAfterConsumerErrorOrCancellation(t *testing.T) {
	for _, cancelInstead := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		failure := errors.New("consumer failure")
		calls := 0
		err := RenderBands(ctx, bandEquivalenceDocument(), testOptions(), 7, func(*image.NRGBA) error {
			calls++
			if cancelInstead {
				cancel()
				return nil
			}
			return failure
		})
		want := failure
		if cancelInstead {
			want = context.Canceled
		}
		if !errors.Is(err, want) || calls != 1 {
			t.Fatalf("cancel=%v: error=%v, calls=%d", cancelInstead, err, calls)
		}
	}
}

func TestRenderBandsPreflightsLimitsBeforeCallbacks(t *testing.T) {
	document := bandEquivalenceDocument()
	prepared, err := prepareWithSessionBands(context.Background(), document, testOptions(), nil, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []string{"pixels", "scratch", "work", "band"} {
		options := testOptions()
		height := 7
		switch limit {
		case "pixels":
			options.MaxPixels = int64(prepared.width*prepared.height - 1)
		case "scratch":
			options.MaxOffscreenBytes = prepared.resources.peakOffscreenBytes - 1
		case "work":
			options.MaxScanlineWork = prepared.resources.scanlineWork - 1
		case "band":
			height = 0
		}
		calls := 0
		err := RenderBands(context.Background(), document, options, height, func(*image.NRGBA) error { calls++; return nil })
		if err == nil || calls != 0 {
			t.Fatalf("%s: error=%v callbacks=%d", limit, err, calls)
		}
	}
}

func TestRenderBandsScratchIndependentOfHeight(t *testing.T) {
	options := testOptions()
	options.MaxHeight, options.MaxPixels = 100_000, 10_000_000
	options.MaxOffscreenBytes = 1 << 20
	root := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 64, Height: 100_000}, Fill: red})
	root.Opacity = .7
	document := d2scene.NewDocument(d2scene.Box{Width: 64, Height: 100_000}, root)
	if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "offscreen") {
		t.Fatalf("full frame error = %v, want offscreen limit", err)
	}
	calls := 0
	if err := RenderBands(context.Background(), document, options, 32, func(band *image.NRGBA) error {
		calls++
		if cap(band.Pix) > 64*32*4 {
			t.Fatalf("canvas capacity = %d", cap(band.Pix))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != int(math.Ceil(100_000.0/32)) {
		t.Fatalf("callbacks = %d", calls)
	}
}

func TestRenderBandsFractionalGeometry(t *testing.T) {
	root := d2scene.NewNode(nil)
	for i := 0; i < 17; i++ {
		x := float64(i)*2.123 + .137
		node := d2scene.NewNode(d2scene.Ellipse{Center: d2scene.Point{X: 37.123 + x, Y: 137.531 + x*7.193}, RadiusX: 29.931, RadiusY: 77.417, Fill: d2scene.SolidPaint{Color: color.NRGBA{R: byte(i * 17), B: byte(i * 7), A: 193}}})
		node.Transform = d2scene.Matrix{A: .913, B: .037, C: -.041, D: 1.013, E: 3.127, F: -2.571}
		root.Children = append(root.Children, node)
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 127, Height: 511}, root)
	options := testOptions()
	want, err := Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, height := range []int{1, 7, 64} {
		got := image.NewNRGBA(want.Bounds())
		if err := RenderBands(context.Background(), document, options, height, func(band *image.NRGBA) error {
			draw.Draw(got, band.Bounds(), band, band.Bounds().Min, draw.Src)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		assertBandPixels(t, got, want)
	}
}

func TestRenderBandsPatternAndSessionAssets(t *testing.T) {
	pattern := stripedPattern(d2scene.UserSpaceOnUse, d2scene.Box{X: -.25, Y: 1.5, Width: 7, Height: 5}, d2scene.Identity())
	document := patternDocument(97, 129, pattern)
	options := testOptions()
	want, err := Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	got := image.NewNRGBA(want.Bounds())
	if err := RenderBands(nil, document, options, 7, func(band *image.NRGBA) error {
		draw.Draw(got, band.Bounds(), band, band.Bounds().Min, draw.Src)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertBandPixels(t, got, want)

	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	source.SetNRGBA(1, 1, color.NRGBA{B: 255, A: 127})
	document = rasterImageDocument("image/png", encodeRasterPNG(t, source), 2, 2, d2scene.Box{Width: 97, Height: 129}, d2scene.AspectRatio{})
	session := newTestRenderSession(t, RenderSessionOptions{MaxCacheEntries: 8, MaxCacheBytes: 1 << 20, MaxConcurrentLoads: 1})
	want, err = Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	got = image.NewNRGBA(want.Bounds())
	if err := session.RenderBands(nil, document, options, 1, func(band *image.NRGBA) error {
		draw.Draw(got, band.Bounds(), band, band.Bounds().Min, draw.Src)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertBandPixels(t, got, want)
	stats := session.Stats()
	if stats.Misses != 1 || stats.Hits != 0 {
		t.Fatalf("asset preparation repeated for bands: stats=%+v", stats)
	}
}

func TestRenderBandsInvalidArguments(t *testing.T) {
	document := bandEquivalenceDocument()
	consume := func(*image.NRGBA) error { t.Fatal("unexpected callback"); return nil }
	if err := RenderBands(nil, document, testOptions(), 7, nil); err == nil {
		t.Fatal("nil consumer accepted")
	}
	var session *RenderSession
	if err := session.RenderBands(nil, document, testOptions(), 7, consume); err == nil {
		t.Fatal("nil session accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RenderBands(ctx, document, testOptions(), 7, consume); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v", err)
	}
	options := testOptions()
	options.MaxEvenOddClipWork = 1
	document.Root = d2scene.NewNode(d2scene.Path{Fill: red, FillRule: d2scene.EvenOdd, Commands: []d2scene.PathCommand{d2scene.MoveTo(0, 0), d2scene.LineTo(80, 0), d2scene.LineTo(80, 100), d2scene.ClosePath()}})
	if err := RenderBands(nil, document, options, 7, consume); err == nil || !strings.Contains(err.Error(), "even-odd clip work") {
		t.Fatalf("even-odd limit error = %v", err)
	}
}

func BenchmarkRenderBandsCanvasMemory(b *testing.B) {
	const width, height = 2048, 4096
	document := d2scene.NewDocument(d2scene.Box{Width: width, Height: height}, d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 20, Y: 20, Width: width - 40, Height: height - 40}, Fill: red}))
	options := testOptions()
	options.MaxWidth, options.MaxHeight, options.MaxPixels = width, height, width*height
	options.Background = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			frame, err := Render(context.Background(), document, options)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkFrame = frame
		}
	})
	b.Run("256-rows", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := RenderBands(context.Background(), document, options, 256, func(band *image.NRGBA) error { return nil }); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestRenderBandsTallRoundedStrokeMatchesFullFrame(t *testing.T) {
	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 2.13, Y: 3.17, Width: 33.73, Height: 32_750.41}, RadiusX: 13.27, RadiusY: 21.39, Fill: red, Stroke: &d2scene.Stroke{Paint: blue, Width: 2.73, MiterLimit: 4}})
	node.Opacity = .73
	document := d2scene.NewDocument(d2scene.Box{Width: 41, Height: 32_767}, node)
	options := testOptions()
	options.MaxHeight, options.MaxPixels = 32_767, 2_000_000
	want, err := Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, height := range []int{37, 256} {
		got := image.NewNRGBA(want.Bounds())
		if err := RenderBands(context.Background(), document, options, height, func(band *image.NRGBA) error {
			draw.Draw(got, band.Bounds(), band, band.Bounds().Min, draw.Src)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		assertBandPixels(t, got, want)
	}
}
