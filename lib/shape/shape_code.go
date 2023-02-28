package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/util-go/go2"
)

type shapeCode struct {
	shapeSquare
}

func NewCode(box *geo.Box) Shape {
	shape := shapeCode{
		shapeSquare: shapeSquare{
			baseShape: &baseShape{
				Type: CODE_TYPE,
				Box:  box,
			},
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeCode) GetDefaultPadding() (paddingX, paddingY float64) {
	return 0, 0
}
