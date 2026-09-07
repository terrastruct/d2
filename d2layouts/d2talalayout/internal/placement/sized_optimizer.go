package placement

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/proximity"
)

type sizedOptimizer struct {
	mutationScratch optimizerMutationSnapshot

	g                *layoutgraph.Graph
	edgeAbductions   []*layoutgraph.EdgeAbduction
	randGenerator    *rand.Rand
	fixedOrigin      *geo.Point
	symmetryCost     float64
	cellSize         float64
	obstacles        []geo.Box
	checkedPositions map[geo.Point]struct{}
	placementScratch placementPointsScratch
	spatialIndex     optimizerSpatialIndex
}

func iterPlacementsAroundPoint(node *layoutgraph.Node, x, y float64, minimizingSelf bool, apply func(x, y float64) bool) {
	width, height := node.Width, node.Height
	x *= node.Graph.CellSize
	y *= node.Graph.CellSize
	increment := node.Graph.CellSize
	if apply(x, y) || !minimizingSelf {
		return
	}
	for currentX := x - width; currentX <= x; currentX += increment {
		for currentY := y - height; currentY <= y; currentY += increment {
			if currentX == x && currentY == y {
				continue
			}
			if apply(currentX, currentY) {
				return
			}
		}
	}
}

// withHubSpokesSuppressed temporarily removes a hub's spoke edges while fn
// evaluates placements. The original edge slice and ordering are restored on
// success, error, or panic.
func withHubSpokesSuppressed(node *layoutgraph.Node, spokes []*layoutgraph.Node, guard *limits.OptimizationWorkGuard, fn func() error) (err error) {
	if node == nil {
		return fmt.Errorf("TALA LocalOptimize cannot suppress spokes on a nil hub")
	}
	if len(spokes) > limits.MaxEngineNodes || len(node.Edges) > limits.MaxEngineEdges {
		return fmt.Errorf("TALA LocalOptimize hub suppression inputs exceed engine limits")
	}
	originalEdges := node.Edges
	removed := make([]bool, len(originalEdges))
	defer func() {
		node.Edges = originalEdges
	}()

	for _, spoke := range spokes {
		if err := guard.Step(); err != nil {
			return err
		}
		if spoke == nil {
			return fmt.Errorf("TALA LocalOptimize found a nil hub spoke")
		}
		for i, edge := range originalEdges {
			if err := guard.Step(); err != nil {
				return err
			}
			if removed[i] {
				continue
			}
			if edge == nil || (edge.From != node && edge.To != node) {
				return fmt.Errorf("TALA LocalOptimize found a malformed hub edge")
			}
			if node.Adjacent(edge) == spoke {
				removed[i] = true
				break
			}
		}
	}
	workingEdges := make([]*layoutgraph.Edge, 0, len(originalEdges))
	for i, edge := range originalEdges {
		if err := guard.Step(); err != nil {
			return err
		}
		if !removed[i] {
			workingEdges = append(workingEdges, edge)
		}
	}
	node.Edges = workingEdges
	return fn()
}

func newSizedOptimizer(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, abductions []*layoutgraph.EdgeAbduction, randGenerator *rand.Rand, obstacles []geo.Box) (*sizedOptimizer, error) {
	if g == nil {
		return nil, fmt.Errorf("TALA LocalOptimize requires a graph")
	}
	if len(g.Nodes) > limits.MaxEngineNodes {
		return nil, fmt.Errorf("TALA LocalOptimize node count exceeds limit %d", limits.MaxEngineNodes)
	}
	if len(g.Edges) > limits.MaxEngineEdges {
		return nil, fmt.Errorf("TALA LocalOptimize edge count exceeds limit %d", limits.MaxEngineEdges)
	}
	if len(abductions) > limits.MaxEngineEdges {
		return nil, fmt.Errorf("TALA LocalOptimize edge abduction count exceeds limit %d", limits.MaxEngineEdges)
	}
	if int64(len(obstacles)) > layoutgraph.MaxTopologyReferences {
		return nil, fmt.Errorf("TALA LocalOptimize obstacle count exceeds limit %d", layoutgraph.MaxTopologyReferences)
	}
	if g.CellSize < 1 || math.IsNaN(g.CellSize) || math.IsInf(g.CellSize, 0) || math.Trunc(g.CellSize) != g.CellSize {
		return nil, fmt.Errorf("TALA LocalOptimize requires a finite positive integer cell size")
	}
	guard, err := limits.NewOptimizationWorkGuard(ctx, "LocalOptimizeSetup", limits.MaxOptimizationWorkUnits)
	if err != nil {
		return nil, err
	}
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node == nil || node.Graph == nil {
			return nil, fmt.Errorf("TALA LocalOptimize found a node without a graph")
		}
		if len(node.Edges) > limits.MaxEngineEdges || len(node.Nears) > limits.MaxEngineNodes {
			return nil, fmt.Errorf("TALA LocalOptimize node %s adjacency references exceed engine limits", node.DebugID())
		}
		if node.Graph.CellSize != g.CellSize {
			return nil, fmt.Errorf("cell size mismatch for node %s", node.DebugID())
		}
	}
	optim := &sizedOptimizer{
		g:              g,
		edgeAbductions: abductions,
		randGenerator:  randGenerator,
		fixedOrigin:    g.ContainerFixedOrigin(root),
		symmetryCost:   g.CellSize,
		cellSize:       g.CellSize,
		obstacles:      obstacles,
	}
	for _, n := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}

		// Pre-process all edge abductions that affect this node
		abductedEdges := make(map[*layoutgraph.Edge]struct{}, len(optim.edgeAbductions))
		adjReplacements := make(map[*layoutgraph.Node]*layoutgraph.Node, len(n.Edges))
		for _, ea := range optim.edgeAbductions {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if ea == nil || ea.Edge == nil {
				return nil, fmt.Errorf("TALA LocalOptimize found a nil edge abduction")
			}
			if ea.CurrentFrom == n || ea.CurrentTo == n {
				if ea.OriginallyFrom != nil || ea.OriginallyTo != nil {
					abductedEdges[ea.Edge] = struct{}{}
					continue
				}
				adj := n.Adjacent(ea.Edge)
				if ea.CurrentFrom == n && ea.OriginallyTo != nil {
					adjReplacements[adj] = ea.OriginallyTo
				} else if ea.CurrentTo == n && ea.OriginallyFrom != nil {
					adjReplacements[adj] = ea.OriginallyFrom
				}
			}
		}

		cellSizeInt := int(optim.cellSize)
		spaceReqs := make(map[*layoutgraph.Node]layoutgraph.LongDistanceNeighborRequirements, len(n.Edges))

		for _, edge := range n.Edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if edge == nil || (edge.From != n && edge.To != n) {
				return nil, fmt.Errorf("TALA LocalOptimize found a malformed incident edge on %s", n.DebugID())
			}
			if _, skip := abductedEdges[edge]; skip {
				continue
			}

			adj := n.Adjacent(edge)
			if replacement, exists := adjReplacements[adj]; exists {
				adj = replacement
			}

			requirements := spaceReqs[adj]
			requirements.EdgeCount++
			if edge.MinWidth > requirements.MaxWidth {
				requirements.MaxWidth = edge.MinWidth
			}
			if edge.MinHeight > requirements.MaxHeight {
				requirements.MaxHeight = edge.MinHeight
			}
			spaceReqs[adj] = requirements
		}

		for _, requirements := range spaceReqs {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if requirements.EdgeCount < 3 {
				continue
			}
			if requirements.MaxWidth <= cellSizeInt && requirements.MaxHeight <= cellSizeInt {
				continue
			}

			n.LongDistanceNeighborRequirements = spaceReqs
			break
		}
	}

	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return optim, nil
}

func (optim *sizedOptimizer) optimize(ctx context.Context, temp float64) (bool, error) {
	return optim.optimizeWithLimit(ctx, temp, limits.MaxOptimizationWorkUnits)
}

func (optim *sizedOptimizer) optimizeWithLimit(ctx context.Context, temp float64, workLimit uint64) (changed bool, err error) {
	ctx, _, err = layoutgraph.EnsureTransactionWorkGuard(ctx, "LocalOptimizeTransactions")
	if err != nil {
		return false, err
	}
	guard, err := limits.NewOptimizationWorkGuard(ctx, "LocalOptimize", workLimit)
	if err != nil {
		return false, err
	}
	if optim == nil || optim.g == nil {
		return false, fmt.Errorf("TALA LocalOptimize requires an optimizer with a graph")
	}
	if optim.randGenerator == nil {
		return false, fmt.Errorf("TALA LocalOptimize requires a random generator")
	}
	defer optim.mutationScratch.release()
	snapshot, err := captureOptimizerMutationStateInto(optim.g, guard, &optim.mutationScratch)
	if err != nil {
		return false, err
	}
	complete := false
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot.restore()
			panic(recovered)
		}
		if !complete {
			snapshot.restore()
		}
	}()

	changed, err = optim.optimizeGuarded(ctx, temp, guard)
	if err != nil {
		return false, err
	}
	if err := guard.Finish(); err != nil {
		return false, err
	}
	complete = true
	return changed, nil
}

func (optim *sizedOptimizer) optimizeGuarded(ctx context.Context, temp float64, guard *limits.OptimizationWorkGuard) (bool, error) {
	changed := false

	if len(optim.g.Nodes) > limits.MaxEngineNodes {
		return false, fmt.Errorf("TALA LocalOptimize node count exceeds limit %d", limits.MaxEngineNodes)
	}
	nodeIndices := make([]int, len(optim.g.Nodes))
	for i := 0; i < len(optim.g.Nodes); i++ {
		if err := guard.Step(); err != nil {
			return false, err
		}
		nodeIndices[i] = i
	}
	if err := limits.Shuffle(nodeIndices, optim.randGenerator, guard); err != nil {
		return false, err
	}

	for _, nodeIndex := range nodeIndices {
		if err := guard.Step(); err != nil {
			return false, err
		}
		node := optim.g.Nodes[nodeIndex]
		if node == nil || node.TopLeft == nil {
			return false, fmt.Errorf("TALA LocalOptimize found an unpositioned graph node")
		}
		if node.FixedTopLeft != nil {
			continue
		}
		if node.Width > 100*optim.cellSize || node.Height > 100*optim.cellSize {
			// leave giant node where it is, we won't find an unoccupied point within 100 cellSize
			continue
		}
		if len(node.Edges) == 0 {
			hasUsableNear := false
			for near := range node.Nears {
				if err := guard.Step(); err != nil {
					return false, err
				}
				isDescendant, err := optimizerIsDescendantOf(near, node.Container, guard)
				if err != nil {
					return false, err
				}
				if isDescendant {
					hasUsableNear = true
					break
				}
			}
			if !hasUsableNear {
				continue
			}
		}
		if _, has := optim.g.NodeToTree[node]; has {
			continue
		}

		protrudingChildren, err := optim.protrudingChildrenGuarded(node, guard)
		if err != nil {
			return false, err
		}
		minimizingSelf := len(protrudingChildren) == 0

		medianPoint, err := optim.medianPointGuarded(node, temp, protrudingChildren, guard)
		if err != nil {
			return false, err
		}
		// All candidate placements for this node see the same boxes for every
		// other graph node. Build one ordered broad-phase index and reuse it until
		// the optimizer commits this node's final position.
		if err := optim.rebuildSpatialIndex(guard); err != nil {
			return false, err
		}

		var checkedPositionsCache map[geo.Point]struct{}
		// Making and using a cache takes many allocs. Only worth it when this node has to search through many siblings for overlap
		if len(nodeIndices) > 10 {
			if optim.checkedPositions == nil {
				optim.checkedPositions = make(map[geo.Point]struct{}, 64)
			} else {
				clear(optim.checkedPositions)
			}
			checkedPositionsCache = optim.checkedPositions
		}
		d, err := optim.findClosestUnoccupiedDistanceGuarded(node, medianPoint, minimizingSelf, checkedPositionsCache, guard)
		if err != nil {
			return false, err
		}

		points, err := optim.fillPlacementPointsGuarded(node, medianPoint, d, minimizingSelf, &optim.placementScratch, guard)
		if err != nil {
			return false, err
		}
		if err := limits.Shuffle(points, optim.randGenerator, guard); err != nil {
			return false, err
		}

		// we want mustImprove to be true for the last 10 optimize calls where temp is 0
		moved, err := optim.moveNodeToBestGuarded(ctx, node, points, temp == 0, guard)
		if err != nil {
			return false, err
		}

		if !moved {
			bestSwapCandidate, err := optim.bestSwapCandidateGuarded(ctx, node, guard)
			if err != nil {
				return false, err
			}
			if bestSwapCandidate != nil {
				changed = true
				if err := optimizerSwapPositions(node, bestSwapCandidate, guard); err != nil {
					return false, err
				}
				if err := optim.syncHerdFencesGuarded(guard); err != nil {
					return false, err
				}
			} else {
				if err := chargeOptimizerTranspose(optim.g, node, optim.edgeAbductions, guard); err != nil {
					return false, err
				}
				ok, err := transpose(ctx, optim.g, node, optim.edgeAbductions)
				if err != nil {
					return false, err
				}
				if err := guard.Step(); err != nil {
					return false, err
				}
				if ok {
					changed = true
				} else {
					// The spokes don't need to move because in the next iteration they'll move
					if spokes, ok := optim.g.Hubs[node]; ok && temp != 0 {
						err := withHubSpokesSuppressed(node, spokes, guard, func() error {
							medianPoint, err := optim.medianPointGuarded(node, temp, protrudingChildren, guard)
							if err != nil {
								return err
							}

							d, err := optim.findClosestUnoccupiedDistanceGuarded(node, medianPoint, minimizingSelf, checkedPositionsCache, guard)
							if err != nil {
								return err
							}
							points, err := optim.fillPlacementPointsGuarded(node, medianPoint, d, minimizingSelf, &optim.placementScratch, guard)
							if err != nil {
								return err
							}
							if err := limits.Shuffle(points, optim.randGenerator, guard); err != nil {
								return err
							}
							_, err = optim.moveNodeToBestGuarded(ctx, node, points, temp == 0, guard)
							return err
						})
						if err != nil {
							return false, err
						}
					}
				}
			}
		} else {
			if err := optim.syncHerdFencesGuarded(guard); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, guard.Finish()
}

func (optim *sizedOptimizer) medianPointGuarded(node *layoutgraph.Node, temp float64, protrudingChildren layoutgraph.Nodes, guard *limits.OptimizationWorkGuard) (*geo.Point, error) {
	width := node.Width / optim.cellSize
	height := node.Height / optim.cellSize
	medianX, medianY, err := optimizerMedianToNeighbors(node, true, optim.edgeAbductions, guard)
	if err != nil {
		return nil, err
	}

	fixedOrigin, err := optimizerFixedOrigin(optim.g, node.OwningContainer(), guard)
	if err != nil {
		return nil, err
	}
	if fixedOrigin != nil {
		// we can't have node positions past the fixedOrigin so set a floor
		if medianX < fixedOrigin.X/optim.cellSize {
			medianX = fixedOrigin.X / optim.cellSize
			// normally the random movement is within [-temp*width, temp*width],
			// but anything left of the fixed origin is invalid so make it [0, 2*temp*width]
			medianX += temp * width
		}
		if medianY < fixedOrigin.Y/optim.cellSize {
			medianY = fixedOrigin.Y / optim.cellSize
			medianY += temp * height
		}
	}

	medianX += ((-1.0 * temp * width) + optim.randGenerator.Float64()*(2.0*temp*width))
	medianY += ((-1.0 * temp * height) + optim.randGenerator.Float64()*(2.0*temp*height))

	if len(protrudingChildren) > 0 {
		childrenMedianX, childrenMedianY, err := optimizerMedian(protrudingChildren, true, guard)
		if err != nil {
			return nil, err
		}
		medianX -= (childrenMedianX - node.TopLeft.X/optim.cellSize)
		medianY -= (childrenMedianY - node.TopLeft.Y/optim.cellSize)
	}

	if fixedOrigin != nil {
		medianX = math.Max(medianX, fixedOrigin.X/optim.cellSize)
		medianY = math.Max(medianY, fixedOrigin.Y/optim.cellSize)
	}

	return geo.NewPoint(
		math.Round(medianX*optim.cellSize),
		math.Round(medianY*optim.cellSize),
	), nil
}

func (optim *sizedOptimizer) protrudingChildrenGuarded(node *layoutgraph.Node, guard *limits.OptimizationWorkGuard) ([]*layoutgraph.Node, error) {
	// If this container were being placed, its median would be the bottom-most node, but that doesn't reflect where we want the node to be
	// We want the node to be placed such that the median of its children with protruding edges is at the calculated median
	// ┌────────────────┐
	// │                │
	// │                │
	// │                │
	// │                │
	// │                │
	// │          ┌──┐  │
	// │          └─┼┘  │
	// │            │   │
	// └────────────┼───┘
	//              │
	//            ┌─▼┐
	//            └──┘
	protrudingChildren := []*layoutgraph.Node{}
	for _, ea := range optim.edgeAbductions {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if ea == nil {
			return nil, fmt.Errorf("TALA LocalOptimize found a nil edge abduction")
		}
		if ea.CurrentFrom == node && ea.OriginallyFrom != nil {
			protrudingChildren = append(protrudingChildren, ea.OriginallyFrom)
		}
		if ea.CurrentTo == node && ea.OriginallyTo != nil {
			protrudingChildren = append(protrudingChildren, ea.OriginallyTo)
		}
	}
	return protrudingChildren, nil
}

func (optim sizedOptimizer) findClosestUnoccupiedDistanceGuarded(node *layoutgraph.Node, p *geo.Point, minimizingSelf bool, checked map[geo.Point]struct{}, guard *limits.OptimizationWorkGuard) (float64, error) {
	if node == nil || node.Graph == nil || node.TopLeft == nil || p == nil {
		return 0, fmt.Errorf("TALA %s placement search requires a positioned node, graph, and point", guard.Location())
	}
	if optim.cellSize <= 0 || math.IsNaN(optim.cellSize) || math.IsInf(optim.cellSize, 0) {
		return 0, fmt.Errorf("TALA %s placement search requires a finite positive cell size", guard.Location())
	}
	if len(checked) > maxOptimizerPlacementCandidates {
		return 0, fmt.Errorf("%w: TALA %s checked placement count exceeds limit %d", limits.ErrOptimizationResourceLimit, guard.Location(), maxOptimizerPlacementCandidates)
	}
	// example with distance=3, we iterate points from least to most distance
	//          3
	//       3  2  3
	//    3  2  1  2  3
	// 3──2──1──0──1──2──3
	//    3  2  1  2  3
	//       3  2  3
	//          3
	// for points at the same distance we iterate right to left (& negative then positive):
	// curr=0       =1            =2               =3        6
	//                                   4                8     4
	//                  2             6     2         10           2
	//     ──0──     3─────0       7───────────0   12─────────────────0
	//                  1             5     1          9           1
	//                                   3                7     3
	//                                                       5
	cellSize := optim.cellSize

	// For each candidate “ring” (curr is in grid units, then scaled by cellSize)
	for curr := 0.0; curr <= 100.0; curr++ {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		currCell := curr * cellSize

		// Start with x at the ring's rightmost point and y = 0.
		// The candidate point offsets are defined relative to p.
		x := currCell
		y := 0.0

		// Continue until x goes below -currCell.
		for x >= -currCell {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			// Check the candidate at (x, -y) if y is nonzero.
			if y != 0 {
				candidate := geo.Point{
					X: roundToNearestCellSize(p.X+x, cellSize),
					Y: roundToNearestCellSize(p.Y-y, cellSize),
				}
				// Enforce fixed-origin constraint if needed.
				if optim.fixedOrigin == nil || (candidate.X >= optim.fixedOrigin.X && candidate.Y >= optim.fixedOrigin.Y) {
					unoccupied, err := optim.findUnoccupiedGuarded(x, -y, node.Width, node.Height, minimizingSelf, node, p, checked, guard)
					if err != nil {
						return 0, err
					}
					if unoccupied {
						return curr, nil
					}
				}
			}
			// Check candidate at (x, y)
			candidate := geo.Point{
				X: roundToNearestCellSize(p.X+x, cellSize),
				Y: roundToNearestCellSize(p.Y+y, cellSize),
			}
			if optim.fixedOrigin == nil || (candidate.X >= optim.fixedOrigin.X && candidate.Y >= optim.fixedOrigin.Y) {
				unoccupied, err := optim.findUnoccupiedGuarded(x, y, node.Width, node.Height, minimizingSelf, node, p, checked, guard)
				if err != nil {
					return 0, err
				}
				if unoccupied {
					return curr, nil
				}
			}

			// Update y: if x > 0, increment y by cellSize; otherwise, decrement y.
			if x > 0 {
				y += cellSize
			} else {
				y -= cellSize
			}
			x -= cellSize
		}
	}
	return 0, fmt.Errorf("could not find closest unoccupied distance for p=%v, node=%s", p, node.DebugID())
}

// findUnoccupied is a tailored closest-unoccupied-distance search over placements around a node.
// It is called often enough to warrant a specialized implementation.
func (optim *sizedOptimizer) findUnoccupiedGuarded(
	x, y, width, height float64, minimizingSelf bool, node *layoutgraph.Node, p *geo.Point, checked map[geo.Point]struct{}, guard *limits.OptimizationWorkGuard,
) (bool, error) {
	if err := guard.Step(); err != nil {
		return false, err
	}
	skip := false
	offset := geo.Point{X: x, Y: y}
	if checked != nil {
		if _, ok := checked[offset]; ok {
			skip = true
		} else {
			if len(checked) >= maxOptimizerPlacementCandidates {
				return false, fmt.Errorf("%w: TALA %s checked placement count exceeds limit %d", limits.ErrOptimizationResourceLimit, guard.Location(), maxOptimizerPlacementCandidates)
			}
			checked[offset] = struct{}{}
		}
	}

	if !skip {
		occupied, err := optim.isPointOccupiedGuarded(x, y, p, node, guard)
		if err != nil {
			return false, err
		}
		if !occupied {
			return true, nil
		}
	}

	if !minimizingSelf {
		return false, nil
	}

	for i := x - width; i <= x; i += optim.cellSize {
		for j := y - height; j <= y; j += optim.cellSize {
			if err := guard.Step(); err != nil {
				return false, err
			}
			// Already attempted above
			if i == x && j == y {
				continue
			}
			if checked != nil {
				offset.X = i
				offset.Y = j
				if _, ok := checked[offset]; ok {
					continue
				}
				if len(checked) >= maxOptimizerPlacementCandidates {
					return false, fmt.Errorf("%w: TALA %s checked placement count exceeds limit %d", limits.ErrOptimizationResourceLimit, guard.Location(), maxOptimizerPlacementCandidates)
				}
				checked[offset] = struct{}{}
			}
			occupied, err := optim.isPointOccupiedGuarded(i, j, p, node, guard)
			if err != nil {
				return false, err
			}
			if !occupied {
				return true, nil
			}
		}
	}
	return false, nil
}

func (optim *sizedOptimizer) isPointOccupiedGuarded(x, y float64, p *geo.Point, node *layoutgraph.Node, guard *limits.OptimizationWorkGuard) (bool, error) {
	point := geo.NewPoint(p.X+x, p.Y+y)
	point.X = roundToNearestCellSize(point.X, optim.cellSize)
	point.Y = roundToNearestCellSize(point.Y, optim.cellSize)
	if optim.fixedOrigin != nil && (point.X < optim.fixedOrigin.X || point.Y < optim.fixedOrigin.Y) {
		return true, nil
	}
	_, occupied, err := optim.indexedIsOccupied(point, guard)
	if err != nil {
		return false, err
	}
	if !occupied {
		overlaps, err := optim.indexedDoesOverlap(node, point, []*layoutgraph.Node{node}, guard)
		if err != nil {
			return false, err
		}
		if !overlaps {
			return false, nil
		}
	}
	return true, nil
}

// placementPointsScratch holds reusable candidate-generation memory.
type placementPointsScratch struct {
	seen   map[uint64]struct{} // Deduplicate coordinates
	points []geo.Point         // Collection of placement points
}

// fillPlacementPointsGuarded fills caller-owned scratch so the optimizer can
// reuse candidate storage across its generate-shuffle-score loop.
func (optim *sizedOptimizer) fillPlacementPointsGuarded(
	node *layoutgraph.Node,
	median *geo.Point,
	minUnocc float64,
	minimizingSelf bool,
	scratch *placementPointsScratch,
	guard *limits.OptimizationWorkGuard,
) ([]geo.Point, error) {
	if node == nil || node.TopLeft == nil || median == nil {
		return nil, fmt.Errorf("TALA %s placement generation requires a positioned node and median", guard.Location())
	}
	if minUnocc < 0 || minUnocc > 100 || math.IsNaN(minUnocc) || math.IsInf(minUnocc, 0) {
		return nil, fmt.Errorf("TALA %s placement distance must be finite and within [0, 100]", guard.Location())
	}
	if optim.cellSize <= 0 || math.IsNaN(optim.cellSize) || math.IsInf(optim.cellSize, 0) {
		return nil, fmt.Errorf("TALA %s placement generation requires a finite positive cell size", guard.Location())
	}
	if scratch.seen == nil {
		scratch.seen = make(map[uint64]struct{}, 128)
	}
	if scratch.points == nil {
		scratch.points = make([]geo.Point, 0, 128)
	}

	// Clear data structures for reuse
	for k := range scratch.seen {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		delete(scratch.seen, k)
	}
	scratch.points = scratch.points[:0]

	// Calculate distance and estimate capacity
	dist := minUnocc + 1

	// Using bit shifting for faster encoding of coordinates
	// No need to create a closure for this simple operation
	// Use direct memoization in the add function

	// The add function checks for duplicates and adds valid points
	var addErr error
	add := func(offX, offY float64) bool {
		if addErr != nil {
			return true
		}
		if err := guard.Step(); err != nil {
			addErr = err
			return true
		}
		pX := roundToNearestCellSize(median.X+offX, optim.cellSize)
		pY := roundToNearestCellSize(median.Y+offY, optim.cellSize)

		// Early rejection based on fixed origin
		if fo := optim.fixedOrigin; fo != nil &&
			(pX < fo.X || pY < fo.Y) {
			return false
		}

		// Encode coordinates to a single 64-bit value for map lookup
		// Inlined the encode function to reduce overhead
		ix := int64(pX / optim.cellSize)
		iy := int64(pY / optim.cellSize)
		key := uint64(uint32(ix))<<32 | uint64(uint32(iy))

		// Check if duplicate
		if _, dup := scratch.seen[key]; dup {
			return false
		}
		if len(scratch.points) >= maxOptimizerPlacementCandidates {
			addErr = fmt.Errorf("%w: TALA LocalOptimize placement candidates exceed limit %d", limits.ErrOptimizationResourceLimit, maxOptimizerPlacementCandidates)
			return true
		}

		// Add to seen and points collections
		scratch.seen[key] = struct{}{}
		scratch.points = append(scratch.points, geo.Point{X: pX, Y: pY})
		return false
	}

	// Generate placement points in diamond pattern
	// Use integer for loop counters when possible to reduce float ops
	for x := dist; x >= -dist; x-- {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		maxY := dist - math.Abs(x)
		for y := 0.0; y <= maxY; y++ {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			// Generate points in both positive and negative Y directions
			if y != 0 {
				iterPlacementsAroundPoint(node, x, -y, minimizingSelf, add)
			}
			iterPlacementsAroundPoint(node, x, y, minimizingSelf, add)
			if addErr != nil {
				return nil, addErr
			}
		}
	}

	// Process long-distance neighbor requirements.
	if data := node.LongDistanceNeighborRequirements; data != nil {
		if int64(len(data)) > layoutgraph.MaxTopologyReferences {
			return nil, fmt.Errorf("TALA %s long-distance neighbor references exceed limit %d", guard.Location(), layoutgraph.MaxTopologyReferences)
		}
		cell := float64(int(optim.cellSize))

		neighbors := make([]*layoutgraph.Node, 0, len(data))
		for adjacent := range data {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			neighbors = append(neighbors, adjacent)
		}
		if err := guard.AddSort(len(neighbors)); err != nil {
			return nil, err
		}
		slices.SortFunc(neighbors, func(a, b *layoutgraph.Node) int {
			return cmp.Compare(a.IDValue(), b.IDValue())
		})

		// Avoid creating closures in loops by using direct function calls
		for _, adj := range neighbors {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			requirements := data[adj]
			if adj == nil || adj.TopLeft == nil {
				return nil, fmt.Errorf("TALA %s found an unpositioned long-distance neighbor", guard.Location())
			}
			maxW := requirements.MaxWidth
			maxH := requirements.MaxHeight

			// Pre-calculate common values
			ax := adj.TopLeft.X - median.X
			ay := adj.TopLeft.Y - median.Y

			// Add specific placement points for width
			if maxW > int(cell) {
				add(math.Floor((ax-node.Width-float64(maxW))/cell)*cell, ay)
				add(math.Ceil((ax+adj.Width+float64(maxW))/cell)*cell, ay)
			}

			// Add specific placement points for height
			if maxH > int(cell) {
				add(ax, math.Floor((ay-node.Height-float64(maxH))/cell)*cell)
				add(ax, math.Ceil((ay+adj.Height+float64(maxH))/cell)*cell)
			}
			if addErr != nil {
				return nil, addErr
			}
		}
	}

	// Add current position for herd assignments
	if node.HerdAssignment != nil {
		if len(scratch.points) >= maxOptimizerPlacementCandidates {
			return nil, fmt.Errorf("%w: TALA LocalOptimize placement candidates exceed limit %d", limits.ErrOptimizationResourceLimit, maxOptimizerPlacementCandidates)
		}
		scratch.points = append(scratch.points, *node.TopLeft)
	}

	// Preserve the original work charge even when the internal caller consumes
	// the scratch slice directly.
	if err := guard.Add(uint64(len(scratch.points))); err != nil {
		return nil, err
	}
	return scratch.points, nil
}

func (optim *sizedOptimizer) moveNodeToBestGuarded(ctx context.Context, node *layoutgraph.Node, points []geo.Point, mustImprove bool, guard *limits.OptimizationWorkGuard) (bool, error) {
	scorer := placementcost.NewNodeEdgeLengthScorer(node, placementcost.EdgeLengthOptions{
		EdgeAbductions: optim.edgeAbductions, IncludeNodeSizes: true,
		EnforceMinimumGap: false, PenalizeDirection: true,
	})
	defer scorer.Close()
	symmetryCost := optim.cellSize * float64(len(node.Edges))
	leastDistance := math.Inf(1)
	if mustImprove {
		if err := chargeOptimizerScoring(node, optim.edgeAbductions, true, guard); err != nil {
			return false, err
		}
		var err error
		leastDistance, err = scorer.Score(ctx)
		if err != nil {
			return false, err
		}
		columnCrossingCost, err := placementcost.ColumnCrossingCost(ctx, node, optim.edgeAbductions)
		if err != nil {
			return false, err
		}
		symmetry, err := placementcost.NodeSymmetry(ctx, node, optim.edgeAbductions)
		if err != nil {
			return false, err
		}
		leastDistance += columnCrossingCost
		leastDistance -= symmetry * symmetryCost
	}
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

	for i := range points {
		if err := guard.Step(); err != nil {
			restoreNodePositions(originalPositions)
			return false, err
		}
		point := &points[i]
		x := point.X
		y := point.Y
		if optim.fixedOrigin != nil && (x < optim.fixedOrigin.X || y < optim.fixedOrigin.Y) {
			continue
		}
		// If the candidate position is the original one, we should be able to move back to it (if we moved somewhere else in this routine)
		canMove := x == currentNodeX && y == currentNodeY
		if !canMove {
			var err error
			canMove, err = optim.indexedCanMove(node, point, guard)
			if err != nil {
				restoreNodePositions(originalPositions)
				return false, err
			}
		}
		if canMove {
			if err := movement.moveAbs(x, y, guard); err != nil {
				restoreNodePositions(originalPositions)
				return false, err
			}
			if err := chargeOptimizerScoring(node, optim.edgeAbductions, true, guard); err != nil {
				restoreNodePositions(originalPositions)
				return false, err
			}

			edgeLength, err := scorer.Score(ctx)
			if err != nil {
				restoreNodePositions(originalPositions)
				return false, err
			}
			if !math.IsInf(leastDistance, 1) && geo.PrecisionCompare(edgeLength-symmetryCost, leastDistance, geo.PRECISION) == 1 {
				// no need to continue, this is clearly worse
				continue
			}
			columnCrossingCost, err := placementcost.ColumnCrossingCost(ctx, node, optim.edgeAbductions)
			if err != nil {
				restoreNodePositions(originalPositions)
				return false, err
			}
			symmetry, err := placementcost.NodeSymmetry(ctx, node, optim.edgeAbductions)
			if err != nil {
				restoreNodePositions(originalPositions)
				return false, err
			}
			edgeLength += columnCrossingCost
			edgeLength -= symmetry * symmetryCost

			for _, o := range optim.obstacles {
				if err := guard.Step(); err != nil {
					restoreNodePositions(originalPositions)
					return false, err
				}
				if o.TopLeft.X == -layoutgraph.ContainerPadding && o.TopLeft.Y == -layoutgraph.ContainerPadding {
					continue
				}
				if node.Box.Overlaps(o) {
					edgeLength += 3 * optim.g.TurnCost()
				}
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
	}

	if err := ctx.Err(); err != nil {
		restoreNodePositions(originalPositions)
		return false, fmt.Errorf("EdgeLength: %w", err)
	}
	if math.IsInf(leastDistance, 1) {
		restoreNodePositions(originalPositions)
		return false, fmt.Errorf("sizedOptimizer: could not find any placement")
	}

	if err := movement.moveAbs(leastDistancePoint.X, leastDistancePoint.Y, guard); err != nil {
		restoreNodePositions(originalPositions)
		return false, err
	}

	complete = true
	return (node.TopLeft.X != currentNodeX) || (node.TopLeft.Y != currentNodeY), nil
}

func (optim *sizedOptimizer) bestSwapCandidateGuarded(ctx context.Context, node *layoutgraph.Node, guard *limits.OptimizationWorkGuard) (*layoutgraph.Node, error) {
	minSwapL := math.Inf(1)

	// TODO (Mon Sep 20 10:57:17 2021) These maybe should be true on enforcing gap size, since when we swap, we don't want resulting swap to be too squished
	if err := chargeOptimizerScoring(node, optim.edgeAbductions, true, guard); err != nil {
		return nil, err
	}
	currentL1, err := placementcost.NodeEdgeLength(ctx, node, placementcost.EdgeLengthOptions{EdgeAbductions: optim.edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		return nil, err
	}
	columnCrossingCost, err := placementcost.ColumnCrossingCost(ctx, node, optim.edgeAbductions)
	if err != nil {
		return nil, err
	}
	symmetry, err := placementcost.NodeSymmetry(ctx, node, optim.edgeAbductions)
	if err != nil {
		return nil, err
	}
	currentL1 += columnCrossingCost
	currentL1 -= symmetry * optim.symmetryCost * float64(len(node.Edges))
	var bestSwapCandidate *layoutgraph.Node = nil

	swapCandidateIndices := make([]int, len(optim.g.Nodes))
	for i := 0; i < len(optim.g.Nodes); i++ {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		swapCandidateIndices[i] = i
	}
	if err := limits.Shuffle(swapCandidateIndices, optim.randGenerator, guard); err != nil {
		return nil, err
	}

	for _, swapCandidateIndex := range swapCandidateIndices {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		swapCandidate := optim.g.Nodes[swapCandidateIndex]
		if swapCandidate == node {
			continue
		}
		if swapCandidate.FixedTopLeft != nil {
			continue
		}
		if !swapCandidate.IsAdjacentTo(node, true) {
			continue
		}
		if _, has := optim.g.NodeToTree[swapCandidate]; has {
			continue
		}
		firstOverlap, err := optim.indexedDoesOverlap(swapCandidate, node.TopLeft, []*layoutgraph.Node{node}, guard)
		if err != nil {
			return nil, err
		}
		secondOverlap, err := optim.indexedDoesOverlap(node, swapCandidate.TopLeft, []*layoutgraph.Node{swapCandidate}, guard)
		if err != nil {
			return nil, err
		}
		if firstOverlap || secondOverlap {
			continue
		}

		if err := chargeOptimizerScoring(swapCandidate, optim.edgeAbductions, true, guard); err != nil {
			return nil, err
		}
		currentL2, err := placementcost.NodeEdgeLength(ctx, swapCandidate, placementcost.EdgeLengthOptions{EdgeAbductions: optim.edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
		if err != nil {
			return nil, err
		}
		columnCrossingCost, err := placementcost.ColumnCrossingCost(ctx, swapCandidate, optim.edgeAbductions)
		if err != nil {
			return nil, err
		}
		symmetry, err := placementcost.NodeSymmetry(ctx, swapCandidate, optim.edgeAbductions)
		if err != nil {
			return nil, err
		}
		currentL2 += columnCrossingCost
		currentL2 -= symmetry * optim.symmetryCost * float64(len(swapCandidate.Edges))

		var swappedL1, swappedL2 float64
		validSwap := false
		err = withOptimizerPositionsSwapped(node, swapCandidate, guard, func() error {

			// Have to check for overlap again, since swapping two of different sizes
			// may cause one to overlap with the other -- so, no exceptions in this check
			firstOverlap, err := optimizerDoesOverlap(swapCandidate, swapCandidate.TopLeft, nil, guard)
			if err != nil {
				return err
			}
			secondOverlap, err := optimizerDoesOverlap(node, node.TopLeft, nil, guard)
			if err != nil {
				return err
			}
			if firstOverlap || secondOverlap {
				return nil
			}
			validSwap = true

			var scoreErr error
			if scoreErr = chargeOptimizerScoring(node, optim.edgeAbductions, true, guard); scoreErr != nil {
				return scoreErr
			}
			swappedL1, scoreErr = placementcost.NodeEdgeLength(ctx, node, placementcost.EdgeLengthOptions{EdgeAbductions: optim.edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
			if scoreErr != nil {
				return scoreErr
			}
			columnCrossingCost, scoreErr = placementcost.ColumnCrossingCost(ctx, node, optim.edgeAbductions)
			if scoreErr != nil {
				return scoreErr
			}
			symmetry, scoreErr = placementcost.NodeSymmetry(ctx, node, optim.edgeAbductions)
			if scoreErr != nil {
				return scoreErr
			}
			swappedL1 += columnCrossingCost
			swappedL1 -= symmetry * optim.symmetryCost * float64(len(node.Edges))

			// No need to compute it if we already know it won't be a good swap.
			if geo.PrecisionCompare(swappedL1, currentL1, geo.PRECISION) < 0 {
				if scoreErr = chargeOptimizerScoring(swapCandidate, optim.edgeAbductions, true, guard); scoreErr != nil {
					return scoreErr
				}
				swappedL2, scoreErr = placementcost.NodeEdgeLength(ctx, swapCandidate, placementcost.EdgeLengthOptions{EdgeAbductions: optim.edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
				if scoreErr != nil {
					return scoreErr
				}
				columnCrossingCost, scoreErr = placementcost.ColumnCrossingCost(ctx, swapCandidate, optim.edgeAbductions)
				if scoreErr != nil {
					return scoreErr
				}
				symmetry, scoreErr = placementcost.NodeSymmetry(ctx, swapCandidate, optim.edgeAbductions)
				if scoreErr != nil {
					return scoreErr
				}
				swappedL2 += columnCrossingCost
				swappedL2 -= symmetry * optim.symmetryCost * float64(len(swapCandidate.Edges))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if !validSwap {
			continue
		}

		if geo.PrecisionCompare(swappedL1, currentL1, geo.PRECISION) < 0 && geo.PrecisionCompare(swappedL1+swappedL2, currentL1+currentL2, geo.PRECISION) < 0 && geo.PrecisionCompare(swappedL1+swappedL2, minSwapL, geo.PRECISION) < 0 {
			minSwapL = swappedL1 + swappedL2
			bestSwapCandidate = swapCandidate
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("EdgeLength: %w", err)
	}
	return bestSwapCandidate, nil
}

func (optim *sizedOptimizer) syncHerdFencesGuarded(guard *limits.OptimizationWorkGuard) error {
	// Preserve the legacy bounding-box semantics (outside labels, modifier
	// elements, loop offsets, routed edges, and fixed origins). Precharging the
	// bounded quadratic scan keeps it under the shared operation budget.
	nodes := uint64(len(optim.g.Nodes))
	if err := guard.AddProduct(nodes, nodes+1); err != nil {
		return err
	}
	if err := guard.Add(uint64(len(optim.g.Edges))); err != nil {
		return err
	}
	proximity.SyncHerdFences(optim.g)
	return guard.Finish()
}
