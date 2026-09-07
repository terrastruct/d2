package trees

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestBuildPlacementTreesForIsolatedTree(t *testing.T) {
	/*
	   Trees 7 and 8 can be merged because they have the same direction and their arrowhead at 0 is the same
	   Trees 1 and 8 can't be merged because they have different directions, even though their arrowhead at 0 is the same
	   Trees 1 and 7 same as above
	   Trees 4 and 7 can't be merged because they don't have the same arrowhead at 0, even though they have the same direction
	   Trees 4 and 8 can't be merged because they don't have the same arrowhead at 0, even though they have the same direction
	*/
	// ┌──────┐           ┌──────┐
	// │      │           │      │
	// │  2   ├──────┬────┤  3   │
	// │      │      │    │      │
	// └──────┘      │    └──────┘                   ┌──────┐
	//               │                               │      │
	//               │                               │  9   │
	//           ┌───▼──┐                            │      │
	//           │      │                            └──▲───┘
	//           │  1   │                               │
	//           │      │             ┌──────┐          │
	//           └──▲───┘             │      ├──────────┤
	//              │                 │   8  │          │
	//              │                 │      │          │
	//              │                 └──▲───┘       ┌──▼───┐
	//              │                    │           │      │
	//              │                    │           │ 10   │
	//           ┌──┴───┐                │           │      │
	//           │      │                │           └──────┘
	//           │  0   ├────────────────┤
	//           │      │                │           ┌──────┐
	//           └───▲──┘                │           │      │
	//               │                   │           │ 11   │
	//               │                ┌──▼───┐       │      │
	//               │                │      │       └──▲───┘
	//               │                │  7   │          │
	//           ┌───┴──┐             │      ├──────────┤
	//           │      │             └──────┘          │
	//           │  4   │                            ┌──▼───┐
	//           │      │                            │      │
	//           └───┬──┘                            │ 12   │
	//               │                               │      │
	//               │                               └──────┘
	//               │
	// ┌──────┐      │      ┌──────┐
	// │      │      │      │      │
	// │  5   ◄──────┴──────►  6   │
	// │      │             │      │
	// └──────┘             └──────┘

	g := layoutgraph.NewGraph()

	var nodes [13]*layoutgraph.Node
	for i := 0; i < len(nodes); i++ {
		nodes[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		g.AddNewNodeToContainer(nil, nodes[i])
	}

	connect := func(i, j int, source, target bool) {
		e := g.Connect(nodes[i], nodes[j])
		if source {
			e.SourceArrowhead = layoutgraph.TriangleArrowhead
		}
		if target {
			e.TargetArrowhead = layoutgraph.TriangleArrowhead
		}
	}

	// top tree
	connect(0, 1, false, true)
	connect(2, 1, false, true)
	connect(3, 1, false, true)

	// bottom tree
	connect(4, 0, false, true)
	connect(4, 5, false, true)
	connect(4, 6, false, true)

	// right top tree
	connect(0, 8, false, true)
	connect(8, 9, false, true)
	connect(8, 10, false, true)

	// right bottom tree
	connect(0, 7, false, false)
	connect(7, 11, false, true)
	connect(7, 12, false, true)

	ctx := context.Background()
	ctx = withTestLogger(ctx, t)
	g.Trees = mustExtractTrees(t, ctx, g)

	trees := mustBuildPlacementTrees(t, g)

	assert.Equal(t, 3, len(trees))
	for _, tree := range trees {
		if len(tree.Children) == 2 {
			set := map[*layoutgraph.Node]struct{}{
				tree.Children[0].Node: {},
				tree.Children[1].Node: {},
			}
			delete(set, nodes[7])
			delete(set, nodes[8])
			if len(set) != 0 {
				t.Fatalf("Expected nodes 7 and 8 to be siblings")
			}
		} else if len(tree.Children) == 1 {
			c := tree.Children[0].Node
			if !(c == nodes[4] || c == nodes[1]) {
				t.Fatal("Expected only nodes 1 and 4 to be isolated children")
			}
		} else {
			t.Fatal("Unexpected children count")
		}
	}
}
