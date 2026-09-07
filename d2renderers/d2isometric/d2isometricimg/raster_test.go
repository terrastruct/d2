package d2isometricimg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"
)

func rasterTestQuad(depth float64, m *Material) []Triangle {
	view := rasterUnit(nativeViewDirection())
	right := rasterUnit(rasterCross(Vec{Y: 1}, view))
	up := rasterUnit(rasterCross(view, right))
	v := func(x, y, u, w float64) Vertex {
		return Vertex{Position: rasterAdd(rasterAdd(rasterMul(right, x), rasterMul(up, y)), rasterMul(view, depth)), Normal: view, U: u, V: w}
	}
	a, b, c, d := v(-1, 1, 0, 0), v(1, 1, 1, 0), v(1, -1, 1, 1), v(-1, -1, 0, 1)
	return []Triangle{{V: [3]Vertex{a, b, c}, Material: m}, {V: [3]Vertex{a, c, d}, Material: m}}
}

func TestNativeRasterDepthAndStaticOwnership(t *testing.T) {
	red := &Material{Color: color.NRGBA{R: 255, A: 255}, Unlit: true}
	green := &Material{Color: color.NRGBA{G: 255, A: 255}, Unlit: true}
	ts := append(rasterTestQuad(-1, red), rasterTestQuad(1, green)...)
	r, err := NewRaster(context.Background(), 96, 64, ts, color.NRGBA{A: 255})
	if err != nil {
		t.Fatal(err)
	}
	a, err := r.Frame(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.RGBAAt(48, 32) != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("front face occlusion %v", a.RGBAAt(48, 32))
	}
	for i, j := 0, len(ts)-1; i < j; i, j = i+1, j-1 {
		ts[i], ts[j] = ts[j], ts[i]
	}
	r2, err := NewRaster(context.Background(), 96, 64, ts, color.NRGBA{A: 255})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := r2.Frame(context.Background(), nil)
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("opaque output depends on triangle order")
	}
	a.Pix[0] = 255
	c, _ := r.Frame(context.Background(), nil)
	if c.Pix[0] == 255 {
		t.Fatal("caller mutated static frame cache")
	}
	dynamic := rasterTestQuad(-2, red)
	hidden, _ := r.Frame(context.Background(), dynamic)
	if !bytes.Equal(hidden.Pix, c.Pix) {
		t.Fatal("dynamic geometry bypasses static depth")
	}
	dynamic = rasterTestQuad(2, red)
	visible, _ := r.Frame(context.Background(), dynamic)
	if visible.RGBAAt(48, 32).R != 255 {
		t.Fatal("front dynamic geometry invisible")
	}
	unchanged, _ := r.Frame(context.Background(), nil)
	if !bytes.Equal(c.Pix, unchanged.Pix) {
		t.Fatal("dynamic frame mutated static cache")
	}
}

func TestNativeRasterSharedEdgeHasSingleSampleOwner(t *testing.T) {
	// Transparent faces reveal both missing samples and double coverage at
	// the triangulation seam. Sample only the strict interior, keeping the
	// ordinary antialiasing of the outer contour out of this check.
	paint := &Material{Color: color.NRGBA{R: 255, A: 128}, Unlit: true}
	background := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for _, reversed := range []bool{false, true} {
		mesh := rasterTestQuad(1, paint)
		if reversed {
			mesh[0], mesh[1] = mesh[1], mesh[0]
		}
		r, err := NewRaster(context.Background(), 96, 64, mesh, background)
		if err != nil {
			t.Fatal(err)
		}
		checked := 0
		for y := 0; y < r.height; y++ {
			for x := 0; x < r.width; x++ {
				p := r.camera.world(float64(x*r.aa)+1, float64(y*r.aa)+1, 0)
				if math.Abs(rasterDot(p, r.camera.right)) > .9 || math.Abs(rasterDot(p, r.camera.up)) > .9 {
					continue
				}
				checked++
				if got := r.output.RGBAAt(x, y); got != (color.RGBA{R: 255, G: 127, B: 127, A: 255}) {
					t.Fatalf("shared edge leaves a gap or blends twice at %d,%d, reversed=%v: %+v", x, y, reversed, got)
				}
			}
		}
		if checked < 1000 {
			t.Fatalf("shared-edge fixture covers only %d interior pixels", checked)
		}
	}
}

func TestNativeRasterPremultipliedLabelEdges(t *testing.T) {
	texture := image.NewRGBA(image.Rect(0, 0, 2, 1))
	texture.SetRGBA(0, 0, color.RGBA{R: 128, A: 128})
	texture.SetRGBA(1, 0, color.RGBA{})
	r, g, b, a := rasterTexture(texture, .5, .5)
	if math.Abs(r-1) > 1e-6 || g != 0 || b != 0 || math.Abs(a-.250980392) > 1e-6 {
		t.Fatalf("premultiplied interpolation %g,%g,%g,%g", r, g, b, a)
	}
	white := &Material{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, Unlit: true}
	base := rasterTestQuad(0, white)
	label := rasterTestQuad(.01, &Material{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, Texture: texture, Unlit: true})
	for i := range label {
		label[i].NoDepthWrite = true
	}
	raster, err := NewRaster(context.Background(), 64, 64, append(base, label...), color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := raster.Frame(context.Background(), nil)
	p := frame.RGBAAt(32, 32)
	if p.R != 255 || p.G >= 255 || p.G < 180 || p.G != p.B {
		t.Fatalf("label edge dark fringe or alpha error: %v", p)
	}
}

func TestNativeRasterAdmissionAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewRaster(ctx, 64, 64, nil, color.NRGBA{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, texture := range []image.Image{(*image.RGBA)(nil), (*image.NRGBA)(nil), (*image.Alpha)(nil), &image.RGBA{Rect: image.Rect(0, 0, 2, 2), Stride: 8, Pix: make([]byte, 4)}, &image.NRGBA{Rect: image.Rect(0, 0, 2, 2), Stride: -1}} {
		if _, err := NewRaster(context.Background(), 64, 64, rasterTestQuad(0, &Material{Color: color.NRGBA{A: 255}, Texture: texture}), color.NRGBA{}); err == nil {
			t.Fatalf("invalid texture %T accepted", texture)
		}
	}
	tests := rasterTestQuad(0, nil)
	tests[0].V[0].Position.X = math.NaN()
	if _, err := NewRaster(context.Background(), 64, 64, tests, color.NRGBA{}); err == nil {
		t.Fatal("NaN accepted")
	}
	if _, err := NewRaster(context.Background(), 4096, 4096, nil, color.NRGBA{}); err == nil {
		t.Fatal("oversized surface accepted")
	}
	w := rasterWork{ctx: context.Background(), remaining: 1}
	if err := w.charge(2); err == nil {
		t.Fatal("work limit ignored")
	}
	r, err := NewRaster(context.Background(), 64, 64, nil, color.NRGBA{R: 12, G: 34, B: 56, A: 1})
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := r.Frame(context.Background(), nil)
	if frame.Bounds() != image.Rect(0, 0, 64, 64) || frame.RGBAAt(10, 10) != (color.RGBA{12, 34, 56, 255}) {
		t.Fatal("opaque background/dimensions")
	}
	if _, err := r.Frame(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestNativeRasterTranslucentBackfaces(t *testing.T) {
	paint := &Material{Color: color.NRGBA{R: 255, A: 128}, Unlit: true}
	back := rasterTestQuad(-1, paint)
	for i := range back {
		for j := range back[i].V {
			back[i].V[j].Normal = rasterMul(back[i].V[j].Normal, -1)
		}
	}
	front := rasterTestQuad(1, paint)
	r, err := NewRaster(context.Background(), 64, 64, append(back, front...), color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := r.Frame(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if p := frame.RGBAAt(32, 32); p != (color.RGBA{255, 127, 127, 255}) {
		t.Fatalf("opacity compounds through a rear face: %v", p)
	}
}

func TestNativeRasterScanlineCoverage(t *testing.T) {
	rng := rand.New(rand.NewSource(47))
	for trial := 0; trial < 300; trial++ {
		var p [3]rasterPoint
		for i := range p {
			p[i] = rasterPoint{x: rng.Float64()*160 - 32, y: rng.Float64()*110 - 24}
		}
		if rasterOrient(p[0], p[1], p[2]) < 0 {
			p[1], p[2] = p[2], p[1]
		}
		x0, y0, x1, y1 := rasterBounds(p, 96, 64)
		for y := y0; y < y1; y++ {
			start, end := rasterRowSpan(p, y, x0, x1)
			if start < x0 || end > x1 || start > end {
				t.Fatalf("unbounded span %d..%d", start, end)
			}
			for x := x0; x < x1; x++ {
				q := rasterPoint{x: float64(x) + .5, y: float64(y) + .5}
				inside := rasterOrient(p[0], p[1], q) >= 0 && rasterOrient(p[1], p[2], q) >= 0 && rasterOrient(p[2], p[0], q) >= 0
				if inside && (x < start || x >= end) {
					t.Fatalf("scanline omitted covered pixel %d,%d in triangle %v", x, y, p)
				}
			}
		}
	}
	// A subpixel-wide cable crossing the entire image must scale with its
	// length, not the area of its otherwise empty 2048-by-2048 rectangle.
	p := [3]rasterPoint{{x: 0, y: 0}, {x: 2048, y: 2048}, {x: 2048, y: 2048.2}}
	work := 0
	for y := 0; y < 2048; y++ {
		a, b := rasterRowSpan(p, y, 0, 2048)
		work += b - a + 3
	}
	if work > 20_000 {
		t.Fatalf("skinny triangle still scans its bounding rectangle: %d", work)
	}
}
