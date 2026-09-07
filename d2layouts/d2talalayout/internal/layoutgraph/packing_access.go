package layoutgraph

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

// ContextWithTransactionWorkGuard lets a layout domain share one aggregate
// request budget with every speculative graph transaction it creates.
func ContextWithTransactionWorkGuard(ctx context.Context, guard *limits.WorkGuard) context.Context {
	return contextWithTransactionWorkGuard(ctx, guard)
}

func Covers(outer, inner *geo.Box) bool {
	return covers(outer, inner)
}

func (graph *Graph) IsBadStateWithWorkGuard(node *Node, graphState *GraphState, ignoreContainerEscape bool, guard *limits.WorkGuard) (bool, error) {
	return graph.isBadStateContext(node, graphState, ignoreContainerEscape, guard)
}

func NewEdgeSegment(start, end *geo.Point, edge *Edge) *EdgeSegment {
	return &EdgeSegment{Segment: geo.Segment{Start: start, End: end}, edge: edge}
}

func (node *Node) AllReachableNodesContext(
	includeContainers, includeNears, traverseTrees bool,
	ignore map[*Node]struct{},
	guard *limits.WorkGuard,
) ([]*Node, error) {
	return node.allReachableNodesGuarded(includeContainers, includeNears, traverseTrees, ignore, guard)
}

func (node *Node) ReachableNodesContext(
	shouldVisit func(*Node) bool,
	includeContainers, includeNears, traverseTrees bool,
	ignore map[*Node]struct{},
	guard *limits.WorkGuard,
) ([]*Node, error) {
	return node.reachableNodesGuarded(shouldVisit, includeContainers, includeNears, traverseTrees, ignore, guard)
}

func (graph *Graph) AllDescendantNodesWithWorkGuard(node *Node, includeClusterNodes bool, guard *limits.WorkGuard) ([]*Node, error) {
	return graph.allDescendantNodesGuarded(node, includeClusterNodes, guard)
}

func (node *Node) BoundingBox(allNodes Nodes) (*geo.Point, *geo.Point) {
	return node.bounds(allNodes)
}

func (node *Node) Orientation(other *Node) geo.Orientation {
	return node.orientation(other)
}

func (node *Node) OverlapsLine(start, end *geo.Point, delta float64) bool {
	return node.overlapsLine(start, end, delta)
}

func (node *Node) ModifierElementAdjustments() (dx, dy float64) {
	return node.modifierElementAdjustments()
}
