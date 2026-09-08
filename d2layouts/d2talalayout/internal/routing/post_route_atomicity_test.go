package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type exactRouteTestSnapshot struct {
	edge   *layoutgraph.Edge
	route  exactTestSlice[*geo.Point]
	values map[*geo.Point]geo.Point
}

func captureExactRouteTest(edge *layoutgraph.Edge) exactRouteTestSnapshot {
	values := make(map[*geo.Point]geo.Point)
	for _, point := range edge.Points[:cap(edge.Points)] {
		if point != nil {
			values[point] = *point
		}
	}
	return exactRouteTestSnapshot{
		edge:   edge,
		route:  captureExactTestSlice(edge.Points),
		values: values,
	}
}

func (snapshot exactRouteTestSnapshot) changed() bool {
	points := snapshot.edge.Points
	if len(points) != len(snapshot.route.header) || cap(points) != cap(snapshot.route.header) {
		return true
	}
	if cap(points) > 0 && &points[:cap(points)][0] != &snapshot.route.header[:cap(snapshot.route.header)][0] {
		return true
	}
	for index, point := range points[:cap(points)] {
		if point != snapshot.route.backing[index] {
			return true
		}
	}
	for point, value := range snapshot.values {
		if *point != value {
			return true
		}
	}
	return false
}

func (snapshot exactRouteTestSnapshot) assertRestored(t *testing.T) {
	t.Helper()
	snapshot.route.assertRestored(t, snapshot.edge.Points, "edge route")
	for point, value := range snapshot.values {
		if *point != value {
			t.Fatalf("point %p = %+v; want %+v", point, *point, value)
		}
	}
}

type cancelWhenRouteMutates struct {
	context.Context
	snapshot exactRouteTestSnapshot
	observed bool
}

func (ctx *cancelWhenRouteMutates) Err() error {
	if ctx.snapshot.changed() {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

type panicWhenRouteMutates struct {
	context.Context
	snapshot exactRouteTestSnapshot
}

type observeRouteMutation struct {
	context.Context
	snapshot exactRouteTestSnapshot
	observed bool
}

func (ctx *observeRouteMutation) Err() error {
	if ctx.snapshot.changed() {
		ctx.observed = true
	}
	return ctx.Context.Err()
}

func (ctx *panicWhenRouteMutates) Err() error {
	if ctx.snapshot.changed() {
		panic("post-route mutation probe")
	}
	return ctx.Context.Err()
}

func routeWithSpareCapacity(points ...*geo.Point) []*geo.Point {
	backing := make([]*geo.Point, len(points)+3)
	copy(backing, points)
	for index := len(points); index < len(backing); index++ {
		backing[index] = geo.NewPoint(float64(10_000+index), float64(20_000+index))
	}
	return backing[:len(points)]
}

func simplificationMutationGraph() (*layoutgraph.Graph, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 10, 10)
	to := layoutgraph.NewNode(2, 10, 10)
	from.TopLeft = geo.NewPoint(1_000, 1_000)
	to.TopLeft = geo.NewPoint(2_000, 2_000)
	g.AddNewNodeToContainer(nil, from)
	g.AddNewNodeToContainer(nil, to)
	edge := g.Connect(from, to)
	edge.Points = routeWithSpareCapacity(
		geo.NewPoint(0, 0),
		geo.NewPoint(10, 0),
		geo.NewPoint(10, 10),
		geo.NewPoint(20, 10),
		geo.NewPoint(20, 0),
	)
	return g, edge
}

func balanceMutationGraph() (*layoutgraph.Graph, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	top := layoutgraph.NewNode(1, 1_000, 1_000)
	bottom := layoutgraph.NewNode(2, 1_000, 1_000)
	top.TopLeft = geo.NewPoint(0, 0)
	bottom.TopLeft = geo.NewPoint(0, 2_000)
	g.AddNewNodeToContainer(nil, top)
	g.AddNewNodeToContainer(nil, bottom)
	edge := g.Connect(top, bottom)
	edge.Points = routeWithSpareCapacity(geo.NewPoint(1, 1_000), geo.NewPoint(1, 2_000))
	return g, edge
}

func branchingMutationGraph() (*layoutgraph.Graph, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	external := layoutgraph.NewNode(1, 10, 10)
	left := layoutgraph.NewNode(2, 10, 10)
	right := layoutgraph.NewNode(3, 10, 10)
	external.TopLeft = geo.NewPoint(10_000, 10_000)
	left.TopLeft = geo.NewPoint(11_000, 11_000)
	right.TopLeft = geo.NewPoint(12_000, 12_000)
	for _, node := range []*layoutgraph.Node{external, left, right} {
		g.AddNewNodeToContainer(nil, node)
	}
	cluster := &layoutgraph.Cluster{
		Vessel:             layoutgraph.NewNode(4, 10, 10),
		Nodes:              []*layoutgraph.Node{left, right},
		Arrangement:        layoutgraph.Row,
		DesiredArrangement: layoutgraph.Row,
		Graph:              g,
	}
	cluster.Vessel.TopLeft = geo.NewPoint(11_000, 11_000)
	cluster.Vessel.Graph = g
	left.Cluster = cluster
	right.Cluster = cluster
	g.Clusters[cluster.Vessel] = cluster

	leftEdge := g.Connect(external, left)
	leftEdge.Points = routeWithSpareCapacity(
		geo.NewPoint(0, 0),
		geo.NewPoint(0, -10),
		geo.NewPoint(-100, -10),
		geo.NewPoint(-100, 100),
	)
	rightEdge := g.Connect(external, right)
	rightEdge.Points = routeWithSpareCapacity(
		geo.NewPoint(0, 0),
		geo.NewPoint(0, 10),
		geo.NewPoint(100, 10),
		geo.NewPoint(100, -5),
	)
	return g, leftEdge
}

func swapMutationGraph() (*layoutgraph.Graph, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	container := g.AddNode(layoutgraph.NewNode(1, 200, 100))
	container.TopLeft = geo.NewPoint(0, 150)
	remote := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	remote.TopLeft = geo.NewPoint(200, 0)
	node := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	node.TopLeft = geo.NewPoint(100, 175)

	first := g.Connect(node, remote)
	first.Points = routeWithSpareCapacity(
		geo.NewPoint(138, 175),
		geo.NewPoint(138, 25),
		geo.NewPoint(200, 25),
	)
	second := g.Connect(remote, node)
	second.Points = routeWithSpareCapacity(
		geo.NewPoint(225, 50),
		geo.NewPoint(225, 75),
		geo.NewPoint(112, 75),
		geo.NewPoint(112, 175),
	)
	g.AddNodeToContainer(nil, container)
	g.AddNodeToContainer(nil, remote)
	g.AddNodeToContainer(container, node)
	return g, first
}

func hostileSegmentGraph(vertical bool) (*layoutgraph.Graph, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 10, 10)
	to := layoutgraph.NewNode(2, 10, 10)
	from.TopLeft = geo.NewPoint(10_000, 10_000)
	to.TopLeft = geo.NewPoint(20_000, 20_000)
	g.AddNewNodeToContainer(nil, from)
	g.AddNewNodeToContainer(nil, to)
	edge := g.Connect(from, to)
	points := make([]*geo.Point, 0, 129)
	for index := 0; index < 129; index++ {
		if vertical {
			points = append(points, geo.NewPoint(0, float64(index)))
		} else {
			points = append(points, geo.NewPoint(float64(index), 0))
		}
	}
	edge.Points = routeWithSpareCapacity(points...)
	return g, edge
}

func TestPostRouteStagesRestoreExactRoutesOnLateCancellation(t *testing.T) {
	tests := []struct {
		name     string
		location string
		make     func() (*layoutgraph.Graph, *layoutgraph.Edge)
		run      func(context.Context, *layoutgraph.Graph) error
	}{
		{name: "simplify", location: "SimplifyEdgeRoutes", make: simplificationMutationGraph, run: func(ctx context.Context, graph *layoutgraph.Graph) error {
			return SimplifyEdgeRoutes(ctx, graph)
		}},
		{name: "balance", location: "BalanceEdgeSegments", make: balanceMutationGraph, run: func(ctx context.Context, graph *layoutgraph.Graph) error {
			return BalanceEdgeSegments(ctx, graph)
		}},
		{name: "cluster branching", location: "FixClusterEdgeBranching", make: branchingMutationGraph, run: func(ctx context.Context, graph *layoutgraph.Graph) error {
			return FixClusterEdgeBranching(ctx, graph)
		}},
		{name: "swap ports", location: "SwapEdgePorts", make: swapMutationGraph, run: func(ctx context.Context, graph *layoutgraph.Graph) error {
			return SwapAllEdgePorts(ctx, graph)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, edge := test.make()
			snapshot := captureExactRouteTest(edge)
			ctx := &cancelWhenRouteMutates{Context: context.Background(), snapshot: snapshot}
			err := test.run(ctx, graph)
			requireCanceledAt(t, err, test.location)
			if !ctx.observed {
				t.Fatal("cancellation probe did not observe a tentative route mutation")
			}
			snapshot.assertRestored(t)
		})
	}
}

func TestPostRouteStagePanicRestoresExactRoute(t *testing.T) {
	graph, edge := simplificationMutationGraph()
	snapshot := captureExactRouteTest(edge)
	ctx := &panicWhenRouteMutates{Context: context.Background(), snapshot: snapshot}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = SimplifyEdgeRoutes(ctx, graph)
	}()
	if fmt.Sprint(recovered) != "post-route mutation probe" {
		t.Fatalf("recovered = %v; want mutation probe", recovered)
	}
	snapshot.assertRestored(t)
}

func TestSwapEdgePortsDoesNotReportRolledBackSwap(t *testing.T) {
	graph, edge := swapMutationGraph()
	snapshot := captureExactRouteTest(edge)
	ctx := &cancelWhenRouteMutates{Context: context.Background(), snapshot: snapshot}
	swapped, err := swapEdgePorts(ctx, graph.Nodes[2])
	requireCanceledAt(t, err, "SwapEdgePorts")
	if swapped {
		t.Fatal("SwapEdgePorts reported a tentative swap that was rolled back")
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe the tentative swap")
	}
	snapshot.assertRestored(t)
}

func TestPostRouteWorkLimitAfterMutationRestoresExactRoute(t *testing.T) {
	graph, edge := simplificationMutationGraph()
	snapshot := captureExactRouteTest(edge)
	// Geometry validation consumes seven units and the first simplification
	// commits tentatively at unit twelve. The next guarded operation crosses
	// this injected limit, proving resource failures roll the mutation back.
	err := simplifyEdgeRoutesWithLimit(context.Background(), graph, 12)
	if err == nil || !strings.Contains(err.Error(), "work exceeds limit 12") {
		t.Fatalf("stage error = %v; want post-mutation work-limit error", err)
	}
	snapshot.assertRestored(t)
}

func TestPostRouteStagesEnforceInjectedWorkLimits(t *testing.T) {
	tests := []struct {
		name string
		make func() (*layoutgraph.Graph, *layoutgraph.Edge)
		run  func(*layoutgraph.Graph, *routeWorkGuard) error
	}{
		{name: "simplify hostile segments", make: func() (*layoutgraph.Graph, *layoutgraph.Edge) { return hostileSegmentGraph(false) }, run: func(graph *layoutgraph.Graph, guard *routeWorkGuard) error {
			_, err := simplifyPoints(graph, graph.Edges[0], guard)
			return err
		}},
		{name: "balance hostile segments", make: func() (*layoutgraph.Graph, *layoutgraph.Edge) { return hostileSegmentGraph(true) }, run: func(graph *layoutgraph.Graph, guard *routeWorkGuard) error {
			return balanceEdgeSegmentsGuarded(graph, guard)
		}},
		{name: "cluster hostile candidates", make: branchingMutationGraph, run: func(graph *layoutgraph.Graph, guard *routeWorkGuard) error {
			return fixClusterEdgeBranchingGuarded(graph, guard)
		}},
		{name: "swap hostile pairs", make: swapMutationGraph, run: func(graph *layoutgraph.Graph, guard *routeWorkGuard) error {
			for _, node := range graph.Nodes {
				if _, err := swapEdgePortsGuarded(node, guard); err != nil {
					return err
				}
			}
			return nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, _ := test.make()
			guard, err := newRouteWorkGuard(context.Background(), test.name, 64)
			if err != nil {
				t.Fatal(err)
			}
			err = test.run(graph, guard)
			if err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
				t.Fatalf("kernel error = %v; want work-limit error", err)
			}
		})
	}
}

func TestPostRouteStageWorkLimitsRollBackAfterMutation(t *testing.T) {
	tests := []struct {
		name string
		make func() (*layoutgraph.Graph, *layoutgraph.Edge)
		run  func(context.Context, *layoutgraph.Graph, uint64) error
	}{
		{name: "simplify", make: simplificationMutationGraph, run: func(ctx context.Context, graph *layoutgraph.Graph, limit uint64) error {
			return simplifyEdgeRoutesWithLimit(ctx, graph, limit)
		}},
		{name: "balance", make: balanceMutationGraph, run: func(ctx context.Context, graph *layoutgraph.Graph, limit uint64) error {
			return balanceEdgeSegmentsWithLimit(ctx, graph, limit)
		}},
		{name: "cluster branching", make: branchingMutationGraph, run: func(ctx context.Context, graph *layoutgraph.Graph, limit uint64) error {
			return fixClusterEdgeBranchingWithLimit(ctx, graph, limit)
		}},
		{name: "swap ports", make: swapMutationGraph, run: func(ctx context.Context, graph *layoutgraph.Graph, limit uint64) error {
			return swapAllEdgePortsWithWorkLimit(ctx, graph, limit)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for limit := uint64(1); limit <= 10_000; limit++ {
				graph, edge := test.make()
				snapshot := captureExactRouteTest(edge)
				ctx := &observeRouteMutation{Context: context.Background(), snapshot: snapshot}
				err := test.run(ctx, graph, limit)
				if err == nil {
					break
				}
				if errors.Is(err, errRouteStageWorkLimit) && ctx.observed {
					snapshot.assertRestored(t)
					return
				}
			}
			t.Fatal("no injected work limit was observed after a tentative route mutation")
		})
	}
}

func TestRouteStagePreflightCapsExtraEdgesBeforeSnapshot(t *testing.T) {
	graph := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 10, 10)
	to := layoutgraph.NewNode(2, 10, 10)
	from.TopLeft = geo.NewPoint(0, 0)
	to.TopLeft = geo.NewPoint(100, 0)
	edge := layoutgraph.NewEdge(from, to)
	edge.Points = make([]*geo.Point, 0, layoutgraph.MaxRoutePoints+1)
	tooManyReferences := make([]*layoutgraph.Edge, layoutgraph.MaxTopologyReferences+1)
	err := runAtomicRouteStage(context.Background(), "extra references", graph, tooManyReferences, maxRouteStageWorkUnits, func(*routeWorkGuard) error {
		t.Fatal("oversized extra-edge list reached the mutation callback")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "extra edge references exceed limit") {
		t.Fatalf("reference error = %v", err)
	}

	err = runAtomicRouteStage(context.Background(), "extra capacity", graph, []*layoutgraph.Edge{edge}, maxRouteStageWorkUnits, func(*routeWorkGuard) error {
		t.Fatal("oversized extra route reached the mutation callback")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "route point capacity exceeds limit") {
		t.Fatalf("capacity error = %v", err)
	}

	repeated := make([]*layoutgraph.Edge, 20)
	for index := range repeated {
		repeated[index] = edge
	}
	edge.Points = []*geo.Point{geo.NewPoint(0, 0), geo.NewPoint(100, 0)}
	err = runAtomicRouteStage(context.Background(), "extra duplicates", graph, repeated, 10, func(*routeWorkGuard) error {
		t.Fatal("repeated extra edges reached the mutation callback")
		return nil
	})
	if !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("duplicate-reference error = %v; want work limit", err)
	}
}

func TestRouteStagePanicRestoresGraphCachesAndNodePositions(t *testing.T) {
	graph, _ := simplificationMutationGraph()
	from := graph.Nodes[0]
	originalPosition := from.TopLeft
	originalValue := *originalPosition
	graph.CellSize = 17

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = runAtomicRouteStage(context.Background(), "cache rollback", graph, nil, maxRouteStageWorkUnits, func(guard *routeWorkGuard) error {
			graph.CellSize = 99
			graph.RestoreRoutingCosts(layoutgraph.RoutingCostState{Crossing: 101, Turn: 102, NonCenterPort: 103})
			from.TopLeft = geo.NewPoint(123, 456)
			if err := guard.step(); err != nil {
				return err
			}
			panic("route graph mutation probe")
		})
	}()
	if recovered != "route graph mutation probe" {
		t.Fatalf("recovered = %v", recovered)
	}
	costs := graph.RoutingCosts()
	if graph.CellSize != 17 || costs.Crossing != 0 || costs.Turn != 0 || costs.NonCenterPort != 0 {
		t.Fatalf("graph caches were not restored: cell=%v crossing=%v turn=%v port=%v", graph.CellSize, costs.Crossing, costs.Turn, costs.NonCenterPort)
	}
	if from.TopLeft != originalPosition || *from.TopLeft != originalValue {
		t.Fatalf("node position = %p %+v; want %p %+v", from.TopLeft, *from.TopLeft, originalPosition, originalValue)
	}
}

func TestRouteWorkGuardAdditionCannotOverflow(t *testing.T) {
	guard, err := newRouteWorkGuard(context.Background(), "test", ^uint64(0))
	if err != nil {
		t.Fatal(err)
	}
	guard.used = ^uint64(0) - 1
	if err := guard.add(1); err != nil {
		t.Fatal(err)
	}
	if err := guard.add(1); err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("overflow-boundary error = %v", err)
	}
}
