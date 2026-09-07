package trees

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type treePreprocessState struct {
	nodes        exactTestSlice[*layoutgraph.Node]
	edges        exactTestSlice[*layoutgraph.Edge]
	rootChildren exactTestSlice[*layoutgraph.Node]
	nodeEdges    map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Edge]
	nodeGraphs   map[*layoutgraph.Node]*layoutgraph.Graph
	nodeParents  map[*layoutgraph.Node]*layoutgraph.Node

	containersMap uintptr
	treesMap      uintptr
	nodeToTreeMap uintptr

	route       exactTestSlice[*geo.Point]
	routeValues []geo.Point
}

type reconnectTreeState struct {
	nodes exactTestSlice[*layoutgraph.Node]
	edges exactTestSlice[*layoutgraph.Edge]

	containersMap uintptr
	treesMap      uintptr
	nodeToTreeMap uintptr
	containers    map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Node]
	trees         map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Tree]
	nodeToTree    map[*layoutgraph.Node]*layoutgraph.Tree

	nodeEdges       map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Edge]
	nodeGraphs      map[*layoutgraph.Node]*layoutgraph.Graph
	nodeContainers  map[*layoutgraph.Node]*layoutgraph.Node
	nodeIsContainer map[*layoutgraph.Node]bool
	nodeBoxes       map[*layoutgraph.Node]geo.Box
	topLeftValues   map[*geo.Point]geo.Point

	treeChildren     map[*layoutgraph.Tree]exactTestSlice[*layoutgraph.Tree]
	treeParents      map[*layoutgraph.Tree]*layoutgraph.Tree
	treeOrientations map[*layoutgraph.Tree]geo.Orientation

	routes           map[*layoutgraph.Edge]exactTestSlice[*geo.Point]
	routePointValues map[*geo.Point]geo.Point
	edgeLabels       map[*layoutgraph.Edge]pointerSnapshot[layoutgraph.Label]
	labelPercentages map[*layoutgraph.Edge]float64
}

func captureReconnectTreeState(g *layoutgraph.Graph) reconnectTreeState {
	runtimeState := collectTreeRuntimeState(g)
	state := reconnectTreeState{
		nodes:            captureExactTestSlice(g.Nodes),
		edges:            captureExactTestSlice(g.Edges),
		containersMap:    reflect.ValueOf(g.Containers).Pointer(),
		treesMap:         reflect.ValueOf(g.Trees).Pointer(),
		nodeToTreeMap:    reflect.ValueOf(g.NodeToTree).Pointer(),
		containers:       make(map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Node], len(g.Containers)),
		trees:            make(map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Tree], len(g.Trees)),
		nodeToTree:       make(map[*layoutgraph.Node]*layoutgraph.Tree, len(g.NodeToTree)),
		nodeEdges:        make(map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Edge], len(runtimeState.nodes)),
		nodeGraphs:       make(map[*layoutgraph.Node]*layoutgraph.Graph, len(runtimeState.nodes)),
		nodeContainers:   make(map[*layoutgraph.Node]*layoutgraph.Node, len(runtimeState.nodes)),
		nodeIsContainer:  make(map[*layoutgraph.Node]bool, len(runtimeState.nodes)),
		nodeBoxes:        make(map[*layoutgraph.Node]geo.Box, len(runtimeState.nodes)),
		topLeftValues:    make(map[*geo.Point]geo.Point, len(runtimeState.nodes)),
		treeChildren:     make(map[*layoutgraph.Tree]exactTestSlice[*layoutgraph.Tree], len(runtimeState.trees)),
		treeParents:      make(map[*layoutgraph.Tree]*layoutgraph.Tree, len(runtimeState.trees)),
		treeOrientations: make(map[*layoutgraph.Tree]geo.Orientation, len(runtimeState.trees)),
		routes:           make(map[*layoutgraph.Edge]exactTestSlice[*geo.Point], len(runtimeState.edges)),
		routePointValues: make(map[*geo.Point]geo.Point),
		edgeLabels:       make(map[*layoutgraph.Edge]pointerSnapshot[layoutgraph.Label], len(runtimeState.edges)),
		labelPercentages: make(map[*layoutgraph.Edge]float64, len(runtimeState.edges)),
	}
	for container, children := range g.Containers {
		state.containers[container] = captureExactTestSlice(children)
	}
	for root, trees := range g.Trees {
		state.trees[root] = captureExactTestSlice(trees)
	}
	maps.Copy(state.nodeToTree, g.NodeToTree)
	for node := range runtimeState.nodes {
		state.nodeEdges[node] = captureExactTestSlice(node.Edges)
		state.nodeGraphs[node] = node.Graph
		state.nodeContainers[node] = node.Container
		state.nodeIsContainer[node] = node.IsContainer()
		state.nodeBoxes[node] = node.Box
		if node.TopLeft != nil {
			state.topLeftValues[node.TopLeft] = *node.TopLeft
		}
	}
	for tree := range runtimeState.trees {
		state.treeChildren[tree] = captureExactTestSlice(tree.Children)
		state.treeParents[tree] = tree.Parent
		state.treeOrientations[tree] = tree.Orientation
	}
	for edge := range runtimeState.edges {
		state.routes[edge] = captureExactTestSlice(edge.Points)
		state.edgeLabels[edge] = snapshotPointer(edge.Label)
		state.labelPercentages[edge] = edge.LabelPercentage
		for _, point := range edge.Points[:cap(edge.Points)] {
			if point != nil {
				state.routePointValues[point] = *point
			}
		}
	}
	return state
}

func (state reconnectTreeState) assertRestored(t *testing.T, g *layoutgraph.Graph) {
	t.Helper()
	state.nodes.assertRestored(t, g.Nodes, "tree reconnection Graph.Nodes")
	state.edges.assertRestored(t, g.Edges, "tree reconnection Graph.Edges")
	if reflect.ValueOf(g.Containers).Pointer() != state.containersMap || len(g.Containers) != len(state.containers) {
		t.Fatal("tree reconnection did not restore the exact container map")
	}
	for container, children := range state.containers {
		got, exists := g.Containers[container]
		if !exists {
			t.Fatal("tree reconnection removed an original container entry")
		}
		children.assertRestored(t, got, "tree reconnection Graph.Containers entry")
	}
	if reflect.ValueOf(g.Trees).Pointer() != state.treesMap || len(g.Trees) != len(state.trees) {
		t.Fatal("tree reconnection did not restore the exact tree map")
	}
	for root, trees := range state.trees {
		got, exists := g.Trees[root]
		if !exists {
			t.Fatal("tree reconnection removed an original tree entry")
		}
		trees.assertRestored(t, got, "tree reconnection Graph.Trees entry")
	}
	if reflect.ValueOf(g.NodeToTree).Pointer() != state.nodeToTreeMap || len(g.NodeToTree) != len(state.nodeToTree) {
		t.Fatal("tree reconnection did not restore the exact node-to-tree map")
	}
	for node, tree := range state.nodeToTree {
		if g.NodeToTree[node] != tree {
			t.Fatalf("tree reconnection changed the node-to-tree entry for node %d", node.ID)
		}
	}
	for node, edges := range state.nodeEdges {
		edges.assertRestored(t, node.Edges, "tree reconnection Node.Edges")
		if node.Graph != state.nodeGraphs[node] {
			t.Fatalf("tree reconnection changed graph ownership for node %d", node.ID)
		}
		if node.Container != state.nodeContainers[node] {
			t.Fatalf("tree reconnection changed container ownership for node %d", node.ID)
		}
		if node.IsContainer() != state.nodeIsContainer[node] {
			t.Fatalf("tree reconnection changed the container flag for node %d", node.ID)
		}
		if node.Box != state.nodeBoxes[node] {
			t.Fatalf("tree reconnection changed the exact box for node %d", node.ID)
		}
	}
	for point, value := range state.topLeftValues {
		if *point != value {
			t.Fatal("tree reconnection changed a node position value")
		}
	}
	for tree, children := range state.treeChildren {
		children.assertRestored(t, tree.Children, "tree reconnection Tree.Children")
		if tree.Parent != state.treeParents[tree] {
			t.Fatal("tree reconnection changed a tree parent")
		}
		if tree.Orientation != state.treeOrientations[tree] {
			t.Fatal("tree reconnection changed a tree orientation")
		}
	}
	for edge, route := range state.routes {
		route.assertRestored(t, edge.Points, "tree reconnection Edge.Points")
		label := state.edgeLabels[edge]
		if edge.Label != label.pointer {
			t.Fatal("tree reconnection changed an edge label pointer")
		}
		if label.pointer != nil && !reflect.DeepEqual(*label.pointer, label.value) {
			t.Fatal("tree reconnection changed an edge label value")
		}
		if edge.LabelPercentage != state.labelPercentages[edge] {
			t.Fatal("tree reconnection changed an edge label percentage")
		}
	}
	for point, value := range state.routePointValues {
		if *point != value {
			t.Fatal("tree reconnection changed a route point value")
		}
	}
}

func captureTreePreprocessState(g *layoutgraph.Graph) treePreprocessState {
	state := treePreprocessState{
		nodes:         captureExactTestSlice(g.Nodes),
		edges:         captureExactTestSlice(g.Edges),
		rootChildren:  captureExactTestSlice(g.Containers[nil]),
		nodeEdges:     make(map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Edge], len(g.Nodes)),
		nodeGraphs:    make(map[*layoutgraph.Node]*layoutgraph.Graph, len(g.Nodes)),
		nodeParents:   make(map[*layoutgraph.Node]*layoutgraph.Node, len(g.Nodes)),
		containersMap: reflect.ValueOf(g.Containers).Pointer(),
		treesMap:      reflect.ValueOf(g.Trees).Pointer(),
		nodeToTreeMap: reflect.ValueOf(g.NodeToTree).Pointer(),
		route:         captureExactTestSlice(g.Edges[0].Points),
		routeValues:   make([]geo.Point, len(g.Edges[0].Points)),
	}
	for _, node := range g.Nodes {
		state.nodeEdges[node] = captureExactTestSlice(node.Edges)
		state.nodeGraphs[node] = node.Graph
		state.nodeParents[node] = node.Container
	}
	for i, point := range g.Edges[0].Points {
		state.routeValues[i] = *point
	}
	return state
}

func (state treePreprocessState) assertRestored(t *testing.T, g *layoutgraph.Graph) {
	t.Helper()
	state.nodes.assertRestored(t, g.Nodes, "tree preprocessing Graph.Nodes")
	state.edges.assertRestored(t, g.Edges, "tree preprocessing Graph.Edges")
	state.rootChildren.assertRestored(t, g.Containers[nil], "tree preprocessing Graph.Containers[nil]")
	if reflect.ValueOf(g.Containers).Pointer() != state.containersMap {
		t.Fatal("tree preprocessing replaced the container map during rollback")
	}
	if reflect.ValueOf(g.Trees).Pointer() != state.treesMap || len(g.Trees) != 0 {
		t.Fatal("tree preprocessing did not restore the original tree map")
	}
	if reflect.ValueOf(g.NodeToTree).Pointer() != state.nodeToTreeMap || len(g.NodeToTree) != 0 {
		t.Fatal("tree preprocessing did not restore the original node-to-tree map")
	}
	for node, edges := range state.nodeEdges {
		edges.assertRestored(t, node.Edges, "tree preprocessing Node.Edges")
		if node.Graph != state.nodeGraphs[node] {
			t.Fatalf("node %d graph owner = %p, want restored %p", node.ID, node.Graph, state.nodeGraphs[node])
		}
		if node.Container != state.nodeParents[node] {
			t.Fatalf("node %d container = %p, want restored %p", node.ID, node.Container, state.nodeParents[node])
		}
	}
	state.route.assertRestored(t, g.Edges[0].Points, "tree preprocessing Edge.Points")
	for i, point := range g.Edges[0].Points {
		if *point != state.routeValues[i] {
			t.Fatalf("route point %d = %v, want restored %v", i, *point, state.routeValues[i])
		}
	}
}

func addTreePreprocessNode(g *layoutgraph.Graph, id int) *layoutgraph.Node {
	node := layoutgraph.NewNode(layoutgraph.EntityID(id), 10, 10)
	node.TopLeft = geo.NewPoint(float64(id*20), 0)
	g.AddNewNodeToContainer(nil, node)
	return node
}

func addTreePreprocessEdge(g *layoutgraph.Graph, from, to *layoutgraph.Node) *layoutgraph.Edge {
	edge := g.Connect(from, to)
	pointBacking := make([]*geo.Point, 4)
	pointBacking[0] = geo.NewPoint(from.TopLeft.X, from.TopLeft.Y)
	pointBacking[1] = geo.NewPoint(to.TopLeft.X, to.TopLeft.Y)
	pointBacking[2] = geo.NewPoint(-100, -100)
	pointBacking[3] = geo.NewPoint(-200, -200)
	edge.Points = pointBacking[:2]
	return edge
}

// A core cycle prevents the attached line from being treated as a wholly
// isolated tree. Its line nodes are therefore removed during extraction and
// reconnected by putBackNonBranchingTrees.
func treePreprocessReconnectGraph(lineLength int) (*layoutgraph.Graph, *layoutgraph.Node) {
	g := layoutgraph.NewGraph()
	core := []*layoutgraph.Node{
		addTreePreprocessNode(g, 1),
		addTreePreprocessNode(g, 2),
		addTreePreprocessNode(g, 3),
	}
	addTreePreprocessEdge(g, core[0], core[1])
	addTreePreprocessEdge(g, core[1], core[2])
	addTreePreprocessEdge(g, core[2], core[0])
	previous := core[0]
	var leaf *layoutgraph.Node
	for i := 0; i < lineLength; i++ {
		leaf = addTreePreprocessNode(g, 10+i)
		addTreePreprocessEdge(g, previous, leaf)
		previous = leaf
	}

	// Spare-capacity sentinels prove rollback restores both headers and backing
	// arrays after append/filter mutations.
	tailNode := layoutgraph.NewNode(9_000, 1, 1)
	nodeBacking := make([]*layoutgraph.Node, len(g.Nodes), len(g.Nodes)+3)
	copy(nodeBacking, g.Nodes)
	fullNodes := nodeBacking[:cap(nodeBacking)]
	for i := len(g.Nodes); i < len(fullNodes); i++ {
		fullNodes[i] = tailNode
	}
	g.Nodes = nodeBacking

	tailEdge := layoutgraph.NewEdge(tailNode, tailNode)
	edgeBacking := make([]*layoutgraph.Edge, len(g.Edges), len(g.Edges)+3)
	copy(edgeBacking, g.Edges)
	fullEdges := edgeBacking[:cap(edgeBacking)]
	for i := len(g.Edges); i < len(fullEdges); i++ {
		fullEdges[i] = tailEdge
	}
	g.Edges = edgeBacking

	childBacking := make([]*layoutgraph.Node, len(g.Containers[nil]), len(g.Containers[nil])+3)
	copy(childBacking, g.Containers[nil])
	fullChildren := childBacking[:cap(childBacking)]
	for i := len(g.Containers[nil]); i < len(fullChildren); i++ {
		fullChildren[i] = tailNode
	}
	g.Containers[nil] = childBacking
	return g, leaf
}

func graphContainsNode(g *layoutgraph.Graph, target *layoutgraph.Node) bool {
	return slices.Contains(g.Nodes, target)
}

type cancelAfterFringeRemoval struct {
	context.Context
	graph     *layoutgraph.Graph
	nodeCount int
	observed  bool
}

func (ctx *cancelAfterFringeRemoval) Err() error {
	if len(ctx.graph.Nodes) < ctx.nodeCount {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestPreprocessTreesCancellationAfterFringeRemovalRestoresExactTopology(t *testing.T) {
	g, _ := treePreprocessReconnectGraph(8)
	state := captureTreePreprocessState(g)
	ctx := &cancelAfterFringeRemoval{
		Context:   context.Background(),
		graph:     g,
		nodeCount: len(g.Nodes),
	}

	err := Preprocess(ctx, g)
	requireCanceledAt(t, err, treePreprocessLocation)
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe an actual fringe removal")
	}
	state.assertRestored(t, g)
}

type cancelAfterTreeReconnect struct {
	context.Context
	graph       *layoutgraph.Graph
	target      *layoutgraph.Node
	wasRemoved  bool
	reconnected bool
}

type cancelAfterTreesInstalled struct {
	context.Context
	graph    *layoutgraph.Graph
	observed bool
}

func (ctx *cancelAfterTreesInstalled) Err() error {
	if len(ctx.graph.Trees) > 0 {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestPreprocessTreesCancellationAfterTreeMapMutationRestoresExactTopology(t *testing.T) {
	g, _ := treePreprocessReconnectGraph(8)
	state := captureTreePreprocessState(g)
	ctx := &cancelAfterTreesInstalled{Context: context.Background(), graph: g}

	err := Preprocess(ctx, g)
	requireCanceledAt(t, err, treePreprocessLocation)
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe the installed tree map")
	}
	state.assertRestored(t, g)
}

func (ctx *cancelAfterTreeReconnect) Err() error {
	present := graphContainsNode(ctx.graph, ctx.target)
	if !present {
		ctx.wasRemoved = true
	}
	if ctx.wasRemoved && present {
		ctx.reconnected = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestPreprocessTreesCancellationAfterReconnectRestoresExactTopology(t *testing.T) {
	g, leaf := treePreprocessReconnectGraph(8)
	state := captureTreePreprocessState(g)
	ctx := &cancelAfterTreeReconnect{
		Context: context.Background(),
		graph:   g,
		target:  leaf,
	}

	err := Preprocess(ctx, g)
	requireCanceledAt(t, err, treePreprocessLocation)
	if !ctx.wasRemoved || !ctx.reconnected {
		t.Fatal("cancellation probe did not observe removal followed by actual reconnection")
	}
	state.assertRestored(t, g)
}

type panicAfterFringeRemoval struct {
	context.Context
	graph     *layoutgraph.Graph
	nodeCount int
	observed  bool
}

func (ctx *panicAfterFringeRemoval) Err() error {
	if len(ctx.graph.Nodes) < ctx.nodeCount {
		ctx.observed = true
		panic("deterministic tree preprocessing probe")
	}
	return ctx.Context.Err()
}

func TestPreprocessTreesPanicAfterMutationRestoresExactTopology(t *testing.T) {
	g, _ := treePreprocessReconnectGraph(8)
	state := captureTreePreprocessState(g)
	ctx := &panicAfterFringeRemoval{
		Context:   context.Background(),
		graph:     g,
		nodeCount: len(g.Nodes),
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = Preprocess(ctx, g)
	}()
	if recovered == nil || !ctx.observed {
		t.Fatal("panic probe did not run after an actual tree preprocessing mutation")
	}
	state.assertRestored(t, g)
}

type observeTreeMutation struct {
	context.Context
	graph     *layoutgraph.Graph
	nodeCount int
	observed  bool
}

func (ctx *observeTreeMutation) Err() error {
	if len(ctx.graph.Nodes) < ctx.nodeCount {
		ctx.observed = true
	}
	return ctx.Context.Err()
}

func TestPreprocessTreesInjectedWorkLimitAfterMutationIsAtomic(t *testing.T) {
	g, _ := treePreprocessReconnectGraph(80)
	snapshotWork := measureTreeSnapshotWork(t, g)
	state := captureTreePreprocessState(g)
	ctx := &observeTreeMutation{
		Context:   context.Background(),
		graph:     g,
		nodeCount: len(g.Nodes),
	}

	err := preprocessTreesWithWorkLimit(ctx, g, snapshotWork+900)
	if err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("PreprocessTrees error = %v, want injected work-limit failure", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("PreprocessTrees error = %v, want resource error rather than cancellation", err)
	}
	if !ctx.observed {
		t.Fatal("work-limit probe did not observe a mutation before rejection")
	}
	state.assertRestored(t, g)
}

func measureTreeSnapshotWork(t *testing.T, g *layoutgraph.Graph) int64 {
	t.Helper()
	guard, err := newWorkGuard(context.Background(), "TreeSnapshotMeasurement")
	if err != nil {
		t.Fatal(err)
	}
	guard.SetLimit(limits.MaxTransactionWorkUnits)
	state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
		CaptureTopology:   true,
		CaptureEdgeRoutes: true,
	})
	if err := state.UpdateWithWorkGuard(g, guard); err != nil {
		t.Fatal(err)
	}
	return guard.Used()
}

func manyBranchingPlacementTreesGraph(t *testing.T, treeCount int) (*layoutgraph.Graph, *layoutgraph.Node) {
	t.Helper()
	g := layoutgraph.NewGraph()
	core := []*layoutgraph.Node{
		addTreePreprocessNode(g, 1),
		addTreePreprocessNode(g, 2),
		addTreePreprocessNode(g, 3),
	}
	addTreePreprocessEdge(g, core[0], core[1])
	addTreePreprocessEdge(g, core[1], core[2])
	addTreePreprocessEdge(g, core[2], core[0])

	var firstLeaf *layoutgraph.Node
	for i := 0; i < treeCount; i++ {
		rootID := 1_000 + 3*i
		root := addTreePreprocessNode(g, rootID)
		first := addTreePreprocessNode(g, rootID+1)
		second := addTreePreprocessNode(g, rootID+2)
		addTreePreprocessEdge(g, core[0], root)
		addTreePreprocessEdge(g, root, first)
		addTreePreprocessEdge(g, root, second)
		if firstLeaf == nil {
			firstLeaf = first
		}
	}

	if err := Preprocess(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != len(core) {
		t.Fatalf("tree preprocessing retained %d nodes, want %d core nodes", len(g.Nodes), len(core))
	}
	if len(g.Trees[core[0]]) != treeCount {
		t.Fatalf("tree preprocessing produced %d roots, want %d", len(g.Trees[core[0]]), treeCount)
	}
	return g, firstLeaf
}

type observeAggregateTreePlacement struct {
	context.Context
	graph               *layoutgraph.Graph
	target              *layoutgraph.Node
	targetTree          *layoutgraph.Tree
	initialNodeCount    int
	originalOrientation geo.Orientation
	observedReconnect   bool
	observedPlacement   bool
	cancelOnPlacement   bool
	panicOnPlacement    bool
}

func (ctx *observeAggregateTreePlacement) Err() error {
	if len(ctx.graph.Nodes) > ctx.initialNodeCount && graphContainsNode(ctx.graph, ctx.target) {
		ctx.observedReconnect = true
		// A changed tree orientation plus a positive level offset can only occur
		// after constructToOrientation has entered the actual layoutTree kernel;
		// reconnecting the tree or zero-initializing positions is insufficient.
		if ctx.targetTree.Orientation != ctx.originalOrientation && ctx.target.TopLeft != nil && ctx.target.TopLeft.Y > 0 {
			ctx.observedPlacement = true
			if ctx.panicOnPlacement {
				panic("deterministic tree placement probe")
			}
			if ctx.cancelOnPlacement {
				return context.Canceled
			}
		}
	}
	return ctx.Context.Err()
}

func TestPlaceTreesAggregateWorkLimitRollsBackEarlierPlacement(t *testing.T) {
	g, firstLeaf := manyBranchingPlacementTreesGraph(t, 12)
	state := captureReconnectTreeState(g)
	ctx := &observeAggregateTreePlacement{
		Context:             context.Background(),
		graph:               g,
		target:              firstLeaf,
		targetTree:          g.NodeToTree[firstLeaf],
		initialNodeCount:    len(g.Nodes),
		originalOrientation: g.NodeToTree[firstLeaf].Orientation,
	}

	// The guarded discovery and first complete placement consume fewer than 5000
	// units. Because the guard is shared, a later placement crosses the
	// limit after the first tree has genuinely been reconnected and positioned.
	err := placeTreesWithWorkLimit(ctx, g, nil, 5000)
	if err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("PlaceTrees error = %v, want aggregate work-limit failure", err)
	}
	if !ctx.observedReconnect || !ctx.observedPlacement {
		t.Fatalf(
			"aggregate probe observed reconnect=%v placement=%v, want both before rejection",
			ctx.observedReconnect,
			ctx.observedPlacement,
		)
	}
	state.assertRestored(t, g)
}

func measurePlaceTreesWork(t *testing.T, treeCount int) int64 {
	t.Helper()
	g, _ := manyBranchingPlacementTreesGraph(t, treeCount)
	guard, err := newWorkGuard(context.Background(), "PlaceTreesMeasurement")
	if err != nil {
		t.Fatal(err)
	}
	guard.SetLimit(limits.MaxTransactionWorkUnits)
	state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
		CaptureTopology:   true,
		CaptureEdgeRoutes: true,
	})
	if err := state.UpdateWithWorkGuard(g, guard); err != nil {
		t.Fatal(err)
	}
	if err := placeTrees(context.Background(), g, nil, guard); err != nil {
		t.Fatal(err)
	}
	if guard.Used() == 0 {
		t.Fatal("PlaceTrees measurement did not enter a guarded kernel")
	}
	return guard.Used()
}

func TestPlaceTreesExactWorkLimitSucceeds(t *testing.T) {
	required := measurePlaceTreesWork(t, 4)
	g, _ := manyBranchingPlacementTreesGraph(t, 4)
	if err := placeTreesWithWorkLimit(context.Background(), g, nil, required); err != nil {
		t.Fatalf("PlaceTrees rejected its exact measured work limit %d: %v", required, err)
	}
}

func TestPlaceTreesOneUnitOverLimitFailsAtomically(t *testing.T) {
	required := measurePlaceTreesWork(t, 4)
	g, _ := manyBranchingPlacementTreesGraph(t, 4)
	state := captureReconnectTreeState(g)

	err := placeTreesWithWorkLimit(context.Background(), g, nil, required-1)
	if err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("PlaceTrees error = %v, want one-unit work-limit failure", err)
	}
	state.assertRestored(t, g)
}

func deepBranchingPlacementTreeGraph(t *testing.T, depth int) *layoutgraph.Graph {
	t.Helper()
	g := layoutgraph.NewGraph()
	core := []*layoutgraph.Node{
		addTreePreprocessNode(g, 1),
		addTreePreprocessNode(g, 2),
		addTreePreprocessNode(g, 3),
	}
	addTreePreprocessEdge(g, core[0], core[1])
	addTreePreprocessEdge(g, core[1], core[2])
	addTreePreprocessEdge(g, core[2], core[0])

	previous := core[0]
	for i := 0; i < depth; i++ {
		node := addTreePreprocessNode(g, 10_000+i)
		addTreePreprocessEdge(g, previous, node)
		previous = node
	}
	for i := 0; i < 2; i++ {
		leaf := addTreePreprocessNode(g, 20_000+i)
		addTreePreprocessEdge(g, previous, leaf)
	}
	if err := Preprocess(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestPlaceTreesCalibratedBudgetAcceptsRepresentativeStress(t *testing.T) {
	t.Run("wide-48", func(t *testing.T) {
		g, _ := manyBranchingPlacementTreesGraph(t, 48)
		if err := Place(context.Background(), g, nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("wide-64", func(t *testing.T) {
		g, _ := manyBranchingPlacementTreesGraph(t, 64)
		if err := Place(context.Background(), g, nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("deep-240", func(t *testing.T) {
		g := deepBranchingPlacementTreeGraph(t, 240)
		if err := Place(context.Background(), g, nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPlaceTreesCalibratedBudgetBoundsMeasuredStress(t *testing.T) {
	const (
		measuredWide64Work = int64(22_133_252)
		measuredWide96Work = int64(69_067_924)
	)
	if limits.MaxPlaceTreesWorkUnits < 3*measuredWide64Work {
		t.Fatalf("PlaceTrees budget %d is less than three times representative work %d", limits.MaxPlaceTreesWorkUnits, measuredWide64Work)
	}
	if limits.MaxPlaceTreesWorkUnits >= measuredWide96Work {
		t.Fatalf("PlaceTrees budget %d does not bound measured hostile work %d", limits.MaxPlaceTreesWorkUnits, measuredWide96Work)
	}
}

func TestPlaceTreesRepresentativeHostileLimitFailureIsAtomic(t *testing.T) {
	g, firstLeaf := manyBranchingPlacementTreesGraph(t, 32)
	state := captureReconnectTreeState(g)
	ctx := &observeAggregateTreePlacement{
		Context:             context.Background(),
		graph:               g,
		target:              firstLeaf,
		targetTree:          g.NodeToTree[firstLeaf],
		initialNodeCount:    len(g.Nodes),
		originalOrientation: g.NodeToTree[firstLeaf].Orientation,
	}

	err := placeTreesWithWorkLimit(ctx, g, nil, 1_000_000)
	if err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("PlaceTrees error = %v, want representative hostile work-limit failure", err)
	}
	if !ctx.observedReconnect || !ctx.observedPlacement {
		t.Fatalf(
			"hostile probe observed reconnect=%v placement=%v, want both before rejection",
			ctx.observedReconnect,
			ctx.observedPlacement,
		)
	}
	state.assertRestored(t, g)
}

func TestPlaceTreesCancellationInsideLayoutKernelRestoresExactWholeCall(t *testing.T) {
	g, firstLeaf := manyBranchingPlacementTreesGraph(t, 4)
	state := captureReconnectTreeState(g)
	ctx := &observeAggregateTreePlacement{
		Context:             context.Background(),
		graph:               g,
		target:              firstLeaf,
		targetTree:          g.NodeToTree[firstLeaf],
		initialNodeCount:    len(g.Nodes),
		originalOrientation: g.NodeToTree[firstLeaf].Orientation,
		cancelOnPlacement:   true,
	}

	err := Place(ctx, g, nil)
	requireCanceledAt(t, err, "PlaceTrees")
	if !ctx.observedReconnect || !ctx.observedPlacement {
		t.Fatalf(
			"mid-kernel probe observed reconnect=%v placement=%v, want both before cancellation",
			ctx.observedReconnect,
			ctx.observedPlacement,
		)
	}
	state.assertRestored(t, g)
}

func TestPlaceTreesPanicInsideLayoutKernelRestoresExactWholeCall(t *testing.T) {
	g, firstLeaf := manyBranchingPlacementTreesGraph(t, 4)
	state := captureReconnectTreeState(g)
	ctx := &observeAggregateTreePlacement{
		Context:             context.Background(),
		graph:               g,
		target:              firstLeaf,
		targetTree:          g.NodeToTree[firstLeaf],
		initialNodeCount:    len(g.Nodes),
		originalOrientation: g.NodeToTree[firstLeaf].Orientation,
		panicOnPlacement:    true,
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = Place(ctx, g, nil)
	}()
	if recovered == nil || !ctx.observedReconnect || !ctx.observedPlacement {
		t.Fatalf(
			"mid-kernel panic probe recovered=%v reconnect=%v placement=%v, want all evidence",
			recovered,
			ctx.observedReconnect,
			ctx.observedPlacement,
		)
	}
	state.assertRestored(t, g)
}

type cancelAfterTreeLabelMutation struct {
	context.Context
	edge               *layoutgraph.Edge
	originalPercentage float64
	observed           bool
}

func (ctx *cancelAfterTreeLabelMutation) Err() error {
	if ctx.edge.LabelPercentage != ctx.originalPercentage {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestPlaceTreesCancellationInsideLabelKernelRestoresExactWholeCall(t *testing.T) {
	g, firstLeaf := manyBranchingPlacementTreesGraph(t, 1)
	targetEdge := g.NodeToTree[firstLeaf].SentinelEdge
	targetEdge.Label = &layoutgraph.Label{}
	targetEdge.MinHeight = 12
	state := captureReconnectTreeState(g)
	ctx := &cancelAfterTreeLabelMutation{
		Context:            context.Background(),
		edge:               targetEdge,
		originalPercentage: targetEdge.LabelPercentage,
	}

	err := Place(ctx, g, nil)
	requireCanceledAt(t, err, "PlaceTrees")
	if !ctx.observed {
		t.Fatal("label-kernel probe did not observe an actual label mutation")
	}
	state.assertRestored(t, g)
}

func TestPlaceTreesErrorAfterPartialReconnectRestoresExactTopology(t *testing.T) {
	g, firstLeaf := manyBranchingPlacementTreesGraph(t, 12)
	var laterRoot *layoutgraph.Tree
	for _, roots := range g.Trees {
		if len(roots) > 1 {
			laterRoot = roots[len(roots)-1]
			break
		}
	}
	if laterRoot == nil || len(laterRoot.Children) < 2 {
		t.Fatal("placement-tree graph did not produce a later branching root")
	}
	laterRoot.Children[len(laterRoot.Children)-1].SentinelEdge = nil
	state := captureReconnectTreeState(g)
	ctx := &observeAggregateTreePlacement{
		Context:             context.Background(),
		graph:               g,
		target:              firstLeaf,
		targetTree:          g.NodeToTree[firstLeaf],
		initialNodeCount:    len(g.Nodes),
		originalOrientation: g.NodeToTree[firstLeaf].Orientation,
	}

	err := Place(ctx, g, nil)
	if err == nil || !strings.Contains(err.Error(), "no complete sentinel edge") {
		t.Fatalf("PlaceTrees error = %v, want incomplete sentinel edge", err)
	}
	if !ctx.observedReconnect {
		t.Fatal("error probe did not observe an actual tree reconnection")
	}
	state.assertRestored(t, g)
}
