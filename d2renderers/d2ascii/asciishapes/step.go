package asciishapes

func DrawStep(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	ix, iy, iw, ih := ctx.Calibrate(x, y, w, h)
	if ih%2 == 1 {
		ih++
	}
	x1, y1, x2, y2 := ix, iy, ix+iw-1, iy+ih-1

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			_x, _y := x-x1, y-y1
			if (x < x1+ih/2 && _x-_y == 0) || (x > x2-ih/2 && absInt(_x-_y) == iw-ih/2) {
				ctx.Canvas.Set(x, y, ctx.Chars.Backslash())
			} else if (x < x1+ih/2 && _x+_y == ih-1) || (x > x2-ih/2 && _x+_y == iw-1+ih/2) {
				ctx.Canvas.Set(x, y, ctx.Chars.ForwardSlash())
			} else if y == y1 && x > x1 && x < x2-ih/2 {
				ctx.Canvas.Set(x, y, ctx.Chars.Overline())
			} else if y == y2 && x > x1 && x < x2-ih/2 {
				ctx.Canvas.Set(x, y, ctx.Chars.Underscore())
			}
		}
	}

	if label != "" {
		ly := LabelY(ctx.Ctx, y1, y2, ih, label, labelPosition)
		lx := x1 + (iw-len(label))/2
		ctx.Canvas.DrawLabel(lx, ly, label)
	}
}
