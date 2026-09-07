package placement

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func abductClusterEdgesForOptimizationFixture(cluster *layoutgraph.Cluster) {
	var abductions []*layoutgraph.EdgeAbduction
	for _, edge := range cluster.Graph.Edges {
		if edge.From.Cluster == cluster {
			abductions = append(abductions, &layoutgraph.EdgeAbduction{
				Edge: edge, OriginallyFrom: edge.From,
				CurrentFrom: cluster.Vessel, CurrentTo: edge.To,
			})
			edge.Reconnect(cluster.Vessel, false)
		}
		if edge.To.Cluster == cluster {
			abductions = append(abductions, &layoutgraph.EdgeAbduction{
				Edge: edge, OriginallyTo: edge.To,
				CurrentTo: cluster.Vessel, CurrentFrom: edge.From,
			})
			edge.Reconnect(cluster.Vessel, true)
		}
	}
	cluster.EdgeAbductions = abductions
}

func TestFlipClustersGapReduce(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)

	a := layoutgraph.NewNode(1, 200, 100)
	a.TopLeft = geo.NewPoint(1000, 1000)
	c1 := layoutgraph.NewNode(3, 200, 100)
	c1.TopLeft = geo.NewPoint(1000, 1220)
	c2 := layoutgraph.NewNode(4, 200, 100)
	c2.TopLeft = geo.NewPoint(1000, 1440)
	b := layoutgraph.NewNode(2, 200, 100)
	b.TopLeft = geo.NewPoint(1000, 1660)

	graph := layoutgraph.NewGraph()
	for _, node := range []*layoutgraph.Node{a, b, c1, c2} {
		graph.AddNewNodeToContainer(nil, node)
	}
	graph.ComputeCellSize()
	graph.Connect(a, c1)
	graph.Connect(a, c2)
	graph.Connect(c1, b)
	graph.Connect(c2, b)

	vessel := layoutgraph.NewNode(5, 200, 320)
	vessel.TopLeft = geo.NewPoint(1000, 1220)
	cluster := &layoutgraph.Cluster{
		Nodes:              []*layoutgraph.Node{c1, c2},
		Graph:              graph,
		Arrangement:        layoutgraph.Column,
		DesiredArrangement: layoutgraph.Row,
		Vessel:             vessel,
	}
	vessel.SetClusterVessel(true)
	grouping.AddCluster(graph, cluster)
	abductClusterEdgesForOptimizationFixture(cluster)

	_, err := OptimizeClusters(ctx, graph)
	assert.NoError(t, err)
	assert.Equal(t, 1000.0, a.TopLeft.Y)
	assert.Equal(t, c1.TopLeft.Y, c2.TopLeft.Y)
	assert.Less(t, b.TopLeft.Y, 1750.0)
}
