package trees

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

// Descendants returns tree descendants in the breadth-first order used when
// restored tree nodes and edges are copied back into an owning graph.
func Descendants(root *layoutgraph.Tree) []*layoutgraph.Tree {
	if root == nil {
		return nil
	}
	descendants := make([]*layoutgraph.Tree, 0)
	queue := []*layoutgraph.Tree{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range current.Children {
			descendants = append(descendants, child)
			queue = append(queue, child)
		}
	}
	return descendants
}
