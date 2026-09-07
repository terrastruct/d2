package engine

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/packing"
)

func TestCombineSubgraphsPreservesEntities(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	newLaidOutGraph := func(ids ...layoutgraph.EntityID) *layoutgraph.Graph {
		graph := layoutgraph.NewGraph()
		for index, id := range ids {
			size := 2.0
			if index == 0 {
				size = 5
			}
			graph.AddNode(layoutgraph.NewNode(id, size, 5))
		}
		for _, node := range graph.Nodes[1:] {
			graph.Connect(graph.Nodes[0], node)
		}
		graph.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{nil: append([]*layoutgraph.Node(nil), graph.Nodes...)}
		pipeline := newPipeline(graph, 1, false)
		if err := pipeline.runAllStages(ctx); err != nil {
			t.Fatal(err)
		}
		return graph
	}

	first := newLaidOutGraph(1, 2, 3)
	second := newLaidOutGraph(4, 5)
	combined, err := packing.CombineSubgraphs(ctx, layoutgraph.NewGraph(), []*layoutgraph.Graph{first, second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(combined.Nodes) != len(first.Nodes)+len(second.Nodes) {
		t.Fatalf("combined node count = %d, want %d", len(combined.Nodes), len(first.Nodes)+len(second.Nodes))
	}
	if len(combined.Edges) != len(first.Edges)+len(second.Edges) {
		t.Fatalf("combined edge count = %d, want %d", len(combined.Edges), len(first.Edges)+len(second.Edges))
	}
}
