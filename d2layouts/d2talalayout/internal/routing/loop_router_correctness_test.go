package routing

import (
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/loops"
	"github.com/d2lang/d2/lib/geo"
)

func TestRouteLoopsSupportsAllFourArrowCategories(t *testing.T) {
	g := layoutgraph.NewGraph()
	node := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	node.TopLeft = geo.NewPoint(0, 0)

	for _, arrows := range []struct {
		source bool
		target bool
	}{
		{source: true},
		{target: true},
		{source: true, target: true},
		{},
	} {
		edge := g.Connect(node, node)
		if arrows.source {
			edge.SourceArrowhead = layoutgraph.TriangleArrowhead
		}
		if arrows.target {
			edge.TargetArrowhead = layoutgraph.TriangleArrowhead
		}
	}

	routed := loops.Route(node)
	if len(routed) != 4 {
		t.Fatalf("expected four routed loops, got %d", len(routed))
	}
	portPairs := make(map[[4]float64]struct{})
	for _, edge := range routed {
		if len(edge.Points) != 5 {
			t.Fatalf("loop %d has %d route points", edge.IDValue(), len(edge.Points))
		}
		key := [4]float64{
			edge.Points[0].X,
			edge.Points[0].Y,
			edge.Points[len(edge.Points)-1].X,
			edge.Points[len(edge.Points)-1].Y,
		}
		portPairs[key] = struct{}{}
	}
	if len(portPairs) != 4 {
		t.Fatalf("expected four distinct loop port pairs, got %d", len(portPairs))
	}
}
