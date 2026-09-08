package quality

import (
	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func countNonSharedCrossings(edges []*layoutgraph.Edge, guard *limits.WorkGuard) (int64, error) {
	var crossingCount int64
	for i, edge := range edges {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		for j := i + 1; j < len(edges); j++ {
			// Charge every pair even when one route has no segments. Iterating a
			// large collection of incomplete routes is still quadratic work.
			if err := guard.Step(); err != nil {
				return 0, err
			}
			count, err := countEdgeCrossings(edge, edges[j], guard)
			if err != nil {
				return 0, err
			}
			crossingCount += count
		}
	}
	return crossingCount, nil
}

func countEdgeCrossings(edge, other *layoutgraph.Edge, guard *limits.WorkGuard) (int64, error) {
	var crossingCount int64
	for i := 0; i < len(edge.Points)-1; i++ {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		for j := 0; j < len(other.Points)-1; j++ {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if isNonSharedCrossing(edge, other, i, j) {
				crossingCount++
			}
		}
	}
	return crossingCount, nil
}

func isNonSharedCrossing(edge, other *layoutgraph.Edge, edgeSegment, otherSegment int) bool {
	if !nonParallelIntersection(
		edge.Points[edgeSegment],
		edge.Points[edgeSegment+1],
		other.Points[otherSegment],
		other.Points[otherSegment+1],
	) {
		return false
	}

	// Skip intersections at the exact end of the segment to avoid counting
	// bends off shared edges.
	segmentStartX := edge.Points[edgeSegment].X
	segmentStartY := edge.Points[edgeSegment].Y
	if segmentStartX == edge.Points[edgeSegment+1].X &&
		(segmentStartX == other.Points[otherSegment].X || segmentStartX == other.Points[otherSegment+1].X) {
		return false
	}
	if segmentStartY == edge.Points[edgeSegment+1].Y &&
		(segmentStartY == other.Points[otherSegment].Y || segmentStartY == other.Points[otherSegment+1].Y) {
		return false
	}
	return true
}

// orientation returns a positive value for a clockwise turn, a negative value
// for a counter-clockwise turn, and zero for collinear points.
func orientation(p, q, r *geo.Point) float64 {
	pqX := q.X - p.X
	pqY := q.Y - p.Y
	prX := r.X - p.X
	prY := r.Y - p.Y
	return pqY*prX - pqX*prY
}

func equalSigns(a, b float64) bool {
	return a > 0 && b > 0 || a == 0 && b == 0 || a < 0 && b < 0
}

// nonParallelIntersection reports whether segments ab and cd intersect at an
// angle. Collinear overlap is intentionally not a crossing for layout scoring.
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
