package asciishapes

import (
	"math"
)

func DrawHex(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := ctx.Calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	hoffset := int(math.Ceil(float64(hi) / 2.0))

	for i := x1; i <= x2; i++ {
		for j := y1; j <= y2; j++ {
			switch {
			case j == y1 && i >= (x1+hoffset) && i <= (x2-hoffset):
				ctx.Canvas.Set(i, j, ctx.Chars.Overline())
			case j == y2 && i >= (x1+hoffset) && i <= (x2-hoffset):
				ctx.Canvas.Set(i, j, ctx.Chars.Underscore())
			case hoffset%2 == 1 && (i == x1 || i == x2) && (y1+hoffset-1) == j:
				ctx.Canvas.Set(i, j, ctx.Chars.Cross())
			case ((j-y1)+(i-x1)+1) == hoffset || ((y2-j)+(x2-i)+1) == hoffset:
				ctx.Canvas.Set(i, j, ctx.Chars.ForwardSlash())
			case ((j-y1)+(x2-i)+1) == hoffset || ((y2-j)+(i-x1)+1) == hoffset:
				ctx.Canvas.Set(i, j, ctx.Chars.Backslash())
			}
		}
	}

	DrawShapeLabel(ctx, x1, y1, x2, y2, wi, hi, label, labelPosition)
}
