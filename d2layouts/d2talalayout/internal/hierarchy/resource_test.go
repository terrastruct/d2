package hierarchy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func canceledHierarchyContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func requireHierarchyCanceledAt(t *testing.T, err error, location string) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(%v, context.Canceled) = false", err)
	}
	if !strings.Contains(err.Error(), location) {
		t.Fatalf("cancellation error = %v, want operation %q", err, location)
	}
}

type cancelHierarchyAfterErrChecks struct {
	context.Context
	remaining int
}

func (ctx *cancelHierarchyAfterErrChecks) Err() error {
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return nil
}

func TestRankDAGCanceledBeforeValidation(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.AddNode(layoutgraph.NewNode(1, 10, 10))
	_, err := rankDAG(canceledHierarchyContext(), g)
	requireHierarchyCanceledAt(t, err, "RankDAG")
}

func TestRankDAGCanceledDuringValidation(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.AddNode(layoutgraph.NewNode(1, 10, 10))
	ctx := &cancelHierarchyAfterErrChecks{Context: context.Background(), remaining: 1}
	_, err := rankDAG(ctx, g)
	requireHierarchyCanceledAt(t, err, "RankDAG")
}

func TestDAGPreparationAndBrandesKopfMidLoopCancellation(t *testing.T) {
	g := layoutgraph.NewGraph()
	byLevel := map[int][]*placementNode{0: {}}
	for i := 0; i < 130; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i*20), 0)
		g.AddNodeUnchecked(node)
		byLevel[0] = append(byLevel[0], newPlacementNode(0, node))
	}
	_, err := makeSimpleDAG(
		&cancelHierarchyAfterErrChecks{Context: context.Background(), remaining: 1},
		g,
	)
	requireHierarchyCanceledAt(t, err, "MakeSimpleDAG")

	_, err = createAlignmentNodes(
		&cancelHierarchyAfterErrChecks{Context: context.Background(), remaining: 1},
		byLevel,
		geo.Top,
		geo.Left,
	)
	requireHierarchyCanceledAt(t, err, "CreateAlignmentNodes")
}

func TestRankDAGLowLimitIsNonVacuous(t *testing.T) {
	g := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	second := layoutgraph.NewNode(2, 10, 10)
	g.AddNodeUnchecked(first)
	g.AddNodeUnchecked(second)
	edge := g.Connect(first, second)
	edge.TargetArrowhead = layoutgraph.TriangleArrowhead

	_, err := rankDAGWithLimit(context.Background(), g, 2)
	if !errors.Is(err, limits.ErrOptimizationResourceLimit) {
		t.Fatalf("error = %v; want optimization resource limit", err)
	}
}

func TestRankDAGHandlesEmptyAndDisconnectedGraphs(t *testing.T) {
	result, err := rankDAG(context.Background(), layoutgraph.NewGraph())
	if err != nil {
		t.Fatal(err)
	}
	if result.levelCount != 0 || len(result.nodeToLevel) != 0 {
		t.Fatalf("empty result = levelCount %d, levels %v", result.levelCount, result.nodeToLevel)
	}

	disconnected := layoutgraph.NewGraph()
	disconnected.AddNodeUnchecked(layoutgraph.NewNode(1, 10, 10))
	disconnected.AddNodeUnchecked(layoutgraph.NewNode(2, 10, 10))
	if _, err := rankDAG(context.Background(), disconnected); err == nil {
		t.Fatal("disconnected rank input unexpectedly succeeded")
	}
}

func TestRankDAGHostileDepthIsIterative(t *testing.T) {
	if testing.Short() {
		t.Skip("hostile-depth regression")
	}
	const count = limits.MaxEngineNodes
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, count)
	for i := range nodes {
		nodes[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		g.AddNodeUnchecked(nodes[i])
		if i > 0 {
			edge := g.Connect(nodes[i-1], nodes[i])
			edge.TargetArrowhead = layoutgraph.TriangleArrowhead
			edge.ID = layoutgraph.EntityID(i)
		}
	}
	for left, right := 0, len(g.Nodes)-1; left < right; left, right = left+1, right-1 {
		g.Nodes[left], g.Nodes[right] = g.Nodes[right], g.Nodes[left]
	}
	result, err := rankDAG(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if result.levelCount != count || result.nodeToLevel[nodes[0]] != 0 || result.nodeToLevel[nodes[count-1]] != count-1 {
		t.Fatalf("hostile chain levels = %d, first %d, last %d", result.levelCount, result.nodeToLevel[nodes[0]], result.nodeToLevel[nodes[count-1]])
	}
}

func TestRankDAGDoesNotHaveATopologyIndependentIterationCap(t *testing.T) {
	const (
		nodeCount = 250
		edgeCount = 1_000
	)
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, nodeCount)
	for i := range nodes {
		nodes[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		g.AddNodeUnchecked(nodes[i])
	}
	connect := func(from, to int) {
		edge := g.Connect(nodes[from], nodes[to])
		edge.ID = layoutgraph.EntityID(len(g.Edges))
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
		edge.SetHierarchyRankWeight(1 + len(g.Edges)%97)
	}
	for i := 0; i+1 < nodeCount; i++ {
		connect(i, i+1)
	}
	for span := 2; len(g.Edges) < edgeCount; span++ {
		for from := 0; from+span < nodeCount && len(g.Edges) < edgeCount; from++ {
			connect(from, from+span)
		}
	}

	result, err := rankDAG(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if result.levelCount != nodeCount {
		t.Fatalf("level count = %d, want %d", result.levelCount, nodeCount)
	}
	for _, edge := range g.Edges {
		if span := result.nodeToLevel[edge.To] - result.nodeToLevel[edge.From]; span < 1 {
			t.Fatalf("edge %d has infeasible span %d", edge.ID, span)
		}
	}
}
