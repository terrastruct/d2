package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
)

type shapeImage struct {
	*baseShape
}

func NewImage(box *geo.Box) Shape {
	return shapeImage{
		baseShape: &baseShape{
			Type: IMAGE_TYPE,
			Box:  box,
		},
	}
}

func (s shapeImage) IsRectangular() bool {
	return true
}

func (s shapeImage) GetDefaultPadding() (paddingX, paddingY float64) {
	return 0, 0
}
