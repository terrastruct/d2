package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func crosshatchMutationGraph() (*layoutgraph.Graph, *layoutgraph.Edge, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	left := layoutgraph.NewNode(1, 100, 100)
	right := layoutgraph.NewNode(2, 100, 100)
	externalLeft := layoutgraph.NewNode(3, 100, 100)
	externalRight := layoutgraph.NewNode(4, 100, 100)
	left.TopLeft = geo.NewPoint(0, 0)
	right.TopLeft = geo.NewPoint(0, 200)
	externalLeft.TopLeft = geo.NewPoint(400, 0)
	externalRight.TopLeft = geo.NewPoint(400, 200)
	for _, node := range []*layoutgraph.Node{left, right, externalLeft, externalRight} {
		g.AddNewNodeToContainer(nil, node)
	}

	vessel := layoutgraph.NewNode(5, 250, 400)
	vessel.TopLeft = geo.NewPoint(-50, -50)
	vessel.Graph = g
	cluster := &layoutgraph.Cluster{
		Vessel:             vessel,
		Nodes:              []*layoutgraph.Node{left, right},
		Arrangement:        layoutgraph.Row,
		DesiredArrangement: layoutgraph.Row,
		Graph:              g,
	}
	left.Cluster = cluster
	right.Cluster = cluster
	g.Clusters[vessel] = cluster

	leftEdge := g.Connect(left, externalLeft)
	rightEdge := g.Connect(right, externalRight)
	// Crosshatch groups by the cluster-side port. The shared synthetic port
	// makes both edges exercise the straight-line conversion path.
	leftEdge.Points = routeWithSpareCapacity(geo.NewPoint(100, 100), geo.NewPoint(400, 50))
	rightEdge.Points = routeWithSpareCapacity(geo.NewPoint(100, 100), geo.NewPoint(400, 250))
	cluster.EdgeAbductions = []*layoutgraph.EdgeAbduction{
		{Edge: leftEdge, OriginallyFrom: left, CurrentFrom: left, CurrentTo: externalLeft},
		{Edge: rightEdge, OriginallyFrom: right, CurrentFrom: right, CurrentTo: externalRight},
	}
	return g, leftEdge, rightEdge
}

func TestCrosshatchCancellationAfterRouteMutationRollsBack(t *testing.T) {
	g, first, second := crosshatchMutationGraph()
	firstSnapshot := captureExactRouteTest(first)
	secondSnapshot := captureExactRouteTest(second)
	ctx := &cancelWhenRouteMutates{Context: context.Background(), snapshot: firstSnapshot}

	err := Crosshatch(ctx, g)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("crosshatch error = %v, want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation did not observe a crosshatch route mutation")
	}
	firstSnapshot.assertRestored(t)
	secondSnapshot.assertRestored(t)
}

func TestCrosshatchWorkLimitIsDeterministicAndAtomic(t *testing.T) {
	// Find the smallest successful injected limit. The immediately lower limit
	// must fail without retaining any tentative route conversion.
	minimum := uint64(1)
	for {
		g, _, _ := crosshatchMutationGraph()
		if err := crosshatchWithWorkLimit(context.Background(), g, minimum); err == nil {
			break
		}
		minimum *= 2
		if minimum > 1<<20 {
			t.Fatal("could not find a successful crosshatch work limit")
		}
	}
	low, high := minimum/2, minimum
	for low+1 < high {
		mid := low + (high-low)/2
		g, _, _ := crosshatchMutationGraph()
		if err := crosshatchWithWorkLimit(context.Background(), g, mid); err == nil {
			high = mid
		} else {
			low = mid
		}
	}
	if high <= 1 {
		t.Fatalf("minimum successful limit = %d, want > 1", high)
	}

	g, first, second := crosshatchMutationGraph()
	firstSnapshot := captureExactRouteTest(first)
	secondSnapshot := captureExactRouteTest(second)
	err := crosshatchWithWorkLimit(context.Background(), g, high-1)
	if err == nil || !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("crosshatchWithWorkLimit error = %v, want route-stage work limit", err)
	}
	firstSnapshot.assertRestored(t)
	secondSnapshot.assertRestored(t)
}

func traceMutationGraph() (*layoutgraph.Graph, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 100, 100)
	to := layoutgraph.NewNode(2, 100, 100)
	from.TopLeft = geo.NewPoint(0, 0)
	to.TopLeft = geo.NewPoint(300, 0)
	from.SetShape(shape.CIRCLE_TYPE)
	to.SetShape(shape.CIRCLE_TYPE)
	g.AddNewNodeToContainer(nil, from)
	g.AddNewNodeToContainer(nil, to)
	edge := g.Connect(from, to)
	edge.Points = routeWithSpareCapacity(geo.NewPoint(100, 0), geo.NewPoint(300, 100))
	return g, edge
}

func TestTraceEdgesCancellationAfterMutationRollsBack(t *testing.T) {
	g, edge := traceMutationGraph()
	snapshot := captureExactRouteTest(edge)
	ctx := &cancelWhenRouteMutates{Context: context.Background(), snapshot: snapshot}

	err := TraceEdgesToShapeBorder(ctx, g)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("TraceEdgesToShapeBorder error = %v, want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation did not observe a traced route mutation")
	}
	snapshot.assertRestored(t)
}
