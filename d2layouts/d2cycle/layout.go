// package d2cycle

// import (
// 	"context"
// 	"math"

// 	"oss.terrastruct.com/d2/d2graph"
// 	"oss.terrastruct.com/d2/lib/geo"
// 	"oss.terrastruct.com/d2/lib/label"
// 	"oss.terrastruct.com/util-go/go2"
// )

// const (
// 	MIN_RADIUS      = 200
// 	PADDING         = 20
// 	MIN_SEGMENT_LEN = 10
// 	ARC_STEPS       = 30
// )

// func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) error {
// 	objects := g.Root.ChildrenArray
// 	if len(objects) == 0 {
// 		return nil
// 	}

// 	for _, obj := range g.Objects {
// 		positionLabelsIcons(obj)
// 	}

// 	radius := calculateRadius(objects)
// 	positionObjects(objects, radius)

// 	for _, edge := range g.Edges {
// 		createCircularArc(edge)
// 	}

// 	return nil
// }

// func calculateRadius(objects []*d2graph.Object) float64 {
// 	numObjects := float64(len(objects))
// 	maxSize := 0.0
// 	for _, obj := range objects {
// 		size := math.Max(obj.Box.Width, obj.Box.Height)
// 		maxSize = math.Max(maxSize, size)
// 	}
// 	minRadius := (maxSize/2.0 + PADDING) / math.Sin(math.Pi/numObjects)
// 	return math.Max(minRadius, MIN_RADIUS)
// }

// func positionObjects(objects []*d2graph.Object, radius float64) {
// 	numObjects := float64(len(objects))
// 	angleOffset := -math.Pi / 2

// 	for i, obj := range objects {
// 		angle := angleOffset + (2*math.Pi*float64(i)/numObjects)
// 		x := radius * math.Cos(angle)
// 		y := radius * math.Sin(angle)

// 		obj.TopLeft = geo.NewPoint(
// 			x-obj.Box.Width/2,
// 			y-obj.Box.Height/2,
// 		)
// 	}
// }

// func createCircularArc(edge *d2graph.Edge) {
// 	if edge.Src == nil || edge.Dst == nil {
// 		return
// 	}

// 	srcCenter := edge.Src.Center()
// 	dstCenter := edge.Dst.Center()

// 	srcAngle := math.Atan2(srcCenter.Y, srcCenter.X)
// 	dstAngle := math.Atan2(dstCenter.Y, dstCenter.X)
// 	if dstAngle < srcAngle {
// 		dstAngle += 2 * math.Pi
// 	}

// 	arcRadius := math.Hypot(srcCenter.X, srcCenter.Y)

// 	path := make([]*geo.Point, 0, ARC_STEPS+1)
// 	for i := 0; i <= ARC_STEPS; i++ {
// 		t := float64(i) / float64(ARC_STEPS)
// 		angle := srcAngle + t*(dstAngle-srcAngle)
// 		x := arcRadius * math.Cos(angle)
// 		y := arcRadius * math.Sin(angle)
// 		path = append(path, geo.NewPoint(x, y))
// 	}
// 	path[0] = srcCenter
// 	path[len(path)-1] = dstCenter

// 	startIndex, newSrc := clampPointOutsideBox(edge.Src.Box, path, 0)
// 	endIndex, newDst := clampPointOutsideBoxReverse(edge.Dst.Box, path, len(path)-1)

// 	path[0] = newSrc
// 	path[len(path)-1] = newDst

// 	edge.Route = path[startIndex : endIndex+1]
// 	edge.IsCurve = true
// }

// func clampPointOutsideBox(box *geo.Box, path []*geo.Point, startIdx int) (int, *geo.Point) {
// 	if startIdx >= len(path)-1 {
// 		return startIdx, path[startIdx]
// 	}
// 	if !boxContains(box, path[startIdx]) {
// 		return startIdx, path[startIdx]
// 	}

// 	for i := startIdx + 1; i < len(path); i++ {
// 		if boxContains(box, path[i]) {
// 			continue
// 		}
// 		seg := geo.NewSegment(path[i-1], path[i])
// 		inters := boxIntersections(box, *seg)
// 		if len(inters) > 0 {
// 			return i, inters[0]
// 		}
// 		return i, path[i]
// 	}
// 	last := len(path) - 1
// 	return last, path[last]
// }

// func clampPointOutsideBoxReverse(box *geo.Box, path []*geo.Point, endIdx int) (int, *geo.Point) {
// 	if endIdx <= 0 {
// 		return endIdx, path[endIdx]
// 	}
// 	if !boxContains(box, path[endIdx]) {
// 		return endIdx, path[endIdx]
// 	}

// 	for j := endIdx - 1; j >= 0; j-- {
// 		if boxContains(box, path[j]) {
// 			continue
// 		}
// 		seg := geo.NewSegment(path[j], path[j+1])
// 		inters := boxIntersections(box, *seg)
// 		if len(inters) > 0 {
// 			return j, inters[0]
// 		}
// 		return j, path[j]
// 	}
// 	return 0, path[0]
// }

// func boxContains(b *geo.Box, p *geo.Point) bool {
// 	return p.X >= b.TopLeft.X &&
// 		p.X <= b.TopLeft.X+b.Width &&
// 		p.Y >= b.TopLeft.Y &&
// 		p.Y <= b.TopLeft.Y+b.Height
// }

// func boxIntersections(b *geo.Box, seg geo.Segment) []*geo.Point {
// 	return b.Intersections(seg)
// }

// func positionLabelsIcons(obj *d2graph.Object) {
// 	if obj.Icon != nil && obj.IconPosition == nil {
// 		if len(obj.ChildrenArray) > 0 {
// 			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
// 			if obj.LabelPosition == nil {
// 				obj.LabelPosition = go2.Pointer(label.OutsideTopRight.String())
// 				return
// 			}
// 		} else if obj.SQLTable != nil || obj.Class != nil || obj.Language != "" {
// 			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
// 		} else {
// 			obj.IconPosition = go2.Pointer(label.InsideMiddleCenter.String())
// 		}
// 	}

// 	if obj.HasLabel() && obj.LabelPosition == nil {
// 		if len(obj.ChildrenArray) > 0 {
// 			obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
// 		} else if obj.HasOutsideBottomLabel() {
// 			obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
// 		} else if obj.Icon != nil {
// 			obj.LabelPosition = go2.Pointer(label.InsideTopCenter.String())
// 		} else {
// 			obj.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
// 		}

// 		if float64(obj.LabelDimensions.Width) > obj.Width ||
// 			float64(obj.LabelDimensions.Height) > obj.Height {
// 			if len(obj.ChildrenArray) > 0 {
// 				obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
// 			} else {
// 				obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
// 			}
// 		}
// 	}
// }
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
)

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

// createCircularArc computes an arc along a circle from the source to destination,
// then computes the exact intersection points between a ray from the object's center
// and its bounding box. This ensures that the arrow precisely touches the boundary.
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

	// Here we use the source center's distance as the arc radius.
	arcRadius := math.Hypot(srcCenter.X, srcCenter.Y)

	// Sample points along the circular arc.
	path := make([]*geo.Point, 0, ARC_STEPS+1)
	for i := 0; i <= ARC_STEPS; i++ {
		t := float64(i) / float64(ARC_STEPS)
		angle := srcAngle + t*(dstAngle-srcAngle)
		x := arcRadius * math.Cos(angle)
		y := arcRadius * math.Sin(angle)
		path = append(path, geo.NewPoint(x, y))
	}
	// Instead of relying on iterative clamping, we compute exact intersection points.
	// For the source, the ray is from the center toward the second sample point.
	newSrc := rayRectangleIntersection(srcCenter, edge.Src.Box, path[1].X-srcCenter.X, path[1].Y-srcCenter.Y)
	// For the destination, the ray is from the center in the direction opposite to the second-to-last sample.
	newDst := rayRectangleIntersection(dstCenter, edge.Dst.Box, dstCenter.X-path[len(path)-2].X, dstCenter.Y-path[len(path)-2].Y)

	path[0] = newSrc
	path[len(path)-1] = newDst

	edge.Route = path
	edge.IsCurve = true
}

// rayRectangleIntersection computes the exact intersection point of a ray starting at 'center'
// with direction (dx, dy) and the boundary of an axis-aligned rectangle 'box'.
// The rectangle is defined by its TopLeft corner, width, and height.
func rayRectangleIntersection(center *geo.Point, box *geo.Box, dx, dy float64) *geo.Point {
	L := box.TopLeft.X
	R := box.TopLeft.X + box.Width
	T := box.TopLeft.Y
	B := box.TopLeft.Y + box.Height

	var tX, tY float64
	if dx > 0 {
		tX = (R - center.X) / dx
	} else if dx < 0 {
		tX = (L - center.X) / dx
	} else {
		tX = math.Inf(1)
	}

	if dy > 0 {
		tY = (B - center.Y) / dy
	} else if dy < 0 {
		tY = (T - center.Y) / dy
	} else {
		tY = math.Inf(1)
	}

	t := tX
	if tY < t {
		t = tY
	}

	return geo.NewPoint(center.X+dx*t, center.Y+dy*t)
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
