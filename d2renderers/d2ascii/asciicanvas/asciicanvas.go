package asciicanvas

import (
	"bytes"
	"strings"
	"unicode"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
)

type Canvas struct {
	grid [][]string
}

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

func (c *Canvas) Set(x, y int, char string) {
	if c.IsInBounds(x, y) {
		c.grid[y][x] = char
	}
}

func (c *Canvas) Get(x, y int) string {
	if c.IsInBounds(x, y) {
		return c.grid[y][x]
	}
	return ""
}

func (c *Canvas) IsInBounds(x, y int) bool {
	return y >= 0 && y < len(c.grid) && x >= 0 && x < len(c.grid[y])
}

func (c *Canvas) Width() int {
	if len(c.grid) > 0 {
		return len(c.grid[0])
	}
	return 0
}

func (c *Canvas) Height() int {
	return len(c.grid)
}

func (c *Canvas) DrawLabel(x, y int, label string) {
	if !c.IsInBounds(x, y) {
		return
	}
	for i, ch := range label {
		c.Set(x+i, y, string(ch))
	}
}

func (c *Canvas) ContainsAlphaNumeric(x, y int) bool {
	char := c.Get(x, y)
	for _, r := range char {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func (c *Canvas) ToByteArray(chars charset.Set) []byte {
	var buf bytes.Buffer
	startRow := 0
	endRow := len(c.grid) - 1

	for i, row := range c.grid {
		if strings.TrimSpace(strings.Join(row, "")) != "" {
			startRow = i
			break
		}
	}

	for i := len(c.grid) - 1; i >= 0; i-- {
		if strings.TrimSpace(strings.Join(c.grid[i], "")) != "" {
			endRow = i
			break
		}
	}

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

	var prevRouteColumns []int
	var prevRouteType rune
	for i := startRow; i <= endRow; i++ {
		rowData := c.grid[i]
		if endCol+1 < len(rowData) {
			rowData = rowData[:endCol+1]
		}
		line := strings.Join(rowData, "")

		var routeColumns []int
		var routeType rune
		isOnlyRouteChars := true

		verticalChar := []rune(chars.Vertical())[0]
		horizontalChar := []rune(chars.Horizontal())[0]

		for pos, char := range line {
			if char == verticalChar || char == horizontalChar {
				routeColumns = append(routeColumns, pos)
				if routeType == 0 {
					routeType = char
				} else if routeType != char {
					isOnlyRouteChars = false
					break
				}
			} else if char != ' ' {
				isOnlyRouteChars = false
			}
		}

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

		if isOnlyRouteChars && hasRoutes && samePattern && len(prevRouteColumns) > 0 {
			continue
		}

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
