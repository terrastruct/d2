package grouping

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

// AssignArrangement selects the cluster axis from node dimensions. Randomness
// is consumed only for square clusters, preserving the engine's seed contract.
func AssignArrangement(cluster *layoutgraph.Cluster, isConnectedToSequence bool, random *rand.Rand) layoutgraph.ClusterArrangement {
	averageWidth, averageHeight := averageClusterDimensions(cluster)
	if isConnectedToSequence {
		return layoutgraph.Row
	}
	if averageWidth > averageHeight {
		return layoutgraph.Column
	}
	if averageWidth < averageHeight {
		return layoutgraph.Row
	}
	if random.Float64() > 0.5 {
		return layoutgraph.Column
	}
	return layoutgraph.Row
}

func averageClusterDimensions(cluster *layoutgraph.Cluster) (width, height float64) {
	for _, node := range cluster.Nodes {
		width += node.Width
		height += node.Height
	}
	count := float64(len(cluster.Nodes))
	return math.Round(width / count), math.Round(height / count)
}

// PaddingBetween calculates the spacing between adjacent cluster members.
func PaddingBetween(cluster *layoutgraph.Cluster, considerPositions bool) float64 {
	averageWidth, averageHeight := averageClusterDimensions(cluster)
	hasIcon := false
	var maxLabelWidth, maxLabelHeight float64
	for _, node := range cluster.Nodes {
		hasIcon = hasIcon || node.Icon != nil
		if node.Label != nil {
			maxLabelWidth = math.Max(maxLabelWidth, node.Label.Width)
			maxLabelHeight = math.Max(maxLabelHeight, node.Label.Height)
		}
	}
	var given float64
	if cluster.Arrangement == layoutgraph.Row {
		given = math.Max(layoutgraph.NodeGap, math.Ceil(averageWidth)*0.1)
		if hasIcon {
			given = math.Max(given, 2*label.PADDING+maxLabelWidth)
		}
	} else {
		given = math.Max(layoutgraph.NodeGap, math.Ceil(averageHeight)*0.1)
		if hasIcon {
			given = math.Max(given, 2*label.PADDING+maxLabelHeight)
		}
	}
	if considerPositions {
		var total float64
		for index := 0; index < len(cluster.Nodes)-1; index++ {
			total += cluster.Nodes[index].DistanceTo(cluster.Nodes[index+1], true)
		}
		positionedPadding := total / float64(len(cluster.Nodes)-1)
		if positionedPadding > 0 {
			return math.Round(math.Min(positionedPadding, given))
		}
	}
	return math.Round(given)
}

// CreateVessel builds the temporary placement node representing a cluster.
func CreateVessel(cluster *layoutgraph.Cluster, vesselID layoutgraph.EntityID) *layoutgraph.Node {
	minimumX, minimumY := math.Inf(1), math.Inf(1)
	for _, node := range cluster.Nodes {
		if node.TopLeft != nil {
			minimumX = math.Min(minimumX, node.TopLeft.X)
			minimumY = math.Min(minimumY, node.TopLeft.Y)
		}
	}
	vessel := layoutgraph.NewNode(vesselID, 0, 0)
	vessel.SetClusterVessel(true)
	cluster.Resize(vessel)
	if cluster.Arrangement == layoutgraph.Row {
		if !math.IsInf(minimumX, 1) && !math.IsInf(minimumY, 1) {
			sort.Slice(cluster.Nodes, func(i, j int) bool { return cluster.Nodes[i].TopLeft.X < cluster.Nodes[j].TopLeft.X })
			vessel.TopLeft = geo.NewPoint(minimumX, minimumY)
		}
	}
	if cluster.Arrangement == layoutgraph.Column {
		if !math.IsInf(minimumX, 1) && !math.IsInf(minimumY, 1) {
			sort.Slice(cluster.Nodes, func(i, j int) bool { return cluster.Nodes[i].TopLeft.Y < cluster.Nodes[j].TopLeft.Y })
			vessel.TopLeft = geo.NewPoint(minimumX, minimumY)
		}
	}
	return vessel
}

// AddCluster installs a discovered cluster and its temporary vessel into the
// mutable layout graph.
func AddCluster(graph *layoutgraph.Graph, cluster *layoutgraph.Cluster) {
	graph.AddNewNodeToContainer(cluster.Container, cluster.Vessel)
	for _, node := range cluster.Nodes {
		node.Cluster = cluster
	}
	var updatedContainerNodes []*layoutgraph.Node
	for _, child := range graph.Containers[cluster.Container] {
		if child.Cluster != cluster {
			updatedContainerNodes = append(updatedContainerNodes, child)
		}
	}
	graph.Containers[cluster.Container] = updatedContainerNodes
	for _, node := range cluster.Nodes {
		graph.RemoveNode(node)
		node.Container = nil
	}
	graph.Clusters[cluster.Vessel] = cluster
}

func abductClusterEdges(cluster *layoutgraph.Cluster, edges []*layoutgraph.Edge, guard *limits.WorkGuard) error {
	var abductions []*layoutgraph.EdgeAbduction
	for _, edge := range edges {
		if err := guard.Step(); err != nil {
			return err
		}
		if edge.From.Cluster == cluster {
			for range len(edge.From.Edges) + len(cluster.Vessel.Edges) {
				if err := guard.Step(); err != nil {
					return err
				}
			}
			abductions = append(abductions, &layoutgraph.EdgeAbduction{
				Edge: edge, OriginallyFrom: edge.From,
				CurrentFrom: cluster.Vessel, CurrentTo: edge.To,
			})
			edge.Reconnect(cluster.Vessel, false)
		}
		if edge.To.Cluster == cluster {
			for range len(edge.To.Edges) + len(cluster.Vessel.Edges) {
				if err := guard.Step(); err != nil {
					return err
				}
			}
			abductions = append(abductions, &layoutgraph.EdgeAbduction{
				Edge: edge, OriginallyTo: edge.To,
				CurrentTo: cluster.Vessel, CurrentFrom: edge.From,
			})
			edge.Reconnect(cluster.Vessel, true)
		}
	}
	cluster.EdgeAbductions = abductions
	return guard.Finish()
}

// AddClusters discovers interchangeable sibling nodes and replaces accepted
// groups with temporary vessels for node placement.
func AddClusters(ctx context.Context, graph *layoutgraph.Graph, randomSeed int64, random *rand.Rand) (err error) {
	if graph == nil {
		return fmt.Errorf("TALA AddClusters requires a graph")
	}
	if random == nil {
		return fmt.Errorf("TALA AddClusters requires a random generator")
	}
	if err := layoutgraph.Validate(ctx, "AddClusters", graph); err != nil {
		return err
	}
	_, guard, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "AddClustersTransactions")
	if err != nil {
		return err
	}
	containerOrder, err := graph.ContainerRDFSOrder(nil, guard)
	if err != nil {
		return err
	}

	stageState := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
		CaptureTopology:   true,
		CaptureEdgeRoutes: true,
	})
	if err := stageState.UpdateWithWorkGuard(graph, guard); err != nil {
		return err
	}
	complete := false
	defer func() {
		if recovered := recover(); recovered != nil {
			layoutgraph.RestoreGraphState(graph, stageState)
			panic(recovered)
		}
		if !complete {
			layoutgraph.RestoreGraphState(graph, stageState)
		}
	}()

	if graph.Clusters == nil {
		graph.Clusters = map[*layoutgraph.Node]*layoutgraph.Cluster{}
	} else {
		clear(graph.Clusters)
	}
	discoveryIndex, err := buildClusterDiscoveryIndex(graph, containerOrder, guard)
	if err != nil {
		return err
	}
	infos := discoveryIndex.infos
	edgeOrder := discoveryIndex.edgeOrder
	reservedIDs := make(map[layoutgraph.EntityID]struct{}, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		reservedIDs[node.ID] = struct{}{}
	}
	for _, vessel := range graph.SequenceOrder() {
		if err := guard.Step(); err != nil {
			return err
		}
		reservedIDs[vessel.ID] = struct{}{}
		for _, node := range graph.Sequences[vessel].Nodes {
			if err := guard.Step(); err != nil {
				return err
			}
			reservedIDs[node.ID] = struct{}{}
		}
	}
	seenTrees := make(map[*layoutgraph.Tree]struct{})
	for _, sentinel := range graph.TreeOrder() {
		if err := guard.Step(); err != nil {
			return err
		}
		if sentinel != nil {
			reservedIDs[sentinel.ID] = struct{}{}
		}
		stack := slices.Clone(graph.Trees[sentinel])
		for len(stack) > 0 {
			if err := guard.Step(); err != nil {
				return err
			}
			last := len(stack) - 1
			tree := stack[last]
			stack = stack[:last]
			if tree == nil {
				continue
			}
			if _, seen := seenTrees[tree]; seen {
				continue
			}
			seenTrees[tree] = struct{}{}
			if tree.Node != nil {
				reservedIDs[tree.Node.ID] = struct{}{}
			}
			stack = append(stack, tree.Children...)
		}
	}
	nextVesselID := func(candidate layoutgraph.EntityID) (layoutgraph.EntityID, error) {
		for {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if _, found := reservedIDs[candidate]; !found {
				return candidate, nil
			}
			if candidate == math.MaxInt64 {
				candidate = 0
			} else {
				candidate++
			}
		}
	}
	chargeClusterKernel := func(cluster *layoutgraph.Cluster) error {
		for range cluster.Nodes {
			for range cluster.Nodes {
				if err := guard.Step(); err != nil {
					return err
				}
			}
		}
		for width := 1; width < len(cluster.Nodes); width *= 2 {
			for range cluster.Nodes {
				if err := guard.Step(); err != nil {
					return err
				}
			}
			if width > len(cluster.Nodes)/2 {
				break
			}
		}
		return guard.Finish()
	}

	maybeCreateCluster := func(cluster *layoutgraph.Cluster, arrangement layoutgraph.ClusterArrangement) (bool, error) {
		for _, node := range cluster.Nodes {
			if err := guard.Step(); err != nil {
				return false, err
			}
			for _, edge := range node.Edges {
				if err := guard.Step(); err != nil {
					return false, err
				}
				isDescendant, err := clusterIsDescendantOfGuarded(node, node.Adjacent(edge), guard)
				if err != nil {
					return false, err
				}
				if isDescendant {
					return false, nil
				}
			}
		}
		vesselID, err := nextVesselID(random.Int63())
		if err != nil {
			return false, err
		}
		clusterEdges, err := clusterIncidentEdges(cluster, infos, edgeOrder, guard)
		if err != nil {
			return false, err
		}
		if err := chargeClusterKernel(cluster); err != nil {
			return false, err
		}
		cluster.Arrangement = arrangement
		cluster.DesiredArrangement = arrangement
		cluster.Padding = PaddingBetween(cluster, false)
		vessel := CreateVessel(cluster, vesselID)
		cluster.Vessel = vessel
		AddCluster(graph, cluster)
		if err := guard.Step(); err != nil {
			return false, err
		}
		if err := abductClusterEdges(cluster, clusterEdges, guard); err != nil {
			return false, err
		}
		if err := discoveryIndex.refreshAfterClusterAbduction(graph, cluster, clusterEdges, guard); err != nil {
			return false, err
		}
		reservedIDs[vesselID] = struct{}{}
		for _, node := range cluster.Nodes {
			if err := guard.Step(); err != nil {
				return false, err
			}
			reservedIDs[node.ID] = struct{}{}
		}
		return true, nil
	}

	const maxSizeDiff = 4.0
	refreshContainerEstimate := func(container *layoutgraph.Node) error {
		if container == nil || !container.IsContainer() {
			return nil
		}
		info := infos[container]
		if info == nil {
			return fmt.Errorf("TALA AddClusters cannot find container discovery index")
		}
		padding := graph.ContainerPadding(container, true)
		var childrenWidth, childrenHeight float64
		for _, child := range graph.Containers[container] {
			if err := guard.Step(); err != nil {
				return err
			}
			childWidth, childHeight := child.Width, child.Height
			if childInfo := infos[child]; childInfo != nil {
				childWidth = childInfo.estimatedWidth
				childHeight = childInfo.estimatedHeight
			}
			childrenWidth += childWidth
			childrenHeight += childHeight
		}
		info.estimatedWidth = max(container.Width, childrenWidth+padding.Left()+padding.Right())
		info.estimatedHeight = max(container.Height, childrenHeight+padding.Top()+padding.Bottom())
		return guard.Finish()
	}
	containers := append(slices.Clone(containerOrder), nil)
	for _, container := range containers {
		if err := guard.Step(); err != nil {
			return err
		}
		rng := rand.New(rand.NewSource(randomSeed))
		children := graph.Containers[container]
		for _, node := range children {
			if err := guard.Step(); err != nil {
				return err
			}
			info := infos[node]
			if info == nil {
				return fmt.Errorf("TALA AddClusters cannot find node discovery index")
			}
			if info.noClustering || node.Cluster != nil {
				continue
			}
			connectedToCluster := false
			for _, edge := range node.Edges {
				if err := guard.Step(); err != nil {
					return err
				}
				if adjacent := node.Adjacent(edge); adjacent != nil && adjacent.IsClusterVessel() {
					connectedToCluster = true
					break
				}
			}
			if connectedToCluster || info.toTableColumn {
				continue
			}
			clusterNodes := []*layoutgraph.Node{node}
			isConnectedToSequence := false
			for _, otherNode := range children {
				if err := guard.Step(); err != nil {
					return err
				}
				if otherNode == node || !otherNode.SameShape(node) {
					continue
				}
				otherInfo := infos[otherNode]
				if otherInfo == nil {
					return fmt.Errorf("TALA AddClusters cannot find comparison-node discovery index")
				}
				if otherInfo.noClustering || otherNode.Cluster != nil || otherInfo.toTableColumn {
					continue
				}
				if maxSizeDiff*otherInfo.estimatedWidth < info.estimatedWidth ||
					maxSizeDiff*info.estimatedWidth < otherInfo.estimatedWidth ||
					maxSizeDiff*otherInfo.estimatedHeight < info.estimatedHeight ||
					maxSizeDiff*info.estimatedHeight < otherInfo.estimatedHeight {
					continue
				}
				sameAdjacentNodes := len(info.neighbors) > 0 && len(info.neighbors) == len(otherInfo.neighbors)
				if sameAdjacentNodes {
					for _, adjacentNode := range info.neighbors {
						if err := guard.Step(); err != nil {
							return err
						}
						isConnectedToSequence = isConnectedToSequence || adjacentNode != nil && adjacentNode.Sequence != nil
						if _, found := otherInfo.neighborSet[adjacentNode]; !found {
							sameAdjacentNodes = false
							break
						}
					}
				}
				if sameAdjacentNodes && info.edgeSignature.matches(otherInfo.edgeSignature) {
					clusterNodes = append(clusterNodes, otherNode)
				}
			}
			if len(clusterNodes) <= 1 {
				continue
			}
			cluster := &layoutgraph.Cluster{Nodes: clusterNodes, Container: container, Graph: graph}
			for _, clusterNode := range clusterNodes {
				if err := guard.Step(); err != nil {
					return err
				}
				if clusterNode.AspectRatio1() {
					cluster.FixedSize = true
					break
				}
			}
			if err := chargeClusterKernel(cluster); err != nil {
				return err
			}
			initialArrangement := AssignArrangement(cluster, isConnectedToSequence, rng)
			success, createErr := maybeCreateCluster(cluster, initialArrangement)
			if createErr != nil {
				return createErr
			}
			if !success {
				if _, err := maybeCreateCluster(cluster, initialArrangement.Flip()); err != nil {
					return err
				}
			}
		}
		if err := refreshContainerEstimate(container); err != nil {
			return err
		}
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	complete = true
	return nil
}
