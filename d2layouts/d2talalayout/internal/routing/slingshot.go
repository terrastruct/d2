package routing

// Slingshot is an optimization to attempt a rule-based routing before falling back to BFS search
// gravitational slingshot: https://en.wikipedia.org/wiki/Gravity_assist
//
// TERMINOLOGY:
// ============
//
//   - Vertical Launch:
//     In any diagonal orientation, there are two L shapes
//     A "vertical launch" is the one that starts vertically, so in the TopLeft orientation below, describes route 1
//
//   - Anchor:
//     The anchor is the OVG node used to turn/slingshot, "a" below
//
// . ┌────────┐
// . │        │     2
// . │        ├─────────┐
// . │   s    │         │
// . │        │         │
// . └───┬────┘         │
// .     │              │
// .     │              ▼
// .   1 │          ┌────────┐
// .     │          │        │
// .     │          │        │
// .     a─────────►│   t    │
// .                │        │
// .                └────────┘

import (
	"context"
	"math"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

// launch simulates the route blasting off from source, going around the anchor, and landing on target
// It returns the distance if successful, with 0.0 meaning it crashed into a node along the way
func (router *ovgEdgeRouter) launch(source, anchor, target *OVGNode, gEdge *layoutgraph.Edge) (float64, error) {
	nodes := layoutgraph.Nodes(router.ovg.NodesInsideBoundingBox)

	gEdge.Points = []*geo.Point{source.Point, anchor.Point}
	intersects, err := routeIntersectsNodeGuarded(nodes, gEdge, router.work)
	if err != nil {
		return 0, err
	}
	if intersects {
		return 0.0, nil
	}

	gEdge.Points = []*geo.Point{target.Point, anchor.Point}
	intersects, err = routeIntersectsNodeGuarded(nodes, gEdge, router.work)
	if err != nil {
		return 0, err
	}
	if intersects {
		return 0.0, nil
	}

	d := geo.EuclideanDistance(
		source.X,
		source.Y,
		anchor.X,
		anchor.Y,
	) + geo.EuclideanDistance(
		target.X,
		target.Y,
		anchor.X,
		anchor.Y,
	)

	return d, nil
}

func (router *ovgEdgeRouter) findLaunchings(isVerticalLaunch bool, orientation geo.Orientation, gSource *layoutgraph.Node) []*OVGNode {
	ovg := router.ovg

	direction := geo.NONE
	switch orientation {
	case geo.TopLeft:
		if isVerticalLaunch {
			direction = geo.Bottom
		} else {
			direction = geo.Right
		}
	case geo.TopRight:
		if isVerticalLaunch {
			direction = geo.Bottom
		} else {
			direction = geo.Left
		}
	case geo.BottomLeft:
		if isVerticalLaunch {
			direction = geo.Top
		} else {
			direction = geo.Right
		}
	case geo.BottomRight:
		if isVerticalLaunch {
			direction = geo.Top
		} else {
			direction = geo.Left
		}
	}
	if direction == geo.NONE {
		return nil
	}

	var out []*OVGNode
	seen := make(map[*OVGNode]struct{}, len(ovg.Ports[gSource]))
	for _, port := range ovg.Ports[gSource] {
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		if port.hasPortDirection(gSource, direction) {
			out = append(out, port)
		}
	}
	return out
}

func (router *ovgEdgeRouter) findLandings(isVerticalLaunch bool, orientation geo.Orientation, gTarget *layoutgraph.Node) []*OVGNode {
	ovg := router.ovg

	direction := geo.NONE
	switch orientation {
	case geo.TopLeft:
		if isVerticalLaunch {
			direction = geo.Left
		} else {
			direction = geo.Top
		}
	case geo.TopRight:
		if isVerticalLaunch {
			direction = geo.Right
		} else {
			direction = geo.Top
		}
	case geo.BottomLeft:
		if isVerticalLaunch {
			direction = geo.Left
		} else {
			direction = geo.Bottom
		}
	case geo.BottomRight:
		if isVerticalLaunch {
			direction = geo.Right
		} else {
			direction = geo.Bottom
		}
	}
	if direction == geo.NONE {
		return nil
	}

	var out []*OVGNode
	seen := make(map[*OVGNode]struct{}, len(ovg.Ports[gTarget]))
	for _, port := range ovg.Ports[gTarget] {
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		if port.hasPortDirection(gTarget, direction) {
			out = append(out, port)
		}
	}
	return out
}

func undershot(n *OVGNode, target *layoutgraph.Node, orientation geo.Orientation, isVerticalLaunch bool) bool {
	switch orientation {
	case geo.TopLeft:
		if isVerticalLaunch && n.Y < target.TopLeft.Y {
			return true
		} else if !isVerticalLaunch && n.X < target.TopLeft.X {
			return true
		}
	case geo.TopRight:
		if isVerticalLaunch && n.Y < target.TopLeft.Y {
			return true
		} else if !isVerticalLaunch && n.X > target.TopLeft.X+target.Width {
			return true
		}
	case geo.BottomLeft:
		if isVerticalLaunch && n.Y > target.TopLeft.Y+target.Height {
			return true
		} else if !isVerticalLaunch && n.X < target.TopLeft.X {
			return true
		}
	case geo.BottomRight:
		if isVerticalLaunch && n.Y > target.TopLeft.Y+target.Height {
			return true
		} else if !isVerticalLaunch && n.X > target.TopLeft.X+target.Width {
			return true
		}
	}
	return false
}

// overshot returns true if n is past the target, given the orientation of the source.
// E.g., if the arrow is here, there is no need to keep searching for L-shape or S-shape routes
// . ┌───┐
// . │ s │
// . └─┬─┘
// .   │
// .   │   ┌───┐
// .   │   │ t │
// .   │   └───┘
// .   ▼
//
// L routes can keep going until n is completely past t
// S routes are overshot when n reaches any part of t
func overshot(n *OVGNode, target *layoutgraph.Node, orientation geo.Orientation, isVerticalLaunch, forLRoute bool) bool {
	switch orientation {
	case geo.TopLeft:
		if isVerticalLaunch {
			if forLRoute && n.Y > target.TopLeft.Y+target.Height {
				return true
			}
			if !forLRoute && n.Y > target.TopLeft.Y {
				return true
			}
		} else {
			if forLRoute && n.X > target.TopLeft.X+target.Width {
				return true
			}
			if !forLRoute && n.X > target.TopLeft.X {
				return true
			}
		}
	case geo.TopRight:
		if isVerticalLaunch {
			if forLRoute && n.Y > target.TopLeft.Y+target.Height {
				return true
			}
			if !forLRoute && n.Y > target.TopLeft.Y {
				return true
			}
		} else {
			if forLRoute && n.X < target.TopLeft.X {
				return true
			}
			if !forLRoute && n.X < target.TopLeft.X+target.Width {
				return true
			}
		}
	case geo.BottomLeft:
		if isVerticalLaunch {
			if forLRoute && n.Y < target.TopLeft.Y {
				return true
			}
			if !forLRoute && n.Y < target.TopLeft.Y+target.Height {
				return true
			}
		} else {
			if forLRoute && n.X > target.TopLeft.X+target.Width {
				return true
			}
			if !forLRoute && n.X > target.TopLeft.X {
				return true
			}
		}
	case geo.BottomRight:
		if isVerticalLaunch {
			if forLRoute && n.Y < target.TopLeft.Y {
				return true
			}
			if !forLRoute && n.Y < target.TopLeft.Y+target.Height {
				return true
			}
		} else {
			if forLRoute && n.X < target.TopLeft.X {
				return true
			}
			if !forLRoute && n.X < target.TopLeft.X+target.Width {
				return true
			}
		}
	}
	return false
}

func isOnFlightPlan(prev, next *OVGNode, orientation geo.Orientation, isVerticalLaunch bool) bool {
	switch orientation {
	case geo.TopLeft:
		if isVerticalLaunch && next.X == prev.X && next.Y > prev.Y {
			return true
		} else if !isVerticalLaunch && next.Y == prev.Y && next.X > prev.X {
			return true
		}
	case geo.TopRight:
		if isVerticalLaunch && next.X == prev.X && next.Y > prev.Y {
			return true
		} else if !isVerticalLaunch && next.Y == prev.Y && next.X < prev.X {
			return true
		}
	case geo.BottomLeft:
		if isVerticalLaunch && next.X == prev.X && next.Y < prev.Y {
			return true
		} else if !isVerticalLaunch && next.Y == prev.Y && next.X > prev.X {
			return true
		}
	case geo.BottomRight:
		if isVerticalLaunch && next.X == prev.X && next.Y < prev.Y {
			return true
		} else if !isVerticalLaunch && next.Y == prev.Y && next.X < prev.X {
			return true
		}
	}
	return false
}

// If s and t exist in a vacuum, the choice of 1 and 2 is indistinguishable
// But consider the x's
// If s and t are connected to those x's, route 1 likely impedes on their routes, while route 2 likely won't
// So routes which are less likely to take up the routing space of other nodes are preferred
//
//	┌────────┐
//	│        │     2
//	│        ├─────────┐
//	│   s    │         │
//	│        │         │
//	└───┬────┘         │
//	    │              │
//
// x     │              ▼
//
//	   1 │          ┌────────┐
//	     │          │        │
//	x    │          │        │
//	     └─────────►│   t    │
//	                │        │
//	                └────────┘
func preferLaunchingVertically(gSource, gTarget *layoutgraph.Node, orientation geo.Orientation) (preferred bool, isStrong bool) {
	numInVerticalLaunchSpace := 0
	numInHorizontalLaunchSpace := 0

	var verticalSpaceSourceOrientations []geo.Orientation
	var verticalSpaceTargetOrientations []geo.Orientation
	var horizontalSpaceSourceOrientations []geo.Orientation
	var horizontalSpaceTargetOrientations []geo.Orientation

	switch orientation {
	case geo.TopLeft:
		verticalSpaceSourceOrientations = []geo.Orientation{geo.BottomLeft, geo.Bottom}
		verticalSpaceTargetOrientations = []geo.Orientation{geo.Left, geo.BottomLeft}
		horizontalSpaceSourceOrientations = []geo.Orientation{geo.TopRight, geo.Right}
		horizontalSpaceTargetOrientations = []geo.Orientation{geo.Top, geo.TopRight}
	case geo.TopRight:
		verticalSpaceSourceOrientations = []geo.Orientation{geo.BottomRight, geo.Bottom}
		verticalSpaceTargetOrientations = []geo.Orientation{geo.Right, geo.BottomRight}
		horizontalSpaceSourceOrientations = []geo.Orientation{geo.TopLeft, geo.Left}
		horizontalSpaceTargetOrientations = []geo.Orientation{geo.Top, geo.TopLeft}
	case geo.BottomLeft:
		verticalSpaceSourceOrientations = []geo.Orientation{geo.TopLeft, geo.Top}
		verticalSpaceTargetOrientations = []geo.Orientation{geo.Left, geo.TopLeft}
		horizontalSpaceSourceOrientations = []geo.Orientation{geo.BottomRight, geo.Right}
		horizontalSpaceTargetOrientations = []geo.Orientation{geo.Bottom, geo.BottomRight}
	case geo.BottomRight:
		verticalSpaceSourceOrientations = []geo.Orientation{geo.TopRight, geo.Top}
		verticalSpaceTargetOrientations = []geo.Orientation{geo.Right, geo.TopRight}
		horizontalSpaceSourceOrientations = []geo.Orientation{geo.BottomLeft, geo.Left}
		horizontalSpaceTargetOrientations = []geo.Orientation{geo.Bottom, geo.BottomLeft}
	}

	for _, e := range gSource.Edges {
		adj := gSource.Adjacent(e)
		if adj == gTarget {
			continue
		}
		o := adj.Orientation(gSource)
		if o == geo.NONE {
			continue
		}
		if slices.Contains(verticalSpaceSourceOrientations, o) {
			numInVerticalLaunchSpace++
		}
		if slices.Contains(horizontalSpaceSourceOrientations, o) {
			numInHorizontalLaunchSpace++
		}
	}

	for _, e := range gTarget.Edges {
		adj := gTarget.Adjacent(e)
		if adj == gSource {
			continue
		}
		o := adj.Orientation(gTarget)
		if o == geo.NONE {
			continue
		}
		if slices.Contains(verticalSpaceTargetOrientations, o) {
			numInVerticalLaunchSpace++
		} else if slices.Contains(horizontalSpaceTargetOrientations, o) {
			numInHorizontalLaunchSpace++
		}
	}

	// Even if there is no distinction, we don't just want to always prefer one or the other, as that can lead to weird looking graphs
	// So we use node ID as a pseudo-random tiebreak
	if numInVerticalLaunchSpace == numInHorizontalLaunchSpace {
		return gSource.ID < gTarget.ID, false
	}
	return numInVerticalLaunchSpace < numInHorizontalLaunchSpace, true
}

func (router *ovgEdgeRouter) fillPathGuarded(from, to *OVGNode) (filled []*OVGNode, success bool, err error) {
	out := []*OVGNode{from}
	curr := from
	for {
		if err := router.work.step(); err != nil {
			return nil, false, err
		}
		prev := curr
		for _, edge := range curr.Edges {
			if err := router.work.step(); err != nil {
				return nil, false, err
			}
			adjacent := curr.Adjacent(edge)
			onPath := false
			if from.X == to.X && adjacent.X == curr.X {
				onPath = from.Y < to.Y && curr.Y < adjacent.Y || from.Y > to.Y && curr.Y > adjacent.Y
			} else if from.Y == to.Y && adjacent.Y == curr.Y {
				onPath = from.X < to.X && curr.X < adjacent.X || from.X > to.X && curr.X > adjacent.X
			}
			if onPath {
				curr = adjacent
				break
			}
		}
		if curr == prev {
			return nil, false, nil
		}
		if curr == to {
			break
		}
		out = append(out, curr)
	}
	return out, true, nil
}

type RouteChecker struct {
	mem     map[*OVGNode]map[*OVGNode]bool
	compute func(from, to *OVGNode) (bool, error)
}

func newFallibleRouteChecker(compute func(from, to *OVGNode) (bool, error)) *RouteChecker {
	return &RouteChecker{
		mem:     make(map[*OVGNode]map[*OVGNode]bool),
		compute: compute,
	}
}
func (checker RouteChecker) cacheCheck(from, to *OVGNode) (isShared bool, hit bool) {
	if _, ok := checker.mem[from]; ok {
		if is, ok := checker.mem[from][to]; ok {
			return is, true
		}
	}
	return false, false
}
func (checker RouteChecker) check(from, to *OVGNode) (isShared bool, err error) {
	if is, ok := checker.cacheCheck(from, to); ok {
		return is, nil
	}
	if _, ok := checker.mem[from]; !ok {
		checker.mem[from] = make(map[*OVGNode]bool)
	}
	checker.mem[from][to], err = checker.compute(from, to)
	if err != nil {
		return false, err
	}
	return checker.mem[from][to], nil
}

func (router *ovgEdgeRouter) arrowheadLabelOverlapPenalty(gEdge *layoutgraph.Edge, route []*OVGNode) (penalty float64, err error) {
	if gEdge.SourceArrowheadLabel == nil && gEdge.TargetArrowheadLabel == nil {
		return 0, nil
	}

	routePoints := make([]*geo.Point, 0, len(route)-2)
	for i := 1; i < len(route)-1; i++ {
		if err := router.work.step(); err != nil {
			return 0, err
		}
		routePoints = append(routePoints, route[i].Point)
	}

	nonAncestors, err := filterEdgeAncestorsGuarded(gEdge, router.graph.Nodes, router.work)
	if err != nil {
		return 0, err
	}

	if l := gEdge.SourceArrowheadLabel; l != nil {
		pal := labeling.PositionArrowheadLabel(gEdge, false, routePoints)
		cost, err := positionedArrowheadLabelCostGuarded(*pal, nonAncestors, router.positionedLabels, router.routes, nil, router.work)
		if err != nil {
			return 0, err
		}
		penalty += cost
	}
	if l := gEdge.TargetArrowheadLabel; l != nil {
		pal := labeling.PositionArrowheadLabel(gEdge, true, routePoints)
		cost, err := positionedArrowheadLabelCostGuarded(*pal, nonAncestors, router.positionedLabels, router.routes, nil, router.work)
		if err != nil {
			return 0, err
		}
		penalty += cost
	}
	return penalty, nil
}

func positionedArrowheadLabelCost(pal labeling.PositionedArrowheadLabel, nodes []*layoutgraph.Node, labels []labeling.PositionedArrowheadLabel, routes []*Route, edges []*layoutgraph.Edge) (penalty float64) {
	for _, other := range labels {
		if pal.Edge == other.Edge && pal.IsTarget == other.IsTarget {
			continue
		}
		if pal.Box.Overlaps(other.Box) {
			if pal.Text == other.Text {
				continue
			}
			return math.Inf(1)
		}
	}

	graph := pal.Edge.From.Graph
	fakeLabelNode := &layoutgraph.Node{
		Box:   pal.Box,
		Graph: graph,
	}
	penalty += 4 * graph.TurnCost() * float64(fakeLabelNode.NodeOverlapCount(nodes, label.PADDING))

	overlappingEdgeCount := 0
	// during edge routing we work with routes, afterwards we use edges
	for _, route := range routes {
		if route.GEdge == pal.Edge {
			continue
		}
		for i := 1; i < len(route.OVGNodes); i++ {
			if fakeLabelNode.OverlapsLine(route.OVGNodes[i-1].Point, route.OVGNodes[i].Point, 0) {
				overlappingEdgeCount++
				break
			}
		}
	}
	for _, edge := range edges {
		if edge == pal.Edge {
			continue
		}
		for i := 1; i < len(edge.Points); i++ {
			if fakeLabelNode.OverlapsLine(edge.Points[i-1], edge.Points[i], 0) {
				overlappingEdgeCount++
				break
			}
		}
	}
	penalty += graph.TurnCost() * float64(overlappingEdgeCount)
	return penalty
}

func (router *ovgEdgeRouter) slingshot(ctx context.Context, gEdge *layoutgraph.Edge) ([]*OVGNode, float64, error) {
	if err := router.bindWork(ctx); err != nil {
		return nil, 0, err
	}
	if err := router.work.step(); err != nil {
		return nil, 0, err
	}
	gSource := gEdge.From
	gTarget := gEdge.To

	if gSource.Cluster != nil || gTarget.Cluster != nil {
		return nil, 0.0, nil
	}

	orientation := gSource.Orientation(gTarget)
	if orientation == geo.NONE {
		// nolint: nilerr
		return nil, 0.0, nil
	}
	if gEdge.HasTableColumn() {
		return nil, 0.0, nil
	}
	if !orientation.IsDiagonal() {
		return nil, 0.0, nil
	}

	occupiedRouteChecker := newFallibleRouteChecker(func(from, to *OVGNode) (bool, error) {
		var overlappingEdges []*layoutgraph.Edge
		overlappingRoutes, err := router.findOverlappingRoutes(from, to)
		if err != nil {
			return false, err
		}
		for _, route := range overlappingRoutes {
			if err := router.work.reserveProduct(uint64(len(route.OVGNodes))+1, 2); err != nil {
				return false, err
			}
			overlappingEdges = append(overlappingEdges, route.GEdge)
			if gEdge.IsDirected() && route.isOpposingColinear(from, to) {
				return true, nil
			}
		}
		canOverlap, err := edgeCanOverlapEdgesGuarded(gEdge, overlappingEdges, nil, nil, router.work)
		if err != nil {
			return false, err
		}
		return len(overlappingEdges) > 0 && !canOverlap, nil
	})

	crossedRouteChecker := newFallibleRouteChecker(func(from, to *OVGNode) (bool, error) {
		return router.edgeSet.intersectsWithGuarded(NewOVGEdge(from, to), router.work)
	})

	var shortestPath []*OVGNode
	var shortestFlight float64

	order := []bool{false, true}
	if err := router.work.reserveSum(uint64(len(gSource.Edges)), uint64(len(gTarget.Edges))); err != nil {
		return nil, 0, err
	}
	verticalPref, strongLaunchPref := preferLaunchingVertically(gSource, gTarget, orientation)
	if verticalPref {
		order = []bool{true, false}
	}

	if !gEdge.IsBetweenTableColumns() {
		// as we route column to column, there's no way to get L shaped routes between tables
		var err error
		shortestPath, shortestFlight, err = router.findLShapedRoute(
			ctx,
			router.routes,
			order,
			verticalPref,
			strongLaunchPref,
			gEdge,
			orientation,
			occupiedRouteChecker,
			crossedRouteChecker,
		)
		if err != nil {
			return nil, 0, err
		}
		if shortestFlight != math.Inf(1) {
			shortestFlight += router.turnCost
			return shortestPath, shortestFlight, nil
		}
	}

	shortestPath, shortestFlight, err := router.findSShapedRoute(
		ctx,
		order,
		verticalPref,
		strongLaunchPref,
		gEdge,
		orientation,
		occupiedRouteChecker,
		crossedRouteChecker,
	)
	return shortestPath, shortestFlight, err
}

func (router *ovgEdgeRouter) findLShapedRoute(
	ctx context.Context,
	routes []*Route,
	order []bool,
	verticalPref bool,
	strongLaunchPref bool,
	gEdge *layoutgraph.Edge,
	orientation geo.Orientation,
	occupiedRouteChecker *RouteChecker,
	crossedRouteChecker *RouteChecker,
) ([]*OVGNode, float64, error) {
	if err := router.bindWork(ctx); err != nil {
		return nil, 0, err
	}
	if err := router.work.step(); err != nil {
		return nil, 0, err
	}
	gSource := gEdge.From
	gTarget := gEdge.To

	source := router.ovg.Centers[gSource]
	target := router.ovg.Centers[gTarget]

	var shortestPath []*OVGNode
	shortestFlight := math.Inf(1)

	gEdgeCopy := layoutgraph.NewEdge(gSource, gTarget)

	for _, isVerticalLaunch := range order {
		if err := router.work.step(); err != nil {
			return nil, 0, err
		}
		if isVerticalLaunch && gEdge.FromTableColumnIndex != nil {
			// this can't happen
			continue
		}
		var sourcePorts, targetPorts []*OVGNode
		if gEdge.FromTableColumnIndex == nil {
			if err := router.work.add(uint64(len(router.ovg.Ports[gSource]))); err != nil {
				return nil, 0, err
			}
			sourcePorts = router.findLaunchings(isVerticalLaunch, orientation, gSource)
		}
		if gEdge.ToTableColumnIndex == nil {
			if err := router.work.add(uint64(len(router.ovg.Ports[gTarget]))); err != nil {
				return nil, 0, err
			}
			targetPorts = router.findLandings(isVerticalLaunch, orientation, gTarget)
		}
		if gEdge.HasTableColumn() {
			sPorts, tPorts, err := router.tablePorts(gEdge, gSource, gTarget, true)
			if err != nil {
				return nil, 0, err
			}
			for port := range sPorts {
				sourcePorts = append(sourcePorts, port)
			}
			for port := range tPorts {
				targetPorts = append(targetPorts, port)
			}
		}

		for _, sPort := range sourcePorts {
			if err := router.work.step(); err != nil {
				return nil, 0, err
			}
			curr := sPort
			for {
				if err := router.work.step(); err != nil {
					return nil, 0, err
				}
				var next *OVGNode
				for _, e := range curr.Edges {
					if err := router.work.step(); err != nil {
						return nil, 0, err
					}
					adj := curr.Adjacent(e)

					if isOnFlightPlan(curr, adj, orientation, isVerticalLaunch) {
						next = adj
						break
					}
				}
				if next == nil || overshot(next, gTarget, orientation, isVerticalLaunch, true) {
					break
				}
				occupied, err := occupiedRouteChecker.check(curr, next)
				if err != nil {
					return nil, 0, err
				}
				if occupied {
					break
				}
				curr = next
				if undershot(next, gTarget, orientation, isVerticalLaunch) {
					continue
				}
				for _, tPort := range targetPorts {
					if err := router.work.step(); err != nil {
						return nil, 0, err
					}
					if isVerticalLaunch && next.Y != tPort.Y {
						continue
					}
					if !isVerticalLaunch && next.X != tPort.X {
						continue
					}

					d, err := router.launch(sPort, next, tPort, gEdgeCopy)
					if err != nil {
						return nil, 0, err
					}
					if d == 0.0 {
						continue
					}

					crossed, err := crossedRouteChecker.check(sPort, next)
					if err != nil {
						return nil, 0, err
					}
					if crossed {
						d += router.crossingCost
					}
					crossed, err = crossedRouteChecker.check(next, tPort)
					if err != nil {
						return nil, 0, err
					}
					if crossed {
						d += router.crossingCost
					}

					if strongLaunchPref {
						if verticalPref && !isVerticalLaunch {
							d += router.crossingCost / 2.0
						}
						if !verticalPref && isVerticalLaunch {
							d += router.crossingCost / 2.0
						}
					}
					if d < shortestFlight {
						path := []*OVGNode{source}
						p, ok, err := router.fillPathGuarded(sPort, next)
						if err != nil {
							return nil, 0, err
						}
						if !ok {
							continue
						}
						path = append(path, p...)
						p, ok, err = router.fillPathGuarded(next, tPort)
						if err != nil {
							return nil, 0, err
						}
						if !ok {
							continue
						}
						p = append(p, tPort)
						// We already checked share for sPort to anchor
						badShare := false
						for i := 0; i < len(p)-2; i++ {
							if err := router.work.step(); err != nil {
								return nil, 0, err
							}
							curr, next := p[i], p[i+1]
							occupied, err := occupiedRouteChecker.check(curr, next)
							if err != nil {
								return nil, 0, err
							}
							if occupied {
								badShare = true
								break
							}
						}
						if badShare {
							continue
						}
						path = append(path, p...)
						path = append(path, target)

						// even if it is ok to share with a route, we don't want the exact same route
						duplicateRoute := false
						for _, r := range routes {
							if err := router.work.step(); err != nil {
								return nil, 0, err
							}
							if nonNilEquals(&r.FromPort, sPort.Point) && nonNilEquals(&r.ToPort, tPort.Point) && len(r.OVGNodes) == len(path) {
								isDuplicate := true
								// check if points between ports are equal
								for i := 2; i < len(path)-2; i++ {
									if err := router.work.step(); err != nil {
										return nil, 0, err
									}
									if !nonNilEquals(r.OVGNodes[i].Point, path[i].Point) {
										isDuplicate = false
										break
									}
								}
								if isDuplicate {
									duplicateRoute = true
									break
								}
							}
						}
						if duplicateRoute {
							continue
						}

						labelPenalty, err := router.arrowheadLabelOverlapPenalty(gEdge, path)
						if err != nil {
							return nil, 0, err
						}
						d += labelPenalty
						if d < shortestFlight {
							shortestFlight = d
							shortestPath = path
						}
					}
				}
			}
		}
	}

	return shortestPath, shortestFlight, nil
}

func (router *ovgEdgeRouter) findSShapedRoute(
	ctx context.Context,
	order []bool,
	verticalPref bool,
	strongLaunchPref bool,
	gEdge *layoutgraph.Edge,
	orientation geo.Orientation,
	occupiedRouteChecker *RouteChecker,
	crossedRouteChecker *RouteChecker,
) ([]*OVGNode, float64, error) {
	if err := router.bindWork(ctx); err != nil {
		return nil, 0, err
	}
	if err := router.work.step(); err != nil {
		return nil, 0, err
	}
	gSource := gEdge.From
	gTarget := gEdge.To

	source := router.ovg.Centers[gSource]
	target := router.ovg.Centers[gTarget]

	var shortestPath []*OVGNode
	shortestFlight := math.Inf(1)

	gEdgeCopy := layoutgraph.NewEdge(gSource, gTarget)

	nodes := layoutgraph.Nodes(router.ovg.NodesInsideBoundingBox)
	for _, isVerticalLaunch := range order {
		if err := router.work.step(); err != nil {
			return nil, 0, err
		}
		var sourcePorts, targetPorts []*OVGNode
		if isVerticalLaunch && gEdge.FromTableColumnIndex != nil {
			// this can't happen
			continue
		}
		if gEdge.FromTableColumnIndex == nil {
			if err := router.work.add(uint64(len(router.ovg.Ports[gSource]))); err != nil {
				return nil, 0, err
			}
			sourcePorts = router.findLaunchings(isVerticalLaunch, orientation, gSource)
		}
		if gEdge.ToTableColumnIndex == nil {
			if err := router.work.add(uint64(len(router.ovg.Ports[gTarget]))); err != nil {
				return nil, 0, err
			}
			targetPorts = router.findLandings(!isVerticalLaunch, orientation, gTarget)
		}
		if gEdge.HasTableColumn() {
			sPorts, tPorts, err := router.tablePorts(gEdge, gSource, gTarget, true)
			if err != nil {
				return nil, 0, err
			}
			for port := range sPorts {
				sourcePorts = append(sourcePorts, port)
			}
			for port := range tPorts {
				targetPorts = append(targetPorts, port)
			}
		}

		sourceAnchors := make(map[*OVGNode]*OVGNode)
		targetAnchors := make(map[*OVGNode]*OVGNode)
		var sAnchorOrder []*OVGNode
		var tAnchorOrder []*OVGNode

		for _, sPort := range sourcePorts {
			if err := router.work.step(); err != nil {
				return nil, 0, err
			}
			curr := sPort
			for {
				if err := router.work.step(); err != nil {
					return nil, 0, err
				}
				var next *OVGNode
				for _, e := range curr.Edges {
					if err := router.work.step(); err != nil {
						return nil, 0, err
					}
					adj := curr.Adjacent(e)

					if isOnFlightPlan(curr, adj, orientation, isVerticalLaunch) {
						next = adj
						break
					}
				}
				if next == nil {
					break
				}
				if overshot(next, gTarget, orientation, isVerticalLaunch, false) {
					break
				}
				occupied, err := occupiedRouteChecker.check(curr, next)
				if err != nil {
					return nil, 0, err
				}
				if occupied {
					break
				}

				sourceAnchors[next] = sPort
				sAnchorOrder = append(sAnchorOrder, next)
				curr = next
			}
		}

		for _, tPort := range targetPorts {
			if err := router.work.step(); err != nil {
				return nil, 0, err
			}
			curr := tPort
			for {
				if err := router.work.step(); err != nil {
					return nil, 0, err
				}
				var next *OVGNode
				for _, e := range curr.Edges {
					if err := router.work.step(); err != nil {
						return nil, 0, err
					}
					adj := curr.Adjacent(e)

					if isOnFlightPlan(curr, adj, orientation.GetOpposite(), isVerticalLaunch) {
						next = adj
						break
					}
				}
				if next == nil {
					break
				}
				if overshot(next, gSource, orientation.GetOpposite(), isVerticalLaunch, false) {
					break
				}
				occupied, err := occupiedRouteChecker.check(curr, next)
				if err != nil {
					return nil, 0, err
				}
				if occupied {
					break
				}

				targetAnchors[next] = tPort
				tAnchorOrder = append(tAnchorOrder, next)
				curr = next
			}
		}

		idealTurnAxes := idealTurnAxes(gSource, gTarget)
		// Prefer anchors closer to the midpoint
		if err := router.work.reserveSort(len(sAnchorOrder)); err != nil {
			return nil, 0, err
		}
		slices.SortStableFunc(sAnchorOrder, func(a, b *OVGNode) int {
			var aDistance, bDistance float64
			if isVerticalLaunch {
				aDistance = math.Abs(a.Y - idealTurnAxes[1].val)
				bDistance = math.Abs(b.Y - idealTurnAxes[1].val)
			} else {
				aDistance = math.Abs(a.X - idealTurnAxes[0].val)
				bDistance = math.Abs(b.X - idealTurnAxes[0].val)
			}
			switch {
			case aDistance < bDistance:
				return -1
			case bDistance < aDistance:
				return 1
			default:
				return 0
			}
		})

		for _, sAnchor := range sAnchorOrder {
			if err := router.work.step(); err != nil {
				return nil, 0, err
			}
			sPort := sourceAnchors[sAnchor]
			for _, tAnchor := range tAnchorOrder {
				if err := router.work.step(); err != nil {
					return nil, 0, err
				}
				tPort := targetAnchors[tAnchor]
				if isVerticalLaunch && sAnchor.Y != tAnchor.Y {
					continue
				}
				if !isVerticalLaunch && sAnchor.X != tAnchor.X {
					continue
				}
				d := 0.0
				pairs := [][]*OVGNode{
					{sPort, sAnchor},
					{sAnchor, tAnchor},
					{tAnchor, tPort},
				}
				intersects := false
				for _, pair := range pairs {
					if err := router.work.step(); err != nil {
						return nil, 0, err
					}
					gEdgeCopy.Points = []*geo.Point{pair[0].Point, pair[1].Point}
					intersectsNode, err := routeIntersectsNodeGuarded(nodes, gEdgeCopy, router.work)
					if err != nil {
						return nil, 0, err
					}
					if intersectsNode {
						intersects = true
						break
					}
					d += geo.EuclideanDistance(pair[0].X, pair[0].Y, pair[1].X, pair[1].Y)
					crossed, err := crossedRouteChecker.check(pair[0], pair[1])
					if err != nil {
						return nil, 0, err
					}
					if crossed {
						d += router.crossingCost
					}
				}
				if intersects {
					continue
				}
				if strongLaunchPref {
					if verticalPref && !isVerticalLaunch {
						d += router.crossingCost / 2.0
					}
					if !verticalPref && isVerticalLaunch {
						d += router.crossingCost / 2.0
					}
				}
				if d < shortestFlight {
					path := []*OVGNode{source}
					p, ok, err := router.fillPathGuarded(sPort, sAnchor)
					if err != nil {
						return nil, 0, err
					}
					if !ok {
						continue
					}
					path = append(path, p...)
					p, ok, err = router.fillPathGuarded(sAnchor, tAnchor)
					if err != nil {
						return nil, 0, err
					}
					if !ok {
						continue
					}
					// add this just for the share check
					p = append(p, tAnchor)
					// We already checked share for sPort to sAnchor and tPort to tAnchor
					badShare := false
					for i := 0; i < len(p)-2; i++ {
						if err := router.work.step(); err != nil {
							return nil, 0, err
						}
						curr, next := p[i], p[i+1]
						occupied, err := occupiedRouteChecker.check(curr, next)
						if err != nil {
							return nil, 0, err
						}
						if occupied {
							badShare = true
							break
						}
					}
					if badShare {
						continue
					}
					path = append(path, p[:len(p)-1]...)
					p, ok, err = router.fillPathGuarded(tAnchor, tPort)
					if err != nil {
						return nil, 0, err
					}
					if !ok {
						continue
					}
					path = append(path, p...)
					path = append(path, tPort)
					path = append(path, target)

					labelPenalty, err := router.arrowheadLabelOverlapPenalty(gEdge, path)
					if err != nil {
						return nil, 0, err
					}
					d += labelPenalty
					if d < shortestFlight {
						shortestFlight = d
						shortestPath = path
					}
				}
			}
		}
	}

	if shortestFlight != math.Inf(1) {
		shortestFlight += router.turnCost * 2
	}

	return shortestPath, shortestFlight, nil
}
