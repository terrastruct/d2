// Package labelgeom contains renderer-compatible label geometry that is
// shared by the mutable graph bounds and label-placement domains.
package labelgeom

import (
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

// ArrowheadTopLeft returns the renderer-compatible top-left position for an
// arrowhead label. D2 stores rendered text dimensions as integers, so the
// float dimensions are intentionally truncated here to preserve historical
// TALA output.
func ArrowheadTopLeft(
	route geo.Route,
	isTarget bool,
	sourceArrowhead, targetArrowhead string,
	width, height float64,
) *geo.Point {
	connection := d2target.BaseConnection()
	connection.Route = route
	connection.SrcArrow = d2target.Arrowhead(sourceArrowhead)
	connection.DstArrow = d2target.Arrowhead(targetArrowhead)

	text := &d2target.Text{
		LabelWidth:  int(width),
		LabelHeight: int(height),
	}
	if isTarget {
		connection.DstLabel = text
	} else {
		connection.SrcLabel = text
	}
	return connection.GetArrowheadLabelPosition(isTarget)
}
