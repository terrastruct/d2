package d2ascii

import (
	"bytes"
	"fmt"
	"math"
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

	const ( // common terminal size
		maxWidth  = 120
		maxHeight = 90
	) // TODO: detect smallest shape then make it as a baseline

	width = min(canvas.w, maxWidth)
	height = min(canvas.h, maxHeight)

	fmt.Println("==== ", canvas.w, canvas.h, "====")
	fmt.Println("==== ", width, height, "====")
	canvas.ReScale(width, height)

	return canvas.TrimBytes(), nil
}

// Canvas handles the ASCII grid and drawing operations
type Canvas struct {
	grid [][]rune
	w, h int

	// Coordinate transformation
	scaleX, scaleY   float64
	offsetX, offsetY int
	pad              int
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

// ReScale reduces the size of ASCII art using a pixel-like sampling technique
// BUG: somehow the text label disappear 😂
func (c *Canvas) ReScale(targetWidth, targetHeight int) {
	// Calculate sampling box size
	boxWidth := float64(c.w) / float64(targetWidth)
	boxHeight := float64(c.h) / float64(targetHeight)

	// Create new grid
	newGrid := make([][]rune, targetHeight)
	for i := range newGrid {
		newGrid[i] = make([]rune, targetWidth)
	}

	// Sample characters from original grid
	for y := 0; y < targetHeight; y++ {
		for x := 0; x < targetWidth; x++ {
			// Calculate sampling box boundaries
			startX := int(float64(x) * boxWidth)
			endX := int(float64(x+1) * boxWidth)
			startY := int(float64(y) * boxHeight)
			endY := int(float64(y+1) * boxHeight)

			// Count character occurrences in the sampling box
			charCount := make(map[rune]int)
			for sy := startY; sy < endY && sy < c.h; sy++ {
				for sx := startX; sx < endX && sx < c.w; sx++ {
					ch := c.grid[sy][sx]
					charCount[ch]++
				}
			}

			// Choose the most appropriate character
			var maxCount int
			var dominant rune = ' '

			// Priority order for characters
			priorities := []rune{'+', '|', '-', '/', '\\', '.', ' '}
			for _, ch := range priorities {
				if count := charCount[ch]; count > maxCount {
					maxCount = count
					dominant = ch
				}
			}

			// Special cases for line preservation
			hasVertical := charCount['|'] > 0 || charCount['+'] > 0
			hasHorizontal := charCount['-'] > 0 || charCount['+'] > 0

			// Determine final character
			if hasVertical && hasHorizontal {
				newGrid[y][x] = '+'
			} else if hasVertical {
				newGrid[y][x] = '|'
			} else if hasHorizontal {
				newGrid[y][x] = '-'
			} else {
				newGrid[y][x] = dominant
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
