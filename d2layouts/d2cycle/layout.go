package d2cycle

import (
	"context"
	"math"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
	"oss.terrastruct.com/util-go/go2"
)

const (
	MIN_RADIUS = 200
	PADDING    = 20
)

// Layout arranges nodes in a circle and routes edges along circular arcs.
func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) error {
	objects := g.Root.ChildrenArray
	if len(objects) == 0 {
		return nil
	}

	if layout != nil {
		if err := layout(ctx, g); err != nil {
			return err
		}
	}

	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	radius := calculateRadius(objects)
	positionObjects(objects, radius)

	for _, edge := range g.Edges {
		if isCycleEdge(g, edge) {
			createCircularArc(edge)
		}
	}

	return nil
}

func calculateRadius(objects []*d2graph.Object) float64 {
	if len(objects) == 1 {
		return 0
	}

	numObjects := float64(len(objects))
	maxSize := 0.0
	for _, obj := range objects {
		size := math.Hypot(obj.Box.Width/2, obj.Box.Height/2)
		maxSize = math.Max(maxSize, size)
	}
	minRadius := (maxSize + PADDING) / math.Sin(math.Pi/numObjects)
	return math.Max(minRadius, MIN_RADIUS)
}

func positionObjects(objects []*d2graph.Object, radius float64) {
	numObjects := float64(len(objects))
	angleOffset := -math.Pi / 2

	for i, obj := range objects {
		angle := angleOffset + (2 * math.Pi * float64(i) / numObjects)
		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)
		obj.MoveWithDescendantsTo(x-obj.Box.Width/2, y-obj.Box.Height/2)
	}
}

func isCycleEdge(g *d2graph.Graph, edge *d2graph.Edge) bool {
	if edge.Src == nil || edge.Dst == nil {
		return false
	}
	return edge.Src.Parent == g.Root && edge.Dst.Parent == g.Root
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
	if arcRadius == 0 {
		edge.Route = fallbackRoute(edge)
		return
	}

	startAngle := trimStartAngle(edge.Src.Box, arcRadius, srcAngle, dstAngle)
	endAngle := trimEndAngle(edge.Dst.Box, arcRadius, startAngle, dstAngle)
	if endAngle <= startAngle {
		edge.Route = fallbackRoute(edge)
		return
	}

	edge.Route = cubicArcRoute(arcRadius, startAngle, endAngle)
	clipArcToShapeBorders(edge)
	edge.IsCurve = true
}

func fallbackRoute(edge *d2graph.Edge) []*geo.Point {
	route := []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
	edge.TraceToShape(route, 0, 1)
	return route
}

func clipArcToShapeBorders(edge *d2graph.Edge) {
	if len(edge.Route) < 4 {
		return
	}

	srcShape := edge.Src.ToShape()
	dstShape := edge.Dst.ToShape()
	edge.Route[0] = shape.TraceToShapeBorder(srcShape, edge.Route[0], edge.Route[1])

	last := len(edge.Route) - 1
	edge.Route[last] = shape.TraceToShapeBorder(dstShape, edge.Route[last], edge.Route[last-1])
}

func trimStartAngle(box *geo.Box, radius, startAngle, endAngle float64) float64 {
	if !box.Contains(pointOnCircle(radius, startAngle)) {
		return startAngle
	}

	low, high := startAngle, endAngle
	for i := 0; i < 64; i++ {
		mid := (low + high) / 2
		if box.Contains(pointOnCircle(radius, mid)) {
			low = mid
		} else {
			high = mid
		}
	}
	return high
}

func trimEndAngle(box *geo.Box, radius, startAngle, endAngle float64) float64 {
	if !box.Contains(pointOnCircle(radius, endAngle)) {
		return endAngle
	}

	low, high := startAngle, endAngle
	for i := 0; i < 64; i++ {
		mid := (low + high) / 2
		if box.Contains(pointOnCircle(radius, mid)) {
			high = mid
		} else {
			low = mid
		}
	}
	return high
}

func pointOnCircle(radius, angle float64) *geo.Point {
	return geo.NewPoint(radius*math.Cos(angle), radius*math.Sin(angle))
}

func cubicArcRoute(radius, startAngle, endAngle float64) []*geo.Point {
	segments := int(math.Ceil((endAngle - startAngle) / (math.Pi / 2)))
	angleStep := (endAngle - startAngle) / float64(segments)

	route := make([]*geo.Point, 0, 1+segments*3)
	route = append(route, pointOnCircle(radius, startAngle))
	for i := 0; i < segments; i++ {
		a1 := startAngle + float64(i)*angleStep
		a2 := a1 + angleStep
		route = append(route, cubicArcSegment(radius, a1, a2)...)
	}
	return route
}

func cubicArcSegment(radius, startAngle, endAngle float64) []*geo.Point {
	delta := endAngle - startAngle
	k := 4.0 / 3.0 * math.Tan(delta/4.0)

	p0 := pointOnCircle(radius, startAngle)
	p3 := pointOnCircle(radius, endAngle)
	p1 := geo.NewPoint(
		p0.X-k*radius*math.Sin(startAngle),
		p0.Y+k*radius*math.Cos(startAngle),
	)
	p2 := geo.NewPoint(
		p3.X+k*radius*math.Sin(endAngle),
		p3.Y-k*radius*math.Cos(endAngle),
	)
	return []*geo.Point{p1, p2, p3}
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
