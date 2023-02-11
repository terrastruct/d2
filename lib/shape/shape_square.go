package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/util-go/go2"
)

type shapeSquare struct {
	*baseShape
}

func NewSquare(box *geo.Box) Shape {
	shape := shapeSquare{
		baseShape: &baseShape{
			Type: SQUARE_TYPE,
			Box:  box,
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeSquare) IsRectangular() bool {
	return true
}
