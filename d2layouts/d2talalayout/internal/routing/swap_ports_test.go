package routing

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// swapEdgePorts swaps edge ports to reduce crossings.
//  1. group edges by orientation at `n`
//  2. sort edges
//  3. if a sequential pair of edges cross, try to swap them
//     edges can't be swapped if the end result passes through another node
//     if the ports have more than one edge and the arrowheads don't match
//
// .       input                       output
// .       ┌──────────                ┌────────────
// .    ┌──┼─────────────             |  ┌─────────────
// . ┌──┴──┴──────┐                ┌──┴──┴──────┐
// . │            ├───┐            │            ├───────┐
// . │    n       ├───┼───┐        │     n      ├───┐   │
// . └────────────┘   │   │        └────────────┘   │   │
func swapEdgePorts(ctx context.Context, n *layoutgraph.Node) (swapped bool, err error) {
	if n == nil || n.Graph == nil {
		return false, fmt.Errorf("TALA SwapEdgePorts requires a graph node")
	}
	err = runAtomicRouteStage(ctx, "SwapEdgePorts", n.Graph, n.Edges, maxRouteStageWorkUnits, func(guard *routeWorkGuard) error {
		var err error
		swapped, err = swapEdgePortsGuarded(n, guard)
		return err
	})
	if err != nil {
		return false, err
	}
	return swapped, err
}

func TestSwapEdgePorts(t *testing.T) {
	//                        ┌─────┐
	//                  ┌─────► n2  │
	//                  e1    └───┬─┘
	//             ┌────┼─────e2──┘
	// ┌───────────┼────┼─────────┐
	// │  n1     ┌─▼────┴─┐       │
	// │         │   n3   │       │
	// │         └────────┘       │
	// └──────────────────────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 200, 100))
	n1.TopLeft = geo.NewPoint(0, 150)

	n2 := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	n2.TopLeft = geo.NewPoint(200, 0)

	n3 := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	n3.TopLeft = geo.NewPoint(100, 175)

	e1 := g.Connect(n3, n2)
	e1.Points = []*geo.Point{
		geo.NewPoint(math.Round(n3.TopLeft.X+0.75*n3.Width), n3.TopLeft.Y),
		geo.NewPoint(math.Round(n3.TopLeft.X+0.75*n3.Width), n2.Center().Y),
		geo.NewPoint(n2.TopLeft.X, n2.Center().Y),
	}
	e2 := g.Connect(n2, n3)
	e2.Points = []*geo.Point{
		geo.NewPoint(n2.Center().X, n2.TopLeft.Y+n2.Height),
		geo.NewPoint(n2.Center().X, n2.TopLeft.Y+n2.Height+25),
		geo.NewPoint(math.Round(n3.TopLeft.X+0.25*n3.Width), n2.TopLeft.Y+n2.Height+25),
		geo.NewPoint(math.Round(n3.TopLeft.X+0.25*n3.Width), n3.TopLeft.Y),
	}

	g.AddNodeToContainer(nil, n1)
	g.AddNodeToContainer(nil, n2)
	g.AddNodeToContainer(n1, n3)

	if _, err := swapEdgePorts(context.Background(), n3); err != nil {
		t.Fatal(err)
	}

	e1Expected := []*geo.Point{
		geo.NewPoint(math.Round(n3.TopLeft.X+0.25*n3.Width), n3.TopLeft.Y),
		geo.NewPoint(math.Round(n3.TopLeft.X+0.25*n3.Width), n2.Center().Y),
		geo.NewPoint(n2.TopLeft.X, n2.Center().Y),
	}
	for i, expected := range e1Expected {
		if !expected.Equals(e1.Points[i]) {
			t.Fatalf("expected %+v at e1.Points[%d], got %+v", expected, i, e1.Points[i])
		}
	}

	e2Expected := []*geo.Point{
		geo.NewPoint(n2.Center().X, n2.TopLeft.Y+n2.Height),
		geo.NewPoint(n2.Center().X, n2.TopLeft.Y+n2.Height+25),
		geo.NewPoint(math.Round(n3.TopLeft.X+0.75*n3.Width), n2.TopLeft.Y+n2.Height+25),
		geo.NewPoint(math.Round(n3.TopLeft.X+0.75*n3.Width), n3.TopLeft.Y),
	}
	for i, expected := range e2Expected {
		if !expected.Equals(e2.Points[i]) {
			t.Fatalf("expected %+v at e2.Points[%d], got %+v", expected, i, e2.Points[i])
		}
	}
}

func TestInvalidSwapNodeOverlap(t *testing.T) {
	// `e2` and `e1` can't be swapped because `e1` would overlap with `n1`
	//      ┌────────┐          ┌─────┐
	//      │   n1   │    ┌─────► n2  │
	//      └──────▲─┘    e1    └───┬─┘
	//          ┌──┼──────┼─────e2──┘
	// ┌────────▼──┴──────┴───────┐
	// │                          │
	// │            n3            │
	// │                          │
	// └──────────────────────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 50))
	n1.TopLeft = geo.NewPoint(50, 0)

	n2 := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	n2.TopLeft = geo.NewPoint(200, 0)

	n3 := g.AddNode(layoutgraph.NewNode(2, 200, 100))
	n3.TopLeft = geo.NewPoint(0, 150)

	e1 := g.Connect(n3, n2)
	e1.Points = []*geo.Point{
		geo.NewPoint(175, n3.TopLeft.Y),
		geo.NewPoint(175, n2.Center().Y),
		geo.NewPoint(n2.TopLeft.X, n2.Center().Y),
	}
	e2 := g.Connect(n2, n3)
	e2.Points = []*geo.Point{
		geo.NewPoint(225, n2.TopLeft.Y+n2.Height),
		geo.NewPoint(225, n2.TopLeft.Y+n2.Height+25),
		geo.NewPoint(75, n2.TopLeft.Y+n2.Height+25),
		geo.NewPoint(75, n3.TopLeft.Y),
	}
	e3 := g.Connect(n3, n1)
	e3.Points = []*geo.Point{
		geo.NewPoint(125, n3.TopLeft.Y),
		geo.NewPoint(125, n1.TopLeft.Y+n1.Height),
	}

	var e1Expected, e2Expected, e3Expected []*geo.Point
	for _, p := range e1.Points {
		e1Expected = append(e1Expected, p.Copy())
	}
	for _, p := range e2.Points {
		e2Expected = append(e2Expected, p.Copy())
	}
	for _, p := range e3.Points {
		e3Expected = append(e3Expected, p.Copy())
	}

	if _, err := swapEdgePorts(context.Background(), n3); err != nil {
		t.Fatal(err)
	}

	for i, expected := range e1Expected {
		if !expected.Equals(e1.Points[i]) {
			t.Fatalf("expected %+v at e1.Points[%d], got %+v", expected, i, e1.Points[i])
		}
	}
	for i, expected := range e2Expected {
		if !expected.Equals(e2.Points[i]) {
			t.Fatalf("expected %+v at e2.Points[%d], got %+v", expected, i, e2.Points[i])
		}
	}
	for i, expected := range e3Expected {
		if !expected.Equals(e3.Points[i]) {
			t.Fatalf("expected %+v at e3.Points[%d], got %+v", expected, i, e3.Points[i])
		}
	}
}

func TestInvalidSwapArrowhead(t *testing.T) {
	// `e2` and `e1` can't be swapped because of the arrowheads
	//  ┌────────┐              ┌─────┐
	//  │   n1   ◄─┐      ┌─────► n2  │
	//  └────────┘ |      e1    └───┬─┘
	//             ├──────┼─────e2──┘
	// ┌───────────▼──────┴───────┐
	// │                          │
	// │            n3            │
	// │                          │
	// └──────────────────────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	n1.TopLeft = geo.NewPoint(10, 0)

	n2 := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	n2.TopLeft = geo.NewPoint(200, 0)

	n3 := g.AddNode(layoutgraph.NewNode(2, 200, 100))
	n3.TopLeft = geo.NewPoint(0, 150)

	e1 := g.Connect(n3, n2)
	e1.SourceArrowhead = layoutgraph.NoArrowhead
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	e1.Points = []*geo.Point{
		geo.NewPoint(175, n3.TopLeft.Y),
		geo.NewPoint(175, n2.Center().Y),
		geo.NewPoint(n2.TopLeft.X, n2.Center().Y),
	}
	e2 := g.Connect(n2, n3)
	e2.SourceArrowhead = layoutgraph.NoArrowhead
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	e2.Points = []*geo.Point{
		geo.NewPoint(225, n2.TopLeft.Y+n2.Height),
		geo.NewPoint(225, n2.TopLeft.Y+n2.Height+25),
		geo.NewPoint(125, n2.TopLeft.Y+n2.Height+25),
		geo.NewPoint(125, n3.TopLeft.Y),
	}
	e3 := g.Connect(n3, n1)
	e3.SourceArrowhead = layoutgraph.TriangleArrowhead
	e3.TargetArrowhead = layoutgraph.TriangleArrowhead
	e3.Points = []*geo.Point{
		geo.NewPoint(125, n3.TopLeft.Y),
		geo.NewPoint(125, n1.Center().Y),
		geo.NewPoint(n1.TopLeft.X+n1.Width, n1.Center().Y),
	}

	var e1Expected, e2Expected, e3Expected []*geo.Point
	for _, p := range e1.Points {
		e1Expected = append(e1Expected, p.Copy())
	}
	for _, p := range e2.Points {
		e2Expected = append(e2Expected, p.Copy())
	}
	for _, p := range e3.Points {
		e3Expected = append(e3Expected, p.Copy())
	}

	if _, err := swapEdgePorts(context.Background(), n3); err != nil {
		t.Fatal(err)
	}

	for i, expected := range e1Expected {
		if !expected.Equals(e1.Points[i]) {
			t.Fatalf("expected %+v at e1.Points[%d], got %+v", expected, i, e1.Points[i])
		}
	}
	for i, expected := range e2Expected {
		if !expected.Equals(e2.Points[i]) {
			t.Fatalf("expected %+v at e2.Points[%d], got %+v", expected, i, e2.Points[i])
		}
	}
	for i, expected := range e3Expected {
		if !expected.Equals(e3.Points[i]) {
			t.Fatalf("expected %+v at e3.Points[%d], got %+v", expected, i, e3.Points[i])
		}
	}
}
