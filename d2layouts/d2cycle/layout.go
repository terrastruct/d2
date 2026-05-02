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
	MIN_RADIUS = 200
	PADDING    = 20
	ARC_STEPS  = 30
)

// Layout arranges nodes in a circle and routes each edge as a circular arc
// that starts and ends on the borders of its source and destination shapes.
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
		angle := angleOffset + (2 * math.Pi * float64(i) / numObjects)
		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)

		obj.TopLeft = geo.NewPoint(
			x-obj.Box.Width/2,
			y-obj.Box.Height/2,
		)
	}
}

// createCircularArc routes a single edge as a circular arc whose endpoints
// lie exactly on the borders of the source and destination shapes. The arc
// belongs to the layout circle, centered at the origin with the given radius;
// the source and destination shape centers both lie on that circle.
func createCircularArc(edge *d2graph.Edge, radius float64) {
	if edge.Src == nil || edge.Dst == nil {
		return
	}

	srcCenter := edge.Src.Center()
	dstCenter := edge.Dst.Center()
	origin := geo.NewPoint(0, 0)

	srcAngle := math.Atan2(srcCenter.Y, srcCenter.X)
	dstAngle := math.Atan2(dstCenter.Y, dstCenter.X)
	if dstAngle < srcAngle {
		dstAngle += 2 * math.Pi
	}
	sweep := dstAngle - srcAngle
	if sweep <= 0 {
		fallbackStraightRoute(edge, srcCenter, dstCenter)
		return
	}

	startAngle, hasStart := nextBoundaryAngle(edge.Src.Box, origin, radius, srcAngle, sweep, true)
	if !hasStart {
		startAngle = srcAngle
	}
	endAngle, hasEnd := nextBoundaryAngle(edge.Dst.Box, origin, radius, srcAngle, sweep, false)
	if !hasEnd {
		endAngle = dstAngle
	}
	if endAngle <= startAngle {
		fallbackStraightRoute(edge, srcCenter, dstCenter)
		return
	}

	path := make([]*geo.Point, 0, ARC_STEPS+1)
	for i := 0; i <= ARC_STEPS; i++ {
		t := float64(i) / float64(ARC_STEPS)
		angle := startAngle + t*(endAngle-startAngle)
		path = append(path, geo.NewPoint(radius*math.Cos(angle), radius*math.Sin(angle)))
	}

	edge.Route = path
	edge.IsCurve = true
}

func fallbackStraightRoute(edge *d2graph.Edge, srcCenter, dstCenter *geo.Point) {
	edge.Route = []*geo.Point{srcCenter, dstCenter}
	edge.IsCurve = false
}

// nextBoundaryAngle scans the angles where the layout circle crosses an edge
// of the box. When forSrc is true it returns the smallest such angle strictly
// greater than srcAngle (the point where the arc exits the source box). When
// forSrc is false it returns the largest such angle strictly less than
// srcAngle+sweep (the point where the arc enters the destination box). The
// boolean is false when no crossing exists in the (srcAngle, srcAngle+sweep)
// range, in which case the caller falls back to the shape center.
func nextBoundaryAngle(box *geo.Box, origin *geo.Point, radius, srcAngle, sweep float64, forSrc bool) (float64, bool) {
	candidates := boxCircleIntersectionAngles(box, origin, radius)
	endAngle := srcAngle + sweep

	var best float64
	found := false
	for _, raw := range candidates {
		a := raw
		for a <= srcAngle {
			a += 2 * math.Pi
		}
		for a > srcAngle+2*math.Pi {
			a -= 2 * math.Pi
		}
		if a >= endAngle {
			continue
		}
		if !found {
			best = a
			found = true
			continue
		}
		if forSrc {
			if a < best {
				best = a
			}
		} else {
			if a > best {
				best = a
			}
		}
	}
	return best, found
}

func boxCircleIntersectionAngles(box *geo.Box, origin *geo.Point, radius float64) []float64 {
	edges := boxEdges(box)
	var angles []float64
	for _, e := range edges {
		for _, p := range e.IntersectCircle(origin, radius) {
			angles = append(angles, math.Atan2(p.Y-origin.Y, p.X-origin.X))
		}
	}
	return angles
}

func boxEdges(box *geo.Box) []geo.Segment {
	tl := box.TopLeft
	tr := geo.NewPoint(tl.X+box.Width, tl.Y)
	bl := geo.NewPoint(tl.X, tl.Y+box.Height)
	br := geo.NewPoint(tl.X+box.Width, tl.Y+box.Height)
	return []geo.Segment{
		{Start: tl, End: tr},
		{Start: tr, End: br},
		{Start: br, End: bl},
		{Start: bl, End: tl},
	}
}

// positionLabelsIcons applies a sensible default label/icon position when one
// has not been explicitly specified.
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
