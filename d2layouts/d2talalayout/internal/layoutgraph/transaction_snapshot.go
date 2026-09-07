package layoutgraph

import (
	"fmt"
	"maps"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type pointerSnapshot[T any] struct {
	pointer *T
	value   T
}

// exactSliceSnapshot retains both the original slice header and every element
// in its backing array. Restoring through the original header preserves
// pointer, length, capacity, and aliases even when a mutation appended into
// spare capacity before rollback.
type exactSliceSnapshot[S ~[]V, V any] struct {
	original S
	backing  []V
}

func captureExactSlice[S ~[]V, V any](values S) exactSliceSnapshot[S, V] {
	if values == nil {
		return exactSliceSnapshot[S, V]{}
	}
	full := values[:cap(values)]
	backing := make([]V, len(full))
	copy(backing, full)
	return exactSliceSnapshot[S, V]{original: values, backing: backing}
}

func (snapshot exactSliceSnapshot[S, V]) restore() S {
	if snapshot.original == nil {
		return nil
	}
	copy(snapshot.original[:cap(snapshot.original)], snapshot.backing)
	return snapshot.original
}

type exactSliceMapSnapshot[K comparable, S ~[]V, V any] struct {
	original map[K]S
	values   map[K]exactSliceSnapshot[S, V]
}

func captureExactSliceMap[K comparable, S ~[]V, V any](values map[K]S) exactSliceMapSnapshot[K, S, V] {
	snapshot := exactSliceMapSnapshot[K, S, V]{original: values}
	if values == nil {
		return snapshot
	}
	snapshot.values = make(map[K]exactSliceSnapshot[S, V], len(values))
	for key, items := range values {
		snapshot.values[key] = captureExactSlice(items)
	}
	return snapshot
}

func (snapshot exactSliceMapSnapshot[K, S, V]) restore() map[K]S {
	if snapshot.original == nil {
		return nil
	}
	clear(snapshot.original)
	for key, items := range snapshot.values {
		snapshot.original[key] = items.restore()
	}
	return snapshot.original
}

func snapshotPointer[T any](pointer *T) pointerSnapshot[T] {
	if pointer == nil {
		return pointerSnapshot[T]{}
	}
	return pointerSnapshot[T]{pointer: pointer, value: *pointer}
}

func (s pointerSnapshot[T]) restore() *T {
	if s.pointer == nil {
		return nil
	}
	*s.pointer = s.value
	return s.pointer
}

type nodeSnapshot struct {
	value Node

	d2ID          pointerSnapshot[string]
	topLeft       pointerSnapshot[geo.Point]
	fontSize      pointerSnapshot[int]
	fixedTopLeft  pointerSnapshot[geo.Point]
	desiredWidth  pointerSnapshot[float64]
	desiredHeight pointerSnapshot[float64]
	label         pointerSnapshot[Label]
	icon          pointerSnapshot[Icon]

	edges                            exactSliceSnapshot[[]*Edge, *Edge]
	nears                            map[*Node]struct{}
	loopOffsets                      map[geo.Orientation]float64
	longDistanceNeighborRequirements map[*Node]LongDistanceNeighborRequirements
}

type edgeSnapshot struct {
	value Edge

	d2ID                 pointerSnapshot[string]
	points               exactSliceSnapshot[[]*geo.Point, *geo.Point]
	pointValues          []pointerSnapshot[geo.Point]
	label                pointerSnapshot[Label]
	sourceArrowheadLabel pointerSnapshot[Label]
	targetArrowheadLabel pointerSnapshot[Label]
	fromTableColumnIndex pointerSnapshot[int]
	toTableColumnIndex   pointerSnapshot[int]
}

type clusterSnapshot struct {
	value          Cluster
	nodes          exactSliceSnapshot[[]*Node, *Node]
	edgeAbductions exactSliceSnapshot[[]*EdgeAbduction, *EdgeAbduction]
}

type sequenceSnapshot struct {
	value          Sequence
	nodes          exactSliceSnapshot[[]*Node, *Node]
	edgeAbductions exactSliceSnapshot[[]*EdgeAbduction, *EdgeAbduction]
}

type treeSnapshot struct {
	value    Tree
	children exactSliceSnapshot[[]*Tree, *Tree]
}

type herdSnapshot struct {
	value              HerdAssignment
	oppositeSidePaired map[*Node]struct{}
	sameSidePaired     map[*Node]struct{}
}

type hierarchySnapshot struct {
	value Hierarchy
	level map[*Node]int
}

type graphSnapshot struct {
	isRootHierarchy bool
	nodes           exactSliceSnapshot[[]*Node, *Node]
	edges           exactSliceSnapshot[[]*Edge, *Edge]
	cellSize        float64

	containers     exactSliceMapSnapshot[*Node, []*Node, *Node]
	clustersRef    map[*Node]*Cluster
	clusters       map[*Node]*Cluster
	trees          exactSliceMapSnapshot[*Node, []*Tree, *Tree]
	nodeToTreeRef  map[*Node]*Tree
	nodeToTree     map[*Node]*Tree
	hubs           exactSliceMapSnapshot[*Node, []*Node, *Node]
	sequencesRef   map[*Node]*Sequence
	sequences      map[*Node]*Sequence
	directionsRef  map[*Node]geo.Orientation
	directions     map[*Node]geo.Orientation
	commonSiblings exactSliceMapSnapshot[*Node, Nodes, *Node]
}

func cloneMapContents[K comparable, V any](values map[K]V) map[K]V {
	if len(values) == 0 {
		return nil
	}
	return maps.Clone(values)
}

func restoreMap[K comparable, V any](original, snapshot map[K]V) map[K]V {
	if original == nil {
		return nil
	}
	clear(original)
	maps.Copy(original, snapshot)
	return original
}

func captureNode(node *Node) nodeSnapshot {
	return nodeSnapshot{
		value:                            *node,
		d2ID:                             snapshotPointer(node.D2ID),
		topLeft:                          snapshotPointer(node.TopLeft),
		fontSize:                         snapshotPointer(node.FontSize),
		fixedTopLeft:                     snapshotPointer(node.FixedTopLeft),
		desiredWidth:                     snapshotPointer(node.DesiredWidth),
		desiredHeight:                    snapshotPointer(node.DesiredHeight),
		label:                            snapshotPointer(node.Label),
		icon:                             snapshotPointer(node.Icon),
		edges:                            captureExactSlice(node.Edges),
		nears:                            cloneMapContents(node.Nears),
		loopOffsets:                      cloneMapContents(node.LoopOffsets),
		longDistanceNeighborRequirements: cloneMapContents(node.LongDistanceNeighborRequirements),
	}
}

func (s nodeSnapshot) restore(node *Node) {
	s.d2ID.restore()
	s.topLeft.restore()
	s.fontSize.restore()
	s.fixedTopLeft.restore()
	s.desiredWidth.restore()
	s.desiredHeight.restore()
	s.label.restore()
	s.icon.restore()

	*node = s.value
	node.Edges = s.edges.restore()
	node.Nears = restoreMap(s.value.Nears, s.nears)
	node.LoopOffsets = restoreMap(s.value.LoopOffsets, s.loopOffsets)
	node.LongDistanceNeighborRequirements = restoreMap(s.value.LongDistanceNeighborRequirements, s.longDistanceNeighborRequirements)
}

func captureEdge(edge *Edge) edgeSnapshot {
	fullPoints := edge.Points[:cap(edge.Points)]
	pointValues := make([]pointerSnapshot[geo.Point], len(fullPoints))
	for i, point := range fullPoints {
		pointValues[i] = snapshotPointer(point)
	}
	return edgeSnapshot{
		value:                *edge,
		d2ID:                 snapshotPointer(edge.D2ID),
		points:               captureExactSlice(edge.Points),
		pointValues:          pointValues,
		label:                snapshotPointer(edge.Label),
		sourceArrowheadLabel: snapshotPointer(edge.SourceArrowheadLabel),
		targetArrowheadLabel: snapshotPointer(edge.TargetArrowheadLabel),
		fromTableColumnIndex: snapshotPointer(edge.FromTableColumnIndex),
		toTableColumnIndex:   snapshotPointer(edge.ToTableColumnIndex),
	}
}

func (s edgeSnapshot) restore(edge *Edge) {
	s.d2ID.restore()
	s.label.restore()
	s.sourceArrowheadLabel.restore()
	s.targetArrowheadLabel.restore()
	s.fromTableColumnIndex.restore()
	s.toTableColumnIndex.restore()
	// s.value.Points retains the original slice header. Repair that backing
	// array before reattaching it so rollback preserves aliases to both the
	// route slice and its point objects.
	for _, point := range s.pointValues {
		point.restore()
	}
	originalPoints := s.points.restore()

	*edge = s.value
	edge.Points = originalPoints
}

func captureGraph(graph *Graph) graphSnapshot {
	return graphSnapshot{
		isRootHierarchy: graph.IsRootHierarchy,
		nodes:           captureExactSlice(graph.Nodes),
		edges:           captureExactSlice(graph.Edges),
		cellSize:        graph.CellSize,
		containers:      captureExactSliceMap(graph.Containers),
		clustersRef:     graph.Clusters,
		clusters:        cloneMapContents(graph.Clusters),
		trees:           captureExactSliceMap(graph.Trees),
		nodeToTreeRef:   graph.NodeToTree,
		nodeToTree:      cloneMapContents(graph.NodeToTree),
		hubs:            captureExactSliceMap(graph.Hubs),
		sequencesRef:    graph.Sequences,
		sequences:       cloneMapContents(graph.Sequences),
		directionsRef:   graph.Directions,
		directions:      cloneMapContents(graph.Directions),
		commonSiblings:  captureExactSliceMap(graph.CommonUncleSiblings),
	}
}

// captureGeometryState records the fields changed by ordinary speculative
// placement transactions without walking and copying the graph's ownership
// maps. Topology-changing transactions additionally call captureRuntimeState.
func (gs *GraphState) captureGeometryStateContext(graph *Graph, guard *limits.WorkGuard) error {
	if gs.captureEdgeRoutes {
		edges := make(map[*Edge]struct{}, len(graph.Edges))
		for _, edge := range graph.Edges {
			if err := guard.Step(); err != nil {
				return err
			}
			if edge == nil {
				return fmt.Errorf("TALA geometry snapshot contains a nil edge")
			}
			edges[edge] = struct{}{}
		}
		for node := range gs.nodeGeometry {
			if err := guard.Step(); err != nil {
				return err
			}
			for _, edge := range node.Edges {
				if err := guard.Step(); err != nil {
					return err
				}
				if edge != nil {
					edges[edge] = struct{}{}
				}
			}
		}
		gs.edgeGeometry = make(map[*Edge]edgeSnapshot, len(edges))
		for edge := range edges {
			if err := guard.Step(); err != nil {
				return err
			}
			for range edge.Points[:cap(edge.Points)] {
				if err := guard.Step(); err != nil {
					return err
				}
			}
			gs.edgeGeometry[edge] = captureEdge(edge)
		}
	} else {
		gs.edgeGeometry = nil
	}

	gs.treeOrientations = make(map[*Tree]geo.Orientation, len(graph.NodeToTree))
	for _, tree := range graph.NodeToTree {
		if err := guard.Step(); err != nil {
			return err
		}
		if tree != nil {
			gs.treeOrientations[tree] = tree.Orientation
		}
	}
	return guard.Finish()
}

func (s graphSnapshot) restore(graph *Graph) {
	graph.IsRootHierarchy = s.isRootHierarchy
	graph.Nodes = s.nodes.restore()
	graph.Edges = s.edges.restore()
	graph.CellSize = s.cellSize
	graph.Containers = s.containers.restore()
	graph.Clusters = restoreMap(s.clustersRef, s.clusters)
	graph.Trees = s.trees.restore()
	graph.NodeToTree = restoreMap(s.nodeToTreeRef, s.nodeToTree)
	graph.Hubs = s.hubs.restore()
	graph.Sequences = restoreMap(s.sequencesRef, s.sequences)
	graph.Directions = restoreMap(s.directionsRef, s.directions)
	graph.CommonUncleSiblings = s.commonSiblings.restore()
}

type runtimeObjectCollection struct {
	nodes          map[*Node]struct{}
	nodeOrder      []*Node
	edges          map[*Edge]struct{}
	clusters       map[*Cluster]struct{}
	sequences      map[*Sequence]struct{}
	trees          map[*Tree]struct{}
	edgeAbductions map[*EdgeAbduction]struct{}
	herds          map[*HerdAssignment]struct{}
	hierarchies    map[*Hierarchy]struct{}
}

// collectRuntimeObjectsContext follows every reference that can keep mutable
// layout state alive outside Graph.Nodes. Both full transaction snapshots and
// narrower field snapshots use this one bounded traversal so their ownership
// coverage cannot drift apart.
func collectRuntimeObjectsContext(graph *Graph, guard *limits.WorkGuard, scope string) (runtimeObjectCollection, error) {
	nodes := make(map[*Node]struct{}, len(graph.Nodes))
	var edges map[*Edge]struct{}
	var clusters map[*Cluster]struct{}
	var sequences map[*Sequence]struct{}
	var trees map[*Tree]struct{}
	var edgeAbductions map[*EdgeAbduction]struct{}
	var herds map[*HerdAssignment]struct{}
	var hierarchies map[*Hierarchy]struct{}

	var nodeQueue []*Node
	var edgeQueue []*Edge
	var clusterQueue []*Cluster
	var sequenceQueue []*Sequence
	var treeQueue []*Tree
	var abductionQueue []*EdgeAbduction
	var herdQueue []*HerdAssignment
	var hierarchyQueue []*Hierarchy
	var captureErr error
	charge := func() bool {
		if captureErr != nil {
			return false
		}
		captureErr = guard.Step()
		return captureErr == nil
	}

	addNode := func(node *Node) {
		if !charge() {
			return
		}
		if node == nil {
			return
		}
		if _, exists := nodes[node]; exists {
			return
		}
		if len(nodes) >= maxEngineNodes {
			captureErr = fmt.Errorf("TALA %s node snapshot exceeds limit %d", scope, maxEngineNodes)
			return
		}
		nodes[node] = struct{}{}
		nodeQueue = append(nodeQueue, node)
	}
	addEdge := func(edge *Edge) {
		if !charge() {
			return
		}
		if edge == nil {
			return
		}
		if _, exists := edges[edge]; exists {
			return
		}
		if len(edges) >= maxEngineEdges {
			captureErr = fmt.Errorf("TALA %s edge snapshot exceeds limit %d", scope, maxEngineEdges)
			return
		}
		if edges == nil {
			edges = make(map[*Edge]struct{})
		}
		edges[edge] = struct{}{}
		edgeQueue = append(edgeQueue, edge)
	}
	addCluster := func(cluster *Cluster) {
		if !charge() {
			return
		}
		if cluster == nil {
			return
		}
		if _, exists := clusters[cluster]; exists {
			return
		}
		if len(clusters) >= maxEngineNodes {
			captureErr = fmt.Errorf("TALA %s cluster snapshot exceeds limit %d", scope, maxEngineNodes)
			return
		}
		if clusters == nil {
			clusters = make(map[*Cluster]struct{})
		}
		clusters[cluster] = struct{}{}
		clusterQueue = append(clusterQueue, cluster)
	}
	addSequence := func(sequence *Sequence) {
		if !charge() {
			return
		}
		if sequence == nil {
			return
		}
		if _, exists := sequences[sequence]; exists {
			return
		}
		if len(sequences) >= maxEngineNodes {
			captureErr = fmt.Errorf("TALA %s sequence snapshot exceeds limit %d", scope, maxEngineNodes)
			return
		}
		if sequences == nil {
			sequences = make(map[*Sequence]struct{})
		}
		sequences[sequence] = struct{}{}
		sequenceQueue = append(sequenceQueue, sequence)
	}
	addTree := func(tree *Tree) {
		if !charge() {
			return
		}
		if tree == nil {
			return
		}
		if _, exists := trees[tree]; exists {
			return
		}
		if len(trees) >= maxEngineNodes {
			captureErr = fmt.Errorf("TALA %s tree snapshot exceeds limit %d", scope, maxEngineNodes)
			return
		}
		if trees == nil {
			trees = make(map[*Tree]struct{})
		}
		trees[tree] = struct{}{}
		treeQueue = append(treeQueue, tree)
	}
	addEdgeAbduction := func(abduction *EdgeAbduction) {
		if !charge() {
			return
		}
		if abduction == nil {
			return
		}
		if _, exists := edgeAbductions[abduction]; exists {
			return
		}
		if len(edgeAbductions) >= maxEngineEdges {
			captureErr = fmt.Errorf("TALA %s edge-abduction snapshot exceeds limit %d", scope, maxEngineEdges)
			return
		}
		if edgeAbductions == nil {
			edgeAbductions = make(map[*EdgeAbduction]struct{})
		}
		edgeAbductions[abduction] = struct{}{}
		abductionQueue = append(abductionQueue, abduction)
	}
	addHerd := func(herd *HerdAssignment) {
		if !charge() {
			return
		}
		if herd == nil {
			return
		}
		if _, exists := herds[herd]; exists {
			return
		}
		if len(herds) >= maxEngineNodes {
			captureErr = fmt.Errorf("TALA %s herd snapshot exceeds limit %d", scope, maxEngineNodes)
			return
		}
		if herds == nil {
			herds = make(map[*HerdAssignment]struct{})
		}
		herds[herd] = struct{}{}
		herdQueue = append(herdQueue, herd)
	}
	addHierarchy := func(hierarchy *Hierarchy) {
		if !charge() {
			return
		}
		if hierarchy == nil {
			return
		}
		if _, exists := hierarchies[hierarchy]; exists {
			return
		}
		if len(hierarchies) >= maxEngineNodes {
			captureErr = fmt.Errorf("TALA %s hierarchy snapshot exceeds limit %d", scope, maxEngineNodes)
			return
		}
		if hierarchies == nil {
			hierarchies = make(map[*Hierarchy]struct{})
		}
		hierarchies[hierarchy] = struct{}{}
		hierarchyQueue = append(hierarchyQueue, hierarchy)
	}

	for _, node := range graph.Nodes {
		addNode(node)
	}
	for _, edge := range graph.Edges {
		addEdge(edge)
	}
	for container, children := range graph.Containers {
		addNode(container)
		for _, child := range children {
			addNode(child)
		}
	}
	for vessel, cluster := range graph.Clusters {
		addNode(vessel)
		addCluster(cluster)
	}
	for vessel, sequence := range graph.Sequences {
		addNode(vessel)
		addSequence(sequence)
	}
	for node, nodeTrees := range graph.Trees {
		addNode(node)
		for _, tree := range nodeTrees {
			addTree(tree)
		}
	}
	for node, tree := range graph.NodeToTree {
		addNode(node)
		addTree(tree)
	}
	for hub, hubNodes := range graph.Hubs {
		addNode(hub)
		for _, node := range hubNodes {
			addNode(node)
		}
	}
	for node, siblings := range graph.CommonUncleSiblings {
		addNode(node)
		for _, sibling := range siblings {
			addNode(sibling)
		}
	}
	for node := range graph.Directions {
		addNode(node)
	}
	if captureErr != nil {
		return runtimeObjectCollection{}, captureErr
	}

	var nodeIndex, edgeIndex, clusterIndex, sequenceIndex int
	var treeIndex, abductionIndex, herdIndex, hierarchyIndex int
	for nodeIndex < len(nodeQueue) || edgeIndex < len(edgeQueue) || clusterIndex < len(clusterQueue) ||
		sequenceIndex < len(sequenceQueue) || treeIndex < len(treeQueue) || abductionIndex < len(abductionQueue) ||
		herdIndex < len(herdQueue) || hierarchyIndex < len(hierarchyQueue) {
		if captureErr != nil {
			return runtimeObjectCollection{}, captureErr
		}
		switch {
		case nodeIndex < len(nodeQueue):
			node := nodeQueue[nodeIndex]
			nodeIndex++
			addNode(node.Container)
			for near := range node.Nears {
				addNode(near)
			}
			for neighbor := range node.LongDistanceNeighborRequirements {
				addNode(neighbor)
			}
			for _, edge := range node.Edges {
				addEdge(edge)
			}
			addCluster(node.Cluster)
			addSequence(node.Sequence)
			addHerd(node.HerdAssignment)
			addHierarchy(node.Hierarchy)

		case edgeIndex < len(edgeQueue):
			edge := edgeQueue[edgeIndex]
			edgeIndex++
			addNode(edge.From)
			addNode(edge.To)

		case clusterIndex < len(clusterQueue):
			cluster := clusterQueue[clusterIndex]
			clusterIndex++
			addNode(cluster.Vessel)
			addNode(cluster.Container)
			for _, node := range cluster.Nodes {
				addNode(node)
			}
			for _, abduction := range cluster.EdgeAbductions {
				addEdgeAbduction(abduction)
			}

		case sequenceIndex < len(sequenceQueue):
			sequence := sequenceQueue[sequenceIndex]
			sequenceIndex++
			addNode(sequence.Vessel)
			addNode(sequence.Container)
			for _, node := range sequence.Nodes {
				addNode(node)
			}
			for _, abduction := range sequence.EdgeAbductions {
				addEdgeAbduction(abduction)
			}

		case treeIndex < len(treeQueue):
			tree := treeQueue[treeIndex]
			treeIndex++
			addNode(tree.Node)
			addEdge(tree.SentinelEdge)
			addTree(tree.Parent)
			for _, child := range tree.Children {
				addTree(child)
			}

		case abductionIndex < len(abductionQueue):
			abduction := abductionQueue[abductionIndex]
			abductionIndex++
			addEdge(abduction.Edge)
			addNode(abduction.OriginallyFrom)
			addNode(abduction.OriginallyTo)
			addNode(abduction.CurrentFrom)
			addNode(abduction.CurrentTo)

		case herdIndex < len(herdQueue):
			herd := herdQueue[herdIndex]
			herdIndex++
			for node := range herd.oppositeSidePaired {
				addNode(node)
			}
			for node := range herd.sameSidePaired {
				addNode(node)
			}

		case hierarchyIndex < len(hierarchyQueue):
			hierarchy := hierarchyQueue[hierarchyIndex]
			hierarchyIndex++
			for node := range hierarchy.level {
				addNode(node)
			}
		}
		if captureErr != nil {
			return runtimeObjectCollection{}, captureErr
		}
	}

	return runtimeObjectCollection{
		nodes:          nodes,
		nodeOrder:      nodeQueue,
		edges:          edges,
		clusters:       clusters,
		sequences:      sequences,
		trees:          trees,
		edgeAbductions: edgeAbductions,
		herds:          herds,
		hierarchies:    hierarchies,
	}, nil
}

func (gs *GraphState) captureRuntimeStateContext(graph *Graph, guard *limits.WorkGuard) error {
	objects, err := collectRuntimeObjectsContext(graph, guard, "transaction runtime")
	if err != nil {
		return err
	}
	nodes := objects.nodes
	edges := objects.edges
	clusters := objects.clusters
	sequences := objects.sequences
	trees := objects.trees
	edgeAbductions := objects.edgeAbductions
	herds := objects.herds
	hierarchies := objects.hierarchies

	if err := guard.Step(); err != nil {
		return err
	}
	gs.graph = captureGraph(graph)
	gs.nodes = make(map[*Node]nodeSnapshot, len(nodes))
	for node := range nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		gs.nodes[node] = captureNode(node)
	}
	gs.edges = make(map[*Edge]edgeSnapshot, len(edges))
	for edge := range edges {
		if err := guard.Step(); err != nil {
			return err
		}
		for range edge.Points[:cap(edge.Points)] {
			if err := guard.Step(); err != nil {
				return err
			}
		}
		gs.edges[edge] = captureEdge(edge)
	}
	gs.clusters = make(map[*Cluster]clusterSnapshot, len(clusters))
	for cluster := range clusters {
		if err := guard.Step(); err != nil {
			return err
		}
		gs.clusters[cluster] = clusterSnapshot{
			value:          *cluster,
			nodes:          captureExactSlice(cluster.Nodes),
			edgeAbductions: captureExactSlice(cluster.EdgeAbductions),
		}
	}
	gs.sequences = make(map[*Sequence]sequenceSnapshot, len(sequences))
	for sequence := range sequences {
		if err := guard.Step(); err != nil {
			return err
		}
		gs.sequences[sequence] = sequenceSnapshot{
			value:          *sequence,
			nodes:          captureExactSlice(sequence.Nodes),
			edgeAbductions: captureExactSlice(sequence.EdgeAbductions),
		}
	}
	gs.trees = make(map[*Tree]treeSnapshot, len(trees))
	for tree := range trees {
		if err := guard.Step(); err != nil {
			return err
		}
		gs.trees[tree] = treeSnapshot{value: *tree, children: captureExactSlice(tree.Children)}
	}
	gs.edgeAbductions = make(map[*EdgeAbduction]EdgeAbduction, len(edgeAbductions))
	for abduction := range edgeAbductions {
		if err := guard.Step(); err != nil {
			return err
		}
		gs.edgeAbductions[abduction] = *abduction
	}
	gs.herds = make(map[*HerdAssignment]herdSnapshot, len(herds))
	for herd := range herds {
		if err := guard.Step(); err != nil {
			return err
		}
		gs.herds[herd] = herdSnapshot{
			value:              *herd,
			oppositeSidePaired: cloneMapContents(herd.oppositeSidePaired),
			sameSidePaired:     cloneMapContents(herd.sameSidePaired),
		}
	}
	gs.hierarchies = make(map[*Hierarchy]hierarchySnapshot, len(hierarchies))
	for hierarchy := range hierarchies {
		if err := guard.Step(); err != nil {
			return err
		}
		gs.hierarchies[hierarchy] = hierarchySnapshot{value: *hierarchy, level: cloneMapContents(hierarchy.level)}
	}
	return guard.Finish()
}
