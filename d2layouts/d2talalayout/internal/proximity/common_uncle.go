package proximity

import (
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// CommonUncleSiblings finds siblings that connect to the same node at their
// parent's level. Placement uses the largest such set as a proximity hint.
func CommonUncleSiblings(graph *layoutgraph.Graph) map[*layoutgraph.Node]layoutgraph.Nodes {
	uncleToCousins := make(map[*layoutgraph.Node]layoutgraph.Nodes)
	common := make(map[*layoutgraph.Node]layoutgraph.Nodes)
	var orderedUncles []*layoutgraph.Node
	for _, container := range graph.ContainerRDFSOrderUnbounded(nil) {
		children := graph.Containers[container]
		for _, node := range children {
			for _, edge := range node.Edges {
				adjacent := node.Adjacent(edge)
				if adjacent.Container == container.Container {
					if _, ok := uncleToCousins[adjacent]; !ok {
						orderedUncles = append(orderedUncles, adjacent)
						uncleToCousins[adjacent] = []*layoutgraph.Node{}
					}
					if !slices.Contains(uncleToCousins[adjacent], node) {
						uncleToCousins[adjacent] = append(uncleToCousins[adjacent], node)
					}
				}
			}
		}
		for _, uncle := range orderedUncles {
			siblings := uncleToCousins[uncle]
			if len(siblings) < 2 {
				continue
			}
			for _, sibling := range siblings {
				if existing, ok := common[sibling]; !ok || len(existing) < len(siblings) {
					common[sibling] = siblings
				}
			}
		}
		uncleToCousins = make(map[*layoutgraph.Node]layoutgraph.Nodes)
	}
	return common
}
