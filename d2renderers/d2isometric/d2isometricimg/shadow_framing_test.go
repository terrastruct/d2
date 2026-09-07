package d2isometricimg

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func shadowFramingEdgeInk(img *image.RGBA, bg color.NRGBA) int {
	ink := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if x != img.Bounds().Min.X && x != img.Bounds().Max.X-1 && y != img.Bounds().Min.Y && y != img.Bounds().Max.Y-1 {
				continue
			}
			p := img.RGBAAt(x, y)
			if p.R != bg.R || p.G != bg.G || p.B != bg.B {
				ink++
			}
		}
	}
	return ink
}

func TestFramingContainsRenderedSolidGroundShadow(t *testing.T) {
	for _, hierarchy := range []bool{false, true} {
		name := "full-height"
		if hierarchy {
			name = "hierarchy"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			n := contactTestQueue()
			n.Type, n.Metadata.Original.Type = d2target.ShapeCylinder, d2target.ShapeCylinder
			n.Size.Y, n.Position.Y = 1.2, .07+1.2/2
			b := &meshBuilder{ctx: ctx, scale: .01}
			if hierarchy {
				b.hierarchyNode(n, "#777777")
			} else {
				b.node(n, "#777777")
			}
			if b.err != nil {
				t.Fatal(b.err)
			}
			var before, after projectedExtent
			before.mesh(b.triangles)
			after.animatedMesh(b.triangles, nil, .01)
			bg := color.NRGBA{R: 245, G: 247, B: 250, A: 255}
			for _, corrected := range []bool{false, true} {
				bounds := before
				if corrected {
					bounds = after
				}
				camera := bounds.camera(384, 256, true)
				rc := cameraAtResolution(camera, camera.width*2, camera.height*2)
				raster, err := newRaster(ctx, camera.width, camera.height, b.triangles, bg, &rc, nil)
				if err != nil {
					t.Fatal(err)
				}
				edgeInk := shadowFramingEdgeInk(raster.output, bg)
				if !corrected && edgeInk == 0 {
					t.Fatal("fixture must demonstrate the prior clipped contact shadow")
				}
				if corrected && edgeInk != 0 {
					t.Fatalf("shadow still reaches %d canvas-edge pixels", edgeInk)
				}
			}
		})
	}
}

func TestCaptureAndTimelineContainAnimatedGroundShadows(t *testing.T) {
	d := d2target.NewDiagram()
	s := d2target.BaseShape()
	s.ID, s.Type, s.Width, s.Height = "store", d2target.ShapeCylinder, 120, 80
	s.Fill, s.Stroke, s.Animated, s.ThreeDee = "#315872", "#182f43", true, true
	d.Shapes = []d2target.Shape{*s}
	ctx := context.Background()
	o := mustNormalize(t, Options{Format: GIF, Width: 384, Height: 256, FitContent: true, Render: d2isometric.RenderOpts{}})
	for _, timeline := range []bool{false, true} {
		if timeline {
			camera, err := timelineCamera(ctx, []*d2target.Diagram{d}, o)
			if err != nil {
				t.Fatal(err)
			}
			o.Width, o.Height, o.camera = camera.width, camera.height, &camera
		}
		capture, err := openCapture(ctx, d, o)
		if err != nil {
			t.Fatal(err)
		}
		for _, phase := range []float64{0, .25, .5, .75} {
			raster, err := capture.scene.frameRaster(ctx, phase, true)
			if err != nil {
				capture.close()
				t.Fatal(err)
			}
			if ink := shadowFramingEdgeInk(raster.output, capture.scene.background); ink != 0 {
				capture.close()
				t.Fatalf("animated shadow clipped %d pixels, timeline=%v phase=%g", ink, timeline, phase)
			}
		}
		capture.close()
	}
}

func TestShadowFramingIgnoresNonCastingGeometry(t *testing.T) {
	material := nativeMaterial("#222222", .5, 0, 1)
	triangle := Triangle{Material: material, CastShadow: true, V: [3]Vertex{{Position: nv(0, 1, 0)}, {Position: nv(1, 1, 0)}, {Position: nv(1, 1, 1)}}}
	for _, kind := range []string{"non-caster", "zero-shadow-opacity", "transparent", "below-receiver"} {
		tri := triangle
		switch kind {
		case "non-caster":
			tri.CastShadow = false
		case "zero-shadow-opacity":
			zero := 0.
			tri.ShadowOpacity = &zero
		case "transparent":
			tri.Material = nativeMaterial("#222222", .5, 0, 0)
		case "below-receiver":
			for i := range tri.V {
				tri.V[i].Position.Y = 0
			}
		}
		var extent projectedExtent
		extent.groundShadows([]Triangle{tri}, nil, .01)
		if extent.valid {
			t.Fatalf("%s invented shadow framing space", kind)
		}
	}
}
