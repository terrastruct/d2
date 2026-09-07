package quality

import (
	"cmp"
	"context"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// Score preserves the original TALA penalty and separate area tie-breaker.
// Diagnostic geometry from Inspect is deliberately absent from this ordering.
type Score struct{ Penalty, Area float64 }

// Evaluate adapts the original evaluator for bounded refinement callers.
func Evaluate(ctx context.Context, graph *layoutgraph.Graph) (Score, error) {
	penalty, area, err := EvaluateWithArea(ctx, graph)
	return Score{Penalty: penalty, Area: area}, err
}

// Compare orders penalties exactly, then areas, with the same handling of
// non-finite values as the original seed-result comparator.
func (score Score) Compare(other Score) int {
	if comparison := compareNumber(score.Penalty, other.Penalty, false); comparison != 0 {
		return comparison
	}
	return compareNumber(score.Area, other.Area, true)
}

func compareNumber(left, right float64, nonnegative bool) int {
	leftValid := finite(left) && (!nonnegative || left >= 0)
	rightValid := finite(right) && (!nonnegative || right >= 0)
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

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
