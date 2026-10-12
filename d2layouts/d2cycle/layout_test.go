package d2cycle

import (
	"math"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/lib/geo"
)

// A curved connection is rendered as a chain of cubic segments, so the render
// target validator requires 1+3n points on its route. Every route this package
// emits samples ARC_STEPS+1 points, which ties the constant to that rule: the
// current value of 30 satisfies it, but nothing in the sampling loops would
// stop a later edit from picking a value that rejects every arc.
func TestARCStepsSamplesACubicPointCount(t *testing.T) {
	points := ARC_STEPS + 1
	if (points-1)%3 != 0 {
		t.Fatalf("ARC_STEPS = %d samples %d points, but a curved route needs 1+3n; pick a multiple of 3", ARC_STEPS, points)
	}
}

// A cycle holding a single shape has no neighbours to make room for, and the
// spacing formula is undefined there: sin(pi/1) rounds to 1.2e-16, which sends
// the radius to about 1e17. Coordinates that large are spaced further apart
// than the shapes are wide, so anything computed on the ring degenerates.
func TestCalculateRadiusStaysFiniteForOneObject(t *testing.T) {
	obj := boxedObject(0, 0, 100, 60)

	radius := calculateRadius([]*d2graph.Object{obj})

	if radius != MIN_RADIUS {
		t.Fatalf("radius for a single object = %v, want %v", radius, MIN_RADIUS)
	}
}

// A self loop shares one centre between source and destination, so the arc
// geometry degenerates: the sweep is zero and both clipped endpoints land on
// the same point. Routing it as a straight line therefore yields a zero length
// route, which renders as NaN and is rejected by the render target validator.
func TestSelfLoopRouteIsRenderable(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cx, cy, w, h float64
	}{
		{"on the ring", MIN_RADIUS, 0, 100, 60},
		{"above the ring centre", 0, -MIN_RADIUS, 100, 60},
		{"off axis", 141, -141, 100, 60},
		{"tiny shape", MIN_RADIUS, 0, 6, 6},
		{"at the ring centre", 0, 0, 100, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := boxedObject(tc.cx, tc.cy, tc.w, tc.h)
			edge := &d2graph.Edge{Src: obj, Dst: obj}

			createCircularArc(edge, MIN_RADIUS)

			if !edge.IsCurve {
				t.Error("a self loop must be marked as a curve")
			}
			if got, want := len(edge.Route), ARC_STEPS+1; got != want {
				t.Fatalf("route has %d points, want %d", got, want)
			}
			for i, point := range edge.Route {
				if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
					t.Fatalf("route[%d] = (%v, %v), want a finite point", i, point.X, point.Y)
				}
				if i == 0 {
					continue
				}
				if previous := edge.Route[i-1]; previous.X == point.X && previous.Y == point.Y {
					t.Fatalf("route[%d:%d] is a zero length segment at (%v, %v)", i-1, i, point.X, point.Y)
				}
			}

			// A loop squashed onto a line is not a loop. Both sides of its
			// bounding box have to span the loop, which is at least
			// SELF_LOOP_MIN_RADIUS across.
			width, height := routeBounds(edge.Route)
			if width < SELF_LOOP_MIN_RADIUS || height < SELF_LOOP_MIN_RADIUS {
				t.Errorf("loop spans %v x %v, want at least %v on both sides", width, height, SELF_LOOP_MIN_RADIUS)
			}
		})
	}
}

func boxedObject(centerX, centerY, width, height float64) *d2graph.Object {
	return &d2graph.Object{
		Box: geo.NewBox(geo.NewPoint(centerX-width/2, centerY-height/2), width, height),
	}
}

func routeBounds(route []*geo.Point) (width, height float64) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, point := range route {
		minX, maxX = math.Min(minX, point.X), math.Max(maxX, point.X)
		minY, maxY = math.Min(minY, point.Y), math.Max(maxY, point.Y)
	}
	return maxX - minX, maxY - minY
}
