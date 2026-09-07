package layoutgraph

import "github.com/d2lang/d2/lib/geo"

// LabelBoxFits reports whether outer completely contains inner.
func LabelBoxFits(outer, inner *geo.Box) bool {
	return covers(outer, inner)
}

// PadLabelCandidate expands (or shrinks, for a negative value) a temporary
// node box by the same amount on every side.
func (node *Node) PadLabelCandidate(padding int) {
	node.pad(padding)
}

// LabelBoxesOverlap reports whether two node boxes overlap with the requested
// padding.
func (node *Node) LabelBoxesOverlap(other *Node, padding float64) bool {
	return node.doesOverlapCalc(other, padding)
}

// LabelOverlapArea returns the rectangular overlap area. Callers must first prove
// that the nodes overlap.
func (node *Node) LabelOverlapArea(other *Node) float64 {
	return node.overlapArea(other)
}

// IsClusterEdge reports whether either endpoint is currently represented by
// a cluster grouping.
func (edge *Edge) IsClusterEdge() bool {
	return edge != nil && edge.isClusterEdge()
}
