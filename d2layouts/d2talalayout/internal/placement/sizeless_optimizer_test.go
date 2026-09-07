package placement

import (
	"context"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func TestPlacementPoints(t *testing.T) {
	// # = medianPoint
	// n5 n1
	// n2    n4
	//    n3 #
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	n1.TopLeft = geo.NewPoint(1, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	n2.TopLeft = geo.NewPoint(0, 1)
	n3 := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	n3.TopLeft = geo.NewPoint(1, 2)
	n4 := g.AddNode(layoutgraph.NewNode(4, 10, 10))
	n4.TopLeft = geo.NewPoint(2, 1)
	n5 := g.AddNode(layoutgraph.NewNode(0, 10, 10))
	n5.TopLeft = geo.NewPoint(0, 0)

	g.Connect(n1, n5)
	g.Connect(n2, n5)
	g.Connect(n3, n5)
	g.Connect(n4, n5)

	r := rand.New(rand.NewSource(1))
	optim, err := newSizelessOptimizer(context.Background(), g, r)
	assert.NoError(t, err)

	ctx := withTestLogger(context.Background(), t)
	medianPoint := geo.NewPoint(2, 2)
	distance, err := optim.FindClosestUnoccupiedDistance(ctx, n5, medianPoint)
	assert.NoError(t, err)
	guard, err := limits.NewOptimizationWorkGuard(ctx, "sizelessOptimizer.optimize", limits.MaxOptimizationWorkUnits)
	assert.NoError(t, err)
	points, err := optim.placementPointsGuarded(n5, medianPoint, distance, guard)
	assert.NoError(t, err)

	// when querying for `#` with d=0, it should return `*` + `#`
	// Note that `n3` and `n4` should also return, but as they overlap with existing nodes, they are filtered out
	// n5 n1
	// n2    n4
	//    n3 #  *
	//       *
	assert.Equal(t, 3, len(points))
	expected := map[geo.Point]struct{}{
		*geo.NewPoint(2, 2): {}, // #
		*geo.NewPoint(2, 3): {}, // right *
		*geo.NewPoint(3, 2): {}, // bottom *
	}
	for _, p := range points {
		delete(expected, *p)
	}
	assert.Equal(t, 0, len(expected), "some points are missing")
}

func TestFindClosestUnoccupiedDistance(t *testing.T) {
	// n5 n1
	// n2    n4
	//    n3
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	n1.TopLeft = geo.NewPoint(1, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	n2.TopLeft = geo.NewPoint(0, 1)
	n3 := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	n3.TopLeft = geo.NewPoint(1, 2)
	n4 := g.AddNode(layoutgraph.NewNode(4, 10, 10))
	n4.TopLeft = geo.NewPoint(2, 1)
	n5 := g.AddNode(layoutgraph.NewNode(0, 10, 10))
	n5.TopLeft = geo.NewPoint(0, 0)

	g.Connect(n1, n5)
	g.Connect(n2, n5)
	g.Connect(n3, n5)
	g.Connect(n4, n5)

	r := rand.New(rand.NewSource(1))
	optim, err := newSizelessOptimizer(context.Background(), g, r)
	assert.NoError(t, err)

	assert.True(t, optim.isOccupied(n1.TopLeft))
	assert.True(t, optim.isOccupied(n2.TopLeft))
	assert.True(t, optim.isOccupied(n3.TopLeft))
	assert.True(t, optim.isOccupied(n4.TopLeft))
	assert.True(t, optim.isOccupied(n5.TopLeft))
	assert.False(t, optim.isOccupied(geo.NewPoint(1, 1)))
	assert.False(t, optim.isOccupied(geo.NewPoint(2, 2)))

	// medianPoint is unoccupied, so FindClosestUnoccupiedDistance returns 0
	// * = medianPoint
	// n5 n1
	// n2    n4
	//    n3 *
	ctx := withTestLogger(context.Background(), t)
	medianPoint := geo.NewPoint(2, 2)
	distance, err := optim.FindClosestUnoccupiedDistance(ctx, n5, medianPoint)
	assert.NoError(t, err)
	assert.Equal(t, 0.0, distance)

	// medianPoint is occupied by n2 (see above), so FindClosestUnoccupiedDistance returns 1
	// * = medianPoint
	// n5 n1
	// *    n4
	//    n3
	medianPoint = geo.NewPoint(1, 2)
	distance, err = optim.FindClosestUnoccupiedDistance(ctx, n5, medianPoint)
	assert.NoError(t, err)
	assert.Equal(t, 1.0, distance)

	// medianPoint is occupied by n5, so FindClosestUnoccupiedDistance returns 0
	// * = medianPoint
	//     n1
	// n2 n5* n4
	//     n3
	// Note: during optimization, if `n5` was under placement, this would return 0 as we remove n5 position before
	// finding its optimal position
	n5.TopLeft.X = 1
	n5.TopLeft.Y = 1
	optim.resetOccupied()
	medianPoint = geo.NewPoint(1, 1)
	distance, err = optim.FindClosestUnoccupiedDistance(ctx, n5, medianPoint)
	assert.NoError(t, err)
	assert.Equal(t, 2.0, distance)
}

func canOptimizeNode(ctx context.Context, n *layoutgraph.Node, g *layoutgraph.Graph) (bool, error) {
	guard, err := limits.NewOptimizationWorkGuard(ctx, "sizelessOptimizer", limits.MaxOptimizationWorkUnits)
	if err != nil {
		return false, err
	}
	return canOptimizeNodeGuarded(n, g, guard)
}

func TestCanOptimizeNode(t *testing.T) {
	g := layoutgraph.NewGraph()
	ctx := context.Background()

	n := layoutgraph.NewNode(1, 10, 10)
	canOptimize, err := canOptimizeNode(ctx, n, g)
	assert.NoError(t, err)
	assert.False(t, canOptimize)

	n.FixedTopLeft = geo.NewPoint(10, 10)
	canOptimize, err = canOptimizeNode(ctx, n, g)
	assert.NoError(t, err)
	assert.False(t, canOptimize)

	n = layoutgraph.NewNode(3, 10, 10)
	g.Connect(n, layoutgraph.NewNode(2, 10, 10))
	g.NodeToTree = make(map[*layoutgraph.Node]*layoutgraph.Tree)
	g.NodeToTree[n] = layoutgraph.NewTree(n)
	canOptimize, err = canOptimizeNode(ctx, n, g)
	assert.NoError(t, err)
	assert.False(t, canOptimize)

	n = layoutgraph.NewNode(4, 10, 10)
	g.Connect(n, layoutgraph.NewNode(2, 10, 10))
	canOptimize, err = canOptimizeNode(ctx, n, g)
	assert.NoError(t, err)
	assert.True(t, canOptimize)

	n = layoutgraph.NewNode(4, 10, 10)
	n.Nears[layoutgraph.NewNode(5, 10, 10)] = struct{}{}
	canOptimize, err = canOptimizeNode(ctx, n, g)
	assert.NoError(t, err)
	assert.True(t, canOptimize)
}

func (optim *sizelessOptimizer) swapCandidates(ctx context.Context, node *layoutgraph.Node) ([]*layoutgraph.Node, error) {
	guard, err := limits.NewOptimizationWorkGuard(ctx, "sizelessOptimizer.optimize", limits.MaxOptimizationWorkUnits)
	if err != nil {
		return nil, err
	}
	return optim.swapCandidatesGuarded(node, guard)
}

func TestSwapCandidates(t *testing.T) {
	// * = medianPoint
	// n5 n1
	// n2    n4
	//    n3 *
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	n1.TopLeft = geo.NewPoint(1, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	n2.TopLeft = geo.NewPoint(0, 1)
	n3 := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	n3.TopLeft = geo.NewPoint(1, 2)
	n4 := g.AddNode(layoutgraph.NewNode(4, 10, 10))
	n4.TopLeft = geo.NewPoint(2, 1)
	n5 := g.AddNode(layoutgraph.NewNode(5, 10, 10))
	n5.TopLeft = geo.NewPoint(0, 0)

	g.Connect(n1, n2)
	g.Connect(n3, n4)
	g.Connect(n3, n5)

	r := rand.New(rand.NewSource(1))
	g.ComputeCellSize()
	optim, err := newSizelessOptimizer(context.Background(), g, r)
	assert.NoError(t, err)
	ctx := context.Background()

	adjacent, err := optim.swapCandidates(ctx, n1)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(adjacent))
	assert.Equal(t, n5, adjacent[0])

	adjacent, err = optim.swapCandidates(ctx, n2)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(adjacent))
	assert.Equal(t, n5, adjacent[0])

	adjacent, err = optim.swapCandidates(ctx, n4)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(adjacent))

	adjacent, err = optim.swapCandidates(ctx, n3)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(adjacent))

	adjacent, err = optim.swapCandidates(ctx, n5)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(adjacent))
	assert.Equal(t, n2, adjacent[0])
	assert.Equal(t, n1, adjacent[1])

	// make n1 non-optimizable, so it shouldn't return here
	n1.FixedTopLeft = geo.NewPoint(1, 0)
	adjacent, err = optim.swapCandidates(ctx, n5)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(adjacent))
	assert.Equal(t, n2, adjacent[0])
}

func TestOptimize(t *testing.T) {
	// just a sanity check test ensuring that if n1, n2, n3, n4 can't move
	// then n5 ends up in the middle position
	// n5 n1
	// n2    n4
	//    n3
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	n1.TopLeft = geo.NewPoint(1, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	n2.TopLeft = geo.NewPoint(0, 1)
	n3 := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	n3.TopLeft = geo.NewPoint(1, 2)
	n4 := g.AddNode(layoutgraph.NewNode(4, 10, 10))
	n4.TopLeft = geo.NewPoint(2, 1)
	n5 := g.AddNode(layoutgraph.NewNode(5, 10, 10))
	n5.TopLeft = geo.NewPoint(0, 0)

	g.Connect(n1, n5)
	g.Connect(n2, n5)
	g.Connect(n3, n5)
	g.Connect(n4, n5)

	// only n5 can move
	n1.FixedTopLeft = n1.TopLeft
	n2.FixedTopLeft = n2.TopLeft
	n3.FixedTopLeft = n3.TopLeft
	n4.FixedTopLeft = n4.TopLeft

	r := rand.New(rand.NewSource(1))
	g.ComputeCellSize()
	optim, err := newSizelessOptimizer(context.Background(), g, r)
	assert.NoError(t, err)
	ctx := context.Background()
	err = optim.optimize(ctx, 0)
	assert.NoError(t, err)

	assert.Equal(t, *geo.NewPoint(1, 0), *n1.TopLeft)
	assert.Equal(t, *geo.NewPoint(0, 1), *n2.TopLeft)
	assert.Equal(t, *geo.NewPoint(1, 2), *n3.TopLeft)
	assert.Equal(t, *geo.NewPoint(2, 1), *n4.TopLeft)
	assert.Equal(t, *geo.NewPoint(1, 1), *n5.TopLeft)

	// n5 should stay in the same place
	err = optim.optimize(ctx, 0)
	assert.NoError(t, err)

	assert.Equal(t, *geo.NewPoint(1, 0), *n1.TopLeft)
	assert.Equal(t, *geo.NewPoint(0, 1), *n2.TopLeft)
	assert.Equal(t, *geo.NewPoint(1, 2), *n3.TopLeft)
	assert.Equal(t, *geo.NewPoint(2, 1), *n4.TopLeft)
	assert.Equal(t, *geo.NewPoint(1, 1), *n5.TopLeft)
}

func TestSizelessOptimizerRebuildsOccupancyAfterCancellation(t *testing.T) {
	g := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	first.TopLeft = geo.NewPoint(0, 0)
	second := layoutgraph.NewNode(2, 10, 10)
	second.TopLeft = geo.NewPoint(2, 0)
	g.AddNewNodeToContainer(nil, first)
	g.AddNewNodeToContainer(nil, second)
	g.Connect(first, second)
	g.ComputeCellSize()

	optim, err := newSizelessOptimizer(context.Background(), g, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatal(err)
	}
	// Cancellation at any guarded optimizer boundary preserves the exact cache
	// and graph state, regardless of whether it precedes or follows a trial move.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 1}
	err = optim.optimize(ctx, 0)
	requireCanceledAt(t, err, "sizelessOptimizer.optimize")

	if len(optim.occupied) != len(g.Nodes) {
		t.Fatalf("occupied node count after cancellation = %d; want %d", len(optim.occupied), len(g.Nodes))
	}
	for _, node := range g.Nodes {
		if got := optim.occupied[*node.TopLeft]; got != node {
			t.Fatalf("occupied[%v] after cancellation = %p; want %p", *node.TopLeft, got, node)
		}
	}
}
