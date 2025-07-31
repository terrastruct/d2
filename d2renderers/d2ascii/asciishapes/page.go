package asciishapes

import (
	"fmt"
)

func DrawPage(ctx *Context, x, y, w, h float64, label, labelPosition string) {

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

	for xi := x1; xi < x2; xi++ {
		for yi := y1; yi < y2; yi++ {
			key := fmt.Sprintf("%d_%d", xi, yi)
			if val, ok := corners[key]; ok {
				ctx.Canvas.Set(xi, yi, val)
			} else if xi == x1 || xi == x2-1 {
				ctx.Canvas.Set(xi, yi, ctx.Chars.Vertical())
			} else if yi == y1 || yi == y2-1 {
				ctx.Canvas.Set(xi, yi, ctx.Chars.Horizontal())
			}
		}
	}
	// The fold
	ctx.Canvas.Set(x2-1, y1, " ")
	ctx.Canvas.Set(x2-2, y1, ctx.Chars.TopRightCorner())
	ctx.Canvas.Set(x2-2, y1+1, ctx.Chars.Backslash())
	ctx.Canvas.Set(x2-1, y1+1, ctx.Chars.TopRightCorner())

	DrawShapeLabel(ctx, x1, y1, x2, y2, wC, hC, label, labelPosition)

}
