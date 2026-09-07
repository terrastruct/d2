package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func newRoutingTestGraph(targetX, targetY float64) (*layoutgraph.Graph, *layoutgraph.Edge) {
	graph := layoutgraph.NewGraph()
	source := layoutgraph.NewNode(1, 40, 40)
	source.TopLeft = geo.NewPoint(0, 0)
	target := layoutgraph.NewNode(2, 40, 40)
	target.TopLeft = geo.NewPoint(targetX, targetY)
	graph.AddNode(source)
	graph.AddNode(target)
	return graph, graph.Connect(source, target)
}

func TestRouteLineReturnsPorts(t *testing.T) {
	graph, edge := newRoutingTestGraph(200, 0)

	from, to, cost, err := routeLine(context.Background(), graph, edge, graph.Edges, nil)
	if err != nil {
		t.Fatal(err)
	}
	if from == nil || to == nil {
		t.Fatalf("route = (%v, %v); want non-nil ports", from, to)
	}
	if cost <= 0 {
		t.Fatalf("route cost = %v; want positive cost", cost)
	}
}

func TestRouteLineIgnoresNonOverlappingEdgesWhenCheckingSharedRoutes(t *testing.T) {
	makeGraph := func() (*layoutgraph.Graph, *layoutgraph.Edge) {
		graph := layoutgraph.NewGraph()
		from := graph.AddNode(layoutgraph.NewNode(1, 100, 100))
		to := graph.AddNode(layoutgraph.NewNode(2, 100, 100))
		from.TopLeft = geo.NewPoint(0, 0)
		to.TopLeft = geo.NewPoint(300, 0)
		candidate := graph.Connect(from, to)
		candidate.TargetArrowhead = layoutgraph.TriangleArrowhead

		sharedTarget := graph.AddNode(layoutgraph.NewNode(3, 100, 100))
		sharedTarget.TopLeft = geo.NewPoint(200, 300)
		shareable := graph.Connect(from, sharedTarget)
		shareable.TargetArrowhead = layoutgraph.TriangleArrowhead
		shareable.Points = []*geo.Point{
			geo.NewPoint(100, 50),
			geo.NewPoint(250, 50),
			geo.NewPoint(250, 300),
		}

		unrelatedFrom := graph.AddNode(layoutgraph.NewNode(4, 100, 100))
		unrelatedTo := graph.AddNode(layoutgraph.NewNode(5, 100, 100))
		unrelatedFrom.TopLeft = geo.NewPoint(0, 500)
		unrelatedTo.TopLeft = geo.NewPoint(300, 500)
		unrelated := graph.Connect(unrelatedFrom, unrelatedTo)
		unrelated.Points = []*geo.Point{geo.NewPoint(100, 550), geo.NewPoint(300, 550)}
		return graph, candidate
	}

	for _, guarded := range []bool{false, true} {
		name := "ordinary"
		if guarded {
			name = "guarded"
		}
		t.Run(name, func(t *testing.T) {
			graph, candidate := makeGraph()
			var from, to *geo.Point
			var err error
			if guarded {
				guard, guardErr := newRouteWorkGuard(context.Background(), "route-line overlap test", maxRouteStageWorkUnits)
				if guardErr != nil {
					t.Fatal(guardErr)
				}
				from, to, _, err = routeLineGuarded(graph, candidate, graph.Edges, nil, guard)
			} else {
				from, to, _, err = routeLine(context.Background(), graph, candidate, graph.Edges, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if from == nil || to == nil || from.X != 100 || from.Y != 50 || to.X != 300 || to.Y != 50 {
				t.Fatalf("route = (%v, %v), want centered shared route ((100, 50), (300, 50))", from, to)
			}
		})
	}
}

func TestStraightEdgeFallbackCanceledBeforeWork(t *testing.T) {
	graph, edge := newRoutingTestGraph(200, 0)
	edge.Points = []*geo.Point{geo.NewPoint(40, 20), geo.NewPoint(200, 20)}
	want := append([]*geo.Point(nil), edge.Points...)

	changed, err := tryStraightEdgeFallback(canceledContext(), graph, edge)
	requireCanceledAt(t, err, "EdgeRouting")
	if changed {
		t.Fatal("canceled fallback reported a route replacement")
	}
	if len(edge.Points) != len(want) {
		t.Fatalf("route length after cancellation = %d; want %d", len(edge.Points), len(want))
	}
	for i := range want {
		if edge.Points[i] != want[i] {
			t.Fatalf("route point %d after cancellation = %p; want %p", i, edge.Points[i], want[i])
		}
	}
}

func TestStraightEdgeFallbackPreservesCandidateRejection(t *testing.T) {
	graph, edge := newRoutingTestGraph(0, 0)
	edge.Points = []*geo.Point{geo.NewPoint(0, 20), geo.NewPoint(40, 20)}
	want := append([]*geo.Point(nil), edge.Points...)

	_, _, _, routeErr := routeLine(context.Background(), graph, edge, graph.Edges, nil)
	if !errors.Is(routeErr, layoutgraph.ErrInvalidCandidate) {
		t.Fatalf("routeLine error = %v; want ErrInvalidCandidate", routeErr)
	}
	changed, err := tryStraightEdgeFallback(context.Background(), graph, edge)
	if err != nil {
		t.Fatalf("ordinary straight-edge miss returned an error: %v", err)
	}
	if changed {
		t.Fatal("ordinary straight-edge miss reported a route replacement")
	}
	if len(edge.Points) != len(want) {
		t.Fatalf("route length after ordinary miss = %d; want %d", len(edge.Points), len(want))
	}
	for i := range want {
		if edge.Points[i] != want[i] {
			t.Fatalf("route point %d after ordinary miss = %p; want %p", i, edge.Points[i], want[i])
		}
	}
}

type failRoutingAfterChecks struct {
	context.Context
	remaining int
	err       error
}

func (ctx *failRoutingAfterChecks) Err() error {
	if ctx.remaining == 0 {
		return ctx.err
	}
	ctx.remaining--
	return nil
}

func TestStraightEdgeFallbackPropagatesUnexpectedErrors(t *testing.T) {
	graph, edge := newRoutingTestGraph(200, 0)
	edge.Points = []*geo.Point{geo.NewPoint(40, 20), geo.NewPoint(200, 20)}
	wantErr := errors.New("unexpected routing failure")
	ctx := &failRoutingAfterChecks{
		Context:   context.Background(),
		remaining: 1,
		err:       wantErr,
	}

	changed, err := tryStraightEdgeFallback(ctx, graph, edge)
	if changed {
		t.Fatal("failed straight-edge fallback reported a route replacement")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("fallback error = %v; want %v", err, wantErr)
	}
}

func TestGuardedStraightEdgeFallbackMatchesOrdinarySuccess(t *testing.T) {
	makeGraph := func() (*layoutgraph.Graph, *layoutgraph.Edge) {
		graph, edge := newRoutingTestGraph(200, 0)
		edge.Points = []*geo.Point{
			geo.NewPoint(40, 20),
			geo.NewPoint(40, 100),
			geo.NewPoint(200, 100),
			geo.NewPoint(200, 20),
		}
		return graph, edge
	}

	ordinaryGraph, ordinaryEdge := makeGraph()
	changed, err := tryStraightEdgeFallback(context.Background(), ordinaryGraph, ordinaryEdge)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("ordinary fallback did not report its route replacement")
	}
	guardedGraph, guardedEdge := makeGraph()
	guard, err := newRouteWorkGuard(context.Background(), "fallback parity", maxRouteStageWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	if err := tryStraightEdgeFallbackGuarded(guardedGraph, guardedEdge, guard); err != nil {
		t.Fatal(err)
	}
	if guard.used == 0 {
		t.Fatal("guarded fallback did not charge any work")
	}
	if len(ordinaryEdge.Points) != 2 || len(guardedEdge.Points) != len(ordinaryEdge.Points) {
		t.Fatalf("route lengths ordinary=%d guarded=%d; want matching straight replacements", len(ordinaryEdge.Points), len(guardedEdge.Points))
	}
	for index := range ordinaryEdge.Points {
		if *guardedEdge.Points[index] != *ordinaryEdge.Points[index] {
			t.Fatalf("route point %d guarded=%v ordinary=%v", index, guardedEdge.Points[index], ordinaryEdge.Points[index])
		}
	}
}

func TestPipelineStraightEdgeFallbackPreservesCancellation(t *testing.T) {
	graph, edge := newRoutingTestGraph(200, 0)
	edge.Points = []*geo.Point{geo.NewPoint(40, 20), geo.NewPoint(200, 20)}
	want := append([]*geo.Point(nil), edge.Points...)

	err := StraightEdgesFallback(canceledContext(), graph)
	requireCanceledAt(t, err, "EdgeRouting")
	if len(edge.Points) != len(want) {
		t.Fatalf("route length after pipeline cancellation = %d; want %d", len(edge.Points), len(want))
	}
	for i := range want {
		if edge.Points[i] != want[i] {
			t.Fatalf("route point %d after pipeline cancellation = %p; want %p", i, edge.Points[i], want[i])
		}
	}
}

func TestRouteLineCanceledBeforeWork(t *testing.T) {
	graph, edge := newRoutingTestGraph(200, 0)
	from, to, cost, err := routeLine(canceledContext(), graph, edge, graph.Edges, nil)
	if from != nil || to != nil || cost != 0 {
		t.Fatalf("canceled route = (%v, %v, %v), want zero result", from, to, cost)
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestRouteLineCanceledDuringPortPairEvaluation(t *testing.T) {
	graph, edge := newRoutingTestGraph(200, 0)

	// Allow the entry, existing-edge, source-port, and target-port checks,
	// then cancel while filtering edges for the first port pair.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 4}
	from, to, cost, err := routeLine(ctx, graph, edge, graph.Edges, nil)
	if from != nil || to != nil || cost != 0 {
		t.Fatalf("canceled route = (%v, %v, %v), want zero result", from, to, cost)
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestSlingshotCanceledDuringLEnumeration(t *testing.T) {
	graph, edge := newRoutingTestGraph(100, 100)
	ovg, err := buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newOVGEdgeRouter(context.Background(), Default, ovg, graph, nil, graph.Edges)
	if err != nil {
		t.Fatal(err)
	}

	// Allow the slingshot and L-search preflights plus the orientation check,
	// then cancel as source-port enumeration begins.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 3}
	path, distance, err := router.slingshot(ctx, edge)
	if path != nil || distance != 0 {
		t.Fatalf("canceled slingshot = (%v, %v), want zero result", path, distance)
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestSlingshotCanceledDuringSEnumeration(t *testing.T) {
	graph, edge := newRoutingTestGraph(100, 100)
	ovg, err := buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newOVGEdgeRouter(context.Background(), Default, ovg, graph, nil, graph.Edges)
	if err != nil {
		t.Fatal(err)
	}
	orientation := edge.From.Orientation(edge.To)
	vertical, strong := preferLaunchingVertically(edge.From, edge.To, orientation)
	order := []bool{false, true}
	if vertical {
		order = []bool{true, false}
	}
	clearRoute := newFallibleRouteChecker(func(_, _ *OVGNode) (bool, error) { return false, nil })

	// Allow the S-search preflight and orientation check, then cancel as its
	// port enumeration begins.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 2}
	path, distance, err := router.findSShapedRoute(ctx, order, vertical, strong, edge, orientation, clearRoute, clearRoute)
	if path != nil || distance != 0 {
		t.Fatalf("canceled S route = (%v, %v), want zero result", path, distance)
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestQuickRouteMissAndCancellation(t *testing.T) {
	graph, edge := newRoutingTestGraph(100, 100)
	ovg, err := buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newOVGEdgeRouter(context.Background(), Default, ovg, graph, nil, graph.Edges)
	if err != nil {
		t.Fatal(err)
	}
	orientation := edge.From.Orientation(edge.To)

	path, distance, err := router.quickRoute(context.Background(), edge, false, orientation)
	if err != nil || path != nil || distance != 0 {
		t.Fatalf("ordinary quick-route miss = (%v, %v, %v), want (nil, 0, nil)", path, distance, err)
	}
	_, _, err = router.quickRoute(canceledContext(), edge, false, orientation)
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestQuickRoutePreservesOrdinaryFallback(t *testing.T) {
	graph, edge := newRoutingTestGraph(70, 0)
	blocker := layoutgraph.NewNode(3, 20, 80)
	blocker.TopLeft = geo.NewPoint(45, -20)
	graph.AddNode(blocker)
	ovg, err := buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newOVGEdgeRouter(context.Background(), Default, ovg, graph, nil, graph.Edges)
	if err != nil {
		t.Fatal(err)
	}

	orientation := edge.From.Orientation(edge.To)
	nodeDistance := edge.From.DistanceTo(edge.To, true)
	if orientation.IsDiagonal() || nodeDistance >= 2*(layoutgraph.MinRouteNodeClearance+1) || nodeDistance <= layoutgraph.MinArrowheadClearance {
		t.Fatalf("test graph does not enter quick-route path: orientation=%v distance=%v", orientation, nodeDistance)
	}
	path, distance, err := router.quickRoute(context.Background(), edge, false, orientation)
	if err != nil || path != nil || distance != 0 {
		t.Fatalf("blocked quick route = (%v, %v, %v), want ordinary fallback (nil, 0, nil)", path, distance, err)
	}
}
