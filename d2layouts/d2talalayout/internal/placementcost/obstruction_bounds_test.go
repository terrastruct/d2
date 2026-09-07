package placementcost

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func obstructionBoundsNode(x, y, width, height float64) *layoutgraph.Node {
	return &layoutgraph.Node{Box: geo.Box{TopLeft: geo.NewPoint(x, y), Width: width, Height: height}}
}

func TestObstructionBoundsStrictSeparation(t *testing.T) {
	bounds := scoringNodeBounds(obstructionBoundsNode(30, 40, -20, -20))
	for _, tc := range []struct {
		name     string
		obstacle *layoutgraph.Node
		excluded bool
	}{
		{"left", obstructionBoundsNode(0, 25, 9, 5), true},
		{"right", obstructionBoundsNode(31, 25, 9, 5), true},
		{"above", obstructionBoundsNode(15, 10, 5, 9), true},
		{"below", obstructionBoundsNode(15, 41, 5, 9), true},
		{"negative_width_left", obstructionBoundsNode(9, 25, -5, 5), true},
		{"negative_height_above", obstructionBoundsNode(15, 19, 5, -5), true},
		{"touch_left", obstructionBoundsNode(0, 25, 10, 5), false},
		{"touch_right", obstructionBoundsNode(30, 25, 9, 5), false},
		{"touch_top", obstructionBoundsNode(15, 10, 5, 10), false},
		{"touch_bottom", obstructionBoundsNode(15, 40, 5, 10), false},
		{"enclosing", obstructionBoundsNode(0, 0, 100, 100), false},
		{"inside", obstructionBoundsNode(15, 25, 5, 5), false},
		{"point_inside", obstructionBoundsNode(20, 30, 0, 0), false},
		{"point_outside", obstructionBoundsNode(31, 30, 0, 0), true},
		{"nil", nil, false},
		{"unpositioned", &layoutgraph.Node{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := bounds.excludes(tc.obstacle); got != tc.excluded {
				t.Fatalf("excluded=%v, want %v", got, tc.excluded)
			}
		})
	}
}

func TestObstructionBoundsFallback(t *testing.T) {
	ordinary := obstructionBoundsNode(10, 20, 30, 40)
	fallbacks := []*layoutgraph.Node{
		nil,
		{},
		obstructionBoundsNode(math.NaN(), 0, 1, 1),
		obstructionBoundsNode(0, math.NaN(), 1, 1),
		obstructionBoundsNode(0, 0, math.NaN(), 1),
		obstructionBoundsNode(0, 0, 1, math.NaN()),
		obstructionBoundsNode(math.Inf(1), 0, 1, 1),
		obstructionBoundsNode(0, math.Inf(-1), 1, 1),
		obstructionBoundsNode(0, 0, math.Inf(1), 1),
		obstructionBoundsNode(0, 0, 1, math.Inf(-1)),
		obstructionBoundsNode(math.MaxFloat64, 0, math.MaxFloat64, 1),
		obstructionBoundsNode(math.MaxFloat64/4, 0, 1, 1),
	}
	for i, node := range fallbacks {
		bounds := scoringNodeBounds(node)
		if bounds.usable || bounds.excludes(ordinary) || bounds.including(scoringNodeBounds(ordinary)).excludes(ordinary) {
			t.Fatalf("fallback %d enabled rejection", i)
		}
		// Finite but very large obstacles can still be rejected by a small,
		// usable envelope; the fallback is required for non-finite obstacles.
		if i < len(fallbacks)-1 && scoringNodeBounds(ordinary).excludes(node) {
			t.Fatalf("non-finite obstacle %d was rejected", i)
		}
	}
	zero := scoringNodeBounds(obstructionBoundsNode(5, 5, 0, 0))
	if !zero.usable || zero.excludes(obstructionBoundsNode(5, 5, 0, 0)) || !zero.excludes(obstructionBoundsNode(6, 5, 0, 0)) {
		t.Fatal("degenerate point envelope did not preserve touching geometry")
	}
}

func TestObstructionBoundsGeneratedRouteParity(t *testing.T) {
	rng := rand.New(rand.NewPCG(275, 637))
	for trial := range 1000 {
		makeNode := func() *layoutgraph.Node {
			return obstructionBoundsNode(
				float64(rng.IntN(1600)-800)/4,
				float64(rng.IntN(1600)-800)/4,
				float64(rng.IntN(800)-400)/4,
				float64(rng.IntN(800)-400)/4,
			)
		}
		a, b, center := makeNode(), makeNode(), makeNode()
		bounds := scoringNodeBounds(a).including(scoringNodeBounds(b)).including(scoringNodeBounds(center))
		// Placement's direct/L routes combine endpoint centers and boundaries.
		// Its alternate routes also use midpoints between the two left/top or
		// right/bottom boundaries. Cover every combination of those coordinates,
		// including negative dimensions and degenerate boxes.
		axisCoordinates := func(startA, sizeA, startB, sizeB, startCenter, sizeCenter float64) []float64 {
			endA, endB := startA+sizeA, startB+sizeB
			lower := min(startA, startB)
			upper := max(endA, endB)
			return []float64{
				startA, endA, startA + sizeA/2,
				startB, endB, startB + sizeB/2,
				startCenter, startCenter + sizeCenter, startCenter + sizeCenter/2,
				lower + math.Abs(max(startA, startB)-lower)/2,
				upper - math.Abs(min(endA, endB)-upper)/2,
			}
		}
		xs := axisCoordinates(a.TopLeft.X, a.Width, b.TopLeft.X, b.Width, center.TopLeft.X, center.Width)
		ys := axisCoordinates(a.TopLeft.Y, a.Height, b.TopLeft.Y, b.Height, center.TopLeft.Y, center.Height)
		for _, x := range xs {
			for _, y := range ys {
				if x < bounds.left || x > bounds.right || y < bounds.top || y > bounds.bottom {
					t.Fatalf("trial %d generated point (%v,%v) outside %+v", trial, x, y, bounds)
				}
			}
		}
		obstacles := []*layoutgraph.Node{
			obstructionBoundsNode(bounds.left-10, bounds.top, 9, bounds.bottom-bounds.top),
			obstructionBoundsNode(bounds.right+10, bounds.top, -9, bounds.bottom-bounds.top),
			obstructionBoundsNode(bounds.left, bounds.top-10, bounds.right-bounds.left, 9),
			obstructionBoundsNode(bounds.left, bounds.bottom+10, bounds.right-bounds.left, -9),
		}
		for _, obstacle := range obstacles {
			if !bounds.excludes(obstacle) {
				t.Fatalf("trial %d did not reject disjoint obstacle", trial)
			}
			for range 32 {
				first := geo.NewPoint(xs[rng.IntN(len(xs))], ys[rng.IntN(len(ys))])
				second := geo.NewPoint(xs[rng.IntN(len(xs))], ys[rng.IntN(len(ys))])
				if obstacle.PassesThrough(first, second) {
					t.Fatalf("trial %d rejected obstacle blocking %v -> %v", trial, first, second)
				}
			}
		}
	}
}
