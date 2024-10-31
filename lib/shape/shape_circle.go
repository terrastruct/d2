package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/util-go/go2"
)

type shapeCircle struct {
	*baseShape
}

func NewCircle(box *geo.Box) Shape {
	shape := shapeCircle{
		baseShape: &baseShape{
			Type: CIRCLE_TYPE,
			Box:  box,
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeCircle) GetInnerBox() *geo.Box {
	width := s.Box.Width
	height := s.Box.Height
	insideTL := s.GetInsidePlacement(width, height, 0, 0)
	tl := s.Box.TopLeft.Copy()
	width -= 2 * (insideTL.X - tl.X)
	height -= 2 * (insideTL.Y - tl.Y)
	return geo.NewBox(&insideTL, width, height)
}

func (s shapeCircle) AspectRatio1() bool {
	return true
}

func (s shapeCircle) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	effectiveWidth := width + 2*paddingX
	effectiveHeight := height + 2*paddingY
	diameter := math.Ceil(math.Max(effectiveWidth, effectiveHeight))
	return diameter, diameter
}

func (s shapeCircle) GetInsidePlacement(width, height, paddingX, paddingY float64) geo.Point {
	centerX := s.Box.TopLeft.X + paddingX
	centerY := s.Box.TopLeft.Y + paddingY
	r := s.Box.Width / 2.
	x := centerX - r
	y := centerY - r

	return geo.Point{
		X: math.Ceil(x),
		Y: math.Ceil(y),
	}
}

func (s shapeCircle) Perimeter() []geo.Intersectable {
	return []geo.Intersectable{geo.NewEllipse(s.Box.Center(), s.Box.Width/2, s.Box.Height/2)}
}

func (s shapeCircle) GetDefaultPadding() (paddingX, paddingY float64) {
	return defaultPadding / math.Sqrt2, defaultPadding / math.Sqrt2
}
