package placement

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"

	"github.com/d2lang/d2/lib/geo"
)

// compaction serves two functions:
// 1. Inflate nodes to create room for more optimal node placements (escaping local maximums)
// 2. Reduce distance between nodes too far apart
//
// We use compaction to inflate when transitioning from sizeless phase to sized phase, which the transition flag is for
type compactionOptions struct {
	edgeAbductions []*layoutgraph.EdgeAbduction
	axis           layoutAxis
	includeSizes   bool
	factor         float64
	transition     bool
	moveWorkLimit  uint64
}

func compaction(ctx context.Context, g *layoutgraph.Graph, options compactionOptions) (err error) {
	if !options.axis.valid() {
		return fmt.Errorf("TALA Compaction requires an axis")
	}
	if options.factor <= 0 || math.IsNaN(options.factor) || math.IsInf(options.factor, 0) {
		return fmt.Errorf("TALA Compaction requires a finite positive factor")
	}
	edgeAbductions := options.edgeAbductions
	isHorizontal := options.axis.isHorizontal()
	includeSizes := options.includeSizes
	factor := options.factor
	transition := options.transition
	if err := layoutgraph.Validate(ctx, "Compaction", g); err != nil {
		return err
	}
	originalCosts := g.RoutingCosts()
	originalPositions, err := snapshotNodePositionsContext(ctx, "Compaction", g.Nodes)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			restoreNodePositions(originalPositions)
			g.RestoreRoutingCosts(originalCosts)
		}
	}()

	visibilityEdges, err := visibilityEdges(ctx, g, isHorizontal, includeSizes)
	if err != nil {
		return err
	}

	inflateAlongAxis(g, isHorizontal, includeSizes, factor, visibilityEdges, transition)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Compaction: %w", err)
	}

	// We're returning early during transition because the below calls assume no overlap
	// And during transition, there can be temporary overlaps along a dimension
	// The main point of the transition is just to inflate anyway, there shouldn't be any compacting needed because
	// they inflate to the minimum possible value
	if transition {
		complete = true
		return nil
	}

	for i := 0; i < 20; i++ {
		changed, err := shiftSubgraphs(ctx, g, isHorizontal, includeSizes, factor, edgeAbductions, visibilityEdges)
		if err != nil {
			return err
		}
		if !changed {
			break
		}
	}
	moveWorkLimit := options.moveWorkLimit
	if moveWorkLimit == 0 || moveWorkLimit > limits.MaxOptimizationWorkUnits {
		moveWorkLimit = limits.MaxOptimizationWorkUnits
	}
	movesGuard, err := limits.NewOptimizationWorkGuard(ctx, "CompactionMoves", moveWorkLimit)
	if err != nil {
		return err
	}

	for i := 0; i < 20; i++ {
		changed, err := compactAlongAxis(ctx, g, isHorizontal, includeSizes, factor, edgeAbductions, visibilityEdges, movesGuard)
		if err != nil {
			return err
		}
		if !changed {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Compaction: %w", err)
	}
	complete = true
	return nil
}

func orderedAlongAxis(g *layoutgraph.Graph, isHorizontal bool) []*layoutgraph.Node {
	orderedNodes := make([]*layoutgraph.Node, len(g.Nodes))
	copy(orderedNodes, g.Nodes)

	sort.Slice(orderedNodes, func(i, j int) bool {
		if isHorizontal {
			if orderedNodes[i].TopLeft.X == orderedNodes[j].TopLeft.X {
				return orderedNodes[i].ID < orderedNodes[j].ID
			}
			return orderedNodes[i].TopLeft.X < orderedNodes[j].TopLeft.X
		} else {
			if orderedNodes[i].TopLeft.Y == orderedNodes[j].TopLeft.Y {
				return orderedNodes[i].ID < orderedNodes[j].ID
			}
			return orderedNodes[i].TopLeft.Y < orderedNodes[j].TopLeft.Y
		}
	})
	return orderedNodes
}

// shiftSubgraphs tries to move entire subgraphs of the visibility graph, for a more global attempt to close gaps
// The primary purpose is to escape local maximas, like in this case below, where no individual nodes would move closer to each other
// .        ┌───┐  ┌───┐
// .   ┌────►   ├──►   │
// .   │    └───┘  └───┘
// . ┌─┴─┐
// . │   │
// . └─▲─┘
// .   │
// . ┌─┴─┐
// . │   │
// . └───┘
func shiftSubgraphs(ctx context.Context, g *layoutgraph.Graph, isHorizontal, includeSizes bool, factor float64, edgeAbductions []*layoutgraph.EdgeAbduction, vEdges layoutgraph.Edges) (bool, error) {
	changed := false
	subgraphSet := make(map[*layoutgraph.Node][]*layoutgraph.Node)
	orderedSubgraphRoots := []*layoutgraph.Node{}

	orderedNodes := orderedAlongAxis(g, isHorizontal)

	for _, node := range orderedNodes {
		nearestFrom := nearestFrom(vEdges, node, isHorizontal, includeSizes)
		if nearestFrom == nil {
			subgraphSet[node] = []*layoutgraph.Node{node}
			orderedSubgraphRoots = append(orderedSubgraphRoots, node)
		} else {
			subgraphSet[nearestFrom] = append(subgraphSet[nearestFrom], node)
		}
	}

	filtered := []*layoutgraph.Node{}
	for _, root := range orderedSubgraphRoots {
		sgHasFixedNode := false
		for _, n := range subgraphSet[root] {
			if n.FixedTopLeft != nil {
				sgHasFixedNode = true
				break
			}
		}
		if sgHasFixedNode {
			delete(subgraphSet, root)
		} else {
			filtered = append(filtered, root)
		}
	}
	orderedSubgraphRoots = filtered
	fixedNodes := g.FixedNodes()

	sort.Slice(orderedSubgraphRoots, func(i, j int) bool {
		if isHorizontal {
			if orderedSubgraphRoots[i].TopLeft.X == orderedSubgraphRoots[j].TopLeft.X {
				return orderedSubgraphRoots[i].ID < orderedSubgraphRoots[j].ID
			}
			return orderedSubgraphRoots[i].TopLeft.X < orderedSubgraphRoots[j].TopLeft.X
		} else {
			if orderedSubgraphRoots[i].TopLeft.Y == orderedSubgraphRoots[j].TopLeft.Y {
				return orderedSubgraphRoots[i].ID < orderedSubgraphRoots[j].ID
			}
			return orderedSubgraphRoots[i].TopLeft.Y < orderedSubgraphRoots[j].TopLeft.Y
		}
	})

	globallyFurthestBehind := globallyFurthestBehind(g, isHorizontal)
	for _, furthestBehind := range orderedSubgraphRoots {
		subgraph := subgraphSet[furthestBehind]
		var possibleMoves []*geo.Point
		var err error
		if isHorizontal {
			if furthestBehind.TopLeft.X == globallyFurthestBehind.TopLeft.X {
				possibleMoves, err = candidateMoves(ctx, g, furthestBehind, factor, isHorizontal, includeSizes, 2, vEdges)
			} else {
				possibleMoves, err = candidateMoves(ctx, g, furthestBehind, factor, isHorizontal, includeSizes, 0, vEdges)
			}
		} else {
			if furthestBehind.TopLeft.Y == globallyFurthestBehind.TopLeft.Y {
				possibleMoves, err = candidateMoves(ctx, g, furthestBehind, factor, isHorizontal, includeSizes, 2, vEdges)
			} else {
				possibleMoves, err = candidateMoves(ctx, g, furthestBehind, factor, isHorizontal, includeSizes, 0, vEdges)
			}
		}
		if err != nil {
			return false, err
		}
		possibleMoves = append(possibleMoves, furthestBehind.TopLeft.Copy())

		// shifting subgraphs shouldn't introduce any overlaps between subgraph nodes, but we still need to check for overlaps with nodes not in this subgraph
		nonSubgraphNodes := []*layoutgraph.Node{}
		nonSubgraphNodes = append(nonSubgraphNodes, fixedNodes...)
		for _, otherN := range g.Nodes {
			in := slices.Contains(subgraph, otherN)
			if !in {
				nonSubgraphNodes = append(nonSubgraphNodes, otherN)
			}
		}

		symmetryCost := 1.0
		if includeSizes {
			symmetryCost = g.CellSize
		}
		symmetryCost *= float64(layoutgraph.Nodes(subgraph).NumAdjacent())

		bestEdgeDistance, err := placementcost.NodesEdgeLength(ctx, layoutgraph.Nodes(subgraph), placementcost.EdgeLengthOptions{EdgeAbductions: edgeAbductions, IncludeNodeSizes: includeSizes, EnforceMinimumGap: false, PenalizeDirection: true})
		if err != nil {
			return false, err
		}
		bestDelta := 0.0

		originalPositions, err := snapshotNodePositionsContext(ctx, "Compaction", subgraph)
		if err != nil {
			return false, err
		}
		for _, move := range possibleMoves {
			var delta float64
			if isHorizontal {
				delta = furthestBehind.TopLeft.X - move.X
			} else {
				delta = furthestBehind.TopLeft.Y - move.Y
			}

			overlaps := false
			for _, n := range subgraph {
				p := n.TopLeft.Copy()
				if isHorizontal {
					p.X -= delta
				} else {
					p.Y -= delta
				}
				if len(fixedNodes) > 0 {
					if n.PointPastFixedOrigin(p.X, p.Y, includeSizes) {
						overlaps = true
						break
					}
				}
				for _, otherN := range nonSubgraphNodes {
					if includeSizes {
						if n.DoesOverlapAt(otherN, p) {
							overlaps = true
							break
						}
					} else {
						if nonNilEquals(otherN.TopLeft, p) {
							overlaps = true
							break
						}
					}
				}
				if overlaps {
					break
				}
			}
			if overlaps {
				continue
			}

			for _, node := range subgraph {
				if isHorizontal {
					node.MoveWithChildren(-delta, 0)
				} else {
					node.MoveWithChildren(0, -delta)
				}
			}

			edgeDistanceAfter, err := placementcost.NodesEdgeLength(ctx, layoutgraph.Nodes(subgraph), placementcost.EdgeLengthOptions{EdgeAbductions: edgeAbductions, IncludeNodeSizes: includeSizes, EnforceMinimumGap: false, PenalizeDirection: true})
			if err != nil {
				restoreNodePositions(originalPositions)
				return false, err
			}
			if includeSizes {
				symmetry, err := placementcost.NodesSymmetry(ctx, layoutgraph.Nodes(subgraph), edgeAbductions)
				if err != nil {
					restoreNodePositions(originalPositions)
					return false, err
				}
				edgeDistanceAfter -= symmetry * symmetryCost
			}

			restoreNodePositions(originalPositions)

			if edgeDistanceAfter < bestEdgeDistance {
				bestEdgeDistance = edgeDistanceAfter
				bestDelta = delta
			}
		}
		if bestDelta != 0 {
			changed = true
			for _, node := range subgraph {
				if isHorizontal {
					node.MoveWithChildren(-bestDelta, 0)
				} else {
					node.MoveWithChildren(0, -bestDelta)
				}
			}
		}
	}
	return changed, nil
}

// inflateAlongAxis makes sure that the appropriate amount of distance (based on factor) between connected (in vGraph) nodes
func inflateAlongAxis(g *layoutgraph.Graph, isHorizontal, includeSizes bool, factor float64, vEdges layoutgraph.Edges, transition bool) {
	orderedNodes := orderedAlongAxis(g, isHorizontal)
	hasFixedNode := g.HasFixedNode()

	for _, node := range orderedNodes {
		nearestFrom := nearestFrom(vEdges, node, isHorizontal, includeSizes)
		if nearestFrom == nil {
			// TODO reconsider this
			if hasFixedNode && transition && node.FixedTopLeft == nil {
				// during transition we want all nodes to move to a cellSize even if they don't have a nearestFrom
				if isHorizontal {
					node.MoveAbsWithChildren(roundToPreviousCellSize(node.TopLeft.X, g.CellSize), node.TopLeft.Y)
				} else {
					node.MoveAbsWithChildren(node.TopLeft.X, roundToPreviousCellSize(node.TopLeft.Y, g.CellSize))
				}
			}
			continue
		}
		// we don't want to move fixed nodes but we transitioning from sizeless to sized is an exception
		if node.FixedTopLeft != nil && !transition {
			continue
		}

		// Edge case:
		// During transition, because there are temporary overlaps, we want to maintain those overlaps for visibility edges in
		// the subsequent iteration of compaction. If one is connected and another is not, then that overlap can be lost.
		// So we force it to be maintained.
		var delta int
		if transition {
			delta = layoutgraph.ConnectedNodeGap
		} else {
			delta = node.DeltaTo(nearestFrom, node.TopLeft)
		}
		floor := compactionFloor(g, nearestFrom, factor, isHorizontal, includeSizes, delta)
		if isHorizontal {
			if includeSizes {
				if floor*g.CellSize > node.TopLeft.X {
					node.MoveAbsWithChildren(floor*g.CellSize, node.TopLeft.Y)
				}
			} else {
				if floor > node.TopLeft.X {
					node.MoveAbsWithChildren(floor, node.TopLeft.Y)
				}
			}
		} else {
			if includeSizes {
				if floor*g.CellSize > node.TopLeft.Y {
					node.MoveAbsWithChildren(node.TopLeft.X, floor*g.CellSize)
				}
			} else {
				if floor > node.TopLeft.Y {
					node.MoveAbsWithChildren(node.TopLeft.X, floor)
				}
			}
		}
	}
}

// Each node moves as far back as possible while making progress towards an adjacent node
// (this is a visibility edge)
// Before
// ┌───┐                       ┌───┐
// │ A ├──────────────────────►│B  │
// └───┘                       └───┘
// After
// ┌───┐     ┌───┐
// │ A ├────►│B  │
// └───┘     └───┘
//
// E.g. if it's moving away from all nodes, then it shouldn't keep moving back, even if there's a gap between it and the nearest behind
func compactAlongAxis(ctx context.Context, g *layoutgraph.Graph, isHorizontal, includeSizes bool, factor float64, edgeAbductions []*layoutgraph.EdgeAbduction, vEdges layoutgraph.Edges, guard *limits.OptimizationWorkGuard) (bool, error) {
	changed := false

	orderedNodes := orderedAlongAxis(g, isHorizontal)

	for _, node := range orderedNodes {
		if node.FixedTopLeft != nil {
			continue
		}
		possibleMoves, err := candidateMoves(ctx, g, node, factor, isHorizontal, includeSizes, 0, vEdges)
		if err != nil {
			return false, err
		}
		possibleMoves = append(possibleMoves, node.TopLeft.Copy())

		moved, err := moveNodeToBest(ctx, g, node, possibleMoves, edgeAbductions, includeSizes, guard)
		if err != nil {
			return false, err
		}
		if moved {
			changed = true
		}
	}

	return changed, nil
}

func visibilityEdges(ctx context.Context, g *layoutgraph.Graph, isHorizontal, includeSizes bool) (layoutgraph.Edges, error) {
	guard, err := limits.NewWorkGuard(ctx, "CompactionVisibility", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	edges := []*layoutgraph.Edge{}
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		for _, otherNode := range g.Nodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if node.ID == otherNode.ID {
				continue
			}

			delta := float64(node.DeltaTo(otherNode, node.TopLeft))
			if !node.VisibilityGraphCandidate(isHorizontal, true, includeSizes, otherNode, delta) {
				continue
			}

			blocked, err := isVisibilityGraphEdgeBlocked(g, isHorizontal, includeSizes, node, otherNode, guard)
			if err != nil {
				return nil, err
			}
			if !blocked {
				edges = append(edges, layoutgraph.NewEdge(node, otherNode))
			}
		}
	}

	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return edges, nil
}

func isVisibilityGraphEdgeBlocked(g *layoutgraph.Graph,
	isHorizontal, includeSizes bool,
	nodeA, nodeB *layoutgraph.Node,
	guard *limits.WorkGuard,
) (bool, error) {
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if (node.ID == nodeA.ID) || (node.ID == nodeB.ID) {
			continue
		}

		if node.IsBlocked(nodeA, nodeB, includeSizes, isHorizontal) {
			return true, nil
		}
	}

	return false, nil
}

func nearestFrom(edges layoutgraph.Edges, node *layoutgraph.Node, isHorizontal, includeSizes bool) *layoutgraph.Node {
	var nearestBehind *layoutgraph.Node

	largestBehindDistance := math.Inf(-1)

	for _, edge := range edges {
		if edge.To == node {
			if isHorizontal {
				if includeSizes {
					if edge.From.TopLeft.X+edge.From.Width > largestBehindDistance {
						largestBehindDistance = edge.From.TopLeft.X + edge.From.Width
						nearestBehind = edge.From
					}
				} else {
					if edge.From.TopLeft.X > largestBehindDistance {
						largestBehindDistance = edge.From.TopLeft.X
						nearestBehind = edge.From
					}
				}
			} else {
				if includeSizes {
					if edge.From.TopLeft.Y+edge.From.Height > largestBehindDistance {
						largestBehindDistance = edge.From.TopLeft.Y + edge.From.Height
						nearestBehind = edge.From
					}
				} else {
					if edge.From.TopLeft.Y > largestBehindDistance {
						largestBehindDistance = edge.From.TopLeft.Y
						nearestBehind = edge.From
					}
				}
			}
		}
	}

	return nearestBehind
}

// compactionFloor returns the floor value along an axis that a node must satisfy, given the anchor.
// if include sizes, it returns it as a number of cell size
func compactionFloor(g *layoutgraph.Graph, anchor *layoutgraph.Node, factor float64, isHorizontal, includeSizes bool, padding int) float64 {
	var floor float64

	if isHorizontal {
		if includeSizes {
			floor = math.Ceil((anchor.TopLeft.X + (factor * anchor.Width)) / g.CellSize)
		} else {
			floor = anchor.TopLeft.X + math.Floor(factor)
		}
	} else {
		if includeSizes {
			floor = math.Ceil((anchor.TopLeft.Y + (factor * anchor.Height)) / g.CellSize)
		} else {
			floor = anchor.TopLeft.Y + math.Floor(factor)
		}
	}

	if includeSizes {
		var anchorVal float64
		if isHorizontal {
			anchorVal = anchor.TopLeft.X + anchor.Width
		} else {
			anchorVal = anchor.TopLeft.Y + anchor.Height
		}
		for (floor*g.CellSize - anchorVal) <= float64(padding) {
			floor += 1
		}
	}

	return floor
}

func globallyFurthestBehind(g *layoutgraph.Graph, isHorizontal bool) *layoutgraph.Node {
	globallyFurthestBehind := g.Nodes[0]
	for _, n := range g.Nodes {
		if isHorizontal {
			if n.TopLeft.X < globallyFurthestBehind.TopLeft.X {
				globallyFurthestBehind = n
			}
		} else {
			if n.TopLeft.Y < globallyFurthestBehind.TopLeft.Y {
				globallyFurthestBehind = n
			}
		}
	}
	return globallyFurthestBehind
}

// candidateMoves returns the points a node may move to while compacting toward the nearest node behind it.
// floorDecrease is only safe to add when called with the node being the globally most behind
func candidateMoves(ctx context.Context, g *layoutgraph.Graph, node *layoutgraph.Node, factor float64, isHorizontal, includeSizes bool, floorDecrease int, vEdges layoutgraph.Edges) ([]*geo.Point, error) {
	guard, err := limits.NewWorkGuard(ctx, "CompactionCandidates", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	nearestBehind := nearestFrom(vEdges, node, isHorizontal, includeSizes)

	// Node is at the start of visibility graph chain
	if nearestBehind == nil {
		nearestBehind = globallyFurthestBehind(g, isHorizontal)
	}

	floor := compactionFloor(g, nearestBehind, factor, isHorizontal, includeSizes, node.DeltaTo(nearestBehind, node.TopLeft))

	hasFixed := g.HasFixedNode()

	if isHorizontal {
		// If it's same node or a node on another axis, no need add size buffer
		var nearestBehindAnotherAxis bool
		if includeSizes {
			nearestBehindAnotherAxis = (nearestBehind.TopLeft.Y > node.TopLeft.Y+node.Height) ||
				(nearestBehind.TopLeft.Y+nearestBehind.Height < node.TopLeft.Y)
		} else {
			nearestBehindAnotherAxis = nearestBehind.TopLeft.Y != node.TopLeft.Y
		}
		if (nearestBehind.TopLeft.X == node.TopLeft.X) || nearestBehindAnotherAxis {
			if includeSizes {
				floor = nearestBehind.TopLeft.X / g.CellSize
				// with fixed positions floor may not be exactly on a cellSize
				if hasFixed {
					floor = math.Round(floor)
				}
			} else {
				floor = nearestBehind.TopLeft.X
			}
		}
	} else {
		var nearestBehindAnotherAxis bool
		if includeSizes {
			nearestBehindAnotherAxis = (nearestBehind.TopLeft.X > node.TopLeft.X+node.Width) ||
				(nearestBehind.TopLeft.X+nearestBehind.Width < node.TopLeft.X)
		} else {
			nearestBehindAnotherAxis = nearestBehind.TopLeft.X != node.TopLeft.X
		}
		if (nearestBehind.TopLeft.Y == node.TopLeft.Y) || nearestBehindAnotherAxis {
			if includeSizes {
				floor = nearestBehind.TopLeft.Y / g.CellSize
				// with fixed positions floor may not be exactly on a cellSize
				if hasFixed {
					floor = math.Round(floor)
				}
			} else {
				floor = nearestBehind.TopLeft.Y
			}
		}
	}

	// Every node needs to include at least itself
	var ceil float64
	if isHorizontal {
		if includeSizes {
			ceil = node.TopLeft.X / g.CellSize
		} else {
			ceil = node.TopLeft.X
		}
	} else {
		if includeSizes {
			ceil = node.TopLeft.Y / g.CellSize
		} else {
			ceil = node.TopLeft.Y
		}
	}
	start := floor - float64(floorDecrease)
	if math.IsNaN(start) || math.IsInf(start, 0) || math.IsNaN(ceil) || math.IsInf(ceil, 0) {
		return nil, invariant.New("compaction candidate range is not finite")
	}
	candidateCount := 0
	if start <= ceil {
		count := math.Floor(ceil-start) + 1
		if count > maxCompactionCandidateCount {
			return nil, fmt.Errorf(
				"TALA compaction candidate count %.0f exceeds limit %d",
				count,
				maxCompactionCandidateCount,
			)
		}
		candidateCount = int(count)
	}
	positions := make([]*geo.Point, 0, candidateCount)
	for i := start; i <= ceil; i++ {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		var p geo.Point
		if isHorizontal {
			if includeSizes {
				p = *geo.NewPoint(i*g.CellSize, node.TopLeft.Y)
			} else {
				p = *geo.NewPoint(i, node.TopLeft.Y)
			}
		} else {
			if includeSizes {
				p = *geo.NewPoint(node.TopLeft.X, i*g.CellSize)
			} else {
				p = *geo.NewPoint(node.TopLeft.X, i)
			}
		}

		positions = append(positions, &p)
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return positions, nil
}
