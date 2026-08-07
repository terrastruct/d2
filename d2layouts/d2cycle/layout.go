package d2cycle

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
	"github.com/d2lang/util-go/go2"
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
	srcShape := edge.Src.ToShape()
	dstShape := edge.Dst.ToShape()
	if sweep <= 0 {
		fallbackStraightRoute(edge, srcShape, dstShape, srcCenter, dstCenter)
		return
	}

	startAngle, hasStart := nextBoundaryAngle(edge.Src.Box, origin, radius, srcAngle, sweep, true)
	endAngle, hasEnd := nextBoundaryAngle(edge.Dst.Box, origin, radius, srcAngle, sweep, false)
	if !hasStart || !hasEnd || endAngle <= startAngle {
		fallbackStraightRoute(edge, srcShape, dstShape, srcCenter, dstCenter)
		return
	}

	path := make([]*geo.Point, 0, ARC_STEPS+1)
	for i := 0; i <= ARC_STEPS; i++ {
		t := float64(i) / float64(ARC_STEPS)
		angle := startAngle + t*(endAngle-startAngle)
		path = append(path, geo.NewPoint(radius*math.Cos(angle), radius*math.Sin(angle)))
	}

	// path[0] / path[len-1] sit on the bounding-box border of the source /
	// destination shape. For non-rectangular shapes (circle, oval, hexagon,
	// cloud, ...) the bounding box border is not the shape border, so trace
	// each endpoint inward from the shape center to the actual shape outline.
	// TraceToShapeBorder is a no-op for rectangular shapes.
	path[0] = shape.TraceToShapeBorder(srcShape, path[0], srcCenter)
	path[len(path)-1] = shape.TraceToShapeBorder(dstShape, path[len(path)-1], dstCenter)

	edge.Route = path
	edge.IsCurve = true
}

// fallbackStraightRoute renders a straight connection whose endpoints are
// clipped to the source and destination shape borders along the line between
// the two centers, used when the analytic arc geometry degenerates (zero
// sweep, no boundary crossing in the arc range, etc.).
func fallbackStraightRoute(edge *d2graph.Edge, srcShape, dstShape shape.Shape, srcCenter, dstCenter *geo.Point) {
	srcBorder := clipToShapeBorder(srcShape, edge.Src.Box, srcCenter, dstCenter)
	dstBorder := clipToShapeBorder(dstShape, edge.Dst.Box, dstCenter, srcCenter)
	edge.Route = []*geo.Point{srcBorder, dstBorder}
	edge.IsCurve = false
}

// clipToShapeBorder returns the point where a ray from `from` (assumed inside
// `box`) toward `toward` first exits the actual shape outline. The bounding
// box is consulted first to obtain a rectangular border point, then the shape
// helper refines it for non-rectangular shapes.
func clipToShapeBorder(shp shape.Shape, box *geo.Box, from, toward *geo.Point) *geo.Point {
	dx := toward.X - from.X
	dy := toward.Y - from.Y
	dist := math.Hypot(dx, dy)
	if dist == 0 {
		return from
	}
	// Extend the ray well past `toward` so the segment definitely exits the
	// box even when `toward` itself sits inside the box.
	diag := math.Hypot(box.Width, box.Height)
	scale := (dist + 2*diag) / dist
	extended := geo.NewPoint(from.X+dx*scale, from.Y+dy*scale)

	rectBorder := extended
	if pts := box.Intersections(geo.Segment{Start: from, End: extended}); len(pts) > 0 {
		rectBorder = pts[0]
	}
	return shape.TraceToShapeBorder(shp, rectBorder, from)
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
