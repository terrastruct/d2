package d2sequence_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2sequence"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/shape"
)

func TestBasicSequenceDiagram(t *testing.T) {
	// ┌────────┐              ┌────────┐
	// │   n1   │              │   n2   │
	// └────┬───┘              └────┬───┘
	//      │                       │
	//      ├───────────────────────►
	//      │                       │
	//      ◄───────────────────────┤
	//      │                       │
	//      ├───────────────────────►
	//      │                       │
	//      ◄───────────────────────┤
	//      │                       │
	input := `
shape: sequence_diagram
n1 -> n2: left to right
n2 -> n1: right to left
n1 -> n2
n2 -> n1
`
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	assert.Nil(t, err)

	n1, has := g.Root.HasChild([]string{"n1"})
	assert.True(t, has)
	n2, has := g.Root.HasChild([]string{"n2"})
	assert.True(t, has)

	n1.Box = geo.NewBox(nil, 100, 100)
	n2.Box = geo.NewBox(nil, 30, 30)

	nEdges := len(g.Edges)

	ctx := log.WithTB(context.Background(), t)
	d2sequence.Layout(ctx, g, func(ctx context.Context, g *d2graph.Graph) error {
		// just set some position as if it had been properly placed
		for _, obj := range g.Objects {
			obj.TopLeft = geo.NewPoint(0, 0)
		}

		for _, edge := range g.Edges {
			edge.Route = []*geo.Point{geo.NewPoint(1, 1)}
		}
		return nil
	})

	// asserts that actors were placed in the expected x order and at y=0
	actors := []*d2graph.Object{
		g.Objects[0],
		g.Objects[1],
	}
	for i := 1; i < len(actors); i++ {
		if actors[i].TopLeft.X < actors[i-1].TopLeft.X {
			t.Fatalf("expected actor[%d].TopLeft.X > actor[%d].TopLeft.X", i, i-1)
		}
		actorBottom := actors[i].TopLeft.Y + actors[i].Height
		prevActorBottom := actors[i-1].TopLeft.Y + actors[i-1].Height
		if actorBottom != prevActorBottom {
			t.Fatalf("expected actor[%d] and actor[%d] to be at the same bottom y", i, i-1)
		}
	}

	nExpectedEdges := nEdges + len(actors)
	if len(g.Edges) != nExpectedEdges {
		t.Fatalf("expected %d edges, got %d", nExpectedEdges, len(g.Edges))
	}

	// assert that edges were placed in y order and have the endpoints at their actors
	// uses `nEdges` because Layout creates some vertical edges to represent the actor lifeline
	for i := 0; i < nEdges; i++ {
		edge := g.Edges[i]
		if len(edge.Route) != 2 {
			t.Fatalf("expected edge[%d] to have only 2 points", i)
		}
		if edge.Route[0].Y != edge.Route[1].Y {
			t.Fatalf("expected edge[%d] to be a horizontal line", i)
		}
		if edge.Src.TopLeft.X < edge.Dst.TopLeft.X {
			// left to right
			if edge.Route[0].X != edge.Src.Center().X {
				t.Fatalf("expected edge[%d] x to be at the actor center", i)
			}

			if edge.Route[1].X != edge.Dst.Center().X {
				t.Fatalf("expected edge[%d] x to be at the actor center", i)
			}
		} else {
			if edge.Route[0].X != edge.Src.Center().X {
				t.Fatalf("expected edge[%d] x to be at the actor center", i)
			}

			if edge.Route[1].X != edge.Dst.Center().X {
				t.Fatalf("expected edge[%d] x to be at the actor center", i)
			}
		}
		if i > 0 {
			prevEdge := g.Edges[i-1]
			if edge.Route[0].Y < prevEdge.Route[0].Y {
				t.Fatalf("expected edge[%d].TopLeft.Y > edge[%d].TopLeft.Y", i, i-1)
			}
		}
	}

	lastSequenceEdge := g.Edges[nEdges-1]
	for i := nEdges; i < nExpectedEdges; i++ {
		edge := g.Edges[i]
		if len(edge.Route) != 2 {
			t.Fatalf("expected lifeline edge[%d] to have only 2 points", i)
		}
		if edge.Route[0].X != edge.Route[1].X {
			t.Fatalf("expected lifeline edge[%d] to be a vertical line", i)
		}
		if edge.Route[0].X != edge.Src.Center().X {
			t.Fatalf("expected lifeline edge[%d] x to be at the actor center", i)
		}
		if edge.Route[0].Y != edge.Src.Height+edge.Src.TopLeft.Y {
			t.Fatalf("expected lifeline edge[%d] to start at the bottom of the source actor", i)
		}
		if edge.Route[1].Y < lastSequenceEdge.Route[0].Y {
			t.Fatalf("expected lifeline edge[%d] to end after the last sequence edge", i)
		}
	}

	// check label positions
	if *g.Edges[0].LabelPosition != label.InsideMiddleCenter.String() {
		t.Fatalf("expected edge label to be placed on %s, got %s", label.InsideMiddleCenter, *g.Edges[0].LabelPosition)
	}

	if *g.Edges[1].LabelPosition != label.InsideMiddleCenter.String() {
		t.Fatalf("expected edge label to be placed on %s, got %s", label.InsideMiddleCenter, *g.Edges[0].LabelPosition)
	}
}

func TestSpansSequenceDiagram(t *testing.T) {
	//   ┌─────┐                 ┌─────┐
	//   │  a  │                 │  b  │
	//   └──┬──┘                 └──┬──┘
	//      ├┐────────────────────►┌┤
	//   t1 ││                     ││ t1
	//      ├┘◄────────────────────└┤
	//      ├┐──────────────────────►
	//   t2 ││                      │
	//      ├┘◄─────────────────────┤

	input := `
shape: sequence_diagram
a: { shape: person }
b

a.t1: {
  shape: diamond
  label: label
}
a.t1 -> b.t1
b.t1 -> a.t1

a.t2 -> b
b -> a.t2`

	ctx := log.WithTB(context.Background(), t)
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	assert.Nil(t, err)

	g.Root.Shape = d2graph.Scalar{Value: d2target.ShapeSequenceDiagram}

	a, has := g.Root.HasChild([]string{"a"})
	assert.True(t, has)

	a_t1, has := a.HasChild([]string{"t1"})
	assert.True(t, has)

	a_t2, has := a.HasChild([]string{"t2"})
	assert.True(t, has)

	b, has := g.Root.HasChild([]string{"b"})
	assert.True(t, has)
	b.Box = geo.NewBox(nil, 30, 30)

	b_t1, has := b.HasChild([]string{"t1"})
	assert.True(t, has)

	a.Box = geo.NewBox(nil, 100, 100)
	a_t1.Box = geo.NewBox(nil, 100, 100)
	a_t2.Box = geo.NewBox(nil, 100, 100)
	b.Box = geo.NewBox(nil, 30, 30)
	b_t1.Box = geo.NewBox(nil, 100, 100)

	d2sequence.Layout(ctx, g, func(ctx context.Context, g *d2graph.Graph) error {
		// just set some position as if it had been properly placed
		for _, obj := range g.Objects {
			obj.TopLeft = geo.NewPoint(0, 0)
		}

		for _, edge := range g.Edges {
			edge.Route = []*geo.Point{geo.NewPoint(1, 1)}
		}
		return nil
	})

	// check properties
	assert.Equal(t, strings.ToLower(shape.PERSON_TYPE), strings.ToLower(a.Shape.Value))

	if a_t1.Label.Value != "" {
		t.Fatalf("expected no label for span, got %s", a_t1.Label.Value)
	}

	if a_t1.Shape.Value != shape.SQUARE_TYPE {
		t.Fatalf("expected square shape for span, got %s", a_t1.Shape.Value)
	}

	if a_t1.Height != b_t1.Height {
		t.Fatalf("expected a.t1 and b.t1 to have the same height, got %.5f and %.5f", a_t1.Height, b_t1.Height)
	}

	for _, span := range []*d2graph.Object{a_t1, a_t2, b_t1} {
		if span.ZIndex != d2sequence.SPAN_Z_INDEX {
			t.Fatalf("expected span ZIndex=%d, got %d", d2sequence.SPAN_Z_INDEX, span.ZIndex)
		}
	}

	// Y diff of the 2 first edges
	expectedHeight := g.Edges[1].Route[0].Y - g.Edges[0].Route[0].Y + (2 * d2sequence.SPAN_MESSAGE_PAD)
	if a_t1.Height != expectedHeight {
		t.Fatalf("expected a.t1 height to be %.5f, got %.5f", expectedHeight, a_t1.Height)
	}

	if a_t1.Width != d2sequence.SPAN_BASE_WIDTH {
		t.Fatalf("expected span width to be %.5f, got %.5f", d2sequence.SPAN_BASE_WIDTH, a_t1.Width)
	}

	// check positions
	if a.Center().X != a_t1.Center().X {
		t.Fatal("expected a_t1.X = a.X")
	}
	if a.Center().X != a_t2.Center().X {
		t.Fatal("expected a_t2.X = a.X")
	}
	if b.Center().X != b_t1.Center().X {
		t.Fatal("expected b_t1.X = b.X")
	}
	if a_t1.TopLeft.Y != b_t1.TopLeft.Y {
		t.Fatal("expected a.t1 and b.t1 to be placed at the same Y")
	}
	if a_t1.TopLeft.Y+d2sequence.SPAN_MESSAGE_PAD != g.Edges[0].Route[0].Y {
		t.Fatal("expected a.t1 to be placed at the same Y of the first message")
	}

	// check routes
	if g.Edges[0].Route[0].X != a_t1.TopLeft.X+a_t1.Width {
		t.Fatal("expected the first message to start on a.t1 top right X")
	}

	if g.Edges[0].Route[1].X != b_t1.TopLeft.X {
		t.Fatal("expected the first message to end on b.t1 top left X")
	}

	if g.Edges[2].Route[1].X != b.Center().X {
		t.Fatal("expected the third message to end on b.t1 center X")
	}
}

func TestNestedSequenceDiagrams(t *testing.T) {
	// ┌────────────────────────────────────────┐
	// |     ┌─────┐    container    ┌─────┐    |
	// |     │  a  │                 │  b  │    |            ┌─────┐
	// |     └──┬──┘                 └──┬──┘    ├────edge1───┤  c  │
	// |        ├┐───────sdEdge1──────►┌┤       |            └─────┘
	// |     t1 ││                     ││ t1    |
	// |        ├┘◄──────sdEdge2───────└┤       |
	// └────────────────────────────────────────┘
	input := `container: {
  shape: sequence_diagram
  a: { shape: person }
  b
  a.t1 -> b.t1: sequence diagram edge 1
  b.t1 -> a.t1: sequence diagram edge 2
}
c
container -> c: edge 1
`
	ctx := log.WithTB(context.Background(), t)
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	assert.Nil(t, err)

	container, has := g.Root.HasChild([]string{"container"})
	assert.True(t, has)
	container.Box = geo.NewBox(nil, 500, 500)

	a, has := container.HasChild([]string{"a"})
	assert.True(t, has)
	a.Box = geo.NewBox(nil, 100, 100)

	a_t1, has := a.HasChild([]string{"t1"})
	assert.True(t, has)
	a_t1.Box = geo.NewBox(nil, 100, 100)

	b, has := container.HasChild([]string{"b"})
	assert.True(t, has)
	b.Box = geo.NewBox(nil, 30, 30)

	b_t1, has := b.HasChild([]string{"t1"})
	assert.True(t, has)
	b_t1.Box = geo.NewBox(nil, 100, 100)

	c := g.Root.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("c")})
	c.Box = geo.NewBox(nil, 100, 100)
	c.Shape = d2graph.Scalar{Value: d2target.ShapeSquare}

	layoutFn := func(ctx context.Context, g *d2graph.Graph) error {
		if len(g.Objects) != 2 {
			t.Fatal("expected only diagram objects for layout")
		}
		for _, obj := range g.Objects {
			if obj == a || obj == a_t1 || obj == b || obj == b_t1 {
				t.Fatal("expected to have removed all sequence diagram objects")
			}
		}
		if len(container.ChildrenArray) != 0 {
			t.Fatalf("expected no `container` children, got %d", len(container.ChildrenArray))
		}

		if len(container.Children) != len(container.ChildrenArray) {
			t.Fatal("container children mismatch")
		}

		assert.Equal(t, 1, len(g.Edges))

		// just set some position as if it had been properly placed
		for _, obj := range g.Objects {
			obj.TopLeft = geo.NewPoint(0, 0)
		}

		for _, edge := range g.Edges {
			edge.Route = []*geo.Point{geo.NewPoint(1, 1)}
		}

		return nil
	}

	if err = d2sequence.Layout(ctx, g, layoutFn); err != nil {
		t.Fatal(err)
	}

	if len(g.Edges) != 5 {
		t.Fatal("expected graph to have all edges and lifelines after layout")
	}

	for _, obj := range g.Objects {
		if obj.TopLeft == nil {
			t.Fatal("expected to have placed all objects")
		}
	}
	for _, edge := range g.Edges {
		if len(edge.Route) == 0 {
			t.Fatal("expected to have routed all edges")
		}
	}
}

func TestSelfEdges(t *testing.T) {
	g := d2graph.NewGraph()
	g.Root.Shape = d2graph.Scalar{Value: d2target.ShapeSequenceDiagram}
	n1 := g.Root.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("n1")})
	n1.Box = geo.NewBox(nil, 100, 100)

	g.Edges = []*d2graph.Edge{
		{
			Src:   n1,
			Dst:   n1,
			Index: 0,
			Attributes: d2graph.Attributes{
				Label: d2graph.Scalar{Value: "left to right"},
			},
		},
	}

	ctx := log.WithTB(context.Background(), t)
	d2sequence.Layout(ctx, g, func(ctx context.Context, g *d2graph.Graph) error {
		return nil
	})

	route := g.Edges[0].Route
	if len(route) != 4 {
		t.Fatalf("expected route to have 4 points, got %d", len(route))
	}

	if route[0].X != route[3].X {
		t.Fatalf("route does not end at the same actor, start at %.5f, end at %.5f", route[0].X, route[3].X)
	}

	if route[3].Y-route[0].Y != d2sequence.MIN_MESSAGE_DISTANCE*1.5 {
		t.Fatalf("expected route height to be %.5f, got %.5f", d2sequence.MIN_MESSAGE_DISTANCE*1.5, route[3].Y-route[0].Y)
	}
}

func TestSequenceToDescendant(t *testing.T) {
	g := d2graph.NewGraph()
	g.Root.Shape = d2graph.Scalar{Value: d2target.ShapeSequenceDiagram}
	a := g.Root.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("a")})
	a.Box = geo.NewBox(nil, 100, 100)
	a.Attributes = d2graph.Attributes{
		Shape: d2graph.Scalar{Value: shape.PERSON_TYPE},
	}
	a_t1 := a.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("t1")})
	a_t1.Box = geo.NewBox(nil, 16, 80)

	g.Edges = []*d2graph.Edge{
		{
			Src:   a,
			Dst:   a_t1,
			Index: 0,
		}, {
			Src:   a_t1,
			Dst:   a,
			Index: 0,
		},
	}

	ctx := log.WithTB(context.Background(), t)
	d2sequence.Layout(ctx, g, func(ctx context.Context, g *d2graph.Graph) error {
		return nil
	})

	route1 := g.Edges[0].Route
	if len(route1) != 4 {
		t.Fatal("expected route with 4 points")
	}
	if route1[0].X != a.Center().X {
		t.Fatal("expected route to start at `a` lifeline")
	}
	if route1[3].X != a_t1.TopLeft.X+a_t1.Width {
		t.Fatal("expected route to end at `a.t1` right side")
	}

	route2 := g.Edges[1].Route
	if len(route2) != 4 {
		t.Fatal("expected route with 4 points")
	}
	if route2[0].X != a_t1.TopLeft.X+a_t1.Width {
		t.Fatal("expected route to start at `a.t1` right side")
	}
	if route2[3].X != a.Center().X {
		t.Fatal("expected route to end at `a` lifeline")
	}
}
