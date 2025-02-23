package d2ascii

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strings"

	"oss.terrastruct.com/d2/d2target"
)

// RenderOpts contains options for ASCII rendering
type RenderOpts struct {
	Pad   *int64   // Optional padding around the diagram
	Scale *float64 // Pixels per ASCII character ratio
}

// Render converts a D2 diagram into ASCII art
func Render(diagram *d2target.Diagram, opts *RenderOpts) ([]byte, error) {
	if opts == nil {
		opts = &RenderOpts{}
	}

	// Default padding matching d2svg
	pad := int(8)
	if opts.Pad != nil {
		pad = int(*opts.Pad)
	}

	// Scale for converting diagram coordinates to ASCII grid
	// Default: roughly 1 ASCII char = 8x4 pixels
	scale := struct{ x, y float64 }{8, 4}
	if opts.Scale != nil {
		s := *opts.Scale
		scale.x = s
		scale.y = s / 2 // Maintain aspect ratio
	}

	// Calculate canvas dimensions
	tl, br := diagram.NestedBoundingBox()
	width := int(math.Ceil(float64(br.X-tl.X+(pad*2)) / scale.x))
	height := int(math.Ceil(float64(br.Y-tl.Y+(pad*2)) / scale.y))

	// Create ASCII canvas
	canvas := NewCanvas(width, height)
	canvas.setScale(scale.x, scale.y)
	canvas.setOffset(-int(tl.X), -int(tl.Y))
	canvas.setPad(pad)

	// Draw shapes
	for _, shape := range diagram.Shapes {
		err := canvas.drawShape(shape)
		if err != nil {
			return nil, err
		}
	}

	// Draw connections
	for _, conn := range diagram.Connections {
		err := canvas.drawConnection(conn)
		if err != nil {
			return nil, err
		}
	}

	width, height = canvas.AutoSize()
	fmt.Println("==== ", canvas.w, canvas.h, "====")
	fmt.Println("==== ", width, height, "====")
	canvas.ReScale(width, height)

	return canvas.TrimBytes(), nil
}

// Canvas handles the ASCII grid and drawing operations
type TextPosition struct {
	x, y, w, h int
	text       string
}

type Canvas struct {
	grid [][]rune
	w, h int

	// Coordinate transformation
	scaleX, scaleY   float64
	offsetX, offsetY int
	pad              int

	// Track text positions
	textPositions []TextPosition
}

func NewCanvas(w, h int) *Canvas {
	grid := make([][]rune, h)
	for i := range grid {
		grid[i] = make([]rune, w)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	return &Canvas{
		grid: grid,
		w:    w,
		h:    h,
	}
}

func (c *Canvas) setScale(x, y float64) {
	c.scaleX = x
	c.scaleY = y
}

func (c *Canvas) setOffset(x, y int) {
	c.offsetX = x
	c.offsetY = y
}

func (c *Canvas) setPad(pad int) {
	c.pad = pad
}

// transformPoint converts diagram coordinates to ASCII grid coordinates
func (c *Canvas) transformPoint(x, y int) (int, int) {
	x = int(float64(x+c.offsetX+c.pad) / c.scaleX)
	y = int(float64(y+c.offsetY+c.pad) / c.scaleY)
	return x, y
}

func (c *Canvas) drawShape(shape d2target.Shape) error {
	x, y := c.transformPoint(int(shape.Pos.X), int(shape.Pos.Y))
	w := int(float64(shape.Width) / c.scaleX)
	h := int(float64(shape.Height) / c.scaleY)

	switch shape.Type {
	case d2target.ShapeCircle:
		return c.drawCircle(x, y, w, h, shape.Label)
	case d2target.ShapeSquare:
		return c.drawRect(x, y, w, h, shape.Label)
	// Add more shape types as needed
	default:
		return c.drawRect(x, y, w, h, shape.Label)
	}
}

func (c *Canvas) drawRect(x, y, w, h int, label string) error {
	// Draw corners
	c.set(x, y, '+')
	c.set(x+w, y, '+')
	c.set(x, y+h, '+')
	c.set(x+w, y+h, '+')

	// Draw horizontal edges
	for i := x + 1; i < x+w; i++ {
		c.set(i, y, '-')
		c.set(i, y+h, '-')
	}

	// Draw vertical edges
	for i := y + 1; i < y+h; i++ {
		c.set(x, i, '|')
		c.set(x+w, i, '|')
	}

	// Draw label
	if label != "" {
		c.drawCenteredText(x+1, y+1, w-1, h-1, label)
	}

	return nil
}

func (c *Canvas) drawCircle(x, y, w, h int, label string) error {
	// Approximate circle with ASCII characters
	c.set(x+w/2, y, '.')
	c.set(x+w/2, y+h, '\'')
	c.set(x, y+h/2, '(')
	c.set(x+w, y+h/2, ')')

	if label != "" {
		c.drawCenteredText(x+1, y+1, w-1, h-1, label)
	}

	return nil
}

func (c *Canvas) drawConnection(conn d2target.Connection) error {
	// Draw a simple line between points for now
	points := make([]struct{ x, y int }, len(conn.Route))
	for i, p := range conn.Route {
		points[i].x, points[i].y = c.transformPoint(int(p.X), int(p.Y))
	}

	for i := 0; i < len(points)-1; i++ {
		c.drawLine(points[i].x, points[i].y, points[i+1].x, points[i+1].y)
	}

	return nil
}

func (c *Canvas) drawLine(x1, y1, x2, y2 int) {
	// Draw horizontal line
	if y1 == y2 {
		for x := min(x1, x2); x <= max(x1, x2); x++ {
			c.set(x, y1, '-')
		}
		return
	}

	// Draw vertical line
	if x1 == x2 {
		for y := min(y1, y2); y <= max(y1, y2); y++ {
			c.set(x1, y, '|')
		}
		return
	}

	// Draw diagonal line
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	steep := dy > dx

	if steep {
		x1, y1 = y1, x1
		x2, y2 = y2, x2
	}
	if x1 > x2 {
		x1, x2 = x2, x1
		y1, y2 = y2, y1
	}

	dx = x2 - x1
	dy = abs(y2 - y1)
	err := dx / 2
	ystep := 1
	if y1 >= y2 {
		ystep = -1
	}

	for ; x1 <= x2; x1++ {
		if steep {
			c.set(y1, x1, '|')
		} else {
			c.set(x1, y1, '/')
		}
		err -= dy
		if err < 0 {
			y1 += ystep
			err += dx
		}
	}
}

func (c *Canvas) drawCenteredText(x, y, w, h int, text string) {
	// Record position first
	c.textPositions = append(c.textPositions, TextPosition{x, y, w, h, text})

	lines := strings.Split(text, "\n")
	startY := y + (h-len(lines))/2

	for i, line := range lines {
		if startY+i >= c.h {
			break
		}
		startX := x + (w-len(line))/2
		for j, ch := range line {
			if startX+j >= c.w {
				break
			}
			c.set(startX+j, startY+i, ch)
		}
	}
}

func (c *Canvas) set(x, y int, ch rune) {
	if x >= 0 && x < c.w && y >= 0 && y < c.h {
		c.grid[y][x] = ch
	}
}

func (c *Canvas) Bytes() []byte {
	var buf bytes.Buffer
	for _, row := range c.grid {
		buf.WriteString(string(row))
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// TrimBytes removes excess whitespace from all sides of the ASCII output
func (c *Canvas) TrimBytes() []byte {
	// Find bounds of content
	minX, minY, maxX, maxY := c.w, c.h, 0, 0

	// Scan for content bounds
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if c.grid[y][x] != ' ' {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}

	// If no content found, return empty
	if minX > maxX || minY > maxY {
		return []byte{}
	}

	// Create trimmed output
	var buf bytes.Buffer
	for y := minY; y <= maxY; y++ {
		buf.WriteString(string(c.grid[y][minX : maxX+1]))
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func (c *Canvas) AutoSize() (width, height int) {
	type boxInfo struct {
		x, y, w, h        int
		text              string
		hasUp, hasDown    bool
		hasLeft, hasRight bool
		hasDiagonal       bool
		originalWidth     int
	}

	boxes := make([]boxInfo, 0)

	// Collect boxes and their connections
	for _, pos := range c.textPositions {
		up, down, left, right, diag := false, false, false, false, false

		// Check surrounding area for connections
		checkRange := 2
		minX := max(0, pos.x-checkRange)
		maxX := min(c.w, pos.x+pos.w+checkRange)
		minY := max(0, pos.y-checkRange)
		maxY := min(c.h, pos.y+pos.h+checkRange)

		// Check vertical connections
		for x := pos.x; x < pos.x+pos.w; x++ {
			if pos.y > 0 && c.grid[pos.y-1][x] == '|' {
				up = true
			}
			if pos.y+pos.h < c.h && c.grid[pos.y+pos.h][x] == '|' {
				down = true
			}
		}

		// Check horizontal and diagonal connections
		for y := minY; y < maxY; y++ {
			for x := minX; x < maxX; x++ {
				ch := c.grid[y][x]
				switch ch {
				case '-':
					if x < pos.x {
						left = true
					} else if x >= pos.x+pos.w {
						right = true
					}
				case '/', '\\':
					diag = true
				}
			}
		}

		boxes = append(boxes, boxInfo{
			x:             pos.x,
			y:             pos.y,
			w:             pos.w,
			h:             pos.h,
			text:          pos.text,
			hasUp:         up,
			hasDown:       down,
			hasLeft:       left,
			hasRight:      right,
			hasDiagonal:   diag,
			originalWidth: pos.w,
		})
	}

	// Sort boxes vertically
	yBoxes := make([]boxInfo, len(boxes))
	copy(yBoxes, boxes)
	sort.Slice(yBoxes, func(i, j int) bool {
		return yBoxes[i].y < yBoxes[j].y
	})

	// Calculate vertical layout with increased padding
	currY := 0
	yMapping := make(map[int]int)

	for i, box := range yBoxes {
		lines := strings.Split(box.text, "\n")
		minHeight := len(lines) + 4 // padding + border

		if box.hasDiagonal { // Add extra height for diagonal connections
			minHeight += 2
		}

		if i == 0 {
			yMapping[box.y] = 2 // Start with some padding
			currY = minHeight + 2
			continue
		}

		spacing := 2 // spacing between boxes
		prevBox := yBoxes[i-1]

		// Add more spacing for connections
		if box.hasUp || prevBox.hasDown {
			spacing = 3
		}
		if box.hasDiagonal || prevBox.hasDiagonal {
			spacing = 4
		}

		yMapping[box.y] = currY + spacing
		currY = yMapping[box.y] + minHeight
	}

	// Calculate final height
	maxH := 0
	for _, box := range yBoxes {
		newY := yMapping[box.y]
		lines := strings.Split(box.text, "\n")
		boxHeight := len(lines) + 4 // padding + border

		if box.hasDiagonal {
			boxHeight += 2
		}

		maxH = max(maxH, newY+boxHeight)
	}

	// Add extra vertical padding for top/bottom connections
	topPad := 2
	bottomPad := 2
	for x := 0; x < c.w; x++ {
		if c.grid[0][x] != ' ' {
			topPad = 3
		}
		if c.grid[c.h-1][x] != ' ' {
			bottomPad = 3
		}
	}
	maxH += topPad + bottomPad

	// preserve the original width of each box
	// but ensure it's wide enough for the content
	maxW := 0
	for _, box := range boxes {
		// Calculate minimum width needed for text
		lines := strings.Split(box.text, "\n")
		textWidth := 0
		for _, line := range lines {
			textWidth = max(textWidth, len(line))
		}

		requiredWidth := textWidth + 4 // Base padding

		// Add extra width for connections
		if box.hasLeft {
			requiredWidth += 2
		}
		if box.hasRight {
			requiredWidth += 2
		}
		if box.hasDiagonal {
			requiredWidth += 4
		}

		// Use the larger of required width or original width
		effectiveWidth := max(requiredWidth, box.originalWidth)
		maxW = max(maxW, box.x+effectiveWidth)
	}

	// Add padding for edge connections
	leftPad := 2
	rightPad := 2
	for y := 0; y < c.h; y++ {
		if c.grid[y][0] != ' ' {
			leftPad = max(leftPad, 3)
		}
		if c.grid[y][c.w-1] != ' ' {
			rightPad = max(rightPad, 3)
		}
	}
	maxW += leftPad + rightPad

	return min(c.w, maxW), min(c.h, maxH)
}

// ReScale reduces the size of ASCII art using a pixel-like sampling technique
func (c *Canvas) ReScale(targetWidth, targetHeight int) {
	scaleX := float64(targetWidth) / float64(c.w)
	scaleY := float64(targetHeight) / float64(c.h)

	// Create new grid
	newGrid := make([][]rune, targetHeight)
	for i := range newGrid {
		newGrid[i] = make([]rune, targetWidth)
		for j := range newGrid[i] {
			newGrid[i][j] = ' '
		}
	}

	// First scale the borders and lines (source -> target mapping)
	for y := 0; y < c.h; y++ {
		targetY := int(float64(y) * scaleY)
		if targetY >= targetHeight {
			continue
		}

		for x := 0; x < c.w; x++ {
			targetX := int(float64(x) * scaleX)
			if targetX >= targetWidth {
				continue
			}

			ch := c.grid[y][x]
			if ch == '+' || ch == '-' || ch == '|' || ch == '/' || ch == '\\' || ch == '.' {
				newGrid[targetY][targetX] = ch
			}
		}
	}

	// Then redraw text at scaled positions
	for _, label := range c.textPositions {
		// Get box dimensions in source coordinates first
		srcBoxCenterY := label.y + label.h/2

		// Split text into lines
		lines := strings.Split(label.text, "\n")
		textHeight := len(lines)

		// Calculate text start Y in source coordinates
		srcStartY := srcBoxCenterY - textHeight/2

		// Scale to target coordinates
		newX := int(float64(label.x) * scaleX)
		newY := int(float64(srcStartY) * scaleY)
		newW := int(float64(label.w) * scaleX)

		// Draw each line centered horizontally
		for i, line := range lines {
			targetY := newY + i
			if targetY >= targetHeight {
				break
			}
			if targetY < 0 {
				continue
			}

			// Center text horizontally within the scaled box
			startX := newX + (newW-len(line))/2
			for j, ch := range line {
				targetX := startX + j
				if targetX >= targetWidth {
					break
				}
				if targetX < 0 {
					continue
				}

				// Only overwrite space or existing text
				existing := newGrid[targetY][targetX]
				if existing == ' ' || (existing != '+' && existing != '-' &&
					existing != '|' && existing != '/' && existing != '\\' &&
					existing != '.') {
					newGrid[targetY][targetX] = ch
				}
			}
		}
	}

	c.grid = newGrid
	c.w = targetWidth
	c.h = targetHeight
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
