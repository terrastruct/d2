package hierarchy

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/d2lang/d2/lib/geo"
)

// The routines below implement the algorithm described in `Fig. 1`, and section
// `4 Extending Sifting to k-Layered Directed Graphs`, of
// "Using Sifting for k-Layer Straightline Crossing Minimization" by Matuszewski et al.
// This iterative algorithm tries to find the optimal position for a given node
// initialize a queue of nodes in descending order
// for each node in the queue
//     swap with its siblings all the way to right
//     swap all the way the left
//     swap to the position with best crossings or length (this is the optimal position)
// swapping with neighbors is a local operation, so it should fast to compute new crossings
// however, as we handle containers, swapping container can lead to major changes and then we
// can't take full advantage of the local swaps
// main changes from the algorithm described in the paper:
// - they don't handle edge length
// - they reverse the queue if there's no improvement
//   - my udnerstanding is that the algorithm always process the whole queue, so it doesn't make sense to reverse it
// - if there's no improvement, allow changes if crossings or length stay the same (escape local minima)
// - if there's still no improvement, stop

func globalSifting(byLevel map[int][]*placementNode) error {
	queue := nodesInDescendingDegreeOrder(byLevel)
	nodeToSiblings := buildNodeToSiblings(queue, byLevel)
	var scratch crossingScratch
	improveIfEqualCrossings := false
	// 10 is just a killswitch (there's no suggested value in the paper)
	for iter := 0; iter < 10; iter++ {
		iterQueue := make([]*placementNode, len(queue))
		copy(iterQueue, queue)
		improved := false
		for len(iterQueue) > 0 {
			node := iterQueue[0]
			iterQueue = iterQueue[1:]
			oldRank := node.rank
			if err := sifting(node, nodeToSiblings[node], byLevel, improveIfEqualCrossings, &scratch); err != nil {
				return err
			}
			if node.rank != oldRank {
				improved = true
			}
		}
		if !improved {
			if improveIfEqualCrossings {
				// no improvement even considering reordering for the same crossings count
				break
			}
			improveIfEqualCrossings = true
		} else {
			improveIfEqualCrossings = false
		}
	}
	return nil
}

func nodesInDescendingDegreeOrder(byLevel map[int][]*placementNode) []*placementNode {
	var nodes []*placementNode
	for level := 0; level < len(byLevel); level++ {
		nodes = append(nodes, allDescendants(byLevel[level], true)...)
	}
	slices.SortStableFunc(nodes, func(a, b *placementNode) int {
		if order := cmp.Compare(b.degree(), a.degree()); order != 0 {
			return order
		}
		// If nodes have the same degree, sort by order from top-left.
		if order := cmp.Compare(a.level, b.level); order != 0 {
			return order
		}
		return cmp.Compare(a.rank, b.rank)
	})
	return nodes
}

func buildNodeToSiblings(nodes []*placementNode, byLevel map[int][]*placementNode) map[*placementNode][]*placementNode {
	nodeToSiblings := make(map[*placementNode][]*placementNode)
	for _, node := range nodes {
		if node.container != nil {
			nodeToSiblings[node] = node.container.children
		} else {
			nodeToSiblings[node] = byLevel[node.level]
		}
	}
	return nodeToSiblings
}

func sifting(node *placementNode, siblings []*placementNode, byLevel map[int][]*placementNode, improveIfEqualCrossings bool, scratch *crossingScratch) error {
	segments := scratch.crossLevelSegments(siblings, true, true)
	bestCrossings := countCrossings(segments)
	bestLength := addLengths(segments)
	bestI := -1

	if bestCrossings == 0 || len(siblings) == 1 {
		return nil
	} else if node == siblings[len(siblings)-1] {
		// handle edge case where the node is already the right most one
		bestI = len(siblings) - 1
	} else {
		// move all the way to the right
		for i := 0; i < len(siblings)-1; i++ {
			if bestI == -1 {
				if siblings[i] == node {
					bestI = i
				} else {
					continue
				}
			}

			siblings[i+1], siblings[i] = siblings[i], siblings[i+1]
			computeLevelRanks(byLevel[node.level])
			segments := scratch.crossLevelSegments(siblings, true, true)
			crossings := countCrossings(segments)
			length := addLengths(segments)
			if improved(crossings, bestCrossings, length, bestLength, improveIfEqualCrossings) {
				bestCrossings = crossings
				bestI = i + 1
				bestLength = length
			}
		}
		if siblings[len(siblings)-1] != node {
			return fmt.Errorf("sifting: expected node %s to have moved to the right of siblings slice", node.graphNode.DebugID())
		}
		if bestI == -1 {
			return fmt.Errorf("sifting: node %s not found", node.graphNode.DebugID())
		}
	}

	// move all the way to the left
	for i := len(siblings) - 1; i > 0; i-- {
		siblings[i-1], siblings[i] = siblings[i], siblings[i-1]
		computeLevelRanks(byLevel[node.level])
		segments := scratch.crossLevelSegments(siblings, true, true)
		crossings := countCrossings(segments)
		length := addLengths(segments)
		if improved(crossings, bestCrossings, length, bestLength, improveIfEqualCrossings) {
			bestCrossings = crossings
			bestI = i - 1
			bestLength = length
		}
	}
	if siblings[0] != node {
		return fmt.Errorf("sifting: expected node %s to have moved to the left of siblings slice", node.graphNode.DebugID())
	}

	// swap the node to the best rank and update the rank of all siblings
	for i := 0; i < len(siblings); i++ {
		if i == bestI {
			// at this moment, the node was already swapped to bestLocalRank
			break
		}
		siblings[i+1], siblings[i] = siblings[i], siblings[i+1]
	}
	computeLevelRanks(byLevel[node.level])
	return nil
}

func improved(crossings, bestCrossings int64, length, bestLength float64, improveIfEqualCrossings bool) bool {
	if !improveIfEqualCrossings {
		return crossings < bestCrossings || (crossings == bestCrossings && geo.PrecisionCompare(length, bestLength, geo.PRECISION) == -1)
	}
	return crossings <= bestCrossings || (crossings == bestCrossings && geo.PrecisionCompare(length, bestLength, geo.PRECISION) < 1)
}
