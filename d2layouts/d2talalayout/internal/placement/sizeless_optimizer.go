package placement

import (
	"context"
	"fmt"
	"maps"
	"math"
	"math/rand"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
)

// sizelessOptimizer holds the state during the first part (sizeless) of the optimization routine described in
// Graph Compact Orthogonal Layout Algorithm by Freivalds and Glagolevs
type sizelessOptimizer struct {
	mutationScratch optimizerMutationSnapshot

	nodes         []*layoutgraph.Node
	g             *layoutgraph.Graph
	randGenerator *rand.Rand

	occupied map[geo.Point]*layoutgraph.Node
}

func newSizelessOptimizer(ctx context.Context, g *layoutgraph.Graph, randGenerator *rand.Rand) (*sizelessOptimizer, error) {
	if g == nil {
		return nil, fmt.Errorf("TALA sizelessOptimizer requires a graph")
	}
	if len(g.Nodes) > limits.MaxEngineNodes {
		return nil, fmt.Errorf("TALA sizelessOptimizer node count exceeds limit %d", limits.MaxEngineNodes)
	}
	if len(g.Edges) > limits.MaxEngineEdges {
		return nil, fmt.Errorf("TALA sizelessOptimizer edge count exceeds limit %d", limits.MaxEngineEdges)
	}
	guard, err := limits.NewOptimizationWorkGuard(ctx, "sizelessOptimizer.setup", limits.MaxOptimizationWorkUnits)
	if err != nil {
		return nil, err
	}
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node == nil || node.Graph == nil {
			return nil, fmt.Errorf("TALA sizelessOptimizer found a node without a graph")
		}
		if len(node.Edges) > limits.MaxEngineEdges || len(node.Nears) > limits.MaxEngineNodes {
			return nil, fmt.Errorf("TALA sizelessOptimizer node %s adjacency references exceed engine limits", node.DebugID())
		}
		if node.Graph.CellSize != g.CellSize {
			return nil, fmt.Errorf("mismatch of cell size, graph=%f, node=%f", g.CellSize, node.Graph.CellSize)
		}
	}
	optim := &sizelessOptimizer{
		nodes:         make([]*layoutgraph.Node, 0, len(g.Nodes)),
		g:             g,
		randGenerator: randGenerator,
	}
	optim.resetOccupied()
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		canOptimize, err := canOptimizeNodeGuarded(node, g, guard)
		if err != nil {
			return nil, err
		}
		if canOptimize {
			optim.nodes = append(optim.nodes, node)
		}
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return optim, nil
}

// resetOccupied resets the occpupied positions of the optimizer
// this is useful if the nodes changed their positions outside `optimize`
func (optim *sizelessOptimizer) resetOccupied() {
	optim.occupied = make(map[geo.Point]*layoutgraph.Node, len(optim.g.Nodes))
	for _, n := range optim.g.Nodes {
		if n.TopLeft == nil {
			continue
		}
		optim.occupied[*n.TopLeft] = n
	}
}

// optimize optimizes sizeless node placement following the routine describe in
// Graph Compact Orthogonal Layout Algorithm by Freivalds and Glagolevs
// Nodes are optimized in random order
// For each node:
// - Compute the median to its connected nodes (or `nears`)
// - Find the closest distance from this median point that is not occupied
// - Generate a set of points from median to (closest distance + 1)
// - Moves the node to the best position, if any
// - If there's no position improves edge length, tries to swap the node with adjacent nodes
func (optim *sizelessOptimizer) optimize(ctx context.Context, temp float64) (err error) {
	return optim.optimizeWithLimit(ctx, temp, limits.MaxOptimizationWorkUnits)
}

func (optim *sizelessOptimizer) optimizeWithLimit(ctx context.Context, temp float64, workLimit uint64) (err error) {
	guard, err := limits.NewOptimizationWorkGuard(ctx, "sizelessOptimizer.optimize", workLimit)
	if err != nil {
		return err
	}
	if optim == nil || optim.g == nil {
		return fmt.Errorf("TALA sizelessOptimizer.optimize requires an optimizer with a graph")
	}
	if optim.randGenerator == nil {
		return fmt.Errorf("TALA sizelessOptimizer.optimize requires a random generator")
	}
	defer optim.mutationScratch.release()
	snapshot, err := captureOptimizerMutationStateInto(optim.g, guard, &optim.mutationScratch)
	if err != nil {
		return err
	}
	occupiedRef := optim.occupied
	occupiedSnapshot := make(map[geo.Point]*layoutgraph.Node, len(optim.occupied))
	for point, node := range optim.occupied {
		if err := guard.Step(); err != nil {
			return err
		}
		occupiedSnapshot[point] = node
	}
	restoreOccupied := func() {
		if occupiedRef == nil {
			optim.occupied = nil
			return
		}
		clear(occupiedRef)
		maps.Copy(occupiedRef, occupiedSnapshot)
		optim.occupied = occupiedRef
	}
	complete := false
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot.restore()
			restoreOccupied()
			panic(recovered)
		}
		if !complete {
			snapshot.restore()
			restoreOccupied()
		}
	}()
	if err := optim.optimizeGuarded(ctx, temp, guard); err != nil {
		return err
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	complete = true
	return nil
}

func (optim *sizelessOptimizer) optimizeGuarded(ctx context.Context, temp float64, guard *limits.OptimizationWorkGuard) error {
	if len(optim.nodes) > limits.MaxEngineNodes {
		return fmt.Errorf("TALA sizelessOptimizer.optimize node count exceeds limit %d", limits.MaxEngineNodes)
	}

	nodeIndices := make([]int, len(optim.nodes))
	for i := 0; i < len(optim.nodes); i++ {
		if err := guard.Step(); err != nil {
			return err
		}
		nodeIndices[i] = i
	}
	if err := limits.Shuffle(nodeIndices, optim.randGenerator, guard); err != nil {
		return err
	}

	for _, nodeIndex := range nodeIndices {
		if err := guard.Step(); err != nil {
			return err
		}
		node := optim.nodes[nodeIndex]
		if node == nil || node.TopLeft == nil {
			return fmt.Errorf("TALA sizelessOptimizer.optimize found an unpositioned node")
		}

		// we should always be able to move to the original position
		delete(optim.occupied, *node.TopLeft)
		medianPoint, err := optim.medianPointGuarded(node, temp, guard)
		if err != nil {
			return err
		}
		distance, err := optim.findClosestUnoccupiedDistanceGuarded(node, medianPoint, guard)
		if err != nil {
			return fmt.Errorf("could not find an unoccupied distance for %s: %w", node.DebugID(), err)
		}
		points, err := optim.placementPointsGuarded(node, medianPoint, distance, guard)
		if err != nil {
			return err
		}
		if err := limits.Shuffle(points, optim.randGenerator, guard); err != nil {
			return err
		}

		moved, err := optim.moveNodeToBestGuarded(ctx, node, points, guard)
		if err != nil {
			return err
		}

		if !moved {
			bestSwapCandidate, err := optim.bestSwapCandidateGuarded(ctx, node, guard)
			if err != nil {
				return err
			}
			if bestSwapCandidate != nil {
				if err := optimizerSwapPositions(node, bestSwapCandidate, guard); err != nil {
					return err
				}
				optim.occupied[*bestSwapCandidate.TopLeft] = bestSwapCandidate
			}
		}
		optim.occupied[*node.TopLeft] = node
	}
	return guard.Finish()
}

func (optim *sizelessOptimizer) medianPointGuarded(node *layoutgraph.Node, temp float64, guard *limits.OptimizationWorkGuard) (*geo.Point, error) {
	medianX, medianY, err := optimizerMedianToNeighbors(node, false, nil, guard)
	if err != nil {
		return nil, err
	}

	cell := optim.g.CellSize
	fixedOrigin, err := optimizerFixedOrigin(optim.g, node.OwningContainer(), guard)
	if err != nil {
		return nil, err
	}
	if fixedOrigin != nil {
		if cell <= 0 || math.IsNaN(cell) || math.IsInf(cell, 0) {
			return nil, fmt.Errorf("TALA %s fixed-origin placement requires a finite positive cell size", guard.Location())
		}
		// we can't have node positions past the fixedOrigin so set a floor
		if medianX < fixedOrigin.X/cell {
			medianX = fixedOrigin.X / cell
			medianX += temp
		}
		if medianY < fixedOrigin.Y/cell {
			medianY = fixedOrigin.Y / cell
			medianY += temp
		}
	}

	medianX += (-temp + optim.randGenerator.Float64()*(2.0*temp))
	medianY += (-temp + optim.randGenerator.Float64()*(2.0*temp))

	if fixedOrigin != nil {
		medianX = math.Max(medianX, fixedOrigin.X/cell)
		medianY = math.Max(medianY, fixedOrigin.Y/cell)
	}

	return geo.NewPoint(math.Round(medianX), math.Round(medianY)), nil
}

// FindClosestUnoccupiedDistance finds the closest distance from `medianPoint` which is not occupied by any other node
// example with distance=3, we iterate points from least to most distance
// . 	      3
// . 	   3  2  3
// . 	3  2  1  2  3
// . 3──2──1──0──1──2──3
// . 	3  2  1  2  3
// . 	   3  2  3
// . 	      3
func (optim *sizelessOptimizer) FindClosestUnoccupiedDistance(ctx context.Context, node *layoutgraph.Node, medianPoint *geo.Point) (float64, error) {
	guard, err := limits.NewOptimizationWorkGuard(ctx, "sizelessOptimizer.optimize", limits.MaxOptimizationWorkUnits)
	if err != nil {
		return 0, err
	}
	return optim.findClosestUnoccupiedDistanceGuarded(node, medianPoint, guard)
}

func (optim *sizelessOptimizer) findClosestUnoccupiedDistanceGuarded(node *layoutgraph.Node, medianPoint *geo.Point, guard *limits.OptimizationWorkGuard) (float64, error) {
	if node == nil || node.Graph == nil || medianPoint == nil {
		return 0, fmt.Errorf("TALA %s placement search requires a node, graph, and median", guard.Location())
	}
	fixedOrigin, err := optimizerFixedOrigin(node.Graph, node.OwningContainer(), guard)
	if err != nil {
		return 0, err
	}
	if fixedOrigin != nil {
		if node.Graph.CellSize <= 0 || math.IsNaN(node.Graph.CellSize) || math.IsInf(node.Graph.CellSize, 0) {
			return 0, fmt.Errorf("TALA %s fixed-origin placement requires a finite positive cell size", guard.Location())
		}
		fixedOrigin.X = math.Round(fixedOrigin.X / node.Graph.CellSize)
		fixedOrigin.Y = math.Round(fixedOrigin.Y / node.Graph.CellSize)
	}
	pastOrigin := func(x, y float64) bool {
		return fixedOrigin != nil && (x < fixedOrigin.X || y < fixedOrigin.Y)
	}

	for curr := 0.; curr <= 100.; curr++ {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		// for points at the same distance we iterate right to left (& negative then positive):
		// . curr=0       =1            =2               =3        6
		// .                                   4                8     4
		// .                  2             6     2         10           2
		// .     ──0──     3─────0       7───────────0   12─────────────────0
		// .                  1             5     1          9           1
		// .                                   3                7     3
		// .                                                       5
		for x, y := curr, 0.; x >= -curr; x-- {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if y != 0 {
				p := geo.NewPoint(medianPoint.X+x, medianPoint.Y-y)
				if !pastOrigin(p.X, p.Y) && !optim.isOccupied(p) {
					return curr, nil
				}
			}
			p := geo.NewPoint(medianPoint.X+x, medianPoint.Y+y)
			if !pastOrigin(p.X, p.Y) && !optim.isOccupied(p) {
				return curr, nil
			}
			if x > 0 {
				y++
			} else {
				y--
			}
		}
	}
	return 0, fmt.Errorf("no unoccupied points within d=100 of p=%v", medianPoint)
}

func (optim *sizelessOptimizer) isOccupied(p *geo.Point) bool {
	_, occupied := optim.occupied[*p]
	return occupied
}

// placementPointsGuarded generates points up to minUnoccupiedDistance+1 from medianPoint.
// The resulting points contain only valid positions (unoccupied and positive if there are fixed position nodes)
// Example:
// minUnoccupiedDistance = 0
// # = medianPoint
// * = resulting points (3 points, the median is included)
// . n5 n1
// . n2    n4
// .    n3 #*  *
// .        *
func (optim *sizelessOptimizer) placementPointsGuarded(node *layoutgraph.Node, medianPoint *geo.Point, minUnoccupiedDistance float64, guard *limits.OptimizationWorkGuard) ([]*geo.Point, error) {
	if node == nil || node.Graph == nil || medianPoint == nil {
		return nil, fmt.Errorf("TALA %s placement generation requires a node, graph, and median", guard.Location())
	}
	if minUnoccupiedDistance < 0 || minUnoccupiedDistance > 100 || math.IsNaN(minUnoccupiedDistance) || math.IsInf(minUnoccupiedDistance, 0) {
		return nil, fmt.Errorf("TALA %s placement distance must be finite and within [0, 100]", guard.Location())
	}
	distance := minUnoccupiedDistance + 1
	capacity := numPointsWithinManhattanDistance(distance) + 1
	if capacity < 0 || capacity > maxOptimizerPlacementCandidates {
		return nil, fmt.Errorf("%w: TALA sizelessOptimizer.optimize placement candidates exceed limit %d", limits.ErrOptimizationResourceLimit, maxOptimizerPlacementCandidates)
	}
	points := make([]*geo.Point, 0, capacity)

	fixedOrigin, err := optimizerFixedOrigin(node.Graph, node.OwningContainer(), guard)
	if err != nil {
		return nil, err
	}
	if fixedOrigin != nil {
		if node.Graph.CellSize <= 0 || math.IsNaN(node.Graph.CellSize) || math.IsInf(node.Graph.CellSize, 0) {
			return nil, fmt.Errorf("TALA %s fixed-origin placement requires a finite positive cell size", guard.Location())
		}
		fixedOrigin.X = math.Round(fixedOrigin.X / node.Graph.CellSize)
		fixedOrigin.Y = math.Round(fixedOrigin.Y / node.Graph.CellSize)
	}
	pastOrigin := func(x, y float64) bool {
		return fixedOrigin != nil && (x < fixedOrigin.X || y < fixedOrigin.Y)
	}

	for x := distance; x >= -distance; x-- {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		for y := 0.0; y <= distance-math.Abs(x); y++ {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if y != 0 {
				point := geo.NewPoint(medianPoint.X+x, medianPoint.Y-y)
				if !pastOrigin(point.X, point.Y) && !optim.isOccupied(point) {
					points = append(points, point)
				}
			}
			point := geo.NewPoint(medianPoint.X+x, medianPoint.Y+y)
			if !pastOrigin(point.X, point.Y) && !optim.isOccupied(point) {
				points = append(points, point)
			}
		}
	}

	return points, nil
}

func (optim *sizelessOptimizer) moveNodeToBestGuarded(ctx context.Context, node *layoutgraph.Node, points []*geo.Point, guard *limits.OptimizationWorkGuard) (bool, error) {
	scorer := placementcost.NewNodeEdgeLengthScorer(node, placementcost.EdgeLengthOptions{
		EdgeAbductions: nil, IncludeNodeSizes: false,
		EnforceMinimumGap: false, PenalizeDirection: true,
	})
	defer scorer.Close()
	leastDistance := math.Inf(1)
	leastDistancePoint := node.TopLeft.Copy()
	currentNodeX := node.TopLeft.X
	currentNodeY := node.TopLeft.Y
	movement, err := captureOptimizerCandidateMovement(node, guard)
	if err != nil {
		return false, err
	}
	originalPositions := movement.positions
	complete := false
	defer func() {
		if !complete {
			restoreNodePositions(originalPositions)
		}
	}()

	for _, point := range points {
		if err := guard.Step(); err != nil {
			restoreNodePositions(originalPositions)
			return false, err
		}
		if point == nil {
			restoreNodePositions(originalPositions)
			return false, fmt.Errorf("TALA sizelessOptimizer.optimize found a nil placement point")
		}
		x := point.X
		y := point.Y
		if err := movement.moveAbs(x, y, guard); err != nil {
			restoreNodePositions(originalPositions)
			return false, err
		}
		if err := chargeOptimizerScoring(node, nil, false, guard); err != nil {
			restoreNodePositions(originalPositions)
			return false, err
		}
		edgeLength, err := scorer.Score(ctx)
		if err != nil {
			restoreNodePositions(originalPositions)
			return false, err
		}
		switch geo.PrecisionCompare(edgeLength, leastDistance, geo.PRECISION) {
		case -1:
			leastDistance = edgeLength
			leastDistancePoint = point
		case 0:
			if point.X == currentNodeX && point.Y == currentNodeY {
				leastDistance = edgeLength
				leastDistancePoint = point
			}
		}
	}

	if err := ctx.Err(); err != nil {
		restoreNodePositions(originalPositions)
		return false, fmt.Errorf("EdgeLength: %w", err)
	}
	if math.IsInf(leastDistance, 1) {
		restoreNodePositions(originalPositions)
		return false, fmt.Errorf("sizelessOptimizer.moveNodeToBest: could not find any placement")
	}

	if err := movement.moveAbs(leastDistancePoint.X, leastDistancePoint.Y, guard); err != nil {
		restoreNodePositions(originalPositions)
		return false, err
	}

	complete = true
	return (node.TopLeft.X != currentNodeX) || (node.TopLeft.Y != currentNodeY), nil
}

func (optim *sizelessOptimizer) bestSwapCandidateGuarded(ctx context.Context, node *layoutgraph.Node, guard *limits.OptimizationWorkGuard) (*layoutgraph.Node, error) {
	// TODO (Mon Sep 20 10:57:17 2021) These maybe should be true on enforcing gap size, since when we swap, we don't want resulting swap to be too squished
	if err := chargeOptimizerScoring(node, nil, false, guard); err != nil {
		return nil, err
	}
	minSwapL1, err := placementcost.NodeEdgeLength(ctx, node, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: false, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		return nil, err
	}
	var bestSwapCandidate *layoutgraph.Node = nil

	swapCandidates, err := optim.swapCandidatesGuarded(node, guard)
	if err != nil {
		return nil, err
	}
	if err := limits.Shuffle(swapCandidates, optim.randGenerator, guard); err != nil {
		return nil, err
	}

	for _, swapCandidate := range swapCandidates {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if err := chargeOptimizerScoring(swapCandidate, nil, false, guard); err != nil {
			return nil, err
		}
		currentL2, err := placementcost.NodeEdgeLength(ctx, swapCandidate, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: false, EnforceMinimumGap: false, PenalizeDirection: true})
		if err != nil {
			return nil, err
		}

		var swappedL1, swappedL2 float64
		err = withOptimizerPositionsSwapped(node, swapCandidate, guard, func() error {
			var scoreErr error
			if scoreErr = chargeOptimizerScoring(node, nil, false, guard); scoreErr != nil {
				return scoreErr
			}
			swappedL1, scoreErr = placementcost.NodeEdgeLength(ctx, node, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: false, EnforceMinimumGap: false, PenalizeDirection: true})
			if scoreErr != nil {
				return scoreErr
			}
			// No need to compute it if we already know it won't be a good swap.
			if geo.PrecisionCompare(swappedL1, minSwapL1, geo.PRECISION) < 0 {
				if scoreErr = chargeOptimizerScoring(swapCandidate, nil, false, guard); scoreErr != nil {
					return scoreErr
				}
				swappedL2, scoreErr = placementcost.NodeEdgeLength(ctx, swapCandidate, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: false, EnforceMinimumGap: false, PenalizeDirection: true})
				if scoreErr != nil {
					return scoreErr
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		// Both positions have to be better than before
		if geo.PrecisionCompare(swappedL1, minSwapL1, geo.PRECISION) < 0 && geo.PrecisionCompare(swappedL2, currentL2, geo.PRECISION) <= 0 {
			minSwapL1 = swappedL1
			bestSwapCandidate = swapCandidate
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("EdgeLength: %w", err)
	}
	return bestSwapCandidate, nil
}

// swapCandidatesGuarded finds the optimizable nodes adjacent to node.
// Adjacent to `node`:
// . 	           (node.X, node.Y-1)
// . (node.X-1, node.Y)  node  (node.X+1, node.Y)
// . 	           (node.X, node.Y+1)
func (optim *sizelessOptimizer) swapCandidatesGuarded(node *layoutgraph.Node, guard *limits.OptimizationWorkGuard) ([]*layoutgraph.Node, error) {
	adjacent := make([]*layoutgraph.Node, 0, 4)
	tl := node.TopLeft

	var adjacentErr error
	adjacentNode := func(xDiff, yDiff float64) *layoutgraph.Node {
		if err := guard.Step(); err != nil {
			adjacentErr = err
			return nil
		}
		if node, exists := optim.occupied[*geo.NewPoint(tl.X+xDiff, tl.Y+yDiff)]; exists {
			canOptimize, err := canOptimizeNodeGuarded(node, optim.g, guard)
			if err != nil {
				adjacentErr = err
				return nil
			}
			if canOptimize {
				return node
			}
		}
		return nil
	}

	if above := adjacentNode(0, -1); above != nil {
		adjacent = append(adjacent, above)
	}
	if below := adjacentNode(0, 1); below != nil {
		adjacent = append(adjacent, below)
	}
	if left := adjacentNode(-1, 0); left != nil {
		adjacent = append(adjacent, left)
	}
	if right := adjacentNode(1, 0); right != nil {
		adjacent = append(adjacent, right)
	}
	if adjacentErr != nil {
		return nil, adjacentErr
	}

	return adjacent, nil
}

// canOptimizeNodeGuarded reports whether a node can be optimized.
// In order to be optimizable, a node must have edges, or at least `nears`, can't have a fixed position and can't be in a tree
func canOptimizeNodeGuarded(node *layoutgraph.Node, g *layoutgraph.Graph, guard *limits.OptimizationWorkGuard) (bool, error) {
	if node == nil || g == nil {
		return false, fmt.Errorf("TALA %s found a nil optimizer node", guard.Location())
	}
	if len(node.Edges) == 0 {
		hasUsableNear := false
		for near := range node.Nears {
			if err := guard.Step(); err != nil {
				return false, err
			}
			usable, err := optimizerIsDescendantOf(near, node.Container, guard)
			if err != nil {
				return false, err
			}
			if usable {
				hasUsableNear = true
				break
			}
		}
		if !hasUsableNear {
			return false, nil
		}
	}
	if node.FixedTopLeft != nil {
		return false, nil
	}
	_, inTree := g.NodeToTree[node]
	return !inTree, nil
}
