package hierarchy

import (
	"context"
	"math"
	"slices"
	"sort"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

// The routines below implement ideas described in:
// - "Fast and Simple Horizontal Coordinate Assignment" by Ulrik Brandes and Boris Kopf
// - the authors' erratum to that paper
// - "Node and Label Placement in a Layered Layout Algorithm" by John Julian Carstens
// 	- The code is based on the pseudo-code available in (page 28, section 3.2.1 The approach of Brandes and Kopf)
//
// The idea is to align nodes vertically so that we make the tallest vertical chain possible because straight edges are better/easier to read.
// To achieve this, the hierarchy is aligned 4 times chaging directions:
// * Top Left: the hierarchy is traversed from top to bottom, left to right.
// * Bottom Right: the hierarchy is traversed from bottom to top, right to left.
// * Top Right: just follow the same as above
// * Bottom left: just follow the same as above
//
// On each iteration the nodes are aligned to the median node above/below (depending on the direction) if possible.
// At the end, get the alignment that resulted in the narrowest layout to balance all alignments within its bounding box.
// Then, align all nodes to the median computed from the 4 alignments above.
//
// Attention: We only perform leaf descendants alignment, no containers are considered during the routine.
// The container edges are copied to the middle descendant and this node is aligned.
// At the end, we just wrap the containers around the descendants back again.

// Just a `tuple` to index the results of a given vertical/horizontal alignment
type alignmentDirection struct {
	vertical   geo.Orientation
	horizontal geo.Orientation
}

func alignHierarchy(ctx context.Context, g *layoutgraph.Graph, byLevel map[int][]*placementNode) error {
	guard, err := limits.NewWorkGuard(ctx, "AlignHierarchy", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	TL := &alignmentDirection{geo.Top, geo.Left}
	TR := &alignmentDirection{geo.Top, geo.Right}
	BR := &alignmentDirection{geo.Bottom, geo.Right}
	BL := &alignmentDirection{geo.Bottom, geo.Left}

	alignments := []*alignmentDirection{TL, TR, BL, BR}

	conflicts, err := markConflicts(ctx, byLevel)
	if err != nil {
		return err
	}
	minX := map[*alignmentDirection]float64{}
	maxX := map[*alignmentDirection]float64{}
	minWidth := math.Inf(1)
	var minWidthAlignment *alignmentDirection
	xs := map[*placementNode][]float64{}
	for _, d := range alignments {
		if err := guard.Step(); err != nil {
			return err
		}
		minX[d] = math.Inf(1)
		maxX[d] = math.Inf(-1)
		alignmentNodes, err := createAlignmentNodes(ctx, byLevel, d.vertical, d.horizontal)
		if err != nil {
			return err
		}
		if err := verticalAlignment(ctx, alignmentNodes, conflicts, d.horizontal); err != nil {
			return err
		}
		if err := horizontalCompaction(ctx, alignmentNodes, d.horizontal); err != nil {
			return err
		}
		for _, n := range alignmentNodes {
			if err := guard.Step(); err != nil {
				return err
			}
			x := n.x + (n.root.blockSize / 2.0) - (n.graphNode.Width / 2.0)
			xs[n.placementNode] = append(xs[n.placementNode], x)
			minX[d] = math.Min(minX[d], n.x)
			maxX[d] = math.Max(maxX[d], n.x+n.blockSize)
		}
		width := maxX[d] - minX[d]
		if width < minWidth {
			minWidth = width
			minWidthAlignment = d
		}
	}

	// balance
	shift := map[*alignmentDirection]float64{}
	for _, d := range alignments {
		if err := guard.Step(); err != nil {
			return err
		}
		if d.horizontal == geo.Left {
			shift[d] = minX[minWidthAlignment] - minX[d]
		} else {
			shift[d] = maxX[minWidthAlignment] - maxX[d]
		}
	}

	// final placement
	type placementUpdate struct {
		node *layoutgraph.Node
		x    float64
	}
	updates := make([]placementUpdate, 0, len(xs))
	for pn, x := range xs {
		if err := guard.Step(); err != nil {
			return err
		}
		for i := 0; i < len(xs[pn]); i++ {
			if err := guard.Step(); err != nil {
				return err
			}
			x[i] += shift[alignments[i]]
		}
		updates = append(updates, placementUpdate{node: pn.graphNode, x: math.Round(median(xs[pn]))})
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	for _, update := range updates {
		update.node.TopLeft.X = update.x
	}
	return syncContainers(g, guard)
}

// Reposition and resize containers based on the position of the aligned children.
// This routine goes from most to least nested container.
func syncContainers(g *layoutgraph.Graph, guard *limits.WorkGuard) error {
	var containers []*layoutgraph.Node
	queue := make([]*layoutgraph.Node, 0, len(g.Nodes))
	queue = append(queue, g.Nodes...)
	for len(queue) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		node := queue[0]
		if node.IsContainer() {
			containers = append(containers, node)
			queue = append(queue, g.Containers[node]...)
		}
		queue = queue[1:]
	}

	// most nested containers are the last ones added to the list
	for i := len(containers) - 1; i > -1; i-- {
		if err := guard.Step(); err != nil {
			return err
		}
		containers[i].WrapChildren()
	}
	return guard.Finish()
}

func median(numbers []float64) float64 {
	if len(numbers) == 0 {
		return 0
	} else if len(numbers) == 1 {
		return numbers[0]
	}

	ordered := make([]float64, len(numbers))
	copy(ordered, numbers)

	slices.Sort(ordered)

	if len(ordered)%2 == 1 {
		return ordered[(len(ordered) / 2)]
	}

	middle := len(ordered) / 2
	return ordered[middle-1]/2 + ordered[middle]/2
}

type alignmentNode struct {
	*placementNode

	prevSibling     *alignmentNode
	root            *alignmentNode
	alignedWith     *alignmentNode
	sink            *alignmentNode
	medianNeighbors []*alignmentNode
	shift           float64
	x               float64
	blockSize       float64
	rightPad        float64
	leftPad         float64
}

func newAlignmentNode(pn *placementNode) *alignmentNode {
	blockSize := pn.graphNode.Width

	node := &alignmentNode{
		placementNode:   pn,
		root:            nil,
		sink:            nil,
		alignedWith:     nil,
		shift:           0,
		x:               0,
		blockSize:       blockSize,
		prevSibling:     nil,
		medianNeighbors: nil,
		rightPad:        0,
		leftPad:         0,
	}

	node.root = node
	node.alignedWith = node
	node.sink = node

	return node
}

// Given layered nodes, creates a slice of alignment nodes sorted in the desired direction
func createAlignmentNodes(ctx context.Context, byLevel map[int][]*placementNode, vertical, horizontal geo.Orientation) ([]*alignmentNode, error) {
	guard, err := limits.NewWorkGuard(ctx, "CreateAlignmentNodes", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	placementToAlignment := make(map[*placementNode]*alignmentNode)
	var nodes []*alignmentNode
	for l := 0; l < len(byLevel); l++ {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		leaves, err := leafNodesContext(byLevel[l], guard)
		if err != nil {
			return nil, err
		}
		for _, node := range leaves {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			nodes = append(nodes, node)
			placementToAlignment[node.placementNode] = node
		}
	}

	sortAlignmentNodes(nodes, vertical, horizontal)
	if err := guard.Finish(); err != nil {
		return nil, err
	}

	x := math.Inf(-1)
	shift := math.Inf(1)
	if horizontal == geo.Right {
		x = math.Inf(1)
		shift = math.Inf(-1)
	}
	var previous *alignmentNode
	for _, node := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if previous != nil && node.level != previous.level {
			previous = nil
		}
		medianNeighbors, err := medianNeighborsContext(node, vertical, horizontal, placementToAlignment, guard)
		if err != nil {
			return nil, err
		}
		node.medianNeighbors = medianNeighbors
		node.prevSibling = previous
		node.x = x
		node.shift = shift
		previous = node
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func leafNodesContext(nodes []*placementNode, guard *limits.WorkGuard) ([]*alignmentNode, error) {
	var leaves []*alignmentNode
	for i := 0; i < len(nodes); i++ {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		node := nodes[i]
		if !node.isContainer {
			leaves = append(leaves, newAlignmentNode(node))
			continue
		}
		// Instead of picking a single 'middle' child, distribute container edges
		// among children whose rank is closest to the other endpoint's rank.
		descendants, err := leafNodesContext(node.children, guard) // recursively collect leaf descendants
		if err != nil {
			return nil, err
		}

		pickChildForEdge := func(edgeEndpoint *placementNode, possible []*alignmentNode) (*alignmentNode, error) {
			// simple heuristic: pick child with rank closest to edgeEndpoint's rank
			best := possible[0]
			minDiff := absInt(edgeEndpoint.rank - best.placementNode.rank)
			for _, cand := range possible {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				diff := absInt(edgeEndpoint.rank - cand.placementNode.rank)
				if diff < minDiff {
					minDiff = diff
					best = cand
				}
			}
			return best, nil
		}

		// rewire "aboves" from container to children
		for above := range node.aboves {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			delete(above.belows, node)
			bestChild, err := pickChildForEdge(above, descendants)
			if err != nil {
				return nil, err
			}
			above.belows[bestChild.placementNode] = struct{}{}
			bestChild.aboves[above] = struct{}{}
		}

		// rewire "belows" from container to children
		for below := range node.belows {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			delete(below.aboves, node)
			bestChild, err := pickChildForEdge(below, descendants)
			if err != nil {
				return nil, err
			}
			below.aboves[bestChild.placementNode] = struct{}{}
			bestChild.belows[below] = struct{}{}
		}

		// Add the descendants to the final leaves
		leaves = append(leaves, descendants...)
	}
	// we need to consider the container padding to properly align the nodes
	// the first container child has the left pad and the last one has the right pad
	// this applied recursively (as this function is recursive) because we can have containers inside containers
	leftNode, rightNode := leaves[0], leaves[len(leaves)-1]
	leftNode.leftPad += leftNode.containerPadding().Left()
	rightNode.rightPad += rightNode.containerPadding().Right()
	return leaves, nil
}

func (pn *placementNode) containerPadding() layoutgraph.Spacing {
	if pn == nil || pn.isDummy {
		return layoutgraph.UniformSpacing(layoutgraph.ContainerPadding)
	}
	var container *layoutgraph.Node
	if pn.container != nil {
		container = pn.container.graphNode
	}
	return pn.graphNode.Graph.ContainerPadding(container, true)
}

func sortAlignmentNodes(nodes []*alignmentNode, vertical, horizontal geo.Orientation) {
	if vertical == geo.Top && horizontal == geo.Left {
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].level == nodes[j].level {
				return nodes[i].rank < nodes[j].rank
			}
			return nodes[i].level < nodes[j].level
		})
	} else if vertical == geo.Top && horizontal == geo.Right {
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].level == nodes[j].level {
				return nodes[j].rank < nodes[i].rank
			}
			return nodes[i].level < nodes[j].level
		})
	} else if vertical == geo.Bottom && horizontal == geo.Left {
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].level == nodes[j].level {
				return nodes[i].rank < nodes[j].rank
			}
			return nodes[j].level < nodes[i].level
		})
	} else if vertical == geo.Bottom && horizontal == geo.Right {
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].level == nodes[j].level {
				return nodes[j].rank < nodes[i].rank
			}
			return nodes[j].level < nodes[i].level
		})
	}
}

func medianNeighborsContext(
	n *alignmentNode,
	vertical, horizontal geo.Orientation,
	placementToAlignment map[*placementNode]*alignmentNode,
	guard *limits.WorkGuard,
) ([]*alignmentNode, error) {
	var neighbors []*alignmentNode

	if vertical == geo.Top {
		for above := range n.aboves {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if above.level == n.level-1 {
				neighbors = append(neighbors, placementToAlignment[above])
			}
		}
	} else {
		for below := range n.belows {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if below.level == n.level+1 {
				neighbors = append(neighbors, placementToAlignment[below])
			}
		}
	}

	if len(neighbors) == 0 {
		return neighbors, nil
	}

	sort.Slice(neighbors, func(i, j int) bool {
		return neighbors[i].graphNode.TopLeft.X < neighbors[j].graphNode.TopLeft.X
	})

	mid := float64(len(neighbors)+1) / 2.0
	if len(neighbors)%2 == 1 {
		// exact median
		medianIndex := int(mid) - 1
		return []*alignmentNode{neighbors[medianIndex]}, nil
	} else {
		// consider left and right median
		leftMedian := int(math.Floor(mid)) - 1
		rightMedian := int(math.Ceil(mid)) - 1
		if horizontal == geo.Right {
			leftMedian, rightMedian = rightMedian, leftMedian
		}
		return []*alignmentNode{neighbors[leftMedian], neighbors[rightMedian]}, nil
	}
}

func allAbove(pn *placementNode) []*placementNode {
	var result []*placementNode
	for above := range pn.aboves {
		result = append(result, above)
	}
	return result
}

func markConflicts(ctx context.Context, byLevel map[int][]*placementNode) (map[*placementNode]map[*placementNode]struct{}, error) {
	guard, err := limits.NewWorkGuard(ctx, "MarkHierarchyConflicts", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	conflicts := make(map[*placementNode]map[*placementNode]struct{})
	addConflict := func(n1, n2 *placementNode) {
		if _, exists := conflicts[n1]; !exists {
			conflicts[n1] = make(map[*placementNode]struct{})
		}
		conflicts[n1][n2] = struct{}{}
	}

	// skip the first and last level because there are no dummy nodes in them
	for level := 1; level < len(byLevel)-1; level++ {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		nextLevel := level + 1
		k0 := 0
		l := 0
		for l1, pn := range byLevel[nextLevel] {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			var k1 int
			if l1 == len(byLevel[nextLevel])-1 {
				last := len(byLevel[level]) - 1
				k1 = byLevel[level][last].rank
			} else if pn.isDummy {
				for _, above := range allAbove(pn) {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					// dummy nodes have only one edge above and one below
					if !above.isDummy {
						continue
					}
					k1 = above.rank
				}
			} else {
				continue
			}
			for ; l <= l1; l++ {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				n := byLevel[nextLevel][l]
				for above := range n.aboves {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					if above.rank < k0 || above.rank > k1 {
						addConflict(n, above)
						addConflict(above, n)
					}
				}
			}
			k0 = k1
		}
	}

	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return conflicts, nil
}

func verticalAlignment(ctx context.Context, nodes []*alignmentNode, conflicts map[*placementNode]map[*placementNode]struct{}, horizontal geo.Orientation) error {
	guard, err := limits.NewWorkGuard(ctx, "VerticalAlignment", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	hasConflict := func(n1, n2 *alignmentNode) bool {
		if c, n1Exists := conflicts[n1.placementNode]; n1Exists {
			_, n2Exists := c[n2.placementNode]
			return n2Exists
		}
		return false
	}

	lastAlignedRank := math.MaxInt
	for _, n := range nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		// given a level with nodes
		// a b c
		// when horizontal = Left
		// a.previousSibling = nil
		// b.previousSibling = a
		// c.previousSibling = b
		// when horizontal = Right
		// a.previousSibling = b
		// b.previousSibling = c
		// c.previousSibling = nil
		// This is how `createAlignmentNodes` creates the slice
		if n.prevSibling == nil {
			if horizontal == geo.Left {
				lastAlignedRank = math.MinInt
			} else {
				lastAlignedRank = math.MaxInt
			}
		}

		for _, m := range n.medianNeighbors {
			if err := guard.Step(); err != nil {
				return err
			}
			if hasConflict(n, m) {
				continue
			} else if n.alignedWith != n {
				// already aligned
				continue
			} else if horizontal == geo.Left && lastAlignedRank >= m.rank {
				// this would align crossing edges, skip
				continue
			} else if horizontal == geo.Right && lastAlignedRank <= m.rank {
				// this would align crossing edges, skip
				continue
			}
			m.alignedWith = n
			n.root = m.root
			n.alignedWith = n.root
			m.root.blockSize = math.Max(m.root.blockSize, n.blockSize)
			m.root.leftPad = math.Max(m.root.leftPad, n.leftPad)
			m.root.rightPad = math.Max(m.root.rightPad, n.rightPad)
			lastAlignedRank = m.rank
		}
	}
	return guard.Finish()
}

func horizontalCompaction(ctx context.Context, nodes []*alignmentNode, horizontalDirection geo.Orientation) error {
	guard, err := limits.NewWorkGuard(ctx, "HorizontalCompaction", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if n.root == n {
			if err := placeBlock(n, horizontalDirection, guard); err != nil {
				return err
			}
		}
	}
	for _, n := range nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		n.x = n.root.x
		if n.root == n && !math.IsInf(n.sink.shift, 0) {
			n.x += n.sink.shift
		}
	}
	return guard.Finish()
}

func placeBlock(root *alignmentNode, horizontal geo.Orientation, guard *limits.WorkGuard) error {
	if err := guard.Step(); err != nil {
		return err
	}
	if !math.IsInf(root.x, 0) {
		return nil
	}
	root.x = 0
	n := root
	for {
		if err := guard.Step(); err != nil {
			return err
		}
		if n.prevSibling != nil {
			prevRoot := n.prevSibling.root
			if err := placeBlock(prevRoot, horizontal, guard); err != nil {
				return err
			}
			if root.sink == root {
				root.sink = prevRoot.sink
			}
			delta := distanceFromPreviousRoot(prevRoot, root, horizontal)
			if root.sink != prevRoot.sink {
				if horizontal == geo.Left {
					prevRoot.sink.shift = math.Min(prevRoot.sink.shift, root.x-prevRoot.x-prevRoot.blockSize-delta)
				} else {
					prevRoot.sink.shift = math.Max(prevRoot.sink.shift, root.x-prevRoot.x+root.blockSize+delta)
				}
			} else {
				if horizontal == geo.Left {
					root.x = math.Max(root.x, prevRoot.x+prevRoot.blockSize+delta)
				} else {
					root.x = math.Min(root.x, prevRoot.x-root.blockSize-delta)
				}
			}
		}
		n = n.alignedWith
		if n == root {
			break
		}
	}
	return nil
}

/*
By default, nodes in hierarchies are siblingSpacing from their siblings.
Though, when a node is inside a container, this distance must be increased by the container padding.
For the first node of a container, this pad is added to the left.
For the last node of a container, the pad is added to the right.
The recursiveness of the pad (containers inside containers) is considered when creating the node.
This function only combines poadding in the given direction.
*/
func distanceFromPreviousRoot(previous, current *alignmentNode, horizontal geo.Orientation) float64 {
	pad := siblingSpacing
	if current.isDummy || (current.prevSibling != nil && current.prevSibling.isDummy) {
		pad = siblingDummySpacing
	}
	if horizontal == geo.Left {
		pad += previous.rightPad + current.leftPad
	} else {
		pad += previous.leftPad + current.rightPad
	}
	return pad
}
