package shape

import (
	"github.com/d2lang/util-go/go2"
	"oss.terrastruct.com/d2/lib/geo"
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
