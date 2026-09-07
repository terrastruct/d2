package placementcost

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/typedpool"
	"github.com/d2lang/d2/lib/geo"
)

type symmetryBoolScratch struct {
	values []bool
}

type symmetryNodeScratch struct {
	nodes []*layoutgraph.Node
	added map[*layoutgraph.Node]struct{}
}

var symmetryBoolPool = typedpool.New(func() *symmetryBoolScratch {
	return &symmetryBoolScratch{values: make([]bool, 0, 16)}
})

var symmetryNodePool = typedpool.New(func() *symmetryNodeScratch {
	return &symmetryNodeScratch{
		nodes: make([]*layoutgraph.Node, 0, 16),
		added: make(map[*layoutgraph.Node]struct{}, 16),
	}
})

func borrowSymmetryBools(length int) *symmetryBoolScratch {
	scratch := symmetryBoolPool.Get()
	if cap(scratch.values) < length {
		scratch.values = make([]bool, length)
		return scratch
	}
	scratch.values = scratch.values[:length]
	clear(scratch.values)
	return scratch
}

func returnSymmetryBools(scratch *symmetryBoolScratch) {
	scratch.values = scratch.values[:0]
	symmetryBoolPool.Put(scratch)
}

func borrowSymmetryNodes(capacity int) *symmetryNodeScratch {
	scratch := symmetryNodePool.Get()
	if cap(scratch.nodes) < capacity {
		scratch.nodes = make([]*layoutgraph.Node, 0, capacity)
	} else {
		scratch.nodes = scratch.nodes[:0]
	}
	clear(scratch.added)
	return scratch
}

func returnSymmetryNodes(scratch *symmetryNodeScratch) {
	clear(scratch.nodes[:cap(scratch.nodes)])
	scratch.nodes = scratch.nodes[:0]
	clear(scratch.added)
	symmetryNodePool.Put(scratch)
}

// NodesSymmetry sums local symmetry scores for nodes in order.
func NodesSymmetry(ctx context.Context, nodes layoutgraph.Nodes, edgeAbductions []*layoutgraph.EdgeAbduction) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	sum := 0.0
	for i, node := range nodes {
		if err := scoringCancellationError(ctx, i); err != nil {
			return 0, err
		}
		symmetry, err := nodeSymmetry(ctx, node, edgeAbductions, true)
		if err != nil {
			return 0, err
		}
		sum += symmetry
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	return sum, nil
}

// NodeSymmetry evaluates a node and the neighboring symmetry it contributes to.
func NodeSymmetry(ctx context.Context, node *layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction) (float64, error) {
	return nodeSymmetry(ctx, node, edgeAbductions, true)
}

func nodeSymmetry(ctx context.Context, node *layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction, checkNeighbors bool) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	score := 0.0
	maxScore := 0

	usedEdgeAbductionsScratch := borrowSymmetryBools(len(edgeAbductions))
	defer returnSymmetryBools(usedEdgeAbductionsScratch)
	usedEdgeAbductions := usedEdgeAbductionsScratch.values
	symmetryNodesScratch := borrowSymmetryNodes(len(node.Edges))
	dedupedAdjacentNodes := symmetryNodesScratch.nodes
	defer func() {
		symmetryNodesScratch.nodes = dedupedAdjacentNodes
		returnSymmetryNodes(symmetryNodesScratch)
	}()
	added := symmetryNodesScratch.added

	// We consider container children with protruding edges like neighbors for symmetry, their score matters for the container score
	if node.IsContainer() {
		childNodes := make([]*layoutgraph.Node, 0)
		// Within node placement, we use edge abductions to discern children; otherwise, the AdjacentNodes are correct
		if edgeAbductions == nil {
			for i, child := range node.Graph.Containers[node] {
				if err := scoringCancellationError(ctx, i); err != nil {
					return 0, err
				}
				for j, e := range child.Edges {
					if err := scoringCancellationError(ctx, j); err != nil {
						return 0, err
					}
					adj := child.Adjacent(e)
					// We only want to consider symmetry of child nodes with protrutions
					if !adj.Container.IsDescendantOf(node) {
						added[child] = struct{}{}
						childNodes = append(childNodes, child)
						break
					}
				}
			}
		} else {
			for i, edgeAbduction := range edgeAbductions {
				if err := scoringCancellationError(ctx, i); err != nil {
					return 0, err
				}
				if usedEdgeAbductions[i] {
					continue
				}
				if edgeAbduction.CurrentFrom == node {
					if edgeAbduction.OriginallyFrom != nil {
						usedEdgeAbductions[i] = true
						if _, in := added[edgeAbduction.OriginallyFrom]; in {
							continue
						}
						added[edgeAbduction.OriginallyFrom] = struct{}{}
						childNodes = append(childNodes, edgeAbduction.OriginallyFrom)
					}
				}
				if edgeAbduction.CurrentTo == node {
					if edgeAbduction.OriginallyTo != nil {
						usedEdgeAbductions[i] = true
						if _, in := added[edgeAbduction.OriginallyTo]; in {
							continue
						}
						added[edgeAbduction.OriginallyTo] = struct{}{}
						childNodes = append(childNodes, edgeAbduction.OriginallyTo)
					}
				}
			}
		}

		for i, nodeReplacement := range childNodes {
			if err := scoringCancellationError(ctx, i); err != nil {
				return 0, err
			}
			symmetry, err := nodeSymmetry(ctx, nodeReplacement, edgeAbductions, checkNeighbors)
			if err != nil {
				return 0, err
			}
			score += symmetry
			maxScore++
		}
	}

	// If a node has multiple edges to node A on one side, and only 1 edge to node B on another symmetric side, count that as symmetrical
	//   ┌───────────┐
	//   ▼           │
	// ┌──┐        ┌─┴┐       ┌──┐
	// │  │◄───────┤  ├──────►│  │
	// └──┘        └──┘       └──┘
	for edgeIndex, e := range node.Edges {
		if err := scoringCancellationError(ctx, edgeIndex); err != nil {
			return 0, err
		}
		adjacentNode := node.Adjacent(e)
		adjacentNodeReplacement := adjacentNode

		// Let's say A and A's container are both connected to B
		// ┌──────────┐
		// │          │
		// │   ┌───┐  │  ┌───┐
		// │   │ A ├──┼─►│B  │
		// │   └───┘  │  └───┘
		// │          │    ▲
		// └──────┬───┘    │
		//        │        │
		//        └────────┘
		// We've already considered A's symmetry above, so we should only count B as an adjacent node if it's not connected to a child
		isConnectedToChild := false
		if node.IsContainer() {
			for i, edgeAbduction := range edgeAbductions {
				if err := scoringCancellationError(ctx, i); err != nil {
					return 0, err
				}
				if (edgeAbduction.CurrentFrom == node) && (edgeAbduction.CurrentTo == adjacentNode) {
					if edgeAbduction.OriginallyFrom != nil {
						isConnectedToChild = true
						break
					}
				}
				if (edgeAbduction.CurrentFrom == adjacentNode) && (edgeAbduction.CurrentTo == node) {
					usedEdgeAbductions[i] = true
					if edgeAbduction.OriginallyTo != nil {
						isConnectedToChild = true
						break
					}
				}
			}
		}
		if isConnectedToChild {
			continue
		}

		// If we're considering symmetry for node B in the above diagram, we'll want to resolve to adjacent node to A, not container for A
		for i, edgeAbduction := range edgeAbductions {
			if err := scoringCancellationError(ctx, i); err != nil {
				return 0, err
			}
			if usedEdgeAbductions[i] {
				continue
			}
			// For symmetry, use cluster vessels
			if edgeAbduction.CurrentFrom != nil && edgeAbduction.CurrentFrom.IsClusterVessel() {
				continue
			}
			if edgeAbduction.CurrentTo != nil && edgeAbduction.CurrentTo.IsClusterVessel() {
				continue
			}
			if node.Graph.IsSequenceVessel(edgeAbduction.CurrentFrom) || node.Graph.IsSequenceVessel(edgeAbduction.CurrentTo) {
				continue
			}
			if (edgeAbduction.CurrentFrom == node) && (edgeAbduction.CurrentTo == adjacentNode) {
				usedEdgeAbductions[i] = true
				if edgeAbduction.OriginallyTo != nil {
					adjacentNodeReplacement = edgeAbduction.OriginallyTo
				}
				break
			}
			if (edgeAbduction.CurrentFrom == adjacentNode) && (edgeAbduction.CurrentTo == node) {
				usedEdgeAbductions[i] = true
				if edgeAbduction.OriginallyFrom != nil {
					adjacentNodeReplacement = edgeAbduction.OriginallyFrom
				}
				break
			}
		}

		// For clusters within containers, the Originally is pointed to the cluster node still
		//              ┌─────────┐
		//              │         │
		//              │         │
		//              │  ┌─┐    │
		// ┌──┐     ┌───┼─►│ │    │
		// │  ├─────┤   │  └─┘    │
		// └──┘     │   │         │
		//          │   │  ┌─┐    │
		//          └───┼─►│ │    │
		//              │  └─┘    │
		//              │         │
		//              └─────────┘
		if adjacentNodeReplacement.Cluster.IsActive() {
			adjacentNodeReplacement = adjacentNodeReplacement.Cluster.Vessel
		} else if adjacentNodeReplacement.Sequence.IsActive() {
			adjacentNodeReplacement = adjacentNodeReplacement.Sequence.Vessel
		}

		if _, in := added[adjacentNodeReplacement]; in {
			continue
		}

		dedupedAdjacentNodes = append(dedupedAdjacentNodes, adjacentNodeReplacement)
		added[adjacentNodeReplacement] = struct{}{}
	}

	// When we're resolving symmetry for children during node placement, all their edges were abducted to their containers, so here we find those
	for i, edgeAbduction := range edgeAbductions {
		if err := scoringCancellationError(ctx, i); err != nil {
			return 0, err
		}
		var connectedNode *layoutgraph.Node
		if edgeAbduction.OriginallyFrom == node {
			// When the adjacent node is also a container
			if edgeAbduction.OriginallyTo != nil &&
				edgeAbduction.CurrentTo != nil &&
				!edgeAbduction.CurrentTo.IsClusterVessel() &&
				!node.Graph.IsSequenceVessel(edgeAbduction.CurrentTo) {
				connectedNode = edgeAbduction.OriginallyTo
			} else {
				connectedNode = edgeAbduction.CurrentTo
			}
		}
		if edgeAbduction.OriginallyTo == node {
			if edgeAbduction.OriginallyFrom != nil &&
				edgeAbduction.CurrentFrom != nil &&
				!edgeAbduction.CurrentFrom.IsClusterVessel() &&
				!node.Graph.IsSequenceVessel(edgeAbduction.CurrentFrom) {
				connectedNode = edgeAbduction.OriginallyFrom
			} else {
				connectedNode = edgeAbduction.CurrentFrom
			}
		}
		if connectedNode != nil {
			if _, in := added[connectedNode]; in {
				continue
			}

			dedupedAdjacentNodes = append(dedupedAdjacentNodes, connectedNode)
			added[connectedNode] = struct{}{}
		}
	}

	for i := 0; i < len(dedupedAdjacentNodes); i++ {
		if err := scoringCancellationError(ctx, i); err != nil {
			return 0, err
		}
		d := node.DistanceTo(dedupedAdjacentNodes[i], true)
		if d > 1200 {
			dedupedAdjacentNodes = append(dedupedAdjacentNodes[:i], dedupedAdjacentNodes[i+1:]...)
			i--
		}
	}

	matchedScratch := borrowSymmetryBools(len(dedupedAdjacentNodes))
	defer returnSymmetryBools(matchedScratch)
	matched := matchedScratch.values
	s, err := computeSymmetryScoreInto(ctx, node, dedupedAdjacentNodes, matched)
	if err != nil {
		return 0, err
	}
	score += s
	maxScore += len(dedupedAdjacentNodes)

	// Node A may not have symmetry itself, but we still don't want it to move because it contributes to B's symmetry
	// So we check neighbors, see that B is symmetrical, which makes A also symmetrical
	// ┌──┐   ┌──┐   ┌──┐
	// │A │◄──┤B ├──►│C │
	// └──┘   └──┘   └──┘
	if checkNeighbors {
		for i, adjacentNode := range dedupedAdjacentNodes {
			if err := scoringCancellationError(ctx, i); err != nil {
				return 0, err
			}
			if matched[i] {
				continue
			}
			adjNodeSymm, err := nodeSymmetry(ctx, adjacentNode, edgeAbductions, false)
			if err != nil {
				return 0, err
			}
			score += adjNodeSymm
		}
	}

	// No adjacent nodes, not a container of a child with protruding edges
	if maxScore == 0 {
		return 0.0, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	return score / float64(maxScore), nil
}

/*
*
// This is ideal. Symmetrical through the center
// ┌──┐   ┌──┐   ┌──┐
// │  │◄──┤  ├──►│  │
// └──┘   └──┘   └──┘
//
// But this is not bad and should still be rewarded
//
//	  ┌──┐
//	┌─┤  ├─┐
//	│ └──┘ │
//	│      │
// ┌▼┐    ┌▼─┐
// │ │    │  │
// └─┘    └──┘
// So we give it a small score
*/
// computeSymmetryScoreInto writes match state into caller-owned storage so the
// optimizer can score symmetry without allocating.
func computeSymmetryScoreInto(ctx context.Context, node *layoutgraph.Node, neigh []*layoutgraph.Node, matchedSlice []bool) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	n := len(neigh)
	if n < 2 {
		return 0, nil
	}
	matchedSlice = matchedSlice[:n]
	clear(matchedSlice)

	score := 0.0

	ownSiblings := node.Graph.Containers[node.Container]
	var otherSiblings []*layoutgraph.Node

	for i, n1 := range neigh {
		if err := scoringCancellationError(ctx, i); err != nil {
			return 0, err
		}
		if matchedSlice[i] {
			continue
		}
		if n1.Container != node.Container {
			otherSiblings = node.Graph.Containers[n1.Container]
		} else {
			otherSiblings = nil
		}

		bestIdx := -1
		best := 0.0

		// When two nodes are considered mirrored for the sake of symmetry, we only want ones under the same container
		// This just doesn't look aesthetically symmetrical, even if it is (in a larger graph at least)
		//                    ┌──────────┐
		//                    │          │
		// ┌──┐        ┌─-┐   │   ┌──┐   │
		// │  │◄───────┤  ├───┼──►│  │   │
		// └──┘        └──┘   │   └──┘   │
		//                    │          │
		//                    └──────────┘
		for j := i + 1; j < n; j++ {
			if err := scoringCancellationError(ctx, j-i-1); err != nil {
				return 0, err
			}
			if matchedSlice[j] {
				continue
			}
			n2 := neigh[j]
			if n1.Container != n2.Container {
				continue
			}
			// When nodes vary by a lot in size, it also doesn't come off as symmetrical
			area1, area2 := n1.Area(), n2.Area()
			if area1 > 2*area2 || area2 > 2*area1 {
				continue
			}

			ms := math.Inf(-1)
			// The two nodes can be symmetrical across the x or y axis
			for _, isX := range [...]bool{true, false} {
				axis := node.TopLeft.X + node.Width/2
				if !isX {
					axis = node.TopLeft.Y + node.Height/2
				}
				if isMirrored(n1, n2, isX, axis) {
					if node.OverlapsAlongDimension(n1, isX, true) &&
						node.OverlapsAlongDimension(n2, isX, true) {
						ms = 2
					} else {
						ms = 0.5
					}
					// no need to continue, as there's no way n1-n2 is aligned both horizontally and vertically
					break
				}
			}
			// skip obstruction search if the nodes aren't aligned
			if ms < 0 {
				continue
			}

			// We only want to count mirrored objects when they have no obstructions from node to it
			// This won't look symmetrical
			//             ┌─────────┐
			//             │         │
			// ┌───┐     ┌─┴┐  ┌─┐  ┌▼─┐
			// │   ◄─────┤  │  │ │  │  │
			// └───┘     └──┘  │ │  └──┘
			//                 └─┘
			isObstructed, err := obstructed(ctx, node, n1, n2, ownSiblings, otherSiblings)
			if err != nil {
				return 0, err
			}
			if isObstructed {
				continue
			}

			if ms > best {
				best, bestIdx = ms, j
				if ms == 2 {
					break
				}
			}
		}

		if bestIdx != -1 {
			matchedSlice[i], matchedSlice[bestIdx] = true, true
			score += best
		}
	}

	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	return score, nil
}

func obstructed(ctx context.Context, center, a, b *layoutgraph.Node, sib1, sib2 []*layoutgraph.Node) (bool, error) {
	bounds := scoringNodeBounds(center).including(scoringNodeBounds(a)).including(scoringNodeBounds(b))
	check := func(sibs []*layoutgraph.Node) (bool, error) {
		for i, s := range sibs {
			if err := scoringCancellationError(ctx, i); err != nil {
				return false, err
			}
			if s == nil || s.TopLeft == nil || s.Graph != center.Graph {
				continue
			}
			if s == center || s == a || s == b {
				continue
			}
			if bounds.excludes(s) {
				continue
			}
			if center.IsDescendantOf(s) || a.IsDescendantOf(s) || b.IsDescendantOf(s) {
				continue
			}
			if s.IsDescendantOf(center) || s.IsDescendantOf(a) || s.IsDescendantOf(b) {
				continue
			}
			if s.PassesThrough(center.Center(), a.Center()) ||
				s.PassesThrough(center.Center(), b.Center()) {
				return true, nil
			}
		}
		return false, nil
	}
	isObstructed, err := check(sib1)
	if err != nil || isObstructed {
		return isObstructed, err
	}
	return check(sib2)
}

type columnCrossingScratch struct {
	edgeToAbduction map[*layoutgraph.Edge]*layoutgraph.EdgeAbduction
	abductionInput  int
	segments        []layoutgraph.CrossingSegment
}

const maxPooledColumnCrossingEntries = 4096

var columnCrossingScratchPool = typedpool.New(func() *columnCrossingScratch {
	return new(columnCrossingScratch)
})

func putColumnCrossingScratch(scratch *columnCrossingScratch) {
	if scratch.abductionInput > maxPooledColumnCrossingEntries ||
		len(scratch.edgeToAbduction) > maxPooledColumnCrossingEntries ||
		cap(scratch.segments) > maxPooledColumnCrossingEntries {
		return
	}
	clear(scratch.edgeToAbduction)
	scratch.abductionInput = 0
	scratch.segments = scratch.segments[:0]
	columnCrossingScratchPool.Put(scratch)
}

// ColumnCrossingCost scores crossings between labeled table-column edges.
func ColumnCrossingCost(ctx context.Context, node *layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	if !node.IsTable() {
		return 0, nil
	}

	scratch := columnCrossingScratchPool.Get()
	defer putColumnCrossingScratch(scratch)
	clear(scratch.edgeToAbduction)
	scratch.abductionInput = len(edgeAbductions)
	scratch.segments = scratch.segments[:0]
	if len(edgeAbductions) > 0 && scratch.edgeToAbduction == nil {
		scratch.edgeToAbduction = make(map[*layoutgraph.Edge]*layoutgraph.EdgeAbduction, min(len(edgeAbductions), maxPooledColumnCrossingEntries))
	}
	for i, abduction := range edgeAbductions {
		if err := scoringCancellationError(ctx, i); err != nil {
			return 0, err
		}
		scratch.edgeToAbduction[abduction.Edge] = abduction
	}

	for i, e := range node.Edges {
		if err := scoringCancellationError(ctx, i); err != nil {
			return 0, err
		}
		var fromPort, toPort geo.Point
		var hasFromPort, hasToPort bool
		if abduction, exists := scratch.edgeToAbduction[e]; exists {
			fromPort, toPort, hasFromPort, hasToPort, _ = e.FacingTablePortValues(abduction.OriginallyFrom, abduction.OriginallyTo)
		} else {
			fromPort, toPort, hasFromPort, hasToPort, _ = e.FacingTablePortValues(nil, nil)
		}

		if !hasFromPort || !hasToPort {
			continue
		}
		scratch.segments = append(scratch.segments, layoutgraph.CrossingSegment{Start: fromPort, End: toPort})
	}

	crossings, err := layoutgraph.CountSegmentCrossingsContext(ctx, scratch.segments)
	if err != nil {
		return 0, err
	}
	return layoutgraph.CrossingCostWeight * float64(crossings) * node.Graph.CellSize, nil
}

// isMirrored checks if nodeA is mirrored to nodeB along the given axis line
// isXAxis = true means they are mirrored along a vertical line
func isMirrored(nodeA, nodeB *layoutgraph.Node, isXAxis bool, axisVal float64) bool {
	nodeAtlX := nodeA.TopLeft.X
	nodeAtlY := nodeA.TopLeft.Y
	nodeAbrX := nodeA.TopLeft.X + nodeA.Width
	nodeAbrY := nodeA.TopLeft.Y + nodeA.Height

	nodeBtlX := nodeB.TopLeft.X
	nodeBtlY := nodeB.TopLeft.Y
	nodeBbrX := nodeB.TopLeft.X + nodeB.Width
	nodeBbrY := nodeB.TopLeft.Y + nodeB.Height

	symmetryTolerance := symmetryToleranceBand * nodeA.Graph.CellSize
	if isXAxis {
		if nodeA.TopLeft.X == nodeB.TopLeft.X {
			return false
		}

		// The left edge of node A should be on other side of axisVal from right edge of node B
		if nodeA.TopLeft.X > nodeB.TopLeft.X {
			// Axis val must be in between
			if !((axisVal > nodeBbrX) && (axisVal < nodeAtlX)) {
				return false
			}
			// Their distances from the axis line should be roughly equal
			if math.Abs((nodeAtlX-axisVal)-(axisVal-nodeBbrX)) > symmetryTolerance {
				return false
			}
		} else if nodeA.TopLeft.X < nodeB.TopLeft.X {
			if !((axisVal > nodeAbrX) && (axisVal < nodeBtlX)) {
				return false
			}
			if math.Abs((nodeBtlX-axisVal)-(axisVal-nodeAbrX)) > symmetryTolerance {
				return false
			}
		}

		// Centers match
		return math.Abs(((nodeAtlY+nodeAbrY)/2.0)-((nodeBtlY+nodeBbrY)/2.0)) <= symmetryTolerance
	}
	if nodeA.TopLeft.Y == nodeB.TopLeft.Y {
		return false
	}
	if nodeA.TopLeft.Y > nodeB.TopLeft.Y {
		if !((axisVal > nodeBbrY) && (axisVal < nodeAtlY)) {
			return false
		}
		if math.Abs((nodeAtlY-axisVal)-(axisVal-nodeBbrY)) > symmetryTolerance {
			return false
		}
	} else if nodeA.TopLeft.Y < nodeB.TopLeft.Y {
		if !((axisVal > nodeAbrY) && (axisVal < nodeBtlY)) {
			return false
		}
		if math.Abs((nodeBtlY-axisVal)-(axisVal-nodeAbrY)) > symmetryTolerance {
			return false
		}
	}

	// Centers match
	return math.Abs(((nodeAtlX+nodeAbrX)/2.0)-((nodeBtlX+nodeBbrX)/2.0)) <= symmetryTolerance
}
