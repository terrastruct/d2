package asciishapes

import (
	"context"
	"log/slog"
	"math"
	"strings"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciicanvas"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
	"oss.terrastruct.com/d2/lib/log"
)

type Context struct {
	Canvas *asciicanvas.Canvas
	Chars  charset.Set
	FW     float64
	FH     float64
	Scale  float64
	Ctx    context.Context
}

const (
	MinLabelPadding     = 2
	LabelOffsetX        = 2
	LabelOffsetY        = 1
	HeadHeight          = 2
	MinCylinderHeight   = 5
	MinStoredDataHeight = 5
	MinPersonHeight     = 5
	MaxCurveHeight      = 3
)

func (ctx *Context) Calibrate(x, y, w, h float64) (int, int, int, int) {
	xC := int(math.Round((x / ctx.FW) * ctx.Scale))
	yC := int(math.Round((y / ctx.FH) * ctx.Scale))
	wC := int(math.Round((w / ctx.FW) * ctx.Scale))
	hC := int(math.Round((h / ctx.FH) * ctx.Scale))

	log.Debug(ctx.Ctx, "calibrate", slog.Float64("origX", x), slog.Float64("origY", y), slog.Float64("origW", w), slog.Float64("origH", h), slog.Int("x", xC), slog.Int("y", yC), slog.Int("w", wC), slog.Int("h", hC), slog.Float64("FW", ctx.FW), slog.Float64("FH", ctx.FH), slog.Float64("Scale", ctx.Scale))

	return xC, yC, wC, hC
}

func LabelY(ctx context.Context, y1, y2, h int, label, labelPosition string) int {
	ly := -1
	log.Debug(ctx, "label Y calculation", slog.Int("y1", y1), slog.Int("y2", y2), slog.Int("height", h), slog.String("position", labelPosition))

	if strings.Contains(labelPosition, "OUTSIDE") {
		if strings.Contains(labelPosition, "BOTTOM") {
			ly = y2 + 1
			log.Debug(ctx, "label position outside bottom", slog.Int("y", ly))
		} else if strings.Contains(labelPosition, "TOP") {
			ly = y1 - 1
			log.Debug(ctx, "label position outside top", slog.Int("y", ly))
		}
	} else {
		if strings.Contains(labelPosition, "TOP") {
			ly = y1 + 1
			log.Debug(ctx, "label position inside top", slog.Int("y", ly))
		} else if strings.Contains(labelPosition, "MIDDLE") {
			ly = y1 + h/2
			log.Debug(ctx, "label position inside middle", slog.Int("y", ly))
		} else if strings.Contains(labelPosition, "BOTTOM") {
			ly = y2 - 1
			log.Debug(ctx, "label position inside bottom", slog.Int("y", ly))
		}
	}
	return ly
}

func DrawShapeLabel(ctx *Context, x1, y1, x2, y2, width, height int, label, labelPosition string) {
	if label == "" {
		log.Debug(ctx.Ctx, "no label to draw")
		return
	}
	log.Debug(ctx.Ctx, "drawing shape label", slog.String("label", label), slog.Int("x1", x1), slog.Int("y1", y1), slog.Int("x2", x2), slog.Int("y2", y2), slog.Int("width", width), slog.Int("height", height))

	ly := LabelY(ctx.Ctx, y1, y2, height, label, labelPosition)

	lines := strings.Split(label, "\n")
	maxLineLen := 0
	for _, line := range lines {
		if len(line) > maxLineLen {
			maxLineLen = len(line)
		}
	}

	lx := x1 + (width-maxLineLen)/2
	log.Debug(ctx.Ctx, "label position calculated", slog.Int("x", lx), slog.Int("y", ly))
	ctx.Canvas.DrawLabel(lx, ly, label)
}

func AdjustWidthForLabel(ctx *Context, x, y, w, h float64, width int, label string) int {
	if label == "" {
		log.Debug(ctx.Ctx, "no label, keeping width", slog.Int("width", width))
		return width
	}

	originalWidth := width
	availableSpace := width - len(label)
	log.Debug(ctx.Ctx, "width adjustment for label", slog.String("label", label), slog.Int("chars", len(label)), slog.Int("width", width), slog.Int("available", availableSpace))

	if availableSpace < MinLabelPadding {
		width = len(label) + MinLabelPadding
		log.Debug(ctx.Ctx, "insufficient space, expanding width", slog.Int("original", originalWidth), slog.Int("new", width), slog.Int("minPadding", MinLabelPadding))
		return width
	}

	if availableSpace%2 == 1 {
		width = width - 1
		log.Debug(ctx.Ctx, "odd spacing, adjusting for centering", slog.Int("original", originalWidth), slog.Int("new", width))
		return width
	}

	log.Debug(ctx.Ctx, "width unchanged", slog.Int("width", width))
	return width
}
