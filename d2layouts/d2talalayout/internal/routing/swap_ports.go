package routing

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// SwapAllEdgePorts applies port swapping to every node under one atomic
// route-stage budget and rollback boundary.
func SwapAllEdgePorts(ctx context.Context, graph *layoutgraph.Graph) error {
	return swapAllEdgePortsWithWorkLimit(ctx, graph, maxRouteStageWorkUnits)
}

// swapAllEdgePortsWithWorkLimit is SwapAllEdgePorts with an explicit budget
// for deterministic resource-boundary tests inside TALA.
func swapAllEdgePortsWithWorkLimit(ctx context.Context, graph *layoutgraph.Graph, workLimit uint64) error {
	return runAtomicRouteStage(ctx, "SwapEdgePorts", graph, nil, workLimit, func(guard *routeWorkGuard) error {
		for _, node := range graph.Nodes {
			if err := guard.step(); err != nil {
				return err
			}
			if _, err := swapEdgePortsGuarded(node, guard); err != nil {
				return err
			}
		}
		return nil
	})
}

func swapEdgePortsGuarded(n *layoutgraph.Node, guard *routeWorkGuard) (bool, error) {
	orientationToEdges := make(map[geo.Orientation][]*layoutgraph.Edge)
	portToEdges := make(map[geo.Point][]*layoutgraph.Edge)
	for _, e := range n.Edges {
		if err := guard.step(); err != nil {
			return false, err
		}
		// TODO: remove `len(e.Points) < 3` and handle straight edges
		if e.IsLoop() || len(e.Points) < 3 || e.HasTableColumn() {
			continue
		}
		if e.HasLargeArrowheadLabel() {
			continue
		}
		p1, p2, _ := pointsAt(e, n)
		var o geo.Orientation
		if p1.X < p2.X {
			o = geo.Right
		} else if p1.X > p2.X {
			o = geo.Left
		} else if p1.Y < p2.Y {
			o = geo.Bottom
		} else {
			o = geo.Top
		}
		orientationToEdges[o] = append(orientationToEdges[o], e)
		portToEdges[*p1] = append(portToEdges[*p1], e)
	}

	swappedSameSide, err := swapEdgesOnSameSide(n, orientationToEdges, portToEdges, guard)
	if err != nil {
		return false, err
	}
	swappedAdjacentSides, err := swapEdgesOnAdjacentSides(n, orientationToEdges, portToEdges, guard)
	if err != nil {
		return false, err
	}

	return swappedAdjacentSides || swappedSameSide, nil
}

// swapEdgesOnSameSide swaps edges on the same node side if they can be swapped
// .       input                       output
// .       ┌──────────                ┌────────────
// .    ┌──┼─────────────             |  ┌─────────────
// . ┌──┴──┴──────┐                ┌──┴──┴──────┐
// . │            ├───┐            │            ├───────┐
// . │    n       ├───┼───┐        │     n      ├───┐   │
// . └────────────┘   │   │        └────────────┘   │   │
func swapEdgesOnSameSide(n *layoutgraph.Node, orientationToEdges map[geo.Orientation][]*layoutgraph.Edge, portToEdges map[geo.Point][]*layoutgraph.Edge, guard *routeWorkGuard) (bool, error) {
	swapped := false
	for o, edges := range orientationToEdges {
		if err := guard.step(); err != nil {
			return false, err
		}
		// sorts by orientation
		if o.IsVertical() {
			if err := stableSortRouteValues(edges, func(a, b *layoutgraph.Edge) bool {
				p1, _, p3 := pointsAt(a, n)
				q1, _, q3 := pointsAt(b, n)
				if p1.X == q1.X {
					return p3.X < q3.X
				}
				return p1.X < q1.X
			}, guard); err != nil {
				return false, err
			}
		} else {
			if err := stableSortRouteValues(edges, func(a, b *layoutgraph.Edge) bool {
				p1, _, p3 := pointsAt(a, n)
				q1, _, q3 := pointsAt(b, n)
				if p1.Y == q1.Y {
					return p3.Y < q3.Y
				}
				return p1.Y < q1.Y
			}, guard); err != nil {
				return false, err
			}
		}
		for i := 0; i < len(edges)-1; i++ {
			if err := guard.step(); err != nil {
				return false, err
			}
			if edgesIntersect(edges[i], edges[i+1], n, o) && canSwapPorts(edges[i], edges[i+1], n, portToEdges) {
				p1, p2, _ := pointsAt(edges[i], n)
				q1, q2, _ := pointsAt(edges[i+1], n)
				swapCoordinate(p1, q1, o)
				swapCoordinate(p2, q2, o)
				doesNotPass, err := doesNotPassThroughNodesGuarded(n.Graph, []*layoutgraph.Node{n}, p1, p2, q1, q2, guard)
				if err != nil {
					return false, err
				}
				if doesNotPass {
					edges[i], edges[i+1] = edges[i+1], edges[i]
					swapped = true
				} else {
					swapCoordinate(p1, q1, o)
					swapCoordinate(p2, q2, o)
				}
			}
		}
	}
	return swapped, nil
}

// pointsAt returns the points composing the two segments starting at n.
// .   segment 1    ┌──────┐
// . ┌──────────────┤   n  │
// . │              └──────┘
// . │ segment 2
func pointsAt(e *layoutgraph.Edge, n *layoutgraph.Node) (*geo.Point, *geo.Point, *geo.Point) {
	if e.From == n {
		return e.Points[0], e.Points[1], e.Points[2]
	}
	return e.Points[len(e.Points)-1], e.Points[len(e.Points)-2], e.Points[len(e.Points)-3]
}

// swapCoordinate swaps the coordinate (x/y) opposite to the given orienation, e.g., if `o` is vertical, swaps x
func swapCoordinate(p, q *geo.Point, o geo.Orientation) {
	if o.IsVertical() {
		p.X, q.X = q.X, p.X
	} else {
		p.Y, q.Y = q.Y, p.Y
	}
}

func doesNotPassThroughNodesGuarded(g *layoutgraph.Graph, ns []*layoutgraph.Node, p1, p2, q1, q2 *geo.Point, guard *routeWorkGuard) (bool, error) {
OUTER:
	for _, node := range g.Nodes {
		if err := guard.step(); err != nil {
			return false, err
		}
		for _, n := range ns {
			if err := guard.step(); err != nil {
				return false, err
			}
			if node == n || n.IsDescendantOf(node) {
				continue OUTER
			}
		}
		pad := 2 * pathNodeProximityFloor
		paddedBox := geo.NewBox(
			geo.NewPoint(node.Box.TopLeft.X-pad, node.Box.TopLeft.Y-pad),
			node.Box.Width+pad*2,
			node.Box.Height+pad*2,
		)
		if segmentIntersectsBox(p1, p2, paddedBox) ||
			segmentIntersectsBox(q1, q2, paddedBox) {
			return false, nil
		}
	}
	return true, nil
}

// edgesIntersect checks if the given edges intersect at `n` like in the examples below
// .              ┌─────┐
// .        ┌─e1──┤  n  │
// . ┌──e2──┼─────┤     │
// . │      │     └─────┘
//
// or
//
// . │   │   ┌─────┐
// . └e2─┼───┤     │
// .     └─e1┤  n  │
// .         └─────┘
// the same patterns repeat to the four shape sides
func edgesIntersect(e1, e2 *layoutgraph.Edge, n *layoutgraph.Node, o geo.Orientation) bool {
	p1, p2, p3 := pointsAt(e1, n)
	q1, q2, q3 := pointsAt(e2, n)
	switch o {
	case geo.Left:
		// .                  ┌──────┐
		// .        p2┌───────┤p1    │
		// . q2       │       │      │
		// . ┌────────┼───────┤q1    │
		// . │        │       └──────┘
		// . │        │
		// . │q3      │p3
		bottomIntersection := q2.X < p2.X && p3.Y > q1.Y
		topIntersection := p2.X < q2.X && q3.Y < p1.Y
		return topIntersection || bottomIntersection
	case geo.Bottom:
		leftIntersection := p2.Y > q2.Y && q3.X < p1.X
		rightIntersection := q2.Y > p2.Y && p3.X > q1.X
		return leftIntersection || rightIntersection
	case geo.Top:
		leftIntersection := p2.Y < q2.Y && q3.X < p1.X
		rightIntersection := q2.Y < p2.Y && p3.X > q1.X
		return leftIntersection || rightIntersection
	case geo.Right:
		topIntersection := p2.X > q2.X && q3.Y < p1.Y
		bottomIntersection := q2.X > p2.X && p3.Y > q1.Y
		return topIntersection || bottomIntersection
	}
	return false
}

// canSwapPorts checks if the ports of the given edges can be swapped
// ports can be swapped if they have only 1 edge each, or if they are shared, if the edges have the same arrowhead
func canSwapPorts(e1, e2 *layoutgraph.Edge, n *layoutgraph.Node, portToEdges map[geo.Point][]*layoutgraph.Edge) bool {
	p1, _, _ := pointsAt(e1, n)
	p2, _, _ := pointsAt(e2, n)

	if len(portToEdges[*p1]) == 1 && len(portToEdges[*p2]) == 1 {
		// only one edge at each port, so arrowheads don't matter
		return true
	}

	// check if they have the same arrowheads
	p1Arrowhead := e1.ArrowheadTo(n)
	p2Arrowhead := e2.ArrowheadTo(n)
	return p1Arrowhead == p2Arrowhead
}

func refineEdgeGuarded(g *layoutgraph.Graph, e *layoutgraph.Edge, guard *routeWorkGuard) (bool, error) {
	if err := guard.step(); err != nil {
		return false, err
	}
	if e.IsLoop() || len(e.Points) <= 2 || e.IsBetweenTableColumns() || e.IsTreeEdge() {
		return false, nil
	}
	occupiedPorts, err := occupiedPortsGuarded(e.From, guard)
	if err != nil {
		return false, err
	}
	toPorts, err := occupiedPortsGuarded(e.To, guard)
	if err != nil {
		return false, err
	}
	for port := range toPorts {
		if err := guard.step(); err != nil {
			return false, err
		}
		occupiedPorts[port] = struct{}{}
	}

	if isEdgeSShaped(e) {
		return makeStraightLineGuarded(g, e, occupiedPorts, guard)
		// TODO: some ideas to improve later on
		// - try to make L-shaped too
		// - if L-shaped and straight line failed, try to move or grow the node a bit
		//   consider that only nodes without FixedTopLeft and fixed size can have these transforms
		// For example, this could be a straight line if the node below moved a bit or if its size increased a bit
		// Note that this should be quite safe to extend/move the node in the direction of the bend as there should be
		// no other nodes there, just need to consider moving other edges on that side so that ports are properly adjusted
		//      ┌────────┐
		//      │        │
		//      └───────┬┘
		// ┌─────────┐  │
		// └─────────┘  │
		//   ┌───────┐  │
		//   │       ├──┘
		//   └───────┘
	} else if isEdgeLShaped(e) {
		return makeStraightLineGuarded(g, e, occupiedPorts, guard)
		// TODO: see the straight line strategy above that could also be applied here
	} else if isEdgeUShaped(e) {
		// TODO: specific improvements for U-shaped routes
		return false, nil
	}

	swapped := false
	if isSShaped(e.Points[:4]) {
		// the first 3 segments are S shaped, try to turn them into L shaped
		//             ┌────────        ┌─────────────
		//     ┌───────┘                │
		// ┌───┴──┐                 ┌───┴──┐
		// │      │                 │      │
		// │      │                 │      │
		// └──────┘                 └──────┘
		p := sToLShapedBendPoint(e.Points[:4])
		doesNotPass, err := doesNotPassThroughNodesGuarded(g, []*layoutgraph.Node{e.From, e.To}, e.Points[0], p, p, e.Points[3], guard)
		if err != nil {
			return false, err
		}
		if doesNotPass {
			if err := removeEdgePointsGuarded(e, guard, 1, 2, 3); err != nil {
				return false, err
			}
			if err := insertEdgePointGuarded(e, p, 1, guard); err != nil {
				return false, err
			}
			swapped = true
		}
	}

	// TODO: try improvements along the edge
	//      input         possible output
	// │                       │
	// │                       │
	// └────┐                  │
	//      │                  │
	//      │                  │
	//      └───────           └────────
	//
	//     ┌────────           ┌────────
	//     │                   │
	//     │                   │
	//     └─────┐             │
	//           │             │
	//           │             │
	// Exceptions to look for:
	// - if the edge new edge doesn't go through other nodes
	// - if the new edge doesn't overlap, or is too close to other segments

	return swapped, nil
}

func occupiedPortsGuarded(n *layoutgraph.Node, guard *routeWorkGuard) (map[geo.Point]struct{}, error) {
	ports := make(map[geo.Point]struct{}, len(n.Edges))
	for _, e := range n.Edges {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if e.From == n {
			ports[*e.SourcePort()] = struct{}{}
		} else {
			ports[*e.TargetPort()] = struct{}{}
		}
	}
	return ports, nil
}

func makeStraightLineGuarded(g *layoutgraph.Graph, e *layoutgraph.Edge, occupiedPorts map[geo.Point]struct{}, guard *routeWorkGuard) (bool, error) {
	fromPort := e.SourcePort()
	toPort := e.TargetPort()
	step := pathNodeProximityFloor
	ranges, isHorizontal, err := tunnelRangesBetweenGuarded(g, e.From, e.To, false, guard)
	if err != nil {
		return false, err
	}
	for _, r := range ranges {
		if err := guard.step(); err != nil {
			return false, err
		}
		// avoid having edges alined with the node boundaries
		for v := r.start + step; v <= r.end-step; v += step {
			if err := guard.step(); err != nil {
				return false, err
			}
			var from, to *geo.Point
			if isHorizontal {
				from = geo.NewPoint(fromPort.X, v)
				to = geo.NewPoint(toPort.X, v)
			} else {
				from = geo.NewPoint(v, fromPort.Y)
				to = geo.NewPoint(v, toPort.Y)
			}
			if _, is := occupiedPorts[*from]; is {
				continue
			}
			if _, is := occupiedPorts[*to]; is {
				continue
			}
			// the ports are free and tunnels ensure there's no node in between
			// so we can make this edge a straight line
			e.Points = []*geo.Point{from, to}
			return true, nil
		}
	}
	return false, nil
}

// sToLShapedBendPoint returns the bend point that transforms an S-shaped route
// into an L-shaped route.
// .             ┌────────        ┌─────────────
// .     ┌───────┘                │
// . ┌───┴──┐                 ┌───┴──┐
// . │      │                 │      │
// . │      │                 │      │
// . └──────┘                 └──────┘
func sToLShapedBendPoint(ps []*geo.Point) *geo.Point {
	if ps[0].X == ps[1].X {
		// vertical
		return geo.NewPoint(ps[1].X, ps[3].Y)
	}
	return geo.NewPoint(ps[3].X, ps[1].Y)
}

func isEdgeUShaped(e *layoutgraph.Edge) bool {
	return isUShaped(e.Points)
}

func isEdgeSShaped(e *layoutgraph.Edge) bool {
	return isSShaped(e.Points)
}

func isSShaped(ps []*geo.Point) bool {
	return len(ps) == 4 && !isUShaped(ps)
}

func isUShaped(ps []*geo.Point) bool {
	if len(ps) != 4 {
		return false
	}

	if ps[0].X == ps[1].X {
		// 0        3
		// │        │
		// │        │
		// 1────────2
		sameDirection := geo.Sign(ps[0].Y-ps[1].Y) == geo.Sign(ps[3].Y-ps[2].Y)
		isHorizontal := ps[1].Y == ps[2].Y
		return sameDirection && isHorizontal
	}

	// 0─────1
	//       │
	//       │
	// 3─────2
	sameDirection := geo.Sign(ps[0].X-ps[1].X) == geo.Sign(ps[3].X-ps[2].X)
	isVertical := ps[1].X == ps[2].X
	return sameDirection && isVertical
}

func isEdgeLShaped(e *layoutgraph.Edge) bool {
	return isLShaped(e.Points)
}

func isLShaped(ps []*geo.Point) bool {
	if len(ps) != 3 {
		return false
	}

	isVertical := ps[0].X == ps[1].X
	isHorizontal := ps[1].Y == ps[2].Y
	// either one of these
	// .               0                          0─────────1
	// .               │                                    │
	// .               │                                    │
	// .               │                                    │
	// .               │                                    2
	// .               1─────2
	return (isVertical && isHorizontal) || (!isVertical && !isHorizontal)
}

// swapPoints swaps both X and Y of the given points
func swapPoints(p1, p2 *geo.Point) {
	p1.X, p2.X = p2.X, p1.X
	p1.Y, p2.Y = p2.Y, p1.Y
}

func copyEdgePointsGuarded(edge *layoutgraph.Edge, guard *routeWorkGuard) ([]*geo.Point, error) {
	points := make([]*geo.Point, 0, len(edge.Points))
	for _, point := range edge.Points {
		if err := guard.step(); err != nil {
			return nil, err
		}
		points = append(points, point.Copy())
	}
	return points, nil
}

func insertEdgePointGuarded(edge *layoutgraph.Edge, point *geo.Point, index int, guard *routeWorkGuard) error {
	if index < 0 || index > len(edge.Points) {
		return fmt.Errorf("TALA %s route insertion index %d is out of bounds", guard.location, index)
	}
	points := make([]*geo.Point, 0, len(edge.Points)+1)
	for pointIndex, existing := range edge.Points {
		if err := guard.step(); err != nil {
			return err
		}
		if pointIndex == index {
			points = append(points, point)
		}
		points = append(points, existing)
	}
	if index == len(edge.Points) {
		points = append(points, point)
	}
	edge.Points = points
	return nil
}

func removeEdgePointsGuarded(edge *layoutgraph.Edge, guard *routeWorkGuard, indices ...int) error {
	capacity := len(edge.Points) - len(indices)
	if capacity < 0 {
		return fmt.Errorf("TALA %s route removal count exceeds route length", guard.location)
	}
	points := make([]*geo.Point, 0, capacity)
OUTER:
	for pointIndex, point := range edge.Points {
		if err := guard.step(); err != nil {
			return err
		}
		for _, removeIndex := range indices {
			if err := guard.step(); err != nil {
				return err
			}
			if pointIndex == removeIndex {
				continue OUTER
			}
		}
		points = append(points, point)
	}
	edge.Points = points
	return nil
}

// swapEdgesOnAdjacentSides swaps edges on adjacent sides (top/bottom with left/right) if they can be swapped
// .       input                    output
// .           │                      │
// .    ┌──────┼──────          ┌─────┘
// .    │      │                │      ┌───────
// . ┌──┴───┐  │             ┌──┴───┐  │
// . │  n   ├──┘             │  n   ├──┘
// . └──────┘                └──────┘
func swapEdgesOnAdjacentSides(n *layoutgraph.Node, orientationToEdges map[geo.Orientation][]*layoutgraph.Edge, portToEdges map[geo.Point][]*layoutgraph.Edge, guard *routeWorkGuard) (bool, error) {
	swapped := false
	topBottomEdges := append(orientationToEdges[geo.Top], orientationToEdges[geo.Bottom]...)
	leftRightEdges := append(orientationToEdges[geo.Left], orientationToEdges[geo.Right]...)
	for _, aEdge := range topBottomEdges {
		if err := guard.step(); err != nil {
			return false, err
		}
		for _, bEdge := range leftRightEdges {
			if err := guard.step(); err != nil {
				return false, err
			}
			// we need to check the edge size here in case we were able to improve it
			if len(aEdge.Points) <= 2 {
				break
			}
			if len(bEdge.Points) <= 2 {
				continue
			}
			if !canSwapPorts(aEdge, bEdge, n, portToEdges) {
				continue
			}
			// .          b3
			// .   a2      │
			// .    ┌──────┼──── a3
			// .    │      │
			// . ┌a1┴───┐  │
			// . │  n   b1─┘ b2
			// . └──────┘
			a1, a2, a3 := pointsAt(aEdge, n)
			b1, b2, b3 := pointsAt(bEdge, n)

			intersection := geo.IntersectionPoint(a2, a3, b2, b3)
			if intersection == nil {
				continue
			}

			aPoints, err := copyEdgePointsGuarded(aEdge, guard)
			if err != nil {
				return false, err
			}
			bPoints, err := copyEdgePointsGuarded(bEdge, guard)
			if err != nil {
				return false, err
			}

			// swap the first segment of the edges
			swapPoints(a1, b1)
			swapPoints(a2, b2)

			intersectionOffset := pathNodeProximityFloor / 2.
			// insert the intersection
			aIntersection := intersection.Copy()
			if aEdge.From == n {
				if err := insertEdgePointGuarded(aEdge, aIntersection, 2, guard); err != nil {
					return false, err
				}
			} else {
				if err := insertEdgePointGuarded(aEdge, aIntersection, len(aEdge.Points)-2, guard); err != nil {
					return false, err
				}
			}
			// shifts the intersection segment a bit so that it doesn't look like a crossing anymore
			if aIntersection.X == a2.X {
				aIntersection.X += intersectionOffset
				a2.X += intersectionOffset
			} else {
				aIntersection.Y -= intersectionOffset
				a2.Y -= intersectionOffset
			}

			// same as above, but for the other edge
			bIntersection := intersection.Copy()
			if bEdge.From == n {
				if err := insertEdgePointGuarded(bEdge, bIntersection, 2, guard); err != nil {
					return false, err
				}
			} else {
				if err := insertEdgePointGuarded(bEdge, bIntersection, len(bEdge.Points)-2, guard); err != nil {
					return false, err
				}
			}
			if bIntersection.X == b2.X {
				bIntersection.X += intersectionOffset
				b2.X += intersectionOffset
			} else {
				bIntersection.Y -= intersectionOffset
				b2.Y -= intersectionOffset
			}

			aImproved, err := refineEdgeGuarded(n.Graph, aEdge, guard)
			if err != nil {
				return false, err
			}
			bImproved, err := refineEdgeGuarded(n.Graph, bEdge, guard)
			if err != nil {
				return false, err
			}
			if !aImproved && !bImproved {
				// if none of the edges improved after splitting at the intersection
				// return back as we got 2 extra bends in the place of a single intersection
				aEdge.Points = aPoints
				bEdge.Points = bPoints
			} else {
				swapped = true
			}
		}
	}
	return swapped, nil
}
