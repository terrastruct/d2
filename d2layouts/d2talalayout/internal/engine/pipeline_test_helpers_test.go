package engine

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/log"
)

func withTestLogger(ctx context.Context, tb testlog.TB) context.Context {
	tb.Helper()
	return log.With(ctx, testlog.New(tb))
}

func newHierarchyWithLevels(levels map[*layoutgraph.Node]int) *layoutgraph.Hierarchy {
	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.ReplaceLevels(levels)
	return hierarchy
}

func (p *pipeline) edgeRoutingStageWithWorkLimit(ctx context.Context, workLimit uint64) error {
	return p.runGraphRouting(ctx, &workLimit)
}

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

func allEdgesHaveCompleteRoutes(edges []*layoutgraph.Edge) bool {
	if len(edges) == 0 {
		return false
	}
	for _, edge := range edges {
		if !hasCompleteEdgeRoute(edge) {
			return false
		}
	}
	return true
}

func layoutWithSnapshots(ctx context.Context, graph *layoutgraph.Graph, seed int64, storeSnapshots bool) (*pipeline, error) {
	return runLayout(ctx, graph, LayoutOptions{
		Seed: seed,
	}, pipelineInstrumentation{storeSnapshots: storeSnapshots}, true)
}

func layoutWithStageTimings(ctx context.Context, graph *layoutgraph.Graph, seed int64) (*pipeline, error) {
	return runLayout(ctx, graph, LayoutOptions{
		Seed: seed,
	}, pipelineInstrumentation{measureStageDurations: true}, true)
}
