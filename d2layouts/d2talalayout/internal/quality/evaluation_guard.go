package quality

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const (
	// maxEvaluationWorkUnits bounds the complete candidate-scoring pass. The
	// evaluator is used by candidate selection and direct internal callers, so
	// the bound belongs at this shared boundary.
	// The deliberately generous ceiling preserves room for substantially larger
	// diagrams while making the accepted record limits incapable of
	// expanding into hundreds of billions of segment comparisons. Corpus peaks
	// are measured in TestEvaluateWorkLimitCoversLayoutCorpus.
	maxEvaluationWorkUnits int64 = 50_000_000

	// The checked-in completed-layout corpus is measured live by the engine test.
	// Keep a calibrated range rather than one mode-specific exact count: test
	// instrumentation may add bounded checks, while falling below the range can
	// reveal lost work accounting and exceeding it requires an explicit limit
	// review.
	calibratedPublicEvaluationCorpusFloor int64 = 50_000
	calibratedPublicEvaluationCorpusCeil  int64 = 75_000
)

func newEvaluationWorkGuard(ctx context.Context, limit int64) (*limits.WorkGuard, error) {
	guard, err := limits.NewWorkGuard(ctx, "Evaluate", limit)
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, fmt.Errorf("TALA Evaluate work limit must not be negative")
	}
	return guard, nil
}

func chargeEvaluationWork(guard *limits.WorkGuard, amount int) error {
	if amount < 0 {
		return fmt.Errorf("TALA Evaluate work charge must not be negative")
	}
	return guard.Add(int64(amount))
}

// chargeEvaluationAreaWork accounts a conservative upper bound for the legacy
// bounding-box kernel before it runs. This is deliberately charged up front:
// Area may scan all nodes for each outside label and walks each route again
// for edge and arrowhead-label bounds. Rejecting the derived cost first keeps a
// direct engine caller from entering a large, non-allocating legacy kernel that
// cannot itself return an error. WorkGuard.Finish immediately after Area still
// observes cancellation that arrives while a bounded pass is running.
func chargeEvaluationAreaWork(g *layoutgraph.Graph, guard *limits.WorkGuard) error {
	if err := chargeEvaluationWork(guard, len(g.Nodes)); err != nil {
		return err
	}
	for _, node := range g.Nodes {
		if node.Label != nil && node.Label.Position.IsOutside() {
			// Bounding-box calculation can ask each of Leftmost,
			// Topmost, Rightmost, and Bottommost to scan the complete node set.
			for range 4 {
				if err := chargeEvaluationWork(guard, len(g.Nodes)); err != nil {
					return err
				}
			}
		}
	}
	for _, edge := range g.Edges {
		multiplier := 1 // Edge bounding-box calculation always walks the route.
		if edge.Label != nil && len(edge.Points) != 0 && edge.Label.Position != label.Unset {
			multiplier++
		}
		if edge.SourceArrowheadLabel != nil {
			multiplier++
		}
		if edge.TargetArrowheadLabel != nil {
			multiplier++
		}
		for range multiplier {
			if err := chargeEvaluationWork(guard, len(edge.Points)); err != nil {
				return err
			}
		}
	}
	return nil
}

// evaluationIsDescendantOf is a cancellation- and budget-aware ancestry check.
// Candidate payload validation already rejects ancestry
// cycles, but keeping a local depth bound makes the evaluator safe when it is
// exercised directly by internal callers and tests as well.
func evaluationIsDescendantOf(
	maybeDescendant *layoutgraph.Node,
	maybeAncestor *layoutgraph.Node,
	guard *limits.WorkGuard,
) (bool, error) {
	for depth := 0; ; depth++ {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if maybeAncestor == maybeDescendant {
			return true, nil
		}
		if maybeDescendant == nil {
			return false, nil
		}
		if depth >= layoutgraph.MaxTopologyDepth {
			return false, fmt.Errorf("TALA Evaluate ancestry depth exceeds limit %d", layoutgraph.MaxTopologyDepth)
		}
		maybeDescendant = maybeDescendant.AncestryParent()
		if maybeDescendant == nil {
			return maybeAncestor == nil, nil
		}
	}
}
