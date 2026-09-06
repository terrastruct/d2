package d2elklayout

import (
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/lib/geo"
)

func TestDescendantShifterMatchesGraphHelper(t *testing.T) {
	const input = `
outside
c: {
  a
  b: {
    x
    y
    x -> y
  }
  a -> b.x
  b.x -> outside
}
c -> outside
c -> c
c -> c.a
`
	reference := descendantShiftTestGraph(t, input)
	indexed := descendantShiftTestGraph(t, input)
	referenceRoot := reference.Root.Children["c"]
	indexedRoot := indexed.Root.Children["c"]
	shifter := newDescendantShifter(indexed)

	for _, shift := range []struct{ dx, dy float64 }{
		{dx: 18},
		{dx: -7},
		{dy: 13},
		{dy: -5},
	} {
		referenceRoot.ShiftDescendants(shift.dx, shift.dy)
		shifter.shift(indexedRoot, shift.dx, shift.dy)
	}

	if len(reference.Objects) != len(indexed.Objects) || len(reference.Edges) != len(indexed.Edges) {
		t.Fatal("test graphs have different topology")
	}
	for i := range reference.Objects {
		got, want := indexed.Objects[i], reference.Objects[i]
		if *got.TopLeft != *want.TopLeft {
			t.Errorf("object %q top-left = %v, want %v", got.AbsID(), got.TopLeft, want.TopLeft)
		}
	}
	for i := range reference.Edges {
		got, want := indexed.Edges[i], reference.Edges[i]
		if len(got.Route) != len(want.Route) {
			t.Fatalf("edge %q route length = %d, want %d", got.AbsID(), len(got.Route), len(want.Route))
		}
		for j := range got.Route {
			if *got.Route[j] != *want.Route[j] {
				t.Errorf("edge %q point %d = %v, want %v", got.AbsID(), j, got.Route[j], want.Route[j])
			}
		}
	}
}

func descendantShiftTestGraph(t *testing.T, input string) *d2graph.Graph {
	t.Helper()
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, obj := range g.Objects {
		obj.Box = geo.NewBox(geo.NewPoint(float64(i*37), float64(i*29)), 80, 60)
	}
	for i, edge := range g.Edges {
		x := float64(i * 50)
		y := float64(i * 30)
		edge.Route = []*geo.Point{
			geo.NewPoint(x, y),
			geo.NewPoint(x+4, y),
			geo.NewPoint(x+8, y),
			geo.NewPoint(x+8, y+6),
			geo.NewPoint(x+12, y+6),
		}
	}
	return g
}
