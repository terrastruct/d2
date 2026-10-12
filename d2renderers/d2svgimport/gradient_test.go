package d2svgimport

import (
	"context"
	"errors"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestImportNodeLocalLinearGradientExactSceneAndCascade(t *testing.T) {
	result := mustImport(t, `<svg viewBox="0 0 100 100">
  <style>
    .paint { fill: url(#OrangeGradient); fill-opacity: .5; stroke: url(#OrangeGradient); stroke-opacity: .25; stroke-width: 2; }
    .first { stop-color: #00ff00; stop-opacity: .75; }
  </style>
  <rect id="target" class="paint" width="100" height="100" fill="#ffffff"/>
  <defs>
    <linearGradient id="OrangeGradient" x1="8.5" y1="90.5" x2="90.5" y2="8.5" gradientUnits="userSpaceOnUse" gradientTransform="translate(2 3) rotate(90)" spreadMethod="pad">
      <stop class="first" offset="0" stop-color="#000000" style="stop-color:#ff000080;stop-opacity:.5"/>
      <stop offset="1" style="stop-color:rgb(0,0,255)"/>
    </linearGradient>
  </defs>
</svg>`)

	if len(result.Root.Children) != 1 || result.Root.Children[0].ID != "target" {
		t.Fatalf("rendered gradient tree = %#v", result.Root.Children)
	}
	rect := result.Root.Children[0].Primitive.(d2scene.Rect)
	fill, ok := rect.Fill.(d2scene.LinearGradient)
	if !ok {
		t.Fatalf("fill = %T, want linear gradient", rect.Fill)
	}
	if fill.Start != (d2scene.Point{X: 8.5, Y: 90.5}) || fill.End != (d2scene.Point{X: 90.5, Y: 8.5}) ||
		fill.Units != d2scene.UserSpaceOnUse || fill.Spread != d2scene.SpreadPad {
		t.Fatalf("gradient geometry = %#v", fill)
	}
	wantTransform := d2scene.Translate(2, 3).Mul(d2scene.Rotate(math.Pi / 2))
	if !nearMatrix(fill.Transform, wantTransform) {
		t.Fatalf("gradient transform = %+v, want %+v", fill.Transform, wantTransform)
	}
	wantFillStops := []d2scene.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 32}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 128}},
	}
	if !equalGradientStops(fill.Stops, wantFillStops) {
		t.Fatalf("fill stops = %#v, want %#v", fill.Stops, wantFillStops)
	}
	if rect.Stroke == nil || rect.Stroke.Width != 2 {
		t.Fatalf("stroke = %#v", rect.Stroke)
	}
	stroke, ok := rect.Stroke.Paint.(d2scene.LinearGradient)
	if !ok {
		t.Fatalf("stroke paint = %T, want linear gradient", rect.Stroke.Paint)
	}
	wantStrokeStops := []d2scene.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 16}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 64}},
	}
	if !equalGradientStops(stroke.Stops, wantStrokeStops) {
		t.Fatalf("stroke stops = %#v, want %#v", stroke.Stops, wantStrokeStops)
	}
	fill.Stops[0].Color.G = 255
	if stroke.Stops[0].Color.G != 0 {
		t.Fatal("fill and stroke gradient stop storage aliases")
	}
	if result.Metrics.ParsedElements != 7 || result.Metrics.DeclaredResources != 2 ||
		result.Metrics.ParsedTransformFuncs != 2 || result.Metrics.EmittedElements != 2 {
		t.Fatalf("gradient metrics = %+v", result.Metrics)
	}
}

func TestImportNodeLinearGradientAllowsFiniteExtremeTransform(t *testing.T) {
	result := mustImport(t, `<svg viewBox="0 0 2 1"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse" gradientTransform="scale(1e300 1e-300)"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs><rect width="2" height="1" fill="url(#g)"/></svg>`)
	gradient := result.Root.Children[0].Primitive.(d2scene.Rect).Fill.(d2scene.LinearGradient)
	if gradient.Transform != (d2scene.Matrix{A: 1e300, D: 1e-300}) {
		t.Fatalf("extreme finite transform = %+v", gradient.Transform)
	}
}

func TestImportNodeLinearGradientUseInstancesOwnStops(t *testing.T) {
	result := mustImport(t, `<svg viewBox="0 0 2 1"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient><rect id="tile" width="1" height="1" fill="url(#g)"/></defs><use id="first" href="#tile"/><use id="second" href="#tile" x="1"/></svg>`)
	if len(result.Root.Children) != 2 || result.Metrics.ExpandedUseInstances != 2 || result.Metrics.EmittedElements != 5 {
		t.Fatalf("gradient use result/metrics = %#v/%+v", result.Root.Children, result.Metrics)
	}
	first := result.Root.Children[0].Children[0].Primitive.(d2scene.Rect).Fill.(d2scene.LinearGradient)
	second := result.Root.Children[1].Children[0].Primitive.(d2scene.Rect).Fill.(d2scene.LinearGradient)
	first.Stops[0].Color.G = 255
	if second.Stops[0].Color.G != 0 {
		t.Fatal("expanded gradient use instances alias stop storage")
	}
}

func TestImportNodeRejectsUnsupportedLinearGradientForms(t *testing.T) {
	valid := `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs><rect id="shape" width="1" height="1" fill="url(#g)"/>`
	tests := []struct {
		name, body, want string
	}{
		{"external-paint", valid[:strings.LastIndex(valid, `fill="`)] + `fill="url(https://example.invalid/g)"/>`, "external references"},
		{"paint-fallback", valid[:strings.LastIndex(valid, `fill="`)] + `fill="url(#g) red"/>`, "fallbacks"},
		{"quoted-paint", valid[:strings.LastIndex(valid, `fill="`)] + `fill="url('#g')"/>`, "one local url(#id)"},
		{"malformed-paint", valid[:strings.LastIndex(valid, `fill="`)] + `fill="url(#g))"/>`, "one local url(#id)"},
		{"unknown-paint", valid[:strings.LastIndex(valid, `fill="`)] + `fill="url(#missing)"/>`, "unknown local id"},
		{"wrong-resource-kind", `<defs><path id="g" d="M0 0L1 1"/></defs><rect width="1" height="1" fill="url(#g)"/>`, "does not name"},
		{"outside-defs", `<linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0"/></linearGradient>`, "direct child of <defs>"},
		{"missing-id", `<defs><linearGradient x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0"/></linearGradient></defs>`, "must declare an id"},
		{"missing-coordinate", `<defs><linearGradient id="g" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0"/></linearGradient></defs>`, `missing required attribute "x1"`},
		{"invalid-gradient-units", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="invalidUnits"><stop offset="0"/></linearGradient></defs>`, "gradientUnits"},
		{"object-bounds-invalid-coordinate", `<defs><linearGradient id="g" x1="invalid"><stop offset="0"/></linearGradient></defs>`, "invalid x1"},
		{"object-bounds-zero-vector", `<defs><linearGradient id="g" x1="0" y1="0" x2="0" y2="0"><stop offset="0"/></linearGradient></defs>`, "zero-length"},
		{"percentage-coordinate", `<defs><linearGradient id="g" x1="0%" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0"/></linearGradient></defs>`, "percentages are unsupported"},
		{"repeat-spread", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse" spreadMethod="repeat"><stop offset="0"/></linearGradient></defs>`, "only the pad"},
		{"href-inheritance", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse" href="#base"><stop offset="0"/></linearGradient></defs>`, `unsupported attribute "href"`},
		{"xlink-inheritance", `<defs xmlns:xlink="http://www.w3.org/1999/xlink"><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse" xlink:href="#base"><stop offset="0"/></linearGradient></defs>`, `unsupported attribute "href"`},
		{"zero-vector", `<defs><linearGradient id="g" x1="1" y1="2" x2="1" y2="2" gradientUnits="userSpaceOnUse"><stop offset="0"/></linearGradient></defs>`, "zero-length"},
		{"singular-transform", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse" gradientTransform="scale(0)"><stop offset="0"/></linearGradient></defs>`, "singular or unrepresentable"},
		{"unrepresentable-inverse", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse" gradientTransform="scale(5e-324)"><stop offset="0"/></linearGradient></defs>`, "singular or unrepresentable"},
		{"empty-transform", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse" gradientTransform=""><stop offset="0"/></linearGradient></defs>`, "empty gradientTransform"},
		{"no-stops", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"/></defs>`, "at least one <stop>"},
		{"foreign-gradient-child", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><rect width="1" height="1"/></linearGradient></defs>`, "unsupported child <rect>"},
		{"orphan-stop", `<defs><stop offset="0"/></defs>`, "only supported inside"},
		{"missing-offset", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop/></linearGradient></defs>`, "missing offset"},
		{"percentage-offset", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0%"/></linearGradient></defs>`, "percentages are unsupported"},
		{"decreasing-offsets", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset=".75"/><stop offset=".25"/></linearGradient></defs>`, "decreasing stop offsets"},
		{"stop-inherit", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="inherit"/></linearGradient></defs>`, "explicit solid color"},
		{"stop-url", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="url(#g)"/></linearGradient></defs>`, "explicit solid color"},
		{"stop-rendering-property", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0" fill="red"/></linearGradient></defs>`, `unsupported attribute "fill"`},
		{"shape-stop-property", `<rect width="1" height="1" stop-color="red"/>`, `unsupported style property "stop-color"`},
		{"gradient-style", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse" style="opacity:.5"><stop offset="0"/></linearGradient></defs>`, `unsupported attribute "style"`},
		{"use-gradient", valid + `<use href="#g"/>`, "references unsupported <linearGradient>"},
		{"radial", `<defs><radialGradient id="g"/></defs>`, "unsupported paint server <radialGradient>"},
		{"pattern", `<defs><pattern id="g"/></defs>`, "unsupported paint server <pattern>"},
		{"duplicate-id", `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0"/></linearGradient><path id="g"/></defs>`, "duplicate id"},
		{"unused-invalid-stop-rule", `<style>.unused{stop-opacity:2}</style>`, "invalid stop-opacity"},
		{"mixed-rule", `<style>.mixed{stop-color:red;fill:blue}</style>`, "mixes stop-only"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "gradient.svg", []byte(`<svg width="2" height="2">`+test.body+`</svg>`), generousImportLimits())
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("result/error = %#v/%v; want %q", result, err, test.want)
			}
		})
	}
}

func TestImportNodeLinearGradientLimitsAndCancellation(t *testing.T) {
	source := []byte(`<svg width="2" height="2"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse" gradientTransform="translate(1) rotate(2)"><stop offset="0"/><stop offset="1"/></linearGradient></defs><rect width="2" height="2" fill="url(#g)"/></svg>`)
	limits := generousImportLimits()
	limits.MaxTransformFunctions = 1
	if result, err := ImportNode(context.Background(), "gradient.svg", source, limits); err == nil || result != nil || !strings.Contains(err.Error(), "transform function count") {
		t.Fatalf("transform limit result/error = %#v/%v", result, err)
	}

	limits = generousImportLimits()
	limits.MaxResources = 1
	resourceSource := []byte(`<svg width="1" height="1"><defs><linearGradient id="first" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0"/></linearGradient><linearGradient id="second" x1="0" y1="0" x2="1" y2="0" gradientUnits="userSpaceOnUse"><stop offset="0"/></linearGradient></defs></svg>`)
	if result, err := ImportNode(context.Background(), "gradient.svg", resourceSource, limits); err == nil || result != nil || !strings.Contains(err.Error(), "ID resource count") {
		t.Fatalf("resource limit result/error = %#v/%v", result, err)
	}

	importer := svgImporter{ctx: &cancelAfterContext{remaining: 1}, source: "cancel.svg", ids: make(map[string]*svgElement)}
	_, err := importer.localLinearGradientPaint("url(#" + strings.Repeat("a", 4088) + ")")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("paint reference cancellation = %v", err)
	}

	_, err = paintWithOpacity(&cancelAfterContext{remaining: 3}, d2scene.LinearGradient{
		Stops: make([]d2scene.GradientStop, 1024),
	}, .5)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("gradient opacity cancellation = %v", err)
	}
}

func equalGradientStops(got, want []d2scene.GradientStop) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func nearMatrix(got, want d2scene.Matrix) bool {
	return nearFloat(got.A, want.A) && nearFloat(got.B, want.B) && nearFloat(got.C, want.C) &&
		nearFloat(got.D, want.D) && nearFloat(got.E, want.E) && nearFloat(got.F, want.F)
}

func TestImportNodeLinearGradientObjectBoundingBoxDefaults(t *testing.T) {
	result := mustImport(t, `<svg viewBox="0 0 10 10">
  <defs>
    <linearGradient id="defaultGradient">
      <stop offset="0" stop-color="red"/>
      <stop offset="1" stop-color="blue"/>
    </linearGradient>
  </defs>
  <rect id="target" width="10" height="10" fill="url(#defaultGradient)"/>
</svg>`)

	if len(result.Root.Children) != 1 || result.Root.Children[0].ID != "target" {
		t.Fatalf("rendered gradient tree = %#v", result.Root.Children)
	}
	rect := result.Root.Children[0].Primitive.(d2scene.Rect)
	fill, ok := rect.Fill.(d2scene.LinearGradient)
	if !ok {
		t.Fatalf("fill = %T, want linear gradient", rect.Fill)
	}
	if fill.Start != (d2scene.Point{X: 0, Y: 0}) || fill.End != (d2scene.Point{X: 1, Y: 0}) ||
		fill.Units != d2scene.ObjectBoundingBox || fill.Spread != d2scene.SpreadPad ||
		fill.Transform != d2scene.Identity() {
		t.Fatalf("default linear gradient geometry = %#v", fill)
	}
	if len(fill.Stops) != 2 || fill.Stops[0].Offset != 0 || fill.Stops[1].Offset != 1 {
		t.Fatalf("default linear gradient stops = %#v", fill.Stops)
	}
}

func TestImportNodeLinearGradientObjectBoundingBoxCoordinateVariants(t *testing.T) {
	tests := []struct {
		name      string
		svg       string
		wantStart d2scene.Point
		wantEnd   d2scene.Point
		wantUnits d2scene.PaintUnits
	}{
		{
			name: "explicit units with percentages",
			svg: `<svg viewBox="0 0 10 10"><defs>
				<linearGradient id="g" gradientUnits="objectBoundingBox" x1="10%" y1="20%" x2="90%" y2="80%">
					<stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/>
				</linearGradient></defs><rect width="10" height="10" fill="url(#g)"/></svg>`,
			wantStart: d2scene.Point{X: 0.1, Y: 0.2},
			wantEnd:   d2scene.Point{X: 0.9, Y: 0.8},
			wantUnits: d2scene.ObjectBoundingBox,
		},
		{
			name: "explicit units with unitless numbers",
			svg: `<svg viewBox="0 0 10 10"><defs>
				<linearGradient id="g" gradientUnits="objectBoundingBox" x1="0.25" y1="0.5" x2="0.75" y2="0.5">
					<stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/>
				</linearGradient></defs><rect width="10" height="10" fill="url(#g)"/></svg>`,
			wantStart: d2scene.Point{X: 0.25, Y: 0.5},
			wantEnd:   d2scene.Point{X: 0.75, Y: 0.5},
			wantUnits: d2scene.ObjectBoundingBox,
		},
		{
			name: "omitted units with partial coordinates",
			svg: `<svg viewBox="0 0 10 10"><defs>
				<linearGradient id="g" y1="0%" y2="100%">
					<stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/>
				</linearGradient></defs><rect width="10" height="10" fill="url(#g)"/></svg>`,
			wantStart: d2scene.Point{X: 0, Y: 0},
			wantEnd:   d2scene.Point{X: 1, Y: 1},
			wantUnits: d2scene.ObjectBoundingBox,
		},
		{
			name: "omitted units with unitless coordinates",
			svg: `<svg viewBox="0 0 10 10"><defs>
				<linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
					<stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/>
				</linearGradient></defs><rect width="10" height="10" fill="url(#g)"/></svg>`,
			wantStart: d2scene.Point{X: 0, Y: 0},
			wantEnd:   d2scene.Point{X: 1, Y: 1},
			wantUnits: d2scene.ObjectBoundingBox,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := mustImport(t, tc.svg)
			rect := result.Root.Children[0].Primitive.(d2scene.Rect)
			fill := rect.Fill.(d2scene.LinearGradient)
			if !nearFloat(fill.Start.X, tc.wantStart.X) || !nearFloat(fill.Start.Y, tc.wantStart.Y) ||
				!nearFloat(fill.End.X, tc.wantEnd.X) || !nearFloat(fill.End.Y, tc.wantEnd.Y) ||
				fill.Units != tc.wantUnits {
				t.Fatalf("geometry = %#v; want start=%v end=%v units=%v", fill, tc.wantStart, tc.wantEnd, tc.wantUnits)
			}
		})
	}
}

func TestImportNodeLinearGradientObjectBoundingBoxRasterize(t *testing.T) {
	// SVG icon from issue #2937
	svgSource := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" width="10" height="10">
		<defs>
			<linearGradient id="g">
				<stop offset="0" stop-color="red"/>
				<stop offset="1" stop-color="blue"/>
			</linearGradient>
		</defs>
		<path d="M0 0H10V10H0Z" fill="url(#g)"/>
	</svg>`

	result := mustImport(t, svgSource)
	document := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, result.Root)
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: 10, MaxHeight: 10, MaxPixels: 100,
		MaxNodes: 10, MaxDepth: 10, MaxPathCommands: 20,
		MaxAnimationTracks: 1, MaxAnimationKeyframes: 1,
		MaxAssets: 1, MaxAssetBytes: 1, MaxDecodedAssetBytes: 1, MaxImportDepth: 10,
		MaxOffscreenBytes: 1 << 20, MaxEvenOddClipWork: 1 << 20,
	})
	if err != nil {
		t.Fatalf("d2raster.Render failed: %v", err)
	}

	leftPixel := color.NRGBAModel.Convert(frame.At(0, 5)).(color.NRGBA)
	rightPixel := color.NRGBAModel.Convert(frame.At(9, 5)).(color.NRGBA)

	if leftPixel.R < 200 || leftPixel.B > 50 || leftPixel.A != 255 {
		t.Fatalf("left pixel = %+v, want mostly red", leftPixel)
	}
	if rightPixel.B < 200 || rightPixel.R > 50 || rightPixel.A != 255 {
		t.Fatalf("right pixel = %+v, want mostly blue", rightPixel)
	}
}

