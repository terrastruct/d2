package asciiroute

import (
	"fmt"
	"math"
	"regexp"

	"oss.terrastruct.com/d2/lib/geo"
)

// processRoute applies all route processing steps: merge, calibrate, and adjust
func processRoute(rd RouteDrawer, routes []*geo.Point) []*geo.Point {
	// Create a deep copy of routes to avoid modifying the original
	routesCopy := make([]*geo.Point, len(routes))
	for i, pt := range routes {
		routesCopy[i] = &geo.Point{X: pt.X, Y: pt.Y}
	}
	
	routesCopy = mergeRoutes(routesCopy)
	calibrateRoutes(rd, routesCopy)
	
	// Adjust route endpoints to avoid overlapping with existing characters
	if len(routesCopy) >= 2 {
		adjustRouteStartPoint(rd, routesCopy)
		adjustRouteEndPoint(rd, routesCopy)
	}
	
	return routesCopy
}

// parseConnectionBoundaries extracts source and destination shape boundaries from connection ID
func parseConnectionBoundaries(rd RouteDrawer, connID string) (frmShapeBoundary, toShapeBoundary Boundary) {
	re := regexp.MustCompile(` -> | <-> | -- `)
	re1 := regexp.MustCompile(`\(([^}]*)\)`)
	re2 := regexp.MustCompile(`(.*)\(`)
	match1 := re1.FindStringSubmatch(connID)
	match2 := re2.FindStringSubmatch(connID)
	
	if len(match1) > 0 {
		parentID := ""
		if len(match2) > 0 {
			parentID = match2[1]
		}
		splitResult := re.Split(match1[1], -1)
		diagram := rd.GetDiagram()
		if diagram != nil {
			for _, shape := range diagram.Shapes {
				if len(splitResult) > 0 && shape.ID == parentID+splitResult[0] {
					tl, br := rd.GetBoundaryForShape(shape)
					frmShapeBoundary = *NewBoundary(tl, br)
				} else if len(splitResult) > 1 && shape.ID == parentID+splitResult[1] {
					tl, br := rd.GetBoundaryForShape(shape)
					toShapeBoundary = *NewBoundary(tl, br)
				}
			}
		}
	}
	return
}

// calibrateRoutes adjusts route coordinates to canvas scale
func calibrateRoutes(rd RouteDrawer, routes []*geo.Point) {
	for i := range routes {
		routes[i].X, routes[i].Y = rd.CalibrateXY(routes[i].X, routes[i].Y)
		routes[i].X -= 1
	}
}

// mergeRoutes combines consecutive route points in the same direction
func mergeRoutes(routes []*geo.Point) []*geo.Point {
	if len(routes) < 2 {
		return routes
	}
	
	mRoutes := []*geo.Point{routes[0]}
	var pt = routes[0]
	dir := geo.Sign(routes[0].X-routes[1].X)*1 + geo.Sign(routes[0].Y-routes[1].Y)*2
	for j := 1; j < len(routes); j++ {
		newDir := geo.Sign(pt.X-routes[j].X)*1 + geo.Sign(pt.Y-routes[j].Y)*2
		if dir != newDir {
			mRoutes = append(mRoutes, pt)
			dir = newDir
		}
		pt = routes[j]
	}
	if mRoutes[len(mRoutes)-1].X != pt.X || mRoutes[len(mRoutes)-1].Y != pt.Y {
		mRoutes = append(mRoutes, pt)
	}
	return mRoutes
}

// calculateTurnDirections determines corner types for route points
func calculateTurnDirections(routes []*geo.Point) map[string]string {
	turnDir := map[string]string{}
	if len(routes) < 3 {
		return turnDir
	}
	
	for i := 1; i < len(routes)-1; i++ {
		curr := routes[i]
		prev := routes[i-1]
		next := routes[i+1]
		
		key := fmt.Sprintf("%d_%d", int(math.Round(curr.X)), int(math.Round(curr.Y)))
		dir := fmt.Sprintf("%d%d%d%d",
			geo.Sign(curr.X-prev.X), geo.Sign(curr.Y-prev.Y),
			geo.Sign(next.X-curr.X), geo.Sign(next.Y-curr.Y),
		)
		turnDir[key] = dir
	}
	return turnDir
}

// adjustRouteStartPoint shifts the start point to find empty space
func adjustRouteStartPoint(rd RouteDrawer, routes []*geo.Point) {
	if len(routes) < 2 {
		return
	}
	
	firstX := routes[0].X
	firstY := routes[0].Y
	secondX := routes[1].X
	secondY := routes[1].Y

	// Determine line direction and keep shifting until empty space
	if math.Abs(firstY-secondY) < 0.1 { // Horizontal line
		deltaX := 0.0
		if secondX > firstX {
			deltaX = 1.0 // Shift start point towards second point (right)
		} else if secondX < firstX {
			deltaX = -1.0 // Shift start point towards second point (left)
		}

		if deltaX != 0 {
			shiftPointUntilEmpty(rd, &routes[0].X, &routes[0].Y, deltaX, 0)
		}
	} else if math.Abs(firstX-secondX) < 0.1 { // Vertical line
		deltaY := 0.0
		if secondY > firstY {
			deltaY = 1.0 // Shift start point towards second point (down)
		} else if secondY < firstY {
			deltaY = -1.0 // Shift start point towards second point (up)
		}

		if deltaY != 0 {
			shiftPointUntilEmpty(rd, &routes[0].X, &routes[0].Y, 0, deltaY)
		}
	}
}

// adjustRouteEndPoint shifts the end point to find empty space
func adjustRouteEndPoint(rd RouteDrawer, routes []*geo.Point) {
	if len(routes) < 2 {
		return
	}
	
	lastIdx := len(routes) - 1
	secondLastIdx := lastIdx - 1

	lastX := routes[lastIdx].X
	lastY := routes[lastIdx].Y
	secondLastX := routes[secondLastIdx].X
	secondLastY := routes[secondLastIdx].Y

	// Determine line direction and keep shifting until empty space
	if math.Abs(lastY-secondLastY) < 0.1 { // Horizontal line
		deltaX := 0.0
		if secondLastX > lastX {
			deltaX = 1.0 // Shift end point towards second-to-last point (right)
		} else if secondLastX < lastX {
			deltaX = -1.0 // Shift end point towards second-to-last point (left)
		}

		if deltaX != 0 {
			shiftPointUntilEmpty(rd, &routes[lastIdx].X, &routes[lastIdx].Y, deltaX, 0)
		}
	} else if math.Abs(lastX-secondLastX) < 0.1 { // Vertical line
		deltaY := 0.0
		if secondLastY > lastY {
			deltaY = 1.0 // Shift end point towards second-to-last point (down)
		} else if secondLastY < lastY {
			deltaY = -1.0 // Shift end point towards second-to-last point (up)
		}

		if deltaY != 0 {
			shiftPointUntilEmpty(rd, &routes[lastIdx].X, &routes[lastIdx].Y, 0, deltaY)
		}
	}
}

// shiftPointUntilEmpty keeps shifting a point by delta until empty space is found
func shiftPointUntilEmpty(rd RouteDrawer, x, y *float64, deltaX, deltaY float64) {
	canvas := rd.GetCanvas()
	for {
		xi := int(math.Round(*x))
		yi := int(math.Round(*y))
		if canvas.IsInBounds(xi, yi) {
			if canvas.Get(xi, yi) == " " {
				break // Found empty space
			}
			*x += deltaX
			*y += deltaY
		} else {
			break // Out of bounds
		}
	}
}