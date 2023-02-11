package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/util-go/go2"
)

type shapeOval struct {
	*baseShape
}

func NewOval(box *geo.Box) Shape {
	shape := shapeOval{
		baseShape: &baseShape{
			Type: OVAL_TYPE,
			Box:  box,
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeOval) GetInnerBox() *geo.Box {
	width := s.Box.Width
	height := s.Box.Height
	insideTL := s.GetInsidePlacement(width, height, 0, 0)
	tl := s.Box.TopLeft.Copy()
	width -= 2 * (insideTL.X - tl.X)
	height -= 2 * (insideTL.Y - tl.Y)
	return geo.NewBox(&insideTL, width, height)
}

func (s shapeOval) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	theta := math.Atan2(height, width)
	// add padding in direction of diagonal so there is padding distance between top left and border
	paddedWidth := width + paddingX*math.Cos(theta)
	paddedHeight := height + paddingY*math.Sin(theta)
	// see https://stackoverflow.com/questions/433371/ellipse-bounding-a-rectangle
	return math.Ceil(math.Sqrt2 * paddedWidth), math.Ceil(math.Sqrt2 * paddedHeight)
}

func (s shapeOval) GetInsidePlacement(width, height, paddingX, paddingY float64) geo.Point {
	// showing the top left arc of the ellipse (drawn with '*')
	// ┌──────────────────* ┬
	// │         *        │ │ry
	// │     *┌───────────┤ │ ┬
	// │  *   │           │ │ │sin*r
	// │*     │           │ │ │
	// *──────┴───────────┼ ┴ ┴
	// ├────────rx────────┤
	//        ├───cos*r───┤
	rx := s.Box.Width / 2
	ry := s.Box.Height / 2
	theta := math.Atan2(ry, rx)
	sin := math.Sin(theta)
	cos := math.Cos(theta)
	// r is the ellipse radius on the line between node.TopLeft and the ellipse center
	// see https://math.stackexchange.com/questions/432902/how-to-get-the-radius-of-an-ellipse-at-a-specific-angle-by-knowing-its-semi-majo
	r := rx * ry / math.Sqrt(math.Pow(rx*sin, 2)+math.Pow(ry*cos, 2))
	// we want to offset r-padding/2 away from the center
	return *geo.NewPoint(s.Box.TopLeft.X+math.Ceil(rx-cos*(r-paddingX/2)), s.Box.TopLeft.Y+math.Ceil(ry-sin*(r-paddingY/2)))
}

func (s shapeOval) Perimeter() []geo.Intersectable {
	return []geo.Intersectable{geo.NewEllipse(s.Box.Center(), s.Box.Width/2, s.Box.Height/2)}
}
