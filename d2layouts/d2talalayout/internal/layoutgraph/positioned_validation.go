package layoutgraph

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const maxPositionedRoutePoints = 1_000_000

// ValidatePositionedGraphSelection checks the topology and positioned route
// geometry consumed by label placement and completed-layout evaluation.
// extraEdges may repeat graph edges once (the normal route-only selection),
// but neither input list may contain aliases within itself.
func ValidatePositionedGraphSelection(ctx context.Context, operation string, graph *Graph, extraEdges []*Edge) error {
	// Keep the central topology preflight first. Besides bounding hidden graph
	// state before candidate allocations, its graph-before-context error order
	// is part of the direct engine API contract.
	if err := validateEngineGraph(ctx, operation, graph); err != nil {
		return err
	}
	if ctx == nil {
		return fmt.Errorf("TALA %s requires a context", operation)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if graph == nil {
		return fmt.Errorf("TALA %s requires a graph", operation)
	}
	if len(graph.Nodes) > limits.MaxEngineNodes {
		return fmt.Errorf("TALA %s node count exceeds limit %d", operation, limits.MaxEngineNodes)
	}
	if len(graph.Edges) > limits.MaxEngineEdges {
		return fmt.Errorf("TALA %s edge count exceeds limit %d", operation, limits.MaxEngineEdges)
	}
	if len(extraEdges) > limits.MaxEngineEdges {
		return fmt.Errorf("TALA %s requested edge count exceeds limit %d", operation, limits.MaxEngineEdges)
	}

	checks := 0
	checkContext := func() error {
		checks++
		if checks%64 != 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
		return nil
	}
	for index, node := range graph.Nodes {
		if err := checkContext(); err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("TALA %s graph node at index %d is nil", operation, index)
		}
		if node.TopLeft == nil {
			return fmt.Errorf("TALA %s graph node %d has no position", operation, node.ID)
		}
		if node.Cluster != nil && node.Cluster.Vessel == nil {
			return fmt.Errorf("TALA %s graph node %d has a cluster without a vessel", operation, node.ID)
		}
		if node.Sequence != nil && node.Sequence.Vessel == nil {
			return fmt.Errorf("TALA %s graph node %d has a sequence without a vessel", operation, node.ID)
		}
	}

	seenGraphEdges := make(map[*Edge]struct{}, len(graph.Edges))
	seenRequestedEdges := make(map[*Edge]struct{}, len(extraEdges))
	validatedEdges := make(map[*Edge]struct{}, len(graph.Edges)+len(extraEdges))
	routePoints := 0
	validateEdge := func(edge *Edge, index int, kind string, seenInList map[*Edge]struct{}) error {
		if err := checkContext(); err != nil {
			return err
		}
		if edge == nil {
			return fmt.Errorf("TALA %s %s at index %d is nil", operation, kind, index)
		}
		if _, duplicate := seenInList[edge]; duplicate {
			return fmt.Errorf("TALA %s %s at index %d repeats edge %d", operation, kind, index, edge.entityID())
		}
		seenInList[edge] = struct{}{}
		if _, validated := validatedEdges[edge]; validated {
			return nil
		}
		validatedEdges[edge] = struct{}{}
		if edge.From == nil || edge.To == nil {
			return fmt.Errorf("TALA %s edge %d has missing endpoints", operation, edge.entityID())
		}
		if len(edge.Points) > maxPositionedRoutePoints-routePoints {
			return fmt.Errorf("TALA %s route point count exceeds limit %d", operation, maxPositionedRoutePoints)
		}
		routePoints += len(edge.Points)
		for pointIndex, point := range edge.Points {
			if err := checkContext(); err != nil {
				return err
			}
			if point == nil {
				return fmt.Errorf("TALA %s edge %d route point at index %d is nil", operation, edge.entityID(), pointIndex)
			}
		}
		if (edge.Label != nil || edge.SourceArrowheadLabel != nil || edge.TargetArrowheadLabel != nil) && len(edge.Points) < 2 {
			return fmt.Errorf("TALA %s labeled edge %d requires at least two route points", operation, edge.entityID())
		}
		return nil
	}
	for index, edge := range graph.Edges {
		if err := validateEdge(edge, index, "graph edge", seenGraphEdges); err != nil {
			return err
		}
	}
	for index, edge := range extraEdges {
		if err := validateEdge(edge, index, "requested edge", seenRequestedEdges); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
