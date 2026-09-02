package d2raster

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"slices"
	"strings"
	"testing"

	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

var (
	black = d2scene.SolidPaint{Color: color.NRGBA{A: 255}}
	red   = d2scene.SolidPaint{Color: color.NRGBA{R: 255, A: 255}}
	green = d2scene.SolidPaint{Color: color.NRGBA{G: 255, A: 255}}
	blue  = d2scene.SolidPaint{Color: color.NRGBA{B: 255, A: 255}}

	benchmarkFrame *image.NRGBA
)

func TestRenderViewBoxDimensionsAndSolidFills(t *testing.T) {
	rect := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{X: 15, Y: 22, Width: 5, Height: 4},
		Fill: red,
	})
	document := d2scene.NewDocument(d2scene.Box{X: 10, Y: 20, Width: 20, Height: 10}, rect)
	document.LogicalWidth = 40
	document.LogicalHeight = 20
	options := testOptions()
	options.Scale = 1.5

	got, err := Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 60 || got.Bounds().Dy() != 30 {
		t.Fatalf("dimensions = %v, want 60x30", got.Bounds())
	}
	assertPixel(t, got.NRGBAAt(20, 10), color.NRGBA{R: 255, A: 255})
	assertPixel(t, got.NRGBAAt(14, 10), color.NRGBA{})
	assertPixel(t, got.NRGBAAt(31, 10), color.NRGBA{})

	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Ellipse{Center: d2scene.Point{X: 10, Y: 10}, RadiusX: 7, RadiusY: 5, Fill: green}),
		d2scene.NewNode(d2scene.Path{Fill: blue, Commands: []d2scene.PathCommand{
			d2scene.MoveTo(22, 4),
			d2scene.QuadraticTo(35, 10, 22, 16),
			d2scene.LineTo(22, 4),
			d2scene.ClosePath(),
		}}),
	}
	got, err = Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 40, Height: 20}, root), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, got.NRGBAAt(10, 10), color.NRGBA{G: 255, A: 255})
	assertPixel(t, got.NRGBAAt(25, 10), color.NRGBA{B: 255, A: 255})
}

func TestViewportMeetUsesIntegerViewportAlignmentAndDoesNotMutate(t *testing.T) {
	newDocument := func(fit d2scene.ViewportFit, align d2scene.ViewportAlign) *d2scene.Document {
		document := d2scene.NewDocument(
			d2scene.Box{X: 10, Y: 20, Width: 10, Height: 10},
			d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 10, Y: 20, Width: 10, Height: 10}, Fill: red}),
		)
		document.LogicalWidth = 10.2
		document.LogicalHeight = 20.2
		document.ViewportFit = fit
		document.ViewportAlign = align
		return document
	}
	options := testOptions()
	options.Scale = 1.25

	tests := []struct {
		name        string
		fit         d2scene.ViewportFit
		align       d2scene.ViewportAlign
		wantScaleX  float64
		wantScaleY  float64
		wantOffsetY float64
		redY        int
		clearY      int
	}{
		{
			name: "stretch remains the zero-value behavior", fit: d2scene.ViewportStretch,
			align: d2scene.ViewportAlignXMinYMin, wantScaleX: 1.275, wantScaleY: 2.525,
			redY: 20, clearY: -1,
		},
		{
			name: "meet xMinYMin", fit: d2scene.ViewportMeet,
			align: d2scene.ViewportAlignXMinYMin, wantScaleX: 1.3, wantScaleY: 1.3,
			redY: 5, clearY: 20,
		},
		{
			name: "meet xMidYMid", fit: d2scene.ViewportMeet,
			align: d2scene.ViewportAlignXMidYMid, wantScaleX: 1.3, wantScaleY: 1.3, wantOffsetY: 6.5,
			redY: 10, clearY: 3,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := newDocument(test.fit, test.align)
			beforeViewBox := document.ViewBox
			beforeLogicalWidth, beforeLogicalHeight := document.LogicalWidth, document.LogicalHeight
			beforeFit, beforeAlign := document.ViewportFit, document.ViewportAlign
			beforeRoot, beforeRootTransform := document.Root, document.Root.Transform
			prepared, err := prepare(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.width != 13 || prepared.height != 26 {
				t.Fatalf("integer viewport = %dx%d, want 13x26", prepared.width, prepared.height)
			}
			transform := prepared.root.primitive.transform
			if math.Abs(transform.A-test.wantScaleX) > 1e-12 || math.Abs(transform.D-test.wantScaleY) > 1e-12 {
				t.Fatalf("viewport scale = (%g,%g), want (%g,%g)", transform.A, transform.D, test.wantScaleX, test.wantScaleY)
			}
			gotOffsetY := transform.F + document.ViewBox.Y*transform.D
			if math.Abs(gotOffsetY-test.wantOffsetY) > 1e-12 {
				t.Fatalf("viewport Y offset = %g, want %g", gotOffsetY, test.wantOffsetY)
			}
			got, err := Render(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			assertPixel(t, got.NRGBAAt(5, test.redY), color.NRGBA{R: 255, A: 255})
			if test.clearY >= 0 {
				assertPixel(t, got.NRGBAAt(5, test.clearY), color.NRGBA{})
			}
			if test.align == d2scene.ViewportAlignXMidYMid {
				assertPixel(t, got.NRGBAAt(5, 23), color.NRGBA{})
			}
			if document.ViewBox != beforeViewBox || document.LogicalWidth != beforeLogicalWidth || document.LogicalHeight != beforeLogicalHeight ||
				document.ViewportFit != beforeFit || document.ViewportAlign != beforeAlign || document.Root != beforeRoot || document.Root.Transform != beforeRootTransform {
				t.Fatalf("Render mutated document viewport or root state: %#v", document)
			}
		})
	}
}

func TestViewportPolicyRejectsInvalidEnums(t *testing.T) {
	tests := []struct {
		name   string
		change func(*d2scene.Document)
		want   string
	}{
		{name: "fit", change: func(document *d2scene.Document) { document.ViewportFit = d2scene.ViewportFit(255) }, want: "invalid viewport fit"},
		{name: "alignment", change: func(document *d2scene.Document) { document.ViewportAlign = d2scene.ViewportAlign(255) }, want: "invalid viewport alignment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := dimensionDocument(10, 10)
			test.change(document)
			_, err := Render(context.Background(), document, testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Render() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRenderNonZeroFillAndAdaptiveCubic(t *testing.T) {
	path := d2scene.Path{Fill: black, Commands: []d2scene.PathCommand{
		d2scene.MoveTo(5, 5),
		d2scene.CubicTo(5, 0, 35, 0, 35, 5),
		d2scene.LineTo(35, 35),
		d2scene.LineTo(5, 35),
		d2scene.ClosePath(),
		// Opposite winding cuts a hole under the non-zero rule.
		d2scene.MoveTo(12, 12),
		d2scene.LineTo(12, 28),
		d2scene.LineTo(28, 28),
		d2scene.LineTo(28, 12),
		d2scene.ClosePath(),
	}}
	document := d2scene.NewDocument(d2scene.Box{Width: 40, Height: 40}, d2scene.NewNode(path))
	prepared, err := prepare(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(prepared.root.primitive.subpaths[0].points); got <= 5 {
		t.Fatalf("adaptive cubic generated only %d points", got)
	}
	image, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if image.NRGBAAt(8, 8).A != 255 {
		t.Fatalf("outer fill pixel = %#v, want opaque", image.NRGBAAt(8, 8))
	}
	if image.NRGBAAt(20, 20).A != 0 {
		t.Fatalf("non-zero hole pixel = %#v, want transparent", image.NRGBAAt(20, 20))
	}
}

func TestAdaptiveFlatteningPreservesCollinearExtremaAndPostCloseCurrentPoint(t *testing.T) {
	path := d2scene.Path{Commands: []d2scene.PathCommand{
		d2scene.MoveTo(0, 0),
		d2scene.CubicTo(10, 0, -10, 0, 0, 0),
		d2scene.ClosePath(),
		d2scene.LineTo(5, 5),
	}}
	count := func() error { return nil }
	paths, err := flattenScenePath(context.Background(), path, d2scene.Scale(25, 25), count)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("subpath count = %d, want 2", len(paths))
	}
	minX, maxX := 0.0, 0.0
	for _, point := range paths[0].points {
		if point.X < minX {
			minX = point.X
		}
		if point.X > maxX {
			maxX = point.X
		}
	}
	if minX >= -2 || maxX <= 2 {
		t.Fatalf("collinear cubic extrema = [%f,%f], want both lobes", minX, maxX)
	}
	if len(paths[1].points) != 2 || !samePoint(paths[1].points[0], (d2scene.Point{})) ||
		!samePoint(paths[1].points[1], (d2scene.Point{X: 5, Y: 5})) {
		t.Fatalf("post-close line = %#v, want origin to (5,5)", paths[1].points)
	}
}

func TestClosedRoundStrokeHasNoVertexPinholes(t *testing.T) {
	stroke := &d2scene.Stroke{Paint: black, Width: 6, Cap: d2scene.CapButt, Join: d2scene.JoinRound}
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Ellipse{Center: d2scene.Point{X: 25, Y: 25}, RadiusX: 15, RadiusY: 12, Stroke: stroke}),
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 45, Y: 10, Width: 20, Height: 25}, Stroke: stroke}),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 75, Height: 50}, root)
	prepared, err := prepare(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	image, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	for childIndex, child := range prepared.root.children {
		for pointIndex, point := range child.primitive.subpaths[0].points {
			pixel := child.primitive.transform.Point(point)
			got := image.NRGBAAt(int(pixel.X+0.5), int(pixel.Y+0.5))
			if got.A < 200 {
				t.Fatalf("child %d centerline vertex %d at %.2f,%.2f is a pinhole: %#v", childIndex, pointIndex, pixel.X, pixel.Y, got)
			}
		}
	}
}

func TestStrokeCapsRoundJoinAndDashes(t *testing.T) {
	line := func(y float64, cap d2scene.LineCap, dashes []float64, offset float64) *d2scene.Node {
		return d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{d2scene.MoveTo(10, y), d2scene.LineTo(50, y)},
			Stroke: &d2scene.Stroke{
				Paint: black, Width: 6, Cap: cap, Join: d2scene.JoinRound,
				Dashes: dashes, DashOffset: offset,
			},
		})
	}
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		line(10, d2scene.CapButt, nil, 0),
		line(25, d2scene.CapRound, nil, 0),
		line(40, d2scene.CapSquare, nil, 0),
		line(55, d2scene.CapButt, []float64{6, 4}, 2),
		d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{d2scene.MoveTo(60, 70), d2scene.LineTo(70, 70), d2scene.LineTo(70, 60)},
			Stroke:   &d2scene.Stroke{Paint: black, Width: 6, Cap: d2scene.CapButt, Join: d2scene.JoinRound},
		}),
	}
	image, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 80, Height: 80}, root), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertAlpha(t, image.NRGBAAt(8, 10), false)  // butt does not extend
	assertAlpha(t, image.NRGBAAt(8, 25), true)   // round extends
	assertAlpha(t, image.NRGBAAt(8, 40), true)   // square extends
	assertAlpha(t, image.NRGBAAt(11, 55), true)  // offset dash on [10,14)
	assertAlpha(t, image.NRGBAAt(16, 55), false) // then off [14,18)
	assertAlpha(t, image.NRGBAAt(20, 55), true)  // next dash
	assertAlpha(t, image.NRGBAAt(70, 70), true)  // solid round join center
}

func TestStrokeJoinAnalyticGeometry(t *testing.T) {
	vertex := d2scene.Point{X: 20, Y: 20}
	previous := d2scene.Point{X: 10, Y: 20}
	tests := []struct {
		name          string
		next          d2scene.Point
		interiorAngle float64
	}{
		{name: "acute", next: d2scene.Point{X: 10, Y: 10}, interiorAngle: 45},
		{name: "right", next: d2scene.Point{X: 20, Y: 10}, interiorAngle: 90},
		{name: "obtuse", next: d2scene.Point{X: 30, Y: 10}, interiorAngle: 135},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			polygon, ok := strokeJoinPolygon(previous, vertex, test.next, 4, d2scene.JoinMiter, 10)
			if !ok || len(polygon) != 4 {
				t.Fatalf("miter polygon = %#v, %v; want four points", polygon, ok)
			}
			miter := polygon[1]
			gotRatio := math.Hypot(miter.X-vertex.X, miter.Y-vertex.Y) / 4
			wantRatio := 1 / math.Sin(test.interiorAngle*math.Pi/360)
			if math.Abs(gotRatio-wantRatio) > 1e-12 {
				t.Fatalf("miter ratio = %.15f, want %.15f", gotRatio, wantRatio)
			}
			bevel, ok := strokeJoinPolygon(previous, vertex, test.next, 4, d2scene.JoinBevel, 0)
			if !ok || len(bevel) != 3 {
				t.Fatalf("bevel polygon = %#v, %v; want three points", bevel, ok)
			}
			cutoff, ok := strokeJoinPolygon(previous, vertex, test.next, 4, d2scene.JoinMiter, wantRatio-0.01)
			if !ok || len(cutoff) != 3 {
				t.Fatalf("cutoff polygon = %#v, %v; want bevel fallback", cutoff, ok)
			}
		})
	}

	stroke, err := prepareStroke(&d2scene.Stroke{Paint: black, Width: 2, Join: d2scene.JoinMiter})
	if err != nil {
		t.Fatal(err)
	}
	if stroke.miterLimit != 4 {
		t.Fatalf("default miter limit = %g, want 4", stroke.miterLimit)
	}
	if _, err := prepareStroke(&d2scene.Stroke{Paint: black, Width: 2, Join: d2scene.JoinMiter, MiterLimit: 0.5}); err == nil {
		t.Fatal("miter limit below one unexpectedly accepted")
	}
	if polygon, ok := strokeJoinPolygon(previous, vertex, d2scene.Point{X: 30, Y: 20}, 4, d2scene.JoinMiter, 4); ok || polygon != nil {
		t.Fatalf("collinear join = %#v, %v; want no wedge", polygon, ok)
	}
	if polygon, ok := strokeJoinPolygon(previous, vertex, d2scene.Point{X: 10, Y: 20}, 4, d2scene.JoinMiter, 4); ok || polygon != nil {
		t.Fatalf("reversal join = %#v, %v; want safe finite bevel", polygon, ok)
	}
}

func TestCleanPointsAliasesCleanInputAndPreservesDuplicateInput(t *testing.T) {
	t.Parallel()

	clean := []d2scene.Point{{X: 1, Y: 2}, {X: 3, Y: 4}, {X: 5, Y: 6}}
	got := cleanPoints(clean, false)
	if len(got) != len(clean) || &got[0] != &clean[0] {
		t.Fatal("clean point input was copied")
	}
	closed := append(append([]d2scene.Point(nil), clean...), clean[0])
	got = cleanPoints(closed, true)
	if len(got) != len(clean) || &got[0] != &closed[0] {
		t.Fatal("sole closing duplicate did not reuse the input slice")
	}

	duplicate := []d2scene.Point{{X: 1, Y: 2}, {X: 1, Y: 2}, {X: 3, Y: 4}, {X: 3, Y: 4}, {X: 1, Y: 2}}
	before := append([]d2scene.Point(nil), duplicate...)
	got = cleanPoints(duplicate, true)
	want := []d2scene.Point{{X: 1, Y: 2}, {X: 3, Y: 4}}
	if !slices.Equal(got, want) {
		t.Fatalf("cleaned points = %#v, want %#v", got, want)
	}
	if !slices.Equal(duplicate, before) {
		t.Fatal("cleanPoints mutated duplicate input")
	}
	if len(got) != 0 && &got[0] == &duplicate[0] {
		t.Fatal("interior-duplicate cleanup aliased scratch over the input")
	}
}

func TestStrokeRunPixelBoundsMatchesSubpathBounds(t *testing.T) {
	t.Parallel()

	runs := []strokeRun{
		{points: []d2scene.Point{{X: -3.25, Y: 7.5}, {X: 19.75, Y: -2.125}}},
		{points: []d2scene.Point{{X: 4.5, Y: 6.75}, {X: 12.25, Y: 18.5}, {X: -1.5, Y: 13.25}}, closed: true},
	}
	paths := make([]subpath, len(runs))
	for index, run := range runs {
		paths[index] = subpath{points: run.points, closed: run.closed}
	}
	canvas := image.Rect(-20, -30, 80, 90)
	for _, test := range []struct {
		transform d2scene.Matrix
		expansion float64
	}{
		{transform: d2scene.Identity()},
		{transform: d2scene.Translate(11.5, -7.25).Mul(d2scene.Rotate(math.Pi / 7)).Mul(d2scene.Scale(2.5, .75)), expansion: 9.125},
	} {
		got := strokeRunPixelBounds(runs, test.transform, test.expansion, canvas)
		want := subpathPixelBounds(paths, test.transform, test.expansion, canvas)
		if got != want {
			t.Fatalf("stroke bounds = %v, want %v", got, want)
		}
	}
	if got := strokeRunPixelBounds(nil, d2scene.Identity(), 1, canvas); !got.Empty() {
		t.Fatalf("empty stroke bounds = %v", got)
	}
}

func TestRenderMiterBevelAndMiterLimit(t *testing.T) {
	renderRightJoin := func(join d2scene.LineJoin, miterLimit float64) *image.NRGBA {
		t.Helper()
		document := d2scene.NewDocument(d2scene.Box{Width: 40, Height: 40}, d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{d2scene.MoveTo(8, 28), d2scene.LineTo(24, 28), d2scene.LineTo(24, 10)},
			Stroke: &d2scene.Stroke{
				Paint: black, Width: 8, Cap: d2scene.CapButt, Join: join, MiterLimit: miterLimit,
				// Keep the join inside one on-run to cover join generation after dashing.
				Dashes: []float64{100, 10},
			},
		}))
		got, err := Render(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	miter := renderRightJoin(d2scene.JoinMiter, 4)
	bevel := renderRightJoin(d2scene.JoinBevel, 0)
	assertAlpha(t, miter.NRGBAAt(27, 31), true)  // miter-only outer corner
	assertAlpha(t, bevel.NRGBAAt(27, 31), false) // clipped by bevel diagonal
	assertAlpha(t, bevel.NRGBAAt(25, 29), true)  // bevel wedge, outside both rectangles
	assertAlpha(t, miter.NRGBAAt(24, 28), true)  // no join-center pinhole
	assertAlpha(t, bevel.NRGBAAt(24, 28), true)

	previous := d2scene.Point{X: 10, Y: 40}
	vertex := d2scene.Point{X: 30, Y: 40}
	next := d2scene.Point{X: 4, Y: 28}
	wideMiter, ok := strokeJoinPolygon(previous, vertex, next, 8, d2scene.JoinMiter, 6)
	if !ok || len(wideMiter) != 4 {
		t.Fatalf("acute miter polygon = %#v, %v", wideMiter, ok)
	}
	sample := d2scene.Point{
		X: (wideMiter[0].X + wideMiter[1].X + wideMiter[2].X) / 3,
		Y: (wideMiter[0].Y + wideMiter[1].Y + wideMiter[2].Y) / 3,
	}
	renderAcute := func(limit float64) *image.NRGBA {
		t.Helper()
		document := d2scene.NewDocument(d2scene.Box{Width: 80, Height: 60}, d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{d2scene.MoveTo(previous.X, previous.Y), d2scene.LineTo(vertex.X, vertex.Y), d2scene.LineTo(next.X, next.Y)},
			Stroke:   &d2scene.Stroke{Paint: black, Width: 16, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: limit},
		}))
		got, err := Render(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	highLimit := renderAcute(6)
	defaultLimit := renderAcute(0)
	assertAlpha(t, highLimit.NRGBAAt(int(sample.X), int(sample.Y)), true)
	assertAlpha(t, defaultLimit.NRGBAAt(int(sample.X), int(sample.Y)), false)
}

func TestClosedMiterBoxesRenderSharpCornersWithoutPinholes(t *testing.T) {
	stroke := &d2scene.Stroke{Paint: black, Width: 6, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4}
	tests := map[string]d2scene.Primitive{
		"rect primitive": d2scene.Rect{Box: d2scene.Box{X: 10, Y: 10, Width: 20, Height: 20}, Stroke: stroke},
		"unfilled box arrowhead path": d2scene.Path{Commands: []d2scene.PathCommand{
			d2scene.MoveTo(10, 10), d2scene.LineTo(10, 30), d2scene.LineTo(30, 30), d2scene.LineTo(30, 10), d2scene.ClosePath(),
		}, Stroke: stroke},
	}
	for name, primitive := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 40, Height: 40}, d2scene.NewNode(primitive)), testOptions())
			if err != nil {
				t.Fatal(err)
			}
			for _, point := range [][2]int{{7, 7}, {32, 7}, {32, 32}, {7, 32}, {10, 10}, {30, 10}, {30, 30}, {10, 30}} {
				if got.NRGBAAt(point[0], point[1]).A < 200 {
					t.Fatalf("sharp corner/centerline pixel %v = %#v", point, got.NRGBAAt(point[0], point[1]))
				}
			}
			assertAlpha(t, got.NRGBAAt(20, 20), false)
		})
	}
}

func TestMiterJoinUnderNonuniformAndReflectedTransforms(t *testing.T) {
	primitive := d2scene.Path{
		Commands: []d2scene.PathCommand{d2scene.MoveTo(2, 20), d2scene.LineTo(12, 20), d2scene.LineTo(12, 4)},
		Stroke:   &d2scene.Stroke{Paint: black, Width: 8, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4},
	}
	positive := d2scene.NewNode(primitive)
	positive.Transform = d2scene.Translate(5, 5).Mul(d2scene.Scale(2, 0.5))
	reflected := d2scene.NewNode(primitive)
	reflected.Transform = d2scene.Translate(75, 5).Mul(d2scene.Scale(-2, 0.5))
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{positive, reflected}
	got, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 80, Height: 30}, root), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range [][2]int{{35, 16}, {29, 15}, {45, 16}, {51, 15}} {
		if got.NRGBAAt(point[0], point[1]).A < 200 {
			t.Fatalf("transformed miter/centerline pixel %v = %#v", point, got.NRGBAAt(point[0], point[1]))
		}
	}
}

func TestCollinearAndReversalJoinsStayFiniteAndSolid(t *testing.T) {
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{d2scene.MoveTo(5, 10), d2scene.LineTo(15, 10), d2scene.LineTo(25, 10)},
			Stroke:   &d2scene.Stroke{Paint: black, Width: 6, Join: d2scene.JoinMiter},
		}),
		d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{d2scene.MoveTo(5, 25), d2scene.LineTo(25, 25), d2scene.LineTo(5, 25)},
			Stroke:   &d2scene.Stroke{Paint: black, Width: 6, Join: d2scene.JoinBevel},
		}),
	}
	got, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 35, Height: 35}, root), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertAlpha(t, got.NRGBAAt(15, 10), true)
	assertAlpha(t, got.NRGBAAt(24, 25), true)
}

func TestGroupOpacityAndBackground(t *testing.T) {
	group := d2scene.NewNode(nil)
	group.Opacity = 0.5
	group.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 2, Y: 2, Width: 12, Height: 12}, Fill: red}),
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 6, Y: 6, Width: 12, Height: 12}, Fill: red}),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 20, Height: 20}, group)

	transparent, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range [][2]int{{3, 3}, {8, 8}} {
		got := transparent.NRGBAAt(point[0], point[1])
		if got.R != 255 || got.A < 127 || got.A > 128 {
			t.Fatalf("group-opacity pixel %v = %#v, want red alpha 127..128", point, got)
		}
	}
	assertPixel(t, transparent.NRGBAAt(0, 0), color.NRGBA{})

	options := testOptions()
	options.Background = color.White
	white, err := Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, white.NRGBAAt(0, 0), color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	got := white.NRGBAAt(8, 8)
	if got.R != 255 || got.G < 127 || got.G > 128 || got.B < 127 || got.B > 128 || got.A != 255 {
		t.Fatalf("red group over white = %#v", got)
	}
}

func TestRasterizerResetDoesNotLeakPathsAcrossPaints(t *testing.T) {
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 2, Y: 2, Width: 10, Height: 10}, Fill: red}),
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 18, Y: 2, Width: 10, Height: 10}, Fill: blue}),
		d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{X: 34, Y: 2, Width: 12, Height: 12}, Fill: green,
			Stroke: &d2scene.Stroke{Paint: black, Width: 2, Cap: d2scene.CapButt, Join: d2scene.JoinRound},
		}),
	}
	image, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 50, Height: 18}, root), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, image.NRGBAAt(6, 6), color.NRGBA{R: 255, A: 255})
	assertPixel(t, image.NRGBAAt(22, 6), color.NRGBA{B: 255, A: 255})
	assertPixel(t, image.NRGBAAt(40, 8), color.NRGBA{G: 255, A: 255})
}

func TestTextAnchorsUnderlineAndErrors(t *testing.T) {
	fontBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("embedded Source Sans Pro font is not loaded")
	}
	assetID := d2scene.AssetID("font:test")
	textNode := func(y float64, anchor d2scene.TextAnchor, underline bool, value string) *d2scene.Node {
		return d2scene.NewNode(d2scene.TextRun{
			Text: value, Origin: d2scene.Point{X: 80, Y: y}, Anchor: anchor,
			Font: d2scene.Font{Size: 20, Asset: assetID}, Fill: black, Underline: underline,
		})
	}
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		textNode(24, d2scene.AnchorStart, false, "AB"),
		textNode(54, d2scene.AnchorMiddle, false, "AB"),
		textNode(84, d2scene.AnchorEnd, false, "AB"),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 160, Height: 100}, root)
	document.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontBytes}
	image, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	startMin, startMax := alphaXBounds(image, 0, 30)
	middleMin, middleMax := alphaXBounds(image, 30, 60)
	endMin, endMax := alphaXBounds(image, 60, 95)
	if startMin < 79 || startMax <= 80 {
		t.Fatalf("start anchor bounds = [%d,%d]", startMin, startMax)
	}
	if middleMin >= 80 || middleMax <= 80 {
		t.Fatalf("middle anchor bounds = [%d,%d]", middleMin, middleMax)
	}
	if endMin >= 80 || endMax > 81 {
		t.Fatalf("end anchor bounds = [%d,%d]", endMin, endMax)
	}

	underlineRoot := textNode(25, d2scene.AnchorStart, true, "A")
	underlineDocument := d2scene.NewDocument(d2scene.Box{Width: 120, Height: 45}, underlineRoot)
	underlineDocument.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontBytes}
	underlined, err := Render(context.Background(), underlineDocument, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if countOpaqueInRow(underlined, 27) < 5 {
		t.Fatalf("underline row has only %d opaque pixels", countOpaqueInRow(underlined, 27))
	}

	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "missing glyph", value: "\U0010ffff"},
		{name: "missing complex script", value: "\u05d0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			node := textNode(25, d2scene.AnchorStart, false, test.value)
			doc := d2scene.NewDocument(d2scene.Box{Width: 120, Height: 45}, node)
			doc.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontBytes}
			placeholder, err := Render(context.Background(), doc, testOptions())
			if err != nil {
				t.Fatal(err)
			}
			minX, maxX := alphaXBounds(placeholder, 0, placeholder.Bounds().Dy())
			if minX > maxX {
				t.Fatal("missing text rendered no placeholder ink")
			}
		})
	}
}

func TestFontAssetFaceIndexIsValidated(t *testing.T) {
	fontBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("embedded Source Sans Pro font is not loaded")
	}
	if _, err := parsePreparedFont(fontBytes, 0); err != nil {
		t.Fatalf("parsePreparedFont(face 0) error = %v", err)
	}
	if _, err := parsePreparedFont(fontBytes, 1); err == nil || !strings.Contains(err.Error(), "face 1") {
		t.Fatalf("parsePreparedFont(face 1) error = %v, want indexed dual-parser collection error", err)
	}
}

func TestPureGoTextShapingAndFallback(t *testing.T) {
	fontBytes := func(family d2fonts.FontFamily) []byte {
		data, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: family, Style: d2fonts.FONT_STYLE_REGULAR})
		if !ok {
			t.Fatalf("font %s is not loaded", family)
		}
		return data
	}
	primary, err := parsePreparedFont(fontBytes(d2fonts.HandDrawn), 0)
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := parsePreparedFont(fontBytes(d2fonts.SourceSansPro), 0)
	if err != nil {
		t.Fatal(err)
	}
	primaryFace, err := primary.faceForShaping()
	if err != nil {
		t.Fatal(err)
	}
	fallbackFace, err := fallback.faceForShaping()
	if err != nil {
		t.Fatal(err)
	}
	const fallbackRune = '\u0416'
	if _, ok := primaryFace.Shaping.NominalGlyph(fallbackRune); ok {
		t.Fatalf("hand-drawn primary unexpectedly covers fallback test rune %U", fallbackRune)
	}
	if _, ok := fallbackFace.Shaping.NominalGlyph(fallbackRune); !ok {
		t.Fatalf("Source Sans fallback does not cover test rune %U", fallbackRune)
	}

	p := &preflight{
		ctx:     context.Background(),
		options: FrameOptions{MaxPathCommands: 1_000, MaxTextRunesPerRun: 1_000, MaxAssets: 10, MaxFontFacesPerText: 10, MaxTextCoverageChecks: 10_000, MaxTextShapingRuns: 1_000},
		fonts:   map[d2scene.AssetID]*preparedFont{"primary": primary, "fallback": fallback},
	}
	glyphs, _, err := p.positionGlyphs(d2scene.TextRun{
		Text: "A\u0416", Font: d2scene.Font{Asset: "primary", Size: 20}, Fallbacks: []d2scene.AssetID{"fallback"},
	}, fixed.I(20))
	if err != nil {
		t.Fatal(err)
	}
	if len(glyphs) != 2 || glyphs[0].asset != "primary" || glyphs[1].asset != "fallback" {
		t.Fatalf("mixed-font glyph assets = %#v", glyphs)
	}

	latin := &preflight{
		ctx:     context.Background(),
		options: FrameOptions{MaxPathCommands: 1_000, MaxTextRunesPerRun: 1_000, MaxAssets: 10, MaxFontFacesPerText: 10, MaxTextCoverageChecks: 10_000, MaxTextShapingRuns: 1_000},
		fonts:   map[d2scene.AssetID]*preparedFont{"latin": fallback},
	}
	composed, _, err := latin.positionGlyphs(d2scene.TextRun{
		Text: "e\u0301", Font: d2scene.Font{Asset: "latin", Size: 20},
	}, fixed.I(20))
	if err != nil {
		t.Fatal(err)
	}
	if len(composed) != 1 {
		t.Fatalf("e + combining acute shaped to %d glyphs, want one composed glyph", len(composed))
	}

	rtl := &preflight{
		ctx:     context.Background(),
		options: FrameOptions{MaxPathCommands: 1_000, MaxTextRunesPerRun: 1_000, MaxAssets: 10, MaxFontFacesPerText: 10, MaxTextCoverageChecks: 10_000, MaxTextShapingRuns: 1_000},
		fonts:   map[d2scene.AssetID]*preparedFont{"latin": fallback},
	}
	rtlGlyphs, rtlAdvance, err := rtl.positionGlyphs(d2scene.TextRun{
		Text: "\u202eABCD\u202c", Font: d2scene.Font{Asset: "latin", Size: 20},
	}, fixed.I(20))
	if err != nil {
		t.Fatal(err)
	}
	if rtlAdvance <= 0 {
		t.Fatalf("RTL advance = %v", rtlAdvance)
	}
	var rtlSources []rune
	previousX := -1.0
	for _, glyph := range rtlGlyphs {
		if fontface.IsDefaultIgnorableRune(glyph.source) {
			continue
		}
		rtlSources = append(rtlSources, glyph.source)
		if glyph.position.X <= previousX {
			t.Fatalf("RTL visible positions are not increasing: %#v", rtlGlyphs)
		}
		previousX = glyph.position.X
	}
	if got, want := string(rtlSources), "DCBA"; got != want {
		t.Fatalf("RTL override visual sources = %q, want %q", got, want)
	}
	mixedBidi, _, err := rtl.positionGlyphs(d2scene.TextRun{
		Text: "A \u202eABC\u202c B", Font: d2scene.Font{Asset: "latin", Size: 20},
	}, fixed.I(20))
	if err != nil {
		t.Fatal(err)
	}
	var visualSources []rune
	for _, glyph := range mixedBidi {
		if glyph.source != ' ' && !fontface.IsDefaultIgnorableRune(glyph.source) {
			visualSources = append(visualSources, glyph.source)
		}
	}
	if got, want := string(visualSources), "ACBAB"; got != want {
		t.Fatalf("mixed bidi visual sources = %q, want %q", got, want)
	}
}

func TestTextShapingWorkLimitsAreEnforcedBeforeUnboundedWork(t *testing.T) {
	fontBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	font, err := parsePreparedFont(fontBytes, 0)
	if err != nil {
		t.Fatal(err)
	}
	newPreflight := func() *preflight {
		return &preflight{
			ctx: context.Background(),
			options: FrameOptions{
				MaxPathCommands: 10, MaxTextRunesPerRun: 10, MaxAssets: 10,
				MaxFontFacesPerText: 10, MaxTextCoverageChecks: 10, MaxTextShapingRuns: 10,
			},
			fonts: map[d2scene.AssetID]*preparedFont{"font": font},
		}
	}
	run := func(value string) d2scene.TextRun {
		return d2scene.TextRun{Text: value, Font: d2scene.Font{Asset: "font", Size: 20}}
	}

	t.Run("aggregate input runes", func(t *testing.T) {
		p := newPreflight()
		p.options.MaxPathCommands = 3
		if _, _, err := p.positionGlyphs(run("\u00ad\u00ad"), fixed.I(20)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := p.positionGlyphs(run("\u00ad\u00ad"), fixed.I(20)); err == nil || !strings.Contains(err.Error(), "rune count exceeds per-run limit 1") {
			t.Fatalf("aggregate rune error = %v", err)
		}
	})

	t.Run("coverage checks", func(t *testing.T) {
		p := newPreflight()
		p.options.MaxTextCoverageChecks = 1
		if _, _, err := p.positionGlyphs(run("AB"), fixed.I(20)); err == nil || !strings.Contains(err.Error(), "font coverage checks exceed limit 1") {
			t.Fatalf("coverage error = %v", err)
		}
	})

	t.Run("shaping runs", func(t *testing.T) {
		p := newPreflight()
		p.options.MaxTextShapingRuns = 1
		if _, _, err := p.positionGlyphs(run("A\u0416"), fixed.I(20)); err == nil || !strings.Contains(err.Error(), "shaping run count exceeds limit 1") {
			t.Fatalf("shaping-run error = %v", err)
		}
	})

	t.Run("font faces", func(t *testing.T) {
		p := newPreflight()
		p.fonts["fallback"] = font
		p.options.MaxFontFacesPerText = 1
		text := run("A")
		text.Fallbacks = []d2scene.AssetID{"fallback"}
		if _, _, err := p.positionGlyphs(text, fixed.I(20)); err == nil || !strings.Contains(err.Error(), "font face count 2 exceeds limit 1") {
			t.Fatalf("font-face error = %v", err)
		}
	})
}

func TestRenderRejectsFallbackReferenceCountBeforeSliceAllocation(t *testing.T) {
	fontBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	run := d2scene.TextRun{
		Text: "A", Font: d2scene.Font{Asset: "font", Size: 20}, Fill: black,
		Fallbacks: []d2scene.AssetID{"fallback", "fallback", "fallback"},
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 40, Height: 30}, d2scene.NewNode(run))
	document.Assets["font"] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontBytes}
	document.Assets["fallback"] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontBytes}
	options := testOptions()
	options.MaxAssets = 2
	if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "fallback font reference count 3 exceeds asset limit 2") {
		t.Fatalf("fallback-reference error = %v", err)
	}
}

func TestUnsupportedSceneFeaturesFailPreflight(t *testing.T) {
	tests := map[string]struct {
		document func() *d2scene.Document
		want     string
	}{
		"missing image asset": {func() *d2scene.Document { return testDocument(d2scene.Image{}) }, "empty asset"},
		"invalid path fill rule": {func() *d2scene.Document {
			return testDocument(d2scene.Path{FillRule: d2scene.FillRule(255)})
		}, "invalid fill rule 255"},
		"filter": {func() *d2scene.Document {
			doc := testDocument(nil)
			var blur *d2scene.GaussianBlur
			doc.Root.Filters = []d2scene.Filter{blur}
			return doc
		}, "nil Gaussian blur"},
		"invalid blend": {func() *d2scene.Document {
			doc := testDocument(nil)
			doc.Root.Blend = d2scene.BlendMode(255)
			return doc
		}, "unsupported blend"},
		"join": {func() *d2scene.Document {
			return testDocument(d2scene.Path{
				Commands: []d2scene.PathCommand{d2scene.MoveTo(1, 1), d2scene.LineTo(5, 5)},
				Stroke:   &d2scene.Stroke{Paint: black, Width: 1, Join: d2scene.LineJoin(255)},
			})
		}, "unsupported line join"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Render(context.Background(), test.document(), testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestLimitsAndCancellation(t *testing.T) {
	base := testOptions()
	tests := map[string]struct {
		document *d2scene.Document
		options  FrameOptions
		want     string
	}{
		"width":  {dimensionDocument(11, 5), withOption(base, func(o *FrameOptions) { o.MaxWidth = 10 }), "frame width"},
		"height": {dimensionDocument(5, 11), withOption(base, func(o *FrameOptions) { o.MaxHeight = 10 }), "frame height"},
		"pixels": {dimensionDocument(11, 11), withOption(base, func(o *FrameOptions) { o.MaxPixels = 120 }), "frame pixels"},
		"offscreen option": {
			dimensionDocument(10, 10), withOption(base, func(o *FrameOptions) { o.MaxOffscreenBytes = 0 }), "every frame resource limit",
		},
		"asset count option": {
			dimensionDocument(10, 10), withOption(base, func(o *FrameOptions) { o.MaxAssets = 0 }), "every frame resource limit",
		},
		"asset bytes option": {
			dimensionDocument(10, 10), withOption(base, func(o *FrameOptions) { o.MaxAssetBytes = 0 }), "every frame resource limit",
		},
		"decoded asset option": {
			dimensionDocument(10, 10), withOption(base, func(o *FrameOptions) { o.MaxDecodedAssetBytes = 0 }), "every frame resource limit",
		},
		"import depth option": {
			dimensionDocument(10, 10), withOption(base, func(o *FrameOptions) { o.MaxImportDepth = 0 }), "every frame resource limit",
		},
		"even-odd work option": {
			dimensionDocument(10, 10), withOption(base, func(o *FrameOptions) { o.MaxEvenOddClipWork = 0 }), "every frame resource limit",
		},
		"scanline work option": {
			dimensionDocument(10, 10), withOption(base, func(o *FrameOptions) { o.MaxScanlineWork = -1 }), "every frame resource limit",
		},
		"nodes": {func() *d2scene.Document {
			root := d2scene.NewNode(nil)
			root.Children = []*d2scene.Node{d2scene.NewNode(nil)}
			return d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, root)
		}(), withOption(base, func(o *FrameOptions) { o.MaxNodes = 1 }), "node count"},
		"depth": {func() *d2scene.Document {
			root := d2scene.NewNode(nil)
			root.Children = []*d2scene.Node{d2scene.NewNode(nil)}
			return d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, root)
		}(), withOption(base, func(o *FrameOptions) { o.MaxDepth = 1 }), "node depth"},
		"path": {testDocument(d2scene.Path{Commands: []d2scene.PathCommand{
			d2scene.MoveTo(1, 1), d2scene.LineTo(8, 1), d2scene.LineTo(8, 8), d2scene.ClosePath(),
		}}), withOption(base, func(o *FrameOptions) { o.MaxPathCommands = 3 }), "path command"},
		"dash expansion": {testDocument(d2scene.Path{
			Commands: []d2scene.PathCommand{d2scene.MoveTo(0, 5), d2scene.LineTo(10, 5)},
			Stroke:   &d2scene.Stroke{Paint: black, Width: 1, Join: d2scene.JoinRound, Dashes: []float64{0.01, 0.01}},
		}), withOption(base, func(o *FrameOptions) { o.MaxPathCommands = 100 }), "path command"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Render(context.Background(), test.document, test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Render(ctx, dimensionDocument(10, 10), testOptions())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled render error = %v, want context.Canceled", err)
	}
}

func TestFinalFrameStorageSingleBackingStorePlatformBoundary(t *testing.T) {
	t.Parallel()

	maxPixels := platformMaxInt() / 4
	bytes, err := finalFrameStorageBytes(int(maxPixels), 1)
	if err != nil || bytes != maxPixels*4 {
		t.Fatalf("exact platform boundary = %d, %v; want %d", bytes, err, maxPixels*4)
	}
	if _, err := finalFrameStorageBytes(int(maxPixels)+1, 1); err == nil || !strings.Contains(err.Error(), "platform integer domain") {
		t.Fatalf("boundary+1 error = %v, want platform-domain rejection", err)
	}
}

func TestRGBAtoNRGBAInPlacePreservesPixelsAndBackingStore(t *testing.T) {
	t.Parallel()

	bounds := image.Rect(7, 11, 12, 12)
	source := image.NewRGBA(bounds)
	tests := []struct {
		name  string
		input color.RGBA
		want  color.NRGBA
	}{
		{name: "opaque", input: color.RGBA{R: 17, G: 34, B: 51, A: 255}, want: color.NRGBA{R: 17, G: 34, B: 51, A: 255}},
		{name: "transparent", input: color.RGBA{}, want: color.NRGBA{}},
		{name: "alpha zero preserves hidden channels", input: color.RGBA{R: 17, G: 34, B: 51}, want: color.NRGBA{R: 17, G: 34, B: 51}},
		{name: "partially transparent", input: color.RGBA{R: 64, G: 32, B: 16, A: 128}, want: color.NRGBA{R: 127, G: 63, B: 31, A: 128}},
		{name: "minimum nonzero alpha", input: color.RGBA{R: 1, G: 1, B: 1, A: 1}, want: color.NRGBA{R: 255, G: 255, B: 255, A: 1}},
	}
	for index, test := range tests {
		source.SetRGBA(bounds.Min.X+index, bounds.Min.Y, test.input)
	}

	// Use image/draw as the byte-exact compatibility oracle.
	want := image.NewNRGBA(bounds)
	draw.Draw(want, bounds, source, bounds.Min, draw.Src)
	backing := &source.Pix[0]
	got, err := rgbaToNRGBAInPlace(context.Background(), source, false)
	if err != nil {
		t.Fatal(err)
	}

	if got.Bounds() != bounds || got.Stride != source.Stride {
		t.Fatalf("NRGBA geometry = bounds %v stride %d, want bounds %v stride %d", got.Bounds(), got.Stride, bounds, source.Stride)
	}
	if &got.Pix[0] != backing {
		t.Fatal("RGBA to NRGBA conversion allocated a second pixel backing store")
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatalf("in-place NRGBA pixels = %v, want reference draw conversion %v", got.Pix, want.Pix)
	}

	for index, test := range tests {
		if actual := got.NRGBAAt(bounds.Min.X+index, bounds.Min.Y); actual != test.want {
			t.Errorf("%s pixel = %#v, want %#v", test.name, actual, test.want)
		}
	}
}

func TestRGBAtoNRGBAInPlaceMatchesDrawForEveryChannelAndAlpha(t *testing.T) {
	t.Parallel()

	bounds := image.Rect(0, 0, 256, 256)
	source := image.NewRGBA(bounds)
	for alpha := 0; alpha <= 0xff; alpha++ {
		for channel := 0; channel <= 0xff; channel++ {
			offset := source.PixOffset(channel, alpha)
			source.Pix[offset], source.Pix[offset+1], source.Pix[offset+2], source.Pix[offset+3] = uint8(channel), uint8(channel), uint8(channel), uint8(alpha)
		}
	}
	want := image.NewNRGBA(bounds)
	draw.Draw(want, bounds, source, image.Point{}, draw.Src)
	got, err := rgbaToNRGBAInPlace(context.Background(), source, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("in-place conversion differs from image/draw for at least one channel/alpha combination")
	}
}

func TestRGBAtoNRGBAInPlaceBoundsLeavesKnownZeroRegionUntouched(t *testing.T) {
	t.Parallel()

	source := image.NewRGBA(image.Rect(3, 5, 11, 13))
	inside := image.Rect(5, 7, 9, 11)
	for y := inside.Min.Y; y < inside.Max.Y; y++ {
		for x := inside.Min.X; x < inside.Max.X; x++ {
			offset := source.PixOffset(x, y)
			source.Pix[offset], source.Pix[offset+1], source.Pix[offset+2], source.Pix[offset+3] = 64, 32, 16, 128
		}
	}

	wantInside := image.NewNRGBA(source.Bounds())
	draw.Draw(wantInside, source.Bounds(), source, source.Bounds().Min, draw.Src)
	got, err := rgbaToNRGBAInPlaceBounds(context.Background(), source, inside, false)
	if err != nil {
		t.Fatal(err)
	}
	for y := source.Rect.Min.Y; y < source.Rect.Max.Y; y++ {
		for x := source.Rect.Min.X; x < source.Rect.Max.X; x++ {
			want := color.NRGBA{}
			if image.Pt(x, y).In(inside) {
				want = wantInside.NRGBAAt(x, y)
			}
			if pixel := got.NRGBAAt(x, y); pixel != want {
				t.Fatalf("pixel (%d,%d) = %#v, want %#v", x, y, pixel, want)
			}
		}
	}
}

func TestRGBAtoNRGBAInPlaceUniformBlocksMatchScalarReference(t *testing.T) {
	t.Parallel()

	bounds := image.Rect(-7, 11, 250, 84)
	stride := bounds.Dx()*4 + 19
	source := &image.RGBA{Pix: make([]byte, stride*bounds.Dy()), Stride: stride, Rect: bounds}
	state := uint32(0x9e3779b9)
	for index := range source.Pix {
		state = state*1664525 + 1013904223
		source.Pix[index] = uint8(state >> 24)
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			offset := source.PixOffset(x, y)
			column := x - bounds.Min.X
			alpha := uint8(0xff)
			switch {
			case column%64 == 62:
				alpha = 173
			case column >= 128 && column < 192:
				alpha = 0
			case y&1 != 0 && column&1 != 0:
				alpha = uint8(column)
			}
			source.Pix[offset], source.Pix[offset+1], source.Pix[offset+2], source.Pix[offset+3] = alpha/2, alpha/3, alpha/4, alpha
		}
	}
	reference := &image.RGBA{Pix: append([]byte(nil), source.Pix...), Stride: source.Stride, Rect: source.Rect}
	got, err := rgbaToNRGBAInPlace(context.Background(), source, false)
	if err != nil {
		t.Fatal(err)
	}
	want, err := rgbaToNRGBAScalarReference(context.Background(), reference)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("uniform-block conversion differs from scalar reference, including row padding")
	}
}

func TestRGBAtoNRGBAInPlaceUniformProbeMatchesEveryPrefixLength(t *testing.T) {
	t.Parallel()

	for _, uniformAlpha := range []uint8{0, 0xff} {
		for partialAt := 0; partialAt < 64; partialAt++ {
			bounds := image.Rect(-3, 5, 61, 6)
			stride := bounds.Dx()*4 + 11
			source := &image.RGBA{Pix: make([]byte, stride), Stride: stride, Rect: bounds}
			for index := range source.Pix {
				source.Pix[index] = uint8(index*37 + 11)
			}
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				offset := source.PixOffset(x, bounds.Min.Y)
				alpha := uniformAlpha
				if x-bounds.Min.X == partialAt {
					alpha = 173
				}
				source.Pix[offset], source.Pix[offset+1], source.Pix[offset+2], source.Pix[offset+3] = alpha/2, alpha/3, alpha/4, alpha
			}
			reference := &image.RGBA{Pix: append([]byte(nil), source.Pix...), Stride: stride, Rect: bounds}
			got, err := rgbaToNRGBAInPlace(context.Background(), source, false)
			if err != nil {
				t.Fatal(err)
			}
			want, err := rgbaToNRGBAScalarReference(context.Background(), reference)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("uniform alpha %d partial position %d differs from scalar reference", uniformAlpha, partialAt)
			}
		}
	}
}

func TestRGBAtoNRGBAInPlaceOpaqueFastPathAndCancellation(t *testing.T) {
	t.Parallel()

	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	source.SetRGBA(0, 0, color.RGBA{R: 17, G: 34, B: 51, A: 255})
	source.SetRGBA(1, 0, color.RGBA{R: 68, G: 85, B: 102, A: 255})
	want := append([]byte(nil), source.Pix...)
	backing := &source.Pix[0]
	got, err := rgbaToNRGBAInPlace(context.Background(), source, true)
	if err != nil {
		t.Fatal(err)
	}
	if &got.Pix[0] != backing || !bytes.Equal(got.Pix, want) {
		t.Fatalf("opaque fast path changed or replaced backing pixels: got %v, want %v", got.Pix, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := rgbaToNRGBAInPlace(ctx, image.NewRGBA(image.Rect(0, 0, 1, 1)), false); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("canceled transparent conversion = %v, %v; want nil, context.Canceled", got, err)
	}

	wide := image.NewRGBA(image.Rect(0, 0, 8_192, 1))
	fillOpaquePixels(wide.Pix, color.NRGBA{R: 17, G: 34, B: 51, A: 255})
	if got, err := rgbaToNRGBAInPlace(&cancelAfterErrChecks{remaining: 1}, wide, false); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("mid-span canceled conversion = %v, %v; want nil, context.Canceled", got, err)
	}
}

func TestRenderTransparentBackgroundReturnsStraightAlpha(t *testing.T) {
	t.Parallel()

	document := testDocument(d2scene.Rect{
		Box:  d2scene.Box{Width: 10, Height: 10},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 200, G: 100, B: 50, A: 128}},
	})
	got, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if pixel := got.NRGBAAt(5, 5); pixel != (color.NRGBA{R: 199, G: 99, B: 49, A: 128}) {
		t.Fatalf("transparent-background pixel = %#v, want straight-alpha fill", pixel)
	}
	if pixel := got.NRGBAAt(9, 9); pixel.A == 255 {
		t.Fatalf("transparent-background edge unexpectedly became opaque: %#v", pixel)
	}
}

func TestRenderOpaqueBackgroundRemainsOpaqueThroughEffects(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{X: 1, Y: 1, Width: 8, Height: 8},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 200, G: 100, B: 50, A: 128}},
	})
	node.Opacity = 0.5
	node.Blend = d2scene.BlendMultiply
	node.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 0.5, SigmaY: 0.5}}
	document := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, node)
	options := testOptions()
	options.Background = color.NRGBA{R: 240, G: 230, B: 220, A: 255}

	got, err := Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Opaque() {
		t.Fatal("source-over effects reduced an opaque background's alpha")
	}
}

func TestFillOpaquePixelsMatchesImageDraw(t *testing.T) {
	t.Parallel()

	paint := color.NRGBA{R: 17, G: 91, B: 203, A: 255}
	for _, size := range []int{1, 2, 3, 7, 64, 4097} {
		got := image.NewRGBA(image.Rect(0, 0, size, 1))
		want := image.NewRGBA(got.Bounds())
		fillOpaquePixels(got.Pix, paint)
		draw.Draw(want, want.Bounds(), image.NewUniform(paint), image.Point{}, draw.Src)
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("%d-pixel fill differs from image/draw", size)
		}
	}
}

func BenchmarkFillOpaquePixels(b *testing.B) {
	const width, height = 2048, 1024
	paint := color.NRGBA{R: 17, G: 91, B: 203, A: 255}
	for _, test := range []struct {
		name string
		fill func(*image.RGBA)
	}{
		{name: "PackedCopy", fill: func(destination *image.RGBA) { fillOpaquePixels(destination.Pix, paint) }},
		{name: "ImageDraw", fill: func(destination *image.RGBA) {
			draw.Draw(destination, destination.Bounds(), image.NewUniform(paint), image.Point{}, draw.Src)
		}},
	} {
		b.Run(test.name, func(b *testing.B) {
			destination := image.NewRGBA(image.Rect(0, 0, width, height))
			b.SetBytes(int64(len(destination.Pix)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				test.fill(destination)
			}
			benchmarkRasterFrame = destination
		})
	}
}

var benchmarkRasterFrame *image.RGBA

func BenchmarkRGBAtoNRGBAUniformAlphaBlocks(b *testing.B) {
	const width, height = 1024, 1024
	patterns := []struct {
		name  string
		alpha func(x, y int) uint8
	}{
		{name: "OpaqueInterior", alpha: func(x, _ int) uint8 {
			if x == 0 || x == width-1 {
				return 173
			}
			return 255
		}},
		{name: "TransparentAndOpaqueRuns", alpha: func(x, _ int) uint8 {
			if x == width/4 || x == width*3/4 {
				return 173
			}
			if x > width/4 && x < width*3/4 {
				return 255
			}
			return 0
		}},
		{name: "LatePartialEveryBlock", alpha: func(x, _ int) uint8 {
			if x%64 == 62 {
				return 173
			}
			return 255
		}},
		{name: "Alternating", alpha: func(x, _ int) uint8 {
			if x&1 == 0 {
				return 255
			}
			return 173
		}},
		{name: "AlphaRamp", alpha: func(x, _ int) uint8 { return uint8(x) }},
	}
	for _, pattern := range patterns {
		pattern := pattern
		b.Run(pattern.name, func(b *testing.B) {
			makeSource := func() *image.RGBA {
				source := image.NewRGBA(image.Rect(0, 0, width, height))
				for y := 0; y < height; y++ {
					for x := 0; x < width; x++ {
						offset := source.PixOffset(x, y)
						alpha := pattern.alpha(x, y)
						source.Pix[offset], source.Pix[offset+1], source.Pix[offset+2], source.Pix[offset+3] = alpha/2, alpha/3, alpha/4, alpha
					}
				}
				return source
			}
			for _, implementation := range []struct {
				name    string
				convert func(context.Context, *image.RGBA) (*image.NRGBA, error)
			}{
				{name: "Scalar", convert: rgbaToNRGBAScalarReference},
				{name: "UniformBlocks", convert: func(ctx context.Context, source *image.RGBA) (*image.NRGBA, error) {
					return rgbaToNRGBAInPlace(ctx, source, false)
				}},
			} {
				b.Run(implementation.name, func(b *testing.B) {
					source := makeSource()
					b.SetBytes(int64(len(source.Pix)))
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						frame, err := implementation.convert(context.Background(), source)
						if err != nil {
							b.Fatal(err)
						}
						benchmarkNRGBAFrame = frame
					}
				})
			}
		})
	}
}

func rgbaToNRGBAScalarReference(ctx context.Context, source *image.RGBA) (*image.NRGBA, error) {
	result := &image.NRGBA{Pix: source.Pix, Stride: source.Stride, Rect: source.Rect}
	bounds := source.Bounds()
	width := bounds.Dx()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		rowOffset := source.PixOffset(bounds.Min.X, y)
		row := source.Pix[rowOffset : rowOffset+width*4]
		for pixel := 0; pixel < width; pixel++ {
			if pixel != 0 && pixel&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			x := pixel * 4
			alpha := uint32(row[x+3])
			if alpha != 0 && alpha != 0xff {
				row[x] = uint8((uint32(row[x]) * 0xffff / alpha) >> 8)
				row[x+1] = uint8((uint32(row[x+1]) * 0xffff / alpha) >> 8)
				row[x+2] = uint8((uint32(row[x+2]) * 0xffff / alpha) >> 8)
			}
		}
	}
	return result, ctx.Err()
}

var benchmarkNRGBAFrame *image.NRGBA

func TestPNGEncodingIsDeterministic(t *testing.T) {
	document := testDocument(d2scene.Rect{
		Box: d2scene.Box{X: 1, Y: 2, Width: 7, Height: 6}, RadiusX: 2, RadiusY: 2,
		Fill: red, Stroke: &d2scene.Stroke{Paint: blue, Width: 1, Cap: d2scene.CapRound, Join: d2scene.JoinRound},
	})
	first, err := renderTestPNG(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderTestPNG(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("two RenderPNG calls produced different bytes")
	}
	decoded, err := png.Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 10 || decoded.Bounds().Dy() != 10 {
		t.Fatalf("decoded dimensions = %v", decoded.Bounds())
	}
	third, err := EncodePNG(context.Background(), decoded)
	if err != nil {
		t.Fatal(err)
	}
	fourth, err := EncodePNG(context.Background(), decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(third, fourth) {
		t.Fatal("two EncodePNG calls produced different bytes")
	}
}

func TestEncodePNGObservesCancellationDuringEncoding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	img := &cancelOnReadImage{
		Image:  image.NewNRGBA(image.Rect(0, 0, 4, 4)),
		cancel: cancel,
	}
	encoded, err := EncodePNG(ctx, img)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EncodePNG cancellation = %v, want context.Canceled", err)
	}
	if !img.canceled {
		t.Fatal("EncodePNG returned before reading the image")
	}
	if encoded != nil {
		t.Fatalf("EncodePNG returned %d bytes after cancellation", len(encoded))
	}
}

func BenchmarkRenderSimple(b *testing.B) {
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{X: 20, Y: 20, Width: 180, Height: 100}, RadiusX: 12, RadiusY: 12,
			Fill: red, Stroke: &d2scene.Stroke{Paint: black, Width: 3, Cap: d2scene.CapRound, Join: d2scene.JoinRound},
		}),
		d2scene.NewNode(d2scene.Ellipse{
			Center: d2scene.Point{X: 330, Y: 70}, RadiusX: 85, RadiusY: 50,
			Fill: green, Stroke: &d2scene.Stroke{Paint: black, Width: 3, Cap: d2scene.CapRound, Join: d2scene.JoinRound},
		}),
		d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{
				d2scene.MoveTo(110, 150), d2scene.CubicTo(175, 115, 280, 205, 350, 160),
			},
			Stroke: &d2scene.Stroke{
				Paint: blue, Width: 5, Cap: d2scene.CapRound, Join: d2scene.JoinRound,
				Dashes: []float64{12, 7}, DashOffset: 3,
			},
		}),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 488, Height: 272}, root)
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

func BenchmarkRenderText(b *testing.B) {
	fontBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		b.Fatal("embedded Source Sans Pro font is not loaded")
	}
	assetID := d2scene.AssetID("font:benchmark")
	document := d2scene.NewDocument(d2scene.Box{Width: 488, Height: 272}, d2scene.NewNode(d2scene.TextRun{
		Text:   "Deterministic D2 raster renderer",
		Origin: d2scene.Point{X: 244, Y: 136}, Anchor: d2scene.AnchorMiddle,
		Font: d2scene.Font{Size: 28, Asset: assetID}, Fill: black, Underline: true,
	}))
	document.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontBytes}
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

func testOptions() FrameOptions {
	return FrameOptions{
		Scale: 1, MaxWidth: 1_000, MaxHeight: 1_000, MaxPixels: 1_000_000,
		MaxNodes: 1_000, MaxDepth: 100, MaxPathCommands: 100_000, MaxTextRunesPerRun: 10_000,
		MaxAnimationTracks: 10_000, MaxAnimationKeyframes: 1_000_000,
		MaxAssets: 1_000, MaxAssetBytes: 64 * 1024 * 1024, MaxDecodedAssetBytes: 64 * 1024 * 1024, MaxImportDepth: 32,
		MaxOffscreenBytes: 64 * 1024 * 1024, MaxEvenOddClipWork: 1_000_000_000,
	}
}

func renderTestPNG(ctx context.Context, document *d2scene.Document, options FrameOptions) ([]byte, error) {
	frame, err := Render(ctx, document, options)
	if err != nil {
		return nil, err
	}
	return EncodePNG(ctx, frame)
}

func renderSessionTestPNG(ctx context.Context, session *RenderSession, document *d2scene.Document, options FrameOptions) ([]byte, error) {
	frame, err := session.Render(ctx, document, options)
	if err != nil {
		return nil, err
	}
	return EncodePNG(ctx, frame)
}

type cancelOnReadImage struct {
	image.Image
	cancel   context.CancelFunc
	canceled bool
}

func (i *cancelOnReadImage) At(x, y int) color.Color {
	if !i.canceled {
		i.canceled = true
		i.cancel()
	}
	return i.Image.At(x, y)
}

func testDocument(primitive d2scene.Primitive) *d2scene.Document {
	return d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, d2scene.NewNode(primitive))
}

func dimensionDocument(width, height float64) *d2scene.Document {
	return d2scene.NewDocument(d2scene.Box{Width: width, Height: height}, d2scene.NewNode(nil))
}

func withOption(options FrameOptions, change func(*FrameOptions)) FrameOptions {
	change(&options)
	return options
}

func assertPixel(t *testing.T, got, want color.NRGBA) {
	t.Helper()
	if got != want {
		t.Fatalf("pixel = %#v, want %#v", got, want)
	}
}

func assertAlpha(t *testing.T, got color.NRGBA, opaque bool) {
	t.Helper()
	if opaque && got.A < 200 {
		t.Fatalf("pixel = %#v, want opaque", got)
	}
	if !opaque && got.A != 0 {
		t.Fatalf("pixel = %#v, want transparent", got)
	}
}

func alphaXBounds(img *image.NRGBA, y0, y1 int) (int, int) {
	minX, maxX := img.Bounds().Dx(), -1
	for y := y0; y < y1; y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if img.NRGBAAt(x, y).A != 0 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
			}
		}
	}
	return minX, maxX
}

func countOpaqueInRow(img *image.NRGBA, y int) int {
	count := 0
	for x := 0; x < img.Bounds().Dx(); x++ {
		if img.NRGBAAt(x, y).A >= 200 {
			count++
		}
	}
	return count
}
