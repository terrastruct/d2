package routing

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func TestEdgeRoutingSplitConsumesStageAggregate(t *testing.T) {
	g := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 10, 10)
	node.TopLeft = geo.NewPoint(0, 0)
	g.AddNewNodeToContainer(nil, node)
	external := layoutgraph.NewNode(2, 10, 10)
	external.TopLeft = geo.NewPoint(100, 0)
	node.Nears[external] = struct{}{}

	guard, err := newRouteWorkGuard(context.Background(), "EdgeRouting", 1)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = g.SplitSubgraphsTracked(context.Background(), layoutgraph.SplitOptions{
		IncludeContainers: true,
		IncludeNears:      true,
	}, guard)
	if !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("SplitSubgraphs error = %v, want shared route-stage work limit", err)
	}
	if external.Graph != nil {
		t.Fatalf("failed split changed Near-only owner to %p", external.Graph)
	}
}

func TestEdgeRoutingSplitChargesEveryTopLevelNodeScan(t *testing.T) {
	g := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 10, 10)
	node.TopLeft = geo.NewPoint(0, 0)
	g.AddNewNodeToContainer(nil, node)

	guard, err := newRouteWorkGuard(context.Background(), "EdgeRouting", ^uint64(0))
	if err != nil {
		t.Fatal(err)
	}
	_, ownership, err := g.SplitSubgraphsTracked(context.Background(), layoutgraph.SplitOptions{
		IncludeContainers: true,
		IncludeNears:      true,
	}, guard)
	if err != nil {
		t.Fatal(err)
	}
	defer ownership.Restore()
	used := guard.used
	// One node is charged by ownership capture, fixed-node discovery, outer
	// discovery, reachability dequeue, container membership, and assignment.
	if used != 6 {
		t.Fatalf("SplitSubgraphs charged %d units, want exact top-level scan accounting 6", used)
	}
}

func TestRouteStageGraphBoundingBoxMatchesLegacyAndConsumesAggregate(t *testing.T) {
	g := layoutgraph.NewGraph()
	for index := 0; index < 8; index++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(index+1), 20+float64(index), 15+float64(index))
		node.TopLeft = geo.NewPoint(float64(index*40), float64((index%3)*30))
		node.Label = &layoutgraph.Label{
			Position: label.OutsideBottomRight,
			Width:    60 + float64(index),
			Height:   15,
		}
		g.AddNewNodeToContainer(nil, node)
	}
	edge := g.Connect(g.Nodes[0], g.Nodes[len(g.Nodes)-1])
	edge.Points = []*geo.Point{
		geo.NewPoint(0, 0),
		geo.NewPoint(25, -50),
		geo.NewPoint(100, -50),
		geo.NewPoint(300, 60),
	}
	edge.Label = &layoutgraph.Label{Position: label.UnlockedMiddle, Width: 35, Height: 12}
	edge.SourceArrowheadLabel = &layoutgraph.Label{Width: 20, Height: 10}
	edge.TargetArrowheadLabel = &layoutgraph.Label{Width: 22, Height: 11}

	wantTopLeft, wantBottomRight := g.BoundingBox()
	guard, err := newRouteWorkGuard(context.Background(), "EdgeRouting", ^uint64(0))
	if err != nil {
		t.Fatal(err)
	}
	gotTopLeft, gotBottomRight, err := routeStageGraphBoundingBox(g, guard)
	if err != nil {
		t.Fatal(err)
	}
	if *gotTopLeft != *wantTopLeft || *gotBottomRight != *wantBottomRight {
		t.Fatalf("guarded bounds = %v, %v; want legacy %v, %v", gotTopLeft, gotBottomRight, wantTopLeft, wantBottomRight)
	}
	used := guard.used
	if used <= uint64(len(g.Nodes)*len(g.Nodes)) {
		t.Fatalf("guarded bounds charged %d units, want outside-label extremity and route scans", used)
	}

	limited, err := newRouteWorkGuard(context.Background(), "EdgeRouting", used-1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := routeStageGraphBoundingBox(g, limited); !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("limited guarded bounds error = %v, want route-stage work limit", err)
	}
}

func TestRouteStageGraphBoundingBoxChargesLabelRouteScans(t *testing.T) {
	g := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 10, 10)
	to := layoutgraph.NewNode(2, 10, 10)
	from.TopLeft = geo.NewPoint(0, 0)
	to.TopLeft = geo.NewPoint(100, 0)
	g.AddNewNodeToContainer(nil, from)
	g.AddNewNodeToContainer(nil, to)
	edge := g.Connect(from, to)
	edge.Points = []*geo.Point{
		geo.NewPoint(10, 5),
		geo.NewPoint(25, -20),
		geo.NewPoint(50, -20),
		geo.NewPoint(75, 5),
		geo.NewPoint(100, 5),
	}

	used := func() uint64 {
		t.Helper()
		guard, err := newRouteWorkGuard(context.Background(), "EdgeRouting", ^uint64(0))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := routeStageGraphBoundingBox(g, guard); err != nil {
			t.Fatal(err)
		}
		return guard.used
	}

	baseline := used()
	pointCount := uint64(len(edge.Points))
	edge.Label = &layoutgraph.Label{Position: label.UnlockedMiddle, Width: 30, Height: 12}
	if delta := used() - baseline; delta != 2*pointCount {
		t.Fatalf("positioned edge label charged %d extra units, want %d", delta, 2*pointCount)
	}
	edge.Label = nil
	edge.SourceArrowheadLabel = &layoutgraph.Label{Width: 20, Height: 10}
	if delta := used() - baseline; delta != 3*pointCount {
		t.Fatalf("source arrowhead label charged %d extra units, want %d", delta, 3*pointCount)
	}
	edge.SourceArrowheadLabel = nil
	edge.TargetArrowheadLabel = &layoutgraph.Label{Width: 20, Height: 10}
	if delta := used() - baseline; delta != 3*pointCount {
		t.Fatalf("target arrowhead label charged %d extra units, want %d", delta, 3*pointCount)
	}
}

func TestEdgeRoutingNestedWorkBudgetIsSharedAcrossSubgraphs(t *testing.T) {
	newComponent := func(id layoutgraph.EntityID) *layoutgraph.Graph {
		g := layoutgraph.NewGraph()
		from := layoutgraph.NewNode(id, 10, 10)
		to := layoutgraph.NewNode(id+1, 10, 10)
		from.TopLeft = geo.NewPoint(0, 0)
		to.TopLeft = geo.NewPoint(100, 0)
		g.AddNewNodeToContainer(nil, from)
		g.AddNewNodeToContainer(nil, to)
		g.Connect(from, to)
		g.ComputeCellSize()
		return g
	}

	stageGuard, err := newRouteWorkGuard(context.Background(), "EdgeRouting", ^uint64(0))
	if err != nil {
		t.Fatal(err)
	}
	routingCtx := contextWithRouteAggregateWork(context.Background(), stageGuard)
	ovgGuard, err := newOVGBuildGuard(routingCtx, defaultOVGBuildLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routeEdgesWithResourceGuards(routingCtx, newComponent(1), nil, ovgGuard, maxRouteSearchWorkUnits); err != nil {
		t.Fatalf("route first subgraph: %v", err)
	}
	if stageGuard.used == 0 {
		t.Fatal("nested OVG and route search consumed no aggregate work")
	}
	// The second subgraph would fit each freshly-created nested guard. Leave one
	// shared unit to prove those helpers cannot reset the whole-stage envelope.
	stageGuard.limit = stageGuard.used + 1

	_, err = routeEdgesWithResourceGuards(routingCtx, newComponent(100), nil, ovgGuard, maxRouteSearchWorkUnits)
	if !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("route second subgraph error = %v, want aggregate stage work limit", err)
	}
}

func TestConcurrentRouteFlavorsShareRaceSafeAggregateBudget(t *testing.T) {
	const stepsPerFlavor = 1_000
	stageGuard, err := newRouteWorkGuard(context.Background(), "EdgeRouting", 2*stepsPerFlavor)
	if err != nil {
		t.Fatal(err)
	}
	ctx := contextWithRouteAggregateWork(context.Background(), stageGuard)
	first, err := newRouteSearchWorkGuard(ctx, Default, maxRouteSearchWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRouteSearchWorkGuard(ctx, ShortestToLongest, maxRouteSearchWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsByFlavor := make(chan error, 2)
	for _, guard := range []*routeSearchWorkGuard{first, second} {
		wait.Add(1)
		go func(guard *routeSearchWorkGuard) {
			defer wait.Done()
			for range stepsPerFlavor {
				if err := guard.step(); err != nil {
					errorsByFlavor <- err
					return
				}
			}
		}(guard)
	}
	wait.Wait()
	close(errorsByFlavor)
	for err := range errorsByFlavor {
		t.Fatalf("route flavor work: %v", err)
	}
	if err := first.step(); !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("post-flavor work error = %v, want shared stage work limit", err)
	}
}
