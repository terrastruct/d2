package routing

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/trees"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func TestTreePathAlignment(t *testing.T) {
	g := layoutgraph.NewGraph()
	n0 := g.AddNode(layoutgraph.NewNode(3607948159, 191, 111))
	n0.TopLeft = geo.NewPoint(1000, 1000)
	n1 := g.AddNode(layoutgraph.NewNode(873436672, 158, 126))
	n1.TopLeft = geo.NewPoint(919, 1211)
	n2 := g.AddNode(layoutgraph.NewNode(2541194078, 130, 126))
	n2.TopLeft = geo.NewPoint(1127, 1211)
	n2.SetShape(shape.STORED_DATA_TYPE)
	n3 := g.AddNode(layoutgraph.NewNode(4207512678, 158, 126))
	n3.TopLeft = geo.NewPoint(919, 1437)
	n4 := g.AddNode(layoutgraph.NewNode(974102386, 100, 100))
	n4.TopLeft = geo.NewPoint(1142, 1450)
	e1 := g.Connect(n0, n1)
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	e2 := g.Connect(n0, n2)
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	e3 := g.Connect(n1, n3)
	e3.TargetArrowhead = layoutgraph.TriangleArrowhead
	e4 := g.Connect(n2, n4)
	e4.TargetArrowhead = layoutgraph.TriangleArrowhead

	ctx := withTestLogger(context.Background(), t)
	for _, node := range g.Nodes {
		g.AddNodeToContainer(nil, node)
	}
	if err := trees.Preprocess(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := trees.Place(ctx, g, nil); err != nil {
		t.Fatal(err)
	}

	ovg, err := buildOVGFromGraph(context.Background(), g, nil)
	if err != nil {
		t.Fatal(err)
	}
	isTreeEdge := g.TreeEdgeMap()
	g.AddIsolatedTreeEdges(isTreeEdge)

	tree := g.NodeToTree[n4]
	route, err := routeSentinelEdge(tree, ovg.Ports, ovg.Centers, nil)
	if err != nil {
		t.Fatalf("Error routing tree edge %v", err)
	}

	if len(ovg.Ports[n4]) != 13 {
		t.Fatalf("Expected 13 port nodes, got %v", len(ovg.Ports[n3]))
	}
	for i := 1; i < len(route.OVGNodes)-3; i++ {
		if route.OVGNodes[i].X != route.OVGNodes[i+1].X {
			t.Fatalf("Expected tree route to be a vertical line, but there is a bend at %d", i)
		}
	}
}
