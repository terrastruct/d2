package trees

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestPlacementTreesDoNotMergeDisconnectedSentinels(t *testing.T) {
	g := layoutgraph.NewGraph()
	directedCenter := layoutgraph.NewNode(101, 10, 10)
	g.AddNewNodeToContainer(nil, directedCenter)
	for i := 0; i < 3; i++ {
		leaf := layoutgraph.NewNode(layoutgraph.EntityID(102+i), 10, 10)
		g.AddNewNodeToContainer(nil, leaf)
		edge := g.Connect(directedCenter, leaf)
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	}

	undirectedCenter := layoutgraph.NewNode(201, 10, 10)
	g.AddNewNodeToContainer(nil, undirectedCenter)
	for i := 0; i < 3; i++ {
		leaf := layoutgraph.NewNode(layoutgraph.EntityID(202+i), 10, 10)
		g.AddNewNodeToContainer(nil, leaf)
		g.Connect(undirectedCenter, leaf)
	}

	if err := Preprocess(withTestLogger(context.Background(), t), g); err != nil {
		t.Fatal(err)
	}
	if len(g.Trees) != 2 {
		t.Fatalf("expected two disconnected tree components, got %d", len(g.Trees))
	}
	placementTrees := mustBuildPlacementTrees(t, g)
	if len(placementTrees) != 2 {
		t.Fatalf("expected two placement trees, got %d", len(placementTrees))
	}
	for _, placementTree := range placementTrees {
		for _, child := range placementTree.Children {
			edge := child.SentinelEdge
			if edge.From != placementTree.Node && edge.To != placementTree.Node {
				t.Fatalf(
					"tree rooted at %d was attached to unrelated sentinel %d",
					child.Node.ID, placementTree.Node.ID,
				)
			}
		}
	}
}
