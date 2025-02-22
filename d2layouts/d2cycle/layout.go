package d2cycle

import (
	"context"
	"math"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

const (
	MIN_RADIUS      = 200
	PADDING         = 20
	MIN_SEGMENT_LEN = 10
	ARC_STEPS       = 30
	EPSILON         = 1e-10 // Small value for floating point comparisons
)

// Layout computes node positions and generates curved edge routes.
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
		createPreciseCircularArc(edge)
	}

	return nil
}

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

func positionObjects(objects []*d2graph.Object, radius float64) {
	numObjects := float64(len(objects))
	angleOffset := -math.Pi / 2

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

// createPreciseCircularArc computes a curved edge path that touches the node boundaries exactly.
func createPreciseCircularArc(edge *d2graph.Edge) {
	if edge.Src == nil || edge.Dst == nil {
		return
	}

	srcCenter := edge.Src.Center()
	dstCenter := edge.Dst.Center()

	// Compute angles for the circular arc.
	srcAngle := math.Atan2(srcCenter.Y, srcCenter.X)
	dstAngle := math.Atan2(dstCenter.Y, dstCenter.X)
	if dstAngle < srcAngle {
		dstAngle += 2 * math.Pi
	}

	arcRadius := math.Hypot(srcCenter.X, srcCenter.Y)

	// Generate the initial arc path.
	path := make([]*geo.Point, 0, ARC_STEPS+1)
	for i := 0; i <= ARC_STEPS; i++ {
		t := float64(i) / float64(ARC_STEPS)
		angle := srcAngle + t*(dstAngle-srcAngle)
		x := arcRadius * math.Cos(angle)
		y := arcRadius * math.Sin(angle)
		path = append(path, geo.NewPoint(x, y))
	}

	// Compute precise intersection points so the arrow touches the node boundaries.
	// For the source, the segment goes from the center (inside) to the next point (outside).
	srcIntersection := findPreciseBoxIntersection(edge.Src.Box, path[0], path[1])
	// For the destination, the segment goes from the center to the previous point (outside).
	dstIntersection := findPreciseBoxIntersection(edge.Dst.Box, path[len(path)-1], path[len(path)-2])

	// Update the endpoints with the snapped intersection points.
	path[0] = srcIntersection
	path[len(path)-1] = dstIntersection

	// Trim intermediate points that still fall inside the boxes.
	startIdx := 0
	endIdx := len(path) - 1
	for i := 1; i < len(path); i++ {
		if !boxContains(edge.Src.Box, path[i]) {
			startIdx = i - 1
			break
		}
	}
	for i := len(path) - 2; i >= 0; i-- {
		if !boxContains(edge.Dst.Box, path[i]) {
			endIdx = i + 1
			break
		}
	}

	edge.Route = path[startIdx : endIdx+1]
	edge.IsCurve = true
}

// findPreciseBoxIntersection returns the intersection point of the line (from p1 to p2) with the box boundary,
// snapped exactly to the nearest edge.
func findPreciseBoxIntersection(box *geo.Box, p1, p2 *geo.Point) *geo.Point {
	// Define the four box edges.
	edges := []geo.Segment{
		*geo.NewSegment(
			geo.NewPoint(box.TopLeft.X, box.TopLeft.Y),
			geo.NewPoint(box.TopLeft.X+box.Width, box.TopLeft.Y),
		), // Top
		*geo.NewSegment(
			geo.NewPoint(box.TopLeft.X+box.Width, box.TopLeft.Y),
			geo.NewPoint(box.TopLeft.X+box.Width, box.TopLeft.Y+box.Height),
		), // Right
		*geo.NewSegment(
			geo.NewPoint(box.TopLeft.X, box.TopLeft.Y+box.Height),
			geo.NewPoint(box.TopLeft.X+box.Width, box.TopLeft.Y+box.Height),
		), // Bottom
		*geo.NewSegment(
			geo.NewPoint(box.TopLeft.X, box.TopLeft.Y),
			geo.NewPoint(box.TopLeft.X, box.TopLeft.Y+box.Height),
		), // Left
	}

	// Construct the line from p1 (inside) to p2 (outside).
	line := *geo.NewSegment(p1, p2)
	var closestIntersection *geo.Point
	minDist := math.MaxFloat64

	// Find the intersection among the four edges that is closest to p1.
	for _, seg := range edges {
		if intersection := findSegmentIntersection(line, seg); intersection != nil {
			dist := math.Hypot(intersection.X-p1.X, intersection.Y-p1.Y)
			if dist < minDist {
				minDist = dist
				closestIntersection = intersection
			}
		}
	}

	if closestIntersection != nil {
		return snapToBoundary(box, closestIntersection)
	}
	return p1
}

// findSegmentIntersection computes the intersection between two line segments s1 and s2 using their parametric form.
func findSegmentIntersection(s1, s2 geo.Segment) *geo.Point {
	x1, y1 := s1.Start.X, s1.Start.Y
	x2, y2 := s1.End.X, s1.End.Y
	x3, y3 := s2.Start.X, s2.Start.Y
	x4, y4 := s2.End.X, s2.End.Y

	denom := (x1-x2)*(y3-y4) - (y1-y2)*(x3-x4)
	if math.Abs(denom) < EPSILON {
		return nil
	}

	t := ((x1-x3)*(y3-y4) - (y1-y3)*(x3-x4)) / denom
	u := -((x1-x2)*(y1-y3) - (y1-y2)*(x1-x3)) / denom

	if t >= 0 && t <= 1 && u >= 0 && u <= 1 {
		x := x1 + t*(x2-x1)
		y := y1 + t*(y2-y1)
		return geo.NewPoint(x, y)
	}
	return nil
}

// snapToBoundary adjusts point p so that it lies exactly on the nearest boundary of box.
func snapToBoundary(box *geo.Box, p *geo.Point) *geo.Point {
	left := box.TopLeft.X
	right := box.TopLeft.X + box.Width
	top := box.TopLeft.Y
	bottom := box.TopLeft.Y + box.Height

	dLeft := math.Abs(p.X - left)
	dRight := math.Abs(p.X - right)
	dTop := math.Abs(p.Y - top)
	dBottom := math.Abs(p.Y - bottom)

	if dLeft < dRight && dLeft < dTop && dLeft < dBottom {
		return geo.NewPoint(left, p.Y)
	} else if dRight < dLeft && dRight < dTop && dRight < dBottom {
		return geo.NewPoint(right, p.Y)
	} else if dTop < dBottom {
		return geo.NewPoint(p.X, top)
	} else {
		return geo.NewPoint(p.X, bottom)
	}
}

// boxContains returns true if point p is inside the box (using EPSILON for floating point tolerance).
func boxContains(b *geo.Box, p *geo.Point) bool {
	return p.X >= b.TopLeft.X-EPSILON &&
		p.X <= b.TopLeft.X+b.Width+EPSILON &&
		p.Y >= b.TopLeft.Y-EPSILON &&
		p.Y <= b.TopLeft.Y+b.Height+EPSILON
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
