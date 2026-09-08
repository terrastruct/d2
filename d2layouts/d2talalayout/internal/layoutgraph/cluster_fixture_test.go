package layoutgraph

import (
	"math"
	"sort"

	"github.com/d2lang/d2/lib/geo"
)

// createClusterVesselFixture keeps transaction and movement invariant tests in
// layoutgraph without introducing an import cycle back through grouping.
func createClusterVesselFixture(cluster *Cluster, vesselID EntityID) *Node {
	minimumX, minimumY := math.Inf(1), math.Inf(1)
	for _, node := range cluster.Nodes {
		if node.TopLeft != nil {
			minimumX = math.Min(minimumX, node.TopLeft.X)
			minimumY = math.Min(minimumY, node.TopLeft.Y)
		}
	}
	vessel := NewNode(vesselID, 0, 0)
	vessel.SetClusterVessel(true)
	cluster.Resize(vessel)
	if !math.IsInf(minimumX, 1) && !math.IsInf(minimumY, 1) {
		switch cluster.Arrangement {
		case Row:
			sort.Slice(cluster.Nodes, func(i, j int) bool {
				return cluster.Nodes[i].TopLeft.X < cluster.Nodes[j].TopLeft.X
			})
		case Column:
			sort.Slice(cluster.Nodes, func(i, j int) bool {
				return cluster.Nodes[i].TopLeft.Y < cluster.Nodes[j].TopLeft.Y
			})
		}
		vessel.TopLeft = geo.NewPoint(minimumX, minimumY)
	}
	return vessel
}

func addClusterFixture(graph *Graph, cluster *Cluster) {
	graph.AddNewNodeToContainer(cluster.Container, cluster.Vessel)
	for _, node := range cluster.Nodes {
		node.Cluster = cluster
	}
	updatedChildren := make([]*Node, 0, len(graph.Containers[cluster.Container]))
	for _, child := range graph.Containers[cluster.Container] {
		if child.Cluster != cluster {
			updatedChildren = append(updatedChildren, child)
		}
	}
	graph.Containers[cluster.Container] = updatedChildren
	for _, node := range cluster.Nodes {
		graph.RemoveNode(node)
		node.Container = nil
	}
	graph.Clusters[cluster.Vessel] = cluster
}
