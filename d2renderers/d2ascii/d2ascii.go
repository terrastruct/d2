// d2ascii implements an ASCII art renderer for d2 diagrams.
// The input is d2exporter's output
package d2ascii

import (
	"math"
	"sort"
	"strings"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
)

const (
	DEFAULT_PADDING = 2
)

type RenderOpts struct {
	Pad *int64
}

// Grid represents a 2D character grid for ASCII art
type Grid struct {
	Width  int
	Height int
	Cells  [][]rune
}

// NewGrid creates a new grid with the specified dimensions
func NewGrid(width, height int) *Grid {
	cells := make([][]rune, height)
	for i := range cells {
		cells[i] = make([]rune, width)
		for j := range cells[i] {
			cells[i][j] = ' '
		}
	}
	return &Grid{
		Width:  width,
		Height: height,
		Cells:  cells,
	}
}

// SetCell sets a character at the specified position
func (g *Grid) SetCell(x, y int, char rune) {
	if x >= 0 && x < g.Width && y >= 0 && y < g.Height {
		g.Cells[y][x] = char
	}
}

// GetCell gets a character at the specified position
func (g *Grid) GetCell(x, y int) rune {
	if x >= 0 && x < g.Width && y >= 0 && y < g.Height {
		return g.Cells[y][x]
	}
	return ' '
}

// String converts the grid to a string representation
func (g *Grid) String() string {
	var lines []string
	for _, row := range g.Cells {
		lines = append(lines, strings.TrimRight(string(row), " "))
	}
	
	// Remove leading empty lines
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	
	// Remove trailing empty lines
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	
	return strings.Join(lines, "\n")
}

// DiagramObject interface for sorting objects by z-index
type DiagramObject interface {
	GetID() string
	GetZIndex() int
}

func dimensions(diagram *d2target.Diagram, pad int) (left, top, width, height int) {
	tl, br := diagram.BoundingBox()
	left = tl.X - pad
	top = tl.Y - pad
	width = br.X - tl.X + pad*2
	height = br.Y - tl.Y + pad*2
	return left, top, width, height
}

func sortObjects(allObjects []DiagramObject) {
	sort.SliceStable(allObjects, func(i, j int) bool {
		// first sort by zIndex
		iZIndex := allObjects[i].GetZIndex()
		jZIndex := allObjects[j].GetZIndex()
		if iZIndex != jZIndex {
			return iZIndex < jZIndex
		}

		// then, if both are shapes, parents come before their children
		iShape, iIsShape := allObjects[i].(d2target.Shape)
		jShape, jIsShape := allObjects[j].(d2target.Shape)
		if iIsShape && jIsShape {
			return iShape.Level < jShape.Level
		}

		// then, shapes come before connections
		_, jIsConnection := allObjects[j].(d2target.Connection)
		return iIsShape && jIsConnection
	})
}

// Render renders a D2 diagram as ASCII art
func Render(diagram *d2target.Diagram, opts *RenderOpts) ([]byte, error) {
	pad := DEFAULT_PADDING
	if opts != nil && opts.Pad != nil {
		pad = int(*opts.Pad)
	}

	left, top, width, height := dimensions(diagram, pad)
	
	// Convert pixel dimensions to character dimensions
	// Approximate character size: 8x16 pixels
	charWidth := int(math.Ceil(float64(width) / 8.0))
	charHeight := int(math.Ceil(float64(height) / 16.0))
	
	// Ensure minimum grid size
	if charWidth < 1 {
		charWidth = 1
	}
	if charHeight < 1 {
		charHeight = 1
	}
	
	grid := NewGrid(charWidth, charHeight)

	// Collect all objects for z-index sorting
	allObjects := make([]DiagramObject, 0, len(diagram.Shapes)+len(diagram.Connections))
	for _, s := range diagram.Shapes {
		allObjects = append(allObjects, s)
	}
	for _, c := range diagram.Connections {
		allObjects = append(allObjects, c)
	}

	sortObjects(allObjects)

	// Render objects in z-index order
	for _, obj := range allObjects {
		if shape, isShape := obj.(d2target.Shape); isShape {
			drawShape(grid, shape, left, top)
		} else if conn, isConnection := obj.(d2target.Connection); isConnection {
			drawConnection(grid, conn, left, top)
		}
	}

	result := grid.String()
	if len(result) > 0 && result[len(result)-1] != '\n' {
		result += "\n"
	}
	
	return []byte(result), nil
}

// drawShape renders a shape as ASCII art
func drawShape(grid *Grid, shape d2target.Shape, offsetX, offsetY int) {
	// Convert pixel coordinates to character coordinates
	x := (shape.Pos.X - offsetX) / 8
	y := (shape.Pos.Y - offsetY) / 16
	w := shape.Width / 8
	h := shape.Height / 16
	
	// Ensure minimum size
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	switch shape.Type {
	case d2target.ShapeRectangle, d2target.ShapeSequenceDiagram, d2target.ShapeHierarchy, "":
		drawRectangle(grid, x, y, w, h, shape)
	case d2target.ShapeCircle:
		drawCircle(grid, x, y, w, h, shape)
	case d2target.ShapeOval:
		drawOval(grid, x, y, w, h, shape)
	case d2target.ShapeHexagon:
		drawHexagon(grid, x, y, w, h, shape)
	case d2target.ShapeDiamond:
		drawDiamond(grid, x, y, w, h, shape)
	case d2target.ShapeParallelogram:
		drawParallelogram(grid, x, y, w, h, shape)
	case d2target.ShapeCylinder:
		drawCylinder(grid, x, y, w, h, shape)
	case d2target.ShapeQueue:
		drawQueue(grid, x, y, w, h, shape)
	case d2target.ShapePackage:
		drawPackage(grid, x, y, w, h, shape)
	case d2target.ShapeStep:
		drawStep(grid, x, y, w, h, shape)
	case d2target.ShapeCallout:
		drawCallout(grid, x, y, w, h, shape)
	case d2target.ShapeStoredData:
		drawStoredData(grid, x, y, w, h, shape)
	case d2target.ShapePerson:
		drawPerson(grid, x, y, w, h, shape)
	case d2target.ShapeCloud:
		drawCloud(grid, x, y, w, h, shape)
	default:
		// Default to rectangle for unknown shapes
		drawRectangle(grid, x, y, w, h, shape)
	}
}

// drawRectangle draws a rectangular shape
func drawRectangle(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Draw corners and edges
	for i := 0; i < w; i++ {
		for j := 0; j < h; j++ {
			var char rune
			if (i == 0 || i == w-1) && (j == 0 || j == h-1) {
				// Corners
				if i == 0 && j == 0 {
					char = '╭'
				} else if i == w-1 && j == 0 {
					char = '╮'
				} else if i == 0 && j == h-1 {
					char = '╰'
				} else {
					char = '╯'
				}
			} else if i == 0 || i == w-1 {
				// Vertical edges
				char = '│'
			} else if j == 0 || j == h-1 {
				// Horizontal edges
				char = '─'
			} else {
				// Interior (don't overwrite)
				continue
			}
			grid.SetCell(x+i, y+j, char)
		}
	}
	
	// Add label if present
	if shape.Label != "" {
		drawLabel(grid, x, y, w, h, shape.Label)
	}
}

// drawCircle draws a circular shape (approximated with ASCII)
func drawCircle(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// For small circles, use a simple representation
	if w <= 3 || h <= 3 {
		grid.SetCell(x+w/2, y+h/2, '●')
		if shape.Label != "" {
			drawLabel(grid, x, y, w, h, shape.Label)
		}
		return
	}
	
	// Draw circle outline
	radiusX := w / 2
	radiusY := h / 2
	
	for i := 0; i < w; i++ {
		for j := 0; j < h; j++ {
			dx := float64(i - w/2)
			dy := float64(j - h/2)
			distance := math.Sqrt((dx*dx)/(float64(radiusX*radiusX)) + (dy*dy)/(float64(radiusY*radiusY)))
			
			if distance >= 0.8 && distance <= 1.2 {
				grid.SetCell(x+i, y+j, '●')
			}
		}
	}
	
	if shape.Label != "" {
		drawLabel(grid, x, y, w, h, shape.Label)
	}
}

// drawOval draws an oval shape
func drawOval(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Similar to circle but with different aspect ratio handling
	drawCircle(grid, x, y, w, h, shape)
}

// drawHexagon draws a hexagonal shape
func drawHexagon(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Simple hexagon representation
	if w < 3 || h < 3 {
		drawRectangle(grid, x, y, w, h, shape)
		return
	}
	
	// Top and bottom edges
	for i := w/4; i < w-w/4; i++ {
		grid.SetCell(x+i, y, '─')
		grid.SetCell(x+i, y+h-1, '─')
	}
	
	// Side edges
	for j := 1; j < h-1; j++ {
		if j < h/2 {
			// Upper half
			grid.SetCell(x+j*w/(h*2), y+j, '/')
			grid.SetCell(x+w-1-j*w/(h*2), y+j, '\\')
		} else {
			// Lower half
			grid.SetCell(x+(h-1-j)*w/(h*2), y+j, '\\')
			grid.SetCell(x+w-1-(h-1-j)*w/(h*2), y+j, '/')
		}
	}
	
	if shape.Label != "" {
		drawLabel(grid, x, y, w, h, shape.Label)
	}
}

// drawDiamond draws a diamond shape
func drawDiamond(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Draw diamond outline
	for i := 0; i < w; i++ {
		for j := 0; j < h; j++ {
			dx := abs(i - w/2)
			dy := abs(j - h/2)
			
			if dx+dy*w/h == w/2 || (dx+dy*w/h >= w/2-1 && dx+dy*w/h <= w/2+1) {
				grid.SetCell(x+i, y+j, '◆')
			}
		}
	}
	
	if shape.Label != "" {
		drawLabel(grid, x, y, w, h, shape.Label)
	}
}

// drawParallelogram draws a parallelogram shape
func drawParallelogram(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	offset := w / 4
	
	// Top edge
	for i := offset; i < w; i++ {
		grid.SetCell(x+i, y, '─')
	}
	
	// Bottom edge
	for i := 0; i < w-offset; i++ {
		grid.SetCell(x+i, y+h-1, '─')
	}
	
	// Side edges
	for j := 1; j < h-1; j++ {
		leftX := x + offset - (j * offset / h)
		rightX := x + w - (j * offset / h)
		grid.SetCell(leftX, y+j, '/')
		grid.SetCell(rightX, y+j, '\\')
	}
	
	if shape.Label != "" {
		drawLabel(grid, x, y, w, h, shape.Label)
	}
}

// drawCylinder draws a cylinder shape
func drawCylinder(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Top ellipse
	for i := 1; i < w-1; i++ {
		grid.SetCell(x+i, y, '─')
	}
	
	// Side edges
	for j := 1; j < h-1; j++ {
		grid.SetCell(x, y+j, '│')
		grid.SetCell(x+w-1, y+j, '│')
	}
	
	// Bottom ellipse
	for i := 1; i < w-1; i++ {
		grid.SetCell(x+i, y+h-1, '─')
	}
	
	// Curved ends
	grid.SetCell(x, y, '╭')
	grid.SetCell(x+w-1, y, '╮')
	grid.SetCell(x, y+h-1, '╰')
	grid.SetCell(x+w-1, y+h-1, '╯')
	
	if shape.Label != "" {
		drawLabel(grid, x, y, w, h, shape.Label)
	}
}

// drawQueue draws a queue shape
func drawQueue(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Draw as rectangle with special queue symbol
	drawRectangle(grid, x, y, w, h, shape)
	
	// Add queue indicator
	if w > 2 && h > 2 {
		grid.SetCell(x+1, y+h/2, '→')
	}
}

// drawPackage draws a package shape
func drawPackage(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Draw main rectangle
	drawRectangle(grid, x, y, w, h, shape)
	
	// Add package tab
	if w > 4 && h > 2 {
		tabW := w / 3
		for i := 0; i < tabW; i++ {
			grid.SetCell(x+i, y-1, '─')
		}
		grid.SetCell(x, y-1, '┌')
		grid.SetCell(x+tabW-1, y-1, '┐')
		grid.SetCell(x+tabW-1, y, '┘')
	}
}

// drawStep draws a step shape
func drawStep(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Draw as rectangle with step indicators
	drawRectangle(grid, x, y, w, h, shape)
	
	// Add step number if available
	if w > 2 && h > 2 {
		grid.SetCell(x+1, y+1, '①')
	}
}

// drawCallout draws a callout shape
func drawCallout(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Draw main rectangle
	drawRectangle(grid, x, y, w, h, shape)
	
	// Add callout pointer
	if h > 2 {
		grid.SetCell(x-1, y+h/2, '◄')
	}
}

// drawStoredData draws a stored data shape
func drawStoredData(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Draw as cylinder (similar representation)
	drawCylinder(grid, x, y, w, h, shape)
}

// drawPerson draws a person shape
func drawPerson(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Simple person representation
	if w < 3 || h < 3 {
		grid.SetCell(x+w/2, y+h/2, '👤')
		return
	}
	
	// Head
	grid.SetCell(x+w/2, y, '●')
	
	// Body
	for j := 1; j < h-1; j++ {
		grid.SetCell(x+w/2, y+j, '│')
	}
	
	// Arms
	if h > 2 {
		grid.SetCell(x+w/2-1, y+h/3, '─')
		grid.SetCell(x+w/2+1, y+h/3, '─')
	}
	
	// Legs
	if h > 3 {
		grid.SetCell(x+w/2-1, y+h-1, '/')
		grid.SetCell(x+w/2+1, y+h-1, '\\')
	}
	
	if shape.Label != "" {
		drawLabel(grid, x, y, w, h, shape.Label)
	}
}

// drawCloud draws a cloud shape
func drawCloud(grid *Grid, x, y, w, h int, shape d2target.Shape) {
	// Simple cloud representation
	if w < 5 || h < 3 {
		grid.SetCell(x+w/2, y+h/2, '☁')
		return
	}
	
	// Cloud outline with bumps
	for i := 1; i < w-1; i++ {
		if i%2 == 0 {
			grid.SetCell(x+i, y, '~')
			grid.SetCell(x+i, y+h-1, '~')
		} else {
			grid.SetCell(x+i, y, '∼')
			grid.SetCell(x+i, y+h-1, '∼')
		}
	}
	
	for j := 1; j < h-1; j++ {
		grid.SetCell(x, y+j, '(')
		grid.SetCell(x+w-1, y+j, ')')
	}
	
	if shape.Label != "" {
		drawLabel(grid, x, y, w, h, shape.Label)
	}
}

// drawConnection renders a connection as ASCII art
func drawConnection(grid *Grid, conn d2target.Connection, offsetX, offsetY int) {
	if len(conn.Route) < 2 {
		return
	}
	
	// Convert route points to character coordinates
	points := make([]geo.Point, len(conn.Route))
	for i, p := range conn.Route {
		points[i] = geo.Point{
			X: (p.X - float64(offsetX)) / 8,
			Y: (p.Y - float64(offsetY)) / 16,
		}
	}
	
	// Draw line segments
	for i := 0; i < len(points)-1; i++ {
		drawLine(grid, points[i], points[i+1])
	}
	
	// Draw arrowheads
	if conn.DstArrow != "" {
		drawArrowhead(grid, points[len(points)-2], points[len(points)-1], true)
	}
	if conn.SrcArrow != "" {
		drawArrowhead(grid, points[1], points[0], false)
	}
	
	// Draw label if present
	if conn.Label != "" {
		midPoint := len(points) / 2
		if midPoint < len(points) {
			drawConnectionLabel(grid, points[midPoint], conn.Label)
		}
	}
}

// drawLine draws a line between two points
func drawLine(grid *Grid, start, end geo.Point) {
	x0, y0 := int(start.X), int(start.Y)
	x1, y1 := int(end.X), int(end.Y)
	
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	
	if dx == 0 {
		// Vertical line
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		for y := y0; y <= y1; y++ {
			grid.SetCell(x0, y, '│')
		}
	} else if dy == 0 {
		// Horizontal line
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		for x := x0; x <= x1; x++ {
			grid.SetCell(x, y0, '─')
		}
	} else {
		// Diagonal line - use Bresenham's algorithm
		steep := dy > dx
		if steep {
			x0, y0 = y0, x0
			x1, y1 = y1, x1
			dx, dy = dy, dx
		}
		
		if x0 > x1 {
			x0, x1 = x1, x0
			y0, y1 = y1, y0
		}
		
		ystep := 1
		if y0 > y1 {
			ystep = -1
		}
		
		err := dx / 2
		y := y0
		
		for x := x0; x <= x1; x++ {
			if steep {
				if y1 > y0 {
					grid.SetCell(y, x, '/')
				} else {
					grid.SetCell(y, x, '\\')
				}
			} else {
				if y1 > y0 {
					grid.SetCell(x, y, '\\')
				} else {
					grid.SetCell(x, y, '/')
				}
			}
			
			err -= dy
			if err < 0 {
				y += ystep
				err += dx
			}
		}
	}
}

// drawArrowhead draws an arrowhead at the end of a line
func drawArrowhead(grid *Grid, from, to geo.Point, isTarget bool) {
	x, y := int(to.X), int(to.Y)
	
	// Determine direction
	dx := to.X - from.X
	dy := to.Y - from.Y
	
	var arrowChar rune
	if abs(int(dx)) > abs(int(dy)) {
		// Horizontal arrow
		if dx > 0 {
			arrowChar = '►'
		} else {
			arrowChar = '◄'
		}
	} else {
		// Vertical arrow
		if dy > 0 {
			arrowChar = '▼'
		} else {
			arrowChar = '▲'
		}
	}
	
	grid.SetCell(x, y, arrowChar)
}

// drawLabel draws text label centered in the given area
func drawLabel(grid *Grid, x, y, w, h int, label string) {
	if label == "" {
		return
	}
	
	// Simple label placement in center
	lines := strings.Split(label, "\n")
	startY := y + (h-len(lines))/2
	
	for i, line := range lines {
		if startY+i >= 0 && startY+i < grid.Height {
			startX := x + (w-len(line))/2
			for j, char := range line {
				if startX+j >= 0 && startX+j < grid.Width {
					grid.SetCell(startX+j, startY+i, char)
				}
			}
		}
	}
}

// drawConnectionLabel draws a connection label
func drawConnectionLabel(grid *Grid, pos geo.Point, label string) {
	x, y := int(pos.X), int(pos.Y)
	
	// Simple label placement
	for i, char := range label {
		grid.SetCell(x+i, y, char)
	}
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
} 