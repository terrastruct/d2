package routing

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// simplifyEdgeRoutes post-processes edge routes to remove unnecessary bending caused by the constraint that routes have to go through OVG nodes
// It identifies unnecessary bends in routes and eliminates them when possible.
func SimplifyEdgeRoutes(ctx context.Context, g *layoutgraph.Graph) error {
	return simplifyEdgeRoutesWithLimit(ctx, g, maxRouteStageWorkUnits)
}

func simplifyEdgeRoutesWithLimit(ctx context.Context, g *layoutgraph.Graph, workLimit uint64) error {
	return runAtomicRouteStage(ctx, "SimplifyEdgeRoutes", g, nil, workLimit, func(guard *routeWorkGuard) error {
		for _, edge := range g.Edges {
			if err := guard.step(); err != nil {
				return err
			}
			for changed := true; changed; {
				newPoints, err := simplifyPoints(g, edge, guard)
				if err != nil {
					return err
				}
				changed = len(newPoints) < len(edge.Points)
				if changed {
					edge.Points = newPoints
				}
			}
		}
		return nil
	})
}

func simplifyPoints(g *layoutgraph.Graph, edge *layoutgraph.Edge, guard *routeWorkGuard) ([]*geo.Point, error) {
	points := edge.Points

	if len(points) < 5 {
		return points, guard.step()
	}

	// Make a copy of the points to avoid modifying the original
	result := []*geo.Point{points[0]}
	i := 0

	/*
			We want to find these and just simplify to L-shape
			 //                 ▲
		   //                 │
		   //                 │
		   //                 │
		   // ────────┐       │
		   //         │       │
		   //         │       │
		   //         │       │
		   //         └───────┘
	*/
	for i < len(points)-4 {
		if err := guard.step(); err != nil {
			return nil, err
		}
		p1 := points[i]
		p2 := points[i+1]
		p3 := points[i+2]
		p4 := points[i+3]
		p5 := points[i+4]

		vec1X, vec1Y := p2.X-p1.X, p2.Y-p1.Y // p1→p2
		vec2X, vec2Y := p3.X-p2.X, p3.Y-p2.Y // p2→p3
		vec3X, vec3Y := p4.X-p3.X, p4.Y-p3.Y // p3→p4
		vec4X, vec4Y := p5.X-p4.X, p5.Y-p4.Y // p4→p5

		isHorizontal1 := isHorizontalSegment(p1, p2)
		isVertical1 := isVerticalSegment(p1, p2)
		isHorizontal2 := isHorizontalSegment(p2, p3)
		isVertical2 := isVerticalSegment(p2, p3)
		isHorizontal3 := isHorizontalSegment(p3, p4)
		isVertical3 := isVerticalSegment(p3, p4)
		isHorizontal4 := isHorizontalSegment(p4, p5)
		isVertical4 := isVerticalSegment(p4, p5)

		// Pattern detection for special case:
		// 1. p2→p3 and p4→p5 are both vertical and going in opposite directions
		// 2. p1→p2 and p3→p4 are both horizontal and going in the same direction
		isOppositeVerticals := isVertical2 && isVertical4 &&
			((vec2Y > 0 && vec4Y < 0) || (vec2Y < 0 && vec4Y > 0))

		isSameHorizontals := isHorizontal1 && isHorizontal3 &&
			((vec1X > 0 && vec3X > 0) || (vec1X < 0 && vec3X < 0))

		// Alternative pattern:
		// 1. p2→p3 and p4→p5 are both horizontal and going in opposite directions
		// 2. p1→p2 and p3→p4 are both vertical and going in the same direction
		isOppositeHorizontals := isHorizontal2 && isHorizontal4 &&
			((vec2X > 0 && vec4X < 0) || (vec2X < 0 && vec4X > 0))

		isSameVerticals := isVertical1 && isVertical3 &&
			((vec1Y > 0 && vec3Y > 0) || (vec1Y < 0 && vec3Y < 0))

		// Check for the pattern described
		patternFound := (isOppositeVerticals && isSameHorizontals) ||
			(isOppositeHorizontals && isSameVerticals)

		var intersection *geo.Point
		hasObstruction := false

		if patternFound {
			// Find the intersection point between the first segment (p1→p2) and fourth segment (p4→p5)
			intersection = findIntersectionPoint(p1, p2, p4, p5)

			// Both emitted legs must be legal. Checking only the extension of
			// the first leg misses obstacles crossed by the second leg, and can
			// even reverse the approach into a fixed row port.
			hasObstruction = intersection == nil
			if intersection != nil {
				if i == 0 && edge.From.ContainsPoint(p1, 1) && !sameRouteDirection(p1, p2, p1, intersection) {
					hasObstruction = true
				}
				if i+4 == len(points)-1 && edge.To.ContainsPoint(p5, 1) && !sameRouteDirection(p4, p5, intersection, p5) {
					hasObstruction = true
				}
				for _, leg := range [][2]*geo.Point{{p1, intersection}, {intersection, p5}} {
					blocked, err := lineIntersectsUnrelatedNode(g, edge, leg[0], leg[1], guard)
					if err != nil {
						return nil, err
					}
					hasObstruction = hasObstruction || blocked
					blocked, err = lineIntersectsOtherEdges(g, edge, leg[0], leg[1], guard)
					if err != nil {
						return nil, err
					}
					hasObstruction = hasObstruction || blocked
				}
			}
		}

		canBeSimplified := patternFound && !hasObstruction

		if canBeSimplified {
			// For a simplified route, we want: p1 → intersection → p5
			// p1 is already in the result array, so just add intersection and p5
			result = append(result, intersection)
			result = append(result, p5)

			i += 4

			// Assume 1 simplification per edge
			break
		} else {
			// If no pattern detected or it can't be simplified, keep the current point and move to the next
			result = append(result, p2)
			i++
		}
	}

	// Add remaining points, making sure we don't go out of bounds
	remainingStart := i + 1
	if remainingStart < len(points) {
		for j := remainingStart; j < len(points); j++ {
			if err := guard.step(); err != nil {
				return nil, err
			}
			result = append(result, points[j])
		}
	}

	return result, nil
}

// findIntersectionPoint calculates the intersection point of the lines p1→p2 and p3→p4
// This is used to find the intersection point between segments
func findIntersectionPoint(p1, p2, p3, p4 *geo.Point) *geo.Point {
	if isHorizontalSegment(p1, p2) && isVerticalSegment(p3, p4) {
		return &geo.Point{
			X: p4.X,
			Y: p1.Y,
		}
	} else if isVerticalSegment(p1, p2) && isHorizontalSegment(p3, p4) {
		return &geo.Point{
			X: p1.X,
			Y: p4.Y,
		}
	}

	return nil
}

func isHorizontalSegment(p1, p2 *geo.Point) bool {
	return p1.Y == p2.Y
}

func isVerticalSegment(p1, p2 *geo.Point) bool {
	return p1.X == p2.X
}

// sameRouteDirection preserves the signed departure/approach direction, not
// merely the axis. A zero-length replacement cannot preserve a port approach.
func sameRouteDirection(a, b, c, d *geo.Point) bool {
	return (b.X-a.X)*(d.X-c.X)+(b.Y-a.Y)*(d.Y-c.Y) > 0 &&
		(b.X-a.X)*(d.Y-c.Y) == (b.Y-a.Y)*(d.X-c.X)
}

// lineIntersectsUnrelatedNode checks every obstacle's open interior. Contact at
// an endpoint is legal; entering the endpoint's node from its far side is not.
// Ancestor containers are allowed because their descendants route inside them.
func lineIntersectsUnrelatedNode(g *layoutgraph.Graph, edge *layoutgraph.Edge, p1, p2 *geo.Point, guard *routeWorkGuard) (bool, error) {
	for _, node := range g.Nodes {
		if err := guard.step(); err != nil {
			return false, err
		}
		if node.TopLeft == nil || node.Width <= 0 || node.Height <= 0 {
			continue
		}
		if node != edge.From && node != edge.To &&
			(edge.From.IsDescendantOf(node) || edge.To.IsDescendantOf(node)) {
			continue
		}
		if orthogonalSegmentEntersNode(node, p1, p2) {
			return true, nil
		}
	}
	return false, nil
}

func orthogonalSegmentEntersNode(node *layoutgraph.Node, a, b *geo.Point) bool {
	const epsilon = 1e-6
	left, right := node.TopLeft.X+epsilon, node.TopLeft.X+node.Width-epsilon
	top, bottom := node.TopLeft.Y+epsilon, node.TopLeft.Y+node.Height-epsilon
	if a.Y == b.Y {
		return top < a.Y && a.Y < bottom && min(a.X, b.X) < right && max(a.X, b.X) > left
	}
	if a.X == b.X {
		return left < a.X && a.X < right && min(a.Y, b.Y) < bottom && max(a.Y, b.Y) > top
	}
	return true
}

// lineIntersectsOtherEdges checks if the line from p1 to p2 intersects with any
// other edges in the graph (excluding the edge being simplified)
func lineIntersectsOtherEdges(g *layoutgraph.Graph, currentEdge *layoutgraph.Edge, p1, p2 *geo.Point, guard *routeWorkGuard) (bool, error) {
	for _, edge := range g.Edges {
		if err := guard.step(); err != nil {
			return false, err
		}
		if edge == currentEdge {
			continue
		}

		if len(edge.Points) < 2 {
			continue
		}

		for i := 0; i < len(edge.Points)-1; i++ {
			if err := guard.step(); err != nil {
				return false, err
			}
			if intersects(p1, p2, edge.Points[i], edge.Points[i+1]) {
				return true, nil
			}
		}
	}

	return false, nil
}
