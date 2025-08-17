package d2ascii

import (
	"fmt"
	"math"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciicanvas"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciiroute"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciishapes"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
	"oss.terrastruct.com/d2/d2target"
)

const (
	defaultFontWidth  = 9.75
	defaultFontHeight = 18.0
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
	fmt.Printf("\033[36m[D2ASCII-SHAPE] GetBoundary for shape %s (%s)\033[0m\n", s.ID, s.Type)
	
	// For multiple shapes, expand boundary to match the expanded rendering
	posX := float64(s.Pos.X)
	posY := float64(s.Pos.Y)
	width := float64(s.Width)
	height := float64(s.Height)
	fmt.Printf("\033[36m[D2ASCII-SHAPE]   Original dimensions: (%.0f,%.0f) %.0fx%.0f\033[0m\n", 
		posX, posY, width, height)

	if s.Multiple {
		fmt.Printf("\033[36m[D2ASCII-SHAPE]   Multiple shape, expanding boundary by %d\033[0m\n", d2target.MULTIPLE_OFFSET)
		posX -= d2target.MULTIPLE_OFFSET   // Move left to include shadow area
		width += d2target.MULTIPLE_OFFSET  // Include shadow width
		height += d2target.MULTIPLE_OFFSET // Include shadow height
		fmt.Printf("\033[36m[D2ASCII-SHAPE]   Expanded dimensions: (%.0f,%.0f) %.0fx%.0f\033[0m\n", 
			posX, posY, width, height)
	}

	// Use the same calibration logic as the drawing functions
	ctx := &asciishapes.Context{
		Canvas: a.canvas,
		Chars:  a.chars,
		FW:     a.FW,
		FH:     a.FH,
		Scale:  a.SCALE,
	}
	x1, y1, wC, hC := ctx.Calibrate(posX, posY, width, height)
	
	
	// Apply the same width adjustments as the drawing code
	preserveWidth := hasConnectionsAtRightEdge(s, a.diagram.Connections, a.FW)
	preserveHeight := hasConnectionsAtTopEdge(s, a.diagram.Connections, a.FH)
	if preserveWidth && s.Label != "" {
		availableSpace := wC - len(s.Label)
		if availableSpace >= asciishapes.MinLabelPadding && availableSpace%2 == 1 {
			// Adjust the original width before recalibrating
			width += float64(int(a.FW / a.SCALE))
			x1, y1, wC, hC = ctx.Calibrate(posX, posY, width, height)
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
	wC = asciishapes.AdjustWidthForLabel(ctx, posX, posY, width, height, wC, s.Label)
	
	x2, y2 := x1+wC, y1+hC
	
	fmt.Printf("\033[36m[D2ASCII-SHAPE]   Boundary matches actual draw bounds: (%d,%d)-(%d,%d) [%dx%d]\033[0m\n", 
		x1, y1, x2, y2, wC, hC)

	return Point{X: x1, Y: y1}, Point{X: x2, Y: y2}
}

func (a *ASCIIartist) GetCanvas() *asciicanvas.Canvas { return a.canvas }
func (a *ASCIIartist) GetChars() charset.Set          { return a.chars }
func (a *ASCIIartist) GetDiagram() *d2target.Diagram  { return &a.diagram }
func (a *ASCIIartist) GetFontWidth() float64          { return a.FW }
func (a *ASCIIartist) GetFontHeight() float64         { return a.FH }
func (a *ASCIIartist) GetScale() float64              { return a.SCALE }
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

func (a *ASCIIartist) Render(diagram *d2target.Diagram, opts *RenderOpts) ([]byte, error) {
	if opts == nil {
		opts = &RenderOpts{}
	}
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

	maxLabelLen := 0
	for _, shape := range diagram.Shapes {
		if len(shape.Label) > maxLabelLen {
			maxLabelLen = len(shape.Label)
		}
	}
	padding := maxLabelLen + asciishapes.MinLabelPadding
	fmt.Printf("\033[36m[D2ASCII-SHAPE] Canvas padding calculated: maxLabelLen=%d, padding=%d\033[0m\n", maxLabelLen, padding)
	fmt.Printf("\033[36m[D2ASCII-SHAPE] Creating canvas: %dx%d (base: %dx%d + padding)\033[0m\n", w+padding+1, h+padding+1, w, h)

	a.canvas = asciicanvas.New(w+padding+1, h+padding+1)

	fmt.Printf("\033[36m[D2ASCII-SHAPE] Processing %d shapes with offset (%d, %d)\033[0m\n", len(diagram.Shapes), xOffset, yOffset)
	for i, shape := range diagram.Shapes {
		fmt.Printf("\033[36m[D2ASCII-SHAPE] Shape %d (%s): %s at (%.0f,%.0f) size %.0fx%.0f\033[0m\n", 
			i, shape.ID, shape.Type, float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height))
		
		originalX, originalY := shape.Pos.X, shape.Pos.Y
		shape.Pos.X += xOffset
		shape.Pos.Y += yOffset
		fmt.Printf("\033[36m[D2ASCII-SHAPE]   Position adjusted: (%.0f,%.0f) -> (%d,%d)\033[0m\n", 
			float64(originalX), float64(originalY), shape.Pos.X, shape.Pos.Y)

		preserveWidth := hasConnectionsAtRightEdge(shape, diagram.Connections, a.FW)
		preserveHeight := hasConnectionsAtTopEdge(shape, diagram.Connections, a.FH)
		fmt.Printf("\033[36m[D2ASCII-SHAPE]   Right edge connections detected: %t\033[0m\n", preserveWidth)

		ctx := &asciishapes.Context{
			Canvas: a.canvas,
			Chars:  a.chars,
			FW:     a.FW,
			FH:     a.FH,
			Scale:  a.SCALE,
		}

		originalWidth := shape.Width
		if preserveWidth && shape.Label != "" {
			wC := int(math.Round((float64(shape.Width) / a.FW) * a.SCALE))
			availableSpace := wC - len(shape.Label)
			fmt.Printf("\033[36m[D2ASCII-SHAPE]   Width preservation check: calibrated=%d, label=%d chars, available=%d\033[0m\n", 
				wC, len(shape.Label), availableSpace)
			if availableSpace >= asciishapes.MinLabelPadding && availableSpace%2 == 1 {
				shape.Width += int(a.FW / a.SCALE)
				fmt.Printf("\033[36m[D2ASCII-SHAPE]   Width adjusted for even spacing: %d -> %d\033[0m\n", 
					originalWidth, shape.Width)
			}
		}

		// For multiple shapes, expand to fill the entire space that would be occupied by the multiple effect
		drawX := float64(shape.Pos.X)
		drawY := float64(shape.Pos.Y)
		drawWidth := float64(shape.Width)
		drawHeight := float64(shape.Height)

		if shape.Multiple {
			fmt.Printf("\033[36m[D2ASCII-SHAPE]   Multiple shape adjustments: offset=%d\033[0m\n", d2target.MULTIPLE_OFFSET)
			// Move position to top-left of total occupied area (shadow extends left and down)
			drawX -= d2target.MULTIPLE_OFFSET // Move left to include shadow area
			// Y stays the same since shadow goes down, not up

			// Expand size to fill entire multiple effect area
			drawWidth += d2target.MULTIPLE_OFFSET  // Include shadow width
			drawHeight += d2target.MULTIPLE_OFFSET // Include shadow height
			fmt.Printf("\033[36m[D2ASCII-SHAPE]   Multiple dimensions: (%.0f,%.0f) %.0fx%.0f -> (%.0f,%.0f) %.0fx%.0f\033[0m\n", 
				float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height),
				drawX, drawY, drawWidth, drawHeight)
		}

		fmt.Printf("\033[36m[D2ASCII-SHAPE]   Final draw parameters: (%.0f,%.0f) %.0fx%.0f, label='%s'\033[0m\n", 
			drawX, drawY, drawWidth, drawHeight, shape.Label)

		fmt.Printf("\033[36m[D2ASCII-SHAPE]   Drawing shape type: %s\033[0m\n", shape.Type)
		switch shape.Type {
		case d2target.ShapeRectangle:
			fmt.Printf("\033[36m[D2ASCII-SHAPE]   -> DrawRect\033[0m\n")
			asciishapes.DrawRect(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition, "", preserveHeight)
		case d2target.ShapeSquare:
			fmt.Printf("\033[36m[D2ASCII-SHAPE]   -> DrawRect (square)\033[0m\n")
			asciishapes.DrawRect(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition, "", preserveHeight)
		case d2target.ShapePage:
			asciishapes.DrawPage(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeHexagon:
			asciishapes.DrawHex(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapePerson:
			asciishapes.DrawPerson(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeStoredData:
			asciishapes.DrawStoredData(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeCylinder:
			asciishapes.DrawCylinder(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapePackage:
			asciishapes.DrawPackage(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeParallelogram:
			asciishapes.DrawParallelogram(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeQueue:
			asciishapes.DrawQueue(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeStep:
			asciishapes.DrawStep(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeCallout:
			asciishapes.DrawCallout(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeDocument:
			asciishapes.DrawDocument(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
		case d2target.ShapeDiamond:
			asciishapes.DrawDiamond(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition)
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
			asciishapes.DrawRect(ctx, drawX, drawY, drawWidth, drawHeight, shape.Label, shape.LabelPosition, symbol, preserveHeight)
		}
	}
	for _, conn := range diagram.Connections {
		for _, r := range conn.Route {
			r.X += float64(xOffset)
			r.Y += float64(yOffset)
		}
		if conn.DstArrow == d2target.NoArrowhead && conn.SrcArrow == d2target.NoArrowhead {
			asciiroute.DrawRoute(a, conn)
		}
	}
	for _, conn := range diagram.Connections {
		if conn.DstArrow != d2target.NoArrowhead || conn.SrcArrow != d2target.NoArrowhead {
			asciiroute.DrawRoute(a, conn)
		}
	}
	return a.canvas.ToByteArray(a.chars), nil
}

func (a *ASCIIartist) calibrateXY(x, y float64) (float64, float64) {
	xC := float64(math.Round((x / a.FW) * a.SCALE))
	yC := float64(math.Round((y / a.FH) * a.SCALE))
	fmt.Printf("[D2ASCII]     CalibrateXY: (%.2f, %.2f) -> (%.2f, %.2f) [FW=%.2f, FH=%.2f, SCALE=%.2f]\n", 
		x, y, xC, yC, a.FW, a.FH, a.SCALE)
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
					fmt.Printf("\033[36m[D2ASCII-SHAPE]     Found horizontal top connection: segment Y=%.2f vs shape Y=%.2f\033[0m\n", 
						segmentY, shapeTop)
					return true
				}
			}
		}
	}

	return false
}
