package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

type shapeOval struct {
	shapeSquare
}

func (s shapeOval) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:    {},
			label.InsideMiddleCenter:  {},
			label.OutsideBottomCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.InsideBottomCenter: {},
			label.InsideTopCenter:    {},
			label.InsideMiddleLeft:   {},
			label.InsideMiddleRight:  {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideLeftMiddle:  {},
			label.OutsideRightMiddle: {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.OutsideTopLeft:     {},
			label.OutsideTopRight:    {},
			label.OutsideLeftTop:     {},
			label.OutsideRightTop:    {},
			label.OutsideBottomLeft:  {},
			label.OutsideBottomRight: {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightBottom: {},
			label.InsideTopLeft:      {},
			label.InsideTopRight:     {},
			label.InsideBottomLeft:   {},
			label.InsideBottomRight:  {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapeOval) SnapPointPercentages() [][]*geo.RelativePoint {
	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(0.25, 0.066),
			geo.NewRelativePoint(0.5, 0),
			geo.NewRelativePoint(0.75, 0.066),
		},
		// Left
		{
			geo.NewRelativePoint(0.066, 0.25),
			geo.NewRelativePoint(0, 0.5),
			geo.NewRelativePoint(0.066, 0.75),
		},
		// Bottom
		{
			geo.NewRelativePoint(0.25, 0.934),
			geo.NewRelativePoint(0.5, 1),
			geo.NewRelativePoint(0.75, 0.934),
		},
		// Right
		{
			geo.NewRelativePoint(0.934, 0.25),
			geo.NewRelativePoint(1, 0.5),
			geo.NewRelativePoint(0.934, 0.75),
		},
	}
}
