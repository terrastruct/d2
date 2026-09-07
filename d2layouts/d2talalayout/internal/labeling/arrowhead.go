package labeling

import (
	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labelgeom"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// PositionedArrowheadLabel is the rendered box reserved beside one end of an
// edge route.
type PositionedArrowheadLabel struct {
	geo.Box
	Edge     *layoutgraph.Edge
	IsTarget bool
	Text     string
}

// PositionArrowheadLabel derives the renderer-compatible label box for one
// arrowhead. A nil result means that endpoint has no arrowhead label.
func PositionArrowheadLabel(edge *layoutgraph.Edge, isTarget bool, route geo.Route) *PositionedArrowheadLabel {
	if edge == nil {
		return nil
	}
	value := edge.SourceArrowheadLabel
	if isTarget {
		value = edge.TargetArrowheadLabel
	}
	if value == nil {
		return nil
	}
	topLeft := labelgeom.ArrowheadTopLeft(
		route,
		isTarget,
		string(edge.SourceArrowhead),
		string(edge.TargetArrowhead),
		value.Width,
		value.Height,
	)
	return &PositionedArrowheadLabel{
		Box:      *geo.NewBox(topLeft, value.Width, value.Height),
		Edge:     edge,
		IsTarget: isTarget,
		Text:     value.Text,
	}
}
