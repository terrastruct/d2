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

	// number of chords used to search for the arc/border crossing
	BORDER_SEARCH_STEPS = 100
	// bisection iterations to refine the crossing angle
	BORDER_REFINE_STEPS = 30
)

// Layout arranges the graph's root objects on a circle and routes each edge
// as a circular arc that starts and ends exactly on the shape borders.
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
	if len(objects) < 2 {
		return MIN_RADIUS
	}
	numObjects := float64(len(objects))
	maxSize := 0.0
	for _, obj := range objects {
		size := math.Max(obj.Box.Width, obj.Box.Height)
		maxSize = math.Max(maxSize, size)
	}
	// ensure neighboring objects don't overlap
	minRadius := (maxSize/2.0 + PADDING) / math.Sin(math.Pi/numObjects)
	return math.Max(minRadius, MIN_RADIUS)
}

func positionObjects(objects []*d2graph.Object, radius float64) {
	numObjects := float64(len(objects))
	// offset so the first object is at the top-center
	angleOffset := -math.Pi / 2

	for i, obj := range objects {
		angle := angleOffset + (2 * math.Pi * float64(i) / numObjects)

		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)

		// center the box at (x, y), snapped to integer coordinates so the
		// box is not shifted later when positions are truncated for export
		obj.TopLeft = geo.NewPoint(
			math.Round(x-obj.Box.Width/2),
			math.Round(y-obj.Box.Height/2),
		)
	}
}

// createCircularArc routes an edge as a circular arc on the layout circle.
// The arc is clipped so it starts on the source shape's border and ends on
// the destination shape's border, then emitted as cubic Bézier segments so
// it renders perfectly smooth.
func createCircularArc(edge *d2graph.Edge) {
	if edge.Src == nil || edge.Dst == nil ||
		edge.Src.TopLeft == nil || edge.Dst.TopLeft == nil {
		return
	}

	srcCenter := edge.Src.Center()
	dstCenter := edge.Dst.Center()

	srcAngle := math.Atan2(srcCenter.Y, srcCenter.X)
	dstAngle := math.Atan2(dstCenter.Y, dstCenter.X)
	// always route the arc in the direction of increasing angle
	// (this also makes a self-referencing edge a full loop)
	if dstAngle <= srcAngle {
		dstAngle += 2 * math.Pi
	}

	arcRadius := (math.Hypot(srcCenter.X, srcCenter.Y) + math.Hypot(dstCenter.X, dstCenter.Y)) / 2

	// clip the arc to the shape borders
	startAngle, foundStart := findBorderCrossing(edge.Src, arcRadius, srcAngle, dstAngle)
	endAngle, foundEnd := findBorderCrossing(edge.Dst, arcRadius, dstAngle, srcAngle)
	if !foundStart || !foundEnd || startAngle >= endAngle {
		// fallback: keep the center-to-center arc
		startAngle = srcAngle
		endAngle = dstAngle
	}

	edge.Route = arcToBeziers(arcRadius, startAngle, endAngle)
	edge.IsCurve = true

	if edge.Label.Value != "" && edge.LabelPosition == nil {
		edge.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
	}
}

// findBorderCrossing finds the angle at which the circle of arcRadius
// (centered at the origin) crosses obj's border, walking from fromAngle
// (at obj's center, inside obj) toward toAngle. It reports whether a
// crossing was found.
func findBorderCrossing(obj *d2graph.Object, arcRadius, fromAngle, toAngle float64) (float64, bool) {
	shape := obj.ToShape()
	var perimeter []geo.Intersectable
	if shape.Is("") || shape.IsRectangular() {
		// rectangular shapes are clipped at their bounding box
		// (they don't define a Perimeter, see shape.TraceToShapeBorder)
		box := shape.GetBox()
		tl := box.TopLeft
		tr := geo.NewPoint(tl.X+box.Width, tl.Y)
		br := geo.NewPoint(tl.X+box.Width, tl.Y+box.Height)
		bl := geo.NewPoint(tl.X, tl.Y+box.Height)
		perimeter = []geo.Intersectable{
			*geo.NewSegment(tl, tr),
			*geo.NewSegment(tr, br),
			*geo.NewSegment(br, bl),
			*geo.NewSegment(bl, tl),
		}
	} else {
		perimeter = shape.Perimeter()
	}
	if len(perimeter) == 0 {
		return 0, false
	}

	arcPoint := func(angle float64) *geo.Point {
		return geo.NewPoint(arcRadius*math.Cos(angle), arcRadius*math.Sin(angle))
	}
	crosses := func(a, b float64) bool {
		seg := *geo.NewSegment(arcPoint(a), arcPoint(b))
		for _, side := range perimeter {
			if len(side.Intersections(seg)) > 0 {
				return true
			}
		}
		return false
	}

	// walk chords from the center outward until one crosses the border
	step := (toAngle - fromAngle) / BORDER_SEARCH_STEPS
	var lo, hi float64
	found := false
	for i := 0; i < BORDER_SEARCH_STEPS; i++ {
		a := fromAngle + float64(i)*step
		b := a + step
		if crosses(a, b) {
			lo, hi = a, b
			found = true
			break
		}
	}
	if !found {
		return 0, false
	}

	// bisect [lo, hi] down to the exact crossing angle
	// invariant: the crossing stays inside [lo, hi]
	for i := 0; i < BORDER_REFINE_STEPS; i++ {
		mid := (lo + hi) / 2
		if crosses(lo, mid) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return (lo + hi) / 2, true
}

// arcToBeziers converts the circular arc between startAngle and endAngle
// (on the circle of the given radius centered at the origin) into a route of
// cubic Bézier segments: [P0, C1, C2, P1, C1, C2, P2, ...].
func arcToBeziers(radius, startAngle, endAngle float64) []*geo.Point {
	span := endAngle - startAngle
	// one Bézier segment per quarter turn keeps the fit accurate
	numSegments := int(math.Ceil(span / (math.Pi / 2)))
	if numSegments < 1 {
		numSegments = 1
	}
	segmentSpan := span / float64(numSegments)
	// standard circular-arc Bézier approximation constant
	k := 4.0 / 3.0 * math.Tan(segmentSpan/4)

	arcPoint := func(angle float64) *geo.Point {
		return geo.NewPoint(radius*math.Cos(angle), radius*math.Sin(angle))
	}
	// unit tangent in the direction of increasing angle
	tangent := func(angle float64) *geo.Point {
		return geo.NewPoint(-math.Sin(angle), math.Cos(angle))
	}

	route := make([]*geo.Point, 0, 1+3*numSegments)
	route = append(route, arcPoint(startAngle))
	for i := 0; i < numSegments; i++ {
		a0 := startAngle + float64(i)*segmentSpan
		a1 := a0 + segmentSpan
		p0, p1 := arcPoint(a0), arcPoint(a1)
		t0, t1 := tangent(a0), tangent(a1)
		route = append(route,
			geo.NewPoint(p0.X+k*radius*t0.X, p0.Y+k*radius*t0.Y),
			geo.NewPoint(p1.X-k*radius*t1.X, p1.Y-k*radius*t1.Y),
			p1,
		)
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
