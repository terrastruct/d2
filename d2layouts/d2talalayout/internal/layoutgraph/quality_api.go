package layoutgraph

import "context"

// ValidatePositionedGraph checks the structural and positioned geometry
// invariants required by algorithms that inspect labels and routed edges.
func ValidatePositionedGraph(ctx context.Context, operation string, graph *Graph) error {
	return ValidatePositionedGraphSelection(ctx, operation, graph, nil)
}

// Area returns the area of the graph's unrounded bounding box, including
// routed edges and positioned labels.
func (g *Graph) Area() float64 {
	return g.area()
}

// IsImage reports whether the node uses the image shape. Image nodes do not
// render their icon as a separately positioned object.
func (n *Node) IsImage() bool {
	return n != nil && n.shapeType == imageType
}

// EffectiveContainer returns the node's active layout container. Active
// cluster and sequence vessels replace the node's direct container while
// those grouping algorithms are running.
func (n *Node) EffectiveContainer() *Node {
	if n == nil {
		return nil
	}
	return n.container()
}

// AncestryParent returns the parent relation used by layout ancestry checks.
// Unlike EffectiveContainer, active cluster and sequence ancestry follows the
// vessel itself.
func (n *Node) AncestryParent() *Node {
	return ancestryParent(n)
}
