package hierarchy

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const (
	minHierarchyLevels = 3

	// Limit automatic workflow recognition to modest graphs. Long rank axes
	// and dense edge sets can exhaust geometry or placement budgets.
	maxAutomaticWorkflowNodes = 128
	maxAutomaticWorkflowEdges = 256
)

// Self-loops are routed, but they do not describe a relationship between
// hierarchy levels. Keep hierarchy detection and ordering consistent with the
// DAG ranker, which removes them before assigning levels.
func isHierarchyStructuralEdge(edge *layoutgraph.Edge) bool {
	return !edge.IsLoop()
}

func countHierarchyStructuralEdges(node *layoutgraph.Node) int {
	count := 0
	for _, edge := range node.Edges {
		if isHierarchyStructuralEdge(edge) {
			count++
		}
	}
	return count
}

// AssignNodeHierarchy identifies hierarchical structures within the subgraphs of this graph
func Assign(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, hierarchicalNodes map[*layoutgraph.Node]struct{}) (err error) {
	if err := layoutgraph.Validate(ctx, "AssignNodeHierarchy", g); err != nil {
		return err
	}
	ctx, guard, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "AssignNodeHierarchyTransactions")
	if err != nil {
		return err
	}
	state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
		CaptureTopology:   true,
		CaptureEdgeRoutes: true,
	})
	if err := state.UpdateWithWorkGuard(g, guard); err != nil {
		return err
	}
	rollback := &layoutgraph.Transaction{Graph: g, PriorGraphState: state}
	complete := false
	defer func() {
		if !complete {
			rollback.Rollback()
		}
	}()
	ownedNodes, err := state.OwnedNodes(g, guard)
	if err != nil {
		return err
	}
	seenHierarchies := make(map[*layoutgraph.Hierarchy]struct{})
	for node := range ownedNodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if node.Hierarchy != nil {
			seenHierarchies[node.Hierarchy] = struct{}{}
		}
	}
	for hierarchy := range seenHierarchies {
		for node := range hierarchy.Levels() {
			if err := guard.Step(); err != nil {
				return err
			}
			if _, owned := ownedNodes[node]; owned {
				delete(hierarchy.Levels(), node)
			}
		}
	}
	for node := range ownedNodes {
		if err := guard.Step(); err != nil {
			return err
		}
		node.Hierarchy = nil
	}
	if err := assign(ctx, g, root, hierarchicalNodes); err != nil {
		return err
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	complete = true
	return nil
}

// RemoveIsolatedMemberships removes leaf nodes that inherited a container's
// hierarchy but have no edges of their own. They remain ordinary routing
// obstacles after hierarchy placement.
func RemoveIsolatedMemberships(g *layoutgraph.Graph) {
	for _, node := range g.Nodes {
		if node.Hierarchy != nil && len(node.Edges) == 0 && !node.IsContainer() {
			delete(node.Hierarchy.Levels(), node)
			node.Hierarchy = nil
		}
	}
}

func assign(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, hierarchicalNodes map[*layoutgraph.Node]struct{}) error {

	var force bool
	if root == nil {
		force = g.IsRootHierarchy
	} else {
		force = root.ForceHierarchy
	}

	hierarchyGraph := layoutgraph.NewGraph()
	hierarchyGraph.CopyEntitiesFrom(g)

	for _, node := range g.Containers[root] {
		hierarchyGraph.AddNodeUnchecked(node)
	}
	edgeAbductions := g.AbductEdges(root, hierarchyGraph)
	edgesRestored := false
	var splitOwnership layoutgraph.NodeGraphOwnershipJournal
	defer func() {
		if !edgesRestored {
			g.RestoreEdgeAbductions(edgeAbductions)
		}
		// Edge reachability can include nodes outside hierarchyGraph.Nodes. Restore
		// those exact owners before retaining the assignment stage's existing owner
		// state for nodes in g.
		splitOwnership.Restore()
		layoutgraph.Nodes(g.Nodes).SetGraphReference(g)
	}()

	for _, child := range g.Containers[root] {
		// if this container can't be in a hierarchy, it may have one inside it
		if _, isHierarchical := hierarchicalNodes[child]; !isHierarchical && child.IsContainer() {
			if err := assign(ctx, g, child, hierarchicalNodes); err != nil {
				return err
			}
		}
	}

	// does not consider Nears when splitting the subgraph
	subgraphs, splitOwnership, err := hierarchyGraph.SplitSubgraphsTracked(ctx, layoutgraph.SplitOptions{}, nil)
	if err != nil {
		return err
	}
	for _, subgraph := range subgraphs {
		hierarchy, err := build(ctx, subgraph, force, hierarchicalNodes, edgeAbductions)
		if err != nil {
			return err
		}
		if hierarchy == nil {
			continue
		}
		for _, node := range subgraph.Nodes {
			node.Hierarchy = hierarchy
			for _, descendant := range g.AllDescendantNodes(node, false) {
				hierarchy.Levels()[descendant] = hierarchy.Levels()[node]
				descendant.Hierarchy = hierarchy
			}
		}
	}
	g.RestoreEdgeAbductions(edgeAbductions)
	edgesRestored = true

	// remove Nears of nodes in hierarchies
	if root == nil {
		for _, node := range g.Nodes {
			nears := node.Nears
			node.Nears = map[*layoutgraph.Node]struct{}{}
			if node.Hierarchy == nil {
				for near := range nears {
					if near.Hierarchy == nil {
						node.AddNear(near)
					}
				}
			}
		}
	}

	return nil
}

// buildHierarchy assigns a hierarchy level to each node in this graph given that all nodes can be in a hierarchy.
// | - Find sources (nodes with no incoming edges) and sinks (nodes without outgoing edges)
// | - If there is at least one of each
// | 	- Add sources to a queue
// | 	- Assign level 0 to sources
// | 	- Walk the queue in BFS manner and assign the level of adjacent nodes as the current node level + 1
// | 		- Keep the greater level assignment to avoid having connected nodes on the same row/level
// | 		- Only if the node wasn't already visited.
// | 		- If it was already visited, keep the previous level as it was used to compute other nodes level
//
// SQL-table-only subgraphs use the orthogonal table placement path rather than
// hierarchy placement, even when their edges form a directed hierarchy.
func build(ctx context.Context, g *layoutgraph.Graph, force bool, hierarchicalNodes map[*layoutgraph.Node]struct{}, edgeAbductions []*layoutgraph.EdgeAbduction) (*layoutgraph.Hierarchy, error) {
	if len(g.Nodes) <= 1 {
		return nil, nil
	}
	sourceCount := 0
	sinkCount := 0
	allNodesAreTable := true
	for _, node := range g.Nodes {
		if _, isHierarchical := hierarchicalNodes[node]; !isHierarchical {
			return nil, nil
		}
		allNodesAreTable = allNodesAreTable && node.IsTable()

		// `if/else if` to ensure source and sink are different nodes
		if isSource(node) {
			sourceCount++
		} else if isSink(node) {
			sinkCount++
		}
	}

	if !force && (allNodesAreTable || sourceCount == 0 || sinkCount == 0) {
		return nil, nil
	}

	hierarchy := layoutgraph.NewHierarchy()
	dag, err := makeSimpleDAG(ctx, g)
	if err != nil {
		return nil, err
	}
	ranks, err := rankDAG(ctx, dag)
	if err != nil {
		return nil, err
	}
	hierarchy.ReplaceLevels(mapDAGToGraphLevels(g, ranks.nodeToLevel))
	hierarchy.LevelCount = ranks.levelCount
	if force || isValid(hierarchy, g, edgeAbductions, sourceCount, sinkCount) {
		return hierarchy, nil
	}

	return nil, nil
}

// isValid prefers compact hierarchies with a dominant edge direction. Fully
// directed branched workflows retain their reading order even when they are
// too tall for the usual compactness heuristic.
func isValid(h *layoutgraph.Hierarchy, g *layoutgraph.Graph, edgeAbductions []*layoutgraph.EdgeAbduction, sourceCount, sinkCount int) bool {
	if h.LevelCount < minHierarchyLevels {
		return false
	}
	// a perfect square hierarchy would have nLevels and nLevels nodes on each level (a matrix nLevels x nLevels)
	// so, taking the ratio to nLevels * nLevels tries to approximate this estimate this hierarchy "squaredness"
	unique := make(map[*layoutgraph.Node]struct{})
	for node := range h.Levels() {
		unique[node] = struct{}{}
		for _, descendant := range g.AllDescendantNodes(node, false) {
			unique[descendant] = struct{}{}
		}
	}
	aspectRatio := float64(len(unique)) / float64(h.LevelCount*h.LevelCount)
	tooTall := aspectRatio < 0.5
	tooWide := aspectRatio > 2.0

	// At least 60% of the structural edges must point forward. The workflow
	// exception requires every structural edge to point forward.
	forwardEdges, backOrNeutralEdges := countEdgeDirection(h, g)
	edgesFlowInOneDirection := float64(forwardEdges) >= 1.5*float64(backOrNeutralEdges)
	allEdgesForward := backOrNeutralEdges == 0
	// More nodes than ranks requires a parallel rank: ordinary chains retain
	// their existing placement path. Count only top-level blocks here, since
	// descendants do not add parallel branches to the outer workflow.
	branchedWorkflow := len(g.Nodes) > h.LevelCount && sourceCount == 1 && sinkCount == 1 && allEdgesForward &&
		len(unique) <= maxAutomaticWorkflowNodes && forwardEdges+backOrNeutralEdges <= maxAutomaticWorkflowEdges
	if tooTall && branchedWorkflow && !minimumWorkflowExtentFits(h, g) {
		branchedWorkflow = false
	}

	// we want to avoid nodes with many edges, as they would create a lot of noisy routes
	nEdgesAbductions := make(map[*layoutgraph.Node]int)
	for _, ea := range edgeAbductions {
		if !isHierarchyStructuralEdge(ea.Edge) {
			continue
		}
		if ea.OriginallyFrom != nil {
			nEdgesAbductions[ea.CurrentFrom] += 1
		}
		if ea.OriginallyTo != nil {
			nEdgesAbductions[ea.CurrentTo] += 1
		}
	}
	maxEdges := int(2 * math.Ceil(math.Sqrt(float64(len(unique)))))
	hasDenselyConnectedNode := false
	for node := range unique {
		edgeCount := countHierarchyStructuralEdges(node) - nEdgesAbductions[node]
		if edgeCount > maxEdges {
			hasDenselyConnectedNode = true
			break
		}
	}

	return !tooWide && (!tooTall || branchedWorkflow) && edgesFlowInOneDirection && !hasDenselyConnectedNode && !is1N1(h, edgeAbductions)
}

// minimumWorkflowExtentFits screens current rank dimensions with the minimum
// placement gaps. Container resizing can change these dimensions, and later
// padding can add space, so this does not guarantee that a layout will fit. It
// is called only for the bounded workflow exception, never for authored or
// ordinary hierarchies.
func minimumWorkflowExtentFits(h *layoutgraph.Hierarchy, g *layoutgraph.Graph) bool {
	horizontal := IsHorizontal(g.Nodes)
	levelSize := make([]float64, h.LevelCount)
	for node, level := range h.Levels() {
		size := node.Height
		if horizontal {
			size = node.Width
		}
		levelSize[level] = math.Max(levelSize[level], size)
	}
	extent := float64(h.LevelCount-1) * (crossingSpacing + 2*layoutgraph.MinPortClearance)
	for _, size := range levelSize {
		extent += math.Ceil(size)
	}
	return extent <= limits.MaxGraphSize
}

// countHierarchyEdgesDirections counts the direction of the edges in the given graph considering the node levels.
// Forward edges are the targeted ones coming (source) from top (lower level value) to (target) bottom (higher level value).
// Undirected edges and bidirectional edges are counted as backwardOrNeutral.
// Targeted edges coming from bottom to top are as backwardOrNeutral.
func countEdgeDirection(h *layoutgraph.Hierarchy, g *layoutgraph.Graph) (int, int) {
	forward := 0
	structural := 0
	for _, e := range g.Edges {
		if !isHierarchyStructuralEdge(e) {
			continue
		}
		structural++
		if !e.IsDirected() {
			continue
		}
		from, to, _ := e.DirectedEndpoints()
		if h.Levels()[to] > h.Levels()[from] {
			forward += 1
		}
	}
	return forward, structural - forward
}

// is1N1Hierarchy identifies if it has the format below
// .      ┌───────┐
// .      │       │
// .    ┌─┤       ├──┐
// .    │ │       │  │
// .    │ └───────┘  │
// . ┌──▼────┐   ┌───▼───┐
// . │       │   │       │
// . │       │   │       │
// . │       │   │       │
// . └──┬────┘   └────┬──┘
// .    │  ┌───────┐  │
// .    │  │       │  │
// .    └──►       ◄──┘
// .       │       │
// .       └───────┘
func is1N1(h *layoutgraph.Hierarchy, edgeAbductions []*layoutgraph.EdgeAbduction) bool {
	if h.LevelCount != 3 {
		return false
	}

	nodesByLevel := make(map[int][]*layoutgraph.Node)
	for node, level := range h.Levels() {
		nodesByLevel[level] = append(nodesByLevel[level], node)
	}
	if len(nodesByLevel[0]) != 1 || len(nodesByLevel[2]) != 1 {
		return false
	}

	// This doesn't qualify
	// .┌──────────────────────────────────────────────────┐
	// .│                                                  │
	// .│                                                  │
	// .│                                                  │
	// .│       ┌────┐      ┌──────┐                       │
	// .│       │    │      │      │        ┌──────┐       │
	// .│       │    │      │      │        │      │       │
	// .│       └───┬┘      └─────┬┘        └────┬─┘       │
	// .│           │             │              │         │
	// .│           │             │              │         │
	// .└───────────┼─────────────┼──────────────┼─────────┘
	// .            │             │              │
	// .            │             │              │
	// .            │             │              │
	// .            │             │              │
	// .            │             │              │
	// .            ▼             │              │
	// .        ┌────────┐        │           ┌──▼───┐
	// .        │        │     ┌──▼──┐        │      │
	// .        │        │     │     │        │      │
	// .        └────┬───┘     └───┬─┘        └───┬──┘
	// .             │             │              │
	// .             │             │              │
	// .             │             │              │
	// .             │             │              │
	// .             │             │              │
	// .             │         ┌───▼───┐          │
	// .             │         │       │          │
	// .             └───────► │       │ ◄────────┘
	// .                       │       │
	// .                       └───────┘
	var sourceEdgeAbduction *layoutgraph.EdgeAbduction
	var sinkEdgeAbduction *layoutgraph.EdgeAbduction
	for _, ea := range edgeAbductions {
		if !isHierarchyStructuralEdge(ea.Edge) {
			continue
		}
		if ea.CurrentFrom == nodesByLevel[0][0] {
			if sourceEdgeAbduction != nil {
				if ea.OriginallyFrom != sourceEdgeAbduction.OriginallyFrom {
					return false
				}
			} else {
				sourceEdgeAbduction = ea
			}
		}
		if ea.CurrentTo == nodesByLevel[0][0] {
			if sourceEdgeAbduction != nil {
				if ea.OriginallyTo != sourceEdgeAbduction.OriginallyTo {
					return false
				}
			} else {
				sourceEdgeAbduction = ea
			}
		}

		if ea.CurrentFrom == nodesByLevel[2][0] {
			if sinkEdgeAbduction != nil {
				if ea.OriginallyFrom != sinkEdgeAbduction.OriginallyFrom {
					return false
				}
			} else {
				sinkEdgeAbduction = ea
			}
		}
		if ea.CurrentTo == nodesByLevel[2][0] {
			if sinkEdgeAbduction != nil {
				if ea.OriginallyTo != sinkEdgeAbduction.OriginallyTo {
					return false
				}
			} else {
				sinkEdgeAbduction = ea
			}
		}
	}

	// all nodes on level 2 must be connected at least once above and once below
	for _, node := range nodesByLevel[1] {
		hasEdgeAbove := false
		hasEdgeBelow := false
		for _, e := range node.Edges {
			if !isHierarchyStructuralEdge(e) {
				continue
			}
			adj := node.Adjacent(e)
			if h.Levels()[adj] == 0 {
				hasEdgeAbove = true
			} else if h.Levels()[adj] == 2 {
				hasEdgeBelow = true
			} else {
				return false
			}
			if hasEdgeAbove && hasEdgeBelow {
				break
			}
		}
		if !hasEdgeAbove || !hasEdgeBelow {
			return false
		}
	}

	// the source (level=0) can't be connected to the sink (level=2)
	source := nodesByLevel[0][0]
	for _, e := range source.Edges {
		if !isHierarchyStructuralEdge(e) {
			continue
		}
		adj := source.Adjacent(e)
		if h.Levels()[adj] != 1 {
			return false
		}
	}

	return true
}

// makeSimpleDAG transforms the input into a DAG and removes duplicate edges
// - undirected (a -- b) and bidirectional (a <-> b) edges are split in 2: a -> b and b -> a
// - cycles are broken by reversing some edges
// - duplicate edges are combined, preserving one rank-weight unit per authored edge
// - loops (self-edges) are removed
func makeSimpleDAG(ctx context.Context, g *layoutgraph.Graph) (*layoutgraph.Graph, error) {
	guard, err := limits.NewWorkGuard(ctx, "MakeSimpleDAG", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	dag := layoutgraph.NewGraph()
	connect := func(from, to *layoutgraph.Node, rankWeight int) {
		e := dag.Connect(from, to)
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		e.ID = layoutgraph.EntityID(len(dag.Edges))
		e.SetHierarchyRankWeight(rankWeight)
	}

	idToNode := make(map[layoutgraph.EntityID]*layoutgraph.Node)
	for _, n := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		newN := dag.AddNode(layoutgraph.NewNode(n.ID, n.Width, n.Height))
		newN.D2ID = n.D2ID
		idToNode[n.ID] = newN
	}

	for _, e := range g.Edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if !isHierarchyStructuralEdge(e) {
			continue
		}
		if e.IsDirected() {
			from, to, _ := e.DirectedEndpoints()
			connect(idToNode[from.ID], idToNode[to.ID], 1)
		} else {
			// An undirected or bidirectional edge expands to opposing arcs so the
			// cycle breaker can choose a feasible orientation. The two arcs still
			// represent one authored edge, not two independent rank objectives.
			// They necessarily converge after cycle removal, so carry its unit
			// weight on one half and zero on the other before simplification.
			connect(idToNode[e.From.ID], idToNode[e.To.ID], 1)
			connect(idToNode[e.To.ID], idToNode[e.From.ID], 0)
		}
	}

	cycleEdges, err := findCycleEdges(ctx, dag)
	if err != nil {
		return nil, err
	}
	if err := reverseEdges(ctx, cycleEdges); err != nil {
		return nil, err
	}
	if err := removeDuplicateEdges(ctx, dag); err != nil {
		return nil, err
	}
	// Preserve preparation's work charge and cancellation checkpoints after
	// simplifying the graph.
	for range dag.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
	}

	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return dag, nil
}

// findCycleEdges implements "A fast and effective heuristic for the feedback arc set problem" by P. Eades
// Here is an explanation of the algorithm https://youtu.be/Z0RGCWxvCxA?t=561
// A rough explanation
// 1. sources and sinks can't be in cycles, so remove them and flag their edges as non cycle
// 2. for the remaining nodes, take the node max(outgoing - incoming)
// 3. remove the node and flag the outgoing as non cycle
// 4. repeat from 1 until all nodes were processed
// return set(all edges) - set(non cycle edges)
func findCycleEdges(ctx context.Context, g *layoutgraph.Graph) (map[*layoutgraph.Edge]struct{}, error) {
	guard, err := limits.NewWorkGuard(ctx, "FindCycleEdges", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	incomingEdges := make(map[*layoutgraph.Node]map[*layoutgraph.Edge]struct{})
	outgoingEdges := make(map[*layoutgraph.Node]map[*layoutgraph.Edge]struct{})
	addEdgeToSet := func(e *layoutgraph.Edge, n *layoutgraph.Node, set map[*layoutgraph.Node]map[*layoutgraph.Edge]struct{}) {
		if nodeEdges, exists := set[n]; exists {
			nodeEdges[e] = struct{}{}
		} else {
			set[n] = map[*layoutgraph.Edge]struct{}{e: {}}
		}
	}

	nodes := make(map[*layoutgraph.Node]struct{})
	// set of edges to take the difference with set of non cycle edges
	edges := make(map[*layoutgraph.Edge]struct{})
	for _, e := range g.Edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		edges[e] = struct{}{}
		nodes[e.From] = struct{}{}
		nodes[e.To] = struct{}{}
		if e.IsTargetedTo(e.From) {
			addEdgeToSet(e, e.From, incomingEdges)
			addEdgeToSet(e, e.To, outgoingEdges)
		} else {
			addEdgeToSet(e, e.From, outgoingEdges)
			addEdgeToSet(e, e.To, incomingEdges)
		}
	}

	nonCycleEdges := make(map[*layoutgraph.Edge]struct{})
	pruneNode := func(n *layoutgraph.Node) error {
		for e := range incomingEdges[n] {
			if err := guard.Step(); err != nil {
				return err
			}
			nonCycleEdges[e] = struct{}{}
			// update the adjacent node
			adj := n.Adjacent(e)
			adjEdges := outgoingEdges[adj]
			delete(adjEdges, e)
			// removes the isolated adjacent node
			if len(adjEdges) == 0 && len(incomingEdges[adj]) == 0 {
				delete(nodes, adj)
				delete(outgoingEdges, adj)
				delete(incomingEdges, adj)
			}
		}
		for e := range outgoingEdges[n] {
			if err := guard.Step(); err != nil {
				return err
			}
			nonCycleEdges[e] = struct{}{}
			// update the adjacent node
			adj := n.Adjacent(e)
			adjEdges := incomingEdges[adj]
			delete(adjEdges, e)
			// removes the isolated adjacent node
			if len(adjEdges) == 0 && len(outgoingEdges[adj]) == 0 {
				delete(nodes, adj)
				delete(outgoingEdges, adj)
				delete(incomingEdges, adj)
			}
		}
		delete(nodes, n)
		delete(incomingEdges, n)
		delete(outgoingEdges, n)
		return nil
	}
	for len(nodes) > 0 {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		for {
			// as we delete sinks we might be creating new sinks
			// so this part runs until all sinks were removed
			foundSink := false
			for n := range nodes {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				// only incoming edges = sink
				if len(outgoingEdges[n]) == 0 {
					foundSink = true
					if err := pruneNode(n); err != nil {
						return nil, err
					}
				}
			}
			if !foundSink {
				break
			}
		}

		// isolated nodes are handled in `pruneNode` when we remove the edge of the adjacent node

		// sources
		for {
			foundSource := false
			for n := range nodes {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				// only outgoing edges = source
				if len(incomingEdges[n]) == 0 {
					foundSource = true
					if err := pruneNode(n); err != nil {
						return nil, err
					}
				}
			}
			if !foundSource {
				break
			}
		}

		if len(nodes) > 0 {
			var maxNode *layoutgraph.Node
			maxDiff := math.Inf(-1)
			for node := range nodes {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				diff := float64(len(outgoingEdges[node]) - len(incomingEdges[node]))
				if diff > maxDiff {
					maxDiff = diff
					maxNode = node
				} else if diff == maxDiff {
					// prefer the node with more edges
					nEdges := len(outgoingEdges[node]) + len(incomingEdges[node])
					nMaxEdges := len(outgoingEdges[maxNode]) + len(incomingEdges[maxNode])
					if nEdges > nMaxEdges {
						maxDiff = diff
						maxNode = node
					} else if len(incomingEdges[node]) == len(incomingEdges[maxNode]) && node.ID < maxNode.ID {
						// if edge count is the same, prefer the smaller ID
						maxDiff = diff
						maxNode = node
					}
				}
			}
			for e := range incomingEdges[maxNode] {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				delete(incomingEdges[maxNode], e)
				delete(outgoingEdges[maxNode.Adjacent(e)], e)
			}
			if err := pruneNode(maxNode); err != nil {
				return nil, err
			}
		}
	}

	for e := range nonCycleEdges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		delete(edges, e)
	}

	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return edges, nil
}

// ReverseEdges exchanges from, to = to, from
// arrowheads don't change because we want `a -> b` to become `b -> a` and not `b <- a`
func reverseEdges(ctx context.Context, edges map[*layoutgraph.Edge]struct{}) error {
	guard, err := limits.NewWorkGuard(ctx, "ReverseEdges", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	for range edges {
		if err := guard.Step(); err != nil {
			return err
		}
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	for e := range edges {
		e.From, e.To = e.To, e.From
	}
	return nil
}

// removeDuplicateEdges combines multiple instances of the same edge (from -> to),
// summing their hierarchy rank weights.
// assumes all edges are directed
func removeDuplicateEdges(ctx context.Context, g *layoutgraph.Graph) error {
	guard, err := limits.NewWorkGuard(ctx, "RemoveDuplicateEdges", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	edges := make(map[*layoutgraph.Node]map[*layoutgraph.Node]*layoutgraph.Edge)
	uniqueEdges := make([]*layoutgraph.Edge, 0, len(g.Edges))
	rankWeights := make(map[*layoutgraph.Edge]int)
	for _, e := range g.Edges {
		if err := guard.Step(); err != nil {
			return err
		}
		if fromEdges, exists := edges[e.From]; exists {
			if retained, exists := fromEdges[e.To]; exists {
				rankWeights[retained] += e.HierarchyRankWeight()
			} else {
				fromEdges[e.To] = e
				uniqueEdges = append(uniqueEdges, e)
				rankWeights[e] = e.HierarchyRankWeight()
			}
		} else {
			edges[e.From] = make(map[*layoutgraph.Node]*layoutgraph.Edge)
			edges[e.From][e.To] = e
			uniqueEdges = append(uniqueEdges, e)
			rankWeights[e] = e.HierarchyRankWeight()
		}
	}

	if err := guard.Finish(); err != nil {
		return err
	}
	for _, e := range uniqueEdges {
		e.SetHierarchyRankWeight(rankWeights[e])
	}
	g.ReplaceEdgesUnchecked(uniqueEdges)
	return nil
}

func mapDAGToGraphLevels(g *layoutgraph.Graph, dagNodeToLevel map[*layoutgraph.Node]int) map[*layoutgraph.Node]int {
	idToNode := make(map[layoutgraph.EntityID]*layoutgraph.Node)
	for _, n := range g.Nodes {
		idToNode[n.ID] = n
	}

	nodeToLevel := make(map[*layoutgraph.Node]int)
	for dagN, level := range dagNodeToLevel {
		nodeToLevel[idToNode[dagN.ID]] = level
	}
	return nodeToLevel
}
