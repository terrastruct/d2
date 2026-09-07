package d2talalayout

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2target"
)

func TestLayoutKeepsForcedHierarchyWithUnrelatedFixedNode(t *testing.T) {
	g := d2graph.NewGraph()
	g.Root.Shape.Value = d2target.ShapeHierarchy
	nodes := make([]*d2graph.Object, 8)
	for index := range nodes {
		nodes[index] = appendD2Object(g.Root, string(rune('a'+index)))
		nodes[index].Width = 80
		nodes[index].Height = 50
	}
	for _, endpoints := range [][2]int{
		{0, 2}, {1, 3}, {2, 4}, {3, 5}, {2, 5}, {4, 6}, {5, 7},
	} {
		g.Edges = append(g.Edges, &d2graph.Edge{
			Src:        nodes[endpoints[0]],
			Dst:        nodes[endpoints[1]],
			DstArrow:   true,
			Attributes: d2graph.Attributes{},
		})
	}
	fixed := appendD2Object(g.Root, "fixed")
	fixed.Width = 80
	fixed.Height = 50
	fixed.Top = &d2graph.Scalar{Value: "350"}
	fixed.Left = &d2graph.Scalar{Value: "700"}

	if err := Layout(context.Background(), g, &Options{Seeds: []int64{1}, MaxConcurrency: 1}); err != nil {
		t.Fatal(err)
	}
	if fixed.TopLeft.X != 700 || fixed.TopLeft.Y != 350 {
		t.Fatalf("fixed node position = %v, want (700,350)", fixed.TopLeft)
	}
	for _, level := range [][2]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}} {
		firstY := nodes[level[0]].TopLeft.Y
		secondY := nodes[level[1]].TopLeft.Y
		if firstY != secondY {
			t.Fatalf("hierarchy level nodes %d and %d have Y coordinates %v and %v", level[0], level[1], firstY, secondY)
		}
	}
	for level := 0; level < 3; level++ {
		if nodes[level*2].TopLeft.Y >= nodes[(level+1)*2].TopLeft.Y {
			t.Fatalf("hierarchy levels are not ordered top-to-bottom: level %d Y=%v, next Y=%v", level, nodes[level*2].TopLeft.Y, nodes[(level+1)*2].TopLeft.Y)
		}
	}
}
