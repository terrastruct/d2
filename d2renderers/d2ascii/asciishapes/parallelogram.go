package asciishapes

// DrawParallelogram draws a parallelogram shape
func DrawParallelogram(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := ctx.Calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1

	for ix := x1; ix <= x2; ix++ {
		for iy := y1; iy <= y2; iy++ {
			_x, _y := ix-x1, iy-y1
			if (_x+_y == hi-1) || (_x+_y == wi-1) {
				ctx.Canvas.Set(ix, iy, ctx.Chars.ForwardSlash())
			} else if iy == y1 && ix >= x1+hi && ix < x2 {
				ctx.Canvas.Set(ix, iy, ctx.Chars.Overline())
			} else if iy == y2 && ix > x1 && ix <= x2-hi {
				ctx.Canvas.Set(ix, iy, ctx.Chars.Underscore())
			}
		}
	}

	DrawShapeLabel(ctx, x1, y1, x2, y2, wi, hi, label, labelPosition)
}