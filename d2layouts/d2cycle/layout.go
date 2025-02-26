package d2cycle

import (
	"context"
	"fmt"
	"math"
	"sort"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

const (
	MIN_RADIUS          = 250       // Increased to provide more space
	PADDING             = 40        // Increased padding between objects
	MIN_SEGMENT_LEN     = 15        // Increased minimum segment length
	ARC_STEPS           = 100       // Keep the same number of steps for arc calculation
	LABEL_MARGIN        = 10        // Margin for labels
	EDGE_BEND_FACTOR    = 0.3       // Controls how much edges bend inward/outward
	EDGE_PADDING_FACTOR = 0.15      // Controls spacing between parallel edges
)

// Layout lays out the graph and computes curved edge routes
func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) error {
	objects := g.Root.ChildrenArray
	if len(objects) == 0 {
		return nil
	}

	// Pre-compute dimensions for all objects
	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	// Calculate optimal radius based on number and size of objects
	radius := calculateOptimalRadius(objects)
	
	// Position objects in a circle
	positionObjects(objects, radius)
	
	// Adjust positions to resolve overlaps
	resolveOverlaps(objects, radius)

	// Create edge routes for all edges
	createEdgeRoutes(g.Edges, objects, radius)

	return nil
}

// calculateOptimalRadius computes an ideal radius based on number and size of objects
func calculateOptimalRadius(objects []*d2graph.Object) float64 {
	numObjects := float64(len(objects))
	
	// Find largest object dimension
	maxSize := 0.0
	totalArea := 0.0
	for _, obj := range objects {
		size := math.Max(obj.Box.Width, obj.Box.Height)
		maxSize = math.Max(maxSize, size)
		totalArea += obj.Box.Width * obj.Box.Height
	}
	
	// Minimum radius based on largest object
	minRadiusBySize := (maxSize/2.0 + PADDING) / math.Sin(math.Pi/numObjects)
	
	// Alternative calculation based on total area
	areaRadius := math.Sqrt(totalArea / (math.Pi * 0.5)) * 1.5
	
	// Use the larger of the minimum values
	calculatedRadius := math.Max(minRadiusBySize, areaRadius)
	
	// Ensure we don't go below minimum radius
	return math.Max(calculatedRadius, MIN_RADIUS)
}

// positionObjects arranges objects in a circle with the given radius
func positionObjects(objects []*d2graph.Object, radius float64) {
	numObjects := float64(len(objects))
	
	// Start from top (-π/2) with equal spacing
	angleOffset := -math.Pi / 2

	// Special case for small number of objects
	if numObjects <= 3 {
		// For 2-3 objects, increase spacing
		angleOffset = -math.Pi / 2
		radius *= 1.2
	}

	for i, obj := range objects {
		angle := angleOffset + (2 * math.Pi * float64(i) / numObjects)
		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)
		obj.TopLeft = geo.NewPoint(
			x - obj.Box.Width/2,
			y - obj.Box.Height/2,
		)
	}
}

// resolveOverlaps detects and fixes overlapping objects
func resolveOverlaps(objects []*d2graph.Object, radius float64) {
	if len(objects) <= 1 {
		return
	}
	
	// Maximum number of iterations to prevent infinite loops
	maxIterations := 10
	iteration := 0
	
	for iteration < maxIterations {
		overlapsResolved := true
		
		// Check each pair of objects for overlap
		for i := 0; i < len(objects); i++ {
			for j := i + 1; j < len(objects); j++ {
				obj1 := objects[i]
				obj2 := objects[j]
				
				// Calculate box centers
				center1 := obj1.Center()
				center2 := obj2.Center()
				
				// Calculate minimum separation needed
				minSepX := (obj1.Box.Width + obj2.Box.Width) / 2 + PADDING
				minSepY := (obj1.Box.Height + obj2.Box.Height) / 2 + PADDING
				
				// Calculate actual separation
				dx := math.Abs(center2.X - center1.X)
				dy := math.Abs(center2.Y - center1.Y)
				
				// Check for overlap
				if dx < minSepX && dy < minSepY {
					overlapsResolved = false
					
					// Calculate push direction (from center to objects)
					angle1 := math.Atan2(center1.Y, center1.X)
					angle2 := math.Atan2(center2.Y, center2.X)
					
					// Push objects outward slightly
					pushFactor := 0.1 * radius
					
					// Update first object position
					newX1 := pushFactor * math.Cos(angle1)
					newY1 := pushFactor * math.Sin(angle1)
					obj1.TopLeft.X += newX1 - obj1.Box.Width/2
					obj1.TopLeft.Y += newY1 - obj1.Box.Height/2
					
					// Update second object position
					newX2 := pushFactor * math.Cos(angle2)
					newY2 := pushFactor * math.Sin(angle2)
					obj2.TopLeft.X += newX2 - obj2.Box.Width/2
					obj2.TopLeft.Y += newY2 - obj2.Box.Height/2
				}
			}
		}
		
		// If no overlaps were found, we're done
		if overlapsResolved {
			break
		}
		
		iteration++
	}
}

// createEdgeRoutes creates routes for all edges in the graph
func createEdgeRoutes(edges []*d2graph.Edge, objects []*d2graph.Object, radius float64) {
	// First categorize edges to identify parallel edges
	edgeGroups := groupParallelEdges(edges)
	
	// Process each group of edges
	for _, group := range edgeGroups {
		if len(group) == 1 {
			// Single edge
			createCircularArc(group[0], radius, 0)
		} else {
			// Multiple parallel edges
			for i, edge := range group {
				// Alternate between inner and outer curves for parallel edges
				offset := float64(i-(len(group)-1)/2) * EDGE_PADDING_FACTOR
				createCircularArc(edge, radius, offset)
			}
		}
	}
}

// groupParallelEdges identifies edges between the same source and destination
func groupParallelEdges(edges []*d2graph.Edge) [][]*d2graph.Edge {
	groups := make(map[string][]*d2graph.Edge)
	
	for _, edge := range edges {
		if edge.Src == nil || edge.Dst == nil {
			continue
		}
		
		// Create a key for each source-destination pair using object IDs or addresses
		// Since GetID() is not available, use pointer addresses as unique identifiers
		srcID := fmt.Sprintf("%p", edge.Src)
		dstID := fmt.Sprintf("%p", edge.Dst)
		key := srcID + "->" + dstID
		
		groups[key] = append(groups[key], edge)
	}
	
	// Convert map to slice of edge groups
	result := make([][]*d2graph.Edge, 0, len(groups))
	for _, group := range groups {
		result = append(result, group)
	}
	
	return result
}

// createCircularArc creates a curved path between source and destination objects
func createCircularArc(edge *d2graph.Edge, baseRadius float64, offset float64) {
	if edge.Src == nil || edge.Dst == nil {
		return
	}

	srcCenter := edge.Src.Center()
	dstCenter := edge.Dst.Center()
	
	// Calculate angles and radii
	srcAngle := math.Atan2(srcCenter.Y, srcCenter.X)
	dstAngle := math.Atan2(dstCenter.Y, dstCenter.X)
	
	// Ensure we go the shorter way around the circle
	if dstAngle < srcAngle {
		if srcAngle - dstAngle > math.Pi {
			dstAngle += 2 * math.Pi
		}
	} else {
		if dstAngle - srcAngle > math.Pi {
			srcAngle += 2 * math.Pi
		}
	}
	
	// Adjust radius based on offset for parallel edges
	arcRadius := baseRadius * (1.0 + offset)
	
	// Control points for the path
	path := make([]*geo.Point, 0, ARC_STEPS+1)
	
	// Add intermediate points along the arc
	for i := 0; i <= ARC_STEPS; i++ {
		t := float64(i) / float64(ARC_STEPS)
		angle := srcAngle + t*(dstAngle-srcAngle)
		
		// Apply an inward bend for better curves
		distanceFactor := 1.0 - EDGE_BEND_FACTOR * math.Sin(t * math.Pi)
		radius := arcRadius * distanceFactor
		
		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)
		path = append(path, geo.NewPoint(x, y))
	}
	
	// Ensure endpoints are exactly at source and destination centers
	path[0] = srcCenter
	path[len(path)-1] = dstCenter

	// Clamp endpoints to the boundaries of the boxes
	_, newSrc := clampPointOutsideBox(edge.Src.Box, path, 0)
	_, newDst := clampPointOutsideBoxReverse(edge.Dst.Box, path, len(path)-1)
	path[0] = newSrc
	path[len(path)-1] = newDst

	// Trim redundant path points
	path = trimPathPoints(path, edge.Src.Box)
	path = trimPathPoints(path, edge.Dst.Box)

	// Smoothen the path
	path = smoothPath(path)

	// Set the final route
	edge.Route = path
	edge.IsCurve = true

	// Add arrow direction point for the end
	if len(edge.Route) >= 2 {
		adjustArrowDirection(edge)
	}
}

// smoothPath applies path smoothing to reduce sharp angles
func smoothPath(path []*geo.Point) []*geo.Point {
	if len(path) <= 3 {
		return path
	}
	
	result := []*geo.Point{path[0]}
	
	// Use a simple moving average for interior points
	for i := 1; i < len(path)-1; i++ {
		prev := path[i-1]
		curr := path[i]
		next := path[i+1]
		
		// Simple weighted average (current point has more weight)
		avgX := (prev.X + 2*curr.X + next.X) / 4
		avgY := (prev.Y + 2*curr.Y + next.Y) / 4
		
		result = append(result, geo.NewPoint(avgX, avgY))
	}
	
	result = append(result, path[len(path)-1])
	return result
}

// adjustArrowDirection ensures the arrow points in the right direction
func adjustArrowDirection(edge *d2graph.Edge) {
	lastIndex := len(edge.Route) - 1
	lastPoint := edge.Route[lastIndex]
	secondLastPoint := edge.Route[lastIndex-1]

	// Calculate tangent vector perpendicular to radius (for smooth entry)
	tangentX := -lastPoint.Y
	tangentY := lastPoint.X
	mag := math.Hypot(tangentX, tangentY)
	if mag > 0 {
		tangentX /= mag
		tangentY /= mag
	}

	// Check current direction
	dx := lastPoint.X - secondLastPoint.X
	dy := lastPoint.Y - secondLastPoint.Y
	segLength := math.Hypot(dx, dy)
	
	if segLength > 0 {
		currentDirX := dx / segLength
		currentDirY := dy / segLength

		// Adjust only if direction needs correction
		dotProduct := currentDirX*tangentX + currentDirY*tangentY
		if segLength < MIN_SEGMENT_LEN || dotProduct < 0.9 {
			// Create new point for smooth arrow entry
			adjustLength := math.Max(MIN_SEGMENT_LEN, segLength * 0.8)
			newSecondLastX := lastPoint.X - tangentX*adjustLength
			newSecondLastY := lastPoint.Y - tangentY*adjustLength
			edge.Route[lastIndex-1] = geo.NewPoint(newSecondLastX, newSecondLastY)
		}
	}
}

// clampPointOutsideBox walks forward along the path until it finds a point outside the box,
// then replaces the point with a precise intersection.
func clampPointOutsideBox(box *geo.Box, path []*geo.Point, startIdx int) (int, *geo.Point) {
	if startIdx >= len(path)-1 {
		return startIdx, path[startIdx]
	}
	if !boxContains(box, path[startIdx]) {
		return startIdx, path[startIdx]
	}

	for i := startIdx + 1; i < len(path); i++ {
		if boxContains(box, path[i]) {
			continue
		}
		seg := geo.NewSegment(path[i-1], path[i])
		inter := findPreciseIntersection(box, *seg)
		if inter != nil {
			return i, inter
		}
		return i, path[i]
	}
	return len(path) - 1, path[len(path)-1]
}

// clampPointOutsideBoxReverse works similarly but in reverse order.
func clampPointOutsideBoxReverse(box *geo.Box, path []*geo.Point, endIdx int) (int, *geo.Point) {
	if endIdx <= 0 {
		return endIdx, path[endIdx]
	}
	if !boxContains(box, path[endIdx]) {
		return endIdx, path[endIdx]
	}

	for j := endIdx - 1; j >= 0; j-- {
		if boxContains(box, path[j]) {
			continue
		}
		seg := geo.NewSegment(path[j], path[j+1])
		inter := findPreciseIntersection(box, *seg)
		if inter != nil {
			return j, inter
		}
		return j, path[j]
	}
	return 0, path[0]
}

// findPreciseIntersection calculates intersection points between seg and all four sides of the box,
// then returns the intersection closest to seg.Start.
func findPreciseIntersection(box *geo.Box, seg geo.Segment) *geo.Point {
	intersections := []struct {
		point *geo.Point
		t     float64
	}{}

	left := box.TopLeft.X
	right := box.TopLeft.X + box.Width
	top := box.TopLeft.Y
	bottom := box.TopLeft.Y + box.Height

	dx := seg.End.X - seg.Start.X
	dy := seg.End.Y - seg.Start.Y

	// Check vertical boundaries.
	if dx != 0 {
		// Left boundary.
		t := (left - seg.Start.X) / dx
		if t >= 0 && t <= 1 {
			y := seg.Start.Y + t*dy
			if y >= top && y <= bottom {
				intersections = append(intersections, struct {
					point *geo.Point
					t     float64
				}{geo.NewPoint(left, y), t})
			}
		}
		// Right boundary.
		t = (right - seg.Start.X) / dx
		if t >= 0 && t <= 1 {
			y := seg.Start.Y + t*dy
			if y >= top && y <= bottom {
				intersections = append(intersections, struct {
					point *geo.Point
					t     float64
				}{geo.NewPoint(right, y), t})
			}
		}
	}

	// Check horizontal boundaries.
	if dy != 0 {
		// Top boundary.
		t := (top - seg.Start.Y) / dy
		if t >= 0 && t <= 1 {
			x := seg.Start.X + t*dx
			if x >= left && x <= right {
				intersections = append(intersections, struct {
					point *geo.Point
					t     float64
				}{geo.NewPoint(x, top), t})
			}
		}
		// Bottom boundary.
		t = (bottom - seg.Start.Y) / dy
		if t >= 0 && t <= 1 {
			x := seg.Start.X + t*dx
			if x >= left && x <= right {
				intersections = append(intersections, struct {
					point *geo.Point
					t     float64
				}{geo.NewPoint(x, bottom), t})
			}
		}
	}

	if len(intersections) == 0 {
		return nil
	}

	// Sort intersections by t (distance from seg.Start) and return the closest.
	sort.Slice(intersections, func(i, j int) bool {
		return intersections[i].t < intersections[j].t
	})
	return intersections[0].point
}

// trimPathPoints removes intermediate points that fall inside the given box while preserving endpoints.
func trimPathPoints(path []*geo.Point, box *geo.Box) []*geo.Point {
	if len(path) <= 2 {
		return path
	}
	trimmed := []*geo.Point{path[0]}
	for i := 1; i < len(path)-1; i++ {
		if !boxContains(box, path[i]) {
			trimmed = append(trimmed, path[i])
		}
	}
	trimmed = append(trimmed, path[len(path)-1])
	return trimmed
}

// boxContains checks if a point is inside a box (strictly inside, not on boundary)
func boxContains(b *geo.Box, p *geo.Point) bool {
	return p.X > b.TopLeft.X &&
		p.X < b.TopLeft.X+b.Width &&
		p.Y > b.TopLeft.Y &&
		p.Y < b.TopLeft.Y+b.Height
}

// positionLabelsIcons positions labels and icons with better handling of overlap
func positionLabelsIcons(obj *d2graph.Object) {
	// Handle icon positioning first
	if obj.Icon != nil && obj.IconPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			// For container objects, place icon at top left
			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
			
			// If no label position is set, place label at top right
			if obj.LabelPosition == nil {
				obj.LabelPosition = go2.Pointer(label.OutsideTopRight.String())
				return
			}
		} else if obj.SQLTable != nil || obj.Class != nil || obj.Language != "" {
			// For structured objects, place icon at top left
			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
		} else {
			// For standard objects, center the icon
			obj.IconPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
	}

	// Now handle label positioning
	if obj.HasLabel() && obj.LabelPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			// For container objects, place label at top center
			obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
		} else if obj.HasOutsideBottomLabel() {
			// For objects with bottom labels, respect that
			obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
		} else if obj.Icon != nil {
			// If there's an icon, place label at top center
			obj.LabelPosition = go2.Pointer(label.InsideTopCenter.String())
		} else {
			// Default positioning in the middle
			obj.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}

		// If label is too large for the object, move it outside
		if float64(obj.LabelDimensions.Width) > obj.Width*0.9 ||
			float64(obj.LabelDimensions.Height) > obj.Height*0.9 {
			if len(obj.ChildrenArray) > 0 {
				obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
			} else {
				obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
			}
		}
	}
}