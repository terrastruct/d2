package routing

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func hasCompleteEdgeRoute(edge *layoutgraph.Edge) bool {
	if len(edge.Points) < 2 {
		return false
	}
	for _, point := range edge.Points {
		if point == nil {
			return false
		}
	}
	return true
}

func RouteEdges(ctx context.Context, g *layoutgraph.Graph, edges []*layoutgraph.Edge) error {
	return routeEdgesWithBudgets(ctx, g, edges, defaultOVGBuildLimits(), maxRouteStageWorkUnits)
}

func routeEdgesWithBudgets(ctx context.Context, g *layoutgraph.Graph, edges []*layoutgraph.Edge, ovgLimits ovgBuildLimits, workLimit uint64) error {
	if g == nil {
		return fmt.Errorf("cannot route edges on a nil graph")
	}
	if ctx == nil {
		return fmt.Errorf("cannot route edges without a context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Reject caller-owned selections before using their length as an allocation
	// hint. Accepted selections are charged again inside the atomic boundary.
	if int64(len(edges)) > layoutgraph.MaxTopologyReferences {
		return fmt.Errorf("TALA EdgeRouting selected edge references exceed limit %d", layoutgraph.MaxTopologyReferences)
	}
	if err := layoutgraph.Validate(ctx, "RouteEdges", g); err != nil {
		return err
	}
	if len(edges) == 0 {
		return nil
	}
	if len(g.Nodes) == 0 {
		return fmt.Errorf("cannot route edges on an empty graph")
	}
	// A valid selection is duplicate-free and contains only graph edges, so it
	// can never be larger than the graph inventory. Check before map allocation.
	if len(edges) > len(g.Edges) {
		return fmt.Errorf("selected edge count %d exceeds graph edge count %d", len(edges), len(g.Edges))
	}

	graphNodes := make(map[*layoutgraph.Node]struct{}, len(g.Nodes))
	for index, node := range g.Nodes {
		if node == nil {
			return fmt.Errorf("graph node at index %d is nil", index)
		}
		graphNodes[node] = struct{}{}
	}
	graphEdges := make(map[*layoutgraph.Edge]struct{}, len(g.Edges))
	for index, edge := range g.Edges {
		if edge == nil {
			return fmt.Errorf("graph edge at index %d is nil", index)
		}
		if edge.From == nil || edge.To == nil || edge.From.TopLeft == nil || edge.To.TopLeft == nil {
			return fmt.Errorf("graph edge %d has unplaced or missing endpoints", edge.IDValue())
		}
		if _, ok := graphNodes[edge.From]; !ok {
			return fmt.Errorf("graph edge %d source node does not belong to the graph", edge.IDValue())
		}
		if _, ok := graphNodes[edge.To]; !ok {
			return fmt.Errorf("graph edge %d target node does not belong to the graph", edge.IDValue())
		}
		graphEdges[edge] = struct{}{}
	}
	selected := make(map[*layoutgraph.Edge]struct{}, len(edges))
	for _, edge := range edges {
		if edge == nil {
			return fmt.Errorf("cannot route a nil edge")
		}
		if _, ok := graphEdges[edge]; !ok {
			return fmt.Errorf("edge %d does not belong to the graph", edge.IDValue())
		}
		if _, duplicate := selected[edge]; duplicate {
			return fmt.Errorf("edge %d was selected more than once", edge.IDValue())
		}
		selected[edge] = struct{}{}
	}
	for _, edge := range g.Edges {
		if _, ok := selected[edge]; ok {
			continue
		}
		if !hasCompleteEdgeRoute(edge) {
			return fmt.Errorf("unselected edge %d has no complete route", edge.IDValue())
		}
	}

	// Standalone routing is one API transaction and one aggregate post-routing
	// work budget. OVG construction and label placement retain their independent
	// derived-resource guards inside this exact rollback boundary.
	return runAtomicRouteStageAfterPreflight(ctx, "EdgeRouting", g, edges, workLimit, func(guard *routeWorkGuard) error {
		if err := guard.add(uint64(len(g.Nodes))); err != nil {
			return err
		}
		g.ComputeCellSize()

		if _, err := routeAdditionalEdgesWithLimits(ctx, g, edges, ovgLimits); err != nil {
			return err
		}
		if err := guard.step(); err != nil {
			return err
		}

		for _, edge := range edges {
			if edge.HasTableColumn() {
				if err := guard.step(); err != nil {
					return err
				}
				continue
			}
			if err := tryStraightEdgeFallbackGuarded(g, edge, guard); err != nil {
				return err
			}
		}

		lockedEdges := make([]*layoutgraph.Edge, 0, len(g.Edges))
		for _, edge := range g.Edges {
			if err := guard.step(); err != nil {
				return err
			}
			if _, selected := selected[edge]; !selected {
				lockedEdges = append(lockedEdges, edge)
			}
		}
		regularEdges := make([]*layoutgraph.Edge, 0, len(edges))
		for _, edge := range edges {
			if err := guard.step(); err != nil {
				return err
			}
			if isSpecialEdgeForBalancing(g, edge) {
				lockedEdges = append(lockedEdges, edge)
				continue
			}
			regularEdges = append(regularEdges, edge)
		}
		if err := balanceRegularEdgesGuarded(g, lockedEdges, regularEdges, guard); err != nil {
			return err
		}
		for _, edge := range edges {
			if err := traceToShapeBorderGuarded(edge, guard); err != nil {
				return err
			}
		}
		if err := reorderDuplicatesInEdgesGuarded(edges, guard); err != nil {
			return err
		}
		if err := guard.step(); err != nil {
			return err
		}
		if err := labeling.PlaceNewEdges(ctx, g, edges); err != nil {
			return err
		}
		return guard.finish()
	})
}
