package packing

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type cancelInsideBinPackCandidateDiscovery struct {
	context.Context
	observed bool
}

func (ctx *cancelInsideBinPackCandidateDiscovery) Err() error {
	callers := make([]uintptr, 24)
	count := runtime.Callers(2, callers)
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".placementCandidatesGuarded") {
			ctx.observed = true
			return context.Canceled
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

type cancelInsideRoutedContainerRouteScan struct {
	context.Context
	observed bool
}

type cancelInsideRoutedInventoryValidation struct {
	context.Context
	observed bool
}

func (ctx *cancelInsideRoutedInventoryValidation) Err() error {
	callers := make([]uintptr, 24)
	count := runtime.Callers(2, callers)
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".allEdgesHaveCompleteRoutesGuarded") {
			ctx.observed = true
			return context.Canceled
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

func (ctx *cancelInsideRoutedContainerRouteScan) Err() error {
	callers := make([]uintptr, 24)
	count := runtime.Callers(2, callers)
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".routedContainerSegmentStaysInsideShrink") {
			ctx.observed = true
			return context.Canceled
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

type packingMutationProbe struct {
	context.Context
	node      *layoutgraph.Node
	position  geo.Point
	route     exactRouteTestSnapshot
	panicMode bool
	observed  bool
}

func (ctx *packingMutationProbe) Err() error {
	changed := ctx.node.TopLeft == nil || *ctx.node.TopLeft != ctx.position || ctx.route.changed()
	if changed {
		ctx.observed = true
		if ctx.panicMode {
			panic("packing mutation probe")
		}
		return context.Canceled
	}
	return ctx.Context.Err()
}

func binPackAtomicityGraph() (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 10, 10)
	from.TopLeft = geo.NewPoint(0, 0)
	to := layoutgraph.NewNode(2, 10, 10)
	to.TopLeft = geo.NewPoint(500, 500)
	isolate := layoutgraph.NewNode(3, 10, 10)
	isolate.TopLeft = geo.NewPoint(1_000, 1_000)
	g.AddNewNodeToContainer(nil, from)
	g.AddNewNodeToContainer(nil, to)
	g.AddNewNodeToContainer(nil, isolate)
	edge := g.Connect(from, to)
	edge.Points = routeWithSpareCapacity(
		geo.NewPoint(10, 5),
		geo.NewPoint(250, 5),
		geo.NewPoint(250, 505),
		geo.NewPoint(500, 505),
	)
	return g, isolate, edge
}

func TestBinPackCancellationRestoresExactGeometryAndRoute(t *testing.T) {
	g, moved, edge := binPackAtomicityGraph()
	positionPointer, positionValue := moved.TopLeft, *moved.TopLeft
	route := captureExactRouteTest(edge)
	ctx := &packingMutationProbe{Context: context.Background(), node: moved, position: positionValue, route: route}

	err := Pack(ctx, g, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BinPack error = %v; want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("BinPack did not reach a post-mutation cancellation check")
	}
	if moved.TopLeft != positionPointer || *moved.TopLeft != positionValue {
		t.Fatal("BinPack cancellation did not restore node geometry exactly")
	}
	route.assertRestored(t)
}

func TestBinPackPanicRestoresExactGeometryAndRoute(t *testing.T) {
	g, moved, edge := binPackAtomicityGraph()
	positionPointer, positionValue := moved.TopLeft, *moved.TopLeft
	route := captureExactRouteTest(edge)
	ctx := &packingMutationProbe{Context: context.Background(), node: moved, position: positionValue, route: route, panicMode: true}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = Pack(ctx, g, nil)
	}()
	if recovered == nil {
		t.Fatal("BinPack did not reach a post-mutation panic check")
	}
	if moved.TopLeft != positionPointer || *moved.TopLeft != positionValue {
		t.Fatal("BinPack panic did not restore node geometry exactly")
	}
	route.assertRestored(t)
}

func TestBinPackCandidateRejectionRestoresRouteBackingBeforeNextCandidate(t *testing.T) {
	g, _, edge := binPackAtomicityGraph()
	original := captureExactRouteTest(edge)
	guard, err := newWorkGuard(context.Background(), 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	txnCtx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)
	txn, err := g.NewRequestTransaction(txnCtx, layoutgraph.TransactionOptions{IgnoreContainerEscape: true, AffectEdgeRoutes: true})
	if err != nil {
		t.Fatal(err)
	}

	txn.AddOp(func() error {
		edge.Points[0].X = 999
		edge.Points[0] = geo.NewPoint(777, 888)
		edge.Points = append(edge.Points, geo.NewPoint(500, 500))
		return errors.New("reject first candidate")
	})
	if err := txn.Commit(context.Background()); err == nil {
		t.Fatal("first candidate unexpectedly succeeded")
	}
	original.assertRestored(t)

	txn.Clear()
	txn.AddOp(func() error {
		if original.changed() {
			t.Fatal("second candidate observed route state leaked by first candidate")
		}
		return nil
	})
	if err := txn.Commit(context.Background()); err != nil {
		t.Fatalf("second candidate failed: %v", err)
	}
	original.assertRestored(t)
}

func TestBinPackWorkLimitAfterMutationRestoresExactState(t *testing.T) {
	g, moved, edge := binPackAtomicityGraph()
	positionPointer, positionValue := moved.TopLeft, *moved.TopLeft
	route := captureExactRouteTest(edge)

	// Snapshot construction and discovery consume less than this fixture's full
	// packing search, so the limit trips after nodes have been translated away.
	err := packWithWorkLimit(context.Background(), g, nil, 75)
	if err == nil {
		t.Fatal("BinPack low work limit unexpectedly succeeded")
	}
	if moved.TopLeft != positionPointer || *moved.TopLeft != positionValue {
		t.Fatal("BinPack work-limit error did not restore node geometry exactly")
	}
	route.assertRestored(t)
}

func TestBinPackCancellationInsideCandidateDiscoveryRestoresExactState(t *testing.T) {
	g, moved, edge := binPackAtomicityGraph()
	positionPointer, positionValue := moved.TopLeft, *moved.TopLeft
	route := captureExactRouteTest(edge)
	ctx := &cancelInsideBinPackCandidateDiscovery{Context: context.Background()}

	err := Pack(ctx, g, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BinPack error = %v, want context cancellation", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not run inside placement candidate discovery")
	}
	if moved.TopLeft != positionPointer || *moved.TopLeft != positionValue {
		t.Fatal("candidate-discovery cancellation did not restore node geometry exactly")
	}
	route.assertRestored(t)
}

func TestBinPackCancellationInsideRoutedInventoryValidationRestoresExactState(t *testing.T) {
	graph := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, 100)
	for index := range nodes {
		nodes[index] = layoutgraph.NewNode(layoutgraph.EntityID(index+1), 10, 10)
		nodes[index].TopLeft = geo.NewPoint(float64(index*20), 0)
		graph.AddNewNodeToContainer(nil, nodes[index])
	}
	edge := graph.Connect(nodes[0], nodes[len(nodes)-1])
	edge.Points = routeWithSpareCapacity(geo.NewPoint(10, 5), geo.NewPoint(1980, 5))
	firstPointer, firstPosition := nodes[0].TopLeft, *nodes[0].TopLeft
	lastPointer, lastPosition := nodes[len(nodes)-1].TopLeft, *nodes[len(nodes)-1].TopLeft
	route := captureExactRouteTest(edge)
	ctx := &cancelInsideRoutedInventoryValidation{Context: context.Background()}

	err := Pack(ctx, graph, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BinPack error = %v, want context cancellation", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not run inside routed edge-inventory validation")
	}
	if nodes[0].TopLeft != firstPointer || *nodes[0].TopLeft != firstPosition ||
		nodes[len(nodes)-1].TopLeft != lastPointer || *nodes[len(nodes)-1].TopLeft != lastPosition {
		t.Fatal("routed edge-inventory cancellation changed node geometry")
	}
	route.assertRestored(t)
}

func TestBinPackCancellationInsideRoutedContainerRouteScanRestoresExactState(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 700, 600)
	container.TopLeft = geo.NewPoint(20, 30)
	desiredWidth := 700.
	container.DesiredWidth = &desiredWidth
	first := layoutgraph.NewNode(2, 20, 20)
	first.TopLeft = geo.NewPoint(80, 90)
	moving := layoutgraph.NewNode(3, 20, 20)
	moving.TopLeft = geo.NewPoint(40, 130)
	external := layoutgraph.NewNode(4, 20, 20)
	external.TopLeft = geo.NewPoint(80, -240)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, first)
	graph.AddNewNodeToContainer(container, moving)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(first, external)
	routePoints := make([]*geo.Point, 0, 300)
	for index := 0; index < cap(routePoints); index++ {
		routePoints = append(routePoints, geo.NewPoint(90, 90-float64(index)))
	}
	edge.Points = routeWithSpareCapacity(routePoints...)
	edge.Style.BorderRadius = &layoutgraph.StyleScalar{Value: "0"}
	rootEdge := graph.Connect(container, external)
	rootEdge.Points = routeWithSpareCapacity(geo.NewPoint(50, 30), geo.NewPoint(50, -230))

	containerPointer, containerPosition := container.TopLeft, *container.TopLeft
	containerWidth, containerHeight := container.Width, container.Height
	firstPointer, firstPosition := first.TopLeft, *first.TopLeft
	movingPointer, movingPosition := moving.TopLeft, *moving.TopLeft
	route := captureExactRouteTest(edge)
	rootRoute := captureExactRouteTest(rootEdge)
	ctx := &cancelInsideRoutedContainerRouteScan{Context: context.Background()}

	err := Pack(ctx, graph, container)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BinPack error = %v, want context cancellation", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not run inside routed-container route scanning")
	}
	if container.TopLeft != containerPointer || *container.TopLeft != containerPosition ||
		container.Width != containerWidth || container.Height != containerHeight {
		t.Fatal("routed-container route-scan cancellation did not restore container geometry exactly")
	}
	if first.TopLeft != firstPointer || *first.TopLeft != firstPosition ||
		moving.TopLeft != movingPointer || *moving.TopLeft != movingPosition {
		t.Fatal("routed-container route-scan cancellation did not restore child geometry exactly")
	}
	route.assertRestored(t)
	rootRoute.assertRestored(t)
}
