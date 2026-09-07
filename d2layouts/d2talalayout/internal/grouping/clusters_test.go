package grouping

import (
	"context"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/log"
)

func clusterHasNode(cluster *layoutgraph.Cluster, node *layoutgraph.Node) bool {
	return slices.Contains(cluster.Nodes, node)
}

func TestAddClusters(t *testing.T) {
	ctx := log.With(context.Background(), testlog.New(t))
	g := layoutgraph.NewGraph()

	// Test basic shared adjacent nodes
	a := layoutgraph.NewNode(1, 5, 5)
	a.TopLeft = geo.NewPoint(0, 100)
	b := layoutgraph.NewNode(2, 5, 5)
	b.TopLeft = geo.NewPoint(80, 80)
	c := layoutgraph.NewNode(3, 5, 5)
	c.TopLeft = geo.NewPoint(80, 120)
	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)
	g.Connect(a, b)
	g.Connect(a, c)
	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {a, b, c},
	}

	randGenerator := rand.New(rand.NewSource(1))
	err := AddClusters(ctx, g, 1, randGenerator)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(g.Clusters) != 1 {
		t.Fatalf("Expected %v clusters, found %v", 1, len(g.Clusters))
	}
	for _, cluster := range g.Clusters {
		if len(cluster.Nodes) != 2 {
			t.Fatalf("Expected %v nodes in cluster, found %v", 2, len(cluster.Nodes))
		}
		for _, node := range []*layoutgraph.Node{b, c} {
			if !clusterHasNode(cluster, node) {
				t.Fatalf("Expected node to exist in cluster")
			}
		}
	}
}

func TestAddClustersAvoidsOrdinaryNodeIDCollision(t *testing.T) {
	ctx := log.With(context.Background(), testlog.New(t))
	const seed int64 = 19
	candidate := rand.New(rand.NewSource(seed)).Int63()

	g := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(candidate, 5, 5)
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

	if err := AddClusters(ctx, g, 1, rand.New(rand.NewSource(seed))); err != nil {
		t.Fatal(err)
	}
	if len(g.Clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(g.Clusters))
	}
	for vessel := range g.Clusters {
		if vessel.ID == candidate {
			t.Fatalf("cluster vessel ID %d collides with ordinary node", vessel.ID)
		}
	}
}

// this test checks that nodes forming a cluster won't be identified as such if they are in a hierarchy
func TestAddClustersHierarchy(t *testing.T) {
	ctx := log.With(context.Background(), testlog.New(t))
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 5, 5)
	a.TopLeft = geo.NewPoint(0, 100)
	b := layoutgraph.NewNode(2, 5, 5)
	b.TopLeft = geo.NewPoint(80, 80)
	c := layoutgraph.NewNode(3, 5, 5)
	c.TopLeft = geo.NewPoint(80, 120)
	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)
	g.Connect(a, b)
	g.Connect(a, c)
	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {a, b, c},
	}
	hierarchy := layoutgraph.NewHierarchy()
	for i, node := range g.Nodes {
		node.Hierarchy = hierarchy
		hierarchy.Levels()[node] = i
	}

	randGenerator := rand.New(rand.NewSource(1))
	err := AddClusters(ctx, g, 1, randGenerator)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(g.Clusters) != 0 {
		t.Fatalf("Expected no clusters")
	}
}

func TestClusterOverlap(t *testing.T) {
	ctx := log.With(context.Background(), testlog.New(t))
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 5, 5)
	a.TopLeft = geo.NewPoint(0, 100)
	b := layoutgraph.NewNode(2, 5, 5)
	b.TopLeft = geo.NewPoint(80, 80)
	c := layoutgraph.NewNode(3, 5, 5)
	c.TopLeft = geo.NewPoint(80, 120)
	// d is close to the bc cluster, but it should still be able to form
	d := layoutgraph.NewNode(4, 5, 5)
	d.TopLeft = geo.NewPoint(85, 125)
	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)
	g.AddNode(d)
	g.Connect(a, b)
	g.Connect(a, c)
	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {a, b, c},
	}

	randGenerator := rand.New(rand.NewSource(1))
	err := AddClusters(ctx, g, 1, randGenerator)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(g.Clusters) != 1 {
		t.Fatalf("Expected %v clusters, found %v", 1, len(g.Clusters))
	}
}

// Having more than one shared adjacent node disqualifies it from being a cluster
func TestMultipleSharedAdjacent(t *testing.T) {
	ctx := log.With(context.Background(), testlog.New(t))
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 5, 5)
	a.TopLeft = geo.NewPoint(0, 100)
	b := layoutgraph.NewNode(2, 5, 5)
	b.TopLeft = geo.NewPoint(80, 80)
	c := layoutgraph.NewNode(3, 5, 5)
	c.TopLeft = geo.NewPoint(80, 120)
	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)
	g.Connect(a, b)
	g.Connect(a, c)

	d := layoutgraph.NewNode(4, 5, 5)
	d.TopLeft = geo.NewPoint(200, 200)
	g.AddNode(d)
	g.Connect(b, d)

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {a, b, c, d},
	}

	randGenerator := rand.New(rand.NewSource(1))
	err := AddClusters(ctx, g, 1, randGenerator)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(g.Clusters) != 0 {
		t.Fatalf("Expected %v clusters, found %v", 0, len(g.Clusters))
	}
}

// Having more than one shared node counts as cluster
func TestConsistentMultipleSharedAdjacent(t *testing.T) {
	ctx := log.With(context.Background(), testlog.New(t))
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 5, 5)
	a.TopLeft = geo.NewPoint(0, 100)
	b := layoutgraph.NewNode(2, 5, 5)
	b.TopLeft = geo.NewPoint(80, 80)
	c := layoutgraph.NewNode(3, 5, 5)
	c.TopLeft = geo.NewPoint(80, 120)
	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)
	g.Connect(a, b)
	g.Connect(a, c)

	d := layoutgraph.NewNode(4, 5, 5)
	d.TopLeft = geo.NewPoint(200, 200)
	g.AddNode(d)
	g.Connect(b, d)
	g.Connect(c, d)

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {a, b, c, d},
	}

	randGenerator := rand.New(rand.NewSource(1))
	err := AddClusters(ctx, g, 1, randGenerator)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(g.Clusters) != 1 {
		t.Fatalf("Expected %v clusters, found %v", 0, len(g.Clusters))
	}
}

func TestJointDistancedClustersWithNears(t *testing.T) {
	/* In this case, 10 and 7 are not considered seed nodes because their edge length
	is less than the graph threshold. However, they are not near any of the other nodes as well
	so the case fails because there are 9 nodes to be clustered, but only 7 were processed.
	This is a valid case because, even though we're dealing with subgraphs, they might be
	connected by an edge through a node outside their container.
	In this example, nodes 5 and 7 share and edge with node 13 (outside the container and not in the example).
	This makes nodes 5 and 7 near each other and then they are considered part of the same subgraph during split.
	It used to panic: Clustered nodes: 7. Total: 9
	*/
	//                                           ┌────────┐
	//                                           │        │
	//                                           │   8    │
	//                                           │        │
	//                                           └───┼────┘
	//                             ┌──────┐          │
	//                             │      │          │                 ┌─────┐
	//                             │  6   │          │                 │ 10  │
	//                             └───┼──┘          │                 │     │
	//                                 │             │                 └──┼──┘
	// ┌───┐                           │             │                    │
	// │   │         ┌─────┐           │          ┌──┼────┐               │
	// │ 9 ┼─────────┼  3  ┼───────────┼──────────┼       │               │
	// └───┘         │     ┼────────┐  │          │  4    │               │
	//               └─────┘       ┌┼──┼──┐       └───────┘               │
	//                             │  11  │                               │
	//                             │      │                            ┌──┼──┐
	//                             └──┼───┘                            │     │
	//                                │                                │  7  │
	//                                │                                └─────┘
	//                                │
	//                                │
	//                                │
	//                                │
	//                             ┌──┼──┐
	//                             │     │
	//                             │  5  │
	//                             │     │
	//                             └─────┘
	g := layoutgraph.NewGraph()
	g.CellSize = 58
	n3 := g.AddNode(layoutgraph.NewNode(3, 50.0, 57.0))
	n3.TopLeft = geo.NewPoint(58.0, 116.0)
	n9 := g.AddNode(layoutgraph.NewNode(9, 56.0, 45.0))
	n9.TopLeft = geo.NewPoint(-58.0, 116.0)
	n11 := g.AddNode(layoutgraph.NewNode(11, 52.0, 55.0))
	n11.TopLeft = geo.NewPoint(174.0, 174.0)
	n4 := g.AddNode(layoutgraph.NewNode(4, 58.0, 49.0))
	n4.TopLeft = geo.NewPoint(290.0, 116.0)
	n5 := g.AddNode(layoutgraph.NewNode(5, 54.0, 50.0))
	n5.TopLeft = geo.NewPoint(174.0, 348.0)
	n6 := g.AddNode(layoutgraph.NewNode(6, 53.0, 44.0))
	n6.TopLeft = geo.NewPoint(174.0, 58.0)
	n8 := g.AddNode(layoutgraph.NewNode(8, 44.0, 54.0))
	n8.TopLeft = geo.NewPoint(290.0, -58.0)
	n7 := g.AddNode(layoutgraph.NewNode(7, 57.0, 49.0))
	n7.TopLeft = geo.NewPoint(522.0, 174.0)
	n10 := g.AddNode(layoutgraph.NewNode(10, 47.0, 56.0))
	n10.TopLeft = geo.NewPoint(522.0, 0.0)
	g.Connect(n5, n11)
	g.Connect(n9, n3)
	g.Connect(n3, n11)
	g.Connect(n11, n6)
	g.Connect(n3, n4)
	g.Connect(n4, n8)
	g.Connect(n10, n7)

	n5.AddNear(n7)
	n7.AddNear(n5)

	// this is the call in g.joinDistancedClusters
	clusters := layoutgraph.Nodes(g.Nodes).DistanceClusters(3 * g.CellSize)
	assert.Equal(t, 2, len(clusters))

	assert.Equal(t, 7, len(clusters[0]))
	assert.Equal(t, layoutgraph.EntityID(3), clusters[0][0].ID)
	assert.Equal(t, layoutgraph.EntityID(4), clusters[0][1].ID)
	assert.Equal(t, layoutgraph.EntityID(5), clusters[0][2].ID)
	assert.Equal(t, layoutgraph.EntityID(6), clusters[0][3].ID)
	assert.Equal(t, layoutgraph.EntityID(8), clusters[0][4].ID)
	assert.Equal(t, layoutgraph.EntityID(9), clusters[0][5].ID)
	assert.Equal(t, layoutgraph.EntityID(11), clusters[0][6].ID)

	assert.Equal(t, 2, len(clusters[1]))
	assert.Equal(t, layoutgraph.EntityID(7), clusters[1][0].ID)
	assert.Equal(t, layoutgraph.EntityID(10), clusters[1][1].ID)

	guard, err := limits.NewWorkGuard(context.Background(), "DistanceClustersParity", limits.MaxTransactionWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	guardedClusters, err := layoutgraph.Nodes(g.Nodes).DistanceClustersWithWorkGuard(3*g.CellSize, guard)
	if err != nil {
		t.Fatal(err)
	}
	if len(guardedClusters) != len(clusters) {
		t.Fatalf("guarded cluster count = %d, want %d", len(guardedClusters), len(clusters))
	}
	for index := range clusters {
		if !slices.Equal(guardedClusters[index], clusters[index]) {
			t.Fatalf("guarded cluster %d = %v, want exact order %v", index, guardedClusters[index], clusters[index])
		}
	}
	if guard.Used() == 0 {
		t.Fatal("guarded distance clustering consumed no work")
	}
}

func distanceClustersTreeNearGraph(depth int) *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	graph.CellSize = 10
	source := layoutgraph.NewNode(1, 10, 10)
	source.TopLeft = geo.NewPoint(0, 0)
	treeNode := layoutgraph.NewNode(2, 10, 10)
	treeNode.TopLeft = geo.NewPoint(980, 0)
	sentinel := layoutgraph.NewNode(3, 10, 10)
	sentinel.TopLeft = geo.NewPoint(1000, 0)
	for _, node := range []*layoutgraph.Node{source, treeNode, sentinel} {
		graph.AddNode(node)
	}

	root := layoutgraph.NewTree(treeNode)
	root.SentinelEdge = layoutgraph.NewEdge(treeNode, sentinel)
	leaf := root
	for range depth {
		child := layoutgraph.NewTree(treeNode)
		child.Parent = leaf
		leaf.Children = append(leaf.Children, child)
		leaf = child
	}
	graph.NodeToTree = map[*layoutgraph.Node]*layoutgraph.Tree{treeNode: leaf}
	source.AddNear(treeNode)
	return graph
}

func distanceClustersTreeNearWork(t *testing.T, depth int) int64 {
	t.Helper()
	graph := distanceClustersTreeNearGraph(depth)
	guard, err := limits.NewWorkGuard(context.Background(), "DistanceClustersTreeNear", limits.MaxTransactionWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	clusters, err := layoutgraph.Nodes(graph.Nodes).DistanceClustersWithWorkGuard(3*graph.CellSize, guard)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) == 0 {
		t.Fatal("guarded tree Near discovery returned no clusters")
	}
	return guard.Used()
}

func TestDistanceClustersChargesNearTreeDepth(t *testing.T) {
	const depth = 128
	shallowWork := distanceClustersTreeNearWork(t, 0)
	deepWork := distanceClustersTreeNearWork(t, depth)
	if got := deepWork - shallowWork; got != depth {
		t.Fatalf("deep tree Near work delta = %d, want one unit per parent hop %d", got, depth)
	}

	graph := distanceClustersTreeNearGraph(depth)
	guard, err := limits.NewWorkGuard(context.Background(), "DistanceClustersTreeNear", deepWork-1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = layoutgraph.Nodes(graph.Nodes).DistanceClustersWithWorkGuard(3*graph.CellSize, guard)
	if err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("deep tree Near error = %v, want exact work-limit rejection", err)
	}
	if guard.Used() != deepWork {
		t.Fatalf("deep tree Near rejected work = %d, want %d", guard.Used(), deepWork)
	}
}

func TestClustersDFSOrder(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container1 := layoutgraph.NewNode(1, 100, 100)
	container2 := layoutgraph.NewNode(2, 100, 100)
	container3 := layoutgraph.NewNode(3, 100, 100)
	container4 := layoutgraph.NewNode(4, 100, 100)
	a := layoutgraph.NewNode(5, 100, 100)
	b := layoutgraph.NewNode(6, 100, 100)
	c := layoutgraph.NewNode(7, 100, 100)
	d := layoutgraph.NewNode(8, 100, 100)
	e := layoutgraph.NewNode(9, 100, 100)
	f := layoutgraph.NewNode(10, 100, 100)
	g := layoutgraph.NewNode(11, 100, 100)
	for _, n := range []*layoutgraph.Node{a, b, c, d, e, f, g, container1, container2, container3, container4} {
		graph.AddNodeUnchecked(n)
	}

	graph.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil:        {container1, a},
		container1: {container2, b},
		container2: {container3, c},
		container3: {container4, d},
		// Note: this order is preserved in the result
		container4: {e, g, f},
	}
	for container, children := range graph.Containers {
		for _, child := range children {
			child.Container = container
		}
		if container != nil {
			container.SetContainer(true)
		}
	}
	graph.Clusters = map[*layoutgraph.Node]*layoutgraph.Cluster{
		a: {Vessel: a},
		b: {Vessel: b},
		c: {Vessel: c},
		d: {Vessel: d},
		e: {Vessel: e},
		f: {Vessel: f},
		g: {Vessel: g},
	}
	for c := range graph.Clusters {
		c.SetClusterVessel(true)
	}

	expected := []*layoutgraph.Node{e, g, f, d, c, b, a}
	order := g.Graph.ClusterRDFSOrder()

	assert.Equal(t, len(expected), len(order))
	for i, vessel := range order {
		assert.Equal(t, expected[i].ID, vessel.ID)
	}
}
