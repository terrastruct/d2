package asciishapes

import "math"

func DrawPerson(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := ctx.Calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	head := HeadHeight
	body := hi - 2
	hw := 2
	if wi%2 == 1 {
		hw = 3
	}
	hoffset := (wi - hw) / 2
	s := body - 1

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			relX, relY := x-x1, y-y1
			relXBody, relYBody := relX, relY-head

			switch {
			case y == y2:
				ctx.Canvas.Set(x, y, ctx.Chars.Overline())
			case y >= y1+head && y < y2:
				if (relX + relY) == body {
					ctx.Canvas.Set(x, y, ctx.Chars.ForwardSlash())
				} else if (float64(relXBody - relYBody - 1)) == math.Abs(float64(wi-(hi-head))) {
					ctx.Canvas.Set(x, y, ctx.Chars.Backslash())
				} else if y == y1+head && x >= x1+s && x <= x2-s {
					ctx.Canvas.Set(x, y, ctx.Chars.Overline())
				}
			case y < y1+head:
				if y == y1 && x >= x1+hoffset && x <= x2-hoffset {
					ctx.Canvas.Set(x, y, ctx.Chars.Overline())
				}
				if y == y1+head-1 && x >= x1+hoffset && x <= x2-hoffset {
					ctx.Canvas.Set(x, y, ctx.Chars.Underscore())
				}
				if (y == y1 && x == x1+hoffset-1) || (y == y1+head-1 && x == x2-hoffset+1) {
					ctx.Canvas.Set(x, y, ctx.Chars.ForwardSlash())
				}
				if (y == y1+head-1 && x == x1+hoffset-1) || (y == y1 && x == x2-hoffset+1) {
					ctx.Canvas.Set(x, y, ctx.Chars.Backslash())
				}
			}
		}
	}

	DrawShapeLabel(ctx, x1, y1, x2, y2, wi, hi, label, labelPosition)
}
