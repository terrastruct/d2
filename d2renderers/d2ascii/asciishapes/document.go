package asciishapes

import "fmt"

// DrawDocument draws a document shape with wavy bottom
func DrawDocument(ctx *Context, x, y, w, h float64, label, labelPosition string) {
	ix, iy, iw, ih := ctx.Calibrate(x, y, w, h)
	x1, y1, x2, y2 := ix, iy, ix+iw-1, iy+ih-1
	n := (iw - 2) / 2
	j := n / 2
	if j > MaxCurveHeight {
		j = MaxCurveHeight
	}
	hcurve := j + 1

	bcurve := "`-._"
	tcurve := ".-`‾"
	
	lcurve := make([]rune, n)
	rcurve := make([]rune, n)
	for i := 0; i < n; i++ {
		if i < hcurve {
			lcurve[i] = rune(bcurve[i])
			rcurve[i] = rune(tcurve[i])
		} else if absInt(i-n+1) < hcurve {
			lcurve[i] = rune(bcurve[absInt(i-n+1)])
			rcurve[i] = rune(tcurve[absInt(i-n+1)])
		} else {
			lcurve[i] = rune(bcurve[3])
			rcurve[i] = rune(tcurve[3])
		}
	}
	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1): ctx.Chars.TopLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y1): ctx.Chars.TopRightCorner(),
	}

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			relX := x - x1
			curveIndex := relX - 1
			k := fmt.Sprintf("%d_%d", x, y)
			if char, ok := corners[k]; ok {
				ctx.Canvas.Set(x, y, char)
			} else if y == y1 && x > x1 && x < x2 {
				ctx.Canvas.Set(x, y, ctx.Chars.Horizontal())
			} else if (x == x1 || x == x2) && y > y1 && y < y2 {
				ctx.Canvas.Set(x, y, ctx.Chars.Vertical())
			} else if y == y2 && x > x1 && relX <= n && curveIndex >= 0 && curveIndex < len(lcurve) {
				ctx.Canvas.Set(x, y, string(lcurve[curveIndex]))
			} else if y == y2-1 && relX > n && x < x2 && (relX-int(iw/2)) < len(rcurve) {
				ctx.Canvas.Set(x, y, string(rcurve[relX-int(iw/2)]))
			}
		}
	}

	if label != "" {
		ly := LabelY(y1, y2, ih-2, label, labelPosition)
		lx := x1 + (iw-len(label))/2
		ctx.Canvas.DrawLabel(lx, ly, label)
	}
}