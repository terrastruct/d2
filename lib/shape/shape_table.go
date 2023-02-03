package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
)

// Table is basically a rectangle
type shapeTable struct {
	shapeSquare
}

func NewTable(box *geo.Box) Shape {
	return shapeTable{
		shapeSquare{
			baseShape: &baseShape{
				Type: TABLE_TYPE,
				Box:  box,
			},
		},
	}
}

func (s shapeTable) GetDefaultPadding() (paddingX, paddingY float64) {
	return 0, 0
}
