package placement

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

// clusterExternalConnectedNodes returns the positioned endpoints outside the
// cluster that remain in the cluster nodes' current placement graph. The
// edge-abduction order is significant: callers use the stable first-seen order
// when comparing opposite-side placements.
func clusterExternalConnectedNodes(cluster *layoutgraph.Cluster) []*layoutgraph.Node {
	currentGraph := cluster.Nodes[0].Graph
	set := make(map[*layoutgraph.Node]struct{})
	externalNodes := make([]*layoutgraph.Node, 0)
	for _, edgeAbduction := range cluster.EdgeAbductions {
		switch {
		case edgeAbduction.OriginallyFrom == nil && edgeAbduction.OriginallyTo != nil:
			if edgeAbduction.CurrentFrom.TopLeft != nil && edgeAbduction.CurrentFrom.Graph == currentGraph {
				if _, ok := set[edgeAbduction.CurrentFrom]; !ok {
					externalNodes = append(externalNodes, edgeAbduction.CurrentFrom)
					set[edgeAbduction.CurrentFrom] = struct{}{}
				}
			}
		case edgeAbduction.OriginallyTo == nil && edgeAbduction.OriginallyFrom != nil:
			if edgeAbduction.CurrentTo.TopLeft != nil && edgeAbduction.CurrentTo.Graph == currentGraph {
				if _, ok := set[edgeAbduction.CurrentTo]; !ok {
					externalNodes = append(externalNodes, edgeAbduction.CurrentTo)
					set[edgeAbduction.CurrentTo] = struct{}{}
				}
			}
		}
	}
	return externalNodes
}
