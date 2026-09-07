package layoutgraph

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustSplitSubgraphs(
	t *testing.T,
	ctx context.Context,
	g *Graph,
	options SplitOptions,
) []*Graph {
	t.Helper()
	graphs, err := g.SplitSubgraphs(ctx, options)
	require.NoError(t, err)
	return graphs
}

func (g *Graph) connectByID(nodeAID, nodeBID EntityID) {
	var nodeA *Node
	var nodeB *Node
	for _, node := range g.Nodes {
		if node.ID == nodeAID {
			nodeA = node
		} else if node.ID == nodeBID {
			nodeB = node
		}
	}
	g.Connect(nodeA, nodeB)
}

func TestGraphSplitting(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	graph := NewGraph()

	graph.AddNode(NewNode(1, 5, 5))
	graph.AddNode(NewNode(2, 2, 5))
	graph.connectByID(1, 2)

	graph.AddNode(NewNode(3, 5, 5))
	graph.AddNode(NewNode(4, 2, 5))
	graph.connectByID(3, 4)

	splitGraphs := mustSplitSubgraphs(t, ctx, graph, SplitOptions{IncludeNears: true})
	if len(splitGraphs) != 2 {
		t.Fatal("Expected 2 subgraphs")
	}

	graph = NewGraph()

	graph.AddNode(NewNode(1, 5, 5))
	graph.AddNode(NewNode(2, 2, 5))
	graph.AddNode(NewNode(3, 2, 5))
	graph.AddNode(NewNode(4, 2, 5))
	graph.connectByID(1, 2)
	graph.connectByID(2, 3)
	graph.connectByID(1, 4)

	graph.AddNode(NewNode(5, 5, 5))
	graph.AddNode(NewNode(6, 2, 5))
	graph.connectByID(5, 6)

	graph.AddNode(NewNode(7, 5, 5))
	graph.AddNode(NewNode(8, 2, 5))

	splitGraphs = mustSplitSubgraphs(t, ctx, graph, SplitOptions{IncludeNears: true})
	if len(splitGraphs) != 4 {
		t.Fatalf("Expected 4 subgraphs, got %v", len(splitGraphs))
	}
}

func TestGraphSplittingNears(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := NewGraph()

	n1 := g.AddNode(NewNode(1, 5, 5))
	n2 := g.AddNode(NewNode(2, 2, 5))
	n3 := g.AddNode(NewNode(3, 2, 5))
	n4 := g.AddNode(NewNode(4, 2, 5))

	n1.AddNear(n2)
	n2.AddNear(n3)
	n3.AddNear(n4)

	subgraphs := mustSplitSubgraphs(t, ctx, g, SplitOptions{IncludeNears: true})
	assert.Equal(t, 1, len(subgraphs))

	subgraphs = mustSplitSubgraphs(t, ctx, g, SplitOptions{})
	assert.Equal(t, 4, len(subgraphs))
	assert.Equal(t, 1, len(subgraphs[0].Nodes))
	assert.Equal(t, 1, len(subgraphs[1].Nodes))
	assert.Equal(t, 1, len(subgraphs[2].Nodes))
	assert.Equal(t, 1, len(subgraphs[3].Nodes))
}

func TestSplitSubgraphsCopyEntities(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := NewGraph()
	g.Containers[NewNode(1, 1, 1)] = []*Node{}
	clusterVessel := NewNode(2, 1, 1)
	g.Clusters[clusterVessel] = &Cluster{Vessel: clusterVessel, Graph: g}
	g.Trees[NewNode(4, 1, 1)] = []*Tree{}
	sequenceVessel := NewNode(5, 1, 1)
	g.Sequences[sequenceVessel] = &Sequence{Vessel: sequenceVessel, Graph: g}
	g.AddNode(NewNode(1, 5, 5))
	g.AddNode(NewNode(2, 2, 5))

	subgraphs := mustSplitSubgraphs(t, ctx, g, SplitOptions{})
	assert.Equal(t, 2, len(subgraphs))

	for _, sg := range subgraphs {
		assert.Equal(t, g.Containers, sg.Containers)
		assert.Equal(t, g.Clusters, sg.Clusters)
		assert.Equal(t, g.Trees, sg.Trees)
		assert.Equal(t, g.Sequences, sg.Sequences)
	}
}

func TestEmptyGraphCostsAreFinite(t *testing.T) {
	graph := NewGraph()
	if got := graph.area(); got != 0 {
		t.Fatalf("empty graph area = %v; want 0", got)
	}
	if got := graph.nonCenterPortCostValue(); math.IsInf(got, 0) || math.IsNaN(got) {
		t.Fatalf("empty graph non-center port cost is not finite: %v", got)
	}
}

func generateNodes(startingID EntityID, n int) []*Node {
	ns := make([]*Node, 0, n)
	for i := 0; i < n; i++ {
		ns = append(ns, NewNode(startingID+EntityID(i), 1, 1))
	}
	return ns
}

// Tests the output of containerRDFSOrder on the following
// containers map.
//
// .        ┌───┐
// .        │ 1 │
// .        └─┬─┘
// .  ┌───┐   │   ┌───┐
// .  │ 2 │◄──┴──►│ 3 │
// .  └─┬─┘       └─┬─┘
// .    │           │
// .  ┌─▼─┐       ┌─▼─┐   ┌───┐
// .  │ 4 │       │ 5 ├──►│ 9 │
// .  └─┬─┘       └─┬─┘   └─┬─┘
// .    │           │       │
// .  ┌─▼─┐       ┌─▼─┐   ┌─┴──┐
// .  │ 6 │       │ 7 │   │ 10 │
// .  └─┬─┘       └───┘   └────┘
// .    │
// .  ┌─▼─┐
// .  │ 8 │
// .  └───┘
//
// Node 11 is just a cluster with separate nodes arranged in the same way.
func TestGraphRDFSOrder(t *testing.T) {
	g := NewGraph()
	g.Containers = map[*Node][]*Node{}

	ns := generateNodes(1, 10)
	for _, n := range ns {
		g.AddNode(n)
	}

	// Add a graph of 10 nodes to containers.
	addToContainers := func(g *Graph, ns []*Node) {
		g.Containers[nil] = append(g.Containers[nil], ns[0])

		g.Containers[ns[0]] = []*Node{ns[1], ns[2]}

		g.Containers[ns[1]] = []*Node{ns[3]}
		g.Containers[ns[3]] = []*Node{ns[5]}
		g.Containers[ns[5]] = []*Node{ns[7]}

		g.Containers[ns[2]] = []*Node{ns[4]}
		g.Containers[ns[4]] = []*Node{ns[6], ns[8]}
		g.Containers[ns[8]] = []*Node{ns[9]}

		for container, children := range g.Containers {
			for _, child := range children {
				child.Container = container
			}
			if container != nil {
				container.isContainer = true
			}
		}
	}

	ns2 := generateNodes(11, 10)
	cl := &Cluster{}
	cl.Nodes = ns2[1:]
	g.Clusters = map[*Node]*Cluster{
		ns2[0]: cl,
	}
	ns2[0].isClusterVessel = true
	for _, n := range cl.Nodes {
		g.AddNode(n)
	}

	addToContainers(g, ns)
	addToContainers(g, ns2)

	var ids []EntityID
	for _, n := range g.containerRDFSOrder(nil) {
		ids = append(ids, n.ID)
	}

	exp := []EntityID{19, 15, 13, 16, 14, 12, 11, 9, 5, 3, 6, 4, 2, 1}
	if !reflect.DeepEqual(exp, ids) {
		t.Fatalf("expected %#v but got %#v", exp, ids)
	}
}

func TestCopyEntitiesFrom(t *testing.T) {
	g1 := NewGraph()
	g1.Containers[NewNode(1, 1, 1)] = []*Node{}
	g1.Clusters[NewNode(2, 1, 1)] = &Cluster{}
	g1.Trees[NewNode(4, 1, 1)] = []*Tree{}
	g1.Sequences[NewNode(5, 1, 1)] = &Sequence{}

	g2 := NewGraph()
	g2.CopyEntitiesFrom(g1)

	assert.Equal(t, g1.Containers, g2.Containers)
	assert.Equal(t, g1.Clusters, g2.Clusters)
	assert.Equal(t, g1.Trees, g2.Trees)
	assert.Equal(t, g1.Sequences, g2.Sequences)
}
