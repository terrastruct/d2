package asciiroute

import (
	"fmt"
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

func processRoute(rd RouteDrawer, routes []*geo.Point, fromBoundary, toBoundary Boundary) []*geo.Point {
	// Create a deep copy of routes to avoid modifying the original
	routesCopy := make([]*geo.Point, len(routes))
	for i, pt := range routes {
		routesCopy[i] = &geo.Point{X: pt.X, Y: pt.Y}
	}

	routesCopy = mergeRoutes(routesCopy)
	calibrateRoutes(rd, routesCopy)

	// Force all route segments to be horizontal or vertical (after calibration)
	routesCopy = forceHorizontalVerticalRoute(routesCopy)

	// Adjust route endpoints to avoid overlapping with existing characters
	if len(routesCopy) >= 2 {
		adjustRouteStartPoint(rd, routesCopy, fromBoundary)
		adjustRouteEndPoint(rd, routesCopy, toBoundary)
	}

	return routesCopy
}

// forceHorizontalVerticalRoute transforms diagonal segments into horizontal and vertical segments
func forceHorizontalVerticalRoute(routes []*geo.Point) []*geo.Point {
	if len(routes) < 2 {
		return routes
	}

	// Check if any diagonal segments exist
	hasDiagonals := false
	for i := 1; i < len(routes); i++ {
		prev := routes[i-1]
		curr := routes[i]
		deltaX := math.Abs(curr.X - prev.X)
		deltaY := math.Abs(curr.Y - prev.Y)

		if deltaX > 0.5 && deltaY > 0.5 {
			hasDiagonals = true
			break
		}
	}

	if !hasDiagonals {
		return routes
	}

	// Transform diagonal segments
	var newRoutes []*geo.Point
	newRoutes = append(newRoutes, routes[0])

	for i := 1; i < len(routes); i++ {
		prev := newRoutes[len(newRoutes)-1]
		curr := routes[i]
		deltaX := math.Abs(curr.X - prev.X)
		deltaY := math.Abs(curr.Y - prev.Y)

		if deltaX > 0.5 && deltaY > 0.5 {
			// Break diagonal into horizontal then vertical
			intermediate := &geo.Point{X: curr.X, Y: prev.Y}
			newRoutes = append(newRoutes, intermediate)
		}

		newRoutes = append(newRoutes, curr)
	}

	return newRoutes
}

func getConnectionBoundaries(rd RouteDrawer, srcID, dstID string) (frmShapeBoundary, toShapeBoundary Boundary) {
	diagram := rd.GetDiagram()
	if diagram != nil {
		for _, shape := range diagram.Shapes {
			if shape.ID == srcID {
				tl, br := rd.GetBoundaryForShape(shape)
				frmShapeBoundary = *NewBoundary(tl, br)
			} else if shape.ID == dstID {
				tl, br := rd.GetBoundaryForShape(shape)
				toShapeBoundary = *NewBoundary(tl, br)
			}
		}
	}
	return
}

func calibrateRoutes(rd RouteDrawer, routes []*geo.Point) {
	for i := range routes {
		routes[i].X, routes[i].Y = rd.CalibrateXY(routes[i].X, routes[i].Y)
	}
}

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

func adjustRouteStartPoint(rd RouteDrawer, routes []*geo.Point, fromBoundary Boundary) {
	if len(routes) < 2 {
		return
	}

	firstX := routes[0].X
	firstY := routes[0].Y
	secondX := routes[1].X
	secondY := routes[1].Y

	// Check if end point is inside the to boundary
	// Move along the vector of the last segment until outside the boundary if so
	if fromBoundary.Contains(int(math.Round(firstX)), int(math.Round(firstY))) {
		vectorX := secondX - firstX
		vectorY := secondY - firstY

		length := math.Sqrt(vectorX*vectorX + vectorY*vectorY)
		if length > 0 {
			vectorX /= length
			vectorY /= length

			for fromBoundary.Contains(int(math.Round(routes[0].X)), int(math.Round(routes[0].Y))) {
				routes[0].X += vectorX
				routes[0].Y += vectorY
			}
		}
		return
	}

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

func adjustRouteEndPoint(rd RouteDrawer, routes []*geo.Point, toBoundary Boundary) {
	if len(routes) < 2 {
		return
	}

	lastIdx := len(routes) - 1
	secondLastIdx := lastIdx - 1

	lastX := routes[lastIdx].X
	lastY := routes[lastIdx].Y
	secondLastX := routes[secondLastIdx].X
	secondLastY := routes[secondLastIdx].Y

	// Check if end point is inside the to boundary
	// Move along the vector of the last segment until outside the boundary if so
	if toBoundary.Contains(int(math.Round(lastX)), int(math.Round(lastY))) {
		vectorX := lastX - secondLastX
		vectorY := lastY - secondLastY

		length := math.Sqrt(vectorX*vectorX + vectorY*vectorY)
		if length > 0 {
			vectorX /= length
			vectorY /= length

			for toBoundary.Contains(int(math.Round(routes[lastIdx].X)), int(math.Round(routes[lastIdx].Y))) {
				routes[lastIdx].X -= vectorX
				routes[lastIdx].Y -= vectorY
			}
		}
		return
	}

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
