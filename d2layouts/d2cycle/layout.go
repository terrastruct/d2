package d2cycle

import (
	"context"
	"math"
	"strconv"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
	"oss.terrastruct.com/util-go/go2"
)

const (
	minRadius             = 200
	padding               = 20
	intersectionTolerance = 0.001
	parameterTolerance    = 1e-7
)

type moveDelta struct {
	x float64
	y float64
}

type edgeKey struct {
	src *d2graph.Object
	dst *d2graph.Object
}

type cycleChain struct {
	edges       []*d2graph.Edge
	sourceOrder int
}

func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) error {
	objects := orderedCycleObjects(g.Root.ChildrenArray, g.Edges)
	if len(objects) == 0 {
		return nil
	}

	if layout != nil {
		if err := layout(ctx, g); err != nil {
			return err
		}
	}

	positionLabelsIcons(g.Root)
	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	moved := positionObjects(objects, calculateRadius(objects))
	updateEdgeRoutes(g, cycleEdgeSet(g.Root.ChildrenArray, g.Edges), moved)
	if g.RootLevel > 0 {
		fitRootToCycle(g)
	}
	return nil
}

func orderedCycleObjects(objects []*d2graph.Object, edges []*d2graph.Edge) []*d2graph.Object {
	cycleEdges := selectedCycleEdges(objects, edges)
	if len(cycleEdges) == 0 {
		return objects
	}
	return orderObjectsByEdges(objects, cycleEdges)
}

func orderObjectsByEdges(objects []*d2graph.Object, edges []*d2graph.Edge) []*d2graph.Object {
	var ordered []*d2graph.Object
	inOrder := make(map[*d2graph.Object]struct{}, len(objects))

	add := func(obj *d2graph.Object) {
		if _, ok := inOrder[obj]; ok {
			return
		}
		ordered = append(ordered, obj)
		inOrder[obj] = struct{}{}
	}

	for _, edge := range edges {
		add(edge.Src)
		add(edge.Dst)
	}
	for _, obj := range objects {
		if _, ok := inOrder[obj]; !ok {
			ordered = append(ordered, obj)
		}
	}
	return ordered
}

func cycleEdgeSet(objects []*d2graph.Object, edges []*d2graph.Edge) map[*d2graph.Edge]struct{} {
	cycleEdges := make(map[*d2graph.Edge]struct{})
	for _, edge := range selectedCycleEdges(objects, edges) {
		cycleEdges[edge] = struct{}{}
	}
	return cycleEdges
}

func selectedCycleEdges(objects []*d2graph.Object, edges []*d2graph.Edge) []*d2graph.Edge {
	chain := bestSourceChain(objects, edges)
	if chain == nil {
		return singleSourceEdges(objects, edges)
	}
	if chain.closed() {
		return chain.edges
	}
	selected := append([]*d2graph.Edge(nil), chain.edges...)
	seen := make(map[edgeKey]struct{}, len(chain.edges))
	for _, edge := range chain.edges {
		seen[edgeKey{src: edge.Src, dst: edge.Dst}] = struct{}{}
	}
	for _, edge := range adjacentRootEdges(orderObjectsByEdges(objects, chain.edges), edges, nil) {
		key := edgeKey{src: edge.Src, dst: edge.Dst}
		if _, ok := seen[key]; ok {
			continue
		}
		selected = append(selected, edge)
		seen[key] = struct{}{}
	}
	return selected
}

func bestSourceChain(objects []*d2graph.Object, edges []*d2graph.Edge) *cycleChain {
	var best *cycleChain
	for _, chain := range sourceChains(objects, edges) {
		if !chain.complete() {
			continue
		}
		if best == nil || betterChain(chain, best) {
			best = chain
		}
	}
	return best
}

func sourceChains(objects []*d2graph.Object, edges []*d2graph.Edge) []*cycleChain {
	rootObjects := rootObjectSet(objects)
	byKey := make(map[*d2ast.Key]*cycleChain)
	var chains []*cycleChain

	for sourceOrder, edge := range edges {
		if edge.Src == edge.Dst || !isRootEdge(edge, rootObjects) {
			continue
		}
		for _, ref := range edge.References {
			if ref.MapKey == nil || len(ref.MapKey.Edges) <= 1 ||
				ref.MapKeyEdgeIndex < 0 || ref.MapKeyEdgeIndex >= len(ref.MapKey.Edges) {
				continue
			}
			chain := byKey[ref.MapKey]
			if chain == nil {
				chain = &cycleChain{
					edges:       make([]*d2graph.Edge, len(ref.MapKey.Edges)),
					sourceOrder: sourceOrder,
				}
				byKey[ref.MapKey] = chain
				chains = append(chains, chain)
			}
			if chain.edges[ref.MapKeyEdgeIndex] == nil {
				chain.edges[ref.MapKeyEdgeIndex] = edge
			}
		}
	}
	return chains
}

func (chain *cycleChain) complete() bool {
	for _, edge := range chain.edges {
		if edge == nil {
			return false
		}
	}
	return len(chain.edges) > 0
}

func betterChain(candidate, best *cycleChain) bool {
	candidateClosed := candidate.closed()
	bestClosed := best.closed()
	if candidateClosed != bestClosed {
		return candidateClosed
	}
	if len(candidate.edges) != len(best.edges) {
		return len(candidate.edges) > len(best.edges)
	}
	return candidate.sourceOrder < best.sourceOrder
}

func (chain *cycleChain) closed() bool {
	if len(chain.edges) < 2 {
		return false
	}
	return chain.edges[len(chain.edges)-1].Dst == chain.edges[0].Src
}

func singleSourceEdges(objects []*d2graph.Object, edges []*d2graph.Edge) []*d2graph.Edge {
	return adjacentRootEdges(objects, edges, func(edge *d2graph.Edge) bool {
		return edgeStatementLen(edge) == 1
	})
}

func adjacentRootEdges(objects []*d2graph.Object, edges []*d2graph.Edge, keep func(*d2graph.Edge) bool) []*d2graph.Edge {
	rootObjects := rootObjectSet(objects)
	successors := cycleSuccessors(objects)
	seen := make(map[edgeKey]struct{})
	var selected []*d2graph.Edge
	for _, edge := range edges {
		if edge.Src == edge.Dst || !isRootEdge(edge, rootObjects) {
			continue
		}
		if successors[edge.Src] != edge.Dst {
			continue
		}
		if keep != nil && !keep(edge) {
			continue
		}
		key := edgeKey{src: edge.Src, dst: edge.Dst}
		if _, ok := seen[key]; ok {
			continue
		}
		selected = append(selected, edge)
		seen[key] = struct{}{}
	}
	return selected
}

func cycleSuccessors(objects []*d2graph.Object) map[*d2graph.Object]*d2graph.Object {
	successors := make(map[*d2graph.Object]*d2graph.Object, len(objects))
	if len(objects) < 2 {
		return successors
	}
	for i, obj := range objects {
		successors[obj] = objects[(i+1)%len(objects)]
	}
	return successors
}

func rootObjectSet(objects []*d2graph.Object) map[*d2graph.Object]struct{} {
	rootObjects := make(map[*d2graph.Object]struct{}, len(objects))
	for _, obj := range objects {
		rootObjects[obj] = struct{}{}
	}
	return rootObjects
}

func isRootEdge(edge *d2graph.Edge, rootObjects map[*d2graph.Object]struct{}) bool {
	_, srcOK := rootObjects[edge.Src]
	_, dstOK := rootObjects[edge.Dst]
	return srcOK && dstOK
}

func edgeStatementLen(edge *d2graph.Edge) int {
	maxLen := 0
	for _, ref := range edge.References {
		if ref.MapKey != nil && len(ref.MapKey.Edges) > maxLen {
			maxLen = len(ref.MapKey.Edges)
		}
	}
	return maxLen
}

func calculateRadius(objects []*d2graph.Object) float64 {
	if len(objects) == 1 {
		return 0
	}

	maxSize := 0.0
	for _, obj := range objects {
		maxSize = math.Max(maxSize, objectVisualRadius(obj))
	}

	numObjects := float64(len(objects))
	return math.Max((maxSize+padding)/math.Sin(math.Pi/numObjects), minRadius)
}

func objectVisualRadius(obj *d2graph.Object) float64 {
	center := obj.Center()
	maxDistance := 0.0
	addBox := func(topLeft *geo.Point, width, height float64) {
		if topLeft == nil {
			return
		}
		for _, p := range []*geo.Point{
			topLeft,
			geo.NewPoint(topLeft.X+width, topLeft.Y),
			geo.NewPoint(topLeft.X, topLeft.Y+height),
			geo.NewPoint(topLeft.X+width, topLeft.Y+height),
		} {
			maxDistance = math.Max(maxDistance, geo.EuclideanDistance(center.X, center.Y, p.X, p.Y))
		}
	}

	addBox(obj.TopLeft, obj.Width, obj.Height)
	if obj.HasLabel() {
		addBox(obj.GetLabelTopLeft(), float64(obj.LabelDimensions.Width), float64(obj.LabelDimensions.Height))
	}
	if iconTL, iconSize := objectIconBox(obj); iconTL != nil {
		addBox(iconTL, iconSize, iconSize)
	}
	return maxDistance
}

func positionObjects(objects []*d2graph.Object, radius float64) map[*d2graph.Object]moveDelta {
	moved := make(map[*d2graph.Object]moveDelta)
	numObjects := float64(len(objects))
	angleOffset := -math.Pi / 2

	for i, obj := range objects {
		angle := angleOffset + 2*math.Pi*float64(i)/numObjects
		x := radius*math.Cos(angle) - obj.Box.Width/2
		y := radius*math.Sin(angle) - obj.Box.Height/2
		delta := moveDelta{x: x - obj.TopLeft.X, y: y - obj.TopLeft.Y}

		recordMove(moved, obj, delta)
		obj.MoveWithDescendants(delta.x, delta.y)
	}
	return moved
}

func recordMove(moved map[*d2graph.Object]moveDelta, obj *d2graph.Object, delta moveDelta) {
	moved[obj] = delta
	obj.IterDescendants(func(_, child *d2graph.Object) {
		moved[child] = delta
	})
}

func updateEdgeRoutes(g *d2graph.Graph, cycleEdges map[*d2graph.Edge]struct{}, moved map[*d2graph.Object]moveDelta) {
	for _, edge := range g.Edges {
		if isCycleEdge(g, edge, cycleEdges) {
			createCircularArc(edge)
			continue
		}

		srcDelta, srcMoved := moved[edge.Src]
		dstDelta, dstMoved := moved[edge.Dst]
		switch {
		case len(edge.Route) == 0:
			routeStraight(edge)
		case srcMoved && dstMoved && srcDelta == dstDelta:
			edge.Move(srcDelta.x, srcDelta.y)
		case srcMoved || dstMoved:
			routeStraight(edge)
		}
	}
}

func isCycleEdge(g *d2graph.Graph, edge *d2graph.Edge, cycleEdges map[*d2graph.Edge]struct{}) bool {
	if edge.Src == nil || edge.Dst == nil ||
		edge.Src == edge.Dst ||
		edge.Src.Parent != g.Root ||
		edge.Dst.Parent != g.Root {
		return false
	}
	_, ok := cycleEdges[edge]
	return ok
}

func createCircularArc(edge *d2graph.Edge) {
	srcCenter := edge.Src.Center()
	dstCenter := edge.Dst.Center()
	srcAngle := math.Atan2(srcCenter.Y, srcCenter.X)
	dstAngle := math.Atan2(dstCenter.Y, dstCenter.X)
	if dstAngle < srcAngle {
		dstAngle += 2 * math.Pi
	}

	radius := math.Hypot(srcCenter.X, srcCenter.Y)
	if radius == 0 {
		routeStraight(edge)
		return
	}

	startAngle := trimStartAngle(edge.Src.ToShape(), radius, srcAngle, dstAngle)
	endAngle := trimEndAngle(edge.Dst.ToShape(), radius, startAngle, dstAngle)
	if endAngle <= startAngle {
		routeStraight(edge)
		return
	}

	edge.Route = cubicArcRoute(radius, startAngle, endAngle)
	edge.IsCurve = true
	if edge.Label.Value != "" && edge.LabelPosition == nil {
		edge.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
	}
}

func routeStraight(edge *d2graph.Edge) {
	if edge.Src == edge.Dst {
		routeSelfLoop(edge)
		return
	}
	edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
	edge.TraceToShape(edge.Route, 0, 1)
	edge.IsCurve = false
	if edge.Label.Value != "" {
		edge.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
	}
}

func routeSelfLoop(edge *d2graph.Edge) {
	center := edge.Src.Center()
	box := edge.Src.Box
	gap := math.Max(math.Max(box.Width, box.Height)/2, padding)
	right := box.TopLeft.X + box.Width + gap
	top := box.TopLeft.Y - gap
	edge.Route = []*geo.Point{
		center,
		geo.NewPoint(right, center.Y),
		geo.NewPoint(right, top),
		geo.NewPoint(center.X, top),
		center.Copy(),
	}
	edge.TraceToShape(edge.Route, 0, len(edge.Route)-1)
	edge.IsCurve = false
	if edge.Label.Value != "" {
		edge.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
	}
}

func trimStartAngle(s shape.Shape, radius, startAngle, endAngle float64) float64 {
	if angle, ok := cycleShapeBorderAngle(s, radius, startAngle, endAngle, true); ok {
		return angle
	}

	box := s.GetBox()
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

func trimEndAngle(s shape.Shape, radius, startAngle, endAngle float64) float64 {
	if angle, ok := cycleShapeBorderAngle(s, radius, startAngle, endAngle, false); ok {
		return angle
	}

	box := s.GetBox()
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

func cycleShapeBorderAngle(s shape.Shape, radius, startAngle, endAngle float64, pickStart bool) (float64, bool) {
	var intersections []*geo.Point
	if s.GetType() == shape.CIRCLE_TYPE {
		intersections = cycleCircleIntersections(radius, s.GetBox())
	} else if s.IsRectangular() {
		intersections = cycleSegmentIntersections(radius, boxSegments(s.GetBox()))
	} else {
		intersections = cyclePerimeterIntersections(radius, s.Perimeter())
		if len(intersections) == 0 {
			return cycleEllipseBorderAngle(s.Perimeter(), radius, startAngle, endAngle, pickStart)
		}
	}

	best := 0.0
	found := false
	for _, p := range intersections {
		angle := normalizeAngleInRange(math.Atan2(p.Y, p.X), startAngle, endAngle)
		if angle < startAngle || angle > endAngle {
			continue
		}
		if !found || (pickStart && angle < best) || (!pickStart && angle > best) {
			best = angle
			found = true
		}
	}
	return best, found
}

func cycleEllipseBorderAngle(perimeter []geo.Intersectable, radius, startAngle, endAngle float64, pickStart bool) (float64, bool) {
	if len(perimeter) != 1 {
		return 0, false
	}
	ellipse, ok := perimeter[0].(*geo.Ellipse)
	if !ok {
		return 0, false
	}

	if pickStart {
		if !ellipseContains(ellipse, pointOnCircle(radius, startAngle)) ||
			ellipseContains(ellipse, pointOnCircle(radius, endAngle)) {
			return 0, false
		}
	} else {
		if ellipseContains(ellipse, pointOnCircle(radius, startAngle)) ||
			!ellipseContains(ellipse, pointOnCircle(radius, endAngle)) {
			return 0, false
		}
	}

	low, high := startAngle, endAngle
	for i := 0; i < 64; i++ {
		mid := (low + high) / 2
		inside := ellipseContains(ellipse, pointOnCircle(radius, mid))
		if pickStart {
			if inside {
				low = mid
			} else {
				high = mid
			}
		} else {
			if inside {
				high = mid
			} else {
				low = mid
			}
		}
	}
	return high, true
}

func ellipseContains(ellipse *geo.Ellipse, p *geo.Point) bool {
	if ellipse.Rx <= 0 || ellipse.Ry <= 0 {
		return false
	}
	dx := (p.X - ellipse.Center.X) / ellipse.Rx
	dy := (p.Y - ellipse.Center.Y) / ellipse.Ry
	return dx*dx+dy*dy <= 1
}

func cycleCircleIntersections(radius float64, box *geo.Box) []*geo.Point {
	return circleCircleIntersections(radius, box.Center(), box.Width/2)
}

func circleCircleIntersections(radius float64, center *geo.Point, shapeRadius float64) []*geo.Point {
	d := math.Hypot(center.X, center.Y)
	if d == 0 || shapeRadius <= 0 {
		return nil
	}

	a := (radius*radius - shapeRadius*shapeRadius + d*d) / (2 * d)
	h2 := radius*radius - a*a
	if h2 < 0 {
		if h2 > -intersectionTolerance {
			h2 = 0
		} else {
			return nil
		}
	}

	h := math.Sqrt(h2)
	x2 := a * center.X / d
	y2 := a * center.Y / d
	rx := -center.Y * h / d
	ry := center.X * h / d
	return []*geo.Point{
		geo.NewPoint(x2+rx, y2+ry),
		geo.NewPoint(x2-rx, y2-ry),
	}
}

func cyclePerimeterIntersections(radius float64, perimeter []geo.Intersectable) []*geo.Point {
	var intersections []*geo.Point
	for _, side := range perimeter {
		switch side := side.(type) {
		case *geo.Segment:
			intersections = append(intersections, circleSegmentIntersections(radius, side)...)
		case geo.Segment:
			intersections = append(intersections, circleSegmentIntersections(radius, &side)...)
		case *geo.BezierCurve:
			intersections = append(intersections, circleBezierIntersections(radius, side)...)
		case geo.BezierCurve:
			intersections = append(intersections, circleBezierIntersections(radius, &side)...)
		case *geo.Ellipse:
			intersections = append(intersections, cycleEllipseIntersections(radius, side)...)
		case geo.Ellipse:
			intersections = append(intersections, cycleEllipseIntersections(radius, &side)...)
		}
	}
	return intersections
}

func cycleEllipseIntersections(radius float64, ellipse *geo.Ellipse) []*geo.Point {
	if math.Abs(ellipse.Rx-ellipse.Ry) <= intersectionTolerance {
		return circleCircleIntersections(radius, ellipse.Center, ellipse.Rx)
	}
	return nil
}

func circleBezierIntersections(radius float64, curve *geo.BezierCurve) []*geo.Point {
	var intersections []*geo.Point
	// Solve the circle/curve intersection directly so tangencies are stable.
	// Sampling can miss the exact point where a curved edge meets the cycle.
	for _, t := range polynomialRootsInUnit(bezierCirclePolynomial(radius, curve.Points())) {
		if math.Abs(circleOffset(radius, curve.At(t))) <= intersectionTolerance {
			intersections = appendUniquePoint(intersections, curve.At(t))
		}
	}
	return intersections
}

func circleOffset(radius float64, p *geo.Point) float64 {
	return math.Hypot(p.X, p.Y) - radius
}

func bezierCirclePolynomial(radius float64, points []*geo.Point) []float64 {
	if len(points) != 4 {
		return nil
	}
	x := cubicPowerCoefficients(points[0].X, points[1].X, points[2].X, points[3].X)
	y := cubicPowerCoefficients(points[0].Y, points[1].Y, points[2].Y, points[3].Y)
	coeffs := polynomialAdd(polynomialMultiply(x, x), polynomialMultiply(y, y))
	coeffs[0] -= radius * radius
	return coeffs
}

func cubicPowerCoefficients(p0, p1, p2, p3 float64) []float64 {
	return []float64{
		p0,
		-3*p0 + 3*p1,
		3*p0 - 6*p1 + 3*p2,
		-p0 + 3*p1 - 3*p2 + p3,
	}
}

func polynomialRootsInUnit(coeffs []float64) []float64 {
	coeffs = trimPolynomial(coeffs)
	degree := len(coeffs) - 1
	if degree <= 0 {
		return nil
	}
	if degree == 1 {
		if coeffs[1] == 0 {
			return nil
		}
		root := -coeffs[0] / coeffs[1]
		if root >= -intersectionTolerance && root <= 1+intersectionTolerance {
			return []float64{clampUnit(root)}
		}
		return nil
	}

	valueTolerance := polynomialValueTolerance(coeffs)
	breaks := []float64{0}
	for _, root := range polynomialRootsInUnit(polynomialDerivative(coeffs)) {
		if root > parameterTolerance && root < 1-parameterTolerance {
			breaks = appendUniqueFloat(breaks, root)
		}
	}
	breaks = appendUniqueFloat(breaks, 1)

	var roots []float64
	for _, t := range breaks {
		if math.Abs(polynomialEval(coeffs, t)) <= valueTolerance {
			roots = appendUniqueFloat(roots, clampUnit(t))
		}
	}
	for i := 0; i+1 < len(breaks); i++ {
		low, high := breaks[i], breaks[i+1]
		lowValue := polynomialEval(coeffs, low)
		highValue := polynomialEval(coeffs, high)
		if lowValue*highValue >= 0 {
			continue
		}
		roots = appendUniqueFloat(roots, bisectPolynomialRoot(coeffs, low, high, lowValue))
	}
	return roots
}

func bisectPolynomialRoot(coeffs []float64, low, high, lowValue float64) float64 {
	for i := 0; i < 64; i++ {
		mid := (low + high) / 2
		midValue := polynomialEval(coeffs, mid)
		if math.Abs(midValue) <= polynomialValueTolerance(coeffs) {
			return mid
		}
		if lowValue*midValue > 0 {
			low = mid
			lowValue = midValue
		} else {
			high = mid
		}
	}
	return (low + high) / 2
}

func polynomialDerivative(coeffs []float64) []float64 {
	if len(coeffs) <= 1 {
		return nil
	}
	derivative := make([]float64, len(coeffs)-1)
	for i := 1; i < len(coeffs); i++ {
		derivative[i-1] = coeffs[i] * float64(i)
	}
	return derivative
}

func polynomialMultiply(a, b []float64) []float64 {
	product := make([]float64, len(a)+len(b)-1)
	for i := range a {
		for j := range b {
			product[i+j] += a[i] * b[j]
		}
	}
	return product
}

func polynomialAdd(a, b []float64) []float64 {
	sum := make([]float64, max(len(a), len(b)))
	for i := range a {
		sum[i] += a[i]
	}
	for i := range b {
		sum[i] += b[i]
	}
	return sum
}

func polynomialEval(coeffs []float64, t float64) float64 {
	value := 0.0
	for i := len(coeffs) - 1; i >= 0; i-- {
		value = value*t + coeffs[i]
	}
	return value
}

func trimPolynomial(coeffs []float64) []float64 {
	tolerance := polynomialValueTolerance(coeffs)
	for len(coeffs) > 0 && math.Abs(coeffs[len(coeffs)-1]) <= tolerance {
		coeffs = coeffs[:len(coeffs)-1]
	}
	return coeffs
}

func polynomialValueTolerance(coeffs []float64) float64 {
	maxCoeff := 0.0
	for _, coeff := range coeffs {
		maxCoeff = math.Max(maxCoeff, math.Abs(coeff))
	}
	return math.Max(1e-9, maxCoeff*1e-10)
}

func appendUniqueFloat(values []float64, value float64) []float64 {
	value = clampUnit(value)
	for _, existing := range values {
		if math.Abs(existing-value) <= parameterTolerance {
			return values
		}
	}
	values = append(values, value)
	for i := len(values) - 1; i > 0 && values[i] < values[i-1]; i-- {
		values[i], values[i-1] = values[i-1], values[i]
	}
	return values
}

func clampUnit(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func appendUniquePoint(points []*geo.Point, p *geo.Point) []*geo.Point {
	for _, existing := range points {
		if geo.EuclideanDistance(existing.X, existing.Y, p.X, p.Y) <= intersectionTolerance {
			return points
		}
	}
	return append(points, p)
}

func cycleSegmentIntersections(radius float64, segments []*geo.Segment) []*geo.Point {
	var intersections []*geo.Point
	for _, segment := range segments {
		intersections = append(intersections, circleSegmentIntersections(radius, segment)...)
	}
	return intersections
}

func circleSegmentIntersections(radius float64, segment *geo.Segment) []*geo.Point {
	dx := segment.End.X - segment.Start.X
	dy := segment.End.Y - segment.Start.Y
	a := dx*dx + dy*dy
	if a == 0 {
		return nil
	}

	b := 2 * (segment.Start.X*dx + segment.Start.Y*dy)
	c := segment.Start.X*segment.Start.X + segment.Start.Y*segment.Start.Y - radius*radius
	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		if discriminant > -intersectionTolerance {
			discriminant = 0
		} else {
			return nil
		}
	}

	root := math.Sqrt(discriminant)
	var intersections []*geo.Point
	for _, t := range []float64{(-b - root) / (2 * a), (-b + root) / (2 * a)} {
		if t < -intersectionTolerance || t > 1+intersectionTolerance {
			continue
		}
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		intersections = append(intersections, geo.NewPoint(
			segment.Start.X+t*dx,
			segment.Start.Y+t*dy,
		))
		if root == 0 {
			break
		}
	}
	return intersections
}

func boxSegments(box *geo.Box) []*geo.Segment {
	tl := box.TopLeft
	tr := geo.NewPoint(tl.X+box.Width, tl.Y)
	br := geo.NewPoint(tr.X, tr.Y+box.Height)
	bl := geo.NewPoint(tl.X, br.Y)
	return []*geo.Segment{
		geo.NewSegment(tl, tr),
		geo.NewSegment(tr, br),
		geo.NewSegment(br, bl),
		geo.NewSegment(bl, tl),
	}
}

func normalizeAngleInRange(angle, startAngle, endAngle float64) float64 {
	for angle < startAngle {
		angle += 2 * math.Pi
	}
	for angle > endAngle && angle-2*math.Pi >= startAngle {
		angle -= 2 * math.Pi
	}
	return angle
}

func cubicArcRoute(radius, startAngle, endAngle float64) []*geo.Point {
	segments := int(math.Ceil((endAngle - startAngle) / (math.Pi / 2)))
	step := (endAngle - startAngle) / float64(segments)

	route := []*geo.Point{pointOnCircle(radius, startAngle)}
	for i := 0; i < segments; i++ {
		a1 := startAngle + float64(i)*step
		route = append(route, cubicArcSegment(radius, a1, a1+step)...)
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

func pointOnCircle(radius, angle float64) *geo.Point {
	return geo.NewPoint(radius*math.Cos(angle), radius*math.Sin(angle))
}

func fitRootToCycle(g *d2graph.Graph) {
	tl, br := cycleBounds(g)
	if math.IsInf(tl.X, 0) || math.IsInf(tl.Y, 0) ||
		math.IsInf(br.X, 0) || math.IsInf(br.Y, 0) {
		return
	}

	dx := -tl.X
	dy := -tl.Y
	if dx != 0 || dy != 0 {
		for _, obj := range g.Root.ChildrenArray {
			obj.MoveWithDescendants(dx, dy)
		}
		for _, edge := range g.Edges {
			edge.Move(dx, dy)
		}
	}
	g.Root.Box = geo.NewBox(geo.NewPoint(0, 0), br.X-tl.X, br.Y-tl.Y)
}

func cycleBounds(g *d2graph.Graph) (tl, br *geo.Point) {
	tl = geo.NewPoint(math.Inf(1), math.Inf(1))
	br = geo.NewPoint(math.Inf(-1), math.Inf(-1))

	addPoint := func(p *geo.Point) {
		if p == nil {
			return
		}
		tl.X = math.Min(tl.X, p.X)
		tl.Y = math.Min(tl.Y, p.Y)
		br.X = math.Max(br.X, p.X)
		br.Y = math.Max(br.Y, p.Y)
	}
	addBox := func(topLeft *geo.Point, width, height float64) {
		if topLeft == nil {
			return
		}
		addPoint(topLeft)
		addPoint(geo.NewPoint(topLeft.X+width, topLeft.Y+height))
	}

	for _, obj := range g.Objects {
		if obj.TopLeft == nil {
			continue
		}
		addPoint(obj.TopLeft)
		addPoint(geo.NewPoint(obj.TopLeft.X+obj.Width, obj.TopLeft.Y+obj.Height))
		if obj.HasLabel() {
			addBox(obj.GetLabelTopLeft(), float64(obj.LabelDimensions.Width), float64(obj.LabelDimensions.Height))
		}
		if iconTL, iconSize := objectIconBox(obj); iconTL != nil {
			addBox(iconTL, iconSize, iconSize)
		}
	}
	for _, edge := range g.Edges {
		for _, p := range edge.Route {
			addPoint(p)
		}
		if labelTL, width, height := edgeLabelBox(edge); labelTL != nil {
			addBox(labelTL, width, height)
		}
		if iconTL, width, height := edgeIconBox(edge); iconTL != nil {
			addBox(iconTL, width, height)
		}
		if labelTL, width, height := edgeArrowheadLabelBox(edge, false); labelTL != nil {
			addBox(labelTL, width, height)
		}
		if labelTL, width, height := edgeArrowheadLabelBox(edge, true); labelTL != nil {
			addBox(labelTL, width, height)
		}
	}
	return tl, br
}

func objectIconBox(obj *d2graph.Object) (*geo.Point, float64) {
	if !obj.HasIcon() || obj.IconPosition == nil {
		return nil, 0
	}
	iconPosition := label.FromString(*obj.IconPosition)
	box := obj.ToShape().GetBox()
	if !iconPosition.IsOutside() {
		box = obj.ToShape().GetInnerBox()
	}
	iconSize := float64(d2target.GetIconSize(box, *obj.IconPosition))
	return iconPosition.GetPointOnBox(box, label.PADDING, iconSize, iconSize), iconSize
}

func edgeLabelBox(edge *d2graph.Edge) (*geo.Point, float64, float64) {
	if edge.Label.Value == "" || len(edge.Route) < 2 {
		return nil, 0, 0
	}
	labelPosition := label.InsideMiddleCenter
	if edge.LabelPosition != nil {
		labelPosition = label.FromString(*edge.LabelPosition)
	}
	if labelPosition == label.Unset {
		labelPosition = label.InsideMiddleCenter
	}
	labelPercentage := 0.0
	if edge.LabelPercentage != nil {
		labelPercentage = *edge.LabelPercentage
	}
	width := float64(edge.LabelDimensions.Width)
	height := float64(edge.LabelDimensions.Height)
	point, _ := labelPosition.GetPointOnRoute(edge.Route, 2, labelPercentage, width, height)
	return point, width, height
}

func edgeIconBox(edge *d2graph.Edge) (*geo.Point, float64, float64) {
	if edge.Icon == nil || len(edge.Route) < 2 {
		return nil, 0, 0
	}
	connection := edgeTargetConnection(edge)
	connection.Icon = edge.Icon
	if edge.IconPosition != nil {
		if position, ok := d2ast.LabelPositionsMapping[edge.IconPosition.Value]; ok {
			connection.IconPosition = position.String()
		} else {
			connection.IconPosition = label.FromString(edge.IconPosition.Value).String()
		}
	} else {
		connection.IconPosition = label.InsideMiddleCenter.String()
	}
	return connection.GetIconPosition(), d2target.DEFAULT_ICON_SIZE, d2target.DEFAULT_ICON_SIZE
}

func edgeArrowheadLabelBox(edge *d2graph.Edge, isDst bool) (*geo.Point, float64, float64) {
	if len(edge.Route) < 2 {
		return nil, 0, 0
	}

	var attrs *d2graph.Attributes
	if isDst {
		attrs = edge.DstArrowhead
	} else {
		attrs = edge.SrcArrowhead
	}
	if attrs == nil || attrs.Label.Value == "" {
		return nil, 0, 0
	}

	connection := edgeTargetConnection(edge)
	width := attrs.LabelDimensions.Width
	height := attrs.LabelDimensions.Height
	text := &d2target.Text{
		Label:       attrs.Label.Value,
		LabelWidth:  width,
		LabelHeight: height,
	}
	if isDst {
		connection.DstLabel = text
	} else {
		connection.SrcLabel = text
	}
	return connection.GetArrowheadLabelPosition(isDst), float64(width), float64(height)
}

func edgeTargetConnection(edge *d2graph.Edge) *d2target.Connection {
	connection := d2target.BaseConnection()
	connection.Route = edge.Route
	if edge.Label.Value != "" {
		connection.Label = edge.Label.Value
		connection.LabelWidth = edge.LabelDimensions.Width
		connection.LabelHeight = edge.LabelDimensions.Height
	}
	if edge.LabelPosition != nil {
		connection.LabelPosition = *edge.LabelPosition
	} else {
		connection.LabelPosition = label.InsideMiddleCenter.String()
	}
	if edge.LabelPercentage != nil {
		connection.LabelPercentage = *edge.LabelPercentage
	}
	if edge.Style.StrokeWidth != nil {
		if strokeWidth, err := strconv.Atoi(edge.Style.StrokeWidth.Value); err == nil {
			connection.StrokeWidth = strokeWidth
		}
	}
	if edge.SrcArrow {
		connection.SrcArrow = d2target.DefaultArrowhead
		if edge.SrcArrowhead != nil {
			connection.SrcArrow = edge.SrcArrowhead.ToArrowhead()
		}
	}
	if edge.DstArrow {
		connection.DstArrow = d2target.DefaultArrowhead
		if edge.DstArrowhead != nil {
			connection.DstArrow = edge.DstArrowhead.ToArrowhead()
		}
	}
	return connection
}

func positionLabelsIcons(obj *d2graph.Object) {
	if obj.Icon != nil && obj.IconPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
			if obj.LabelPosition == nil {
				obj.LabelPosition = go2.Pointer(label.OutsideTopRight.String())
			}
		} else if obj.SQLTable != nil || obj.Class != nil || obj.Language != "" {
			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
		} else {
			obj.IconPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
	}

	if obj.IsCycleDiagram() && len(obj.ChildrenArray) > 0 && obj.HasLabel() && obj.Attributes.LabelPosition == nil {
		obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
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
	avoidLabelIconOverlap(obj)
}

func avoidLabelIconOverlap(obj *d2graph.Object) {
	if obj.Attributes.LabelPosition != nil || !obj.HasLabel() || !obj.HasIcon() ||
		obj.LabelPosition == nil || obj.IconPosition == nil {
		return
	}
	labelTL := obj.GetLabelTopLeft()
	iconTL, iconSize := objectIconBox(obj)
	if labelTL == nil || iconTL == nil {
		return
	}
	labelBox := geo.Box{
		TopLeft: labelTL,
		Width:   float64(obj.LabelDimensions.Width),
		Height:  float64(obj.LabelDimensions.Height),
	}
	iconBox := geo.Box{
		TopLeft: iconTL,
		Width:   iconSize,
		Height:  iconSize,
	}
	if boxesIntersect(labelBox, iconBox) {
		obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
	}
}

func boxesIntersect(a, b geo.Box) bool {
	return a.TopLeft.X < b.TopLeft.X+b.Width &&
		a.TopLeft.X+a.Width > b.TopLeft.X &&
		a.TopLeft.Y < b.TopLeft.Y+b.Height &&
		a.TopLeft.Y+a.Height > b.TopLeft.Y
}
