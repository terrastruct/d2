package d2isometricimg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func classicInkTestNode() d2isometric.Node {
	return d2isometric.Node{Type: d2target.ShapeRectangle, Size: nv(2, 2, 2), Fill: "#dce5eb", Stroke: "#a9235e", StrokeExplicit: true, StrokeWidth: 4, Opacity: 1,
		Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{Width: 200, Height: 200, Stroke: "#a9235e"}}}
}

func classicInkTestCube(open bool) *meshBuilder {
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	m := &Material{Color: color.NRGBA{255, 255, 255, 255}, Unlit: true}
	if open {
		b.extrudedProfile([]Vec{nv(-1, 1, -1), nv(1, 1, -1), nv(1, 1, 1), nv(-1, 1, 1)}, -1, nil, m)
	} else {
		b.box(Vec{}, nv(2, 2, 2), m, 0)
	}
	return b
}

func TestClassicInkSelectsPhysicalCubeEdges(t *testing.T) {
	n := classicInkTestNode()
	for _, open := range []bool{false, true} {
		b := classicInkTestCube(open)
		before := append([]Triangle(nil), b.triangles...)
		segments, err := classicInkSegments(b.ctx, n, b.triangles)
		if err != nil {
			t.Fatal(err)
		}
		if len(segments) != 9 {
			t.Fatalf("open=%v: got %d edges, want 4 top, 2 bottom, 3 upright", open, len(segments))
		}
		top, bottom, upright := 0, 0, 0
		for _, s := range segments {
			delta := nsub(s.c, s.a)
			nonzero := 0
			for _, v := range []float64{delta.X, delta.Y, delta.Z} {
				if math.Abs(v) > 1e-9 {
					nonzero++
				}
			}
			if nonzero != 1 {
				t.Fatalf("triangulation diagonal became ink: %+v", s)
			}
			if s.a.Y == 1 && s.c.Y == 1 {
				top++
			} else if s.a.Y == -1 && s.c.Y == -1 {
				bottom++
			} else {
				upright++
			}
		}
		if top != 4 || bottom != 2 || upright != 3 {
			t.Fatalf("incorrect visible edges: top=%d bottom=%d upright=%d", top, bottom, upright)
		}
		if !reflect.DeepEqual(before, b.triangles) {
			t.Fatal("outline extraction changed physical geometry")
		}
		// Reversing triangle traversal cannot move a contour or change dash phase.
		for i, j := 0, len(b.triangles)-1; i < j; i, j = i+1, j-1 {
			b.triangles[i], b.triangles[j] = b.triangles[j], b.triangles[i]
		}
		reversed, err := classicInkSegments(b.ctx, n, b.triangles)
		if err != nil || len(reversed) != len(segments) {
			t.Fatal("reversed mesh changed contour count", err)
		}
		for i, s := range segments {
			if reversed[i].a != s.a || reversed[i].c != s.c {
				t.Fatal("triangle traversal changed contour order")
			}
		}
	}
}

func TestClassicInkPreservesDecorativeTopBorders(t *testing.T) {
	n := classicInkTestNode()
	n.Metadata.Original.DoubleBorder = true
	b := classicInkTestCube(true)
	segments, err := classicInkSegments(b.ctx, n, b.triangles)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 5 {
		t.Fatalf("decorated source rim was inked twice: %d edges", len(segments))
	}
	for _, s := range segments {
		if s.a.Y == 1 && s.c.Y == 1 {
			t.Fatal("source cap border duplicated")
		}
	}
	for _, kind := range []string{d2target.ShapeClass, d2target.ShapeSQLTable, d2target.ShapeText, d2target.ShapeCode, d2target.ShapeImage} {
		n := classicInkTestNode()
		n.Type = kind
		if nativeClassicRim(n) {
			t.Fatalf("%s lost document-owned borders", kind)
		}
	}
	n = classicInkTestNode()
	n.Fill = "transparent"
	if nativeClassicRim(n) {
		t.Fatal("transparent node lost its source outline")
	}
	// A source viewport quad has no bearing on the physical shape outline.
	n = classicInkTestNode()
	tex := image.NewRGBA(image.Rect(0, 0, 1, 1))
	tex.SetRGBA(0, 0, color.RGBA{255, 255, 255, 255})
	first := len(b.triangles)
	b.surfaceTexture(tex, labelSurface{center: nv(0, 1.0005, 0), width: 3, depth: 3}, 1)
	for i := first; i < len(b.triangles); i++ {
		b.triangles[i].CastShadow = true
		b.triangles[i].NoDepthWrite = false
		b.triangles[i].DepthBias = 0
	}
	segments, err = classicInkSegments(b.ctx, n, b.triangles)
	if err != nil || len(segments) != 9 {
		t.Fatal("texture viewport created extra border geometry", len(segments), err)
	}
	for _, s := range segments {
		for _, p := range []Vec{s.a, s.c} {
			if math.Abs(p.X) > 1 || math.Abs(p.Z) > 1 {
				t.Fatal("outline followed the texture viewport outside the source footprint")
			}
		}
	}
}

func TestClassicInkSmoothSphereContourIgnoresTessellation(t *testing.T) {
	n := classicInkTestNode()
	n.Type = d2target.ShapeCloud
	b := &meshBuilder{ctx: context.Background()}
	b.sphere(Vec{}, nv(1, 1, 1), nativeMaterial("white", 1, 0, 1), 16, 8)
	segments, err := classicInkSegments(b.ctx, n, b.triangles)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) < 16 || len(segments) > 64 {
		t.Fatalf("sphere contour has %d segments", len(segments))
	}
	view := nativeCameraAxes().direction
	for _, s := range segments {
		for _, p := range []Vec{s.a, s.c} {
			// The smooth normal zero crossing is the sphere's great-circle
			// plane. Mesh-edge silhouettes have appreciable signed depth here.
			if math.Abs(ndot(p, view)) > 1e-7 || nlen(p) < .92 {
				t.Fatalf("mesh noise survived smooth silhouette extraction: %v", p)
			}
		}
	}
	paths := classicInkPaths(segments)
	if len(paths) != 1 || classicInkKeyOf(paths[0][0].a) != classicInkKeyOf(paths[0][len(paths[0])-1].c) {
		t.Fatal("smooth silhouette is not one closed contour")
	}
}

func TestClassicInkCylinderHasRimsWithoutFacetLadder(t *testing.T) {
	n := classicInkTestNode()
	n.Type = d2target.ShapeCylinder
	b := &meshBuilder{ctx: context.Background()}
	m := nativeMaterial("white", 1, 0, 1)
	b.solidUpright(nv(0, -1, 0), 2, 2, 2, false, nativeSolidPaint{cap: m, wall: m})
	segments, err := classicInkSegments(b.ctx, n, b.triangles)
	if err != nil {
		t.Fatal(err)
	}
	top, side, bottom := 0, 0, 0
	for _, s := range segments {
		if s.a.Y == 1 && s.c.Y == 1 {
			top++
		} else if s.a.Y == -1 && s.c.Y == -1 {
			bottom++
		} else {
			side++
			if s.a.X != s.c.X || s.a.Z != s.c.Z {
				t.Fatal("smooth wall triangulation was drawn as a structural edge")
			}
		}
	}
	if top != 72 || bottom < 30 || bottom > 42 || side != 2 {
		t.Fatalf("unexpected cylinder linework: top=%d bottom=%d side=%d", top, bottom, side)
	}
}

func TestClassicInkCylinderAndTorsoRimsJoinVisibleSilhouettes(t *testing.T) {
	for _, kind := range []string{d2target.ShapeCylinder, d2target.ShapePerson} {
		t.Run(kind, func(t *testing.T) {
			n := classicInkTestNode()
			n.Type = kind
			n.Stroke = "#df1040"
			n.StrokeWidth = 2
			b := &meshBuilder{ctx: context.Background(), scale: .01}
			material := &Material{Color: color.NRGBA{255, 255, 255, 255}, Unlit: true}
			floor, height := .07, 1.2
			center := nv(7, floor+height/2, 11)
			if kind == d2target.ShapeCylinder {
				b.solidUpright(nv(center.X, floor, center.Z), 2.4, 1.6, height, false, nativeSolidPaint{cap: material, wall: material})
			} else {
				b.cylinder(center, .7, .55, height, .45, material, material, 32, false)
			}
			segments, err := classicInkSegments(b.ctx, n, b.triangles)
			if err != nil {
				t.Fatal(err)
			}
			var joins [][2]classicInkSegment
			for _, side := range segments {
				if math.Abs(side.a.Y-side.c.Y) < height*.9 {
					continue
				}
				if side.a.Y > side.c.Y {
					side.a, side.c = side.c, side.a
				}
				found := false
				for _, rim := range segments {
					if math.Abs(rim.a.Y-floor) > 1e-9 || math.Abs(rim.c.Y-floor) > 1e-9 {
						continue
					}
					if classicInkKeyOf(rim.c) == classicInkKeyOf(side.a) {
						rim.a, rim.c = rim.c, rim.a
					}
					if classicInkKeyOf(rim.a) == classicInkKeyOf(side.a) {
						joins = append(joins, [2]classicInkSegment{side, rim})
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("straight silhouette ends without a connected lower rim at %v", side.a)
				}
			}
			if len(joins) != 2 {
				t.Fatalf("expected two silhouette/rim joins, got %d", len(joins))
			}
			b.classicInkEdges(n, b.triangles)
			if b.err != nil {
				t.Fatal(b.err)
			}
			camera := nativeCameraAxes()
			camera.width = 1200
			camera.height = 1000
			camera.scale = 300
			camera.centerX, camera.centerY = ndot(center, camera.right), ndot(center, camera.up)
			r, err := newRaster(b.ctx, 600, 500, b.triangles, color.NRGBA{255, 255, 255, 255}, &camera, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, join := range joins {
				for _, segment := range join {
					for distance := 0.; distance <= .04; distance += .004 {
						at := nlerp(segment.a, segment.c, min(1, distance/nlen(nsub(segment.c, segment.a))))
						p := camera.project(at)
						// Probe the supersample nearest the geometric outline. A final
						// pixel also averages samples outside the silhouette, so its
						// coverage depends on the camera's subpixel alignment.
						pixel := r.pixels.RGBAAt(int(p.x), int(p.y))
						if pixel != (color.RGBA{223, 16, 64, 255}) {
							t.Fatalf("visible rim/silhouette joint has a raster gap at %v: %v", at, pixel)
						}
					}
				}
			}
		})
	}
}

func TestClassicInkAuthoredPaintDashAndCaptionOrder(t *testing.T) {
	for _, stroke := range []string{"none", "transparent", "rgba(20,40,60,0)"} {
		n := classicInkTestNode()
		n.Stroke = stroke
		b := classicInkTestCube(false)
		before := len(b.triangles)
		b.classicInkEdges(n, b.triangles)
		if b.err != nil || len(b.triangles) != before {
			t.Fatalf("%q emitted visible ink", stroke)
		}
	}
	n := classicInkTestNode()
	n.StrokeWidth = 0
	b := classicInkTestCube(false)
	before := len(b.triangles)
	b.classicInkEdges(n, b.triangles)
	if len(b.triangles) != before {
		t.Fatal("zero-width stroke emitted ink")
	}
	n = classicInkTestNode()
	n.Stroke = "rgb(20,40,60)"
	n.StrokeDash = 2.5
	b = classicInkTestCube(false)
	tex := image.NewRGBA(image.Rect(0, 0, 1, 1))
	tex.SetRGBA(0, 0, color.RGBA{255, 255, 255, 255})
	b.surfaceTexture(tex, labelSurface{center: nv(0, 1.01, 0), width: .5, depth: .5}, 1)
	caption := append([]Triangle(nil), b.triangles[len(b.triangles)-2:]...)
	before = len(b.triangles)
	b.classicInkEdges(n, b.triangles)
	if b.err != nil {
		t.Fatal(b.err)
	}
	if added := len(b.triangles) - before; added < 50 || added > 3000 {
		t.Fatalf("dash subdivision did not terminate at source spacing: %d triangles", added)
	}
	if !reflect.DeepEqual(caption, b.triangles[len(b.triangles)-2:]) {
		t.Fatal("outline insertion moved the final source caption")
	}
	for _, tri := range b.triangles[before-2 : len(b.triangles)-2] {
		if tri.Material.Color != (color.NRGBA{20, 40, 60, 255}) || !tri.Material.Unlit || tri.CastShadow || tri.NoDepthWrite {
			t.Fatalf("source stroke paint/depth behavior lost: %+v", tri)
		}
	}
}

func TestClassicInkTranslucentGroupRetainsCompensatedSourceCap(t *testing.T) {
	for _, strokeAlpha := range []bool{false, true} {
		n := classicInkTestNode()
		if strokeAlpha {
			n.Stroke = "rgba(20,40,60,0.5)"
		} else {
			n.Opacity = .35
		}
		if nativeClassicRim(n) {
			t.Fatal("translucent source lost its compensated cap border")
		}
		b := classicInkTestCube(false)
		before := append([]Triangle(nil), b.triangles...)
		b.classicInkEdges(n, b.triangles)
		if b.err != nil || !reflect.DeepEqual(before, b.triangles) {
			t.Fatal("additional ink compounded source group opacity", b.err)
		}
	}
}

func TestClassicInkRasterWeightAndOcclusion(t *testing.T) {
	n := classicInkTestNode()
	n.Stroke = "#df1040"
	n.StrokeWidth = 8
	b := classicInkTestCube(true)
	b.classicInkEdges(n, b.triangles)
	if b.err != nil {
		t.Fatal(b.err)
	}
	camera := nativeCameraAxes()
	camera.width = 960
	camera.height = 960
	camera.scale = 200
	white := color.NRGBA{255, 255, 255, 255}
	r, err := newRaster(context.Background(), 480, 480, b.triangles, white, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Each rear and front top edge keeps both halves of its centered
	// 8-pixel stroke, even though the cap and side wall meet at that edge.
	for _, pair := range [][2]Vec{{nv(-1, 1, -1), nv(1, 1, -1)}, {nv(-1, 1, 1), nv(1, 1, 1)}} {
		mid := nmul(nadd(pair[0], pair[1]), .5)
		side := nunit(ncross(camera.direction, nsub(pair[1], pair[0])))
		for _, offset := range []float64{-.023, .023} {
			p := camera.project(nadd(mid, nmul(side, offset)))
			pixel := r.output.RGBAAt(int(p.x)/2, int(p.y)/2)
			if pixel.R < 150 || pixel.G > 100 || pixel.B > 150 {
				t.Fatalf("centered top stroke lost one half: %v at %v", pixel, mid)
			}
		}
	}
	// A nearer unlit surface must occlude every ink pixel, regardless of
	// its tiny coplanar bias. Ink is not a screen overlay.
	cover := &meshBuilder{ctx: context.Background()}
	v := func(x, y float64) Vec {
		return nadd(nadd(nmul(camera.right, x), nmul(camera.up, y)), nmul(camera.direction, 10))
	}
	m := &Material{Color: color.NRGBA{40, 170, 70, 255}, Unlit: true}
	cover.flat(v(-3, -3), v(3, -3), v(3, 3), m, false)
	cover.flat(v(-3, -3), v(3, 3), v(-3, 3), m, false)
	covered, err := newRaster(context.Background(), 480, 480, append(append([]Triangle(nil), b.triangles...), cover.triangles...), white, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := newRaster(context.Background(), 480, 480, cover.triangles, white, &camera, nil)
	if err != nil || !bytes.Equal(covered.output.Pix, baseline.output.Pix) {
		t.Fatal("hidden structural ink leaked through a nearer component", err)
	}
}

func TestClassicInkCancellationAndWorkLimit(t *testing.T) {
	n := classicInkTestNode()
	b := classicInkTestCube(false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := classicInkSegments(ctx, n, b.triangles); !errors.Is(err, context.Canceled) {
		t.Fatal("outline ignored cancellation", err)
	}
	if _, err := classicInkSegments(context.Background(), n, make([]Triangle, maxClassicInkTriangles+1)); err == nil {
		t.Fatal("unbounded outline input accepted")
	}
}

func TestClassicInkSmoothContourHasContinuousRasterCoverage(t *testing.T) {
	n := classicInkTestNode()
	n.Type = d2target.ShapeCloud
	n.Stroke = "#df1040"
	n.StrokeWidth = 6
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.sphere(Vec{}, nv(1, 1, 1), &Material{Color: color.NRGBA{255, 255, 255, 255}, Unlit: true}, 20, 12)
	segments, err := classicInkSegments(b.ctx, n, b.triangles)
	if err != nil {
		t.Fatal(err)
	}
	b.classicInkEdges(n, b.triangles)
	if b.err != nil {
		t.Fatal(b.err)
	}
	camera := nativeCameraAxes()
	camera.width = 768
	camera.height = 768
	camera.scale = 240
	r, err := newRaster(b.ctx, 384, 384, b.triangles, color.NRGBA{255, 255, 255, 255}, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range segments {
		for _, fraction := range []float64{.2, .5, .8} {
			p := camera.project(nlerp(s.a, s.c, fraction))
			pixel := r.output.RGBAAt(int(p.x)/2, int(p.y)/2)
			if pixel.R < 150 || pixel.G > 100 || pixel.B > 150 {
				t.Fatalf("smooth contour has a self-occluded gap at %.2f of %v..%v: %v", fraction, s.a, s.c, pixel)
			}
		}
	}
}

func TestClassicInkMultipleRearCopyCannotBleedAcrossPrimaryCap(t *testing.T) {
	n := classicInkTestNode()
	n.Stroke = "#167541"
	n.StrokeWidth = 6
	material := &Material{Color: color.NRGBA{255, 255, 255, 255}, Unlit: true}
	build := func(rearInk bool) []Triangle {
		b := &meshBuilder{ctx: context.Background(), scale: .01}
		b.box(nv(.2, .0225-.0153, -.2), nv(2, .045, 1.4), material, 0)
		if rearInk {
			b.classicInkEdges(n, b.triangles)
		}
		first := len(b.triangles)
		b.box(nv(0, .0225, 0), nv(2, .045, 1.4), material, 0)
		b.classicInkEdges(n, b.triangles[first:])
		if b.err != nil {
			t.Fatal(b.err)
		}
		return b.triangles
	}
	camera := nativeCameraAxes()
	camera.width = 1000
	camera.height = 900
	camera.scale = 250
	white := color.NRGBA{255, 255, 255, 255}
	with, err := newRaster(context.Background(), 500, 450, build(true), white, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	without, err := newRaster(context.Background(), 500, 450, build(false), white, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The physical copies are separated by only .0153 after relief. The
	// rear contour remains occluded across the entire primary cap interior.
	for x := -.85; x <= .85; x += .025 {
		for z := -.55; z <= .55; z += .025 {
			p := camera.project(nv(x, .045, z))
			px, py := int(p.x)/2, int(p.y)/2
			if with.output.RGBAAt(px, py) != without.output.RGBAAt(px, py) {
				t.Fatalf("rear copy ink leaked across primary cap at %v (%d,%d): %v vs %v", nv(x, .045, z), px, py, with.output.RGBAAt(px, py), without.output.RGBAAt(px, py))
			}
		}
	}
}
