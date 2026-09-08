package quality

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

// Metrics describe a positioned drawing for bounded repair triggers and safety
// checks. They are not weights or ordering components in the layout score.
// Obstruction measurements cover rectangular shapes and polygonal routes only.
type Metrics struct {
	NodeOverlaps, RouteObstructions int64
	TextOcclusions                  int64
	Crossings, Detour, RouteLength  float64
}

// Inspect reads diagnostics without mutating the graph or influencing Score.
func Inspect(ctx context.Context, graph *layoutgraph.Graph) (Metrics, error) {
	metrics, _, err := inspectWithLimit(ctx, graph, maxEvaluationWorkUnits)
	return metrics, err
}

func inspectWithLimit(ctx context.Context, graph *layoutgraph.Graph, workLimit int64) (Metrics, int64, error) {
	if graph == nil {
		return Metrics{}, 0, fmt.Errorf("cannot evaluate a nil graph")
	}
	if err := layoutgraph.ValidatePositionedGraph(ctx, "Inspect", graph); err != nil {
		return Metrics{}, 0, err
	}
	guard, err := limits.NewWorkGuard(ctx, "Inspect", workLimit)
	if err != nil {
		return Metrics{}, 0, err
	}
	var score Metrics
	fail := func(err error) (Metrics, int64, error) { return Metrics{}, guard.Used(), err }
	// Normalize by typical noncontainer dimensions, rather than drawing bounds:
	// opening empty space or expanding a container cannot make wire cheaper.
	scale, nodeCount := 0.0, 0
	for _, node := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return fail(err)
		}
		if !node.IsContainer() && !node.IsInvisible {
			scale += (node.Width + node.Height) / 2
			nodeCount++
		}
	}
	scale = math.Max(1, scale/float64(max(1, nodeCount)))
	visible := make([]*layoutgraph.Edge, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		if err := guard.Step(); err != nil {
			return fail(err)
		}
		if edge.IsInvisible {
			continue
		}
		visible = append(visible, edge)
		length := 0.0
		for i := 1; i < len(edge.Points); i++ {
			if err := guard.Step(); err != nil {
				return fail(err)
			}
			a, b := edge.Points[i-1], edge.Points[i]
			length += math.Hypot(b.X-a.X, b.Y-a.Y)
		}
		score.RouteLength += length / scale
		if len(edge.Points) < 2 || edge.IsLoop() {
			continue
		}
		first, last := edge.Points[0], edge.Points[len(edge.Points)-1]
		direct := math.Abs(last.X-first.X) + math.Abs(last.Y-first.Y)
		score.Detour += math.Max(0, length-direct) / math.Max(scale, direct)

	}
	crossings, err := countNonSharedCrossings(visible, guard)
	if err != nil {
		return fail(err)
	}
	score.Crossings = float64(crossings)
	if err := measureGeometry(graph, &score, guard); err != nil {
		return fail(err)
	}
	if err := measureLabels(graph, &score, guard); err != nil {
		return fail(err)
	}
	if !finite(score.RouteLength) || !finite(score.Detour) {
		return fail(fmt.Errorf("TALA Inspect produced non-finite geometry"))
	}
	if err := guard.Finish(); err != nil {
		return fail(err)
	}
	return score, guard.Used(), nil
}
