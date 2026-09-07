package placement

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

const optimizeClustersCacheSentinel uint64 = 0xc1a57e

type optimizeClustersFixture struct {
	graph   *layoutgraph.Graph
	cluster *layoutgraph.Cluster
	nodes   []*layoutgraph.Node
}

func addOptimizeClustersFixture(graph *layoutgraph.Graph, id layoutgraph.EntityID, x float64) optimizeClustersFixture {
	a := layoutgraph.NewNode(id, 200, 100)
	a.TopLeft = geo.NewPoint(x, 1000)
	c1 := layoutgraph.NewNode(id+2, 200, 100)
	c1.TopLeft = geo.NewPoint(x, 1220)
	c2 := layoutgraph.NewNode(id+3, 200, 100)
	c2.TopLeft = geo.NewPoint(x, 1440)
	b := layoutgraph.NewNode(id+1, 200, 100)
	b.TopLeft = geo.NewPoint(x, 1660)
	for _, node := range []*layoutgraph.Node{a, b, c1, c2} {
		graph.AddNewNodeToContainer(nil, node)
	}
	graph.Connect(a, c1)
	graph.Connect(a, c2)
	graph.Connect(c1, b)
	graph.Connect(c2, b)

	vessel := layoutgraph.NewNode(id+4, 200, 320)
	vessel.TopLeft = geo.NewPoint(x, 1220)
	cluster := &layoutgraph.Cluster{
		Nodes:              []*layoutgraph.Node{c1, c2},
		Graph:              graph,
		Arrangement:        layoutgraph.Column,
		DesiredArrangement: layoutgraph.Column,
		Vessel:             vessel,
	}
	vessel.SetClusterVessel(true)
	grouping.AddCluster(graph, cluster)
	abductClusterEdgesForOptimizationFixture(cluster)
	return optimizeClustersFixture{
		graph:   graph,
		cluster: cluster,
		nodes:   []*layoutgraph.Node{a, b, c1, c2, vessel},
	}
}

func newOptimizeClustersFixtures(count int) (*layoutgraph.Graph, []optimizeClustersFixture) {
	graph := layoutgraph.NewGraph()
	fixtures := make([]optimizeClustersFixture, 0, count)
	for i := 0; i < count; i++ {
		fixtures = append(fixtures, addOptimizeClustersFixture(graph, layoutgraph.EntityID(10*i+1), float64(1000+5000*i)))
	}
	graph.CellSize = 200
	graph.StoreEdgeLengthCost(optimizeClustersCacheSentinel, 17)
	graph.RestoreRoutingCosts(layoutgraph.RoutingCostState{Crossing: 3, Turn: 5, NonCenterPort: 7})
	return graph, fixtures
}

type optimizeClustersNodeSnapshot struct {
	topLeft pointerSnapshot[geo.Point]
	width   float64
	height  float64
}

type optimizeClustersClusterSnapshot struct {
	arrangement layoutgraph.ClusterArrangement
	desired     layoutgraph.ClusterArrangement
	padding     float64
}

type optimizeClustersStateSnapshot struct {
	graph      *layoutgraph.Graph
	nodes      map[*layoutgraph.Node]optimizeClustersNodeSnapshot
	clusters   map[*layoutgraph.Cluster]optimizeClustersClusterSnapshot
	cellSize   float64
	routeCosts layoutgraph.RoutingCostState
	cacheValue float64
	cacheCount int
}

func captureOptimizeClustersState(graph *layoutgraph.Graph, fixtures []optimizeClustersFixture) optimizeClustersStateSnapshot {
	nodes := make(map[*layoutgraph.Node]optimizeClustersNodeSnapshot)
	clusters := make(map[*layoutgraph.Cluster]optimizeClustersClusterSnapshot)
	for _, fixture := range fixtures {
		for _, node := range fixture.nodes {
			nodes[node] = optimizeClustersNodeSnapshot{
				topLeft: snapshotPointer(node.TopLeft),
				width:   node.Width,
				height:  node.Height,
			}
		}
		clusters[fixture.cluster] = optimizeClustersClusterSnapshot{
			arrangement: fixture.cluster.Arrangement,
			desired:     fixture.cluster.DesiredArrangement,
			padding:     fixture.cluster.Padding,
		}
	}
	cacheValue, _ := graph.LookupEdgeLengthCost(optimizeClustersCacheSentinel)
	return optimizeClustersStateSnapshot{
		graph:      graph,
		nodes:      nodes,
		clusters:   clusters,
		cellSize:   graph.CellSize,
		routeCosts: graph.RoutingCosts(),
		cacheValue: cacheValue,
		cacheCount: graph.EdgeLengthCacheEntries(),
	}
}

func (snapshot optimizeClustersStateSnapshot) assertRestored(t *testing.T) {
	t.Helper()
	for node, want := range snapshot.nodes {
		if node.TopLeft != want.topLeft.pointer || node.TopLeft == nil || *node.TopLeft != want.topLeft.value {
			t.Fatalf("node %d TopLeft = %p %+v; want %p %+v", node.ID, node.TopLeft, node.TopLeft, want.topLeft.pointer, want.topLeft.value)
		}
		if node.Width != want.width || node.Height != want.height {
			t.Fatalf("node %d dimensions = %vx%v; want %vx%v", node.ID, node.Width, node.Height, want.width, want.height)
		}
	}
	for cluster, want := range snapshot.clusters {
		if cluster.Arrangement != want.arrangement || cluster.DesiredArrangement != want.desired || cluster.Padding != want.padding {
			t.Fatalf(
				"cluster policy = (%q, %q, %v); want (%q, %q, %v)",
				cluster.Arrangement, cluster.DesiredArrangement, cluster.Padding,
				want.arrangement, want.desired, want.padding,
			)
		}
	}
	if snapshot.graph.CellSize != snapshot.cellSize {
		t.Fatalf("CellSize = %v; want %v", snapshot.graph.CellSize, snapshot.cellSize)
	}
	if got := snapshot.graph.RoutingCosts(); got != snapshot.routeCosts {
		t.Fatalf("routing costs = %+v; want %+v", got, snapshot.routeCosts)
	}
	cacheValue, ok := snapshot.graph.LookupEdgeLengthCost(optimizeClustersCacheSentinel)
	if !ok || cacheValue != snapshot.cacheValue || snapshot.graph.EdgeLengthCacheEntries() != snapshot.cacheCount {
		t.Fatalf(
			"edge-length cache = (%v, %v, %d entries); want (%v, true, %d entries)",
			cacheValue, ok, snapshot.graph.EdgeLengthCacheEntries(), snapshot.cacheValue, snapshot.cacheCount,
		)
	}
}

type interruptOptimizeClustersAtEdgeLength struct {
	context.Context
	cluster   *layoutgraph.Cluster
	panicWith any
	observed  bool
}

func (ctx *interruptOptimizeClustersAtEdgeLength) Err() error {
	if ctx.cluster.Arrangement == layoutgraph.Row {
		probe := &cancelWhenStackContains{Context: ctx.Context, function: "placementcost.EdgeLength"}
		if err := probe.Err(); err != nil {
			ctx.observed = true
			if ctx.panicWith != nil {
				panic(ctx.panicWith)
			}
			return err
		}
	}
	return ctx.Context.Err()
}

type interruptOptimizeClustersAfterCacheWrite struct {
	context.Context
	graph     *layoutgraph.Graph
	entrySize int
	observed  bool
}

func (ctx *interruptOptimizeClustersAfterCacheWrite) Err() error {
	if ctx.graph.EdgeLengthCacheEntries() > ctx.entrySize {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

type interruptOptimizeClustersAfterDesiredArrangement struct {
	context.Context
	earlier  *layoutgraph.Cluster
	cluster  *layoutgraph.Cluster
	observed bool
}

func (ctx *interruptOptimizeClustersAfterDesiredArrangement) Err() error {
	if ctx.earlier.Arrangement == layoutgraph.Row && ctx.cluster.DesiredArrangement == layoutgraph.Row {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

type interruptOptimizeClustersAfterCellSize struct {
	context.Context
	graph    *layoutgraph.Graph
	observed bool
}

func (ctx *interruptOptimizeClustersAfterCellSize) Err() error {
	if ctx.graph.CellSize != 0 {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestOptimizeClustersWorkLimitRestoresDesiredArrangement(t *testing.T) {
	graph, fixtures := newOptimizeClustersFixtures(1)
	snapshot := captureOptimizeClustersState(graph, fixtures)
	guard, err := limits.NewWorkGuard(context.Background(), "OptimizeClustersAtomicity", limits.MaxTransactionWorkUnits)
	require.NoError(t, err)
	guard.SetLimit(1)
	ctx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)

	changed, err := OptimizeClusters(ctx, graph)
	if changed {
		t.Fatal("OptimizeClusters reported a committed change after its work-limit error")
	}
	require.ErrorContains(t, err, "work exceeds limit 1")
	require.Equal(t, int64(2), guard.Used())
	snapshot.assertRestored(t)
}

func TestOptimizeClustersCancellationRestoresAcceptedFlip(t *testing.T) {
	graph, fixtures := newOptimizeClustersFixtures(1)
	snapshot := captureOptimizeClustersState(graph, fixtures)
	ctx := &interruptOptimizeClustersAtEdgeLength{Context: context.Background(), cluster: fixtures[0].cluster}

	changed, err := OptimizeClusters(ctx, graph)
	if changed {
		t.Fatal("OptimizeClusters reported a committed change after cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OptimizeClusters error = %v; want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe the accepted cluster flip")
	}
	snapshot.assertRestored(t)
}

func TestOptimizeClustersPanicRestoresAcceptedFlip(t *testing.T) {
	graph, fixtures := newOptimizeClustersFixtures(1)
	snapshot := captureOptimizeClustersState(graph, fixtures)
	sentinel := &struct{ name string }{name: "OptimizeClusters panic"}
	ctx := &interruptOptimizeClustersAtEdgeLength{
		Context: context.Background(), cluster: fixtures[0].cluster, panicWith: sentinel,
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = OptimizeClusters(ctx, graph)
	}()
	if recovered != sentinel {
		t.Fatalf("panic = %v; want exact sentinel %v", recovered, sentinel)
	}
	if !ctx.observed {
		t.Fatal("panic probe did not observe the accepted cluster flip")
	}
	snapshot.assertRestored(t)
}

func TestOptimizeClustersLaterClusterFailureRestoresEarlierCluster(t *testing.T) {
	graph, fixtures := newOptimizeClustersFixtures(2)
	snapshot := captureOptimizeClustersState(graph, fixtures)
	order := graph.ClusterRDFSOrder()
	require.Len(t, order, 2)
	require.Same(t, fixtures[0].cluster.Vessel, order[0])
	ctx := &interruptOptimizeClustersAfterDesiredArrangement{
		Context: context.Background(), earlier: fixtures[0].cluster, cluster: fixtures[1].cluster,
	}

	changed, err := OptimizeClusters(ctx, graph)
	if changed {
		t.Fatal("OptimizeClusters reported a committed change after later-cluster cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OptimizeClusters error = %v; want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not reach the second cluster")
	}
	snapshot.assertRestored(t)
}

func TestOptimizeClustersCancellationRestoresPlacementCosts(t *testing.T) {
	graph, fixtures := newOptimizeClustersFixtures(1)
	snapshot := captureOptimizeClustersState(graph, fixtures)
	ctx := &interruptOptimizeClustersAfterCacheWrite{
		Context: context.Background(), graph: graph, entrySize: graph.EdgeLengthCacheEntries(),
	}

	changed, err := OptimizeClusters(ctx, graph)
	if changed {
		t.Fatal("OptimizeClusters reported a committed change after scoring cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OptimizeClusters error = %v; want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe placement-cache population")
	}
	snapshot.assertRestored(t)
}

func TestOptimizeClustersCancellationRestoresComputedCellSize(t *testing.T) {
	graph, fixtures := newOptimizeClustersFixtures(1)
	// Offset both external nodes so Row alignment accepts a vessel move and
	// reaches the subsequent gap-reduction pass that computes CellSize.
	fixtures[0].nodes[0].TopLeft.X += 200
	fixtures[0].nodes[1].TopLeft.X += 200
	graph.CellSize = 0
	snapshot := captureOptimizeClustersState(graph, fixtures)
	ctx := &interruptOptimizeClustersAfterCellSize{Context: context.Background(), graph: graph}

	changed, err := OptimizeClusters(ctx, graph)
	if changed {
		t.Fatal("OptimizeClusters reported a committed change after gap-reduction cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OptimizeClusters error = %v; want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe CellSize computation")
	}
	snapshot.assertRestored(t)
}

func optimizeClustersWorkWithExtraCacheEntries(t *testing.T, extra int) int64 {
	t.Helper()
	graph, _ := newOptimizeClustersFixtures(2)
	for i := 0; i < extra; i++ {
		graph.StoreEdgeLengthCost(optimizeClustersCacheSentinel+uint64(i)+1, float64(i))
	}
	guard, err := limits.NewWorkGuard(context.Background(), "OptimizeClustersCacheCharge", limits.MaxTransactionWorkUnits)
	require.NoError(t, err)
	ctx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)
	_, err = OptimizeClusters(ctx, graph)
	require.NoError(t, err)
	return guard.Used()
}

func TestOptimizeClustersChargesPlacementCostSnapshotOnce(t *testing.T) {
	const extra = 8
	withoutExtra := optimizeClustersWorkWithExtraCacheEntries(t, 0)
	withExtra := optimizeClustersWorkWithExtraCacheEntries(t, extra)
	require.Equal(t, int64(extra), withExtra-withoutExtra)
}

func TestOptimizeClustersRejectedFlipRetainsDesiredArrangement(t *testing.T) {
	graph, fixtures := newOptimizeClustersFixtures(1)
	fixture := fixtures[0]
	leftBlocker := layoutgraph.NewNode(100, 90, 100)
	leftBlocker.TopLeft = geo.NewPoint(900, 1220)
	rightBlocker := layoutgraph.NewNode(101, 200, 100)
	rightBlocker.TopLeft = geo.NewPoint(1210, 1220)
	graph.AddNewNodeToContainer(nil, leftBlocker)
	graph.AddNewNodeToContainer(nil, rightBlocker)

	changed, err := OptimizeClusters(context.Background(), graph)
	require.NoError(t, err)
	if changed {
		t.Fatal("OptimizeClusters reported a committed change for rejected flip candidates")
	}
	require.Equal(t, layoutgraph.Column, fixture.cluster.Arrangement)
	require.Equal(t, layoutgraph.Row, fixture.cluster.DesiredArrangement)
}

func TestOptimizeClustersSuccessCommitsStage(t *testing.T) {
	graph, fixtures := newOptimizeClustersFixtures(1)
	pointers := make(map[*layoutgraph.Node]*geo.Point)
	for _, node := range fixtures[0].nodes {
		pointers[node] = node.TopLeft
	}

	changed, err := OptimizeClusters(context.Background(), graph)
	require.NoError(t, err)
	if !changed {
		t.Fatal("OptimizeClusters did not report its accepted cluster change")
	}
	require.Equal(t, layoutgraph.Row, fixtures[0].cluster.Arrangement)
	require.Equal(t, layoutgraph.Row, fixtures[0].cluster.DesiredArrangement)
	for node, pointer := range pointers {
		if node.TopLeft != pointer {
			t.Fatalf("node %d TopLeft pointer changed", node.ID)
		}
	}
}

func TestOptimizeClustersEmptyGraphPreservesContextPreflight(t *testing.T) {
	changed, err := OptimizeClusters(context.Background(), layoutgraph.NewGraph())
	require.NoError(t, err)
	if changed {
		t.Fatal("OptimizeClusters changed an empty graph")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = OptimizeClusters(canceled, layoutgraph.NewGraph())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OptimizeClusters error = %v; want context.Canceled", err)
	}
}
