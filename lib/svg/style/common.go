package style

import (
	"fmt"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/svg"
)

func ShapeStyle(shape d2target.Shape) string {
	out := ""

	out += fmt.Sprintf(`opacity:%f;`, shape.Opacity)
	out += fmt.Sprintf(`stroke-width:%d;`, shape.StrokeWidth)
	if shape.StrokeDash != 0 {
		dashSize, gapSize := svg.GetStrokeDashAttributes(float64(shape.StrokeWidth), shape.StrokeDash)
		out += fmt.Sprintf(`stroke-dasharray:%f,%f;`, dashSize, gapSize)
	}

	return out
}

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

func ConnectionStyle(connection d2target.Connection) string {
	out := ""

	out += fmt.Sprintf(`opacity:%f;`, connection.Opacity)
	out += fmt.Sprintf(`stroke-width:%d;`, connection.StrokeWidth)
	if connection.StrokeDash != 0 {
		dashSize, gapSize := svg.GetStrokeDashAttributes(float64(connection.StrokeWidth), connection.StrokeDash)
		out += fmt.Sprintf(`stroke-dasharray:%f,%f;`, dashSize, gapSize)
	}

	return out
}

func ConnectionTheme(connection d2target.Connection) (stroke string) {
	return connection.Stroke
}
