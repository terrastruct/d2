package asciiroute

import (
	"fmt"
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
	return x >= b.TL.X && x <= b.BR.X && y >= b.TL.Y && y <= b.BR.Y
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

	fmt.Printf("[D2ASCII] Starting edge route for connection %s -> %s\n", conn.Src, conn.Dst)
	fmt.Printf("[D2ASCII] Initial route points (%d points):\n", len(routes))
	for i, pt := range routes {
		fmt.Printf("[D2ASCII]   Point %d: (%.2f, %.2f)\n", i, pt.X, pt.Y)
	}

	frmShapeBoundary, toShapeBoundary := getConnectionBoundaries(rd, conn.Src, conn.Dst)
	fmt.Printf("[D2ASCII] Source boundary: TL(%d,%d) BR(%d,%d)\n", 
		frmShapeBoundary.TL.X, frmShapeBoundary.TL.Y, 
		frmShapeBoundary.BR.X, frmShapeBoundary.BR.Y)
	fmt.Printf("[D2ASCII] Dest boundary: TL(%d,%d) BR(%d,%d)\n", 
		toShapeBoundary.TL.X, toShapeBoundary.TL.Y, 
		toShapeBoundary.BR.X, toShapeBoundary.BR.Y)

	routes = processRoute(rd, routes, frmShapeBoundary, toShapeBoundary)

	turnDir := calculateTurnDirections(routes)
	fmt.Printf("[D2ASCII] Turn directions calculated: %d turns\n", len(turnDir))
	for key, dir := range turnDir {
		fmt.Printf("[D2ASCII]   Turn at %s: direction %s\n", key, dir)
	}

	var labelPos *RouteLabelPosition
	if strings.TrimSpace(label) != "" {
		labelPos = calculateBestLabelPosition(rd, routes, label)
		if labelPos != nil {
			fmt.Printf("[D2ASCII] Label position calculated: segment %d, pos (%d, %d), maxDiff %.2f\n", 
				labelPos.I, labelPos.X, labelPos.Y, labelPos.MaxDiff)
		}
	}

	corners, arrows := getCharacterMaps(rd)

	fmt.Printf("[D2ASCII] Drawing %d segments\n", len(routes)-1)
	for i := 1; i < len(routes); i++ {
		fmt.Printf("[D2ASCII] Drawing segment %d: (%.2f,%.2f) -> (%.2f,%.2f)\n", 
			i-1, routes[i-1].X, routes[i-1].Y, routes[i].X, routes[i].Y)
		drawSegmentBetweenPoints(rd, routes[i-1], routes[i], i, conn, corners, arrows, turnDir, frmShapeBoundary, toShapeBoundary, labelPos, label)
	}
	fmt.Printf("[D2ASCII] Edge route completed for %s -> %s\n", conn.Src, conn.Dst)
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
