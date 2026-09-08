package placement

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type cancelWhenStackContains struct {
	context.Context
	function string
}

func (ctx *cancelWhenStackContains) Err() error {
	var callers [32]uintptr
	count := runtime.Callers(2, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, ctx.function) {
			return context.Canceled
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

type panicWhenPositionChanges struct {
	context.Context
	node *layoutgraph.Node
	x    float64
	y    float64
}

type cancelWhenPositionChanges struct {
	context.Context
	node *layoutgraph.Node
	x    float64
	y    float64
}

type cancelDuringCompactionSubgraphSnapshot struct {
	context.Context
	graph         *layoutgraph.Graph
	node          *layoutgraph.Node
	x             float64
	y             float64
	originalCosts layoutgraph.RoutingCostState
	observed      bool
	costsChanged  bool
}

func (ctx *cancelDuringCompactionSubgraphSnapshot) Err() error {
	if ctx.node.TopLeft.X == ctx.x && ctx.node.TopLeft.Y == ctx.y {
		return ctx.Context.Err()
	}
	var callers [32]uintptr
	count := runtime.Callers(2, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, ".snapshotNodePositionsContext") {
			ctx.observed = true
			ctx.costsChanged = ctx.graph.RoutingCosts() != ctx.originalCosts
			return context.Canceled
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

func (ctx *cancelWhenPositionChanges) Err() error {
	if ctx.node.TopLeft.X != ctx.x || ctx.node.TopLeft.Y != ctx.y {
		return context.Canceled
	}
	return ctx.Context.Err()
}

func (ctx *panicWhenPositionChanges) Err() error {
	if ctx.node.TopLeft.X != ctx.x || ctx.node.TopLeft.Y != ctx.y {
		panic("deterministic cancellation probe")
	}
	return ctx.Context.Err()
}

func compactionMutationGraph() (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Node) {
	g := layoutgraph.NewGraph()
	anchor := layoutgraph.NewNode(1, 1, 1)
	anchor.TopLeft = geo.NewPoint(0, 0)
	moving := layoutgraph.NewNode(2, 1, 1)
	moving.TopLeft = geo.NewPoint(1, 0)
	child := layoutgraph.NewNode(3, 1, 1)
	child.TopLeft = geo.NewPoint(2, 0)
	g.AddNodeUnchecked(anchor)
	g.AddNodeUnchecked(moving)
	moving.SetContainer(true)
	g.Containers[moving] = []*layoutgraph.Node{child}
	child.Container = moving
	child.Graph = g
	g.Connect(anchor, moving)
	g.CellSize = 1
	return g, moving, child
}

func TestCompactionRejectsIncompleteOptions(t *testing.T) {
	tests := []struct {
		name    string
		options compactionOptions
	}{
		{name: "missing axis", options: compactionOptions{factor: 1}},
		{name: "missing factor", options: compactionOptions{axis: horizontalAxis}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := compaction(context.Background(), nil, test.options); err == nil {
				t.Fatal("compaction accepted incomplete options")
			}
		})
	}
}

func TestCompactionPostInflationCancellationRestoresExactSubtreePositions(t *testing.T) {
	g, moving, child := compactionMutationGraph()
	movingPointer, movingValue := moving.TopLeft, *moving.TopLeft
	childPointer, childValue := child.TopLeft, *child.TopLeft
	ctx := &cancelWhenPositionChanges{
		Context: context.Background(),
		node:    moving,
		x:       moving.TopLeft.X,
		y:       moving.TopLeft.Y,
	}

	err := compaction(ctx, g, compactionOptions{
		axis:       horizontalAxis,
		factor:     10,
		transition: true,
	})
	requireCanceledAt(t, err, "Compaction")
	if moving.TopLeft != movingPointer || *moving.TopLeft != movingValue {
		t.Fatal("compaction cancellation did not restore the moved node exactly")
	}
	if child.TopLeft != childPointer || *child.TopLeft != childValue {
		t.Fatal("compaction cancellation did not restore the moved subtree exactly")
	}
}

func TestCompactionPostInflationPanicRestoresExactSubtreePositions(t *testing.T) {
	g, moving, child := compactionMutationGraph()
	movingPointer, movingValue := moving.TopLeft, *moving.TopLeft
	childPointer, childValue := child.TopLeft, *child.TopLeft
	ctx := &panicWhenPositionChanges{
		Context: context.Background(),
		node:    moving,
		x:       moving.TopLeft.X,
		y:       moving.TopLeft.Y,
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = compaction(ctx, g, compactionOptions{
			axis:       horizontalAxis,
			factor:     10,
			transition: true,
		})
	}()
	if recovered == nil {
		t.Fatal("compaction did not reach the deterministic post-inflation panic")
	}
	if moving.TopLeft != movingPointer || *moving.TopLeft != movingValue {
		t.Fatal("compaction panic did not restore the moved node exactly")
	}
	if child.TopLeft != childPointer || *child.TopLeft != childValue {
		t.Fatal("compaction panic did not restore the moved subtree exactly")
	}
}

func TestCompactionSubgraphSnapshotCancellationRestoresExactState(t *testing.T) {
	g, moving, child := compactionMutationGraph()
	trailing := layoutgraph.NewNode(4, 1, 1)
	trailing.TopLeft = geo.NewPoint(2, 1)
	g.AddNodeUnchecked(trailing)
	g.Connect(moving, trailing)
	positions := captureExactOptimizerPositions(g)
	childPointer, childValue := child.TopLeft, *child.TopLeft
	originalCosts := g.RoutingCosts()
	ctx := &cancelDuringCompactionSubgraphSnapshot{
		Context:       context.Background(),
		graph:         g,
		node:          moving,
		x:             moving.TopLeft.X,
		y:             moving.TopLeft.Y,
		originalCosts: originalCosts,
	}

	err := compaction(ctx, g, compactionOptions{
		axis:         horizontalAxis,
		includeSizes: true,
		factor:       10,
	})
	requireCanceledAt(t, err, "Compaction")
	if !ctx.observed {
		t.Fatal("compaction did not observe cancellation during the post-inflation subgraph snapshot")
	}
	if !ctx.costsChanged {
		t.Fatal("compaction did not mutate routing costs before the subgraph snapshot cancellation")
	}
	requireExactOptimizerPositions(t, positions)
	if child.TopLeft != childPointer || *child.TopLeft != childValue {
		t.Fatal("compaction cancellation did not restore the moved descendant exactly")
	}
	if costs := g.RoutingCosts(); costs != originalCosts {
		t.Fatalf("routing costs after rollback = %+v; want %+v", costs, originalCosts)
	}
}

type cancelDuringReachabilityAfterNodeCleared struct {
	context.Context
	node *layoutgraph.Node
}

func (ctx *cancelDuringReachabilityAfterNodeCleared) Err() error {
	if ctx.node.TopLeft == nil {
		probe := &cancelWhenStackContains{Context: ctx.Context, function: "reachableNodesGuarded"}
		return probe.Err()
	}
	return ctx.Context.Err()
}

func TestInitializeNodesMidReachabilityCancellationRestoresExactPositions(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.CellSize = 10
	firstFixed := layoutgraph.NewNode(1, 10, 10)
	firstFixed.TopLeft = geo.NewPoint(0, 0)
	firstFixed.FixedTopLeft = geo.NewPoint(0, 0)
	firstOther := layoutgraph.NewNode(2, 10, 10)
	firstOther.TopLeft = geo.NewPoint(20, 0)
	g.AddNodeUnchecked(firstFixed)
	g.AddNodeUnchecked(firstOther)
	g.Connect(firstFixed, firstOther)

	secondFixed := layoutgraph.NewNode(100, 10, 10)
	secondFixed.TopLeft = geo.NewPoint(0, 100)
	secondFixed.FixedTopLeft = geo.NewPoint(0, 100)
	g.AddNodeUnchecked(secondFixed)
	previous := secondFixed
	for i := 0; i < 130; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(101+i), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i*20), 100)
		g.AddNodeUnchecked(node)
		g.Connect(previous, node)
		previous = node
	}

	positions := make(map[*layoutgraph.Node]pointerSnapshot[geo.Point], len(g.Nodes))
	for _, node := range g.Nodes {
		positions[node] = snapshotPointer(node.TopLeft)
	}
	err := initializeNodes(&cancelDuringReachabilityAfterNodeCleared{
		Context: context.Background(),
		node:    firstOther,
	}, g)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("InitializeNodes error = %v, want context.Canceled", err)
	}
	for node, position := range positions {
		if node.TopLeft != position.pointer || (node.TopLeft != nil && *node.TopLeft != position.value) {
			t.Fatalf("node %d position was not restored after reachability cancellation", node.ID)
		}
	}
}

func TestTransposeMidReachabilityCancellationReturnsErrorWithoutMutation(t *testing.T) {
	g := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 20, 20)
	container.TopLeft = geo.NewPoint(0, 0)
	container.SetContainer(true)
	node := layoutgraph.NewNode(2, 5, 5)
	node.TopLeft = geo.NewPoint(1, 1)
	node.Container = container
	center := layoutgraph.NewNode(3, 5, 5)
	center.TopLeft = geo.NewPoint(100, 0)
	g.AddNodeUnchecked(container)
	g.AddNodeUnchecked(node)
	g.AddNodeUnchecked(center)
	g.Containers[container] = []*layoutgraph.Node{node}
	g.Connect(node, center)
	previous := container
	for i := 0; i < 130; i++ {
		connected := layoutgraph.NewNode(layoutgraph.EntityID(10+i), 5, 5)
		connected.TopLeft = geo.NewPoint(float64(i*10), 50)
		g.AddNodeUnchecked(connected)
		g.Connect(previous, connected)
		previous = connected
	}
	positions := make(map[*layoutgraph.Node]geo.Point, len(g.Nodes))
	for _, current := range g.Nodes {
		positions[current] = *current.TopLeft
	}

	_, err := transpose(
		&cancelWhenStackContains{Context: context.Background(), function: "reachableNodesGuarded"},
		g,
		node,
		nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("transpose error = %v, want context.Canceled", err)
	}
	for current, position := range positions {
		if *current.TopLeft != position {
			t.Fatalf("transpose changed node %d before returning cancellation", current.ID)
		}
	}
}

func TestEquidistanceMidReachabilityCancellationIsAtomic(t *testing.T) {
	g := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 10, 10)
	node.TopLeft = geo.NewPoint(0, 0)
	back := layoutgraph.NewNode(2, 10, 10)
	back.TopLeft = geo.NewPoint(-30, 0)
	front := layoutgraph.NewNode(3, 10, 10)
	front.TopLeft = geo.NewPoint(30, 0)
	other := layoutgraph.NewNode(4, 10, 10)
	other.TopLeft = geo.NewPoint(0, -30)
	for _, current := range []*layoutgraph.Node{node, back, front, other} {
		g.AddNodeUnchecked(current)
	}
	g.Connect(node, back)
	g.Connect(node, front)
	g.Connect(node, other)
	previous := other
	for i := 0; i < 130; i++ {
		connected := layoutgraph.NewNode(layoutgraph.EntityID(10+i), 10, 10)
		connected.TopLeft = geo.NewPoint(0, float64(-50-i*20))
		g.AddNodeUnchecked(connected)
		g.Connect(previous, connected)
		previous = connected
	}
	positions := make(map[*layoutgraph.Node]pointerSnapshot[geo.Point], len(g.Nodes))
	for _, current := range g.Nodes {
		positions[current] = snapshotPointer(current.TopLeft)
	}

	_, err := Equidistance(
		&cancelWhenStackContains{Context: context.Background(), function: "reachableNodesGuarded"},
		g,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Equidistance error = %v, want context.Canceled", err)
	}
	for current, position := range positions {
		if current.TopLeft != position.pointer || *current.TopLeft != position.value {
			t.Fatalf("Equidistance did not restore node %d exactly", current.ID)
		}
	}
}
