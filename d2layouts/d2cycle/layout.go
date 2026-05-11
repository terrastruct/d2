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
	minRadius        = 200.0
	cyclePadding     = 60.0
	maxArcSweep      = math.Pi / 2
	intersectionPass = 32
)

func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) error {
	if len(g.Root.ChildrenArray) == 0 {
		return nil
	}

	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	radius := calculateRadius(g.Root.ChildrenArray)
	center := positionObjects(g.Root.ChildrenArray, radius)
	sizeRoot(g.Root)

	for _, edge := range g.Edges {
		routeCircularArc(edge, center, radius)
	}

	return nil
}

func calculateRadius(objects []*d2graph.Object) float64 {
	if len(objects) < 2 {
		return minRadius
	}

	maxHalfDiagonal := 0.0
	for _, obj := range objects {
		maxHalfDiagonal = math.Max(maxHalfDiagonal, math.Hypot(obj.Width/2, obj.Height/2))
	}

	chordRadius := (maxHalfDiagonal + cyclePadding) / math.Sin(math.Pi/float64(len(objects)))
	return math.Max(minRadius, chordRadius)
}

func positionObjects(objects []*d2graph.Object, radius float64) *geo.Point {
	maxHalfWidth, maxHalfHeight := 0.0, 0.0
	for _, obj := range objects {
		maxHalfWidth = math.Max(maxHalfWidth, obj.Width/2)
		maxHalfHeight = math.Max(maxHalfHeight, obj.Height/2)
	}

	center := geo.NewPoint(radius+maxHalfWidth+cyclePadding, radius+maxHalfHeight+cyclePadding)
	for i, obj := range objects {
		angle := angleForIndex(i, len(objects))
		nodeCenter := pointOnCircle(center, radius, angle)
		obj.TopLeft = geo.NewPoint(nodeCenter.X-obj.Width/2, nodeCenter.Y-obj.Height/2)
	}
	return center
}

func sizeRoot(root *d2graph.Object) {
	maxRight, maxBottom := 0.0, 0.0
	for _, obj := range root.ChildrenArray {
		maxRight = math.Max(maxRight, obj.TopLeft.X+obj.Width)
		maxBottom = math.Max(maxBottom, obj.TopLeft.Y+obj.Height)
	}
	root.TopLeft = geo.NewPoint(0, 0)
	root.Box = geo.NewBox(root.TopLeft, maxRight+cyclePadding, maxBottom+cyclePadding)
}

func routeCircularArc(edge *d2graph.Edge, center *geo.Point, radius float64) {
	if edge.Src == nil || edge.Dst == nil || radius == 0 {
		return
	}

	srcAngle := angleOf(center, edge.Src.Center())
	dstAngle := angleOf(center, edge.Dst.Center())
	if dstAngle <= srcAngle {
		dstAngle += 2 * math.Pi
	}

	startAngle := exitAngle(edge.Src.Box, center, radius, srcAngle, dstAngle)
	endAngle := enterAngle(edge.Dst.Box, center, radius, startAngle, dstAngle)
	if endAngle <= startAngle {
		edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
		edge.TraceToShape(edge.Route, 0, 1)
		return
	}

	edge.Route = cubicArcRoute(center, radius, startAngle, endAngle)
	edge.IsCurve = true
	if edge.Label.Value != "" {
		edge.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
	}
}

func angleForIndex(index, total int) float64 {
	return -math.Pi/2 + 2*math.Pi*float64(index)/float64(total)
}

func angleOf(center, point *geo.Point) float64 {
	return math.Atan2(point.Y-center.Y, point.X-center.X)
}

func pointOnCircle(center *geo.Point, radius, angle float64) *geo.Point {
	return geo.NewPoint(center.X+radius*math.Cos(angle), center.Y+radius*math.Sin(angle))
}

func exitAngle(box *geo.Box, center *geo.Point, radius, startAngle, endAngle float64) float64 {
	if box == nil || !box.Contains(pointOnCircle(center, radius, startAngle)) {
		return startAngle
	}

	lo, hi := startAngle, endAngle
	for i := 0; i < intersectionPass; i++ {
		mid := (lo + hi) / 2
		if box.Contains(pointOnCircle(center, radius, mid)) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}

func enterAngle(box *geo.Box, center *geo.Point, radius, startAngle, endAngle float64) float64 {
	if box == nil || !box.Contains(pointOnCircle(center, radius, endAngle)) {
		return endAngle
	}

	lo, hi := startAngle, endAngle
	for i := 0; i < intersectionPass; i++ {
		mid := (lo + hi) / 2
		if box.Contains(pointOnCircle(center, radius, mid)) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi
}

func cubicArcRoute(center *geo.Point, radius, startAngle, endAngle float64) []*geo.Point {
	route := []*geo.Point{pointOnCircle(center, radius, startAngle)}
	remaining := endAngle - startAngle
	segments := int(math.Ceil(remaining / maxArcSweep))
	sweep := remaining / float64(segments)

	for i := 0; i < segments; i++ {
		a0 := startAngle + sweep*float64(i)
		a1 := a0 + sweep
		k := 4.0 / 3.0 * math.Tan((a1-a0)/4.0)

		p0 := pointOnCircle(center, radius, a0)
		p3 := pointOnCircle(center, radius, a1)
		cp1 := geo.NewPoint(
			p0.X+(-math.Sin(a0))*radius*k,
			p0.Y+(math.Cos(a0))*radius*k,
		)
		cp2 := geo.NewPoint(
			p3.X-(-math.Sin(a1))*radius*k,
			p3.Y-(math.Cos(a1))*radius*k,
		)
		route = append(route, cp1, cp2, p3)
	}
	return route
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
