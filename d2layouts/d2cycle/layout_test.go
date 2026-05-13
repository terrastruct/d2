package d2cycle

import (
	"context"
	"math"
	"strings"
	"testing"

	"oss.terrastruct.com/d2/d2compiler"
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
