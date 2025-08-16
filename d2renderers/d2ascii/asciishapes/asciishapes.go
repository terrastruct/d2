package asciishapes

import (
	"fmt"
	"math"
	"strings"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciicanvas"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
)

type Context struct {
	Canvas *asciicanvas.Canvas
	Chars  charset.Set
	FW     float64
	FH     float64
	Scale  float64
}

const (
	MinLabelPadding     = 2
	LabelOffsetX        = 2
	LabelOffsetY        = 1
	HeadHeight          = 2
	MinCylinderHeight   = 5
	MinStoredDataHeight = 5
	MaxCurveHeight      = 3
)

func (ctx *Context) Calibrate(x, y, w, h float64) (int, int, int, int) {
	xC := int(math.Round((x / ctx.FW) * ctx.Scale))
	yC := int(math.Round((y / ctx.FH) * ctx.Scale))
	wC := int(math.Round((w / ctx.FW) * ctx.Scale))
	hC := int(math.Round((h / ctx.FH) * ctx.Scale))

	fmt.Printf("\033[36m[D2ASCII-SHAPE]     Calibrate: (%.0f,%.0f) %.0fx%.0f -> (%d,%d) %dx%d [FW=%.2f, FH=%.2f, Scale=%.2f]\033[0m\n", 
		x, y, w, h, xC, yC, wC, hC, ctx.FW, ctx.FH, ctx.Scale)

	return xC, yC, wC, hC
}

func LabelY(y1, y2, h int, label, labelPosition string) int {
	ly := -1
	fmt.Printf("\033[36m[D2ASCII-SHAPE]     Label Y calculation: bounds=%d-%d, height=%d, position='%s'\033[0m\n", 
		y1, y2, h, labelPosition)
	
	if strings.Contains(labelPosition, "OUTSIDE") {
		if strings.Contains(labelPosition, "BOTTOM") {
			ly = y2 + 1
			fmt.Printf("\033[36m[D2ASCII-SHAPE]       Outside bottom: y=%d\033[0m\n", ly)
		} else if strings.Contains(labelPosition, "TOP") {
			ly = y1 - 1
			fmt.Printf("\033[36m[D2ASCII-SHAPE]       Outside top: y=%d\033[0m\n", ly)
		}
	} else {
		if strings.Contains(labelPosition, "TOP") {
			ly = y1 + 1
			fmt.Printf("\033[36m[D2ASCII-SHAPE]       Inside top: y=%d\033[0m\n", ly)
		} else if strings.Contains(labelPosition, "MIDDLE") {
			ly = y1 + h/2
			fmt.Printf("\033[36m[D2ASCII-SHAPE]       Inside middle: y=%d\033[0m\n", ly)
		} else if strings.Contains(labelPosition, "BOTTOM") {
			ly = y2 - 1
			fmt.Printf("\033[36m[D2ASCII-SHAPE]       Inside bottom: y=%d\033[0m\n", ly)
		}
	}
	return ly
}

func DrawShapeLabel(ctx *Context, x1, y1, x2, y2, width, height int, label, labelPosition string) {
	if label == "" {
		fmt.Printf("\033[36m[D2ASCII-SHAPE]     No label to draw\033[0m\n")
		return
	}
	fmt.Printf("\033[36m[D2ASCII-SHAPE]     Drawing label '%s' in bounds (%d,%d)-(%d,%d) [%dx%d]\033[0m\n", 
		label, x1, y1, x2, y2, width, height)
	
	ly := LabelY(y1, y2, height, label, labelPosition)
	lx := x1 + (width-len(label))/2
	fmt.Printf("\033[36m[D2ASCII-SHAPE]     Label position calculated: (%d, %d)\033[0m\n", lx, ly)
	ctx.Canvas.DrawLabel(lx, ly, label)
}

func adjustWidthForLabel(ctx *Context, x, y, w, h float64, width int, label string) int {
	if label == "" {
		fmt.Printf("\033[36m[D2ASCII-SHAPE]     No label, keeping width: %d\033[0m\n", width)
		return width
	}

	originalWidth := width
	availableSpace := width - len(label)
	fmt.Printf("\033[36m[D2ASCII-SHAPE]     Width adjustment for label '%s' (%d chars): width=%d, available=%d\033[0m\n", 
		label, len(label), width, availableSpace)
	
	if availableSpace < MinLabelPadding {
		width = len(label) + MinLabelPadding
		fmt.Printf("\033[36m[D2ASCII-SHAPE]     Insufficient space, expanding: %d -> %d (min padding=%d)\033[0m\n", 
			originalWidth, width, MinLabelPadding)
		return width
	}

	if availableSpace%2 == 1 {
		width = width - 1
		fmt.Printf("\033[36m[D2ASCII-SHAPE]     Odd spacing, adjusting for centering: %d -> %d\033[0m\n", 
			originalWidth, width)
		return width
	}

	fmt.Printf("\033[36m[D2ASCII-SHAPE]     Width unchanged: %d\033[0m\n", width)
	return width
}
