package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
)

// Text is basically a rectangle
type shapeText struct {
	shapeSquare
}

func NewText(box *geo.Box) Shape {
	return shapeText{
		shapeSquare: shapeSquare{
			baseShape: &baseShape{
				Type: TEXT_TYPE,
				Box:  box,
			},
		},
	}
}
