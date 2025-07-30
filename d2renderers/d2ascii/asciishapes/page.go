package asciishapes

import (
	"fmt"
)

// DrawPage draws a page/document shape with folded corner
func DrawPage(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := ctx.Calibrate(x, y, w, h)
	// Adjust width for optimal label symmetry
	wi = adjustWidthForLabel(wi, label)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	x3 := x2 - wi/3
	y3 := y2 - hi/2

	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1): ctx.Chars.TopLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y1): ctx.Chars.TopRightCorner(),
		fmt.Sprintf("%d_%d", x1, y2): ctx.Chars.BottomLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y2): ctx.Chars.BottomRightCorner(),
	}

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			key := fmt.Sprintf("%d_%d", x, y)
			if val, ok := corners[key]; ok && !(x > x3 && y < y3) {
				ctx.Canvas.Set(x, y, val)
			} else if x == x1 || (x == x2 && y > y3) {
				ctx.Canvas.Set(x, y, ctx.Chars.Vertical())
			} else if (y == y1 && x < x3) || y == y2 {
				ctx.Canvas.Set(x, y, ctx.Chars.Horizontal())
			} else if (x == x3 && y == y1) || (x == x2 && y == y3) {
				ctx.Canvas.Set(x, y, ctx.Chars.TopRightCorner())
			} else if x == x3 && y == y3 {
				ctx.Canvas.Set(x, y, ctx.Chars.BottomLeftCorner())
			} else if x == x2 && y == y3 {
				ctx.Canvas.Set(x, y, ctx.Chars.TopRightCorner())
			} else if x == x3 && y < y3 {
				ctx.Canvas.Set(x, y, ctx.Chars.Vertical())
			} else if x > x3 && y == y3 {
				ctx.Canvas.Set(x, y, ctx.Chars.Horizontal())
			} else if x > x3 && x < x2 && y < y3 && y > y1 {
				ctx.Canvas.Set(x, y, ctx.Chars.Backslash())
			} else {
				ctx.Canvas.Set(x, y, " ")
			}
		}
	}

	DrawShapeLabel(ctx, x1, y1, x2, y2, wi, hi, label, labelPosition)
}