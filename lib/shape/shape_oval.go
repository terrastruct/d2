package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

type shapeOval struct {
	*baseShape
}

func NewOval(box *geo.Box) Shape {
	return shapeOval{
		baseShape: &baseShape{
			Type: OVAL_TYPE,
			Box:  box,
		},
	}
}

func (s shapeOval) GetDimensionsToFit(width, height, padding float64) (float64, float64) {
	theta := math.Atan2(height, width)
	// add padding in direction of diagonal so there is padding distance between top left and border
	paddedWidth := width + 2*padding*math.Cos(theta)
	paddedHeight := height + 2*padding*math.Sin(theta)
	// see https://stackoverflow.com/questions/433371/ellipse-bounding-a-rectangle
	return math.Ceil(math.Sqrt2 * paddedWidth), math.Ceil(math.Sqrt2 * paddedHeight)
}

func (s shapeOval) GetInsidePlacement(width, height, padding float64) geo.Point {
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
	// we want to offset r-padding away from the center
	return *geo.NewPoint(s.Box.TopLeft.X+math.Ceil(rx-cos*(r-padding)), s.Box.TopLeft.Y+math.Ceil(ry-sin*(r-padding)))
}

func (s shapeOval) Perimeter() []geo.Intersectable {
	return []geo.Intersectable{geo.NewEllipse(s.Box.Center(), s.Box.Width/2, s.Box.Height/2)}
}
