package d2raster

import (
	"context"
	"image"
	"image/draw"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestStrokeRegionsIncludeLargeRotatedRoundCapsAndJoins(t *testing.T) {
	for _, join := range []bool{false, true} {
		commands := []d2scene.PathCommand{d2scene.MoveTo(0, 0), d2scene.LineTo(-100, 0)}
		stroke := &d2scene.Stroke{Paint: red, Width: 20000, Cap: d2scene.CapRound, Join: d2scene.JoinBevel, MiterLimit: 4}
		if join {
			commands = []d2scene.PathCommand{d2scene.MoveTo(-100, -100), d2scene.LineTo(0, 0), d2scene.LineTo(-100, 0)}
			stroke.Cap = d2scene.CapButt
			stroke.Join = d2scene.JoinRound
		}
		// A distant run keeps the complete primitive's bounds broad enough that
		// the rotated cubic's fringe is not clipped by the old full-frame bounds.
		commands = append(commands, d2scene.MoveTo(100000, 0), d2scene.LineTo(100100, 0))
		node := d2scene.NewNode(d2scene.Path{Commands: commands, Stroke: stroke})
		node.Opacity = .73
		angle := 20 * math.Pi / 180
		node.Transform = d2scene.Matrix{A: math.Cos(angle), B: math.Sin(angle), C: -math.Sin(angle), D: math.Cos(angle)}
		document := d2scene.NewDocument(d2scene.Box{X: -20, Y: 10000, Width: 40, Height: 8}, node)
		options := testOptions()
		want, err := Render(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		if want.NRGBAAt(19, 2).A == 0 {
			t.Fatalf("join=%v: missing reference cubic fringe", join)
		}
		got := image.NewNRGBA(want.Bounds())
		if err := RenderBands(context.Background(), document, options, 1, func(b *image.NRGBA) error { draw.Draw(got, b.Bounds(), b, b.Bounds().Min, draw.Src); return nil }); err != nil {
			t.Fatal(err)
		}
		assertBandPixels(t, got, want)
	}
}

func TestStrokeRegionExtentEnclosesCircleControls(t *testing.T) {
	const radius = 10000.0
	const control = .5522847498307936
	stroke := &preparedStroke{width: radius * 2, cap: d2scene.CapRound, join: d2scene.JoinBevel}
	for _, matrix := range []d2scene.Matrix{d2scene.Identity(), {A: .9396926207859084, B: .3420201433256687, C: -.3420201433256687, D: .9396926207859084}, {A: 1.3, B: .2, C: -.7, D: .9}} {
		extent := strokeRegionExtent(stroke, matrix)
		for _, point := range []d2scene.Point{{X: radius, Y: radius * control}, {X: radius * control, Y: radius}, {X: -radius, Y: radius * control}, {X: -radius * control, Y: -radius}} {
			transformed := matrix.Point(point)
			if math.Abs(transformed.X) > extent || math.Abs(transformed.Y) > extent {
				t.Fatalf("control%v exceeds extent%v", transformed, extent)
			}
		}
	}
	if got := strokeExtent(stroke, d2scene.Identity()); got != radius {
		t.Fatalf("full-frame extent changed: %v", got)
	}
}

func TestRegionBoundsRejectCancellingTransforms(t *testing.T) {
	prepared, err := prepareWithSessionBands(context.Background(), regionMaskDocument(), testOptions(), nil, 64)
	if err != nil {
		t.Fatal(err)
	}
	mask := prepared.root.mask
	base := mask.root.children[0].primitive
	// Preserve finite, small transformed corners while introducing a huge
	// intermediate translation. It must prevent the white-backdrop shortcut.
	base.transform.E = 1e18
	for i := range base.subpaths[0].points {
		base.subpaths[0].points[i].X = -1e18 + base.subpaths[0].points[i].X
	}
	destination := image.Rect(20, 20, 180, 150)
	got, err := maskRenderBounds(context.Background(), mask, destination, true)
	if err != nil || got != destination {
		t.Fatalf("mask fallback=%v error=%v", got, err)
	}
	primitive := &preparedPrimitive{bounds: destination, referenceBounds: destination, transform: d2scene.Translate(1e18, 0), stroke: &preparedStroke{width: 2, join: d2scene.JoinBevel}, strokeRuns: []strokeRun{{points: []d2scene.Point{{X: -1e18 + 512, Y: 30}, {X: -1e18 + 1024, Y: 30}}}}}
	got, err = strokeRegionBounds(context.Background(), primitive, destination)
	if err != nil || got != destination {
		t.Fatalf("stroke fallback=%v error=%v", got, err)
	}
	if boundedStrokeLocalPoint(d2scene.Point{X: 1e18, Y: 1e18}, 2) {
		t.Fatal("large cancelling local products admitted")
	}
	if !boundedStrokeLocalPoint(d2scene.Point{X: 100000, Y: -100000}, 4) {
		t.Fatal("ordinary local coordinates rejected")
	}
}
