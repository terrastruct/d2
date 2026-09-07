package grouping

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type joinPositionSnapshot struct {
	node    *layoutgraph.Node
	pointer *geo.Point
	value   geo.Point
}

func captureJoinPositions(nodes ...*layoutgraph.Node) []joinPositionSnapshot {
	snapshots := make([]joinPositionSnapshot, 0, len(nodes))
	for _, node := range nodes {
		snapshots = append(snapshots, joinPositionSnapshot{
			node:    node,
			pointer: node.TopLeft,
			value:   *node.TopLeft,
		})
	}
	return snapshots
}

func assertJoinPositionsRestored(t *testing.T, snapshots []joinPositionSnapshot) {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.node.TopLeft != snapshot.pointer || *snapshot.node.TopLeft != snapshot.value {
			t.Fatalf("node %d position = %p %v; want exact %p %v", snapshot.node.ID, snapshot.node.TopLeft, snapshot.node.TopLeft, snapshot.pointer, snapshot.value)
		}
	}
}

func joinAtomicityGraph() (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Node, *layoutgraph.Node) {
	graph := layoutgraph.NewGraph()
	graph.CellSize = 10

	left := layoutgraph.NewNode(1, 10, 10)
	left.TopLeft = geo.NewPoint(0, 0)
	graph.AddNewNodeToContainer(nil, left)

	right := layoutgraph.NewNode(2, 10, 10)
	right.TopLeft = geo.NewPoint(1000, 0)
	graph.AddNewNodeToContainer(nil, right)
	graph.Connect(left, right)

	child := layoutgraph.NewNode(3, 2, 2)
	child.TopLeft = geo.NewPoint(1, 1)
	child.Graph = graph
	graph.AddNodeToContainer(left, child)

	return graph, left, right, child
}

type joinMutationObserverContext struct {
	context.Context
	node      *layoutgraph.Node
	original  geo.Point
	observed  bool
	panicWith any
}

func (ctx *joinMutationObserverContext) Err() error {
	if *ctx.node.TopLeft != ctx.original {
		ctx.observed = true
		if ctx.panicWith != nil {
			panic(ctx.panicWith)
		}
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestJoinDistancedClustersCancellationRestoresExactGeometry(t *testing.T) {
	graph, left, right, child := joinAtomicityGraph()
	snapshots := captureJoinPositions(left, right, child)
	ctx := &joinMutationObserverContext{
		Context:  context.Background(),
		node:     left,
		original: *left.TopLeft,
	}

	err := JoinDistancedClusters(ctx, graph)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("JoinDistancedClusters error = %v, want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe a cluster movement")
	}
	assertJoinPositionsRestored(t, snapshots)
}

func TestJoinDistancedClustersPanicRestoresExactGeometry(t *testing.T) {
	graph, left, right, child := joinAtomicityGraph()
	snapshots := captureJoinPositions(left, right, child)
	sentinel := &struct{}{}
	ctx := &joinMutationObserverContext{
		Context:   context.Background(),
		node:      left,
		original:  *left.TopLeft,
		panicWith: sentinel,
	}

	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		_ = JoinDistancedClusters(ctx, graph)
	}()
	if recovered != sentinel {
		t.Fatalf("panic = %v, want exact sentinel %p", recovered, sentinel)
	}
	if !ctx.observed {
		t.Fatal("panic probe did not observe a cluster movement")
	}
	assertJoinPositionsRestored(t, snapshots)
}

func TestJoinDistancedClustersRestoresSharedPositionAliases(t *testing.T) {
	graph, left, right, child := joinAtomicityGraph()
	child.TopLeft = left.TopLeft
	snapshots := captureJoinPositions(left, right, child)
	ctx := &joinMutationObserverContext{
		Context:  context.Background(),
		node:     left,
		original: *left.TopLeft,
	}

	err := JoinDistancedClusters(ctx, graph)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("JoinDistancedClusters error = %v, want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe a cluster movement")
	}
	assertJoinPositionsRestored(t, snapshots)
	if left.TopLeft != child.TopLeft {
		t.Fatal("join did not restore the shared TopLeft pointer")
	}
}

func TestJoinDistancedClustersSuccessCommitsGeometry(t *testing.T) {
	graph, left, right, child := joinAtomicityGraph()
	leftPointer, rightPointer, childPointer := left.TopLeft, right.TopLeft, child.TopLeft
	leftBefore, rightBefore, childBefore := *left.TopLeft, *right.TopLeft, *child.TopLeft

	if err := JoinDistancedClusters(context.Background(), graph); err != nil {
		t.Fatal(err)
	}
	if left.TopLeft != leftPointer || right.TopLeft != rightPointer || child.TopLeft != childPointer {
		t.Fatal("successful join replaced a TopLeft pointer")
	}
	if *left.TopLeft == leftBefore || *right.TopLeft == rightBefore || *child.TopLeft == childBefore {
		t.Fatal("successful join did not commit every expected movement")
	}
	if child.TopLeft.X-left.TopLeft.X != childBefore.X-leftBefore.X ||
		child.TopLeft.Y-left.TopLeft.Y != childBefore.Y-leftBefore.Y {
		t.Fatal("successful join changed the child offset")
	}
}

func TestJoinDistancedClustersRejectsExhaustedSharedWorkBeforeMutation(t *testing.T) {
	graph, left, right, child := joinAtomicityGraph()
	snapshots := captureJoinPositions(left, right, child)
	guard, err := limits.NewWorkGuard(context.Background(), "AutolayoutTransactions", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); err != nil {
		t.Fatal(err)
	}
	ctx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)

	err = JoinDistancedClusters(ctx, graph)
	if err == nil || !strings.Contains(err.Error(), "work exceeds limit 1") {
		t.Fatalf("JoinDistancedClusters error = %v, want exhausted shared work error", err)
	}
	if guard.Used() != 2 {
		t.Fatalf("shared work used = %d, want first rejected unit 2", guard.Used())
	}
	assertJoinPositionsRestored(t, snapshots)
}

func TestJoinDistancedClustersWorkBoundaryIsAtomicAndExact(t *testing.T) {
	baselineGraph, baselineLeft, baselineRight, baselineChild := joinAtomicityGraph()
	baselineGuard, err := limits.NewWorkGuard(context.Background(), "AutolayoutTransactions", limits.MaxTransactionWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	baselineContext := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), baselineGuard)
	if err := JoinDistancedClusters(baselineContext, baselineGraph); err != nil {
		t.Fatal(err)
	}
	work := baselineGuard.Used()
	if work <= 1 {
		t.Fatalf("successful join used %d work units, want more than one", work)
	}
	want := [...]geo.Point{*baselineLeft.TopLeft, *baselineRight.TopLeft, *baselineChild.TopLeft}

	exactGraph, exactLeft, exactRight, exactChild := joinAtomicityGraph()
	exactGuard, err := limits.NewWorkGuard(context.Background(), "AutolayoutTransactions", work)
	if err != nil {
		t.Fatal(err)
	}
	exactContext := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), exactGuard)
	if err := JoinDistancedClusters(exactContext, exactGraph); err != nil {
		t.Fatalf("JoinDistancedClusters at exact work boundary: %v", err)
	}
	got := [...]geo.Point{*exactLeft.TopLeft, *exactRight.TopLeft, *exactChild.TopLeft}
	if got != want {
		t.Fatalf("exact-boundary positions = %v, want %v", got, want)
	}
	if exactGuard.Used() != work {
		t.Fatalf("exact-boundary work used = %d, want %d", exactGuard.Used(), work)
	}

	limitedGraph, limitedLeft, limitedRight, limitedChild := joinAtomicityGraph()
	limitedSnapshots := captureJoinPositions(limitedLeft, limitedRight, limitedChild)
	limitedGuard, err := limits.NewWorkGuard(context.Background(), "AutolayoutTransactions", work-1)
	if err != nil {
		t.Fatal(err)
	}
	limitedContext := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), limitedGuard)
	err = JoinDistancedClusters(limitedContext, limitedGraph)
	if err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("JoinDistancedClusters below exact work boundary error = %v, want work limit", err)
	}
	if limitedGuard.Used() != work {
		t.Fatalf("limited work used = %d, want first rejected unit %d", limitedGuard.Used(), work)
	}
	assertJoinPositionsRestored(t, limitedSnapshots)
}

func TestJoinDistancedClustersFastPathsChargeSharedWork(t *testing.T) {
	tests := []struct {
		name     string
		joinWork int64
		build    func() (*layoutgraph.Graph, []*layoutgraph.Node)
	}{
		{
			name:     "single movable",
			joinWork: 1,
			build: func() (*layoutgraph.Graph, []*layoutgraph.Node) {
				graph := layoutgraph.NewGraph()
				node := layoutgraph.NewNode(1, 10, 10)
				node.TopLeft = geo.NewPoint(0, 0)
				graph.AddNode(node)
				return graph, []*layoutgraph.Node{node}
			},
		},
		{
			name:     "all fixed",
			joinWork: 2,
			build: func() (*layoutgraph.Graph, []*layoutgraph.Node) {
				graph := layoutgraph.NewGraph()
				left := layoutgraph.NewNode(1, 10, 10)
				left.TopLeft = geo.NewPoint(0, 0)
				left.FixedTopLeft = left.TopLeft.Copy()
				right := layoutgraph.NewNode(2, 10, 10)
				right.TopLeft = geo.NewPoint(20, 0)
				right.FixedTopLeft = right.TopLeft.Copy()
				graph.AddNode(left)
				graph.AddNode(right)
				return graph, []*layoutgraph.Node{left, right}
			},
		},
		{
			name:     "no join references",
			joinWork: 4,
			build: func() (*layoutgraph.Graph, []*layoutgraph.Node) {
				graph := layoutgraph.NewGraph()
				left := layoutgraph.NewNode(1, 10, 10)
				left.TopLeft = geo.NewPoint(0, 0)
				right := layoutgraph.NewNode(2, 10, 10)
				right.TopLeft = geo.NewPoint(20, 0)
				graph.AddNode(left)
				graph.AddNode(right)
				return graph, []*layoutgraph.Node{left, right}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const prechargedWork = int64(1)
			exactLimit := prechargedWork + test.joinWork

			exactGraph, _ := test.build()
			exactGuard, err := limits.NewWorkGuard(context.Background(), "AutolayoutTransactions", exactLimit)
			if err != nil {
				t.Fatal(err)
			}
			if err := exactGuard.Add(prechargedWork); err != nil {
				t.Fatal(err)
			}
			exactContext := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), exactGuard)
			if err := JoinDistancedClusters(exactContext, exactGraph); err != nil {
				t.Fatalf("JoinDistancedClusters at exact fast-path boundary: %v", err)
			}
			if exactGuard.Used() != exactLimit {
				t.Fatalf("exact fast-path work used = %d, want %d", exactGuard.Used(), exactLimit)
			}

			limitedGraph, nodes := test.build()
			snapshots := captureJoinPositions(nodes...)
			limitedGuard, err := limits.NewWorkGuard(context.Background(), "AutolayoutTransactions", exactLimit-1)
			if err != nil {
				t.Fatal(err)
			}
			if err := limitedGuard.Add(prechargedWork); err != nil {
				t.Fatal(err)
			}
			limitedContext := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), limitedGuard)

			err = JoinDistancedClusters(limitedContext, limitedGraph)
			if err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
				t.Fatalf("JoinDistancedClusters error = %v, want below-boundary shared work error", err)
			}
			if limitedGuard.Used() != exactLimit {
				t.Fatalf("limited fast-path work used = %d, want first rejected unit %d", limitedGuard.Used(), exactLimit)
			}
			assertJoinPositionsRestored(t, snapshots)
		})
	}
}

func TestJoinGuardedAPIsRejectNilWorkGuard(t *testing.T) {
	if _, err := layoutgraph.Nodes(nil).DistanceClustersWithWorkGuard(1, nil); err == nil {
		t.Fatal("DistanceClustersWithWorkGuard accepted a nil guard")
	}
	if _, err := layoutgraph.Nodes(nil).CenterWithWorkGuard(nil); err == nil {
		t.Fatal("CenterWithWorkGuard accepted a nil guard")
	}
	if _, _, err := layoutgraph.Nodes(nil).BoundingBoxWithWorkGuard(nil); err == nil {
		t.Fatal("BoundingBoxWithWorkGuard accepted a nil guard")
	}
	graph := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 1, 1)
	node.TopLeft = geo.NewPoint(0, 0)
	graph.AddNode(node)
	if _, err := graph.WouldOverlapWithWorkGuard(node, node.TopLeft, nil, nil, nil); err == nil {
		t.Fatal("WouldOverlapWithWorkGuard accepted a nil guard")
	}
	guard, err := limits.NewWorkGuard(context.Background(), "GuardedAPINilInputs", limits.MaxTransactionWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.WouldOverlapWithWorkGuard(nil, node.TopLeft, nil, nil, guard); err == nil {
		t.Fatal("WouldOverlapWithWorkGuard accepted a nil node")
	}
	if _, err := graph.WouldOverlapWithWorkGuard(node, nil, nil, nil, guard); err == nil {
		t.Fatal("WouldOverlapWithWorkGuard accepted a nil point")
	}
}
