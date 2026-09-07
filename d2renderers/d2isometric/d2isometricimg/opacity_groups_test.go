package d2isometricimg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
)

func opacityTestQuad(camera rasterCamera, box image.Rectangle, depth float64, material *Material, decal bool) []Triangle {
	vertex := func(x, y int) Vertex {
		return Vertex{Position: camera.world(float64(x*rasterAA), float64(y*rasterAA), depth), Normal: camera.direction}
	}
	a, b, c, d := vertex(box.Min.X, box.Min.Y), vertex(box.Max.X, box.Min.Y), vertex(box.Max.X, box.Max.Y), vertex(box.Min.X, box.Max.Y)
	return []Triangle{{V: [3]Vertex{a, b, c}, Material: material, NoDepthWrite: decal}, {V: [3]Vertex{a, c, d}, Material: material, NoDepthWrite: decal}}
}

func opacityTestObject(camera rasterCamera) []Triangle {
	paint := func(c color.NRGBA) *Material { return &Material{Color: c, Unlit: true} }
	mesh := opacityTestQuad(camera, image.Rect(24, 20, 275, 267), -.5, paint(color.NRGBA{R: 200, G: 75, B: 45, A: 255}), false)
	mesh = append(mesh, opacityTestQuad(camera, image.Rect(40, 35, 260, 255), 0, paint(color.NRGBA{R: 70, G: 150, B: 210, A: 255}), false)...)
	// A printed cell covers another physical face, while the lower plinth and
	// rear red face must be occluded inside the object before its single fade.
	mesh = append(mesh, opacityTestQuad(camera, image.Rect(65, 60, 160, 92), .003, paint(color.NRGBA{R: 10, G: 30, B: 40, A: 200}), true)...)
	return mesh
}

func TestOpacityGroupComposesFacesAndInkBeforeFade(t *testing.T) {
	ctx := context.Background()
	camera := aaTestCamera(320, 300)
	background := color.NRGBA{R: 243, G: 247, B: 249, A: 255}
	mesh := opacityTestObject(camera)
	opaque, err := newRaster(ctx, 320, 300, mesh, background, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, opacity := range []float64{.2, .5, .8} {
		group := &nativeOpacityGroup{Opacity: opacity}
		grouped := append([]Triangle(nil), mesh...)
		for i := range grouped {
			grouped[i].OpacityGroup = group
		}
		r, err := newRaster(ctx, 320, 300, grouped, background, &camera, nil)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 300; y++ {
			for x := 0; x < 320; x++ {
				i := opaque.output.PixOffset(x, y)
				for channel, bg := range []uint8{background.R, background.G, background.B, background.A} {
					want := int(math.Round(float64(opaque.output.Pix[i+channel])*opacity + float64(bg)*(1-opacity)))
					got := int(r.output.Pix[i+channel])
					if got-want > 1 || want-got > 1 {
						t.Fatalf("opacity %.1f at %d,%d channel %d: got %d, want %d after one fade", opacity, x, y, channel, got, want)
					}
				}
			}
		}
	}
}

func TestOpacityGroupOccludesInternallyAndRespectsOtherDepth(t *testing.T) {
	ctx := context.Background()
	camera := aaTestCamera(320, 300)
	background := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	mesh := opacityTestObject(camera)
	group := &nativeOpacityGroup{Opacity: .5}
	for i := range mesh {
		mesh[i].OpacityGroup = group
	}
	occluder := opacityTestQuad(camera, image.Rect(100, 100, 180, 180), 2, &Material{Color: color.NRGBA{G: 255, A: 255}, Unlit: true}, false)
	owner := &nativePaintOwner{Opaque: true}
	for i := range occluder {
		occluder[i].PaintOwner = owner
	}
	mesh = append(mesh, occluder...)
	r, err := newRaster(ctx, 320, 300, mesh, background, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p := r.output.RGBAAt(140, 140); p != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("front object was faded or overpainted: %v", p)
	}
	if p := r.output.RGBAAt(210, 140); p != (color.RGBA{R: 163, G: 203, B: 233, A: 255}) {
		t.Fatalf("hidden red plinth leaked through blue cap: %v", p)
	}
}

func TestOpacityGroupFrameReplaysDepthAndStrips(t *testing.T) {
	ctx := context.Background()
	camera := aaTestCamera(320, 300)
	background := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	mesh := opacityTestObject(camera)
	group := &nativeOpacityGroup{Opacity: .5}
	for i := range mesh {
		mesh[i].OpacityGroup = group
	}
	r, err := newRaster(ctx, 320, 300, mesh, background, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.static == nil || r.pixels != nil || len(r.depth) != 0 {
		t.Fatal("partial-opacity scene retained a full sample buffer")
	}
	for _, z := range []float64{-2, 2} {
		dynamic := opacityTestQuad(camera, image.Rect(75, 77, 215, 240), z, &Material{Color: color.NRGBA{G: 255, A: 255}, Unlit: true}, false)
		want, err := newRaster(ctx, 320, 300, append(append([]Triangle(nil), mesh...), dynamic...), background, &camera, nil)
		if err != nil {
			t.Fatal(err)
		}
		for replay := 0; replay < 2; replay++ {
			frame, err := r.Frame(ctx, dynamic)
			if err != nil || !bytes.Equal(frame.Pix, want.output.Pix) {
				t.Fatalf("dynamic depth %.1f changed when replayed through static group: %v", z, err)
			}
		}
	}
	copy, err := r.Frame(ctx, nil)
	if err != nil || !bytes.Equal(copy.Pix, r.output.Pix) {
		t.Fatal("group replay changed cached static output", err)
	}
}

func TestOpacityGroupFadesBackgroundDecalsAndKeepsForeground(t *testing.T) {
	ctx := context.Background()
	camera := aaTestCamera(320, 300)
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	group := &nativeOpacityGroup{Opacity: .5}
	for _, z := range []float64{-1, 1} {
		background := opacityTestQuad(camera, image.Rect(20, 20, 280, 280), z, &Material{Color: white, Unlit: true}, false)
		background = append(background, opacityTestQuad(camera, image.Rect(80, 80, 200, 200), z+.003, &Material{Color: color.NRGBA{A: 255}, Unlit: true}, true)...)
		object := opacityTestQuad(camera, image.Rect(40, 40, 240, 240), 0, &Material{Color: color.NRGBA{R: 200, G: 100, B: 50, A: 255}, Unlit: true}, false)
		for i := range object {
			object[i].OpacityGroup = group
		}
		r, err := newRaster(ctx, 320, 300, append(object, background...), white, &camera, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := color.RGBA{R: 100, G: 50, B: 25, A: 255}
		if z > 0 {
			want = color.RGBA{A: 255}
		}
		if got := r.output.RGBAAt(140, 140); got != want {
			t.Fatalf("external decal depth %.1f: got %v, want %v", z, got, want)
		}
	}
}

func TestOpacityGroupInterleavesExternalTransparentPaint(t *testing.T) {
	ctx := context.Background()
	camera := aaTestCamera(320, 300)
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	greenTexture := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	greenTexture.SetNRGBA(0, 0, color.NRGBA{G: 255, A: 128})
	for _, tc := range []struct {
		name  string
		paint *Material
		decal bool
		owned bool
	}{
		{"material", &Material{Color: color.NRGBA{G: 255, A: 128}, Unlit: true}, false, false},
		{"texture", &Material{Color: white, Texture: greenTexture, Unlit: true}, false, false},
		{"text decal", &Material{Color: white, Texture: greenTexture, Unlit: true}, true, false},
		{"owned material", &Material{Color: color.NRGBA{G: 255, A: 128}, Unlit: true}, false, true},
		{"owned texture", &Material{Color: white, Texture: greenTexture, Unlit: true}, false, true},
		{"owned text decal", &Material{Color: white, Texture: greenTexture, Unlit: true}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, z := range []float64{-1, 1} {
				object := opacityTestQuad(camera, image.Rect(40, 40, 240, 240), 0, &Material{Color: color.NRGBA{R: 255, A: 255}, Unlit: true}, false)
				group := &nativeOpacityGroup{Opacity: .5}
				for i := range object {
					object[i].OpacityGroup = group
				}
				external := opacityTestQuad(camera, image.Rect(80, 80, 200, 200), z, tc.paint, tc.decal)
				if tc.owned {
					owner := &nativePaintOwner{}
					for i := range external {
						external[i].PaintOwner = owner
					}
				}
				r, err := newRaster(ctx, 320, 300, append(object, external...), white, &camera, nil)
				if err != nil {
					t.Fatal(err)
				}
				want := color.RGBA{R: 191, G: 128, B: 64, A: 255}
				if z > 0 {
					want = color.RGBA{R: 127, G: 192, B: 64, A: 255}
				}
				if got := r.output.RGBAAt(140, 140); got != want {
					t.Fatalf("external alpha paint depth %.1f: got %v, want %v", z, got, want)
				}
			}
		})
	}
}

func TestOpacityGroupFreezeAdmissionAndWork(t *testing.T) {
	ctx := context.Background()
	camera := aaTestCamera(320, 300)
	mesh := opacityTestObject(camera)
	group := &nativeOpacityGroup{Opacity: .5}
	for i := range mesh {
		mesh[i].OpacityGroup = group
	}
	frozen, err := rasterSnapshot(ctx, mesh)
	if err != nil {
		t.Fatal(err)
	}
	group.Opacity = .9
	for _, tri := range frozen {
		if tri.OpacityGroup == group || tri.OpacityGroup != frozen[0].OpacityGroup || tri.OpacityGroup.Opacity != .5 {
			t.Fatal("freeze lost group identity or retained mutable opacity")
		}
	}
	for _, opacity := range []float64{-.1, 1.1, math.NaN(), math.Inf(1)} {
		group.Opacity = opacity
		if err := rasterValidate(ctx, mesh); err == nil {
			t.Fatal("invalid object opacity admitted")
		}
	}
	r := &Raster{width: 320, height: 300, aa: 2, camera: camera}
	plan := r.rasterPrepare(frozen)
	dst := image.NewRGBA(image.Rect(0, 0, 640, 600))
	depth := make([]float32, 640*600)
	work := rasterWork{ctx: ctx, remaining: 1}
	if err := r.drawPrepared(&work, dst, depth, plan, nil); err == nil {
		t.Fatal("opacity group bypassed work limit")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	work = rasterWork{ctx: cancelled, remaining: rasterMaxWork}
	if err := r.drawPrepared(&work, dst, depth, plan, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("opacity group bypassed cancellation")
	}
	work = rasterWork{ctx: ctx, remaining: rasterMaxWork}
	if err := r.drawPrepared(&work, dst, depth, plan, nil); err != nil {
		t.Fatal(err)
	}
	if len(work.groupDepth) > camera.width*rasterStripRows*r.aa || len(work.groupPixels) > camera.width*rasterStripRows*r.aa*4 {
		t.Fatal("object scratch exceeded one sample strip")
	}
}

func TestOpacityGroupShadowAndDeferredSnapshot(t *testing.T) {
	ctx := context.Background()
	camera := aaTestCamera(320, 300)
	mesh := opacityTestObject(camera)
	for i := range mesh {
		mesh[i].CastShadow = true
	}
	work := rasterWork{ctx: ctx, remaining: rasterMaxWork}
	opaqueShadow, err := rasterBuildShadow(&work, mesh)
	if err != nil {
		t.Fatal(err)
	}
	group := &nativeOpacityGroup{Opacity: .5}
	for i := range mesh {
		mesh[i].OpacityGroup = group
	}
	work = rasterWork{ctx: ctx, remaining: rasterMaxWork}
	fadedShadow, err := rasterBuildShadow(&work, mesh)
	if err != nil {
		t.Fatal(err)
	}
	covered := 0
	for i, opacity := range opaqueShadow.opacity {
		if opacity == 0 {
			continue
		}
		covered++
		if fadedShadow.opacity[i] != uint8(math.Round(float64(opacity)*.5)) {
			t.Fatal("group shadow did not receive exactly one opacity multiplier")
		}
	}
	if covered == 0 {
		t.Fatal("shadow fixture did not cover any pixels")
	}
	r, err := NewRaster(ctx, 320, 300, mesh, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if err != nil {
		t.Fatal(err)
	}
	dynamic := opacityTestQuad(r.camera, image.Rect(80, 80, 180, 180), -100, &Material{Color: color.NRGBA{G: 255, A: 255}, Unlit: true}, false)
	want, err := r.Frame(ctx, dynamic)
	if err != nil {
		t.Fatal(err)
	}
	group.Opacity = .9
	mesh[0].Material.Color = color.NRGBA{R: 255, A: 255}
	got, err := r.Frame(ctx, dynamic)
	if err != nil || !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("small deferred scene retained caller-owned group/material state", err)
	}
}

func TestDistantOpacityGroupKeepsOrdinaryCapAndLabel(t *testing.T) {
	ctx := context.Background()
	n := fidelityNode("rectangle")
	n.Metadata.Original.Label = "Native cap label"
	n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 130, 24
	n.Metadata.Original.FontSize = 20
	b := reliefTestBuild(t, n, true)
	for i := range b.triangles {
		b.triangles[i].CastShadow = false
	}
	background := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	camera := rasterFit(b.triangles, nativeViewDirection(), 640, 600, 1.2)
	baseline, err := newRaster(ctx, 320, 300, b.triangles, background, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.static != nil || baseline.pixels == nil {
		t.Fatal("paint-owner metadata changed the opaque-only raster path")
	}
	owner := b.triangles[0].PaintOwner
	if owner == nil || !owner.Opaque {
		t.Fatal("native opaque shape has no opaque paint owner")
	}
	for _, tri := range b.triangles {
		if tri.PaintOwner != owner {
			t.Fatal("native cap and label lost shared paint ownership")
		}
	}
	distant := opacityTestQuad(camera, image.Rect(400, 400, 410, 410), 0, &Material{Color: color.NRGBA{R: 255, A: 255}, Unlit: true}, false)
	group := &nativeOpacityGroup{Opacity: .5}
	for i := range distant {
		distant[i].OpacityGroup = group
	}
	mixed, err := newRaster(ctx, 320, 300, append(append([]Triangle(nil), b.triangles...), distant...), background, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(baseline.output.Pix, mixed.output.Pix) {
		changed := 0
		for i, value := range baseline.output.Pix {
			if value != mixed.output.Pix[i] {
				changed++
			}
		}
		t.Fatalf("distant group changed %d channels of the ordinary native cap/label", changed)
	}
	frozen, err := rasterSnapshot(ctx, b.triangles)
	if err != nil {
		t.Fatal(err)
	}
	owner.Opaque = false
	for _, tri := range frozen {
		if tri.PaintOwner == owner || tri.PaintOwner != frozen[0].PaintOwner || !tri.PaintOwner.Opaque {
			t.Fatal("snapshot lost paint-owner identity or retained its mutable flags")
		}
	}
}
