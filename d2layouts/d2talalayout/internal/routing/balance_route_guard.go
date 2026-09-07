package routing

import (
	"math"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func removeDuplicatePointsGuarded(points []*geo.Point, guard *routeWorkGuard) ([]*geo.Point, error) {
	if len(points) <= 1 {
		return points, guard.step()
	}

	result := make([]*geo.Point, 0, len(points))
	result = append(result, points[0])
	for index := 1; index < len(points); index++ {
		if err := guard.step(); err != nil {
			return nil, err
		}
		previous := points[index-1]
		current := points[index]
		if previous.X != current.X || previous.Y != current.Y {
			result = append(result, current)
		}
	}

	if len(result) == 1 {
		secondPoint := result[0].Copy()
		secondPoint.X++
		result = append(result, secondPoint)
	}
	return result, nil
}

func nodeSegmentsGuarded(nodes layoutgraph.Nodes, isHorizontal bool, guard *routeWorkGuard) ([]*geo.Segment, error) {
	out := make([]*geo.Segment, 0, len(nodes)*2)
	for _, node := range nodes {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if isHorizontal {
			out = append(out, &geo.Segment{Start: node.TopLeft, End: geo.NewPoint(node.TopLeft.X+node.Width, node.TopLeft.Y)})
			out = append(out, &geo.Segment{Start: geo.NewPoint(node.TopLeft.X, node.TopLeft.Y+node.Height), End: geo.NewPoint(node.TopLeft.X+node.Width, node.TopLeft.Y+node.Height)})
		} else {
			out = append(out, &geo.Segment{Start: node.TopLeft, End: geo.NewPoint(node.TopLeft.X, node.TopLeft.Y+node.Height)})
			out = append(out, &geo.Segment{Start: geo.NewPoint(node.TopLeft.X+node.Width, node.TopLeft.Y), End: geo.NewPoint(node.TopLeft.X+node.Width, node.TopLeft.Y+node.Height)})
		}
	}
	return out, nil
}

func edgeSegmentsGuarded(edges layoutgraph.Edges, isHorizontal bool, guard *routeWorkGuard) ([]*layoutgraph.EdgeSegment, error) {
	var out []*layoutgraph.EdgeSegment
	for _, edge := range edges {
		if err := guard.step(); err != nil {
			return nil, err
		}
		for index := 0; index < len(edge.Points)-1; index++ {
			if err := guard.step(); err != nil {
				return nil, err
			}
			var start, end *geo.Point
			if isHorizontal {
				if edge.Points[index].Y == edge.Points[index+1].Y {
					if edge.Points[index].X < edge.Points[index+1].X {
						start, end = edge.Points[index], edge.Points[index+1]
					} else {
						start, end = edge.Points[index+1], edge.Points[index]
					}
				}
			} else if edge.Points[index].X == edge.Points[index+1].X {
				if edge.Points[index].Y < edge.Points[index+1].Y {
					start, end = edge.Points[index], edge.Points[index+1]
				} else {
					start, end = edge.Points[index+1], edge.Points[index]
				}
			}
			if start != nil && end != nil {
				out = append(out, layoutgraph.NewEdgeSegment(start, end, edge))
			}
		}
	}
	return out, nil
}

// routeSegmentBounds is the cancellable equivalent of geo.Segment.GetBounds.
// Keep its comparisons byte-for-byte equivalent so successful layouts do not
// change while long locked-segment scans become interruptible.
func routeSegmentBounds(segment geo.Segment, segments []*geo.Segment, buffer float64, guard *routeWorkGuard) (float64, float64, error) {
	ceil := math.Inf(1)
	floor := math.Inf(-1)
	if segment.Start.X == segment.End.X && segment.Start.Y == segment.End.Y {
		return floor, ceil, guard.step()
	}
	isHorizontal := segment.Start.X == segment.End.X
	for _, otherSegment := range segments {
		if err := guard.step(); err != nil {
			return 0, 0, err
		}
		if isHorizontal {
			if otherSegment.End.Y < segment.Start.Y-buffer {
				continue
			}
			if otherSegment.Start.Y > segment.End.Y+buffer {
				continue
			}
			if otherSegment.Start.X <= segment.Start.X {
				floor = math.Max(floor, otherSegment.Start.X)
			}
			if otherSegment.Start.X > segment.Start.X {
				ceil = math.Min(ceil, otherSegment.Start.X)
			}
		} else {
			if otherSegment.End.X < segment.Start.X-buffer {
				continue
			}
			if otherSegment.Start.X > segment.End.X+buffer {
				continue
			}
			if otherSegment.Start.Y <= segment.Start.Y {
				floor = math.Max(floor, otherSegment.Start.Y)
			}
			if otherSegment.Start.Y > segment.Start.Y {
				ceil = math.Min(ceil, otherSegment.Start.Y)
			}
		}
	}
	return floor, ceil, nil
}

func evenlyDistributeGuarded(floor, ceil float64, count int, guard *routeWorkGuard) ([]float64, error) {
	if count <= 0 || floor >= ceil {
		return []float64{}, guard.step()
	}
	increment := math.Floor((ceil - floor) / float64(count+1))
	if increment <= 0 {
		return []float64{}, guard.step()
	}
	out := make([]float64, 0, count)
	for index := 1; index <= count; index++ {
		if err := guard.step(); err != nil {
			return nil, err
		}
		out = append(out, floor+float64(index)*increment)
	}
	return out, nil
}

type balanceOrderStatus int

const (
	balanceOrderPreserved balanceOrderStatus = iota
	balanceOrderReversed
	balanceOrderContactChanged
)

// checkBalanceOrder checks a batch against parallel routes that
// belong to another movement range. Such routes are not locked yet when the
// narrowest range is balanced, but moving past one can drag an attached elbow
// across it. Sorting within each batch alone cannot preserve this ordering.
//
// Check every member, including partial shared trunks, before changing any
// coordinates. Rejecting the whole proposal preserves shared routes; the caller
// still locks the unchanged batch so balancing always makes progress.
func checkBalanceOrder(batch []*layoutgraph.EdgeSegment, batchSet map[*layoutgraph.EdgeSegment]bool, segments []*layoutgraph.EdgeSegment, proposed []float64, isHorizontal bool, guard *routeWorkGuard) (balanceOrderStatus, error) {
	moving := false
	for i, segment := range batch {
		if err := guard.step(); err != nil {
			return balanceOrderPreserved, err
		}
		old := segment.Start.Y
		if isHorizontal {
			old = segment.Start.X
		}
		if proposed[i] != old {
			moving = true
			break
		}
	}
	if !moving {
		return balanceOrderPreserved, nil
	}
	status := balanceOrderPreserved
	// Skip batch members before scanning proposals. A parallel bundle already
	// balanced together needs only a linear scan when no outside routes exist.
	for _, other := range segments {
		if err := guard.step(); err != nil {
			return balanceOrderPreserved, err
		}
		if batchSet[other] {
			continue
		}
		for i, segment := range batch {
			if err := guard.step(); err != nil {
				return balanceOrderPreserved, err
			}
			// Segments of one route may meet while eliminating an elbow.
			// Preserve ordering between distinct routes, not within a route.
			if segment.Owner() == other.Owner() {
				continue
			}
			old, position := segment.Start.Y, other.Start.Y
			start, end := segment.Start.X, segment.End.X
			otherStart, otherEnd := other.Start.X, other.End.X
			if isHorizontal {
				old, position = segment.Start.X, other.Start.X
				start, end = segment.Start.Y, segment.End.Y
				otherStart, otherEnd = other.Start.Y, other.End.Y
			}
			if proposed[i] == old {
				continue
			}
			// Closed intervals matter: an elbow at the end of either segment
			// moves with it, so touching intervals can also acquire a crossing.
			if math.Max(start, end) < math.Min(otherStart, otherEnd) || math.Max(otherStart, otherEnd) < math.Min(start, end) {
				continue
			}
			if old == position || proposed[i] == position {
				// Creating a shared coordinate or splitting one is not a
				// crossing improvement: crossing counts omit collinear contact.
				return balanceOrderContactChanged, nil
			}
			if (old < position) != (proposed[i] < position) {
				status = balanceOrderReversed
				// Keep checking: a later relation may prohibit fallback.
			}
		}
	}
	return status, nil
}
