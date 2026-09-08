package routing

import (
	"context"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func newDuplicateEdgeTestGraph() (*layoutgraph.Graph, []*layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	from := g.AddNode(layoutgraph.NewNode(1, 40, 40))
	from.TopLeft = geo.NewPoint(0, 0)
	to := g.AddNode(layoutgraph.NewNode(2, 40, 40))
	to.TopLeft = geo.NewPoint(0, 200)

	edges := make([]*layoutgraph.Edge, 3)
	for i := range edges {
		edges[i] = g.Connect(from, to)
		x := float64(10 * (i + 1))
		edges[i].Points = []*geo.Point{geo.NewPoint(x, 40), geo.NewPoint(x, 200)}
	}
	edges[1].Label = &layoutgraph.Label{Text: "labeled"}
	return g, edges
}

func newFinalDuplicateEdgeTestGraph(reverseUnlabeled bool) (*layoutgraph.Graph, []*layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	from := g.AddNode(layoutgraph.NewNode(1, 40, 40))
	from.TopLeft = geo.NewPoint(0, 0)
	to := g.AddNode(layoutgraph.NewNode(2, 40, 40))
	to.TopLeft = geo.NewPoint(0, 200)

	first := g.Connect(from, to)
	first.Points = routeWithSpareCapacity(geo.NewPoint(10, 40), geo.NewPoint(10, 200))
	var last *layoutgraph.Edge
	if reverseUnlabeled {
		last = g.Connect(to, from)
		last.Points = routeWithSpareCapacity(geo.NewPoint(30, 200), geo.NewPoint(30, 40))
	} else {
		last = g.Connect(from, to)
		last.Points = routeWithSpareCapacity(geo.NewPoint(30, 40), geo.NewPoint(30, 200))
	}
	labeled := g.Connect(from, to)
	labeled.Points = routeWithSpareCapacity(geo.NewPoint(20, 40), geo.NewPoint(20, 200))
	labeled.Label = &layoutgraph.Label{Text: "labeled"}
	return g, []*layoutgraph.Edge{first, last, labeled}
}

func copyEdgePointSlices(edges []*layoutgraph.Edge) [][]*geo.Point {
	points := make([][]*geo.Point, len(edges))
	for i, edge := range edges {
		points[i] = append([]*geo.Point(nil), edge.Points...)
	}
	return points
}

func requireSameEdgePointSlices(t *testing.T, edges []*layoutgraph.Edge, want [][]*geo.Point) {
	t.Helper()
	for i, edge := range edges {
		if len(edge.Points) != len(want[i]) {
			t.Fatalf("edge %d route length = %d; want %d", i, len(edge.Points), len(want[i]))
		}
		for j := range want[i] {
			if edge.Points[j] != want[i][j] {
				t.Fatalf("edge %d point %d = %p; want %p", i, j, edge.Points[j], want[i][j])
			}
		}
	}
}

func TestReorderDuplicatesCanceledBeforeWork(t *testing.T) {
	g, edges := newDuplicateEdgeTestGraph()
	want := copyEdgePointSlices(edges)

	err := ReorderDuplicates(canceledContext(), g)
	requireCanceledAt(t, err, "ReorderDuplicates")
	requireSameEdgePointSlices(t, edges, want)
}

func TestReorderDuplicatesCanceledDuringDuplicateScan(t *testing.T) {
	g, edges := newDuplicateEdgeTestGraph()
	want := copyEdgePointSlices(edges)

	// Allow the preflight, unlabeled-edge check, labeled-edge check, and first
	// real duplicate check, then cancel while the quadratic scan is still running.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 4}
	err := ReorderDuplicates(ctx, g)
	requireCanceledAt(t, err, "ReorderDuplicates")
	requireSameEdgePointSlices(t, edges, want)
}

type cancelWhenRoutesChange struct {
	context.Context
	edges    []*layoutgraph.Edge
	want     [][]*geo.Point
	observed bool
}

func (ctx *cancelWhenRoutesChange) Err() error {
	for i, edge := range ctx.edges {
		if len(edge.Points) != len(ctx.want[i]) {
			ctx.observed = true
			return context.Canceled
		}
		for j := range ctx.want[i] {
			if edge.Points[j] != ctx.want[i][j] {
				ctx.observed = true
				return context.Canceled
			}
		}
	}
	return nil
}

type panicWhenDuplicateRoutesChange struct {
	context.Context
	snapshots []exactRouteTestSnapshot
	value     any
	observed  bool
}

func (ctx *panicWhenDuplicateRoutesChange) Err() error {
	for _, snapshot := range ctx.snapshots {
		if snapshot.changed() {
			ctx.observed = true
			panic(ctx.value)
		}
	}
	return ctx.Context.Err()
}

func TestReorderDuplicatesRollsBackCancellationAfterSwap(t *testing.T) {
	g, edges := newDuplicateEdgeTestGraph()
	want := copyEdgePointSlices(edges)
	ctx := &cancelWhenRoutesChange{Context: context.Background(), edges: edges, want: want}

	err := ReorderDuplicates(ctx, g)
	requireCanceledAt(t, err, "ReorderDuplicates")
	if !ctx.observed {
		t.Fatal("test context did not observe a route swap")
	}
	requireSameEdgePointSlices(t, edges, want)
}

func TestReorderDuplicatesCommitsSuccessfulSwap(t *testing.T) {
	for _, reverseUnlabeled := range []bool{false, true} {
		g, edges := newFinalDuplicateEdgeTestGraph(reverseUnlabeled)
		want := copyEdgePointSlices(edges)
		if err := ReorderDuplicates(t.Context(), g); err != nil {
			t.Fatalf("reverseUnlabeled=%v: ReorderDuplicates error = %v", reverseUnlabeled, err)
		}

		if reverseUnlabeled {
			slices.Reverse(want[1])
			slices.Reverse(want[2])
		}
		want[1], want[2] = want[2], want[1]
		requireSameEdgePointSlices(t, edges, want)
	}
}

func TestReorderDuplicatesRollsBackPanicAfterSwap(t *testing.T) {
	// The labeled edge is last in graph order, so the first post-swap context
	// poll is the function's final check. The reverse case also exercises both
	// in-place route reversals.
	for _, reverseUnlabeled := range []bool{false, true} {
		g, edges := newFinalDuplicateEdgeTestGraph(reverseUnlabeled)
		snapshots := make([]exactRouteTestSnapshot, len(edges))
		for i, edge := range edges {
			snapshots[i] = captureExactRouteTest(edge)
		}
		panicValue := &struct{ name string }{name: "reorder duplicates mutation probe"}
		ctx := &panicWhenDuplicateRoutesChange{
			Context:   context.Background(),
			snapshots: snapshots,
			value:     panicValue,
		}

		var recovered any
		func() {
			defer func() { recovered = recover() }()
			_ = ReorderDuplicates(ctx, g)
		}()
		if recovered != panicValue {
			t.Fatalf("reverseUnlabeled=%v: recovered = %v; want exact mutation probe %p", reverseUnlabeled, recovered, panicValue)
		}
		if !ctx.observed {
			t.Fatalf("reverseUnlabeled=%v: context did not observe a route swap", reverseUnlabeled)
		}
		for _, snapshot := range snapshots {
			snapshot.assertRestored(t)
		}
	}
}
