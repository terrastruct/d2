package routing

import (
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func estimateRouteCostGuarded(edges layoutgraph.Edges, edge *layoutgraph.Edge, guard workBudget) (float64, error) {
	length := 0.0
	for index := 0; index < len(edge.Points)-1; index++ {
		if err := guard.step(); err != nil {
			return 0, err
		}
		length += geo.EuclideanDistance(
			edge.Points[index].X,
			edge.Points[index].Y,
			edge.Points[index+1].X,
			edge.Points[index+1].Y,
		)
	}
	cost := length * math.Pow(turnPenalty, float64(len(edge.Points)-2))

	var crossingCount int64
	for _, otherEdge := range edges {
		if err := guard.step(); err != nil {
			return 0, err
		}
		if otherEdge == edge {
			continue
		}
		for edgeSegment := 0; edgeSegment < len(edge.Points)-1; edgeSegment++ {
			for otherSegment := 0; otherSegment < len(otherEdge.Points)-1; otherSegment++ {
				if err := guard.step(); err != nil {
					return 0, err
				}
				if isNonSharedCrossing(edge, otherEdge, edgeSegment, otherSegment) {
					crossingCount++
				}
			}
		}
	}
	cost += layoutgraph.CrossingCostWeight * float64(crossingCount)
	return cost, nil
}

func routeIntersectsNodeGuarded(nodes layoutgraph.Nodes, edge *layoutgraph.Edge, guard workBudget) (bool, error) {
	for _, otherNode := range nodes {
		if err := guard.step(); err != nil {
			return false, err
		}
		if edge.From.IsDescendantOf(otherNode) || edge.To.IsDescendantOf(otherNode) {
			continue
		}
		for index := 0; index < len(edge.Points)-1; index++ {
			if err := guard.step(); err != nil {
				return false, err
			}
			if otherNode.PassesThrough(edge.Points[index], edge.Points[index+1]) {
				return true, nil
			}
		}
	}
	return false, nil
}
