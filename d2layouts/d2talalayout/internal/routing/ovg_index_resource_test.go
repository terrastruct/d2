package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func sparseOVGIndexGraph() *layoutgraph.Graph {
	graph := newOVGCancellationGraph(200, 0)
	for i := range 1000 {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+3), 40, 40)
		node.TopLeft = geo.NewPoint(10000, float64(i*100))
		graph.AddNode(node)
	}
	return graph
}

func TestOVGProximityIndexDoesNotChargeEliminatedComparisons(t *testing.T) {
	graph := sparseOVGIndexGraph()
	limits := defaultOVGBuildLimits()
	limits.work = 10_000
	guard, err := newOVGBuildGuard(t.Context(), limits)
	if err != nil {
		t.Fatal(err)
	}
	index, err := newOVGPointProximityIndex(graph, []float64{20}, guard)
	if err != nil {
		t.Fatal(err)
	}
	linearGuard := newOVGBuildGuardForTest(t.Context(), t)
	// Only the first node can be near x=20. The other thousand nodes must
	// consume construction work once, not a full scan on every query.
	for i := range 1000 {
		point := geo.NewPoint(20, float64(i-100))
		got, err := index.pointNear(point.X, point.Y, guard)
		if err != nil {
			t.Fatalf("indexed query %d exhausted a budget sufficient for its actual work: %v", i, err)
		}
		want, err := linearGuard.pointNearGraphNode(graph, point)
		if err != nil || got != want {
			t.Fatalf("point %v: indexed=%v linear=%v err=%v", point, got, want, err)
		}
	}
}

func TestOVGPortIndexDoesNotChargeEliminatedComparisons(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		graph := sparseOVGIndexGraph()
		if blocked {
			obstacle := layoutgraph.NewNode(1003, 20, 40)
			obstacle.TopLeft = geo.NewPoint(80, 0)
			graph.AddNode(obstacle)
		}
		ovg := NewOVG(graph.Nodes)
		ovg.Ports[graph.Nodes[0]] = []*OVGNode{NewOVGNode(geo.NewPoint(40, 20))}
		ovg.Ports[graph.Nodes[1]] = []*OVGNode{NewOVGNode(geo.NewPoint(200, 20))}
		limits := defaultOVGBuildLimits()
		limits.work = 20_000
		guard, err := newOVGBuildGuard(t.Context(), limits)
		if err != nil {
			t.Fatal(err)
		}
		index, err := newOVGPortIndex(ovg.Ports, graph, nil, guard)
		if err != nil {
			t.Fatal(err)
		}
		candidate := NewOVGNode(geo.NewPoint(120, 20))
		// Both ports align horizontally. The remote nodes are irrelevant to
		// their sight lines, while the optional nearby obstacle must block one.
		for i := range 1000 {
			got, err := candidate.hasUnobstructedLineToPorts(ovg, index, 2, guard)
			if err != nil {
				t.Fatalf("blocked=%v query=%d exhausted a budget sufficient for actual work: %v", blocked, i, err)
			}
			if got == blocked {
				t.Fatalf("blocked=%v: unobstructed=%v", blocked, got)
			}
		}
	}
}

func TestOVGIndexConstructionConsumesBudgetsAndCancels(t *testing.T) {
	graph := sparseOVGIndexGraph()
	ports := map[*layoutgraph.Node][]*OVGNode{
		graph.Nodes[0]: {NewOVGNode(geo.NewPoint(20, 20))},
	}
	for name, build := range map[string]func(*ovgBuildGuard) error{
		"proximity": func(guard *ovgBuildGuard) error {
			_, err := newOVGPointProximityIndex(graph, []float64{20}, guard)
			return err
		},
		"ports": func(guard *ovgBuildGuard) error {
			_, err := newOVGPortIndex(ports, graph, nil, guard)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Run("local", func(t *testing.T) {
				limits := defaultOVGBuildLimits()
				limits.work = 10
				guard, err := newOVGBuildGuard(t.Context(), limits)
				if err != nil {
					t.Fatal(err)
				}
				requireOVGResourceError(t, build(guard), "work units")
			})
			t.Run("aggregate", func(t *testing.T) {
				aggregate, err := newRouteWorkGuard(t.Context(), "EdgeRouting", 10)
				if err != nil {
					t.Fatal(err)
				}
				guard := newOVGBuildGuardForTest(contextWithRouteAggregateWork(t.Context(), aggregate), t)
				if err := build(guard); !errors.Is(err, errRouteStageWorkLimit) {
					t.Fatalf("index construction error=%v, want aggregate work limit", err)
				}
			})
			t.Run("cancellation", func(t *testing.T) {
				var guard *ovgBuildGuard
				ctx := &cancelWhenOVGChanges{
					Context:      context.Background(),
					shouldCancel: func() bool { return guard != nil && guard.work >= 10 },
				}
				guard = newOVGBuildGuardForTest(ctx, t)
				requireCanceledAt(t, build(guard), "EdgeRouting")
			})
		})
	}
}
