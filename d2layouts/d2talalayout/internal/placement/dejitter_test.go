package placement

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// .               +--------> b
// . a +-----------+
func TestBasicDejitter(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 8, 8)
	a.TopLeft = geo.NewPoint(4, 4)

	b := layoutgraph.NewNode(2, 8, 8)
	b.TopLeft = geo.NewPoint(20, 3)

	graph.AddNode(a)
	graph.AddNode(b)

	edge := graph.Connect(a, b)
	edge.Points = []*geo.Point{
		// Right-mid port of a
		geo.NewPoint(12, 4),
		// Bends
		geo.NewPoint(16, 4),
		geo.NewPoint(16, 3),
		// Left-mid port of b
		geo.NewPoint(20, 3),
	}

	Dejitter(ctx, graph)

	expectedPoints := []*geo.Point{
		geo.NewPoint(12, 3),
		geo.NewPoint(20, 3),
	}

	if len(expectedPoints) != len(edge.Points) {
		t.Fatal("Length of points don't match")
	}

	for i, expectedPoint := range expectedPoints {
		if !edge.Points[i].Equals(expectedPoint) {
			t.Fatalf("Expected point %v, got point %v", expectedPoint, edge.Points[i])
		}
	}
}

// TestDejitterProhibitOne tests that we can dejitter a->b by moving b instead of a, since
// moving a causes additional jitters but moving b doesn't
// .                        +--------> b
// . d +----> a +-----------+
func TestDejitterProhibitOne(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := layoutgraph.NewGraph()

	d := layoutgraph.NewNode(1, 8, 8)
	d.TopLeft = geo.NewPoint(4, 4)

	a := layoutgraph.NewNode(2, 8, 8)
	a.TopLeft = geo.NewPoint(20, 4)

	b := layoutgraph.NewNode(3, 8, 8)
	b.TopLeft = geo.NewPoint(40, 3)

	graph.AddNode(a)
	graph.AddNode(b)
	graph.AddNode(d)

	abEdge := graph.Connect(a, b)
	abEdge.Points = []*geo.Point{
		// Right-mid port of a
		geo.NewPoint(28, 4),
		// Bends
		geo.NewPoint(34, 4),
		geo.NewPoint(34, 3),
		// Left-mid port of b
		geo.NewPoint(40, 3),
	}

	daEdge := graph.Connect(d, a)
	daEdge.Points = []*geo.Point{
		// Right-mid port of d
		geo.NewPoint(12, 4),
		// Left-mid port of a
		geo.NewPoint(20, 4),
	}

	Dejitter(ctx, graph)

	// Since b moves, it moves down to match a
	expectedPoints := []*geo.Point{
		geo.NewPoint(28, 4),
		geo.NewPoint(40, 4),
	}

	if len(expectedPoints) != len(abEdge.Points) {
		t.Fatal("Length of points don't match")
	}

	for i, expectedPoint := range expectedPoints {
		if !abEdge.Points[i].Equals(expectedPoint) {
			t.Fatalf("Expected point %v, got point %v", expectedPoint, abEdge.Points[i])
		}
	}
}

// TestNoDejitter1 tests that we cannot dejitter a->b due to the fact it would
// cause additional jitters on d->a and b->c
// .                        +--------> b  +----> c
// . d +----> a +-----------+
func TestNoDejitter1(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := layoutgraph.NewGraph()

	d := layoutgraph.NewNode(1, 8, 8)
	d.TopLeft = geo.NewPoint(4, 4)

	a := layoutgraph.NewNode(2, 8, 8)
	a.TopLeft = geo.NewPoint(20, 4)

	b := layoutgraph.NewNode(3, 8, 8)
	b.TopLeft = geo.NewPoint(40, 3)

	c := layoutgraph.NewNode(4, 8, 8)
	c.TopLeft = geo.NewPoint(60, 3)

	graph.AddNode(a)
	graph.AddNode(b)
	graph.AddNode(c)
	graph.AddNode(d)

	abEdge := graph.Connect(a, b)
	abEdge.Points = []*geo.Point{
		// Right-mid port of a
		geo.NewPoint(28, 4),
		// Bends
		geo.NewPoint(34, 4),
		geo.NewPoint(34, 3),
		// Left-mid port of b
		geo.NewPoint(40, 3),
	}

	daEdge := graph.Connect(d, a)
	daEdge.Points = []*geo.Point{
		// Right-mid port of d
		geo.NewPoint(12, 4),
		// Left-mid port of a
		geo.NewPoint(20, 4),
	}

	bcEdge := graph.Connect(b, c)
	bcEdge.Points = []*geo.Point{
		// Right-mid port of b
		geo.NewPoint(48, 3),
		// Left-mid port of c
		geo.NewPoint(60, 3),
	}

	Dejitter(ctx, graph)

	// Equal to given
	expectedPoints := []*geo.Point{
		geo.NewPoint(28, 4),
		geo.NewPoint(34, 4),
		geo.NewPoint(34, 3),
		geo.NewPoint(40, 3),
	}

	if len(expectedPoints) != len(abEdge.Points) {
		t.Fatal("Length of points don't match")
	}

	for i, expectedPoint := range expectedPoints {
		if !abEdge.Points[i].Equals(expectedPoint) {
			t.Fatalf("Expected point %v, got point %v", expectedPoint, abEdge.Points[i])
		}
	}
}

// TestDejitterWithTangentConnections tests that we can dejitter a->b when they have connections
// that are tangent from the line we need to dejitter (and thus would't add additional jitters)
// .                           c
// .                           ^
// .                           |
// .                           |
// .                +--------> b
// .  a +-----------+
// .  |
// .  |
// .  v
// .  d
func TestDejitterWithTangentConnections(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := layoutgraph.NewGraph()

	d := layoutgraph.NewNode(1, 8, 8)
	d.TopLeft = geo.NewPoint(4, 40)

	a := layoutgraph.NewNode(2, 8, 8)
	a.TopLeft = geo.NewPoint(4, 20)

	b := layoutgraph.NewNode(3, 8, 8)
	b.TopLeft = geo.NewPoint(20, 18)

	c := layoutgraph.NewNode(4, 8, 8)
	c.TopLeft = geo.NewPoint(20, 4)

	graph.AddNode(a)
	graph.AddNode(b)
	graph.AddNode(c)
	graph.AddNode(d)

	abEdge := graph.Connect(a, b)
	abEdge.Points = []*geo.Point{
		// Right-mid port of a
		geo.NewPoint(12, 24),
		// Bends
		geo.NewPoint(16, 24),
		geo.NewPoint(16, 22),
		// Left-mid port of b
		geo.NewPoint(20, 22),
	}

	adEdge := graph.Connect(a, d)
	adEdge.Points = []*geo.Point{
		// Bottom-mid port of a
		geo.NewPoint(8, 28),
		// Top-mid port of d
		geo.NewPoint(8, 40),
	}

	bcEdge := graph.Connect(b, c)
	bcEdge.Points = []*geo.Point{
		// Top-mid port of b
		geo.NewPoint(24, 18),
		// Bottom-mid port of c
		geo.NewPoint(24, 12),
	}

	Dejitter(ctx, graph)

	expectedPoints := []*geo.Point{
		geo.NewPoint(12, 22),
		geo.NewPoint(20, 22),
	}

	if len(expectedPoints) != len(abEdge.Points) {
		t.Fatal("Length of points don't match")
	}

	for i, expectedPoint := range expectedPoints {
		if !abEdge.Points[i].Equals(expectedPoint) {
			t.Fatalf("Expected point %v, got point %v", expectedPoint, abEdge.Points[i])
		}
	}
}
