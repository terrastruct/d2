package asciiroute

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/log"
)

func processRoute(ctx context.Context, rd RouteDrawer, routes []*geo.Point, fromBoundary, toBoundary Boundary) []*geo.Point {
	log.Debug(ctx, "processing route", slog.Int("points", len(routes)))

	// Create a deep copy of routes to avoid modifying the original
	routesCopy := make([]*geo.Point, len(routes))
	for i, pt := range routes {
		routesCopy[i] = &geo.Point{X: pt.X, Y: pt.Y}
	}

	log.Debug(ctx, "step 1: merging collinear route segments")
	beforeMerge := len(routesCopy)
	routesCopy = mergeRoutes(routesCopy)
	log.Debug(ctx, "merged points", slog.Int("before", beforeMerge), slog.Int("after", len(routesCopy)))
	for i, pt := range routesCopy {
		log.Debug(ctx, "after merge point", slog.Int("index", i), slog.Float64("x", pt.X), slog.Float64("y", pt.Y))
	}

	log.Debug(ctx, "step 2: calibrating coordinates to ASCII grid")
	calibrateRoutes(ctx, rd, routesCopy)
	for i, pt := range routesCopy {
		log.Debug(ctx, "calibrated point", slog.Int("index", i), slog.Float64("x", pt.X), slog.Float64("y", pt.Y))
	}

	// Force all route segments to be horizontal or vertical (after calibration)
	log.Debug(ctx, "step 3: forcing horizontal/vertical segments")
	beforeForce := len(routesCopy)
	routesCopy = forceHorizontalVerticalRoute(routesCopy)
	log.Debug(ctx, "adjusted points", slog.Int("before", beforeForce), slog.Int("after", len(routesCopy)))
	for i, pt := range routesCopy {
		log.Debug(ctx, "after h/v force point", slog.Int("index", i), slog.Float64("x", pt.X), slog.Float64("y", pt.Y))
	}

	// Adjust route endpoints to avoid overlapping with existing characters
	if len(routesCopy) >= 2 {
		log.Debug(ctx, "step 4: adjusting start point to avoid overlaps")
		startBefore := fmt.Sprintf("(%.2f, %.2f)", routesCopy[0].X, routesCopy[0].Y)
		adjustRouteStartPoint(ctx, rd, routesCopy, fromBoundary)
		log.Debug(ctx, "start point adjusted", slog.String("before", startBefore), slog.Float64("afterX", routesCopy[0].X), slog.Float64("afterY", routesCopy[0].Y))

		log.Debug(ctx, "step 5: adjusting end point to avoid overlaps")
		endIdx := len(routesCopy) - 1
		endBefore := fmt.Sprintf("(%.2f, %.2f)", routesCopy[endIdx].X, routesCopy[endIdx].Y)
		routesCopy = adjustRouteEndPoint(ctx, rd, routesCopy, toBoundary)
		log.Debug(ctx, "end point adjusted", slog.String("before", endBefore), slog.Float64("afterX", routesCopy[endIdx].X), slog.Float64("afterY", routesCopy[endIdx].Y))
	}

	log.Debug(ctx, "final processed route", slog.Int("points", len(routesCopy)))
	for i, pt := range routesCopy {
		log.Debug(ctx, "final point", slog.Int("index", i), slog.Float64("x", pt.X), slog.Float64("y", pt.Y))
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

func calibrateRoutes(ctx context.Context, rd RouteDrawer, routes []*geo.Point) {
	for i := range routes {
		origX, origY := routes[i].X, routes[i].Y
		routes[i].X, routes[i].Y = rd.CalibrateXY(routes[i].X, routes[i].Y)
		log.Debug(ctx, "calibrate point", slog.Int("index", i), slog.Float64("origX", origX), slog.Float64("origY", origY), slog.Float64("newX", routes[i].X), slog.Float64("newY", routes[i].Y))
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

func adjustRouteStartPoint(ctx context.Context, rd RouteDrawer, routes []*geo.Point, fromBoundary Boundary) {
	if len(routes) < 2 {
		return
	}

	firstX := routes[0].X
	firstY := routes[0].Y
	secondX := routes[1].X
	secondY := routes[1].Y

	log.Debug(ctx, "adjusting start point", slog.Float64("firstX", firstX), slog.Float64("firstY", firstY), slog.Float64("secondX", secondX), slog.Float64("secondY", secondY))
	fmt.Printf("ADJUST START: from (%.3f,%.3f) to (%.3f,%.3f)\n", firstX, firstY, secondX, secondY)
	fmt.Printf("START SEGMENT: deltaX=%.3f deltaY=%.3f\n", secondX-firstX, secondY-firstY)
	if math.Abs(firstY-secondY) < 0.1 {
		fmt.Printf("START: This is a HORIZONTAL line (Y diff=%.3f < 0.1)\n", math.Abs(firstY-secondY))
	} else if math.Abs(firstX-secondX) < 0.1 {
		fmt.Printf("START: This is a VERTICAL line (X diff=%.3f < 0.1)\n", math.Abs(firstX-secondX))
	} else {
		fmt.Printf("START: This is a DIAGONAL line (X diff=%.3f, Y diff=%.3f)\n", math.Abs(firstX-secondX), math.Abs(firstY-secondY))
	}

	// Check if end point is inside the to boundary
	// Move along the vector of the last segment until outside the boundary if so
	if fromBoundary.Contains(int(math.Round(firstX)), int(math.Round(firstY))) {
		log.Debug(ctx, "start point inside source boundary, moving along vector")
		fmt.Printf("START BOUNDARY: point (%.0f,%.0f) is inside boundary TL(%d,%d)-BR(%d,%d)\n",
			firstX, firstY, fromBoundary.TL.X, fromBoundary.TL.Y, fromBoundary.BR.X, fromBoundary.BR.Y)
		vectorX := secondX - firstX
		vectorY := secondY - firstY

		length := math.Sqrt(vectorX*vectorX + vectorY*vectorY)
		if length > 0 {
			vectorX /= length
			vectorY /= length
			log.Debug(ctx, "movement vector", slog.Float64("x", vectorX), slog.Float64("y", vectorY))
			fmt.Printf("START VECTOR: moving along segment direction (%.3f,%.3f)\n", vectorX, vectorY)

			steps := 0
			for fromBoundary.Contains(int(math.Round(routes[0].X)), int(math.Round(routes[0].Y))) {
				routes[0].X += vectorX
				routes[0].Y += vectorY
				steps++
			}
			log.Debug(ctx, "moved to exit boundary", slog.Int("steps", steps), slog.Float64("x", routes[0].X), slog.Float64("y", routes[0].Y))
			fmt.Printf("START MOVED: after %d steps to (%.3f,%.3f)\n", steps, routes[0].X, routes[0].Y)
		}
		return
	}

	// Determine line direction and keep shifting until empty space
	if math.Abs(firstX-secondX) < 0.1 { // Vertical line (X coordinates are same)
		log.Debug(ctx, "horizontal line detected")
		deltaX := 0.0
		if secondX > firstX {
			deltaX = 1.0 // Shift start point towards second point (right)
			log.Debug(ctx, "shifting start point right")
		} else if secondX < firstX {
			deltaX = -1.0 // Shift start point towards second point (left)
			log.Debug(ctx, "shifting start point left")
		}

		if deltaX != 0 {
			shiftPointUntilEmpty(ctx, rd, &routes[0].X, &routes[0].Y, deltaX, 0)
		}
	} else if math.Abs(firstX-secondX) < 0.1 { // Vertical line
		log.Debug(ctx, "vertical line detected")
		deltaY := 0.0
		if secondY > firstY {
			deltaY = 1.0 // Shift start point towards second point (down)
			log.Debug(ctx, "shifting start point down")
		} else if secondY < firstY {
			deltaY = -1.0 // Shift start point towards second point (up)
			log.Debug(ctx, "shifting start point up")
		}

		if deltaY != 0 {
			shiftPointUntilEmpty(ctx, rd, &routes[0].X, &routes[0].Y, 0, deltaY)
		}
	}
}

func adjustRouteEndPoint(ctx context.Context, rd RouteDrawer, routes []*geo.Point, toBoundary Boundary) []*geo.Point {
	if len(routes) < 2 {
		return routes
	}

	lastIdx := len(routes) - 1
	secondLastIdx := lastIdx - 1

	lastX := routes[lastIdx].X
	lastY := routes[lastIdx].Y
	secondLastX := routes[secondLastIdx].X
	secondLastY := routes[secondLastIdx].Y

	log.Debug(ctx, "adjusting end point", slog.Float64("lastX", lastX), slog.Float64("lastY", lastY), slog.Float64("secondLastX", secondLastX), slog.Float64("secondLastY", secondLastY))

	lastXInt := int(math.Round(lastX))
	lastYInt := int(math.Round(lastY))

	// Check if end point is inside the to boundary
	// Move along the vector of the last segment until outside the boundary if so
	if toBoundary.Contains(lastXInt, lastYInt) {
		log.Debug(ctx, "end point inside dest boundary, moving along vector")
		vectorX := lastX - secondLastX
		vectorY := lastY - secondLastY

		length := math.Sqrt(vectorX*vectorX + vectorY*vectorY)
		if length > 0 {
			vectorX /= length
			vectorY /= length
			log.Debug(ctx, "movement vector", slog.Float64("x", vectorX), slog.Float64("y", vectorY))

			steps := 0
			for toBoundary.Contains(int(math.Round(routes[lastIdx].X)), int(math.Round(routes[lastIdx].Y))) {
				routes[lastIdx].X -= vectorX
				routes[lastIdx].Y -= vectorY
				steps++
			}
			log.Debug(ctx, "moved to exit boundary", slog.Int("steps", steps), slog.Float64("x", routes[lastIdx].X), slog.Float64("y", routes[lastIdx].Y))
		}
		return routes
	}

	// Determine line direction and keep shifting until empty space
	if math.Abs(lastY-secondLastY) < 0.1 { // Horizontal line (Y coordinates are same)
		log.Debug(ctx, "horizontal line detected")
		deltaX := 0.0
		if secondLastX > lastX {
			deltaX = 1.0 // Shift end point towards second-to-last point (right)
			log.Debug(ctx, "shifting end point right")
		} else if secondLastX < lastX {
			deltaX = -1.0 // Shift end point towards second-to-last point (left)
			log.Debug(ctx, "shifting end point left")
		}

		if deltaX != 0 {
			shiftPointUntilEmpty(ctx, rd, &routes[lastIdx].X, &routes[lastIdx].Y, deltaX, 0)
		}
	} else if math.Abs(lastX-secondLastX) < 0.1 { // Vertical line (X coordinates are same)
		log.Debug(ctx, "vertical line detected")
		deltaY := 0.0
		if secondLastY > lastY {
			deltaY = 1.0 // Shift end point towards second-to-last point (down)
			log.Debug(ctx, "shifting end point down")
		} else if secondLastY < lastY {
			deltaY = -1.0 // Shift end point towards second-to-last point (up)
			log.Debug(ctx, "shifting end point up")
		}

		if deltaY != 0 {
			shiftPointUntilEmpty(ctx, rd, &routes[lastIdx].X, &routes[lastIdx].Y, 0, deltaY)
		}
	}

	return routes
}

func shiftPointUntilEmpty(ctx context.Context, rd RouteDrawer, x, y *float64, deltaX, deltaY float64) {
	canvas := rd.GetCanvas()
	startX, startY := *x, *y
	steps := 0
	for {
		xi := int(math.Round(*x))
		yi := int(math.Round(*y))
		if canvas.IsInBounds(xi, yi) {
			char := canvas.Get(xi, yi)
			if char == " " {
				log.Debug(ctx, "found empty space", slog.Int("steps", steps), slog.Float64("startX", startX), slog.Float64("startY", startY), slog.Float64("x", *x), slog.Float64("y", *y))
				break // Found empty space
			}
			log.Debug(ctx, "position occupied, shifting", slog.Int("x", xi), slog.Int("y", yi), slog.String("char", string(char)), slog.Float64("deltaX", deltaX), slog.Float64("deltaY", deltaY))
			*x += deltaX
			*y += deltaY
			steps++
		} else {
			log.Debug(ctx, "position out of bounds, stopping", slog.Int("x", xi), slog.Int("y", yi))
			break // Out of bounds
		}
	}
}
