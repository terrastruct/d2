package d2isometricimg

import (
	"context"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestNativeReadingAxisAndForeshortening(t *testing.T) {
	// Compare unit surface-text vectors independently of image fit/size.
	projection := func(direction Vec) (tilt, area float64) {
		direction = nunit(direction)
		right := nunit(ncross(nv(0, 1, 0), direction))
		up := nunit(ncross(direction, right))
		u := nv(ndot(nv(1, 0, 0), right), ndot(nv(1, 0, 0), up), 0)
		v := nv(ndot(nv(0, 0, 1), right), ndot(nv(0, 0, 1), up), 0)
		// Projected unit-square area measures foreshortening independently
		// of how azimuth distributes that compression between the text axes.
		return math.Abs(math.Atan2(u.Y, u.X)), math.Abs(u.X*v.Y - u.Y*v.X)
	}
	oldTilt, oldArea := projection(nv(1, 1, 1))
	view := nativeViewDirection()
	tilt, area := projection(view)
	if tilt >= oldTilt-.07 || area <= oldArea*1.1 {
		t.Fatalf("camera did not improve reading axis: tilt %f -> %f area %f -> %f", oldTilt, tilt, oldArea, area)
	}
	// Gentle skew reduces the printed text's slope while preserving the
	// viewing height, so improved reading does not flatten the physical solids.
	_, previousArea := projection(nv(.6, 1.35, 1))
	azimuth := math.Atan2(view.X, view.Z)
	if math.Abs(azimuth-15*math.Pi/180) > 1e-12 || tilt > 12*math.Pi/180 || math.Abs(area-previousArea) > 1e-12 {
		t.Fatal("gentle skew lost its shallow reading axis or original elevation")
	}
}

func TestNativeRouteStylingPreservesAuthoredPaint(t *testing.T) {
	a := d2isometric.Edge{ID: "a", Stroke: "#ff0080", StrokeExplicit: true}
	if nativeRouteTint(a) != a.Stroke {
		t.Fatal("authored stroke overridden")
	}
	a.StrokeExplicit = false
	tint := nativeRouteTint(a)
	_ = nativeRouteTint(d2isometric.Edge{ID: "unrelated"})
	if nativeRouteTint(a) != tint || tint == a.Stroke {
		t.Fatal("default route color is unstable or used non-explicit paint")
	}
}

func TestNativeSlopedTubeAndCasing(t *testing.T) {
	points := []Vec{nv(0, .08, 0), nv(.4, .23, 0), nv(.8, .08, 0)}
	b := &meshBuilder{ctx: context.Background()}
	b.routeCasing(points, .024, 1)
	for _, tri := range b.triangles {
		n := ncross(nsub(tri.V[1].Position, tri.V[0].Position), nsub(tri.V[2].Position, tri.V[0].Position))
		if n.Y <= 0 || tri.CastShadow {
			t.Fatal("route casing is inverted or casting its own shadow")
		}
	}
	b.triangles = nil
	b.tube(points, .024, nativeMaterial("#456789", .6, .05, 1))
	for _, tri := range b.triangles {
		for _, v := range tri.V {
			if math.Abs(nlen(v.Normal)-1) > 1e-8 {
				t.Fatal("sloped tube normal is not normalized")
			}
		}
	}
}

func TestNativeCasingDoesNotRevealTransparentStroke(t *testing.T) {
	b := &meshBuilder{ctx: context.Background()}
	e := d2isometric.Edge{ID: "hidden", Points: []Vec{nv(0, .08, 0), nv(4, .08, 0)}, StrokeWidth: 2, Stroke: "transparent", StrokeExplicit: true, Opacity: 1}
	packets := b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
	if b.err != nil || len(b.triangles) != 0 {
		t.Fatalf("casing revealed invisible wire: %d triangles, %v", len(b.triangles), b.err)
	}
	for _, packet := range packets {
		if packet.material.Color.A != 0 {
			t.Fatal("traffic revealed invisible wire")
		}
	}
}

func TestNativeDashPreservesBridgeAndCornerVertices(t *testing.T) {
	points := []Vec{nv(0, .08, 0), nv(2, .08, 0), nv(2.2, .2, 0), nv(2.4, .08, 0), nv(4, .08, 0), nv(4, .08, 3)}
	lengths := routeLengths(points)
	section := nativeRouteSection(points, lengths, .15, .95)
	for _, required := range points[1 : len(points)-1] {
		found := false
		for _, p := range section {
			found = found || p == required
		}
		if !found {
			t.Fatalf("dash removed a bridge/corner vertex: %+v", required)
		}
	}
	if section[0] != pathPoint(points, lengths, .15) || section[len(section)-1] != pathPoint(points, lengths, .95) {
		t.Fatal("dash endpoints differ from the packet's arc-length path")
	}
}
