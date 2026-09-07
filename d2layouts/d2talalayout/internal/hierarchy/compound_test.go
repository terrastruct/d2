package hierarchy

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func compoundTestGraph() (*layoutgraph.Graph, []*layoutgraph.Node, []*layoutgraph.Node) {
	g := layoutgraph.NewGraph()
	outer := make([]*layoutgraph.Node, 6)
	for i := range outer {
		n := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 120, 90)
		n.TopLeft = geo.NewPoint(float64(i)*400, 300)
		g.AddNewNodeToContainer(nil, n)
		outer[i] = n
	}
	for _, pair := range [][2]int{{0, 2}, {0, 3}, {1, 3}, {2, 4}, {3, 5}} {
		connectFixedHierarchyTestEdge(g, outer[pair[0]], outer[pair[1]])
	}
	container := outer[3]
	container.Width = 300
	container.Height = 220
	children := []*layoutgraph.Node{layoutgraph.NewNode(7, 80, 50), layoutgraph.NewNode(8, 80, 50)}
	for i, n := range children {
		n.TopLeft = geo.NewPoint(container.TopLeft.X+40+float64(i)*120, container.TopLeft.Y+60+float64(i)*60)
		g.AddNewNodeToContainer(container, n)
	}
	connectFixedHierarchyTestEdge(g, children[0], children[1])
	// A real boundary edge must retain its descendant endpoint after composition.
	connectFixedHierarchyTestEdge(g, children[1], outer[5])
	g.Directions[container] = geo.Right
	return g, outer, children
}

func TestCompoundPreservesDetailedInteriorsAndOuterDirection(t *testing.T) {
	for _, direction := range []geo.Orientation{geo.Bottom, geo.Top, geo.Left, geo.Right} {
		t.Run(fmt.Sprint(direction), func(t *testing.T) {
			g, outer, children := compoundTestGraph()
			g.Directions[nil] = direction
			inner := newHierarchyWithLevels(map[*layoutgraph.Node]int{children[0]: 0, children[1]: 1})
			children[0].Hierarchy = inner
			children[1].Hierarchy = inner
			offsets := make([]geo.Point, len(children))
			for i, n := range children {
				offsets[i] = geo.Point{X: n.TopLeft.X - outer[3].TopLeft.X, Y: n.TopLeft.Y - outer[3].TopLeft.Y}
			}
			endpoints := make(map[*layoutgraph.Edge][2]*layoutgraph.Node)
			for _, e := range g.Edges {
				endpoints[e] = [2]*layoutgraph.Node{e.From, e.To}
			}
			changed, err := PlaceCompound(t.Context(), g, rand.New(rand.NewSource(1)))
			if err != nil {
				t.Fatal(err)
			}
			if !changed {
				t.Fatal("detailed container did not receive compound layout")
			}
			for i, n := range children {
				got := geo.Point{X: n.TopLeft.X - outer[3].TopLeft.X, Y: n.TopLeft.Y - outer[3].TopLeft.Y}
				if got != offsets[i] || n.Hierarchy != inner || n.HierarchyLevel() != i {
					t.Fatalf("interior geometry or rank changed: %v vs %v", got, offsets[i])
				}
				if !outer[3].Surrounds(n, 0) {
					t.Fatal("child escaped rigid block")
				}
			}
			if g.Direction(outer[3]) != geo.Right {
				t.Fatal("explicit inner direction changed")
			}
			for _, e := range g.Edges {
				if endpoints[e] != [2]*layoutgraph.Node{e.From, e.To} {
					t.Fatal("boundary endpoint was replaced by container")
				}
			}
			for _, pair := range [][2]int{{0, 2}, {0, 3}, {1, 3}, {2, 4}, {3, 5}} {
				a, b := outer[pair[0]], outer[pair[1]]
				valid := false
				switch direction {
				case geo.Bottom:
					valid = a.TopLeft.Y+a.Height < b.TopLeft.Y
				case geo.Top:
					valid = b.TopLeft.Y+b.Height < a.TopLeft.Y
				case geo.Left:
					valid = b.TopLeft.X+b.Width < a.TopLeft.X
				case geo.Right:
					valid = a.TopLeft.X+a.Width < b.TopLeft.X
				}
				if !valid {
					t.Fatalf("outer flow %d -> %d does not honor %v", a.ID, b.ID, direction)
				}
			}
			for i, a := range outer {
				for _, b := range outer[i+1:] {
					if a.DoesOverlapExact(b) {
						t.Fatal("opaque blocks overlap")
					}
				}
			}
		})
	}
}

func TestCompoundKeepsFixedAndDisconnectedGraphsUnchanged(t *testing.T) {
	for _, kind := range []string{"fixed descendant", "disconnected", "no internal edges"} {
		t.Run(kind, func(t *testing.T) {
			g, _, children := compoundTestGraph()
			switch kind {
			case "fixed descendant":
				children[0].FixedTopLeft = children[0].TopLeft.Copy()
			case "disconnected":
				n := layoutgraph.NewNode(20, 50, 50)
				n.TopLeft = geo.NewPoint(4000, 100)
				g.AddNewNodeToContainer(nil, n)
			case "no internal edges":
				for _, e := range slices.Clone(g.Edges) {
					if e.From == children[0] && e.To == children[1] {
						g.Disconnect(e)
					}
				}
			}
			before, err := graphjson.Serialize(t.Context(), g)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := PlaceCompound(t.Context(), g, nil)
			if err != nil {
				t.Fatal(err)
			}
			if changed {
				t.Fatal("unsupported graph was changed")
			}
			after, err := graphjson.Serialize(t.Context(), g)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("skipped candidate mutated graph")
			}
		})
	}
}

type cancelAfterCompoundMove struct {
	context.Context
	node     *layoutgraph.Node
	position geo.Point
}

func (c *cancelAfterCompoundMove) Err() error {
	if *c.node.TopLeft != c.position {
		return context.Canceled
	}
	return c.Context.Err()
}

func TestCompoundCancellationRollsBackGeometryAndMembership(t *testing.T) {
	g, outer, _ := compoundTestGraph()
	before, err := graphjson.Serialize(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	positions := make(map[*layoutgraph.Node]*geo.Point)
	for _, n := range g.Nodes {
		positions[n] = n.TopLeft
	}
	ctx := &cancelAfterCompoundMove{Context: context.Background(), node: outer[3], position: *outer[3].TopLeft}
	changed, err := PlaceCompound(ctx, g, nil)
	if changed || !errors.Is(err, context.Canceled) {
		t.Fatalf("changed=%v error=%v, want cancellation", changed, err)
	}
	after, err := graphjson.Serialize(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("cancellation changed graph state")
	}
	for n, point := range positions {
		if n.TopLeft != point {
			t.Fatal("cancellation replaced coordinate identity")
		}
	}
}

func TestCompoundOrderingUsesBoundaryOffsets(t *testing.T) {
	g := layoutgraph.NewGraph()
	h := layoutgraph.NewHierarchy()
	h.LevelCount = 2
	add := func(id layoutgraph.EntityID, x, y, w float64, level int) *compoundBlock {
		n := layoutgraph.NewNode(id, w, 60)
		n.TopLeft = geo.NewPoint(x, y)
		g.AddNewNodeToContainer(nil, n)
		n.Hierarchy = h
		h.Levels()[n] = level
		return &compoundBlock{proxy: n}
	}
	source := add(1, 0, 0, 200, 0)
	left := add(2, 0, 200, 40, 1)
	right := add(3, 160, 200, 40, 1)
	edges := []compoundInterface{{from: source, to: left, fromOffset: geo.Point{X: 180}, toOffset: geo.Point{X: 20}}, {from: source, to: right, fromOffset: geo.Point{X: 20}, toOffset: geo.Point{X: 20}}}
	guard, err := limits.NewWorkGuard(t.Context(), "compound ordering test", 10000)
	if err != nil {
		t.Fatal(err)
	}
	before, err := compoundInterfaceScore(edges, false, guard)
	if err != nil {
		t.Fatal(err)
	}
	if before.crossings != 1 {
		t.Fatalf("crossings=%d want 1", before.crossings)
	}
	if err := orderCompoundInterfaces([]*compoundBlock{source, left, right}, edges, false, guard); err != nil {
		t.Fatal(err)
	}
	after, err := compoundInterfaceScore(edges, false, guard)
	if err != nil {
		t.Fatal(err)
	}
	if after.crossings != 0 || left.proxy.TopLeft.X <= right.proxy.TopLeft.X {
		t.Fatal("descendant port order did not reorder outer targets")
	}
}
