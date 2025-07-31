package asciishapes

// DrawCylinder draws a cylinder shape
func DrawCylinder(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := ctx.Calibrate(x, y, w, h)
	// Adjust width for optimal label symmetry
	wi = adjustWidthForLabel(ctx, x, y, w, h, wi, label)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1

	for ix := x1; ix <= x2; ix++ {
		for iy := y1; iy <= y2; iy++ {
			switch {
			case iy != y1 && iy != y2 && (ix == x1 || ix == x2):
				ctx.Canvas.Set(ix, iy, ctx.Chars.Vertical())
			case iy == y1 || iy == y2 || iy == y1+1:
				if iy == y1 {
					if ix == x1+1 || ix == x2-1 {
						ctx.Canvas.Set(ix, iy, ctx.Chars.Dot())
					} else if ix == x1+2 || ix == x2-2 {
						ctx.Canvas.Set(ix, iy, ctx.Chars.Hyphen())
					} else if ix > x1+2 && ix < x2-2 {
						ctx.Canvas.Set(ix, iy, ctx.Chars.Overline())
					}
				} else if iy == y2 || iy == y1+1 {
					if ix == x1+1 {
						ctx.Canvas.Set(ix, iy, ctx.Chars.Backslash())
					} else if ix == x2-1 {
						ctx.Canvas.Set(ix, iy, ctx.Chars.ForwardSlash())
					} else if ix == x1+2 || ix == x2-2 {
						ctx.Canvas.Set(ix, iy, ctx.Chars.Hyphen())
					} else if ix > x1+2 && ix < x2-2 {
						ctx.Canvas.Set(ix, iy, ctx.Chars.Underscore())
					}
				}
			}
		}
	}

	if label != "" {
		ly := LabelY(y1+1, y2, hi, label, labelPosition)
		lx := x1 + (wi-len(label))/2
		ctx.Canvas.DrawLabel(lx, ly, label)
	}
}