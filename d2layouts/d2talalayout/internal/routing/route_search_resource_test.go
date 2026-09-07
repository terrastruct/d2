package routing

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func requireRouteSearchLimit(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, errRouteSearchWorkLimit) {
		t.Fatalf("error = %v, want errRouteSearchWorkLimit", err)
	}
}

func TestRouteSearchGuardCancellationPrecedesOverflow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	guard, err := newRouteSearchWorkGuard(ctx, Default, math.MaxUint64)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	err = guard.reserveProduct(math.MaxUint64, 2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestRouteSearchGuardRejectsOverflow(t *testing.T) {
	guard, err := newRouteSearchWorkGuard(context.Background(), Default, math.MaxUint64)
	if err != nil {
		t.Fatal(err)
	}
	guard.used = math.MaxUint64
	requireRouteSearchLimit(t, guard.step())
	if guard.used != math.MaxUint64 {
		t.Fatalf("overflow changed used work to %d", guard.used)
	}

	guard.used = 0
	requireRouteSearchLimit(t, guard.reserveProduct(math.MaxUint64, 2))
	if guard.used != 0 {
		t.Fatalf("overflowing product consumed %d work", guard.used)
	}

	requireRouteSearchLimit(t, guard.reserveSum(math.MaxUint64, 1))
	if guard.used != 0 {
		t.Fatalf("overflowing sum consumed %d work", guard.used)
	}
}

func TestRouteSearchRejectsBeforeEdgeSliceAllocation(t *testing.T) {
	graph, _ := newRoutingTestGraph(100, 100)
	telemetry := &routeSearchTelemetry{}
	ctx := withRouteSearchTelemetry(context.Background(), telemetry)
	_, err := newOVGEdgeRouterWithWorkLimit(ctx, Default, NewOVG(nil), graph, nil, graph.Edges, 0)
	requireRouteSearchLimit(t, err)
	samples := telemetry.snapshot()
	if len(samples) != 1 || samples[0] != 0 {
		t.Fatalf("constructor telemetry = %+v, want one zero-work preallocation rejection", samples)
	}
}

func TestAddRouteWorkLimitStopsAfterRealIndexing(t *testing.T) {
	graph, edge := newRoutingTestGraph(100, 100)
	ovg, err := buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newOVGEdgeRouter(context.Background(), Default, ovg, graph, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _, err := router.search(context.Background(), edge)
	if err != nil {
		t.Fatal(err)
	}
	route := &Route{
		GEdge:    edge,
		OVGNodes: nodes,
		FromPort: *nodes[1].Point,
		ToPort:   *nodes[len(nodes)-2].Point,
	}

	start := router.work.used
	router.work.limit = start + uint64(len(nodes)) + 2
	err = router.addRoute(route)
	requireRouteSearchLimit(t, err)
	if router.work.used <= start {
		t.Fatalf("addRoute consumed no real work: start=%d used=%d", start, router.work.used)
	}
	if len(router.pointToRoute) == 0 {
		t.Fatal("addRoute limit fired before any route index mutation")
	}
}

func TestRouteFinalizationWorkLimitIsAtomic(t *testing.T) {
	graph, edge := newRoutingTestGraph(100, 100)
	ovg, err := buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newOVGEdgeRouter(context.Background(), Default, ovg, graph, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _, err := router.search(context.Background(), edge)
	if err != nil {
		t.Fatal(err)
	}
	route := &Route{
		GEdge:    edge,
		OVGNodes: nodes,
		FromPort: *nodes[1].Point,
		ToPort:   *nodes[len(nodes)-2].Point,
	}
	before := make([]*geoPointSnapshot, len(edge.Points))
	for index, point := range edge.Points {
		before[index] = newGeoPointSnapshot(point)
	}
	guard, err := newRouteSearchWorkGuard(context.Background(), Default, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = finalizeSelectedRoutes(context.Background(), graph, []*Route{route}, nil, guard, false)
	requireRouteSearchLimit(t, err)
	if guard.used == 0 {
		t.Fatal("finalization limit fired without executing real work")
	}
	if !equalGeoPointSnapshots(before, edge.Points) {
		t.Fatal("route finalization changed edge points before its fallible work completed")
	}
}

func TestRouteFinalizationRejectsMalformedSelectedSetAtomically(t *testing.T) {
	graph, edge := newRoutingTestGraph(100, 100)
	edge.Points = []*geo.Point{geo.NewPoint(0, 50), geo.NewPoint(100, 50)}
	route := &Route{
		GEdge: edge,
		OVGNodes: []*OVGNode{
			NewOVGNode(geo.NewPoint(0, 50)),
			NewOVGNode(geo.NewPoint(25, 50)),
			NewOVGNode(geo.NewPoint(75, 50)),
			NewOVGNode(geo.NewPoint(100, 50)),
		},
	}
	selected := map[*layoutgraph.Edge]struct{}{edge: {}}
	tests := []struct {
		name   string
		routes []*Route
	}{
		{name: "nil route", routes: []*Route{nil, route}},
		{name: "duplicate route", routes: []*Route{route, route}},
		{name: "omitted route"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := make([]*geoPointSnapshot, len(edge.Points))
			for index, point := range edge.Points {
				before[index] = newGeoPointSnapshot(point)
			}
			guard, err := newRouteSearchWorkGuard(context.Background(), Default, maxRouteSearchWorkUnits)
			if err != nil {
				t.Fatal(err)
			}
			err = finalizeSelectedRoutes(context.Background(), graph, test.routes, selected, guard, false)
			if err == nil {
				t.Fatal("malformed selected route set was accepted")
			}
			if !equalGeoPointSnapshots(before, edge.Points) {
				t.Fatal("malformed selected route set changed the graph")
			}
		})
	}
}

func TestRouteSearchLimitPreservesGraphRoutes(t *testing.T) {
	graph, edge := newRoutingTestGraph(600, 420)
	// Force substantial real search work without coupling this atomicity test to
	// a serialized intermediate pipeline state.
	for row := 0; row < 5; row++ {
		for column := 0; column < 5; column++ {
			obstacle := layoutgraph.NewNode(layoutgraph.EntityID(3+row*5+column), 50, 50)
			obstacle.TopLeft = geo.NewPoint(float64(100+column*90), float64(60+row*70))
			graph.AddNode(obstacle)
		}
	}
	edge.Points = []*geo.Point{
		geo.NewPoint(40, 20),
		geo.NewPoint(70, 20),
		geo.NewPoint(70, -40),
		geo.NewPoint(620, -40),
		geo.NewPoint(620, 420),
	}
	before := make(map[*layoutgraph.Edge][]*geoPointSnapshot, len(graph.Edges))
	for _, edge := range graph.Edges {
		points := make([]*geoPointSnapshot, len(edge.Points))
		for index, point := range edge.Points {
			points[index] = newGeoPointSnapshot(point)
		}
		before[edge] = points
	}

	telemetry := &routeSearchTelemetry{}
	ctx := withRouteSearchTelemetry(withTestLogger(context.Background(), t), telemetry)
	_, err := routeEdgesWithSearchWorkLimit(ctx, graph, nil, 5_000)
	requireRouteSearchLimit(t, err)
	samples := telemetry.snapshot()
	if len(samples) == 0 {
		t.Fatal("route-search failure produced no telemetry")
	}
	var observedWork bool
	for _, sample := range samples {
		observedWork = observedWork || sample > 0
	}
	if !observedWork {
		t.Fatal("route-search failure occurred without executing real guarded work")
	}
	for _, edge := range graph.Edges {
		if !equalGeoPointSnapshots(before[edge], edge.Points) {
			t.Fatalf("edge %s changed after route-search limit", edge.DebugID())
		}
	}
}

type geoPointSnapshot struct {
	pointer any
	x, y    float64
}

func newGeoPointSnapshot(point *geo.Point) *geoPointSnapshot {
	if point == nil {
		return &geoPointSnapshot{}
	}
	return &geoPointSnapshot{pointer: point, x: point.X, y: point.Y}
}

func equalGeoPointSnapshots(want []*geoPointSnapshot, got []*geo.Point) bool {
	if len(want) != len(got) {
		return false
	}
	for index, snapshot := range want {
		if snapshot.pointer != got[index] {
			return false
		}
		if got[index] != nil && (snapshot.x != got[index].X || snapshot.y != got[index].Y) {
			return false
		}
	}
	return true
}

func TestRouteFlavorCountBoundsWholeOperation(t *testing.T) {
	routers := append(routeFlavorTestRouters(), &ovgEdgeRouter{flavor: TopDownLeftRight})
	responses, err := generateRouteFlavorResponsesWith(
		context.Background(),
		routers,
		false,
		func(*ovgEdgeRouter, context.Context, bool) GenerateRouteResponse {
			t.Fatal("worker ran after flavor-count preflight")
			return GenerateRouteResponse{}
		},
	)
	requireRouteSearchLimit(t, err)
	if responses != nil {
		t.Fatalf("responses = %v, want nil", responses)
	}
}
