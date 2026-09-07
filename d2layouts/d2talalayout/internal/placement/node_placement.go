package placement

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/loops"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/packing"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/proximity"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/trees"
	"github.com/d2lang/d2/lib/geo"
)

// placeNodes is phase 1 of the algorithm
func placeNodes(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, randomSeed int64, prevEdgeAbductions []*layoutgraph.EdgeAbduction, ancestorObstacles []geo.Box) error {
	// Edge cases
	if len(g.Nodes) == 0 {
		return nil
	}

	if len(g.Nodes) == 1 {
		node := g.Nodes[0]
		if node.FixedTopLeft != nil {
			node.TopLeft = node.FixedTopLeft.Copy()
		} else {
			node.TopLeft = geo.NewPoint(0, 0)
		}
		g.SyncNestedGeometry()
		// there may be trees that need to be reconnected to this node
		if err := trees.Place(ctx, g, root); err != nil {
			return err
		}
		return direct(ctx, g, g.Nodes, root, directOptions{})
	}

	// This graph is one undergoing node placement in this call
	childrenGraph := layoutgraph.NewGraph()
	childrenGraph.CopyEntitiesFrom(g)

	// Add every direct child of the current root to the new graph
	for _, child := range g.Containers[root] {
		childrenGraph.AddNodeUnchecked(child)
	}
	if len(childrenGraph.Nodes) == 0 {
		return nil
	}

	// When performing layout for a fixed child A, if child B is also fixed it creates an obstacle to try to avoid for A's children layout
	// .     ┌─A@(10,0)───────────────────┐  │    ┌─A@(10,0)─┐
	// .     │                            │  │    │          │
	// .     │  ┌────┐   ┌────┐   ┌────┐  │  │    │  ┌────┐  │
	// .     │  │    ├───►    ├───►    │  │  │    │  │    │  │
	// .     │  └────┘   └────┘   └────┘  │  │    │  └─┬──┘  │
	// .     │                            │  │    │  ┌─▼──┐  │
	// .     └────────────────────────────┘  │    │  │    │  │
	// .                                     │    │  └──┬─┘  │
	// . ┌─B@(0,100)─┐                       │ ┌─B@(0,100)─┐ │
	// . │           │                       │ │  │  ┌──▼─┐│ │
	// . │           │ A's PREFERRED LAYOUT  │ │  │  │    ││ │ A's LAYOUT TO AVOID
	// . │           │                       │ │  │  └────┘│ │
	// . └───────────┘                       │ └──└────────┘─┘
	// we want to pass each child these other obstacles to try to avoid
	// Note: it isn't always possible to avoid overlaps depending on the fixed positions
	// Note: we don't know final sizes of A and B (dependencies: B size -> B children layout -> A size -> A children layout -> B size -> ...)
	var fixedObstacles []geo.Box
	for _, n := range layoutgraph.Nodes(g.Containers[root]).FixedNodes() {
		// TODO improve by computing minimum width/height (width >= widest child + padding, width >= rightmost fixed child)
		box := geo.Box{TopLeft: n.FixedTopLeft.Copy(), Width: n.Width, Height: n.Height}
		fixedObstacles = append(fixedObstacles, box)
	}

	// we want to take the ancestor obstacles for layout at this level, and translate them to be relative to this root node
	if root != nil && root.FixedTopLeft != nil && len(ancestorObstacles) > 1 {
		dx, dy := -root.FixedTopLeft.X-layoutgraph.ContainerPadding, -root.FixedTopLeft.Y-layoutgraph.ContainerPadding
		if dx != 0 || dy != 0 {
			for _, b := range ancestorObstacles {
				b.TopLeft.X += dx
				b.TopLeft.Y += dy
			}
			defer func() {
				for _, b := range ancestorObstacles {
					b.TopLeft.X -= dx
					b.TopLeft.Y -= dy
				}
			}()
		}
	}

	edgeAbductions := g.AbductEdges(root, childrenGraph)
	abductionsRestored := false
	defer func() {
		if abductionsRestored {
			return
		}
		g.RestoreEdgeAbductions(edgeAbductions)
		layoutgraph.Nodes(g.Nodes).SetGraphReference(g)
	}()

	childrenOrder, err := PlaceChildrenOrder(ctx, g.Containers[root], edgeAbductions)
	if err != nil {
		return err
	}
	for _, child := range childrenOrder {
		if child.Hierarchy != nil {
			continue
		}
		// If the direct child is a container with more children, then recurse first
		// So we go from most nested to least (root)
		if child.IsContainer() {
			if err := placeNodes(ctx, g, child, randomSeed, edgeAbductions, fixedObstacles); err != nil {
				return err
			}
			loops.UpdateOffsets(child)
			// descendant may have placed trees so we need to update containers map
			childrenGraph.Containers = g.Containers
		}
		if child.IsClusterVessel() {
			for _, n := range g.Clusters[child].Nodes {
				if n.IsContainer() {
					if err := placeNodes(ctx, g, n, randomSeed, edgeAbductions, fixedObstacles); err != nil {
						return err
					}
					childrenGraph.Containers = g.Containers
				}
			}
		}
	}

	// we don't know how the subgraphs will be combined (unless it is only one),
	// but we can try to layout each subgraph assuming it is the only one and therefore it should try to avoid fixed ancestors
	// then when we combine subgraphs, we can also avoid fixed ancestors

	if err := proximity.AssignNears(ctx, childrenGraph, root, prevEdgeAbductions); err != nil {
		return err
	}
	if err := proximity.AssignHerds(ctx, childrenGraph, root, prevEdgeAbductions); err != nil {
		return err
	}
	subgraphs, splitOwnership, err := childrenGraph.SplitSubgraphsTracked(ctx, layoutgraph.SplitOptions{
		IncludeNears:  true,
		TraverseTrees: true,
	}, nil)
	if err != nil {
		return err
	}
	splitCommitted := false
	defer func() {
		if !splitCommitted {
			// SplitSubgraphs redirects every reachable node to a temporary graph,
			// including Near-only nodes outside g.Nodes. Restore their exact prior
			// owners if any later placement work fails or panics.
			splitOwnership.Restore()
		}
	}()
	for _, subgraph := range subgraphs {
		subgraph.ComputeCellSize()
		layoutgraph.

			// Put all the nested nodes Graph reference as the subgraph
			// There are calls that use the node's graph reference
			Nodes(subgraph.Nodes).SetGraphReference(subgraph)

		// TODO: this must be a better check, like only select the nodes that must be placed
		// probably, we want to skip placing nodes that were already placed
		if subgraph.Nodes[0].Hierarchy == nil {
			rng := rand.New(rand.NewSource(randomSeed))
			if err := placeNodesOrthogonally(ctx, root, subgraph, edgeAbductions, rng, ancestorObstacles, randomSeed); err != nil {
				return err
			}
			for _, n := range subgraph.Nodes {
				if n.IsClusterVessel() {
					if _, err := optimizeCluster(ctx, subgraph.Clusters[n], true); err != nil {
						return err
					}
				}
			}
		}

		if err := trees.Place(ctx, subgraph, root); err != nil {
			return err
		}
		// Tree nodes and edges have been restored within subgraph but we need to also add these back to g
		for _, n := range subgraph.Nodes {
			for _, treeRoot := range g.Trees[n] {
				for _, tree := range append(trees.Descendants(treeRoot), treeRoot) {
					g.AddNodeUnchecked(tree.Node)
					g.AddEdge(tree.SentinelEdge)
				}
			}
		}
		// reconnectTree restores the node to subgraph.Containers[root] but if the slice grows from an append,
		// subgraph.Containers[root] will be a different slice than g.Containers[root] so we need to sync the entry in g.Containers
		g.Containers[root] = subgraph.Containers[root]
		for _, otherSubgraph := range subgraphs {
			otherSubgraph.Containers[root] = subgraph.Containers[root]
		}

		// To determine within mirroring which container contents are reachable and should be recursively mirrored,
		// the edges need to be temporarily restored
		subgraph.RestoreEdgeAbductions(edgeAbductions)
		directErr := direct(ctx, subgraph, subgraph.Nodes, root, directOptions{})
		subgraph.ApplyEdgeAbductions(edgeAbductions)
		if directErr != nil {
			return directErr
		}

		if err := validatePlacedNodes(root, subgraph.Nodes); err != nil {
			return err
		}
	}
	combined, err := packing.CombineSubgraphs(ctx, g, subgraphs, ancestorObstacles)
	if err != nil {
		return err
	}

	if err := orientSourceInterior(ctx, combined, root, ancestorObstacles); err != nil {
		return err
	}
	// Set the dimensions of the root according to outcome of node placement
	if root != nil {
		root.FitToGraph(combined, g.ContainerPadding(root, false))
		// The root may be a cluster node, in which case all the other cluster nodes have to also be resized
		for _, c := range g.Clusters {
			is := slices.Contains(c.Nodes, root)
			if is {
				c.Resize(c.Vessel)
			}
		}
	}

	g.RestoreEdgeAbductions(edgeAbductions)
	layoutgraph.Nodes(g.Nodes).SetGraphReference(g)
	abductionsRestored = true
	splitCommitted = true

	return nil
}

// This routine is heavily based on the algorithm proposed in Graph Compact Orthogonal Layout Algorithm by Freivalds and Glagolevs
func placeNodesOrthogonally(ctx context.Context, root *layoutgraph.Node, subgraph *layoutgraph.Graph,
	edgeAbductions []*layoutgraph.EdgeAbduction, randGenerator *rand.Rand,
	obstacles []geo.Box, seed ...int64) error {
	fixed := subgraph.FixedNodes()
	initialized := false
	var err error
	if len(seed) > 0 && seed[0]%2 == 0 {
		initialized, err = initializeByGraphDistance(ctx, subgraph)
	}
	if err == nil && !initialized {
		err = initializeNodes(ctx, subgraph)
	}
	if err != nil {
		return err
	}
	// After nodes in this nesting level have been initialized, the inner cluster nodes and container children need to also move
	subgraph.SyncNestedGeometry()

	numIterations := int(float64(nodePlacementIterations) * math.Sqrt(float64(len(subgraph.Nodes))))
	temp := 2 * math.Sqrt(float64(len(subgraph.Nodes)))
	coolingFactor := math.Pow(0.2/temp, 1.0/float64(numIterations))

	// run compaction every 9 iterations alternating between horizontal and vertical compaction
	compactionIteration := 9
	compactionAxis := horizontalAxis

	sizeless, err := newSizelessOptimizer(ctx, subgraph, randGenerator)
	if err != nil {
		return err
	}
	for i := 0; i < numIterations/2; i++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("PlaceNodes: %w", err)
		}
		if err := sizeless.optimize(ctx, temp); err != nil {
			return err
		}

		if (i % compactionIteration) == 0 {
			if err := compaction(ctx, subgraph, compactionOptions{
				axis:   compactionAxis,
				factor: compactionFactor,
			}); err != nil {
				return err
			}
			compactionAxis = compactionAxis.opposite()
			sizeless.resetOccupied()
		}
		temp = temp * coolingFactor
	}

	if err := compaction(ctx, subgraph, compactionOptions{
		edgeAbductions: edgeAbductions,
		axis:           horizontalAxis,
		includeSizes:   true,
		factor:         compactionFactor,
		transition:     true,
	}); err != nil {
		return err
	}
	if err := compaction(ctx, subgraph, compactionOptions{
		edgeAbductions: edgeAbductions,
		axis:           verticalAxis,
		includeSizes:   true,
		factor:         compactionFactor,
		transition:     true,
	}); err != nil {
		return err
	}

	// if there are fixed nodes, we want to shift them to their fixed top left positions
	if len(fixed) > 0 {
		dx := fixed[0].FixedTopLeft.X - fixed[0].TopLeft.X
		dy := fixed[0].FixedTopLeft.Y - fixed[0].TopLeft.Y
		for _, n := range subgraph.Nodes {
			n.MoveWithChildren(dx, dy)
		}
		for _, n := range fixed {
			// adjust precisely ignoring cell size alignment
			n.MoveAbsWithChildren(n.FixedTopLeft.X, n.FixedTopLeft.Y)
		}
	}

	if err := validateCellSize(subgraph); err != nil {
		return err
	}
	for _, n := range subgraph.Nodes {
		if n.FixedTopLeft != nil {
			continue
		}

		x := n.TopLeft.X
		y := n.TopLeft.Y
		if math.Mod(n.TopLeft.X, subgraph.CellSize) != 0 {
			x = roundToPreviousCellSize(n.TopLeft.X, subgraph.CellSize)
		}
		if math.Mod(n.TopLeft.Y, subgraph.CellSize) != 0 {
			y = roundToPreviousCellSize(n.TopLeft.Y, subgraph.CellSize)
		}
		if x != n.TopLeft.X || y != n.TopLeft.Y {
			n.MoveAbsWithChildren(x, y)
		}
	}

	if err := validateGridAlignment(subgraph); err != nil {
		return err
	}

	// The turn cost is cached for edge length estimates, and we compute here
	subgraph.TurnCost()
	// Since the compaction factor is rather high, we estimate the overall maxLength would actually be ~half it
	subgraph.HalveTurnCost()

	sized, err := newSizedOptimizer(ctx, subgraph, root, edgeAbductions, randGenerator, obstacles)
	if err != nil {
		return err
	}

	proximity.SyncHerdFences(subgraph)
	for i := (numIterations / 2) + 1; i < numIterations; i++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("PlaceNodes: %w", err)
		}
		_, err := sized.optimize(ctx, temp)
		if err != nil {
			return err
		}

		if (i % compactionIteration) == 0 {
			factor := math.Max(1.0, 1.0+(2.0*float64(numIterations-i-30)/(0.5*float64(numIterations))))
			if err := compaction(ctx, subgraph, compactionOptions{
				edgeAbductions: edgeAbductions,
				axis:           compactionAxis,
				includeSizes:   true,
				factor:         factor,
			}); err != nil {
				return err
			}
			compactionAxis = compactionAxis.opposite()

			if err := grouping.JoinDistancedClusters(ctx, subgraph); err != nil {
				return err
			}
			proximity.SyncHerdFences(subgraph)
		}
		temp = temp * coolingFactor
	}
	if err := grouping.JoinDistancedClusters(ctx, subgraph); err != nil {
		return err
	}
	proximity.SyncHerdFences(subgraph)

	for i := 0; i < 10; i++ {
		changed, err := sized.optimize(ctx, 0.0)
		if err != nil {
			return err
		}
		if !changed {
			break
		}
	}

	if len(fixed) > 0 {
		dx := fixed[0].FixedTopLeft.X - fixed[0].TopLeft.X
		dy := fixed[0].FixedTopLeft.Y - fixed[0].TopLeft.Y
		for _, n := range subgraph.Nodes {
			n.MoveWithChildren(dx, dy)
		}
		for _, n := range fixed {
			// adjust precisely ignoring cell size alignment
			n.TopLeft = n.FixedTopLeft.Copy()
		}
	}
	return nil
}

func validatePlacedNodes(root *layoutgraph.Node, nodes []*layoutgraph.Node) error {
	for _, node := range nodes {
		if node.TopLeft == nil {
			return invariant.Errorf("node %d was not placed under container %d", node.ID, root.IDValue())
		}
	}
	return nil
}

func validateGridAlignment(g *layoutgraph.Graph) error {
	if err := validateCellSize(g); err != nil {
		return err
	}
	for _, node := range g.Nodes {
		if node.FixedTopLeft != nil {
			continue
		}
		if node.TopLeft == nil {
			return invariant.Errorf("node %d has no position", node.ID)
		}
		if math.Mod(node.TopLeft.X, g.CellSize) != 0 || math.Mod(node.TopLeft.Y, g.CellSize) != 0 {
			return invariant.Errorf(
				"node %d at (%v, %v) is not aligned to cell size %v",
				node.ID,
				node.TopLeft.X,
				node.TopLeft.Y,
				g.CellSize,
			)
		}
	}
	return nil
}

func validateCellSize(g *layoutgraph.Graph) error {
	if g.CellSize < 1 || math.IsNaN(g.CellSize) || math.IsInf(g.CellSize, 0) || math.Trunc(g.CellSize) != g.CellSize {
		return invariant.Errorf("invalid cell size %v", g.CellSize)
	}
	return nil
}
func PlaceChildrenOrder(ctx context.Context, nodes []*layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction) ([]*layoutgraph.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("PlaceChildrenOrder: %w", err)
	}
	expected := make(map[*layoutgraph.Node]struct{}, len(nodes))
	connected := make(map[*layoutgraph.Node]map[*layoutgraph.Node]struct{}, len(nodes))
	for _, node := range nodes {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("PlaceChildrenOrder: %w", err)
		}
		if node == nil {
			return nil, invariant.New("container has a nil child")
		}
		if _, duplicate := expected[node]; duplicate {
			return nil, invariant.Errorf("container has duplicate child %s", node.DebugID())
		}
		expected[node] = struct{}{}
		connected[node] = make(map[*layoutgraph.Node]struct{})
	}
	for _, edgeAbduction := range edgeAbductions {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("PlaceChildrenOrder: %w", err)
		}
		if edgeAbduction == nil {
			return nil, invariant.New("child ordering has a nil edge abduction")
		}
		from := edgeAbduction.CurrentFrom
		to := edgeAbduction.CurrentTo
		if _, ok := expected[from]; ok {
			if _, adjacent := expected[to]; adjacent {
				connected[from][to] = struct{}{}
			}
		}
		if _, ok := expected[to]; ok {
			if _, adjacent := expected[from]; adjacent {
				connected[to][from] = struct{}{}
			}
		}
	}

	ordered := make([]*layoutgraph.Node, 0, len(nodes))
	orderedSet := make(map[*layoutgraph.Node]struct{}, len(nodes))
	appendNode := func(node *layoutgraph.Node) {
		ordered = append(ordered, node)
		orderedSet[node] = struct{}{}
		delete(connected, node)
	}
	for _, node := range nodes {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("PlaceChildrenOrder: %w", err)
		}
		if len(connected[node]) == 0 {
			appendNode(node)
		}
	}

	for len(ordered) < len(nodes) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("PlaceChildrenOrder: %w", err)
		}
		leastDegree := len(nodes) + 1
		var start *layoutgraph.Node
		for _, node := range nodes {
			adjacent, pending := connected[node]
			if pending && len(adjacent) < leastDegree {
				leastDegree = len(adjacent)
				start = node
			}
		}
		if start == nil {
			return nil, invariant.New("could not order all container children")
		}

		visited := make(map[*layoutgraph.Node]struct{})
		queue := []*layoutgraph.Node{start}
		for len(queue) > 0 {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("PlaceChildrenOrder: %w", err)
			}
			current := queue[0]
			queue = queue[1:]
			if _, seen := visited[current]; seen {
				continue
			}
			visited[current] = struct{}{}
			if _, alreadyOrdered := orderedSet[current]; alreadyOrdered {
				continue
			}
			appendNode(current)
			for _, edgeAbduction := range edgeAbductions {
				var adjacent *layoutgraph.Node
				if edgeAbduction.CurrentFrom == current {
					adjacent = edgeAbduction.CurrentTo
				} else if edgeAbduction.CurrentTo == current {
					adjacent = edgeAbduction.CurrentFrom
				}
				if _, ok := expected[adjacent]; ok {
					queue = append(queue, adjacent)
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("PlaceChildrenOrder: %w", err)
	}
	return ordered, nil
}

// syncHerdFences sets the Val of the herd to the bounds of the graph
// .   ┌─────┐
// .   │     │
// .   │     │
// .   │     │
// .   └─────┘
