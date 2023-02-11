package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/util-go/go2"
)

// Text is basically a rectangle
type shapeText struct {
	shapeSquare
}

func NewText(box *geo.Box) Shape {
	shape := shapeText{
		shapeSquare: shapeSquare{
			baseShape: &baseShape{
				Type: TEXT_TYPE,
				Box:  box,
			},
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeText) GetDefaultPadding() (paddingX, paddingY float64) {
	return 0, 0
}
