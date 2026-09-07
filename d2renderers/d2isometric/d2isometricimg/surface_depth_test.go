package d2isometricimg

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/d2lang/d2/d2target"
)

func TestSurfaceDecalSurvivesLargeTexturedSubstrate(t *testing.T) {
	for _, position := range []Vec{nv(-1.5, .101, -1.5), nv(1.5, .101, 1.5)} {
		b := &meshBuilder{ctx: context.Background()}
		face, icon := image.NewRGBA(image.Rect(0, 0, 2, 2)), image.NewRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				face.SetRGBA(x, y, color.RGBA{R: 40, G: 70, B: 180, A: 255})
				icon.SetRGBA(x, y, color.RGBA{R: 250, G: 10, B: 100, A: 255})
			}
		}
		b.surfaceTexture(face, labelSurface{center: nv(0, .1, 0), width: 4, depth: 4}, 1)
		for i := range b.triangles {
			b.triangles[i].NoDepthWrite = false
		}
		b.surfaceTexture(icon, labelSurface{center: position, width: .6, depth: .6}, 1)
		r, err := NewRaster(context.Background(), 320, 240, b.triangles, color.NRGBA{255, 255, 255, 255})
		if err != nil {
			t.Fatal(err)
		}
		pink := 0
		for i := 0; i < len(r.output.Pix); i += 4 {
			if r.output.Pix[i] > 200 && r.output.Pix[i+1] < 60 && r.output.Pix[i+2] > 60 {
				pink++
			}
		}
		if pink < 40 {
			t.Fatalf("face painted over its surface icon at %+v: %d pink pixels", position, pink)
		}
	}
}

func TestShadowRespectsTextureAlphaAndAuthoredOpacity(t *testing.T) {
	b := &meshBuilder{ctx: context.Background()}
	transparent := image.NewRGBA(image.Rect(0, 0, 2, 2))
	b.surfaceTexture(transparent, labelSurface{center: nv(0, .5, 0), width: 1, depth: 1}, 1)
	for i := range b.triangles {
		b.triangles[i].CastShadow = true
	}
	work := rasterWork{ctx: context.Background(), remaining: rasterMaxWork}
	shadow, err := rasterBuildShadow(&work, b.triangles)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range shadow.opacity {
		if a != 0 {
			t.Fatal("transparent face cast a rectangular shadow")
		}
	}
	for _, bad := range []float64{-1, 1.1, math.NaN()} {
		b.triangles[0].ShadowOpacity = &bad
		if err := rasterValidate(context.Background(), b.triangles); err == nil {
			t.Fatalf("accepted invalid shadow opacity %v", bad)
		}
	}
}

func TestAuthoredShapeMotionUsesFixedCameraAndExactOneSecondCycle(t *testing.T) {
	d := d2target.NewDiagram()
	a, b := d2target.BaseShape(), d2target.BaseShape()
	a.ID, a.Type, a.Width, a.Height, a.Animated = "animated", d2target.ShapeRectangle, 120, 80, true
	b.ID, b.Type, b.Width, b.Height = "stationary", d2target.ShapeRectangle, 120, 80
	b.Pos.X, b.Shadow = 440, true
	d.Shapes = []d2target.Shape{*a, *b}
	n := nativeFixtureScene(t, d)
	if len(n.animatedNodes) != 1 {
		t.Fatal("authored animated shape lost its animation")
	}
	start, err := n.Frame(context.Background(), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	mid, err := n.Frame(context.Background(), .5, true)
	if err != nil {
		t.Fatal(err)
	}
	end, err := n.Frame(context.Background(), 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(start.Pix, mid.Pix) {
		t.Fatal("authored motion did not render")
	}
	if !bytes.Equal(start.Pix, end.Pix) {
		t.Fatal("one-second authored animation does not return to its exact initial frame")
	}
	r, err := n.frameRaster(context.Background(), .5, true)
	if err != nil {
		t.Fatal(err)
	}
	if r.camera != n.raster.camera {
		t.Fatal("animated geometry refit the camera")
	}
	if r.shadow.camera != n.raster.shadow.camera {
		t.Fatal("animated geometry refit the shadow camera")
	}
	// Verify actual stationary pixels, not just the view-camera state. A
	// changing shadow-map fit used to make unrelated shadows shimmer.
	x0, y0, x1, y1 := 640, 480, 0, 0
	for _, x := range []float64{4.4, 5.6} {
		for _, z := range []float64{0, .8} {
			for _, y := range []float64{0, .3} {
				p := n.raster.camera.project(nv(x, y, z))
				px, py := int(p.x)/n.raster.aa, int(p.y)/n.raster.aa
				x0, y0, x1, y1 = min(x0, px), min(y0, py), max(x1, px), max(y1, py)
			}
		}
	}
	for y := max(0, y0-10); y < min(480, y1+10); y++ {
		for x := max(0, x0-10); x < min(640, x1+10); x++ {
			if start.RGBAAt(x, y) != mid.RGBAAt(x, y) {
				t.Fatalf("stationary region shimmered at %d,%d", x, y)
			}
		}
	}
}
