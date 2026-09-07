package trees

import (
	"cmp"
	"context"
	"math"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func extractTreesInContainer(g *layoutgraph.Graph, container *layoutgraph.Node, guard *limits.WorkGuard) (map[*layoutgraph.Node][]*layoutgraph.Tree, error) {
	containerNodes := g.Containers[container]
	nodeToTree := make(map[*layoutgraph.Node]*layoutgraph.Tree)
	for _, node := range containerNodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node == nil {
			return nil, treePreprocessBadState("tree container contains a nil node")
		}
		if node.IsTable() {
			return nil, nil
		}
		nodeToTree[node] = layoutgraph.NewTree(node)
	}
	// TODO: Compare this adjacency matrix with repeated ConnectionTo lookups.
	edgeBetween, err := buildTreeAdjacencyMatrix(g, guard)
	if err != nil {
		return nil, err
	}

	// Terminal node: tree never reaches here, but roots can connect to graph here
	// Sentinel node: the node that a tree root is connected to
	isTerminal := make(map[*layoutgraph.Node]bool)
	sentinelToTreeRoots := make(map[*layoutgraph.Node][]*layoutgraph.Tree)
	sentinelToArrowheads := make(map[*layoutgraph.Node]map[layoutgraph.Arrowhead]struct{})

	// mark all containers as terminal so they aren't in trees
	for container := range g.Containers {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		isTerminal[container] = true
	}
	for vessel := range g.Sequences {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		isTerminal[vessel] = true
	}
	// if node has Nears, don't put it in a tree since placement doesn't work with both Nears and Trees
	// TODO: place trees early enough that they are restored before Near placement.
	for _, n := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if len(n.Nears) > 0 || n.FixedTopLeft != nil {
			isTerminal[n] = true
		}
	}

	for {
		fringeNodes := make([]*layoutgraph.Node, 0)
		candidates, err := treeFringeNodes(containerNodes, guard)
		if err != nil {
			return nil, err
		}
		for _, fn := range candidates {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if _, is := isTerminal[fn]; !is {
				fringeNodes = append(fringeNodes, fn)
			}
		}
		if len(fringeNodes) == 0 {
			break
		}

		for _, fn := range fringeNodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			// Note: fn can be marked terminal if it is the sentinel of another fringe node
			// in this set which has its incoming edge directions not matching the sentinel edge
			if _, is := isTerminal[fn]; is {
				continue
			}

			if len(fn.Edges) != 1 || fn.Edges[0] == nil {
				return nil, treePreprocessBadState("fringe node %d has invalid degree", fn.ID)
			}
			sentinelNode := fn.Adjacent(fn.Edges[0])
			if sentinelNode == nil {
				return nil, treePreprocessBadState("fringe node %d has no sentinel", fn.ID)
			}
			// if there are a pair of fringe nodes remaining, mark the first one as terminal and continue
			// otherwise both will have their tree nodes as children of each other
			if len(sentinelNode.Edges) == 1 && sentinelNode.Adjacent(sentinelNode.Edges[0]) == fn {
				if _, is := isTerminal[sentinelNode]; !is {
					isTerminal[fn] = true
					continue
				}
			}
			tree := nodeToTree[fn]
			if tree == nil {
				return nil, treePreprocessBadState("fringe node %d has no tree record", fn.ID)
			}
			tree.SentinelEdge = edgeBetween(fn, sentinelNode)
			if tree.SentinelEdge == nil {
				return nil, treePreprocessBadState("fringe node %d has no graph sentinel edge", fn.ID)
			}

			if _, exists := sentinelToTreeRoots[sentinelNode]; !exists {
				sentinelToTreeRoots[sentinelNode] = make([]*layoutgraph.Tree, 0)
				sentinelToArrowheads[sentinelNode] = make(map[layoutgraph.Arrowhead]struct{})
			}

			ah := tree.SentinelEdge.ArrowheadTo(sentinelNode)

			sentinelToTreeRoots[sentinelNode] = append(sentinelToTreeRoots[sentinelNode], tree)
			sentinelToArrowheads[sentinelNode][ah] = struct{}{}

			if treeRoots, has := sentinelToTreeRoots[fn]; has {
				// Note: treeRoots may contain a terminal node if a root was marked from another direction leading to it
				treeRoots, err = filterTerminalTrees(treeRoots, isTerminal, guard)
				if err != nil {
					return nil, err
				}
				if len(treeRoots) == 0 {
					continue
				}
				directionCounts, err := countTreeDirections(treeRoots, guard)
				if err != nil {
					return nil, err
				}
				// the fringe node is terminal if it has incoming edges with more than one direction
				// or multiple different arrowheads
				if len(directionCounts) > 1 || len(sentinelToArrowheads[fn]) > 1 {
					isTerminal[fn] = true
				} else {
					// merge the roots with the fringe node tree
					for _, t := range treeRoots {
						if err := addTreeChild(tree, t, guard); err != nil {
							return nil, err
						}
					}

					_, hasUndirected := directionCounts[Undirected]
					sentinelEdgeDirection := treeEdgeDirection(fn, tree.SentinelEdge)
					_, hasSentinelDirection := directionCounts[sentinelEdgeDirection]
					// sentinel node is terminal if the fringe node's incoming edges don't match the edge to the sentinel node
					// a directed edge can merge with Undirected edges and an Undirected edge can merge with matching directed edges
					sentinelIsTerminal := !(hasUndirected || sentinelEdgeDirection == Undirected) && !hasSentinelDirection

					if sentinelIsTerminal {
						isTerminal[sentinelNode] = true
					}
				}
			}
		}

		for _, fn := range fringeNodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if _, is := isTerminal[fn]; !is {
				if err := removeNodeFromGraph(fn, g, guard); err != nil {
					return nil, err
				}
				if err := disconnectTreeEdge(g, nodeToTree[fn].SentinelEdge, guard); err != nil {
					return nil, err
				}
				// remove node from container to children map
				if err := removeNodeFromContainer(g, container, fn, guard); err != nil {
					return nil, err
				}
			}
		}
	}

	terminalNodeToTreeRoots := make(map[*layoutgraph.Node][]*layoutgraph.Tree)
	// Note: sentinelToTreeRoots will only have nodes in this container, but can contain nodes in the tree.
	// by iterating over g.Nodes, we only consider the nodes remaining in the graph, and by checking
	// sentinelToTreeRoots, we get the tree roots connected to the nodes remaining in the graph
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if treeRoots, has := sentinelToTreeRoots[node]; has {
			filteredRoots, err := filterTerminalTrees(treeRoots, isTerminal, guard)
			if err != nil {
				return nil, err
			}
			if len(filteredRoots) == 0 {
				continue
			}
			// if the sentinel is a sequence, we must keep it as is since it can't be inside the tree
			if _, is := g.Sequences[node]; is {
				terminalNodeToTreeRoots[node] = filteredRoots
			} else {
				rootSentinel, roots, err := handleTreeSubgraph(g, node, container, filteredRoots, guard)
				if err != nil {
					return nil, err
				}
				terminalNodeToTreeRoots[rootSentinel] = roots
			}
		}
	}

	if err := guard.Check(); err != nil {
		return nil, err
	}
	return terminalNodeToTreeRoots, nil
}

func extractTrees(g *layoutgraph.Graph, guard *limits.WorkGuard) (map[*layoutgraph.Node][]*layoutgraph.Tree, error) {
	// extract trees in the base graph (nil container) and in all containers
	terminalNodeToTreeRoots := make(map[*layoutgraph.Node][]*layoutgraph.Tree)
	// TODO: investigate why this has to be in RDFSOrder
	containerOrder, err := treeContainerRDFSOrder(g, nil, guard)
	if err != nil {
		return nil, err
	}
	containerOrder = append(containerOrder, nil)
	for _, container := range containerOrder {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		containerTrees, err := extractTreesInContainer(g, container, guard)
		if err != nil {
			return nil, err
		}
		for rootSentinel, roots := range containerTrees {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			terminalNodeToTreeRoots[rootSentinel] = roots
		}
	}

	return terminalNodeToTreeRoots, nil
}

// handleTreeSubgraph merges tree parts that share an internal root sentinel.
// Entire tree subgraphs can produce multiple parts because extraction assumes
// every tree is attached to a non-tree core.
func handleTreeSubgraph(g *layoutgraph.Graph, rootSentinel, container *layoutgraph.Node, roots []*layoutgraph.Tree, guard *limits.WorkGuard) (*layoutgraph.Node, []*layoutgraph.Tree, error) {
	// if rootSentinel has connections after extract trees it isn't a subgraph,
	//  and if there isn't more than 1 tree root there aren't any parts to merge
	if !(len(rootSentinel.Edges) == 0 && len(roots) > 1) {
		return rootSentinel, roots, nil
	}
	if rootSentinel.IsContainer() {
		return rootSentinel, roots, nil
	}

	var newRootSentinel *layoutgraph.Node
	var newTreeRoot *layoutgraph.Tree
	// Of the roots connecting to rootSentinel, each can only possibly lead to the root of a merged tree if
	//   the directions are valid for a tree.
	// e.g. with 1 root via an outward edge and 2 roots via inward edges,
	//   only the outward edge root could possibly lead to the root of a single merged tree.
	// Example     ┌───┐
	//             │ 1 ├...?
	//             └─┬─┘
	//      ...?     │
	//     ┌─┴─┐   ┌─▼─┐   ┌───┐
	//     │ 2 ◄───┤gN ◄───┤ 3 ├...?
	//     └───┘   └───┘   └───┘
	// Here only the root 2 could possibly be the root of the merged tree (due to edge directions)
	candidateRoots, err := candidateTreeRoots(roots, guard)
	if err != nil {
		return nil, nil, err
	}
	for _, candidateRoot := range candidateRoots {
		if err := guard.Step(); err != nil {
			return nil, nil, err
		}
		// if the root tree is a line, then it can be the merged tree root
		newRoot, err := treeEndOfLine(candidateRoot, guard)
		if err != nil {
			return nil, nil, err
		}
		if newRoot != nil {
			// We just need to join the other roots as children of a new Tree for rootSentinel,
			jointNode := layoutgraph.NewTree(rootSentinel)
			for _, otherRoot := range roots {
				if err := guard.Step(); err != nil {
					return nil, nil, err
				}
				if otherRoot != newRoot {
					if err := addTreeChild(jointNode, otherRoot, guard); err != nil {
						return nil, nil, err
					}
				}
			}

			// Note: the root's Sentinel Edge is overwritten by reversing the chain, so we have to record it beforehand
			edgeToRootSentinel := candidateRoot.SentinelEdge
			//   flip the tree nodes along the root,
			if err := reverseTreeChain(candidateRoot, guard); err != nil {
				return nil, nil, err
			}
			//   and connect the node and the flipped chain together.
			if err := addTreeChild(candidateRoot, jointNode, guard); err != nil {
				return nil, nil, err
			}
			jointNode.SentinelEdge = edgeToRootSentinel

			newRootSentinel = newRoot.Node
			newTreeRoot = newRoot.Children[0]
			newTreeRoot.Parent = nil
			break
		}
	}

	if newRootSentinel != nil && newTreeRoot != nil {
		if err := removeNodeFromGraph(rootSentinel, g, guard); err != nil {
			return nil, nil, err
		}
		g.AddNodeUnchecked(newRootSentinel)
		if err := guard.Step(); err != nil {
			return nil, nil, err
		}

		filtered := make([]*layoutgraph.Node, 0)
		for _, n := range g.Containers[container] {
			if err := guard.Step(); err != nil {
				return nil, nil, err
			}
			if n == rootSentinel {
				continue
			}
			filtered = append(filtered, n)
		}
		filtered = append(filtered, newRootSentinel)
		g.Containers[container] = filtered

		return newRootSentinel, []*layoutgraph.Tree{newTreeRoot}, nil
	}
	return rootSentinel, roots, nil
}

// prepare tree for operation defined to Bottom, when we want it to work for the provided orientation
// Note: must also call after the operation to cancel out the effects of this function
func invertOrientationToBottom(t *layoutgraph.Tree, orientation geo.Orientation, guard *limits.WorkGuard) error {
	switch orientation {
	case geo.Top:
		return flip(t, guard)
	case geo.Right:
		return swapDimensions(t, guard)
	case geo.Left:
		if err := swapDimensions(t, guard); err != nil {
			return err
		}
		return flip(t, guard)
	}
	return guard.Check()
}

func constructToOrientation(placementTree *layoutgraph.Tree, orientation geo.Orientation, guard *limits.WorkGuard) error {
	if err := setOrientation(placementTree, orientation, guard); err != nil {
		return err
	}
	if err := invertOrientationToBottom(placementTree, orientation, guard); err != nil {
		return err
	}
	if err := layoutTree(placementTree, guard); err != nil {
		return err
	}
	if err := centerAlignChildren(placementTree, guard); err != nil {
		return err
	}
	return invertOrientationToBottom(placementTree, orientation, guard)
}

func treePlacementFixedOrigin(g *layoutgraph.Graph, container *layoutgraph.Node, guard *limits.WorkGuard) (*geo.Point, error) {
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node == nil {
			return nil, treePreprocessBadState("tree placement encountered a nil graph node")
		}
		if node.EffectiveContainer() != container {
			continue
		}
		if point := node.FixedOrigin(); point != nil {
			return point, nil
		}
	}
	return nil, nil
}

func placeAtOrientationGuarded(ctx context.Context, g *layoutgraph.Graph, placementTree *layoutgraph.Tree, orientation geo.Orientation, guard *limits.WorkGuard) (float64, error) {
	ctx = layoutgraph.ContextWithTransactionWorkGuard(ctx, guard)
	if placementTree == nil || placementTree.Node == nil || placementTree.Node.TopLeft == nil {
		return 0, treePreprocessBadState("tree placement encountered an incomplete placement tree")
	}
	if err := guard.Finish(); err != nil {
		return 0, err
	}
	positionOf := func(n *layoutgraph.Node) float64 {
		switch orientation {
		case geo.Left:
			return n.TopLeft.X
		case geo.Right:
			return n.TopLeft.X + n.Width
		case geo.Top:
			return n.TopLeft.Y
		default:
			return n.TopLeft.Y + n.Height
		}
	}
	moveTree := func(from, to float64) error {
		dx := 0.0
		dy := 0.0
		switch {
		case orientation == geo.Left || orientation == geo.Right:
			dx = math.Round(to - from)
		case orientation == geo.Top || orientation == geo.Bottom:
			dy = math.Round(to - from)
		}
		for _, child := range placementTree.Children {
			if err := guard.Step(); err != nil {
				return err
			}
			if err := offsetSubtree(child, dx, dy, guard); err != nil {
				return err
			}
		}
		return guard.Finish()
	}
	isFurtherOut := func(a, b float64) bool {
		if orientation == geo.Left || orientation == geo.Top {
			return a < b
		} else {
			return a > b
		}
	}

	// 0. Find the root position and border position
	rootPosition := positionOf(placementTree.Node)
	borderPosition := rootPosition
	descendants, err := treeDescendants(placementTree, guard)
	if err != nil {
		return 0, err
	}
	isNodeOfTree := make(map[*layoutgraph.Node]struct{}, len(descendants)+1)
	isNodeOfTree[placementTree.Node] = struct{}{}
	for _, descendant := range descendants {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		isNodeOfTree[descendant.Node] = struct{}{}
	}
	for _, n := range g.Nodes {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if n == nil || n.TopLeft == nil {
			return 0, treePreprocessBadState("tree placement encountered an incomplete graph node")
		}
		if _, is := isNodeOfTree[n]; !is {
			position := positionOf(n)
			if isFurtherOut(position, borderPosition) {
				borderPosition = position
			}
		}
	}

	// 1. Move the tree to the starting border position
	//   we could include the obstacle(s) at the border to find this state, but we also want to
	//   start the transaction attempts from here since there will not be any existing overlaps
	currentBestPosition := borderPosition
	currentBestDistance := math.Abs(currentBestPosition - rootPosition)

	if err := moveTree(0, borderPosition); err != nil {
		return 0, err
	}
	// if there are fixed nodes in the container, there shouldn't be any negative coordinates in the initial placement
	fixedOrigin, err := treePlacementFixedOrigin(g, placementTree.Node.EffectiveContainer(), guard)
	if err != nil {
		return 0, err
	}
	if fixedOrigin != nil {
		hasNegative := false
		placementNodes := append([]*layoutgraph.Tree{placementTree}, descendants...)
		for _, tree := range placementNodes {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if tree.Node.TopLeft.X < fixedOrigin.X || tree.Node.TopLeft.Y < fixedOrigin.Y {
				hasNegative = true
				break
			}
		}
		if hasNegative {
			currentBestDistance = math.Inf(1)
		}
	}

	// 2a. find nodes between the root and the border to find placement options closer to the root node
	//     we will try to place the tree next to each of these to find the best option
	obstaclePositions := make([]float64, 0)
	for _, n := range g.Nodes {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if _, is := isNodeOfTree[n]; !is {
			position := positionOf(n)
			if isFurtherOut(borderPosition, position) && isFurtherOut(position, rootPosition) {
				obstaclePositions = append(obstaclePositions, position)
			}
		}
	}
	// 2b. We add the root position as an option and we start at the border for the edge cases, other obstacles are in-between
	obstaclePositions = append(obstaclePositions, rootPosition)

	txn, err := g.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{})
	if err != nil {
		return 0, err
	}
	for _, obstaclePosition := range obstaclePositions {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		// 2c. Try moving the tree next to each obstacle
		txn.AddOp(func() error {
			return moveTree(currentBestPosition, obstaclePosition)
		})
		// 2d. Undo if overlapping or too close to other nodes
		if err := txn.Commit(ctx); err != nil {
			txn.Clear()
			if layoutgraph.IsCandidateRejection(err) {
				continue
			}
			return 0, err
		}
		if err := guard.Finish(); err != nil {
			txn.Clear()
			return 0, err
		}

		// 2e. Keep this position if it is closer to the root, otherwise undo the movement
		obstacleDistance := math.Abs(obstaclePosition - rootPosition)
		if obstacleDistance < currentBestDistance {
			currentBestPosition = obstaclePosition
			currentBestDistance = obstacleDistance
			if err := txn.UpdateState(); err != nil {
				txn.Clear()
				return 0, err
			}
		} else {
			txn.Rollback()
		}
		txn.Clear()
	}

	if err := guard.Finish(); err != nil {
		return 0, err
	}
	return currentBestDistance, nil
}

// trees look nicer due to their internal relative placement and consistent edges, but they
// have the drawback of fixed placement options on the sides of the rootSentinel its connected to
// when the tree doesn't branch at all, we'd prefer to have node placement since there's little benefit to tree placement
func putBackNonBranchingTrees(g *layoutgraph.Graph, guard *limits.WorkGuard) error {
	containerOrder, err := treeContainerRDFSOrder(g, nil, guard)
	if err != nil {
		return err
	}
	containerOrder = append(containerOrder, nil)
	for _, container := range containerOrder {
		if err := guard.Step(); err != nil {
			return err
		}
		containerNodes := g.Containers[container]
		for _, node := range containerNodes {
			if err := guard.Step(); err != nil {
				return err
			}
			roots, has := g.Trees[node]
			if !has {
				continue
			}
			if len(roots) == 0 {
				return treePreprocessBadState("tree sentinel %d has no roots", node.ID)
			}
			isolated, err := isIsolatedTreeGuarded(g, node, guard)
			if err != nil {
				return err
			}
			firstBranches := false
			if isolated && len(roots) == 1 {
				firstBranches, err = treeBranches(roots[0], guard)
				if err != nil {
					return err
				}
			}
			if isolated && (len(roots) > 1 || firstBranches) {
				// if it is a branching isolated tree, don't put back its roots
				_, isVessel := g.Sequences[node]
				// for a sequence vessel sentinel: put non-branching roots back since tree placement
				// isn't tailored for sequences so it's only worth keeping branching roots
				if !isVessel {
					continue
				}
			}
			branchingRoots := make([]*layoutgraph.Tree, 0, len(roots))
			for _, root := range roots {
				if err := guard.Step(); err != nil {
					return err
				}
				branches, err := treeBranches(root, guard)
				if err != nil {
					return err
				}
				if !branches {
					reconnectionTree := layoutgraph.NewTree(node)
					if err := addTreeChild(reconnectionTree, root, guard); err != nil {
						return err
					}
					if err := reconnectTreeGuarded(g, reconnectionTree, container, guard); err != nil {
						return err
					}
				} else {
					branchingRoots = append(branchingRoots, root)
				}
			}
			if len(branchingRoots) > 0 {
				g.Trees[node] = branchingRoots
			} else {
				delete(g.Trees, node)
			}
		}
	}
	return guard.Check()
}

func preprocessTreesWithWorkLimit(ctx context.Context, g *layoutgraph.Graph, workLimit int64) (err error) {
	if err := layoutgraph.Validate(ctx, treePreprocessLocation, g); err != nil {
		return err
	}
	guard, err := newWorkGuard(ctx, treePreprocessLocation)
	if err != nil {
		return err
	}
	guard.SetLimit(workLimit)

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

	trees, err := extractTrees(g, guard)
	if err != nil {
		return err
	}
	g.Trees = trees
	if err := guard.Step(); err != nil {
		return err
	}
	if err := guard.Check(); err != nil {
		return err
	}
	if err := putBackNonBranchingTrees(g, guard); err != nil {
		return err
	}
	if err := buildNodeToTreeGuarded(g, guard); err != nil {
		return err
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	complete = true
	return nil
}

func Preprocess(ctx context.Context, g *layoutgraph.Graph) error {
	return preprocessTreesWithWorkLimit(ctx, g, limits.MaxEngineWorkUnits)
}

// if width >= height this is the aspect ratio, otherwise it is 1/the aspect ratio
// this way the value is always >= 1, and 1:10 has the same value as 10:1
func normalizedAspectRatio(g *layoutgraph.Graph, guard *limits.WorkGuard) (float64, error) {
	// BoundingBox has no guard parameter and may inspect every peer node while
	// resolving outside labels. Charge its node-pair and route-point traversal
	// here so cancellation and the aggregate work limit cover the calculation.
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if node == nil || node.TopLeft == nil {
			return 0, treePreprocessBadState("tree placement encountered an incomplete graph node")
		}
		for range g.Nodes {
			if err := guard.Step(); err != nil {
				return 0, err
			}
		}
	}
	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if edge == nil {
			return 0, treePreprocessBadState("tree placement encountered a nil graph edge")
		}
		for _, point := range edge.Points {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if point == nil {
				return 0, treePreprocessBadState("tree placement encountered a nil route point")
			}
		}
	}
	tl, br := g.BoundingBox()
	if tl == nil || br == nil {
		return 0, treePreprocessBadState("tree placement could not compute a graph bounding box")
	}
	if err := guard.Finish(); err != nil {
		return 0, err
	}
	width := br.X - tl.X
	height := br.Y - tl.Y
	if width >= height {
		return width / height, nil
	}
	return height / width, nil
}

// reconnect tree to the graph with the best placement we can find
func placeTree(ctx context.Context, g *layoutgraph.Graph, placementTree *layoutgraph.Tree, container *layoutgraph.Node, existingPlacements map[geo.Orientation]bool, guard *limits.WorkGuard) (geo.Orientation, error) {
	// 0. initialize TopLeft to non-nil for placement
	descendants, err := treeDescendants(placementTree, guard)
	if err != nil {
		return 0, err
	}
	for _, t := range descendants {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		t.Node.TopLeft = new(geo.Point)
		if err := guard.Finish(); err != nil {
			return 0, err
		}
	}

	// 1. restore the tree to the graph
	if err := reconnectTreeGuarded(g, placementTree, container, guard); err != nil {
		return 0, err
	}

	// 2. find the best orientation to place the tree close to the root node
	var bestOrientation geo.Orientation
	bestDistance := math.Inf(1)
	bestRatio := math.Inf(1)

	defaultOrder := []geo.Orientation{geo.Bottom, geo.Top, geo.Left, geo.Right}
	order := []geo.Orientation{}

	isIsolatedTree, err := isIsolatedTreeGuarded(g, placementTree.Node, guard)
	if err != nil {
		return bestOrientation, err
	}
	if len(existingPlacements) == 1 {
		for _, o := range defaultOrder {
			if _, in := existingPlacements[o]; in {
				bestOrientation = o.GetOpposite()
				break
			}
		}
	} else {
		// for isolated trees, don't place on the same side as an already placed tree
		if isIsolatedTree {
			for _, o := range defaultOrder {
				if _, in := existingPlacements[o]; in {
					continue
				}
				order = append(order, o)
			}
			if len(order) == 0 {
				// we ran out of unused sides
				order = defaultOrder
			}
		} else {
			order = defaultOrder
		}

		direction := g.Direction(container)
		for _, orientation := range order {
			if err := guard.Step(); err != nil {
				return bestOrientation, err
			}
			if err := constructToOrientation(placementTree, orientation, guard); err != nil {
				return bestOrientation, err
			}
			distance, err := placeAtOrientationGuarded(ctx, g, placementTree, orientation, guard)
			if err != nil {
				return bestOrientation, err
			}
			ratio, err := normalizedAspectRatio(g, guard)
			if err != nil {
				return bestOrientation, err
			}

			// slightly favor the container direction
			if orientation != direction {
				distance += directionPenalty
			}

			if distance < bestDistance {
				bestOrientation = orientation
				bestDistance = distance
				bestRatio = ratio
			} else if distance == bestDistance && ratio < bestRatio {
				// break ties with width & height ratio closest to 1 (most square)
				bestOrientation = orientation
				bestRatio = ratio
			}
		}
	}

	// 3. place using the best orientation
	if err := guard.Step(); err != nil {
		return bestOrientation, err
	}
	if err := constructToOrientation(placementTree, bestOrientation, guard); err != nil {
		return bestOrientation, err
	}
	if _, err := placeAtOrientationGuarded(ctx, g, placementTree, bestOrientation, guard); err != nil {
		return bestOrientation, err
	}

	if err := positionTreeEdgeLabels(placementTree, isIsolatedTree, guard); err != nil {
		return bestOrientation, err
	}
	return bestOrientation, nil
}

func positionTreeEdgeLabels(placementTree *layoutgraph.Tree, isIsolatedTree bool, guard *limits.WorkGuard) (err error) {
	orientation := placementTree.Orientation
	if err := invertOrientationToBottom(placementTree, orientation, guard); err != nil {
		return err
	}
	defer func() {
		restoreErr := invertOrientationToBottom(placementTree, orientation, guard)
		if err == nil {
			err = restoreErr
		}
	}()

	var labelTrees []*layoutgraph.Tree
	for _, root := range placementTree.Children {
		if err := guard.Step(); err != nil {
			return err
		}
		// if it is an isolated tree, we also position the root's sentinel edge label (because we route it in an s-shape)
		if isIsolatedTree {
			labelTrees = append(labelTrees, root)
		} else {
			for _, child := range root.Children {
				if err := guard.Step(); err != nil {
					return err
				}
				labelTrees = append(labelTrees, child)
			}
		}
	}
	for _, tree := range labelTrees {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := validateBottomOrientation(tree, guard); err != nil {
			return err
		}
	}
	for _, tree := range labelTrees {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := positionEdgeLabels(tree, guard); err != nil {
			return err
		}
	}
	return guard.Finish()
}

func placeTrees(ctx context.Context, g *layoutgraph.Graph, container *layoutgraph.Node, guard *limits.WorkGuard) error {
	placementTrees, err := buildPlacementTrees(g, guard)
	if err != nil {
		return err
	}
	treeSizes := make(map[*layoutgraph.Tree]int, len(placementTrees))
	for _, placementTree := range placementTrees {
		size, err := treeSize(placementTree, guard)
		if err != nil {
			return err
		}
		treeSizes[placementTree] = size
	}
	// Sorting cannot return an error from its comparator. Charge a stable
	// n*ceil(log2(n)) upper bound first, one step at a time, so sorting remains
	// covered by the same overflow-safe aggregate budget.
	for width := 1; width < len(placementTrees); {
		for range placementTrees {
			if err := guard.Step(); err != nil {
				return err
			}
		}
		if width > len(placementTrees)/2 {
			break
		}
		width *= 2
	}
	// Note: we sort placementTrees so larger trees are placed first and to have a deterministic order
	slices.SortFunc(placementTrees, func(a, b *layoutgraph.Tree) int {
		if order := cmp.Compare(treeSizes[b], treeSizes[a]); order != 0 {
			return order
		}
		// We compare each placement tree's first child's ID, since placement tree roots can be the same rootSentinel (if a rootSentinel has multiple roots).
		return cmp.Compare(a.Children[0].Node.ID, b.Children[0].Node.ID)
	})
	// record placement sides to prevent repeats in isolated trees
	rootSentinelPlacements := make(map[*layoutgraph.Node]map[geo.Orientation]bool)
	for _, placementTree := range placementTrees {
		if err := guard.Step(); err != nil {
			return err
		}
		if _, in := rootSentinelPlacements[placementTree.Node]; !in {
			rootSentinelPlacements[placementTree.Node] = make(map[geo.Orientation]bool)
		}
		placedOrientation, err := placeTree(ctx, g, placementTree, container, rootSentinelPlacements[placementTree.Node], guard)
		if err != nil {
			return err
		}
		rootSentinelPlacements[placementTree.Node][placedOrientation] = true
		if err := guard.Finish(); err != nil {
			return err
		}
	}
	// disconnect from placement tree
	for _, rootSentinel := range g.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		for _, root := range g.Trees[rootSentinel] {
			if err := guard.Step(); err != nil {
				return err
			}
			root.Parent = nil
			if err := guard.Finish(); err != nil {
				return err
			}
		}
	}
	return guard.Check()
}

func placeTreesWithWorkLimit(ctx context.Context, g *layoutgraph.Graph, container *layoutgraph.Node, workLimit int64) error {
	if err := layoutgraph.Validate(ctx, "PlaceTrees", g); err != nil {
		return err
	}
	guard, err := newWorkGuard(ctx, "PlaceTrees")
	if err != nil {
		return err
	}
	guard.SetLimit(workLimit)

	state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
		CaptureTopology:   true,
		CaptureEdgeRoutes: true,
	})
	if err := state.UpdateWithWorkGuard(g, guard); err != nil {
		return err
	}
	if container != nil {
		if !state.TracksNode(container) {
			return treePreprocessBadState("cannot place trees into an unknown container")
		}
	}
	rollback := &layoutgraph.Transaction{Graph: g, PriorGraphState: state}
	complete := false
	defer func() {
		if !complete {
			rollback.Rollback()
		}
	}()

	if err := placeTrees(ctx, g, container, guard); err != nil {
		return err
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	complete = true
	return nil
}

func Place(ctx context.Context, g *layoutgraph.Graph, container *layoutgraph.Node) error {
	return placeTreesWithWorkLimit(ctx, g, container, limits.MaxPlaceTreesWorkUnits)
}

func buildPlacementTrees(g *layoutgraph.Graph, guard *limits.WorkGuard) ([]*layoutgraph.Tree, error) {
	var placementTrees []*layoutgraph.Tree
	for _, rootSentinel := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		firstPlacementTree := len(placementTrees)
		roots, has := g.Trees[rootSentinel]
		if !has {
			// not actually a rootSentinel
			continue
		}

		isIsolated, err := isIsolatedTreeGuarded(g, rootSentinel, guard)
		if err != nil {
			return nil, err
		}
		if isIsolated {
			// for an isolated tree and we should try to place all the matching roots together
			byDirection, err := rootsByTreeDirection(roots, guard)
			if err != nil {
				return nil, err
			}
			undirected := byDirection[Undirected]
			delete(byDirection, Undirected)
			for _, matchingRoots := range byDirection {
				arrowheadSentinels := make(map[layoutgraph.Arrowhead]*layoutgraph.Tree)

				for _, root := range matchingRoots {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					if root == nil || root.SentinelEdge == nil {
						return nil, treePreprocessBadState("tree placement encountered an incomplete root")
					}
					arrowhead := root.SentinelEdge.ArrowheadTo(rootSentinel)
					if _, has := arrowheadSentinels[arrowhead]; !has {
						arrowheadSentinels[arrowhead] = layoutgraph.NewTree(rootSentinel)
						placementTrees = append(placementTrees, arrowheadSentinels[arrowhead])
					}
					if err := addTreeChild(arrowheadSentinels[arrowhead], root, guard); err != nil {
						return nil, err
					}
				}
			}
			if len(undirected) > 0 {
				// merges undirected trees to the largest one that has no arrowhead in the sentinel edge
				var mergeWith *layoutgraph.Tree
				for _, tree := range placementTrees[firstPlacementTree:] {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					if mergeWith != nil && len(tree.Children) <= len(mergeWith.Children) {
						continue
					}
					// all children should have compatible edges, so we just need to check the first one
					if len(tree.Children) > 0 && !tree.Children[0].SentinelEdge.HasArrowTo(rootSentinel) {
						mergeWith = tree
					}
				}
				if mergeWith == nil {
					// if it could not fit with any of the existing trees, we need to create a new for undirected trees
					mergeWith = layoutgraph.NewTree(rootSentinel)
					placementTrees = append(placementTrees, mergeWith)
				}
				for _, root := range undirected {
					if err := addTreeChild(mergeWith, root, guard); err != nil {
						return nil, err
					}
				}
			}
		} else {
			for _, root := range roots {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				// For placement, create a tree node with the rootSentinel connected to the tree root
				placementTree := layoutgraph.NewTree(rootSentinel)
				if err := addTreeChild(placementTree, root, guard); err != nil {
					return nil, err
				}
				placementTrees = append(placementTrees, placementTree)
			}
		}
	}
	return placementTrees, guard.Check()
}

// dejitter performs micro-movements on nodes to remove the jittery connections that can be straightened out with slight adjustments in one direction
// E.g. jittery line:
// .             +--------->
// . +-----------+
