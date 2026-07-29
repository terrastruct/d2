package asciishapes

import (
	"math"
)

func DrawStoredData(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := ctx.Calibrate(x, y, w, h)
	if hi < MinStoredDataHeight {
		hi = MinStoredDataHeight
	} else if hi%2 == 0 {
		hi++
	}
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	hoffset := (hi + 1) / 2

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			relX, relY := x-x1, y-y1

			switch {
			case y == y1+hoffset-1 && x == x1:
				ctx.Canvas.Set(x, y, ctx.Chars.Vertical())
			case x < x1+hoffset:
				if y < y1+hoffset && (relX+relY) == hoffset-1 {
					ctx.Canvas.Set(x, y, ctx.Chars.ForwardSlash())
				} else if y >= y1+hoffset && int(math.Abs(float64(relX-relY))) == hoffset-1 {
					ctx.Canvas.Set(x, y, ctx.Chars.Backslash())
				}
			case x >= x1+hoffset:
				if y == y1 && x < x2 {
					ctx.Canvas.Set(x, y, ctx.Chars.Overline())
				} else if y == y2 && x < x2 {
					ctx.Canvas.Set(x, y, ctx.Chars.Underscore())
				} else if x > x2-hoffset {
					if y == y1+hoffset-1 && x == x2-(hoffset-1) {
						ctx.Canvas.Set(x, y, ctx.Chars.Vertical())
					} else if (relX + relY) == wi-1 {
						ctx.Canvas.Set(x, y, ctx.Chars.ForwardSlash())
					} else if int(math.Abs(float64(relX-relY))) == int(math.Abs(float64(wi-hi))) {
						ctx.Canvas.Set(x, y, ctx.Chars.Backslash())
					}
				}
			}
		}
	}

	DrawShapeLabel(ctx, x1, y1, x2, y2, wi, hi, label, labelPosition)
}
