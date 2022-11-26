package d2sequence

import (
	"context"
	"testing"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/log"
)

func TestLayout(t *testing.T) {
	g := d2graph.NewGraph(nil)
	g.Objects = []*d2graph.Object{
		{
			ID:  "Alice",
			Box: geo.NewBox(nil, 100, 100),
		},
		{
			ID:  "Bob",
			Box: geo.NewBox(nil, 30, 30),
		},
	}

	g.Edges = []*d2graph.Edge{
		{
			Src: g.Objects[0],
			Dst: g.Objects[1],
		},
		{
			Src: g.Objects[1],
			Dst: g.Objects[0],
		},
		{
			Src: g.Objects[0],
			Dst: g.Objects[1],
		},
		{
			Src: g.Objects[1],
			Dst: g.Objects[0],
		},
	}
	nEdges := len(g.Edges)

	ctx := log.WithTB(context.Background(), t, nil)
	Layout(ctx, g)

	// asserts that objects were placed in the expected x order and at y=0
	objectsOrder := []*d2graph.Object{
		g.Objects[0],
		g.Objects[1],
	}
	for i := 1; i < len(objectsOrder); i++ {
		if objectsOrder[i].TopLeft.X < objectsOrder[i-1].TopLeft.X {
			t.Fatalf("expected object[%d].TopLeft.X > object[%d].TopLeft.X", i, i-1)
		}
		if objectsOrder[i].Center().Y != objectsOrder[i-1].Center().Y {
			t.Fatalf("expected object[%d] and object[%d] to be at the same center y", i, i-1)
		}
	}

	nExpectedEdges := nEdges + len(objectsOrder)
	if len(g.Edges) != nExpectedEdges {
		t.Fatalf("expected %d edges, got %d", nExpectedEdges, len(g.Edges))
	}

	// assert that edges were placed in y order and have the endpoints at their objects
	// uses `nEdges` because Layout creates some vertical edges to represent the object lifeline
	for i := 0; i < nEdges; i++ {
		edge := g.Edges[i]
		if len(edge.Route) != 2 {
			t.Fatalf("expected edge[%d] to have only 2 points", i)
		}
		if edge.Route[0].Y != edge.Route[1].Y {
			t.Fatalf("expected edge[%d] to be a horizontal line", i)
		}
		if edge.Route[0].X != edge.Src.Center().X {
			t.Fatalf("expected edge[%d] source endpoint to be at the middle of the source object", i)
		}
		if edge.Route[1].X != edge.Dst.Center().X {
			t.Fatalf("expected edge[%d] target endpoint to be at the middle of the target object", i)
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
			t.Fatalf("expected edge[%d] to have only 2 points", i)
		}
		if edge.Route[0].X != edge.Route[1].X {
			t.Fatalf("expected edge[%d] to be a vertical line", i)
		}
		if edge.Route[0].X != edge.Src.Center().X {
			t.Fatalf("expected edge[%d] x to be at the object center", i)
		}
		if edge.Route[0].Y != edge.Src.Height+edge.Src.TopLeft.Y {
			t.Fatalf("expected edge[%d] to start at the bottom of the source object", i)
		}
		if edge.Route[1].Y < lastSequenceEdge.Route[0].Y {
			t.Fatalf("expected edge[%d] to end after the last sequence edge", i)
		}
	}
}
