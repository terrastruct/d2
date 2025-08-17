package asciiroute

import (
	"fmt"
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

func processRoute(rd RouteDrawer, routes []*geo.Point, fromBoundary, toBoundary Boundary) []*geo.Point {
	fmt.Printf("[D2ASCII] Processing route with %d points\n", len(routes))
	
	// Create a deep copy of routes to avoid modifying the original
	routesCopy := make([]*geo.Point, len(routes))
	for i, pt := range routes {
		routesCopy[i] = &geo.Point{X: pt.X, Y: pt.Y}
	}

	fmt.Printf("[D2ASCII] Step 1: Merging collinear route segments\n")
	beforeMerge := len(routesCopy)
	routesCopy = mergeRoutes(routesCopy)
	fmt.Printf("[D2ASCII]   Merged from %d to %d points\n", beforeMerge, len(routesCopy))
	for i, pt := range routesCopy {
		fmt.Printf("[D2ASCII]   After merge point %d: (%.2f, %.2f)\n", i, pt.X, pt.Y)
	}

	fmt.Printf("[D2ASCII] Step 2: Calibrating coordinates to ASCII grid\n")
	calibrateRoutes(rd, routesCopy)
	for i, pt := range routesCopy {
		fmt.Printf("[D2ASCII]   Calibrated point %d: (%.2f, %.2f)\n", i, pt.X, pt.Y)
	}

	// Force all route segments to be horizontal or vertical (after calibration)
	fmt.Printf("[D2ASCII] Step 3: Forcing horizontal/vertical segments\n")
	beforeForce := len(routesCopy)
	routesCopy = forceHorizontalVerticalRoute(routesCopy)
	fmt.Printf("[D2ASCII]   Adjusted from %d to %d points\n", beforeForce, len(routesCopy))
	for i, pt := range routesCopy {
		fmt.Printf("[D2ASCII]   After H/V force point %d: (%.2f, %.2f)\n", i, pt.X, pt.Y)
	}

	// Adjust route endpoints to avoid overlapping with existing characters
	if len(routesCopy) >= 2 {
		fmt.Printf("[D2ASCII] Step 4: Adjusting start point to avoid overlaps\n")
		startBefore := fmt.Sprintf("(%.2f, %.2f)", routesCopy[0].X, routesCopy[0].Y)
		adjustRouteStartPoint(rd, routesCopy, fromBoundary)
		fmt.Printf("[D2ASCII]   Start point: %s -> (%.2f, %.2f)\n", startBefore, routesCopy[0].X, routesCopy[0].Y)
		
		fmt.Printf("[D2ASCII] Step 5: Adjusting end point to avoid overlaps\n")
		endIdx := len(routesCopy) - 1
		endBefore := fmt.Sprintf("(%.2f, %.2f)", routesCopy[endIdx].X, routesCopy[endIdx].Y)
		routesCopy = adjustRouteEndPoint(rd, routesCopy, toBoundary)
		fmt.Printf("[D2ASCII]   End point: %s -> (%.2f, %.2f)\n", endBefore, routesCopy[endIdx].X, routesCopy[endIdx].Y)
	}

	fmt.Printf("[D2ASCII] Final processed route (%d points):\n", len(routesCopy))
	for i, pt := range routesCopy {
		fmt.Printf("[D2ASCII]   Final point %d: (%.2f, %.2f)\n", i, pt.X, pt.Y)
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
			fmt.Printf("[D2ASCII]   Found diagonal segment %d: (%.2f,%.2f) -> (%.2f,%.2f), deltaX=%.2f, deltaY=%.2f\n",
				i-1, prev.X, prev.Y, curr.X, curr.Y, deltaX, deltaY)
			hasDiagonals = true
			break
		}
	}

	if !hasDiagonals {
		fmt.Printf("[D2ASCII]   No diagonal segments found, keeping route as-is\n")
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
			fmt.Printf("[D2ASCII]   Breaking diagonal: (%.2f,%.2f) -> (%.2f,%.2f) into H: (%.2f,%.2f) -> (%.2f,%.2f) and V: (%.2f,%.2f) -> (%.2f,%.2f)\n",
				prev.X, prev.Y, curr.X, curr.Y,
				prev.X, prev.Y, intermediate.X, intermediate.Y,
				intermediate.X, intermediate.Y, curr.X, curr.Y)
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
		origX, origY := routes[i].X, routes[i].Y
		routes[i].X, routes[i].Y = rd.CalibrateXY(routes[i].X, routes[i].Y)
		fmt.Printf("[D2ASCII]   Calibrate point %d: (%.2f, %.2f) -> (%.2f, %.2f)\n", 
			i, origX, origY, routes[i].X, routes[i].Y)
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

	fmt.Printf("[D2ASCII]   Adjusting start point: (%.2f, %.2f) -> (%.2f, %.2f)\n", 
		firstX, firstY, secondX, secondY)

	// Check if end point is inside the to boundary
	// Move along the vector of the last segment until outside the boundary if so
	if fromBoundary.Contains(int(math.Round(firstX)), int(math.Round(firstY))) {
		fmt.Printf("[D2ASCII]   Start point inside source boundary, moving along vector\n")
		vectorX := secondX - firstX
		vectorY := secondY - firstY

		length := math.Sqrt(vectorX*vectorX + vectorY*vectorY)
		if length > 0 {
			vectorX /= length
			vectorY /= length
			fmt.Printf("[D2ASCII]   Movement vector: (%.2f, %.2f)\n", vectorX, vectorY)

			steps := 0
			for fromBoundary.Contains(int(math.Round(routes[0].X)), int(math.Round(routes[0].Y))) {
				routes[0].X += vectorX
				routes[0].Y += vectorY
				steps++
			}
			fmt.Printf("[D2ASCII]   Moved %d steps to exit boundary: (%.2f, %.2f)\n", 
				steps, routes[0].X, routes[0].Y)
		}
		return
	}

	// Determine line direction and keep shifting until empty space
	if math.Abs(firstY-secondY) < 0.1 { // Horizontal line
		fmt.Printf("[D2ASCII]   Horizontal line detected\n")
		deltaX := 0.0
		if secondX > firstX {
			deltaX = 1.0 // Shift start point towards second point (right)
			fmt.Printf("[D2ASCII]   Shifting start point right\n")
		} else if secondX < firstX {
			deltaX = -1.0 // Shift start point towards second point (left)
			fmt.Printf("[D2ASCII]   Shifting start point left\n")
		}

		if deltaX != 0 {
			shiftPointUntilEmpty(rd, &routes[0].X, &routes[0].Y, deltaX, 0)
		}
	} else if math.Abs(firstX-secondX) < 0.1 { // Vertical line
		fmt.Printf("[D2ASCII]   Vertical line detected\n")
		deltaY := 0.0
		if secondY > firstY {
			deltaY = 1.0 // Shift start point towards second point (down)
			fmt.Printf("[D2ASCII]   Shifting start point down\n")
		} else if secondY < firstY {
			deltaY = -1.0 // Shift start point towards second point (up)
			fmt.Printf("[D2ASCII]   Shifting start point up\n")
		}

		if deltaY != 0 {
			shiftPointUntilEmpty(rd, &routes[0].X, &routes[0].Y, 0, deltaY)
		}
	}
}

func adjustRouteEndPoint(rd RouteDrawer, routes []*geo.Point, toBoundary Boundary) []*geo.Point {
	if len(routes) < 2 {
		return routes
	}

	lastIdx := len(routes) - 1
	secondLastIdx := lastIdx - 1

	lastX := routes[lastIdx].X
	lastY := routes[lastIdx].Y
	secondLastX := routes[secondLastIdx].X
	secondLastY := routes[secondLastIdx].Y

	fmt.Printf("[D2ASCII]   Adjusting end point: (%.2f, %.2f) <- (%.2f, %.2f)\n", 
		lastX, lastY, secondLastX, secondLastY)

	lastXInt := int(math.Round(lastX))
	lastYInt := int(math.Round(lastY))

	// Check if end point is inside the to boundary
	// Move along the vector of the last segment until outside the boundary if so
	if toBoundary.Contains(lastXInt, lastYInt) {
		fmt.Printf("[D2ASCII]   End point inside dest boundary, moving along vector\n")
		vectorX := lastX - secondLastX
		vectorY := lastY - secondLastY

		length := math.Sqrt(vectorX*vectorX + vectorY*vectorY)
		if length > 0 {
			vectorX /= length
			vectorY /= length
			fmt.Printf("[D2ASCII]   Movement vector: (%.2f, %.2f)\n", vectorX, vectorY)

			steps := 0
			for toBoundary.Contains(int(math.Round(routes[lastIdx].X)), int(math.Round(routes[lastIdx].Y))) {
				routes[lastIdx].X -= vectorX
				routes[lastIdx].Y -= vectorY
				steps++
			}
			fmt.Printf("[D2ASCII]   Moved %d steps to exit boundary: (%.2f, %.2f)\n", 
				steps, routes[lastIdx].X, routes[lastIdx].Y)
		}
		return routes
	}


	// Determine line direction and keep shifting until empty space
	if math.Abs(lastY-secondLastY) < 0.1 { // Horizontal line
		fmt.Printf("[D2ASCII]   Horizontal line detected\n")
		deltaX := 0.0
		if secondLastX > lastX {
			deltaX = 1.0 // Shift end point towards second-to-last point (right)
			fmt.Printf("[D2ASCII]   Shifting end point right\n")
		} else if secondLastX < lastX {
			deltaX = -1.0 // Shift end point towards second-to-last point (left)
			fmt.Printf("[D2ASCII]   Shifting end point left\n")
		}

		if deltaX != 0 {
			shiftPointUntilEmpty(rd, &routes[lastIdx].X, &routes[lastIdx].Y, deltaX, 0)
		}
	} else if math.Abs(lastX-secondLastX) < 0.1 { // Vertical line
		fmt.Printf("[D2ASCII]   Vertical line detected\n")
		deltaY := 0.0
		if secondLastY > lastY {
			deltaY = 1.0 // Shift end point towards second-to-last point (down)
			fmt.Printf("[D2ASCII]   Shifting end point down\n")
		} else if secondLastY < lastY {
			deltaY = -1.0 // Shift end point towards second-to-last point (up)
			fmt.Printf("[D2ASCII]   Shifting end point up\n")
		}

		if deltaY != 0 {
			shiftPointUntilEmpty(rd, &routes[lastIdx].X, &routes[lastIdx].Y, 0, deltaY)
		}
	}
	
	return routes
}

func shiftPointUntilEmpty(rd RouteDrawer, x, y *float64, deltaX, deltaY float64) {
	canvas := rd.GetCanvas()
	startX, startY := *x, *y
	steps := 0
	for {
		xi := int(math.Round(*x))
		yi := int(math.Round(*y))
		if canvas.IsInBounds(xi, yi) {
			char := canvas.Get(xi, yi)
			if char == " " {
				fmt.Printf("[D2ASCII]     Found empty space after %d steps: (%.2f, %.2f) -> (%.2f, %.2f)\n", 
					steps, startX, startY, *x, *y)
				break // Found empty space
			}
			fmt.Printf("[D2ASCII]     Position (%d, %d) occupied by '%s', shifting by (%.2f, %.2f)\n", 
				xi, yi, string(char), deltaX, deltaY)
			*x += deltaX
			*y += deltaY
			steps++
		} else {
			fmt.Printf("[D2ASCII]     Position (%d, %d) out of bounds, stopping\n", xi, yi)
			break // Out of bounds
		}
	}
}
