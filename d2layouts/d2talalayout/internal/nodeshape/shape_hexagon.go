package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

type shapeHexagon struct {
	shapeSquare
}

func (s shapeHexagon) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:    {},
			label.InsideMiddleCenter:  {},
			label.OutsideBottomCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.InsideMiddleLeft:   {},
			label.InsideMiddleRight:  {},
			label.InsideTopCenter:    {},
			label.InsideBottomCenter: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideLeftMiddle:  {},
			label.OutsideRightMiddle: {},
			label.InsideTopLeft:      {},
			label.InsideTopRight:     {},
			label.InsideBottomLeft:   {},
			label.InsideBottomRight:  {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.OutsideTopLeft:     {},
			label.OutsideTopRight:    {},
			label.OutsideLeftTop:     {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightTop:    {},
			label.OutsideRightBottom: {},
			label.OutsideBottomLeft:  {},
			label.OutsideBottomRight: {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapeHexagon) SnapPointPercentages() [][]*geo.RelativePoint {
	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(0.25, 0),
			geo.NewRelativePoint(0.5, 0),
			geo.NewRelativePoint(0.75, 0),
		},
		// Left
		{
			geo.NewRelativePoint(0.125, 0.25),
			geo.NewRelativePoint(0, 0.5),
			geo.NewRelativePoint(0.125, 0.75),
		},
		// Bottom
		{
			geo.NewRelativePoint(0.25, 1),
			geo.NewRelativePoint(0.5, 1),
			geo.NewRelativePoint(0.75, 1),
		},
		// Right
		{
			geo.NewRelativePoint(0.875, 0.25),
			geo.NewRelativePoint(1, 0.5),
			geo.NewRelativePoint(0.875, 0.75),
		},
	}
}
