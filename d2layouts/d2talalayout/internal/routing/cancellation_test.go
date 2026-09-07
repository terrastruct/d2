package routing

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestRouteEdgesCancellationWaitsForFlavorWorkers(t *testing.T) {
	// A canceled receiver used to close the response channel while route-flavor
	// workers still owned sends to it. Repeat enough times to exercise each
	// scheduling order; a send-after-close crashes the whole test process.
	for i := 0; i < 50; i++ {
		routers := []*ovgEdgeRouter{
			{flavor: ShortestToLongest},
			{flavor: LongestToShortest},
			{flavor: Default},
		}
		_, err := generateRouteFlavorResponses(canceledContext(), routers, false)
		requireCanceledAt(t, err, "EdgeRouting")
	}
}

func TestBestRoutePreservesCancellation(t *testing.T) {
	router := &ovgEdgeRouter{}
	source := layoutgraph.NewNode(1, 10, 10)
	source.TopLeft = geo.NewPoint(0, 0)
	target := layoutgraph.NewNode(2, 10, 10)
	target.TopLeft = geo.NewPoint(100, 0)
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 1}
	_, err := router.bestRoute(ctx, source, target, nil, &OVGNode{}, nil, nil, nil)
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestCollectRouteFlavorResponsesPreservesCancellation(t *testing.T) {
	responses := make(chan routeFlavorResult)
	close(responses)
	got, err := collectRouteFlavorResponses(canceledContext(), responses, func() {})
	if got != nil {
		t.Fatalf("responses = %v, want nil", got)
	}
	requireCanceledAt(t, err, "EdgeRouting")
}
