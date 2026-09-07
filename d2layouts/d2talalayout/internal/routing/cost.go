package routing

import (
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// estimateRouteCost scores one candidate route against the routes that have
// already been selected. Crossing semantics belong to routing: intersections
// at the exact end of an axis-aligned segment are bends off a shared route,
// not route crossings.
func estimateRouteCost(edges layoutgraph.Edges, edge *layoutgraph.Edge) float64 {
	cost := edge.Length()
	cost *= math.Pow(turnPenalty, float64(len(edge.Points)-2))

	var crossingCount int64
	for _, otherEdge := range edges {
		if otherEdge == edge {
			continue
		}
		crossingCount += countNonSharedCrossings(edge, otherEdge)
	}
	cost += layoutgraph.CrossingCostWeight * float64(crossingCount)
	return cost
}

func countNonSharedCrossings(edge, otherEdge *layoutgraph.Edge) int64 {
	var crossingCount int64
	for edgeSegment := 0; edgeSegment < len(edge.Points)-1; edgeSegment++ {
		for otherSegment := 0; otherSegment < len(otherEdge.Points)-1; otherSegment++ {
			if isNonSharedCrossing(edge, otherEdge, edgeSegment, otherSegment) {
				crossingCount++
			}
		}
	}
	return crossingCount
}

func isNonSharedCrossing(edge, otherEdge *layoutgraph.Edge, edgeSegment, otherSegment int) bool {
	if !nonParallelIntersection(
		edge.Points[edgeSegment],
		edge.Points[edgeSegment+1],
		otherEdge.Points[otherSegment],
		otherEdge.Points[otherSegment+1],
	) {
		return false
	}

	// Skip intersections at the exact end of the segment to avoid counting
	// bends off shared edges.
	segmentStartX := edge.Points[edgeSegment].X
	segmentStartY := edge.Points[edgeSegment].Y
	if segmentStartX == edge.Points[edgeSegment+1].X &&
		(segmentStartX == otherEdge.Points[otherSegment].X || segmentStartX == otherEdge.Points[otherSegment+1].X) {
		return false
	}
	if segmentStartY == edge.Points[edgeSegment+1].Y &&
		(segmentStartY == otherEdge.Points[otherSegment].Y || segmentStartY == otherEdge.Points[otherSegment+1].Y) {
		return false
	}
	return true
}

func equalSigns(first, second float64) bool {
	return first > 0 && second > 0 || first == 0 && second == 0 || first < 0 && second < 0
}

// nonParallelIntersection reports whether segments ab and cd intersect at an
// angle. Collinear overlap is intentionally not a routing crossing.
func nonParallelIntersection(a, b, c, d *geo.Point) bool {
	abcOrientation := orientation(a, b, c)
	abdOrientation := orientation(a, b, d)
	if equalSigns(abcOrientation, abdOrientation) {
		return false
	}

	cdaOrientation := orientation(c, d, a)
	cdbOrientation := orientation(c, d, b)
	return !equalSigns(cdaOrientation, cdbOrientation)
}
