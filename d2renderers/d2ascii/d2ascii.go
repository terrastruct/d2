package d2ascii

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciicanvas"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciishapes"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
)

// Font dimensions
const (
	defaultFontWidth  = 9.75
	defaultFontHeight = 18.0
	defaultScale      = 1.0
)

// Route drawing constants
const (
	maxRouteAttempts = 200
	labelOffsetX = 2
)

type ASCIIartist struct {
	canvas  *asciicanvas.Canvas
	FW      float64
	FH      float64
	chars   charset.Set
	entr    string
	bcurve  string
	tcurve  string
	SCALE   float64
	diagram d2target.Diagram
}
type RenderOpts struct {
	Scale *float64
	Charset charset.Type
}
// Point represents a 2D coordinate
type Point struct {
	X int
	Y int
}

type Boundary struct {
	TL Point
	BR Point
}

func (b *Boundary) Contains(x int, y int) bool {
	return x >= b.TL.X && x <= b.BR.X && y >= b.TL.Y && y <= b.BR.Y
}

func NewBoundary(tl, br Point) *Boundary {
	return &Boundary{
		TL: tl,
		BR: br,
	}
}

func (a *ASCIIartist) GetBoundary(s d2target.Shape) (Point, Point) {
	x1 := int(math.Round((float64(s.Pos.X) / a.FW) * a.SCALE))
	y1 := int(math.Round((float64(s.Pos.Y) / a.FH) * a.SCALE))
	x2 := int(math.Round(((float64(s.Pos.X) + float64(s.Width) - 1) / a.FW) * a.SCALE))
	y2 := int(math.Round(((float64(s.Pos.Y) + float64(s.Height) - 1) / a.FH) * a.SCALE))
	return Point{X: x1, Y: y1}, Point{X: x2, Y: y2}
}



func NewASCIIartist() *ASCIIartist {
	artist := &ASCIIartist{
		FW:      defaultFontWidth,
		FH:      defaultFontHeight,
		SCALE:   defaultScale,
		entr:    "\n",
		bcurve:  "`-._",
		tcurve:  ".-`‾",
		chars:   charset.New(charset.Unicode),
		diagram: *d2target.NewDiagram(),
	}

	return artist
}

// calculateExtendedBounds calculates bounds including connection labels
func (a *ASCIIartist) calculateExtendedBounds(diagram *d2target.Diagram) (tl, br d2target.Point) {
	tl, br = diagram.NestedBoundingBox()

	// Extend bounds to include connection labels
	for _, conn := range diagram.Connections {
		if conn.Label != "" && len(conn.Route) > 1 {
			// Find longest route segment for label placement
			maxDiff := 0.0
			bestX := 0.0
			for i := 0; i < len(conn.Route)-1; i++ {
				diffY := math.Abs(conn.Route[i].Y - conn.Route[i+1].Y)
				diffX := math.Abs(conn.Route[i].X - conn.Route[i+1].X)
				diff := math.Max(diffY, diffX)
				if diff > maxDiff {
					maxDiff = diff
					bestX = conn.Route[i].X
					if diff == diffX {
						bestX = conn.Route[i].X + (math.Copysign(1, conn.Route[i+1].X-conn.Route[i].X) * diff / 2)
					}
				}
			}
			labelX := bestX - float64(len(conn.Label))/2*a.FW
			labelX2 := bestX + float64(len(conn.Label))/2*a.FW
			// Estimate Y position (this is approximate since exact positioning is complex)
			midY := (conn.Route[0].Y + conn.Route[len(conn.Route)-1].Y) / 2
			labelY := midY - a.FH
			labelY2 := midY + a.FH
			if int(labelX) < tl.X {
				tl.X = int(labelX)
			}
			if int(labelX2) > br.X {
				br.X = int(labelX2)
			}
			if int(labelY) < tl.Y {
				tl.Y = int(labelY)
			}
			if int(labelY2) > br.Y {
				br.Y = int(labelY2)
			}
		}

		// Check destination and source arrow labels
		if conn.DstLabel != nil && len(conn.Route) > 0 {
			lastRoute := conn.Route[len(conn.Route)-1]
			labelX := lastRoute.X - float64(len(conn.DstLabel.Label))*a.FW
			labelX2 := lastRoute.X + float64(len(conn.DstLabel.Label))*a.FW
			labelY := lastRoute.Y - a.FH
			labelY2 := lastRoute.Y + a.FH
			if int(labelX) < tl.X {
				tl.X = int(labelX)
			}
			if int(labelX2) > br.X {
				br.X = int(labelX2)
			}
			if int(labelY) < tl.Y {
				tl.Y = int(labelY)
			}
			if int(labelY2) > br.Y {
				br.Y = int(labelY2)
			}
		}

		if conn.SrcLabel != nil && len(conn.Route) > 0 {
			firstRoute := conn.Route[0]
			labelX := firstRoute.X - float64(len(conn.SrcLabel.Label))*a.FW
			labelX2 := firstRoute.X + float64(len(conn.SrcLabel.Label))*a.FW
			labelY := firstRoute.Y - a.FH
			labelY2 := firstRoute.Y + a.FH
			if int(labelX) < tl.X {
				tl.X = int(labelX)
			}
			if int(labelX2) > br.X {
				br.X = int(labelX2)
			}
			if int(labelY) < tl.Y {
				tl.Y = int(labelY)
			}
			if int(labelY2) > br.Y {
				br.Y = int(labelY2)
			}
		}
	}

	return tl, br
}

func (a *ASCIIartist) Render(diagram *d2target.Diagram, opts *RenderOpts) ([]byte, error) {
	if opts == nil {
		opts = &RenderOpts{}
	}
	// Allow changing character set for this render
	if opts.Charset == charset.ASCII {
		a.chars = charset.New(charset.ASCII)
	}
	xOffset := 0
	yOffset := 0
	a.diagram = *diagram
	tl, br := a.calculateExtendedBounds(diagram)
	if tl.X < 0 {
		xOffset = -tl.X
		br.X += -tl.X
		tl.X = 0
	}
	if tl.Y < 0 {
		yOffset = -tl.Y
		br.Y += -tl.Y
		tl.Y = 0
	}
	w := int(math.Ceil(float64(br.X - tl.X)))
	h := int(math.Ceil(float64(br.Y - tl.Y)))

	w = int(math.Round((float64(w) / a.FW) * a.SCALE))
	h = int(math.Round((float64(h) / a.FH) * a.SCALE))

	// Add padding to account for potential width/height adjustments in drawing functions
	maxLabelLen := 0
	for _, shape := range diagram.Shapes {
		if len(shape.Label) > maxLabelLen {
			maxLabelLen = len(shape.Label)
		}
	}
	padding := maxLabelLen + asciishapes.MinLabelPadding // Match the maximum possible adjustment in drawRect

	a.canvas = asciicanvas.New(w+padding+1, h+padding+1)

	// Draw shapes
	for _, shape := range diagram.Shapes {
		if shape.Classes != nil && slices.Contains(shape.Classes, "NONE") {
			continue
		}
		shape.Pos.X += xOffset
		shape.Pos.Y += yOffset
		
		// Create shape context
		ctx := &asciishapes.Context{
			Canvas: a.canvas,
			Chars:  a.chars,
			FW:     a.FW,
			FH:     a.FH,
			Scale:  a.SCALE,
		}
		
		switch shape.Type {
		case d2target.ShapeRectangle:
			asciishapes.DrawRect(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition, "")
		case d2target.ShapeSquare:
			asciishapes.DrawRect(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition, "")
		case d2target.ShapePage:
			asciishapes.DrawPage(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeHexagon:
			asciishapes.DrawHex(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapePerson:
			asciishapes.DrawPerson(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeStoredData:
			asciishapes.DrawStoredData(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeCylinder:
			asciishapes.DrawCylinder(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapePackage:
			asciishapes.DrawPackage(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeParallelogram:
			asciishapes.DrawParallelogram(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeQueue:
			asciishapes.DrawQueue(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeStep:
			asciishapes.DrawStep(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeCallout:
			asciishapes.DrawCallout(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeDocument:
			asciishapes.DrawDocument(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeDiamond:
			asciishapes.DrawDiamond(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		default:
			symbol := ""
			switch shape.Type {
			case d2target.ShapeCloud:
				symbol = a.chars.Cloud()
			case d2target.ShapeCircle:
				symbol = a.chars.Circle()
			case d2target.ShapeOval:
				symbol = a.chars.Oval()
			default:
				symbol = ""
			}
			asciishapes.DrawRect(ctx, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition, symbol)
		}
	}
	// Draw connections
	// First pass: draw routes without arrowheads (like sequence diagram lifelines)
	for _, conn := range diagram.Connections {
		for _, r := range conn.Route {
			r.X += float64(xOffset)
			r.Y += float64(yOffset)
		}
		if conn.DstArrow == d2target.NoArrowhead && conn.SrcArrow == d2target.NoArrowhead {
			a.drawRoute(conn)
		}
	}
	// Second pass: draw routes with arrowheads (so they can detect boundaries and push back)
	for _, conn := range diagram.Connections {
		if conn.DstArrow != d2target.NoArrowhead || conn.SrcArrow != d2target.NoArrowhead {
			a.drawRoute(conn)
		}
	}
	return a.canvas.ToByteArray(), nil
}

func (a *ASCIIartist) calibrateXY(x, y float64) (float64, float64) {
	xC := float64(math.Round((x / a.FW) * a.SCALE))
	yC := float64(math.Round((y / a.FH) * a.SCALE))
	return xC, yC
}
// parseConnectionBoundaries extracts source and destination shape boundaries from connection ID
func (aa *ASCIIartist) parseConnectionBoundaries(connID string) (frmShapeBoundary, toShapeBoundary Boundary) {
	re := regexp.MustCompile(` -> | <-> | -- `)
	re1 := regexp.MustCompile(`\(([^}]*)\)`)
	re2 := regexp.MustCompile(`(.*)\(`)
	match1 := re1.FindStringSubmatch(connID)
	match2 := re2.FindStringSubmatch(connID)
	
	if len(match1) > 0 {
		parentID := ""
		if len(match2) > 0 {
			parentID = match2[1]
		}
		splitResult := re.Split(match1[1], -1)
		for _, shape := range aa.diagram.Shapes {
			if len(splitResult) > 0 && shape.ID == parentID+splitResult[0] {
				tl, br := aa.GetBoundary(shape)
				frmShapeBoundary = *NewBoundary(tl, br)
			} else if len(splitResult) > 1 && shape.ID == parentID+splitResult[1] {
				tl, br := aa.GetBoundary(shape)
				toShapeBoundary = *NewBoundary(tl, br)
			}
		}
	}
	return
}

func (aa *ASCIIartist) drawRoute(conn d2target.Connection) {
	routes := conn.Route
	label := conn.Label
	frmShapeBoundary, toShapeBoundary := aa.parseConnectionBoundaries(conn.ID)
	routes = mergeRoutes(routes)
	aa.calibrateRoutes(routes)
	
	// Adjust route endpoints to avoid overlapping with existing characters
	if len(routes) >= 2 {
		aa.adjustRouteStartPoint(routes)
		aa.adjustRouteEndPoint(routes)
	}

	// Calculate turn directions for corners
	turnDir := aa.calculateTurnDirections(routes)

	// Calculate best label position if label exists
	var labelPos *routeLabelPosition
	if strings.TrimSpace(label) != "" {
		labelPos = aa.calculateBestLabelPosition(routes, label)
	}

	corners := map[string]string{
		"-100-1": aa.chars.BottomLeftCorner(), "0110": aa.chars.BottomLeftCorner(),
		"-1001": aa.chars.TopLeftCorner(), "0-110": aa.chars.TopLeftCorner(),
		"0-1-10": aa.chars.TopRightCorner(), "1001": aa.chars.TopRightCorner(),
		"01-10": aa.chars.BottomRightCorner(), "100-1": aa.chars.BottomRightCorner(),
	}
	arrows := map[string]string{
		"0-1": aa.chars.ArrowUp(), "10": aa.chars.ArrowRight(), "01": aa.chars.ArrowDown(), "-10": aa.chars.ArrowLeft(),
	}

	for i := 1; i < len(routes); i++ {
		aa.drawSegmentBetweenPoints(routes[i-1], routes[i], i, conn, corners, arrows, turnDir, frmShapeBoundary, toShapeBoundary, labelPos, label)
	}
}
// calibrateRoutes adjusts route coordinates to canvas scale
func (aa *ASCIIartist) calibrateRoutes(routes []*geo.Point) {
	for i := range routes {
		routes[i].X, routes[i].Y = aa.calibrateXY(routes[i].X, routes[i].Y)
		routes[i].X -= 1
	}
}

// mergeRoutes combines consecutive route points in the same direction
func mergeRoutes(routes []*geo.Point) []*geo.Point {
	if len(routes) < 2 {
		return routes
	}
	
	mRoutes := []*geo.Point{routes[0]}
	var pt = routes[0]
	dir := geo.Sign(routes[0].X-routes[1].X)*1 + geo.Sign(routes[0].Y-routes[1].Y)*2
	for j := 1; j < len(routes); j++ {
		newDir := geo.Sign(pt.X-routes[j].X)*1 + geo.Sign(pt.Y-routes[j].Y)*2
		if dir != newDir {
			mRoutes = append(mRoutes, pt)
			dir = newDir
		}
		pt = routes[j]
	}
	if mRoutes[len(mRoutes)-1].X != pt.X || mRoutes[len(mRoutes)-1].Y != pt.Y {
		mRoutes = append(mRoutes, pt)
	}
	return mRoutes
}
// adjustRouteStartPoint shifts the start point to find empty space
func (aa *ASCIIartist) adjustRouteStartPoint(routes []*geo.Point) {
	if len(routes) < 2 {
		return
	}
	
	firstX := routes[0].X
	firstY := routes[0].Y
	secondX := routes[1].X
	secondY := routes[1].Y

	// Determine line direction and keep shifting until empty space
	if math.Abs(firstY-secondY) < 0.1 { // Horizontal line
		deltaX := 0.0
		if secondX > firstX {
			deltaX = 1.0 // Shift start point towards second point (right)
		} else if secondX < firstX {
			deltaX = -1.0 // Shift start point towards second point (left)
		}

		if deltaX != 0 {
			aa.shiftPointUntilEmpty(&routes[0].X, &routes[0].Y, deltaX, 0)
		}
	} else if math.Abs(firstX-secondX) < 0.1 { // Vertical line
		deltaY := 0.0
		if secondY > firstY {
			deltaY = 1.0 // Shift start point towards second point (down)
		} else if secondY < firstY {
			deltaY = -1.0 // Shift start point towards second point (up)
		}

		if deltaY != 0 {
			aa.shiftPointUntilEmpty(&routes[0].X, &routes[0].Y, 0, deltaY)
		}
	}
}

// adjustRouteEndPoint shifts the end point to find empty space
func (aa *ASCIIartist) adjustRouteEndPoint(routes []*geo.Point) {
	if len(routes) < 2 {
		return
	}
	
	lastIdx := len(routes) - 1
	secondLastIdx := lastIdx - 1

	lastX := routes[lastIdx].X
	lastY := routes[lastIdx].Y
	secondLastX := routes[secondLastIdx].X
	secondLastY := routes[secondLastIdx].Y

	// Determine line direction and keep shifting until empty space
	if math.Abs(lastY-secondLastY) < 0.1 { // Horizontal line
		deltaX := 0.0
		if secondLastX > lastX {
			deltaX = 1.0 // Shift end point towards second-to-last point (right)
		} else if secondLastX < lastX {
			deltaX = -1.0 // Shift end point towards second-to-last point (left)
		}

		if deltaX != 0 {
			aa.shiftPointUntilEmpty(&routes[lastIdx].X, &routes[lastIdx].Y, deltaX, 0)
		}
	} else if math.Abs(lastX-secondLastX) < 0.1 { // Vertical line
		deltaY := 0.0
		if secondLastY > lastY {
			deltaY = 1.0 // Shift end point towards second-to-last point (down)
		} else if secondLastY < lastY {
			deltaY = -1.0 // Shift end point towards second-to-last point (up)
		}

		if deltaY != 0 {
			aa.shiftPointUntilEmpty(&routes[lastIdx].X, &routes[lastIdx].Y, 0, deltaY)
		}
	}
}

// shiftPointUntilEmpty keeps shifting a point by delta until empty space is found
func (aa *ASCIIartist) shiftPointUntilEmpty(x, y *float64, deltaX, deltaY float64) {
	for {
		xi := int(math.Round(*x))
		yi := int(math.Round(*y))
		if aa.canvas.IsInBounds(xi, yi) {
			if aa.canvas.Get(xi, yi) == " " {
				break // Found empty space
			}
			*x += deltaX
			*y += deltaY
		} else {
			break // Out of bounds
		}
	}
}

// calculateTurnDirections determines corner types for route points
func (aa *ASCIIartist) calculateTurnDirections(routes []*geo.Point) map[string]string {
	turnDir := map[string]string{}
	if len(routes) < 3 {
		return turnDir
	}
	
	for i := 1; i < len(routes)-1; i++ {
		curr := routes[i]
		prev := routes[i-1]
		next := routes[i+1]
		
		key := fmt.Sprintf("%d_%d", int(math.Round(curr.X)), int(math.Round(curr.Y)))
		dir := fmt.Sprintf("%d%d%d%d",
			geo.Sign(curr.X-prev.X), geo.Sign(curr.Y-prev.Y),
			geo.Sign(next.X-curr.X), geo.Sign(next.Y-curr.Y),
		)
		turnDir[key] = dir
	}
	return turnDir
}

// isShapeBoundaryChar checks if a character is a shape boundary
func (aa *ASCIIartist) isShapeBoundaryChar(char string) bool {
	return char == charset.UnicodeHorizontal || char == charset.UnicodeVertical ||
		char == charset.UnicodeTopLeftCorner || char == charset.UnicodeTopRightCorner ||
		char == charset.UnicodeBottomLeftCorner || char == charset.UnicodeBottomRightCorner ||
		char == charset.UnicodeTopLeftArc || char == charset.UnicodeTopRightArc ||
		char == charset.UnicodeBottomLeftArc || char == charset.UnicodeBottomRightArc
}

// drawArrowhead places an arrowhead at the given position
func (aa *ASCIIartist) drawArrowhead(x, y int, sx, sy float64, arrows map[string]string) {
	arrowKey := fmt.Sprintf("%d%d", geo.Sign(sx), geo.Sign(sy))
	
	// Check if we're about to place arrow on a shape boundary character
	if aa.canvas.IsInBounds(x, y) &&
		aa.isShapeBoundaryChar(aa.canvas.Get(x, y)) {
		// Place arrow one step back to avoid touching boundary
		arrowX := x - int(math.Round(sx))
		arrowY := y - int(math.Round(sy))
		if aa.canvas.IsInBounds(arrowX, arrowY) {
			aa.canvas.Set(arrowX, arrowY, arrows[arrowKey])
		} else {
			aa.canvas.Set(x, y, arrows[arrowKey])
		}
	} else {
		aa.canvas.Set(x, y, arrows[arrowKey])
	}
}

// drawDestinationLabel draws a label near the destination arrow
func (aa *ASCIIartist) drawDestinationLabel(label string, cx, cy, sx, sy float64) {
	ly := 0
	lx := 0
	if math.Abs(sx) > 0 {
		ly = int(cy - 1)
		if sx > 0 {
			lx = int(cx) - 1 - len(label)
		} else {
			lx = int(cx)
		}
	} else if math.Abs(sy) > 0 {
		ly = int(cy - 1)
		lx = int(cx + 1)
	}
	for j, ch := range label {
		aa.canvas.Set(lx+j+labelOffsetX, ly, string(ch))
	}
}

// drawSourceLabel draws a label near the source arrow
func (aa *ASCIIartist) drawSourceLabel(label string, ax, cy, cx, sx, sy float64) {
	ly := 0
	lx := 0
	if math.Abs(sx) > 0 {
		ly = int(cy - 1)
		if sx > 0 {
			lx = int(ax)
		} else {
			lx = int(ax) - 1 - len(label)
		}
	} else if math.Abs(sy) > 0 {
		ly = int(cy - 1)
		lx = int(cx + 1)
	}
	for j, ch := range label {
		aa.canvas.Set(lx+j, ly, string(ch))
	}
}

// drawRouteSegment draws a single segment of the route (horizontal/vertical line)
func (aa *ASCIIartist) drawRouteSegment(x, y int, sx, sy float64, frmBoundary, toBoundary Boundary) {
	if !aa.isInBounds(x, y) {
		return
	}
	
	overWrite := aa.canvas.Get(x, y) != " "
	
	if sx == 0 { // Vertical line
		aa.drawVerticalSegment(x, y, sy, overWrite, frmBoundary, toBoundary)
	} else { // Horizontal line
		aa.drawHorizontalSegment(x, y, sx, overWrite, frmBoundary, toBoundary)
	}
}

// drawVerticalSegment draws a vertical line segment
func (aa *ASCIIartist) drawVerticalSegment(x, y int, sy float64, overWrite bool, frmBoundary, toBoundary Boundary) {
	if overWrite && aa.shouldDrawTJunction(x, y, frmBoundary, toBoundary, true) {
		if sy > 0 {
			aa.canvas.Set(x, y, aa.chars.TDown())
		} else {
			aa.canvas.Set(x, y, aa.chars.TUp())
		}
	} else if overWrite && aa.shouldSkipOverwrite(x, y, frmBoundary, toBoundary) {
		// skip
	} else {
		aa.canvas.Set(x, y, aa.chars.Vertical())
	}
}

// drawHorizontalSegment draws a horizontal line segment
func (aa *ASCIIartist) drawHorizontalSegment(x, y int, sx float64, overWrite bool, frmBoundary, toBoundary Boundary) {
	if overWrite && aa.shouldDrawTJunction(x, y, frmBoundary, toBoundary, false) {
		if sx > 0 {
			aa.canvas.Set(x, y, aa.chars.TRight())
		} else {
			aa.canvas.Set(x, y, aa.chars.TLeft())
		}
	} else {
		aa.canvas.Set(x, y, aa.chars.Horizontal())
	}
}

// isInBounds checks if coordinates are within canvas bounds
func (aa *ASCIIartist) isInBounds(x, y int) bool {
	return aa.canvas.IsInBounds(x, y)
}

// shouldDrawTJunction determines if a T-junction should be drawn at intersection
func (aa *ASCIIartist) shouldDrawTJunction(x, y int, frmBoundary, toBoundary Boundary, isVertical bool) bool {
	if isVertical {
		// Check if we're crossing a horizontal boundary line
		if (y == frmBoundary.BR.Y || y == frmBoundary.TL.Y) &&
			aa.canvas.Get(x, y) == charset.UnicodeHorizontal {
			return true
		}
		if (y == toBoundary.BR.Y || y == toBoundary.TL.Y) &&
			aa.canvas.Get(x, y) == charset.UnicodeHorizontal {
			return true
		}
	} else {
		// Check if we're crossing a vertical boundary line
		if (x == frmBoundary.BR.X-1 || x == frmBoundary.TL.X-1) &&
			aa.canvas.Get(x, y) == charset.UnicodeVertical {
			return true
		}
		if (x == toBoundary.BR.X-1 || x == toBoundary.TL.X-1) &&
			aa.canvas.Get(x, y) == charset.UnicodeVertical {
			return true
		}
	}
	return false
}

// shouldSkipOverwrite determines if we should skip overwriting certain characters
func (aa *ASCIIartist) shouldSkipOverwrite(x, y int, frmBoundary, toBoundary Boundary) bool {
	if (aa.canvas.Get(x, y) == charset.UnicodeUnderscore && (y == frmBoundary.BR.Y || y == toBoundary.BR.Y)) ||
		(aa.canvas.Get(x, y) == charset.UnicodeOverline && (y == frmBoundary.TL.Y || y == toBoundary.TL.Y)) {
		return true
	}
	return false
}

// routeLabelPosition holds calculated position for route label
type routeLabelPosition struct {
	I        int     // Index of route segment
	X        int     // X coordinate for label
	Y        int     // Y coordinate offset
	maxDiff  float64 // Maximum difference for the segment
}

// shouldDrawAt checks if label should be drawn at current position
func (pos *routeLabelPosition) shouldDrawAt(currentIndex, x, y int, ax, ay, sx, sy float64) bool {
	if pos.I != currentIndex {
		return false
	}
	
	if sy != 0 {
		return int(math.Round(ay))+int(math.Round(pos.maxDiff/2))*geo.Sign(sy) == y
	}
	
	if sx != 0 {
		return int(math.Round(ax))+int(math.Round(pos.maxDiff/2))*geo.Sign(sx) == x
	}
	
	return false
}

// calculateBestLabelPosition finds the best position for a connection label
func (aa *ASCIIartist) calculateBestLabelPosition(routes []*geo.Point, label string) *routeLabelPosition {
	if len(routes) < 2 {
		return nil
	}
	
	maxDiff := 0.0
	bestIndex := -1
	bestX := 0.0
	scaleOld := 0.0
	
	for i := 0; i < len(routes)-1; i++ {
		diffY := math.Abs(routes[i].Y - routes[i+1].Y)
		diffX := math.Abs(routes[i].X - routes[i+1].X)
		diff := math.Max(diffY, diffX)
		scale := (math.Abs(float64(geo.Sign(diffX)))*aa.FW + math.Abs(float64(geo.Sign(diffY)))*aa.FH)
		
		if diff*scale > maxDiff*scaleOld {
			maxDiff = diff
			bestIndex = i
			bestX = routes[i].X
			
			// Center label on horizontal segments
			if diff == diffX && i+1 < len(routes) {
				direction := geo.Sign(routes[i+1].X - routes[i].X)
				bestX = routes[i].X + (float64(direction) * diff / 2)
			}
		}
		scaleOld = scale
	}
	
	if bestIndex == -1 {
		return nil
	}
	
	return &routeLabelPosition{
		I:       bestIndex,
		X:       int(math.Round(bestX)) - len(label)/2,
		Y:       int(math.Round(maxDiff / 2)),
		maxDiff: maxDiff,
	}
}

// drawConnectionLabel draws a label on a connection route
func (aa *ASCIIartist) drawConnectionLabel(labelPos *routeLabelPosition, label, labelPosition string, x, y int, sx, sy float64, routes []*geo.Point, i int) {
	if sy != 0 {
		// Vertical segment - clear current position and draw label horizontally
		if aa.isInBounds(x, y) {
			aa.canvas.Set(x, y, " ")
		}
		for j, ch := range label {
			if aa.isInBounds(labelPos.X+j, y) {
				aa.canvas.Set(labelPos.X+j, y, string(ch))
			}
		}
	} else if sx != 0 {
		// Horizontal segment - draw label above or below
		yFactor := 0
		if strings.Contains(labelPosition, "TOP") {
			yFactor = -1
		} else if strings.Contains(labelPosition, "BOTTOM") {
			yFactor = 1
		}
		
		// Adjust X position based on LEFT/RIGHT preference
		xPos := labelPos.X
		if strings.Contains(labelPosition, "LEFT") {
			xPos = int(routes[labelPos.I+absInt((geo.Sign(sx)-1)/2)].X)
		} else if strings.Contains(labelPosition, "RIGHT") {
			xPos = int(routes[labelPos.I+((geo.Sign(sx)+1)/2)].X) - len(label)/2
		}
		
		for j, ch := range label {
			if aa.isInBounds(xPos+j, y+yFactor) {
				aa.canvas.Set(xPos+j, y+yFactor, string(ch))
			}
		}
	}
}

// drawSegmentBetweenPoints draws a route segment between two points
func (aa *ASCIIartist) drawSegmentBetweenPoints(start, end *geo.Point, segmentIndex int, conn d2target.Connection, 
	corners, arrows, turnDir map[string]string, frmBoundary, toBoundary Boundary, labelPos *routeLabelPosition, label string) {
	
	ax, ay := start.X, start.Y
	cx, cy := end.X, end.Y

	sx := cx - ax
	sy := cy - ay
	step := math.Max(math.Abs(sx), math.Abs(sy))
	if step == 0 {
		return
	}
	sx /= step
	sy /= step

	fx, fy := ax, ay
	attempt := 0
	x := int(math.Round(ax))
	y := int(math.Round(ay))
	
	for {
		attempt++
		if x == int(math.Round(cx)) && y == int(math.Round(cy)) || attempt == maxRouteAttempts {
			break
		}
		x = int(math.Round(fx))
		y = int(math.Round(fy))

		// Skip if out of bounds or contains alphanumeric character
		if !aa.isInBounds(x, y) || aa.containsAlphaNumeric(x, y) {
			fx += sx
			fy += sy
			continue
		}

		// Draw the appropriate character at this position
		aa.drawRoutePoint(x, y, sx, sy, segmentIndex, len(conn.Route), ax, ay, cx, cy,
			conn, corners, arrows, turnDir, frmBoundary, toBoundary)

		// Draw label if we're at the right position
		if labelPos != nil && labelPos.shouldDrawAt(segmentIndex-1, x, y, ax, ay, sx, sy) {
			aa.drawConnectionLabel(labelPos, label, conn.LabelPosition, x, y, sx, sy, conn.Route, segmentIndex)
		}
		
		fx += sx
		fy += sy
	}
}

// containsAlphaNumeric checks if a canvas position contains alphanumeric characters
func (aa *ASCIIartist) containsAlphaNumeric(x, y int) bool {
	return aa.canvas.ContainsAlphaNumeric(x, y)
}

// drawRoutePoint draws the appropriate character at a route point
func (aa *ASCIIartist) drawRoutePoint(x, y int, sx, sy float64, segmentIndex, routeLen int, 
	ax, ay, cx, cy float64, conn d2target.Connection, corners, arrows, turnDir map[string]string,
	frmBoundary, toBoundary Boundary) {
	
	key := fmt.Sprintf("%d_%d", x, y)
	
	// Check for corners first
	if char, ok := corners[turnDir[key]]; ok {
		aa.canvas.Set(x, y, char)
		return
	}
	
	// Check for destination arrow
	if segmentIndex == routeLen-1 && x == int(math.Round(cx)) && y == int(math.Round(cy)) && conn.DstArrow != d2target.NoArrowhead {
		aa.drawArrowhead(x, y, sx, sy, arrows)
		if conn.DstLabel != nil {
			aa.drawDestinationLabel(conn.DstLabel.Label, cx, cy, sx, sy)
		}
		return
	}
	
	// Check for source arrow
	if segmentIndex == 1 && x == int(math.Round(ax)) && y == int(math.Round(ay)) && conn.SrcArrow != d2target.NoArrowhead {
		arrowKey := fmt.Sprintf("%d%d", geo.Sign(sx)*-1, geo.Sign(sy)*-1)
		aa.canvas.Set(x, y, arrows[arrowKey])
		if conn.SrcLabel != nil {
			aa.drawSourceLabel(conn.SrcLabel.Label, ax, cy, cx, sx, sy)
		}
		return
	}
	
	// Default: draw route segment
	aa.drawRouteSegment(x, y, sx, sy, frmBoundary, toBoundary)
}

func absInt(a int) int {
	return int(math.Abs(float64(a)))
}
