package packing

import (
	"math"
	"strconv"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type routedContainerSide uint8

type routedContainerBoxDecision uint8

const (
	routedContainerDeferToSideConstraints routedContainerBoxDecision = iota
	routedContainerKeepOriginalBox
	routedContainerUseProposedBox
)

const (
	routedContainerTop routedContainerSide = 1 << iota
	routedContainerLeft
	routedContainerBottom
	routedContainerRight

	// D2 renders an edge with this corner radius when no explicit edge style
	// overrides it. TALA retains nil style provenance, so the safety proof must
	// account for the renderer default instead of treating nil as a straight path.
	routedContainerDefaultEdgeBorderRadius  = 10.
	routedContainerMaxBorderRadiusTextBytes = 64
)

// binPackCanUseRoutedContainerBox reports whether the box currently proposed
// on root preserves every affected routed segment and supported root route
// attachment from original. It is intentionally narrow: root self-loops,
// root-to-descendant routes, and curved routes keep the original box. A direct
// root route is required to supply the side-attachment contract for an
// admissible shrink; descendant-only routed containers defer to BinPack's
// ordinary side constraints. The new box must remain inside the old one, and
// every side touched by a root endpoint must remain geometrically identical.
// Preserving the complete side also keeps renderer-only rounded corners from
// moving onto a fixed endpoint.
func binPackCanUseRoutedContainerBox(
	graph *layoutgraph.Graph,
	root *layoutgraph.Node,
	original *geo.Box,
	graphIncidentEdges []*layoutgraph.Edge,
	guard *limits.WorkGuard,
) (routedContainerBoxDecision, error) {
	if err := guard.Step(); err != nil {
		return routedContainerDeferToSideConstraints, err
	}
	if graph == nil || root == nil || root.Graph != graph {
		return routedContainerKeepOriginalBox, nil
	}
	if len(graphIncidentEdges) == 0 {
		// With no root-incident route, ordinary side constraints determine which
		// coordinates the wrapped box may change.
		return routedContainerDeferToSideConstraints, guard.Finish()
	}
	if root.TopLeft == nil || root.Shape == nil || original == nil || original.TopLeft == nil {
		return routedContainerKeepOriginalBox, nil
	}

	descendants, err := graph.AllDescendantNodesWithWorkGuard(root, true, guard)
	if err != nil {
		return routedContainerDeferToSideConstraints, err
	}
	descendantSet := make(map[*layoutgraph.Node]struct{}, len(descendants))
	for _, node := range descendants {
		if err := guard.Step(); err != nil {
			return routedContainerDeferToSideConstraints, err
		}
		if node == nil {
			return routedContainerKeepOriginalBox, nil
		}
		descendantSet[node] = struct{}{}
	}

	affectedEdges := make([]*layoutgraph.Edge, 0, len(graphIncidentEdges))
	affectedEdgeSet := make(map[*layoutgraph.Edge]struct{}, len(graphIncidentEdges))
	for _, edge := range graph.Edges {
		if err := guard.Step(); err != nil {
			return routedContainerDeferToSideConstraints, err
		}
		if edge == nil || edge.From == nil || edge.To == nil {
			return routedContainerKeepOriginalBox, nil
		}
		_, fromDescendant := descendantSet[edge.From]
		_, toDescendant := descendantSet[edge.To]
		if edge.From != root && edge.To != root && !fromDescendant && !toDescendant {
			continue
		}
		if _, duplicate := affectedEdgeSet[edge]; duplicate {
			return routedContainerKeepOriginalBox, nil
		}
		affectedEdgeSet[edge] = struct{}{}
		affectedEdges = append(affectedEdges, edge)
	}
	graphEdges := make(map[*layoutgraph.Edge]struct{}, len(graphIncidentEdges))
	for _, edge := range graphIncidentEdges {
		if err := guard.Step(); err != nil {
			return routedContainerKeepOriginalBox, err
		}
		if edge == nil || (edge.From != root && edge.To != root) {
			return routedContainerKeepOriginalBox, nil
		}
		if _, duplicate := graphEdges[edge]; duplicate {
			return routedContainerKeepOriginalBox, nil
		}
		if _, graphOwned := affectedEdgeSet[edge]; !graphOwned {
			return routedContainerKeepOriginalBox, nil
		}
		graphEdges[edge] = struct{}{}
	}
	if !root.IsContainer() || !root.CanContain() || !root.Shape.IsRectangular() {
		return routedContainerKeepOriginalBox, nil
	}
	if dx, dy := root.ModifierElementAdjustments(); dx != 0 || dy != 0 {
		return routedContainerKeepOriginalBox, nil
	}
	if !routedContainerBoxIsFinite(&root.Box) || !routedContainerBoxIsFinite(original) {
		return routedContainerKeepOriginalBox, nil
	}
	if !routedContainerBoxCovers(original, &root.Box) {
		return routedContainerKeepOriginalBox, nil
	}
	if root.FixedTopLeft != nil &&
		(root.TopLeft.X != original.TopLeft.X || root.TopLeft.Y != original.TopLeft.Y) {
		return routedContainerKeepOriginalBox, nil
	}

	checkedEndpoint := false
	rootEdges := make(map[*layoutgraph.Edge]struct{}, len(root.Edges))
	for _, edge := range root.Edges {
		if err := guard.Step(); err != nil {
			return routedContainerKeepOriginalBox, err
		}
		if edge == nil || edge.From == nil || edge.To == nil || edge.From == edge.To || len(edge.Points) < 2 {
			return routedContainerKeepOriginalBox, nil
		}
		if _, exists := graphEdges[edge]; !exists {
			return routedContainerKeepOriginalBox, nil
		}
		if _, duplicate := rootEdges[edge]; duplicate {
			return routedContainerKeepOriginalBox, nil
		}
		rootEdges[edge] = struct{}{}
		incident := false
		if edge.From == root {
			incident = true
			checkedEndpoint = true
			if err := guard.Step(); err != nil {
				return routedContainerKeepOriginalBox, err
			}
			if !routedContainerEndpointKeepsSides(original, &root.Box, edge.Points[0]) {
				return routedContainerKeepOriginalBox, nil
			}
		}
		if edge.To == root {
			incident = true
			checkedEndpoint = true
			if err := guard.Step(); err != nil {
				return routedContainerKeepOriginalBox, err
			}
			if !routedContainerEndpointKeepsSides(original, &root.Box, edge.Points[len(edge.Points)-1]) {
				return routedContainerKeepOriginalBox, nil
			}
		}
		if !incident {
			return routedContainerKeepOriginalBox, nil
		}
		adjacent := edge.From
		if adjacent == root {
			adjacent = edge.To
		}
		inside, err := binPackIsDescendantOf(adjacent, root, guard)
		if err != nil {
			return routedContainerKeepOriginalBox, err
		}
		if inside {
			// A route to a descendant may leave and re-enter the proposed
			// box even while its root endpoint remains attached.
			return routedContainerKeepOriginalBox, nil
		}
	}
	if len(rootEdges) != len(graphEdges) || !checkedEndpoint {
		return routedContainerKeepOriginalBox, nil
	}
	routesPreserved, err := routedContainerRoutesStayInsideShrink(
		affectedEdges, original, &root.Box, guard,
	)
	if err != nil {
		return routedContainerKeepOriginalBox, err
	}
	if err := guard.Finish(); err != nil {
		return routedContainerKeepOriginalBox, err
	}
	if routesPreserved {
		return routedContainerUseProposedBox, nil
	}
	return routedContainerKeepOriginalBox, nil
}

// routedContainerRoutesStayInsideShrink proves that shrinking a routed
// container does not discard any part of an affected route that was inside the
// original outer box. For each straight route segment, its intersection with
// the original rectangle is another segment. Both clipped endpoints must be in
// the proposed rectangle; convexity then contains the complete clipped segment.
// This is deliberately an engine outer-box invariant: layoutgraph does not
// carry arbitrary D2 node BorderRadius, so it does not claim containment by a
// renderer-specific rounded node outline. Direct root attachments separately
// preserve their complete endpoint-bearing side.
func routedContainerRoutesStayInsideShrink(
	edges []*layoutgraph.Edge,
	original, proposed *geo.Box,
	guard *limits.WorkGuard,
) (bool, error) {
	for _, edge := range edges {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if edge == nil || edge.From == nil || edge.To == nil || len(edge.Points) < 2 || edge.IsCurve {
			return false, nil
		}

		for _, point := range edge.Points {
			if err := guard.Step(); err != nil {
				return false, err
			}
			if !routedContainerPointIsFinite(point) {
				return false, nil
			}
		}
		roundedCornersPreserved, err := routedContainerRoundedCornersStayInsideShrink(
			edge, original, proposed, guard,
		)
		if err != nil {
			return false, err
		}
		if !roundedCornersPreserved {
			return false, nil
		}

		for index := 0; index < len(edge.Points)-1; index++ {
			if err := guard.Step(); err != nil {
				return false, err
			}
			preserved, err := routedContainerSegmentStaysInsideShrink(
				original, proposed, edge.Points[index], edge.Points[index+1], guard,
			)
			if err != nil {
				return false, err
			}
			if !preserved {
				return false, nil
			}
		}
	}
	return true, guard.Finish()
}

// routedContainerRoundedCornersStayInsideShrink accounts for the renderer's
// rounded polyline corners. The ordinary rounded corner is a cubic Bezier
// contained by the entry/corner/exit control hull. A hull wholly inside the
// proposed box is preserved; a hull wholly outside one side of the original box
// never affected the container. Ambiguous hulls and the renderer's combined
// short-segment curve fail closed.
func routedContainerRoundedCornersStayInsideShrink(
	edge *layoutgraph.Edge,
	original, proposed *geo.Box,
	guard *limits.WorkGuard,
) (bool, error) {
	radius := routedContainerDefaultEdgeBorderRadius
	if edge.Style.BorderRadius != nil {
		if len(edge.Style.BorderRadius.Value) > routedContainerMaxBorderRadiusTextBytes {
			return false, nil
		}
		var err error
		radius, err = strconv.ParseFloat(edge.Style.BorderRadius.Value, 64)
		if err != nil {
			return false, nil
		}
	}
	if !routedContainerCoordinateIsFinite(radius) || radius < 0 {
		return false, nil
	}

	for index := 1; index < len(edge.Points)-1; index++ {
		if err := guard.Step(); err != nil {
			return false, err
		}
		previous, corner, next := edge.Points[index-1], edge.Points[index], edge.Points[index+1]
		incomingX, incomingY := corner.X-previous.X, corner.Y-previous.Y
		outgoingX, outgoingY := next.X-corner.X, next.Y-corner.Y
		incomingLength := math.Hypot(incomingX, incomingY)
		outgoingLength := math.Hypot(outgoingX, outgoingY)
		if !routedContainerCoordinateIsFinite(incomingLength) || incomingLength <= 0 ||
			!routedContainerCoordinateIsFinite(outgoingLength) || outgoingLength <= 0 {
			return false, nil
		}
		if radius == 0 {
			continue
		}
		if incomingLength < radius || outgoingLength/2 < radius {
			// Short corners use a different effective radius and may combine
			// controls across vertices. This proof only admits the normal branch.
			return false, nil
		}

		entry := geo.Point{
			X: corner.X - incomingX/incomingLength*radius,
			Y: corner.Y - incomingY/incomingLength*radius,
		}
		exit := geo.Point{
			X: corner.X + outgoingX/outgoingLength*radius,
			Y: corner.Y + outgoingY/outgoingLength*radius,
		}
		if !routedContainerPointIsFinite(&entry) || !routedContainerPointIsFinite(&exit) ||
			!routedContainerControlHullIsPreserved(original, proposed, &entry, corner, &exit) {
			return false, nil
		}
	}
	return true, nil
}

func routedContainerControlHullIsPreserved(original, proposed *geo.Box, points ...*geo.Point) bool {
	allInsideProposed := true
	allLeft, allTop, allRight, allBottom := true, true, true, true
	originalLeft, originalTop := original.TopLeft.X, original.TopLeft.Y
	originalRight := originalLeft + original.Width
	originalBottom := originalTop + original.Height
	for _, point := range points {
		if !routedContainerPointIsFinite(point) {
			return false
		}
		allInsideProposed = allInsideProposed && proposed.Contains(point)
		allLeft = allLeft && point.X < originalLeft
		allTop = allTop && point.Y < originalTop
		allRight = allRight && point.X > originalRight
		allBottom = allBottom && point.Y > originalBottom
	}
	return allInsideProposed || allLeft || allTop || allRight || allBottom
}

// routedContainerSegmentStaysInsideShrink clips a straight segment to the
// closed original rectangle with Liang-Barsky. An empty intersection is
// unaffected; otherwise both ends of the clipped segment must be contained by
// the closed proposed rectangle.
func routedContainerSegmentStaysInsideShrink(
	original, proposed *geo.Box,
	start, end *geo.Point,
	guard *limits.WorkGuard,
) (bool, error) {
	if !routedContainerBoxIsFinite(original) || !routedContainerBoxIsFinite(proposed) ||
		!routedContainerPointIsFinite(start) || !routedContainerPointIsFinite(end) {
		return false, nil
	}
	if start.X == end.X && start.Y == end.Y {
		return false, nil
	}
	originalEnter, originalExit, originalIntersects, valid, err := routedContainerSegmentBoxInterval(
		original, start, end, guard,
	)
	if err != nil || !valid {
		return false, err
	}
	if !originalIntersects {
		return true, nil
	}
	proposedEnter, proposedExit, proposedIntersects, valid, err := routedContainerSegmentBoxInterval(
		proposed, start, end, guard,
	)
	if err != nil || !valid {
		return false, err
	}
	return proposedIntersects && proposedEnter <= originalEnter && proposedExit >= originalExit, nil
}

func routedContainerSegmentBoxInterval(
	box *geo.Box,
	start, end *geo.Point,
	guard *limits.WorkGuard,
) (enter, exit float64, intersects, valid bool, err error) {
	dx, dy := end.X-start.X, end.Y-start.Y
	if !routedContainerCoordinateIsFinite(dx) || !routedContainerCoordinateIsFinite(dy) {
		return 0, 0, false, false, nil
	}

	left, top := box.TopLeft.X, box.TopLeft.Y
	right, bottom := left+box.Width, top+box.Height
	constraints := [][2]float64{
		{-dx, start.X - left},
		{dx, right - start.X},
		{-dy, start.Y - top},
		{dy, bottom - start.Y},
	}
	tEnter, tExit := 0., 1.
	for _, constraint := range constraints {
		if err := guard.Step(); err != nil {
			return 0, 0, false, false, err
		}
		p, q := constraint[0], constraint[1]
		if p == 0 {
			if q < 0 {
				return 0, 0, false, true, nil
			}
			continue
		}
		ratio := q / p
		if !routedContainerCoordinateIsFinite(ratio) {
			return 0, 0, false, false, nil
		}
		if p < 0 {
			if ratio > tExit {
				return 0, 0, false, true, nil
			}
			if ratio > tEnter {
				tEnter = ratio
			}
		} else {
			if ratio < tEnter {
				return 0, 0, false, true, nil
			}
			if ratio < tExit {
				tExit = ratio
			}
		}
	}
	return tEnter, tExit, true, true, nil
}

func routedContainerEndpointKeepsSides(original, proposed *geo.Box, point *geo.Point) bool {
	originalSides := routedContainerPointSides(original, point)
	if originalSides == 0 {
		return false
	}
	originalLeft, originalTop := original.TopLeft.X, original.TopLeft.Y
	originalRight := originalLeft + original.Width
	originalBottom := originalTop + original.Height
	proposedLeft, proposedTop := proposed.TopLeft.X, proposed.TopLeft.Y
	proposedRight := proposedLeft + proposed.Width
	proposedBottom := proposedTop + proposed.Height

	if originalSides&routedContainerTop != 0 &&
		(proposedTop != originalTop || proposedLeft != originalLeft || proposedRight != originalRight) {
		return false
	}
	if originalSides&routedContainerLeft != 0 &&
		(proposedLeft != originalLeft || proposedTop != originalTop || proposedBottom != originalBottom) {
		return false
	}
	if originalSides&routedContainerBottom != 0 &&
		(proposedBottom != originalBottom || proposedLeft != originalLeft || proposedRight != originalRight) {
		return false
	}
	if originalSides&routedContainerRight != 0 &&
		(proposedRight != originalRight || proposedTop != originalTop || proposedBottom != originalBottom) {
		return false
	}
	return true
}

func routedContainerPointSides(box *geo.Box, point *geo.Point) routedContainerSide {
	if !routedContainerBoxIsFinite(box) || !routedContainerPointIsFinite(point) {
		return 0
	}
	left, top := box.TopLeft.X, box.TopLeft.Y
	right, bottom := left+box.Width, top+box.Height
	var sides routedContainerSide
	if point.Y == top && point.X >= left && point.X <= right {
		sides |= routedContainerTop
	}
	if point.X == left && point.Y >= top && point.Y <= bottom {
		sides |= routedContainerLeft
	}
	if point.Y == bottom && point.X >= left && point.X <= right {
		sides |= routedContainerBottom
	}
	if point.X == right && point.Y >= top && point.Y <= bottom {
		sides |= routedContainerRight
	}
	return sides
}

func routedContainerPointIsFinite(point *geo.Point) bool {
	return point != nil && routedContainerCoordinateIsFinite(point.X) && routedContainerCoordinateIsFinite(point.Y)
}

func routedContainerBoxCovers(outer, inner *geo.Box) bool {
	outerRight := outer.TopLeft.X + outer.Width
	outerBottom := outer.TopLeft.Y + outer.Height
	innerRight := inner.TopLeft.X + inner.Width
	innerBottom := inner.TopLeft.Y + inner.Height
	return inner.TopLeft.X >= outer.TopLeft.X && inner.TopLeft.Y >= outer.TopLeft.Y &&
		innerRight <= outerRight && innerBottom <= outerBottom
}

func routedContainerBoxIsFinite(box *geo.Box) bool {
	return box != nil && box.TopLeft != nil && box.Width > 0 && box.Height > 0 &&
		routedContainerCoordinateIsFinite(box.TopLeft.X) && routedContainerCoordinateIsFinite(box.TopLeft.Y) &&
		routedContainerCoordinateIsFinite(box.Width) && routedContainerCoordinateIsFinite(box.Height) &&
		routedContainerCoordinateIsFinite(box.TopLeft.X+box.Width) &&
		routedContainerCoordinateIsFinite(box.TopLeft.Y+box.Height)
}

func routedContainerCoordinateIsFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
