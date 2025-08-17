package asciishapes

import (
	"fmt"
	"strings"
)

func DrawRect(ctx *Context, x, y, w, h float64, label, labelPosition, symbol string, preserveHeight ...bool) {
	fmt.Printf("\033[36m[D2ASCII-SHAPE]   DrawRect: (%.0f,%.0f) %.0fx%.0f, label='%s', symbol='%s'\033[0m\n",
		x, y, w, h, label, symbol)

	x1, y1, wC, hC := ctx.Calibrate(x, y, w, h)
	originalHC := hC
	shouldPreserveHeight := len(preserveHeight) > 0 && preserveHeight[0]
	if label != "" && hC%2 == 0 && !shouldPreserveHeight {
		if hC > 2 {
			hC--
			y1++
			fmt.Printf("\033[36m[D2ASCII-SHAPE]     Height adjustment for label centering: %d -> %d, y1: %d -> %d\033[0m\n",
				originalHC, hC, y1-1, y1)
		} else {
			hC++
			fmt.Printf("\033[36m[D2ASCII-SHAPE]     Height expanded for small shape: %d -> %d\033[0m\n",
				originalHC, hC)
		}
	}
	wC = AdjustWidthForLabel(ctx, x, y, w, h, wC, label)
	x2, y2 := x1+wC, y1+hC
	fmt.Printf("\033[36m[D2ASCII-SHAPE]     Final draw bounds: (%d,%d) to (%d,%d) [%dx%d] (actual shape area)\033[0m\n",
		x1, y1, x2, y2, wC, hC)
	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1): ctx.Chars.TopLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y1): ctx.Chars.TopRightCorner(),
		fmt.Sprintf("%d_%d", x1, y2): ctx.Chars.BottomLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y2): ctx.Chars.BottomRightCorner(),
	}

	charsDrawn := 0
	for xi := x1; xi <= x2; xi++ {
		for yi := y1; yi <= y2; yi++ {
			key := fmt.Sprintf("%d_%d", xi, yi)
			if val, ok := corners[key]; ok {
				ctx.Canvas.Set(xi, yi, val)
			} else if strings.TrimSpace(symbol) != "" && yi == y1 && xi == x1+1 {
				ctx.Canvas.Set(xi, yi, symbol)
			} else if xi == x1 || xi == x2 {
				ctx.Canvas.Set(xi, yi, ctx.Chars.Vertical())
			} else if yi == y1 || yi == y2 {
				ctx.Canvas.Set(xi, yi, ctx.Chars.Horizontal())
			} else {
				continue
			}
			charsDrawn++
		}
	}
	fmt.Printf("\033[36m[D2ASCII-SHAPE]     Drew %d border characters\033[0m\n", charsDrawn)

	DrawShapeLabel(ctx, x1, y1, x2, y2, wC, hC, label, labelPosition)
}
