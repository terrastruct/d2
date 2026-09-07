package layoutgraph

import "github.com/d2lang/d2/lib/label"

// Icon is the icon-placement state attached to a layout node.
type Icon struct {
	Position label.Position

	positionFixed bool
}

// PositionFixed reports whether Position was supplied by the caller and must
// be preserved by automatic icon placement.
func (i *Icon) PositionFixed() bool {
	return i.positionFixed
}

// FixPosition marks Position as supplied by the caller.
func (i *Icon) FixPosition() {
	i.positionFixed = true
}
