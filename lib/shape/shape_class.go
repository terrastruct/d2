package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
)

// Class is basically a rectangle
type shapeClass struct {
	shapeSquare
}

func NewClass(box *geo.Box) Shape {
	return shapeClass{
		shapeSquare{
			baseShape: &baseShape{
				Type: CLASS_TYPE,
				Box:  box,
			},
		},
	}
}

func (s shapeClass) GetDefaultPadding() (paddingX, paddingY float64) {
	return 0, 0
}
