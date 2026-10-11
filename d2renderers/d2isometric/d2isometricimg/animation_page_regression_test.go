package d2isometricimg

import (
	"context"
	"image/color"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func TestAnimationFramingKeepsTinyNodeAndTrafficGeometry(t *testing.T) {
	tinyNode := d2target.NewDiagram()
	shape := d2target.BaseShape()
	shape.ID, shape.Type, shape.Width, shape.Height = "tiny", d2target.ShapeText, 1, 1
	shape.Fill, shape.Stroke, shape.StrokeWidth, shape.Animated = "#c42b25", "none", 0, true
	tinyNode.Shapes = []d2target.Shape{*shape}
	tinyRoute := captureDiagram()
	for i := range tinyRoute.Shapes {
		s := &tinyRoute.Shapes[i]
		s.Width, s.Height, s.Opacity, s.Label = 1, 1, 0, ""
		s.Pos = d2target.Point{X: i * 2}
	}
	e := &tinyRoute.Connections[0]
	e.SrcArrow, e.DstArrow = d2target.NoArrowhead, d2target.NoArrowhead
	e.Route = []*geo.Point{{X: 0}, {X: 2}}
	for _, fixture := range []struct {
		name string
		d    *d2target.Diagram
	}{{"tiny-node", tinyNode}, {"tiny-traffic", tinyRoute}} {
		for _, timeline := range []bool{false, true} {
			mode := "capture"
			if timeline {
				mode = "timeline"
			}
			t.Run(fixture.name+"/"+mode, func(t *testing.T) {
				ctx := context.Background()
				o := mustNormalize(t, Options{Format: GIF, Width: 320, Height: 200, FitContent: true, Render: d2isometric.RenderOpts{}})
				if timeline {
					camera, err := timelineCamera(ctx, []*d2target.Diagram{fixture.d}, o)
					if err != nil {
						t.Fatal(err)
					}
					o.Width, o.Height, o.camera = camera.width, camera.height, &camera
				}
				capture, err := openCapture(ctx, fixture.d, o)
				if err != nil {
					t.Fatal(err)
				}
				defer capture.close()
				scene := capture.scene
				camera := cameraAtResolution(scene.raster.camera, scene.width, scene.height)
				staticCamera := rasterFit(scene.triangles, nativeViewDirection(), scene.width, scene.height, 1.08)
				var dynamic []Triangle
				for _, node := range scene.animatedNodes {
					for _, triangle := range scene.triangles[node.first:node.last] {
						for i := range triangle.V {
							// Authored shape motion reaches four source pixels at
							// half a second, farther than this tiny face's width.
							triangle.V[i].Position.Z -= 4 * scene.pixelScale
						}
						dynamic = append(dynamic, triangle)
					}
				}
				packets := &meshBuilder{ctx: ctx}
				for _, packet := range scene.packets {
					for _, phase := range []float64{0, .25, .5, .75, 1} {
						packets.sphere(pathPoint(packet.points, packet.lengths, phase), nv(nativeTrafficRadius, nativeTrafficRadius, nativeTrafficRadius), packet.material, 10, 8)
					}
				}
				dynamic = append(dynamic, packets.triangles...)
				if len(dynamic) == 0 {
					t.Fatal("fixture has no animated geometry")
				}
				inside := func(c rasterCamera, v Vec) bool {
					p := c.project(v)
					return p.x >= 0 && p.y >= 0 && p.x <= float64(c.width) && p.y <= float64(c.height)
				}
				previouslyClipped := false
				for _, triangle := range dynamic {
					for _, vertex := range triangle.V {
						previouslyClipped = previouslyClipped || !inside(staticCamera, vertex.Position)
						if !inside(camera, vertex.Position) {
							t.Fatalf("animation leaves the fixed %dx%d frame: %+v", camera.width, camera.height, camera.project(vertex.Position))
						}
					}
				}
				if !previouslyClipped {
					t.Fatal("fixture must demonstrate clipping when only static geometry is framed")
				}
				for _, phase := range []float64{0, .5} {
					frame, err := capture.frameImage(phase, true)
					if err != nil || frame.Bounds().Dx() != camera.width || frame.Bounds().Dy() != camera.height {
						t.Fatal("animation changed frame dimensions", err)
					}
				}
			})
		}
	}
}

func TestOutsideMarkdownPageLinksFollowFinalSurfaces(t *testing.T) {
	for _, position := range []string{"OUTSIDE_BOTTOM_CENTER", "OUTSIDE_RIGHT_MIDDLE"} {
		for _, hierarchy := range []bool{false, true} {
			mode := "full-depth"
			if hierarchy {
				mode = "hierarchy"
			}
			t.Run(position+"/"+mode, func(t *testing.T) {
				ctx := context.Background()
				d := d2target.NewDiagram()
				s := d2target.BaseShape()
				s.ID, s.Type, s.Width, s.Height = "documentation", d2target.ShapeRectangle, 260, 160
				s.Label, s.Language = "[Read the guide](https://example.test/guide)", "markdown"
				s.LabelWidth, s.LabelHeight, s.FontSize, s.LabelPosition = 120, 26, 16, position
				s.Fill, s.Stroke = "#dddddd", "#666666"
				d.Shapes = []d2target.Shape{*s}
				source, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
				if err != nil {
					t.Fatal(err)
				}
				text, err := newTextPainter(ctx, 1)
				if err != nil {
					t.Fatal(err)
				}
				rich, err := newRichLabelPainter(ctx, 1)
				if err != nil {
					t.Fatal(err)
				}
				rich.configureFontFamilies(nil, nil)
				rich.configureTheme(0, nil)
				b := &meshBuilder{ctx: ctx, text: text, rich: rich, scale: source.PixelScale, options: nativeSceneOptions{links: &d2scenebuild.LinkBudget{MaxRegions: 10, MaxStringBytes: 1000}}}
				if hierarchy {
					b.hierarchyNode(source.Nodes[0], "#777777")
				} else {
					b.node(source.Nodes[0], "#777777")
				}
				if b.err != nil {
					t.Fatal(b.err)
				}
				if len(b.links) != 1 || b.links[0].region.URL != "https://example.test/guide" {
					t.Fatalf("outside Markdown link missing: %+v", b.links)
				}
				// Link corners must live on the final printed face, after both
				// clearance translation and hierarchy compression. Checking the
				// actual mesh catches either transform being omitted from metadata.
				lo, hi := nv(math.Inf(1), math.Inf(1), math.Inf(1)), nv(math.Inf(-1), math.Inf(-1), math.Inf(-1))
				for _, triangle := range b.triangles[len(b.triangles)-2:] {
					for _, vertex := range triangle.V {
						p := vertex.Position
						lo, hi = nv(min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z)), nv(max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z))
					}
				}
				for _, p := range b.links[0].points {
					if p.X < lo.X-1e-9 || p.X > hi.X+1e-9 || math.Abs(p.Y-lo.Y) > 1e-9 || p.Z < lo.Z-1e-9 || p.Z > hi.Z+1e-9 {
						t.Fatalf("link point %+v missed final label face [%+v,%+v]", p, lo, hi)
					}
				}
				native, err := finishNativeScene(b, source, 640, 400, nil)
				if err != nil {
					t.Fatal(err)
				}
				links, err := projectPageLinks(ctx, native)
				if err != nil || len(links) != 1 {
					t.Fatal("final page link projection failed", err)
				}
				// Isolate the finalized caption with the page's exact camera so
				// blue physical sidewalls cannot be mistaken for hyperlink ink.
				captionRaster, err := newRaster(ctx, native.width, native.height, b.triangles[len(b.triangles)-2:], color.NRGBA{255, 255, 255, 255}, &native.raster.camera, nil)
				if err != nil {
					t.Fatal(err)
				}
				frame, err := captionRaster.Frame(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				blue, linked := 0, 0
				box := links[0].Box
				for y := 0; y < frame.Bounds().Dy(); y++ {
					for x := 0; x < frame.Bounds().Dx(); x++ {
						c := color.NRGBAModel.Convert(frame.At(x, y)).(color.NRGBA)
						if int(c.B) > int(c.R)*14/10 && int(c.B) > int(c.G)*11/10 && c.B > 80 {
							blue++
							if float64(x) >= box.X-1 && float64(x) <= box.X+box.Width+1 && float64(y) >= box.Y-1 && float64(y) <= box.Y+box.Height+1 {
								linked++
							}
						}
					}
				}
				if blue < 10 || linked*100 < blue*98 {
					t.Fatalf("projected hit area misses visible link ink: %d/%d pixels inside %+v", linked, blue, box)
				}
			})
		}
	}
}
