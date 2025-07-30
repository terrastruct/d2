package asciiroute

import (
	"math"
	"strings"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciicanvas"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
	"oss.terrastruct.com/d2/d2target"
)

// Constants for route drawing
const (
	MaxRouteAttempts = 200
	LabelOffsetX     = 2
)

// Point represents a 2D coordinate
type Point struct {
	X int
	Y int
}

// Boundary represents a rectangular boundary
type Boundary struct {
	TL Point
	BR Point
}

// Contains checks if a point is within the boundary
func (b *Boundary) Contains(x int, y int) bool {
	return x >= b.TL.X && x <= b.BR.X && y >= b.TL.Y && y <= b.BR.Y
}

// NewBoundary creates a new boundary from two points
func NewBoundary(tl, br Point) *Boundary {
	return &Boundary{
		TL: tl,
		BR: br,
	}
}

// RouteDrawer handles route drawing operations
type RouteDrawer interface {
	GetCanvas() *asciicanvas.Canvas
	GetChars() charset.Set
	GetDiagram() *d2target.Diagram
	GetFontWidth() float64
	GetFontHeight() float64
	GetScale() float64
	GetBoundaryForShape(s d2target.Shape) (Point, Point)
	CalibrateXY(x, y float64) (float64, float64)
}

// DrawRoute is the main entry point for drawing a route connection
func DrawRoute(rd RouteDrawer, conn d2target.Connection) {
	routes := conn.Route
	label := conn.Label
	
	// Parse connection boundaries to understand shape positions
	frmShapeBoundary, toShapeBoundary := parseConnectionBoundaries(rd, conn.ID)
	
	// Process the route: merge, calibrate, and adjust endpoints
	routes = processRoute(rd, routes)
	
	// Calculate turn directions for corners
	turnDir := calculateTurnDirections(routes)

	// Calculate best label position if label exists
	var labelPos *RouteLabelPosition
	if strings.TrimSpace(label) != "" {
		labelPos = calculateBestLabelPosition(rd, routes, label)
	}

	// Get character maps for drawing
	corners, arrows := getCharacterMaps(rd)

	// Draw each segment of the route
	for i := 1; i < len(routes); i++ {
		drawSegmentBetweenPoints(rd, routes[i-1], routes[i], i, conn, corners, arrows, turnDir, frmShapeBoundary, toShapeBoundary, labelPos, label)
	}
}

// getCharacterMaps returns the corner and arrow character maps for drawing
func getCharacterMaps(rd RouteDrawer) (corners, arrows map[string]string) {
	chars := rd.GetChars()
	corners = map[string]string{
		"-100-1": chars.BottomLeftCorner(), "0110": chars.BottomLeftCorner(),
		"-1001": chars.TopLeftCorner(), "0-110": chars.TopLeftCorner(),
		"0-1-10": chars.TopRightCorner(), "1001": chars.TopRightCorner(),
		"01-10": chars.BottomRightCorner(), "100-1": chars.BottomRightCorner(),
	}
	arrows = map[string]string{
		"0-1": chars.ArrowUp(), "10": chars.ArrowRight(), "01": chars.ArrowDown(), "-10": chars.ArrowLeft(),
	}
	return
}

// Helper function for general math operations
func absInt(a int) int {
	return int(math.Abs(float64(a)))
}