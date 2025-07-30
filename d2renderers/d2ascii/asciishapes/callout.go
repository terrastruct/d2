package asciishapes

import "fmt"

// DrawCallout draws a callout shape
func DrawCallout(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	ix, iy, iw, ih := ctx.Calibrate(x, y, w, h)
	x1, y1, x2, y2 := ix, iy, ix+iw-1, iy+ih-1
	body := (ih + 1) / 2
	tail := ih / 2

	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1):      ctx.Chars.TopLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y1):      ctx.Chars.TopRightCorner(),
		fmt.Sprintf("%d_%d", x1, y2-tail): ctx.Chars.BottomLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y2-tail): ctx.Chars.BottomRightCorner(),
	}

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			relX, relY := x-x1, y-y1
			k := fmt.Sprintf("%d_%d", x, y)
			if char, ok := corners[k]; ok {
				ctx.Canvas.Set(x, y, char)
			} else if (y == y1 || y == y2-tail) && x > x1 && x < x2 {
				ctx.Canvas.Set(x, y, ctx.Chars.Horizontal())
			} else if (x == x1 || x == x2) && y > y1 && y < y2-tail {
				ctx.Canvas.Set(x, y, ctx.Chars.Vertical())
			} else if x == x2-(tail+2) && y > y2-tail {
				ctx.Canvas.Set(x, y, ctx.Chars.Vertical())
			} else if y > y2-tail && relX+relY == iw {
				ctx.Canvas.Set(x, y, ctx.Chars.ForwardSlash())
			}
		}
	}

	if label != "" {
		ly := LabelY(y1, y2, body, label, labelPosition)
		lx := x1 + (iw-len(label))/2
		ctx.Canvas.DrawLabel(lx, ly, label)
	}
}