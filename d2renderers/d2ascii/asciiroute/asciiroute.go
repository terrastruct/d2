package asciiroute

import (
	"context"
	"log/slog"
	"math"
	"strings"

	"oss.terrastruct.com/d2/d2renderers/d2ascii/asciicanvas"
	"oss.terrastruct.com/d2/d2renderers/d2ascii/charset"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/log"
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
	GetContext() context.Context
}

func DrawRoute(rd RouteDrawer, conn d2target.Connection) {
	routes := conn.Route
	label := conn.Label
	ctx := rd.GetContext()

	log.Debug(ctx, "starting edge route", slog.String("src", conn.Src), slog.String("dst", conn.Dst))
	log.Debug(ctx, "initial route points", slog.Int("count", len(routes)))
	for i, pt := range routes {
		log.Debug(ctx, "route point", slog.Int("index", i), slog.Float64("x", pt.X), slog.Float64("y", pt.Y))
	}

	frmShapeBoundary, toShapeBoundary := getConnectionBoundaries(rd, conn.Src, conn.Dst)
	log.Debug(ctx, "boundaries", slog.Int("srcTLX", frmShapeBoundary.TL.X), slog.Int("srcTLY", frmShapeBoundary.TL.Y), slog.Int("srcBRX", frmShapeBoundary.BR.X), slog.Int("srcBRY", frmShapeBoundary.BR.Y), slog.Int("dstTLX", toShapeBoundary.TL.X), slog.Int("dstTLY", toShapeBoundary.TL.Y), slog.Int("dstBRX", toShapeBoundary.BR.X), slog.Int("dstBRY", toShapeBoundary.BR.Y))

	routes = processRoute(ctx, rd, routes, frmShapeBoundary, toShapeBoundary)

	turnDir := calculateTurnDirections(routes)
	log.Debug(ctx, "turn directions calculated", slog.Int("count", len(turnDir)))
	for key, dir := range turnDir {
		log.Debug(ctx, "turn direction", slog.String("key", key), slog.String("dir", dir))
	}

	var labelPos *RouteLabelPosition
	if strings.TrimSpace(label) != "" {
		labelPos = calculateBestLabelPosition(rd, routes, label)
		if labelPos != nil {
			log.Debug(ctx, "label position calculated", slog.Int("segmentIndex", labelPos.I), slog.Int("x", labelPos.X), slog.Int("y", labelPos.Y), slog.Float64("maxDiff", labelPos.MaxDiff))
		}
	}

	corners, arrows := getCharacterMaps(rd)

	log.Debug(ctx, "drawing segments", slog.Int("count", len(routes)-1))
	for i := 1; i < len(routes); i++ {
		log.Debug(ctx, "drawing segment", slog.Int("index", i-1), slog.Float64("x1", routes[i-1].X), slog.Float64("y1", routes[i-1].Y), slog.Float64("x2", routes[i].X), slog.Float64("y2", routes[i].Y))
		drawSegmentBetweenPoints(ctx, rd, routes[i-1], routes[i], i, conn, corners, arrows, turnDir, frmShapeBoundary, toShapeBoundary, labelPos, label)
	}
	log.Debug(ctx, "edge route completed", slog.String("src", conn.Src), slog.String("dst", conn.Dst))
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
