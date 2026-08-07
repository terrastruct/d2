package shape

import (
	"github.com/d2lang/util-go/go2"
	"oss.terrastruct.com/d2/lib/geo"
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
