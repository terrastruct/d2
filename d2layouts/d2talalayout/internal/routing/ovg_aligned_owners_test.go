package routing

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestOVGAlignedOwnersMatchesOrderedScan(t *testing.T) {
	rng := rand.New(rand.NewPCG(53, 97))
	ports := make(map[*layoutgraph.Node][]*OVGNode)
	for i := range 64 {
		owner := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 40, 40)
		for range 9 {
			ports[owner] = append(ports[owner], NewOVGNode(geo.NewPoint(float64(rng.IntN(11)-5), float64(rng.IntN(11)-5))))
		}
	}
	guard := newOVGBuildGuardForTest(t.Context(), t)
	index, err := newOVGPortIndex(ports, nil, nil, guard)
	if err != nil {
		t.Fatal(err)
	}
	for x := -6; x <= 6; x++ {
		for y := -6; y <= 6; y++ {
			var want, got []int
			for i, owner := range index.owners {
				for _, port := range ports[owner] {
					if port.X == float64(x) || port.Y == float64(y) {
						want = append(want, i)
						break
					}
				}
			}
			aligned := index.alignedOwners(float64(x), float64(y))
			for {
				owner, ok, err := aligned.next(guard)
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					break
				}
				got = append(got, owner)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("(%d,%d): owners=%v want=%v", x, y, got, want)
			}
		}
	}
}

func TestOVGAlignedOwnerWorkIsBounded(t *testing.T) {
	for _, kind := range []string{"local", "aggregate", "cancellation"} {
		t.Run(kind, func(t *testing.T) {
			var guard *ovgBuildGuard
			ctx := context.Background()
			limits := defaultOVGBuildLimits()
			switch kind {
			case "local":
				limits.work = 6
			case "aggregate":
				aggregate, err := newRouteWorkGuard(ctx, "EdgeRouting", 6)
				if err != nil {
					t.Fatal(err)
				}
				ctx = contextWithRouteAggregateWork(ctx, aggregate)
			case "cancellation":
				ctx = &cancelWhenOVGChanges{Context: ctx, shouldCancel: func() bool { return guard != nil && guard.work >= 6 }}
			}
			var err error
			guard, err = newOVGBuildGuard(ctx, limits)
			if err != nil {
				t.Fatal(err)
			}
			aligned := ovgAlignedOwners{byX: make([]int, 1000), byY: make([]int, 1000)}
			_, _, err = aligned.next(guard)
			switch kind {
			case "local":
				requireOVGResourceError(t, err, "work units")
			case "aggregate":
				if !errors.Is(err, errRouteStageWorkLimit) {
					t.Fatalf("error=%v want aggregate limit", err)
				}
			case "cancellation":
				requireCanceledAt(t, err, "EdgeRouting")
			}
		})
	}
}
