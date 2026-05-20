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
	minRadius   = 200.
	padding     = 24.
	maxArcSweep = math.Pi / 2
	epsilon     = 1e-6
)

type anglePoint struct {
	angle float64
	point *geo.Point
}

// Layout arranges the root children around a circle and routes their edges as
// cubic Bezier arcs trimmed to each shape's visible perimeter.
func Layout(_ context.Context, g *d2graph.Graph, _ d2graph.LayoutGraph) error {
	objects := g.Root.ChildrenArray
	if len(objects) == 0 {
		return nil
	}

	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	radius := calculateRadius(objects)
	positionObjects(objects, radius)

	center := geo.NewPoint(0, 0)
	for _, edge := range g.Edges {
		routeCircularArc(edge, center)
	}
	normalizeGraph(g)
	return nil
}

func calculateRadius(objects []*d2graph.Object) float64 {
	if len(objects) < 2 {
		return minRadius
	}

	maxHalfDiagonal := 0.
	for _, obj := range objects {
		maxHalfDiagonal = math.Max(maxHalfDiagonal, math.Hypot(obj.Width/2, obj.Height/2))
	}

	minimum := (maxHalfDiagonal + padding) / math.Sin(math.Pi/float64(len(objects)))
	return math.Max(minimum, minRadius)
}

func positionObjects(objects []*d2graph.Object, radius float64) {
	for i, obj := range objects {
		angle := -math.Pi/2 + 2*math.Pi*float64(i)/float64(len(objects))
		center := pointOnCircle(geo.NewPoint(0, 0), radius, angle)
		obj.TopLeft = geo.NewPoint(center.X-obj.Width/2, center.Y-obj.Height/2)
	}
}

func normalizeGraph(g *d2graph.Graph) {
	tl := geo.NewPoint(math.Inf(1), math.Inf(1))
	br := geo.NewPoint(math.Inf(-1), math.Inf(-1))

	for _, obj := range g.Objects {
		if obj.TopLeft == nil {
			continue
		}
		tl.X = math.Min(tl.X, obj.TopLeft.X)
		tl.Y = math.Min(tl.Y, obj.TopLeft.Y)
		br.X = math.Max(br.X, obj.TopLeft.X+obj.Width)
		br.Y = math.Max(br.Y, obj.TopLeft.Y+obj.Height)
	}
	for _, edge := range g.Edges {
		for _, point := range edge.Route {
			tl.X = math.Min(tl.X, point.X)
			tl.Y = math.Min(tl.Y, point.Y)
			br.X = math.Max(br.X, point.X)
			br.Y = math.Max(br.Y, point.Y)
		}
	}

	if math.IsInf(tl.X, 0) || math.IsInf(tl.Y, 0) {
		return
	}

	dx := -tl.X
	dy := -tl.Y
	for _, obj := range g.Objects {
		if obj.TopLeft != nil {
			obj.TopLeft.X += dx
			obj.TopLeft.Y += dy
		}
	}
	for _, edge := range g.Edges {
		edge.Move(dx, dy)
	}
	g.Root.Box = geo.NewBox(geo.NewPoint(0, 0), br.X-tl.X, br.Y-tl.Y)
}

func routeCircularArc(edge *d2graph.Edge, center *geo.Point) {
	if edge.Src == nil || edge.Dst == nil {
		return
	}

	srcCenter := edge.Src.Center()
	dstCenter := edge.Dst.Center()
	radius := (distance(center, srcCenter) + distance(center, dstCenter)) / 2
	if radius == 0 {
		return
	}

	startAngle := angleOf(center, srcCenter)
	endAngle := advanceAngle(startAngle, angleOf(center, dstCenter))
	if edge.Src == edge.Dst || endAngle-startAngle < epsilon {
		endAngle = startAngle + 2*math.Pi
	}

	start := findBoundaryAngle(edge.Src, center, radius, startAngle, endAngle, true)
	end := findBoundaryAngle(edge.Dst, center, radius, startAngle, endAngle, false)
	if start == nil || end == nil || end.angle-start.angle < epsilon {
		routeStraight(edge)
		return
	}

	edge.Route = buildArcRoute(center, radius, start.angle, end.angle)
	if len(edge.Route) >= 4 {
		edge.Route[0] = start.point
		edge.Route[len(edge.Route)-1] = end.point
		edge.IsCurve = true
		return
	}

	routeStraight(edge)
}

func routeStraight(edge *d2graph.Edge) {
	edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
	edge.TraceToShape(edge.Route, 0, 1)
	edge.IsCurve = false
}

func findBoundaryAngle(obj *d2graph.Object, center *geo.Point, radius, startAngle, endAngle float64, fromStart bool) *anglePoint {
	var selected *anglePoint
	for _, point := range circleShapeIntersections(obj.ToShape(), center, radius, startAngle, endAngle) {
		angle := advanceAngle(startAngle, angleOf(center, point))
		if angle <= startAngle+epsilon || angle >= endAngle-epsilon {
			continue
		}

		candidate := &anglePoint{
			angle: angle,
			point: truncatePoint(point),
		}
		if selected == nil ||
			(fromStart && candidate.angle < selected.angle) ||
			(!fromStart && candidate.angle > selected.angle) {
			selected = candidate
		}
	}

	if selected != nil {
		return selected
	}
	return fallbackBoundaryAngle(obj, center, radius, startAngle, endAngle, fromStart)
}

func fallbackBoundaryAngle(obj *d2graph.Object, center *geo.Point, radius, startAngle, endAngle float64, fromStart bool) *anglePoint {
	box := obj.Box
	if box == nil {
		return nil
	}

	low := startAngle
	high := endAngle
	if fromStart {
		for i := 0; i < 48; i++ {
			mid := (low + high) / 2
			if box.Contains(pointOnCircle(center, radius, mid)) {
				low = mid
			} else {
				high = mid
			}
		}
		angle := high
		point := shape.TraceToShapeBorder(obj.ToShape(), pointOnCircle(center, radius, angle), pointOnCircle(center, radius, angle+0.001))
		return &anglePoint{angle: angle, point: truncatePoint(point)}
	}

	for i := 0; i < 48; i++ {
		mid := (low + high) / 2
		if box.Contains(pointOnCircle(center, radius, mid)) {
			high = mid
		} else {
			low = mid
		}
	}
	angle := low
	point := shape.TraceToShapeBorder(obj.ToShape(), pointOnCircle(center, radius, angle), pointOnCircle(center, radius, angle-0.001))
	return &anglePoint{angle: angle, point: truncatePoint(point)}
}

func buildArcRoute(center *geo.Point, radius, startAngle, endAngle float64) []*geo.Point {
	sweep := endAngle - startAngle
	segments := int(math.Ceil(sweep / maxArcSweep))
	if segments < 1 {
		segments = 1
	}

	route := []*geo.Point{truncatePoint(pointOnCircle(center, radius, startAngle))}
	for i := 0; i < segments; i++ {
		a0 := startAngle + sweep*float64(i)/float64(segments)
		a1 := startAngle + sweep*float64(i+1)/float64(segments)
		p0 := pointOnCircle(center, radius, a0)
		p3 := pointOnCircle(center, radius, a1)
		k := 4. / 3. * math.Tan((a1-a0)/4.)

		c1 := geo.NewPoint(
			p0.X+k*radius*-math.Sin(a0),
			p0.Y+k*radius*math.Cos(a0),
		)
		c2 := geo.NewPoint(
			p3.X-k*radius*-math.Sin(a1),
			p3.Y-k*radius*math.Cos(a1),
		)

		route = append(route, truncatePoint(c1), truncatePoint(c2), truncatePoint(p3))
	}
	return route
}

func circleShapeIntersections(s shape.Shape, center *geo.Point, radius, startAngle, endAngle float64) []*geo.Point {
	sweep := endAngle - startAngle
	steps := int(math.Ceil(sweep / (math.Pi / 180)))
	if steps < 24 {
		steps = 24
	}

	points := make([]*geo.Point, 0)
	for i := 0; i < steps; i++ {
		a0 := startAngle + sweep*float64(i)/float64(steps)
		a1 := startAngle + sweep*float64(i+1)/float64(steps)
		circleSegment := geo.Segment{
			Start: pointOnCircle(center, radius, a0),
			End:   pointOnCircle(center, radius, a1),
		}
		for _, perimeter := range s.Perimeter() {
			for _, point := range perimeter.Intersections(circleSegment) {
				if !hasNearbyPoint(points, point) {
					points = append(points, point)
				}
			}
		}
	}
	return points
}

func hasNearbyPoint(points []*geo.Point, point *geo.Point) bool {
	for _, existing := range points {
		if distance(existing, point) < 0.001 {
			return true
		}
	}
	return false
}

func pointOnCircle(center *geo.Point, radius, angle float64) *geo.Point {
	return geo.NewPoint(center.X+radius*math.Cos(angle), center.Y+radius*math.Sin(angle))
}

func angleOf(center, point *geo.Point) float64 {
	return math.Atan2(point.Y-center.Y, point.X-center.X)
}

func advanceAngle(reference, angle float64) float64 {
	for angle <= reference {
		angle += 2 * math.Pi
	}
	return angle
}

func distance(a, b *geo.Point) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func truncatePoint(point *geo.Point) *geo.Point {
	point = point.Copy()
	point.TruncateDecimals()
	return point
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
