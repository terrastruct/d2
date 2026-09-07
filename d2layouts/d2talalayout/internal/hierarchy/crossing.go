package hierarchy

import (
	"math"
	"slices"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

const crossingComparisonPrecision = 0.0001

type crossingScratch struct {
	segments []layoutgraph.CrossingSegment
}

// The routines below are part of the implementation of the `ordering` procedure
// described in section `3. Vertex Ordering Within Ranks` of
// "A Technique for Drawing Directed Graphs" by Gansner et al.
// This is an iterative algorithm that tries to find the best node permutation (order) for each level
// for each iteration:
//     for each level
//         sort the nodes based on the `rank` of its connections
//         try swapping the nodes with their neighbors if crossings or length improve
//         recurse to children (sorting, swapping)
// main differences from the algorithm described in the paper:
// - they don't handle containers
// - they don't consider the edge lengths in case crossings stay the same
// - they consider only one direction when computing crossings

func initializeRanks(byLevel map[int][]*placementNode) {
	for i := 0; i < len(byLevel); i++ {
		computeLevelRanks(byLevel[i])
	}
}

// minimizes crossings iteratively. After minimization, recomputes the nodes ranks
func minimizeHierarchyCrossings(byLevel map[int][]*placementNode) {
	widestLevel := 0
	for _, nodes := range byLevel {
		widestLevel = max(widestLevel, len(nodes))
	}
	var scratch crossingScratch
	iterCount := widestLevel
	for n := 0; n < iterCount; n++ {
		for level := 0; level < len(byLevel); level++ {
			minimizeCrossings(byLevel[level], byLevel, level > 0, &scratch)
		}
	}
}

/*
This is the main routine to find the optimal node rank compared to its siblings.
Ranks are optimized considering edges above the given level.
|	Except for the first (top level), in which the optimization considers the edges below
Crossings take precedence to be optimized, then if they don't change, it tries to get the best length.
To find the best rank it first sorts the nodes so that it is "pulled" towards its connected neighbors in the direction under optimization.
Then, it tries to swap each node with its neighbors.
Once the nodes were ranked properly, if any of these nodes are containers, apply the same routine recursively to it's descendants.
The premise for containers is that they contain the same edges as their descendants so they are first placed at a proper rank.
Then, their descendants only need to be optimized against themselves without considering the container siblings.
*/
func minimizeCrossings(nodes []*placementNode, byLevel map[int][]*placementNode, isTopDown bool, scratch *crossingScratch) {
	sortLevelNodesByAdacencyPosition(nodes, isTopDown)
	computeLevelRanks(byLevel[nodes[0].level])
	segments := scratch.crossLevelSegments(nodes, true, true)
	crossings := countCrossings(segments)
	length := addLengths(segments)
	for j := 0; j < len(nodes); j++ {
		newCrossings, newLength, swapIndex := bestIndexBySwappingNeighbors(nodes, j, byLevel, scratch)
		if newCrossings < crossings {
			crossings = newCrossings
			length = newLength
			nodes[j], nodes[swapIndex] = nodes[swapIndex], nodes[j]
		} else if newCrossings == crossings && geo.PrecisionCompare(newLength, length, crossingComparisonPrecision) == -1 {
			crossings = newCrossings
			length = newLength
			nodes[j], nodes[swapIndex] = nodes[swapIndex], nodes[j]
		}
		computeLevelRanks(byLevel[nodes[0].level])
	}
	for _, node := range nodes {
		if len(node.children) > 0 && node.optimizeChildrenCrossings {
			minimizeCrossings(node.children, byLevel, isTopDown, scratch)
		}
	}
}

/*
Swap a given node with it's 3 neighbors to the right and returns the best crossing and index of this swap
This function returns the level nodes as they were, it does not change their position.
For the last nodes, it tries to swap them with the first ones in a sort of "circular" approach.
*/
func bestIndexBySwappingNeighbors(levelNodes []*placementNode, nodeIndex int, byLevel map[int][]*placementNode, scratch *crossingScratch) (int64, float64, int) {
	bestCrossing := int64(math.MaxInt64)
	bestLength := math.Inf(1)
	bestIndex := 0
	for i := nodeIndex + 1; i < nodeIndex+4; i++ {
		// swap to count new crossings
		j := i % len(levelNodes)
		levelNodes[nodeIndex], levelNodes[j] = levelNodes[j], levelNodes[nodeIndex]
		computeLevelRanks(byLevel[levelNodes[nodeIndex].level])
		segments := scratch.crossLevelSegments(levelNodes, true, true)
		newCrossings := countCrossings(segments)
		newLength := addLengths(segments)
		if newCrossings < bestCrossing {
			bestCrossing = newCrossings
			bestIndex = j
			bestLength = newLength
		} else if newCrossings == bestCrossing && geo.PrecisionCompare(newLength, bestLength, crossingComparisonPrecision) == -1 {
			bestCrossing = newCrossings
			bestIndex = j
			bestLength = newLength
		}
		// swap back, so that node[j] swaps with the next one
		levelNodes[j], levelNodes[nodeIndex] = levelNodes[nodeIndex], levelNodes[j]
	}
	return bestCrossing, bestLength, bestIndex
}

// adjacentLevelNodes can be either the nodes above or below, depending on the optimization direction
func (scratch *crossingScratch) crossLevelSegments(nodes []*placementNode, aboves, belows bool) []layoutgraph.CrossingSegment {
	segmentCount := 0
	iterAllDescendants(nodes, func(pn *placementNode) {
		if aboves {
			segmentCount += len(pn.aboves)
		}
		if belows {
			segmentCount += len(pn.belows)
		}
	})
	if cap(scratch.segments) < segmentCount {
		scratch.segments = make([]layoutgraph.CrossingSegment, 0, segmentCount)
	} else {
		scratch.segments = scratch.segments[:0]
	}
	iterAllDescendants(nodes, func(pn *placementNode) {
		if aboves {
			for connected := range pn.aboves {
				scratch.segments = append(scratch.segments, layoutgraph.CrossingSegment{
					Start: geo.Point{X: float64(pn.rank), Y: float64(pn.level)},
					End:   geo.Point{X: float64(connected.rank), Y: float64(connected.level)},
				})
			}
		}
		if belows {
			for connected := range pn.belows {
				scratch.segments = append(scratch.segments, layoutgraph.CrossingSegment{
					Start: geo.Point{X: float64(pn.rank), Y: float64(pn.level)},
					End:   geo.Point{X: float64(connected.rank), Y: float64(connected.level)},
				})
			}
		}
	})
	slices.SortStableFunc(scratch.segments, func(a, b layoutgraph.CrossingSegment) int {
		if a.Start.X == b.Start.X {
			switch {
			case a.End.X < b.End.X:
				return -1
			case b.End.X < a.End.X:
				return 1
			default:
				return 0
			}
		}
		switch {
		case a.Start.X < b.Start.X:
			return -1
		case b.Start.X < a.Start.X:
			return 1
		default:
			return 0
		}
	})

	return scratch.segments
}

func iterAllDescendants(nodes []*placementNode, f func(*placementNode)) {
	for _, pn := range nodes {
		f(pn)
	}
	for _, pn := range nodes {
		iterAllDescendants(pn.children, f)
	}
}

func allDescendants(nodes []*placementNode, forOptimization bool) []*placementNode {
	allNodes := make([]*placementNode, len(nodes))
	copy(allNodes, nodes)
	for _, pn := range nodes {
		if !forOptimization || pn.optimizeChildrenCrossings {
			allNodes = append(allNodes, allDescendants(pn.children, forOptimization)...)
		}
	}
	return allNodes
}

func addLengths(segments []layoutgraph.CrossingSegment) float64 {
	length := 0.0
	for _, s := range segments {
		length += geo.EuclideanDistance(s.Start.X, s.Start.Y, s.End.X, s.End.Y)
	}
	return length
}

func countCrossings(segments []layoutgraph.CrossingSegment) int64 {
	return layoutgraph.CountSegmentCrossings(segments)
}

func sortLevelNodesByAdacencyPosition(nodes []*placementNode, useConnectionsAbove bool) {
	adjacentLevelAverage := func(pn *placementNode) float64 {
		nodes := pn.aboves
		if !useConnectionsAbove {
			nodes = pn.belows
		}
		if len(nodes) == 0 {
			return 0
		}

		sum := 0
		for connected := range nodes {
			sum += connected.rank
		}
		return float64(sum) / float64(len(nodes))
	}

	slices.SortStableFunc(nodes, func(a, b *placementNode) int {
		aAverage := adjacentLevelAverage(a)
		bAverage := adjacentLevelAverage(b)
		switch {
		case aAverage < bAverage:
			return -1
		case bAverage < aAverage:
			return 1
		default:
			return 0
		}
	})
}

/*
Computes the nodes rank (index) from left to right as if they were sorted by TopLeft.X.
Ranking is performed by Depth First Search (DFS) adding nodes in reverse order.
By adding the nodes on the right first, they get stacked first and popped only after the nodes more on the left, yielding the desired
effect of processing the nodes on the left before the ones on the right
*/
func computeLevelRanks(nodes []*placementNode) {
	stack := make([]*placementNode, len(nodes))
	for i := 0; i < len(nodes); i++ {
		stack[i] = nodes[len(nodes)-i-1]
	}
	for rank := 0; len(stack) > 0; rank++ {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		current.rank = rank
		for i := len(current.children) - 1; i > -1; i-- {
			stack = append(stack, current.children[i])
		}
	}
}
