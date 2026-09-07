package d2isometricimg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func aaTestCamera(width, height int) rasterCamera {
	c := nativeCameraAxes()
	c.width, c.height, c.scale = width*rasterAA, height*rasterAA, rasterAA
	c.centerX, c.centerY = float64(width)/2, -float64(height)/2
	return c
}

func aaTestTriangle(camera rasterCamera, depth float64, material *Material) Triangle {
	vertex := func(x, y float64) Vertex {
		return Vertex{Position: nadd(nadd(nmul(camera.right, x), nmul(camera.up, -y)), nmul(camera.direction, depth)), Normal: camera.direction}
	}
	return Triangle{Material: material, V: [3]Vertex{vertex(30.125, 20.25), vertex(240.625, 100.125), vertex(65, 260.5)}}
}

func TestNativeAACoverageAcrossFormerSizeCutoff(t *testing.T) {
	black := &Material{Color: color.NRGBA{A: 255}, Unlit: true}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	var reference []byte
	for _, height := range []int{2000, 2001} {
		camera := aaTestCamera(2000, height)
		tri := aaTestTriangle(camera, 0, black)
		r, err := newRaster(context.Background(), 2000, height, []Triangle{tri}, white, &camera, nil)
		if err != nil {
			t.Fatal(err)
		}
		if r.aa != 2 {
			t.Fatal("large export lost subpixel geometry coverage")
		}
		if height == 2001 && (r.static == nil || r.pixels != nil || len(r.depth) != 0) {
			t.Fatal("large export retained a full supersampled color/depth surface")
		}
		var crop []byte
		partial := 0
		for y := 0; y < 300; y++ {
			for x := 0; x < 300; x++ {
				p := r.output.RGBAAt(x, y)
				crop = append(crop, p.R, p.G, p.B, p.A)
				if p.R > 0 && p.R < 255 {
					partial++
				}
			}
		}
		if partial < 200 {
			t.Fatalf("diagonal has only %d partially covered pixels", partial)
		}
		if reference == nil {
			reference = crop
		} else if !bytes.Equal(reference, crop) {
			t.Fatal("crossing the previous 4M-pixel cutoff changed diagonal coverage")
		}
	}
}

func TestNativeStripReplayMatchesCachedDepthAndAlpha(t *testing.T) {
	ctx := context.Background()
	camera := aaTestCamera(320, 300)
	base := aaTestTriangle(camera, 0, &Material{Color: color.NRGBA{G: 190, A: 255}, Unlit: true})
	decal := aaTestTriangle(camera, .01, &Material{Color: color.NRGBA{B: 255, A: 90}, Unlit: true})
	decal.NoDepthWrite = true
	for i := range decal.V {
		decal.V[i].Position = nadd(decal.V[i].Position, nmul(camera.right, 8))
	}
	ts := []Triangle{base, decal}
	r, err := newRaster(ctx, 320, 300, ts, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	dynamic := []Triangle{aaTestTriangle(camera, -1, &Material{Color: color.NRGBA{R: 255, A: 255}, Unlit: true}), aaTestTriangle(camera, 1, &Material{Color: color.NRGBA{R: 255, A: 110}, Unlit: true})}
	for i := range dynamic[1].V {
		dynamic[1].V[i].Position = nadd(dynamic[1].V[i].Position, nmul(camera.up, -15))
	}
	expected, err := r.Frame(ctx, dynamic)
	if err != nil {
		t.Fatal(err)
	}
	plan := r.rasterPrepare(ts)
	r.static = &plan
	r.pixels, r.depth = nil, nil
	static := image.NewRGBA(r.output.Rect)
	work := rasterWork{ctx: ctx, remaining: rasterMaxWork}
	if err := r.renderStrips(&work, static, nil); err != nil || !bytes.Equal(static.Pix, r.output.Pix) {
		t.Fatal("strip boundaries changed static coverage, depth or alpha", err)
	}
	for phase := 0; phase < 2; phase++ {
		frame, err := r.Frame(ctx, dynamic)
		if err != nil || !bytes.Equal(frame.Pix, expected.Pix) {
			t.Fatal("strip replay changed dynamic occlusion or blended pixels", err)
		}
	}
	unchanged, err := r.Frame(ctx, nil)
	if err != nil || !bytes.Equal(unchanged.Pix, static.Pix) {
		t.Fatal("dynamic strip replay mutated cached static pixels", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.Frame(cancelled, dynamic); !errors.Is(err, context.Canceled) {
		t.Fatal("strip replay ignored cancellation")
	}
	work = rasterWork{ctx: ctx, remaining: 1}
	if err := r.renderStrips(&work, static, nil); err == nil {
		t.Fatal("strip rendering bypassed the work limit")
	}
}

func TestNativeAAMaximumCanvas(t *testing.T) {
	material := &Material{Color: color.NRGBA{R: 100, G: 130, B: 170, A: 255}, Unlit: true}
	ts := rasterTestQuad(0, material)
	for i := range ts {
		ts[i].CastShadow = true
	}
	r, err := NewRaster(context.Background(), 4000, 3000, ts, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if err != nil {
		t.Fatal(err)
	}
	if r.aa != 2 || r.static == nil || r.pixels != nil || len(r.depth) != 0 || r.output.Bounds() != image.Rect(0, 0, 4000, 3000) {
		t.Fatal("maximum supported output lost AA or retained all working samples")
	}
	// A caller may reuse its input after construction. A small occluded object
	// forces deferred replay, which must still use the original static scene.
	material.Color.R, ts[0].V[0].Position.X = 255, 1000
	dynamic := rasterTestQuad(0, &Material{Color: color.NRGBA{R: 255, A: 255}, Unlit: true})
	for i := range dynamic {
		for j := range dynamic[i].V {
			dynamic[i].V[j].Position = nadd(nmul(dynamic[i].V[j].Position, .02), nmul(r.camera.direction, -10))
		}
	}
	frame, err := r.Frame(context.Background(), dynamic)
	if err != nil || !bytes.Equal(frame.Pix, r.output.Pix) {
		t.Fatal("large replay lost input ownership or static depth", err)
	}
}

func TestNativeAACaptureCameraAboveCutoff(t *testing.T) {
	d := d2target.NewDiagram()
	s := d2target.BaseShape()
	s.ID, s.Type, s.Width, s.Height = "service", d2target.ShapeRectangle, 120, 90
	s.Fill, s.Stroke = "#346381", "#152b3a"
	d.Shapes = []d2target.Shape{*s}
	o := mustNormalize(t, Options{Width: 2048, Height: 2048, Render: d2isometric.RenderOpts{}})
	capture, err := openCapture(context.Background(), d, o)
	if err != nil {
		t.Fatal(err)
	}
	defer capture.close()
	r := capture.scene.raster
	if r.aa != 2 || r.camera.width != 4096 || r.camera.height != 4096 || r.static == nil {
		t.Fatal("native scene supplied a 1x camera to the supersampled export")
	}
	frame, err := capture.frameImage(0, false)
	if err != nil || frame.Bounds() != image.Rect(0, 0, 2048, 2048) {
		t.Fatal("large capture did not produce the requested image", err)
	}
	// Fixed cameras carry framing, not an alternate coverage policy. A legacy
	// caller supplying logical pixels is normalized without changing its view.
	camera := cameraAtResolution(r.camera, 2048, 2048)
	legacy, err := newRaster(context.Background(), 2048, 2048, capture.scene.triangles, capture.scene.background, &camera, &r.shadow.camera)
	if err != nil || !bytes.Equal(legacy.output.Pix, frame.Pix) {
		t.Fatal("logical camera override changed AA framing or sample dimensions", err)
	}
}

func TestNativeDeferredSnapshotRetainsTexturePrecision(t *testing.T) {
	texture := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	texture.SetNRGBA(0, 0, color.NRGBA{R: 101, G: 73, B: 202, A: 127})
	material := &Material{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, Texture: texture, Unlit: true}
	ts := rasterTestQuad(0, material)
	snapshot, err := rasterSnapshot(context.Background(), ts)
	if err != nil {
		t.Fatal(err)
	}
	for _, uv := range [][2]float64{{.25, .25}, {.3, .4}, {.5, .5}} {
		r, g, b, a := rasterTexture(texture, uv[0], uv[1])
		x, y, z, w := rasterTexture(snapshot[0].Material.Texture, uv[0], uv[1])
		if r != x || g != y || b != z || a != w {
			t.Fatal("deferred snapshot changed translucent texture precision")
		}
	}
	texture.SetNRGBA(0, 0, color.NRGBA{})
	material.Color.R, ts[0].V[0].Position.X = 0, 100
	if snapshot[0].Material.Color.R != 255 || snapshot[0].V[0].Position.X == 100 {
		t.Fatal("deferred snapshot retained mutable geometry or material")
	}
	_, _, _, alpha := rasterTexture(snapshot[0].Material.Texture, .25, .25)
	if alpha == 0 {
		t.Fatal("deferred snapshot retained a mutable caller texture")
	}
}

func TestNativeGroundFootprintPreservesEveryShadowSample(t *testing.T) {
	ctx := context.Background()
	n := contactTestQueue()
	n.Type = d2target.ShapeCylinder
	b := &meshBuilder{ctx: ctx, scale: .01}
	b.solidNode(n)
	if b.err != nil {
		t.Fatal(b.err)
	}
	r, err := NewRaster(ctx, 320, 240, b.triangles, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if err != nil {
		t.Fatal(err)
	}
	limited := image.NewRGBA(image.Rect(0, 0, r.camera.width, r.camera.height))
	work := rasterWork{ctx: ctx, remaining: rasterMaxWork}
	if err := r.paintGround(&work, limited, r.background); err != nil {
		t.Fatal(err)
	}
	full := image.NewRGBA(limited.Rect)
	r.groundBounds = full.Rect
	if err := r.paintGround(&work, full, r.background); err != nil || !bytes.Equal(limited.Pix, full.Pix) {
		t.Fatal("finite ground footprint changed a filtered shadow sample", err)
	}
}
