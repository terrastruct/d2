package asciishapes

import (
	"fmt"
)

func abs[T ~int](x T) T {
	if x < 0 {
		return -x
	}
	return x
}

func max[T ~int](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func DrawBorder(ctx *Context, x1, y1, x2, y2 int) {
	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1): ctx.Chars.TopLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y1): ctx.Chars.TopRightCorner(),
		fmt.Sprintf("%d_%d", x1, y2): ctx.Chars.BottomLeftCorner(),
		fmt.Sprintf("%d_%d", x2, y2): ctx.Chars.BottomRightCorner(),
	}

	for xi := x1; xi <= x2; xi++ {
		for yi := y1; yi <= y2; yi++ {
			key := fmt.Sprintf("%d_%d", xi, yi)
			if val, ok := corners[key]; ok {
				ctx.Canvas.Set(xi, yi, val)
			} else if xi == x1 || xi == x2 {
				ctx.Canvas.Set(xi, yi, ctx.Chars.Vertical())
			} else if yi == y1 || yi == y2 {
				ctx.Canvas.Set(xi, yi, ctx.Chars.Horizontal())
			}
		}
	}
}

func DrawHeader(ctx *Context, x1, y1, x2, wC int, label string) int {
	headerHeight := 2
	headerY2 := y1 + headerHeight - 1

	for yi := y1; yi <= headerY2; yi++ {
		for xi := x1 + 1; xi < x2; xi++ {
			ctx.Canvas.Set(xi, yi, " ")
		}
	}

	labelX := x1 + (wC-len(label))/2
	ctx.Canvas.DrawLabel(labelX, y1, label)

	currentY := headerY2 + 1
	for xi := x1; xi <= x2; xi++ {
		if xi == x1 {
			ctx.Canvas.Set(xi, currentY, ctx.Chars.TRight())
		} else if xi == x2 {
			ctx.Canvas.Set(xi, currentY, ctx.Chars.TLeft())
		} else {
			ctx.Canvas.Set(xi, currentY, ctx.Chars.Horizontal())
		}
	}

	return currentY + 1
}

func DrawSeparatorLine(ctx *Context, x1, x2, y int) {
	for xi := x1; xi <= x2; xi++ {
		if xi == x1 {
			ctx.Canvas.Set(xi, y, ctx.Chars.TRight())
		} else if xi == x2 {
			ctx.Canvas.Set(xi, y, ctx.Chars.TLeft())
		} else {
			ctx.Canvas.Set(xi, y, ctx.Chars.Horizontal())
		}
	}
}

func DrawFieldWithType(ctx *Context, x1, x2, y, wC int, name, fieldType string) {
	ctx.Canvas.DrawLabel(x1+1, y, name)

	if fieldType != "" {
		maxTypeWidth := wC - len(name) - 4
		if len(fieldType) > maxTypeWidth && maxTypeWidth > 3 {
			fieldType = fieldType[:maxTypeWidth-3] + "..."
		}
		typeX := x2 - len(fieldType) - 1
		ctx.Canvas.DrawLabel(typeX, y, fieldType)
	}
}
