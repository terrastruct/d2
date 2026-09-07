package routing

import (
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// balanceReversalRemovesCrossings permits an otherwise rejected reversal only
// when it removes crossings without increasing them for any affected route
// pair. This preserves useful uncrossing of routes that entered balancing in
// the wrong order. It is a fallback, not a second scorer for every batch.
//
// Proposed point copies also move adjacent legs and respect pointer aliases.
// The graph stays untouched until the caller accepts the entire batch.
func balanceReversalRemovesCrossings(g *layoutgraph.Graph, batch []*layoutgraph.EdgeSegment, proposed []float64, isHorizontal bool, guard *routeWorkGuard) (bool, error) {
	pointCopies := make(map[*geo.Point]*geo.Point)
	for i, segment := range batch {
		if err := guard.step(); err != nil {
			return false, err
		}
		for _, point := range [2]*geo.Point{segment.Start, segment.End} {
			if err := guard.step(); err != nil {
				return false, err
			}
			candidate := *point
			if isHorizontal {
				candidate.X = proposed[i]
			} else {
				candidate.Y = proposed[i]
			}
			if previous, exists := pointCopies[point]; exists && *previous != candidate {
				return false, nil
			}
			pointCopies[point] = &candidate
		}
	}

	if err := guard.add(uint64(len(g.Edges))); err != nil {
		return false, err
	}
	candidates := make([]*layoutgraph.Edge, len(g.Edges))
	var affected []int
	for i, edge := range g.Edges {
		if err := guard.step(); err != nil {
			return false, err
		}
		changed := false
		for _, point := range edge.Points {
			if err := guard.step(); err != nil {
				return false, err
			}
			if candidate, exists := pointCopies[point]; exists && *candidate != *point {
				changed = true
			}
		}
		if !changed {
			continue
		}
		if edge.IsCurve {
			return false, nil
		}
		if err := guard.add(uint64(len(edge.Points))); err != nil {
			return false, err
		}
		candidate := *edge
		candidate.Points = make([]*geo.Point, len(edge.Points))
		for j, point := range edge.Points {
			if err := guard.step(); err != nil {
				return false, err
			}
			candidate.Points[j] = point
			if replacement, exists := pointCopies[point]; exists {
				candidate.Points[j] = replacement
			}
			if j > 0 {
				before, after := edge.Points[j-1], candidate.Points[j-1]
				if before.X != point.X && before.Y != point.Y || after.X != candidate.Points[j].X && after.Y != candidate.Points[j].Y {
					return false, nil
				}
			}
		}
		candidates[i] = &candidate
		affected = append(affected, i)
	}

	improved := false
	for _, i := range affected {
		if err := guard.step(); err != nil {
			return false, err
		}
		for j, other := range g.Edges {
			if err := guard.step(); err != nil {
				return false, err
			}
			if i == j || candidates[j] != nil && j < i {
				continue
			}
			// A curve's control polygon cannot prove its rendered crossings.
			if other.IsCurve {
				return false, nil
			}
			otherCandidate := candidates[j]
			if otherCandidate == nil {
				otherCandidate = other
			}
			before, after := 0, 0
			for a := 0; a+1 < len(g.Edges[i].Points); a++ {
				if err := guard.step(); err != nil {
					return false, err
				}
				for b := 0; b+1 < len(other.Points); b++ {
					if err := guard.step(); err != nil {
						return false, err
					}
					if isNonSharedCrossing(g.Edges[i], other, a, b) {
						before++
					}
					if isNonSharedCrossing(candidates[i], otherCandidate, a, b) {
						after++
					}
					// Dragged adjacent legs must not gain a new shared run.
					// Existing shared trunks may extend as a crossing is removed.
					oldOverlap := balanceCollinearOverlap(g.Edges[i].Points[a], g.Edges[i].Points[a+1], other.Points[b], other.Points[b+1])
					newOverlap := balanceCollinearOverlap(candidates[i].Points[a], candidates[i].Points[a+1], otherCandidate.Points[b], otherCandidate.Points[b+1])
					if oldOverlap == 0 && newOverlap > 0 {
						return false, nil
					}
				}
			}
			if after > before {
				return false, nil
			}
			improved = improved || after < before
		}
	}
	return improved, nil
}

func balanceCollinearOverlap(a, b, c, d *geo.Point) float64 {
	if a.X == b.X && c.X == d.X && a.X == c.X {
		return math.Max(0, math.Min(math.Max(a.Y, b.Y), math.Max(c.Y, d.Y))-math.Max(math.Min(a.Y, b.Y), math.Min(c.Y, d.Y)))
	}
	if a.Y == b.Y && c.Y == d.Y && a.Y == c.Y {
		return math.Max(0, math.Min(math.Max(a.X, b.X), math.Max(c.X, d.X))-math.Max(math.Min(a.X, b.X), math.Min(c.X, d.X)))
	}
	return 0
}
