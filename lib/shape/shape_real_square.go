package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/util-go/go2"
)

type shapeRealSquare struct {
	*baseShape
}

func NewRealSquare(box *geo.Box) Shape {
	shape := shapeRealSquare{
		baseShape: &baseShape{
			Type: REAL_SQUARE_TYPE,
			Box:  box,
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeRealSquare) AspectRatio1() bool {
	return true
}

func (s shapeRealSquare) IsRectangular() bool {
	return true
}

func (s shapeRealSquare) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	sideLength := math.Ceil(math.Max(width+paddingX, height+paddingY))
	return sideLength, sideLength
}
