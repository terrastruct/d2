package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"strings"
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

func assertNativeCaptionFont(t *testing.T, style d2target.Text, surface labelSurface) {
	t.Helper()
	painter, err := newTextPainter(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	b := &meshBuilder{scale: .01}
	_, layout, err := painter.texture(style.Label, b.printStyle(surface, style, "#223344", 1, "edge"))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(layout.FontSize-float64(style.FontSize)*.01) > 1e-9 || strings.Join(layout.Lines, "\n") != style.Label {
		t.Fatalf("caption changed authored font or lines: font %.3fpx, want %dpx; lines %q", layout.FontSize/.01, style.FontSize, layout.Lines)
	}
}

func TestNativeCurvedCaptionsPreserveCompiledPrintMetrics(t *testing.T) {
	d := sourcePanelFixture(t, "stable/edge-label-overflow/dagre/board.exp.json")
	before, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := d2isometric.BuildScene(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, edge := range scene.Edges {
		if edge.Label == "" || !edge.Metadata.Original.IsCurve || len(edge.Points) <= len(edge.Metadata.Original.Route) {
			continue
		}
		style := edge.Metadata.Original.Text
		width, depth := nativeRouteCaptionSize(style, false, scene.PixelScale)
		if width < float64(style.LabelWidth)*scene.PixelScale || depth < float64(style.LabelHeight)*scene.PixelScale {
			t.Fatalf("curve samples squeezed the %dx%dpx compiled print area into %.3fx%.3fpx", style.LabelWidth, style.LabelHeight, width/scene.PixelScale, depth/scene.PixelScale)
		}
		assertNativeCaptionFont(t, style, labelSurface{width: width, depth: depth})
		seen++
	}
	if seen == 0 {
		t.Fatal("fixture lost its curved connection captions")
	}
	after, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("caption sizing mutated compiled shape positions or other source data")
	}
}

func TestNativeRouteCaptionPreservesMultilineFont(t *testing.T) {
	style := d2target.Text{Label: "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight", LabelWidth: 64, LabelHeight: 168, FontSize: 16}
	width, depth := nativeRouteCaptionSize(style, false, .01)
	if depth < 1.68 {
		t.Fatalf("multiline caption lost compiled height: %.3fpx, want at least 168px", depth/.01)
	}
	assertNativeCaptionFont(t, style, labelSurface{width: width, depth: depth})
}

func TestNativeRouteCaptionsKeepEndpointAndIconPrintSpace(t *testing.T) {
	ctx := context.Background()
	styles := []d2target.Text{
		{Label: "Main connection", LabelWidth: 180, LabelHeight: 21, FontSize: 16},
		{Label: "Source endpoint", LabelWidth: 180, LabelHeight: 21, FontSize: 16},
		{Label: "Target endpoint", LabelWidth: 180, LabelHeight: 21, FontSize: 16},
	}
	iconURL := iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
	for _, withIcon := range []bool{false, true} {
		t.Run(fmt.Sprintf("icon=%t", withIcon), func(t *testing.T) {
			painter, err := newTextPainter(ctx, len(styles))
			if err != nil {
				t.Fatal(err)
			}
			icons, err := newSurfaceIconPainter(ctx, 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Each caption is much wider than the complete route. Neighboring
			// main/end labels must never receive fractional shares of its span.
			e := d2isometric.Edge{ID: "short", Label: styles[0].Label, Points: []Vec{nv(0, .08, 0), nv(.01, .08, 0), nv(.02, .08, 0)}, Opacity: 1, SourceLabel: &styles[1], TargetLabel: &styles[2]}
			e.Metadata.Original.Text = styles[0]
			if withIcon {
				e.Icon, e.Metadata.Original.Icon = iconURL.String(), iconURL
			}
			b := &meshBuilder{ctx: ctx, scale: .01, text: painter, icons: icons}
			b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
			if b.err != nil {
				t.Fatal(b.err)
			}
			var surfaces []labelSurface
			for i := 0; i < len(b.triangles); i++ {
				tri := b.triangles[i]
				if tri.Material.Texture == nil {
					continue
				}
				surfaces = append(surfaces, labelSurface{width: nlen(nsub(tri.V[2].Position, tri.V[1].Position)), depth: nlen(nsub(tri.V[1].Position, tri.V[0].Position))})
				i++ // Both triangles belong to the same printed rectangle.
			}
			if withIcon {
				if len(surfaces) != 4 || surfaces[0].width < .3 || surfaces[0].depth < .3 {
					t.Fatalf("connection icon lost its print area: %+v", surfaces)
				}
				surfaces = surfaces[1:]
			}
			if len(surfaces) != len(styles) {
				t.Fatalf("missing main or endpoint caption: %d surfaces", len(surfaces))
			}
			for i, surface := range surfaces {
				if surface.width < float64(styles[i].LabelWidth)*.01 || surface.depth < float64(styles[i].LabelHeight)*.01 {
					t.Fatalf("caption %d lost compiled print area: %+v", i, surface)
				}
				assertNativeCaptionFont(t, styles[i], surface)
			}
		})
	}

	// Long labels require more glyph-overhang allowance; icon partitioning
	// must not fall back to a vertical arrangement and squeeze their height.
	style := d2target.Text{Label: "long label", LabelWidth: 2400, LabelHeight: 21, FontSize: 16}
	width, depth := nativeRouteCaptionSize(style, true, .01)
	icon, text := surfaceIconLayout(labelSurface{width: width, depth: depth}, d2target.Shape{Text: style, Icon: iconURL}, .01, "edge")
	if icon.width < .3 || text.width < 24 || text.depth < .21 {
		t.Fatalf("long label's icon squeezed authored print dimensions: icon %+v, text %+v", icon, text)
	}
	assertNativeCaptionFont(t, style, text)
}

func TestNativeRouteCaptionMissingMetricsUseBoundedFallback(t *testing.T) {
	short := d2target.Text{Label: "missing dimensions", FontSize: 16}
	long := short
	long.Label = strings.Repeat("x", maxLabelRunes)
	w1, d1 := nativeRouteCaptionSize(short, false, .01)
	w2, d2 := nativeRouteCaptionSize(long, false, .01)
	if w1 != w2 || d1 != d2 || !captionFinite(w1, d1) || w1 <= 0 || d1 <= 0 || w1 > 1 || d1 > 1 {
		t.Fatalf("missing metrics produced an invalid or unbounded fallback: %gx%g and %gx%g", w1, d1, w2, d2)
	}
	width, depth := nativeRouteCaptionSize(d2target.Text{}, true, .01)
	if width < .32 || depth < .32 || width > 1 || depth > 1 {
		t.Fatalf("icon-only caption has no usable bounded print area: %gx%g", width, depth)
	}
}

func TestNativeOverflowCaptionsKeepSourceOrientation(t *testing.T) {
	for _, fixture := range []string{"stable/edge-label-overflow/dagre/board.exp.json", "regression/opacity-on-label/elk/board.exp.json"} {
		t.Run(fixture, func(t *testing.T) {
			diagram := sourcePanelFixture(t, fixture)
			before, err := json.Marshal(diagram)
			if err != nil {
				t.Fatal(err)
			}
			scene, err := d2isometric.BuildScene(diagram, nil)
			if err != nil {
				t.Fatal(err)
			}
			seen := 0
			var surfaces []labelSurface
			for _, edge := range scene.Edges {
				if edge.Label == "" {
					continue
				}
				style := edge.Metadata.Original.Text
				width, depth := nativeRouteCaptionSize(style, false, scene.PixelScale)
				surface, ok := nativeConnectionCaptionSurface(edge.Points, edge.Metadata.Original, width, depth, scene.PixelScale)
				if !ok || surface.angle != 0 {
					t.Fatalf("overflowing label %q still rotates toward the raised endpoints: %+v", edge.Label, surface)
				}
				anchor, _ := nativeConnectionCaptionSurface(edge.Points, edge.Metadata.Original, .01, depth, scene.PixelScale)
				labelTopLeft := edge.Metadata.Original.GetLabelTopLeft()
				routeStart := edge.Metadata.Original.Route[0]
				wantX := edge.Points[0].X + (labelTopLeft.X+float64(style.LabelWidth)/2-routeStart.X)*scene.PixelScale
				wantZ := edge.Points[0].Z + (labelTopLeft.Y+float64(style.LabelHeight)/2-routeStart.Y)*scene.PixelScale
				if surface.center.X != wantX || surface.center.Z != wantZ || surface.center.Y != anchor.center.Y || surface.width != width || surface.depth != depth {
					t.Fatal("source orientation lost the compiled label anchor, elevation, or print dimensions")
				}
				for _, prior := range surfaces {
					if projectedTestSurfacesOverlap(prior, surface) {
						t.Fatal("labels separated by the compiled layout overlap in the final projection")
					}
				}
				surfaces = append(surfaces, surface)
				assertNativeCaptionFont(t, style, surface)
				seen++
			}
			if seen == 0 {
				t.Fatal("fixture lost its overflowing captions")
			}
			after, err := json.Marshal(diagram)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("caption orientation changed source nodes, routes, or label metrics")
			}
		})
	}
}

func TestNativeCaptionOrientationUsesCompleteRouteRuns(t *testing.T) {
	original := d2target.Connection{LabelPosition: "INSIDE_MIDDLE_CENTER"}
	for _, segments := range []int{1, 10, 1000} {
		points := make([]Vec, segments+1)
		for i := range points {
			points[i] = nv(0, .08, float64(i)*10/float64(segments))
		}
		surface, ok := nativeConnectionCaptionSurface(points, original, 1.2, .3, .01)
		if !ok || surface.angle != math.Pi/2 || nlen(nsub(surface.center, nv(0, .086, 5))) > 1e-10 {
			t.Fatalf("subdividing a long straight leg changed print direction or anchor: %d segments, %+v", segments, surface)
		}
	}
	// A long adjacent leg cannot supply space beyond a corner. Only the run
	// containing the authored anchor can hold a leg-aligned caption.
	points := []Vec{nv(0, .08, 0), nv(0, .08, 1), nv(9, .08, 1)}
	original.LabelPosition, original.LabelPercentage = "UNLOCKED_MIDDLE", .05
	surface, ok := nativeConnectionCaptionSurface(points, original, 1.2, .3, .01)
	if !ok || surface.angle != 0 || nlen(nsub(surface.center, nv(0, .086, .5))) > 1e-10 {
		t.Fatalf("an adjacent leg caused the label to extend across the elbow: %+v", surface)
	}
}
