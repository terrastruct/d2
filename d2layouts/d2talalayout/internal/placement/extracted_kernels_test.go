package placement

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func TestEdgeDirectionCountsUseSemanticArrowEndpoints(t *testing.T) {
	left := layoutgraph.NewNode(1, 10, 10)
	left.TopLeft = geo.NewPoint(0, 0)
	right := layoutgraph.NewNode(2, 10, 10)
	right.TopLeft = geo.NewPoint(100, 0)
	targetRight := layoutgraph.NewEdge(left, right)
	targetRight.TargetArrowhead = layoutgraph.TriangleArrowhead
	sourceLeft := layoutgraph.NewEdge(left, right)
	sourceLeft.SourceArrowhead = layoutgraph.TriangleArrowhead
	equivalentTargetLeft := layoutgraph.NewEdge(right, left)
	equivalentTargetLeft.TargetArrowhead = layoutgraph.TriangleArrowhead
	if got, want := edgeDirectionCounts(targetRight), (directionCounts{right: 1}); got != want {
		t.Fatalf("target-right counts = %+v; want %+v", got, want)
	}
	if got, want := edgeDirectionCounts(sourceLeft), (directionCounts{left: 1}); got != want {
		t.Fatalf("source-left counts = %+v; want %+v", got, want)
	}
	if got, want := edgeDirectionCounts(equivalentTargetLeft), edgeDirectionCounts(sourceLeft); got != want {
		t.Fatalf("equivalent target-left counts = %+v; want %+v", got, want)
	}
}

func TestOptimizerCanMove(t *testing.T) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	first.TopLeft = geo.NewPoint(5, 5)
	graph.AddNode(first)
	second := layoutgraph.NewNode(2, 10, 10)
	second.TopLeft = geo.NewPoint(-1000, -1000)
	graph.AddNode(second)
	guard, err := limits.NewOptimizationWorkGuard(context.Background(), "TestOptimizerCanMove", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	canMove := func(node *layoutgraph.Node, point *geo.Point, includeSizes bool) bool {
		t.Helper()
		ok, err := optimizerCanMove(node, point, includeSizes, guard)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if canMove(second, geo.NewPoint(5, 5), false) {
		t.Fatal("occupied point accepted")
	}
	if !canMove(second, geo.NewPoint(0, 0), false) || canMove(second, geo.NewPoint(0, 0), true) {
		t.Fatal("size-aware overlap semantics changed")
	}
	if canMove(second, geo.NewPoint(20, 20), true) || !canMove(second, geo.NewPoint(35, 35), true) {
		t.Fatal("padding-aware overlap semantics changed")
	}
	if !canMove(first, geo.NewPoint(5, 5), true) || !canMove(first, geo.NewPoint(5, 5), false) {
		t.Fatal("node cannot retain its own position")
	}
}

func TestCompareDirectionCountsIsReflexiveAndPreservesTies(t *testing.T) {
	values := []directionCount{
		{direction: geo.Left, count: 2},
		{direction: geo.Top, count: 2},
		{direction: geo.Right, count: 2},
		{direction: geo.Bottom, count: 2},
	}
	for _, value := range values {
		if got := compareDirectionCounts(value, value, geo.Right); got != 0 {
			t.Fatalf("compareDirectionCounts(%+v, itself) = %d; want 0", value, got)
		}
	}

	slices.SortStableFunc(values, func(a, b directionCount) int {
		return compareDirectionCounts(a, b, geo.Right)
	})
	want := []directionCount{
		{direction: geo.Right, count: 2},
		{direction: geo.Left, count: 2},
		{direction: geo.Top, count: 2},
		{direction: geo.Bottom, count: 2},
	}
	if !slices.Equal(values, want) {
		t.Fatalf("equal-count order = %+v; want %+v", values, want)
	}
}

func TestDirectionTransforms(t *testing.T) {
	tests := []struct {
		counts           directionCounts
		direction        geo.Orientation
		mirrorX, mirrorY bool
	}{
		{direction: geo.Right},
		{direction: geo.Right, counts: directionCounts{right: 1}},
		{direction: geo.Left, counts: directionCounts{right: 1}, mirrorX: true},
		{direction: geo.Top, counts: directionCounts{bottom: 1}, mirrorY: true},
		{direction: geo.Bottom, counts: directionCounts{top: 1}, mirrorY: true},
		{direction: geo.Right, counts: directionCounts{left: 2, top: 1}, mirrorX: true, mirrorY: true},
		{direction: geo.Right, counts: directionCounts{left: 1, top: 1, right: 1}, mirrorY: true},
		{direction: geo.Bottom, counts: directionCounts{left: 1, bottom: 1}, mirrorX: true},
		{direction: geo.NONE, counts: directionCounts{left: 1, bottom: 1}, mirrorX: true},
	}
	for _, test := range tests {
		got := test.counts.transformsTo(test.direction)
		if got.mirrorX != test.mirrorX || got.mirrorY != test.mirrorY {
			t.Fatalf("counts %+v direction %v => %+v; want x=%v y=%v", test.counts, test.direction, got, test.mirrorX, test.mirrorY)
		}
	}
}

type cancelWhenMirrorChanges struct {
	context.Context
	node        *layoutgraph.Node
	position    geo.Point
	tree        *layoutgraph.Tree
	orientation geo.Orientation
	waitForTree bool
	observed    bool
}

func (ctx *cancelWhenMirrorChanges) Err() error {
	changed := ctx.node.TopLeft == nil || *ctx.node.TopLeft != ctx.position
	if ctx.waitForTree {
		changed = ctx.tree.Orientation != ctx.orientation
	}
	if changed {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestMirrorAxesMidReachabilityCancellationReturnsBeforeMutation(t *testing.T) {
	graph := layoutgraph.NewGraph()
	var previous *layoutgraph.Node
	positions := make(map[*layoutgraph.Node]geo.Point)
	for i := 0; i < 130; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i*20), 0)
		graph.AddNodeUnchecked(node)
		positions[node] = *node.TopLeft
		if previous != nil {
			graph.Connect(previous, node)
		}
		previous = node
	}
	err := mirrorAxes(&cancelWhenStackContains{Context: context.Background(), function: "reachableNodesGuarded"}, graph, true, true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("mirrorAxes error = %v, want context.Canceled", err)
	}
	for node, position := range positions {
		if *node.TopLeft != position {
			t.Fatalf("mirror changed node %d before cancellation", node.ID)
		}
	}
}

func TestMirrorAxesMutationCancellationRestoresExactState(t *testing.T) {
	for _, test := range []struct {
		name        string
		waitForTree bool
	}{{name: "node"}, {name: "tree", waitForTree: true}} {
		t.Run(test.name, func(t *testing.T) {
			graph := layoutgraph.NewGraph()
			node := layoutgraph.NewNode(1, 10, 10)
			node.TopLeft = geo.NewPoint(25, 40)
			graph.AddNodeUnchecked(node)
			tree := &layoutgraph.Tree{Node: node, Orientation: geo.Right}
			graph.NodeToTree = map[*layoutgraph.Node]*layoutgraph.Tree{node: tree}
			pointer, position, orientation := node.TopLeft, *node.TopLeft, tree.Orientation
			ctx := &cancelWhenMirrorChanges{Context: context.Background(), node: node, position: position, tree: tree, orientation: orientation, waitForTree: test.waitForTree}
			err := mirrorAxes(ctx, graph, true, false)
			if !errors.Is(err, context.Canceled) || !ctx.observed {
				t.Fatalf("mirror cancellation = %v observed=%v", err, ctx.observed)
			}
			if node.TopLeft != pointer || *node.TopLeft != position || tree.Orientation != orientation {
				t.Fatal("mirror cancellation did not restore exact state")
			}
		})
	}
}
