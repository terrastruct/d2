package placementcost

import (
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// obstructionBounds encloses the straight, L-shaped, and alternate midpoint
// routes considered by placement scoring. An obstacle outside it cannot block
// any of those routes. It is only a rejection filter; intersecting boxes still
// go through the original ancestry and segment predicates in their original
// order.
type obstructionBounds struct {
	left, top, right, bottom float64
	usable                   bool
}

func scoringNodeBounds(node *layoutgraph.Node) obstructionBounds {
	if node == nil || node.TopLeft == nil {
		return obstructionBounds{}
	}
	x, y := node.TopLeft.X, node.TopLeft.Y
	right, bottom := x+node.Width, y+node.Height
	// Keep non-finite and overflow-prone trial geometry on the original path.
	const bound = math.MaxFloat64 / 8
	usable := x >= -bound && x <= bound && y >= -bound && y <= bound &&
		right >= -bound && right <= bound && bottom >= -bound && bottom <= bound
	return obstructionBounds{min(x, right), min(y, bottom), max(x, right), max(y, bottom), usable}
}

func (bounds obstructionBounds) including(other obstructionBounds) obstructionBounds {
	return obstructionBounds{
		min(bounds.left, other.left), min(bounds.top, other.top),
		max(bounds.right, other.right), max(bounds.bottom, other.bottom),
		bounds.usable && other.usable,
	}
}

func (bounds obstructionBounds) excludes(node *layoutgraph.Node) bool {
	if !bounds.usable || node == nil || node.TopLeft == nil {
		return false
	}
	x, y := node.TopLeft.X, node.TopLeft.Y
	right, bottom := x+node.Width, y+node.Height
	// Paired comparisons also handle negative dimensions, and never reject a
	// dimension containing NaN. Infinite obstacle bounds use the original path.
	if math.IsInf(x, 0) || math.IsInf(y, 0) || math.IsInf(right, 0) || math.IsInf(bottom, 0) ||
		math.IsNaN(x) || math.IsNaN(y) || math.IsNaN(right) || math.IsNaN(bottom) {
		return false
	}
	return (x < bounds.left && right < bounds.left) ||
		(x > bounds.right && right > bounds.right) ||
		(y < bounds.top && bottom < bounds.top) ||
		(y > bounds.bottom && bottom > bounds.bottom)
}
