package asciishapes

import "math"

func DrawDiamond(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	ix, iy, iw, ih := ctx.Calibrate(x, y, w, h)
	if ih%2 == 0 {
		ih++
	}
	if iw%2 == 0 {
		iw++
	}
	x1, y1, x2, y2 := ix, iy, ix+iw-1, iy+ih-1

	diagPath := [][2]int{
		{x1, y1 + ih/2},
		{x1 + iw/2, y1},
		{x2, y1 + ih/2},
		{x1 + iw/2, y2},
		{x1, y1 + ih/2},
	}

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			relX, relY := x-x1, y-y1
			if (y == y1 || y == y2) && relX == iw/2 {
				ctx.Canvas.Set(x, y, ctx.Chars.Tilde())
			} else if (x == x1 || x == x2) && relY == ih/2 {
				ctx.Canvas.Set(x, y, ctx.Chars.Hyphen())
			}
		}
	}

	for i := 0; i < len(diagPath)-1; i++ {
		a, c := diagPath[i], diagPath[i+1]
		dx, dy := c[0]-a[0], c[1]-a[1]
		step := max(absInt(dx), absInt(dy))
		sx, sy := float64(dx)/float64(step), float64(dy)/float64(step)
		fx, fy := float64(a[0]), float64(a[1])
		for j := 0; j < step; j++ {
			fx += sx
			fy += sy
			x := int(math.Round(fx))
			y := int(math.Round(fy))
			ctx.Canvas.Set(x, y, ctx.Chars.Star())
		}
	}

	if label != "" {
		ly := LabelY(ctx.Ctx, y1, y2, ih, label, labelPosition)
		lx := x1 + (iw-len(label))/2
		ctx.Canvas.DrawLabel(lx, ly, label)
	}
}
