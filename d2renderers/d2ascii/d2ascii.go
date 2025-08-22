// Set DEBUG_ASCII=1 environment variable to enable verbose ASCII rendering debug logs.
package d2ascii

import (
	"context"
	"log/slog"
	"math"
	"os"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciicanvas"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciiroute"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciishapes"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/log"
)

const (
	defaultFontWidth  = 1
	defaultFontHeight = 1
	defaultScale      = 1.0
)

const (
	maxRouteAttempts = asciiroute.MaxRouteAttempts
	labelOffsetX     = asciiroute.LabelOffsetX
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
	ctx     context.Context
}
type RenderOpts struct {
	Scale   *float64
	Charset charset.Type
}

type Point = asciiroute.Point

type Boundary = asciiroute.Boundary

func NewBoundary(tl, br Point) *Boundary {
	return asciiroute.NewBoundary(tl, br)
}

func (a *ASCIIartist) GetBoundary(s d2target.Shape) (Point, Point) {
	log.Debug(a.ctx, "GetBoundary called", slog.String("id", s.ID), slog.String("label", s.Label))

	// For multiple shapes, expand boundary to match the expanded rendering
	posX := float64(s.Pos.X)
	posY := float64(s.Pos.Y)
	width := float64(s.Width)
	height := float64(s.Height)
	log.Debug(a.ctx, "original shape dimensions", slog.Float64("posX", posX), slog.Float64("posY", posY), slog.Float64("width", width), slog.Float64("height", height))

	if s.Multiple {
		// ASCII rendering doesn't need offset - multiple shapes are indicated by character style, not shadows
		// Use offset of 0 to prevent boundary inflation that breaks edge routing
		log.Debug(a.ctx, "multiple shape - no boundary adjustment needed for ASCII", slog.String("id", s.ID))
	}

	// Use the same calibration logic as the drawing functions
	shapeCtx := &asciishapes.Context{
		Canvas: a.canvas,
		Chars:  a.chars,
		FW:     a.FW,
		FH:     a.FH,
		Scale:  a.SCALE,
		Ctx:    a.ctx,
	}
	x1, y1, wC, hC := shapeCtx.Calibrate(posX, posY, width, height)
	log.Debug(a.ctx, "calibrated dimensions", slog.Int("x1", x1), slog.Int("y1", y1), slog.Int("wC", wC), slog.Int("hC", hC))

	// Apply the same width adjustments as the drawing code
	preserveWidth := hasConnectionsAtRightEdge(s, a.diagram.Connections, a.FW)
	preserveHeight := hasConnectionsAtTopEdge(s, a.diagram.Connections, a.FH)
	if preserveWidth && s.Label != "" {
		availableSpace := wC - len(s.Label)
		if availableSpace >= asciishapes.MinLabelPadding && availableSpace%2 == 1 {
			// Adjust the original width before recalibrating
			width += float64(int(a.FW / a.SCALE))
			x1, y1, wC, hC = shapeCtx.Calibrate(posX, posY, width, height)
		}
	}

	// Apply the same height adjustments as DrawRect for labels
	if s.Label != "" && hC%2 == 0 && !preserveHeight {
		if hC > 2 {
			hC--
			y1++
		} else {
			hC++
		}
	}

	// Apply the same width adjustments as DrawRect for labels
	wC = asciishapes.AdjustWidthForLabel(shapeCtx, posX, posY, width, height, wC, s.Label)
	log.Debug(a.ctx, "final width after label adjustment", slog.Int("wC", wC))

	x2, y2 := x1+wC, y1+hC
	log.Debug(a.ctx, "final boundary", slog.Int("x1", x1), slog.Int("y1", y1), slog.Int("x2", x2), slog.Int("y2", y2))

	return Point{X: x1, Y: y1}, Point{X: x2, Y: y2}
}

func (a *ASCIIartist) GetCanvas() *asciicanvas.Canvas { return a.canvas }
func (a *ASCIIartist) GetChars() charset.Set          { return a.chars }
func (a *ASCIIartist) GetDiagram() *d2target.Diagram  { return &a.diagram }
func (a *ASCIIartist) GetFontWidth() float64          { return a.FW }
func (a *ASCIIartist) GetFontHeight() float64         { return a.FH }
func (a *ASCIIartist) GetScale() float64              { return a.SCALE }
func (a *ASCIIartist) GetContext() context.Context    { return a.ctx }
func (a *ASCIIartist) GetBoundaryForShape(s d2target.Shape) (asciiroute.Point, asciiroute.Point) {
	p1, p2 := a.GetBoundary(s)
	return asciiroute.Point{X: p1.X, Y: p1.Y}, asciiroute.Point{X: p2.X, Y: p2.Y}
}
func (a *ASCIIartist) CalibrateXY(x, y float64) (float64, float64) {
	return a.calibrateXY(x, y)
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

func (a *ASCIIartist) calculateExtendedBounds(diagram *d2target.Diagram) (tl, br d2target.Point) {
	tl, br = diagram.NestedBoundingBox()
	log.Debug(a.ctx, "initial bounding box", slog.Int("tl.X", tl.X), slog.Int("tl.Y", tl.Y), slog.Int("br.X", br.X), slog.Int("br.Y", br.Y))

	// Log each shape's contribution to bounds
	for i, shape := range diagram.Shapes {
		log.Debug(a.ctx, "shape bounds",
			slog.Int("index", i),
			slog.String("id", shape.ID),
			slog.String("label", shape.Label),
			slog.Int("pos.X", shape.Pos.X),
			slog.Int("pos.Y", shape.Pos.Y),
			slog.Int("width", shape.Width),
			slog.Int("height", shape.Height),
			slog.Int("right_edge", shape.Pos.X+shape.Width),
			slog.Int("bottom_edge", shape.Pos.Y+shape.Height))
	}

	for _, conn := range diagram.Connections {
		if conn.Label != "" && len(conn.Route) > 1 {
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

func (a *ASCIIartist) Render(ctx context.Context, diagram *d2target.Diagram, opts *RenderOpts) ([]byte, error) {
	if opts == nil {
		opts = &RenderOpts{}
	}

	if os.Getenv("DEBUG_ASCII") == "" {
		ctx = log.Leveled(ctx, slog.LevelInfo)
	}
	a.ctx = ctx
	chars := a.chars
	if opts.Charset == charset.ASCII {
		chars = charset.New(charset.ASCII)
	} else if opts.Charset == charset.Unicode {
		chars = charset.New(charset.Unicode)
	}
	originalChars := a.chars
	a.chars = chars
	defer func() {
		a.chars = originalChars
	}()
	xOffset := 0
	yOffset := 0
	a.diagram = *diagram
	tl, br := a.calculateExtendedBounds(diagram)
	log.Debug(ctx, "extended bounds calculated", slog.Int("tl.X", tl.X), slog.Int("tl.Y", tl.Y), slog.Int("br.X", br.X), slog.Int("br.Y", br.Y))
	if tl.X < 0 {
		xOffset = -tl.X
		br.X += -tl.X
		tl.X = 0
		log.Debug(ctx, "adjusted for negative X", slog.Int("xOffset", xOffset), slog.Int("new_br.X", br.X))
	}
	if tl.Y < 0 {
		yOffset = -tl.Y
		br.Y += -tl.Y
		tl.Y = 0
		log.Debug(ctx, "adjusted for negative Y", slog.Int("yOffset", yOffset), slog.Int("new_br.Y", br.Y))
	}
	w := int(math.Ceil(float64(br.X - tl.X)))
	h := int(math.Ceil(float64(br.Y - tl.Y)))
	log.Debug(ctx, "raw canvas dimensions", slog.Int("width", w), slog.Int("height", h))

	w = int(math.Round((float64(w) / a.FW) * a.SCALE))
	h = int(math.Round((float64(h) / a.FH) * a.SCALE))

	// Calculate the actual needed canvas size based on shape positions after offset adjustments
	canvasWidth := br.X + xOffset
	canvasHeight := br.Y + yOffset
	log.Debug(ctx, "calculated canvas size", slog.Int("canvasWidth", canvasWidth), slog.Int("canvasHeight", canvasHeight), slog.Int("xOffset", xOffset), slog.Int("yOffset", yOffset))

	// Use the larger of the two width calculations to ensure all content fits
	if w < canvasWidth {
		w = canvasWidth
		log.Debug(ctx, "using absolute canvas width", slog.Int("width", w))
	}
	if h < canvasHeight {
		h = canvasHeight
		log.Debug(ctx, "using absolute canvas height", slog.Int("height", h))
	}

	// Add minimal padding for label overflow
	padding := 2
	log.Debug(ctx, "canvas setup", slog.Int("padding", padding), slog.Int("final_width", w+padding), slog.Int("final_height", h+padding))

	a.canvas = asciicanvas.New(w+padding, h+padding)

	log.Debug(ctx, "processing shapes", slog.Int("count", len(diagram.Shapes)), slog.Int("xOffset", xOffset), slog.Int("yOffset", yOffset))
	for i, shape := range diagram.Shapes {
		log.Debug(ctx, "processing shape", slog.Int("index", i), slog.String("id", shape.ID), slog.String("type", shape.Type), slog.Float64("x", float64(shape.Pos.X)), slog.Float64("y", float64(shape.Pos.Y)), slog.Float64("width", float64(shape.Width)), slog.Float64("height", float64(shape.Height)))

		originalX, originalY := shape.Pos.X, shape.Pos.Y
		adjustedX := shape.Pos.X + xOffset
		adjustedY := shape.Pos.Y + yOffset
		log.Debug(ctx, "position adjusted", slog.Float64("originalX", float64(originalX)), slog.Float64("originalY", float64(originalY)), slog.Int("newX", adjustedX), slog.Int("newY", adjustedY))

		preserveWidth := hasConnectionsAtRightEdge(shape, diagram.Connections, a.FW)
		preserveHeight := hasConnectionsAtTopEdge(shape, diagram.Connections, a.FH)
		log.Debug(ctx, "edge connections", slog.Bool("preserveWidth", preserveWidth))

		shapeCtx := &asciishapes.Context{
			Canvas: a.canvas,
			Chars:  a.chars,
			FW:     a.FW,
			FH:     a.FH,
			Scale:  a.SCALE,
			Ctx:    ctx,
		}

		originalWidth := shape.Width
		adjustedWidth := shape.Width
		if preserveWidth && shape.Label != "" {
			wC := int(math.Round((float64(shape.Width) / a.FW) * a.SCALE))
			availableSpace := wC - len(shape.Label)
			log.Debug(ctx, "width preservation check", slog.Int("calibrated", wC), slog.Int("labelChars", len(shape.Label)), slog.Int("available", availableSpace))
			if availableSpace >= asciishapes.MinLabelPadding && availableSpace%2 == 1 {
				adjustedWidth += int(a.FW / a.SCALE)
				log.Debug(ctx, "width adjusted", slog.Int("originalWidth", originalWidth), slog.Int("newWidth", adjustedWidth))
			}
		}

		// For multiple shapes, expand to fill the entire space that would be occupied by the multiple effect
		drawX := float64(adjustedX)
		drawY := float64(adjustedY)
		drawWidth := float64(adjustedWidth)
		drawHeight := float64(shape.Height)

		if shape.Multiple {
			// ASCII rendering handles multiple shapes through character styling, not position/size offsets
			// No dimensional adjustments needed - just draw at original size and position
			log.Debug(ctx, "multiple shape - using original dimensions for ASCII", slog.String("id", shape.ID))
		}

		log.Debug(ctx, "final draw parameters", slog.Float64("x", drawX), slog.Float64("y", drawY), slog.Float64("width", drawWidth), slog.Float64("height", drawHeight), slog.String("label", shape.Label))

		log.Debug(ctx, "drawing shape", slog.String("type", shape.Type))
		switch shape.Type {
		case d2target.ShapeRectangle:
			asciishapes.DrawRect(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition, "", preserveHeight)
		case d2target.ShapeSquare:
			asciishapes.DrawRect(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition, "", preserveHeight)
		case d2target.ShapePage:
			asciishapes.DrawPage(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeHexagon:
			asciishapes.DrawHex(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapePerson:
			asciishapes.DrawPerson(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeStoredData:
			asciishapes.DrawStoredData(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeCylinder:
			asciishapes.DrawCylinder(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapePackage:
			asciishapes.DrawPackage(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeParallelogram:
			asciishapes.DrawParallelogram(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeQueue:
			asciishapes.DrawQueue(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeStep:
			asciishapes.DrawStep(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeCallout:
			asciishapes.DrawCallout(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeDocument:
			asciishapes.DrawDocument(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeDiamond:
			asciishapes.DrawDiamond(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeClass:
			asciishapes.DrawClass(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape)
		case d2target.ShapeSQLTable:
			asciishapes.DrawSQLTable(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape)
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
			asciishapes.DrawRect(shapeCtx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition, symbol, preserveHeight)
		}
	}
	for _, conn := range diagram.Connections {
		adjustedRoute := make([]*geo.Point, len(conn.Route))
		for i, r := range conn.Route {
			adjustedRoute[i] = &geo.Point{
				X: r.X + float64(xOffset),
				Y: r.Y + float64(yOffset),
			}
		}

		tempConn := conn
		tempConn.Route = adjustedRoute

		if conn.DstArrow == d2target.NoArrowhead && conn.SrcArrow == d2target.NoArrowhead {
			asciiroute.DrawRoute(a, tempConn)
		}
	}
	for _, conn := range diagram.Connections {
		if conn.DstArrow != d2target.NoArrowhead || conn.SrcArrow != d2target.NoArrowhead {
			adjustedRoute := make([]*geo.Point, len(conn.Route))
			for i, r := range conn.Route {
				adjustedRoute[i] = &geo.Point{
					X: r.X + float64(xOffset),
					Y: r.Y + float64(yOffset),
				}
			}

			tempConn := conn
			tempConn.Route = adjustedRoute

			asciiroute.DrawRoute(a, tempConn)
		}
	}
	return a.canvas.ToByteArray(a.chars), nil
}

func (a *ASCIIartist) calibrateXY(x, y float64) (float64, float64) {
	xC := float64(math.Round((x / a.FW) * a.SCALE))
	yC := float64(math.Round((y / a.FH) * a.SCALE))
	return xC, yC
}

func absInt(a int) int {
	return int(math.Abs(float64(a)))
}

func hasConnectionsAtRightEdge(shape d2target.Shape, connections []d2target.Connection, fontWidth float64) bool {
	shapeRight := float64(shape.Pos.X + shape.Width)
	shapeTop := float64(shape.Pos.Y)
	shapeBottom := float64(shape.Pos.Y + shape.Height)

	for _, conn := range connections {
		if len(conn.Route) == 0 {
			continue
		}

		firstPoint := conn.Route[0]
		lastPoint := conn.Route[len(conn.Route)-1]

		tolerance := fontWidth / 2

		if math.Abs(firstPoint.X-shapeRight) < tolerance &&
			firstPoint.Y >= shapeTop && firstPoint.Y <= shapeBottom {
			return true
		}

		if math.Abs(lastPoint.X-shapeRight) < tolerance &&
			lastPoint.Y >= shapeTop && lastPoint.Y <= shapeBottom {
			return true
		}
	}

	return false
}

func hasConnectionsAtTopEdge(shape d2target.Shape, connections []d2target.Connection, fontHeight float64) bool {
	shapeTop := float64(shape.Pos.Y)
	shapeLeft := float64(shape.Pos.X)
	shapeRight := float64(shape.Pos.X + shape.Width)

	for _, conn := range connections {
		if len(conn.Route) < 2 {
			continue
		}

		// Check if route has horizontal segments connecting to top edge
		for i := 0; i < len(conn.Route)-1; i++ {
			p1 := conn.Route[i]
			p2 := conn.Route[i+1]

			// Check if this is a horizontal segment
			if math.Abs(p1.Y-p2.Y) < 0.1 {
				segmentY := p1.Y
				segmentLeft := math.Min(p1.X, p2.X)
				segmentRight := math.Max(p1.X, p2.X)

				tolerance := fontHeight

				// Check if horizontal segment connects to shape's top edge
				if math.Abs(segmentY-shapeTop) < tolerance &&
					segmentRight >= shapeLeft && segmentLeft <= shapeRight {
					return true
				}
			}
		}
	}

	return false
}
