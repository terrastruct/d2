package routing

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"sync"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/loops"

	"github.com/d2lang/d2/lib/geo"
)

type GenerateRouteResponse struct {
	Routes   []*Route
	Distance float64
	Err      error
	Flavor   RouteGenerationFlavor
	work     *routeSearchWorkGuard
}

// routeWorkerPanic records a panic from a route-flavor worker for its
// coordinator. It intentionally does not carry a stack: invariant failures
// are reported as typed errors, while stack capture belongs at process or API
// boundaries that can keep it out of user-facing errors.
type routeWorkerPanic struct {
	flavor RouteGenerationFlavor
}

func (p *routeWorkerPanic) asError() error {
	return invariant.Errorf("edge routing flavor %s violated an invariant", p.flavor)
}

type routeFlavorWorker func(*ovgEdgeRouter, context.Context, bool) GenerateRouteResponse

type routeFlavorResult struct {
	index       int
	response    GenerateRouteResponse
	workerPanic *routeWorkerPanic
}

func runRouteFlavorWorker(
	ctx context.Context,
	index int,
	router *ovgEdgeRouter,
	straightLineFallback bool,
	worker routeFlavorWorker,
) (result routeFlavorResult) {
	var flavor RouteGenerationFlavor
	if router != nil {
		flavor = router.flavor
	}
	result.index = index
	result.response.Flavor = flavor
	defer func() {
		if recovered := recover(); recovered != nil {
			result = routeFlavorResult{
				index:       index,
				response:    GenerateRouteResponse{Flavor: flavor},
				workerPanic: &routeWorkerPanic{flavor: flavor},
			}
		}
	}()

	result.response = worker(router, ctx, straightLineFallback)
	// The coordinator owns flavor identity and ordering. Do not trust worker
	// output for either, even though the production worker currently sets both.
	result.response.Flavor = flavor
	return result
}

func generateRouteFlavorResponses(ctx context.Context, routers []*ovgEdgeRouter, straightLineFallback bool) ([]GenerateRouteResponse, error) {
	return generateRouteFlavorResponsesWith(ctx, routers, straightLineFallback, (*ovgEdgeRouter).generateRoutes)
}

func generateRouteFlavorResponsesWith(
	ctx context.Context,
	routers []*ovgEdgeRouter,
	straightLineFallback bool,
	worker routeFlavorWorker,
) ([]GenerateRouteResponse, error) {
	if len(routers) > maxRouteSearchFlavors {
		return nil, fmt.Errorf(
			"%w: edge routing declared %d flavors; limit %d",
			errRouteSearchWorkLimit,
			len(routers),
			maxRouteSearchFlavors,
		)
	}
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	responseChan := make(chan routeFlavorResult, len(routers))
	var workers sync.WaitGroup
	for i, router := range routers {
		workers.Go(func() {
			responseChan <- runRouteFlavorWorker(workerCtx, i, router, straightLineFallback, worker)
		})
	}
	go func() {
		workers.Wait()
		close(responseChan)
	}()
	return collectRouteFlavorResponses(ctx, responseChan, cancelWorkers)
}

func collectRouteFlavorResponses(ctx context.Context, responseChan <-chan routeFlavorResult, cancelWorkers context.CancelFunc) ([]GenerateRouteResponse, error) {
	results := make([]routeFlavorResult, 0, cap(responseChan))
	done := ctx.Done()
	var ctxErr error
	workersCanceled := false
	cancel := func() {
		if !workersCanceled {
			workersCanceled = true
			cancelWorkers()
		}
	}

	for {
		select {
		case <-done:
			ctxErr = ctx.Err()
			cancel()
			// Disable this select arm after observing cancellation so the result
			// channel can be drained without a permanently-ready competitor.
			done = nil
		case result, ok := <-responseChan:
			if !ok {
				// Worker completion order is scheduler-dependent. Restore the declared
				// router order before selecting either failures or successes.
				slices.SortStableFunc(results, func(a, b routeFlavorResult) int {
					return cmp.Compare(a.index, b.index)
				})
				for _, result := range results {
					if result.workerPanic != nil {
						return nil, result.workerPanic.asError()
					}
				}
				if ctxErr == nil {
					ctxErr = ctx.Err()
				}
				if ctxErr != nil {
					return nil, fmt.Errorf("EdgeRouting: %w", ctxErr)
				}
				responses := make([]GenerateRouteResponse, len(results))
				for i, result := range results {
					responses[i] = result.response
				}
				return responses, nil
			}
			results = append(results, result)
			if result.workerPanic != nil {
				// A panic invalidates the whole routing attempt. Cancel sibling work,
				// but keep draining until every worker has joined.
				cancel()
			}
		}
	}
}

func successfulRouteFlavorResponses(responses []GenerateRouteResponse) (map[RouteGenerationFlavor]GenerateRouteResponse, error) {
	successful := make(map[RouteGenerationFlavor]GenerateRouteResponse, len(responses))
	var firstError error
	for _, response := range responses {
		if response.Err != nil {
			// An ordinary flavor can fail while another valid flavor succeeds. The
			// whole-stage aggregate is different: once exhausted, accepting a flavor
			// that happened to finish first would make the resource bound depend on
			// goroutine scheduling and let total routing work exceed its contract.
			if errors.Is(response.Err, errRouteStageWorkLimit) {
				return nil, response.Err
			}
			if firstError == nil {
				firstError = response.Err
			}
			continue
		}
		successful[response.Flavor] = response
	}

	if len(successful) > 0 {
		return successful, nil
	}
	if firstError != nil {
		return nil, firstError
	}
	return nil, invariant.New("edge routing produced no flavor responses")
}

type selectedRouteUpdate struct {
	edge   *layoutgraph.Edge
	points []*geo.Point
}

// finalizeSelectedRoutes accounts route grouping, sorting, reversal, and point
// materialization against the winning flavor's remaining budget. All fallible
// work completes before graph edges are changed, preserving route atomicity.
func finalizeSelectedRoutes(
	ctx context.Context,
	g *layoutgraph.Graph,
	routes []*Route,
	selectedEdges map[*layoutgraph.Edge]struct{},
	work *routeSearchWorkGuard,
	reorderParallelEdges bool,
) (err error) {
	if work == nil {
		return invariant.New("selected edge-routing flavor has no work guard")
	}
	if g == nil {
		return invariant.New("selected edge-routing flavor has no graph")
	}
	if err := work.bind(ctx); err != nil {
		return err
	}
	finished := false
	defer func() {
		if !finished {
			_ = work.finish()
		}
	}()
	if selectedEdges != nil {
		selectedRoutes := make([]*Route, 0, len(selectedEdges))
		found := make(map[*layoutgraph.Edge]struct{}, len(selectedEdges))
		for _, route := range routes {
			if err := work.step(); err != nil {
				return err
			}
			if route == nil {
				return invariant.New("selected edge-routing flavor produced a nil route")
			}
			if _, selected := selectedEdges[route.GEdge]; !selected {
				continue
			}
			if _, duplicate := found[route.GEdge]; duplicate {
				return invariant.New("selected edge-routing flavor produced duplicate routes for an edge")
			}
			found[route.GEdge] = struct{}{}
			selectedRoutes = append(selectedRoutes, route)
		}
		if len(found) != len(selectedEdges) {
			return invariant.New("selected edge-routing flavor omitted a requested edge")
		}
		routes = selectedRoutes
	}

	for _, route := range routes {
		if err := work.step(); err != nil {
			return err
		}
		if route == nil || route.GEdge == nil || route.GEdge.From == nil || route.GEdge.To == nil || len(route.OVGNodes) < 2 {
			return invariant.New("selected edge-routing flavor produced an invalid route")
		}
		for _, node := range route.OVGNodes {
			if err := work.step(); err != nil {
				return err
			}
			if node == nil || node.Point == nil {
				return invariant.New("selected edge-routing flavor produced an invalid route node")
			}
		}
	}

	if reorderParallelEdges {
		if err := reorderSelectedRoutes(g, routes, work); err != nil {
			return err
		}
	}

	if err := work.add(uint64(len(routes))); err != nil {
		return err
	}
	updates := make([]selectedRouteUpdate, 0, len(routes))
	for _, route := range routes {
		if err := work.step(); err != nil {
			return err
		}
		if err := work.add(uint64(len(route.OVGNodes))); err != nil {
			return err
		}
		updates = append(updates, selectedRouteUpdate{
			edge:   route.GEdge,
			points: route.createSegmentEndpoints(),
		})
	}
	if err := work.finish(); err != nil {
		return err
	}

	// The commit contains no fallible work. A route-search error or cancellation
	// above therefore leaves every graph edge byte-for-byte unchanged.
	for _, update := range updates {
		update.edge.Points = update.points
	}
	finished = true
	return nil
}

func reorderSelectedRoutes(g *layoutgraph.Graph, routes []*Route, work *routeSearchWorkGuard) error {
	if err := work.add(uint64(len(routes))); err != nil {
		return err
	}
	buckets := make([][]*Route, 0)
	grouped := make(map[*Route]struct{}, len(routes))
	for index, route := range routes {
		if err := work.step(); err != nil {
			return err
		}
		if _, ok := grouped[route]; ok {
			continue
		}
		bucket := []*Route{route}
		grouped[route] = struct{}{}
		for _, other := range routes[index+1:] {
			if err := work.step(); err != nil {
				return err
			}
			if _, ok := grouped[other]; ok {
				continue
			}
			canSwap, err := route.canSwapEdgesGuarded(other, routes, work)
			if err != nil {
				return err
			}
			if canSwap {
				bucket = append(bucket, other)
				grouped[other] = struct{}{}
			}
		}
		if len(bucket) > 1 {
			buckets = append(buckets, bucket)
		}
	}

	if err := work.add(uint64(len(g.Edges))); err != nil {
		return err
	}
	edgeIndices := make(map[*layoutgraph.Edge]int, len(g.Edges))
	for index, edge := range g.Edges {
		if err := work.step(); err != nil {
			return err
		}
		edgeIndices[edge] = index
	}

	for _, bucket := range buckets {
		if err := work.step(); err != nil {
			return err
		}
		if err := work.reserveSort(len(bucket)); err != nil {
			return err
		}
		// Sort bucket from left-right or top-bottom.
		sort.Slice(bucket, func(i, j int) bool {
			e1 := bucket[i]
			e2 := bucket[j]
			n1 := e1.GEdge.From
			n2 := e1.GEdge.To
			if n1.Orientation(n2).IsVertical() {
				if e1.GEdge.From == e2.GEdge.From {
					return e1.FromPort.X < e2.FromPort.X
				}
				return e1.FromPort.X < e2.ToPort.X
			}
			if e1.GEdge.From == e2.GEdge.From {
				return e1.FromPort.Y < e2.FromPort.Y
			}
			return e1.FromPort.Y < e2.ToPort.Y
		})

		if err := work.add(uint64(len(bucket))); err != nil {
			return err
		}
		edges := make([]*layoutgraph.Edge, len(bucket))
		for index, route := range bucket {
			if err := work.step(); err != nil {
				return err
			}
			edges[index] = route.GEdge
		}
		if err := work.reserveSort(len(edges)); err != nil {
			return err
		}
		sort.Slice(edges, func(i, j int) bool {
			left, leftFound := edgeIndices[edges[i]]
			if !leftFound {
				left = -1
			}
			right, rightFound := edgeIndices[edges[j]]
			if !rightFound {
				right = -1
			}
			return left < right
		})

		for index, route := range bucket {
			if err := work.step(); err != nil {
				return err
			}
			if route.GEdge.From != edges[index].From {
				if err := work.add(uint64(len(route.OVGNodes))); err != nil {
					return err
				}
				route.FromPort, route.ToPort = route.ToPort, route.FromPort
				slices.Reverse(route.OVGNodes)
			}
			route.GEdge = edges[index]
		}
	}
	return work.check()
}

func routeEdgesWithResourceGuards(
	ctx context.Context,
	g *layoutgraph.Graph,
	nearbyNodes []*layoutgraph.Node,
	ovgGuard *ovgBuildGuard,
	searchWorkLimit uint64,
) (*OVG, error) {
	ovg, err := buildOVGFromGraphWithGuard(g, nearbyNodes, ovgGuard)
	if err != nil {
		return ovg, err
	}

	// Route all tree edges first, then route the remaining edges
	existingRoutes := make([]*Route, 0)
	isTreeEdge := g.TreeEdgeMap()
	g.AddIsolatedTreeEdges(isTreeEdge)

	existingPortArrowheads := make(map[*OVGNode]map[layoutgraph.Arrowhead]struct{})
	isRouted := map[*layoutgraph.Edge]struct{}{}
	for _, node := range g.Nodes {
		// we route each tree edge by iterating over the tree nodes, and routing with its sentinel edge
		// this way we know the edge parent/child relation (tree.Node is the child), and that the node is in nodeToTree (not a rootSentinel)
		if tree, has := g.NodeToTree[node]; has {
			// we don't route the edge from tree root to rootSentinel here since the root may move relative to the rootSentinel
			if _, is := isTreeEdge[tree.SentinelEdge]; !is {
				continue
			}

			route, err := routeSentinelEdge(tree, ovg.Ports, ovg.Centers, existingPortArrowheads)
			if err != nil {
				continue
			}

			existingRoutes = append(existingRoutes, route)
			fromPort := route.OVGNodes[1]
			toPort := route.OVGNodes[len(route.OVGNodes)-2]
			if _, has := existingPortArrowheads[fromPort]; !has {
				existingPortArrowheads[fromPort] = make(map[layoutgraph.Arrowhead]struct{})
			}
			if _, has := existingPortArrowheads[toPort]; !has {
				existingPortArrowheads[toPort] = make(map[layoutgraph.Arrowhead]struct{})
			}
			existingPortArrowheads[fromPort][tree.SentinelEdge.SourceArrowhead] = struct{}{}
			existingPortArrowheads[toPort][tree.SentinelEdge.TargetArrowhead] = struct{}{}
			isRouted[tree.SentinelEdge] = struct{}{}
		} else {
			for _, routedEdge := range loops.Route(node) {
				existingRoutes = append(existingRoutes, makeLoopRoute(routedEdge, ovg))
				isRouted[routedEdge] = struct{}{}
			}
		}
	}
	unroutedEdges := make([]*layoutgraph.Edge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if _, is := isRouted[e]; !is {
			unroutedEdges = append(unroutedEdges, e)
		}
	}

	// cache these ahead of time
	g.CrossingCost()
	g.TurnCost()
	g.NonCenterPortCost()

	flavors := []RouteGenerationFlavor{ShortestToLongest, LongestToShortest, Default}
	if g.Nodes[0].Hierarchy != nil {
		flavors = []RouteGenerationFlavor{TopDownLeftRight}
	}
	routers := make([]*ovgEdgeRouter, 0, len(flavors))
	for _, flavor := range flavors {
		router, err := newOVGEdgeRouterWithWorkLimit(ctx, flavor, ovg, g, existingRoutes, unroutedEdges, searchWorkLimit)
		if err != nil {
			return ovg, err
		}
		routers = append(routers, router)
	}
	responses, err := generateRouteFlavorResponses(ctx, routers, true)
	if err != nil {
		return ovg, err
	}

	var bestRoutes []*Route
	var bestWork *routeSearchWorkGuard
	leastDistance := math.Inf(1)

	flavorToResponse, err := successfulRouteFlavorResponses(responses)
	if err != nil {
		return ovg, err
	}

	// compare responses in fixed order
	for _, flavor := range flavors {
		if response, has := flavorToResponse[flavor]; has {
			if geo.PrecisionCompare(response.Distance, leastDistance, geo.PRECISION) < 0 {
				leastDistance = response.Distance
				bestRoutes = response.Routes
				bestWork = response.work
			}
		}
	}

	if err := finalizeSelectedRoutes(ctx, g, bestRoutes, nil, bestWork, true); err != nil {
		return ovg, err
	}

	return ovg, nil
}

func routeAdditionalEdgesWithLimits(ctx context.Context, g *layoutgraph.Graph, edges []*layoutgraph.Edge, limits ovgBuildLimits) (*OVG, error) {
	return routeAdditionalEdgesWithResourceLimits(ctx, g, edges, limits, maxRouteSearchWorkUnits)
}

func routeAdditionalEdgesWithResourceLimits(
	ctx context.Context,
	g *layoutgraph.Graph,
	edges []*layoutgraph.Edge,
	limits ovgBuildLimits,
	searchWorkLimit uint64,
) (*OVG, error) {
	newEdges := make(map[*layoutgraph.Edge]struct{})
	for _, e := range edges {
		newEdges[e] = struct{}{}
	}

	ovg, err := buildOVGFromGraphWithLimits(ctx, g, nil, limits)
	if err != nil {
		return ovg, err
	}
	guard := ovg.buildGuard

	existingRoutes := make([]*Route, 0)
	for _, e := range g.Edges {
		if err := guard.step(); err != nil {
			return ovg, err
		}
		if _, in := newEdges[e]; in {
			continue
		}

		var ovgNodes []*OVGNode
		var lastPoint *geo.Point

		addPoint := func(p *geo.Point) error {
			if err := guard.step(); err != nil {
				return err
			}
			// Skip if this point is the same as the last one
			if lastPoint != nil && *lastPoint == *p {
				return nil
			}
			lastPoint = p

			var ovgNode *OVGNode
			var has bool
			if ovgNode, has = ovg.OccupiedPoints[*p]; has {
				ovgNodes = append(ovgNodes, ovgNode)
			} else {
				ovgNode = NewOVGNode(p)
				// The base OVG was indexed before authoritative routes were added.
				ovgNode.Index = len(ovg.Nodes)
				ovgNodes = append(ovgNodes, ovgNode)
				if err := guard.addNodeUnchecked(ovg, ovgNode); err != nil {
					return err
				}
			}

			// Only connect if we have at least 2 different points
			if len(ovgNodes) > 1 {
				if _, err := guard.connect(ovg, ovgNodes[len(ovgNodes)-2], ovgNodes[len(ovgNodes)-1]); err != nil {
					return err
				}
			}
			return nil
		}

		if err := addPoint(e.From.Center()); err != nil {
			return ovg, err
		}
		for _, p := range e.Points {
			if err := addPoint(p); err != nil {
				return ovg, err
			}
		}
		if err := addPoint(e.To.Center()); err != nil {
			return ovg, err
		}

		// Only create route if we have at least 2 different points
		if len(ovgNodes) > 1 {
			route := &Route{
				GEdge:    e,
				OVGNodes: ovgNodes,
				FromPort: *e.Points[0],
				ToPort:   *e.Points[len(e.Points)-1],
			}
			existingRoutes = append(existingRoutes, route)
		}
	}

	// cache these ahead of time
	g.CrossingCost()
	g.TurnCost()
	g.NonCenterPortCost()

	flavors := []RouteGenerationFlavor{ShortestToLongest, LongestToShortest, Default}
	if g.Nodes[0].Hierarchy != nil {
		flavors = []RouteGenerationFlavor{TopDownLeftRight}
	}
	routers := make([]*ovgEdgeRouter, 0, len(flavors))
	for _, flavor := range flavors {
		router, err := newOVGEdgeRouterWithWorkLimit(ctx, flavor, ovg, g, existingRoutes, edges, searchWorkLimit)
		if err != nil {
			return ovg, err
		}
		router.considerNodeLabels = true
		routers = append(routers, router)
	}
	responses, err := generateRouteFlavorResponses(ctx, routers, true)
	if err != nil {
		return ovg, err
	}

	var bestRoutes []*Route
	var bestWork *routeSearchWorkGuard
	leastDistance := math.Inf(1)

	flavorToResponse, err := successfulRouteFlavorResponses(responses)
	if err != nil {
		return ovg, err
	}

	// compare responses in fixed order
	for _, flavor := range flavors {
		if response, has := flavorToResponse[flavor]; has {
			if geo.PrecisionCompare(response.Distance, leastDistance, geo.PRECISION) < 0 {
				leastDistance = response.Distance
				bestRoutes = response.Routes
				bestWork = response.work
			}
		}
	}

	if err := finalizeSelectedRoutes(ctx, g, bestRoutes, newEdges, bestWork, false); err != nil {
		return ovg, err
	}

	return ovg, nil
}

// For Trees, ideal route between parent & child is S shaped bend or straight line
// .  e.g    ┌───┐        ┌───┐
// .         │   │        │   │
// .         └─▲─┘        └─▲─┘
// .       ┌───┴───┐        │
// .     ┌─┴─┐   ┌─┴─┐    ┌─┴─┐
// .     │   │   │   │    │   │
// .     └─▲─┘   └───┘    └─▲─┘
// .   ┌───┴───┐            │
// . ┌─┴─┐   ┌─┴─┐        ┌─┴─┐
// . │   │   │   │        │   │
// . └───┘   └───┘        └───┘
func routeSentinelEdge(tree *layoutgraph.Tree,
	portNodes map[*layoutgraph.Node][]*OVGNode,
	centers map[*layoutgraph.Node]*OVGNode,
	portArrowheads map[*OVGNode]map[layoutgraph.Arrowhead]struct{},
) (*Route, error) {
	// Note: we are the child node since we are considering the sentinel edge
	edge := tree.SentinelEdge
	treePath := treeEdgePath(tree, portNodes, portArrowheads)

	nodesFromSourcePortToTargetPort, err := routeInSShape(treePath)
	if err != nil {
		return nil, err
	}

	// validate route
	for _, n := range nodesFromSourcePortToTargetPort {
		if n.isPort() {
			validOwner := false
			for owner := range n.portOwners() {
				if edge.From.IsDescendantOf(owner) || edge.To.IsDescendantOf(owner) {
					validOwner = true
					break
				}
			}
			if !validOwner {
				return nil, fmt.Errorf("routing through other node's port")
			}
		}
	}

	ovgNodes := make([]*OVGNode, 0)
	ovgNodes = append(ovgNodes, centers[edge.From])
	ovgNodes = append(ovgNodes, nodesFromSourcePortToTargetPort...)
	ovgNodes = append(ovgNodes, centers[edge.To])

	return &Route{
		GEdge:    edge,
		OVGNodes: ovgNodes,
		FromPort: *treePath.SourcePortNode.Point,
		ToPort:   *treePath.TargetPortNode.Point,
	}, nil
}

// create the straight line or S shaped route between source and target ports
// return the ovg nodes along the way (including source and target port)
func routeInSShape(treePath TreeEdgePath) ([]*OVGNode, error) {
	sourceOrientationToTarget := treePath.SourceOrientationToTarget
	// Create the route in 3 steps
	// 1. Route from source port to source midpoint
	// 2. Route from source midpoint to target midpoint
	// 3. Route from target midpoint to target port
	//                  ┌───┐
	//                  │ T │
	//                  └─┬─┘
	//                    3
	//       ┌──────2─────┘
	//       1
	//     ┌─┴─┐
	//     │ S │
	//     └───┘
	current := treePath.SourcePortNode
	getAdjInDirection := func(currentOrientationToAdjacent geo.Orientation) *OVGNode {
		var closestAdjNodeInDirection *OVGNode
		for _, e := range current.Edges {
			adjNode := current.adjacent(e)
			switch currentOrientationToAdjacent {
			case geo.Left:
				// if current is Left of adjacent, we want the closest y-aligned adjacent node to the Right of current
				if current.Point.Y == adjNode.Point.Y && adjNode.Point.X > current.Point.X {
					if closestAdjNodeInDirection == nil || adjNode.Point.X < closestAdjNodeInDirection.Point.X {
						closestAdjNodeInDirection = adjNode
					}
				}
			case geo.Right:
				if current.Point.Y == adjNode.Point.Y && adjNode.Point.X < current.Point.X {
					if closestAdjNodeInDirection == nil || adjNode.Point.X > closestAdjNodeInDirection.Point.X {
						closestAdjNodeInDirection = adjNode
					}
				}
			case geo.Top:
				if current.Point.X == adjNode.Point.X && adjNode.Point.Y > current.Point.Y {
					if closestAdjNodeInDirection == nil || adjNode.Point.Y < closestAdjNodeInDirection.Point.Y {
						closestAdjNodeInDirection = adjNode
					}
				}
			default:
				if current.Point.X == adjNode.Point.X && adjNode.Point.Y < current.Point.Y {
					if closestAdjNodeInDirection == nil || adjNode.Point.Y > closestAdjNodeInDirection.Point.Y {
						closestAdjNodeInDirection = adjNode
					}
				}
			}
		}
		return closestAdjNodeInDirection
	}

	nodes := make([]*OVGNode, 0)
	nodes = append(nodes, treePath.SourcePortNode)

	// Step 1. source port to source midpoint
	for !nonNilEquals(current.Point, treePath.SourceMidpoint) {
		current = getAdjInDirection(sourceOrientationToTarget)
		if current == nil {
			return nil, invariant.New("couldn't find s shaped route (source port to source midpoint)")
		}
		nodes = append(nodes, current)
	}

	// Step 2. source midpoint to target midpoint
	var crossDirection geo.Orientation
	if sourceOrientationToTarget == geo.Left || sourceOrientationToTarget == geo.Right {
		if treePath.SourcePortNode.Point.Y < treePath.TargetPortNode.Point.Y {
			crossDirection = geo.Top
		} else {
			crossDirection = geo.Bottom
		}
	} else {
		if treePath.SourcePortNode.Point.X < treePath.TargetPortNode.Point.X {
			crossDirection = geo.Left
		} else {
			crossDirection = geo.Right
		}
	}

	for !nonNilEquals(current.Point, treePath.TargetMidpoint) {
		current = getAdjInDirection(crossDirection)
		if current == nil {
			return nil, invariant.New("couldn't find s shaped route (source midpoint to target midpoint)")
		}
		nodes = append(nodes, current)
	}

	// Step 3. target midpoint to target port
	for !nonNilEquals(current.Point, treePath.TargetPortNode.Point) {
		current = getAdjInDirection(sourceOrientationToTarget)
		if current == nil {
			return nil, invariant.New("couldn't find s shaped route (target midpoint to target port)")
		}
		nodes = append(nodes, current)
	}

	return nodes, nil
}

type RouteGenerationFlavor string

const (
	ShortestToLongest RouteGenerationFlavor = "ShortestToLongest"
	LongestToShortest RouteGenerationFlavor = "LongestToShortest"
	Default           RouteGenerationFlavor = "Default"
	TopDownLeftRight  RouteGenerationFlavor = "TopDownLeftRight"
)

func routeLine(ctx context.Context, g *layoutgraph.Graph, edge *layoutgraph.Edge, allEdges []*layoutgraph.Edge, routes []*Route) (*geo.Point, *geo.Point, float64, error) {
	return routeLineChecked(g, edge, allEdges, routes, func() error {
		return checkEdgeRoutingCanceled(ctx)
	}, nil)

}

func routeLineGuarded(g *layoutgraph.Graph, edge *layoutgraph.Edge, allEdges []*layoutgraph.Edge, routes []*Route, guard *routeWorkGuard) (*geo.Point, *geo.Point, float64, error) {
	return routeLineChecked(g, edge, allEdges, routes, guard.step, guard)
}

func routeLineChecked(
	g *layoutgraph.Graph,
	edge *layoutgraph.Edge,
	allEdges []*layoutgraph.Edge,
	routes []*Route,
	checkWork func() error,
	guard workBudget,
) (*geo.Point, *geo.Point, float64, error) {
	if err := checkWork(); err != nil {
		return nil, nil, 0, err
	}
	fromOrientation := edge.From.Orientation(edge.To)
	if fromOrientation == geo.NONE {
		return nil, nil, 0.0, layoutgraph.ErrInvalidCandidate
	}
	fromPorts := edge.From.PortsByOrientation(fromOrientation.GetOpposite())

	toOrientation := edge.To.Orientation(edge.From)
	if toOrientation == geo.NONE {
		return nil, nil, 0.0, layoutgraph.ErrInvalidCandidate
	}
	toPorts := edge.To.PortsByOrientation(toOrientation.GetOpposite())

	var fromPortEdges, toPortEdges map[geo.Point][]*layoutgraph.Edge
	if guard == nil {
		fromPortEdges = layoutgraph.Edges(allEdges).PortEdges(edge.From)
		toPortEdges = layoutgraph.Edges(allEdges).PortEdges(edge.To)
	} else {
		var err error
		fromPortEdges, err = portEdgesGuarded(layoutgraph.Edges(allEdges), edge.From, guard)
		if err != nil {
			return nil, nil, 0, err
		}
		toPortEdges, err = portEdgesGuarded(layoutgraph.Edges(allEdges), edge.To, guard)
		if err != nil {
			return nil, nil, 0, err
		}
	}
	// we filter out the current edge here to keep the logic for arrowhead matching simpler
	filterEdge := func(portEdges []*layoutgraph.Edge) ([]*layoutgraph.Edge, error) {
		filteredEdges := make([]*layoutgraph.Edge, 0)
		for _, e := range portEdges {
			if err := checkWork(); err != nil {
				return nil, err
			}
			if e != edge {
				filteredEdges = append(filteredEdges, e)
			}
		}
		return filteredEdges, nil
	}
	for fromPort, portEdges := range fromPortEdges {
		filteredEdges, err := filterEdge(portEdges)
		if err != nil {
			return nil, nil, 0, err
		}
		fromPortEdges[fromPort] = filteredEdges
	}
	for toPort, portEdges := range toPortEdges {
		filteredEdges, err := filterEdge(portEdges)
		if err != nil {
			return nil, nil, 0, err
		}
		toPortEdges[toPort] = filteredEdges
	}

	fromNodeCenterPorts := make(map[geo.Point]bool)
	for _, centerPort := range edge.From.CenterPorts() {
		fromNodeCenterPorts[centerPort] = true
	}
	toNodeCenterPorts := make(map[geo.Point]bool)
	for _, centerPort := range edge.To.CenterPorts() {
		toNodeCenterPorts[centerPort] = true
	}
	var sourceClusterNodes, targetClusterNodes map[*layoutgraph.Node]bool
	if guard == nil {
		sourceClusterNodes, targetClusterNodes = sourceAndTargetClusterNodes(g, edge.From, edge.To)
	} else {
		var err error
		sourceClusterNodes, targetClusterNodes, err = sourceAndTargetClusterNodesGuarded(g, edge.From, edge.To, guard)
		if err != nil {
			return nil, nil, 0, err
		}
	}

	var nonAncestors layoutgraph.Nodes
	if edge.SourceArrowheadLabel != nil || edge.TargetArrowheadLabel != nil {
		if guard == nil {
			nonAncestors = filterEdgeAncestors(edge, g.Nodes)
		} else {
			var err error
			nonAncestors, err = filterEdgeAncestorsGuarded(edge, g.Nodes, guard)
			if err != nil {
				return nil, nil, 0, err
			}
		}
	}

	var positionedLabels []labeling.PositionedArrowheadLabel
	for i, e := range allEdges {
		if err := checkWork(); err != nil {
			return nil, nil, 0, err
		}
		if e == edge {
			continue
		}
		var route []*geo.Point
		if routes != nil {
			route = routes[i].createSegmentEndpoints()
		} else {
			route = e.Points
		}
		if len(route) == 0 {
			continue
		}
		if guard != nil {
			if err := guard.add(uint64(len(route))); err != nil {
				return nil, nil, 0, err
			}
		}
		if pal := labeling.PositionArrowheadLabel(e, false, route); pal != nil {
			positionedLabels = append(positionedLabels, *pal)
		}
		if pal := labeling.PositionArrowheadLabel(e, true, route); pal != nil {
			positionedLabels = append(positionedLabels, *pal)
		}
	}

	bestCost := math.Inf(1)
	var bestFromPort, bestToPort *geo.Point

	// for each pair, place an edge, get the cost, but multiply it by some penalty factor for being non-orthogonal
	for fromI, fromPort := range fromPorts {
		if err := checkWork(); err != nil {
			return nil, nil, 0, err
		}
		fromPortCost := 0.0
		// Note: remove fromPortEdges could be empty if there are no edges after filtering
		if fromPortEdges, has := fromPortEdges[fromPort]; has && len(fromPortEdges) > 0 {
			// we don't consider ports used by a duplicate edge
			var duplicate bool
			if guard == nil {
				duplicate = edge.HasDuplicateIn(fromPortEdges)
			} else {
				var err error
				duplicate, err = edgeHasDuplicateInGuarded(edge, fromPortEdges, guard)
				if err != nil {
					return nil, nil, 0, err
				}
			}
			if duplicate {
				// Edge case option: consider ports used by a duplicate edge, as long
				// as the pair of source and target ports are not both the same (e.g. 1/2 shared ok)
				continue
			}

			// if the port is to be shared, all the edges must match arrowheads
			allMatch := true
			for _, otherFromPortEdge := range fromPortEdges {
				if err := checkWork(); err != nil {
					return nil, nil, 0, err
				}
				if otherFromPortEdge.From == edge.From {
					// other edge source arrow must match
					if edge.SourceArrowhead != otherFromPortEdge.SourceArrowhead {
						allMatch = false
						break
					}
				} else {
					// other edge target arrow must match
					if edge.SourceArrowhead != otherFromPortEdge.TargetArrowhead {
						allMatch = false
						break
					}
				}
			}
			if !allMatch {
				continue
			}

			// sharing a port with matching arrowheads is not ideal so other source ports should be preferred
			if edge.HasSourceArrow() {
				fromPortCost = maxSideLength(edge.From, fromOrientation) / 2.0
			}
		}

		for toI, toPort := range toPorts {
			if err := checkWork(); err != nil {
				return nil, nil, 0, err
			}
			toPortCost := 0.0
			if toPortEdges, has := toPortEdges[toPort]; has && len(toPortEdges) > 0 {
				var duplicate bool
				if guard == nil {
					duplicate = edge.HasDuplicateIn(toPortEdges)
				} else {
					var err error
					duplicate, err = edgeHasDuplicateInGuarded(edge, toPortEdges, guard)
					if err != nil {
						return nil, nil, 0, err
					}
				}
				if duplicate {
					continue
				}

				allMatch := true
				for _, otherToPortEdge := range toPortEdges {
					if err := checkWork(); err != nil {
						return nil, nil, 0, err
					}
					if otherToPortEdge.To == edge.To {
						if edge.TargetArrowhead != otherToPortEdge.TargetArrowhead {
							allMatch = false
							break
						}
					} else {
						if edge.TargetArrowhead != otherToPortEdge.SourceArrowhead {
							allMatch = false
							break
						}
					}
				}
				if !allMatch {
					continue
				}

				toPortCost = maxSideLength(edge.To, toOrientation) / 2.0
			}

			edgeOption := layoutgraph.NewEdge(edge.From, edge.To)
			edgeOption.Points = []*geo.Point{&(fromPorts[fromI]), &(toPorts[toI])}

			// don't consider option if edge crosses over another node
			var intersectsNode bool
			if guard == nil {
				intersectsNode = layoutgraph.Nodes(g.Nodes).IntersectsNode(edgeOption)
			} else {
				var err error
				intersectsNode, err = routeIntersectsNodeGuarded(layoutgraph.Nodes(g.Nodes), edgeOption, guard)
				if err != nil {
					return nil, nil, 0, err
				}
			}
			if intersectsNode {
				continue
			}

			otherEdges, err := filterEdge(allEdges)
			if err != nil {
				return nil, nil, 0, err
			}

			var overlappingEdges []*layoutgraph.Edge
			if guard == nil {
				overlappingEdges = findOverlappingEdges(&fromPorts[fromI], &toPorts[toI], otherEdges)
			} else {
				var err error
				overlappingEdges, err = overlappingEdgesGuarded(&fromPorts[fromI], &toPorts[toI], otherEdges, guard)
				if err != nil {
					return nil, nil, 0, err
				}
			}
			if len(overlappingEdges) > 0 {
				canOverlap := false
				if guard == nil {
					canOverlap = edgeCanOverlapEdges(edge, overlappingEdges, sourceClusterNodes, targetClusterNodes)
				} else {
					var err error
					canOverlap, err = edgeCanOverlapEdgesGuarded(edge, overlappingEdges, sourceClusterNodes, targetClusterNodes, guard)
					if err != nil {
						return nil, nil, 0, err
					}
				}
				if !canOverlap {
					continue
				}
			}

			var routeCost float64
			if guard == nil {
				routeCost = estimateRouteCost(layoutgraph.Edges(otherEdges), edgeOption)
			} else {
				var err error
				routeCost, err = estimateRouteCostGuarded(layoutgraph.Edges(otherEdges), edgeOption, guard)
				if err != nil {
					return nil, nil, 0, err
				}
			}
			cost := fromPortCost + toPortCost + routeCost

			if fromPort.X != toPort.X && fromPort.Y != toPort.Y {
				isAxisAligned := false
				// Reward axis-aligned lines
				if geo.PrecisionCompare(fromPort.X, toPort.X, layoutgraph.AxisAlignmentTolerance) == 0 {
					isAxisAligned = true
				}
				if geo.PrecisionCompare(fromPort.Y, toPort.Y, layoutgraph.AxisAlignmentTolerance) == 0 {
					isAxisAligned = true
				}
				if !isAxisAligned {
					cost *= nonOrthogonalFactor
				}
			}

			// skip straight edges with sharp angles with source or target node
			if hasSharpAngleToBorder(&fromPorts[fromI], &toPorts[toI], fromOrientation) {
				continue
			}
			if hasSharpAngleToBorder(&fromPorts[fromI], &toPorts[toI], toOrientation) {
				continue
			}

			// Reward lines that go through center
			if _, is := fromNodeCenterPorts[fromPort]; !is {
				cost += g.NonCenterPortCost()
			}
			if _, is := toNodeCenterPorts[toPort]; !is {
				cost += g.NonCenterPortCost()
			}

			if l := edge.SourceArrowheadLabel; l != nil {
				routePoints := []*geo.Point{&(fromPorts[fromI]), &(toPorts[toI])}
				pal := labeling.PositionArrowheadLabel(edge, false, routePoints)
				if guard == nil {
					cost += positionedArrowheadLabelCost(*pal, nonAncestors, positionedLabels, nil, otherEdges)
				} else {
					labelCost, err := positionedArrowheadLabelCostGuarded(*pal, nonAncestors, positionedLabels, nil, otherEdges, guard)
					if err != nil {
						return nil, nil, 0, err
					}
					cost += labelCost
				}
			}
			if l := edge.TargetArrowheadLabel; l != nil {
				routePoints := []*geo.Point{&(fromPorts[fromI]), &(toPorts[toI])}
				pal := labeling.PositionArrowheadLabel(edge, true, routePoints)
				if guard == nil {
					cost += positionedArrowheadLabelCost(*pal, nonAncestors, positionedLabels, nil, otherEdges)
				} else {
					labelCost, err := positionedArrowheadLabelCostGuarded(*pal, nonAncestors, positionedLabels, nil, otherEdges, guard)
					if err != nil {
						return nil, nil, 0, err
					}
					cost += labelCost
				}
			}

			// get the best cost of these pairs
			if cost < bestCost {
				bestFromPort, bestToPort = &(fromPorts[fromI]), &(toPorts[toI])
				bestCost = cost
			}
		}
	}

	if bestCost == math.Inf(1) {
		return nil, nil, 0.0, layoutgraph.ErrInvalidCandidate
	}

	return bestFromPort, bestToPort, bestCost, nil
}

// return true if gEdge can overlap with the other edges provided
func edgeCanOverlapEdges(edge *layoutgraph.Edge, otherEdges []*layoutgraph.Edge, sourceClusterNodes, targetClusterNodes map[*layoutgraph.Node]bool) bool {
	if len(otherEdges) == 0 {
		return true
	}
	if edge.TargetArrowheadLabel != nil || edge.SourceArrowheadLabel != nil {
		return false
	}
	for _, e := range otherEdges {
		if e.TargetArrowheadLabel != nil || e.SourceArrowheadLabel != nil {
			return false
		}
		if !edge.EquivalentStyles(e) {
			return false
		}
	}

	isClusterEdge := false
	if len(sourceClusterNodes) > 0 {
		hasNonSourceClusterEdge := false
		for _, otherEdge := range otherEdges {
			_, isFromSourceCluster := sourceClusterNodes[otherEdge.From]
			_, isToSourceCluster := sourceClusterNodes[otherEdge.To]
			if !isFromSourceCluster && !isToSourceCluster {
				hasNonSourceClusterEdge = true
				break
			}
		}
		if !hasNonSourceClusterEdge {
			isClusterEdge = true
		}
	}
	if !isClusterEdge && len(targetClusterNodes) > 0 {
		hasNonTargetClusterEdge := false
		for _, otherEdge := range otherEdges {
			_, isFromTargetCluster := targetClusterNodes[otherEdge.From]
			_, isToTargetCluster := targetClusterNodes[otherEdge.To]
			if !isFromTargetCluster && !isToTargetCluster {
				hasNonTargetClusterEdge = true
				break
			}
		}
		if !hasNonTargetClusterEdge {
			isClusterEdge = true
		}
	}

	// TODO find a better solution for edge routing with labels. Shared edges with
	// labels require special handling unless they belong to clusters.

	// if directed edges share a source or target, they can share an edge
	allMatchingDirected := edge.IsDirected()
	if allMatchingDirected {
		for _, otherEdge := range otherEdges {
			if !otherEdge.IsDirected() {
				allMatchingDirected = false
				break
			}
			if !(edge.To == otherEdge.To || edge.From == otherEdge.From) {
				allMatchingDirected = false
				break
			}
			if edge.TargetArrowhead != otherEdge.TargetArrowhead || edge.SourceArrowhead != otherEdge.SourceArrowhead {
				allMatchingDirected = false
				break
			}
		}
	}
	if allMatchingDirected {
		return true
	}

	// bidirectional / undirected edges can overlap as long as source/target arrowheads match and
	// if they are part of the same cluster or if all edges share a node
	// TODO support overlapping bidirectional edges with different source/target arrowheads.
	var allBidirectional, allUndirected bool
	allBidirectional = edge.IsBidirectional() && edge.OwnArrowheadsMatch()
	if allBidirectional {
		for _, otherEdge := range otherEdges {
			if !otherEdge.IsBidirectional() || !edge.MatchingArrowheads(otherEdge) {
				return false
			}
		}
	} else {
		allUndirected = edge.IsUndirected()
		if allUndirected {
			for _, otherEdge := range otherEdges {
				if !otherEdge.IsUndirected() {
					return false
				}
			}
		}
	}
	if !allBidirectional && !allUndirected {
		return false
	}

	// Note: ovg_edge_router treats edges between tables columns as clusters so those are also included here
	// if edges are of the same cluster, bidirectional / undirected edges can overlap
	if isClusterEdge {
		return true
	}

	// Note: assuming no self-loops here (node appears once per edge)
	nodeCounts := make(map[*layoutgraph.Node]int)
	nodeCounts[edge.From]++
	nodeCounts[edge.To]++
	for _, otherEdge := range otherEdges {
		nodeCounts[otherEdge.From]++
		nodeCounts[otherEdge.To]++
	}
	// if all edges are connected to a shared node, then they can overlap without it becoming unclear which nodes are connected
	hasSharedNode := false
	totalEdgeCount := len(otherEdges) + 1
	for _, count := range nodeCounts {
		if count == totalEdgeCount {
			hasSharedNode = true
			break
		}
	}
	return hasSharedNode
}

func isVerticalOrHorizontalOverlap(start, end, otherStart, otherEnd *geo.Point) bool {
	bothVertical := start.X == end.X && otherStart.X == otherEnd.X
	bothHorizontal := start.Y == end.Y && otherStart.Y == otherEnd.Y
	if bothVertical || bothHorizontal {
		if intersects(otherStart, otherEnd, start, end) {
			return true
		}
	}
	return false
}

func findOverlappingEdges(start, end *geo.Point, otherEdges []*layoutgraph.Edge) []*layoutgraph.Edge {
	overlappingEdges := make([]*layoutgraph.Edge, 0)
	for _, e := range otherEdges {
		for i := 0; i < len(e.Points)-1; i++ {
			if isVerticalOrHorizontalOverlap(start, end, e.Points[i], e.Points[i+1]) {
				overlappingEdges = append(overlappingEdges, e)
				break
			}
		}
	}
	return overlappingEdges
}

func sourceAndTargetClusterNodes(g *layoutgraph.Graph, source, target *layoutgraph.Node) (map[*layoutgraph.Node]bool, map[*layoutgraph.Node]bool) {
	sourceClusterNodes := make(map[*layoutgraph.Node]bool)
	targetClusterNodes := make(map[*layoutgraph.Node]bool)

	for _, key := range g.ClusterOrder() {
		cluster := g.Clusters[key]
		isSource := false
		isTarget := false
		for _, clusterNode := range cluster.Nodes {
			if clusterNode == source {
				isSource = true
			}
			if clusterNode == target {
				isTarget = true
			}
		}

		if isSource {
			for _, clusterNode := range cluster.Nodes {
				if clusterNode == source {
					continue
				}
				sourceClusterNodes[clusterNode] = true
			}
		}

		if isTarget {
			for _, clusterNode := range cluster.Nodes {
				if clusterNode == target {
					continue
				}
				targetClusterNodes[clusterNode] = true
			}
		}
	}
	return sourceClusterNodes, targetClusterNodes
}

// return the length of the side of the node
// if orientation is on a corner return whichever side is longer
func maxSideLength(node *layoutgraph.Node, orientation geo.Orientation) float64 {
	switch {
	case orientation == geo.Top || orientation == geo.Bottom:
		return node.Width
	case orientation == geo.Left || orientation == geo.Right:
		return node.Height
	default:
		return math.Max(node.Width, node.Height)
	}
}

// | Consider straight edge alternatives for each edge.
// | For each edge we consider each pair of ports between the source and target nodes.
// | By default we don't consider an alternative if it uses an occupied port, but a port can be shared if:
// |   (1) it is not used by a duplicate edge, and (2) the edge and all edges at the port have matching arrowheads
//
// | Edge case: if there are more duplicate edges than ports, we end up running out of alternatives.
// |   we could allow duplicate edges to share ports as long as they're not the same pair
// |      (in which case we wouldn't run out of options until there are more duplicates than port pairs)
type straightEdgeFallbackChange struct {
	edge   *layoutgraph.Edge
	points []*geo.Point
}

type straightEdgeFallbackRollback struct {
	graph         *layoutgraph.Graph
	costs         layoutgraph.RoutingCostState
	costsCaptured bool
	firstChange   straightEdgeFallbackChange
	hasChange     bool
	moreChanges   []straightEdgeFallbackChange
}

func (rollback *straightEdgeFallbackRollback) captureCosts(graph *layoutgraph.Graph) {
	if rollback.costsCaptured {
		return
	}
	rollback.graph = graph
	rollback.costs = graph.RoutingCosts()
	rollback.costsCaptured = true
}

func (rollback *straightEdgeFallbackRollback) record(edge *layoutgraph.Edge, points []*geo.Point) {
	change := straightEdgeFallbackChange{edge: edge, points: points}
	if !rollback.hasChange {
		rollback.firstChange = change
		rollback.hasChange = true
		return
	}
	rollback.moreChanges = append(rollback.moreChanges, change)
}

func (rollback *straightEdgeFallbackRollback) restore() {
	for index := len(rollback.moreChanges) - 1; index >= 0; index-- {
		change := rollback.moreChanges[index]
		change.edge.Points = change.points
	}
	if rollback.hasChange {
		rollback.firstChange.edge.Points = rollback.firstChange.points
	}
	if rollback.costsCaptured {
		rollback.graph.RestoreRoutingCosts(rollback.costs)
	}
}

func isStraightEdgeFallbackCandidate(edge *layoutgraph.Edge, clusterNodes map[*layoutgraph.Node]bool, isTreeEdge map[*layoutgraph.Edge]bool) bool {
	if _, has := clusterNodes[edge.From]; has {
		return false
	}
	if _, has := clusterNodes[edge.To]; has {
		return false
	}
	if _, is := isTreeEdge[edge]; is {
		return false
	}
	if edge.From.Hierarchy != nil {
		return false
	}
	return !edge.HasTableColumn()
}

func StraightEdgesFallback(ctx context.Context, g *layoutgraph.Graph) error {
	if err := checkEdgeRoutingCanceled(ctx); err != nil {
		return err
	}

	clusterNodes := make(map[*layoutgraph.Node]bool)
	for _, cluster := range g.Clusters {
		if err := checkEdgeRoutingCanceled(ctx); err != nil {
			return err
		}
		for _, cn := range cluster.Nodes {
			if err := checkEdgeRoutingCanceled(ctx); err != nil {
				return err
			}
			clusterNodes[cn] = true
		}
	}
	isTreeEdge := g.TreeEdgeMap()
	g.AddIsolatedTreeEdges(isTreeEdge)
	if err := checkEdgeRoutingCanceled(ctx); err != nil {
		return err
	}
	for index, edge := range g.Edges {
		if err := checkEdgeRoutingCanceled(ctx); err != nil {
			return err
		}
		if !isStraightEdgeFallbackCandidate(edge, clusterNodes, isTreeEdge) {
			continue
		}
		return straightEdgesFallbackFrom(ctx, g, clusterNodes, isTreeEdge, index)
	}
	return nil
}

func straightEdgesFallbackFrom(
	ctx context.Context,
	g *layoutgraph.Graph,
	clusterNodes map[*layoutgraph.Node]bool,
	isTreeEdge map[*layoutgraph.Edge]bool,
	firstIndex int,
) error {
	var rollback straightEdgeFallbackRollback
	rollback.captureCosts(g)
	complete := false
	defer func() {
		if !complete {
			rollback.restore()
		}
	}()

	for offset, edge := range g.Edges[firstIndex:] {
		if offset != 0 {
			if err := checkEdgeRoutingCanceled(ctx); err != nil {
				return err
			}
			if !isStraightEdgeFallbackCandidate(edge, clusterNodes, isTreeEdge) {
				continue
			}
		}
		originalPoints := edge.Points
		changed, err := tryStraightEdgeFallback(ctx, g, edge)
		if changed {
			rollback.record(edge, originalPoints)
		}
		if err != nil {
			return err
		}
	}
	if err := checkEdgeRoutingCanceled(ctx); err != nil {
		return err
	}
	complete = true
	return nil
}

func tryStraightEdgeFallback(ctx context.Context, g *layoutgraph.Graph, edge *layoutgraph.Edge) (bool, error) {
	if err := checkEdgeRoutingCanceled(ctx); err != nil {
		return false, err
	}

	originalCost := estimateRouteCost(layoutgraph.Edges(g.Edges), edge)
	fromPort, toPort, lineCost, err := routeLine(ctx, g, edge, g.Edges, nil)
	if err != nil {
		if layoutgraph.IsCandidateRejection(err) {
			// A straight replacement is optional. Keep the original route when
			// no valid line exists.
			return false, nil
		}
		return false, err
	}

	// If dejitter fails but the segment is really short, we would prefer a straight line (all else equivalent)
	if len(edge.Points) == 4 {
		if geo.EuclideanDistance(
			edge.Points[1].X,
			edge.Points[1].Y,
			edge.Points[2].X,
			edge.Points[2].Y,
		) <= lowerJitterThreshold {
			// remove the straight line penalty from this line
			lineCost /= nonOrthogonalFactor
		}
	}

	if err := checkEdgeRoutingCanceled(ctx); err != nil {
		return false, err
	}
	if lineCost < originalCost {
		edge.Points = []*geo.Point{fromPort, toPort}
		return true, nil
	}
	return false, nil
}

// true if angle between line and border (based on orientation) is less than min angle
func hasSharpAngleToBorder(fromPoint, toPoint *geo.Point, orientation geo.Orientation) bool {
	switch {
	case orientation == geo.Top || orientation == geo.Bottom:
		// border is to bottom
		//    \       /
		//     \ ok  /
		//      \   /
		//  min˚ \ / min˚
		// =====border====
		//  min˚ / \ min˚
		//      /   \
		//     / ok  \
		// border is to top
		dx := toPoint.X - fromPoint.X
		dy := toPoint.Y - fromPoint.Y
		angle := math.Abs(180 * math.Atan2(dy, dx) / math.Pi)
		if angle > 90 {
			angle = 180 - angle
		}
		return angle < minStraightEdgeAngle
	case orientation == geo.Left || orientation == geo.Right:
		//    border
		//   \  ||  /
		//    \˚||˚/
		//right\||/ left
		// ok  /||\  ok
		//    / || \
		//   / ˚||˚ \
		dx := toPoint.X - fromPoint.X
		dy := toPoint.Y - fromPoint.Y
		angle := math.Abs(180 * math.Atan2(dx, dy) / math.Pi)
		if angle > 90 {
			angle = 180 - angle
		}
		return angle < minStraightEdgeAngle
	}
	return false
}

func ReorderDuplicates(ctx context.Context, g *layoutgraph.Graph) error {
	return reorderDuplicatesInEdges(ctx, g.Edges)
}

func reorderDuplicatesInEdges(ctx context.Context, edges []*layoutgraph.Edge) error {
	checkCanceled := func() error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ReorderDuplicates: %w", err)
		}
		return nil
	}
	if err := checkCanceled(); err != nil {
		return err
	}
	type routeSnapshot struct {
		edge   *layoutgraph.Edge
		points []*geo.Point
		values []*geo.Point
	}
	snapshots := make([]routeSnapshot, 0, len(edges))
	seen := make(map[*layoutgraph.Edge]struct{}, len(edges))
	for _, edge := range edges {
		if _, ok := seen[edge]; ok {
			continue
		}
		seen[edge] = struct{}{}
		snapshots = append(snapshots, routeSnapshot{
			edge:   edge,
			points: edge.Points,
			values: append([]*geo.Point(nil), edge.Points...),
		})
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		for _, snapshot := range snapshots {
			copy(snapshot.points, snapshot.values)
			snapshot.edge.Points = snapshot.points
		}
	}()

	// look for labeled straight edges that have duplicates
	for _, e := range edges {
		if err := checkCanceled(); err != nil {
			return err
		}
		if e.HasTableColumn() {
			continue
		}
		if e.Label == nil || !e.IsStraight() {
			continue
		}

		hasOverlappingEnd := e.HasOverlappingEnd()

		duplicates := []int{}
		for i, other := range edges {
			if err := checkCanceled(); err != nil {
				return err
			}
			if other == e {
				continue
			}
			if !other.IsDuplicateOf(e) || !other.IsStraight() {
				continue
			}
			// Note: only need to check for matching arrowheads if one of the edges overlaps another edge
			if hasOverlappingEnd || other.HasOverlappingEnd() {
				sourceArrow := e.SourceArrowhead
				targetArrow := e.TargetArrowhead
				if other.From == e.To {
					// Note: since other is in opposite direction, we reverse points when swapping below
					sourceArrow, targetArrow = targetArrow, sourceArrow
				}
				if other.TargetArrowhead == targetArrow && other.SourceArrowhead == sourceArrow {
					duplicates = append(duplicates, i)
				}
			} else {
				duplicates = append(duplicates, i)
			}
		}

		if len(duplicates) > 1 {
			start := e.Points[0]
			end := e.Points[len(e.Points)-1]

			dx := end.X - start.X
			dy := end.Y - start.Y

			sourceOrder := make([]*layoutgraph.Edge, 0, len(duplicates)+1)
			targetOrder := make([]*layoutgraph.Edge, 0, len(duplicates)+1)

			sourceOrder = append(sourceOrder, e)
			targetOrder = append(targetOrder, e)

			for _, i := range duplicates {
				dup := edges[i]
				sourceOrder = append(sourceOrder, dup)
				targetOrder = append(targetOrder, dup)
			}

			if dy > dx {
				// . ┌───────┐
				// . │       │
				// . └─┬─┬─┬─┘
				// .   0 1 2 source order
				// .   │ │ │
				// .   0 1 2 target order
				// . ┌─▼─▼─▼─┐
				// . │       │
				// . └───────┘
				sort.Slice(sourceOrder, func(i, j int) bool {
					return sourceOrder[i].Points[0].X < sourceOrder[j].Points[0].X
				})
				sort.Slice(targetOrder, func(i, j int) bool {
					ei, ej := targetOrder[i], targetOrder[j]
					return ei.Points[len(ei.Points)-1].X < ej.Points[len(ej.Points)-1].X
				})
			} else {
				sort.Slice(sourceOrder, func(i, j int) bool {
					return sourceOrder[i].Points[0].Y < sourceOrder[j].Points[0].Y
				})
				sort.Slice(targetOrder, func(i, j int) bool {
					ei, ej := targetOrder[i], targetOrder[j]
					return ei.Points[len(ei.Points)-1].Y < ej.Points[len(ej.Points)-1].Y
				})
			}
			if err := checkCanceled(); err != nil {
				return err
			}

			consistentOrder := true
			for i := range sourceOrder {
				if sourceOrder[i] != targetOrder[i] {
					consistentOrder = false
					break
				}
			}
			if !consistentOrder {
				continue
			}
			currentIndex := -1
			for i := range sourceOrder {
				if sourceOrder[i] == e {
					currentIndex = i
					break
				}
			}
			if currentIndex == 0 || currentIndex == len(sourceOrder)-1 {
				continue
			}
			first := sourceOrder[0]
			last := sourceOrder[len(sourceOrder)-1]
			if first.Label != nil && last.Label != nil {
				continue
			}
			if err := checkCanceled(); err != nil {
				return err
			}

			if last.Label == nil {
				e.Points, last.Points = last.Points, e.Points
				if last.From == e.To {
					// need to reverse order of points
					for i, j := 0, len(e.Points)-1; i < j; i, j = i+1, j-1 {
						e.Points[i], e.Points[j] = e.Points[j], e.Points[i]
					}
					for i, j := 0, len(last.Points)-1; i < j; i, j = i+1, j-1 {
						last.Points[i], last.Points[j] = last.Points[j], last.Points[i]
					}
				}
			} else if first.Label == nil {
				e.Points, first.Points = first.Points, e.Points
				if first.From == e.To {
					for i, j := 0, len(e.Points)-1; i < j; i, j = i+1, j-1 {
						e.Points[i], e.Points[j] = e.Points[j], e.Points[i]
					}
					for i, j := 0, len(first.Points)-1; i < j; i, j = i+1, j-1 {
						first.Points[i], first.Points[j] = first.Points[j], first.Points[i]
					}
				}
			}
		}
	}
	if err := checkCanceled(); err != nil {
		return err
	}
	complete = true
	return nil
}

func Crosshatch(ctx context.Context, g *layoutgraph.Graph) error {
	return crosshatchWithWorkLimit(ctx, g, maxRouteStageWorkUnits)
}

func crosshatchWithWorkLimit(ctx context.Context, g *layoutgraph.Graph, workLimit uint64) error {
	return runAtomicRouteStage(ctx, "Crosshatch", g, nil, workLimit, func(guard *routeWorkGuard) error {
		vessels := make([]*layoutgraph.Node, 0, len(g.Clusters))
		for vessel := range g.Clusters {
			if err := guard.step(); err != nil {
				return err
			}
			vessels = append(vessels, vessel)
		}
		layoutgraph.SortNodesByID(vessels)
		for _, vessel := range vessels {
			if err := guard.step(); err != nil {
				return err
			}
			cluster := g.Clusters[vessel]
			if cluster == nil {
				return invariant.New("nil cluster while crosshatching routes")
			}
			externalEdges, err := externalEdgesForClusterGuarded(cluster, guard)
			if err != nil {
				return err
			}
			if len(externalEdges) < 2 {
				continue
			}

			portToEdges, err := groupEdgesByClusterPortGuarded(cluster, externalEdges, guard)
			if err != nil {
				return err
			}
			ports := make([]geo.Point, 0, len(portToEdges))
			for port := range portToEdges {
				if err := guard.step(); err != nil {
					return err
				}
				ports = append(ports, port)
			}
			sort.Slice(ports, func(i, j int) bool {
				if ports[i].X != ports[j].X {
					return ports[i].X < ports[j].X
				}
				return ports[i].Y < ports[j].Y
			})
			for _, port := range ports {
				edges := portToEdges[port]
				if len(edges) < 2 {
					continue
				}
				for _, edge := range edges {
					if err := convertToStraightLineGuarded(g, edge, guard); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func externalEdgesForClusterGuarded(cluster *layoutgraph.Cluster, guard *routeWorkGuard) ([]*layoutgraph.Edge, error) {
	var externalEdges []*layoutgraph.Edge
	for _, edgeAbduction := range cluster.EdgeAbductions {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if edgeAbduction == nil || edgeAbduction.Edge == nil {
			return nil, invariant.New("nil cluster edge abduction while crosshatching routes")
		}
		externalEdges = append(externalEdges, edgeAbduction.Edge)
	}
	return externalEdges, nil
}

func groupEdgesByClusterPortGuarded(cluster *layoutgraph.Cluster, edges []*layoutgraph.Edge, guard *routeWorkGuard) (map[geo.Point][]*layoutgraph.Edge, error) {
	portToEdges := make(map[geo.Point][]*layoutgraph.Edge)

	for _, edge := range edges {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if edge == nil {
			return nil, invariant.New("nil edge while grouping cluster ports")
		}
		// Find which cluster node this edge connects to and get the port
		var clusterPort *geo.Point
		for _, clusterNode := range cluster.Nodes {
			if err := guard.step(); err != nil {
				return nil, err
			}
			if edge.From == clusterNode {
				clusterPort = edge.SourcePort()
				break
			} else if edge.To == clusterNode {
				clusterPort = edge.TargetPort()
				break
			}
		}

		if clusterPort != nil {
			portToEdges[*clusterPort] = append(portToEdges[*clusterPort], edge)
		}
	}

	return portToEdges, nil
}

func convertToStraightLineGuarded(g *layoutgraph.Graph, edge *layoutgraph.Edge, guard *routeWorkGuard) error {
	if err := guard.step(); err != nil {
		return err
	}
	if edge == nil || edge.From == nil || edge.To == nil || edge.From.TopLeft == nil || edge.To.TopLeft == nil {
		return invariant.New("invalid edge while crosshatching routes")
	}
	fromCenter := &geo.Point{
		X: edge.From.TopLeft.X + edge.From.Width/2,
		Y: edge.From.TopLeft.Y + edge.From.Height/2,
	}
	toCenter := &geo.Point{
		X: edge.To.TopLeft.X + edge.To.Width/2,
		Y: edge.To.TopLeft.Y + edge.To.Height/2,
	}

	fromBorder := findLineBorderIntersection(fromCenter, toCenter, edge.From)
	toBorder := findLineBorderIntersection(toCenter, fromCenter, edge.To)

	if fromBorder != nil && toBorder != nil {
		intersects, err := straightLineIntersectsNonAncestorNodesGuarded(g, edge, fromBorder, toBorder, guard)
		if err != nil {
			return err
		}
		if intersects {
			return nil
		}
		if err := guard.finish(); err != nil {
			return err
		}
		edge.Points = []*geo.Point{fromBorder, toBorder}
		if err := guard.finish(); err != nil {
			return err
		}
	}
	return nil
}

// findLineBorderIntersection finds where a line from nodeCenter towards targetPoint intersects the node's border
func findLineBorderIntersection(nodeCenter, targetPoint *geo.Point, node *layoutgraph.Node) *geo.Point {
	// Calculate direction vector
	dx := targetPoint.X - nodeCenter.X
	dy := targetPoint.Y - nodeCenter.Y

	if dx == 0 && dy == 0 {
		return nodeCenter
	}

	// Node boundaries
	left := node.TopLeft.X
	right := node.TopLeft.X + node.Width
	top := node.TopLeft.Y
	bottom := node.TopLeft.Y + node.Height

	var t float64 = math.Inf(1)

	// Check intersection with each edge of the rectangle
	if dx > 0 {
		// Right edge intersection
		t = math.Min(t, (right-nodeCenter.X)/dx)
	} else if dx < 0 {
		// Left edge intersection
		t = math.Min(t, (left-nodeCenter.X)/dx)
	}

	if dy > 0 {
		// Bottom edge intersection
		t = math.Min(t, (bottom-nodeCenter.Y)/dy)
	} else if dy < 0 {
		// Top edge intersection
		t = math.Min(t, (top-nodeCenter.Y)/dy)
	}

	if t == math.Inf(1) || t <= 0 {
		return nodeCenter
	}

	return &geo.Point{
		X: nodeCenter.X + t*dx,
		Y: nodeCenter.Y + t*dy,
	}
}

// straightLineIntersectsNonAncestorNodes checks if a straight line between two points
// intersects any nodes that are not ancestors of the edge's endpoints
func straightLineIntersectsNonAncestorNodesGuarded(g *layoutgraph.Graph, edge *layoutgraph.Edge, p1, p2 *geo.Point, guard *routeWorkGuard) (bool, error) {
	// Check intersection with all nodes in the graph
	for _, node := range g.Nodes {
		if err := guard.step(); err != nil {
			return false, err
		}
		// Skip if this node is an ancestor of either endpoint
		fromDescendant, err := isDescendantOfWithRouteGuard(edge.From, node, guard)
		if err != nil {
			return false, err
		}
		toDescendant, err := isDescendantOfWithRouteGuard(edge.To, node, guard)
		if err != nil {
			return false, err
		}
		if fromDescendant || toDescendant {
			continue
		}

		// Skip if this node is one of the endpoints
		if node == edge.From || node == edge.To {
			continue
		}

		// Check if the straight line passes through this node
		if node.PassesThrough(p1, p2) {
			return true, nil
		}
	}

	return false, nil
}
