package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

type shapeStoredData struct {
	shapeSquare
}

func (s shapeStoredData) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:    {},
			label.InsideTopCenter:     {},
			label.InsideMiddleCenter:  {},
			label.InsideBottomCenter:  {},
			label.OutsideBottomCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.OutsideTopRight:    {},
			label.OutsideBottomRight: {},
			label.InsideTopRight:     {},
			label.InsideBottomRight:  {},
			label.InsideMiddleRight:  {},
			label.InsideTopLeft:      {},
			label.InsideBottomLeft:   {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.InsideMiddleLeft:  {},
			label.OutsideLeftMiddle: {},
			label.OutsideTopLeft:    {},
			label.OutsideBottomLeft: {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.OutsideRightMiddle: {},
			label.OutsideLeftTop:     {},
			label.OutsideRightTop:    {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightBottom: {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapeStoredData) SnapPointPercentages() [][]*geo.RelativePoint {
	box := s.GetBox()
	width := box.Width
	height := box.Height
	wedgeWidth := 15.0
	if width < wedgeWidth {
		wedgeWidth = width / 2
	}
	controlPointsMultiplier := 0.27

	topLeftMidpoint := geo.NewBezierCurve([]*geo.Point{
		geo.NewPoint(wedgeWidth, 0.0),
		geo.NewPoint(wedgeWidth-(wedgeWidth*controlPointsMultiplier), 0.0),
		geo.NewPoint(0.0, height*controlPointsMultiplier),
		geo.NewPoint(0.0, height/2.0),
	}).At(0.5)

	bottomLeftMidpoint := geo.NewBezierCurve([]*geo.Point{
		geo.NewPoint(wedgeWidth, height),
		geo.NewPoint(wedgeWidth-(wedgeWidth*controlPointsMultiplier), height),
		geo.NewPoint(0.0, height-height*controlPointsMultiplier),
		geo.NewPoint(0.0, height/2.0),
	}).At(0.5)

	topRightMidpoint := geo.NewBezierCurve([]*geo.Point{
		geo.NewPoint(width, 0),
		geo.NewPoint(width-(wedgeWidth*controlPointsMultiplier), 0),
		geo.NewPoint(width-wedgeWidth, height*controlPointsMultiplier),
		geo.NewPoint(width-wedgeWidth, height/2.0),
	}).At(0.5)

	bottomRightMidpoint := geo.NewBezierCurve([]*geo.Point{
		geo.NewPoint(width-wedgeWidth, height/2.0),
		geo.NewPoint(width-wedgeWidth, height-height*controlPointsMultiplier),
		geo.NewPoint(width-wedgeWidth*controlPointsMultiplier, height),
		geo.NewPoint(width, height),
	}).At(0.5)

	sideWidthPercentage := (width - wedgeWidth) / width
	sideWidthStartPercentage := 1 - sideWidthPercentage

	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(sideWidthStartPercentage+(0.25*sideWidthPercentage), 0),
			geo.NewRelativePoint(sideWidthStartPercentage+(0.5*sideWidthPercentage), 0),
			geo.NewRelativePoint(sideWidthStartPercentage+(0.75*sideWidthPercentage), 0),
		},
		// Left
		{
			geo.NewRelativePoint(topLeftMidpoint.X/width, topLeftMidpoint.Y/height),
			geo.NewRelativePoint(0.0, 0.5),
			geo.NewRelativePoint(bottomLeftMidpoint.X/width, bottomLeftMidpoint.Y/height),
		},
		// Bottom
		{
			geo.NewRelativePoint(sideWidthStartPercentage+(0.25*sideWidthPercentage), 1),
			geo.NewRelativePoint(sideWidthStartPercentage+(0.5*sideWidthPercentage), 1),
			geo.NewRelativePoint(sideWidthStartPercentage+(0.75*sideWidthPercentage), 1),
		},
		// Right
		{
			geo.NewRelativePoint(topRightMidpoint.X/width, topRightMidpoint.Y/height),
			geo.NewRelativePoint((width-wedgeWidth)/width, 0.5),
			geo.NewRelativePoint(bottomRightMidpoint.X/width, bottomRightMidpoint.Y/height),
		},
	}
}
