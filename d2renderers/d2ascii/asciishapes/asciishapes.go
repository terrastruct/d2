package asciishapes

import (
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

	return xC, yC, wC, hC
}

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

func DrawShapeLabel(ctx *Context, x1, y1, x2, y2, width, height int, label, labelPosition string) {
	if label == "" {
		return
	}
	ly := LabelY(y1, y2, height, label, labelPosition)
	lx := x1 + (width-len(label))/2
	ctx.Canvas.DrawLabel(lx, ly, label)
}

func adjustWidthForLabel(ctx *Context, x, y, w, h float64, width int, label string) int {
	if label == "" {
		return width
	}

	availableSpace := width - len(label)
	if availableSpace < MinLabelPadding {
		return len(label) + MinLabelPadding
	}

	if availableSpace%2 == 1 {
		return width - 1
	}

	return width
}
