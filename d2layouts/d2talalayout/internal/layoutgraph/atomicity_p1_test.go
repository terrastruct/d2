package layoutgraph

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/d2lang/d2/lib/geo"
)

type exactTestSlice[T comparable] struct {
	header  []T
	backing []T
}

func captureExactTestSlice[T comparable](values []T) exactTestSlice[T] {
	return exactTestSlice[T]{header: values, backing: slices.Clone(values[:cap(values)])}
}

func (snapshot exactTestSlice[T]) assertRestored(t *testing.T, got []T, name string) {
	t.Helper()
	if len(got) != len(snapshot.header) || cap(got) != cap(snapshot.header) {
		t.Fatalf("%s header = len %d cap %d; want len %d cap %d", name, len(got), cap(got), len(snapshot.header), cap(snapshot.header))
	}
	if cap(got) > 0 && &got[:cap(got)][0] != &snapshot.header[:cap(snapshot.header)][0] {
		t.Fatalf("%s backing array identity changed", name)
	}
	if !slices.Equal(got[:cap(got)], snapshot.backing) {
		t.Fatalf("%s backing array contents changed", name)
	}
}

func TestTopologyRollbackRestoresEveryExactSliceBacking(t *testing.T) {
	g := NewGraph()
	node := NewNode(1, 10, 10)
	node.TopLeft = geo.NewPoint(0, 0)
	g.AddNodeUnchecked(node)
	edge := g.Connect(node, node)
	pointTail := geo.NewPoint(99, 99)
	pointBacking := make([]*geo.Point, 4)
	pointBacking[0] = geo.NewPoint(0, 0)
	for i := 1; i < len(pointBacking); i++ {
		pointBacking[i] = pointTail
	}
	edge.Points = pointBacking[:1]

	tailNode := NewNode(90, 1, 1)
	tailEdge := NewEdge(tailNode, tailNode)
	tailAbduction := &EdgeAbduction{Edge: tailEdge}

	clusterVessel := NewNode(2, 1, 1)
	clusterNodes := []*Node{node, tailNode, tailNode}[:1]
	clusterAbductions := []*EdgeAbduction{{Edge: edge}, tailAbduction, tailAbduction}[:1]
	cluster := &Cluster{Vessel: clusterVessel, Nodes: clusterNodes, EdgeAbductions: clusterAbductions, Graph: g}
	g.Clusters[clusterVessel] = cluster

	sequenceVessel := NewNode(3, 1, 1)
	sequenceNodes := []*Node{node, tailNode, tailNode}[:1]
	sequenceAbductions := []*EdgeAbduction{{Edge: edge}, tailAbduction, tailAbduction}[:1]
	sequence := &Sequence{Vessel: sequenceVessel, Nodes: sequenceNodes, EdgeAbductions: sequenceAbductions, Graph: g}
	g.Sequences[sequenceVessel] = sequence

	childTree := &Tree{Node: tailNode}
	treeChildren := []*Tree{childTree, childTree, childTree}[:1]
	tree := &Tree{Node: node, Children: treeChildren}
	treeRoots := []*Tree{tree, childTree, childTree}[:1]
	g.Trees[nil] = treeRoots
	g.NodeToTree = make(map[*Node]*Tree)
	g.NodeToTree[node] = tree

	hubNodes := []*Node{node, tailNode, tailNode}[:1]
	g.Hubs[node] = hubNodes
	commonNodes := Nodes{node, tailNode, tailNode}[:1]
	g.CommonUncleSiblings = make(map[*Node]Nodes)
	g.CommonUncleSiblings[node] = commonNodes
	containerNodes := []*Node{node, tailNode, tailNode}[:1]
	g.Containers[nil] = containerNodes

	state := &GraphState{captureTopology: true, captureEdgeRoutes: true}
	updateGraphStateForTest(state, g)
	rollback := &Transaction{Graph: g, PriorGraphState: state}
	clustersMap := g.Clusters
	clustersMapPointer := reflect.ValueOf(clustersMap).Pointer()

	clusterNodesBefore := captureExactTestSlice(cluster.Nodes)
	clusterAbductionsBefore := captureExactTestSlice(cluster.EdgeAbductions)
	sequenceNodesBefore := captureExactTestSlice(sequence.Nodes)
	sequenceAbductionsBefore := captureExactTestSlice(sequence.EdgeAbductions)
	treeChildrenBefore := captureExactTestSlice(tree.Children)
	treeRootsBefore := captureExactTestSlice(g.Trees[nil])
	hubsBefore := captureExactTestSlice(g.Hubs[node])
	commonBefore := captureExactTestSlice([]*Node(g.CommonUncleSiblings[node]))
	routeBefore := captureExactTestSlice(edge.Points)
	routePointBefore := *edge.Points[0]

	cluster.Nodes = append(cluster.Nodes, tailNode)
	cluster.EdgeAbductions = append(cluster.EdgeAbductions, tailAbduction)
	sequence.Nodes = append(sequence.Nodes, tailNode)
	sequence.EdgeAbductions = append(sequence.EdgeAbductions, tailAbduction)
	tree.Children = append(tree.Children, childTree)
	g.Trees[nil] = append(g.Trees[nil], childTree)
	g.Hubs[node] = append(g.Hubs[node], tailNode)
	g.CommonUncleSiblings[node] = append(g.CommonUncleSiblings[node], tailNode)
	edge.Points[0].X = 123
	edge.Points = append(edge.Points, geo.NewPoint(5, 5))
	clustersMap[tailNode] = &Cluster{Vessel: tailNode, Graph: g}
	g.Clusters = map[*Node]*Cluster{clusterVessel: cluster}

	rollback.Rollback()
	if reflect.ValueOf(g.Clusters).Pointer() != clustersMapPointer || len(clustersMap) != 1 || clustersMap[clusterVessel] != cluster {
		t.Fatal("topology rollback did not restore the original cluster map and its aliases")
	}
	clusterNodesBefore.assertRestored(t, cluster.Nodes, "Cluster.Nodes")
	clusterAbductionsBefore.assertRestored(t, cluster.EdgeAbductions, "Cluster.EdgeAbductions")
	sequenceNodesBefore.assertRestored(t, sequence.Nodes, "Sequence.Nodes")
	sequenceAbductionsBefore.assertRestored(t, sequence.EdgeAbductions, "Sequence.EdgeAbductions")
	treeChildrenBefore.assertRestored(t, tree.Children, "Tree.Children")
	treeRootsBefore.assertRestored(t, g.Trees[nil], "Graph.Trees[nil]")
	hubsBefore.assertRestored(t, g.Hubs[node], "Graph.Hubs[node]")
	commonBefore.assertRestored(t, []*Node(g.CommonUncleSiblings[node]), "Graph.CommonUncleSiblings[node]")
	routeBefore.assertRestored(t, edge.Points, "Edge.Points")
	if *edge.Points[0] != routePointBefore {
		t.Fatal("edge route point value was not restored")
	}
}

func TestEnginePreflightFindsHiddenOversizedTopologyAndAllowsNilTreeRoot(t *testing.T) {
	g := NewGraph()
	hidden := make([]*Node, maxEngineNodes+1)
	for i := range hidden {
		hidden[i] = NewNode(EntityID(i+1), 1, 1)
	}
	g.Containers[nil] = hidden
	if err := validateEngineGraph(context.Background(), "test", g); err == nil {
		t.Fatal("preflight accepted more than 10,000 nodes hidden outside Graph.Nodes")
	}

	valid := NewGraph()
	rootNode := NewNode(1, 1, 1)
	rootTree := &Tree{Node: rootNode}
	valid.Trees[nil] = []*Tree{rootTree}
	if err := validateEngineGraph(context.Background(), "test", valid); err != nil {
		t.Fatalf("preflight rejected a valid nil tree-root key: %v", err)
	}
}

func TestEnginePreflightBoundsCapacityDepthCyclesAndNilRecords(t *testing.T) {
	t.Run("aggregate slice capacity", func(t *testing.T) {
		g := NewGraph()
		node := NewNode(1, 1, 1)
		backing := make([]*Node, 1, maxEngineTopologyReferences+2)
		backing[0] = node
		g.Nodes = backing
		err := validateEngineGraph(context.Background(), "test", g)
		if err == nil || !strings.Contains(err.Error(), "topology references") {
			t.Fatalf("preflight error = %v, want aggregate-reference limit", err)
		}
	})

	t.Run("route slice capacity", func(t *testing.T) {
		g := NewGraph()
		from := g.AddNode(NewNode(1, 1, 1))
		to := g.AddNode(NewNode(2, 1, 1))
		edge := g.Connect(from, to)
		edge.Points = make([]*geo.Point, 0, maxEngineRoutePoints+1)
		err := validateEngineGraph(context.Background(), "test", g)
		if err == nil || !strings.Contains(err.Error(), "route point count") {
			t.Fatalf("preflight error = %v, want route-capacity limit", err)
		}
	})

	t.Run("nil structural child", func(t *testing.T) {
		g := NewGraph()
		g.Containers[nil] = []*Node{nil}
		err := validateEngineGraph(context.Background(), "test", g)
		if err == nil || !strings.Contains(err.Error(), "nil container child") {
			t.Fatalf("preflight error = %v, want nil-child rejection", err)
		}
	})

	t.Run("descendant cycle", func(t *testing.T) {
		g := NewGraph()
		a := NewNode(1, 1, 1)
		b := NewNode(2, 1, 1)
		a.isContainer = true
		b.isContainer = true
		g.AddNodeUnchecked(a)
		g.AddNodeUnchecked(b)
		g.Containers[a] = []*Node{b}
		g.Containers[b] = []*Node{a}
		err := validateEngineGraph(context.Background(), "test", g)
		if err == nil || !strings.Contains(err.Error(), "cycle") {
			t.Fatalf("preflight error = %v, want descendant-cycle rejection", err)
		}
	})

	t.Run("container depth", func(t *testing.T) {
		g := NewGraph()
		var parent *Node
		for i := 0; i <= maxEngineTopologyDepth; i++ {
			node := NewNode(EntityID(i+1), 1, 1)
			node.isContainer = true
			node.Container = parent
			g.AddNodeUnchecked(node)
			g.Containers[parent] = []*Node{node}
			parent = node
		}
		err := validateEngineGraph(context.Background(), "test", g)
		if err == nil || !strings.Contains(err.Error(), "depth") {
			t.Fatalf("preflight error = %v, want container-depth rejection", err)
		}
	})

	t.Run("tree child cycle", func(t *testing.T) {
		g := NewGraph()
		a := &Tree{Node: NewNode(1, 1, 1)}
		b := &Tree{Node: NewNode(2, 1, 1)}
		a.Children = []*Tree{b}
		b.Children = []*Tree{a}
		g.Trees[nil] = []*Tree{a}
		err := validateEngineGraph(context.Background(), "test", g)
		if err == nil || !strings.Contains(err.Error(), "tree child cycle") {
			t.Fatalf("preflight error = %v, want tree-cycle rejection", err)
		}
	})

	t.Run("effective container cycle", func(t *testing.T) {
		g := NewGraph()
		a := NewNode(1, 1, 1)
		b := NewNode(2, 1, 1)
		vesselA := NewNode(3, 1, 1)
		vesselB := NewNode(4, 1, 1)
		vesselA.isClusterVessel = true
		vesselB.isClusterVessel = true
		for _, node := range []*Node{a, b, vesselA, vesselB} {
			g.AddNodeUnchecked(node)
		}
		clusterA := &Cluster{Vessel: vesselA, Nodes: []*Node{a}, Graph: g}
		clusterB := &Cluster{Vessel: vesselB, Nodes: []*Node{b}, Graph: g}
		a.Cluster = clusterA
		b.Cluster = clusterB
		vesselA.Container = b
		vesselB.Container = a
		g.Clusters[vesselA] = clusterA
		g.Clusters[vesselB] = clusterB

		err := validateEngineGraph(context.Background(), "test", g)
		if err == nil || !strings.Contains(err.Error(), "effective container parent cycle") {
			t.Fatalf("preflight error = %v, want effective-container-cycle rejection", err)
		}
	})

	t.Run("effective container depth", func(t *testing.T) {
		g := NewGraph()
		members := make([]*Node, maxEngineTopologyDepth+1)
		vessels := make([]*Node, len(members))
		for i := range members {
			members[i] = NewNode(EntityID(i+1), 1, 1)
			vessels[i] = NewNode(EntityID(10_000+i), 1, 1)
			vessels[i].isClusterVessel = true
			g.AddNodeUnchecked(members[i])
			g.AddNodeUnchecked(vessels[i])
		}
		for i := range members {
			cluster := &Cluster{Vessel: vessels[i], Nodes: []*Node{members[i]}, Graph: g}
			members[i].Cluster = cluster
			if i+1 < len(members) {
				vessels[i].Container = members[i+1]
			}
			g.Clusters[vessels[i]] = cluster
		}

		err := validateEngineGraph(context.Background(), "test", g)
		if err == nil || !strings.Contains(err.Error(), "effective container parent depth") {
			t.Fatalf("preflight error = %v, want effective-container-depth rejection", err)
		}
	})

	for _, test := range []struct {
		name     string
		setOwner func(*Node, *Node, *Graph)
	}{
		{
			name: "cluster ancestry",
			setOwner: func(member, vessel *Node, graph *Graph) {
				cluster := &Cluster{Vessel: vessel, Graph: graph}
				member.Cluster = cluster
				graph.Clusters[vessel] = cluster
			},
		},
		{
			name: "sequence ancestry",
			setOwner: func(member, vessel *Node, graph *Graph) {
				sequence := &Sequence{Vessel: vessel, Graph: graph}
				member.Sequence = sequence
				graph.Sequences[vessel] = sequence
			},
		},
	} {
		t.Run(test.name+" cycle", func(t *testing.T) {
			g := NewGraph()
			a := NewNode(1, 1, 1)
			b := NewNode(2, 1, 1)
			g.AddNodeUnchecked(a)
			g.AddNodeUnchecked(b)
			test.setOwner(a, b, g)
			test.setOwner(b, a, g)

			err := validateEngineGraph(context.Background(), "test", g)
			if err == nil || !strings.Contains(err.Error(), "ancestry parent cycle") {
				t.Fatalf("preflight error = %v, want ancestry-cycle rejection", err)
			}
		})

		t.Run(test.name+" depth", func(t *testing.T) {
			g := NewGraph()
			nodes := make([]*Node, maxEngineTopologyDepth+1)
			for i := range nodes {
				nodes[i] = NewNode(EntityID(i+1), 1, 1)
				g.AddNodeUnchecked(nodes[i])
			}
			for i := 0; i+1 < len(nodes); i++ {
				test.setOwner(nodes[i], nodes[i+1], g)
			}

			err := validateEngineGraph(context.Background(), "test", g)
			if err == nil || !strings.Contains(err.Error(), "ancestry parent depth") {
				t.Fatalf("preflight error = %v, want ancestry-depth rejection", err)
			}
		})
	}
}
