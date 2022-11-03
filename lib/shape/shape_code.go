package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
)

type shapeCode struct {
	shapeSquare
}

func NewCode(box *geo.Box) Shape {
	return shapeCode{
		shapeSquare: shapeSquare{
			baseShape: &baseShape{
				Type: CODE_TYPE,
				Box:  box,
			},
		},
	}
}
