package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/util-go/go2"
)

// Table is basically a rectangle
type shapeTable struct {
	shapeSquare
}

func NewTable(box *geo.Box) Shape {
	shape := shapeTable{
		shapeSquare{
			baseShape: &baseShape{
				Type: TABLE_TYPE,
				Box:  box,
			},
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeTable) GetDefaultPadding() (paddingX, paddingY float64) {
	return 0, 0
}
