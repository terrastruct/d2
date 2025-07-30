package asciishapes

import (
	"fmt"
)

// DrawRect draws a rectangle shape
func DrawRect(ctx *Context, x, y, w, h float64, label, labelPosition, symbol string) {
	x1, y1, wC, hC := ctx.Calibrate(x, y, w, h)
	if label != "" && hC%2 == 0 {
		if hC > 2 {
			hC--
			y1++
		} else {
			hC++
		}
	}
	// Adjust width for optimal label symmetry
	wC = adjustWidthForLabel(wC, label)
	x2, y2 := x1+wC, y1+hC
	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1):     ctx.Chars.TopLeftCorner(),
		fmt.Sprintf("%d_%d", x2-1, y1):   ctx.Chars.TopRightCorner(),
		fmt.Sprintf("%d_%d", x1, y2-1):   ctx.Chars.BottomLeftCorner(),
		fmt.Sprintf("%d_%d", x2-1, y2-1): ctx.Chars.BottomRightCorner(),
	}
	FillRectangle(ctx, x1, y1, x2, y2, corners, symbol)
	DrawShapeLabel(ctx, x1, y1, x2, y2, wC, hC, label, labelPosition)
}

// adjustWidthForLabel adjusts width to ensure label fits with proper symmetry
func adjustWidthForLabel(width int, label string) int {
	if label == "" {
		return width
	}
	
	availableSpace := width - len(label)
	if availableSpace < MinLabelPadding {
		return len(label) + MinLabelPadding
	}
	
	if availableSpace%2 == 1 {
		// For now, just reduce by 1 for odd spacing
		// In the full implementation, this would check for connections
		return width - 1
	}
	
	return width
}