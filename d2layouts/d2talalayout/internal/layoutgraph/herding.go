package layoutgraph

import (
	"maps"

	"github.com/d2lang/d2/lib/geo"
)

// HerdAssignment records the side of a container toward which a node should
// be placed, together with the uncles paired on the same and opposite sides.
// The proximity package owns the algorithms that produce and synchronize
// these assignments.
type HerdAssignment struct {
	// The whole point of herding to one side is so another herd can pair on the
	// opposite side. Keep unique counts of connected uncles on each side.
	oppositeSidePaired map[*Node]struct{}
	sameSidePaired     map[*Node]struct{}

	// Orientation is the side to herd to: TOP, LEFT, BOTTOM, or RIGHT.
	Orientation geo.Orientation

	// Val is the y-axis coordinate for TOP/BOTTOM and the x-axis coordinate for
	// LEFT/RIGHT.
	Val float64
}

func NewHerdAssignment() *HerdAssignment {
	return &HerdAssignment{
		oppositeSidePaired: make(map[*Node]struct{}),
		sameSidePaired:     make(map[*Node]struct{}),
	}
}

func (ha *HerdAssignment) Copy() *HerdAssignment {
	return &HerdAssignment{
		oppositeSidePaired: maps.Clone(ha.oppositeSidePaired),
		sameSidePaired:     maps.Clone(ha.sameSidePaired),
		Orientation:        ha.Orientation,
		Val:                ha.Val,
	}
}
