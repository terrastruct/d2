package grouping

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

// ResetClusters removes inactive vessel metadata so a later layout rediscovers
// clusters from ordinary nodes and edges.
func ResetClusters(graph *layoutgraph.Graph) {
	if len(graph.Clusters) == 0 {
		return
	}
	bulkFilter := len(graph.Clusters) > 1
	var singleRetiredVessel *layoutgraph.Node
	retiredVesselCount := 0
	for vessel, cluster := range graph.Clusters {
		if cluster == nil {
			continue
		}
		for _, abduction := range cluster.EdgeAbductions {
			if abduction == nil || abduction.Edge == nil {
				continue
			}
			if abduction.OriginallyFrom != nil {
				abduction.Edge.Reconnect(abduction.OriginallyFrom, false)
			}
			if abduction.OriginallyTo != nil {
				abduction.Edge.Reconnect(abduction.OriginallyTo, true)
			}
		}
		for _, node := range cluster.Nodes {
			if node != nil && node.Cluster == cluster {
				node.Cluster = nil
			}
			if node != nil && node.Nears != nil {
				delete(node.Nears, vessel)
			}
		}
		if vessel == nil {
			continue
		}
		for near := range vessel.Nears {
			if near != nil && near.Nears != nil {
				delete(near.Nears, vessel)
			}
		}
		vessel.Nears = map[*layoutgraph.Node]struct{}{}
		if bulkFilter {
			retiredVesselCount++
			if retiredVesselCount == 1 {
				singleRetiredVessel = vessel
			}
		} else {
			filteredNodes := graph.Nodes[:0]
			for _, node := range graph.Nodes {
				if node != vessel {
					filteredNodes = append(filteredNodes, node)
				}
			}
			graph.Nodes = filteredNodes
			for container, children := range graph.Containers {
				filtered := children[:0]
				for _, child := range children {
					if child != vessel {
						filtered = append(filtered, child)
					}
				}
				graph.Containers[container] = filtered
			}
		}
		vessel.Container = nil
		vessel.Graph = nil
		vessel.UnmarkClusterVessel()
	}
	if !bulkFilter {
		clear(graph.Clusters)
		return
	}
	if retiredVesselCount == 0 {
		clear(graph.Clusters)
		return
	}
	if retiredVesselCount == 1 {
		filteredNodes := graph.Nodes[:0]
		for _, node := range graph.Nodes {
			if node != singleRetiredVessel {
				filteredNodes = append(filteredNodes, node)
			}
		}
		graph.Nodes = filteredNodes
		for container, children := range graph.Containers {
			filtered := children[:0]
			for _, child := range children {
				if child != singleRetiredVessel {
					filtered = append(filtered, child)
				}
			}
			graph.Containers[container] = filtered
		}
		clear(graph.Clusters)
		return
	}
	// The loop above detaches every removable vessel. Check that cheap marker
	// before consulting Clusters so ordinary survivors avoid a map lookup.
	filteredNodes := graph.Nodes[:0]
	for _, node := range graph.Nodes {
		remove := node != nil && node.Graph == nil && graph.Clusters[node] != nil
		if !remove {
			filteredNodes = append(filteredNodes, node)
		}
	}
	graph.Nodes = filteredNodes
	for container, children := range graph.Containers {
		filtered := children[:0]
		for _, child := range children {
			remove := child != nil && child.Graph == nil && graph.Clusters[child] != nil
			if !remove {
				filtered = append(filtered, child)
			}
		}
		graph.Containers[container] = filtered
	}
	clear(graph.Clusters)
}

// Cleanup restores cluster and sequence members after node placement and
// retires the temporary vessels while retaining rediscovery metadata.
func Cleanup(graph *layoutgraph.Graph) {
	for _, key := range graph.ClusterOrder() {
		cluster := graph.Clusters[key]
		cluster.ArrangeClusterNodes()
		for _, node := range cluster.Nodes {
			graph.AddNewNodeToContainer(cluster.Container, node)
		}
		for _, abduction := range cluster.EdgeAbductions {
			if abduction.OriginallyFrom != nil {
				abduction.Edge.Reconnect(abduction.OriginallyFrom, false)
			}
			if abduction.OriginallyTo != nil {
				abduction.Edge.Reconnect(abduction.OriginallyTo, true)
			}
		}
		if len(cluster.Vessel.Nears) != 0 {
			for _, near := range cluster.Vessel.OrderedNears() {
				delete(near.Nears, cluster.Vessel)
				for _, node := range cluster.Nodes {
					near.AddNear(node)
				}
			}
			cluster.Vessel.Nears = map[*layoutgraph.Node]struct{}{}
		}
		graph.RemoveNode(cluster.Vessel)
		updated := make([]*layoutgraph.Node, 0)
		for _, child := range graph.Containers[cluster.Container] {
			if child != cluster.Vessel {
				updated = append(updated, child)
			}
		}
		graph.Containers[cluster.Container] = updated
		cluster.Vessel.Container = nil
		cluster.Vessel.Graph = nil
	}

	for _, vessel := range graph.SequenceOrder() {
		sequence := graph.Sequences[vessel]
		sequence.ArrangeSteps()
		for _, node := range sequence.Nodes {
			graph.AddNewNodeToContainer(sequence.Container, node)
		}
		for _, abduction := range sequence.EdgeAbductions {
			if abduction.OriginallyFrom != nil {
				abduction.Edge.Reconnect(abduction.OriginallyFrom, false)
			}
			if abduction.OriginallyTo != nil {
				abduction.Edge.Reconnect(abduction.OriginallyTo, true)
			}
		}
		if len(sequence.Vessel.Nears) != 0 {
			for _, near := range sequence.Vessel.OrderedNears() {
				delete(near.Nears, sequence.Vessel)
				for _, node := range sequence.Nodes {
					near.AddNear(node)
				}
			}
			sequence.Vessel.Nears = map[*layoutgraph.Node]struct{}{}
		}
		graph.RemoveNode(vessel)
		var updated []*layoutgraph.Node
		for _, child := range graph.Containers[sequence.Container] {
			if child != vessel {
				updated = append(updated, child)
			}
		}
		graph.Containers[sequence.Container] = updated
		sequence.Vessel.Container = nil
		sequence.Vessel.Graph = nil
	}
	for _, node := range graph.Nodes {
		node.HerdAssignment = nil
	}
}
