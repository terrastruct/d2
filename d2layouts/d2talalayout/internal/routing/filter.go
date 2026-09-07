package routing

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

// filterEdgeAncestors removes container ancestors of either endpoint from the
// obstacle set used to route an edge. The endpoints themselves remain so
// arrowhead-label placement can still account for their boxes.
func filterEdgeAncestors(edge *layoutgraph.Edge, nodes layoutgraph.Nodes) (nonAncestors layoutgraph.Nodes) {
	for _, node := range nodes {
		if node != edge.From && node != edge.To &&
			(edge.From.IsDescendantOf(node) || edge.To.IsDescendantOf(node)) {
			continue
		}
		nonAncestors = append(nonAncestors, node)
	}
	return nonAncestors
}
