package asciicanvas

import (
	"bytes"
	"strings"
	"unicode"
)

// Canvas represents an ASCII art canvas
type Canvas struct {
	grid [][]string
}

// New creates a new Canvas with the specified dimensions
func New(width, height int) *Canvas {
	grid := make([][]string, height)
	for i := range grid {
		grid[i] = make([]string, width)
		for j := range grid[i] {
			grid[i][j] = " "
		}
	}
	return &Canvas{grid: grid}
}

// Set sets a character at the specified position
func (c *Canvas) Set(x, y int, char string) {
	if c.IsInBounds(x, y) {
		c.grid[y][x] = char
	}
}

// Get retrieves the character at the specified position
func (c *Canvas) Get(x, y int) string {
	if c.IsInBounds(x, y) {
		return c.grid[y][x]
	}
	return ""
}

// IsInBounds checks if the given coordinates are within canvas bounds
func (c *Canvas) IsInBounds(x, y int) bool {
	return y >= 0 && y < len(c.grid) && x >= 0 && x < len(c.grid[y])
}

// Width returns the width of the canvas
func (c *Canvas) Width() int {
	if len(c.grid) > 0 {
		return len(c.grid[0])
	}
	return 0
}

// Height returns the height of the canvas
func (c *Canvas) Height() int {
	return len(c.grid)
}

// DrawLabel draws a label at the specified position
func (c *Canvas) DrawLabel(x, y int, label string) {
	if !c.IsInBounds(x, y) {
		return
	}
	for i, ch := range label {
		c.Set(x+i, y, string(ch))
	}
}

// ContainsAlphaNumeric checks if the position contains alphanumeric characters
func (c *Canvas) ContainsAlphaNumeric(x, y int) bool {
	char := c.Get(x, y)
	for _, r := range char {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// ToByteArray converts the canvas to a byte array, trimming empty space
func (c *Canvas) ToByteArray() []byte {
	var buf bytes.Buffer
	startRow := 0
	endRow := len(c.grid) - 1

	// Skip empty lines at the beginning
	for i, row := range c.grid {
		if strings.TrimSpace(strings.Join(row, "")) != "" {
			startRow = i
			break
		}
	}

	// Skip empty lines at the end
	for i := len(c.grid) - 1; i >= 0; i-- {
		if strings.TrimSpace(strings.Join(c.grid[i], "")) != "" {
			endRow = i
			break
		}
	}

	// Find the rightmost column with non-space content
	endCol := 0
	if len(c.grid) > 0 {
		for col := len(c.grid[0]) - 1; col >= 0; col-- {
			hasContent := false
			for row := startRow; row <= endRow; row++ {
				if col < len(c.grid[row]) && c.grid[row][col] != " " {
					hasContent = true
					break
				}
			}
			if hasContent {
				endCol = col
				break
			}
		}
	}

	// Post-process to compress consecutive route-only lines (vertical or horizontal)
	var prevRouteColumns []int
	var prevRouteType rune
	for i := startRow; i <= endRow; i++ {
		// Only include characters up to endCol
		rowData := c.grid[i]
		if endCol+1 < len(rowData) {
			rowData = rowData[:endCol+1]
		}
		line := strings.Join(rowData, "")

		// Find positions of route characters and check if line is route-only
		var routeColumns []int
		var routeType rune
		isOnlyRouteChars := true
		for pos, char := range line {
			if char == '│' || char == '─' {
				routeColumns = append(routeColumns, pos)
				if routeType == 0 {
					routeType = char
				} else if routeType != char {
					// Mixed route types on same line, don't compress
					isOnlyRouteChars = false
					break
				}
			} else if char != ' ' {
				isOnlyRouteChars = false
			}
		}

		// Check if this line has route characters in same columns and type as previous
		hasRoutes := len(routeColumns) > 0
		samePattern := len(routeColumns) == len(prevRouteColumns) && routeType == prevRouteType
		if samePattern {
			for j, col := range routeColumns {
				if j >= len(prevRouteColumns) || col != prevRouteColumns[j] {
					samePattern = false
					break
				}
			}
		}

		// Skip if this is route-only line with same pattern as previous route-only line
		if isOnlyRouteChars && hasRoutes && samePattern && len(prevRouteColumns) > 0 {
			continue
		}

		// Update previous pattern only if this line is route-only
		if isOnlyRouteChars && hasRoutes {
			prevRouteColumns = routeColumns
			prevRouteType = routeType
		} else {
			prevRouteColumns = nil
			prevRouteType = 0
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}