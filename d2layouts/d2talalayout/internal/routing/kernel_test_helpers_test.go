package routing

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func newHierarchyWithLevels(levels map[*layoutgraph.Node]int) *layoutgraph.Hierarchy {
	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.ReplaceLevels(levels)
	return hierarchy
}

func newOVGBuildGuardForTest(ctx context.Context, t testing.TB) *ovgBuildGuard {
	t.Helper()
	guard, err := newOVGBuildGuard(ctx, defaultOVGBuildLimits())
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func newBackgroundOVGBuildGuardForTest(t testing.TB) *ovgBuildGuard {
	return newOVGBuildGuardForTest(context.Background(), t)
}

func buildOVGFromGraph(ctx context.Context, g *layoutgraph.Graph, nearbyNodes []*layoutgraph.Node) (*OVG, error) {
	return buildOVGFromGraphWithLimits(ctx, g, nearbyNodes, defaultOVGBuildLimits())
}

// routeEdges routes edges in the given subgraph.
// `nearbyNodes` are nodes inside the subgraph bounding box, but that aren't part of the subgraph
func routeEdges(ctx context.Context, g *layoutgraph.Graph, nearbyNodes []*layoutgraph.Node) (*OVG, error) {
	return routeEdgesWithSearchWorkLimit(ctx, g, nearbyNodes, maxRouteSearchWorkUnits)
}

func routeEdgesWithSearchWorkLimit(ctx context.Context, g *layoutgraph.Graph, nearbyNodes []*layoutgraph.Node, searchWorkLimit uint64) (*OVG, error) {
	guard, err := newOVGBuildGuard(ctx, defaultOVGBuildLimits())
	if err != nil {
		return nil, err
	}
	return routeEdgesWithResourceGuards(ctx, g, nearbyNodes, guard, searchWorkLimit)
}

func routeEdgesWithLimits(ctx context.Context, g *layoutgraph.Graph, edges []*layoutgraph.Edge, limits ovgBuildLimits) error {
	return routeEdgesWithBudgets(ctx, g, edges, limits, maxRouteStageWorkUnits)
}

func routeEdgesWithWorkLimit(ctx context.Context, g *layoutgraph.Graph, edges []*layoutgraph.Edge, workLimit uint64) error {
	return routeEdgesWithBudgets(ctx, g, edges, defaultOVGBuildLimits(), workLimit)
}

func buildTunnelsForTest(t testing.TB, g *layoutgraph.Graph) []*Tunnel {
	t.Helper()
	tunnels, err := buildTunnels(g, newBackgroundOVGBuildGuardForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	return tunnels
}

func newOVGEdgeRouter(ctx context.Context, flavor RouteGenerationFlavor, ovg *OVG, g *layoutgraph.Graph, existingRoutes []*Route, edges []*layoutgraph.Edge) (*ovgEdgeRouter, error) {
	return newOVGEdgeRouterWithWorkLimit(ctx, flavor, ovg, g, existingRoutes, edges, maxRouteSearchWorkUnits)
}
