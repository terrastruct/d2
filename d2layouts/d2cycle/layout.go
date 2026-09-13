// Package d2cycle implements the layout for objects of shape cycle.
//
// Children of a cycle object are arranged with their centers on a circle and
// edges between them are routed along that circle. Arcs are exact circular
// curves (chains of cubics using the canonical k = 4/3·tan(Δθ/4) control
// offset) whose endpoints are trimmed to each endpoint shape's visible border.
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
	// minRadius is the smallest radius a cycle is drawn with.
	minRadius = 200.
	// padding is the minimum gap between adjacent children along the circle.
	padding = 24.
	// maxArcSweep bounds each cubic segment so the Bezier approximation of a
	// circular arc stays visually exact.
	maxArcSweep = math.Pi / 2
	// chordStep is the angular resolution used when intersecting the cycle
	// circle with a child shape's perimeter. The sagitta of a 1° chord is
	// ~R·2.4e-5, far below a pixel.
	chordStep = math.Pi / 180
	epsilon   = 1e-6
)

// Layout arranges the direct children of the graph's root on a circle, keeps
// the internal layout of every child intact, and routes root-level edges as
// circular arcs trimmed to the endpoints' borders.
func Layout(ctx context.Context, g *d2graph.Graph, coreLayout d2graph.LayoutGraph) error {
	objects := g.Root.ChildrenArray
	if len(objects) == 0 {
		return nil
	}

	// Lay out children with the core engine first so nested content inside
	// each child is fully positioned and routed before the child is moved
	// onto the circle. The core layout may replace object pointers, so the
	// children are re-read afterwards.
	if coreLayout != nil {
		if err := coreLayout(ctx, g); err != nil {
			return err
		}
	}

	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	radius := calculateRadius(g.Root.ChildrenArray)
	if radius > 0 {
		deltas := positionObjects(g.Root.ChildrenArray, radius)
		applyDeltas(g, deltas)
		directions := arcDirections(g)
		for _, edge := range g.Edges {
			if isCycleEdge(g, edge) {
				routeCircularArc(edge, radius, directions[edge])
			}
		}
	}

	normalizeGraph(g)
	return nil
}

// calculateRadius returns the circle radius that fits all children with
// padding between neighbors, or 0 when there is nothing to place on a ring.
func calculateRadius(objects []*d2graph.Object) float64 {
	if len(objects) < 2 {
		return 0
	}

	maxHalfDiagonal := 0.
	for _, obj := range objects {
		maxHalfDiagonal = math.Max(maxHalfDiagonal, math.Hypot(obj.Width/2, obj.Height/2))
	}

	minimum := (maxHalfDiagonal + padding) / math.Sin(math.Pi/float64(len(objects)))
	return math.Max(minimum, minRadius)
}

// positionObjects centers each child on the circle, the first at 12 o'clock
// and the rest clockwise, and returns the translation applied to each child.
func positionObjects(objects []*d2graph.Object, radius float64) map[*d2graph.Object]*geo.Point {
	deltas := make(map[*d2graph.Object]*geo.Point, len(objects))
	for i, obj := range objects {
		angle := -math.Pi/2 + 2*math.Pi*float64(i)/float64(len(objects))
		x := radius * math.Cos(angle)
		y := radius * math.Sin(angle)

		var dx, dy float64
		if obj.TopLeft != nil {
			dx = x - obj.Width/2 - obj.TopLeft.X
			dy = y - obj.Height/2 - obj.TopLeft.Y
		}
		deltas[obj] = geo.NewPoint(dx, dy)
	}
	return deltas
}

// applyDeltas moves every child together with its descendants and keeps all
// edge routes attached to the moved endpoints.
func applyDeltas(g *d2graph.Graph, deltas map[*d2graph.Object]*geo.Point) {
	if len(deltas) == 0 {
		return
	}

	for obj, delta := range deltas {
		obj.MoveWithDescendants(delta.X, delta.Y)
	}

	for _, edge := range g.Edges {
		if len(edge.Route) == 0 || edge.Src == nil || edge.Dst == nil {
			continue
		}
		srcDelta := ancestorDelta(edge.Src, deltas)
		dstDelta := ancestorDelta(edge.Dst, deltas)
		if srcDelta == nil && dstDelta == nil {
			continue
		}
		if srcDelta != nil && dstDelta != nil && sameDelta(srcDelta, dstDelta) {
			// Both endpoints belong to the same child: preserve the core
			// layout's route shape by translating it entirely.
			edge.Move(srcDelta.X, srcDelta.Y)
			continue
		}
		if srcDelta != nil {
			edge.Route[0].X += srcDelta.X
			edge.Route[0].Y += srcDelta.Y
		}
		if dstDelta != nil {
			last := edge.Route[len(edge.Route)-1]
			last.X += dstDelta.X
			last.Y += dstDelta.Y
		}
	}
}

// ancestorDelta returns the delta of the root child that obj descends from.
func ancestorDelta(obj *d2graph.Object, deltas map[*d2graph.Object]*geo.Point) *geo.Point {
	for curr := obj; curr != nil; curr = curr.Parent {
		if delta, ok := deltas[curr]; ok {
			return delta
		}
	}
	return nil
}

func sameDelta(a, b *geo.Point) bool {
	return a == b || (a.X == b.X && a.Y == b.Y)
}

// isCycleEdge reports whether both endpoints are direct children of the cycle
// root, i.e. the edge should follow the circle.
func isCycleEdge(g *d2graph.Graph, edge *d2graph.Edge) bool {
	if edge.Src == nil || edge.Dst == nil {
		return false
	}
	return edge.Src.Parent == g.Root && edge.Dst.Parent == g.Root
}

// arcDirections decides the travel direction of every cycle edge. With only
// two children on the ring, edges between the same pair in opposite
// directions take opposite semicircles so their arcs do not overlap. On
// larger rings the opposite arc would cross other children, so every edge
// takes the shorter way around.
func arcDirections(g *d2graph.Graph) map[*d2graph.Edge]float64 {
	ringSize := len(g.Root.ChildrenArray)
	type pairKey struct{ a, b string }
	pairs := make(map[pairKey][]*d2graph.Edge)
	order := make([]pairKey, 0, len(g.Edges))

	for _, edge := range g.Edges {
		if !isCycleEdge(g, edge) {
			continue
		}
		a, b := edge.Src.AbsID(), edge.Dst.AbsID()
		if a > b {
			a, b = b, a
		}
		key := pairKey{a, b}
		if _, ok := pairs[key]; !ok {
			order = append(order, key)
		}
		pairs[key] = append(pairs[key], edge)
	}

	directions := make(map[*d2graph.Edge]float64, len(g.Edges))
	for _, key := range order {
		edges := pairs[key]
		opposite := ringSize == 2 && len(edges) > 1 &&
			edges[0].Src.AbsID() != edges[len(edges)-1].Src.AbsID()
		for i, edge := range edges {
			srcAngle := math.Atan2(edge.Src.Center().Y, edge.Src.Center().X)
			dstAngle := math.Atan2(edge.Dst.Center().Y, edge.Dst.Center().X)
			cw := modulo2Pi(dstAngle - srcAngle)
			cclw := modulo2Pi(srcAngle - dstAngle)
			if edge.Src == edge.Dst {
				directions[edge] = -1
			} else if opposite && i > 0 {
				// Same angular travel direction from the reversed endpoints
				// traces the complementary arc, so the two edges do not
				// overlap.
				directions[edge] = directions[edges[0]]
			} else if cw <= cclw {
				directions[edge] = 1
			} else {
				directions[edge] = -1
			}
		}
	}
	return directions
}

// routeCircularArc replaces the edge's route with an exact circular arc along
// the ring, trimmed to the visible borders of both endpoints. dir is +1 for
// clockwise travel, -1 for counter-clockwise.
func routeCircularArc(edge *d2graph.Edge, radius, dir float64) {
	if edge.Src == nil || edge.Dst == nil || len(edge.Route) == 0 {
		return
	}

	center := geo.NewPoint(0, 0)
	srcAngle := math.Atan2(edge.Src.Center().Y, edge.Src.Center().X)
	dstAngle := math.Atan2(edge.Dst.Center().Y, edge.Dst.Center().X)

	exit := borderCrossingFor(edge.Src, center, radius, srcAngle, dir)
	entry := borderCrossingFor(edge.Dst, center, radius, dstAngle, -dir)
	if exit == nil || entry == nil {
		routeStraight(edge)
		return
	}

	// Total swept angle from exit to entry travelling in direction dir.
	sweep := modulo2Pi(entry.angle - exit.angle)
	if dir < 0 {
		sweep -= 2 * math.Pi
	}
	if math.Abs(sweep) < epsilon {
		routeStraight(edge)
		return
	}

	start := exit.point
	if start == nil {
		start = pointOnCircle(center, radius, exit.angle)
	}
	end := entry.point
	if end == nil {
		end = pointOnCircle(center, radius, entry.angle)
	}

	edge.Route = buildArcRoute(center, radius, exit.angle, exit.angle+sweep)
	edge.Route[0] = start
	edge.Route[len(edge.Route)-1] = end
	edge.IsCurve = true
}

func routeStraight(edge *d2graph.Edge) {
	if edge.Src == nil || edge.Dst == nil {
		return
	}
	edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
	edge.TraceToShape(edge.Route, 0, 1)
	edge.IsCurve = false
}

type borderCrossing struct {
	angle float64
	// point is set when the crossing lies on the shape's visible perimeter;
	// otherwise the point on the circle at angle is used.
	point *geo.Point
}

// borderCrossingFor returns where the circle leaves obj's border when
// travelling from the object's center angle in the given direction (searchDir
// is the direction to search in: +dir for an exit, -dir for an entry).
func borderCrossingFor(obj *d2graph.Object, center *geo.Point, radius, objAngle, searchDir float64) *borderCrossing {
	if p := perimeterCrossing(obj, center, radius, objAngle, searchDir); p != nil {
		return p
	}

	// Fall back to the box border: bisect between the object's center angle
	// (inside) and a quarter turn away (outside), then project onto the
	// visible border for non-rectangular shapes.
	inside := objAngle
	outside := objAngle + searchDir*math.Pi/2
	for i := 0; i < 48; i++ {
		mid := (inside + outside) / 2
		if obj.Box.Contains(pointOnCircle(center, radius, mid)) {
			inside = mid
		} else {
			outside = mid
		}
	}
	if math.Abs(outside-objAngle) < epsilon {
		return nil
	}
	p := pointOnCircle(center, radius, outside)
	if s := obj.ToShape(); s != nil && !s.IsRectangular() {
		p = shape.TraceToShapeBorder(s, p, pointOnCircle(center, radius, outside-searchDir*0.001))
	}
	return &borderCrossing{angle: math.Atan2(p.Y-center.Y, p.X-center.X), point: p}
}

// perimeterCrossing intersects the circle with the shape's visible perimeter
// by marching a quarter turn from the object's center angle with chords. It
// returns the crossing closest to the center angle: the border exit when
// marching forward, the border entry when marching backward. nil when the
// shape has no perimeter geometry or no crossing was found.
func perimeterCrossing(obj *d2graph.Object, center *geo.Point, radius, objAngle, searchDir float64) *borderCrossing {
	s := obj.ToShape()
	if s == nil || s.IsRectangular() {
		return nil
	}
	perimeter := s.Perimeter()
	if len(perimeter) == 0 {
		return nil
	}

	from := objAngle
	to := objAngle + searchDir*math.Pi/2
	steps := int(math.Ceil(math.Abs(to-from) / chordStep))
	if steps < 8 {
		steps = 8
	}

	var best *borderCrossing
	bestDist := math.Inf(1)
	for i := 0; i < steps; i++ {
		a0 := from + (to-from)*float64(i)/float64(steps)
		a1 := from + (to-from)*float64(i+1)/float64(steps)
		chord := geo.Segment{
			Start: pointOnCircle(center, radius, a0),
			End:   pointOnCircle(center, radius, a1),
		}
		for _, elem := range perimeter {
			for _, p := range elem.Intersections(chord) {
				angle := math.Atan2(p.Y-center.Y, p.X-center.X)
				dist := math.Abs(moduloSigned(angle - objAngle))
				if dist < bestDist {
					bestDist = dist
					best = &borderCrossing{angle: angle, point: p}
				}
			}
		}
	}
	return best
}

// buildArcRoute returns a route of cubic Bezier control points tracing the
// circle from startAngle to endAngle, split into segments of at most
// maxArcSweep. Every chain endpoint lies exactly on the circle.
func buildArcRoute(center *geo.Point, radius, startAngle, endAngle float64) []*geo.Point {
	sweep := endAngle - startAngle
	segments := int(math.Ceil(math.Abs(sweep) / maxArcSweep))
	if segments < 1 {
		segments = 1
	}

	route := make([]*geo.Point, 1, 3*segments+1)
	route[0] = pointOnCircle(center, radius, startAngle)
	for i := 0; i < segments; i++ {
		a0 := startAngle + sweep*float64(i)/float64(segments)
		a1 := startAngle + sweep*float64(i+1)/float64(segments)
		p0 := pointOnCircle(center, radius, a0)
		p3 := pointOnCircle(center, radius, a1)
		// Canonical circular arc to cubic Bezier control point offset.
		k := 4. / 3. * math.Tan((a1-a0)/4.)

		c1 := geo.NewPoint(
			p0.X-k*radius*math.Sin(a0),
			p0.Y+k*radius*math.Cos(a0),
		)
		c2 := geo.NewPoint(
			p3.X+k*radius*math.Sin(a1),
			p3.Y-k*radius*math.Cos(a1),
		)

		route = append(route, c1, c2, p3)
	}
	return route
}

func pointOnCircle(center *geo.Point, radius, angle float64) *geo.Point {
	return geo.NewPoint(center.X+radius*math.Cos(angle), center.Y+radius*math.Sin(angle))
}

// normalizeGraph shifts all objects and routes so the bounding box starts at
// (0, 0) and records the bounding box on the root.
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
	if dx != 0 || dy != 0 {
		for _, obj := range g.Objects {
			if obj.TopLeft != nil {
				obj.TopLeft.X += dx
				obj.TopLeft.Y += dy
			}
		}
		for _, edge := range g.Edges {
			edge.Move(dx, dy)
		}
	}
	g.Root.Box = geo.NewBox(geo.NewPoint(0, 0), br.X-tl.X, br.Y-tl.Y)
}

func modulo2Pi(angle float64) float64 {
	angle = math.Mod(angle, 2*math.Pi)
	if angle < 0 {
		angle += 2 * math.Pi
	}
	return angle
}

// moduloSigned reduces an angle to [0, 2π) keeping the magnitude of signed
// offsets small enough for nearest-crossing comparisons.
func moduloSigned(angle float64) float64 {
	angle = math.Mod(angle, 2*math.Pi)
	if angle < 0 {
		angle += 2 * math.Pi
	}
	return angle
}

// positionLabelsIcons matches the default label and icon placement used by
// the core layouts.
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
		if float64(obj.LabelDimensions.Width) > obj.Width || float64(obj.LabelDimensions.Height) > obj.Height {
			if len(obj.ChildrenArray) > 0 {
				obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
			} else {
				obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
			}
		}
	}
}
