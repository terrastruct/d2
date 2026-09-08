package layoutgraph

import (
	"context"
	"fmt"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/typedpool"
)

const (
	maxEngineTopologyReferences  int64 = 1_000_000
	maxEngineRoutePoints         int64 = limits.MaxEngineRoutePoints
	maxEngineTopologyDepth             = limits.MaxEngineTreeDepth
	maxEnginePreflightWork       int64 = maxEngineTopologyReferences * 8
	maxPooledParentRelationNodes       = 4096
)

type parentRelationScratch struct {
	state  map[*Node]uint8
	depths map[*Node]int
	path   []*Node
}

var parentRelationScratchPool = typedpool.New(func() *parentRelationScratch {
	return &parentRelationScratch{
		state:  make(map[*Node]uint8, 128),
		depths: make(map[*Node]int, 128),
		path:   make([]*Node, 0, 16),
	}
})

func borrowParentRelationScratch(nodeCount int) (*parentRelationScratch, bool) {
	if nodeCount > maxPooledParentRelationNodes {
		return &parentRelationScratch{
			state:  make(map[*Node]uint8, nodeCount),
			depths: make(map[*Node]int, nodeCount),
			path:   make([]*Node, 0, min(nodeCount, maxEngineTopologyDepth)),
		}, false
	}
	scratch := parentRelationScratchPool.Get()
	clear(scratch.state)
	clear(scratch.depths)
	clear(scratch.path[:cap(scratch.path)])
	scratch.path = scratch.path[:0]
	return scratch, true
}

func returnParentRelationScratch(scratch *parentRelationScratch, pooled bool) {
	if !pooled {
		return
	}
	clear(scratch.state)
	clear(scratch.depths)
	clear(scratch.path[:cap(scratch.path)])
	scratch.path = scratch.path[:0]
	parentRelationScratchPool.Put(scratch)
}

func validateNodeParentRelation(
	guard workStepper,
	nodes map[*Node]struct{},
	relation string,
	parentOf func(*Node) *Node,
) error {
	scratch, pooled := borrowParentRelationScratch(len(nodes))
	defer returnParentRelationScratch(scratch, pooled)
	state := scratch.state
	depths := scratch.depths
	for node := range nodes {
		if state[node] == 2 {
			continue
		}
		scratch.path = scratch.path[:0]
		current := node
		for current != nil && state[current] == 0 {
			if err := guard.Step(); err != nil {
				return err
			}
			state[current] = 1
			scratch.path = append(scratch.path, current)
			current = parentOf(current)
		}
		if current != nil && state[current] == 1 {
			return fmt.Errorf("TALA engine %s cycle detected at node %d", relation, current.ID)
		}
		depth := 0
		if current != nil {
			depth = depths[current]
		}
		for _, p := range slices.Backward(scratch.path) {
			depth++
			if depth > maxEngineTopologyDepth {
				return fmt.Errorf("TALA engine %s depth exceeds limit %d", relation, maxEngineTopologyDepth)
			}
			depths[p] = depth
			state[p] = 2
		}
	}
	return nil
}

// ancestryParent matches the precedence used by isDescendantOf. Keep this
// separate from container: active cluster and sequence ancestry follows the
// vessel itself, while container follows the vessel's direct container.
func ancestryParent(node *Node) *Node {
	if node == nil {
		return nil
	}
	if node.Container != nil {
		return node.Container
	}
	if node.Cluster != nil {
		return node.Cluster.Vessel
	}
	if node.Sequence != nil {
		return node.Sequence.Vessel
	}
	return nil
}

// validateCombineNodeTopology validates node-owned records across all graphs as
// one bounded inventory. Split subgraphs intentionally share the large
// graph-owned entity maps, but each subgraph has its own Nodes and Edges slices
// and may expose additional topology through those nodes. Walking the union
// makes that validation linear in the distinct runtime records rather than in
// the number of subgraphs.
func validateCombineNodeTopology(ctx context.Context, graphs []*Graph) error {
	guard, err := limits.NewWorkGuard(ctx, "CombineSubgraphs", maxEngineWorkUnits)
	if err != nil {
		return err
	}
	guard.SetLimit(maxEnginePreflightWork)

	var referenceCount int64
	var routePointCount int64
	addReference := func(kind string) error {
		if referenceCount >= maxEngineTopologyReferences {
			return fmt.Errorf(
				"TALA CombineSubgraphs topology references exceed limit %d while visiting %s",
				maxEngineTopologyReferences,
				kind,
			)
		}
		referenceCount++
		return guard.Step()
	}
	accountSpareCapacity := func(kind string, length, capacity int) error {
		if capacity < length {
			return fmt.Errorf("TALA CombineSubgraphs %s has invalid slice capacity", kind)
		}
		spare := int64(capacity - length)
		if spare > maxEngineTopologyReferences-referenceCount {
			return fmt.Errorf(
				"TALA CombineSubgraphs topology references exceed limit %d while reserving %s capacity",
				maxEngineTopologyReferences,
				kind,
			)
		}
		referenceCount += spare
		return guard.Check()
	}
	structuralNil := func(kind string) error {
		return fmt.Errorf("TALA CombineSubgraphs node-owned topology contains nil %s", kind)
	}

	nodes := make(map[*Node]struct{})
	edges := make(map[*Edge]struct{})
	clusters := make(map[*Cluster]struct{})
	sequences := make(map[*Sequence]struct{})
	abductions := make(map[*EdgeAbduction]struct{})
	herds := make(map[*HerdAssignment]struct{})
	hierarchies := make(map[*Hierarchy]struct{})

	var nodeQueue []*Node
	var edgeQueue []*Edge
	var clusterQueue []*Cluster
	var sequenceQueue []*Sequence
	var abductionQueue []*EdgeAbduction
	var herdQueue []*HerdAssignment
	var hierarchyQueue []*Hierarchy

	queueNode := func(node *Node, required bool, kind string) error {
		if node == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := nodes[node]; seen {
			return nil
		}
		if len(nodes) >= maxEngineNodes {
			return fmt.Errorf("TALA CombineSubgraphs unique node count exceeds limit %d", maxEngineNodes)
		}
		nodes[node] = struct{}{}
		nodeQueue = append(nodeQueue, node)
		return nil
	}
	queueEdge := func(edge *Edge, required bool, kind string) error {
		if edge == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := edges[edge]; seen {
			return nil
		}
		if len(edges) >= maxEngineEdges {
			return fmt.Errorf("TALA CombineSubgraphs unique edge count exceeds limit %d", maxEngineEdges)
		}
		edges[edge] = struct{}{}
		edgeQueue = append(edgeQueue, edge)
		return nil
	}
	queueCluster := func(cluster *Cluster, required bool, kind string) error {
		if cluster == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := clusters[cluster]; !seen {
			clusters[cluster] = struct{}{}
			clusterQueue = append(clusterQueue, cluster)
		}
		return nil
	}
	queueSequence := func(sequence *Sequence, required bool, kind string) error {
		if sequence == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := sequences[sequence]; !seen {
			sequences[sequence] = struct{}{}
			sequenceQueue = append(sequenceQueue, sequence)
		}
		return nil
	}
	queueAbduction := func(abduction *EdgeAbduction, required bool, kind string) error {
		if abduction == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := abductions[abduction]; !seen {
			abductions[abduction] = struct{}{}
			abductionQueue = append(abductionQueue, abduction)
		}
		return nil
	}
	queueHerd := func(herd *HerdAssignment, required bool, kind string) error {
		if herd == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := herds[herd]; !seen {
			herds[herd] = struct{}{}
			herdQueue = append(herdQueue, herd)
		}
		return nil
	}
	queueHierarchy := func(hierarchy *Hierarchy, required bool, kind string) error {
		if hierarchy == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := hierarchies[hierarchy]; !seen {
			hierarchies[hierarchy] = struct{}{}
			hierarchyQueue = append(hierarchyQueue, hierarchy)
		}
		return nil
	}

	for _, graph := range graphs {
		if graph == nil {
			return fmt.Errorf("TALA CombineSubgraphs received a nil graph")
		}
		for i, node := range graph.Nodes {
			if node == nil {
				return fmt.Errorf("TALA CombineSubgraphs graph node at index %d is nil", i)
			}
			if err := queueNode(node, true, "graph node"); err != nil {
				return err
			}
		}
		for i, edge := range graph.Edges {
			if edge == nil {
				return fmt.Errorf("TALA CombineSubgraphs graph edge at index %d is nil", i)
			}
			if err := queueEdge(edge, true, "graph edge"); err != nil {
				return err
			}
		}
	}

	var nodeIndex, edgeIndex, clusterIndex, sequenceIndex int
	var abductionIndex, herdIndex, hierarchyIndex int
	for nodeIndex < len(nodeQueue) || edgeIndex < len(edgeQueue) || clusterIndex < len(clusterQueue) ||
		sequenceIndex < len(sequenceQueue) || abductionIndex < len(abductionQueue) ||
		herdIndex < len(herdQueue) || hierarchyIndex < len(hierarchyQueue) {
		switch {
		case nodeIndex < len(nodeQueue):
			node := nodeQueue[nodeIndex]
			nodeIndex++
			if err := queueNode(node.Container, false, "node container"); err != nil {
				return err
			}
			if err := accountSpareCapacity("node edges", len(node.Edges), cap(node.Edges)); err != nil {
				return err
			}
			for _, edge := range node.Edges {
				if err := queueEdge(edge, true, "node edge"); err != nil {
					return err
				}
			}
			for near := range node.Nears {
				if err := queueNode(near, true, "near node"); err != nil {
					return err
				}
			}
			for neighbor := range node.LongDistanceNeighborRequirements {
				if err := queueNode(neighbor, true, "long-distance neighbor requirement key"); err != nil {
					return err
				}
			}
			if err := queueCluster(node.Cluster, false, "node cluster"); err != nil {
				return err
			}
			if err := queueSequence(node.Sequence, false, "node sequence"); err != nil {
				return err
			}
			if err := queueHerd(node.HerdAssignment, false, "node herd assignment"); err != nil {
				return err
			}
			if err := queueHierarchy(node.Hierarchy, false, "node hierarchy"); err != nil {
				return err
			}

		case edgeIndex < len(edgeQueue):
			edge := edgeQueue[edgeIndex]
			edgeIndex++
			if err := queueNode(edge.From, true, "edge source"); err != nil {
				return err
			}
			if err := queueNode(edge.To, true, "edge target"); err != nil {
				return err
			}
			pointCapacity := int64(cap(edge.Points))
			if pointCapacity > maxEngineRoutePoints-routePointCount {
				return fmt.Errorf("TALA CombineSubgraphs route point count exceeds limit %d", maxEngineRoutePoints)
			}
			routePointCount += pointCapacity
			for _, point := range edge.Points {
				if point == nil {
					return structuralNil("edge route point")
				}
				if err := guard.Step(); err != nil {
					return err
				}
			}

		case clusterIndex < len(clusterQueue):
			cluster := clusterQueue[clusterIndex]
			clusterIndex++
			if err := queueNode(cluster.Vessel, true, "cluster vessel"); err != nil {
				return err
			}
			if err := queueNode(cluster.Container, false, "cluster container"); err != nil {
				return err
			}
			if err := accountSpareCapacity("cluster nodes", len(cluster.Nodes), cap(cluster.Nodes)); err != nil {
				return err
			}
			for _, node := range cluster.Nodes {
				if err := queueNode(node, true, "cluster node"); err != nil {
					return err
				}
			}
			if err := accountSpareCapacity("cluster edge abductions", len(cluster.EdgeAbductions), cap(cluster.EdgeAbductions)); err != nil {
				return err
			}
			for _, abduction := range cluster.EdgeAbductions {
				if err := queueAbduction(abduction, true, "cluster edge abduction"); err != nil {
					return err
				}
			}

		case sequenceIndex < len(sequenceQueue):
			sequence := sequenceQueue[sequenceIndex]
			sequenceIndex++
			if err := queueNode(sequence.Vessel, true, "sequence vessel"); err != nil {
				return err
			}
			if err := queueNode(sequence.Container, false, "sequence container"); err != nil {
				return err
			}
			if err := accountSpareCapacity("sequence nodes", len(sequence.Nodes), cap(sequence.Nodes)); err != nil {
				return err
			}
			for _, node := range sequence.Nodes {
				if err := queueNode(node, true, "sequence node"); err != nil {
					return err
				}
			}
			if err := accountSpareCapacity("sequence edge abductions", len(sequence.EdgeAbductions), cap(sequence.EdgeAbductions)); err != nil {
				return err
			}
			for _, abduction := range sequence.EdgeAbductions {
				if err := queueAbduction(abduction, true, "sequence edge abduction"); err != nil {
					return err
				}
			}

		case abductionIndex < len(abductionQueue):
			abduction := abductionQueue[abductionIndex]
			abductionIndex++
			if err := queueEdge(abduction.Edge, true, "abducted edge"); err != nil {
				return err
			}
			for _, endpoint := range []struct {
				node *Node
				kind string
			}{
				{abduction.OriginallyFrom, "abduction original source"},
				{abduction.OriginallyTo, "abduction original target"},
				{abduction.CurrentFrom, "abduction current source"},
				{abduction.CurrentTo, "abduction current target"},
			} {
				if err := queueNode(endpoint.node, false, endpoint.kind); err != nil {
					return err
				}
			}

		case herdIndex < len(herdQueue):
			herd := herdQueue[herdIndex]
			herdIndex++
			for node := range herd.oppositeSidePaired {
				if err := queueNode(node, true, "opposite-side herd node"); err != nil {
					return err
				}
			}
			for node := range herd.sameSidePaired {
				if err := queueNode(node, true, "same-side herd node"); err != nil {
					return err
				}
			}

		case hierarchyIndex < len(hierarchyQueue):
			hierarchy := hierarchyQueue[hierarchyIndex]
			hierarchyIndex++
			for node := range hierarchy.level {
				if err := queueNode(node, true, "hierarchy level node"); err != nil {
					return err
				}
			}
		}
	}

	if err := validateNodeParentRelation(guard, nodes, "container parent", func(node *Node) *Node {
		return node.Container
	}); err != nil {
		return err
	}
	if err := validateNodeParentRelation(guard, nodes, "effective container parent", func(node *Node) *Node {
		return node.container()
	}); err != nil {
		return err
	}
	if err := validateNodeParentRelation(guard, nodes, "ancestry parent", ancestryParent); err != nil {
		return err
	}
	return guard.Finish()
}

// validateEngineGraph performs a bounded, iterative walk of every runtime
// topology reference before layout code allocates snapshots or starts work.
// It intentionally validates structural safety rather than semantic layout
// policy, which belongs at the adapter boundary.
func validateEngineGraph(ctx context.Context, location string, graph *Graph) error {
	if graph == nil {
		return fmt.Errorf("TALA engine requires a graph")
	}
	guard, err := limits.NewWorkGuard(ctx, location, maxEngineWorkUnits)
	if err != nil {
		return err
	}
	guard.SetLimit(maxEnginePreflightWork)

	var referenceCount int64
	var routePointCount int64
	addReference := func(kind string) error {
		if referenceCount >= maxEngineTopologyReferences {
			return fmt.Errorf("TALA engine topology references exceed limit %d while visiting %s", maxEngineTopologyReferences, kind)
		}
		referenceCount++
		return guard.Step()
	}
	accountSpareCapacity := func(kind string, length, capacity int) error {
		if capacity < length {
			return fmt.Errorf("TALA engine %s has invalid slice capacity", kind)
		}
		spare := int64(capacity - length)
		if spare > maxEngineTopologyReferences-referenceCount {
			return fmt.Errorf("TALA engine topology references exceed limit %d while reserving %s capacity", maxEngineTopologyReferences, kind)
		}
		referenceCount += spare
		return guard.Check()
	}
	structuralNil := func(kind string) error {
		return fmt.Errorf("TALA engine topology contains nil %s", kind)
	}

	nodes := make(map[*Node]struct{})
	edges := make(map[*Edge]struct{})
	clusters := make(map[*Cluster]struct{})
	sequences := make(map[*Sequence]struct{})
	trees := make(map[*Tree]struct{})
	abductions := make(map[*EdgeAbduction]struct{})
	herds := make(map[*HerdAssignment]struct{})
	hierarchies := make(map[*Hierarchy]struct{})

	var nodeQueue []*Node
	var edgeQueue []*Edge
	var clusterQueue []*Cluster
	var sequenceQueue []*Sequence
	var treeQueue []*Tree
	var abductionQueue []*EdgeAbduction
	var herdQueue []*HerdAssignment
	var hierarchyQueue []*Hierarchy

	queueNode := func(node *Node, required bool, kind string) error {
		if node == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := nodes[node]; seen {
			return nil
		}
		if len(nodes) >= maxEngineNodes {
			return fmt.Errorf("TALA engine unique node count exceeds limit %d", maxEngineNodes)
		}
		nodes[node] = struct{}{}
		nodeQueue = append(nodeQueue, node)
		return nil
	}
	queueEdge := func(edge *Edge, required bool, kind string) error {
		if edge == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := edges[edge]; seen {
			return nil
		}
		if len(edges) >= maxEngineEdges {
			return fmt.Errorf("TALA engine unique edge count exceeds limit %d", maxEngineEdges)
		}
		edges[edge] = struct{}{}
		edgeQueue = append(edgeQueue, edge)
		return nil
	}
	queueCluster := func(cluster *Cluster, required bool, kind string) error {
		if cluster == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := clusters[cluster]; !seen {
			clusters[cluster] = struct{}{}
			clusterQueue = append(clusterQueue, cluster)
		}
		return nil
	}
	queueSequence := func(sequence *Sequence, required bool, kind string) error {
		if sequence == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := sequences[sequence]; !seen {
			sequences[sequence] = struct{}{}
			sequenceQueue = append(sequenceQueue, sequence)
		}
		return nil
	}
	queueTree := func(tree *Tree, required bool, kind string) error {
		if tree == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := trees[tree]; !seen {
			trees[tree] = struct{}{}
			treeQueue = append(treeQueue, tree)
		}
		return nil
	}
	queueAbduction := func(abduction *EdgeAbduction, required bool, kind string) error {
		if abduction == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := abductions[abduction]; !seen {
			abductions[abduction] = struct{}{}
			abductionQueue = append(abductionQueue, abduction)
		}
		return nil
	}
	queueHerd := func(herd *HerdAssignment, required bool, kind string) error {
		if herd == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := herds[herd]; !seen {
			herds[herd] = struct{}{}
			herdQueue = append(herdQueue, herd)
		}
		return nil
	}
	queueHierarchy := func(hierarchy *Hierarchy, required bool, kind string) error {
		if hierarchy == nil {
			if required {
				return structuralNil(kind)
			}
			return nil
		}
		if err := addReference(kind); err != nil {
			return err
		}
		if _, seen := hierarchies[hierarchy]; !seen {
			hierarchies[hierarchy] = struct{}{}
			hierarchyQueue = append(hierarchyQueue, hierarchy)
		}
		return nil
	}

	if err := accountSpareCapacity("graph nodes", len(graph.Nodes), cap(graph.Nodes)); err != nil {
		return err
	}
	for i, node := range graph.Nodes {
		if node == nil {
			return fmt.Errorf("graph node at index %d is nil", i)
		}
		if err := queueNode(node, true, "graph node"); err != nil {
			return err
		}
	}
	if err := accountSpareCapacity("graph edges", len(graph.Edges), cap(graph.Edges)); err != nil {
		return err
	}
	for i, edge := range graph.Edges {
		if edge == nil {
			return fmt.Errorf("graph edge at index %d is nil", i)
		}
		if edge.From == nil || edge.To == nil {
			return fmt.Errorf("graph edge %d has unplaced or missing endpoints", edge.entityID())
		}
		if err := queueEdge(edge, true, "graph edge"); err != nil {
			return err
		}
	}

	for container, children := range graph.Containers {
		if err := queueNode(container, false, "container key"); err != nil {
			return err
		}
		if err := accountSpareCapacity("container children", len(children), cap(children)); err != nil {
			return err
		}
		for _, child := range children {
			if err := queueNode(child, true, "container child"); err != nil {
				return err
			}
		}
	}
	for vessel, cluster := range graph.Clusters {
		if err := queueNode(vessel, true, "cluster vessel key"); err != nil {
			return err
		}
		if err := queueCluster(cluster, true, "cluster record"); err != nil {
			return err
		}
	}
	for vessel, sequence := range graph.Sequences {
		if err := queueNode(vessel, true, "sequence vessel key"); err != nil {
			return err
		}
		if err := queueSequence(sequence, true, "sequence record"); err != nil {
			return err
		}
	}
	for root, roots := range graph.Trees {
		// Root-sentinel trees are intentionally keyed by nil.
		if err := queueNode(root, false, "tree root key"); err != nil {
			return err
		}
		if err := accountSpareCapacity("tree roots", len(roots), cap(roots)); err != nil {
			return err
		}
		for _, tree := range roots {
			if err := queueTree(tree, true, "tree root"); err != nil {
				return err
			}
		}
	}
	for node, tree := range graph.NodeToTree {
		if err := queueNode(node, true, "node-to-tree key"); err != nil {
			return err
		}
		if err := queueTree(tree, true, "node-to-tree value"); err != nil {
			return err
		}
	}
	for hub, spokes := range graph.Hubs {
		if err := queueNode(hub, true, "hub key"); err != nil {
			return err
		}
		if err := accountSpareCapacity("hub spokes", len(spokes), cap(spokes)); err != nil {
			return err
		}
		for _, spoke := range spokes {
			if err := queueNode(spoke, true, "hub spoke"); err != nil {
				return err
			}
		}
	}
	for node := range graph.Directions {
		if err := queueNode(node, false, "direction key"); err != nil {
			return err
		}
	}
	for node, siblings := range graph.CommonUncleSiblings {
		if err := queueNode(node, true, "common-sibling key"); err != nil {
			return err
		}
		if err := accountSpareCapacity("common siblings", len(siblings), cap(siblings)); err != nil {
			return err
		}
		for _, sibling := range siblings {
			if err := queueNode(sibling, true, "common sibling"); err != nil {
				return err
			}
		}
	}

	var nodeIndex, edgeIndex, clusterIndex, sequenceIndex int
	var treeIndex, abductionIndex, herdIndex, hierarchyIndex int
	for nodeIndex < len(nodeQueue) || edgeIndex < len(edgeQueue) || clusterIndex < len(clusterQueue) ||
		sequenceIndex < len(sequenceQueue) || treeIndex < len(treeQueue) || abductionIndex < len(abductionQueue) ||
		herdIndex < len(herdQueue) || hierarchyIndex < len(hierarchyQueue) {
		switch {
		case nodeIndex < len(nodeQueue):
			node := nodeQueue[nodeIndex]
			nodeIndex++
			if err := queueNode(node.Container, false, "node container"); err != nil {
				return err
			}
			if err := accountSpareCapacity("node edges", len(node.Edges), cap(node.Edges)); err != nil {
				return err
			}
			for _, edge := range node.Edges {
				if err := queueEdge(edge, true, "node edge"); err != nil {
					return err
				}
			}
			for near := range node.Nears {
				if err := queueNode(near, true, "near node"); err != nil {
					return err
				}
			}
			for neighbor := range node.LongDistanceNeighborRequirements {
				if err := queueNode(neighbor, true, "long-distance neighbor requirement key"); err != nil {
					return err
				}
			}
			if err := queueCluster(node.Cluster, false, "node cluster"); err != nil {
				return err
			}
			if err := queueSequence(node.Sequence, false, "node sequence"); err != nil {
				return err
			}
			if err := queueHerd(node.HerdAssignment, false, "node herd assignment"); err != nil {
				return err
			}
			if err := queueHierarchy(node.Hierarchy, false, "node hierarchy"); err != nil {
				return err
			}

		case edgeIndex < len(edgeQueue):
			edge := edgeQueue[edgeIndex]
			edgeIndex++
			if err := queueNode(edge.From, true, "edge source"); err != nil {
				return err
			}
			if err := queueNode(edge.To, true, "edge target"); err != nil {
				return err
			}
			pointCount := int64(cap(edge.Points))
			if pointCount > maxEngineRoutePoints-routePointCount {
				return fmt.Errorf("TALA engine route point count exceeds limit %d", maxEngineRoutePoints)
			}
			routePointCount += pointCount
			for _, point := range edge.Points {
				if point == nil {
					return structuralNil("edge route point")
				}
				if err := guard.Step(); err != nil {
					return err
				}
			}

		case clusterIndex < len(clusterQueue):
			cluster := clusterQueue[clusterIndex]
			clusterIndex++
			if err := queueNode(cluster.Vessel, true, "cluster vessel"); err != nil {
				return err
			}
			if err := queueNode(cluster.Container, false, "cluster container"); err != nil {
				return err
			}
			if err := accountSpareCapacity("cluster nodes", len(cluster.Nodes), cap(cluster.Nodes)); err != nil {
				return err
			}
			for _, node := range cluster.Nodes {
				if err := queueNode(node, true, "cluster node"); err != nil {
					return err
				}
			}
			if err := accountSpareCapacity("cluster edge abductions", len(cluster.EdgeAbductions), cap(cluster.EdgeAbductions)); err != nil {
				return err
			}
			for _, abduction := range cluster.EdgeAbductions {
				if err := queueAbduction(abduction, true, "cluster edge abduction"); err != nil {
					return err
				}
			}

		case sequenceIndex < len(sequenceQueue):
			sequence := sequenceQueue[sequenceIndex]
			sequenceIndex++
			if err := queueNode(sequence.Vessel, true, "sequence vessel"); err != nil {
				return err
			}
			if err := queueNode(sequence.Container, false, "sequence container"); err != nil {
				return err
			}
			if err := accountSpareCapacity("sequence nodes", len(sequence.Nodes), cap(sequence.Nodes)); err != nil {
				return err
			}
			for _, node := range sequence.Nodes {
				if err := queueNode(node, true, "sequence node"); err != nil {
					return err
				}
			}
			if err := accountSpareCapacity("sequence edge abductions", len(sequence.EdgeAbductions), cap(sequence.EdgeAbductions)); err != nil {
				return err
			}
			for _, abduction := range sequence.EdgeAbductions {
				if err := queueAbduction(abduction, true, "sequence edge abduction"); err != nil {
					return err
				}
			}

		case treeIndex < len(treeQueue):
			tree := treeQueue[treeIndex]
			treeIndex++
			if err := queueNode(tree.Node, true, "tree node"); err != nil {
				return err
			}
			if err := queueTree(tree.Parent, false, "tree parent"); err != nil {
				return err
			}
			if err := queueEdge(tree.SentinelEdge, false, "tree sentinel edge"); err != nil {
				return err
			}
			if err := accountSpareCapacity("tree children", len(tree.Children), cap(tree.Children)); err != nil {
				return err
			}
			for _, child := range tree.Children {
				if err := queueTree(child, true, "tree child"); err != nil {
					return err
				}
			}

		case abductionIndex < len(abductionQueue):
			abduction := abductionQueue[abductionIndex]
			abductionIndex++
			if err := queueEdge(abduction.Edge, true, "abducted edge"); err != nil {
				return err
			}
			for _, endpoint := range []struct {
				node *Node
				kind string
			}{
				{abduction.OriginallyFrom, "abduction original source"},
				{abduction.OriginallyTo, "abduction original target"},
				{abduction.CurrentFrom, "abduction current source"},
				{abduction.CurrentTo, "abduction current target"},
			} {
				if err := queueNode(endpoint.node, false, endpoint.kind); err != nil {
					return err
				}
			}

		case herdIndex < len(herdQueue):
			herd := herdQueue[herdIndex]
			herdIndex++
			for node := range herd.oppositeSidePaired {
				if err := queueNode(node, true, "opposite-side herd node"); err != nil {
					return err
				}
			}
			for node := range herd.sameSidePaired {
				if err := queueNode(node, true, "same-side herd node"); err != nil {
					return err
				}
			}

		case hierarchyIndex < len(hierarchyQueue):
			hierarchy := hierarchyQueue[hierarchyIndex]
			hierarchyIndex++
			for node := range hierarchy.level {
				if err := queueNode(node, true, "hierarchy level node"); err != nil {
					return err
				}
			}
		}
	}

	// Validate parent chains independently of the declared container map so a
	// stale or hidden parent pointer cannot introduce recursion or excessive
	// depth into later snapshot/layout walks.
	if err := validateNodeParentRelation(guard, nodes, "container parent", func(node *Node) *Node {
		return node.Container
	}); err != nil {
		return err
	}

	// Active clusters and sequences override Node.Container in container.
	// Validate that effective parent relation separately: ancestry helpers walk
	// it directly and would otherwise recurse forever even when the raw
	// Container pointers form a valid forest.
	if err := validateNodeParentRelation(guard, nodes, "effective container parent", func(node *Node) *Node {
		return node.container()
	}); err != nil {
		return err
	}
	if err := validateNodeParentRelation(guard, nodes, "ancestry parent", ancestryParent); err != nil {
		return err
	}

	// Every descendant walk in the engine follows the union of direct
	// containers, active cluster membership, and active sequence membership.
	// Validate that combined ownership graph, not just Containers, so later
	// recursive movement/snapshot helpers have a firm depth bound.
	type descendantFrame struct {
		node     *Node
		children []*Node
		index    int
	}
	descendantChildren := func(node *Node) ([]*Node, error) {
		var children []*Node
		if node == nil || node.isContainer {
			children = append(children, graph.Containers[node]...)
		}
		if node != nil && node.isClusterVessel {
			cluster := graph.Clusters[node]
			if cluster == nil {
				return nil, fmt.Errorf("TALA engine cluster vessel %d has no cluster record", node.ID)
			}
			children = append(children, cluster.Nodes...)
		}
		if sequence, exists := graph.Sequences[node]; exists {
			if sequence == nil {
				return nil, structuralNil("sequence record")
			}
			children = append(children, sequence.Nodes...)
		}
		return children, nil
	}
	descendantColor := make(map[*Node]uint8, len(nodes)+1)
	descendantHeight := make(map[*Node]int, len(nodes)+1)
	descendantStarts := make([]*Node, 0, len(nodes)+1)
	if _, hasRoot := graph.Containers[nil]; hasRoot {
		descendantStarts = append(descendantStarts, nil)
	}
	for node := range nodes {
		if node != nil {
			descendantStarts = append(descendantStarts, node)
		}
	}
	for _, start := range descendantStarts {
		if descendantColor[start] != 0 {
			continue
		}
		children, err := descendantChildren(start)
		if err != nil {
			return err
		}
		descendantColor[start] = 1
		stack := []descendantFrame{{node: start, children: children}}
		for len(stack) > 0 {
			frame := &stack[len(stack)-1]
			if frame.index >= len(frame.children) {
				ownHeight := 1
				if frame.node == nil {
					ownHeight = 0
				}
				height := ownHeight
				for _, child := range frame.children {
					if childHeight := ownHeight + descendantHeight[child]; childHeight > height {
						height = childHeight
					}
				}
				if height > maxEngineTopologyDepth {
					return fmt.Errorf("TALA engine descendant depth exceeds limit %d", maxEngineTopologyDepth)
				}
				descendantHeight[frame.node] = height
				descendantColor[frame.node] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			child := frame.children[frame.index]
			frame.index++
			if err := guard.Step(); err != nil {
				return err
			}
			switch descendantColor[child] {
			case 1:
				return fmt.Errorf("TALA engine descendant cycle detected at node %d", child.ID)
			case 0:
				children, err := descendantChildren(child)
				if err != nil {
					return err
				}
				descendantColor[child] = 1
				stack = append(stack, descendantFrame{node: child, children: children})
			}
		}
	}

	// Tree child links and parent links are both traversed because either side
	// can be stale independently.
	type treeFrame struct {
		tree  *Tree
		index int
	}
	treeColor := make(map[*Tree]uint8, len(trees))
	treeHeight := make(map[*Tree]int, len(trees))
	for start := range trees {
		if treeColor[start] != 0 {
			continue
		}
		treeColor[start] = 1
		stack := []treeFrame{{tree: start}}
		for len(stack) > 0 {
			frame := &stack[len(stack)-1]
			if frame.index >= len(frame.tree.Children) {
				height := 1
				for _, child := range frame.tree.Children {
					if childHeight := treeHeight[child] + 1; childHeight > height {
						height = childHeight
					}
				}
				if height > maxEngineTopologyDepth {
					return fmt.Errorf("TALA engine tree depth exceeds limit %d", maxEngineTopologyDepth)
				}
				treeHeight[frame.tree] = height
				treeColor[frame.tree] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			child := frame.tree.Children[frame.index]
			frame.index++
			if err := guard.Step(); err != nil {
				return err
			}
			switch treeColor[child] {
			case 1:
				return fmt.Errorf("TALA engine tree child cycle detected")
			case 0:
				treeColor[child] = 1
				stack = append(stack, treeFrame{tree: child})
			}
		}
	}
	parentState := make(map[*Tree]uint8, len(trees))
	parentDepth := make(map[*Tree]int, len(trees))
	for start := range trees {
		if parentState[start] == 2 {
			continue
		}
		var path []*Tree
		current := start
		for current != nil && parentState[current] == 0 {
			if err := guard.Step(); err != nil {
				return err
			}
			parentState[current] = 1
			path = append(path, current)
			current = current.Parent
		}
		if current != nil && parentState[current] == 1 {
			return fmt.Errorf("TALA engine tree parent cycle detected")
		}
		depth := 0
		if current != nil {
			depth = parentDepth[current]
		}
		for _, p := range slices.Backward(path) {
			depth++
			if depth > maxEngineTopologyDepth {
				return fmt.Errorf("TALA engine tree depth exceeds limit %d", maxEngineTopologyDepth)
			}
			parentDepth[p] = depth
			parentState[p] = 2
		}
	}

	// A serialized tree is a value tree, not a graph: every runtime Tree must
	// therefore have exactly one owning root/parent path. The inventory above
	// deliberately de-duplicates pointers to keep preflight work linear, so do a
	// separate forest walk before serialization. Without this check, a small DAG
	// whose children repeatedly share the same subtree would expand
	// exponentially while Serialize copied it into graph JSON values.
	type treeOwnershipFrame struct {
		tree   *Tree
		parent *Tree
	}
	ownedTrees := make(map[*Tree]*Tree, len(trees))
	treeByNode := make(map[*Node]*Tree, len(trees))
	for _, roots := range graph.Trees {
		for _, root := range roots {
			stack := []treeOwnershipFrame{{tree: root}}
			for len(stack) > 0 {
				frame := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if err := guard.Step(); err != nil {
					return err
				}
				if owner, exists := ownedTrees[frame.tree]; exists {
					if frame.parent == nil {
						return fmt.Errorf("TALA engine tree is listed as more than one root")
					}
					if owner == frame.parent {
						return fmt.Errorf("TALA engine tree is repeated under one parent")
					}
					return fmt.Errorf("TALA engine tree is shared by multiple parents")
				}
				ownedTrees[frame.tree] = frame.parent
				if existing, exists := treeByNode[frame.tree.Node]; exists && existing != frame.tree {
					return fmt.Errorf("TALA engine node %d is owned by multiple trees", frame.tree.Node.ID)
				}
				treeByNode[frame.tree.Node] = frame.tree

				for _, child := range slices.Backward(frame.tree.Children) {

					if child.Parent != frame.tree {
						return fmt.Errorf("TALA engine tree child has an inconsistent parent")
					}
					stack = append(stack, treeOwnershipFrame{tree: child, parent: frame.tree})
				}
			}
		}
	}
	for _, roots := range graph.Trees {
		for _, root := range roots {
			if root.Parent == nil {
				continue
			}
			// Placement-tree construction temporarily attaches an installed root beneath a
			// placement-only wrapper. That wrapper is not part of Graph.Trees, but
			// its backlink must still be exact. An installed parent would instead
			// give this root two locations in the serialized forest.
			if _, installed := ownedTrees[root.Parent]; installed {
				return fmt.Errorf("TALA engine tree root also has an installed parent")
			}
			occurrences := 0
			for _, child := range root.Parent.Children {
				if err := guard.Step(); err != nil {
					return err
				}
				if child == root {
					occurrences++
				}
			}
			if occurrences != 1 {
				return fmt.Errorf("TALA engine tree root has an inconsistent placement parent")
			}
		}
	}

	// NodeToTree is derived state and may legitimately be absent on a freshly
	// constructed graph. When present alongside an installed forest, however,
	// it must be the exact inverse of Tree.Node; accepting partial or orphaned
	// aliases would make later layout helpers observe a different topology from
	// the serializer.
	for node, tree := range graph.NodeToTree {
		if tree.Node != node {
			return fmt.Errorf("TALA engine node-to-tree alias does not match the tree node")
		}
		if len(ownedTrees) > 0 {
			if _, owned := ownedTrees[tree]; !owned {
				return fmt.Errorf("TALA engine node-to-tree alias references a tree outside the installed forest")
			}
		}
	}
	if len(graph.NodeToTree) > 0 && len(ownedTrees) > 0 {
		if len(graph.NodeToTree) != len(treeByNode) {
			return fmt.Errorf("TALA engine node-to-tree aliases do not cover the installed forest")
		}
		for node, tree := range treeByNode {
			if graph.NodeToTree[node] != tree {
				return fmt.Errorf("TALA engine node-to-tree aliases do not match the installed forest")
			}
		}
	}

	return guard.Finish()
}
