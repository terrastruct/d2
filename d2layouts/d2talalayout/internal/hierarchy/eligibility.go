package hierarchy

import (
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/nodeshape"
	"github.com/d2lang/d2/lib/shape"
)

// Candidates identifies the graph nodes that may participate in hierarchy
// discovery. Existing hierarchy membership is derived state and deliberately
// does not disqualify a node, so repeated layouts can reconsider topology and
// edge-direction changes.
func Candidates(graph *layoutgraph.Graph) map[*layoutgraph.Node]struct{} {
	candidates := make(map[*layoutgraph.Node]struct{})
	for _, node := range graph.Nodes {
		if _, inTree := graph.NodeToTree[node]; inTree {
			continue
		}
		if graph.IsTreeSentinel(node) || node.Sequence != nil || node.FixedTopLeft != nil {
			continue
		}
		if isSimpleCandidate(graph, node) {
			candidates[node] = struct{}{}
			continue
		}
		if node.IsContainer() && shapeCanBeContainer(node.Shape) && !node.AspectRatio1() && isEligibleContainer(graph, node) {
			candidates[node] = struct{}{}
		}
	}
	return candidates
}

func isSimpleCandidate(graph *layoutgraph.Graph, node *layoutgraph.Node) bool {
	return !(node.IsContainer() || graph.IsTreeSentinel(node) || node.IsClusterVessel() ||
		graph.IsSequenceVessel(node) || node.Cluster != nil)
}

// Hierarchy containers grow only horizontally. These shapes can grow in one
// direction without distorting their visual meaning.
func shapeCanBeContainer(candidate nodeshape.Shape) bool {
	switch candidate.GetType() {
	case "", shape.SQUARE_TYPE, shape.PACKAGE_TYPE, shape.STORED_DATA_TYPE, shape.QUEUE_TYPE, shape.STEP_TYPE:
		return true
	default:
		return false
	}
}

// A container can participate only when it has no internal edges and none of
// its descendants belongs to another specialized layout structure.
func isEligibleContainer(graph *layoutgraph.Graph, root *layoutgraph.Node) bool {
	for _, child := range graph.Containers[root] {
		if child.FixedTopLeft != nil {
			return false
		}
		if _, inTree := graph.NodeToTree[child]; inTree {
			return false
		}
		if graph.IsTreeSentinel(child) || child.Sequence != nil {
			return false
		}
		if !child.IsContainer() && !isSimpleCandidate(graph, child) {
			return false
		}
		for _, edge := range child.Edges {
			if !isHierarchyStructuralEdge(edge) {
				continue
			}
			if child.Adjacent(edge).IsDescendantOf(root) {
				return false
			}
		}
		if child.IsContainer() && !isEligibleContainer(graph, child) {
			return false
		}
	}
	return true
}
