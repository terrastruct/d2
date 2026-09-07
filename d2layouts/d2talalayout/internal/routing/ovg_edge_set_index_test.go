package routing

import (
	"context"
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/d2lang/d2/lib/geo"
)

func TestOVGEdgeSetIndexedCrossings(t *testing.T) {
	set := newOvgEdgeSet()
	rng := rand.New(rand.NewPCG(143, 731))
	for range 200 {
		x, y := float64(rng.IntN(100))-50.25, float64(rng.IntN(100))-50.25
		length := float64(rng.IntN(15) + 1)
		if rng.IntN(2) == 0 {
			set.add(NewOVGEdge(NewOVGNode(geo.NewPoint(x, y)), NewOVGNode(geo.NewPoint(x+length, y))))
		} else {
			set.add(NewOVGEdge(NewOVGNode(geo.NewPoint(x, y)), NewOVGNode(geo.NewPoint(x, y+length))))
		}
	}
	for range 2000 {
		x, y := float64(rng.IntN(140))-70.25, float64(rng.IntN(140))-70.25
		length := float64(rng.IntN(31) - 15)
		if length == 0 {
			length = 0.5
		}
		other := geo.NewPoint(x+length, y)
		if rng.IntN(2) == 0 {
			other = geo.NewPoint(x, y+length)
		}
		query := NewOVGEdge(NewOVGNode(geo.NewPoint(x, y)), NewOVGNode(other))
		want := false
		for edge := range set.edges {
			if query.sharePoints(edge) {
				continue
			}
			if query.isHorizontal() && edge.isHorizontal() {
				want = query.From.Y == edge.From.Y &&
					max(min(query.From.X, query.To.X), min(edge.From.X, edge.To.X)) <
						min(max(query.From.X, query.To.X), max(edge.From.X, edge.To.X))
			} else if query.isVertical() && edge.isVertical() {
				want = query.From.X == edge.From.X &&
					max(min(query.From.Y, query.To.Y), min(edge.From.Y, edge.To.Y)) <
						min(max(query.From.Y, query.To.Y), max(edge.From.Y, edge.To.Y))
			} else {
				want = intersects(query.From.Point, query.To.Point, edge.From.Point, edge.To.Point)
			}
			if want {
				break
			}
		}
		if got := set.intersectsWith(query); got != want {
			t.Fatalf("query %v -> %v: got %v, want %v", query.From.Point, query.To.Point, got, want)
		}
	}
}

func sparseCrossingFixture(axisCount int) (*ovgEdgeSet, *OVGEdge) {
	set := newOvgEdgeSet()
	for i := range axisCount {
		x := float64(i)
		set.add(NewOVGEdge(NewOVGNode(geo.NewPoint(x, 10)), NewOVGNode(geo.NewPoint(x, 20))))
	}
	x := float64(axisCount) - 2.75
	query := NewOVGEdge(NewOVGNode(geo.NewPoint(x, 15)), NewOVGNode(geo.NewPoint(x+1, 15)))
	return set, query
}

func TestOVGEdgeSetIndexedCrossingWork(t *testing.T) {
	set, query := sparseCrossingFixture(512)
	for _, transpose := range []bool{false, true} {
		if transpose {
			transposed := newOvgEdgeSet()
			for edge := range set.edges {
				transposed.add(NewOVGEdge(NewOVGNode(geo.NewPoint(edge.From.Y, edge.From.X)), NewOVGNode(geo.NewPoint(edge.To.Y, edge.To.X))))
			}
			set = transposed
			query = NewOVGEdge(NewOVGNode(geo.NewPoint(query.From.Y, query.From.X)), NewOVGNode(geo.NewPoint(query.To.Y, query.To.X)))
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		guard, err := newRouteSearchWorkGuard(ctx, ShortestToLongest, 1000)
		if err != nil {
			t.Fatal(err)
		}
		got, err := set.intersectsWithGuarded(query, guard)
		// 510 axes before the crossing, its axis, and its edge.
		if err != nil || !got || guard.used != 512 {
			t.Fatalf("transpose=%v: intersection=%v, work=%d, err=%v", transpose, got, guard.used, err)
		}
		guard, err = newRouteSearchWorkGuard(ctx, ShortestToLongest, 100)
		if err != nil {
			t.Fatal(err)
		}
		got, err = set.intersectsWithGuarded(query, guard)
		if got || !errors.Is(err, errRouteSearchWorkLimit) || guard.used != 100 {
			t.Fatalf("transpose=%v: limited intersection=%v, work=%d, err=%v", transpose, got, guard.used, err)
		}
	}
}

func TestOVGEdgeSetIndexedCrossingAggregateWork(t *testing.T) {
	set, query := sparseCrossingFixture(512)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	aggregate, err := newRouteWorkGuard(ctx, "EdgeRouting", 100)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := newRouteSearchWorkGuard(contextWithRouteAggregateWork(ctx, aggregate), ShortestToLongest, 1000)
	if err != nil {
		t.Fatal(err)
	}
	got, err := set.intersectsWithGuarded(query, guard)
	if got || !errors.Is(err, errRouteStageWorkLimit) || guard.used != 101 || aggregate.used != 100 {
		t.Fatalf("intersection=%v, work=%d, aggregate=%d, err=%v", got, guard.used, aggregate.used, err)
	}
}

func TestOVGEdgeSetIndexedCrossingSyntheticCancellation(t *testing.T) {
	set, query := sparseCrossingFixture(512)
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 10}
	guard, err := newRouteSearchWorkGuard(ctx, ShortestToLongest, 1000)
	if err != nil {
		t.Fatal(err)
	}
	got, err := set.intersectsWithGuarded(query, guard)
	if got || !errors.Is(err, context.Canceled) || guard.used != 9 {
		t.Fatalf("intersection=%v, work=%d, err=%v", got, guard.used, err)
	}
}

func BenchmarkOVGEdgeSetSparseCrossing(b *testing.B) {
	set, query := sparseCrossingFixture(512)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	guard, err := newRouteSearchWorkGuard(ctx, ShortestToLongest, ^uint64(0))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		got, err := set.intersectsWithGuarded(query, guard)
		if err != nil || !got {
			b.Fatalf("intersection=%v, err=%v", got, err)
		}
	}
}
