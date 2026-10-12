package d2isometricimg

import (
	"context"
	"fmt"
	"image/color"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func captionMaskTestEdge() d2isometric.Edge {
	e := d2isometric.Edge{ID: "caption", Label: ".", Points: []Vec{nv(-3, .08, 0), nv(3, .08, 0)}, StrokeWidth: 8, StrokeDash: 2, Stroke: "#a12639", StrokeExplicit: true, Opacity: 1}
	e.Metadata.Original.LabelPosition = "INSIDE_MIDDLE_CENTER"
	e.Metadata.Original.Text = d2target.Text{Label: e.Label, Color: "#173849", FontSize: 16, LabelWidth: 100, LabelHeight: 30}
	return e
}

func TestConnectionCaptionBackgroundRequiresExplicitFill(t *testing.T) {
	green := "#20c870"
	for _, tc := range []struct {
		name, fill, labelFill string
		want                  color.NRGBA
	}{
		{name: "default"},
		{name: "explicit transparent", fill: "transparent"},
		{name: "explicit white", fill: "#ffffff", want: color.NRGBA{255, 255, 255, 255}},
		{name: "explicit theme token", fill: "N7", want: color.NRGBA{32, 200, 112, 255}},
		{name: "text label fill", labelFill: "#e0b080", want: color.NRGBA{224, 176, 128, 255}},
		{name: "connection fill precedence", fill: "#ffffff", labelFill: "#e0b080", want: color.NRGBA{255, 255, 255, 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			painter, err := newTextPainter(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			e := captionMaskTestEdge()
			e.Metadata.Original.Fill, e.Metadata.Original.LabelFill = tc.fill, tc.labelFill
			b := &meshBuilder{ctx: ctx, scale: .01, text: painter}
			b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer(), &d2isometric.Scene{Background: "#ccddee", ThemeOverrides: &d2target.ThemeOverrides{N7: &green}})
			if b.err != nil {
				t.Fatal(b.err)
			}
			for _, triangle := range b.triangles {
				if tex := triangle.Material.Texture; tex != nil {
					got := color.NRGBAModel.Convert(tex.At(tex.Bounds().Min.X, tex.Bounds().Min.Y)).(color.NRGBA)
					if got != tc.want {
						t.Fatalf("caption corner got %v, want explicit backing %v", got, tc.want)
					}
					return
				}
			}
			t.Fatal("caption texture disappeared")
		})
	}
}

func TestRouteCaptionKnockoutPreservesProjectedDashesAndAlpha(t *testing.T) {
	for _, angle := range []float64{math.Pi / 2, math.Pi / 4, -math.Pi / 3} {
		t.Run(fmt.Sprintf("angle=%.3f", angle), func(t *testing.T) {
			ctx := context.Background()
			boardColor := color.NRGBA{126, 172, 191, 255}
			board := nativeMaterial("#7eacbf", 1, 0, 1)
			board.Unlit = true
			b := &meshBuilder{ctx: ctx, scale: .01}
			b.flat(nv(-4, -.3, -4), nv(-4, -.3, 4), nv(4, -.3, 4), board, false)
			b.flat(nv(-4, -.3, -4), nv(4, -.3, 4), nv(4, -.3, -4), board, false)
			first := len(b.triangles)
			direction := nv(math.Cos(angle), 0, math.Sin(angle))
			center := nv(0, .08, 0)
			points := []Vec{nadd(center, nmul(direction, -3)), nadd(center, nmul(direction, 3))}
			ink := nativeMaterial("#a12639", 1, 0, .5)
			ink.Unlit = true
			for _, dash := range nativeRouteDashes(points, routeLengths(points), 0, 1, 2, 8000) {
				b.routeCasing(dash, .075, .5)
				b.routeInk(dash, .075, ink)
			}
			baseline := append([]Triangle(nil), b.triangles...)
			surface := labelSurface{center: nadd(center, nv(0, .081, 0)), width: 1.6, depth: .27, angle: angle}
			b.routeCaptionKnockout(first, surface)
			if b.err != nil {
				t.Fatal(b.err)
			}
			if !reflect.DeepEqual(b.triangles[:first], baseline[:first]) {
				t.Fatal("caption clipping altered the earlier colored surface")
			}
			full, err := NewRaster(ctx, 560, 420, baseline, boardColor)
			if err != nil {
				t.Fatal(err)
			}
			clipped, err := newRaster(ctx, 560, 420, b.triangles, boardColor, &full.camera, nil)
			if err != nil {
				t.Fatal(err)
			}
			before, err := full.Frame(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			after, err := clipped.Frame(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Derive local caption coordinates from the raster camera itself,
			// independently of the clipper's common-plane projection helper.
			origin := full.camera.project(surface.center)
			along := full.camera.project(nadd(surface.center, direction))
			across := full.camera.project(nadd(surface.center, nv(-direction.Z, 0, direction.X)))
			ux, uy := along.x-origin.x, along.y-origin.y
			vx, vy := across.x-origin.x, across.y-origin.y
			det := ux*vy - uy*vx
			removed, preserved := 0, 0
			for y := 0; y < after.Bounds().Dy(); y++ {
				for x := 0; x < after.Bounds().Dx(); x++ {
					dx, dy := (float64(x)+.5)*float64(full.aa)-origin.x, (float64(y)+.5)*float64(full.aa)-origin.y
					u, v := (dx*vy-dy*vx)/det, (dy*ux-dx*uy)/det
					old := color.NRGBAModel.Convert(before.At(x, y)).(color.NRGBA)
					got := color.NRGBAModel.Convert(after.At(x, y)).(color.NRGBA)
					if math.Abs(u) < surface.width/2-.05 && math.Abs(v) < surface.depth/2-.04 {
						if got != boardColor {
							t.Fatalf("wire or casing leaked through transparent gap at (%d,%d): got %v, want %v", x, y, got, boardColor)
						}
						if old != boardColor {
							removed++
						}
					} else if math.Abs(u) > surface.width/2+.05 || math.Abs(v) > surface.depth/2+.05 {
						if got != old {
							t.Fatalf("caption changed dash phase or alpha outside its gap at (%d,%d): before %v, after %v", x, y, old, got)
						}
						if old != boardColor {
							preserved++
						}
					}
				}
			}
			if removed < 10 || preserved < 100 {
				t.Fatalf("fixture did not exercise the gap and retained dashes: removed %d, preserved %d", removed, preserved)
			}
		})
	}
}

func TestRouteCaptionKnockoutPreservesTriangleMetadata(t *testing.T) {
	ctx := context.Background()
	mat := nativeMaterial("#223344", .7, .2, .5)
	opacity, alpha, ground := .25, uint8(96), -.5
	original := Triangle{
		V:        [3]Vertex{{Position: nv(-2, .08, -.1)}, {Position: nv(2, .08, -.1)}, {Position: nv(2, .08, .1)}},
		Material: mat, CastShadow: true, ShadowOpacity: &opacity, ShadowFillAlpha: &alpha, ShadowGround: &ground,
		DepthBias: .007, NoDepthWrite: true, OpacityGroup: &nativeOpacityGroup{}, PaintOwner: &nativePaintOwner{}, svgCoverageOnly: true,
	}
	b := &meshBuilder{ctx: ctx, triangles: []Triangle{original}}
	b.routeCaptionKnockout(0, labelSurface{center: nv(0, .1, 0), width: 1, depth: .4})
	if b.err != nil {
		t.Fatal(b.err)
	}
	if len(b.triangles) < 2 {
		t.Fatal("fixture failed to split triangle across both sides of the caption")
	}
	for _, triangle := range b.triangles {
		triangle.V = original.V
		if !reflect.DeepEqual(triangle, original) {
			t.Fatal("clipping dropped source triangle metadata")
		}
	}
}

func TestConnectionCaptionKnockoutPreservesEarlierGeometryAndMarkers(t *testing.T) {
	ctx := context.Background()
	e := captionMaskTestEdge()
	e.SourceArrow, e.TargetArrow = d2target.CircleArrowhead, d2target.ArrowArrowhead
	painter, err := newTextPainter(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	earlier := Triangle{V: [3]Vertex{{Position: nv(-1, -.1, -1)}, {Position: nv(-1, -.1, 1)}, {Position: nv(1, -.1, 1)}}, Material: nativeMaterial("#afc1ce", 1, 0, 1)}
	with := &meshBuilder{ctx: ctx, scale: .01, text: painter, triangles: []Triangle{earlier}}
	with.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
	e.Label = ""
	without := &meshBuilder{ctx: ctx, scale: .01, triangles: []Triangle{earlier}}
	without.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
	if with.err != nil || without.err != nil {
		t.Fatal(with.err, without.err)
	}
	if !reflect.DeepEqual(with.triangles[0], earlier) {
		t.Fatal("new caption cut through geometry belonging to an earlier object")
	}
	markers := func(triangles []Triangle) []Triangle {
		var out []Triangle
		for _, triangle := range triangles {
			if triangle.Material.Texture == nil && triangle.DepthBias > 0 {
				out = append(out, triangle)
			}
		}
		return out
	}
	before, after := markers(without.triangles), markers(with.triangles)
	if len(before) == 0 || !reflect.DeepEqual(before, after) {
		t.Fatal("caption changed endpoint marker geometry")
	}
}

func TestConnectionCaptionGapRevealsUnderlyingSurface(t *testing.T) {
	for _, angle := range []float64{math.Pi / 2, math.Pi / 4} {
		t.Run(fmt.Sprintf("angle=%.3f", angle), func(t *testing.T) {
			ctx := context.Background()
			painter, err := newTextPainter(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			e := captionMaskTestEdge()
			e.StrokeDash = 0
			e.Metadata.Original.LabelHeight = 8
			direction := nv(math.Cos(angle), 0, math.Sin(angle))
			e.Points = []Vec{nadd(nv(0, .08, 0), nmul(direction, -3)), nadd(nv(0, .08, 0), nmul(direction, 3))}
			boardColor := color.NRGBA{126, 172, 191, 255}
			board := nativeMaterial("#7eacbf", 1, 0, 1)
			board.Unlit = true
			b := &meshBuilder{ctx: ctx, scale: .01, text: painter}
			b.flat(nv(-4, -.3, -4), nv(-4, -.3, 4), nv(4, -.3, 4), board, false)
			b.flat(nv(-4, -.3, -4), nv(4, -.3, 4), nv(4, -.3, -4), board, false)
			// The reference contains precisely the same board and caption, but
			// no wire. Inside the gap, it must match the complete render.
			reference := append([]Triangle(nil), b.triangles...)
			without := &meshBuilder{ctx: ctx, scale: .01, triangles: append([]Triangle(nil), b.triangles...)}
			b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
			e.Label = ""
			without.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
			if b.err != nil || without.err != nil {
				t.Fatal(b.err, without.err)
			}
			var labelTriangle Triangle
			for _, triangle := range b.triangles {
				if triangle.Material.Texture != nil {
					if labelTriangle.Material == nil {
						labelTriangle = triangle
					}
					reference = append(reference, triangle)
				}
			}
			if labelTriangle.Material == nil {
				t.Fatal("caption disappeared")
			}
			actual, err := NewRaster(ctx, 800, 600, b.triangles, boardColor)
			if err != nil {
				t.Fatal(err)
			}
			ref, err := newRaster(ctx, 800, 600, reference, boardColor, &actual.camera, nil)
			if err != nil {
				t.Fatal(err)
			}
			unlabeled, err := newRaster(ctx, 800, 600, without.triangles, boardColor, &actual.camera, nil)
			if err != nil {
				t.Fatal(err)
			}
			gotImage, err := actual.Frame(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantImage, err := ref.Frame(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			before, err := unlabeled.Frame(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Read the actual printed quad and project it with the raster
			// camera, so a caller using the wrong caption height is detected.
			p := actual.camera.project(labelTriangle.V[0].Position)
			q := actual.camera.project(labelTriangle.V[1].Position)
			r := actual.camera.project(labelTriangle.V[2].Position)
			ux, uy, vx, vy := r.x-q.x, r.y-q.y, q.x-p.x, q.y-p.y
			det := ux*vy - uy*vx
			removed := 0
			for y := 0; y < gotImage.Bounds().Dy(); y++ {
				for x := 0; x < gotImage.Bounds().Dx(); x++ {
					dx, dy := (float64(x)+.5)*float64(actual.aa)-p.x, (float64(y)+.5)*float64(actual.aa)-p.y
					u, v := (dx*vy-dy*vx)/det, (dy*ux-dx*uy)/det
					if u <= .07 || u >= .93 || v <= .15 || v >= .85 {
						continue
					}
					got, want := gotImage.RGBAAt(x, y), wantImage.RGBAAt(x, y)
					if got != want {
						t.Fatalf("wire/casing still intersects caption at (%d,%d): got %v, want %v", x, y, got, want)
					}
					if before.RGBAAt(x, y) != want {
						removed++
					}
				}
			}
			if removed < 20 {
				t.Fatalf("fixture did not exercise enough wire beneath the caption: %d pixels", removed)
			}
		})
	}
}
