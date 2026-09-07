package d2isometricimg

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func svgVisibilityRectangle(x0, y0, x1, y1, z, dx, dy float64) []svgPoint {
	return []svgPoint{{x0, y0, z}, {x1, y0, z + dx*(x1-x0)}, {x1, y1, z + dx*(x1-x0) + dy*(y1-y0)}, {x0, y1, z + dy*(y1-y0)}}
}

func svgVisibilityArea(fragments [][]svgPoint) float64 {
	var area float64
	for _, fragment := range fragments {
		area += math.Abs(svgPolygonArea(fragment))
	}
	return area
}

func TestSVGVisibilityCutsCrossingDepthPlanes(t *testing.T) {
	faces := []svgVisibilityFace{
		{points: svgVisibilityRectangle(0, 0, 10, 10, 0, 0, 0), opaque: true},
		{points: svgVisibilityRectangle(0, 0, 10, 10, -5, 1, 0), opaque: true},
	}
	visible, err := svgVisibleFaces(context.Background(), faces)
	if err != nil {
		t.Fatal(err)
	}
	for i, fragments := range visible {
		if area := svgVisibilityArea(fragments); math.Abs(area-50) > 1e-8 {
			t.Fatalf("face %d has visible area %v, want 50", i, area)
		}
		plane, _ := svgPolygonPlane(faces[i].points)
		for _, fragment := range fragments {
			for _, p := range fragment {
				if math.Abs(plane.at(p)-p.z) > 1e-10 {
					t.Fatalf("cut moved face %d off its own depth plane: %+v", i, p)
				}
			}
		}
	}
}

func TestSVGVisibilitySubtractsUnionWithoutOverlappingFragments(t *testing.T) {
	faces := []svgVisibilityFace{
		{points: svgVisibilityRectangle(0, 0, 10, 10, 0, 0, 0)},
		{points: svgVisibilityRectangle(2, 2, 6, 8, 1, 0, 0), opaque: true},
		{points: svgVisibilityRectangle(4, 2, 8, 8, 2, 0, 0), opaque: true},
	}
	visible, err := svgVisibleFaces(context.Background(), faces)
	if err != nil {
		t.Fatal(err)
	}
	if area := svgVisibilityArea(visible[0]); math.Abs(area-64) > 1e-8 {
		t.Fatalf("overlapping occluders were not subtracted as a union: area %v, want 64", area)
	}
	for i, a := range visible[0] {
		for _, b := range visible[0][i+1:] {
			intersection := a
			for k, p := range b {
				intersection, _ = svgSplitPolygon(intersection, svgEdgeDistance(p, b[(k+1)%len(b)]))
			}
			if math.Abs(svgPolygonArea(intersection)) > 1e-9 {
				t.Fatal("visible fragments overlap and would compound alpha")
			}
		}
	}
}

func TestSVGVisibilitySourceOpacityGroupsAndDecals(t *testing.T) {
	a, b := &nativeOpacityGroup{Opacity: .5}, &nativeOpacityGroup{Opacity: .3}
	quad := func(z float64) []svgPoint { return svgVisibilityRectangle(0, 0, 10, 10, z, 0, 0) }
	faces := []svgVisibilityFace{
		{points: quad(0)},                                                    // ordinary backdrop
		{points: quad(1), opaque: true, group: a},                            // rear face inside a
		{points: quad(2), opaque: true, group: a},                            // cap inside a
		{points: quad(3), group: a},                                          // printed decal, does not write depth
		{points: quad(4), opaque: true, group: b},                            // separate faded object
		{points: svgVisibilityRectangle(0, 0, 2, 10, 5, 0, 0), opaque: true}, // ordinary front occluder
	}
	visible, err := svgVisibleFaces(context.Background(), faces)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{80, 0, 80, 80, 80, 20}
	for i := range want {
		if area := svgVisibilityArea(visible[i]); math.Abs(area-want[i]) > 1e-8 {
			t.Fatalf("face %d visible area %v, want %v; group faces only occlude their own group and decals never occlude", i, area, want[i])
		}
	}
}

func TestSVGVisibilityCoplanarPaintOrderAndDepthBias(t *testing.T) {
	quad := func(z float64) []svgPoint { return svgVisibilityRectangle(0, 0, 10, 10, z, 0, 0) }
	for _, tc := range []struct {
		name   string
		faces  []svgVisibilityFace
		winner int
	}{
		{"source order", []svgVisibilityFace{{points: quad(0), opaque: true, order: 3}, {points: quad(0), opaque: true, order: 2}}, 0},
		{"input tie", []svgVisibilityFace{{points: quad(0), opaque: true}, {points: quad(0), opaque: true}}, 1},
		{"depth bias", []svgVisibilityFace{{points: quad(.001), opaque: true}, {points: quad(0), opaque: true, order: 50}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			visible, err := svgVisibleFaces(context.Background(), tc.faces)
			if err != nil {
				t.Fatal(err)
			}
			for i, fragments := range visible {
				want := 0.0
				if i == tc.winner {
					want = 100
				}
				if area := svgVisibilityArea(fragments); area != want {
					t.Fatalf("face %d area %v, want %v", i, area, want)
				}
			}
		})
	}
}

func svgVisibilityContains(points []svgPoint, p svgPoint) bool {
	if len(points) == 0 {
		return false
	}
	for i, a := range points {
		if svgEdgeDistance(a, points[(i+1)%len(points)])(p) < -1e-9 {
			return false
		}
	}
	return true
}

func TestSVGVisibilityContourKeepsItsOwnSubstrateUnderTheInk(t *testing.T) {
	owner, foreign := &nativePaintOwner{Opaque: true}, &nativePaintOwner{Opaque: true}
	group := &nativeOpacityGroup{Opacity: .5}
	for _, tc := range []struct {
		name   string
		owners [2]*nativePaintOwner
		groups [2]*nativeOpacityGroup
		area   float64
	}{
		{"same owner", [2]*nativePaintOwner{owner, owner}, [2]*nativeOpacityGroup{}, 100},
		{"same source opacity group", [2]*nativePaintOwner{}, [2]*nativeOpacityGroup{group, group}, 100},
		{"foreign contour", [2]*nativePaintOwner{owner, foreign}, [2]*nativeOpacityGroup{}, 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			faces := []svgVisibilityFace{
				{points: svgVisibilityRectangle(0, 0, 10, 10, 0, 0, 0), opaque: true, owner: tc.owners[0], group: tc.groups[0]},
				{points: svgVisibilityRectangle(2, 2, 8, 8, 1, 0, 0), opaque: true, contour: true, owner: tc.owners[1], group: tc.groups[1]},
			}
			visible, err := svgVisibleFaces(context.Background(), faces)
			if err != nil {
				t.Fatal(err)
			}
			if area := svgVisibilityArea(visible[0]); math.Abs(area-tc.area) > 1e-8 {
				t.Fatalf("substrate area %v, want %v", area, tc.area)
			}
		})
	}
}

// Dense crossing planes exercise intersections and cyclic projected overlap.
// A pointwise depth oracle is independent of the polygon subtraction/grid and
// checks every face, including same-group occlusion and non-occluding paint.
func TestSVGVisibilityMatchesDepthOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	groups := []*nativeOpacityGroup{nil, {Opacity: .5}, {Opacity: .8}}
	var faces []svgVisibilityFace
	for i := 0; i < 28; i++ {
		x, y := rng.Float64()*12, rng.Float64()*12
		faces = append(faces, svgVisibilityFace{
			points: svgVisibilityRectangle(x, y, x+3+rng.Float64()*8, y+3+rng.Float64()*8, rng.Float64()*8, rng.Float64()*2-1, rng.Float64()*2-1),
			opaque: i%4 != 0, group: groups[i%3], order: i,
		})
	}
	visible, err := svgVisibleFaces(context.Background(), faces)
	if err != nil {
		t.Fatal(err)
	}
	for i, face := range faces {
		plane, _ := svgPolygonPlane(face.points)
		for y := .137; y < 23; y += .431 {
			for x := .193; x < 23; x += .479 {
				p := svgPoint{x: x, y: y}
				want := svgVisibilityContains(face.points, p)
				if want {
					for j, other := range faces {
						if j == i || !other.opaque || (other.group != nil && other.group != face.group) || !svgVisibilityContains(other.points, p) {
							continue
						}
						op, _ := svgPolygonPlane(other.points)
						if op.at(p) > plane.at(p) {
							want = false
							break
						}
					}
				}
				matches := 0
				for _, fragment := range visible[i] {
					if svgVisibilityContains(fragment, p) {
						matches++
					}
				}
				if got := matches != 0; got != want || matches > 1 {
					t.Fatalf("face %d point %.3f,%.3f: got visible=%v in %d fragments, want %v", i, x, y, got, matches, want)
				}
			}
		}
	}
}

func TestSVGVisibilityGridAndTranslationStayDeterministic(t *testing.T) {
	var faces []svgVisibilityFace
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			faces = append(faces, svgVisibilityFace{points: svgVisibilityRectangle(1e8+float64(x)*10, 1e8+float64(y)*10, 1e8+float64(x)*10+4, 1e8+float64(y)*10+4, 0, .1, .2), opaque: true})
		}
	}
	// This large face must enter the overflow bucket and occlude half the grid.
	faces = append(faces, svgVisibilityFace{points: svgVisibilityRectangle(1e8, 1e8, 1e8+195, 1e8+400, 20, 0, 0), opaque: true})
	before := make([][]svgPoint, len(faces))
	for i, f := range faces {
		before[i] = append([]svgPoint(nil), f.points...)
	}
	a, err := svgVisibleFaces(context.Background(), faces)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svgVisibleFaces(context.Background(), faces)
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("visibility is not deterministic: %v", err)
	}
	for i, f := range faces {
		if !reflect.DeepEqual(before[i], f.points) {
			t.Fatalf("modified input face %d", i)
		}
		if i == len(faces)-1 {
			continue
		}
		want := 16.0
		if i%40 < 20 {
			want = 0
		}
		if area := svgVisibilityArea(a[i]); math.Abs(area-want) > 1e-5 {
			t.Fatalf("translated face %d visible area %v, want %v", i, area, want)
		}
	}
}

func TestSVGVisibilityBoundsCancellationAndDegenerates(t *testing.T) {
	for _, faces := range []int{0, 10, 1000, 40_000} {
		if got := svgVisibilityWorkLimit(faces); got != 100_000_000 {
			t.Fatalf("ordinary diagram budget changed: %d faces -> %d", faces, got)
		}
	}
	if got := svgVisibilityWorkLimit(100_000); got != 204_800_000 {
		t.Fatalf("large mesh did not get a linear allowance: %d", got)
	}
	for _, faces := range []int{1_000_000, int(^uint(0) >> 1)} {
		if got := svgVisibilityWorkLimit(faces); got != rasterMaxWork {
			t.Fatalf("large mesh budget exceeds native cap: %d", got)
		}
	}
	face := svgVisibilityFace{points: svgVisibilityRectangle(0, 0, 10, 10, 0, 0, 0), opaque: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svgVisibleFaces(ctx, []svgVisibilityFace{face}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error: %v", err)
	}
	limits := svgDefaultVisibilityLimits
	limits.work = 3
	if _, err := svgVisibleFacesWithLimits(context.Background(), []svgVisibilityFace{face}, limits); err == nil || !strings.Contains(err.Error(), "work limit") {
		t.Fatalf("work limit error: %v", err)
	}
	limits = svgDefaultVisibilityLimits
	limits.fragments = 1
	front := svgVisibilityFace{points: svgVisibilityRectangle(2, 2, 8, 8, 1, 0, 0), opaque: true}
	if _, err := svgVisibleFacesWithLimits(context.Background(), []svgVisibilityFace{face, front}, limits); err == nil || !strings.Contains(err.Error(), "fragment limit") {
		t.Fatalf("fragment limit error: %v", err)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 2e9} {
		if _, err := svgVisibleFaces(context.Background(), []svgVisibilityFace{{points: []svgPoint{{bad, 0, 0}, {1, 0, 0}, {0, 1, 0}}}}); err == nil {
			t.Fatalf("accepted invalid coordinate %v", bad)
		}
	}
	visible, err := svgVisibleFaces(context.Background(), []svgVisibilityFace{{}, {points: []svgPoint{{0, 0, 0}, {1, 1, 1}, {2, 2, 2}}}})
	if err != nil || len(visible[0]) != 0 || len(visible[1]) != 0 {
		t.Fatalf("degenerate faces were not omitted: %v %v", visible, err)
	}
}
