package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

type shapeParallelogram struct {
	shapeSquare
}

func (s shapeParallelogram) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.InsideTopLeft:      {},
			label.InsideMiddleCenter: {},
			label.InsideBottomRight:  {},
			label.OutsideTopRight:    {},
			label.OutsideBottomLeft:  {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:    {},
			label.InsideTopCenter:     {},
			label.InsideBottomCenter:  {},
			label.OutsideBottomCenter: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideLeftMiddle:  {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightTop:    {},
			label.OutsideRightMiddle: {},
			label.InsideTopRight:     {},
			label.InsideMiddleLeft:   {},
			label.InsideMiddleRight:  {},
			label.InsideBottomLeft:   {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.OutsideTopLeft:     {},
			label.OutsideLeftTop:     {},
			label.OutsideRightBottom: {},
			label.OutsideBottomRight: {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapeParallelogram) SnapPointPercentages() [][]*geo.RelativePoint {
	width := s.GetBox().Width
	wedgeWidth := 26.0
	if width < wedgeWidth {
		wedgeWidth = width / 2.0
	}

	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(wedgeWidth/width+((width-wedgeWidth)/width)*0.25, 0),
			geo.NewRelativePoint(wedgeWidth/width+((width-wedgeWidth)/width)*0.5, 0),
			geo.NewRelativePoint(wedgeWidth/width+((width-wedgeWidth)/width)*0.75, 0),
		},
		// Left
		{
			geo.NewRelativePoint((wedgeWidth/2.0+wedgeWidth)/2.0/width, 0.25),
			geo.NewRelativePoint(wedgeWidth/2.0/width, 0.5),
			geo.NewRelativePoint(wedgeWidth/2.0/2.0/width, 0.75),
		},
		// Bottom
		{
			geo.NewRelativePoint(((width-wedgeWidth)/width)*0.25, 1),
			geo.NewRelativePoint(((width-wedgeWidth)/width)*0.5, 1),
			geo.NewRelativePoint(((width-wedgeWidth)/width)*0.75, 1),
		},
		// Right
		{
			geo.NewRelativePoint((width-wedgeWidth)/width+(wedgeWidth/2.0+wedgeWidth)/2.0/width, 0.25),
			geo.NewRelativePoint((width-wedgeWidth)/width+wedgeWidth/2.0/width, 0.5),
			geo.NewRelativePoint((width-wedgeWidth)/width+wedgeWidth/2.0/2.0/width, 0.75),
		},
	}
}
