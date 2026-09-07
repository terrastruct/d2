package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func selectedCompoundTestGraph() (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Node) {
	g := layoutgraph.NewGraph()
	roots := make([]*layoutgraph.Node, 4)
	for i := range roots {
		n := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 100, 80)
		n.TopLeft = geo.NewPoint(float64(i)*400, 0)
		g.AddNewNodeToContainer(nil, n)
		roots[i] = n
	}
	container := roots[1]
	container.Width, container.Height = 300, 200
	first, second := layoutgraph.NewNode(5, 60, 40), layoutgraph.NewNode(6, 60, 40)
	first.TopLeft = geo.NewPoint(460, 60)
	second.TopLeft = geo.NewPoint(580, 100)
	g.AddNewNodeToContainer(container, first)
	g.AddNewNodeToContainer(container, second)
	g.Directions[container] = geo.Right
	for _, ends := range [][2]*layoutgraph.Node{{roots[0], first}, {first, second}, {second, roots[2]}, {roots[0], roots[3]}, {roots[3], roots[2]}} {
		e := g.Connect(ends[0], ends[1])
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		e.Points = []*geo.Point{e.From.Center(), e.To.Center()}
	}
	return g, container, first
}

func TestCompoundCandidateReroutesAnIsolatedSelectedDrawing(t *testing.T) {
	g, container, child := selectedCompoundTestGraph()
	before, err := layoutgraph.Clone(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	offset := geo.Point{X: child.TopLeft.X - container.TopLeft.X, Y: child.TopLeft.Y - container.TopLeft.Y}
	candidate, err := CompoundCandidate(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	if candidate == g {
		t.Fatal("eligible selected drawing did not produce a compound trial")
	}
	requireGraphsSerializeEqual(t.Context(), t, before, g)
	var movedContainer, movedChild *layoutgraph.Node
	for _, n := range candidate.Nodes {
		if n.ID == container.ID {
			movedContainer = n
		}
		if n.ID == child.ID {
			movedChild = n
		}
	}
	if movedContainer == nil || movedChild == nil {
		t.Fatal("candidate lost nodes")
	}
	got := geo.Point{X: movedChild.TopLeft.X - movedContainer.TopLeft.X, Y: movedChild.TopLeft.Y - movedContainer.TopLeft.Y}
	if got != offset || candidate.Direction(movedContainer) != geo.Right {
		t.Fatal("compound rerouting changed the authored interior")
	}
	for _, e := range candidate.Edges {
		if len(e.Points) < 2 || !e.From.ContainsPointOnBox(e.Points[0]) || !e.To.ContainsPointOnBox(e.Points[len(e.Points)-1]) {
			t.Fatal("candidate did not rebuild complete endpoint-attached routes")
		}
	}
	again, err := CompoundCandidate(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	requireGraphsSerializeEqual(t.Context(), t, candidate, again)
}

func TestCompoundCandidateSkipsFixedAndOrdinaryDrawings(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		g, _, child := selectedCompoundTestGraph()
		if fixed {
			child.FixedTopLeft = child.TopLeft.Copy()
		} else {
			g = layoutgraph.NewGraph()
			for i := 0; i < 4; i++ {
				n := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 100, 80)
				n.TopLeft = geo.NewPoint(float64(i)*200, 0)
				g.AddNewNodeToContainer(nil, n)
			}
		}
		candidate, err := CompoundCandidate(t.Context(), g)
		if err != nil || candidate != g {
			t.Fatalf("inapplicable trial should retain incumbent pointer: fixed=%v err=%v", fixed, err)
		}
	}
}

func TestCompoundCandidateCancellationPreservesInput(t *testing.T) {
	g, _, _ := selectedCompoundTestGraph()
	before, err := layoutgraph.Clone(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := CompoundCandidate(ctx, g); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
	requireGraphsSerializeEqual(t.Context(), t, before, g)
}
