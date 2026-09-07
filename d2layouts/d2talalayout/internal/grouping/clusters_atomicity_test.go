package grouping

import (
	"context"
	"errors"
	"maps"
	"math/rand"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type addClustersAtomicState struct {
	nodes          exactTestSlice[*layoutgraph.Node]
	edges          exactTestSlice[*layoutgraph.Edge]
	containersMap  uintptr
	containers     map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Node]
	nodeEdges      map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Edge]
	nodeGraphs     map[*layoutgraph.Node]*layoutgraph.Graph
	nodeContainers map[*layoutgraph.Node]*layoutgraph.Node
	nodeBoxes      map[*layoutgraph.Node]geo.Box
	nodeTopLeft    map[*layoutgraph.Node]*geo.Point
	topLeftValues  map[*layoutgraph.Node]geo.Point
	clustersMap    uintptr
	clusters       map[*layoutgraph.Node]*layoutgraph.Cluster
	nodeClusters   map[*layoutgraph.Node]*layoutgraph.Cluster
	clusterVessels map[*layoutgraph.Node]bool
	edgeEndpoints  map[*layoutgraph.Edge][2]*layoutgraph.Node
}

func captureAddClustersAtomicState(g *layoutgraph.Graph) addClustersAtomicState {
	state := addClustersAtomicState{
		nodes:          captureExactTestSlice(g.Nodes),
		edges:          captureExactTestSlice(g.Edges),
		containersMap:  reflect.ValueOf(g.Containers).Pointer(),
		containers:     make(map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Node], len(g.Containers)),
		nodeEdges:      make(map[*layoutgraph.Node]exactTestSlice[*layoutgraph.Edge], len(g.Nodes)),
		nodeGraphs:     make(map[*layoutgraph.Node]*layoutgraph.Graph, len(g.Nodes)),
		nodeContainers: make(map[*layoutgraph.Node]*layoutgraph.Node, len(g.Nodes)),
		nodeBoxes:      make(map[*layoutgraph.Node]geo.Box, len(g.Nodes)),
		nodeTopLeft:    make(map[*layoutgraph.Node]*geo.Point, len(g.Nodes)),
		topLeftValues:  make(map[*layoutgraph.Node]geo.Point, len(g.Nodes)),
		clustersMap:    reflect.ValueOf(g.Clusters).Pointer(),
		clusters:       make(map[*layoutgraph.Node]*layoutgraph.Cluster, len(g.Clusters)),
		nodeClusters:   make(map[*layoutgraph.Node]*layoutgraph.Cluster),
		clusterVessels: make(map[*layoutgraph.Node]bool),
		edgeEndpoints:  make(map[*layoutgraph.Edge][2]*layoutgraph.Node, len(g.Edges)),
	}
	maps.Copy(state.clusters, g.Clusters)
	for container, children := range g.Containers {
		state.containers[container] = captureExactTestSlice(children)
	}
	for _, node := range g.Nodes {
		state.nodeClusters[node] = node.Cluster
		state.clusterVessels[node] = node.IsClusterVessel()
		state.nodeEdges[node] = captureExactTestSlice(node.Edges)
		state.nodeGraphs[node] = node.Graph
		state.nodeContainers[node] = node.Container
		state.nodeBoxes[node] = node.Box
		state.nodeTopLeft[node] = node.TopLeft
		if node.TopLeft != nil {
			state.topLeftValues[node] = *node.TopLeft
		}
	}
	for _, edge := range g.Edges {
		state.edgeEndpoints[edge] = [2]*layoutgraph.Node{edge.From, edge.To}
	}
	return state
}

func (state addClustersAtomicState) assertRestored(t *testing.T, g *layoutgraph.Graph) {
	t.Helper()
	state.nodes.assertRestored(t, g.Nodes, "AddClusters Graph.Nodes")
	state.edges.assertRestored(t, g.Edges, "AddClusters Graph.Edges")
	if reflect.ValueOf(g.Containers).Pointer() != state.containersMap || len(g.Containers) != len(state.containers) {
		t.Fatal("AddClusters changed the container map")
	}
	for container, children := range state.containers {
		children.assertRestored(t, g.Containers[container], "AddClusters Graph.Containers entry")
	}
	if reflect.ValueOf(g.Clusters).Pointer() != state.clustersMap || len(g.Clusters) != len(state.clusters) {
		t.Fatal("AddClusters did not restore the exact cluster map")
	}
	for vessel, cluster := range state.clusters {
		if g.Clusters[vessel] != cluster {
			t.Fatalf("AddClusters changed cluster map entry for vessel %d", vessel.ID)
		}
	}
	for node, cluster := range state.nodeClusters {
		if node.Cluster != cluster || node.IsClusterVessel() != state.clusterVessels[node] {
			t.Fatalf("AddClusters changed cluster ownership for node %d", node.ID)
		}
		state.nodeEdges[node].assertRestored(t, node.Edges, "AddClusters Node.Edges")
		if node.Graph != state.nodeGraphs[node] || node.Container != state.nodeContainers[node] || node.Box != state.nodeBoxes[node] || node.TopLeft != state.nodeTopLeft[node] {
			t.Fatalf("AddClusters changed exact node state for node %d", node.ID)
		}
		if node.TopLeft != nil && *node.TopLeft != state.topLeftValues[node] {
			t.Fatalf("AddClusters changed position for node %d", node.ID)
		}
	}
	for edge, endpoints := range state.edgeEndpoints {
		if edge.From != endpoints[0] || edge.To != endpoints[1] {
			t.Fatalf("AddClusters changed endpoints for edge %d", edge.ID)
		}
	}
}

func addClustersLateFailureGraph() *layoutgraph.Graph {
	g := layoutgraph.NewGraph()
	positions := []geo.Point{
		{X: 0, Y: 100}, {X: 80, Y: 80}, {X: 80, Y: 120},
		{X: 1_000, Y: 100}, {X: 1_000, Y: 100}, {X: 1_000, Y: 100},
	}
	nodes := make([]*layoutgraph.Node, len(positions))
	for index, position := range positions {
		node := layoutgraph.NewNode(layoutgraph.EntityID(index+1), 5, 5)
		node.TopLeft = geo.NewPoint(position.X, position.Y)
		g.AddNewNodeToContainer(nil, node)
		nodes[index] = node
	}
	g.Connect(nodes[0], nodes[1])
	g.Connect(nodes[0], nodes[2])
	g.Connect(nodes[3], nodes[4])
	g.Connect(nodes[3], nodes[5])

	tailNode := layoutgraph.NewNode(99, 1, 1)
	tailEdge := layoutgraph.NewEdge(tailNode, tailNode)
	nodeBacking := make([]*layoutgraph.Node, len(g.Nodes)+3)
	copy(nodeBacking, g.Nodes)
	for index := len(g.Nodes); index < len(nodeBacking); index++ {
		nodeBacking[index] = tailNode
	}
	g.Nodes = nodeBacking[:len(g.Nodes)]
	containerBacking := make([]*layoutgraph.Node, len(g.Containers[nil])+3)
	copy(containerBacking, g.Containers[nil])
	for index := len(g.Containers[nil]); index < len(containerBacking); index++ {
		containerBacking[index] = tailNode
	}
	g.Containers[nil] = containerBacking[:len(g.Containers[nil])]
	edgeBacking := make([]*layoutgraph.Edge, len(g.Edges)+3)
	copy(edgeBacking, g.Edges)
	for index := len(g.Edges); index < len(edgeBacking); index++ {
		edgeBacking[index] = tailEdge
	}
	g.Edges = edgeBacking[:len(g.Edges)]
	for _, node := range nodes {
		backing := make([]*layoutgraph.Edge, len(node.Edges)+2)
		copy(backing, node.Edges)
		for index := len(node.Edges); index < len(backing); index++ {
			backing[index] = tailEdge
		}
		node.Edges = backing[:len(node.Edges)]
	}
	return g
}

type observeClusterMutationContext struct {
	context.Context
	graph    *layoutgraph.Graph
	observed bool
	cancel   bool
	panic    bool
}

func (ctx *observeClusterMutationContext) Err() error {
	if len(ctx.graph.Clusters) > 0 {
		ctx.observed = true
		if ctx.panic {
			panic("AddClusters post-mutation probe")
		}
		if ctx.cancel {
			return context.Canceled
		}
	}
	return ctx.Context.Err()
}

func TestAddClustersCancellationAfterMutationRestoresExactWholeStageState(t *testing.T) {
	g := addClustersLateFailureGraph()
	state := captureAddClustersAtomicState(g)
	ctx := &observeClusterMutationContext{Context: context.Background(), graph: g, cancel: true}
	err := AddClusters(ctx, g, 1, rand.New(rand.NewSource(1)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AddClusters error = %v, want context cancellation", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe a cluster mutation")
	}
	state.assertRestored(t, g)
}

func TestAddClustersPanicAfterMutationRestoresExactWholeStageState(t *testing.T) {
	g := addClustersLateFailureGraph()
	state := captureAddClustersAtomicState(g)
	ctx := &observeClusterMutationContext{Context: context.Background(), graph: g, panic: true}
	defer func() {
		if recovered := recover(); recovered != "AddClusters post-mutation probe" {
			t.Fatalf("panic = %v, want post-mutation probe", recovered)
		}
		state.assertRestored(t, g)
	}()
	_ = AddClusters(ctx, g, 1, rand.New(rand.NewSource(1)))
}

func TestAddClustersAggregateWorkFailureAfterMutationRestoresExactState(t *testing.T) {
	// Find the exact request budget required by the same graph, then verify that
	// the immediately smaller aggregate budget leaves no tentative cluster
	// mutation behind. Cancellation/panic tests above independently prove the
	// post-publication rollback path.
	minimum := int64(1)
	for {
		graph := addClustersLateFailureGraph()
		guard, err := limits.NewWorkGuard(context.Background(), "AddClustersTransactions", minimum)
		if err != nil {
			t.Fatal(err)
		}
		ctx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)
		err = AddClusters(ctx, graph, 1, rand.New(rand.NewSource(1)))
		if err == nil {
			break
		}
		minimum *= 2
		if minimum > limits.MaxTransactionWorkUnits {
			t.Fatal("could not bound AddClusters request work")
		}
	}
	low, high := minimum/2, minimum
	for low+1 < high {
		mid := low + (high-low)/2
		graph := addClustersLateFailureGraph()
		guard, err := limits.NewWorkGuard(context.Background(), "AddClustersTransactions", mid)
		if err != nil {
			t.Fatal(err)
		}
		ctx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)
		err = AddClusters(ctx, graph, 1, rand.New(rand.NewSource(1)))
		if err == nil {
			high = mid
		} else {
			low = mid
		}
	}

	graph := addClustersLateFailureGraph()
	state := captureAddClustersAtomicState(graph)
	guard, err := limits.NewWorkGuard(context.Background(), "AddClustersTransactions", high-1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), guard)
	err = AddClusters(ctx, graph, 1, rand.New(rand.NewSource(1)))
	if err == nil {
		t.Fatal("AddClusters unexpectedly succeeded below its minimum request budget")
	}
	state.assertRestored(t, graph)
}

func TestAddClustersPreservesExternalRandomConsumption(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(1, 5, 5)
	b := layoutgraph.NewNode(2, 5, 5)
	c := layoutgraph.NewNode(3, 5, 5)
	a.TopLeft = geo.NewPoint(0, 100)
	b.TopLeft = geo.NewPoint(80, 80)
	c.TopLeft = geo.NewPoint(80, 120)
	for _, node := range []*layoutgraph.Node{a, b, c} {
		g.AddNewNodeToContainer(nil, node)
	}
	g.Connect(a, b)
	g.Connect(a, c)

	const seed = int64(19)
	random := rand.New(rand.NewSource(seed))
	if err := AddClusters(context.Background(), g, 1, random); err != nil {
		t.Fatal(err)
	}
	reference := rand.New(rand.NewSource(seed))
	_ = reference.Int63() // one candidate vessel, matching the production traversal
	if got, want := random.Int63(), reference.Int63(); got != want {
		t.Fatalf("next random value = %d, want %d after one vessel draw", got, want)
	}
}

func TestAddClustersCanceledBeforeWorkPreservesExistingOwnership(t *testing.T) {
	graph := layoutgraph.NewGraph()
	vessel := layoutgraph.NewNode(1, 10, 10)
	cluster := &layoutgraph.Cluster{Vessel: vessel, Graph: graph}
	graph.Clusters[vessel] = cluster
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := AddClusters(ctx, graph, 1, rand.New(rand.NewSource(1)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AddClusters error = %v, want context.Canceled", err)
	}
	if graph.Clusters[vessel] != cluster {
		t.Fatal("AddClusters mutated cluster ownership after preflight cancellation")
	}
}
