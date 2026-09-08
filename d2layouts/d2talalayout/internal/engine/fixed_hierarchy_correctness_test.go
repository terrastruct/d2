package engine

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestHierarchyStageDoesNotMoveFixedMember(t *testing.T) {
	g := layoutgraph.NewGraph()
	nodes := []*layoutgraph.Node{
		layoutgraph.NewNode(1, 40, 30),
		layoutgraph.NewNode(2, 40, 30),
		layoutgraph.NewNode(3, 40, 30),
	}
	for index, node := range nodes {
		node.TopLeft = geo.NewPoint(float64(100+index*130), float64(400-index*70))
		g.AddNewNodeToContainer(nil, node)
	}
	for index := 1; index < len(nodes); index++ {
		edge := g.Connect(nodes[index-1], nodes[index])
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	}
	hierarchy := newHierarchyWithLevels(map[*layoutgraph.Node]int{
		nodes[0]: 0,
		nodes[1]: 1,
		nodes[2]: 2,
	})
	for _, node := range nodes {
		node.Hierarchy = hierarchy
	}
	nodes[1].FixedTopLeft = nodes[1].TopLeft.Copy()
	before := nodes[1].TopLeft.Copy()

	pipeline := newPipeline(g, 1, false)
	if err := pipeline.preprocessHierarchies(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !nodes[1].TopLeft.Equals(before) {
		t.Fatalf("fixed member moved from %v to %v", before, nodes[1].TopLeft)
	}
}
