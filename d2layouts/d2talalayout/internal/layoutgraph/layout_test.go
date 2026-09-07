package layoutgraph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDescendantOfClusterVessel(t *testing.T) {
	graph := NewGraph()
	vessel := NewNode(1, 100, 100)
	clusterNode := NewNode(2, 100, 100)
	clusterNodeChild := NewNode(3, 100, 100)
	graph.AddNodeToContainer(clusterNode, clusterNodeChild)
	addClusterFixture(graph, &Cluster{
		Vessel: vessel,
		Nodes:  []*Node{clusterNode},
	})
	assert.True(t, clusterNode.isDescendantOf(vessel))
	assert.True(t, clusterNodeChild.isDescendantOf(vessel))
}

func TestRestoreSubgraphEdgeAbductions(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	// ┌──────────────────────────┐         ┌───────────────────────────────┐
	// │                          │         │  4                            │
	// │  0                       │         │ ┌───────────────────────────┐ │
	// │    ┌───────────────┐     │         │ │  6                        │ │
	// │    │               │     │         │ │   ┌───────────────────┐   │ │
	// │    │      1        │     │         │ │   │                   │   │ │
	// │    │               │     │         │ │   │      7            │   │ │
	// │    └┬───┬──────────┘     │         │ │   │                   │   │ │
	// │     │   │                │         │ │   └────────┬──────────┘   │ │
	// │     │   │                │         │ │            │              │ │
	// └─┬───┼───┼───┬────────────┘         │ └───────┬────┼──────────────┘ │
	//   │   │   │   │                      │         │    │                │
	//   │   │   │   │                      └────┬────┼────┼────────────────┘
	//   │   │   │   │                           │    │    │
	//   │   │   │   │                           │    │    │
	// ┌─▼───▼───┼───┼────────────┐              │    │    │
	// │         │   │            │              │    │    │
	// │ 2       │   │            │              │    │    │
	// │     ┌───▼───▼──────┐     │              │    │    │
	// │     │              │     │            ┌─▼────▼────▼──────────────┐
	// │     │      3       │     │            │                          │
	// │     └──────────────┘     │            │         5                │
	// │                          │            │                          │
	// └──────────────────────────┘            │                          │
	//                                         └──────────────────────────┘
	g1 := NewGraph()
	for i := 0; i < 8; i++ {
		g1.AddNode(NewNode(EntityID(i), 50, 50))
	}

	g1.AddNodeToContainer(nil, g1.Nodes[0])
	g1.AddNodeToContainer(g1.Nodes[0], g1.Nodes[1])
	g1.AddNodeToContainer(nil, g1.Nodes[2])
	g1.AddNodeToContainer(g1.Nodes[2], g1.Nodes[3])
	g1.AddNodeToContainer(nil, g1.Nodes[4])
	g1.AddNodeToContainer(g1.Nodes[4], g1.Nodes[6])
	g1.AddNodeToContainer(g1.Nodes[6], g1.Nodes[7])
	g1.AddNodeToContainer(nil, g1.Nodes[5])

	connect := func(i, j int) {
		e := g1.Connect(g1.Nodes[i], g1.Nodes[j])
		e.SourceArrowhead = NoArrowhead
		e.TargetArrowhead = TriangleArrowhead
	}

	connect(0, 2)
	connect(0, 3)
	connect(1, 2)
	connect(1, 3)
	connect(4, 5)
	connect(6, 5)
	connect(7, 5)

	g2 := NewGraph()
	g2.CopyEntitiesFrom(g1)
	for _, child := range g1.Containers[nil] {
		g2.AddNodeUnchecked(child)
	}

	abductions := g1.abductEdges(nil, g2)
	assert.Equal(t, 5, len(abductions))

	subgraphs := mustSplitSubgraphs(t, ctx, g2, SplitOptions{})
	assert.Equal(t, 2, len(subgraphs))

	// check left subgraph nodes
	assert.Equal(t, 0, len(g1.Nodes[1].Edges))
	assert.Equal(t, 0, len(g1.Nodes[3].Edges))
	assert.Equal(t, 4, len(g1.Nodes[0].Edges))
	assert.Equal(t, 4, len(g1.Nodes[2].Edges))

	// check right subgraph nodes
	assert.Equal(t, 0, len(g1.Nodes[7].Edges))
	assert.Equal(t, 0, len(g1.Nodes[6].Edges))
	assert.Equal(t, 3, len(g1.Nodes[4].Edges))
	assert.Equal(t, 3, len(g1.Nodes[5].Edges))

	// restore left subgraph abductions
	abductions = subgraphs[0].restoreEdgeAbductions(abductions)
	assert.Equal(t, 2, len(abductions))

	// check left subgraph nodes
	assert.Equal(t, 2, len(g1.Nodes[1].Edges))
	assert.Equal(t, 2, len(g1.Nodes[3].Edges))
	assert.Equal(t, 2, len(g1.Nodes[0].Edges))
	assert.Equal(t, 2, len(g1.Nodes[2].Edges))

	// check right subgraph nodes (keep the abductions, only restored the left subgraph)
	assert.Equal(t, 0, len(g1.Nodes[7].Edges))
	assert.Equal(t, 0, len(g1.Nodes[6].Edges))
	assert.Equal(t, 3, len(g1.Nodes[4].Edges))
	assert.Equal(t, 3, len(g1.Nodes[5].Edges))

	// restore left subgraph abductions (again)
	abductions = subgraphs[0].restoreEdgeAbductions(abductions)
	assert.Equal(t, 2, len(abductions))

	// check left subgraph nodes
	assert.Equal(t, 2, len(g1.Nodes[1].Edges))
	assert.Equal(t, 2, len(g1.Nodes[3].Edges))
	assert.Equal(t, 2, len(g1.Nodes[0].Edges))
	assert.Equal(t, 2, len(g1.Nodes[2].Edges))

	// check right subgraph nodes (keep the abductions, only restored the left subgraph)
	assert.Equal(t, 0, len(g1.Nodes[7].Edges))
	assert.Equal(t, 0, len(g1.Nodes[6].Edges))
	assert.Equal(t, 3, len(g1.Nodes[4].Edges))
	assert.Equal(t, 3, len(g1.Nodes[5].Edges))

	// restore right subgraph abductions
	abductions = subgraphs[1].restoreEdgeAbductions(abductions)
	assert.Equal(t, 0, len(abductions))

	// check left subgraph nodes
	assert.Equal(t, 2, len(g1.Nodes[1].Edges))
	assert.Equal(t, 2, len(g1.Nodes[3].Edges))
	assert.Equal(t, 2, len(g1.Nodes[0].Edges))
	assert.Equal(t, 2, len(g1.Nodes[2].Edges))

	// check right subgraph nodes (keep the abductions, only restored the left subgraph)
	assert.Equal(t, 1, len(g1.Nodes[7].Edges))
	assert.Equal(t, 1, len(g1.Nodes[6].Edges))
	assert.Equal(t, 1, len(g1.Nodes[4].Edges))
	assert.Equal(t, 3, len(g1.Nodes[5].Edges))
}
