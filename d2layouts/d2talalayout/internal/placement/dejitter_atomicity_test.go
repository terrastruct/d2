package placement

import (
	"context"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

const dejitterCacheKey uint64 = 0xdecafbad

type dejitterMutationContext struct {
	context.Context
	edge       *layoutgraph.Edge
	panicValue any
	observed   bool
}

func (ctx *dejitterMutationContext) Err() error {
	if len(ctx.edge.Points) == 2 {
		ctx.observed = true
		if ctx.panicValue != nil {
			panic(ctx.panicValue)
		}
		return context.Canceled
	}
	return ctx.Context.Err()
}

type dejitterStateSnapshot struct {
	graph *layoutgraph.Graph
	node  *layoutgraph.Node
	edge  *layoutgraph.Edge

	topLeft      *geo.Point
	topLeftValue geo.Point
	route        []*geo.Point
	backing      []*geo.Point
	pointValues  map[*geo.Point]geo.Point
	costs        layoutgraph.RoutingCostState
	cacheValue   float64
}

func captureDejitterState(graph *layoutgraph.Graph, node *layoutgraph.Node, edge *layoutgraph.Edge) dejitterStateSnapshot {
	cacheValue, _ := graph.LookupEdgeLengthCost(dejitterCacheKey)
	pointValues := make(map[*geo.Point]geo.Point)
	for _, point := range edge.Points[:cap(edge.Points)] {
		if point != nil {
			pointValues[point] = *point
		}
	}
	return dejitterStateSnapshot{
		graph:        graph,
		node:         node,
		edge:         edge,
		topLeft:      node.TopLeft,
		topLeftValue: *node.TopLeft,
		route:        edge.Points,
		backing:      slices.Clone(edge.Points[:cap(edge.Points)]),
		pointValues:  pointValues,
		costs:        graph.RoutingCosts(),
		cacheValue:   cacheValue,
	}
}

func (snapshot dejitterStateSnapshot) assertRestored(t *testing.T) {
	t.Helper()
	if snapshot.node.TopLeft != snapshot.topLeft || *snapshot.node.TopLeft != snapshot.topLeftValue {
		t.Fatalf("node TopLeft = %p %+v; want %p %+v", snapshot.node.TopLeft, *snapshot.node.TopLeft, snapshot.topLeft, snapshot.topLeftValue)
	}
	if len(snapshot.edge.Points) != len(snapshot.route) || cap(snapshot.edge.Points) != cap(snapshot.route) {
		t.Fatalf("route header = len %d cap %d; want len %d cap %d", len(snapshot.edge.Points), cap(snapshot.edge.Points), len(snapshot.route), cap(snapshot.route))
	}
	if cap(snapshot.route) > 0 && &snapshot.edge.Points[:cap(snapshot.edge.Points)][0] != &snapshot.route[:cap(snapshot.route)][0] {
		t.Fatal("route backing array identity changed")
	}
	if !slices.Equal(snapshot.edge.Points[:cap(snapshot.edge.Points)], snapshot.backing) {
		t.Fatal("route backing array contents changed")
	}
	for point, value := range snapshot.pointValues {
		if *point != value {
			t.Fatalf("route point %p = %+v; want %+v", point, *point, value)
		}
	}
	if got := snapshot.graph.RoutingCosts(); got != snapshot.costs {
		t.Fatalf("routing costs = %+v; want %+v", got, snapshot.costs)
	}
	cacheValue, ok := snapshot.graph.LookupEdgeLengthCost(dejitterCacheKey)
	if !ok || cacheValue != snapshot.cacheValue || snapshot.graph.EdgeLengthCacheEntries() != 1 {
		t.Fatalf("edge-length cache = (%v, %v, %d entries); want (%v, true, 1 entry)", cacheValue, ok, snapshot.graph.EdgeLengthCacheEntries(), snapshot.cacheValue)
	}
}

func dejitterAtomicityGraph(finalCandidate bool) (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Edge) {
	graph := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(1, 8, 8)
	a.TopLeft = geo.NewPoint(4, 4)
	b := layoutgraph.NewNode(2, 8, 8)
	b.TopLeft = geo.NewPoint(20, 3)
	if finalCandidate {
		b.FixedTopLeft = geo.NewPoint(20, 3)
		graph.AddNode(b)
		graph.AddNode(a)
	} else {
		graph.AddNode(a)
		graph.AddNode(b)
	}
	edge := graph.Connect(a, b)
	backing := make([]*geo.Point, 6)
	copy(backing, []*geo.Point{
		geo.NewPoint(12, 4),
		geo.NewPoint(16, 4),
		geo.NewPoint(16, 3),
		geo.NewPoint(20, 3),
		geo.NewPoint(10_004, 20_004),
		geo.NewPoint(10_005, 20_005),
	})
	edge.Points = backing[:4]
	graph.RestoreRoutingCosts(layoutgraph.RoutingCostState{Crossing: 3, Turn: 5, NonCenterPort: 7})
	graph.StoreEdgeLengthCost(dejitterCacheKey, 11)
	return graph, a, edge
}

func TestDejitterCancellationRestoresAcceptedMutation(t *testing.T) {
	graph, node, edge := dejitterAtomicityGraph(false)
	snapshot := captureDejitterState(graph, node, edge)
	ctx := &dejitterMutationContext{Context: context.Background(), edge: edge}

	dejittered, err := Dejitter(ctx, graph)
	if dejittered {
		t.Fatal("Dejitter reported a committed mutation after cancellation")
	}
	requireCanceledAt(t, err, "EdgeLength")
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe a committed route mutation")
	}
	snapshot.assertRestored(t)
}

func TestDejitterPanicRestoresAcceptedMutation(t *testing.T) {
	graph, node, edge := dejitterAtomicityGraph(false)
	snapshot := captureDejitterState(graph, node, edge)
	sentinel := &struct{ name string }{name: "dejitter panic"}
	ctx := &dejitterMutationContext{Context: context.Background(), edge: edge, panicValue: sentinel}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = Dejitter(ctx, graph)
	}()
	if recovered != sentinel {
		t.Fatalf("panic = %v; want exact sentinel %v", recovered, sentinel)
	}
	if !ctx.observed {
		t.Fatal("panic probe did not observe a committed route mutation")
	}
	snapshot.assertRestored(t)
}

func TestDejitterFinalCancellationRestoresAcceptedMutation(t *testing.T) {
	graph, node, edge := dejitterAtomicityGraph(true)
	snapshot := captureDejitterState(graph, node, edge)
	ctx := &dejitterMutationContext{Context: context.Background(), edge: edge}

	dejittered, err := Dejitter(ctx, graph)
	if dejittered {
		t.Fatal("Dejitter reported a committed mutation after final cancellation")
	}
	requireCanceledAt(t, err, "DejitterTransactions")
	if !ctx.observed {
		t.Fatal("final cancellation probe did not observe a committed route mutation")
	}
	snapshot.assertRestored(t)
}

func TestDejitterSuccessCommitsAcceptedMutation(t *testing.T) {
	graph, node, edge := dejitterAtomicityGraph(false)
	topLeft := node.TopLeft

	dejittered, err := Dejitter(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if !dejittered {
		t.Fatal("Dejitter did not report its accepted mutation")
	}
	if node.TopLeft != topLeft || *node.TopLeft != (geo.Point{X: 4, Y: 3}) {
		t.Fatalf("node TopLeft = %p %+v; want %p {4 3}", node.TopLeft, *node.TopLeft, topLeft)
	}
	want := []geo.Point{{X: 12, Y: 3}, {X: 20, Y: 3}}
	if len(edge.Points) != len(want) {
		t.Fatalf("route length = %d; want %d", len(edge.Points), len(want))
	}
	for index, point := range edge.Points {
		if *point != want[index] {
			t.Fatalf("route point %d = %+v; want %+v", index, *point, want[index])
		}
	}
	if costs := graph.RoutingCosts(); costs != (layoutgraph.RoutingCostState{Crossing: 3, Turn: 5, NonCenterPort: 7}) {
		t.Fatalf("routing costs = %+v; want sentinels", costs)
	}
	if cacheValue, ok := graph.LookupEdgeLengthCost(dejitterCacheKey); !ok || cacheValue != 11 || graph.EdgeLengthCacheEntries() != 1 {
		t.Fatalf("edge-length cache = (%v, %v, %d entries); want (11, true, 1 entry)", cacheValue, ok, graph.EdgeLengthCacheEntries())
	}
}
