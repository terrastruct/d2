package placement

import (
	"context"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/lib/geo"
)

type cancelAfterSwapChecks struct {
	context.Context
	node      *layoutgraph.Node
	swappedTo geo.Point
	remaining int
}

func bestSwapCandidateForTest(ctx context.Context, optim *sizedOptimizer, node *layoutgraph.Node) (*layoutgraph.Node, error) {
	guard, err := limits.NewOptimizationWorkGuard(ctx, "LocalOptimize", limits.MaxOptimizationWorkUnits)
	if err != nil {
		return nil, err
	}
	if err := optim.rebuildSpatialIndex(guard); err != nil {
		return nil, err
	}
	return optim.bestSwapCandidateGuarded(ctx, node, guard)
}

func (optim *sizedOptimizer) findClosestUnoccupiedDistanceForTest(ctx context.Context, node *layoutgraph.Node, p *geo.Point, minimizingSelf bool, checked map[geo.Point]struct{}) (float64, error) {
	guard, err := limits.NewOptimizationWorkGuard(ctx, "LocalOptimize", limits.MaxOptimizationWorkUnits)
	if err != nil {
		return 0, err
	}
	if err := optim.rebuildSpatialIndex(guard); err != nil {
		return 0, err
	}
	return optim.findClosestUnoccupiedDistanceGuarded(node, p, minimizingSelf, checked, guard)
}

// moveNodeToBestForTest moves the node to the best point provided in `points`.
// best = minimizes edge length, considering symmetry
func (optim *sizedOptimizer) moveNodeToBestForTest(ctx context.Context, node *layoutgraph.Node, points []geo.Point, mustImprove bool) (bool, error) {
	guard, err := limits.NewOptimizationWorkGuard(ctx, "LocalOptimize", limits.MaxOptimizationWorkUnits)
	if err != nil {
		return false, err
	}
	if err := optim.rebuildSpatialIndex(guard); err != nil {
		return false, err
	}
	return optim.moveNodeToBestGuarded(ctx, node, points, mustImprove, guard)
}

func (ctx *cancelAfterSwapChecks) Err() error {
	if ctx.node.TopLeft != nil && *ctx.node.TopLeft == ctx.swappedTo {
		if ctx.remaining == 0 {
			return context.Canceled
		}
		ctx.remaining--
	}
	return nil
}

// TODO: some tests that are worth considering in the future
// - test with protruding children (minimizingSelf = false)
// - test with edge abductions
// - test with a larger node size (100?)

func TestMedianPoint(t *testing.T) {
	// ┌────────┐   ┌────────┐
	// │        │   │        │
	// │   n5   │   │   n1   │
	// │        │   │        │
	// └────────┘   └────────┘
	//
	// ┌────────┐                ┌────────┐
	// │        │                │        │
	// │  n2    │                │   n3   │
	// │        │                │        │
	// └────────┘                └────────┘
	//
	//               ┌────────┐
	//               │        │
	//               │   n4   │
	//               │        │
	//               └────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	n1.TopLeft = geo.NewPoint(100, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	n2.TopLeft = geo.NewPoint(0, 100)
	n3 := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	n3.TopLeft = geo.NewPoint(200, 100)
	n4 := g.AddNode(layoutgraph.NewNode(4, 50, 50))
	n4.TopLeft = geo.NewPoint(100, 200)
	n5 := g.AddNode(layoutgraph.NewNode(5, 50, 50))
	n5.TopLeft = geo.NewPoint(0, 0)

	g.Connect(n1, n5)
	g.Connect(n2, n5)
	g.Connect(n3, n5)
	g.Connect(n4, n5)

	g.ComputeCellSize()
	randGenerator := rand.New(rand.NewSource(1))
	optim, err := newSizedOptimizer(context.Background(), g, nil, nil, randGenerator, nil)
	assert.NoError(t, err)

	ctx := withTestLogger(context.Background(), t)
	guard, err := limits.NewOptimizationWorkGuard(ctx, "LocalOptimize", limits.MaxOptimizationWorkUnits)
	assert.NoError(t, err)
	medianPoint, err := optim.medianPointGuarded(n5, 0, nil, guard)
	assert.NoError(t, err)

	assert.Equal(t, *geo.NewPoint(125, 125), *medianPoint)
}

func TestSizedOptimizerDoesNotTruncateLargeParallelEdgeRequirements(t *testing.T) {
	graph := layoutgraph.NewGraph()
	graph.CellSize = 100
	from := graph.AddNode(layoutgraph.NewNode(1, 50, 50))
	to := graph.AddNode(layoutgraph.NewNode(2, 50, 50))
	from.TopLeft = geo.NewPoint(0, 0)
	to.TopLeft = geo.NewPoint(100, 0)
	for i := 0; i < 256; i++ {
		edge := graph.Connect(from, to)
		edge.MinWidth = 5000
		edge.MinHeight = 6000
	}

	_, err := newSizedOptimizer(context.Background(), graph, nil, nil, rand.New(rand.NewSource(1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	requirements, ok := from.LongDistanceNeighborRequirements[to]
	if !ok {
		t.Fatal("large parallel-edge requirements were not retained")
	}
	if requirements.EdgeCount != 256 || requirements.MaxWidth != 5000 || requirements.MaxHeight != 6000 {
		t.Fatalf("requirements were truncated: %+v", requirements)
	}
}

func TestIterPlacementsAroundPoint(t *testing.T) {
	graph := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 10, 10)
	graph.AddNodeUnchecked(node)
	graph.CellSize = 2

	points := make([]*geo.Point, 0)
	iterPlacementsAroundPoint(node, 5, 5, true, func(x, y float64) bool {
		points = append(points, geo.NewPoint(x, y))
		return false
	})

	expectedPoints := []*geo.Point{
		// corners
		geo.NewPoint(0, 0),
		geo.NewPoint(0, 10),
		geo.NewPoint(10, 0),
		geo.NewPoint(10, 10),

		// bottom side touching point
		geo.NewPoint(2, 0),
		geo.NewPoint(4, 0),
		geo.NewPoint(6, 0),
		geo.NewPoint(8, 0),

		// right side touching point
		geo.NewPoint(0, 2),
		geo.NewPoint(0, 4),
		geo.NewPoint(0, 6),
		geo.NewPoint(0, 8),

		// left side touching point
		geo.NewPoint(10, 2),
		geo.NewPoint(10, 4),
		geo.NewPoint(10, 6),
		geo.NewPoint(10, 8),

		// top side touching point
		geo.NewPoint(2, 10),
		geo.NewPoint(4, 10),
		geo.NewPoint(6, 10),
		geo.NewPoint(8, 10),
	}

	for _, expectedPoint := range expectedPoints {
		found := false
		notMatched := []geo.Point{}
		for _, point := range points {
			if *point == *expectedPoint {
				found = true
				break
			}
			notMatched = append(notMatched, *point)
		}
		if !found {
			t.Fatalf("point %v not found, tried: %v", *expectedPoint, notMatched)
		}
	}
}

func TestSizedFindClosestUnoccupiedDistance(t *testing.T) {
	// ┌────────┐  ┌────────┐
	// │        │  │        │
	// │   n5   │  │   n1   │
	// │        │  │        │
	// └────────┘  └────────┘
	//
	// ┌────────┐   ┌────────┐  ┌────────┐
	// │        │   │        │  │        │
	// │  n2    │   │   n6   │  │   n3   │
	// │        │   │        │  │        │
	// └────────┘   └────────┘  └────────┘
	//
	//              ┌────────┐
	//              │        │
	//              │   n4   │
	//              │        │
	//              └────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	n1.TopLeft = geo.NewPoint(100, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	n2.TopLeft = geo.NewPoint(0, 100)
	n3 := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	n3.TopLeft = geo.NewPoint(200, 100)
	n4 := g.AddNode(layoutgraph.NewNode(4, 50, 50))
	n4.TopLeft = geo.NewPoint(100, 200)
	n5 := g.AddNode(layoutgraph.NewNode(5, 50, 50))
	n5.TopLeft = geo.NewPoint(0, 0)
	// n6 is added later

	g.Connect(n1, n5)
	g.Connect(n2, n5)
	g.Connect(n3, n5)
	g.Connect(n4, n5)

	g.ComputeCellSize()
	randGenerator := rand.New(rand.NewSource(1))
	optim, err := newSizedOptimizer(context.Background(), g, nil, nil, randGenerator, nil)
	assert.NoError(t, err)
	ctx := context.Background()

	// the node position should not be considered occupied
	d, err := optim.findClosestUnoccupiedDistanceForTest(ctx, n5, n5.TopLeft, true, nil)
	assert.NoError(t, err)
	assert.Equal(t, 0., d)

	// at 100, 100, n5 would be 50 units away from other nodes
	// but since the nodes are connected, their delta is 60, requiring
	// to find another place
	d, err = optim.findClosestUnoccupiedDistanceForTest(ctx, n5, geo.NewPoint(100, 100), true, nil)
	assert.NoError(t, err)
	assert.Equal(t, 4., d)

	// when checking for `n4` position, it is occupied
	// though, it can't use any of the adjacent cells because of the padding we must keep
	// between nodes, so the minimum distance in this case is 3
	d, err = optim.findClosestUnoccupiedDistanceForTest(ctx, n5, n4.TopLeft.Copy(), true, nil)
	assert.NoError(t, err)
	assert.Equal(t, 3., d)

	n6 := g.AddNode(layoutgraph.NewNode(6, 50, 50))
	n6.TopLeft = geo.NewPoint(100, 100)
	g.ComputeCellSize()
	randGenerator = rand.New(rand.NewSource(1))
	optim, err = newSizedOptimizer(context.Background(), g, nil, nil, randGenerator, nil)
	assert.NoError(t, err)

	// same as above, `n6` and none of the adjacent cells can be occupied because of
	// the padding, so the min distance here is 4
	d, err = optim.findClosestUnoccupiedDistanceForTest(ctx, n5, n6.TopLeft.Copy(), true, nil)
	assert.NoError(t, err)
	assert.Equal(t, 4., d)
}

func TestFindClosestUnoccupiedDistanceNoConnections(t *testing.T) {
	// ┌────────┐  ┌────────┐
	// │        │  │        │
	// │   n5   │  │   n1   │
	// │        │  │        │
	// └────────┘  └────────┘
	//
	// ┌────────┐   ┌────────┐  ┌────────┐
	// │        │   │        │  │        │
	// │  n2    │   │   n6   │  │   n3   │
	// │        │   │        │  │        │
	// └────────┘   └────────┘  └────────┘
	//
	//              ┌────────┐
	//              │        │
	//              │   n4   │
	//              │        │
	//              └────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	n1.TopLeft = geo.NewPoint(100, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	n2.TopLeft = geo.NewPoint(0, 100)
	n3 := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	n3.TopLeft = geo.NewPoint(200, 100)
	n4 := g.AddNode(layoutgraph.NewNode(4, 50, 50))
	n4.TopLeft = geo.NewPoint(100, 200)
	n5 := g.AddNode(layoutgraph.NewNode(5, 50, 50))
	n5.TopLeft = geo.NewPoint(0, 0)
	// n6 is added later

	g.ComputeCellSize()
	randGenerator := rand.New(rand.NewSource(1))
	optim, err := newSizedOptimizer(context.Background(), g, nil, nil, randGenerator, nil)
	assert.NoError(t, err)
	ctx := context.Background()

	// the node position should not be considered occupied
	d, err := optim.findClosestUnoccupiedDistanceForTest(ctx, n5, n5.TopLeft, true, nil)
	assert.NoError(t, err)
	assert.Equal(t, 0., d)

	// at 100, 100, n5 would be 50 units away from other nodes
	// but here the nodes are not connected and then this is a valid position
	d, err = optim.findClosestUnoccupiedDistanceForTest(ctx, n5, geo.NewPoint(100, 100), true, nil)
	assert.NoError(t, err)
	assert.Equal(t, 0., d)

	// Boundary contact is valid for disconnected nodes, so one cell reaches a
	// non-overlapping position beside n4.
	d, err = optim.findClosestUnoccupiedDistanceForTest(ctx, n5, n4.TopLeft.Copy(), true, nil)
	assert.NoError(t, err)
	assert.Equal(t, 1., d)

	n6 := g.AddNode(layoutgraph.NewNode(6, 50, 50))
	n6.TopLeft = geo.NewPoint(100, 100)
	g.ComputeCellSize()
	randGenerator = rand.New(rand.NewSource(1))
	optim, err = newSizedOptimizer(context.Background(), g, nil, nil, randGenerator, nil)
	assert.NoError(t, err)

	// With the center occupied, three cells reaches a boundary-touching position
	// beside the surrounding disconnected nodes.
	d, err = optim.findClosestUnoccupiedDistanceForTest(ctx, n5, n6.TopLeft.Copy(), true, nil)
	assert.NoError(t, err)
	assert.Equal(t, 3., d)
}

func TestSizedOptimizerGetPlacementPoints(t *testing.T) {
	// ┌────────┐   ┌────────┐
	// │        │   │        │
	// │   n5   │   │   n1   │
	// │        │   │        │
	// └────────┘   └────────┘
	//
	// ┌────────┐                ┌────────┐
	// │        │                │        │
	// │  n2    │                │   n3   │
	// │        │                │        │
	// └────────┘                └────────┘
	//
	//               ┌────────┐
	//               │        │
	//               │   n4   │
	//               │        │
	//               └────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	n1.TopLeft = geo.NewPoint(100, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	n2.TopLeft = geo.NewPoint(0, 100)
	n3 := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	n3.TopLeft = geo.NewPoint(200, 100)
	n4 := g.AddNode(layoutgraph.NewNode(4, 50, 50))
	n4.TopLeft = geo.NewPoint(100, 200)
	n5 := g.AddNode(layoutgraph.NewNode(5, 50, 50))
	n5.TopLeft = geo.NewPoint(0, 0)

	g.Connect(n1, n5)
	g.Connect(n2, n5)
	g.Connect(n3, n5)
	g.Connect(n4, n5)

	g.ComputeCellSize()
	randGenerator := rand.New(rand.NewSource(1))
	optim, err := newSizedOptimizer(context.Background(), g, nil, nil, randGenerator, nil)
	assert.NoError(t, err)

	ctx := withTestLogger(context.Background(), t)
	guard, err := limits.NewOptimizationWorkGuard(ctx, "LocalOptimize", limits.MaxOptimizationWorkUnits)
	assert.NoError(t, err)
	points, err := optim.fillPlacementPointsGuarded(n5, geo.NewPoint(100, 100), 1, true, &placementPointsScratch{}, guard)
	assert.NoError(t, err)
	// TODO: fix. We should get (d + 1)^2 = 4 points
	assert.Equal(t, 24, len(points))

	// TODO: once fixed, test the coordinates too

	// checks that all nodes are multiples of cell sizes
	for _, p := range points {
		assert.Equal(t, 0., math.Mod(p.X, optim.cellSize))
		assert.Equal(t, 0., math.Mod(p.Y, optim.cellSize))
	}
}

func TestMoveNodeToBest(t *testing.T) {
	// ┌────────┐   ┌────────┐
	// │        │   │        │
	// │   n5   │   │   n1   │
	// │        │   │        │
	// └────────┘   └────────┘
	//
	// ┌────────┐                ┌────────┐
	// │        │                │        │
	// │  n2    │                │   n3   │
	// │        │                │        │
	// └────────┘                └────────┘
	//
	//               ┌────────┐
	//               │        │
	//               │   n4   │
	//               │        │
	//               └────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	n1.TopLeft = geo.NewPoint(100, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	n2.TopLeft = geo.NewPoint(0, 100)
	n3 := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	n3.TopLeft = geo.NewPoint(200, 100)
	n4 := g.AddNode(layoutgraph.NewNode(4, 50, 50))
	n4.TopLeft = geo.NewPoint(100, 200)
	n5 := g.AddNode(layoutgraph.NewNode(5, 50, 50))
	n5.TopLeft = geo.NewPoint(0, 0)

	g.Connect(n1, n5)
	g.Connect(n2, n5)
	g.Connect(n3, n5)
	g.Connect(n4, n5)

	g.ComputeCellSize()
	randGenerator := rand.New(rand.NewSource(1))
	optim, err := newSizedOptimizer(context.Background(), g, nil, nil, randGenerator, nil)
	assert.NoError(t, err)

	ctx := withTestLogger(context.Background(), t)
	// (100, 100) is invalid, so n5 should stay at its current position
	points := []geo.Point{*n5.TopLeft.Copy(), *geo.NewPoint(100, 100)}
	changed, err := optim.moveNodeToBestForTest(ctx, n5, points, false)
	assert.NoError(t, err)
	assert.False(t, changed)

	// (300, 200) overlaps with n3 (given the padding of connected nodes)
	// so, (350, 200) is the best movement here
	points = []geo.Point{*geo.NewPoint(300, 200), *geo.NewPoint(400, 200), *geo.NewPoint(350, 200)}
	changed, err = optim.moveNodeToBestForTest(ctx, n5, points, false)
	assert.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, *geo.NewPoint(350, 200), *n5.TopLeft)
}

func TestBestSwapCandidate(t *testing.T) {
	//              ┌────────┐
	//              │        │
	//              │   n1   │
	//              │        │
	//              └────────┘
	//
	// ┌────────┐                ┌────────┐      ┌────────┐
	// │        │                │        │      │        │
	// │  n2    │                │   n3   │      │   n5   │
	// │        │                │        │      │        │
	// └────────┘                └────────┘      └────────┘
	//
	//               ┌────────┐
	//               │        │
	//               │   n4   │
	//               │        │
	//               └────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	n1.TopLeft = geo.NewPoint(100, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	n2.TopLeft = geo.NewPoint(0, 100)
	n3 := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	n3.TopLeft = geo.NewPoint(250, 100)
	n4 := g.AddNode(layoutgraph.NewNode(4, 50, 50))
	n4.TopLeft = geo.NewPoint(100, 200)
	n5 := g.AddNode(layoutgraph.NewNode(5, 50, 50))
	n5.TopLeft = geo.NewPoint(350, 200)

	g.Connect(n1, n5)
	g.Connect(n2, n5)
	g.Connect(n3, n5)
	g.Connect(n4, n5)

	g.ComputeCellSize()
	randGenerator := rand.New(rand.NewSource(1))
	optim, err := newSizedOptimizer(context.Background(), g, nil, nil, randGenerator, nil)
	assert.NoError(t, err)

	ctx := withTestLogger(context.Background(), t)
	swapCandidate, err := bestSwapCandidateForTest(ctx, optim, n5)
	assert.NoError(t, err)
	// A cell smaller than the connected-node delta leaves no legal adjacent swap.
	assert.Nil(t, swapCandidate)

	// A larger cell makes the adjacent position satisfy the connected-node delta.
	g.CellSize = 100
	n5.TopLeft = geo.NewPoint(400, 100)
	swapCandidate, err = bestSwapCandidateForTest(ctx, optim, n5)
	assert.NoError(t, err)
	assert.Equal(t, n3, swapCandidate)
}

func TestSizedSwapRestoresPositionsOnSymmetryCancellation(t *testing.T) {
	g := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	first.TopLeft = geo.NewPoint(0, 0)
	second := layoutgraph.NewNode(2, 10, 10)
	second.TopLeft = geo.NewPoint(80, 0)
	g.AddNewNodeToContainer(nil, first)
	g.AddNewNodeToContainer(nil, second)
	g.ComputeCellSize()
	g.CellSize = 100

	optim, err := newSizedOptimizer(context.Background(), g, nil, nil, rand.New(rand.NewSource(1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstPoint := first.TopLeft
	secondPoint := second.TopLeft
	firstPosition := *first.TopLeft
	secondPosition := *second.TopLeft
	ctx := &cancelAfterSwapChecks{
		Context:   context.Background(),
		node:      first,
		swappedTo: secondPosition,
		remaining: 3, // edgeLength and the non-table column check succeed.
	}

	candidate, err := bestSwapCandidateForTest(ctx, optim, first)
	requireCanceledAt(t, err, "EdgeLength")
	if candidate != nil {
		t.Fatalf("swap candidate after cancellation = %v; want nil", candidate)
	}
	if first.TopLeft != firstPoint || *first.TopLeft != firstPosition {
		t.Fatalf("first position after cancellation = %p %v; want %p %v", first.TopLeft, first.TopLeft, firstPoint, firstPosition)
	}
	if second.TopLeft != secondPoint || *second.TopLeft != secondPosition {
		t.Fatalf("second position after cancellation = %p %v; want %p %v", second.TopLeft, second.TopLeft, secondPoint, secondPosition)
	}
}

func TestHubSpokeSuppressionRestoresExactEdgeSliceOnCancellation(t *testing.T) {
	g := layoutgraph.NewGraph()
	hub := layoutgraph.NewNode(1, 10, 10)
	hub.TopLeft = geo.NewPoint(0, 0)
	spoke := layoutgraph.NewNode(2, 10, 10)
	spoke.TopLeft = geo.NewPoint(20, 0)
	other := layoutgraph.NewNode(3, 10, 10)
	other.TopLeft = geo.NewPoint(40, 0)
	g.AddNewNodeToContainer(nil, hub)
	g.AddNewNodeToContainer(nil, spoke)
	g.AddNewNodeToContainer(nil, other)
	g.Connect(hub, spoke)
	g.Connect(hub, other)
	g.ComputeCellSize()

	originalEdges := hub.Edges
	originalFirstAddress := &hub.Edges[0]
	guard, err := limits.NewOptimizationWorkGuard(t.Context(), "LocalOptimize", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = withHubSpokesSuppressed(hub, []*layoutgraph.Node{spoke}, guard, func() error {
		if len(hub.Edges) != len(originalEdges)-1 {
			t.Fatalf("suppressed edge count = %d; want %d", len(hub.Edges), len(originalEdges)-1)
		}
		_, err := placementcost.NodeEdgeLength(ctx, hub, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: false, EnforceMinimumGap: false, PenalizeDirection: true})
		return err
	})
	requireCanceledAt(t, err, "EdgeLength")

	if len(hub.Edges) != len(originalEdges) || &hub.Edges[0] != originalFirstAddress {
		t.Fatal("hub edge slice identity was not restored")
	}
	for i := range originalEdges {
		if hub.Edges[i] != originalEdges[i] {
			t.Fatalf("hub edge %d after cancellation = %p; want %p", i, hub.Edges[i], originalEdges[i])
		}
	}
}
