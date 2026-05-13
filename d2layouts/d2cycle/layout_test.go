package d2cycle

import (
	"context"
	"math"
	"strings"
	"testing"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
)

func TestCycleEdgesStartOnNonRectangularShapeBorder(t *testing.T) {
	g, _, err := d2compiler.Compile("", strings.NewReader(`
shape: cycle
a.shape: circle
a -> b -> c
`), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, obj := range g.Objects {
		obj.Box = geo.NewBox(geo.NewPoint(0, 0), 100, 100)
	}

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	var edgeStart *geo.Point
	var srcCenter *geo.Point
	for _, edge := range g.Edges {
		if edge.Src.ID == "a" {
			edgeStart = edge.Route[0]
			srcCenter = edge.Src.Center()
			break
		}
	}
	if edgeStart == nil {
		t.Fatal("expected edge from a")
	}

	got := geo.EuclideanDistance(srcCenter.X, srcCenter.Y, edgeStart.X, edgeStart.Y)
	if math.Abs(got-50) > 0.5 {
		t.Fatalf("expected cycle edge to start on circle border radius 50, got %.2f", got)
	}
}

func TestSingleObjectCycleIsCentered(t *testing.T) {
	g, _, err := d2compiler.Compile("", strings.NewReader(`
shape: cycle
a
`), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, obj := range g.Objects {
		obj.Box = geo.NewBox(geo.NewPoint(0, 0), 100, 100)
	}

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	a := g.Root.ChildrenArray[0]
	if math.Abs(a.Center().X) > 0.001 || math.Abs(a.Center().Y) > 0.001 {
		t.Fatalf("expected single cycle object centered at origin, got %v", a.Center())
	}
}

func TestCycleLayoutUsesCoreLayoutForNestedContent(t *testing.T) {
	g, _, err := d2compiler.Compile("", strings.NewReader(`
shape: cycle
a: {
  x
  y
  x -> y
}
b
a -> b
`), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, obj := range g.Objects {
		obj.Box = geo.NewBox(geo.NewPoint(0, 0), 100, 100)
	}

	layoutCalled := false
	layout := func(ctx context.Context, g *d2graph.Graph) error {
		layoutCalled = true
		for _, obj := range g.Objects {
			switch obj.ID {
			case "x":
				obj.TopLeft = geo.NewPoint(20, 30)
			case "y":
				obj.TopLeft = geo.NewPoint(160, 30)
			}
		}
		for _, edge := range g.Edges {
			edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
		}
		return nil
	}

	if err := Layout(context.Background(), g, layout); err != nil {
		t.Fatal(err)
	}
	if !layoutCalled {
		t.Fatal("expected cycle layout to call provided layout function")
	}

	var x, y *d2graph.Object
	var internalEdge *d2graph.Edge
	for _, obj := range g.Objects {
		switch obj.ID {
		case "x":
			x = obj
		case "y":
			y = obj
		}
	}
	for _, edge := range g.Edges {
		if edge.Src == x && edge.Dst == y {
			internalEdge = edge
			break
		}
	}
	if x == nil || y == nil || internalEdge == nil {
		t.Fatal("expected nested objects and internal edge")
	}
	if x.TopLeft.X == y.TopLeft.X && x.TopLeft.Y == y.TopLeft.Y {
		t.Fatal("expected nested objects to keep distinct core-layout positions")
	}
	if internalEdge.IsCurve {
		t.Fatal("expected internal nested edge to keep core-layout route instead of cycle arc")
	}
	if got := len(internalEdge.Route); got != 2 {
		t.Fatalf("expected internal nested edge route from core layout, got %d points", got)
	}
}
