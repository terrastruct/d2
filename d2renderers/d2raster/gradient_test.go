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
	"github.com/d2lang/d2/d2renderers/d2scene"
)

var gradientStops = []d2scene.GradientStop{
	{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
	{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
}

func TestNormalizeGradientStopsUsesSVGSourceOrderRules(t *testing.T) {
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
