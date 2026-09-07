package engine

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/hierarchy"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placement"
)

// CompoundCandidate gives the selected drawing at most one rigid-container
// placement trial. It never runs another full seed pipeline. The adapter must
// validate and compare the completed proposal against the ordinary incumbent.
func CompoundCandidate(ctx context.Context, graph *layoutgraph.Graph) (*layoutgraph.Graph, error) {
	if ctx == nil || graph == nil {
		return nil, fmt.Errorf("TALA compound candidate requires a context and graph")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(graph.Nodes) > 128 || len(graph.Edges) > 256 || len(graph.Containers[nil]) < 3 || len(graph.Containers[nil]) > 64 {
		return graph, nil
	}
	detailed := false
	for _, node := range graph.Containers[nil] {
		detailed = detailed || node.IsContainer()
	}
	if !detailed || graph.HasFixedNode() {
		return graph, nil
	}
	candidate, err := layoutgraph.Clone(ctx, graph)
	if err != nil {
		return nil, err
	}
	// This is a deterministic post-selection proposal, independent of seed
	// completion order. Requested seeds still run exactly once each.
	changed, err := hierarchy.PlaceCompound(ctx, candidate, rand.New(rand.NewSource(0)))
	if err != nil {
		return nil, err
	}
	if !changed {
		return graph, nil
	}
	// This is an outer-flow refinement. Avoid a routing trial when its block
	// arrangement makes more external connections cross between disjoint lanes.
	// This is an admission heuristic, not a bound on the completed layout score:
	// routing and labels still determine whether an admitted proposal is kept.
	worseAlignment := compoundCrossAxisDetours(candidate) > compoundCrossAxisDetours(graph)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if worseAlignment {
		return graph, nil
	}
	if err := reroutePlaced(ctx, candidate); err != nil {
		return nil, err
	}
	tl, br := candidate.BoundingBox()
	if tl != nil && br != nil && (br.X-tl.X > maxGraphSize || br.Y-tl.Y > maxGraphSize) {
		return graph, nil
	}
	return candidate, nil
}

// compoundCrossAxisDetours counts external connections whose outer blocks have
// disjoint projections perpendicular to the flow. Their logical blocks offer
// no shared lane along that flow; labels and visual modifiers are not included.
// Endpoints inside the same rigid block do not participate: their arrangement
// is unchanged by this proposal.
func compoundCrossAxisDetours(g *layoutgraph.Graph) int {
	rootOf := make(map[*layoutgraph.Node]*layoutgraph.Node, len(g.Nodes))
	for _, n := range g.Nodes {
		root := n
		for root.Container != nil {
			root = root.Container
		}
		rootOf[n] = root
	}
	horizontal := g.Direction(nil).IsHorizontal()
	detours := 0
	for _, edge := range g.Edges {
		from, to := rootOf[edge.From], rootOf[edge.To]
		if from == to {
			continue
		}
		if horizontal {
			if from.TopLeft.Y+from.Height < to.TopLeft.Y || to.TopLeft.Y+to.Height < from.TopLeft.Y {
				detours++
			}
		} else if from.TopLeft.X+from.Width < to.TopLeft.X || to.TopLeft.X+to.Width < from.TopLeft.X {
			detours++
		}
	}
	return detours
}

func reroutePlaced(ctx context.Context, g *layoutgraph.Graph) error {
	p := newPipeline(g, 0, false)
	p.resetFullLayoutRouteState()
	// Reuse routing and label cleanup on placed nodes without regrouping,
	// rescaling, or invoking another full seed attempt.
	stages := []func(context.Context) error{
		p.edgeRoutingStage, p.simplifyEdgeRoutes, p.swapEdgePorts,
		p.straightEdgesFallback, p.balanceEdgeSegments,
		p.fixClusterEdgeBranching, p.traceEdgesToShapeBorder,
		p.reorderDuplicates, p.placeLabels,
	}
	for _, stage := range stages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := stage(ctx); err != nil {
			return err
		}
	}
	// Authored fixed origins have already had prescale padding removed.
	if !g.HasFixedNode() {
		placement.Normalize(g)
	}
	return ctx.Err()
}
