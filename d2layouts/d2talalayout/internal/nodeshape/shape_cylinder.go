package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

type shapeCylinder struct {
	shapeSquare
}

func (s shapeCylinder) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:    {},
			label.InsideMiddleCenter:  {},
			label.OutsideBottomCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.InsideTopCenter:    {},
			label.InsideBottomCenter: {},
			label.InsideMiddleLeft:   {},
			label.OutsideTopRight:    {},
			label.OutsideBottomRight: {},
			label.OutsideLeftMiddle:  {},
			label.OutsideRightMiddle: {},
			label.OutsideTopLeft:     {},
			label.OutsideBottomLeft:  {},
			label.InsideTopLeft:      {},
			label.InsideBottomLeft:   {},
			label.InsideMiddleRight:  {},
			label.InsideTopRight:     {},
			label.InsideBottomRight:  {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideLeftTop:     {},
			label.OutsideRightTop:    {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightBottom: {},
		}
	case Bad:
		return map[label.Position]struct{}{}
	}
	return map[label.Position]struct{}{}
}

func (s shapeCylinder) SnapPointPercentages() [][]*geo.RelativePoint {
	box := s.GetBox()
	width := box.Width
	height := box.Height
	arcDepth := 24.0
	controlPointsMultiplier := 0.45

	if height < arcDepth*2 {
		arcDepth = height / 2.0
	}

	topLeftMidpoint := geo.NewBezierCurve([]*geo.Point{
		geo.NewPoint(0, arcDepth),
		geo.NewPoint(0, 0),
		geo.NewPoint(width*controlPointsMultiplier, 0),
		geo.NewPoint(width/2.0, 0),
	}).At(0.5)
	topRightMidpoint := geo.NewBezierCurve([]*geo.Point{
		geo.NewPoint(width/2.0, 0),
		geo.NewPoint(width-(width*controlPointsMultiplier), 0),
		geo.NewPoint(width, 0),
		geo.NewPoint(width, arcDepth),
	}).At(0.5)
	bottomRightMidpoint := geo.NewBezierCurve([]*geo.Point{
		geo.NewPoint(width, height-arcDepth),
		geo.NewPoint(width, height),
		geo.NewPoint(width-width*controlPointsMultiplier, height),
		geo.NewPoint(width/2.0, height),
	}).At(0.5)
	bottomLeftMidpoint := geo.NewBezierCurve([]*geo.Point{
		geo.NewPoint(width/2.0, height),
		geo.NewPoint(width*controlPointsMultiplier, height),
		geo.NewPoint(0, height),
		geo.NewPoint(0, height-arcDepth),
	}).At(0.5)

	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(topLeftMidpoint.X/width, topLeftMidpoint.Y/height),
			geo.NewRelativePoint(0.5, 0),
			geo.NewRelativePoint(topRightMidpoint.X/width, topRightMidpoint.Y/height),
		},
		// Left
		{
			geo.NewRelativePoint(0, arcDepth/height+((height-arcDepth*2)/height)*0.25),
			geo.NewRelativePoint(0, arcDepth/height+((height-arcDepth*2)/height)*0.5),
			geo.NewRelativePoint(0, arcDepth/height+((height-arcDepth*2)/height)*0.75),
		},
		// Bottom
		{
			geo.NewRelativePoint(bottomLeftMidpoint.X/width, bottomLeftMidpoint.Y/height),
			geo.NewRelativePoint(0.5, 1),
			geo.NewRelativePoint(bottomRightMidpoint.X/width, bottomRightMidpoint.Y/height),
		},
		// Right
		{
			geo.NewRelativePoint(1, arcDepth/height+((height-arcDepth*2)/height)*0.25),
			geo.NewRelativePoint(1, arcDepth/height+((height-arcDepth*2)/height)*0.5),
			geo.NewRelativePoint(1, arcDepth/height+((height-arcDepth*2)/height)*0.75),
		},
	}
}
