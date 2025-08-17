package asciiroute

import (
	"fmt"
	"math"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
)

func drawSegmentBetweenPoints(rd RouteDrawer, start, end *geo.Point, segmentIndex int, conn d2target.Connection,
	corners, arrows, turnDir map[string]string, frmBoundary, toBoundary Boundary, labelPos *RouteLabelPosition, label string) {

	ax, ay := start.X, start.Y
	cx, cy := end.X, end.Y

	fmt.Printf("[D2ASCII]   Drawing segment %d: (%.2f,%.2f) -> (%.2f,%.2f)\n",
		segmentIndex-1, ax, ay, cx, cy)

	sx := cx - ax
	sy := cy - ay
	step := math.Max(math.Abs(sx), math.Abs(sy))
	if step == 0 {
		fmt.Printf("[D2ASCII]   Zero-length segment, skipping\n")
		return
	}
	sx /= step
	sy /= step

	fmt.Printf("[D2ASCII]   Step vector: (%.2f, %.2f), total steps: %.0f\n", sx, sy, step)

	fx, fy := ax, ay
	attempt := 0
	x := int(math.Round(ax))
	y := int(math.Round(ay))

	for {
		attempt++
		if x == int(math.Round(cx)) && y == int(math.Round(cy)) || attempt == MaxRouteAttempts {
			if attempt == MaxRouteAttempts {
				fmt.Printf("[D2ASCII]   Max route attempts (%d) reached\n", MaxRouteAttempts)
			} else {
				fmt.Printf("[D2ASCII]   Reached segment endpoint at (%d, %d)\n", x, y)
			}
			break
		}
		x = int(math.Round(fx))
		y = int(math.Round(fy))

		// Skip if out of bounds or contains alphanumeric character
		if !isInBounds(rd, x, y) {
			fmt.Printf("[D2ASCII]   Position (%d, %d) out of bounds, skipping\n", x, y)
			fx += sx
			fy += sy
			continue
		}
		if containsAlphaNumeric(rd, x, y) {
			fmt.Printf("[D2ASCII]   Position (%d, %d) contains alphanumeric, skipping\n", x, y)
			fx += sx
			fy += sy
			continue
		}

		// Draw the appropriate character at this position
		drawRoutePoint(rd, x, y, sx, sy, segmentIndex, len(conn.Route), ax, ay, cx, cy,
			conn, corners, arrows, turnDir, frmBoundary, toBoundary)

		// Draw label if we're at the right position
		if labelPos != nil && labelPos.ShouldDrawAt(segmentIndex-1, x, y, ax, ay, sx, sy) {
			drawConnectionLabel(rd, labelPos, label, conn.LabelPosition, x, y, sx, sy, conn.Route, segmentIndex)
		}

		fx += sx
		fy += sy
	}
}

func drawRoutePoint(rd RouteDrawer, x, y int, sx, sy float64, segmentIndex, routeLen int,
	ax, ay, cx, cy float64, conn d2target.Connection, corners, arrows, turnDir map[string]string,
	frmBoundary, toBoundary Boundary) {

	canvas := rd.GetCanvas()
	key := fmt.Sprintf("%d_%d", x, y)
	existingChar := canvas.Get(x, y)

	// Check for corners first
	if char, ok := corners[turnDir[key]]; ok {
		fmt.Printf("[D2ASCII]     Drawing corner at (%d, %d): '%s' (direction: %s)\n", x, y, char, turnDir[key])
		canvas.Set(x, y, char)
		return
	}

	// Check for destination arrow
	if segmentIndex == routeLen-1 && x == int(math.Round(cx)) && y == int(math.Round(cy)) && conn.DstArrow != d2target.NoArrowhead {
		fmt.Printf("[D2ASCII]     Drawing destination arrow at (%d, %d)\n", x, y)
		drawArrowhead(rd, x, y, sx, sy, arrows)
		if conn.DstLabel != nil {
			fmt.Printf("[D2ASCII]     Drawing destination label: %s\n", conn.DstLabel.Label)
			drawDestinationLabel(rd, conn.DstLabel.Label, cx, cy, sx, sy)
		}
		return
	}

	// Check for source arrow
	if segmentIndex == 1 && x == int(math.Round(ax)) && y == int(math.Round(ay)) && conn.SrcArrow != d2target.NoArrowhead {
		fmt.Printf("[D2ASCII]     Drawing source arrow at (%d, %d)\n", x, y)
		arrowKey := fmt.Sprintf("%d%d", geo.Sign(sx)*-1, geo.Sign(sy)*-1)
		canvas.Set(x, y, arrows[arrowKey])
		if conn.SrcLabel != nil {
			fmt.Printf("[D2ASCII]     Drawing source label: %s\n", conn.SrcLabel.Label)
			drawSourceLabel(rd, conn.SrcLabel.Label, ax, cy, cx, sx, sy)
		}
		return
	}

	// Default: draw route segment
	fmt.Printf("[D2ASCII]     Drawing route segment at (%d, %d), existing: '%s'\n",
		x, y, existingChar)
	drawRouteSegment(rd, x, y, sx, sy, frmBoundary, toBoundary)
}

func drawRouteSegment(rd RouteDrawer, x, y int, sx, sy float64, frmBoundary, toBoundary Boundary) {
	if !isInBounds(rd, x, y) {
		return
	}

	canvas := rd.GetCanvas()
	existingChar := canvas.Get(x, y)
	overWrite := existingChar != " "

	if sx == 0 { // Vertical line
		fmt.Printf("[D2ASCII]       Drawing vertical segment at (%d, %d), overwrite=%t, existing='%s'\n",
			x, y, overWrite, existingChar)
		drawVerticalSegment(rd, x, y, sy, overWrite, frmBoundary, toBoundary)
	} else { // Horizontal line
		fmt.Printf("[D2ASCII]       Drawing horizontal segment at (%d, %d), overwrite=%t, existing='%s'\n",
			x, y, overWrite, existingChar)
		drawHorizontalSegment(rd, x, y, sx, overWrite, frmBoundary, toBoundary)
	}

	newChar := canvas.Get(x, y)
	if newChar != existingChar {
		fmt.Printf("[D2ASCII]       Character placed: '%s' -> '%s' at (%d, %d)\n",
			existingChar, newChar, x, y)
	}
}

func drawVerticalSegment(rd RouteDrawer, x, y int, sy float64, overWrite bool, frmBoundary, toBoundary Boundary) {
	canvas := rd.GetCanvas()
	chars := rd.GetChars()

	if overWrite && shouldDrawTJunction(rd, x, y, frmBoundary, toBoundary, true) {
		if sy > 0 {
			fmt.Printf("[D2ASCII]         Drawing T-junction down at (%d, %d)\n", x, y)
			canvas.Set(x, y, chars.TDown())
		} else {
			fmt.Printf("[D2ASCII]         Drawing T-junction up at (%d, %d)\n", x, y)
			canvas.Set(x, y, chars.TUp())
		}
	} else if overWrite && shouldSkipOverwrite(rd, x, y, frmBoundary, toBoundary) {
		fmt.Printf("[D2ASCII]         Skipping overwrite at (%d, %d)\n", x, y)
	} else {
		fmt.Printf("[D2ASCII]         Drawing vertical line at (%d, %d)\n", x, y)
		canvas.Set(x, y, chars.Vertical())
	}
}

func drawHorizontalSegment(rd RouteDrawer, x, y int, sx float64, overWrite bool, frmBoundary, toBoundary Boundary) {
	canvas := rd.GetCanvas()
	chars := rd.GetChars()

	if overWrite && shouldDrawTJunction(rd, x, y, frmBoundary, toBoundary, false) {
		if sx > 0 {
			fmt.Printf("[D2ASCII]         Drawing T-junction right at (%d, %d)\n", x, y)
			canvas.Set(x, y, chars.TRight())
		} else {
			fmt.Printf("[D2ASCII]         Drawing T-junction left at (%d, %d)\n", x, y)
			canvas.Set(x, y, chars.TLeft())
		}
	} else {
		fmt.Printf("[D2ASCII]         Drawing horizontal line at (%d, %d)\n", x, y)
		canvas.Set(x, y, chars.Horizontal())
	}
}

func drawArrowhead(rd RouteDrawer, x, y int, sx, sy float64, arrows map[string]string) {
	canvas := rd.GetCanvas()
	arrowKey := fmt.Sprintf("%d%d", geo.Sign(sx), geo.Sign(sy))

	// Check if we're about to place arrow on a shape boundary character
	if canvas.IsInBounds(x, y) &&
		isShapeBoundaryChar(rd, canvas.Get(x, y)) {
		// Place arrow one step back to avoid touching boundary
		arrowX := x - int(math.Round(sx))
		arrowY := y - int(math.Round(sy))
		if canvas.IsInBounds(arrowX, arrowY) {
			canvas.Set(arrowX, arrowY, arrows[arrowKey])
		} else {
			canvas.Set(x, y, arrows[arrowKey])
		}
	} else {
		canvas.Set(x, y, arrows[arrowKey])
	}
}

// Drawing helper functions

func isInBounds(rd RouteDrawer, x, y int) bool {
	return rd.GetCanvas().IsInBounds(x, y)
}

func containsAlphaNumeric(rd RouteDrawer, x, y int) bool {
	return rd.GetCanvas().ContainsAlphaNumeric(x, y)
}

func isShapeBoundaryChar(rd RouteDrawer, char string) bool {
	chars := rd.GetChars()
	return char == chars.Horizontal() || char == chars.Vertical() ||
		char == chars.TopLeftCorner() || char == chars.TopRightCorner() ||
		char == chars.BottomLeftCorner() || char == chars.BottomRightCorner() ||
		char == chars.TopLeftArc() || char == chars.TopRightArc() ||
		char == chars.BottomLeftArc() || char == chars.BottomRightArc()
}

func shouldDrawTJunction(rd RouteDrawer, x, y int, frmBoundary, toBoundary Boundary, isVertical bool) bool {
	canvas := rd.GetCanvas()
	chars := rd.GetChars()
	if isVertical {
		// Check if we're crossing a horizontal boundary line
		if (y == frmBoundary.BR.Y || y == frmBoundary.TL.Y) &&
			canvas.Get(x, y) == chars.Horizontal() {
			return true
		}
		if (y == toBoundary.BR.Y || y == toBoundary.TL.Y) &&
			canvas.Get(x, y) == chars.Horizontal() {
			return true
		}
	} else {
		// Check if we're crossing a vertical boundary line
		if (x == frmBoundary.BR.X-1 || x == frmBoundary.TL.X-1) &&
			canvas.Get(x, y) == chars.Vertical() {
			return true
		}
		if (x == toBoundary.BR.X-1 || x == toBoundary.TL.X-1) &&
			canvas.Get(x, y) == chars.Vertical() {
			return true
		}
	}
	return false
}

func shouldSkipOverwrite(rd RouteDrawer, x, y int, frmBoundary, toBoundary Boundary) bool {
	canvas := rd.GetCanvas()
	chars := rd.GetChars()
	if (canvas.Get(x, y) == chars.Underscore() && (y == frmBoundary.BR.Y || y == toBoundary.BR.Y)) ||
		(canvas.Get(x, y) == chars.Overline() && (y == frmBoundary.TL.Y || y == toBoundary.TL.Y)) {
		return true
	}
	return false
}
