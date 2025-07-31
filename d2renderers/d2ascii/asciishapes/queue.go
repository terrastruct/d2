package asciishapes

func DrawQueue(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := ctx.Calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1

	for ix := x1; ix <= x2; ix++ {
		for iy := y1; iy <= y2; iy++ {
			switch {
			case (iy == y1 && (ix == x1+1 || ix == x2-2)) || (iy == y2 && ix == x2-1):
				ctx.Canvas.Set(ix, iy, ctx.Chars.ForwardSlash())
			case (iy == y1 && ix == x2-1) || (iy == y2 && (ix == x1+1 || ix == x2-2)):
				ctx.Canvas.Set(ix, iy, ctx.Chars.Backslash())
			case (ix == x1 || ix == x2 || ix == x2-3) && (iy > y1 && iy < y2):
				ctx.Canvas.Set(ix, iy, ctx.Chars.Vertical())
			case iy == y1 && ix > x1+1 && ix < x2-1:
				ctx.Canvas.Set(ix, iy, ctx.Chars.Overline())
			case iy == y2 && ix > x1+1 && ix < x2-3:
				ctx.Canvas.Set(ix, iy, ctx.Chars.Underscore())
			}
		}
	}

	DrawShapeLabel(ctx, x1, y1, x2, y2, wi, hi, label, labelPosition)
}
