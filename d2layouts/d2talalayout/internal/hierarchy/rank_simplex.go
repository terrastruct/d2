package hierarchy

import (
	"errors"
	"math"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

// rankSimplex is the graphical network-simplex formulation from Gansner,
// Koutsofios, North, and Vo, "A Technique for Drawing Directed Graphs,"
// IEEE TSE 19(3), 1993, section 2.3. It works directly with feasible ranks,
// tight spanning trees, and the cut values of tree edges.
type rankSimplex struct {
	problem       *rankProblem
	incidentStart []int
	incidentEdges []int
	balance       []int64
	scratch       rankSimplexScratch
}

// rankSimplexScratch is reused across tree exchanges. A rankSimplex belongs to
// one solve, so none of these slices are shared across calls to rankDAG.
type rankSimplexScratch struct {
	parent          []int
	parentEdge      []int
	order           []int
	stack           []int
	subtreeBalance  []int64
	cutValue        []int64
	head            []bool
	queue           []int
	computedBalance []int64
}

func (problem *rankProblem) solve(initialLevels []int64, guard *limits.OptimizationWorkGuard) ([]int64, int64, error) {
	solver, err := newRankSimplex(problem, guard)
	if err != nil {
		return nil, 0, err
	}
	levels := slices.Clone(initialLevels)
	tree, err := solver.feasibleTree(levels, guard)
	if err != nil {
		return nil, 0, err
	}
	if err := solver.optimize(levels, tree, guard); err != nil {
		return nil, 0, err
	}
	dualValue, err := solver.certify(levels, tree, guard)
	if err != nil {
		return nil, 0, err
	}
	return levels, dualValue, nil
}

func newRankSimplex(problem *rankProblem, guard *limits.OptimizationWorkGuard) (*rankSimplex, error) {
	nodeCount := len(problem.nodes)
	degree := make([]int, nodeCount)
	balance := make([]int64, nodeCount)
	for _, edge := range problem.edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		degree[edge.from]++
		degree[edge.to]++
		var ok bool
		balance[edge.from], ok = checkedAddInt64(balance[edge.from], edge.weight)
		if !ok {
			return nil, errors.New("supply overflow")
		}
		balance[edge.to], ok = checkedSubInt64(balance[edge.to], edge.weight)
		if !ok {
			return nil, errors.New("demand overflow")
		}
	}

	incidentStart := make([]int, nodeCount+1)
	for node, count := range degree {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		incidentStart[node+1] = incidentStart[node] + count
	}
	incidentEdges := make([]int, len(problem.edges)*2)
	next := slices.Clone(incidentStart[:nodeCount])
	for _, edgeIndex := range problem.pivotOrder {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		edge := problem.edges[edgeIndex]
		incidentEdges[next[edge.from]] = edgeIndex
		next[edge.from]++
		incidentEdges[next[edge.to]] = edgeIndex
		next[edge.to]++
	}

	total := int64(0)
	for _, amount := range balance {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		var ok bool
		total, ok = checkedAddInt64(total, amount)
		if !ok {
			return nil, errors.New("balance overflow")
		}
	}
	if total != 0 {
		return nil, invariant.Errorf("unbalanced supplies: %d", total)
	}
	return &rankSimplex{
		problem:       problem,
		incidentStart: incidentStart,
		incidentEdges: incidentEdges,
		balance:       balance,
		scratch: rankSimplexScratch{
			parent:          make([]int, nodeCount),
			parentEdge:      make([]int, nodeCount),
			order:           make([]int, 0, nodeCount),
			stack:           make([]int, 0, nodeCount),
			subtreeBalance:  make([]int64, nodeCount),
			cutValue:        make([]int64, len(problem.edges)),
			head:            make([]bool, nodeCount),
			queue:           make([]int, 0, nodeCount),
			computedBalance: make([]int64, nodeCount),
		},
	}, nil
}

// feasibleTree implements figure 2-2 of Gansner et al. Starting from the
// longest-path feasible ranking, it grows a maximal tree of tight edges. When
// that tree does not span, it shifts the tree by the minimum boundary slack so
// at least one more edge becomes tight. The graph stays feasible throughout.
func (solver *rankSimplex) feasibleTree(levels []int64, guard *limits.OptimizationWorkGuard) ([]bool, error) {
	nodeCount := len(solver.problem.nodes)
	tree := make([]bool, len(solver.problem.edges))
	inTree := make([]bool, nodeCount)
	inTree[0] = true
	treeNodeCount := 1
	queue := []int{0}

	growTight := func() error {
		for head := 0; head < len(queue); head++ {
			if err := guard.Step(); err != nil {
				return err
			}
			node := queue[head]
			for i := solver.incidentStart[node]; i < solver.incidentStart[node+1]; i++ {
				if err := guard.Step(); err != nil {
					return err
				}
				edgeIndex := solver.incidentEdges[i]
				edge := solver.problem.edges[edgeIndex]
				other := edge.from
				if other == node {
					other = edge.to
				}
				if inTree[other] {
					continue
				}
				slack, err := rankSlack(edge, levels)
				if err != nil {
					return err
				}
				if slack != 0 {
					continue
				}
				tree[edgeIndex] = true
				inTree[other] = true
				treeNodeCount++
				queue = append(queue, other)
			}
		}
		queue = queue[:0]
		return nil
	}
	if err := growTight(); err != nil {
		return nil, err
	}

	boundary := make([]int, 0, len(solver.problem.edges))
	for treeNodeCount < nodeCount {
		bestSlack := int64(math.MaxInt64)
		boundary = boundary[:0]
		for _, edgeIndex := range solver.problem.pivotOrder {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			edge := solver.problem.edges[edgeIndex]
			if inTree[edge.from] == inTree[edge.to] {
				continue
			}
			slack, err := rankSlack(edge, levels)
			if err != nil {
				return nil, err
			}
			if slack < bestSlack {
				bestSlack = slack
				boundary = append(boundary[:0], edgeIndex)
			} else if slack == bestSlack {
				boundary = append(boundary, edgeIndex)
			}
		}
		if len(boundary) == 0 {
			return nil, invariant.New("could not construct a spanning feasible tree")
		}

		selected := solver.problem.edges[boundary[0]]
		selectedTailInTree := inTree[selected.from]
		delta := bestSlack
		if !selectedTailInTree {
			var ok bool
			delta, ok = checkedSubInt64(0, bestSlack)
			if !ok {
				return nil, errors.New("feasible-tree shift overflow")
			}
		}
		for node, present := range inTree {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if !present {
				continue
			}
			var ok bool
			levels[node], ok = checkedAddInt64(levels[node], delta)
			if !ok {
				return nil, errors.New("feasible-tree level overflow")
			}
		}

		// All minimum-slack boundary edges whose slack decreased are now
		// tight. Add them in canonical edge order, then traverse any tight
		// component reached through their outside endpoints.
		for _, edgeIndex := range boundary {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			edge := solver.problem.edges[edgeIndex]
			if inTree[edge.from] == inTree[edge.to] {
				continue
			}
			tailInTree := inTree[edge.from]
			if bestSlack != 0 && tailInTree != selectedTailInTree {
				continue
			}
			slack, err := rankSlack(edge, levels)
			if err != nil {
				return nil, err
			}
			if slack != 0 {
				continue
			}
			outside := edge.from
			if tailInTree {
				outside = edge.to
			}
			tree[edgeIndex] = true
			inTree[outside] = true
			treeNodeCount++
			queue = append(queue, outside)
		}
		if len(queue) == 0 {
			return nil, invariant.New("feasible-tree shift did not add a node")
		}
		if err := growTight(); err != nil {
			return nil, err
		}
	}
	return tree, nil
}

// optimize performs the feasible-tree exchanges in figure 2-1. The paper
// requires an anti-cycling rule for degenerate (zero-shift) pivots. We use the
// finite smallest-index rule of Bland (Mathematics of Operations Research
// 2(2), 1977): the first negative-cut tree edge leaves, and minimum-slack
// entering-edge ties use the first edge. rankProblem's stable-ID pivot order
// makes both choices deterministic and independent of input slice order.
func (solver *rankSimplex) optimize(levels []int64, tree []bool, guard *limits.OptimizationWorkGuard) error {
	for {
		if err := solver.computeCutValues(tree, guard); err != nil {
			return err
		}
		cutValue := solver.scratch.cutValue
		leaving := -1
		for _, edgeIndex := range solver.problem.pivotOrder {
			if err := guard.Step(); err != nil {
				return err
			}
			if tree[edgeIndex] && cutValue[edgeIndex] < 0 {
				leaving = edgeIndex
				break
			}
		}
		if leaving < 0 {
			return nil
		}

		if err := solver.computeHeadComponent(tree, leaving, guard); err != nil {
			return err
		}
		head := solver.scratch.head
		entering := -1
		minimumSlack := int64(math.MaxInt64)
		for _, edgeIndex := range solver.problem.pivotOrder {
			if err := guard.Step(); err != nil {
				return err
			}
			edge := solver.problem.edges[edgeIndex]
			if tree[edgeIndex] || !head[edge.from] || head[edge.to] {
				continue
			}
			slack, err := rankSlack(edge, levels)
			if err != nil {
				return err
			}
			if slack < minimumSlack {
				minimumSlack = slack
				entering = edgeIndex
			}
		}
		if entering < 0 {
			return invariant.Errorf("negative cut on edge %d has no entering edge", solver.problem.edges[leaving].id)
		}

		for node, inHead := range head {
			if err := guard.Step(); err != nil {
				return err
			}
			if !inHead {
				continue
			}
			var ok bool
			levels[node], ok = checkedAddInt64(levels[node], minimumSlack)
			if !ok {
				return errors.New("simplex level overflow")
			}
		}
		if slack, err := rankSlack(solver.problem.edges[entering], levels); err != nil {
			return err
		} else if slack != 0 {
			return invariant.Errorf("entering edge %d has slack %d after exchange", solver.problem.edges[entering].id, slack)
		}
		tree[leaving] = false
		tree[entering] = true
	}
}

// computeCutValues roots the undirected feasible tree at node zero. Summing
// the prescribed dual supplies from the leaves inward gives every tree edge's
// cut value in linear time and stores the result in solver.scratch.cutValue.
func (solver *rankSimplex) computeCutValues(tree []bool, guard *limits.OptimizationWorkGuard) error {
	nodeCount := len(solver.problem.nodes)
	treeEdgeCount := 0
	for _, present := range tree {
		if err := guard.Step(); err != nil {
			return err
		}
		if present {
			treeEdgeCount++
		}
	}
	if treeEdgeCount != nodeCount-1 {
		return invariant.Errorf("basis has %d tree edges, want %d", treeEdgeCount, nodeCount-1)
	}

	const unvisited = -2
	parent := solver.scratch.parent
	parentEdge := solver.scratch.parentEdge
	for node := range parent {
		if err := guard.Step(); err != nil {
			return err
		}
		parent[node] = unvisited
		parentEdge[node] = -1
	}
	parent[0] = -1
	order := solver.scratch.order[:0]
	stack := append(solver.scratch.stack[:0], 0)
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		order = append(order, node)
		for i := solver.incidentStart[node]; i < solver.incidentStart[node+1]; i++ {
			if err := guard.Step(); err != nil {
				return err
			}
			edgeIndex := solver.incidentEdges[i]
			if !tree[edgeIndex] || edgeIndex == parentEdge[node] {
				continue
			}
			edge := solver.problem.edges[edgeIndex]
			other := edge.from
			if other == node {
				other = edge.to
			}
			if parent[other] != unvisited {
				return invariant.Errorf("basis contains a cycle through edge %d", edge.id)
			}
			parent[other] = node
			parentEdge[other] = edgeIndex
			stack = append(stack, other)
		}
	}
	if len(order) != nodeCount {
		return invariant.New("basis is disconnected")
	}

	subtreeBalance := solver.scratch.subtreeBalance
	for node, amount := range solver.balance {
		if err := guard.Step(); err != nil {
			return err
		}
		subtreeBalance[node] = amount
	}
	cutValue := solver.scratch.cutValue
	clear(cutValue)
	for i := len(order) - 1; i > 0; i-- {
		if err := guard.Step(); err != nil {
			return err
		}
		node := order[i]
		edgeIndex := parentEdge[node]
		edge := solver.problem.edges[edgeIndex]
		amount := subtreeBalance[node]
		if edge.from == node {
			cutValue[edgeIndex] = amount
		} else if edge.to == node {
			var ok bool
			cutValue[edgeIndex], ok = checkedSubInt64(0, amount)
			if !ok {
				return errors.New("cut-value overflow")
			}
		} else {
			return invariant.Errorf("malformed tree edge %d", edge.id)
		}
		var ok bool
		subtreeBalance[parent[node]], ok = checkedAddInt64(subtreeBalance[parent[node]], amount)
		if !ok {
			return errors.New("subtree-balance overflow")
		}
	}
	if subtreeBalance[0] != 0 {
		return invariant.Errorf("tree balance is %d, want zero", subtreeBalance[0])
	}
	return nil
}

// computeHeadComponent stores the head component formed by removing leaving in
// solver.scratch.head.
func (solver *rankSimplex) computeHeadComponent(tree []bool, leaving int, guard *limits.OptimizationWorkGuard) error {
	head := solver.scratch.head
	clear(head)
	start := solver.problem.edges[leaving].to
	head[start] = true
	queue := append(solver.scratch.queue[:0], start)
	for cursor := 0; cursor < len(queue); cursor++ {
		if err := guard.Step(); err != nil {
			return err
		}
		node := queue[cursor]
		for i := solver.incidentStart[node]; i < solver.incidentStart[node+1]; i++ {
			if err := guard.Step(); err != nil {
				return err
			}
			edgeIndex := solver.incidentEdges[i]
			if edgeIndex == leaving || !tree[edgeIndex] {
				continue
			}
			edge := solver.problem.edges[edgeIndex]
			other := edge.from
			if other == node {
				other = edge.to
			}
			if !head[other] {
				head[other] = true
				queue = append(queue, other)
			}
		}
	}
	if len(queue) == len(solver.problem.nodes) {
		return invariant.Errorf("leaving edge %d does not cut the tree", solver.problem.edges[leaving].id)
	}
	return nil
}

// certify constructs the ranking LP's dual flow using only the final tight
// spanning tree. A tree cut value is exactly the unique flow on that edge that
// satisfies the authored edge-weight balances. Nonnegative cut values provide
// dual feasibility; equality with the independently computed primal objective
// in rankDAG then certifies exact optimality.
func (solver *rankSimplex) certify(levels []int64, tree []bool, guard *limits.OptimizationWorkGuard) (int64, error) {
	if err := solver.computeCutValues(tree, guard); err != nil {
		return 0, err
	}
	cutValue := solver.scratch.cutValue
	computedBalance := solver.scratch.computedBalance
	clear(computedBalance)
	dualValue := int64(0)
	for edgeIndex, edge := range solver.problem.edges {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		flow := int64(0)
		if tree[edgeIndex] {
			flow = cutValue[edgeIndex]
			if flow < 0 {
				return 0, invariant.Errorf("final tree edge %d has negative cut value %d", edge.id, flow)
			}
			span, err := rankSpan(edge, levels)
			if err != nil {
				return 0, err
			}
			if span != rankMinSpan {
				return 0, invariant.Errorf("final tree edge %d is not tight", edge.id)
			}
		}
		var ok bool
		computedBalance[edge.from], ok = checkedAddInt64(computedBalance[edge.from], flow)
		if ok {
			computedBalance[edge.to], ok = checkedSubInt64(computedBalance[edge.to], flow)
		}
		if !ok {
			return 0, errors.New("certificate balance overflow")
		}
		contribution, ok := checkedMulInt64(flow, rankMinSpan)
		if ok {
			dualValue, ok = checkedAddInt64(dualValue, contribution)
		}
		if !ok {
			return 0, errors.New("dual value overflow")
		}
	}
	for node, want := range solver.balance {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if computedBalance[node] != want {
			return 0, invariant.Errorf("certificate balance mismatch on node %s: got %d, want %d", solver.problem.nodes[node].DebugID(), computedBalance[node], want)
		}
	}
	return dualValue, nil
}

func rankSlack(edge rankEdge, levels []int64) (int64, error) {
	span, err := rankSpan(edge, levels)
	if err != nil {
		return 0, err
	}
	slack, ok := checkedSubInt64(span, rankMinSpan)
	if !ok {
		return 0, errors.New("edge slack overflow")
	}
	if slack < 0 {
		return 0, invariant.Errorf("infeasible span %d", span)
	}
	return slack, nil
}

func rankSpan(edge rankEdge, levels []int64) (int64, error) {
	span, ok := checkedSubInt64(levels[edge.to], levels[edge.from])
	if !ok {
		return 0, errors.New("edge span overflow")
	}
	return span, nil
}
