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

// Layout lays out the graph and computes curved edge routes
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
		createCircularArc(edge)
	}

	return nil
}

// calculateRadius now guards against a division-by-zero error when there is only one object.
func calculateRadius(objects []*d2graph.Object) float64 {
	if len(objects) < 2 {
		// When there is a single object, we can simply use MIN_RADIUS.
		return MIN_RADIUS
	}
	numObjects := float64(len(objects))
	maxSize := 0.0
	for _, obj := range objects {
		size := math.Max(obj.Box.Width, obj.Box.Height)
		maxSize = math.Max(maxSize, size)
	}
	minRadius := (maxSize/2.0 + PADDING) / math.Sin(math.Pi/numObjects)
	return math.Max(minRadius, MIN_RADIUS)
}

func positionObjects(objects []*d2graph.Object, radius float64) {
	numObjects := float64(len(objects))
	angleOffset := -math.Pi / 2

	for i, obj := range objects {
		angle := angleOffset + (2 * math.Pi * float64(i) / numObjects)
		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)
		obj.TopLeft = geo.NewPoint(
			x-obj.Box.Width/2,
			y-obj.Box.Height/2,
		)
	}
}

func createCircularArc(edge *d2graph.Edge) {
	if edge.Src == nil || edge.Dst == nil {
		return
	}

	srcCenter := edge.Src.Center()
	dstCenter := edge.Dst.Center()

	srcAngle := math.Atan2(srcCenter.Y, srcCenter.X)
	dstAngle := math.Atan2(dstCenter.Y, dstCenter.X)
	if dstAngle < srcAngle {
		dstAngle += 2 * math.Pi
	}

	arcRadius := math.Hypot(srcCenter.X, srcCenter.Y)

	path := make([]*geo.Point, 0, ARC_STEPS+1)
	for i := 0; i <= ARC_STEPS; i++ {
		t := float64(i) / float64(ARC_STEPS)
		angle := srcAngle + t*(dstAngle-srcAngle)
		x := arcRadius * math.Cos(angle)
		y := arcRadius * math.Sin(angle)
		path = append(path, geo.NewPoint(x, y))
	}
	path[0] = srcCenter
	path[len(path)-1] = dstCenter

	// Clamp endpoints to the boundaries of the source and destination boxes.
	_, newSrc := clampPointOutsideBox(edge.Src.Box, path, 0)
	_, newDst := clampPointOutsideBoxReverse(edge.Dst.Box, path, len(path)-1)
	path[0] = newSrc
	path[len(path)-1] = newDst

	// Trim redundant path points that fall inside node boundaries.
	path = trimPathPoints(path, edge.Src.Box)
	path = trimPathPoints(path, edge.Dst.Box)

	edge.Route = path
	edge.IsCurve = true

	if len(edge.Route) >= 2 {
		lastIndex := len(edge.Route) - 1
		lastPoint := edge.Route[lastIndex]
		secondLastPoint := edge.Route[lastIndex-1]

		tangentX := -lastPoint.Y
		tangentY := lastPoint.X
		mag := math.Hypot(tangentX, tangentY)
		if mag > 0 {
			tangentX /= mag
			tangentY /= mag
		}
		const MIN_SEGMENT_LEN = 4.159

		dx := lastPoint.X - secondLastPoint.X
		dy := lastPoint.Y - secondLastPoint.Y
		segLength := math.Hypot(dx, dy)
		if segLength > 0 {
			currentDirX := dx / segLength
			currentDirY := dy / segLength

			// Check if we need to adjust the direction
			if segLength < MIN_SEGMENT_LEN || (currentDirX*tangentX+currentDirY*tangentY) < 0.999 {
				// Create new point along tangent direction
				adjustLength := MIN_SEGMENT_LEN // Now float64
				if segLength >= MIN_SEGMENT_LEN {
					adjustLength = segLength // Both are float64 now
				}
				newSecondLastX := lastPoint.X - tangentX*adjustLength
				newSecondLastY := lastPoint.Y - tangentY*adjustLength
				edge.Route[lastIndex-1] = geo.NewPoint(newSecondLastX, newSecondLastY)
			}
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

// boxContains uses strict inequalities so that points exactly on the boundary are considered outside.
func boxContains(b *geo.Box, p *geo.Point) bool {
	return p.X > b.TopLeft.X &&
		p.X < b.TopLeft.X+b.Width &&
		p.Y > b.TopLeft.Y &&
		p.Y < b.TopLeft.Y+b.Height
}

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
