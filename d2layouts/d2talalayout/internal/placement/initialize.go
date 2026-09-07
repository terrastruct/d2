package placement

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/lib/geo"
)

func initializeNodes(ctx context.Context, g *layoutgraph.Graph) (err error) {
	if err := layoutgraph.Validate(ctx, "InitializeNodes", g); err != nil {
		return err
	}
	if len(g.Nodes) == 0 {
		return nil
	}
	originalPositions, err := snapshotNodePositionsContext(ctx, "InitializeNodes", g.Nodes)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			restoreNodePositions(originalPositions)
		}
	}()
	reachabilityGuard, err := limits.NewWorkGuard(ctx, "InitializeNodesReachability", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	sizelessFactor := g.CellSize * compactionFactor

	init := func(node *layoutgraph.Node) error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("InitializeNodes: %w", err)
		}
		if node.TopLeft != nil {
			return nil
		}

		leastDistance := math.Inf(1)
		leastDistancePoint := node.TopLeft

		sizeless, err := newSizelessOptimizer(ctx, g, nil)
		if err != nil {
			return err
		}
		candidates, err := nodeCandidatePositions(ctx, node, g, sizeless)
		if err != nil {
			return err
		}
		for _, p := range candidates {
			_, isOccupied := occupied(g, p)
			if isOccupied {
				continue
			}

			node.TopLeft = p

			edgeLength, err := placementcost.NodeEdgeLength(ctx, node, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: false, EnforceMinimumGap: false, PenalizeDirection: true})
			if err != nil {
				node.TopLeft = nil
				return err
			}

			if edgeLength < leastDistance {
				leastDistance = edgeLength
				leastDistancePoint = p
			}

			node.TopLeft = nil
		}

		node.TopLeft = leastDistancePoint
		return nil
	}

	// the first subgraph has all the fixed node subgraphs (each may be disconnected)
	if g.HasFixedNode() {
		fixedNodes := g.FixedNodes()
		nodes := make([]*layoutgraph.Node, 0, len(fixedNodes))

		for _, fn := range fixedNodes {
			reachable, err := fn.AllReachableNodesContext(false, true, true, nil, reachabilityGuard)
			if err != nil {
				return err
			}
			nodes = append(nodes, reachable...)
			for _, n := range reachable {
				n.TopLeft = nil
			}
		}

		for _, fn := range fixedNodes {
			fn.TopLeft = geo.NewPoint(
				math.Ceil(fn.FixedTopLeft.X/sizelessFactor),
				math.Ceil(fn.FixedTopLeft.Y/sizelessFactor),
			)
		}

		for _, n := range nodes {
			if err := init(n); err != nil {
				return err
			}
		}
	} else {
		bfsOrder, err := g.Nodes[0].AllReachableNodesContext(false, true, true, nil, reachabilityGuard)
		if err != nil {
			return err
		}

		for _, n := range bfsOrder[1:] {
			n.TopLeft = nil
		}
		g.Nodes[0].TopLeft = geo.NewPoint(float64(len(g.Nodes)), float64(len(g.Nodes)))

		for _, node := range bfsOrder[1:] {
			if err := init(node); err != nil {
				return err
			}
		}
	}

	for _, node := range g.Nodes {
		if node.TopLeft == nil {
			return fmt.Errorf("missed initializing node %v", node.DebugID())
		}
	}

	if err := reachabilityGuard.Finish(); err != nil {
		return err
	}
	complete = true
	return nil
}

func nodeCandidatePositions(ctx context.Context, node *layoutgraph.Node, g *layoutgraph.Graph, opt *sizelessOptimizer) (positions []*geo.Point, err error) {
	floatMedianX, floatMedianY := medianToNeighbors(node, false, nil)
	medianX := math.Floor(floatMedianX)
	medianY := math.Floor(floatMedianY)
	d, err := opt.FindClosestUnoccupiedDistance(ctx, node, geo.NewPoint(medianX, medianY))
	if err != nil {
		return nil, err
	}
	topLeftX := medianX - d - 2
	topLeftY := medianY - d - 2
	bottomRightX := medianX + d + 2
	bottomRightY := medianY + d + 2
	if g.HasFixedNode() {
		topLeftX = max(topLeftX, 0)
		topLeftY = max(topLeftY, 0)
	}

	if node.IsMajorityTarget() {
		// if it's, mainly, a target node, give preference to place it to bottom-right
		for x := bottomRightX; x >= topLeftX; x-- {
			for y := bottomRightY; y >= topLeftY; y-- {
				positions = append(positions, geo.NewPoint(x, y))
			}
		}
	} else {
		for x := topLeftX; x <= bottomRightX; x++ {
			for y := topLeftY; y <= bottomRightY; y++ {
				positions = append(positions, geo.NewPoint(x, y))
			}
		}
	}

	return positions, nil
}
