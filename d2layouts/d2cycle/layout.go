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
	ARC_STEPS       = 30 // high resolution for smooth arcs

)

// Layout arranges nodes in a circle, ensures label/icon positions are set,
// then routes edges with arcs that get clipped at node borders.
func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) error {
	objects := g.Root.ChildrenArray
	if len(objects) == 0 {
		return nil
	}

	// Make sure every object that has label/icon also has a default position
	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	// Arrange objects in a circle
	radius := calculateRadius(objects)
	positionObjects(objects, radius)

	// Create arcs
	for _, edge := range g.Edges {
		createCircularArc(edge)
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
	// ensure enough radius to fit all objects
	minRadius := (maxSize/2.0 + PADDING) / math.Sin(math.Pi/numObjects)
	return math.Max(minRadius, MIN_RADIUS)
}

func positionObjects(objects []*d2graph.Object, radius float64) {
	numObjects := float64(len(objects))
	// Offset so i=0 is top-center
	angleOffset := -math.Pi / 2

	for i, obj := range objects {
		angle := angleOffset + (2 * math.Pi * float64(i) / numObjects)

		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)

		// center the box at (x, y)
		obj.TopLeft = geo.NewPoint(
			x-obj.Box.Width/2,
			y-obj.Box.Height/2,
		)
	}
}

// createCircularArc samples a smooth arc from center to center, then
// forces the endpoints onto each shape's border, and finally calls
// TraceToShape to clip any additional overrun.
func createCircularArc(edge *d2graph.Edge) {
	if edge.Src == nil || edge.Dst == nil {
		return
	}

	srcCenter := edge.Src.Center()
	dstCenter := edge.Dst.Center()

	// angles from origin
	srcAngle := math.Atan2(srcCenter.Y, srcCenter.X)
	dstAngle := math.Atan2(dstCenter.Y, dstCenter.X)
	if dstAngle < srcAngle {
		dstAngle += 2 * math.Pi
	}

	arcRadius := math.Hypot(srcCenter.X, srcCenter.Y)

	// Sample points along the arc
	path := make([]*geo.Point, 0, ARC_STEPS+1)
	for i := 0; i <= ARC_STEPS; i++ {
		t := float64(i) / float64(ARC_STEPS)
		angle := srcAngle + t*(dstAngle-srcAngle)
		x := arcRadius * math.Cos(angle)
		y := arcRadius * math.Sin(angle)
		path = append(path, geo.NewPoint(x, y))
	}
	// Set start/end to exact centers
	path[0] = srcCenter
	path[len(path)-1] = dstCenter

	// Use TraceToShape to clip route to node borders
	edge.Route = path
	startIndex, endIndex := edge.TraceToShape(edge.Route, 0, len(edge.Route)-1)
	if startIndex < endIndex {
		edge.Route = edge.Route[startIndex : endIndex+1]
	}
	edge.IsCurve = true
}

// clampPointOutsideBox walks forward from 'startIdx' until the path segment
// leaves the bounding box. Then it sets path[startIdx] to the intersection.
// If we never find it, we return (startIdx, path[startIdx]) meaning we can't clamp.
func clampPointOutsideBox(box *geo.Box, path []*geo.Point, startIdx int) (int, *geo.Point) {
	if startIdx >= len(path)-1 {
		return startIdx, path[startIdx]
	}
	// If path[startIdx] is outside, no clamp needed
	if !boxContains(box, path[startIdx]) {
		return startIdx, path[startIdx]
	}

	// Walk forward looking for outside
	for i := startIdx + 1; i < len(path); i++ {
		insideNext := boxContains(box, path[i])
		if insideNext {
			// still inside -> keep going
			continue
		}
		// crossing from inside to outside between path[i-1], path[i]
		seg := geo.NewSegment(path[i-1], path[i])
		inters := boxIntersections(box, *seg)
		if len(inters) > 0 {
			// use first intersection
			return i, inters[0]
		}
		// fallback => no intersection found
		return i, path[i]
	}
	// entire remainder is inside, so we can't clamp
	// Just return the end
	last := len(path) - 1
	return last, path[last]
}

// clampPointOutsideBoxReverse scans backward from endIdx while path[j] is in the box.
// Once we find crossing (outside→inside), we return (j, intersection).
func clampPointOutsideBoxReverse(box *geo.Box, path []*geo.Point, endIdx int) (int, *geo.Point) {
	if endIdx <= 0 {
		return endIdx, path[endIdx]
	}
	if !boxContains(box, path[endIdx]) {
		// already outside
		return endIdx, path[endIdx]
	}

	for j := endIdx - 1; j >= 0; j-- {
		if boxContains(box, path[j]) {
			continue
		}
		// crossing from outside -> inside between path[j], path[j+1]
		seg := geo.NewSegment(path[j], path[j+1])
		inters := boxIntersections(box, *seg)
		if len(inters) > 0 {
			return j, inters[0]
		}
		return j, path[j]
	}

	// entire path inside
	return 0, path[0]
}

// Helper if your geo.Box doesn’t implement Contains()
func boxContains(b *geo.Box, p *geo.Point) bool {
	// typical bounding-box check
	return p.X >= b.TopLeft.X &&
		p.X <= b.TopLeft.X+b.Width &&
		p.Y >= b.TopLeft.Y &&
		p.Y <= b.TopLeft.Y+b.Height
}

// Helper if your geo.Box doesn’t implement Intersections(geo.Segment) yet
func boxIntersections(b *geo.Box, seg geo.Segment) []*geo.Point {
	// We'll assume d2's standard geo.Box has a built-in Intersections(*Segment) method.
	// If not, implement manually. For example, checking each of the 4 edges:
	//   left, right, top, bottom
	// For simplicity, if you do have b.Intersections(...) you can just do:
	//     return b.Intersections(seg)
	return b.Intersections(seg)
	// If you don't have that, you'd code the line-rect intersection yourself.
}

// positionLabelsIcons is basically your logic that sets default label/icon positions if needed
func positionLabelsIcons(obj *d2graph.Object) {
	// If there's an icon but no icon position, give it a default
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

	// If there's a label but no label position, give it a default
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

		// If the label is bigger than the shape, fallback to outside positions
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
