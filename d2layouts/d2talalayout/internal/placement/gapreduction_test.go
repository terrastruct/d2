package placement

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
)

func TestGapNormalizationBasic(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 10, 10)
	a.TopLeft = geo.NewPoint(0, 0)

	b := layoutgraph.NewNode(2, 10, 10)
	b.TopLeft = geo.NewPoint(1000, 0)

	graph.AddNode(a)
	graph.AddNode(b)
	graph.Connect(a, b)

	txn := mustNewTransaction(t, graph, layoutgraph.TransactionOptions{AffectContainers: true})
	_, _ = gapNormalization(ctx, layoutgraph.Nodes(graph.Nodes), txn, graph, gapNormalizationOptions{
		axis:      horizontalAxis,
		direction: forwardDirection,
	})

	assert.Equal(t, a.TopLeft.X+a.Width+placementcost.IdealGapSize, b.TopLeft.X)
}

func TestGapNormalizationRejectsIncompleteOptions(t *testing.T) {
	graph := layoutgraph.NewGraph()
	txn := mustNewTransaction(t, graph, layoutgraph.TransactionOptions{AffectContainers: true})
	tests := []struct {
		name    string
		options gapNormalizationOptions
	}{
		{name: "missing axis", options: gapNormalizationOptions{direction: forwardDirection}},
		{name: "missing direction", options: gapNormalizationOptions{axis: horizontalAxis}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := gapNormalization(context.Background(), nil, txn, graph, test.options); err == nil {
				t.Fatal("gap normalization accepted incomplete options")
			}
		})
	}
}

func TestGapNormalizationBothDirections(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 10, 10)
	a.TopLeft = geo.NewPoint(0, 0)

	b := layoutgraph.NewNode(2, 10, 10)
	b.TopLeft = geo.NewPoint(1000, 0)

	c := layoutgraph.NewNode(3, 10, 10)
	c.TopLeft = geo.NewPoint(1000, 1000)

	graph.AddNode(a)
	graph.AddNode(b)
	graph.AddNode(c)
	graph.Connect(a, b)

	txn := mustNewTransaction(t, graph, layoutgraph.TransactionOptions{AffectContainers: true})
	_, _ = gapNormalization(ctx, layoutgraph.Nodes(graph.Nodes), txn, graph, gapNormalizationOptions{
		axis:      horizontalAxis,
		direction: forwardDirection,
	})
	_, _ = gapNormalization(ctx, layoutgraph.Nodes(graph.Nodes), txn, graph, gapNormalizationOptions{
		axis:      verticalAxis,
		direction: forwardDirection,
	})

	assert.Equal(t, a.TopLeft.X+a.Width+placementcost.IdealGapSize, b.TopLeft.X)
	assert.Equal(t, 0.0, b.TopLeft.Y)
	assert.Equal(t, 1000.0, c.TopLeft.X)
	assert.Equal(t, 1000.0, c.TopLeft.Y)
}

func TestGapNormalizationMovesConnectedSubgraphTogether(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 10, 100)
	a.TopLeft = geo.NewPoint(0, 0)

	b := layoutgraph.NewNode(2, 10, 10)
	b.TopLeft = geo.NewPoint(1000, 0)

	c := layoutgraph.NewNode(3, 10, 10)
	c.TopLeft = geo.NewPoint(2000, 0)

	graph.AddNode(a)
	graph.AddNode(b)
	graph.AddNode(c)
	graph.Connect(a, b)
	graph.Connect(a, c)
	graph.Connect(b, c)
	originalBCGap := c.TopLeft.X - b.TopLeft.X

	txn := mustNewTransaction(t, graph, layoutgraph.TransactionOptions{AffectContainers: true})
	_, _ = gapNormalization(ctx, layoutgraph.Nodes(graph.Nodes), txn, graph, gapNormalizationOptions{
		axis:      horizontalAxis,
		direction: forwardDirection,
	})

	assert.Equal(t, a.TopLeft.X+a.Width+placementcost.IdealGapSize, b.TopLeft.X)
	assert.Equal(t, 1000.0, originalBCGap)
	assert.Equal(t, originalBCGap, c.TopLeft.X-b.TopLeft.X)
}

func TestGapNormalizationWithContainer(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 10, 100)
	a.TopLeft = geo.NewPoint(100, 100)

	b := layoutgraph.NewNode(2, 10, 10)
	b.TopLeft = geo.NewPoint(1000, 100)

	c := layoutgraph.NewNode(3, 2000, 3000)
	c.TopLeft = geo.NewPoint(0, 0)

	graph.AddNode(a)
	graph.AddNode(b)
	graph.AddNode(c)
	graph.Connect(a, b)

	graph.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {c},
		c:   {a, b},
	}

	if _, err := NormalizeGaps(ctx, graph); err != nil {
		t.Fatal("failed")
	}

	assert.Equal(t, 100.0, a.TopLeft.X)
	assert.Equal(t, 100.0, a.TopLeft.Y)
	assert.Equal(t, a.TopLeft.X+a.Width+placementcost.IdealGapSize, b.TopLeft.X)

	assert.Equal(t, 0.0, c.TopLeft.X)
	assert.Equal(t, 0.0, c.TopLeft.Y)
	assert.Equal(t, 2000.0, c.Width)
	assert.Equal(t, 3000.0, c.Height)
	assert.True(t, c.Covers(a))
	assert.True(t, c.Covers(b))
}

func TestGapContainerExpansionPushing(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)

	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 10, 10)
	a.TopLeft = geo.NewPoint(100, 100)

	b := layoutgraph.NewNode(2, 30, 30)
	b.TopLeft = geo.NewPoint(90, 90)

	c := layoutgraph.NewNode(3, 10, 10)
	c.TopLeft = geo.NewPoint(130, 90)

	graph.AddNode(a)
	graph.AddNode(b)
	graph.AddNode(c)

	graph.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {b, c},
		b:   {a},
	}

	if _, err := NormalizeGaps(ctx, graph); err != nil {
		t.Fatal("failed")
	}

	if c.TopLeft.X >= b.TopLeft.X && c.TopLeft.X <= (b.TopLeft.X+b.Width) {
		t.Fatalf("node %d at %v overlaps container %d at %v with width %v", c.ID, c.TopLeft, b.ID, b.TopLeft, b.Width)
	}
}

// TODO: add bidirectional gap-normalization coverage proving both outer nodes
// move toward a connected center node.
