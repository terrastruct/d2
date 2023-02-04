package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

type shapeCircle struct {
	*baseShape
}

func NewCircle(box *geo.Box) Shape {
	return shapeCircle{
		baseShape: &baseShape{
			Type: CIRCLE_TYPE,
			Box:  box,
		},
	}
}

func (s shapeCircle) GetInnerBox() *geo.Box {
	width := s.Box.Width
	height := s.Box.Height
	insideTL := s.GetInsidePlacement(width, height, 0)
	tl := s.Box.TopLeft.Copy()
	width -= 2 * (insideTL.X - tl.X)
	height -= 2 * (insideTL.Y - tl.Y)
	return geo.NewBox(&insideTL, width, height)
}

func (s shapeCircle) AspectRatio1() bool {
	return true
}

func (s shapeCircle) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	diameter := math.Ceil(math.Sqrt(math.Pow(width+paddingX, 2) + math.Pow(height+paddingY, 2)))
	return diameter, diameter
}

func (s shapeCircle) GetInsidePlacement(width, height, padding float64) geo.Point {
	return *geo.NewPoint(s.Box.TopLeft.X+math.Ceil(s.Box.Width/2-width/2), s.Box.TopLeft.Y+math.Ceil(s.Box.Height/2-height/2))
}

func (s shapeCircle) Perimeter() []geo.Intersectable {
	return []geo.Intersectable{geo.NewEllipse(s.Box.Center(), s.Box.Width/2, s.Box.Height/2)}
}

func (s shapeCircle) GetDefaultPadding() (paddingX, paddingY float64) {
	return defaultPadding / math.Sqrt2, defaultPadding / math.Sqrt2
}
