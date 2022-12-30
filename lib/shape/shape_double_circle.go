package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

type shapeDoubleCircle struct {
	*baseShape
}

func NewDoubleCircle(box *geo.Box) Shape {
	return shapeDoubleCircle{
		baseShape: &baseShape{
			Type: DOUBLE_CIRCLE_TYPE,
			Box:  box,
		},
	}
}

func (s shapeDoubleCircle) AspectRatio1() bool {
	return true
}

func (s shapeDoubleCircle) GetDimensionsToFit(width, height, padding float64) (float64, float64) {
	radius := math.Ceil(math.Sqrt(math.Pow(width/2, 2)+math.Pow(height/2, 2))) + padding
	return radius * 2, radius * 2
}

func (s shapeDoubleCircle) GetInsidePlacement(width, height, padding float64) geo.Point {
	return *geo.NewPoint(s.Box.TopLeft.X+math.Ceil(s.Box.Width/2-width/2), s.Box.TopLeft.Y+math.Ceil(s.Box.Height/2-height/2))
}

func (s shapeDoubleCircle) Perimeter() []geo.Intersectable {
	return []geo.Intersectable{geo.NewEllipse(s.Box.Center(), s.Box.Width/2, s.Box.Height/2)}
}
