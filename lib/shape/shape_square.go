package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
)

type shapeSquare struct {
	*baseShape
}

func NewSquare(box *geo.Box) Shape {
	return shapeSquare{
		baseShape: &baseShape{
			Type: SQUARE_TYPE,
			Box:  box,
		},
	}
}

func (s shapeSquare) IsRectangular() bool {
	return true
}
