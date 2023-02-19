package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/util-go/go2"
)

// Class is basically a rectangle
type shapeClass struct {
	shapeSquare
}

func NewClass(box *geo.Box) Shape {
	shape := shapeClass{
		shapeSquare{
			baseShape: &baseShape{
				Type: CLASS_TYPE,
				Box:  box,
			},
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeClass) GetDefaultPadding() (paddingX, paddingY float64) {
	return 0, 0
}
