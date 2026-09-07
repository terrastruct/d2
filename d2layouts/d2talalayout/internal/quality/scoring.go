// Package quality evaluates completed TALA layouts. It reads layoutgraph's
// positioned graph model without mutating it; lower scores are better.
package quality

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// EvaluateWithArea returns the human-facing layout score and the graph area
// separately. Area is a strict tie-breaker: encoding it into the float score
// made 100 smaller than 99 (0.1 versus 0.99) and could outweigh real layout
// differences.
func EvaluateWithArea(ctx context.Context, graph *layoutgraph.Graph) (float64, float64, error) {
	score, area, _, err := evaluateWithAreaLimit(ctx, graph, maxEvaluationWorkUnits)
	return score, area, err
}

func evaluateWithAreaLimit(ctx context.Context, graph *layoutgraph.Graph, workLimit int64) (float64, float64, int64, error) {
	if graph == nil {
		return 0, 0, 0, fmt.Errorf("cannot evaluate a nil graph")
	}
	// Keep this preflight at the quality boundary so candidate evaluation and
	// direct internal callers receive the same resource bounds.
	// It runs before label-overlap scratch slices and maps are constructed.
	if err := layoutgraph.ValidatePositionedGraph(ctx, "Evaluate", graph); err != nil {
		return 0, 0, 0, err
	}
	guard, err := newEvaluationWorkGuard(ctx, workLimit)
	if err != nil {
		return 0, 0, 0, err
	}
	score := 0.0
	for _, e := range graph.Edges {
		if err := guard.Step(); err != nil {
			return 0, 0, guard.Used(), err
		}
		if e.From.Cluster != nil || e.To.Cluster != nil {
			continue
		}
		turns := float64(max(0, len(e.Points)-2))
		score += turns * 0.5

		for i := 0; i < len(e.Points)-1; i++ {
			if err := guard.Step(); err != nil {
				return 0, 0, guard.Used(), err
			}
			curr := e.Points[i]
			next := e.Points[i+1]
			// Extra penalty for diagonals
			if curr.X != next.X && curr.Y != next.Y {
				score += 3
			}
		}
	}
	crossings, err := countNonSharedCrossings(graph.Edges, guard)
	if err != nil {
		return 0, 0, guard.Used(), err
	}
	score += float64(crossings)
	labelScore, err := scoreExistingLabelPlacements(graph, guard)
	if err != nil {
		return 0, 0, guard.Used(), err
	}
	score += 1.0 - labelScore
	if err := chargeEvaluationAreaWork(graph, guard); err != nil {
		return 0, 0, guard.Used(), err
	}
	area := graph.Area()
	if err := guard.Finish(); err != nil {
		return 0, 0, guard.Used(), err
	}
	return score, area, guard.Used(), nil
}
