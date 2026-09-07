package routing

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestRouteEdges(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 8, 8)
	a.TopLeft = geo.NewPoint(4, 4)
	b := layoutgraph.NewNode(2, 8, 8)
	b.TopLeft = geo.NewPoint(16, 16)
	c := layoutgraph.NewNode(3, 8, 8)
	c.TopLeft = geo.NewPoint(2, 18)
	d := layoutgraph.NewNode(4, 12, 8)
	d.TopLeft = geo.NewPoint(20, 6)
	e := layoutgraph.NewNode(5, 4, 16)
	e.TopLeft = geo.NewPoint(26, 16)
	for _, node := range []*layoutgraph.Node{a, b, c, d, e} {
		graph.AddNode(node)
	}

	graph.Connect(a, b)
	graph.Connect(a, c)
	graph.Connect(a, d)
	graph.Connect(b, c)
	graph.Connect(b, e)
	graph.Connect(a, e)

	if err := RouteEdges(ctx, graph, nil); err != nil {
		t.Fatal(err)
	}
}

func TestHasSharpAngleToBorder(t *testing.T) {
	if hasSharpAngleToBorder(geo.NewPoint(0, 0), geo.NewPoint(20, 20), geo.Right) {
		t.Fatal("45-degree approach to right border reported sharp")
	}
	if !hasSharpAngleToBorder(geo.NewPoint(0, 10), geo.NewPoint(20, 0), geo.Top) {
		t.Fatal("26.6-degree left-to-right approach to top border was not reported sharp")
	}
	if !hasSharpAngleToBorder(geo.NewPoint(20, 10), geo.NewPoint(0, 0), geo.Top) {
		t.Fatal("26.6-degree right-to-left approach to top border was not reported sharp")
	}
}
