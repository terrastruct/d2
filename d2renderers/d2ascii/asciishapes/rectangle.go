package asciishapes

import (
	"fmt"
	"log/slog"
	"strings"

	"oss.terrastruct.com/d2/lib/log"
)

func DrawRect(ctx *Context, x, y, w, h float64, label, labelPosition, symbol string, preserveHeight ...bool) {
	log.Debug(ctx.Ctx, "drawing rectangle", slog.Float64("x", x), slog.Float64("y", y), slog.Float64("w", w), slog.Float64("h", h), slog.String("label", label), slog.String("symbol", symbol))

	x1, y1, wC, hC := ctx.Calibrate(x, y, w, h)
	originalHC := hC
	shouldPreserveHeight := len(preserveHeight) > 0 && preserveHeight[0]
	if label != "" && hC%2 == 0 && !shouldPreserveHeight {
		if hC > 2 {
			hC--
			y1++
			log.Debug(ctx.Ctx, "height adjustment for label centering", slog.Int("original", originalHC), slog.Int("new", hC), slog.Int("oldY1", y1-1), slog.Int("newY1", y1))
		} else {
			hC++
			log.Debug(ctx.Ctx, "height expanded for small shape", slog.Int("original", originalHC), slog.Int("new", hC))
		}
	}
	wC = AdjustWidthForLabel(ctx, x, y, w, h, wC, label)
	x2, y2 := x1+wC, y1+hC
	log.Debug(ctx.Ctx, "final draw bounds", slog.Int("x1", x1), slog.Int("y1", y1), slog.Int("x2", x2), slog.Int("y2", y2), slog.Int("w", wC), slog.Int("h", hC))
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
	log.Debug(ctx.Ctx, "drew border characters", slog.Int("count", charsDrawn))

	DrawShapeLabel(ctx, x1, y1, x2, y2, wC, hC, label, labelPosition)
}
