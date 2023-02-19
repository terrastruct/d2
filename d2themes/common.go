package d2themes

import (
	"oss.terrastruct.com/d2/d2target"
)

func ShapeTheme(shape d2target.Shape) (fill, stroke string) {
	if shape.Type == d2target.ShapeSQLTable || shape.Type == d2target.ShapeClass {
		// Fill is used for header fill in these types
		// This fill property is just background of rows
		fill = shape.Stroke
		// Stroke (border) of these shapes should match the header fill
		stroke = shape.Fill
	} else {
		fill = shape.Fill
		stroke = shape.Stroke
	}
	return fill, stroke
}

func ConnectionTheme(connection d2target.Connection) (stroke string) {
	return connection.Stroke
}
