package layoutgraph

// connectedOwners stores the usual single parent without a slice allocation.
// Additional runtime ownership views are retained, rather than assuming that
// Node.Container fully describes graph-owned container/cluster membership.
type connectedOwners struct {
	first map[*Node]*Node
	more  map[*Node][]*Node
}

func (owners *connectedOwners) add(child, parent *Node) {
	if owners.first[child] == nil {
		owners.first[child] = parent
		return
	}
	if owners.more == nil {
		owners.more = make(map[*Node][]*Node)
	}
	owners.more[child] = append(owners.more[child], parent)
}

// ConnectedNodeSet returns the same reachable set as ConnectedNodes, with no
// traversal-order guarantee. Use it only when all members are treated equally;
// order-sensitive nearest-node tie breaking must continue using ConnectedNodes.
func (node *Node) ConnectedNodeSet(excludedNodes []*Node, graph *Graph) []*Node {
	// Tiny graphs and flat graphs have too little ownership scanning to amortize
	// an index. Keep their existing traversal.
	if len(graph.Nodes) < 32 || len(graph.Clusters) == 0 && len(graph.Containers) <= 1 {
		return node.connectedNodes(excludedNodes, graph)
	}
	excluded := make(map[*Node]bool, len(excludedNodes))
	for _, n := range excludedNodes {
		excluded[n] = true
	}
	excludedChildren := make(map[*Node]bool)
	owners := connectedOwners{first: make(map[*Node]*Node)}
	for container, children := range graph.Containers {
		if container == nil {
			continue
		}
		blocked := false
		for _, child := range children {
			if excluded[child] {
				blocked = true
				break
			}
		}
		excludedChildren[container] = blocked
		if blocked || excluded[container] {
			continue
		}
		for _, child := range children {
			owners.add(child, container)
		}
	}
	for vessel, cluster := range graph.Clusters {
		for _, member := range cluster.Nodes {
			if !member.isContainer || excludedChildren[member] {
				continue
			}
			for _, child := range graph.Containers[member] {
				owners.add(child, vessel)
			}
		}
	}
	nodes := make([]*Node, 0)
	queue := []*Node{node}
	seen := map[*Node]bool{node: true}
	add := func(n *Node) {
		if !seen[n] {
			seen[n] = true
			queue = append(queue, n)
		}
	}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		blocked := false
		for _, e := range excludedNodes {
			if current.isDescendantOf(e) || e.isDescendantOf(current) && !node.isDescendantOf(current) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		nodes = append(nodes, current)
		for _, edge := range current.Edges {
			adjacent := current.adjacent(edge)
			if !excluded[adjacent] {
				add(adjacent)
			}
		}
		if current.isClusterVessel {
			for _, member := range graph.Clusters[current].Nodes {
				if member.isContainer {
					for _, child := range graph.Containers[member] {
						add(child)
					}
				}
			}
		}
		if owner := owners.first[current]; owner != nil {
			add(owner)
		}
		for _, owner := range owners.more[current] {
			add(owner)
		}
		if current != nil && !excluded[current] && !excludedChildren[current] {
			for _, child := range graph.Containers[current] {
				add(child)
			}
		}
	}
	return nodes
}
