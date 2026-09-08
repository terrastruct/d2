package placement

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func compactionTinyGuardTestGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	graph.CellSize = 10
	moving := layoutgraph.NewNode(1, 10, 10)
	moving.TopLeft = geo.NewPoint(0, 0)
	graph.AddNewNodeToContainer(nil, moving)
	child := layoutgraph.NewNode(2, 1, 1)
	child.TopLeft = geo.NewPoint(1, 1)
	graph.AddNewNodeToContainer(moving, child)
	anchor := layoutgraph.NewNode(3, 10, 10)
	anchor.TopLeft = geo.NewPoint(100, 0)
	graph.AddNewNodeToContainer(nil, anchor)
	graph.Connect(moving, anchor)
	return graph
}

func compactionGuardTestGraph() (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Node) {
	graph := layoutgraph.NewGraph()
	moving := layoutgraph.NewNode(1, 80, 80)
	moving.TopLeft = geo.NewPoint(0, 0)
	graph.AddNewNodeToContainer(nil, moving)
	child := layoutgraph.NewNode(2, 10, 10)
	child.TopLeft = geo.NewPoint(10, 10)
	graph.AddNewNodeToContainer(moving, child)
	anchor := layoutgraph.NewNode(3, 50, 50)
	anchor.TopLeft = geo.NewPoint(200, 0)
	graph.AddNewNodeToContainer(nil, anchor)
	trailing := layoutgraph.NewNode(4, 50, 50)
	trailing.TopLeft = geo.NewPoint(400, 100)
	graph.AddNewNodeToContainer(nil, trailing)
	graph.Connect(anchor, moving)
	graph.Connect(moving, trailing)
	graph.ComputeCellSize()
	return graph, moving, child
}

func firstCompactionPassWork(ctx context.Context, graph *layoutgraph.Graph) (uint64, bool, error) {
	visibilityEdges, err := visibilityEdges(ctx, graph, true, true)
	if err != nil {
		return 0, false, err
	}
	inflateAlongAxis(graph, true, true, 1, visibilityEdges, false)
	for range 20 {
		changed, err := shiftSubgraphs(ctx, graph, true, true, 1, nil, visibilityEdges)
		if err != nil {
			return 0, false, err
		}
		if !changed {
			break
		}
	}
	guard, err := limits.NewOptimizationWorkGuard(ctx, "CompactionMoves", limits.MaxOptimizationWorkUnits)
	if err != nil {
		return 0, false, err
	}
	changed, err := compactAlongAxis(ctx, graph, true, true, 1, nil, visibilityEdges, guard)
	return guard.Used(), changed, err
}

func requireSameGraphNodeGeometry(t *testing.T, got, want *layoutgraph.Graph) {
	t.Helper()
	if len(got.Nodes) != len(want.Nodes) {
		t.Fatalf("node count = %d; want %d", len(got.Nodes), len(want.Nodes))
	}
	for index, gotNode := range got.Nodes {
		wantNode := want.Nodes[index]
		if gotNode.ID != wantNode.ID || gotNode.TopLeft == nil || wantNode.TopLeft == nil ||
			*gotNode.TopLeft != *wantNode.TopLeft || gotNode.Width != wantNode.Width || gotNode.Height != wantNode.Height {
			t.Fatalf("node %d geometry = %#v; want %#v", gotNode.ID, gotNode.Box, wantNode.Box)
		}
	}
}

func TestCompactionMoveGuardExactWorkBoundaryRestoresAcceptedMove(t *testing.T) {
	const cacheState = 0x41
	const cacheCost = 123.5
	options := compactionOptions{axis: horizontalAxis, includeSizes: true, factor: 1}
	baselineGraph, _, baselineChild := compactionGuardTestGraph()
	baselineChildBefore := snapshotPointer(baselineChild.TopLeft)
	if err := compaction(context.Background(), baselineGraph, options); err != nil {
		t.Fatal(err)
	}
	if baselineChild.TopLeft == baselineChildBefore.pointer && *baselineChild.TopLeft == baselineChildBefore.value {
		t.Fatal("representative compaction did not move the descendant")
	}
	baselineCosts := baselineGraph.RoutingCosts()
	if baselineCosts.Crossing == 0 && baselineCosts.Turn == 0 && baselineCosts.NonCenterPort == 0 {
		t.Fatal("representative compaction did not initialize routing costs")
	}

	minimum := uint64(1)
	maximum := limits.MaxOptimizationWorkUnits
	for minimum < maximum {
		middle := minimum + (maximum-minimum)/2
		graph, _, _ := compactionGuardTestGraph()
		options.moveWorkLimit = middle
		if err := compaction(context.Background(), graph, options); err == nil {
			maximum = middle
		} else if errors.Is(err, limits.ErrOptimizationResourceLimit) {
			minimum = middle + 1
		} else {
			t.Fatalf("compaction with work budget %d failed unexpectedly: %v", middle, err)
		}
	}
	if minimum < 2 {
		t.Fatalf("minimum compaction work = %d; want at least 2", minimum)
	}

	exactGraph, _, _ := compactionGuardTestGraph()
	options.moveWorkLimit = minimum
	if err := compaction(context.Background(), exactGraph, options); err != nil {
		t.Fatalf("compaction failed at exact work budget %d: %v", minimum, err)
	}
	requireSameGraphNodeGeometry(t, exactGraph, baselineGraph)

	firstPassGraph, _, _ := compactionGuardTestGraph()
	firstPassWork, changed, err := firstCompactionPassWork(context.Background(), firstPassGraph)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first compaction pass did not accept a move")
	}
	if minimum-1 <= firstPassWork {
		t.Fatalf("W-1 = %d does not complete the accepted first pass using %d work units", minimum-1, firstPassWork)
	}

	limitedGraph, limitedParent, limitedChild := compactionGuardTestGraph()
	limitedPositions := captureExactOptimizerPositions(limitedGraph)
	originalCosts := limitedGraph.RoutingCosts()
	limitedGraph.StoreEdgeLengthCost(cacheState, cacheCost)
	if cost, ok := limitedGraph.LookupEdgeLengthCost(cacheState); !ok || cost != cacheCost {
		t.Fatal("failed to seed placement cache")
	}
	cacheEntries := limitedGraph.EdgeLengthCacheEntries()
	options.moveWorkLimit = minimum - 1
	err = compaction(context.Background(), limitedGraph, options)
	requireOptimizationResourceError(t, err)
	requireExactOptimizerPositions(t, limitedPositions)
	if limitedParent.TopLeft != limitedPositions[limitedParent].pointer || limitedChild.TopLeft != limitedPositions[limitedChild].pointer {
		t.Fatal("compaction did not restore exact parent and descendant point identities")
	}
	if costs := limitedGraph.RoutingCosts(); costs != originalCosts {
		t.Fatalf("routing costs after rollback = %+v; want %+v", costs, originalCosts)
	}
	if got := limitedGraph.EdgeLengthCacheEntries(); got != cacheEntries {
		t.Fatalf("placement cache entries = %d; want %d", got, cacheEntries)
	}
	if cost, ok := limitedGraph.LookupEdgeLengthCost(cacheState); !ok || cost != cacheCost {
		t.Fatalf("placement cache sentinel = (%v, %v); want (%v, true)", cost, ok, cacheCost)
	}
}

type panicAtCompactionMoveGuard struct {
	context.Context
	graph             *layoutgraph.Graph
	originalPositions map[*layoutgraph.Node]exactOptimizerPosition
	originalCosts     layoutgraph.RoutingCostState
	observed          bool
}

func (ctx *panicAtCompactionMoveGuard) Err() error {
	var callers [32]uintptr
	count := runtime.Callers(2, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "limits.NewOptimizationWorkGuard") {
			geometryChanged := false
			for node, original := range ctx.originalPositions {
				if node.TopLeft != original.pointer || node.TopLeft == nil || *node.TopLeft != original.value {
					geometryChanged = true
					break
				}
			}
			costsChanged := ctx.graph.RoutingCosts() != ctx.originalCosts
			if geometryChanged && costsChanged {
				ctx.observed = true
				panic("compaction move guard probe")
			}
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

func TestCompactionMoveGuardPanicRestoresExactState(t *testing.T) {
	const cacheState = 0x42
	const cacheCost = 456.25
	graph, parent, child := compactionGuardTestGraph()
	positions := captureExactOptimizerPositions(graph)
	originalCosts := graph.RoutingCosts()
	graph.StoreEdgeLengthCost(cacheState, cacheCost)
	cacheEntries := graph.EdgeLengthCacheEntries()
	ctx := &panicAtCompactionMoveGuard{
		Context:           context.Background(),
		graph:             graph,
		originalPositions: positions,
		originalCosts:     originalCosts,
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = compaction(ctx, graph, compactionOptions{axis: horizontalAxis, includeSizes: true, factor: 1})
	}()
	if recovered != "compaction move guard probe" {
		t.Fatalf("panic = %v; want compaction move guard probe", recovered)
	}
	if !ctx.observed {
		t.Fatal("panic did not observe accepted geometry and initialized routing costs")
	}
	requireExactOptimizerPositions(t, positions)
	if parent.TopLeft != positions[parent].pointer || child.TopLeft != positions[child].pointer {
		t.Fatal("compaction panic did not restore exact parent and descendant point identities")
	}
	if costs := graph.RoutingCosts(); costs != originalCosts {
		t.Fatalf("routing costs after panic = %+v; want %+v", costs, originalCosts)
	}
	if got := graph.EdgeLengthCacheEntries(); got != cacheEntries {
		t.Fatalf("placement cache entries after panic = %d; want %d", got, cacheEntries)
	}
	if cost, ok := graph.LookupEdgeLengthCost(cacheState); !ok || cost != cacheCost {
		t.Fatalf("placement cache sentinel after panic = (%v, %v); want (%v, true)", cost, ok, cacheCost)
	}
}

type failOnCompactionMoveMutation struct {
	context.Context
	graph         *layoutgraph.Graph
	movePositions map[*layoutgraph.Node]exactOptimizerPosition
	observed      bool
	panic         bool
}

func (ctx *failOnCompactionMoveMutation) Err() error {
	var callers [32]uintptr
	count := runtime.Callers(2, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	insideMove := false
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, ".moveNodeToBest") {
			insideMove = true
			break
		}
		if !more {
			break
		}
	}
	if !insideMove {
		return ctx.Context.Err()
	}
	if ctx.movePositions == nil {
		ctx.movePositions = captureExactOptimizerPositions(ctx.graph)
		return ctx.Context.Err()
	}
	for node, original := range ctx.movePositions {
		if node.TopLeft != original.pointer || node.TopLeft == nil || *node.TopLeft != original.value {
			ctx.observed = true
			if ctx.panic {
				panic("compaction move mutation probe")
			}
			return context.Canceled
		}
	}
	return ctx.Context.Err()
}

func TestCompactionMoveCancellationAndPanicRestoreExactState(t *testing.T) {
	for _, test := range []struct {
		name  string
		panic bool
	}{
		{name: "cancellation"},
		{name: "panic", panic: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			const cacheState = 0x43
			const cacheCost = 789.75
			graph, parent, child := compactionGuardTestGraph()
			positions := captureExactOptimizerPositions(graph)
			originalCosts := graph.RoutingCosts()
			graph.StoreEdgeLengthCost(cacheState, cacheCost)
			cacheEntries := graph.EdgeLengthCacheEntries()
			ctx := &failOnCompactionMoveMutation{Context: context.Background(), graph: graph, panic: test.panic}

			var err error
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				err = compaction(ctx, graph, compactionOptions{axis: horizontalAxis, includeSizes: true, factor: 1})
			}()
			if test.panic {
				if recovered != "compaction move mutation probe" {
					t.Fatalf("panic = %v; want compaction move mutation probe", recovered)
				}
			} else {
				requireCanceledAt(t, err, "EdgeLength")
			}
			if !ctx.observed {
				t.Fatal("compaction did not reach a trial move mutation")
			}
			requireExactOptimizerPositions(t, positions)
			if parent.TopLeft != positions[parent].pointer || child.TopLeft != positions[child].pointer {
				t.Fatal("compaction did not restore exact parent and descendant point identities")
			}
			if costs := graph.RoutingCosts(); costs != originalCosts {
				t.Fatalf("routing costs after failure = %+v; want %+v", costs, originalCosts)
			}
			if got := graph.EdgeLengthCacheEntries(); got != cacheEntries {
				t.Fatalf("placement cache entries after failure = %d; want %d", got, cacheEntries)
			}
			if cost, ok := graph.LookupEdgeLengthCost(cacheState); !ok || cost != cacheCost {
				t.Fatalf("placement cache sentinel after failure = (%v, %v); want (%v, true)", cost, ok, cacheCost)
			}
		})
	}
}

func TestMoveNodeToBestPreservesGuardCancellationLocation(t *testing.T) {
	graph := compactionTinyGuardTestGraph()
	node := graph.Nodes[0]
	guardCtx := &cancelAfterErrChecks{Context: context.Background(), remaining: 1}
	guard, err := limits.NewOptimizationWorkGuard(guardCtx, "CompactionMoves", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}

	changed, err := moveNodeToBest(
		context.Background(),
		graph,
		node,
		[]*geo.Point{node.TopLeft.Copy()},
		nil,
		true,
		guard,
	)
	if changed {
		t.Fatal("canceled move reported a geometry change")
	}
	requireCanceledAt(t, err, "CompactionMoves")
}

func BenchmarkCompactionMoveScoringRepresentative(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		graph, _, _ := compactionGuardTestGraph()
		b.StartTimer()
		if err := compaction(ctx, graph, compactionOptions{axis: horizontalAxis, includeSizes: true, factor: 1}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompactionMoveScoringTiny(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		graph := compactionTinyGuardTestGraph()
		b.StartTimer()
		if err := compaction(ctx, graph, compactionOptions{axis: horizontalAxis, factor: 1}); err != nil {
			b.Fatal(err)
		}
	}
}
