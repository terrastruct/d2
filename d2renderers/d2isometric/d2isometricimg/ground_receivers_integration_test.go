package d2isometricimg

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestHierarchyFixtureGroundShadowIgnoresUnrelatedDepth(t *testing.T) {
	ctx := context.Background()
	d := sourcePanelFixture(t, "regression/dagre_child_id_id/elk/board.exp.json")
	s, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	scene, err := newNativeSceneWithOptions(ctx, s, 480, 320, nil, nil, nativeSceneOptions{deferRaster: true, fitContent: true})
	if err != nil {
		t.Fatal(err)
	}
	global := rasterShadowGround(scene.triangles)
	if math.Abs(global-(-.222)) > 1e-8 {
		t.Fatalf("fixture no longer reproduces the lowered unrelated receiver: %g", global)
	}
	// The standalone id occupies the far-left original 59px source shape;
	// every one of its paint/shadow triangles must use its own ground plane.
	count := 0
	for _, triangle := range scene.triangles {
		id := true
		for _, v := range triangle.V {
			id = id && v.Position.X < 1
		}
		if !id || triangle.ShadowGround == nil {
			continue
		}
		count++
		if *triangle.ShadowGround != .08 {
			t.Fatal("lowered y container displaced the standalone id receiver")
		}
	}
	if count == 0 {
		t.Fatal("fixture contained no standalone id shadow geometry")
	}
	// Change only the deeper domain. The implicit common receiver moves, but
	// the same original id vertex must cast to the same final screen point.
	lower := append([]Triangle(nil), scene.triangles...)
	for i := range lower {
		if g := lower[i].ShadowGround; g != nil && *g < -.1 {
			receiver := *g - .4
			lower[i].ShadowGround = &receiver
			for j := range lower[i].V {
				lower[i].V[j].Position.Y -= .4
			}
		}
	}
	deeper := rasterShadowGround(lower)
	a := rasterGroundTriangles(scene.triangles, global, nativeViewDirection())
	b := rasterGroundTriangles(lower, deeper, nativeViewDirection())
	project := func(p Vec, ground float64) routeCaptionPoint {
		light := rasterShadowDirection()
		p = nsub(p, nmul(light, (p.Y-ground-rasterShadowNormalOffset)/light.Y))
		p.Y = ground
		return captionProjection(p)
	}
	for i, triangle := range scene.triangles {
		if triangle.ShadowGround == nil || *triangle.ShadowGround != .08 {
			continue
		}
		for j := range triangle.V {
			p, q := project(a[i].V[j].Position, global), project(b[i].V[j].Position, deeper)
			if math.Hypot(p.x-q.x, p.z-q.z) > 1e-10 {
				t.Fatal("unrelated nested descent shifted the standalone projected shadow")
			}
		}
	}
	// Root receiver metadata must affect only the implicit background atlas;
	// the physical light camera, depth and source alpha stay byte-identical.
	legacy := append([]Triangle(nil), scene.triangles...)
	for i := range legacy {
		legacy[i].ShadowGround = nil
	}
	raster, err := NewRaster(ctx, 240, 180, scene.triangles, scene.background)
	if err != nil {
		t.Fatal(err)
	}
	control, err := NewRaster(ctx, 240, 180, legacy, scene.background)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(raster.shadow, control.shadow) {
		t.Fatal("root ground receivers changed physical surface shadows")
	}
	if reflect.DeepEqual(raster.groundShadow, control.groundShadow) || !reflect.DeepEqual(control.groundShadow, control.shadow) {
		t.Fatal("local receivers did not use an independent ground-only atlas")
	}
}

func TestHierarchyAnimationKeepsBothShadowCameras(t *testing.T) {
	d := sourcePanelFixture(t, "regression/dagre_child_id_id/elk/board.exp.json")
	for i := range d.Shapes {
		if d.Shapes[i].ID == "id" {
			d.Shapes[i].Animated = true
		}
	}
	ctx := context.Background()
	s, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	scene, err := newNativeSceneWithOptions(ctx, s, 384, 256, nil, nil, nativeSceneOptions{fitContent: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(scene.animatedNodes) == 0 || reflect.DeepEqual(scene.raster.shadow.camera, scene.raster.groundShadow.camera) {
		t.Fatal("fixture does not exercise animated independent ground shadows")
	}
	for _, phase := range []float64{.25, .5, .75} {
		frame, err := scene.frameRaster(ctx, phase, true)
		if err != nil {
			t.Fatal(err)
		}
		if frame.camera != scene.raster.camera || frame.shadow.camera != scene.raster.shadow.camera || frame.groundShadow.camera != scene.raster.groundShadow.camera {
			t.Fatal("animation changed its view, physical or ground shadow camera")
		}
		if shadowFramingEdgeInk(frame.output, scene.background) != 0 {
			t.Fatal("local animated shadow reaches the image boundary")
		}
	}
}
