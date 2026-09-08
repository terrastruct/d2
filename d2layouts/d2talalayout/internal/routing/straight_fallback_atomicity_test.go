package routing

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func straightFallbackMutationGraph(edgeCount int) (*layoutgraph.Graph, []*layoutgraph.Edge) {
	graph := layoutgraph.NewGraph()
	edges := make([]*layoutgraph.Edge, 0, edgeCount)
	for index := 0; index < edgeCount; index++ {
		y := float64(index * 300)
		from := graph.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(index*2+1), 40, 40))
		to := graph.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(index*2+2), 40, 40))
		from.TopLeft = geo.NewPoint(0, y)
		to.TopLeft = geo.NewPoint(200, y)

		edge := graph.Connect(from, to)
		edge.Points = routeWithSpareCapacity(
			geo.NewPoint(40, y+20),
			geo.NewPoint(40, y+100),
			geo.NewPoint(200, y+100),
			geo.NewPoint(200, y+20),
		)
		edges = append(edges, edge)
	}
	return graph, edges
}

func TestStraightEdgesFallbackCommitsSuccessfulReplacement(t *testing.T) {
	graph, edges := straightFallbackMutationGraph(1)
	original := captureExactRouteTest(edges[0])

	if err := StraightEdgesFallback(t.Context(), graph); err != nil {
		t.Fatal(err)
	}
	if !original.changed() {
		t.Fatal("straight-edge fallback did not replace the route")
	}
	if len(edges[0].Points) != 2 {
		t.Fatalf("route length = %d; want 2", len(edges[0].Points))
	}
	if graph.RoutingCosts().NonCenterPort == 0 {
		t.Fatal("successful fallback did not retain the initialized routing-cost cache")
	}
}

func TestStraightEdgesFallbackRollsBackCancellationAfterReplacement(t *testing.T) {
	for _, test := range []struct {
		name         string
		edgeCount    int
		observeIndex int
	}{
		{name: "final replacement", edgeCount: 1},
		{name: "before later edge", edgeCount: 2},
		{name: "after two replacements", edgeCount: 2, observeIndex: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph, edges := straightFallbackMutationGraph(test.edgeCount)
			snapshots := make([]exactRouteTestSnapshot, len(edges))
			for index, edge := range edges {
				snapshots[index] = captureExactRouteTest(edge)
			}
			wantCosts := graph.RoutingCosts()
			ctx := &cancelWhenRouteMutates{
				Context:  context.Background(),
				snapshot: snapshots[test.observeIndex],
			}

			err := StraightEdgesFallback(ctx, graph)
			requireCanceledAt(t, err, "EdgeRouting")
			if !ctx.observed {
				t.Fatal("context did not observe the straight-edge replacement")
			}
			for _, snapshot := range snapshots {
				snapshot.assertRestored(t)
			}
			if got := graph.RoutingCosts(); got != wantCosts {
				t.Fatalf("routing costs = %+v; want %+v", got, wantCosts)
			}
		})
	}
}

type cancelWhenRoutingCostsChange struct {
	context.Context
	graph    *layoutgraph.Graph
	want     layoutgraph.RoutingCostState
	observed bool
}

func (ctx *cancelWhenRoutingCostsChange) Err() error {
	if ctx.graph.RoutingCosts() != ctx.want {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestStraightEdgesFallbackRestoresCostsWhenCanceledDuringEvaluation(t *testing.T) {
	graph, edges := straightFallbackMutationGraph(1)
	snapshot := captureExactRouteTest(edges[0])
	wantCosts := graph.RoutingCosts()
	ctx := &cancelWhenRoutingCostsChange{
		Context: context.Background(),
		graph:   graph,
		want:    wantCosts,
	}

	err := StraightEdgesFallback(ctx, graph)
	requireCanceledAt(t, err, "EdgeRouting")
	if !ctx.observed {
		t.Fatal("context did not observe routing-cost initialization")
	}
	snapshot.assertRestored(t)
	if got := graph.RoutingCosts(); got != wantCosts {
		t.Fatalf("routing costs = %+v; want %+v", got, wantCosts)
	}
}

func TestStraightEdgesFallbackFinalPollRestoresCostOnlyMutation(t *testing.T) {
	graph, edge := newRoutingTestGraph(200, 0)
	edge.Points = routeWithSpareCapacity(geo.NewPoint(40, 20), geo.NewPoint(200, 20))
	snapshot := captureExactRouteTest(edge)
	wantCosts := graph.RoutingCosts()
	// Allow the entry and preprocessing polls, full straight-line evaluation,
	// and the helper's post-evaluation poll. Cancel on the stage's final poll,
	// after routing costs changed but the already-straight route did not.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 30}

	err := StraightEdgesFallback(ctx, graph)
	requireCanceledAt(t, err, "EdgeRouting")
	snapshot.assertRestored(t)
	if got := graph.RoutingCosts(); got != wantCosts {
		t.Fatalf("routing costs = %+v; want %+v", got, wantCosts)
	}
}

type panicWhenStraightFallbackChanges struct {
	context.Context
	snapshot exactRouteTestSnapshot
	value    any
	observed bool
}

func (ctx *panicWhenStraightFallbackChanges) Err() error {
	if ctx.snapshot.changed() {
		ctx.observed = true
		panic(ctx.value)
	}
	return ctx.Context.Err()
}

func TestStraightEdgesFallbackRollsBackPanicAfterReplacement(t *testing.T) {
	for _, test := range []struct {
		name         string
		edgeCount    int
		observeIndex int
	}{
		{name: "final replacement", edgeCount: 1},
		{name: "before later edge", edgeCount: 2},
		{name: "after two replacements", edgeCount: 2, observeIndex: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph, edges := straightFallbackMutationGraph(test.edgeCount)
			snapshots := make([]exactRouteTestSnapshot, len(edges))
			for index, edge := range edges {
				snapshots[index] = captureExactRouteTest(edge)
			}
			wantCosts := graph.RoutingCosts()
			panicValue := &struct{ name string }{name: "straight-edge fallback mutation probe"}
			ctx := &panicWhenStraightFallbackChanges{
				Context:  context.Background(),
				snapshot: snapshots[test.observeIndex],
				value:    panicValue,
			}

			var recovered any
			func() {
				defer func() {
					recovered = recover()
				}()
				_ = StraightEdgesFallback(ctx, graph)
			}()
			if recovered != panicValue {
				t.Fatalf("recovered panic = %#v; want exact sentinel %#v", recovered, panicValue)
			}
			if !ctx.observed {
				t.Fatal("context did not observe the straight-edge replacement")
			}
			for _, snapshot := range snapshots {
				snapshot.assertRestored(t)
			}
			if got := graph.RoutingCosts(); got != wantCosts {
				t.Fatalf("routing costs = %+v; want %+v", got, wantCosts)
			}
		})
	}
}
