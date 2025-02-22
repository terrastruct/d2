package d2cycle

import (
	"context"
	"math"
	"sort"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

const (
	MIN_RADIUS      = 200
	PADDING         = 20
	MIN_SEGMENT_LEN = 10
	ARC_STEPS       = 100
)

// Layout lays out the graph and computes curved edge routes.
func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) error {
	objects := g.Root.ChildrenArray
	if len(objects) == 0 {
		return nil
	}

	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	radius := calculateRadius(objects)
	positionObjects(objects, radius)

	for _, edge := range g.Edges {
		createCircularArc(edge, radius)
	}

	return nil
}

// calculateRadius determines the radius of the circular layout based on the number and size of objects.
func calculateRadius(objects []*d2graph.Object) float64 {
	numObjects := float64(len(objects))
	maxSize := 0.0
	for _, obj := range objects {
		size := math.Max(obj.Box.Width, obj.Box.Height)
		maxSize = math.Max(maxSize, size)
	}
	minRadius := (maxSize/2.0 + PADDING) / math.Sin(math.Pi/numObjects)
	return math.Max(minRadius, MIN_RADIUS)
}

// positionObjects arranges objects in a circular pattern around the origin.
func positionObjects(objects []*d2graph.Object, radius float64) {
	numObjects := float64(len(objects))
	angleOffset := -math.Pi / 2 // Start at the top of the circle

	for i, obj := range objects {
		angle := angleOffset + (2*math.Pi*float64(i)/numObjects)
		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)
		obj.TopLeft = geo.NewPoint(
			x-obj.Box.Width/2,
			y-obj.Box.Height/2,
		)
	}
}

// createCircularArc generates a curved edge route with corrected arrow orientation.
func createCircularArc(edge *d2graph.Edge, radius float64) {
	if edge.Src == nil || edge.Dst == nil {
		return
	}

	srcCenter := edge.Src.Center()
	dstCenter := edge.Dst.Center()

	// Generate initial arc path from source center to destination center
	path := generateArcPoints(srcCenter, dstCenter, radius, ARC_STEPS)

	// Clamp endpoints to the boundaries of the source and destination boxes
	_, newSrc := clampPointOutsideBox(edge.Src.Box, path, 0)
	_, newDst := clampPointOutsideBoxReverse(edge.Dst.Box, path, len(path)-1)
	path[0] = newSrc
	path[len(path)-1] = newDst

	// Add a point before newDst along the tangent direction to correct arrow orientation
	if len(path) >= 2 {
		dstAngle := math.Atan2(newDst.Y, newDst.X)
		tangent := geo.NewPoint(-math.Sin(dstAngle), math.Cos(dstAngle))
		ε := 0.01 * radius // Small offset, e.g., 1% of radius
		preDst := geo.NewPoint(newDst.X-ε*tangent.X, newDst.Y-ε*tangent.Y)
		// Insert preDst before newDst
		path = append(path[:len(path)-1], preDst, newDst)
	}

	// Trim redundant path points that fall inside node boundaries
	path = trimPathPoints(path, edge.Src.Box)
	path = trimPathPoints(path, edge.Dst.Box)

	edge.Route = path
	edge.IsCurve = true
}

// generateArcPoints creates points along a circular arc from src to dst.
func generateArcPoints(src, dst *geo.Point, radius float64, steps int) []*geo.Point {
	// Calculate angles relative to the center (0,0)
	srcAngle := math.Atan2(src.Y, src.X)
	dstAngle := math.Atan2(dst.Y, dst.X)

	// Ensure the arc goes the shorter way
	if dstAngle < srcAngle {
		dstAngle += 2 * math.Pi
	}
	angleDiff := dstAngle - srcAngle
	if angleDiff > math.Pi {
		dstAngle -= 2 * math.Pi
	}

	// Generate points along the arc
	path := make([]*geo.Point, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		angle := srcAngle + t*(dstAngle-srcAngle)
		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)
		path = append(path, geo.NewPoint(x, y))
	}
	return path
}

// clampPointOutsideBox finds the first point outside the box and computes the precise intersection.
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
	return len(path)-1, path[len(path)-1]
}

// clampPointOutsideBoxReverse works similarly but traverses the path in reverse.
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

// findPreciseIntersection calculates the closest intersection between a segment and box boundaries.
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

	// Check vertical boundaries
	if dx != 0 {
		// Left boundary
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
		// Right boundary
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

	// Check horizontal boundaries
	if dy != 0 {
		// Top boundary
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
		// Bottom boundary
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

	// Sort intersections by t (distance from seg.Start) and return the closest
	sort.Slice(intersections, func(i, j int) bool {
		return intersections[i].t < intersections[j].t
	})
	return intersections[0].point
}

// trimPathPoints removes intermediate points inside the box while retaining endpoints.
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

// boxContains checks if a point is strictly inside the box (boundary points are outside).
func boxContains(b *geo.Box, p *geo.Point) bool {
	return p.X > b.TopLeft.X &&
		p.X < b.TopLeft.X+b.Width &&
		p.Y > b.TopLeft.Y &&
		p.Y < b.TopLeft.Y+b.Height
}

// positionLabelsIcons sets default positions for labels and icons on objects.
func positionLabelsIcons(obj *d2graph.Object) {
	if obj.Icon != nil && obj.IconPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
			if obj.LabelPosition == nil {
				obj.LabelPosition = go2.Pointer(label.OutsideTopRight.String())
				return
			}
		} else if obj.SQLTable != nil || obj.Class != nil || obj.Language != "" {
			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
		} else {
			obj.IconPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
	}

	if obj.HasLabel() && obj.LabelPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
		} else if obj.HasOutsideBottomLabel() {
			obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
		} else if obj.Icon != nil {
			obj.LabelPosition = go2.Pointer(label.InsideTopCenter.String())
		} else {
			obj.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}

		if float64(obj.LabelDimensions.Width) > obj.Width ||
			float64(obj.LabelDimensions.Height) > obj.Height {
			if len(obj.ChildrenArray) > 0 {
				obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
			} else {
				obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
			}
		}
	}
}