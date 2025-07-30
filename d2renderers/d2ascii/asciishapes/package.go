package asciishapes

import "fmt"

// DrawPackage draws a package shape
func DrawPackage(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := ctx.Calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	x3, y3 := x1+wi/2, y1+1

	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1): ctx.Chars.TopLeftCorner(),
		fmt.Sprintf("%d_%d", x3, y1): ctx.Chars.TopRightCorner(),
		fmt.Sprintf("%d_%d", x2, y3): ctx.Chars.TopRightCorner(),
		fmt.Sprintf("%d_%d", x3, y3): ctx.Chars.BottomLeftCorner(),
		fmt.Sprintf("%d_%d", x1, y2): ctx.Chars.BottomLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y2): ctx.Chars.BottomRightCorner(),
	}

	for ix := x1; ix <= x2; ix++ {
		for iy := y1; iy <= y2; iy++ {
			key := fmt.Sprintf("%d_%d", ix, iy)
			if char, ok := corners[key]; ok {
				ctx.Canvas.Set(ix, iy, char)
			} else if (iy == y1 && ix > x1 && ix < x3) || (iy == y2 && ix > x1 && ix < x2) || (iy == y3 && ix > x3 && ix < x2) {
				ctx.Canvas.Set(ix, iy, ctx.Chars.Horizontal())
			} else if (ix == x1 && iy > y1 && iy < y2) || (ix == x2 && iy > y3 && iy < y2) {
				ctx.Canvas.Set(ix, iy, ctx.Chars.Vertical())
			}
		}
	}

	DrawShapeLabel(ctx, x1, y1, x2, y2, wi, hi, label, labelPosition)
}