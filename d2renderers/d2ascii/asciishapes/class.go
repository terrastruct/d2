package asciishapes

import (
	"fmt"
	"log/slog"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/log"
)

func DrawClass(ctx *Context, x, y, w, h float64, shape d2target.Shape) {
	log.Debug(ctx.Ctx, "drawing class", slog.Float64("x", x), slog.Float64("y", y), slog.Float64("w", w), slog.Float64("h", h), slog.String("label", shape.Label))

	x1, y1, wC, hC := ctx.Calibrate(x, y, w, h)
	x2, y2 := x1+wC, y1+hC

	DrawBorder(ctx, x1, y1, x2, y2)

	if len(shape.Fields) == 0 && len(shape.Methods) == 0 {
		DrawShapeLabel(ctx, x1, y1, x2, y2, wC, hC, shape.Label, shape.LabelPosition)
		return
	}

	currentY := y1 + 1

	if shape.Label != "" {
		currentY = DrawHeader(ctx, x1, currentY, x2, wC, shape.Label)
	}

	if len(shape.Fields) > 0 {
		for _, field := range shape.Fields {
			if currentY >= y2 {
				break
			}

			fieldName := fmt.Sprintf("%s%s", field.VisibilityToken(), field.Name)
			DrawFieldWithType(ctx, x1, x2, currentY, wC, fieldName, field.Type)
			currentY++
		}

		if len(shape.Methods) > 0 && currentY < y2 {
			DrawSeparatorLine(ctx, x1, x2, currentY)
			currentY++
		}
	}

	for _, method := range shape.Methods {
		if currentY >= y2 {
			break
		}

		methodName := fmt.Sprintf("%s%s", method.VisibilityToken(), method.Name)
		DrawFieldWithType(ctx, x1, x2, currentY, wC, methodName, method.Return)
		currentY++
	}
}
