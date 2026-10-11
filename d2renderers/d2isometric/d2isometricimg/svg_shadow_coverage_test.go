package d2isometricimg

import (
	"context"
	"image"
	"image/color"
	"math"
	"regexp"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// Check silhouette alpha before the outer blur. The native SVG importer does
// not implement filters, so omit the blur and RGB-only shadow recoloring here;
// neither changes the interior alpha whose double application caused the ring.
func rasterSVGShadowCoverage(t *testing.T, scene *nativeScene) *image.NRGBA {
	t.Helper()
	w := &nativeSVGWriter{ctx: context.Background()}
	writeSVGGroundShadow(w, scene)
	if w.err != nil {
		t.Fatal(w.err)
	}
	source := regexp.MustCompile(`<filter\b[^>]*>.*?</filter>`).ReplaceAllString(w.buf.String(), "")
	source = regexp.MustCompile(` filter="[^"]*"`).ReplaceAllString(source, "")
	return rasterVectorFragment(t, `<g xmlns:xlink="http://www.w3.org/1999/xlink" transform="scale(0.0025)">`+source+`</g>`, image.Rect(0, 0, 400, 400))
}

func TestSVGShadowOpaqueCapAndWallHaveOneStrength(t *testing.T) {
	ctx := context.Background()
	texture := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			texture.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	root := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: d2scene.SolidPaint{Color: color.NRGBA{255, 255, 255, 255}}})
	surface := &nativeVectorSurface{document: &d2scene.Document{Root: root, ViewBox: d2scene.Box{Width: 1, Height: 1}, LogicalWidth: 1, LogicalHeight: 1}}
	material := &Material{Color: color.NRGBA{255, 255, 255, 255}, Texture: texture, Vector: surface}
	b := &meshBuilder{ctx: ctx}
	// The cap and right wall overlap in the light projection. The wall alone
	// and cap alone must have the same alpha as that overlap, not a dark rim.
	a := Vertex{Position: nv(-1, .8, -.5), Normal: nv(0, 1, 0)}
	c := Vertex{Position: nv(1, .8, -.5), Normal: nv(0, 1, 0), U: 1}
	d := Vertex{Position: nv(1, .8, .5), Normal: nv(0, 1, 0), U: 1, V: 1}
	e := Vertex{Position: nv(-1, .8, .5), Normal: nv(0, 1, 0), V: 1}
	b.triangle(a, d, c, material, true)
	b.triangle(a, e, d, material, true)
	cap := append([]Triangle(nil), b.triangles...)
	b.flat(c.Position, nv(1, .15, -.5), nv(1, .15, .5), nativeMaterial("white", .5, 0, 1), true)
	b.flat(c.Position, nv(1, .15, .5), d.Position, nativeMaterial("white", .5, 0, 1), true)
	nativePhysicalShadows(b.triangles, false)
	nativePhysicalShadows(cap, false)
	camera := nativeCameraAxes()
	camera.scale, camera.width, camera.height = 100, 400, 400
	scene := &nativeScene{camera: camera, width: 400, height: 400, triangles: b.triangles}
	full := rasterSVGShadowCoverage(t, scene)
	scene.triangles = cap
	onlyCap := rasterSVGShadowCoverage(t, scene)
	want := uint8(math.Round(.11 * 184))
	overlap, wallOnly := 0, 0
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			got, withoutWall := full.NRGBAAt(x, y).A, onlyCap.NRGBAAt(x, y).A
			if got > want+1 {
				t.Fatalf("shadow darkens at overlapping opaque faces: alpha %d, want %d", got, want)
			}
			if withoutWall == want {
				overlap++
				if got != want {
					t.Fatal("adding a wall changed the filled cap's shadow strength")
				}
			}
			if withoutWall == 0 && got == want {
				wallOnly++
			}
		}
	}
	if overlap < 100 || wallOnly < 100 {
		t.Fatalf("fixture missed cap or wall shadow coverage: cap=%d wall=%d", overlap, wallOnly)
	}
}

func TestSVGShadowRetainsTransparentTextureHole(t *testing.T) {
	texture := image.NewRGBA(image.Rect(0, 0, 4, 1))
	root := d2scene.NewNode(nil)
	for _, x := range []int{0, 3} {
		texture.SetRGBA(x, 0, color.RGBA{255, 255, 255, 255})
		root.Children = append(root.Children, d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: float64(x) / 4, Width: .25, Height: 1}, Fill: d2scene.SolidPaint{Color: color.NRGBA{255, 255, 255, 255}}}))
	}
	surface := &nativeVectorSurface{document: &d2scene.Document{Root: root, ViewBox: d2scene.Box{Width: 1, Height: 1}, LogicalWidth: 1, LogicalHeight: 1}}
	material := &Material{Color: color.NRGBA{255, 255, 255, 255}, Texture: texture, Vector: surface}
	b := &meshBuilder{ctx: context.Background()}
	vertex := func(x, z, u, v float64) Vertex {
		return Vertex{Position: nv(x, .8, z), Normal: nv(0, 1, 0), U: u, V: v}
	}
	a, c, d, e := vertex(-1, -.5, 0, 0), vertex(1, -.5, 1, 0), vertex(1, .5, 1, 1), vertex(-1, .5, 0, 1)
	b.triangle(a, d, c, material, true)
	b.triangle(a, e, d, material, true)
	camera := nativeCameraAxes()
	camera.scale, camera.width, camera.height = 100, 400, 400
	scene := &nativeScene{camera: camera, width: 400, height: 400, triangles: b.triangles}
	frame := rasterSVGShadowCoverage(t, scene)
	ground, light := rasterShadowGround(b.triangles), rasterShadowDirection()
	for _, x := range []float64{-.75, 0, .75} {
		p := nv(x, .8, 0)
		p = nsub(p, nmul(light, (p.Y-ground-rasterShadowNormalOffset)/light.Y))
		p.Y = ground
		q := camera.project(p)
		alpha := frame.NRGBAAt(int(q.x), int(q.y)).A
		if x == 0 && alpha != 0 || x != 0 && alpha < 27 {
			t.Fatalf("texture alpha changed at x=%g: %d", x, alpha)
		}
	}
}
