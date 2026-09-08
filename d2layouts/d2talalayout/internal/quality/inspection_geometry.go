package quality

import (
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

// measureGeometry counts occupied rectangular interiors. Container containment
// is intentional, and an edge may cross a boundary enclosing either endpoint.
// Curved routes and nonrectangular silhouettes require renderer geometry and are
// deliberately not treated as invalid based only on their bounding boxes.
func measureGeometry(g *layoutgraph.Graph, score *Metrics, guard *limits.WorkGuard) error {
	for i, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if node.IsInvisible || !node.IsRectangular() {
			continue
		}
		for _, other := range g.Nodes[i+1:] {
			if err := guard.Step(); err != nil {
				return err
			}
			if other.IsInvisible || !other.IsRectangular() || !boxesOverlapWithPadding(node.Box, other.Box, 0) {
				continue
			}
			related, err := relatedNodes(node, other, guard)
			if err != nil {
				return err
			}
			if !related {
				score.NodeOverlaps++
			}
		}
		for _, edge := range g.Edges {
			if err := guard.Step(); err != nil {
				return err
			}
			if edge.IsInvisible || edge.IsCurve {
				continue
			}
			ancestor, err := endpointAncestor(edge, node, guard)
			if err != nil {
				return err
			}
			if ancestor && edge.From != node && edge.To != node {
				continue
			}
			for i := 1; i < len(edge.Points); i++ {
				if err := guard.Step(); err != nil {
					return err
				}
				// The first/last approach may begin inside a shape before
				// border tracing. Later reentry into an endpoint is an obstacle.
				if i == 1 && edge.From == node || i == len(edge.Points)-1 && edge.To == node {
					continue
				}
				if segmentEntersBox(node.Box, edge.Points[i-1], edge.Points[i]) {
					score.RouteObstructions++
					break // One obstruction per edge/node, independent of subdivision.
				}
			}
		}
	}
	return nil
}

func relatedNodes(a, b *layoutgraph.Node, guard *limits.WorkGuard) (bool, error) {
	yes, err := evaluationIsDescendantOf(a, b, guard)
	if yes || err != nil {
		return yes, err
	}
	return evaluationIsDescendantOf(b, a, guard)
}

func endpointAncestor(edge *layoutgraph.Edge, node *layoutgraph.Node, guard *limits.WorkGuard) (bool, error) {
	yes, err := evaluationIsDescendantOf(edge.From, node, guard)
	if yes || err != nil {
		return yes, err
	}
	return evaluationIsDescendantOf(edge.To, node, guard)
}

// An open-rectangle slab intersection: touching or following a border does not
// occlude the interior. This also handles diagonal segments without sampling.
func segmentEntersBox(box geo.Box, a, b *geo.Point) bool {
	if box.Width <= 0 || box.Height <= 0 || a.X == b.X && a.Y == b.Y {
		return false
	}
	low, high := 0.0, 1.0
	for _, axis := range [][4]float64{
		{a.X, b.X - a.X, box.TopLeft.X, box.TopLeft.X + box.Width},
		{a.Y, b.Y - a.Y, box.TopLeft.Y, box.TopLeft.Y + box.Height},
	} {
		start, delta, minValue, maxValue := axis[0], axis[1], axis[2], axis[3]
		if delta == 0 {
			if start <= minValue || start >= maxValue {
				return false
			}
			continue
		}
		first, last := (minValue-start)/delta, (maxValue-start)/delta
		if first > last {
			first, last = last, first
		}
		low, high = math.Max(low, first), math.Min(high, last)
		if low >= high {
			return false
		}
	}
	return low < high
}
