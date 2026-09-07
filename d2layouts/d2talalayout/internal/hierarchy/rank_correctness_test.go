package hierarchy

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func TestRankDAGFindsOptimalRanks(t *testing.T) {
	g, nodes := newDirectedGraph(5, [][2]int{
		{0, 1}, {0, 2}, {0, 4}, {1, 3}, {2, 3}, {2, 4},
	})
	result, err := rankDAG(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if cost := rankCost(t, g, nodes, result.nodeToLevel); cost != 7 {
		t.Fatalf("rank cost = %d, want 7", cost)
	}
}

func TestRankDAGImprovesLongestPathThroughSimplexExchange(t *testing.T) {
	g, nodes := newDirectedGraph(4, [][2]int{
		{0, 3}, {1, 2}, {2, 3},
	})
	result, err := rankDAG(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if cost := rankCost(t, g, nodes, result.nodeToLevel); cost != 3 {
		t.Fatalf("rank cost = %d, want 3", cost)
	}
	want := []int{1, 0, 1, 2}
	for i, node := range nodes {
		if got := result.nodeToLevel[node]; got != want[i] {
			t.Fatalf("node %d level = %d, want %d", node.ID, got, want[i])
		}
	}
}

func TestRankDAGUsesDeterministicNormalizedOptimum(t *testing.T) {
	// The four-node spine fixes levels 0 through 3. Node 4 can occupy level 1
	// or 2 without changing the two-edge side path's total span.
	g, nodes := newDirectedGraph(5, [][2]int{
		{0, 1}, {1, 2}, {2, 3}, {0, 4}, {4, 3},
	})
	result, err := rankDAG(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 1, 2, 3, 1}
	for i, node := range nodes {
		if got := result.nodeToLevel[node]; got != want[i] {
			t.Fatalf("node %d level = %d, want %d", node.ID, got, want[i])
		}
	}

	alternate := map[*layoutgraph.Node]int{
		nodes[0]: 0,
		nodes[1]: 1,
		nodes[2]: 2,
		nodes[3]: 3,
		nodes[4]: 2,
	}
	if got, sameCost := rankCost(t, g, nodes, result.nodeToLevel), rankCost(t, g, nodes, alternate); got != sameCost {
		t.Fatalf("selected rank cost = %d, alternate optimum cost = %d", got, sameCost)
	}
}

func TestRankDAGTieUsesSimplexBasisWithoutSecondaryPostpass(t *testing.T) {
	// The longest-path optimum is componentwise earlier, but the paper's
	// deterministic feasible-tree construction selects a different primary
	// optimum. rankDAG intentionally normalizes the simplex result without a
	// secondary least-rank or crowd-balancing pass.
	g, nodes := newDirectedGraph(6, [][2]int{
		{0, 1}, {2, 3}, {4, 0}, {4, 3}, {5, 1}, {5, 2},
	})
	result, err := rankDAG(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{2, 3, 1, 2, 1, 0}
	for nodeIndex, node := range nodes {
		if got := result.nodeToLevel[node]; got != want[nodeIndex] {
			t.Fatalf("node %d level = %d, want %d", node.ID, got, want[nodeIndex])
		}
	}
	earlier := map[*layoutgraph.Node]int{
		nodes[0]: 1,
		nodes[1]: 2,
		nodes[2]: 1,
		nodes[3]: 2,
		nodes[4]: 0,
		nodes[5]: 0,
	}
	if got, alternate := rankCost(t, g, nodes, result.nodeToLevel), rankCost(t, g, nodes, earlier); got != alternate {
		t.Fatalf("selected rank cost = %d, earlier optimum cost = %d", got, alternate)
	}
}

func TestRankSimplexTreeFlowCertificate(t *testing.T) {
	g, _ := newDirectedGraph(4, [][2]int{
		{0, 1}, {0, 2}, {0, 3}, {1, 3}, {2, 3},
	})
	for edgeIndex, weight := range []int{2, 1, 1, 1, 3} {
		g.Edges[edgeIndex].SetHierarchyRankWeight(weight)
	}
	guard, err := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	problem, err := newRankProblem(g, guard)
	if err != nil {
		t.Fatal(err)
	}
	levels, err := problem.longestPathLevels(guard)
	if err != nil {
		t.Fatal(err)
	}
	solver, err := newRankSimplex(problem, guard)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := solver.feasibleTree(levels, guard)
	if err != nil {
		t.Fatal(err)
	}
	if err := solver.optimize(levels, tree, guard); err != nil {
		t.Fatal(err)
	}
	if err := solver.computeCutValues(tree, guard); err != nil {
		t.Fatal(err)
	}
	cutValue := solver.scratch.cutValue
	wantFlow := map[[2]int]int64{
		{0, 1}: 4,
		{1, 3}: 3,
		{2, 3}: 2,
	}
	for edgeIndex, edge := range problem.edges {
		got := int64(0)
		if tree[edgeIndex] {
			got = cutValue[edgeIndex]
		}
		if got != wantFlow[[2]int{edge.from, edge.to}] {
			t.Fatalf("tree flow %d -> %d = %d, want %d", edge.from, edge.to, got, wantFlow[[2]int{edge.from, edge.to}])
		}
	}
	dualValue, err := solver.certify(levels, tree, guard)
	if err != nil {
		t.Fatal(err)
	}
	if dualValue != 9 {
		t.Fatalf("dual value = %d, want 9", dualValue)
	}
}

func TestRankSimplexHandlesTreeEdgeOppositeRootOrder(t *testing.T) {
	g, nodes := newDirectedGraph(2, [][2]int{{1, 0}})
	g.Edges[0].SetHierarchyRankWeight(3)
	result, err := rankDAG(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if got := []int{result.nodeToLevel[nodes[0]], result.nodeToLevel[nodes[1]]}; !slices.Equal(got, []int{1, 0}) {
		t.Fatalf("levels = %v, want [1 0]", got)
	}
}

func TestRankSimplexCertificateRejectsNonoptimalTree(t *testing.T) {
	g, _ := newDirectedGraph(4, [][2]int{
		{0, 1}, {0, 2}, {0, 3}, {1, 3}, {2, 3},
	})
	for edgeIndex, weight := range []int{2, 1, 1, 1, 3} {
		g.Edges[edgeIndex].SetHierarchyRankWeight(weight)
	}
	guard, err := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	problem, err := newRankProblem(g, guard)
	if err != nil {
		t.Fatal(err)
	}
	levels, err := problem.longestPathLevels(guard)
	if err != nil {
		t.Fatal(err)
	}
	solver, err := newRankSimplex(problem, guard)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := solver.feasibleTree(levels, guard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := solver.certify(levels, tree, guard); err == nil {
		t.Fatal("nonoptimal feasible tree unexpectedly passed the dual certificate")
	} else if !errors.Is(err, invariant.ErrViolation) {
		t.Fatalf("certificate error = %v, want invariant.ErrViolation", err)
	}
}

func TestRankSimplexBlandOrderHandlesDegeneratePivot(t *testing.T) {
	// The initial tight tree uses edges 1, 2, 3, and 4. Edge 2 has a
	// negative cut, while edges 5 and 6 are equally eligible zero-slack
	// entering edges. Bland's stable-ID rule must choose edge 5 and finish
	// without relying on an iteration cap.
	g, _ := newDirectedGraph(5, [][2]int{
		{0, 1}, {0, 2}, {1, 3}, {1, 4}, {2, 3}, {2, 4},
	})
	for edgeIndex, weight := range []int{1, 1, 1, 1, 2, 2} {
		g.Edges[edgeIndex].SetHierarchyRankWeight(weight)
	}
	guard, err := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	problem, err := newRankProblem(g, guard)
	if err != nil {
		t.Fatal(err)
	}
	levels, err := problem.longestPathLevels(guard)
	if err != nil {
		t.Fatal(err)
	}
	originalLevels := slices.Clone(levels)
	solver, err := newRankSimplex(problem, guard)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := solver.feasibleTree(levels, guard)
	if err != nil {
		t.Fatal(err)
	}
	if err := solver.computeCutValues(tree, guard); err != nil {
		t.Fatal(err)
	}
	cutValue := solver.scratch.cutValue
	leaving := -1
	for _, edgeIndex := range problem.pivotOrder {
		if tree[edgeIndex] && cutValue[edgeIndex] < 0 {
			leaving = edgeIndex
			break
		}
	}
	if leaving < 0 {
		t.Fatal("initial tree has no negative-cut edge")
	}
	if problem.edges[leaving].id != 2 {
		t.Fatalf("leaving edge ID = %d, want 2", problem.edges[leaving].id)
	}
	if err := solver.computeHeadComponent(tree, leaving, guard); err != nil {
		t.Fatal(err)
	}
	head := solver.scratch.head
	eligible := make([]layoutgraph.EntityID, 0, 2)
	for _, edgeIndex := range problem.pivotOrder {
		edge := problem.edges[edgeIndex]
		if tree[edgeIndex] || !head[edge.from] || head[edge.to] {
			continue
		}
		slack, err := rankSlack(edge, levels)
		if err != nil {
			t.Fatal(err)
		}
		if slack == 0 {
			eligible = append(eligible, edge.id)
		}
	}
	if !slices.Equal(eligible, []layoutgraph.EntityID{5, 6}) {
		t.Fatalf("eligible entering edge IDs = %v, want [5 6]", eligible)
	}

	if err := solver.optimize(levels, tree, guard); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(levels, originalLevels) {
		t.Fatalf("degenerate pivot changed levels from %v to %v", originalLevels, levels)
	}
	for edgeIndex, edge := range problem.edges {
		if edge.id == 5 && !tree[edgeIndex] {
			t.Fatal("smallest-ID eligible edge 5 did not enter the tree")
		}
		if edge.id == 6 && tree[edgeIndex] {
			t.Fatal("larger-ID tied edge 6 entered the tree")
		}
	}
	if _, err := solver.certify(levels, tree, guard); err != nil {
		t.Fatal(err)
	}
}

func TestRankSimplexCompletesManyExchanges(t *testing.T) {
	// This graph exercises a long sequence of tree exchanges. Completion is
	// governed by the shared work budget, not an arbitrary pivot count.
	g := manyExchangeRankGraph(t, 25, 10, 1_000)
	if _, err := rankDAG(context.Background(), g); err != nil {
		t.Fatal(err)
	}
}

func TestRankDAGPreservesParallelEdgeRankWeight(t *testing.T) {
	// A -> B -> C -> D fixes a four-level spine. Ten X -> D edges should
	// outweigh the single A -> X edge and place X immediately above D.
	// Collapsing the parallel edges to a unit-weight edge instead leaves X
	// immediately below A: cost 24 instead of the feasible optimum 15.
	g, nodes := newDirectedGraph(5, [][2]int{
		{0, 1}, {1, 2}, {2, 3}, {0, 4},
		{4, 3}, {4, 3}, {4, 3}, {4, 3}, {4, 3},
		{4, 3}, {4, 3}, {4, 3}, {4, 3}, {4, 3},
	})
	dag := mustMakeSimpleDAG(t, context.Background(), g)

	result, err := rankDAG(context.Background(), dag)
	if err != nil {
		t.Fatal(err)
	}
	levels := mapDAGToGraphLevels(g, result.nodeToLevel)
	if cost := rankCost(t, g, nodes, levels); cost != 15 {
		t.Fatalf("rank cost = %d, want 15; levels=%v", cost, rankLevelsByID(nodes, levels))
	}
}

func TestRankDAGFindsOptimalRanksForSmallDAGs(t *testing.T) {
	for nodeCount := 2; nodeCount <= 5; nodeCount++ {
		pairs := make([][2]int, 0, nodeCount*(nodeCount-1)/2)
		for from := 0; from < nodeCount; from++ {
			for to := from + 1; to < nodeCount; to++ {
				pairs = append(pairs, [2]int{from, to})
			}
		}
		for mask := 1; mask < 1<<len(pairs); mask++ {
			edges := make([][2]int, 0, len(pairs))
			for index, pair := range pairs {
				if mask&(1<<index) != 0 {
					edges = append(edges, pair)
				}
			}
			g, nodes := newDirectedGraph(nodeCount, edges)
			if !isConnectedForRanker(g, nodes) {
				continue
			}
			want := minimumRankCost(g, nodes)
			result, err := rankDAG(context.Background(), g)
			if err != nil {
				t.Fatalf("nodes=%d mask=%b: %v", nodeCount, mask, err)
			}
			got := rankCost(t, g, nodes, result.nodeToLevel)
			if got != want {
				t.Fatalf("nodes=%d mask=%b: rank cost = %d, want %d", nodeCount, mask, got, want)
			}
		}
	}
}

func TestRankDAGFindsOptimalRanksForWeightedSmallDAGs(t *testing.T) {
	for nodeCount := 2; nodeCount <= 4; nodeCount++ {
		pairs := make([][2]int, 0, nodeCount*(nodeCount-1)/2)
		for from := 0; from < nodeCount; from++ {
			for to := from + 1; to < nodeCount; to++ {
				pairs = append(pairs, [2]int{from, to})
			}
		}
		// Each base-three digit means absent, unit weight, or weight three.
		// Exhausting these choices exercises every DAG topology on four nodes
		// and objectives where multiplicity changes the optimum.
		caseCount := 1
		for range pairs {
			caseCount *= 3
		}
		for encoded := 1; encoded < caseCount; encoded++ {
			g := layoutgraph.NewGraph()
			nodes := make([]*layoutgraph.Node, nodeCount)
			for index := range nodes {
				nodes[index] = g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(index+1), 1, 1))
			}
			choices := encoded
			for _, endpoints := range pairs {
				choice := choices % 3
				choices /= 3
				if choice == 0 {
					continue
				}
				edge := g.Connect(nodes[endpoints[0]], nodes[endpoints[1]])
				edge.ID = layoutgraph.EntityID(len(g.Edges))
				edge.TargetArrowhead = layoutgraph.TriangleArrowhead
				if choice == 2 {
					edge.SetHierarchyRankWeight(3)
				}
			}
			if !isConnectedForRanker(g, nodes) {
				continue
			}
			want := minimumRankCost(g, nodes)
			result, err := rankDAG(context.Background(), g)
			if err != nil {
				t.Fatalf("nodes=%d encoded=%d: %v", nodeCount, encoded, err)
			}
			got := rankCost(t, g, nodes, result.nodeToLevel)
			if got != want {
				t.Fatalf("nodes=%d encoded=%d: rank cost = %d, want %d", nodeCount, encoded, got, want)
			}
		}
	}
}

func TestRankDAGRejectsInvalidExplicitRankWeight(t *testing.T) {
	for _, weight := range []int{-1, 0, limits.MaxEngineEdges + 1} {
		g, _ := newDirectedGraph(2, [][2]int{{0, 1}})
		g.Edges[0].SetHierarchyRankWeight(weight)
		if _, err := rankDAG(context.Background(), g); err == nil {
			t.Fatalf("rank weight %d was accepted", weight)
		}
	}
}

func TestRankDAGFindsOptimalRanksForRandomWeightedDAGs(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	for testCase := 0; testCase < 200; testCase++ {
		const nodeCount = 5
		g, nodes := newDirectedGraph(nodeCount, nil)
		connected := make([][]bool, nodeCount)
		for node := range connected {
			connected[node] = make([]bool, nodeCount)
		}
		connect := func(from, to int) {
			edge := g.Connect(nodes[from], nodes[to])
			edge.ID = layoutgraph.EntityID(len(g.Edges))
			edge.TargetArrowhead = layoutgraph.TriangleArrowhead
			edge.SetHierarchyRankWeight(1 + random.Intn(50))
			connected[from][to] = true
		}
		// Start with a random-parent tree rather than a fixed chain so the
		// random corpus exercises both the tight certificate and tree exchanges.
		for to := 1; to < nodeCount; to++ {
			connect(random.Intn(to), to)
		}
		for from := 0; from < nodeCount; from++ {
			for to := from + 1; to < nodeCount; to++ {
				if connected[from][to] || random.Intn(3) == 0 {
					continue
				}
				connect(from, to)
			}
		}

		result, err := rankDAG(context.Background(), g)
		if err != nil {
			t.Fatalf("case %d: %v", testCase, err)
		}
		got := rankCost(t, g, nodes, result.nodeToLevel)
		if want := minimumRankCost(g, nodes); got != want {
			t.Fatalf("case %d: rank cost = %d, want %d", testCase, got, want)
		}
	}
}

func TestRankDAGIsIndependentOfInputSliceOrder(t *testing.T) {
	g, nodes := newDirectedGraph(6, [][2]int{
		{0, 1}, {0, 2}, {0, 4}, {1, 3}, {2, 3}, {2, 4}, {3, 5}, {4, 5},
	})
	for i, edge := range g.Edges {
		edge.SetHierarchyRankWeight(i%4 + 1)
	}
	first, err := rankDAG(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}

	slices.Reverse(g.Nodes)
	slices.Reverse(g.Edges)
	for _, node := range g.Nodes {
		slices.Reverse(node.Edges)
	}
	second, err := rankDAG(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range nodes {
		if first.nodeToLevel[node] != second.nodeToLevel[node] {
			t.Fatalf("node %d level changed from %d to %d", node.ID, first.nodeToLevel[node], second.nodeToLevel[node])
		}
	}
}

func TestRankDAGRejectsCycles(t *testing.T) {
	g, _ := newDirectedGraph(3, [][2]int{{0, 1}, {1, 2}, {2, 0}})
	if _, err := rankDAG(context.Background(), g); err == nil {
		t.Fatal("cyclic rank input unexpectedly succeeded")
	}
}

func newDirectedGraph(nodeCount int, edges [][2]int) (*layoutgraph.Graph, []*layoutgraph.Node) {
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, nodeCount)
	for index := range nodes {
		nodes[index] = g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(index+1), 1, 1))
	}
	for _, endpoints := range edges {
		edge := g.Connect(nodes[endpoints[0]], nodes[endpoints[1]])
		edge.ID = layoutgraph.EntityID(len(g.Edges))
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	}
	return g, nodes
}

func isConnectedForRanker(g *layoutgraph.Graph, nodes []*layoutgraph.Node) bool {
	seen := map[*layoutgraph.Node]struct{}{nodes[0]: {}}
	queue := []*layoutgraph.Node{nodes[0]}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, edge := range node.Edges {
			adjacent := node.Adjacent(edge)
			if _, exists := seen[adjacent]; !exists {
				seen[adjacent] = struct{}{}
				queue = append(queue, adjacent)
			}
		}
	}
	return len(seen) == len(nodes) && len(g.Edges) > 0
}

func rankCost(t *testing.T, g *layoutgraph.Graph, nodes []*layoutgraph.Node, levels map[*layoutgraph.Node]int) int {
	t.Helper()
	cost := 0
	for _, edge := range g.Edges {
		length := levels[edge.To] - levels[edge.From]
		if length < 1 {
			t.Fatalf(
				"infeasible rank: edge %d -> %d has length %d; levels=%v",
				edge.From.ID,
				edge.To.ID,
				length,
				rankLevelsByID(nodes, levels),
			)
		}
		cost += edge.HierarchyRankWeight() * length
	}
	return cost
}

func rankLevelsByID(nodes []*layoutgraph.Node, levels map[*layoutgraph.Node]int) map[layoutgraph.EntityID]int {
	result := make(map[layoutgraph.EntityID]int, len(nodes))
	for _, node := range nodes {
		result[node.ID] = levels[node]
	}
	return result
}

func minimumRankCost(g *layoutgraph.Graph, nodes []*layoutgraph.Node) int {
	best := math.MaxInt
	levels := make([]int, len(nodes))
	var assign func(int)
	assign = func(index int) {
		if index < len(nodes) {
			for levels[index] = 0; levels[index] < len(nodes); levels[index]++ {
				assign(index + 1)
			}
			return
		}
		cost := 0
		for _, edge := range g.Edges {
			length := levels[int(edge.To.ID)-1] - levels[int(edge.From.ID)-1]
			if length < 1 {
				return
			}
			cost += edge.HierarchyRankWeight() * length
		}
		best = min(best, cost)
	}
	assign(0)
	return best
}
