package placement

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/lib/geo"
)

const normalizeGapsCacheSentinel uint64 = 0x6e6f726d67617073

type normalizeGapsNodeSnapshot struct {
	topLeft pointerSnapshot[geo.Point]
	width   float64
	height  float64
}

type normalizeGapsStateSnapshot struct {
	graph      *layoutgraph.Graph
	nodes      map[*layoutgraph.Node]normalizeGapsNodeSnapshot
	cellSize   float64
	routeCosts layoutgraph.RoutingCostState
	cache      map[uint64]float64
	cacheCount int
}

func newNormalizeGapsAtomicityGraph(t *testing.T, extraCacheEntries int) (*layoutgraph.Graph, *layoutgraph.Node) {
	t.Helper()
	graph := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(1, 10, 10)
	a.TopLeft = geo.NewPoint(0, 0)
	b := layoutgraph.NewNode(2, 10, 10)
	b.TopLeft = geo.NewPoint(1000, 0)
	graph.AddNode(a)
	graph.AddNode(b)
	graph.Connect(a, b)
	graph.StoreEdgeLengthCost(normalizeGapsCacheSentinel, 17)
	for i := 0; i < extraCacheEntries; i++ {
		graph.StoreEdgeLengthCost(normalizeGapsCacheSentinel+uint64(i)+1, float64(i))
	}
	graph.RestoreRoutingCosts(layoutgraph.RoutingCostState{Crossing: 3, Turn: 5, NonCenterPort: 7})
	return graph, b
}

func newNormalizeGapsNoScoreBeforeScoreGraph(t *testing.T) (*layoutgraph.Graph, *layoutgraph.Node) {
	t.Helper()
	graph := layoutgraph.NewGraph()
	moving := layoutgraph.NewNode(1, 10, 10)
	moving.TopLeft = geo.NewPoint(1000, 0)
	anchor := layoutgraph.NewNode(2, 10, 10)
	anchor.TopLeft = geo.NewPoint(0, 0)
	graph.AddNode(moving)
	graph.AddNode(anchor)
	graph.Connect(anchor, moving)
	graph.StoreEdgeLengthCost(normalizeGapsCacheSentinel, 17)
	graph.RestoreRoutingCosts(layoutgraph.RoutingCostState{Crossing: 3, Turn: 5, NonCenterPort: 7})
	return graph, moving
}

func captureNormalizeGapsState(graph *layoutgraph.Graph) normalizeGapsStateSnapshot {
	nodes := make(map[*layoutgraph.Node]normalizeGapsNodeSnapshot, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodes[node] = normalizeGapsNodeSnapshot{
			topLeft: snapshotPointer(node.TopLeft),
			width:   node.Width,
			height:  node.Height,
		}
	}
	cacheCount := graph.EdgeLengthCacheEntries()
	cache := make(map[uint64]float64, cacheCount)
	for i := 0; i < cacheCount; i++ {
		key := normalizeGapsCacheSentinel + uint64(i)
		value, ok := graph.LookupEdgeLengthCost(key)
		if ok {
			cache[key] = value
		}
	}
	return normalizeGapsStateSnapshot{
		graph:      graph,
		nodes:      nodes,
		cellSize:   graph.CellSize,
		routeCosts: graph.RoutingCosts(),
		cache:      cache,
		cacheCount: cacheCount,
	}
}

func (snapshot normalizeGapsStateSnapshot) assertRestored(t *testing.T) {
	t.Helper()
	for node, want := range snapshot.nodes {
		if node.TopLeft != want.topLeft.pointer || node.TopLeft == nil || *node.TopLeft != want.topLeft.value {
			t.Fatalf("node %d TopLeft = %p %+v; want %p %+v", node.ID, node.TopLeft, node.TopLeft, want.topLeft.pointer, want.topLeft.value)
		}
		if node.Width != want.width || node.Height != want.height {
			t.Fatalf("node %d dimensions = %vx%v; want %vx%v", node.ID, node.Width, node.Height, want.width, want.height)
		}
	}
	if snapshot.graph.CellSize != snapshot.cellSize {
		t.Fatalf("CellSize = %v; want %v", snapshot.graph.CellSize, snapshot.cellSize)
	}
	if got := snapshot.graph.RoutingCosts(); got != snapshot.routeCosts {
		t.Fatalf("routing costs = %+v; want %+v", got, snapshot.routeCosts)
	}
	if got := snapshot.graph.EdgeLengthCacheEntries(); got != snapshot.cacheCount {
		t.Fatalf("edge-length cache has %d entries; want %d", got, snapshot.cacheCount)
	}
	for key, want := range snapshot.cache {
		got, ok := snapshot.graph.LookupEdgeLengthCost(key)
		if !ok || got != want {
			t.Fatalf("edge-length cache[%x] = (%v, %v); want (%v, true)", key, got, ok, want)
		}
	}
}

func normalizeGapsStackContains(function string) bool {
	var callers [32]uintptr
	count := runtime.Callers(2, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, function) {
			return true
		}
		if !more {
			return false
		}
	}
}

type interruptNormalizeGapsAfterCommit struct {
	context.Context
	node      *layoutgraph.Node
	original  geo.Point
	panicWith any
	observed  bool
}

func (ctx *interruptNormalizeGapsAfterCommit) Err() error {
	if ctx.node.TopLeft == nil || *ctx.node.TopLeft == ctx.original {
		return ctx.Context.Err()
	}
	if normalizeGapsStackContains("CloneGeometryContext") && !normalizeGapsStackContains("reduceGapToNeighbors") {
		ctx.observed = true
		if ctx.panicWith != nil {
			panic(ctx.panicWith)
		}
		return context.Canceled
	}
	return ctx.Context.Err()
}

type interruptNormalizeGapsAtFinalPoll struct {
	context.Context
	graph     *layoutgraph.Graph
	entryTurn float64
	observed  bool
}

func (ctx *interruptNormalizeGapsAtFinalPoll) Err() error {
	if ctx.graph.RoutingCosts().Turn != ctx.entryTurn && normalizeGapsStackContains("placement.NormalizeGaps") {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestNormalizeGapsCancellationAfterCommitRestoresStage(t *testing.T) {
	graph, moving := newNormalizeGapsAtomicityGraph(t, 0)
	snapshot := captureNormalizeGapsState(graph)
	ctx := &interruptNormalizeGapsAfterCommit{
		Context:  context.Background(),
		node:     moving,
		original: *moving.TopLeft,
	}

	changed, err := NormalizeGaps(ctx, graph)
	if changed {
		t.Fatal("NormalizeGaps reported a committed change after cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NormalizeGaps error = %v; want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe the first accepted move")
	}
	snapshot.assertRestored(t)
}

func TestNormalizeGapsCancellationAfterNoScoreCommitRestoresStage(t *testing.T) {
	graph, moving := newNormalizeGapsNoScoreBeforeScoreGraph(t)
	snapshot := captureNormalizeGapsState(graph)
	ctx := &interruptNormalizeGapsAfterCommit{
		Context:  context.Background(),
		node:     moving,
		original: *moving.TopLeft,
	}

	changed, err := NormalizeGaps(ctx, graph)
	if changed {
		t.Fatal("NormalizeGaps reported a committed change after cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NormalizeGaps error = %v; want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe the accepted move after a no-score commit")
	}
	snapshot.assertRestored(t)
}

func TestNormalizeGapsPanicAfterCommitRestoresStage(t *testing.T) {
	graph, moving := newNormalizeGapsAtomicityGraph(t, 0)
	snapshot := captureNormalizeGapsState(graph)
	sentinel := &struct{ name string }{name: "NormalizeGaps panic"}
	ctx := &interruptNormalizeGapsAfterCommit{
		Context: context.Background(), node: moving, original: *moving.TopLeft, panicWith: sentinel,
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = NormalizeGaps(ctx, graph)
	}()
	if recovered != sentinel {
		t.Fatalf("panic = %v; want exact sentinel %v", recovered, sentinel)
	}
	if !ctx.observed {
		t.Fatal("panic probe did not observe the first accepted move")
	}
	snapshot.assertRestored(t)
}

func TestNormalizeGapsFinalPollRestoresNoScoreState(t *testing.T) {
	graph := layoutgraph.NewGraph()
	graph.StoreEdgeLengthCost(normalizeGapsCacheSentinel, 17)
	graph.RestoreRoutingCosts(layoutgraph.RoutingCostState{Crossing: 3, Turn: 5, NonCenterPort: 7})
	snapshot := captureNormalizeGapsState(graph)
	ctx := &interruptNormalizeGapsAtFinalPoll{
		Context: context.Background(), graph: graph, entryTurn: snapshot.routeCosts.Turn,
	}

	changed, err := NormalizeGaps(ctx, graph)
	if changed {
		t.Fatal("NormalizeGaps reported a committed change after final-poll cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NormalizeGaps error = %v; want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not reach the final poll after ResetTurnCost")
	}
	snapshot.assertRestored(t)
}

func TestNormalizeGapsSuccessCommitsStage(t *testing.T) {
	graph, moving := newNormalizeGapsAtomicityGraph(t, 0)
	pointer := moving.TopLeft

	changed, err := NormalizeGaps(context.Background(), graph)
	require.NoError(t, err)
	if !changed {
		t.Fatal("NormalizeGaps did not report its accepted move")
	}
	if moving.TopLeft != pointer {
		t.Fatal("NormalizeGaps replaced the moved node's TopLeft pointer")
	}
	require.Equal(t, 0.0+10.0+placementcost.IdealGapSize, moving.TopLeft.X)
	require.Equal(t, 10.0, graph.CellSize)
}

func normalizeGapsWorkWithExtraCacheEntries(t *testing.T, extra int) int64 {
	t.Helper()
	graph, _ := newNormalizeGapsAtomicityGraph(t, extra)
	guard, err := limits.NewWorkGuard(context.Background(), "NormalizeGapsCacheCharge", limits.MaxTransactionWorkUnits)
	require.NoError(t, err)
	ctx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)
	changed, err := NormalizeGaps(ctx, graph)
	require.NoError(t, err)
	require.True(t, changed)
	return guard.Used()
}

func normalizeGapsNoScoreWorkWithExtraCacheEntries(t *testing.T, extra int) int64 {
	t.Helper()
	graph := layoutgraph.NewGraph()
	for i := 0; i < extra; i++ {
		graph.StoreEdgeLengthCost(normalizeGapsCacheSentinel+uint64(i), float64(i))
	}
	guard, err := limits.NewWorkGuard(context.Background(), "NormalizeGapsNoScoreCacheCharge", limits.MaxTransactionWorkUnits)
	require.NoError(t, err)
	ctx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)
	changed, err := NormalizeGaps(ctx, graph)
	require.NoError(t, err)
	require.False(t, changed)
	return guard.Used()
}

func TestNormalizeGapsChargesPlacementCostSnapshotOnce(t *testing.T) {
	const extra = 8
	require.Equal(t,
		normalizeGapsNoScoreWorkWithExtraCacheEntries(t, 0),
		normalizeGapsNoScoreWorkWithExtraCacheEntries(t, extra),
	)

	withoutExtra := normalizeGapsWorkWithExtraCacheEntries(t, 0)
	withExtra := normalizeGapsWorkWithExtraCacheEntries(t, extra)
	require.Equal(t, int64(extra), withExtra-withoutExtra)

	graph, _ := newNormalizeGapsAtomicityGraph(t, extra)
	snapshot := captureNormalizeGapsState(graph)
	guard, err := limits.NewWorkGuard(context.Background(), "NormalizeGapsCacheCharge", limits.MaxTransactionWorkUnits)
	require.NoError(t, err)
	guard.SetLimit(withExtra - 1)
	ctx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)

	changed, err := NormalizeGaps(ctx, graph)
	if changed {
		t.Fatal("NormalizeGaps reported a committed change after its work-limit error")
	}
	require.Error(t, err)
	require.Equal(t, withExtra, guard.Used())
	snapshot.assertRestored(t)
}

func TestNormalizeGapsPreservesContextAndGraphPreflightOrder(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		changed, err := NormalizeGaps(context.Background(), layoutgraph.NewGraph())
		require.NoError(t, err)
		if changed {
			t.Fatal("NormalizeGaps changed an empty graph")
		}
	})

	t.Run("nil context", func(t *testing.T) {
		//lint:ignore SA1012 This test verifies the nil-context preflight error.
		_, err := NormalizeGaps(nil, nil)
		require.ErrorContains(t, err, "requires a context")
	})

	t.Run("pre-canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := NormalizeGaps(ctx, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("NormalizeGaps error = %v; want context.Canceled", err)
		}
	})

	t.Run("nil graph", func(t *testing.T) {
		_, err := NormalizeGaps(context.Background(), nil)
		require.ErrorContains(t, err, "transaction graph is nil")
	})
}
