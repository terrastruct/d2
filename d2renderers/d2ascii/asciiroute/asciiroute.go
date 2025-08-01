package asciiroute

import (
	"math"
	"strings"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciicanvas"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
	"oss.terrastruct.com/d2/d2target"
)

const (
	MaxRouteAttempts = 200
	LabelOffsetX     = 2
)

type Point struct {
	X int
	Y int
}

type Boundary struct {
	TL Point
	BR Point
}

func (b *Boundary) Contains(x int, y int) bool {
	return x > b.TL.X && x < b.BR.X && y > b.TL.Y && y < b.BR.Y
}

func NewBoundary(tl, br Point) *Boundary {
	return &Boundary{
		TL: tl,
		BR: br,
	}
}

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

func DrawRoute(rd RouteDrawer, conn d2target.Connection) {
	routes := conn.Route
	label := conn.Label

	frmShapeBoundary, toShapeBoundary := getConnectionBoundaries(rd, conn.Src, conn.Dst)

	routes = processRoute(rd, routes, frmShapeBoundary, toShapeBoundary)

	turnDir := calculateTurnDirections(routes)

	var labelPos *RouteLabelPosition
	if strings.TrimSpace(label) != "" {
		labelPos = calculateBestLabelPosition(rd, routes, label)
	}

	corners, arrows := getCharacterMaps(rd)

	for i := 1; i < len(routes); i++ {
		drawSegmentBetweenPoints(rd, routes[i-1], routes[i], i, conn, corners, arrows, turnDir, frmShapeBoundary, toShapeBoundary, labelPos, label)
	}
}

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

func absInt(a int) int {
	return int(math.Abs(float64(a)))
}
