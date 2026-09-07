package d2talalayout

import (
	"cmp"
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/quality"
)

// layoutScore is the stable ordering value for a completed layout attempt. Lower
// penalty is better; area breaks otherwise equal penalties.
type layoutScore struct {
	penalty float64
	area    float64
}

// compare reports whether score sorts before, with, or after other. It returns
// -1 when score is better, 0 when both scores are equivalent, and 1 when score
// is worse.
func (score layoutScore) compare(other layoutScore) int {
	if comparison := comparePenalty(score.penalty, other.penalty); comparison != 0 {
		return comparison
	}
	return compareArea(score.area, other.area)
}

func compareArea(left, right float64) int {
	leftValid := isFiniteResultNumber(left) && left >= 0
	rightValid := isFiniteResultNumber(right) && right >= 0
	if leftValid && !rightValid {
		return -1
	}
	if !leftValid && rightValid {
		return 1
	}
	if !leftValid {
		return 0
	}
	return cmp.Compare(left, right)
}

func comparePenalty(left, right float64) int {
	leftFinite := isFiniteResultNumber(left)
	rightFinite := isFiniteResultNumber(right)
	if leftFinite && !rightFinite {
		return -1
	}
	if !leftFinite && rightFinite {
		return 1
	}
	if !leftFinite {
		return 0
	}
	// Approximate equality is not transitive, so using a tolerance here can
	// make the selected seed depend on the order that workers finish.
	return cmp.Compare(left, right)
}

// seedResult is a successfully validated and scored local layout attempt.
type seedResult struct {
	bindings      translation
	graph         *layoutgraph.Graph
	sequenceEdges map[layoutgraph.EntityID]struct{}
	score         layoutScore
}

// evaluateSeedResult validates a completed local attempt against its immutable
// input and computes its ordering score.
func evaluateSeedResult(ctx context.Context, input seedInput, graph *layoutgraph.Graph) (seedResult, error) {
	if err := ctx.Err(); err != nil {
		return seedResult{}, err
	}
	if input.graph == nil {
		return seedResult{}, fmt.Errorf("TALA seed input is empty")
	}
	if graph == nil {
		return seedResult{}, fmt.Errorf("TALA seed result is empty")
	}
	if err := validateCompletedGraph(ctx, graph); err != nil {
		return seedResult{}, fmt.Errorf("validate TALA seed result: %w", err)
	}
	sequenceEdges, err := validateLayoutResultTopology(ctx, input.graph, graph)
	if err != nil {
		return seedResult{}, fmt.Errorf("validate TALA seed result topology: %w", err)
	}
	if err := validateLayoutResultMetadata(ctx, input.graph, graph, sequenceEdges); err != nil {
		return seedResult{}, fmt.Errorf("validate TALA seed result metadata: %w", err)
	}
	penalty, area, err := quality.EvaluateWithArea(ctx, graph)
	if err != nil {
		return seedResult{}, err
	}
	if !isFiniteResultNumber(penalty) {
		return seedResult{}, fmt.Errorf("TALA seed result has a non-finite score")
	}
	return seedResult{
		bindings:      input.bindings,
		graph:         graph,
		sequenceEdges: sequenceEdges,
		score:         layoutScore{penalty: penalty, area: area},
	}, nil
}
