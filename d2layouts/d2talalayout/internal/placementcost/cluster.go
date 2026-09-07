package placementcost

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

func clusterExactlyTwoExternalConnectedNodes(cluster *layoutgraph.Cluster) (first, second *layoutgraph.Node, exactlyTwo bool) {
	currentGraph := cluster.Nodes[0].Graph
	moreThanTwo := false
	for _, edgeAbduction := range cluster.EdgeAbductions {
		var externalNode *layoutgraph.Node
		switch {
		case edgeAbduction.OriginallyFrom == nil && edgeAbduction.OriginallyTo != nil:
			// edge coming into cluster
			if edgeAbduction.CurrentFrom.TopLeft != nil && edgeAbduction.CurrentFrom.Graph == currentGraph {
				externalNode = edgeAbduction.CurrentFrom
			}
		case edgeAbduction.OriginallyTo == nil && edgeAbduction.OriginallyFrom != nil:
			// edge exiting from cluster
			if edgeAbduction.CurrentTo.TopLeft != nil && edgeAbduction.CurrentTo.Graph == currentGraph {
				externalNode = edgeAbduction.CurrentTo
			}
		}
		if externalNode == nil || externalNode == first || externalNode == second {
			continue
		}
		if first == nil {
			first = externalNode
			continue
		}
		if second == nil {
			second = externalNode
			continue
		}
		moreThanTwo = true
	}
	return first, second, !moreThanTwo && first != nil && second != nil
}
