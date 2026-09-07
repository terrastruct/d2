package hierarchy

import (
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func manyExchangeRankGraph(tb testing.TB, width, height, edgeCount int) *layoutgraph.Graph {
	tb.Helper()
	nodeCount := width * height
	if edgeCount < width*(height-1) || edgeCount > nodeCount*(nodeCount-1)/2 {
		tb.Fatalf("invalid many-exchange rank graph size: %dx%d, %d edges", width, height, edgeCount)
	}
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, nodeCount)
	for i := range nodes {
		nodes[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		g.AddNodeUnchecked(nodes[i])
	}
	connect := func(fromLayer, fromColumn, toLayer, toColumn int) {
		from := fromLayer*width + fromColumn
		to := toLayer*width + toColumn
		edge := g.Connect(nodes[from], nodes[to])
		edge.ID = layoutgraph.EntityID(len(g.Edges))
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
		edge.SetHierarchyRankWeight(1 + (from*31+to*17)%997)
	}
	for layer := 0; layer+1 < height; layer++ {
		for column := 0; column < width; column++ {
			connect(layer, column, layer+1, column)
		}
	}
	for span := 2; len(g.Edges) < edgeCount && span < height; span++ {
		for layer := 0; layer+span < height && len(g.Edges) < edgeCount; layer++ {
			for column := 0; column < width && len(g.Edges) < edgeCount; column++ {
				connect(layer, column, layer+span, (column+span-1)%width)
			}
		}
	}
	if len(g.Edges) != edgeCount {
		tb.Fatalf("could only construct %d of %d many-exchange rank graph edges", len(g.Edges), edgeCount)
	}
	return g
}
