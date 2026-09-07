package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func newStandaloneTableEdgeTestGraph() (*layoutgraph.Graph, *layoutgraph.Edge) {
	graph := layoutgraph.NewGraph()
	from := graph.AddNode(layoutgraph.NewNode(1, 100, 100))
	to := graph.AddNode(layoutgraph.NewNode(2, 100, 100))
	from.TopLeft = geo.NewPoint(0, 0)
	to.TopLeft = geo.NewPoint(300, 0)
	from.SetShape(shape.TABLE_TYPE)
	to.SetShape(shape.TABLE_TYPE)
	from.SetNumColumns(3)
	to.SetNumColumns(3)
	edge := graph.Connect(from, to)
	fromColumn, toColumn := 0, 2
	edge.FromTableColumnIndex = &fromColumn
	edge.ToTableColumnIndex = &toColumn
	return graph, edge
}

func TestRouteEdgesKeepsSelectedTableEdgeOnRowPorts(t *testing.T) {
	graph, edge := newStandaloneTableEdgeTestGraph()
	wantFrom, wantTo, hasFrom, hasTo, _ := edge.FacingTablePortValues(nil, nil)
	if !hasFrom || !hasTo {
		t.Fatal("table edge did not expose both row ports")
	}

	fixedTo := graph.AddNode(layoutgraph.NewNode(3, 40, 40))
	fixedTo.TopLeft = geo.NewPoint(150, 200)
	fixed := graph.Connect(edge.From, fixedTo)
	fixed.Points = routeWithSpareCapacity(
		geo.NewPoint(100, 50),
		geo.NewPoint(120, 50),
		geo.NewPoint(120, 180),
		geo.NewPoint(170, 180),
		geo.NewPoint(170, 200),
	)
	fixedSnapshot := captureExactRouteTest(fixed)

	assertRoute := func() {
		t.Helper()
		if len(edge.Points) < 2 {
			t.Fatalf("selected table edge has %d route points", len(edge.Points))
		}
		if got := *edge.Points[0]; got != wantFrom {
			t.Fatalf("source port = %v, want table row port %v", got, wantFrom)
		}
		if got := *edge.Points[len(edge.Points)-1]; got != wantTo {
			t.Fatalf("target port = %v, want table row port %v", got, wantTo)
		}
		fixedSnapshot.assertRestored(t)
	}

	if err := RouteEdges(context.Background(), graph, []*layoutgraph.Edge{edge}); err != nil {
		t.Fatal(err)
	}
	assertRoute()
	firstRoute := make([]geo.Point, len(edge.Points))
	for index, point := range edge.Points {
		firstRoute[index] = *point
	}
	if err := RouteEdges(context.Background(), graph, []*layoutgraph.Edge{edge}); err != nil {
		t.Fatal(err)
	}
	assertRoute()
	if len(edge.Points) != len(firstRoute) {
		t.Fatalf("repeated table routing produced %d points, want %d", len(edge.Points), len(firstRoute))
	}
	for index, point := range firstRoute {
		if *edge.Points[index] != point {
			t.Fatalf("repeated table route point %d = %v, want %v", index, edge.Points[index], point)
		}
	}
}

func TestRouteEdgesCancellationRestoresTentativeTableRoute(t *testing.T) {
	graph, edge := newStandaloneTableEdgeTestGraph()
	edge.Points = routeWithSpareCapacity()
	snapshot := captureExactRouteTest(edge)
	cellSize := graph.CellSize
	ctx := &cancelWhenEdgeIsRouted{Context: context.Background(), edge: edge}

	err := RouteEdges(ctx, graph, []*layoutgraph.Edge{edge})
	requireCanceledAt(t, err, "EdgeRouting")
	if !ctx.observed {
		t.Fatal("test context did not observe the completed table route")
	}
	snapshot.assertRestored(t)
	if graph.CellSize != cellSize {
		t.Fatalf("cell size = %v; want restored value %v", graph.CellSize, cellSize)
	}
	costs := graph.RoutingCosts()
	if costs.Crossing != 0 || costs.Turn != 0 || costs.NonCenterPort != 0 {
		t.Fatalf("route cost caches were not restored: %+v", costs)
	}
}

func TestRouteEdgesTableWorkLimitIsExactAndAtomic(t *testing.T) {
	build := func() (*layoutgraph.Graph, *layoutgraph.Edge) {
		graph, edge := newStandaloneTableEdgeTestGraph()
		edge.Points = routeWithSpareCapacity()
		return graph, edge
	}

	minimum := uint64(1)
	for {
		graph, edge := build()
		err := routeEdgesWithWorkLimit(context.Background(), graph, []*layoutgraph.Edge{edge}, minimum)
		if err == nil {
			break
		}
		if !errors.Is(err, errRouteStageWorkLimit) {
			t.Fatalf("work limit %d returned %v", minimum, err)
		}
		minimum *= 2
		if minimum > 1<<20 {
			t.Fatal("could not find a successful standalone table-edge work limit")
		}
	}
	low, high := minimum/2, minimum
	for low+1 < high {
		mid := low + (high-low)/2
		graph, edge := build()
		err := routeEdgesWithWorkLimit(context.Background(), graph, []*layoutgraph.Edge{edge}, mid)
		if err == nil {
			high = mid
			continue
		}
		if !errors.Is(err, errRouteStageWorkLimit) {
			t.Fatalf("work limit %d returned %v", mid, err)
		}
		low = mid
	}

	graph, edge := build()
	if err := routeEdgesWithWorkLimit(context.Background(), graph, []*layoutgraph.Edge{edge}, high); err != nil {
		t.Fatalf("exact table-edge work limit %d was rejected: %v", high, err)
	}
	wantFrom, wantTo, hasFrom, hasTo, _ := edge.FacingTablePortValues(nil, nil)
	if !hasFrom || !hasTo {
		t.Fatal("table edge did not expose both row ports")
	}
	if got := *edge.Points[0]; got != wantFrom {
		t.Fatalf("exact-limit source port = %v, want %v", got, wantFrom)
	}
	if got := *edge.Points[len(edge.Points)-1]; got != wantTo {
		t.Fatalf("exact-limit target port = %v, want %v", got, wantTo)
	}

	graph, edge = build()
	snapshot := captureExactRouteTest(edge)
	cellSize := graph.CellSize
	ctx := &observeRouteMutation{Context: context.Background(), snapshot: snapshot}
	err := routeEdgesWithWorkLimit(ctx, graph, []*layoutgraph.Edge{edge}, high-1)
	if err == nil || !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("one-unit-short table-edge error = %v, want route-stage work limit", err)
	}
	if !ctx.observed {
		t.Fatal("one-unit-short work limit did not observe tentative table geometry")
	}
	snapshot.assertRestored(t)
	costs := graph.RoutingCosts()
	if graph.CellSize != cellSize || costs.Crossing != 0 || costs.Turn != 0 || costs.NonCenterPort != 0 {
		t.Fatalf("one-unit-short rollback did not restore graph state: cell=%v costs=%+v", graph.CellSize, costs)
	}
}
