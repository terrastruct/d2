package placement

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestInitializeNodesHandlesNearCycle(t *testing.T) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	second := layoutgraph.NewNode(2, 10, 10)
	third := layoutgraph.NewNode(3, 10, 10)
	for _, node := range []*layoutgraph.Node{first, second, third} {
		graph.AddNewNodeToContainer(nil, node)
	}
	first.AddNear(second)
	second.AddNear(third)
	third.AddNear(first)

	graph.ComputeCellSize()
	if err := initializeNodes(withTestLogger(context.Background(), t), graph); err != nil {
		t.Fatal(err)
	}
	for _, node := range []*layoutgraph.Node{first, second, third} {
		if node.TopLeft == nil {
			t.Fatalf("node %v was not initialized", node.ID)
		}
	}
}
