package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestEstimateRouteCostCountsNonSharedCrossingsWithFixedWidthAccumulator(t *testing.T) {
	first := &layoutgraph.Edge{Points: []*geo.Point{geo.NewPoint(0, 0), geo.NewPoint(10, 10)}}
	second := &layoutgraph.Edge{Points: []*geo.Point{geo.NewPoint(0, 10), geo.NewPoint(10, 0)}}

	var crossings int64 = countNonSharedCrossings(first, second)
	if crossings != 1 {
		t.Fatalf("crossing count = %d, want 1", crossings)
	}
	want := first.Length() + layoutgraph.CrossingCostWeight
	if got := estimateRouteCost(layoutgraph.Edges{first, second}, first); got != want {
		t.Fatalf("route cost = %v, want %v", got, want)
	}
}

func TestEstimateRouteCostGuardedMatchesAndChargesEveryScan(t *testing.T) {
	first := &layoutgraph.Edge{Points: []*geo.Point{geo.NewPoint(0, 0), geo.NewPoint(10, 10)}}
	second := &layoutgraph.Edge{Points: []*geo.Point{geo.NewPoint(0, 10), geo.NewPoint(10, 0)}}
	edges := layoutgraph.Edges{first, second}

	guard, err := newRouteWorkGuard(context.Background(), "route cost", 4)
	if err != nil {
		t.Fatal(err)
	}
	got, err := estimateRouteCostGuarded(edges, first, guard)
	if err != nil {
		t.Fatal(err)
	}
	if want := estimateRouteCost(edges, first); got != want {
		t.Fatalf("guarded route cost = %v, want %v", got, want)
	}
	used := guard.used
	if used != 4 {
		t.Fatalf("guarded route cost charged %d units, want 4", used)
	}

	limited, err := newRouteWorkGuard(context.Background(), "route cost", used-1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := estimateRouteCostGuarded(edges, first, limited); !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("limited route cost error = %v, want route-stage work limit", err)
	}
}

func TestEstimateRouteCostGuardedChargesEdgesWithoutSegments(t *testing.T) {
	edges := make(layoutgraph.Edges, 1_000)
	for index := range edges {
		edges[index] = &layoutgraph.Edge{}
	}
	guard, err := newRouteWorkGuard(context.Background(), "route cost", 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := estimateRouteCostGuarded(edges, &layoutgraph.Edge{}, guard); !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("empty-route scan error = %v, want route-stage work limit", err)
	}
}
