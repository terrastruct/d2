package asciishapes

import (
	"fmt"
)

func DrawRect(ctx *Context, x, y, w, h float64, label, labelPosition, symbol string) {
	x1, y1, wC, hC := ctx.Calibrate(x, y, w, h)
	if label != "" && hC%2 == 0 {
		if hC > 2 {
			hC--
			y1++
		} else {
			hC++
		}
	}
	wC = adjustWidthForLabel(ctx, x, y, w, h, wC, label)
	x2, y2 := x1+wC, y1+hC
	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1):     ctx.Chars.TopLeftCorner(),
		fmt.Sprintf("%d_%d", x2-1, y1):   ctx.Chars.TopRightCorner(),
		fmt.Sprintf("%d_%d", x1, y2-1):   ctx.Chars.BottomLeftCorner(),
		fmt.Sprintf("%d_%d", x2-1, y2-1): ctx.Chars.BottomRightCorner(),
	}
	FillRectangle(ctx, x1, y1, x2, y2, corners, symbol)
	DrawShapeLabel(ctx, x1, y1, x2, y2, wC, hC, label, labelPosition)
}
