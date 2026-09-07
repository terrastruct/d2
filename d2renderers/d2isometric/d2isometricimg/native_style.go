package d2isometricimg

import (
	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

// Theme-default bodies use the classic study's restrained ink palette.
// Authored colors and theme overrides retain their resolved source values.
func nativeClassicNode(n d2isometric.Node) d2isometric.Node {
	if n.Type == d2target.ShapeClass || n.Type == d2target.ShapeSQLTable {
		return n // Structured documents own their header/body color contract.
	}
	if !n.StrokeExplicit && nativeToken(n.Metadata.Original.Stroke) {
		n.Stroke = "#263c4e"
	}
	return n
}
