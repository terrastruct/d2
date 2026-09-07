package graphbounds

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func newParityGuard(t *testing.T) *limits.WorkGuard {
	t.Helper()
	guard, err := limits.NewWorkGuard(context.Background(), "GraphBoundsParity", 1_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func assertPointParity(t *testing.T, name string, got, want *geo.Point) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
		return
	}
	if got.X != want.X || got.Y != want.Y {
		t.Fatalf("%s = %v, want legacy %v", name, got, want)
	}
}

func assertGeometryParity(t *testing.T, nodes layoutgraph.Nodes) {
	t.Helper()
	guard := newParityGuard(t)
	for i, node := range nodes {
		wantTopLeft, wantBottomRight := node.BoundingBox(nodes)
		gotTopLeft, gotBottomRight, err := NodeBoundingBox(node, nodes, guard)
		if err != nil {
			t.Fatalf("node %d guarded bounding box: %v", i, err)
		}
		assertPointParity(t, "node top-left", gotTopLeft, wantTopLeft)
		assertPointParity(t, "node bottom-right", gotBottomRight, wantBottomRight)
	}
	wantTopLeft, wantBottomRight := nodes.BoundingBox()
	gotTopLeft, gotBottomRight, err := BoundingBox(nodes, guard)
	if err != nil {
		t.Fatal(err)
	}
	assertPointParity(t, "nodes top-left", gotTopLeft, wantTopLeft)
	assertPointParity(t, "nodes bottom-right", gotBottomRight, wantBottomRight)

	wantTopLeft, wantBottomRight = nodes.FixedBoundingBox()
	gotTopLeft, gotBottomRight, err = FixedBoundingBox(nodes, guard)
	if err != nil {
		t.Fatal(err)
	}
	assertPointParity(t, "fixed top-left", gotTopLeft, wantTopLeft)
	assertPointParity(t, "fixed bottom-right", gotBottomRight, wantBottomRight)
}

func TestGuardedGeometryMatchesLegacyBranches(t *testing.T) {
	outsidePositions := []label.Position{
		label.OutsideTopLeft,
		label.OutsideTopCenter,
		label.OutsideTopRight,
		label.OutsideLeftTop,
		label.OutsideLeftMiddle,
		label.OutsideLeftBottom,
		label.OutsideRightTop,
		label.OutsideRightMiddle,
		label.OutsideRightBottom,
		label.OutsideBottomLeft,
		label.OutsideBottomCenter,
		label.OutsideBottomRight,
	}

	t.Run("loops and modifiers", func(t *testing.T) {
		n1 := layoutgraph.NewNode(1, 31.75, 42.25)
		n1.TopLeft = geo.NewPoint(17.5, -11.25)
		n1.LoopOffsets = map[geo.Orientation]float64{
			geo.Top: 3.5, geo.Bottom: 4.25, geo.Left: 5.75, geo.Right: 6.5,
		}
		n1.IsMultiple = true
		n2 := layoutgraph.NewNode(2, 52.5, 27.75)
		n2.TopLeft = geo.NewPoint(89.25, 63.5)
		n2.SetShape(shape.HEXAGON_TYPE)
		n2.Is3D = true
		assertGeometryParity(t, layoutgraph.Nodes{n1, n2})
	})

	for _, position := range outsidePositions {
		t.Run("outside-"+position.String(), func(t *testing.T) {
			target := layoutgraph.NewNode(1, 40, 30)
			target.TopLeft = geo.NewPoint(100, 100)
			target.Label = &layoutgraph.Label{Position: position, Width: 83, Height: 19}
			target.Icon = &layoutgraph.Icon{Position: position}
			moreExtreme := layoutgraph.NewNode(2, 20, 20)
			moreExtreme.TopLeft = geo.NewPoint(20, 20)
			assertGeometryParity(t, layoutgraph.Nodes{target, moreExtreme})
			assertGeometryParity(t, layoutgraph.Nodes{target})
		})
	}

	t.Run("fixed origin", func(t *testing.T) {
		container := layoutgraph.NewNode(10, 500, 400)
		container.TopLeft = geo.NewPoint(0, 0)
		n1 := layoutgraph.NewNode(1, 40, 30)
		n1.TopLeft = geo.NewPoint(80, 30)
		n1.FixedTopLeft = geo.NewPoint(70, 20)
		n1.Container = container
		n2 := layoutgraph.NewNode(2, 35, 50)
		n2.TopLeft = geo.NewPoint(150, 90)
		n2.Container = container
		assertGeometryParity(t, layoutgraph.Nodes{n1, n2})
	})

	t.Run("empty", func(t *testing.T) {
		assertGeometryParity(t, nil)
	})

	t.Run("invalid unplaced node", func(t *testing.T) {
		node := layoutgraph.NewNode(1, 20, 20)
		wantTopLeft, wantBottomRight := layoutgraph.Nodes{node}.BoundingBox()
		if wantTopLeft != nil || wantBottomRight != nil {
			t.Fatalf("legacy unplaced box = %v, %v; want nil, nil", wantTopLeft, wantBottomRight)
		}
		_, _, err := BoundingBox(layoutgraph.Nodes{node}, newParityGuard(t))
		if err == nil || !strings.Contains(err.Error(), "BinPack bounding box contains an unplaced node") {
			t.Fatalf("unplaced-node error = %v", err)
		}
	})

	t.Run("container ancestry cycle", func(t *testing.T) {
		a := layoutgraph.NewNode(1, 20, 20)
		b := layoutgraph.NewNode(2, 20, 20)
		a.TopLeft = geo.NewPoint(0, 0)
		b.TopLeft = geo.NewPoint(30, 0)
		a.Container = b
		b.Container = a
		_, _, err := FixedBoundingBox(layoutgraph.Nodes{a}, newParityGuard(t))
		if err == nil || !strings.Contains(err.Error(), "BinPack found a cycle in container ancestry") {
			t.Fatalf("container-cycle error = %v", err)
		}
	})
}

func TestGuardedGeometryRandomizedLegacyParity(t *testing.T) {
	rng := rand.New(rand.NewSource(0xB1A5))
	outsidePositions := []label.Position{
		label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight,
		label.OutsideLeftTop, label.OutsideLeftMiddle, label.OutsideLeftBottom,
		label.OutsideRightTop, label.OutsideRightMiddle, label.OutsideRightBottom,
		label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight,
	}
	for iteration := 0; iteration < 300; iteration++ {
		count := 1 + rng.Intn(8)
		nodes := make(layoutgraph.Nodes, 0, count)
		var container *layoutgraph.Node
		if rng.Intn(2) == 0 {
			container = layoutgraph.NewNode(10_000+layoutgraph.EntityID(iteration), 1_000, 1_000)
			container.TopLeft = geo.NewPoint(-100, -100)
		}
		for i := 0; i < count; i++ {
			node := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 5+rng.Float64()*150, 5+rng.Float64()*150)
			node.TopLeft = geo.NewPoint(-300+rng.Float64()*600, -300+rng.Float64()*600)
			node.Container = container
			node.LoopOffsets = map[geo.Orientation]float64{
				geo.Top: rng.Float64() * 30, geo.Bottom: rng.Float64() * 30,
				geo.Left: rng.Float64() * 30, geo.Right: rng.Float64() * 30,
			}
			switch rng.Intn(5) {
			case 0:
				node.Is3D = true
			case 1:
				node.SetShape(shape.HEXAGON_TYPE)
				node.Is3D = true
			case 2:
				node.IsMultiple = true
			}
			if rng.Intn(3) == 0 {
				node.Label = &layoutgraph.Label{
					Position: outsidePositions[rng.Intn(len(outsidePositions))],
					Width:    1 + rng.Float64()*180,
					Height:   1 + rng.Float64()*80,
				}
			}
			if rng.Intn(4) == 0 {
				node.Icon = &layoutgraph.Icon{Position: outsidePositions[rng.Intn(len(outsidePositions))]}
			}
			if rng.Intn(7) == 0 {
				node.FixedTopLeft = geo.NewPoint(
					node.TopLeft.X-50+rng.Float64()*100,
					node.TopLeft.Y-50+rng.Float64()*100,
				)
			}
			nodes = append(nodes, node)
		}
		t.Run("iteration", func(t *testing.T) {
			assertGeometryParity(t, nodes)
		})
	}
}

var errInjectedWorkLimit = errors.New("injected graph-bounds work limit")

type countingGuard struct {
	used     int
	limit    int
	finishes int
}

func (guard *countingGuard) Step() error {
	guard.used++
	if guard.limit > 0 && guard.used > guard.limit {
		return errInjectedWorkLimit
	}
	return nil
}

func (guard *countingGuard) Finish() error {
	guard.finishes++
	return nil
}

func TestFixedBoundsPreserveAggregateWorkAccounting(t *testing.T) {
	a := layoutgraph.NewNode(1, 20, 10)
	b := layoutgraph.NewNode(2, 30, 15)
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(40, 20)
	nodes := layoutgraph.Nodes{a, b}

	guard := &countingGuard{}
	if _, _, err := FixedBoundingBox(nodes, guard); err != nil {
		t.Fatal(err)
	}
	if guard.used != 5 || guard.finishes != 3 {
		t.Fatalf("fixed bounds accounting = %d steps, %d finishes; want 5, 3", guard.used, guard.finishes)
	}

	labelNode := layoutgraph.NewNode(3, 40, 30)
	labelNode.TopLeft = geo.NewPoint(100, 100)
	labelNode.Label = &layoutgraph.Label{Position: label.OutsideLeftMiddle, Width: 80, Height: 10}
	other := layoutgraph.NewNode(4, 20, 20)
	other.TopLeft = geo.NewPoint(20, 100)
	limited := &countingGuard{limit: 2}
	if _, _, err := NodeBoundingBox(labelNode, layoutgraph.Nodes{labelNode, other}, limited); !errors.Is(err, errInjectedWorkLimit) {
		t.Fatalf("outside-label scan error = %v, want injected work limit", err)
	}
	if limited.used != 3 {
		t.Fatalf("outside-label scan stopped at %d steps, want first rejected step 3", limited.used)
	}
}
