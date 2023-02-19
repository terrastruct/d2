package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/util-go/go2"
)

type shapeImage struct {
	*baseShape
}

func NewImage(box *geo.Box) Shape {
	shape := shapeImage{
		baseShape: &baseShape{
			Type: IMAGE_TYPE,
			Box:  box,
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeImage) IsRectangular() bool {
	return true
}

func (s shapeImage) GetDefaultPadding() (paddingX, paddingY float64) {
	return 0, 0
}
