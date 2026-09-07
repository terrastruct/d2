package trees

import (
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const treePreprocessLocation = "PreprocessTrees"

func treePreprocessBadState(format string, args ...any) error {
	return invariant.Errorf(format, args...)
}

// treeContainerRDFSOrder is the bounded, iterative equivalent of
// containerRDFSOrder. The stack is populated in forward slice order so its
// LIFO traversal preserves the existing reverse-DFS output order exactly.
func treeContainerRDFSOrder(g *layoutgraph.Graph, root *layoutgraph.Node, guard *limits.WorkGuard) ([]*layoutgraph.Node, error) {
	if root != nil && !root.IsContainer() {
		return nil, nil
	}

	type task struct {
		node *layoutgraph.Node
		emit bool
	}
	stack := make([]task, 0, len(g.Containers[root]))
	for _, child := range g.Containers[root] {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		stack = append(stack, task{node: child})
	}

	order := make([]*layoutgraph.Node, 0, len(g.Containers))
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.node == nil {
			return nil, treePreprocessBadState("tree preprocessing encountered a nil container child")
		}
		if current.emit {
			order = append(order, current.node)
			continue
		}

		if current.node.IsContainer() {
			stack = append(stack, task{node: current.node, emit: true})
			for _, child := range g.Containers[current.node] {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				stack = append(stack, task{node: child})
			}
			continue
		}

		if current.node.IsClusterVessel() {
			cluster := g.Clusters[current.node]
			if cluster == nil {
				return nil, treePreprocessBadState("cluster vessel %d has no cluster record", current.node.ID)
			}
			for _, clusterNode := range cluster.Nodes {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				if clusterNode == nil {
					return nil, treePreprocessBadState("cluster vessel %d contains a nil node", current.node.ID)
				}
				if clusterNode.IsContainer() {
					stack = append(stack, task{node: clusterNode})
				}
			}
		}
	}
	return order, nil
}

func buildTreeAdjacencyMatrix(g *layoutgraph.Graph, guard *limits.WorkGuard) (func(a, b *layoutgraph.Node) *layoutgraph.Edge, error) {
	fromNodeToToNodeToEdge := make(map[*layoutgraph.Node]map[*layoutgraph.Node]*layoutgraph.Edge)
	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if edge == nil || edge.From == nil || edge.To == nil {
			return nil, treePreprocessBadState("tree preprocessing encountered an edge with missing endpoints")
		}
		if _, has := fromNodeToToNodeToEdge[edge.From]; !has {
			fromNodeToToNodeToEdge[edge.From] = make(map[*layoutgraph.Node]*layoutgraph.Edge)
		}
		fromNodeToToNodeToEdge[edge.From][edge.To] = edge
	}
	return func(a, b *layoutgraph.Node) *layoutgraph.Edge {
		edge, has := fromNodeToToNodeToEdge[a][b]
		if !has {
			edge = fromNodeToToNodeToEdge[b][a]
		}
		return edge
	}, nil
}

func treeFringeNodes(nodes []*layoutgraph.Node, guard *limits.WorkGuard) ([]*layoutgraph.Node, error) {
	inSet := make(map[*layoutgraph.Node]struct{}, len(nodes))
	for _, node := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node == nil {
			return nil, treePreprocessBadState("tree preprocessing encountered a nil node")
		}
		inSet[node] = struct{}{}
	}

	fringeNodes := make([]*layoutgraph.Node, 0)
	for _, node := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if len(node.Edges) != 1 {
			continue
		}
		edge := node.Edges[0]
		if edge == nil || (edge.From != node && edge.To != node) {
			return nil, treePreprocessBadState("node %d contains a non-incident fringe edge", node.ID)
		}
		adjacent := node.Adjacent(edge)
		if _, is := inSet[adjacent]; is {
			fringeNodes = append(fringeNodes, node)
		}
	}
	return fringeNodes, nil
}

func filterTerminalTrees(trees []*layoutgraph.Tree, terminals map[*layoutgraph.Node]bool, guard *limits.WorkGuard) ([]*layoutgraph.Tree, error) {
	filtered := make([]*layoutgraph.Tree, 0)
	for _, tree := range trees {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if tree == nil || tree.Node == nil {
			return nil, treePreprocessBadState("tree preprocessing encountered a tree without a node")
		}
		if _, is := terminals[tree.Node]; !is {
			filtered = append(filtered, tree)
		}
	}
	return filtered, nil
}

// dominantTreeEdgeDirection preserves the recursive helper's preorder search:
// the first non-undirected sentinel direction wins, otherwise the root's
// undirected direction is returned.
func dominantTreeEdgeDirection(root *layoutgraph.Tree, includeRoot bool, guard *limits.WorkGuard) (TreeEdgeDirection, error) {
	if root == nil || root.Node == nil {
		return Undirected, treePreprocessBadState("tree preprocessing encountered an incomplete tree")
	}
	if root.SentinelEdge == nil {
		return Undirected, treePreprocessBadState("tree node %d has no sentinel edge", root.Node.ID)
	}

	stack := make([]*layoutgraph.Tree, 0, len(root.Children)+1)
	if includeRoot {
		stack = append(stack, root)
	} else {
		for _, v := range slices.Backward(root.Children) {
			if err := guard.Step(); err != nil {
				return Undirected, err
			}
			stack = append(stack, v)
		}
	}
	seen := make(map[*layoutgraph.Tree]struct{})
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return Undirected, err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || current.Node == nil || current.SentinelEdge == nil {
			return Undirected, treePreprocessBadState("tree preprocessing encountered an incomplete tree")
		}
		if _, exists := seen[current]; exists {
			return Undirected, treePreprocessBadState("tree preprocessing encountered a repeated tree node")
		}
		seen[current] = struct{}{}
		direction := treeEdgeDirection(current.Node, current.SentinelEdge)
		if direction != Undirected {
			return direction, nil
		}
		for _, v := range slices.Backward(current.Children) {
			if err := guard.Step(); err != nil {
				return Undirected, err
			}
			stack = append(stack, v)
		}
	}
	return treeEdgeDirection(root.Node, root.SentinelEdge), nil
}

func countTreeDirections(trees []*layoutgraph.Tree, guard *limits.WorkGuard) (map[TreeEdgeDirection]int, error) {
	directionCounts := make(map[TreeEdgeDirection]int)
	for _, tree := range trees {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		direction, err := dominantTreeEdgeDirection(tree, true, guard)
		if err != nil {
			return nil, err
		}
		directionCounts[direction]++
	}
	return directionCounts, nil
}

func candidateTreeRoots(roots []*layoutgraph.Tree, guard *limits.WorkGuard) ([]*layoutgraph.Tree, error) {
	rootsByDirection := make(map[TreeEdgeDirection][]*layoutgraph.Tree)
	for _, root := range roots {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		direction, err := dominantTreeEdgeDirection(root, false, guard)
		if err != nil {
			return nil, err
		}
		rootsByDirection[direction] = append(rootsByDirection[direction], root)
	}

	nOutwards := len(rootsByDirection[Outwards])
	nInwards := len(rootsByDirection[Inwards])
	nBidirectional := len(rootsByDirection[Bidirectional])
	nUndirected := len(rootsByDirection[Undirected])
	candidates := make([]*layoutgraph.Tree, 0, len(roots))
	appendRoots := func(values []*layoutgraph.Tree) error {
		for _, value := range values {
			if err := guard.Step(); err != nil {
				return err
			}
			candidates = append(candidates, value)
		}
		return nil
	}
	if nOutwards == 1 && 1+nInwards+nUndirected == len(roots) {
		if err := appendRoots(rootsByDirection[Outwards]); err != nil {
			return nil, err
		}
	}
	if nInwards == 1 && 1+nOutwards+nUndirected == len(roots) {
		if err := appendRoots(rootsByDirection[Inwards]); err != nil {
			return nil, err
		}
	}
	if nUndirected > 0 && nOutwards > 1 && nUndirected+nOutwards == len(roots) {
		if err := appendRoots(rootsByDirection[Undirected]); err != nil {
			return nil, err
		}
	}
	if nUndirected > 0 && nInwards > 1 && nUndirected+nInwards == len(roots) {
		if err := appendRoots(rootsByDirection[Undirected]); err != nil {
			return nil, err
		}
	}
	if nBidirectional+nUndirected == len(roots) {
		if err := appendRoots(rootsByDirection[Bidirectional]); err != nil {
			return nil, err
		}
		if err := appendRoots(rootsByDirection[Undirected]); err != nil {
			return nil, err
		}
	}
	if err := guard.Check(); err != nil {
		return nil, err
	}
	return candidates, nil
}

func treeEndOfLine(root *layoutgraph.Tree, guard *limits.WorkGuard) (*layoutgraph.Tree, error) {
	seen := make(map[*layoutgraph.Tree]struct{})
	current := root
	for {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if current == nil {
			return nil, treePreprocessBadState("tree preprocessing encountered a nil tree")
		}
		if _, exists := seen[current]; exists {
			return nil, treePreprocessBadState("tree preprocessing encountered a tree cycle")
		}
		seen[current] = struct{}{}
		switch len(current.Children) {
		case 0:
			return current, nil
		case 1:
			current = current.Children[0]
		default:
			return nil, nil
		}
	}
}

func removeTreeChild(parent, child *layoutgraph.Tree, guard *limits.WorkGuard) error {
	filtered := make([]*layoutgraph.Tree, 0, len(parent.Children))
	for _, current := range parent.Children {
		if err := guard.Step(); err != nil {
			return err
		}
		if current != child {
			filtered = append(filtered, current)
		}
	}
	parent.Children = filtered
	return nil
}

func addTreeChild(parent, child *layoutgraph.Tree, guard *limits.WorkGuard) error {
	if parent == nil || child == nil {
		return treePreprocessBadState("tree preprocessing cannot connect a nil tree")
	}
	if child.Parent != nil {
		if err := removeTreeChild(child.Parent, child, guard); err != nil {
			return err
		}
	}
	if err := guard.Step(); err != nil {
		return err
	}
	child.Parent = parent
	parent.Children = append(parent.Children, child)
	return nil
}

func reverseTreeChain(root *layoutgraph.Tree, guard *limits.WorkGuard) error {
	if root == nil {
		return treePreprocessBadState("tree preprocessing cannot reverse a nil tree")
	}
	chain := []*layoutgraph.Tree{root}
	seen := make(map[*layoutgraph.Tree]struct{})
	current := root
	for len(current.Children) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		if len(current.Children) != 1 {
			return treePreprocessBadState("tree preprocessing attempted to reverse a branching tree")
		}
		if _, exists := seen[current]; exists {
			return treePreprocessBadState("tree preprocessing encountered a tree cycle")
		}
		seen[current] = struct{}{}
		next := current.Children[0]
		if next == nil {
			return treePreprocessBadState("tree preprocessing encountered a nil tree child")
		}
		current.SentinelEdge = next.SentinelEdge
		next.SentinelEdge = nil
		if err := removeTreeChild(current, next, guard); err != nil {
			return err
		}
		chain = append(chain, next)
		current = next
	}
	for i := len(chain) - 1; i > 0; i-- {
		if err := addTreeChild(chain[i], chain[i-1], guard); err != nil {
			return err
		}
	}
	return nil
}

func treeBranches(root *layoutgraph.Tree, guard *limits.WorkGuard) (bool, error) {
	seen := make(map[*layoutgraph.Tree]struct{})
	current := root
	for {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if current == nil {
			return false, treePreprocessBadState("tree preprocessing encountered a nil tree")
		}
		if _, exists := seen[current]; exists {
			return false, treePreprocessBadState("tree preprocessing encountered a tree cycle")
		}
		seen[current] = struct{}{}
		switch len(current.Children) {
		case 0:
			return false, nil
		case 1:
			current = current.Children[0]
		default:
			return true, nil
		}
	}
}

func treeSize(root *layoutgraph.Tree, guard *limits.WorkGuard) (int, error) {
	if root == nil {
		return 0, treePreprocessBadState("tree preprocessing cannot size a nil tree")
	}
	stack := []*layoutgraph.Tree{root}
	seen := make(map[*layoutgraph.Tree]struct{})
	size := 0
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || current.Node == nil {
			return 0, treePreprocessBadState("tree preprocessing encountered an incomplete tree")
		}
		if _, exists := seen[current]; exists {
			return 0, treePreprocessBadState("tree preprocessing encountered a repeated tree node")
		}
		seen[current] = struct{}{}
		size++
		for _, v := range slices.Backward(current.Children) {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			stack = append(stack, v)
		}
	}
	return size, nil
}

func treeDescendants(root *layoutgraph.Tree, guard *limits.WorkGuard) ([]*layoutgraph.Tree, error) {
	if root == nil {
		return nil, treePreprocessBadState("tree preprocessing cannot traverse a nil tree")
	}
	descendants := make([]*layoutgraph.Tree, 0)
	queue := []*layoutgraph.Tree{root}
	seen := make(map[*layoutgraph.Tree]struct{})
	for index := 0; index < len(queue); index++ {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		current := queue[index]
		if current == nil || current.Node == nil {
			return nil, treePreprocessBadState("tree preprocessing encountered an incomplete tree")
		}
		if _, exists := seen[current]; exists {
			return nil, treePreprocessBadState("tree preprocessing encountered a repeated tree node")
		}
		seen[current] = struct{}{}
		for _, child := range current.Children {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			descendants = append(descendants, child)
			queue = append(queue, child)
		}
	}
	return descendants, nil
}

func rootsByTreeDirection(treeRoots []*layoutgraph.Tree, guard *limits.WorkGuard) (map[TreeEdgeDirection][]*layoutgraph.Tree, error) {
	rootsByDirection := make(map[TreeEdgeDirection][]*layoutgraph.Tree)
	for _, root := range treeRoots {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		direction, err := dominantTreeEdgeDirection(root, false, guard)
		if err != nil {
			return nil, err
		}
		rootsByDirection[direction] = append(rootsByDirection[direction], root)
	}
	return rootsByDirection, nil
}

func removeNodeFromGraph(node *layoutgraph.Node, graph *layoutgraph.Graph, guard *limits.WorkGuard) error {
	if len(graph.Nodes) == 0 {
		return treePreprocessBadState("node %d is missing from the graph", node.ID)
	}
	newNodes := make([]*layoutgraph.Node, 0, len(graph.Nodes)-1)
	found := false
	for _, current := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if current == node {
			found = true
			continue
		}
		newNodes = append(newNodes, current)
	}
	if !found {
		return treePreprocessBadState("node %d is missing from the graph", node.ID)
	}
	graph.Nodes = newNodes
	return nil
}

func removeEdgeFromNode(node *layoutgraph.Node, edge *layoutgraph.Edge, guard *limits.WorkGuard) error {
	for i, current := range node.Edges {
		if err := guard.Step(); err != nil {
			return err
		}
		if current == edge {
			node.Edges = append(node.Edges[:i], node.Edges[i+1:]...)
			return nil
		}
	}
	return treePreprocessBadState("edge %d is missing from node %d", edge.ID, node.ID)
}

func disconnectTreeEdge(graph *layoutgraph.Graph, edge *layoutgraph.Edge, guard *limits.WorkGuard) error {
	if edge == nil || edge.From == nil || edge.To == nil {
		return treePreprocessBadState("tree preprocessing cannot disconnect an incomplete edge")
	}
	if err := removeEdgeFromNode(edge.From, edge, guard); err != nil {
		return err
	}
	if edge.To != edge.From {
		if err := removeEdgeFromNode(edge.To, edge, guard); err != nil {
			return err
		}
	} else {
		// Disconnect historically calls removeEdge for each endpoint even on a
		// self-loop. Preserve the second removal only when a duplicate exists.
		for _, current := range edge.To.Edges {
			if err := guard.Step(); err != nil {
				return err
			}
			if current == edge {
				if err := removeEdgeFromNode(edge.To, edge, guard); err != nil {
					return err
				}
				break
			}
		}
	}

	newEdges := make([]*layoutgraph.Edge, 0, len(graph.Edges))
	found := false
	for _, current := range graph.Edges {
		if err := guard.Step(); err != nil {
			return err
		}
		if current == edge {
			found = true
			continue
		}
		newEdges = append(newEdges, current)
	}
	if !found {
		return treePreprocessBadState("edge %d is missing from the graph", edge.ID)
	}
	graph.Edges = newEdges
	return nil
}

func removeNodeFromContainer(graph *layoutgraph.Graph, container, node *layoutgraph.Node, guard *limits.WorkGuard) error {
	filtered := make([]*layoutgraph.Node, 0)
	found := false
	for _, current := range graph.Containers[container] {
		if err := guard.Step(); err != nil {
			return err
		}
		if current == node {
			found = true
			continue
		}
		filtered = append(filtered, current)
	}
	if !found {
		return treePreprocessBadState("node %d is missing from its tree container", node.ID)
	}
	graph.Containers[container] = filtered
	return nil
}

func reconnectTreeGuarded(g *layoutgraph.Graph, tree *layoutgraph.Tree, container *layoutgraph.Node, guard *limits.WorkGuard) error {
	if tree == nil {
		return treePreprocessBadState("tree preprocessing cannot reconnect a nil tree")
	}
	stack := []*layoutgraph.Tree{tree}
	seen := make(map[*layoutgraph.Tree]struct{})
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || current.Node == nil {
			return treePreprocessBadState("tree preprocessing encountered an incomplete tree")
		}
		if _, exists := seen[current]; exists {
			return treePreprocessBadState("tree preprocessing encountered a repeated tree node")
		}
		seen[current] = struct{}{}

		if current.Parent != nil {
			edge := current.SentinelEdge
			if edge == nil || edge.From == nil || edge.To == nil {
				return treePreprocessBadState("tree node %d has no complete sentinel edge", current.Node.ID)
			}
			g.AddNodeUnchecked(current.Node)
			g.AddNodeToContainer(container, current.Node)
			edge.From.AddIncidentEdgeUnchecked(edge)
			edge.To.AddIncidentEdgeUnchecked(edge)
			g.Edges = append(g.Edges, edge)
			if err := guard.Step(); err != nil {
				return err
			}
		}

		for _, v := range slices.Backward(current.Children) {
			if err := guard.Step(); err != nil {
				return err
			}
			stack = append(stack, v)
		}
	}
	return nil
}

func isIsolatedTreeGuarded(g *layoutgraph.Graph, node *layoutgraph.Node, guard *limits.WorkGuard) (bool, error) {
	if node == nil {
		return false, treePreprocessBadState("tree preprocessing encountered a nil tree sentinel")
	}
	if node.IsContainer() {
		return false, nil
	}
	for _, edge := range node.Edges {
		if err := guard.Step(); err != nil {
			return false, err
		}
		isToTree := false
		for _, root := range g.Trees[node] {
			if err := guard.Step(); err != nil {
				return false, err
			}
			if root != nil && edge == root.SentinelEdge {
				isToTree = true
				break
			}
		}
		if !isToTree {
			return false, nil
		}
	}
	return true, nil
}

func buildNodeToTreeGuarded(g *layoutgraph.Graph, guard *limits.WorkGuard) error {
	nodeToTree := make(map[*layoutgraph.Node]*layoutgraph.Tree)
	order := make([]*layoutgraph.Node, 0, len(g.Trees))
	for rootSentinel := range g.Trees {
		if err := guard.Step(); err != nil {
			return err
		}
		order = append(order, rootSentinel)
	}
	layoutgraph.SortNodesByID(order)
	if err := guard.Check(); err != nil {
		return err
	}
	for _, rootSentinel := range order {
		if err := guard.Step(); err != nil {
			return err
		}
		for _, root := range g.Trees[rootSentinel] {
			if root == nil || root.Node == nil {
				return treePreprocessBadState("tree sentinel contains an incomplete root")
			}
			queue := []*layoutgraph.Tree{root}
			seen := make(map[*layoutgraph.Tree]struct{})
			for index := 0; index < len(queue); index++ {
				if err := guard.Step(); err != nil {
					return err
				}
				current := queue[index]
				if current == nil || current.Node == nil {
					return treePreprocessBadState("tree preprocessing encountered an incomplete tree")
				}
				if _, exists := seen[current]; exists {
					return treePreprocessBadState("tree preprocessing encountered a repeated tree node")
				}
				seen[current] = struct{}{}
				nodeToTree[current.Node] = current
				for _, child := range current.Children {
					if err := guard.Step(); err != nil {
						return err
					}
					queue = append(queue, child)
				}
			}
		}
	}
	g.NodeToTree = nodeToTree
	return guard.Check()
}
