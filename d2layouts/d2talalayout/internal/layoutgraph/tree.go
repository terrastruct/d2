package layoutgraph

import "github.com/d2lang/d2/lib/geo"

type Tree struct {
	Node         *Node
	Parent       *Tree
	Children     []*Tree
	SentinelEdge *Edge
	Orientation  geo.Orientation
}

type Trees []*Tree

func NewTree(node *Node) *Tree {
	t := new(Tree)
	t.Node = node
	t.Children = make([]*Tree, 0)
	return t
}

func (t *Tree) recordInternalTreeEdges(isTreeEdge map[*Edge]bool) {
	if t.Parent != nil && t.SentinelEdge != nil {
		isTreeEdge[t.SentinelEdge] = true
	}
	for _, c := range t.Children {
		c.recordInternalTreeEdges(isTreeEdge)
	}
}

func (g *Graph) buildIsTreeEdgeMap() map[*Edge]bool {
	// Note: this only includes internal tree edges. It doesn't include the edge between the tree root and the rootSentinel
	isTreeEdge := make(map[*Edge]bool)
	for _, rootSentinel := range g.TreeOrder() {
		for _, root := range g.Trees[rootSentinel] {
			root.recordInternalTreeEdges(isTreeEdge)
		}
	}
	return isTreeEdge
}

func (g *Graph) isIsolatedTree(node *Node) bool {
	if node.isContainer {
		return false
	}
	// it is an isolated tree if all edges are to trees
	for _, edge := range node.Edges {
		isToTree := false
		for _, root := range g.Trees[node] {
			if edge == root.SentinelEdge {
				isToTree = true
				break
			}
		}
		if !isToTree {
			return false
		}
	}
	return true
}

func (g *Graph) addIsolatedTreeEdgesToMap(isTreeEdge map[*Edge]bool) {
	for _, rootSentinel := range g.TreeOrder() {
		if g.isIsolatedTree(rootSentinel) {
			for _, root := range g.Trees[rootSentinel] {
				if root.SentinelEdge != nil {
					isTreeEdge[root.SentinelEdge] = true
				}
			}
		}
	}
}

// tree is either the source or target of its sentinel edge, return true if it is the source
func (t *Tree) isSentinelEdgeSource() bool {
	return t.SentinelEdge.From == t.Node
}

// return the node this tree is connected to via its sentinel edge
func (t *Tree) sentinelNode() *Node {
	if t.isSentinelEdgeSource() {
		return t.SentinelEdge.To
	}
	return t.SentinelEdge.From
}

func (tree *Tree) root() *Tree {
	if tree.Parent == nil {
		return tree
	}
	return tree.Parent.root()
}
