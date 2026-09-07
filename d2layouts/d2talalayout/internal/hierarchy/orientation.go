package hierarchy

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

// IsHorizontal reports the orientation shared by hierarchy placement and
// hierarchy-aware routing. Entity-relationship diagrams use horizontal
// hierarchy placement regardless of the container direction.
func IsHorizontal(nodes []*layoutgraph.Node) bool {
	if len(nodes) == 0 {
		return false
	}
	if nodes[0].ContainerDirection().IsHorizontal() {
		return true
	}
	for _, node := range nodes {
		if node != nil && node.IsTable() {
			return true
		}
	}
	return false
}
