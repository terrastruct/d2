package d2isometricimg

import (
	"context"
	"image/color"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestNativeConnectionCaptionPositionAndPercentage(t *testing.T) {
	points := []Vec{nv(0, .1, 0), nv(4, .1, 0), nv(4, .1, 4)}
	original := d2target.Connection{LabelPosition: "UNLOCKED_TOP", LabelPercentage: .75, StrokeWidth: 2}
	s, ok := nativeConnectionCaptionSurface(points, original, 2, .4, .01)
	if !ok || math.Abs(s.center.X-4.26) > 1e-8 || math.Abs(s.center.Z-2) > 1e-8 || math.Abs(s.angle-math.Pi/2) > 1e-8 || s.width != 2 || s.depth != .4 {
		t.Fatalf("percentage/normal/angle not preserved: %+v %v", s, ok)
	}
	original.LabelPosition = "UNLOCKED_BOTTOM"
	s, _ = nativeConnectionCaptionSurface(points, original, 2, .4, .01)
	if math.Abs(s.center.X-3.74) > 1e-8 {
		t.Fatalf("below position ignored: %+v", s)
	}
	original.LabelPosition = "INSIDE_MIDDLE_LEFT"
	s, _ = nativeConnectionCaptionSurface(points, original, 2, .4, .01)
	if s.center.X != 2 || s.center.Z != 0 {
		t.Fatalf("fixed label position ignored: %+v", s)
	}
	original.LabelPosition = "UNLOCKED_MIDDLE"
	original.LabelPercentage = 0
	s, _ = nativeConnectionCaptionSurface(points, original, 2, .4, .01)
	if s.center.X != 0 || s.center.Z != 0 {
		t.Fatal("zero percentage treated as unset")
	}
	original.LabelPosition = ""
	if _, ok := nativeConnectionCaptionSurface(points, original, 2, .4, .01); ok {
		t.Fatal("unset caption bypasses collision placer")
	}
}

func TestNativeOnEdgeCaptionMasksWireWithResolvedThemeFill(t *testing.T) {
	ctx := context.Background()
	painter, err := newTextPainter(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := &meshBuilder{ctx: ctx, scale: .01, text: painter}
	e := d2isometric.Edge{ID: "caption", Label: "we get", Points: []Vec{nv(0, .08, 0), nv(4, .08, 0)}, StrokeWidth: 6, Stroke: "#f00000", StrokeExplicit: true, Opacity: 1}
	e.Metadata.Original.Fill = "N7"
	e.Metadata.Original.LabelPosition = "INSIDE_MIDDLE_CENTER"
	e.Metadata.Original.Text = d2target.Text{Label: e.Label, Color: "#102030", FontSize: 20, LabelWidth: 80, LabelHeight: 25}
	green := "#00ff66"
	scene := &d2isometric.Scene{ThemeOverrides: &d2target.ThemeOverrides{N7: &green}}
	b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer(), scene)
	if b.err != nil {
		t.Fatal(b.err)
	}
	found := false
	for _, tri := range b.triangles {
		if tri.Material.Texture == nil {
			continue
		}
		found = true
		for _, v := range tri.V {
			if v.Position.Y <= .08+nativeRouteRadius(e) {
				t.Fatal("on-edge label plane is inside the cable")
			}
		}
	}
	if !found {
		t.Fatal("on-edge caption disappeared")
	}
	raster, err := NewRaster(ctx, 320, 200, b.triangles, color.NRGBA{255, 255, 255, 255})
	if err != nil {
		t.Fatal(err)
	}
	img, err := raster.Frame(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	greenPixels := 0
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			r, g, _, _ := img.At(x, y).RGBA()
			if g > 50000 && r < 10000 {
				greenPixels++
			}
		}
	}
	if greenPixels < 50 {
		t.Fatalf("theme-backed on-edge label is obscured in raster output: %d pixels", greenPixels)
	}
}

func TestNativePacketsRequireAuthoredAnimation(t *testing.T) {
	e := d2isometric.Edge{ID: "lifeline", Points: []Vec{nv(0, .08, 0), nv(4, .08, 0)}, StrokeWidth: 2, Stroke: "#223344", StrokeExplicit: true, Opacity: 1}
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	if packets := b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer()); len(packets) != 0 || b.err != nil {
		t.Fatalf("unanimated lifeline packets: %d %v", len(packets), b.err)
	}
	e.Metadata.Original.Animated = true
	e.SourceArrow, e.TargetArrow = d2target.UnfilledTriangleArrowhead, d2target.UnfilledTriangleArrowhead
	if packets := b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer()); len(packets) != 1 || b.err != nil {
		t.Fatalf("authored animation omitted: %d %v", len(packets), b.err)
	} else if packets[0].points[0] != e.Points[0] || packets[0].points[len(packets[0].points)-1] != e.Points[len(e.Points)-1] {
		t.Fatal("hollow arrow wire clearance changed traffic endpoint routes")
	}
	e.Stroke = "transparent"
	if packets := b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer()); len(packets) != 0 {
		t.Fatal("transparent route emits traffic")
	}
}

func TestNativeConnectionCaptionFillAndOpacity(t *testing.T) {
	ctx := context.Background()
	painter, err := newTextPainter(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := &meshBuilder{ctx: ctx, scale: .01, text: painter}
	e := d2isometric.Edge{ID: "filled-label", Label: "data", Points: []Vec{nv(0, .08, 0), nv(4, .08, 0)}, StrokeWidth: 2, Stroke: "#223344", StrokeExplicit: true, Opacity: .5}
	e.Metadata.Original.Fill = "#ff0080"
	e.Metadata.Original.Text = d2target.Text{Label: "data", FontSize: 16, Color: "#ffffff", LabelWidth: 50, LabelHeight: 20}
	b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
	if b.err != nil {
		t.Fatal(b.err)
	}
	for _, tri := range b.triangles {
		if tex := tri.Material.Texture; tex != nil {
			r, g, blue, a := tex.At(tex.Bounds().Min.X, tex.Bounds().Min.Y).RGBA()
			if a < 32000 || a > 33000 || r < 32000 || g != 0 || blue < 15000 || blue > 17000 {
				t.Fatalf("caption fill/opacity not forwarded exactly once: %d %d %d %d", r, g, blue, a)
			}
			return
		}
	}
	t.Fatal("caption texture was not drawn")
}
