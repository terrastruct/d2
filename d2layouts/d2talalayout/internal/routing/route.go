package routing

import (
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type Route struct {
	GEdge    *layoutgraph.Edge
	OVGNodes []*OVGNode
	FromPort geo.Point
	ToPort   geo.Point
}

func (r *Route) createSegmentEndpoints() []*geo.Point {
	points := make([]*geo.Point, 0)

	points = append(points, r.OVGNodes[1].Point.Copy())
	for i := 2; i < len(r.OVGNodes)-2; i++ {
		lastPoint := r.OVGNodes[i-1].Point
		currentPoint := r.OVGNodes[i].Point
		nextPoint := r.OVGNodes[i+1].Point
		if (currentPoint.X == lastPoint.X) && (currentPoint.X != nextPoint.X) {
			points = append(points, currentPoint.Copy())
		} else if (currentPoint.Y == lastPoint.Y) && (currentPoint.Y != nextPoint.Y) {
			points = append(points, currentPoint.Copy())
		}
	}
	points = append(points, r.OVGNodes[len(r.OVGNodes)-2].Point.Copy())
	return points
}

// The entire route is colinear
func (r *Route) isEntireColinear(from, to *OVGNode) bool {
	if from.Y == to.Y {
		y := from.Y
		minX, maxX := from.X, to.X
		if maxX < minX {
			minX, maxX = maxX, minX
		}
		for i := 1; i < len(r.OVGNodes)-2; i++ {
			rFrom, rTo := r.OVGNodes[i], r.OVGNodes[i+1]
			if rFrom.Y != y || rTo.Y != y {
				return false
			}
			rMinX, rMaxX := rFrom.X, rTo.X
			if rMaxX < rMinX {
				rMinX, rMaxX = rMaxX, rMinX
			}
			if rMinX < minX || rMaxX > maxX {
				return false
			}
		}
		return true
	}

	if from.X == to.X {
		x := from.X
		minY, maxY := from.Y, to.Y
		if maxY < minY {
			minY, maxY = maxY, minY
		}
		for i := 1; i < len(r.OVGNodes)-2; i++ {
			rFrom, rTo := r.OVGNodes[i], r.OVGNodes[i+1]
			if rFrom.X != x || rTo.X != x {
				return false
			}
			rMinY, rMaxY := rFrom.Y, rTo.Y
			if rMaxY < rMinY {
				rMinY, rMaxY = rMaxY, rMinY
			}
			if rMinY < minY || rMaxY > maxY {
				return false
			}
		}
		return true
	}

	return false
}

// isOpposingColinear returns true if the segment <from, to> spans colinearly in the opposite direction to any segment of the route
// This only applies to routes which are directed, otherwise there is no notion of opposite
// If the segments share a single endpoint, that doesn't count
// TODO: allow bidirectional edges with different arrowheads to overlap in the
// same direction; see edgeCanOverlapEdges.
func (r *Route) isOpposingColinear(from, to *OVGNode) bool {
	if !r.GEdge.IsDirected() {
		return false
	}
	if from.Y == to.Y {
		y := from.Y
		minX, maxX := from.X, to.X
		if maxX < minX {
			minX, maxX = maxX, minX
		}
		for i := 0; i < len(r.OVGNodes)-2; i++ {
			if r.OVGNodes[i+1].Y != y {
				i++
				continue
			}
			if r.OVGNodes[i].Y != y {
				continue
			}
			rFrom, rTo := r.OVGNodes[i], r.OVGNodes[i+1]
			if rTo.X != rFrom.X {
				if rFrom.X < rTo.X {
					if maxX < rFrom.X || rTo.X < minX {
						continue
					}
				} else {
					if maxX < rTo.X || rFrom.X < minX {
						continue
					}
				}

				rMinX, rMaxX := rFrom.X, rTo.X
				if rMaxX < rMinX {
					rMinX, rMaxX = rMaxX, rMinX
				}

				if rMinX <= from.X && from.X <= rMaxX && (to.X < rMinX || rMaxX < to.X) {
					if nonNilEquals(from.Point, rFrom.Point) || nonNilEquals(from.Point, rTo.Point) {
						return false
					}
				} else if rMinX <= to.X && to.X <= rMaxX && (from.X < rMinX || rMaxX < from.X) {
					if nonNilEquals(to.Point, rFrom.Point) || nonNilEquals(to.Point, rTo.Point) {
						return false
					}
				}

				if (rTo.X > rFrom.X) && (to.X < from.X) {
					return true
				} else if (rTo.X < rFrom.X) && (to.X > from.X) {
					return true
				}
			}
		}
	} else if from.X == to.X {
		x := from.X
		minY, maxY := from.Y, to.Y
		if maxY < minY {
			minY, maxY = maxY, minY
		}
		for i := 0; i < len(r.OVGNodes)-2; i++ {
			if r.OVGNodes[i+1].X != x {
				i++
				continue
			}
			if r.OVGNodes[i].X != x {
				continue
			}
			rFrom, rTo := r.OVGNodes[i], r.OVGNodes[i+1]

			if rTo.Y != rFrom.Y {
				if rFrom.Y < rTo.Y {
					if maxY < rFrom.Y || rTo.Y < minY {
						continue
					}
				} else {
					if maxY < rTo.Y || rFrom.Y < minY {
						continue
					}
				}

				rMinY, rMaxY := rFrom.Y, rTo.Y
				if rMaxY < rMinY {
					rMinY, rMaxY = rMaxY, rMinY
				}

				if rMinY <= from.Y && from.Y <= rMaxY && (to.Y < rMinY || rMaxY < to.Y) {
					if nonNilEquals(from.Point, rFrom.Point) || nonNilEquals(from.Point, rTo.Point) {
						return false
					}
				} else if rMinY <= to.Y && to.Y <= rMaxY && (from.Y < rMinY || rMaxY < from.Y) {
					if nonNilEquals(to.Point, rFrom.Point) || nonNilEquals(to.Point, rTo.Point) {
						return false
					}
				}

				if (rTo.Y > rFrom.Y) && (to.Y < from.Y) {
					return true
				} else if (rTo.Y < rFrom.Y) && (to.Y > from.Y) {
					return true
				}
			}
		}
	}

	return false
}

// A route can swap edges with another route if:
// - The two endpoints are the same nodes
// - No other edges are using the ports
//   - TODO can iterate on this to allow some instances
//
// - The two endpoints is not diagonal
// - The ports that the two routes are connected to on each node are on the same side
// TODO consider min distance for labels
// Not self-route
func (r *Route) canSwapEdgesGuarded(r2 *Route, allRoutes []*Route, guard workBudget) (bool, error) {
	if r.GEdge.From == r.GEdge.To {
		return false, nil
	}
	if r2.GEdge.From == r2.GEdge.To {
		return false, nil
	}
	if !((r.GEdge.From == r2.GEdge.From && r.GEdge.To == r2.GEdge.To) ||
		(r.GEdge.From == r2.GEdge.To && r.GEdge.To == r2.GEdge.From)) {
		return false, nil
	}

	if r.GEdge.From.Orientation(r.GEdge.To).IsDiagonal() {
		return false, nil
	}

	if r.GEdge.From == r2.GEdge.From && r.GEdge.To == r2.GEdge.To {
		if r.GEdge.From.Orientation(r.GEdge.To).IsVertical() {
			if r.FromPort.Y != r2.FromPort.Y {
				return false, nil
			}
			if r.ToPort.Y != r2.ToPort.Y {
				return false, nil
			}
		}
		if r.GEdge.From.Orientation(r.GEdge.To).IsHorizontal() {
			if r.FromPort.X != r2.FromPort.X {
				return false, nil
			}
			if r.ToPort.X != r2.ToPort.X {
				return false, nil
			}
		}
	}

	for _, r3 := range allRoutes {
		if guard != nil {
			if err := guard.step(); err != nil {
				return false, err
			}
		}
		if r == r3 || r2 == r3 {
			continue
		}
		if r.FromPort == r3.FromPort || r.FromPort == r3.ToPort {
			return false, nil
		}
		if r.ToPort == r3.FromPort || r.ToPort == r3.ToPort {
			return false, nil
		}
		if r2.FromPort == r3.FromPort || r2.FromPort == r3.ToPort {
			return false, nil
		}
		if r2.ToPort == r3.FromPort || r2.ToPort == r3.ToPort {
			return false, nil
		}
	}

	return true, nil
}
