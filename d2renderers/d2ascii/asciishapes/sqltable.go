package asciishapes

import (
	"log/slog"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/log"
)

func DrawSQLTable(ctx *Context, x, y, w, h float64, shape d2target.Shape) {
	log.Debug(ctx.Ctx, "drawing sql table", slog.Float64("x", x), slog.Float64("y", y), slog.Float64("w", w), slog.Float64("h", h), slog.String("label", shape.Label))

	x1, y1, wC, hC := ctx.Calibrate(x, y, w, h)
	x2, y2 := x1+wC, y1+hC

	DrawBorder(ctx, x1, y1, x2, y2)

	if len(shape.Columns) == 0 {
		DrawShapeLabel(ctx, x1, y1, x2, y2, wC, hC, shape.Label, shape.LabelPosition)
		return
	}

	currentY := y1 + 1

	if shape.Label != "" {
		currentY = DrawHeader(ctx, x1, currentY, x2, wC, shape.Label)
	}

	for _, column := range shape.Columns {
		if currentY >= y2 {
			break
		}

		DrawFieldWithType(ctx, x1, x2, currentY, wC, column.Name.Label, column.Type.Label)
		currentY++
	}
}
