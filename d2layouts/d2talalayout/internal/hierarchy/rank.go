package hierarchy

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

// rankResult is the complete output of hierarchical rank assignment. Levels
// are zero based; levelCount is zero only for an empty graph.
type rankResult struct {
	nodeToLevel map[*layoutgraph.Node]int
	levelCount  int
}

// rankDAG minimizes the weighted sum of edge spans subject to every edge
// spanning at least one level. It returns a normalized deterministic optimum.
// The input must be a connected simple DAG.
//
// The solver follows the feasible-tree network-simplex formulation in
// Gansner et al., IEEE TSE 19(3), 1993. At termination it independently
// constructs the dual flow induced by the final tree; primal/dual equality is
// an exact optimality certificate, not just a feasibility check.
func rankDAG(ctx context.Context, g *layoutgraph.Graph) (rankResult, error) {
	result, err := rankDAGWithLimit(ctx, g, limits.MaxOptimizationWorkUnits)
	if err != nil {
		return rankResult{}, fmt.Errorf("TALA RankDAG failed: %w", err)
	}
	return result, nil
}

func rankDAGWithLimit(ctx context.Context, g *layoutgraph.Graph, workLimit uint64) (rankResult, error) {
	guard, err := limits.NewOptimizationWorkGuard(ctx, "RankDAG", workLimit)
	if err != nil {
		return rankResult{}, err
	}
	problem, err := newRankProblem(g, guard)
	if err != nil {
		return rankResult{}, err
	}
	if len(problem.nodes) == 0 {
		if err := guard.Finish(); err != nil {
			return rankResult{}, err
		}
		return rankResult{nodeToLevel: make(map[*layoutgraph.Node]int)}, nil
	}

	initialLevels, err := problem.longestPathLevels(guard)
	if err != nil {
		return rankResult{}, err
	}
	if len(problem.edges) == 0 {
		if err := guard.Finish(); err != nil {
			return rankResult{}, err
		}
		return rankResult{
			nodeToLevel: map[*layoutgraph.Node]int{problem.nodes[0]: 0},
			levelCount:  1,
		}, nil
	}

	levels, dualValue, err := problem.solve(initialLevels, guard)
	if err != nil {
		return rankResult{}, err
	}
	result, primalValue, err := problem.makeResult(levels, guard)
	if err != nil {
		return rankResult{}, err
	}
	if primalValue != dualValue {
		return rankResult{}, invariant.Errorf("optimality certificate failed: primal cost %d, dual value %d", primalValue, dualValue)
	}
	if err := guard.Finish(); err != nil {
		return rankResult{}, err
	}
	return result, nil
}

type rankEdge struct {
	from   int
	to     int
	weight int64
	id     layoutgraph.EntityID
}

const rankMinSpan int64 = 1

type rankProblem struct {
	nodes         []*layoutgraph.Node
	edges         []rankEdge
	outgoingStart []int
	pivotOrder    []int
}

func (problem *rankProblem) makeResult(levels []int64, guard *limits.OptimizationWorkGuard) (rankResult, int64, error) {
	minLevel := int64(math.MaxInt64)
	maxLevel := int64(math.MinInt64)
	for _, level := range levels {
		if err := guard.Step(); err != nil {
			return rankResult{}, 0, err
		}
		minLevel = min(minLevel, level)
		maxLevel = max(maxLevel, level)
	}

	primalValue := int64(0)
	for _, edge := range problem.edges {
		if err := guard.Step(); err != nil {
			return rankResult{}, 0, err
		}
		span, ok := checkedSubInt64(levels[edge.to], levels[edge.from])
		if !ok {
			return rankResult{}, 0, errors.New("edge span overflow")
		}
		if span < rankMinSpan {
			return rankResult{}, 0, invariant.Errorf("infeasible span %d", span)
		}
		weightedSpan, ok := checkedMulInt64(edge.weight, span)
		if !ok {
			return rankResult{}, 0, errors.New("objective overflow")
		}
		primalValue, ok = checkedAddInt64(primalValue, weightedSpan)
		if !ok {
			return rankResult{}, 0, errors.New("objective overflow")
		}
	}

	levelRange, ok := checkedSubInt64(maxLevel, minLevel)
	if !ok || levelRange > int64(math.MaxInt-1) {
		return rankResult{}, 0, errors.New("level count overflow")
	}
	result := rankResult{
		nodeToLevel: make(map[*layoutgraph.Node]int, len(problem.nodes)),
		levelCount:  int(levelRange) + 1,
	}
	for node, level := range levels {
		if err := guard.Step(); err != nil {
			return rankResult{}, 0, err
		}
		normalized, ok := checkedSubInt64(level, minLevel)
		if !ok {
			return rankResult{}, 0, errors.New("normalized level overflow")
		}
		if normalized > int64(math.MaxInt) {
			return rankResult{}, 0, errors.New("normalized level overflow")
		}
		result.nodeToLevel[problem.nodes[node]] = int(normalized)
	}
	return result, primalValue, nil
}

func checkedSubInt64(a, b int64) (int64, bool) {
	if b > 0 && a < math.MinInt64+b || b < 0 && a > math.MaxInt64+b {
		return 0, false
	}
	return a - b, true
}

func checkedAddInt64(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func checkedMulInt64(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if a == -1 && b == math.MinInt64 || b == -1 && a == math.MinInt64 {
		return 0, false
	}
	product := a * b
	return product, product/b == a
}
