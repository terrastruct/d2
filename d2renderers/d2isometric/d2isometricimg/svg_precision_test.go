package d2isometricimg

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestSVGVisibilityIgnoresProjectionFMAResiduals(t *testing.T) {
	// Camera projection can contract multiplication and addition on ARM but
	// round them separately on amd64. Construct both results explicitly so
	// the regression also exercises the difference on a single architecture.
	epsilon := math.Ldexp(1, -27)
	project := func(value, magnitude float64, fused bool) float64 {
		a, b := 1+epsilon, magnitude*(1-epsilon)
		if fused {
			return math.FMA(a, b, value-magnitude)
		}
		return float64(a*b) + (value - magnitude)
	}
	original := []svgVisibilityFace{
		{points: svgVisibilityRectangle(0, 0, 10, 10, 0, 0, 0)},
		{points: svgVisibilityRectangle(2, 2, 6, 8, 1, 0, 0), opaque: true, order: 1},
		{points: svgVisibilityRectangle(4, 2, 8, 8, 2, 0, 0), opaque: true, order: 2},
	}
	makeFaces := func(fused bool) (raw, canonical []svgVisibilityFace) {
		for i, face := range original {
			p, q := face, face
			p.points, q.points = nil, nil
			for j, point := range face.points {
				magnitude := math.Ldexp(1, 20+(i+j)%3)
				x := project(point.x, magnitude, fused)
				y := project(point.y, -magnitude, fused)
				z := project(point.z, 1, fused)
				p.points = append(p.points, svgPoint{x, y, z})
				q.points = append(q.points, svgProjectedPoint(x, y, z))
			}
			raw, canonical = append(raw, p), append(canonical, q)
		}
		return raw, canonical
	}
	rawRounded, rounded := makeFaces(false)
	rawFused, fused := makeFaces(true)
	if reflect.DeepEqual(rawRounded, rawFused) {
		t.Fatal("fixture did not produce distinct FMA projection residuals")
	}
	left, err := svgVisibleFaces(context.Background(), rounded)
	if err != nil {
		t.Fatal(err)
	}
	right, err := svgVisibleFaces(context.Background(), fused)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatal("subpixel projection residual changed visible-fragment topology or coordinates")
	}
	for i := range left {
		if svgPolygonPath(left[i]) != svgPolygonPath(right[i]) {
			t.Fatalf("projection contraction changed serialized face %d", i)
		}
	}
	if area := svgVisibilityArea(left[0]); area != 64 {
		t.Fatalf("canonical projection changed the visible silhouette: area %g, want 64", area)
	}
	// Precision normalization cannot flatten the deliberately raised print
	// plane into the face below it.
	base, raised := svgProjectedPoint(0, 0, 1), svgProjectedPoint(0, 0, 1+.0005)
	if raised.z-base.z < .00049 {
		t.Fatal("projection precision erased the authored depth bias")
	}
}

func TestSVGPredicatesRoundCancellationProducts(t *testing.T) {
	epsilon := math.Ldexp(1, -27)
	a, b := 1+epsilon, 1-epsilon
	if math.FMA(a, b, -1) == 0 || float64(a*b)-1 != 0 {
		t.Fatal("fixture no longer distinguishes fused and rounded products")
	}
	origin, endpoint, point := svgPoint{}, svgPoint{x: a, y: 1}, svgPoint{x: 1, y: b}
	if distance := svgEdgeDistance(origin, endpoint)(point); distance != 0 {
		t.Fatalf("collinear rounded edge acquired a side from an FMA residual: %g", distance)
	}
	if area := svgPolygonArea([]svgPoint{origin, endpoint, point}); area != 0 {
		t.Fatalf("rounded zero-area triangle acquired area from an FMA residual: %g", area)
	}
	plane := svgDepthPlane{origin: svgPoint{z: -1}, dx: a}
	if depth := plane.at(svgPoint{x: b}); depth != 0 {
		t.Fatalf("coplanar rounded depth acquired an FMA residual: %g", depth)
	}
}

func TestSVGIntersectionRoundsInterpolatedProducts(t *testing.T) {
	epsilon := math.Ldexp(1, -27)
	// The edge from (-1, -(1-e)) to (e, e) crosses y=0 at t=1-e.
	// Its x/z products round to exactly 1 before adding -1; a fused
	// interpolation instead leaves -2^-54, changing subsequent half-planes.
	polygon := []svgPoint{{-1, -(1 - epsilon), -1}, {epsilon, epsilon, epsilon}, {2, 1, 2}}
	inside, outside := svgSplitPolygon(polygon, func(p svgPoint) float64 { return p.y })
	for _, fragment := range [][]svgPoint{inside, outside} {
		if len(fragment) < 3 {
			t.Fatal("a valid side of the split disappeared")
		}
		found := false
		for _, point := range fragment {
			if point.y == 0 && math.Abs(point.x) < epsilon/2 {
				found = true
				if point.x != 0 || point.z != 0 {
					t.Fatalf("shared intersection carries an FMA residual: %+v", point)
				}
			}
		}
		if !found {
			t.Fatal("split omitted its shared intersection")
		}
	}
}

func TestSVGPolygonPathCanonicalizesSharedVertices(t *testing.T) {
	triangle := []svgPoint{{2, 3, 0}, {5, 4, 0}, {3, 7, 0}}
	want := svgPolygonPath([][]svgPoint{triangle})
	for _, polygon := range [][]svgPoint{
		{triangle[1], triangle[2], triangle[0]},
		{triangle[2], triangle[1], triangle[0]},
		{triangle[0], {2 + 1e-9, 3 - 1e-9, 0}, triangle[1], triangle[2], triangle[0]},
	} {
		if got := svgPolygonPath([][]svgPoint{polygon}); got != want {
			t.Fatalf("equivalent clipped loop serialized differently: %s != %s", got, want)
		}
	}
}

func TestSVGLinksPreserveEqualAreaOrderAfterProjectionRounding(t *testing.T) {
	// Residuals far below a serialized pixel must not reorder overlapping
	// same-size hit regions; that order chooses which source link is clickable.
	camera := rasterCamera{right: nv(1, 0, 0), up: nv(0, -1, 0), direction: nv(0, 0, 1), scale: 1}
	scene := &nativeScene{camera: camera}
	for i, url := range []string{"first", "second", "third"} {
		width := 10. + float64(i)*1e-10
		scene.links = append(scene.links, nativeLink{
			points: [4]Vec{nv(0, 0, 0), nv(width, 0, 0), nv(width, 10, 0), nv(0, 10, 0)},
			region: d2scene.LinkRegion{URL: url},
		})
	}
	w := &nativeSVGWriter{ctx: context.Background()}
	writeSVGLinks(w, scene)
	if w.err != nil {
		t.Fatal(w.err)
	}
	output := w.buf.String()
	previous := -1
	for _, url := range []string{"first", "second", "third"} {
		at := strings.Index(output, `xlink:href="`+url+`"`)
		if at <= previous {
			t.Fatalf("subpixel area residual reordered equal source links: %s", output)
		}
		previous = at
	}
	if len(scene.links) != 3 || scene.links[0].region.URL != "first" {
		t.Fatal("link serialization mutated source order")
	}
}

func TestClassicInkDepthRoundsCancellationProducts(t *testing.T) {
	epsilon := math.Ldexp(1, -27)
	face := classicInkFacet{points: [3]Vec{nv(0, 0, -2), nv(1, 0, 2*(1+epsilon)), nv(0, 1, 0)}}
	point := nv((1-epsilon)/2, 0, 0)
	// The two depth terms round to -(1+e) and 1, leaving exactly -e.
	// Fusing the second multiplication instead leaves an extra -2^-54.
	if math.FMA(point.X, face.points[1].Z, -(1+epsilon)) == -epsilon {
		t.Fatal("fixture no longer distinguishes fused and rounded interpolation")
	}
	depth, inside := face.depthAt(point)
	if !inside || depth != -epsilon {
		t.Fatalf("ink support interpolation retained an FMA residual: depth=%g inside=%t, want %g", depth, inside, -epsilon)
	}
}

func TestRasterCameraRoundsScaledCoordinates(t *testing.T) {
	epsilon := math.Ldexp(1, -27)
	camera := rasterCamera{
		right: nv(1, 0, 0), up: nv(0, 1, 0), direction: nv(0, 0, 1),
		scale: 1 - epsilon, width: 64, height: 64,
	}
	point := nv(-32*(1+epsilon), 32*(1+epsilon), 0)
	if math.FMA(point.X, camera.scale, 32) == 0 {
		t.Fatal("fixture does not distinguish fused camera projection")
	}
	if projected := camera.project(point); projected.x != 0 || projected.y != 0 {
		t.Fatalf("camera retained multiply-add residuals before projection quantization: %+v", projected)
	}
}

func TestStructuredRailRoundsSourceContourTransform(t *testing.T) {
	// Three source units map onto two world units around the origin. The
	// rounded product 3*(2/3) is exactly 2, giving corners at +/-1. Fusing
	// the product with the origin instead displaces the positive corner.
	node := classicInkTestNode()
	node.Metadata.Original.Width, node.Metadata.Original.Height = 3, 3
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.structuredRail(node, 0, 3, 0, 1, "white")
	if b.err != nil {
		t.Fatal(b.err)
	}
	vertices := 0
	for _, triangle := range b.triangles {
		for _, vertex := range triangle.V {
			if vertex.Normal != nv(0, 1, 0) {
				continue
			}
			vertices++
			if math.Abs(vertex.Position.X) != 1 || math.Abs(vertex.Position.Z) != 1 {
				t.Fatalf("row cap no longer exactly matches its rounded source footprint: %+v", vertex.Position)
			}
		}
	}
	if vertices != 6 {
		t.Fatalf("expected two source cap triangles, inspected %d vertices", vertices)
	}
}
