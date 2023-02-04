package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

type shapeRealSquare struct {
	*baseShape
}

func NewRealSquare(box *geo.Box) Shape {
	return shapeRealSquare{
		baseShape: &baseShape{
			Type: REAL_SQUARE_TYPE,
			Box:  box,
		},
	}
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
