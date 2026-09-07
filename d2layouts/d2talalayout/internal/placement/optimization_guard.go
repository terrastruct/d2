package placement

import (
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const (
	// A work limit bounds CPU, but retaining every unique placement until that
	// limit is reached could still consume too much memory. This independent cap
	// keeps the reusable placement buffer and its deduplication map predictable.
	maxOptimizerPlacementCandidates = 1_000_000
	maxRetainedPlacementCandidates  = 16_384
)

func optimizerIsDescendantOf(descendant, ancestor *layoutgraph.Node, guard *limits.OptimizationWorkGuard) (bool, error) {
	seen := make(map[*layoutgraph.Node]struct{})
	for current := descendant; ; {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if current == ancestor {
			return true, nil
		}
		if current == nil {
			return ancestor == nil, nil
		}
		if _, exists := seen[current]; exists {
			return false, fmt.Errorf("TALA %s found a cycle in node ancestry at %s", guard.Location(), current.DebugID())
		}
		if len(seen) >= limits.MaxEngineNodes {
			return false, fmt.Errorf("TALA %s ancestry exceeds node limit %d", guard.Location(), limits.MaxEngineNodes)
		}
		seen[current] = struct{}{}
		switch {
		case current.Container != nil:
			current = current.Container
		case current.Cluster != nil:
			current = current.Cluster.Vessel
		case current.Sequence != nil:
			current = current.Sequence.Vessel
		default:
			current = nil
		}
	}
}

func optimizerTreeRoot(tree *layoutgraph.Tree, guard *limits.OptimizationWorkGuard) (*layoutgraph.Tree, error) {
	seen := make(map[*layoutgraph.Tree]struct{})
	for tree != nil && tree.Parent != nil {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if _, exists := seen[tree]; exists {
			return nil, fmt.Errorf("TALA %s found a cycle in tree ancestry", guard.Location())
		}
		if len(seen) >= limits.MaxEngineNodes {
			return nil, fmt.Errorf("TALA %s tree ancestry exceeds node limit %d", guard.Location(), limits.MaxEngineNodes)
		}
		seen[tree] = struct{}{}
		tree = tree.Parent
	}
	return tree, nil
}

func optimizerFixedOrigin(g *layoutgraph.Graph, container *layoutgraph.Node, guard *limits.OptimizationWorkGuard) (*geo.Point, error) {
	if g == nil {
		return nil, fmt.Errorf("TALA %s fixed-origin lookup requires a graph", guard.Location())
	}
	if len(g.Nodes) > limits.MaxEngineNodes {
		return nil, fmt.Errorf("TALA %s fixed-origin node count exceeds limit %d", guard.Location(), limits.MaxEngineNodes)
	}
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node == nil || node.OwningContainer() != container {
			continue
		}
		if origin := node.FixedOrigin(); origin != nil {
			return origin, nil
		}
	}
	return nil, nil
}

func optimizerOrderedNears(node *layoutgraph.Node, guard *limits.OptimizationWorkGuard) ([]*layoutgraph.Node, error) {
	if node == nil {
		return nil, fmt.Errorf("TALA %s cannot order nears for a nil node", guard.Location())
	}
	if len(node.Nears) > limits.MaxEngineNodes {
		return nil, fmt.Errorf("TALA %s near count exceeds limit %d", guard.Location(), limits.MaxEngineNodes)
	}
	nears := make([]*layoutgraph.Node, 0, len(node.Nears))
	for near := range node.Nears {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if near == nil {
			return nil, fmt.Errorf("TALA %s found a nil near on %s", guard.Location(), node.DebugID())
		}
		nears = append(nears, near)
	}
	if err := guard.AddSort(len(nears)); err != nil {
		return nil, err
	}
	layoutgraph.SortNodesByID(nears)
	return nears, nil
}

func appendBoundedOptimizerNodes(dst, src []*layoutgraph.Node, guard *limits.OptimizationWorkGuard) ([]*layoutgraph.Node, error) {
	if int64(len(dst)) > layoutgraph.MaxTopologyReferences-int64(len(src)) {
		return nil, fmt.Errorf("TALA %s optimizer node references exceed limit %d", guard.Location(), layoutgraph.MaxTopologyReferences)
	}
	return append(dst, src...), nil
}

func appendOptimizerTreeNears(nears []*layoutgraph.Node, tree *layoutgraph.Tree, guard *limits.OptimizationWorkGuard) ([]*layoutgraph.Node, error) {
	if tree == nil {
		return nears, nil
	}
	stack := []*layoutgraph.Tree{tree}
	seen := map[*layoutgraph.Tree]struct{}{tree: {}}
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if current == nil {
			return nil, fmt.Errorf("TALA %s found a nil tree node", guard.Location())
		}
		if current.Node == nil {
			return nil, fmt.Errorf("TALA %s found a tree entry without a node", guard.Location())
		}
		ordered, err := optimizerOrderedNears(current.Node, guard)
		if err != nil {
			return nil, err
		}
		nears, err = appendBoundedOptimizerNodes(nears, ordered, guard)
		if err != nil {
			return nil, err
		}
		if len(current.Children) > limits.MaxEngineNodes {
			return nil, fmt.Errorf("TALA %s tree child references exceed limit %d", guard.Location(), limits.MaxEngineNodes)
		}
		for _, child := range slices.Backward(current.Children) {
			if err := guard.Step(); err != nil {
				return nil, err
			}

			if child == nil {
				return nil, fmt.Errorf("TALA %s found a nil tree child", guard.Location())
			}
			if _, exists := seen[child]; exists {
				return nil, fmt.Errorf("TALA %s found a cycle or shared child in a tree", guard.Location())
			}
			if len(seen) >= limits.MaxEngineNodes {
				return nil, fmt.Errorf("TALA %s tree exceeds node limit %d", guard.Location(), limits.MaxEngineNodes)
			}
			seen[child] = struct{}{}
			stack = append(stack, child)
		}
	}
	return nears, nil
}

func optimizerAdjacents(node *layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction, guard *limits.OptimizationWorkGuard) ([]*layoutgraph.Node, error) {
	if node == nil || node.Graph == nil {
		return nil, fmt.Errorf("TALA %s adjacency lookup requires a node with a graph", guard.Location())
	}
	if len(node.Edges) > limits.MaxEngineEdges || len(edgeAbductions) > limits.MaxEngineEdges {
		return nil, fmt.Errorf("TALA %s adjacency inputs exceed edge limit %d", guard.Location(), limits.MaxEngineEdges)
	}
	adjacents := make([]*layoutgraph.Node, 0, len(node.Edges))
	usedEdgeAbductionsScratch := borrowEdgeAbductionBools(len(edgeAbductions))
	defer returnEdgeAbductionBools(usedEdgeAbductionsScratch)
	usedEdgeAbductions := usedEdgeAbductionsScratch.values
	for _, edge := range node.Edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if edge == nil || (edge.From != node && edge.To != node) {
			return nil, fmt.Errorf("TALA %s found a malformed incident edge on %s", guard.Location(), node.DebugID())
		}
		adjacentNode := node.Adjacent(edge)
		if adjacentNode == nil || adjacentNode.TopLeft == nil {
			continue
		}
		addNode := adjacentNode
		for i, edgeAbduction := range edgeAbductions {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if edgeAbduction == nil {
				return nil, fmt.Errorf("TALA %s found a nil edge abduction", guard.Location())
			}
			if usedEdgeAbductions[i] {
				continue
			}
			if edgeAbduction.CurrentFrom == node && edgeAbduction.CurrentTo == adjacentNode && edgeAbduction.OriginallyTo != nil {
				usedEdgeAbductions[i] = true
				addNode = edgeAbduction.OriginallyTo
				break
			}
			if edgeAbduction.CurrentFrom == adjacentNode && edgeAbduction.CurrentTo == node && edgeAbduction.OriginallyFrom != nil {
				usedEdgeAbductions[i] = true
				addNode = edgeAbduction.OriginallyFrom
				break
			}
		}
		adjacents = append(adjacents, addNode)
	}
	if len(adjacents) != 0 {
		return adjacents, nil
	}

	orderedNears, err := optimizerOrderedNears(node, guard)
	if err != nil {
		return nil, err
	}
	for _, near := range orderedNears {
		if near.Cluster.IsActive() {
			near = near.Cluster.Vessel
		} else if near.Sequence.IsActive() {
			near = near.Sequence.Vessel
		}
		if tree, exists := node.Graph.NodeToTree[near]; exists {
			root, err := optimizerTreeRoot(tree, guard)
			if err != nil {
				return nil, err
			}
			if root == nil || root.SentinelNode() == nil {
				return nil, fmt.Errorf("TALA %s found a tree without a sentinel", guard.Location())
			}
			near = root.SentinelNode()
		}
		if near == nil || near.TopLeft == nil {
			continue
		}
		isDescendant, err := optimizerIsDescendantOf(near, node.Container, guard)
		if err != nil {
			return nil, err
		}
		if isDescendant {
			adjacents = append(adjacents, near)
		}
	}
	if len(adjacents) != 0 {
		return adjacents, nil
	}

	var nears []*layoutgraph.Node
	if node.IsClusterVessel() {
		cluster := node.Graph.Clusters[node]
		if cluster == nil {
			return nil, fmt.Errorf("TALA %s found a cluster vessel without a cluster", guard.Location())
		}
		for _, clusterNode := range cluster.Nodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			ordered, err := optimizerOrderedNears(clusterNode, guard)
			if err != nil {
				return nil, err
			}
			nears, err = appendBoundedOptimizerNodes(nears, ordered, guard)
			if err != nil {
				return nil, err
			}
		}
	}
	if sequence := node.Graph.Sequences[node]; sequence != nil {
		for _, sequenceNode := range sequence.Nodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			ordered, err := optimizerOrderedNears(sequenceNode, guard)
			if err != nil {
				return nil, err
			}
			nears, err = appendBoundedOptimizerNodes(nears, ordered, guard)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(node.Graph.Trees[node]) > limits.MaxEngineNodes {
		return nil, fmt.Errorf("TALA %s tree roots exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
	}
	for _, tree := range node.Graph.Trees[node] {
		nears, err = appendOptimizerTreeNears(nears, tree, guard)
		if err != nil {
			return nil, err
		}
	}
	for _, near := range nears {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		addNode := near
		if near.Cluster.IsActive() {
			addNode = near.Cluster.Vessel
		} else if near.Sequence.IsActive() {
			addNode = near.Sequence.Vessel
		}
		if tree, exists := node.Graph.NodeToTree[addNode]; exists {
			root, err := optimizerTreeRoot(tree, guard)
			if err != nil {
				return nil, err
			}
			if root == nil || root.SentinelNode() == nil {
				return nil, fmt.Errorf("TALA %s found a tree without a sentinel", guard.Location())
			}
			addNode = root.SentinelNode()
		}
		if addNode != nil && addNode.TopLeft != nil {
			adjacents = append(adjacents, addNode)
		}
	}
	return adjacents, nil
}

func optimizerMedian(nodes layoutgraph.Nodes, includeSizes bool, guard *limits.OptimizationWorkGuard) (float64, float64, error) {
	if len(nodes) == 0 {
		return 0, 0, fmt.Errorf("TALA %s cannot compute a median without positioned neighbors", guard.Location())
	}
	if int64(len(nodes)) > layoutgraph.MaxTopologyReferences {
		return 0, 0, fmt.Errorf("TALA %s median inputs exceed limit %d", guard.Location(), layoutgraph.MaxTopologyReferences)
	}
	if len(nodes) == 1 {
		node := nodes[0]
		if err := guard.Step(); err != nil {
			return 0, 0, err
		}
		if node == nil || node.TopLeft == nil {
			return 0, 0, fmt.Errorf("TALA %s cannot compute a median with an unpositioned neighbor", guard.Location())
		}
		if err := guard.AddSort(1); err != nil {
			return 0, 0, err
		}
		if err := guard.AddSort(1); err != nil {
			return 0, 0, err
		}
		if includeSizes {
			cellSize := node.Graph.CellSize
			if cellSize <= 0 || math.IsNaN(cellSize) || math.IsInf(cellSize, 0) {
				return 0, 0, fmt.Errorf("TALA %s requires a finite positive cell size", guard.Location())
			}
			return (node.TopLeft.X + node.Width/2) / cellSize, (node.TopLeft.Y + node.Height/2) / cellSize, nil
		}
		return node.TopLeft.X + 0.5, node.TopLeft.Y + 0.5, nil
	}
	orderedByX := make([]*layoutgraph.Node, len(nodes))
	orderedByY := make([]*layoutgraph.Node, len(nodes))
	for i, node := range nodes {
		if err := guard.Step(); err != nil {
			return 0, 0, err
		}
		if node == nil || node.TopLeft == nil {
			return 0, 0, fmt.Errorf("TALA %s cannot compute a median with an unpositioned neighbor", guard.Location())
		}
		orderedByX[i] = node
		orderedByY[i] = node
	}
	if err := guard.AddSort(len(nodes)); err != nil {
		return 0, 0, err
	}
	sort.Slice(orderedByX, func(i, j int) bool {
		iNode, jNode := orderedByX[i], orderedByX[j]
		iX, jX := iNode.TopLeft.X, jNode.TopLeft.X
		if includeSizes {
			iX += iNode.Width / 2
			jX += jNode.Width / 2
		}
		if iX == jX {
			return iNode.ID < jNode.ID
		}
		return iX < jX
	})
	if err := guard.AddSort(len(nodes)); err != nil {
		return 0, 0, err
	}
	sort.Slice(orderedByY, func(i, j int) bool {
		iNode, jNode := orderedByY[i], orderedByY[j]
		iY, jY := iNode.TopLeft.Y, jNode.TopLeft.Y
		if includeSizes {
			iY += iNode.Height / 2
			jY += jNode.Height / 2
		}
		if iY == jY {
			return iNode.ID < jNode.ID
		}
		return iY < jY
	})

	middle := len(nodes) / 2
	var medianX, medianY float64
	if includeSizes {
		medianX = orderedByX[middle].TopLeft.X + orderedByX[middle].Width/2
		medianY = orderedByY[middle].TopLeft.Y + orderedByY[middle].Height/2
		if len(nodes)%2 == 0 {
			medianX = (medianX + orderedByX[middle-1].TopLeft.X + orderedByX[middle-1].Width/2) / 2
			medianY = (medianY + orderedByY[middle-1].TopLeft.Y + orderedByY[middle-1].Height/2) / 2
		}
		cellSize := nodes[0].Graph.CellSize
		if cellSize <= 0 || math.IsNaN(cellSize) || math.IsInf(cellSize, 0) {
			return 0, 0, fmt.Errorf("TALA %s requires a finite positive cell size", guard.Location())
		}
		medianX /= cellSize
		medianY /= cellSize
	} else {
		medianX = orderedByX[middle].TopLeft.X + 0.5
		medianY = orderedByY[middle].TopLeft.Y + 0.5
		if len(nodes)%2 == 0 {
			medianX = (medianX + orderedByX[middle-1].TopLeft.X) / 2
			medianY = (medianY + orderedByY[middle-1].TopLeft.Y) / 2
		}
	}
	return medianX, medianY, nil
}

func optimizerMedianToNeighbors(node *layoutgraph.Node, includeSizes bool, edgeAbductions []*layoutgraph.EdgeAbduction, guard *limits.OptimizationWorkGuard) (float64, float64, error) {
	adjacents, err := optimizerAdjacents(node, edgeAbductions, guard)
	if err != nil {
		return 0, 0, err
	}
	return optimizerMedian(layoutgraph.Nodes(adjacents), includeSizes, guard)
}

func optimizerDoesOverlap(node *layoutgraph.Node, point *geo.Point, exceptions []*layoutgraph.Node, guard *limits.OptimizationWorkGuard) (bool, error) {
	if node == nil || node.Graph == nil || point == nil {
		return false, fmt.Errorf("TALA %s overlap check requires a node, graph, and point", guard.Location())
	}
	if len(node.Graph.Nodes) > limits.MaxEngineNodes || len(exceptions) > limits.MaxEngineNodes {
		return false, fmt.Errorf("TALA %s overlap inputs exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
	}
	right := point.X + node.Width
	bottom := point.Y + node.Height
	for _, otherNode := range node.Graph.Nodes {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if otherNode == nil {
			return false, fmt.Errorf("TALA %s found a nil graph node", guard.Location())
		}
		if otherNode == node {
			continue
		}
		excluded := false
		for _, exception := range exceptions {
			if err := guard.Step(); err != nil {
				return false, err
			}
			if exception == otherNode {
				excluded = true
				break
			}
		}
		if excluded || otherNode.TopLeft == nil {
			continue
		}
		const maxSafeDelta = 500.0
		if point.X > otherNode.TopLeft.X+otherNode.Width+maxSafeDelta ||
			point.X+node.Width+maxSafeDelta < otherNode.TopLeft.X ||
			point.Y > otherNode.TopLeft.Y+otherNode.Height+maxSafeDelta ||
			point.Y+node.Height+maxSafeDelta < otherNode.TopLeft.Y {
			continue
		}
		if err := guard.Add(uint64(len(node.Edges))); err != nil {
			return false, err
		}
		delta := float64(node.DeltaTo(otherNode, point))
		if point.X < otherNode.TopLeft.X+otherNode.Width+delta && right+delta > otherNode.TopLeft.X &&
			point.Y < otherNode.TopLeft.Y+otherNode.Height+delta && bottom+delta > otherNode.TopLeft.Y {
			return true, nil
		}
	}
	return false, nil
}

func optimizerIsOccupied(g *layoutgraph.Graph, point *geo.Point, guard *limits.OptimizationWorkGuard) (*layoutgraph.Node, bool, error) {
	if g == nil || point == nil {
		return nil, false, fmt.Errorf("TALA %s occupancy check requires a graph and point", guard.Location())
	}
	if len(g.Nodes) > limits.MaxEngineNodes {
		return nil, false, fmt.Errorf("TALA %s occupancy node count exceeds limit %d", guard.Location(), limits.MaxEngineNodes)
	}
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, false, err
		}
		if node == nil {
			return nil, false, fmt.Errorf("TALA %s found a nil graph node", guard.Location())
		}
		if node.TopLeft != nil && point != nil && nonNilEquals(node.TopLeft, point) {
			return node, true, nil
		}
	}
	return nil, false, nil
}

func optimizerCanMove(node *layoutgraph.Node, point *geo.Point, includeSizes bool, guard *limits.OptimizationWorkGuard) (bool, error) {
	if node == nil || node.Graph == nil || point == nil {
		return false, fmt.Errorf("TALA %s movement check requires a node, graph, and point", guard.Location())
	}
	if nonNilEquals(node.TopLeft, point) {
		return true, nil
	}
	_, occupied, err := optimizerIsOccupied(node.Graph, point, guard)
	if err != nil || occupied || !includeSizes {
		return !occupied && err == nil, err
	}
	overlaps, err := optimizerDoesOverlap(node, point, nil, guard)
	return !overlaps && err == nil, err
}

// chargeOptimizerScoring accounts for the nested edge, abduction, blocker, and
// symmetry scans performed by the legacy scoring helpers. Those helpers poll
// context internally; this conservative charge additionally makes repeated
// candidate scoring obey the shared operation budget.
func chargeOptimizerScoring(node *layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction, includeSymmetry bool, guard *limits.OptimizationWorkGuard) error {
	if node == nil || node.Graph == nil {
		return fmt.Errorf("TALA %s scoring requires a node with a graph", guard.Location())
	}
	edges := uint64(len(node.Edges) + 1)
	searchSpace := uint64(len(node.Graph.Nodes) + len(edgeAbductions) + 1)
	if includeSymmetry {
		if math.MaxUint64-searchSpace < edges {
			return fmt.Errorf("%w: %s scoring work arithmetic overflow", limits.ErrOptimizationResourceLimit, guard.Location())
		}
		searchSpace += edges
	}
	return guard.AddProduct(edges, searchSpace)
}

// chargeOptimizerTranspose bounds the independent reachability, rotation, and
// scoring guards used by transpose under the optimizer's single operation
// budget. Cheap eligibility checks avoid charging the worst case when
// transpose will return immediately.
func chargeOptimizerTranspose(g *layoutgraph.Graph, node *layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction, guard *limits.OptimizationWorkGuard) error {
	if g == nil || node == nil || node.Graph == nil {
		return fmt.Errorf("TALA %s transpose requires a node with a graph", guard.Location())
	}
	if len(g.Nodes) > limits.MaxEngineNodes || len(g.Edges) > limits.MaxEngineEdges || len(edgeAbductions) > limits.MaxEngineEdges {
		return fmt.Errorf("TALA %s transpose inputs exceed engine limits", guard.Location())
	}
	for _, abduction := range edgeAbductions {
		if err := guard.Step(); err != nil {
			return err
		}
		if abduction == nil {
			return fmt.Errorf("TALA %s found a nil edge abduction", guard.Location())
		}
	}
	if node.Hierarchy != nil || node.FixedTopLeft != nil || len(node.Edges) < 1 || len(node.Edges) > 2 {
		return guard.Step()
	}
	if _, exists := g.NodeToTree[node]; exists || g.IsTreeSentinel(node) {
		return guard.Step()
	}

	nodes := uint64(len(g.Nodes) + 1)
	edges := uint64(len(g.Edges) + 1)
	abductions := uint64(len(edgeAbductions) + 1)
	// Reachability is performed several times while selecting a rotation side.
	for range 6 {
		if err := guard.Add(nodes + edges + abductions); err != nil {
			return err
		}
	}
	// At most four trial rotations and one committed rotation are scored.
	for range 5 {
		if edgeAbductions == nil {
			if err := guard.AddProduct(nodes, edges); err != nil {
				return err
			}
			if err := guard.AddProduct(edges, edges); err != nil {
				return err
			}
		} else if err := guard.AddProduct(uint64(len(node.Edges)+1), nodes+abductions); err != nil {
			return err
		}
	}
	// Rotating containers can translate descendants for each trial.
	for range 4 {
		if err := guard.AddProduct(nodes, nodes); err != nil {
			return err
		}
	}
	return nil
}

func optimizerDescendants(node *layoutgraph.Node, guard *limits.OptimizationWorkGuard) ([]*layoutgraph.Node, error) {
	if node == nil || node.Graph == nil {
		return nil, fmt.Errorf("TALA %s movement requires a node with a graph", guard.Location())
	}
	g := node.Graph
	type pendingDescendant struct {
		node *layoutgraph.Node
	}
	seen := map[*layoutgraph.Node]struct{}{node: {}}
	stack := make([]pendingDescendant, 0)
	push := func(child *layoutgraph.Node) error {
		if err := guard.Step(); err != nil {
			return err
		}
		if child == nil {
			return fmt.Errorf("TALA %s found a nil descendant", guard.Location())
		}
		if _, exists := seen[child]; exists {
			return nil
		}
		if len(seen) >= limits.MaxEngineNodes {
			return fmt.Errorf("TALA %s descendant count exceeds limit %d", guard.Location(), limits.MaxEngineNodes)
		}
		seen[child] = struct{}{}
		stack = append(stack, pendingDescendant{node: child})
		return nil
	}
	pushChildren := func(parent *layoutgraph.Node) error {
		if sequence := g.Sequences[parent]; sequence != nil {
			if len(sequence.Nodes) > limits.MaxEngineNodes {
				return fmt.Errorf("TALA %s sequence child references exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
			}
			for _, v := range slices.Backward(sequence.Nodes) {
				if err := push(v); err != nil {
					return err
				}
			}
		}
		if parent != nil && parent.IsClusterVessel() {
			if cluster := g.Clusters[parent]; cluster != nil {
				if len(cluster.Nodes) > limits.MaxEngineNodes {
					return fmt.Errorf("TALA %s cluster child references exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
				}
				for _, v := range slices.Backward(cluster.Nodes) {
					if err := push(v); err != nil {
						return err
					}
				}
			}
		}
		if parent == nil || parent.IsContainer() {
			children := g.Containers[parent]
			if len(children) > limits.MaxEngineNodes {
				return fmt.Errorf("TALA %s container child references exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
			}
			for _, c := range slices.Backward(children) {
				if err := push(c); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := pushChildren(node); err != nil {
		return nil, err
	}
	descendants := make([]*layoutgraph.Node, 0, len(stack))
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		last := len(stack) - 1
		current := stack[last].node
		stack = stack[:last]
		descendants = append(descendants, current)
		if err := pushChildren(current); err != nil {
			return nil, err
		}
	}
	return descendants, nil
}

func optimizerMoveNodeAbs(node *layoutgraph.Node, x, y float64, guard *limits.OptimizationWorkGuard) error {
	if node == nil || node.TopLeft == nil {
		return fmt.Errorf("TALA %s cannot move an unpositioned node", guard.Location())
	}
	if node.TopLeft.X == x && node.TopLeft.Y == y {
		return guard.Step()
	}
	// Most optimizer candidates are ordinary leaves. The general path below
	// walks ownership maps and snapshots descendants for atomic rollback, but
	// those structures are provably empty for a non-container/non-vessel leaf.
	// Moving the single positioned node cannot fail, so avoid allocating a seen
	// map and snapshot slice for every candidate.
	if node.Graph != nil && !node.IsContainer() && !node.IsClusterVessel() && node.Graph.Sequences[node] == nil {
		node.Translate(x-node.TopLeft.X, y-node.TopLeft.Y)
		return nil
	}
	descendants, err := optimizerDescendants(node, guard)
	if err != nil {
		return err
	}
	snapshots := make([]nodePositionSnapshot, 0, len(descendants)+1)
	snapshots = append(snapshots, nodePositionSnapshot{node: node, topLeft: snapshotPointer(node.TopLeft)})
	for _, child := range descendants {
		if err := guard.Step(); err != nil {
			return err
		}
		if child.TopLeft == nil {
			return fmt.Errorf("TALA %s cannot move an unpositioned descendant", guard.Location())
		}
		snapshots = append(snapshots, nodePositionSnapshot{node: child, topLeft: snapshotPointer(child.TopLeft)})
	}
	complete := false
	defer func() {
		if !complete {
			restoreNodePositions(snapshots)
		}
	}()
	dx, dy := x-node.TopLeft.X, y-node.TopLeft.Y
	node.Translate(dx, dy)
	for _, child := range descendants {
		if err := guard.Step(); err != nil {
			return err
		}
		child.Translate(dx, dy)
	}
	complete = true
	return nil
}

func optimizerSwapPositions(nodeA, nodeB *layoutgraph.Node, guard *limits.OptimizationWorkGuard) (err error) {
	if nodeA == nil || nodeB == nil || nodeA.TopLeft == nil || nodeB.TopLeft == nil {
		return fmt.Errorf("TALA %s cannot swap unpositioned nodes", guard.Location())
	}
	snapshots, err := captureOptimizerNodePositions([]*layoutgraph.Node{nodeA, nodeB}, guard)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if recovered := recover(); recovered != nil {
			restoreNodePositions(snapshots)
			panic(recovered)
		}
		if !complete {
			restoreNodePositions(snapshots)
		}
	}()
	ax, ay := nodeA.TopLeft.X, nodeA.TopLeft.Y
	if err := optimizerMoveNodeAbs(nodeA, nodeB.TopLeft.X, nodeB.TopLeft.Y, guard); err != nil {
		return err
	}
	if err := optimizerMoveNodeAbs(nodeB, ax, ay, guard); err != nil {
		return err
	}
	complete = true
	return nil
}

type optimizerNodeMutationSnapshot struct {
	node    *layoutgraph.Node
	topLeft pointerSnapshot[geo.Point]
	width   float64
	height  float64
}

type optimizerClusterMutationSnapshot struct {
	cluster     *layoutgraph.Cluster
	arrangement layoutgraph.ClusterArrangement
	desired     layoutgraph.ClusterArrangement
	padding     float64
}

type optimizerHerdMutationSnapshot struct {
	herd  *layoutgraph.HerdAssignment
	value layoutgraph.HerdAssignment
}

type optimizerMutationSnapshot struct {
	nodes        []optimizerNodeMutationSnapshot
	clusters     []optimizerClusterMutationSnapshot
	herds        []optimizerHerdMutationSnapshot
	costSnapshot *layoutgraph.PlacementCostSnapshot

	// Scratch is owned by one optimizer invocation at a time. Reusing storage
	// avoids rebuilding allocation capacity on every annealing iteration; the
	// topology and rollback values are still captured afresh on each call.
	seenNodes    map[*layoutgraph.Node]struct{}
	seenClusters map[*layoutgraph.Cluster]struct{}
	seenHerds    map[*layoutgraph.HerdAssignment]struct{}
}

func captureOptimizerMutationState(g *layoutgraph.Graph, guard *limits.OptimizationWorkGuard) (*optimizerMutationSnapshot, error) {
	return captureOptimizerMutationStateInto(g, guard, new(optimizerMutationSnapshot))
}

// release clears graph references while retaining request-local buffer capacity.
// It is also safe after an interrupted capture, when only a prefix was recorded.
func (snapshot *optimizerMutationSnapshot) release() {
	clear(snapshot.nodes)
	clear(snapshot.clusters)
	clear(snapshot.herds)
	snapshot.nodes = snapshot.nodes[:0]
	snapshot.clusters = snapshot.clusters[:0]
	snapshot.herds = snapshot.herds[:0]
	snapshot.costSnapshot = nil
	clear(snapshot.seenNodes)
	clear(snapshot.seenClusters)
	clear(snapshot.seenHerds)
}

func captureOptimizerMutationStateInto(g *layoutgraph.Graph, guard *limits.OptimizationWorkGuard, snapshot *optimizerMutationSnapshot) (*optimizerMutationSnapshot, error) {
	snapshot.release()
	if g == nil {
		return nil, fmt.Errorf("TALA %s requires a graph", guard.Location())
	}
	if len(g.Nodes) > limits.MaxEngineNodes {
		return nil, fmt.Errorf("TALA %s snapshot node count exceeds limit %d", guard.Location(), limits.MaxEngineNodes)
	}
	if len(g.Containers) > limits.MaxEngineNodes+1 || len(g.Clusters) > limits.MaxEngineNodes || len(g.Sequences) > limits.MaxEngineNodes {
		return nil, fmt.Errorf("TALA %s optimizer topology maps exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
	}
	cacheEntries := g.EdgeLengthCacheEntries()
	if int64(cacheEntries) > layoutgraph.MaxTopologyReferences {
		return nil, fmt.Errorf("TALA %s edge-length cache entries exceed limit %d", guard.Location(), layoutgraph.MaxTopologyReferences)
	}
	for range cacheEntries {
		if err := guard.Step(); err != nil {
			return nil, err
		}
	}
	costSnapshot := g.SnapshotPlacementCosts()
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	snapshot.costSnapshot = costSnapshot
	if snapshot.seenNodes == nil {
		snapshot.seenNodes = make(map[*layoutgraph.Node]struct{}, len(g.Nodes))
	}
	seenNodes := snapshot.seenNodes
	enqueue := func(node *layoutgraph.Node) error {
		if err := guard.Step(); err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("TALA %s found a nil graph node", guard.Location())
		}
		if _, exists := seenNodes[node]; exists {
			return nil
		}
		if len(seenNodes) >= limits.MaxEngineNodes {
			return fmt.Errorf("TALA %s snapshot node count exceeds limit %d", guard.Location(), limits.MaxEngineNodes)
		}
		seenNodes[node] = struct{}{}
		snapshot.nodes = append(snapshot.nodes, optimizerNodeMutationSnapshot{
			node: node, topLeft: snapshotPointer(node.TopLeft), width: node.Width, height: node.Height,
		})
		return nil
	}
	for _, node := range g.Nodes {
		if err := enqueue(node); err != nil {
			return nil, err
		}
	}
	// The snapshot already contains the queue in discovery order.
	for i := 0; i < len(snapshot.nodes); i++ {
		node := snapshot.nodes[i].node
		if len(g.Containers[node]) > limits.MaxEngineNodes {
			return nil, fmt.Errorf("TALA %s container child references exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
		}
		for _, child := range g.Containers[node] {
			if err := enqueue(child); err != nil {
				return nil, err
			}
		}
		if cluster := g.Clusters[node]; cluster != nil {
			if len(cluster.Nodes) > limits.MaxEngineNodes {
				return nil, fmt.Errorf("TALA %s cluster child references exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
			}
			for _, child := range cluster.Nodes {
				if err := enqueue(child); err != nil {
					return nil, err
				}
			}
		}
		if sequence := g.Sequences[node]; sequence != nil {
			if len(sequence.Nodes) > limits.MaxEngineNodes {
				return nil, fmt.Errorf("TALA %s sequence child references exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
			}
			for _, child := range sequence.Nodes {
				if err := enqueue(child); err != nil {
					return nil, err
				}
			}
		}
	}

	if snapshot.seenClusters == nil && len(g.Clusters) != 0 {
		snapshot.seenClusters = make(map[*layoutgraph.Cluster]struct{})
	}
	seenClusters := snapshot.seenClusters
	for _, cluster := range g.Clusters {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if cluster == nil {
			continue
		}
		if _, exists := seenClusters[cluster]; exists {
			continue
		}
		seenClusters[cluster] = struct{}{}
		snapshot.clusters = append(snapshot.clusters, optimizerClusterMutationSnapshot{
			cluster: cluster, arrangement: cluster.Arrangement, desired: cluster.DesiredArrangement, padding: cluster.Padding,
		})
	}

	seenHerds := snapshot.seenHerds
	for _, nodeSnapshot := range snapshot.nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		herd := nodeSnapshot.node.HerdAssignment
		if herd == nil {
			continue
		}
		if _, exists := seenHerds[herd]; exists {
			continue
		}
		if seenHerds == nil {
			seenHerds = make(map[*layoutgraph.HerdAssignment]struct{})
			snapshot.seenHerds = seenHerds
		}
		seenHerds[herd] = struct{}{}
		snapshot.herds = append(snapshot.herds, optimizerHerdMutationSnapshot{herd: herd, value: *herd})
	}
	return snapshot, nil
}

func (snapshot *optimizerMutationSnapshot) restore() {
	if snapshot == nil {
		return
	}
	for _, node := range snapshot.nodes {
		node.node.TopLeft = node.topLeft.restore()
		node.node.Width = node.width
		node.node.Height = node.height
	}
	for _, cluster := range snapshot.clusters {
		cluster.cluster.Arrangement = cluster.arrangement
		cluster.cluster.DesiredArrangement = cluster.desired
		cluster.cluster.Padding = cluster.padding
	}
	for _, herd := range snapshot.herds {
		*herd.herd = herd.value
	}
	snapshot.costSnapshot.Restore()
}

func captureOptimizerNodePositions(nodes []*layoutgraph.Node, guard *limits.OptimizationWorkGuard) ([]nodePositionSnapshot, error) {
	if len(nodes) <= 2 {
		leaves := true
		for _, node := range nodes {
			if node == nil {
				return nil, fmt.Errorf("TALA %s found a nil node", guard.Location())
			}
			if node.Graph == nil || node.IsContainer() || node.IsClusterVessel() || node.Graph.Sequences[node] != nil {
				leaves = false
				break
			}
		}
		if leaves {
			snapshots := make([]nodePositionSnapshot, 0, len(nodes))
			for _, node := range nodes {
				duplicate := false
				for _, snapshot := range snapshots {
					if snapshot.node == node {
						duplicate = true
						break
					}
				}
				if duplicate {
					continue
				}
				if err := guard.Step(); err != nil {
					return nil, err
				}
				snapshots = append(snapshots, nodePositionSnapshot{node: node, topLeft: snapshotPointer(node.TopLeft)})
			}
			return snapshots, nil
		}
	}
	if len(nodes) == 1 {
		// optimizerDescendants already deduplicates its traversal, including
		// the root. A single root needs no second map or combined node slice.
		descendants, err := optimizerDescendants(nodes[0], guard)
		if err != nil {
			return nil, err
		}
		snapshots := make([]nodePositionSnapshot, len(descendants)+1)
		for i := range snapshots {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			node := nodes[0]
			if i > 0 {
				node = descendants[i-1]
			}
			snapshots[i] = nodePositionSnapshot{node: node, topLeft: snapshotPointer(node.TopLeft)}
		}
		return snapshots, nil
	}
	seen := make(map[*layoutgraph.Node]struct{})
	snapshots := make([]nodePositionSnapshot, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			return nil, fmt.Errorf("TALA %s found a nil node", guard.Location())
		}
		all := []*layoutgraph.Node{node}
		descendants, err := optimizerDescendants(node, guard)
		if err != nil {
			return nil, err
		}
		all = append(all, descendants...)
		for _, current := range all {
			if _, exists := seen[current]; exists {
				continue
			}
			if len(seen) >= limits.MaxEngineNodes {
				return nil, fmt.Errorf("TALA %s position snapshot exceeds node limit %d", guard.Location(), limits.MaxEngineNodes)
			}
			if err := guard.Step(); err != nil {
				return nil, err
			}
			seen[current] = struct{}{}
			snapshots = append(snapshots, nodePositionSnapshot{node: current, topLeft: snapshotPointer(current.TopLeft)})
		}
	}
	return snapshots, nil
}

// withOptimizerPositionsSwapped owns the rollback point for a speculative swap
// and its scoring callback. The former nested atomic swap captured the same
// geometry twice without any intervening mutation. Reuse this rollback point,
// replaying the second capture's exact work and cancellation checkpoints.
func withOptimizerPositionsSwapped(nodeA, nodeB *layoutgraph.Node, guard *limits.OptimizationWorkGuard, fn func() error) (err error) {
	before := guard.Used()
	snapshots, err := captureOptimizerNodePositions([]*layoutgraph.Node{nodeA, nodeB}, guard)
	if err != nil {
		return err
	}
	captureWork := guard.Used() - before
	defer restoreNodePositions(snapshots)
	if nodeA == nil || nodeB == nil || nodeA.TopLeft == nil || nodeB.TopLeft == nil {
		return fmt.Errorf("TALA %s cannot swap unpositioned nodes", guard.Location())
	}
	for range captureWork {
		if err := guard.Step(); err != nil {
			return err
		}
	}
	ax, ay := nodeA.TopLeft.X, nodeA.TopLeft.Y
	if err := optimizerMoveNodeAbs(nodeA, nodeB.TopLeft.X, nodeB.TopLeft.Y, guard); err != nil {
		return err
	}
	if err := optimizerMoveNodeAbs(nodeB, ax, ay, guard); err != nil {
		return err
	}
	return fn()
}
