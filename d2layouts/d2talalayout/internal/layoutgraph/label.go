package layoutgraph

import "github.com/d2lang/d2/lib/label"

// Label is the label state attached to a layout node or edge.
type Label struct {
	Text     string
	Position label.Position
	Width    float64
	Height   float64

	positionFixed bool
}

// PositionFixed reports whether Position was supplied by the caller and must
// be preserved by automatic label placement.
func (l *Label) PositionFixed() bool {
	return l.positionFixed
}

// FixPosition marks Position as supplied by the caller.
func (l *Label) FixPosition() {
	l.positionFixed = true
}
