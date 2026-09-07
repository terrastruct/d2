package routing

import (
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/quality"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func hasFixedBalancingPorts(edge *layoutgraph.Edge) bool {
	return edge.HasTableColumn() || edge.From.Shape.GetType() == shape.DIAMOND_TYPE || edge.To.Shape.GetType() == shape.DIAMOND_TYPE
}

// balancePortInteriorsGuarded gives fixed-port routes a separate conservative
// balancing pass. Other routes, endpoints, and their approach segments act as
// immutable corridor boundaries. Reject the whole tentative batch if moving
// connected bends creates an obstruction, crossing, or substantial detour.
func balancePortInteriorsGuarded(g *layoutgraph.Graph, guard *routeWorkGuard) error {
	var selected, locked []*layoutgraph.Edge
	for _, e := range g.Edges {
		if err := guard.step(); err != nil {
			return err
		}
		_, fromTree := g.NodeToTree[e.From]
		_, toTree := g.NodeToTree[e.To]
		if hasFixedBalancingPorts(e) && !e.IsCurve && !e.IsLoop() && !fromTree && !toTree && len(e.Points) >= 6 {
			selected = append(selected, e)
		} else {
			locked = append(locked, e)
		}
	}
	if len(selected) == 0 {
		return nil
	}
	beforeMetrics, err := quality.Inspect(guard.ctx, g)
	if err != nil {
		return err
	}
	snapshot, err := captureRouteMutations(g, nil, guard)
	if err != nil {
		return err
	}
	before, err := copyRoutePoints(selected, guard)
	if err != nil {
		return err
	}
	if err := balanceRegularEdgesGuarded(g, locked, selected, guard); err != nil {
		return err
	}
	for _, e := range selected {
		valid, err := changedRouteIsClear(g, e, before[e], true, guard)
		if err != nil {
			return err
		}
		if !valid {
			snapshot.restore()
			return nil
		}
	}
	afterMetrics, err := quality.Inspect(guard.ctx, g)
	if err != nil {
		return err
	}
	if afterMetrics.RouteObstructions > beforeMetrics.RouteObstructions || afterMetrics.Crossings > beforeMetrics.Crossings ||
		afterMetrics.TextOcclusions > beforeMetrics.TextOcclusions || afterMetrics.RouteLength > beforeMetrics.RouteLength*1.1 {
		snapshot.restore()
	}
	return nil
}

func copyRoutePoints(edges []*layoutgraph.Edge, guard *routeWorkGuard) (map[*layoutgraph.Edge][]*geo.Point, error) {
	result := make(map[*layoutgraph.Edge][]*geo.Point, len(edges))
	for _, e := range edges {
		if err := guard.add(uint64(len(e.Points)) + 1); err != nil {
			return nil, err
		}
		points := make([]*geo.Point, len(e.Points))
		for i, p := range e.Points {
			v := *p
			points[i] = &v
		}
		result[e] = points
	}
	return result, nil
}

func changedRouteIsClear(g *layoutgraph.Graph, e *layoutgraph.Edge, before []*geo.Point, fixedPorts bool, guard *routeWorkGuard) (bool, error) {
	if len(e.Points) < 2 || len(before) < 2 {
		return false, nil
	}
	if fixedPorts && (*e.Points[0] != *before[0] || *e.Points[len(e.Points)-1] != *before[len(before)-1] ||
		!sameRouteDirection(before[0], before[1], e.Points[0], e.Points[1]) ||
		!sameRouteDirection(before[len(before)-2], before[len(before)-1], e.Points[len(e.Points)-2], e.Points[len(e.Points)-1])) {
		return false, nil
	}
	for i := 1; i < len(e.Points); i++ {
		a, b := e.Points[i-1], e.Points[i]
		unchanged := false
		for j := 1; j < len(before); j++ {
			if err := guard.step(); err != nil {
				return false, err
			}
			if *a == *before[j-1] && *b == *before[j] {
				unchanged = true
				break
			}
		}
		if unchanged {
			continue
		}
		if a.X != b.X && a.Y != b.Y {
			return false, nil
		}
		blocked, err := lineIntersectsUnrelatedNode(g, e, a, b, guard)
		if err != nil {
			return false, err
		}
		if blocked {
			return false, nil
		}
	}
	return true, nil
}
