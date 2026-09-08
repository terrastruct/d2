package routing

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// SubgraphRouteObserver observes each completed subgraph route at the exact
// point where its OVG and the graph's current route state agree. Snapshot
// serialization belongs to the engine orchestrator and can be implemented by
// this optional observer without making routing depend on engine.
type SubgraphRouteObserver interface {
	SubgraphRouted(*OVG) error
}

// RouteCompletionObserver observes the route-complete transition inside the
// atomic routing stage. The engine retains pipeline-status ownership while the
// routing guard can still observe cancellation before commit.
type RouteCompletionObserver interface {
	RoutingCompleted()
}

// GraphRouteOptions describes the route state owned by the pipeline around a
// whole-graph routing pass.
type GraphRouteOptions struct {
	ForceReroute              bool
	RoutesPreviouslyCompleted bool
	Observer                  SubgraphRouteObserver
	CompletionObserver        RouteCompletionObserver
}

// RouteGraph runs the complete graph-routing stage. The returned boolean is
// the updated value of RoutesPreviouslyCompleted; on error the caller must
// retain its prior value.
func RouteGraph(ctx context.Context, graph *layoutgraph.Graph, options GraphRouteOptions) (bool, error) {
	return RouteGraphWithWorkLimit(ctx, graph, options, maxEdgeRoutingStageWorkUnits)
}

// RouteGraphWithWorkLimit is RouteGraph with an explicit aggregate budget.
// It exists for deterministic resource-boundary testing inside TALA.
func RouteGraphWithWorkLimit(ctx context.Context, graph *layoutgraph.Graph, options GraphRouteOptions, workLimit uint64) (routingComplete bool, err error) {
	routingComplete = options.RoutesPreviouslyCompleted
	if graph == nil {
		return routingComplete, fmt.Errorf("TALA EdgeRouting requires a graph")
	}
	guard, err := newRouteWorkGuard(ctx, "EdgeRouting", workLimit)
	if err != nil {
		return routingComplete, err
	}

	// Previously completed routes take an early success path below, but still
	// cross the same topology, geometry, and resource boundary as routes built here.
	if err := layoutgraph.Validate(ctx, "EdgeRouting", graph); err != nil {
		return routingComplete, err
	}
	if err := validateRouteStageGeometry(graph, nil, guard); err != nil {
		return routingComplete, err
	}
	if len(graph.Edges) == 0 {
		return routingComplete, guard.finish()
	}

	// Existing routes are authoritative only after this pipeline has completed
	// its first routing pass. A fresh full layout must reroute stale points from
	// a previous layout after moving nodes.
	if !options.ForceReroute && options.RoutesPreviouslyCompleted {
		routedEdges := 0
		for _, edge := range graph.Edges {
			if err := guard.step(); err != nil {
				return routingComplete, err
			}
			if len(edge.Points) == 0 {
				continue
			}
			if !hasCompleteEdgeRoute(edge) {
				return routingComplete, invariant.Errorf(
					"edge %d has an incomplete route: expected at least two non-nil points, got %d",
					edge.IDValue(), len(edge.Points),
				)
			}
			routedEdges++
		}
		if routedEdges == len(graph.Edges) {
			routingComplete = true
			if options.CompletionObserver != nil {
				options.CompletionObserver.RoutingCompleted()
			}
			return routingComplete, guard.finish()
		}
		if routedEdges != 0 {
			return routingComplete, invariant.Errorf(
				"graph is partially routed: %d of %d edges have routes",
				routedEdges, len(graph.Edges),
			)
		}
	}

	err = runAtomicRouteStageWithValidatedGeometry(graph, nil, guard, func(guard *routeWorkGuard) error {
		routingCtx := contextWithRouteAggregateWork(ctx, guard)
		ovgGuard, guardErr := newOVGBuildGuard(routingCtx, defaultOVGBuildLimits())
		if guardErr != nil {
			return guardErr
		}

		subgraphs, splitOwnership, splitErr := graph.SplitSubgraphsTracked(ctx, layoutgraph.SplitOptions{
			IncludeContainers: true,
			IncludeNears:      true,
		}, guard)
		if splitErr != nil {
			return splitErr
		}
		// SplitSubgraphs intentionally points reachable nodes at temporary graphs.
		// Restore the exact prior owner on every exit, including Near-only nodes
		// outside Graph.Nodes and failures before the normal restoration pass.
		defer splitOwnership.Restore()
		if err := guard.add(uint64(len(subgraphs))); err != nil {
			return err
		}
		for _, subgraph := range subgraphs {
			if err := guard.step(); err != nil {
				return err
			}
			if len(subgraph.Edges) == 0 {
				continue
			}
			if err := guard.add(uint64(len(subgraph.Nodes))); err != nil {
				return err
			}
			topLeft, bottomRight, boundsErr := routeStageGraphBoundingBox(subgraph, guard)
			if boundsErr != nil {
				return boundsErr
			}
			topLeft.X -= overshootAmount
			topLeft.Y -= overshootAmount
			bottomRight.X += overshootAmount
			bottomRight.Y += overshootAmount
			nodesInBoundingBox := make([]*layoutgraph.Node, 0)
			for _, otherSubgraph := range subgraphs {
				if err := guard.step(); err != nil {
					return err
				}
				if otherSubgraph == subgraph {
					continue
				}
				for _, node := range otherSubgraph.Nodes {
					if err := guard.step(); err != nil {
						return err
					}
					if node.IsWithinBounds(topLeft, bottomRight) {
						// If another node not in this subgraph overlaps one of its
						// nodes, it may sit on the center or block a port into the
						// center and therefore is not an obstruction.
						overlapsSubgraph := false
						for _, subgraphNode := range subgraph.Nodes {
							if err := guard.step(); err != nil {
								return err
							}
							if node.DoesOverlapExact(subgraphNode) {
								overlapsSubgraph = true
								break
							}
						}
						if !overlapsSubgraph {
							nodesInBoundingBox = append(nodesInBoundingBox, node)
						}
					}
				}
			}

			if err := guard.add(uint64(len(subgraph.Nodes)) + uint64(len(subgraph.Edges))); err != nil {
				return err
			}
			subgraph.ComputeCellSize()
			ovg, routeErr := routeEdgesWithResourceGuards(routingCtx, subgraph, nodesInBoundingBox, ovgGuard, maxRouteSearchWorkUnits)
			if routeErr != nil {
				return routeErr
			}
			// Observe cancellation or an exhausted aggregate budget after routing
			// has mutated edges and before optional engine-owned serialization.
			if err := guard.step(); err != nil {
				return err
			}
			if options.Observer != nil {
				if err := options.Observer.SubgraphRouted(ovg); err != nil {
					return err
				}
			}
		}

		// SplitSubgraphs points members at their temporary graph. Restore every
		// member and descendant once; repeated recursive restoration is quadratic
		// for deep containment trees.
		seenNodes := make(map[*layoutgraph.Node]struct{})
		nodesToRestore := make([]*layoutgraph.Node, 0, len(graph.Nodes))
		enqueue := func(node *layoutgraph.Node) error {
			if err := guard.step(); err != nil {
				return err
			}
			if node == nil {
				return nil
			}
			if _, seen := seenNodes[node]; seen {
				return nil
			}
			seenNodes[node] = struct{}{}
			nodesToRestore = append(nodesToRestore, node)
			return nil
		}
		for _, subgraph := range subgraphs {
			for _, node := range subgraph.Nodes {
				if err := enqueue(node); err != nil {
					return err
				}
			}
		}
		for index := 0; index < len(nodesToRestore); index++ {
			node := nodesToRestore[index]
			if originalGraph, captured := splitOwnership.OriginalGraph(node); captured {
				node.Graph = originalGraph
			} else {
				node.Graph = graph
			}
			if node.IsContainer() {
				for _, child := range graph.Containers[node] {
					if err := enqueue(child); err != nil {
						return err
					}
				}
			}
			if node.IsClusterVessel() {
				if cluster := graph.Clusters[node]; cluster != nil {
					for _, child := range cluster.Nodes {
						if err := enqueue(child); err != nil {
							return err
						}
					}
				}
			}
			if sequence := graph.Sequences[node]; sequence != nil {
				for _, child := range sequence.Nodes {
					if err := enqueue(child); err != nil {
						return err
					}
				}
			}
		}

		routingComplete = true
		if options.CompletionObserver != nil {
			options.CompletionObserver.RoutingCompleted()
		}
		return nil
	})
	return routingComplete, err
}
