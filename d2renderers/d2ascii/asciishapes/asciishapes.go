package asciishapes

import (
	"fmt"
	"math"
	"strings"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciicanvas"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
)

// Context provides the drawing context for shapes
type Context struct {
	Canvas *asciicanvas.Canvas
	Chars  charset.Set
	FW     float64 // Font width
	FH     float64 // Font height
	Scale  float64
}

// Constants for shape drawing
const (
	MinLabelPadding = 2
	LabelOffsetX    = 2
	LabelOffsetY    = 1
	HeadHeight      = 2
	MinCylinderHeight = 5
	MinStoredDataHeight = 5
	MaxCurveHeight = 3
)

// Calibrate converts coordinates from diagram space to canvas space
func (ctx *Context) Calibrate(x, y, w, h float64) (int, int, int, int) {
	xC := int(math.Round((x / ctx.FW) * ctx.Scale))
	yC := int(math.Round((y / ctx.FH) * ctx.Scale))
	wC := int(math.Round((w / ctx.FW) * ctx.Scale))
	hC := int(math.Round((h / ctx.FH) * ctx.Scale))
	return xC, yC, wC, hC
}

// LabelY calculates the Y position for a label
func LabelY(y1, y2, h int, label, labelPosition string) int {
	ly := -1
	if strings.Contains(labelPosition, "OUTSIDE") {
		if strings.Contains(labelPosition, "BOTTOM") {
			ly = y2 + 1
		} else if strings.Contains(labelPosition, "TOP") {
			ly = y1 - 1
		}
	} else {
		if strings.Contains(labelPosition, "TOP") {
			ly = y1 + 1
		} else if strings.Contains(labelPosition, "MIDDLE") {
			ly = y1 + h/2
		} else if strings.Contains(labelPosition, "BOTTOM") {
			ly = y2 - 1
		}
	}
	return ly
}

// DrawShapeLabel draws a centered label for a shape
func DrawShapeLabel(ctx *Context, x1, y1, x2, y2, width, height int, label, labelPosition string) {
	if label == "" {
		return
	}
	ly := LabelY(y1, y2, height, label, labelPosition)
	lx := x1 + (width-len(label))/2
	ctx.Canvas.DrawLabel(lx, ly, label)
}

// FillRectangle fills a rectangular area with appropriate border characters
func FillRectangle(ctx *Context, x1, y1, x2, y2 int, corners map[string]string, symbol string) {
	for xi := x1; xi < x2; xi++ {
		for yi := y1; yi < y2; yi++ {
			key := fmt.Sprintf("%d_%d", xi, yi)
			if val, ok := corners[key]; ok {
				ctx.Canvas.Set(xi, yi, val)
			} else if strings.TrimSpace(symbol) != "" && yi == y1 && xi == x1+1 {
				ctx.Canvas.Set(xi, yi, symbol)
			} else if xi == x1 || xi == x2-1 {
				ctx.Canvas.Set(xi, yi, ctx.Chars.Vertical())
			} else if yi == y1 || yi == y2-1 {
				ctx.Canvas.Set(xi, yi, ctx.Chars.Horizontal())
			}
		}
	}
}

// adjustWidthForLabel adjusts width to ensure label fits with proper symmetry
func adjustWidthForLabel(ctx *Context, x, y, w, h float64, width int, label string) int {
	if label == "" {
		return width
	}
	
	availableSpace := width - len(label)
	if availableSpace < MinLabelPadding {
		return len(label) + MinLabelPadding
	}
	
	if availableSpace%2 == 1 {
		// Reduce by 1 for even spacing
		return width - 1
	}
	
	return width
}