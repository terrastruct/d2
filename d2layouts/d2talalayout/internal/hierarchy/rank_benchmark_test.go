package hierarchy

import (
	"context"
	"runtime"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func BenchmarkRankDAG(b *testing.B) {
	benchmarks := []struct {
		name      string
		nodeCount int
		edgeCount int
	}{
		{name: "chain_10000", nodeCount: 10_000, edgeCount: 9_999},
		{name: "sparse_1000_4000", nodeCount: 1_000, edgeCount: 4_000},
		{name: "dense_1000_50000", nodeCount: 1_000, edgeCount: 50_000},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			g := benchmarkRankGraph(b, benchmark.nodeCount, benchmark.edgeCount)
			b.ReportAllocs()
			for b.Loop() {
				result, err := rankDAG(context.Background(), g)
				if err != nil {
					b.Fatal(err)
				}
				runtime.KeepAlive(result)
			}
		})
	}
	b.Run("many_exchanges_250_1000", func(b *testing.B) {
		g := manyExchangeRankGraph(b, 25, 10, 1_000)
		b.ReportAllocs()
		for b.Loop() {
			result, err := rankDAG(context.Background(), g)
			if err != nil {
				b.Fatal(err)
			}
			runtime.KeepAlive(result)
		}
	})
}

func benchmarkRankGraph(tb testing.TB, nodeCount, edgeCount int) *layoutgraph.Graph {
	tb.Helper()
	if nodeCount < 2 || edgeCount < nodeCount-1 {
		tb.Fatalf("invalid rank benchmark size: %d nodes, %d edges", nodeCount, edgeCount)
	}
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, nodeCount)
	for i := range nodes {
		nodes[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		g.AddNodeUnchecked(nodes[i])
	}
	connect := func(from, to int) {
		edge := g.Connect(nodes[from], nodes[to])
		edge.ID = layoutgraph.EntityID(len(g.Edges))
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
		edge.SetHierarchyRankWeight(1 + (from*31+to*17)%997)
	}
	for from := 0; from+1 < nodeCount; from++ {
		connect(from, from+1)
	}
	for span := 2; len(g.Edges) < edgeCount && span < nodeCount; span++ {
		for from := 0; from+span < nodeCount && len(g.Edges) < edgeCount; from++ {
			connect(from, from+span)
		}
	}
	if len(g.Edges) != edgeCount {
		tb.Fatalf("could only construct %d of %d rank benchmark edges", len(g.Edges), edgeCount)
	}
	return g
}
