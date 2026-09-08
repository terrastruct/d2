package routing

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func smallOVGBuildLimits() ovgBuildLimits {
	return ovgBuildLimits{
		intersectionCandidates: 100,
		nodes:                  100,
		edges:                  100,
		work:                   10_000,
	}
}

func requireOVGResourceError(t *testing.T, err error, resource string) {
	t.Helper()
	if !errors.Is(err, errOVGResourceLimit) {
		t.Fatalf("error = %v, want errOVGResourceLimit", err)
	}
	if !strings.Contains(err.Error(), resource) {
		t.Fatalf("error = %q, want resource %q", err, resource)
	}
}

func TestDefaultOVGBuildLimitsPreserveResourceCaps(t *testing.T) {
	limits := defaultOVGBuildLimits()
	if limits.intersectionCandidates != 1_000_000 || limits.nodes != 200_000 || limits.edges != 500_000 {
		t.Fatalf("default structural limits changed: candidates=%d nodes=%d edges=%d", limits.intersectionCandidates, limits.nodes, limits.edges)
	}
	if limits.work != 250_000_000 {
		t.Fatalf("default work limit = %d, want 250000000", limits.work)
	}

	guard, err := newOVGBuildGuard(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.reserveWork(limits.work); err != nil {
		t.Fatalf("exact default work budget must remain usable: %v", err)
	}
	requireOVGResourceError(t, guard.step(), "work units")
}

func TestOVGIntersectionLimitRejectsBeforeCartesianWork(t *testing.T) {
	limits := smallOVGBuildLimits()
	limits.intersectionCandidates = 3
	guard, err := newOVGBuildGuard(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	ovg := newBuildOVG(nil, guard)
	xs := map[float64]struct{}{0: {}, 1: {}}
	ys := map[float64]struct{}{0: {}, 1: {}}

	err = ovg.addIntersections(layoutgraph.NewGraph(), xs, ys, guard)
	requireOVGResourceError(t, err, "intersection candidate count")
	if len(ovg.Nodes) != 0 {
		t.Fatalf("OVG gained %d nodes after candidate preflight failure", len(ovg.Nodes))
	}
	if guard.work != 0 {
		t.Fatalf("work = %d, want zero before Cartesian product", guard.work)
	}
}

func TestOVGRequiredGuardCancellationPrecedesResourceChecks(t *testing.T) {
	limits := smallOVGBuildLimits()
	limits.intersectionCandidates = 0
	ctx, cancel := context.WithCancel(context.Background())
	guard, err := newOVGBuildGuard(ctx, limits)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	ovg := newBuildOVG(nil, guard)
	err = ovg.addIntersections(layoutgraph.NewGraph(), map[float64]struct{}{0: {}},
		map[float64]struct{}{0: {}},
		guard,
	)
	requireCanceledAt(t, err, "EdgeRouting")
	if guard.candidates != 0 || guard.work != 0 || len(ovg.Nodes) != 0 {
		t.Fatalf("canceled helper changed state: candidates=%d work=%d nodes=%d", guard.candidates, guard.work, len(ovg.Nodes))
	}
}

func TestOVGWorkLimitRejectsBeforeCartesianWork(t *testing.T) {
	limits := smallOVGBuildLimits()
	limits.work = 3
	guard, err := newOVGBuildGuard(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	ovg := newBuildOVG(nil, guard)
	xs := map[float64]struct{}{0: {}, 1: {}}
	ys := map[float64]struct{}{0: {}, 1: {}}

	err = ovg.addIntersections(layoutgraph.NewGraph(), xs, ys, guard)
	requireOVGResourceError(t, err, "work units")
	if len(ovg.Nodes) != 0 {
		t.Fatalf("OVG gained %d nodes after work preflight failure", len(ovg.Nodes))
	}
}

func TestOVGNodeAndEdgeLimitsBoundActualConstruction(t *testing.T) {
	t.Run("nodes", func(t *testing.T) {
		limits := smallOVGBuildLimits()
		limits.nodes = 1
		guard, err := newOVGBuildGuard(context.Background(), limits)
		if err != nil {
			t.Fatal(err)
		}
		ovg := newBuildOVG(nil, guard)
		xs := map[float64]struct{}{0: {}, 1: {}}
		ys := map[float64]struct{}{0: {}}

		err = ovg.addIntersections(layoutgraph.NewGraph(), xs, ys, guard)
		requireOVGResourceError(t, err, "node count")
		if len(ovg.Nodes) != 1 {
			t.Fatalf("nodes = %d, want construction capped at 1", len(ovg.Nodes))
		}
	})

	t.Run("edges", func(t *testing.T) {
		limits := smallOVGBuildLimits()
		limits.edges = 1
		guard, err := newOVGBuildGuard(context.Background(), limits)
		if err != nil {
			t.Fatal(err)
		}
		ovg := newBuildOVG(nil, guard)
		for _, x := range []float64{0, 10, 20} {
			if _, err := guard.addNode(ovg, NewOVGNode(geo.NewPoint(x, 0))); err != nil {
				t.Fatal(err)
			}
		}

		err = ovg.connectNodes(layoutgraph.NewGraph(), guard)
		requireOVGResourceError(t, err, "edge count")
		if len(ovg.Edges) != 1 {
			t.Fatalf("edges = %d, want construction capped at 1", len(ovg.Edges))
		}
	})
}

func newOVGHierarchyResourceGraph() (*layoutgraph.Graph, *layoutgraph.Hierarchy) {
	graph := layoutgraph.NewGraph()
	top := graph.AddNode(layoutgraph.NewNode(1, 80, 60))
	top.TopLeft = geo.NewPoint(0, 0)
	bottom := graph.AddNode(layoutgraph.NewNode(2, 80, 60))
	bottom.TopLeft = geo.NewPoint(0, 160)
	graph.Connect(top, bottom)
	graph.Directions[nil] = geo.Bottom
	hierarchy := newHierarchyWithLevels(map[*layoutgraph.Node]int{top: 0, bottom: 1})
	top.Hierarchy = hierarchy
	bottom.Hierarchy = hierarchy
	return graph, hierarchy
}

func TestOVGHierarchyWorkLimitStopsAfterRealConstruction(t *testing.T) {
	graph, hierarchy := newOVGHierarchyResourceGraph()
	baseline, err := newOVGBuildGuard(context.Background(), defaultOVGBuildLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newOVGForHierarchy(graph, hierarchy, baseline); err != nil {
		t.Fatal(err)
	}
	if baseline.nodes == 0 || baseline.work < 4 {
		t.Fatalf("baseline hierarchy accounting was vacuous: nodes=%d work=%d", baseline.nodes, baseline.work)
	}

	limits := defaultOVGBuildLimits()
	limits.work = baseline.work / 2
	limited, err := newOVGBuildGuard(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newOVGForHierarchy(graph, hierarchy, limited)
	requireOVGResourceError(t, err, "work units")
	if limited.nodes == 0 || limited.work == 0 {
		t.Fatalf("hierarchy limit failed before real construction: nodes=%d work=%d", limited.nodes, limited.work)
	}
}

func newOVGTunnelResourceGraph() *layoutgraph.Graph {
	graph := newOVGCancellationGraph(300, 0)
	graph.Connect(graph.Nodes[0], graph.Nodes[1])
	graph.Connect(graph.Nodes[0], graph.Nodes[1])
	return graph
}

func TestOVGTunnelWorkLimitStopsAfterDerivedNodeAllocation(t *testing.T) {
	graph := newOVGTunnelResourceGraph()
	baseline, err := newOVGBuildGuard(context.Background(), defaultOVGBuildLimits())
	if err != nil {
		t.Fatal(err)
	}
	tunnels, err := buildTunnels(graph, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(tunnels) == 0 || baseline.nodes == 0 || baseline.work < 2 {
		t.Fatalf("baseline tunnel accounting was vacuous: tunnels=%d nodes=%d work=%d", len(tunnels), baseline.nodes, baseline.work)
	}

	limits := defaultOVGBuildLimits()
	limits.work = baseline.work - 1
	limited, err := newOVGBuildGuard(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	_, err = buildTunnels(graph, limited)
	requireOVGResourceError(t, err, "work units")
	if limited.nodes == 0 || limited.work == 0 {
		t.Fatalf("tunnel limit failed before derived allocations: nodes=%d work=%d", limited.nodes, limited.work)
	}
}

func TestOVGFullBuilderWorkLimitStopsMidConstruction(t *testing.T) {
	graph := newOVGCancellationGraph(200, 100)
	baseline, err := newOVGBuildGuard(context.Background(), defaultOVGBuildLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildOVGFromGraphWithGuard(graph, nil, baseline); err != nil {
		t.Fatal(err)
	}
	if baseline.nodes == 0 || baseline.candidates == 0 || baseline.work < 4 {
		t.Fatalf("baseline builder accounting was vacuous: nodes=%d candidates=%d work=%d", baseline.nodes, baseline.candidates, baseline.work)
	}

	limits := defaultOVGBuildLimits()
	limits.work = baseline.work / 3
	limited, err := newOVGBuildGuard(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	_, err = buildOVGFromGraphWithGuard(graph, nil, limited)
	requireOVGResourceError(t, err, "work units")
	if limited.nodes == 0 || limited.candidates == 0 || limited.work == 0 {
		t.Fatalf("full-builder limit failed before real derived work: nodes=%d candidates=%d work=%d", limited.nodes, limited.candidates, limited.work)
	}
}

func TestOVGFullBuilderCancellationAfterDerivedNodeAllocation(t *testing.T) {
	graph := newOVGCancellationGraph(200, 100)
	var guard *ovgBuildGuard
	ctx := &cancelWhenOVGChanges{
		Context: context.Background(),
		shouldCancel: func() bool {
			return guard != nil && guard.nodes >= 5
		},
	}
	var err error
	guard, err = newOVGBuildGuard(ctx, defaultOVGBuildLimits())
	if err != nil {
		t.Fatal(err)
	}
	_, err = buildOVGFromGraphWithGuard(graph, nil, guard)
	requireCanceledAt(t, err, "EdgeRouting")
	if guard.nodes < 5 || guard.work == 0 {
		t.Fatalf("cancellation occurred before derived allocation: nodes=%d work=%d", guard.nodes, guard.work)
	}
}

func TestOVGBuildLimitsPreserveSuccessfulOutputOrder(t *testing.T) {
	relaxed := ovgBuildLimits{
		intersectionCandidates: math.MaxUint64,
		nodes:                  math.MaxUint64,
		edges:                  math.MaxUint64,
		work:                   math.MaxUint64,
	}
	referenceGraph, referenceEdge := newRoutingTestGraph(200, 100)
	if err := routeEdgesWithLimits(context.Background(), referenceGraph, []*layoutgraph.Edge{referenceEdge}, defaultOVGBuildLimits()); err != nil {
		t.Fatal(err)
	}
	actualGraph, actualEdge := newRoutingTestGraph(200, 100)
	if err := routeEdgesWithLimits(context.Background(), actualGraph, []*layoutgraph.Edge{actualEdge}, relaxed); err != nil {
		t.Fatal(err)
	}
	referencePoints := make([]geo.Point, len(referenceEdge.Points))
	actualPoints := make([]geo.Point, len(actualEdge.Points))
	for i, point := range referenceEdge.Points {
		referencePoints[i] = *point
	}
	for i, point := range actualEdge.Points {
		actualPoints[i] = *point
	}
	if !reflect.DeepEqual(actualPoints, referencePoints) {
		t.Fatalf("successful guarded routing changed route point order: got %v, want %v", actualPoints, referencePoints)
	}
	if actualEdge.LabelPercentage != referenceEdge.LabelPercentage {
		t.Fatalf("successful guarded routing changed metadata: got label=%v, want label=%v", actualEdge.LabelPercentage, referenceEdge.LabelPercentage)
	}
}

func TestCheckedOVGEdgeCapacityIs386Safe(t *testing.T) {
	if _, ok := limits.CheckedMulUint64(math.MaxUint64, 2); ok {
		t.Fatal("checked multiplication accepted uint64 overflow")
	}
	if _, ok := limits.CheckedAddUint64(math.MaxUint64, 1); ok {
		t.Fatal("checked addition accepted uint64 overflow")
	}

	capacity, err := checkedOVGEdgeCapacity(0, 6, 2, 3, 100, math.MaxInt32)
	if err != nil {
		t.Fatal(err)
	}
	if capacity != 7 {
		t.Fatalf("capacity = %d, want 7", capacity)
	}

	_, err = checkedOVGEdgeCapacity(0, 2_000_000_000, 1, 1, math.MaxUint64, math.MaxInt32)
	requireOVGResourceError(t, err, "allocation capacity")

	_, err = checkedOVGEdgeCapacity(0, math.MaxUint64, 1, 1, math.MaxUint64, math.MaxUint64)
	requireOVGResourceError(t, err, "edge capacity")
}

func TestRouteEdgesResourceFailurePreservesSelectedRoute(t *testing.T) {
	graph, edge := newRoutingTestGraph(200, 100)
	edge.Points = []*geo.Point{
		geo.NewPoint(40, 20),
		geo.NewPoint(80, 20),
		geo.NewPoint(80, 100),
		geo.NewPoint(200, 100),
	}
	edge.LabelPercentage = 0.375
	graph.CellSize = 123
	beforeSlice := edge.Points
	beforePointers := append([]*geo.Point(nil), edge.Points...)
	beforeValues := make([]geo.Point, len(edge.Points))
	for i, point := range edge.Points {
		beforeValues[i] = *point
	}

	limits := defaultOVGBuildLimits()
	limits.intersectionCandidates = 1
	err := routeEdgesWithLimits(context.Background(), graph, []*layoutgraph.Edge{edge}, limits)
	requireOVGResourceError(t, err, "intersection candidate count")

	if len(edge.Points) != len(beforeSlice) || &edge.Points[0] != &beforeSlice[0] {
		t.Fatalf("route slice identity changed after resource failure")
	}
	for i, point := range edge.Points {
		if point != beforePointers[i] || *point != beforeValues[i] {
			t.Fatalf("route point %d changed after resource failure: got %p %v, want %p %v", i, point, point, beforePointers[i], beforeValues[i])
		}
	}
	if edge.LabelPercentage != 0.375 {
		t.Fatalf("edge metadata changed after resource failure: labelPercentage=%v", edge.LabelPercentage)
	}
	if graph.CellSize != 123 {
		t.Fatalf("cell size changed after resource failure: got %v, want 123", graph.CellSize)
	}
}
