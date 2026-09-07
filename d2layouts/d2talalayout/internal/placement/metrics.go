package placement

import (
	"sort"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

// occupied reports whether point is the top-left position of a graph node.
func occupied(graph *layoutgraph.Graph, point *geo.Point) (*layoutgraph.Node, bool) {
	for _, node := range graph.Nodes {
		if node.TopLeft != nil && point != nil && *node.TopLeft == *point {
			return node, true
		}
	}
	return nil, false
}

// withinMaxSize reports whether the graph fits inside the supported placement
// bounds.
func withinMaxSize(graph *layoutgraph.Graph) bool {
	topLeft, bottomRight := graph.BoundingBox()
	width := bottomRight.X - topLeft.X
	height := bottomRight.Y - topLeft.Y
	return width <= limits.MaxGraphSize && height <= limits.MaxGraphSize
}

// intersectsOtherNode reports whether the center-to-center segment crosses an
// unrelated graph node.
func intersectsOtherNode(graph *layoutgraph.Graph, first, second *layoutgraph.Node) bool {
	firstCenter := first.Center()
	secondCenter := second.Center()
	for _, other := range graph.Nodes {
		if other == first || other == second {
			continue
		}
		if first.IsDescendantOf(other) || second.IsDescendantOf(other) {
			continue
		}
		if other.IsDescendantOf(first) || other.IsDescendantOf(second) {
			continue
		}
		if other.PassesThrough(firstCenter, secondCenter) {
			return true
		}
	}
	return false
}

func median(nodes layoutgraph.Nodes, includeSizes bool) (float64, float64) {
	orderedByX := append(layoutgraph.Nodes(nil), nodes...)
	orderedByY := append(layoutgraph.Nodes(nil), nodes...)
	sort.Slice(orderedByX, func(i, j int) bool {
		left, right := orderedByX[i], orderedByX[j]
		leftX, rightX := left.TopLeft.X, right.TopLeft.X
		if includeSizes {
			leftX += left.Width / 2
			rightX += right.Width / 2
		}
		if leftX == rightX {
			return left.ID < right.ID
		}
		return leftX < rightX
	})
	sort.Slice(orderedByY, func(i, j int) bool {
		left, right := orderedByY[i], orderedByY[j]
		leftY, rightY := left.TopLeft.Y, right.TopLeft.Y
		if includeSizes {
			leftY += left.Height / 2
			rightY += right.Height / 2
		}
		if leftY == rightY {
			return left.ID < right.ID
		}
		return leftY < rightY
	})

	middle := len(nodes) / 2
	if includeSizes {
		medianX := orderedByX[middle].TopLeft.X + orderedByX[middle].Width/2
		medianY := orderedByY[middle].TopLeft.Y + orderedByY[middle].Height/2
		if len(nodes)%2 == 0 {
			medianX = (medianX + orderedByX[middle-1].TopLeft.X + orderedByX[middle-1].Width/2) / 2
			medianY = (medianY + orderedByY[middle-1].TopLeft.Y + orderedByY[middle-1].Height/2) / 2
		}
		return medianX / nodes[0].Graph.CellSize, medianY / nodes[0].Graph.CellSize
	}
	medianX := orderedByX[middle].TopLeft.X + 0.5
	medianY := orderedByY[middle].TopLeft.Y + 0.5
	if len(nodes)%2 == 0 {
		medianX = (medianX + orderedByX[middle-1].TopLeft.X) / 2
		medianY = (medianY + orderedByY[middle-1].TopLeft.Y) / 2
	}
	return medianX, medianY
}

// medianToNeighbors approximates the two-dimensional geometric median of a
// node's currently positioned placement neighbors.
func medianToNeighbors(node *layoutgraph.Node, includeSizes bool, abductions []*layoutgraph.EdgeAbduction) (float64, float64) {
	return median(adjacents(node, abductions), includeSizes)
}

func adjacents(node *layoutgraph.Node, abductions []*layoutgraph.EdgeAbduction) []*layoutgraph.Node {
	result := make([]*layoutgraph.Node, 0, len(node.Edges))
	usedScratch := borrowEdgeAbductionBools(len(abductions))
	defer returnEdgeAbductionBools(usedScratch)
	used := usedScratch.values
	for _, edge := range node.Edges {
		adjacent := node.Adjacent(edge)
		if adjacent.TopLeft == nil {
			continue
		}
		add := adjacent
		for i, abduction := range abductions {
			if used[i] {
				continue
			}
			if abduction.CurrentFrom == node && abduction.CurrentTo == adjacent && abduction.OriginallyTo != nil {
				used[i], add = true, abduction.OriginallyTo
				break
			}
			if abduction.CurrentFrom == adjacent && abduction.CurrentTo == node && abduction.OriginallyFrom != nil {
				used[i], add = true, abduction.OriginallyFrom
				break
			}
		}
		result = append(result, add)
	}
	if len(result) != 0 {
		return result
	}

	for _, near := range node.OrderedNears() {
		if near.Cluster.IsActive() {
			near = near.Cluster.Vessel
		} else if near.Sequence.IsActive() {
			near = near.Sequence.Vessel
		}
		if tree, ok := node.Graph.NodeToTree[near]; ok {
			for tree.Parent != nil {
				tree = tree.Parent
			}
			near = tree.SentinelNode()
		}
		if near.TopLeft != nil && near.IsDescendantOf(node.Container) {
			result = append(result, near)
		}
	}
	if len(result) != 0 {
		return result
	}

	var nears []*layoutgraph.Node
	if node.IsClusterVessel() {
		for _, child := range node.Graph.Clusters[node].Nodes {
			nears = append(nears, child.OrderedNears()...)
		}
	}
	if sequence := node.Graph.Sequences[node]; sequence != nil {
		for _, child := range sequence.Nodes {
			nears = append(nears, child.OrderedNears()...)
		}
	}
	for _, tree := range node.Graph.Trees[node] {
		queue := []*layoutgraph.Tree{tree}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			nears = append(nears, current.Node.OrderedNears()...)
			queue = append(queue, current.Children...)
		}
	}
	for _, near := range nears {
		add := near
		if near.Cluster.IsActive() {
			add = near.Cluster.Vessel
		} else if near.Sequence.IsActive() {
			add = near.Sequence.Vessel
		}
		if tree, ok := node.Graph.NodeToTree[add]; ok {
			for tree.Parent != nil {
				tree = tree.Parent
			}
			add = tree.SentinelNode()
		}
		if add.TopLeft != nil {
			result = append(result, add)
		}
	}
	return result
}
