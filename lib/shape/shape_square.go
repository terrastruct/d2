package shape

import (
	"github.com/d2lang/util-go/go2"
	"oss.terrastruct.com/d2/lib/geo"
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
