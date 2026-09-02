package d2raster

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

var gradientStops = []d2scene.GradientStop{
	{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
	{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
}

func TestNormalizeGradientStopsUsesSVGSourceOrderRules(t *testing.T) {
	alreadyNormalized := []d2scene.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 1, A: 255}},
		{Offset: .75, Color: color.NRGBA{R: 2, A: 255}},
		{Offset: .75, Color: color.NRGBA{R: 3, A: 255}},
		{Offset: 1, Color: color.NRGBA{R: 4, A: 255}},
	}
	borrowed, err := normalizeGradientStops(alreadyNormalized)
	if err != nil {
		t.Fatal(err)
	}
	if &borrowed[0] != &alreadyNormalized[0] {
		t.Fatal("already-normalized immutable stops were copied")
	}

	input := []d2scene.GradientStop{
		{Offset: -2, Color: color.NRGBA{R: 1, A: 255}},
		{Offset: .75, Color: color.NRGBA{R: 2, A: 255}},
		{Offset: .25, Color: color.NRGBA{R: 3, A: 255}},
		{Offset: 2, Color: color.NRGBA{R: 4, A: 255}},
	}
	got, err := normalizeGradientStops(input)
	if err != nil {
		t.Fatal(err)
	}
	wantOffsets := []float64{0, .75, .75, 1}
	for index, want := range wantOffsets {
		if got[index].Offset != want {
			t.Fatalf("stop %d offset = %g, want %g", index, got[index].Offset, want)
		}
	}
	if input[0].Offset != -2 || input[2].Offset != .25 || input[3].Offset != 2 {
		t.Fatalf("normalization mutated caller stops: %#v", input)
	}
	if got := interpolateGradientStops(got, .75); got.R != 3 {
		t.Fatalf("last repeated stop does not own exact offset: %#v", got)
	}
	if got := interpolateGradientStops(got, math.Nextafter(.75, 0)); got.R != 2 {
		t.Fatalf("first repeated stop does not own left approach: %#v", got)
	}

	for name, stops := range map[string][]d2scene.GradientStop{
		"empty":        nil,
		"not a number": {{Offset: math.NaN()}},
		"infinity":     {{Offset: math.Inf(1)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeGradientStops(stops); err == nil {
				t.Fatal("normalizeGradientStops unexpectedly succeeded")
			}
		})
	}
}

func BenchmarkNormalizeGradientStops(b *testing.B) {
	for _, test := range []struct {
		name  string
		stops []d2scene.GradientStop
	}{
		{name: "AlreadyNormalized", stops: []d2scene.GradientStop{{Offset: 0}, {Offset: .25}, {Offset: .75}, {Offset: 1}}},
		{name: "NeedsNormalization", stops: []d2scene.GradientStop{{Offset: -1}, {Offset: .75}, {Offset: .25}, {Offset: 2}}},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := normalizeGradientStops(test.stops); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestCOLRv1GradientUsesPremultipliedLinearLight(t *testing.T) {
	t.Parallel()

	blackToWhite := []d2scene.GradientStop{
		{Offset: 0, Color: color.NRGBA{A: 255}},
		{Offset: 1, Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	}
	if got, want := interpolateCOLRv1GradientStops(blackToWhite, .5), (color.NRGBA{R: 188, G: 188, B: 188, A: 255}); got != want {
		t.Fatalf("linear-light midpoint = %#v, want %#v", got, want)
	}
	if got, want := interpolateGradientStops(blackToWhite, .5), (color.NRGBA{R: 128, G: 128, B: 128, A: 255}); got != want {
		t.Fatalf("SVG sRGB midpoint changed = %#v, want %#v", got, want)
	}

	opaqueRedToTransparentBlue := []d2scene.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255}},
	}
	if got, want := interpolateCOLRv1GradientStops(opaqueRedToTransparentBlue, .5), (color.NRGBA{R: 255, A: 128}); got != want {
		t.Fatalf("premultiplied-alpha midpoint = %#v, want %#v", got, want)
	}
	if got, want := averageCOLRv1GradientColor(opaqueRedToTransparentBlue), (color.NRGBA{R: 255, A: 128}); got != want {
		t.Fatalf("premultiplied-alpha average = %#v, want %#v", got, want)
	}
}

func TestLinearSRGBByteTableMatchesScalarConversion(t *testing.T) {
	t.Parallel()

	for value := range 256 {
		want := srgbToLinear(float64(value) / 255)
		if math.Float64bits(linearSRGBByte[value]) != math.Float64bits(want) {
			t.Fatalf("linearSRGBByte[%d] = %.17g, want bit-identical %.17g", value, linearSRGBByte[value], want)
		}
	}
}

func TestPremultipliedSRGBByteToLinearMatchesScalarConversion(t *testing.T) {
	t.Parallel()

	for alpha := range 256 {
		for channel := range 256 {
			want := 0.0
			if alpha != 0 {
				want = srgbToLinear(float64(channel) / float64(alpha))
			}
			got := premultipliedSRGBByteToLinear(uint8(channel), uint8(alpha))
			if math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("premultipliedSRGBByteToLinear(%d, %d) = %.17g, want bit-identical %.17g", channel, alpha, got, want)
			}
		}
	}
}

func BenchmarkCOLRv1GradientInterpolation(b *testing.B) {
	stops := []d2scene.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 12, G: 34, B: 56, A: 255}},
		{Offset: .35, Color: color.NRGBA{R: 231, G: 19, B: 101, A: 173}},
		{Offset: 1, Color: color.NRGBA{R: 7, G: 211, B: 249, A: 61}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	var result color.NRGBA
	for index := range b.N {
		result = interpolateCOLRv1GradientStops(stops, float64(index%10_000)/9_999)
	}
	benchmarkGradientColor = result
}

var benchmarkGradientColor color.NRGBA

func TestSpreadMethodsCoverPositiveAndNegativePeriods(t *testing.T) {
	tests := []struct {
		spread d2scene.SpreadMethod
		value  float64
		want   float64
	}{
		{d2scene.SpreadPad, -2.25, 0},
		{d2scene.SpreadPad, 1.25, 1},
		{d2scene.SpreadRepeat, -2.25, .75},
		{d2scene.SpreadRepeat, 2.25, .25},
		{d2scene.SpreadRepeat, 1, 0},
		{d2scene.SpreadReflect, -2.25, .25},
		{d2scene.SpreadReflect, 1.25, .75},
		{d2scene.SpreadReflect, 2.25, .25},
		{d2scene.SpreadReflect, 1, 1},
	}
	for _, test := range tests {
		if got := spreadParameter(test.value, test.spread); math.Abs(got-test.want) > 1e-12 {
			t.Errorf("spreadParameter(%g, %d) = %g, want %g", test.value, test.spread, got, test.want)
		}
	}
}

func TestSpreadParameterFastPathMatchesScalarFormula(t *testing.T) {
	t.Parallel()

	values := []float64{
		math.Copysign(0, -1), 0, math.SmallestNonzeroFloat64,
		math.Nextafter(1, 0), 1, math.Nextafter(1, 2),
		-2.25, -.5, 2.25, math.MaxFloat64, -math.MaxFloat64,
	}
	for index := -10_000; index <= 10_000; index++ {
		values = append(values, float64(index)/97)
	}
	for _, spread := range []d2scene.SpreadMethod{d2scene.SpreadReflect, d2scene.SpreadRepeat} {
		for _, value := range values {
			got := spreadParameter(value, spread)
			want := referenceSpreadParameter(value, spread)
			if math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("spreadParameter(%g, %d) = %g (%x), want %g (%x)", value, spread, got, math.Float64bits(got), want, math.Float64bits(want))
			}
		}
	}
}

func TestConcentricRadialFastPathMatchesReferenceParameter(t *testing.T) {
	paint, err := prepareRadialGradient(d2scene.RadialGradient{
		Center: d2scene.Point{X: 17.25, Y: -8.5}, Focal: d2scene.Point{X: 17.25, Y: -8.5}, Radius: 31.75,
		Stops: gradientStops, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}, d2scene.Box{Width: 100, Height: 100}, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	gradient := &paint.gradient
	if !gradient.radialConcentric {
		t.Fatal("concentric radial gradient did not select fast path")
	}
	state := uint64(1)
	points := []d2scene.Point{gradient.radialFocal, {X: math.SmallestNonzeroFloat64}, {X: math.MaxFloat64 / 4, Y: math.MaxFloat64 / 4}}
	for range 100_000 {
		state = state*6364136223846793005 + 1442695040888963407
		x := float64(int32(state>>32)) / 65536
		state = state*6364136223846793005 + 1442695040888963407
		y := float64(int32(state>>32)) / 65536
		points = append(points, d2scene.Point{X: x, Y: y})
	}
	for _, point := range points {
		got, gotOK := gradient.radialParameter(point)
		want, wantOK := referenceRadialParameter(gradient, point)
		if gotOK != wantOK || math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("point %#v: fast = %g/%v (%x), reference = %g/%v (%x)", point, got, gotOK, math.Float64bits(got), want, wantOK, math.Float64bits(want))
		}
	}
}

func referenceRadialParameter(gradient *preparedGradient, point d2scene.Point) (float64, bool) {
	qx := point.X - gradient.radialFocal.X
	qy := point.Y - gradient.radialFocal.Y
	b := qx*gradient.radialDelta.X + qy*gradient.radialDelta.Y + gradient.radialFocalRadius*gradient.radialDeltaRadius
	c := qx*qx + qy*qy - gradient.radialFocalRadius*gradient.radialFocalRadius
	a := gradient.radialA
	discriminant := b*b - a*c
	if discriminant < 0 {
		discriminantScale := math.Abs(b*b) + math.Abs(a*c) + 1
		if discriminant >= -1e-14*discriminantScale {
			discriminant = 0
		} else {
			return 0, false
		}
	}
	root := math.Sqrt(discriminant)
	for _, parameter := range [...]float64{(b + root) / a, (b - root) / a} {
		if gradient.radialSolutionValid(parameter) {
			return parameter, true
		}
	}
	return 0, false
}

func referenceSpreadParameter(value float64, spread d2scene.SpreadMethod) float64 {
	switch spread {
	case d2scene.SpreadReflect:
		value = math.Mod(value, 2)
		if value < 0 {
			value += 2
		}
		if value > 1 {
			value = 2 - value
		}
		return value
	case d2scene.SpreadRepeat:
		return value - math.Floor(value)
	default:
		return spreadParameter(value, spread)
	}
}

func TestLinearGradientObjectBoundingBoxFillAndStroke(t *testing.T) {
	gradient := d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
	}
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 10, Y: 5, Width: 80, Height: 10}, Fill: gradient}),
		d2scene.NewNode(d2scene.Rect{
			Box:    d2scene.Box{X: 10, Y: 25, Width: 80, Height: 20},
			Stroke: &d2scene.Stroke{Paint: gradient, Width: 4, Join: d2scene.JoinMiter},
		}),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 100, Height: 50}, root)
	got, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertColorNear(t, got.NRGBAAt(20, 10), color.NRGBA{R: 222, B: 33, A: 255}, 3)
	assertColorNear(t, got.NRGBAAt(80, 10), color.NRGBA{R: 30, B: 225, A: 255}, 3)
	assertColorNear(t, got.NRGBAAt(20, 25), color.NRGBA{R: 222, B: 33, A: 255}, 3)
	assertColorNear(t, got.NRGBAAt(80, 25), color.NRGBA{R: 30, B: 225, A: 255}, 3)
	assertPixel(t, got.NRGBAAt(50, 20), color.NRGBA{})
}

func TestAxisLinearGradientFastPathsMatchGeneralSampling(t *testing.T) {
	for _, test := range []struct {
		name      string
		start     d2scene.Point
		end       d2scene.Point
		transform d2scene.Matrix
		wantKind  preparedPaintKind
	}{
		{name: "horizontal transformed", start: d2scene.Point{X: -3, Y: 7}, end: d2scene.Point{X: 11, Y: 7}, transform: d2scene.Rotate(.17).Mul(d2scene.Translate(4, -9)), wantKind: preparedLinearXGradient},
		{name: "vertical transformed", start: d2scene.Point{X: -3, Y: 7}, end: d2scene.Point{X: -3, Y: 19}, transform: d2scene.Rotate(.17).Mul(d2scene.Translate(4, -9)), wantKind: preparedLinearYGradient},
		{name: "horizontal axis aligned", start: d2scene.Point{X: -3, Y: 7}, end: d2scene.Point{X: 11, Y: 7}, transform: d2scene.Identity(), wantKind: preparedLinearXOnlyGradient},
		{name: "vertical axis aligned", start: d2scene.Point{X: -3, Y: 7}, end: d2scene.Point{X: -3, Y: 19}, transform: d2scene.Identity(), wantKind: preparedLinearYOnlyGradient},
	} {
		t.Run(test.name, func(t *testing.T) {
			paint, err := prepareLinearGradient(d2scene.LinearGradient{
				Start: test.start, End: test.end, Stops: gradientStops,
				Units: d2scene.UserSpaceOnUse, Transform: test.transform,
			}, d2scene.Box{Width: 100, Height: 100}, d2scene.Scale(1.3, .7))
			if err != nil {
				t.Fatal(err)
			}
			if paint.kind != test.wantKind {
				t.Fatalf("prepared kind = %d, want %d", paint.kind, test.wantKind)
			}
			general := *paint
			general.kind = preparedLinearGradient
			assertSample := func(x, y float64) {
				t.Helper()
				got, gotOK := paint.colorAt(x, y)
				want, wantOK := general.colorAt(x, y)
				if gotOK != wantOK || got != want {
					t.Fatalf("point (%g,%g): fast = %#v/%v, general = %#v/%v", x, y, got, gotOK, want, wantOK)
				}
			}
			maximumCoordinate := float64(^uint(0) >> 1)
			for _, point := range []d2scene.Point{
				{X: maximumCoordinate, Y: maximumCoordinate},
				{X: -maximumCoordinate, Y: -maximumCoordinate},
				{X: maximumCoordinate, Y: -maximumCoordinate},
			} {
				assertSample(point.X, point.Y)
			}
			state := uint64(1)
			for range 100_000 {
				state = state*6364136223846793005 + 1442695040888963407
				x := float64(int32(state>>32)) / 65536
				state = state*6364136223846793005 + 1442695040888963407
				y := float64(int32(state>>32)) / 65536
				assertSample(x, y)
			}

			bounds := image.Rect(-17, -11, 19, 23)
			mask := image.NewAlpha(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
			gotFrame := image.NewRGBA(bounds)
			for index := range mask.Pix {
				state = state*6364136223846793005 + 1442695040888963407
				mask.Pix[index] = uint8(state >> 56)
			}
			for index := range gotFrame.Pix {
				state = state*6364136223846793005 + 1442695040888963407
				gotFrame.Pix[index] = uint8(state >> 56)
			}
			wantFrame := image.NewRGBA(bounds)
			copy(wantFrame.Pix, gotFrame.Pix)
			if err := drawAxisLinearGradientMaskPixels(context.Background(), gotFrame, bounds, mask, &paint.gradient, paint.kind); err != nil {
				t.Fatal(err)
			}
			if err := drawPaintMaskPixels(context.Background(), wantFrame, bounds, mask, paint); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(gotFrame.Pix, wantFrame.Pix) {
				t.Fatal("axis mask fast path differs from per-pixel paint sampling")
			}
		})
	}
}

func TestAxisLinearGradientFastPathsRejectPotentialCoordinateOverflow(t *testing.T) {
	for _, test := range []struct {
		name      string
		start     d2scene.Point
		end       d2scene.Point
		transform d2scene.Matrix
		point     d2scene.Point
		fastKind  preparedPaintKind
	}{
		{
			name: "horizontal transformed", start: d2scene.Point{}, end: d2scene.Point{X: 1},
			transform: d2scene.Matrix{A: 1, C: -1e-308, D: 1e-308}, point: d2scene.Point{X: -1.5, Y: 2},
			fastKind: preparedLinearXGradient,
		},
		{
			name: "vertical transformed", start: d2scene.Point{}, end: d2scene.Point{Y: 1},
			transform: d2scene.Matrix{A: 1e-308, B: -1e-308, D: 1}, point: d2scene.Point{X: 2, Y: -1.5},
			fastKind: preparedLinearYGradient,
		},
		{
			name: "horizontal axis aligned", start: d2scene.Point{}, end: d2scene.Point{X: 1},
			transform: d2scene.Scale(1, 1e-308), point: d2scene.Point{X: .5, Y: 2},
			fastKind: preparedLinearXOnlyGradient,
		},
		{
			name: "vertical axis aligned", start: d2scene.Point{}, end: d2scene.Point{Y: 1},
			transform: d2scene.Scale(1e-308, 1), point: d2scene.Point{X: 2, Y: .5},
			fastKind: preparedLinearYOnlyGradient,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			paint, err := prepareLinearGradient(d2scene.LinearGradient{
				Start: test.start, End: test.end, Stops: gradientStops,
				Units: d2scene.UserSpaceOnUse, Transform: test.transform,
			}, d2scene.Box{Width: 1, Height: 1}, d2scene.Identity())
			if err != nil {
				t.Fatal(err)
			}
			if paint.kind != preparedLinearGradient {
				t.Fatalf("prepared kind = %d, want overflow-safe general kind %d", paint.kind, preparedLinearGradient)
			}
			got, gotOK := paint.colorAt(test.point.X, test.point.Y)
			if gotOK || got != (color.NRGBA{}) {
				t.Fatalf("overflowing unused coordinate = %#v/%v, want transparent/unpainted", got, gotOK)
			}

			// This forced kind models the optimization without its preparation
			// guard and proves that the pathological sample exercises the exact
			// behavior the guard protects.
			unsafeFast := *paint
			unsafeFast.kind = test.fastKind
			if _, ok := unsafeFast.colorAt(test.point.X, test.point.Y); !ok {
				t.Fatal("forced unsafe fast path unexpectedly remained unpainted")
			}
		})
	}
}

func TestLinearGradientMaskFastPathMatchesPaintSampling(t *testing.T) {
	paint, err := prepareLinearGradient(d2scene.LinearGradient{
		Start: d2scene.Point{X: -.25, Y: .1}, End: d2scene.Point{X: 1.1, Y: .9}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.17), Spread: d2scene.SpreadRepeat,
	}, d2scene.Box{X: -17, Y: -11, Width: 36, Height: 34}, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if paint.kind != preparedLinearGradient {
		t.Fatalf("prepared kind = %d, want general linear kind", paint.kind)
	}
	bounds := image.Rect(-17, -11, 19, 23)
	mask := image.NewAlpha(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	gotFrame := image.NewRGBA(bounds)
	state := uint64(1)
	for index := range mask.Pix {
		state = state*6364136223846793005 + 1442695040888963407
		mask.Pix[index] = uint8(state >> 56)
	}
	for index := range gotFrame.Pix {
		state = state*6364136223846793005 + 1442695040888963407
		gotFrame.Pix[index] = uint8(state >> 56)
	}
	wantFrame := image.NewRGBA(bounds)
	copy(wantFrame.Pix, gotFrame.Pix)
	if err := drawLinearGradientMaskPixels(context.Background(), gotFrame, bounds, mask, &paint.gradient); err != nil {
		t.Fatal(err)
	}
	if err := drawPaintMaskPixels(context.Background(), wantFrame, bounds, mask, paint); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotFrame.Pix, wantFrame.Pix) {
		t.Fatal("linear mask fast path differs from per-pixel paint sampling")
	}
}

func TestRadialGradientMaskFastPathMatchesPaintSampling(t *testing.T) {
	for name, source := range map[string]d2scene.RadialGradient{
		"transformed": {
			Center: d2scene.Point{X: .5, Y: .5}, Radius: .5,
			Focal: d2scene.Point{X: .4, Y: .45}, FocalRadius: .05,
			Stops: gradientStops, Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.1), Spread: d2scene.SpreadReflect,
		},
		"tangent repeat": {
			Center: d2scene.Point{X: 1}, Radius: 1,
			Focal: d2scene.Point{}, Stops: gradientStops,
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(), Spread: d2scene.SpreadRepeat,
		},
	} {
		t.Run(name, func(t *testing.T) {
			paint, err := prepareRadialGradient(source, d2scene.Box{X: -17, Y: -11, Width: 36, Height: 34}, d2scene.Identity())
			if err != nil {
				t.Fatal(err)
			}
			bounds := image.Rect(-17, -11, 19, 23)
			mask := image.NewAlpha(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
			gotFrame := image.NewRGBA(bounds)
			state := uint64(1)
			for index := range mask.Pix {
				state = state*6364136223846793005 + 1442695040888963407
				mask.Pix[index] = uint8(state >> 56)
			}
			for index := range gotFrame.Pix {
				state = state*6364136223846793005 + 1442695040888963407
				gotFrame.Pix[index] = uint8(state >> 56)
			}
			wantFrame := image.NewRGBA(bounds)
			copy(wantFrame.Pix, gotFrame.Pix)
			if err := drawRadialGradientMaskPixels(context.Background(), gotFrame, bounds, mask, &paint.gradient); err != nil {
				t.Fatal(err)
			}
			if err := drawPaintMaskPixels(context.Background(), wantFrame, bounds, mask, paint); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(gotFrame.Pix, wantFrame.Pix) {
				t.Fatal("radial mask fast path differs from per-pixel paint sampling")
			}
		})
	}
}

func TestGradientPaintsTextFillAndStroke(t *testing.T) {
	fontBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("embedded Source Sans Pro font is not loaded")
	}
	assetID := d2scene.AssetID("font:gradient")
	gradient := d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 170, Height: 55}, d2scene.NewNode(d2scene.TextRun{
		Text: "GRADIENT", Origin: d2scene.Point{X: 10, Y: 38}, Anchor: d2scene.AnchorStart,
		Font: d2scene.Font{Size: 30, Asset: assetID}, Fill: gradient,
		Stroke: &d2scene.Stroke{Paint: gradient, Width: .75, Join: d2scene.JoinRound},
		Ink:    d2scene.NewBounds(10, 8, 155, 42),
	}))
	document.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontBytes}
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	redDominant, blueDominant := false, false
	for y := 0; y < frame.Bounds().Dy(); y++ {
		for x := 0; x < frame.Bounds().Dx(); x++ {
			pixel := frame.NRGBAAt(x, y)
			if pixel.A < 200 {
				continue
			}
			redDominant = redDominant || pixel.R > pixel.B+40
			blueDominant = blueDominant || pixel.B > pixel.R+40
		}
	}
	if !redDominant || !blueDominant {
		t.Fatalf("text gradient lacks terminal colors: red=%v blue=%v", redDominant, blueDominant)
	}
}

func TestLinearGradientCoordinateSystemsAndTransforms(t *testing.T) {
	objectBounds := d2scene.Box{X: 10, Y: 20, Width: 40, Height: 20}
	objectTransform := d2scene.Translate(5, 7).Mul(d2scene.Scale(2, 3))
	objectGradient := d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Translate(.25, 0), Spread: d2scene.SpreadPad,
	}
	paint, err := prepareLinearGradient(objectGradient, objectBounds, objectTransform)
	if err != nil {
		t.Fatal(err)
	}
	// bbox maps the transformed vector to local x=20..60, then the object
	// transform maps that to device x=45..125.
	assertSampleNear(t, paint, 85, 67, color.NRGBA{R: 128, B: 128, A: 255}, 1)

	userGradient := d2scene.LinearGradient{
		Start: d2scene.Point{X: 10}, End: d2scene.Point{X: 50}, Stops: gradientStops,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Translate(5, 0), Spread: d2scene.SpreadPad,
	}
	paint, err = prepareLinearGradient(userGradient, objectBounds, objectTransform)
	if err != nil {
		t.Fatal(err)
	}
	// The user-space vector is shifted to local x=15..55, then transformed to
	// device x=35..115.
	assertSampleNear(t, paint, 75, 7, color.NRGBA{R: 128, B: 128, A: 255}, 1)

	reflected := d2scene.Translate(120, 0).Mul(d2scene.Scale(-1, 1))
	paint, err = prepareLinearGradient(d2scene.LinearGradient{
		Start: d2scene.Point{X: 0}, End: d2scene.Point{X: 100}, Stops: gradientStops,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
	}, objectBounds, reflected)
	if err != nil {
		t.Fatal(err)
	}
	assertSampleNear(t, paint, 70, 0, color.NRGBA{R: 128, B: 128, A: 255}, 1)
}

func TestLinearGradientSpreadRendering(t *testing.T) {
	for name, test := range map[string]struct {
		spread d2scene.SpreadMethod
		x      float64
		want   color.NRGBA
	}{
		"pad before":       {d2scene.SpreadPad, -2.5, color.NRGBA{R: 255, A: 255}},
		"pad after":        {d2scene.SpreadPad, 12.5, color.NRGBA{B: 255, A: 255}},
		"repeat negative":  {d2scene.SpreadRepeat, -2.5, color.NRGBA{R: 64, B: 191, A: 255}},
		"repeat positive":  {d2scene.SpreadRepeat, 12.5, color.NRGBA{R: 191, B: 64, A: 255}},
		"reflect negative": {d2scene.SpreadReflect, -2.5, color.NRGBA{R: 191, B: 64, A: 255}},
		"reflect positive": {d2scene.SpreadReflect, 12.5, color.NRGBA{R: 64, B: 191, A: 255}},
	} {
		t.Run(name, func(t *testing.T) {
			paint, err := prepareLinearGradient(d2scene.LinearGradient{
				Start: d2scene.Point{}, End: d2scene.Point{X: 10}, Stops: gradientStops,
				Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(), Spread: test.spread,
			}, d2scene.Box{Width: 1, Height: 1}, d2scene.Identity())
			if err != nil {
				t.Fatal(err)
			}
			assertSampleNear(t, paint, test.x, 0, test.want, 1)
		})
	}
}

func TestGradientInterpolationIsUnpremultipliedAndCompositesAlpha(t *testing.T) {
	got := lerpNRGBA(color.NRGBA{}, color.NRGBA{R: 255, A: 255}, .5)
	if got != (color.NRGBA{R: 128, A: 128}) {
		t.Fatalf("unpremultiplied interpolation = %#v", got)
	}
	destination := []byte{0, 255, 0, 255}
	compositeNRGBAOverRGBA(destination, got, 255)
	want := []byte{64, 127, 0, 255}
	if !bytes.Equal(destination, want) {
		t.Fatalf("alpha composite = %v, want %v", destination, want)
	}

	partial := []byte{0, 0, 0, 0}
	compositeNRGBAOverRGBA(partial, color.NRGBA{R: 240, G: 80, B: 20, A: 200}, 128)
	if partial[3] != 100 || partial[0] != 94 || partial[1] != 31 || partial[2] != 8 {
		t.Fatalf("coverage composite = %v", partial)
	}
}

func TestRadialGradientConcentricBBoxAndFocalCircle(t *testing.T) {
	paint, err := prepareRadialGradient(d2scene.RadialGradient{
		Center: d2scene.Point{X: .5, Y: .5}, Radius: .5,
		Focal: d2scene.Point{X: .5, Y: .5}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
	}, d2scene.Box{Width: 100, Height: 50}, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	assertSampleNear(t, paint, 50, 25, color.NRGBA{R: 255, A: 255}, 1)
	assertSampleNear(t, paint, 75, 25, color.NRGBA{R: 128, B: 128, A: 255}, 1)
	assertSampleNear(t, paint, 50, 37.5, color.NRGBA{R: 128, B: 128, A: 255}, 1)
	assertSampleNear(t, paint, 100, 25, color.NRGBA{B: 255, A: 255}, 1)

	paint, err = prepareRadialGradient(d2scene.RadialGradient{
		Center: d2scene.Point{}, Radius: 10,
		Focal: d2scene.Point{X: 2}, FocalRadius: 2, Stops: gradientStops,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
	}, d2scene.Box{Width: 1, Height: 1}, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	// At t=.5 the interpolated circle has center x=1 and radius 6, so x=7
	// lies exactly on its positive-x edge.
	assertSampleNear(t, paint, 7, 0, color.NRGBA{R: 128, B: 128, A: 255}, 1)
	assertSampleNear(t, paint, 2, 0, color.NRGBA{R: 255, A: 255}, 1)
}

func TestRadialGradientConeTransparencyAndDegenerateCircles(t *testing.T) {
	paint, err := prepareRadialGradient(d2scene.RadialGradient{
		Center: d2scene.Point{}, Radius: 10,
		Focal: d2scene.Point{X: 20}, Stops: gradientStops,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
	}, d2scene.Box{Width: 1, Height: 1}, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := paint.colorAt(30, 0); ok || got != (color.NRGBA{}) {
		t.Fatalf("outside-cone sample = %#v, %v, want transparent", got, ok)
	}
	if _, ok := paint.colorAt(20, 0); !ok {
		t.Fatal("focal point was not painted")
	}

	paint, err = prepareRadialGradient(d2scene.RadialGradient{
		Center: d2scene.Point{X: 4, Y: 5}, Radius: 3,
		Focal: d2scene.Point{X: 4, Y: 5}, FocalRadius: 3, Stops: gradientStops,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(), Spread: d2scene.SpreadReflect,
	}, d2scene.Box{Width: 1, Height: 1}, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := paint.colorAt(4, 5); ok || got != (color.NRGBA{}) {
		t.Fatalf("overlapping-circle sample = %#v, %v, want transparent", got, ok)
	}
}

func TestRadialGradientRepeatTangentUsesStopAverage(t *testing.T) {
	paint, err := prepareRadialGradient(d2scene.RadialGradient{
		Center: d2scene.Point{}, Radius: 10,
		Focal: d2scene.Point{X: 10}, Stops: gradientStops,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(), Spread: d2scene.SpreadRepeat,
	}, d2scene.Box{Width: 1, Height: 1}, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	// SVG 2 specifies the weighted stop average on the otherwise undefined
	// side of a tangent repeating radial gradient.
	assertSampleNear(t, paint, 20, 0, color.NRGBA{R: 128, B: 128, A: 255}, 1)
}

func TestGradientInvalidInputsFailDuringPreflight(t *testing.T) {
	validLinear := func() d2scene.LinearGradient {
		return d2scene.LinearGradient{
			Start: d2scene.Point{}, End: d2scene.Point{X: 1}, Stops: append([]d2scene.GradientStop(nil), gradientStops...),
			Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
		}
	}
	validRadial := func() d2scene.RadialGradient {
		return d2scene.RadialGradient{
			Center: d2scene.Point{X: .5, Y: .5}, Radius: .5, Focal: d2scene.Point{X: .5, Y: .5},
			Stops: append([]d2scene.GradientStop(nil), gradientStops...), Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
		}
	}
	tests := map[string]struct {
		paint     func() d2scene.Paint
		box       d2scene.Box
		transform d2scene.Matrix
		want      string
	}{
		"no stops":                    {func() d2scene.Paint { value := validLinear(); value.Stops = nil; return value }, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), "no stops"},
		"non-finite stop":             {func() d2scene.Paint { value := validLinear(); value.Stops[0].Offset = math.NaN(); return value }, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), "non-finite offset"},
		"invalid units":               {func() d2scene.Paint { value := validLinear(); value.Units = d2scene.PaintUnits(255); return value }, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), "invalid paint units"},
		"invalid spread":              {func() d2scene.Paint { value := validLinear(); value.Spread = d2scene.SpreadMethod(255); return value }, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), "invalid spread"},
		"singular gradient transform": {func() d2scene.Paint { value := validLinear(); value.Transform = d2scene.Scale(0, 1); return value }, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), "singular gradient transform"},
		"singular object transform":   {func() d2scene.Paint { return validLinear() }, d2scene.Box{Width: 10, Height: 10}, d2scene.Scale(1, 0), "singular gradient transform"},
		"zero bbox width":             {func() d2scene.Paint { return validLinear() }, d2scene.Box{Width: 0, Height: 10}, d2scene.Identity(), "zero width or height"},
		"zero bbox height":            {func() d2scene.Paint { return validLinear() }, d2scene.Box{Width: 10, Height: 0}, d2scene.Identity(), "zero width or height"},
		"linear overflow": {func() d2scene.Paint {
			value := validLinear()
			value.Units = d2scene.UserSpaceOnUse
			value.Start.X = -math.MaxFloat64
			value.End.X = math.MaxFloat64
			return value
		}, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), "finite numeric domain"},
		"negative radial radius":     {func() d2scene.Paint { value := validRadial(); value.Radius = -1; return value }, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), "negative radius"},
		"non-finite radial geometry": {func() d2scene.Paint { value := validRadial(); value.Focal.X = math.Inf(1); return value }, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), "non-finite geometry"},
		"radial overflow": {func() d2scene.Paint {
			value := validRadial()
			value.Units = d2scene.UserSpaceOnUse
			value.Center.X = math.MaxFloat64
			value.Focal.X = -math.MaxFloat64
			return value
		}, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), "finite numeric domain"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			document := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, d2scene.NewNode(d2scene.Rect{Box: test.box, Fill: test.paint()}))
			document.Root.Transform = test.transform
			_, err := prepare(context.Background(), document, testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("prepare() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestGradientPointerPaintsAndColorAnimationGate(t *testing.T) {
	gradient := &d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(),
	}
	if _, err := prepareAnimatedPaint(gradient, nil, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity()); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareAnimatedPaint((*d2scene.LinearGradient)(nil), nil, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity()); err == nil || !strings.Contains(err.Error(), "nil linear") {
		t.Fatalf("nil pointer error = %v", err)
	}
	animated := color.NRGBA{G: 255, A: 255}
	if _, err := prepareAnimatedPaint(gradient, &animated, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity()); err == nil || !strings.Contains(err.Error(), "non-solid") {
		t.Fatalf("gradient color animation error = %v", err)
	}
}

func TestZeroVectorGradientMatchesSolidFastPathExactly(t *testing.T) {
	last := color.NRGBA{R: 17, G: 91, B: 203, A: 177}
	box := d2scene.Box{X: 2, Y: 2, Width: 6, Height: 6}
	solidDocument := testDocument(d2scene.Rect{Box: box, Fill: d2scene.SolidPaint{Color: last}})
	gradientDocument := testDocument(d2scene.Rect{Box: box, Fill: d2scene.LinearGradient{
		Start: d2scene.Point{X: .5, Y: .5}, End: d2scene.Point{X: .5, Y: .5},
		Stops: []d2scene.GradientStop{{Offset: 0, Color: red.Color}, {Offset: 1, Color: last}},
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(),
	}})
	solidFrame, err := Render(context.Background(), solidDocument, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	gradientFrame, err := Render(context.Background(), gradientDocument, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(solidFrame.Pix, gradientFrame.Pix) {
		t.Fatal("zero-vector gradient diverged from equivalent solid rendering")
	}
}

func TestConstantGradientMaskDiffIsConfinedToAntialiasEdges(t *testing.T) {
	constant := color.NRGBA{R: 37, G: 149, B: 211, A: 255}
	box := d2scene.Box{X: 5.25, Y: 6.75, Width: 79.5, Height: 51.5}
	shape := func(fill d2scene.Paint) *d2scene.Document {
		return d2scene.NewDocument(d2scene.Box{Width: 100, Height: 70}, d2scene.NewNode(d2scene.Rect{
			Box: box, RadiusX: 9.5, RadiusY: 9.5, Fill: fill,
		}))
	}
	solidFrame, err := Render(context.Background(), shape(d2scene.SolidPaint{Color: constant}), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	gradientFrame, err := Render(context.Background(), shape(d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1},
		Stops: []d2scene.GradientStop{{Offset: 0, Color: constant}, {Offset: 1, Color: constant}},
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(),
	}), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	changed, maximum := 0, 0
	for y := 0; y < solidFrame.Bounds().Dy(); y++ {
		for x := 0; x < solidFrame.Bounds().Dx(); x++ {
			sr, sg, sb, sa := solidFrame.NRGBAAt(x, y).RGBA()
			gr, gg, gb, ga := gradientFrame.NRGBAAt(x, y).RGBA()
			for _, pair := range [][2]uint32{{sr, gr}, {sg, gg}, {sb, gb}, {sa, ga}} {
				difference := int(pair[0]>>8) - int(pair[1]>>8)
				if difference < 0 {
					difference = -difference
				}
				if difference != 0 {
					changed++
				}
				if difference > maximum {
					maximum = difference
				}
			}
		}
	}
	// The bounds-sized mask quantizes vector coverage to 8 bits once before
	// compositing, so premultiplied edge channels may differ by two from the
	// direct 16-bit uniform fast path. Interior pixels remain exact.
	if changed > 500 || maximum > 2 {
		t.Fatalf("constant-gradient diff changed %d channels with max delta %d", changed, maximum)
	}
}

func TestGradientLoopObservesCancellation(t *testing.T) {
	paint, err := prepareLinearGradient(d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 512}, Stops: gradientStops,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}, d2scene.Box{Width: 512, Height: 512}, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterContext{after: 1}
	dst := image.NewRGBA(image.Rect(0, 0, 512, 512))
	scratch := &rasterScratch{offscreen: offscreenBudget{limit: 512 * 512}}
	err = drawGradientMask(ctx, dst, dst.Bounds(), paint, scratch, func(mask *image.Alpha) error {
		for index := range mask.Pix {
			mask.Pix[index] = 255
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("drawGradientMask() error = %v, want context.Canceled", err)
	}
	if scratch.offscreen.live != 0 {
		t.Fatalf("gradient mask reservation after cancellation = %d bytes, want 0", scratch.offscreen.live)
	}
}

func TestStreamedGradientCoverageMatchesAlphaMaskRandomized(t *testing.T) {
	t.Parallel()

	const width, height = 67, 53
	bounds := image.Rect(11, 17, 11+width, 17+height)
	objectBounds := d2scene.Box{X: 11, Y: 17, Width: width, Height: height}
	linearAxis, err := prepareLinearGradient(d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(), Spread: d2scene.SpreadReflect,
	}, objectBounds, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	linearGeneral, err := prepareLinearGradient(d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1, Y: .3}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.17), Spread: d2scene.SpreadRepeat,
	}, objectBounds, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	radial, err := prepareRadialGradient(d2scene.RadialGradient{
		Center: d2scene.Point{X: .5, Y: .5}, Radius: .55,
		Focal: d2scene.Point{X: .37, Y: .41}, FocalRadius: .03,
		Stops: []d2scene.GradientStop{
			{Offset: 0, Color: color.NRGBA{R: 255, G: 200, A: 180}},
			{Offset: .55, Color: color.NRGBA{G: 180, B: 220, A: 240}},
			{Offset: 1, Color: color.NRGBA{R: 30, B: 120, A: 255}},
		},
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(-.11), Spread: d2scene.SpreadReflect,
	}, objectBounds, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	paints := []*preparedPaint{linearAxis, linearGeneral, radial}

	state := uint64(0xd1b54a32d192ed03)
	next := func() uint64 {
		state ^= state >> 12
		state ^= state << 25
		state ^= state >> 27
		return state * 0x2545f4914f6cdd1d
	}
	coordinate := func(limit int) float32 {
		return float32(float64(int64(next()%uint64(limit*8+32))-16) / 8)
	}
	type polygon [][2]float32

	for iteration := range 80 {
		var polygons []polygon
		for range int(next()%8) + 1 {
			points := make(polygon, int(next()%7)+2)
			for index := range points {
				points[index] = [2]float32{coordinate(width), coordinate(height)}
			}
			polygons = append(polygons, points)
		}
		populate := func(rasterizer *scanline.Rasterizer) error {
			for _, points := range polygons {
				rasterizer.MoveTo(points[0][0], points[0][1])
				for _, point := range points[1:] {
					rasterizer.LineTo(point[0], point[1])
				}
				rasterizer.ClosePath()
			}
			return nil
		}

		seed := image.NewRGBA(bounds)
		for offset := 0; offset < len(seed.Pix); offset += 4 {
			alpha := uint8(next())
			seed.Pix[offset+3] = alpha
			seed.Pix[offset+0] = uint8(next() % (uint64(alpha) + 1))
			seed.Pix[offset+1] = uint8(next() % (uint64(alpha) + 1))
			seed.Pix[offset+2] = uint8(next() % (uint64(alpha) + 1))
		}

		for paintIndex, paint := range paints {
			want := image.NewRGBA(bounds)
			copy(want.Pix, seed.Pix)
			wantScratch := &rasterScratch{offscreen: offscreenBudget{limit: math.MaxInt64}}
			err := drawPaintMask(context.Background(), want, bounds, paint, wantScratch, "oracle gradient Alpha mask", func(mask *image.Alpha) error {
				rasterizer := wantScratch.reset(mask.Bounds())
				if err := populate(rasterizer); err != nil {
					return err
				}
				return rasterizer.WriteAlpha(context.Background(), wantScratch.workBudget(), mask)
			})
			if err != nil {
				t.Fatalf("iteration %d paint %d Alpha-mask oracle: %v", iteration, paintIndex, err)
			}

			got := image.NewRGBA(bounds)
			copy(got.Pix, seed.Pix)
			gotScratch := &rasterScratch{offscreen: offscreenBudget{limit: math.MaxInt64}}
			if err := drawRasterizedPaint(context.Background(), got, bounds, paint, gotScratch, "gradient fill", populate); err != nil {
				t.Fatalf("iteration %d paint %d streamed coverage: %v", iteration, paintIndex, err)
			}
			if !bytes.Equal(got.Pix, want.Pix) {
				for index := range got.Pix {
					if got.Pix[index] != want.Pix[index] {
						t.Fatalf("iteration %d paint %d first differing byte %d: streamed=%d Alpha-mask=%d", iteration, paintIndex, index, got.Pix[index], want.Pix[index])
					}
				}
				t.Fatalf("iteration %d paint %d output lengths differ", iteration, paintIndex)
			}
		}
	}
}

func TestStreamedGradientCoverageObservesCancellation(t *testing.T) {
	const width = 8_192
	bounds := image.Rect(0, 0, width, 2)
	paint, err := prepareLinearGradient(d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: width}, Stops: gradientStops,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}, d2scene.Box{Width: width, Height: 2}, d2scene.Identity())
	if err != nil {
		t.Fatal(err)
	}
	scratch := &rasterScratch{offscreen: offscreenBudget{limit: math.MaxInt64}}
	ctx := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: 4}
	err = drawRasterizedPaint(ctx, image.NewRGBA(bounds), bounds, paint, scratch, "gradient fill", func(rasterizer *scanline.Rasterizer) error {
		rasterizer.MoveTo(0, 0)
		rasterizer.LineTo(width, 0)
		rasterizer.LineTo(width, 2)
		rasterizer.LineTo(0, 2)
		rasterizer.ClosePath()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("drawRasterizedPaint() error = %v, want context.Canceled", err)
	}
	if ctx.calls < 4 {
		t.Fatalf("context checks = %d, want cancellation during streamed row", ctx.calls)
	}
	if scratch.offscreen.live != 0 {
		t.Fatalf("offscreen bytes after cancellation = %d, want 0", scratch.offscreen.live)
	}
}

func BenchmarkRenderGradient(b *testing.B) {
	gradient := d2scene.RadialGradient{
		Center: d2scene.Point{X: .5, Y: .5}, Radius: .5,
		Focal: d2scene.Point{X: .4, Y: .45}, FocalRadius: .05,
		Stops: []d2scene.GradientStop{
			{Offset: 0, Color: color.NRGBA{R: 255, G: 240, B: 128, A: 230}},
			{Offset: .6, Color: color.NRGBA{R: 80, G: 120, B: 255, A: 180}},
			{Offset: 1, Color: color.NRGBA{R: 20, G: 10, B: 80, A: 255}},
		},
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.1), Spread: d2scene.SpreadReflect,
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 488, Height: 272}, d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 20, Y: 20, Width: 448, Height: 232}, RadiusX: 18, RadiusY: 18,
		Fill: gradient, Stroke: &d2scene.Stroke{Paint: gradient, Width: 4, Join: d2scene.JoinRound},
	}))
	options := testOptions()
	options.Scale = 2
	options.MaxWidth, options.MaxHeight, options.MaxPixels = 2_000, 2_000, 4_000_000
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

func BenchmarkRenderConcentricGradient(b *testing.B) {
	gradient := d2scene.RadialGradient{
		Center: d2scene.Point{X: .5, Y: .5}, Radius: .5,
		Focal: d2scene.Point{X: .5, Y: .5},
		Stops: []d2scene.GradientStop{
			{Offset: 0, Color: color.NRGBA{R: 255, G: 240, B: 128, A: 230}},
			{Offset: .6, Color: color.NRGBA{R: 80, G: 120, B: 255, A: 180}},
			{Offset: 1, Color: color.NRGBA{R: 20, G: 10, B: 80, A: 255}},
		},
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.1), Spread: d2scene.SpreadReflect,
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 488, Height: 272}, d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 20, Y: 20, Width: 448, Height: 232}, RadiusX: 18, RadiusY: 18,
		Fill: gradient, Stroke: &d2scene.Stroke{Paint: gradient, Width: 4, Join: d2scene.JoinRound},
	}))
	options := testOptions()
	options.Scale = 2
	options.MaxWidth, options.MaxHeight, options.MaxPixels = 2_000, 2_000, 4_000_000
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

func BenchmarkRenderLinearGradient(b *testing.B) {
	gradient := d2scene.LinearGradient{
		Start: d2scene.Point{X: 0, Y: .5}, End: d2scene.Point{X: 1, Y: .5},
		Stops: []d2scene.GradientStop{
			{Offset: 0, Color: color.NRGBA{R: 255, G: 240, B: 128, A: 230}},
			{Offset: .6, Color: color.NRGBA{R: 80, G: 120, B: 255, A: 180}},
			{Offset: 1, Color: color.NRGBA{R: 20, G: 10, B: 80, A: 255}},
		},
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.1), Spread: d2scene.SpreadReflect,
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 488, Height: 272}, d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 20, Y: 20, Width: 448, Height: 232}, RadiusX: 18, RadiusY: 18,
		Fill: gradient, Stroke: &d2scene.Stroke{Paint: gradient, Width: 4, Join: d2scene.JoinRound},
	}))
	options := testOptions()
	options.Scale = 2
	options.MaxWidth, options.MaxHeight, options.MaxPixels = 2_000, 2_000, 4_000_000
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

func BenchmarkLinearGradientAxisSampling(b *testing.B) {
	gradient := d2scene.LinearGradient{
		Start: d2scene.Point{Y: .5}, End: d2scene.Point{X: 1, Y: .5}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.1), Spread: d2scene.SpreadReflect,
	}
	paint, err := prepareLinearGradient(gradient, d2scene.Box{X: 20, Y: 20, Width: 448, Height: 232}, d2scene.Scale(2, 2))
	if err != nil {
		b.Fatal(err)
	}
	general := *paint
	general.kind = preparedLinearGradient
	gradient.Transform = d2scene.Identity()
	axisAligned, err := prepareLinearGradient(gradient, d2scene.Box{X: 20, Y: 20, Width: 448, Height: 232}, d2scene.Scale(2, 2))
	if err != nil {
		b.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		paint *preparedPaint
	}{
		{name: "Axis", paint: paint},
		{name: "AxisAligned", paint: axisAligned},
		{name: "General", paint: &general},
	} {
		b.Run(test.name, func(b *testing.B) {
			var sample color.NRGBA
			for index := range b.N {
				sample, _ = test.paint.colorAt(float64(index%976)+.5, float64(index%544)+.5)
			}
			benchmarkGradientColor = sample
		})
	}
}

func BenchmarkDrawPaintMaskRepeated(b *testing.B) {
	const repetitions = 16
	bounds := image.Rect(0, 0, 64, 64)
	dst := image.NewRGBA(bounds)
	paint := &preparedPaint{kind: preparedSolidPaint, solid: color.NRGBA{R: 255, A: 255}}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		scratch := &rasterScratch{offscreen: offscreenBudget{limit: repetitions * int64(bounds.Dx()*bounds.Dy())}}
		for range repetitions {
			if err := drawPaintMask(ctx, dst, bounds, paint, scratch, "benchmark Alpha mask", func(mask *image.Alpha) error {
				return nil
			}); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkDrawLinearGradientMask(b *testing.B) {
	bounds := image.Rect(0, 0, 488, 272)
	paint, err := prepareLinearGradient(d2scene.LinearGradient{
		Start: d2scene.Point{Y: .5}, End: d2scene.Point{X: 1, Y: .5}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.1), Spread: d2scene.SpreadReflect,
	}, d2scene.Box{Width: 488, Height: 272}, d2scene.Identity())
	if err != nil {
		b.Fatal(err)
	}
	mask := image.NewAlpha(bounds)
	for index := range mask.Pix {
		mask.Pix[index] = 0xff
	}
	ctx := context.Background()
	for _, test := range []struct {
		name string
		draw func(context.Context, *image.RGBA, image.Rectangle, *image.Alpha) error
	}{
		{name: "Specialized", draw: func(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha) error {
			return drawAxisLinearGradientMaskPixels(ctx, dst, bounds, mask, &paint.gradient, paint.kind)
		}},
		{name: "KindDispatch", draw: func(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha) error {
			return drawPaintMaskPixels(ctx, dst, bounds, mask, paint)
		}},
	} {
		b.Run(test.name, func(b *testing.B) {
			dst := image.NewRGBA(bounds)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := test.draw(ctx, dst, bounds, mask); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDrawGeneralLinearGradientMaskPixels(b *testing.B) {
	bounds := image.Rect(0, 0, 488, 272)
	paint, err := prepareLinearGradient(d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1, Y: 1}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.1), Spread: d2scene.SpreadReflect,
	}, d2scene.Box{Width: 488, Height: 272}, d2scene.Identity())
	if err != nil {
		b.Fatal(err)
	}
	mask := image.NewAlpha(bounds)
	for index := range mask.Pix {
		mask.Pix[index] = 0xff
	}
	ctx := context.Background()
	for _, test := range []struct {
		name string
		draw func(context.Context, *image.RGBA, image.Rectangle, *image.Alpha) error
	}{
		{name: "Specialized", draw: func(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha) error {
			return drawLinearGradientMaskPixels(ctx, dst, bounds, mask, &paint.gradient)
		}},
		{name: "KindDispatch", draw: func(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha) error {
			return drawPaintMaskPixels(ctx, dst, bounds, mask, paint)
		}},
	} {
		b.Run(test.name, func(b *testing.B) {
			dst := image.NewRGBA(bounds)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := test.draw(ctx, dst, bounds, mask); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDrawVerticalLinearGradientMask(b *testing.B) {
	bounds := image.Rect(0, 0, 488, 272)
	paint, err := prepareLinearGradient(d2scene.LinearGradient{
		Start: d2scene.Point{X: .5}, End: d2scene.Point{X: .5, Y: 1}, Stops: gradientStops,
		Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(), Spread: d2scene.SpreadReflect,
	}, d2scene.Box{Width: 488, Height: 272}, d2scene.Identity())
	if err != nil {
		b.Fatal(err)
	}
	mask := image.NewAlpha(bounds)
	for index := range mask.Pix {
		mask.Pix[index] = 0xff
	}
	ctx := context.Background()
	for _, test := range []struct {
		name string
		draw func(context.Context, *image.RGBA, image.Rectangle, *image.Alpha) error
	}{
		{name: "Specialized", draw: func(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha) error {
			return drawAxisLinearGradientMaskPixels(ctx, dst, bounds, mask, &paint.gradient, paint.kind)
		}},
		{name: "KindDispatch", draw: func(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha) error {
			return drawPaintMaskPixels(ctx, dst, bounds, mask, paint)
		}},
	} {
		b.Run(test.name, func(b *testing.B) {
			dst := image.NewRGBA(bounds)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := test.draw(ctx, dst, bounds, mask); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDrawRadialGradientMaskPixels(b *testing.B) {
	bounds := image.Rect(0, 0, 488, 272)
	paint, err := prepareRadialGradient(d2scene.RadialGradient{
		Center: d2scene.Point{X: .5, Y: .5}, Radius: .5,
		Focal: d2scene.Point{X: .4, Y: .45}, FocalRadius: .05,
		Stops: gradientStops, Units: d2scene.ObjectBoundingBox, Transform: d2scene.Rotate(.1), Spread: d2scene.SpreadReflect,
	}, d2scene.Box{Width: 488, Height: 272}, d2scene.Identity())
	if err != nil {
		b.Fatal(err)
	}
	mask := image.NewAlpha(bounds)
	for index := range mask.Pix {
		mask.Pix[index] = 0xff
	}
	ctx := context.Background()
	for _, test := range []struct {
		name string
		draw func(context.Context, *image.RGBA, image.Rectangle, *image.Alpha) error
	}{
		{name: "Specialized", draw: func(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha) error {
			return drawRadialGradientMaskPixels(ctx, dst, bounds, mask, &paint.gradient)
		}},
		{name: "KindDispatch", draw: func(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha) error {
			return drawPaintMaskPixels(ctx, dst, bounds, mask, paint)
		}},
	} {
		b.Run(test.name, func(b *testing.B) {
			dst := image.NewRGBA(bounds)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := test.draw(ctx, dst, bounds, mask); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestCompositeNRGBAOpaqueFullCoverageMatchesReferenceArithmeticExhaustively(t *testing.T) {
	t.Parallel()

	for source := 0; source <= 0xff; source++ {
		for destination := 0; destination <= 0xff; destination++ {
			got := []byte{uint8(destination), uint8(destination), uint8(destination), uint8(destination)}
			want := append([]byte(nil), got...)
			paint := color.NRGBA{R: uint8(source), G: uint8(source), B: uint8(source), A: 0xff}
			compositeNRGBAOverRGBA(got, paint, 0xff)
			referenceCompositeNRGBAOverRGBA(want, paint, 0xff)
			if !bytes.Equal(got, want) {
				t.Fatalf("opaque full-coverage source=%d destination=%d: got %v, want reference %v", source, destination, got, want)
			}
		}
	}
}

func TestCompositeNRGBAOverTransparentPixelMatchesReferenceArithmetic(t *testing.T) {
	t.Parallel()

	state := uint32(1)
	for range 1_000_000 {
		state = state*1664525 + 1013904223
		paint := color.NRGBA{R: uint8(state), G: uint8(state >> 8), B: uint8(state >> 16), A: uint8(state >> 24)}
		state = state*1664525 + 1013904223
		coverage := uint8(state >> 24)
		got := []byte{0, 0, 0, 0}
		want := []byte{0, 0, 0, 0}
		compositeNRGBAOverRGBA(got, paint, coverage)
		referenceCompositeNRGBAOverRGBA(want, paint, coverage)
		if !bytes.Equal(got, want) {
			t.Fatalf("source=%#v coverage=%d: got %v, want reference %v", paint, coverage, got, want)
		}
	}
}

func BenchmarkCompositeNRGBAOverRGBA(b *testing.B) {
	tests := []struct {
		name     string
		paint    color.NRGBA
		coverage uint8
	}{
		{name: "OpaqueFullCoverage", paint: color.NRGBA{R: 229, G: 71, B: 19, A: 255}, coverage: 255},
		{name: "OpaquePartialCoverage", paint: color.NRGBA{R: 229, G: 71, B: 19, A: 255}, coverage: 173},
		{name: "TranslucentFullCoverage", paint: color.NRGBA{R: 229, G: 71, B: 19, A: 173}, coverage: 255},
	}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			pixel := []byte{17, 91, 203, 239}
			b.SetBytes(4)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				compositeNRGBAOverRGBA(pixel, test.paint, test.coverage)
			}
			copy(benchmarkCompositeNRGBAPixel[:], pixel)
		})
	}
}

func referenceCompositeNRGBAOverRGBA(destination []byte, source color.NRGBA, coverage uint8) {
	mul255 := func(a, b uint32) uint32 { return (a*b + 127) / 255 }
	sourceAlpha := mul255(uint32(source.A), uint32(coverage))
	inverseAlpha := 255 - sourceAlpha
	for channel, value := range [...]uint8{source.R, source.G, source.B} {
		premultiplied := mul255(uint32(value), sourceAlpha)
		result := premultiplied + mul255(uint32(destination[channel]), inverseAlpha)
		if result > 255 {
			result = 255
		}
		destination[channel] = uint8(result)
	}
	alpha := sourceAlpha + mul255(uint32(destination[3]), inverseAlpha)
	if alpha > 255 {
		alpha = 255
	}
	destination[3] = uint8(alpha)
}

var benchmarkCompositeNRGBAPixel [4]byte

func assertSampleNear(t *testing.T, paint *preparedPaint, x, y float64, want color.NRGBA, tolerance uint8) {
	t.Helper()
	got, ok := paint.colorAt(x, y)
	if !ok {
		t.Fatalf("sample at (%g,%g) is transparent, want %#v", x, y, want)
	}
	assertColorNear(t, got, want, tolerance)
}

func assertColorNear(t *testing.T, got, want color.NRGBA, tolerance uint8) {
	t.Helper()
	for index, pair := range [][2]uint8{{got.R, want.R}, {got.G, want.G}, {got.B, want.B}, {got.A, want.A}} {
		difference := int(pair[0]) - int(pair[1])
		if difference < 0 {
			difference = -difference
		}
		if difference > int(tolerance) {
			t.Fatalf("color = %#v, want %#v within %d (channel %d differs by %d)", got, want, tolerance, index, difference)
		}
	}
}

type cancelAfterContext struct {
	calls int
	after int
}

func (ctx *cancelAfterContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *cancelAfterContext) Done() <-chan struct{}       { return nil }
func (ctx *cancelAfterContext) Value(any) any               { return nil }
func (ctx *cancelAfterContext) Err() error {
	ctx.calls++
	if ctx.calls > ctx.after {
		return context.Canceled
	}
	return nil
}
