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
	length := math.Max(width+paddingX, height+paddingY)
	diameter := math.Ceil(math.Sqrt2 * length)
	return diameter, diameter
}

func (s shapeCircle) GetInsidePlacement(width, height, paddingX, paddingY float64) geo.Point {
	r := s.GetBox().Width / 2
	halfLength := r * math.Sqrt2 / 2.
	p := geo.Point{
		X: s.GetBox().TopLeft.X + math.Ceil(r-halfLength+paddingX/2.),
		Y: s.GetBox().TopLeft.Y + math.Ceil(r-halfLength+paddingY/2.),
	}
	return p
}

func (s shapeCircle) Perimeter() []geo.Intersectable {
	return []geo.Intersectable{geo.NewEllipse(s.Box.Center(), s.Box.Width/2, s.Box.Height/2)}
}

func (s shapeCircle) GetDefaultPadding() (paddingX, paddingY float64) {
	return defaultPadding / math.Sqrt2, defaultPadding / math.Sqrt2
}
