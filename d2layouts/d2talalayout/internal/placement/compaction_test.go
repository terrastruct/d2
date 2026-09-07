package placement

import (
	"context"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/lib/log"
)

func mustVisibilityEdges(t *testing.T, ctx context.Context, g *layoutgraph.Graph, isHorizontal, includeSizes bool) layoutgraph.Edges {
	t.Helper()
	edges, err := visibilityEdges(ctx, g, isHorizontal, includeSizes)
	if err != nil {
		t.Fatal(err)
	}
	return edges
}

func TestVisibilityGraphOverlap(t *testing.T) {
	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 5, 5)
	a.TopLeft = geo.NewPoint(5, 5)
	graph.AddNode(a)

	b := layoutgraph.NewNode(2, 5, 5)
	b.TopLeft = geo.NewPoint(6, 5)
	graph.AddNode(b)

	vEdges := mustVisibilityEdges(t, context.Background(), graph, true, true)

	assert.Equal(t, 1, len(vEdges))
	assert.Equal(t, a, vEdges[0].From)
	assert.Equal(t, b, vEdges[0].To)
}

func TestVisibilityGraphInitialization(t *testing.T) {
	/**
																				+--------------+
																				|              |
																				|              |
	+----+                                |      D       |
	|    |                   +--------+   |              |
	|    |                   |        |   |              |
	| A  |                   |        |   +--------------+
	|    |      +------+     |    C   |
	|    |      |      |     |        |
	+----+      |  B   |     +--------+
							|      |
							+------+

	*/
	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 4, 6)
	a.TopLeft = geo.NewPoint(0, 4)
	graph.AddNode(a)

	b := layoutgraph.NewNode(2, 6, 4)
	b.TopLeft = geo.NewPoint(12, 8)
	graph.AddNode(b)

	c := layoutgraph.NewNode(3, 9, 5)
	c.TopLeft = geo.NewPoint(25, 5)
	graph.AddNode(c)

	d := layoutgraph.NewNode(4, 9, 6)
	d.TopLeft = geo.NewPoint(38, 1)
	graph.AddNode(d)

	vEdges := mustVisibilityEdges(t, context.Background(), graph, true, true)

	pairs := [][]*layoutgraph.Node{
		{a, b},
		{a, c},
		{a, d},
		{b, c},
		{c, d},
	}

	for _, pair := range pairs {
		found := false
		for _, e := range vEdges {
			if e.From == pair[0] && e.To == pair[1] {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("Edge that should exist doesn't exist")
		}
	}
}

func TestShiftSubgraphsWontChange(t *testing.T) {
	/* This test case ensures that nodes won't move infinitely when shifting the subgraph.
	There was a case during compaction (that calls shiftSubgraphs) that these subgraphs would move far away
	generating huge containers with plenty of empty space in between them (see the PR for this change for an example).
	The issue it tests:
	1. Nodes 15 and 19 are part of the same visibility subgraph and are aligned on the same vertical line
	2. Moving the root 15 also moves 19, so the total edge length never changes as thye move together
	3. shiftSubgraphs initialized the best length as +Inf instead of the current edge length
	4. Any edge length is better than +Inf, so the first move was taken as the best movement
	5. Further movement didn't improve edge length because 15 and 19 are moved together and the length is the same
	6. So there would always be a movement, and it would always move nodes further apart
	*/
	g := layoutgraph.NewGraph()
	n15 := g.AddNode(layoutgraph.NewNode(15, 49.000000, 60.000000))
	n15.TopLeft = geo.NewPoint(-180.000000, -60.000000)
	n19 := g.AddNode(layoutgraph.NewNode(19, 52.000000, 46.000000))
	n19.TopLeft = geo.NewPoint(-180.000000, 60.000000)
	n21 := g.AddNode(layoutgraph.NewNode(21, 57.000000, 57.000000))
	n21.TopLeft = geo.NewPoint(540.000000, 180.000000)
	n17 := g.AddNode(layoutgraph.NewNode(17, 60.000000, 50.000000))
	n17.TopLeft = geo.NewPoint(-300.000000, 60.000000)
	n22 := g.AddNode(layoutgraph.NewNode(22, 53.000000, 47.000000))
	n22.TopLeft = geo.NewPoint(540.000000, 60.000000)
	n18 := g.AddNode(layoutgraph.NewNode(18, 50.000000, 53.000000))
	n18.TopLeft = geo.NewPoint(540.000000, 300.000000)
	n16 := g.AddNode(layoutgraph.NewNode(16, 51.000000, 48.000000))
	n16.TopLeft = geo.NewPoint(660.000000, -60.000000)
	n20 := g.AddNode(layoutgraph.NewNode(20, 52.000000, 60.000000))
	n20.TopLeft = geo.NewPoint(660.000000, -180.000000)
	g.Connect(n19, n15)
	g.Connect(n22, n21)
	g.Connect(n18, n21)
	g.CellSize = 60
	ctx := context.Background()
	ctx = withTestLogger(ctx, t)
	ctx = log.Leveled(ctx, slog.LevelError)
	isHorizontal := false
	includeSizes := true
	factor := 2.4015748031496065
	transition := false

	visibilityEdges := mustVisibilityEdges(t, context.Background(), g, isHorizontal, includeSizes)
	inflateAlongAxis(g, isHorizontal, includeSizes, factor, visibilityEdges, transition)
	changed, err := shiftSubgraphs(ctx, g, isHorizontal, includeSizes, factor, []*layoutgraph.EdgeAbduction{}, visibilityEdges)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("Didn't expect any movement")
	}
}
