package trees

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func mustExtractTrees(t *testing.T, ctx context.Context, g *layoutgraph.Graph) map[*layoutgraph.Node][]*layoutgraph.Tree {
	t.Helper()
	if err := layoutgraph.Validate(ctx, "ExtractTrees", g); err != nil {
		t.Fatal(err)
	}
	guard, err := newWorkGuard(ctx, "ExtractTrees")
	if err != nil {
		t.Fatal(err)
	}
	trees, err := extractTrees(g, guard)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Finish(); err != nil {
		t.Fatal(err)
	}
	return trees
}

func makeNode(id int) *layoutgraph.Node {
	return layoutgraph.NewNode(layoutgraph.EntityID(id), 100, 100)
}

func createDirectedTreeGraph() (*layoutgraph.Graph, map[int]*layoutgraph.Node) {
	//         ┌───┐
	//         │ 0 │
	//         └┬─▲┘
	//          │ │
	// ┌───┐   ┌▼─┴┐
	// │ 1 ◄───┤ 2 │
	// └───┘   └▲─▲┘
	//       ┌──┘ └──┐
	//     ┌─┴─┐   ┌─┴─┐
	//     │ 3 │   │ 4 │
	//     └─▲─┘   └─┬─┘
	//       │       │
	//     ┌─┴─┐   ┌─▼─┐
	//     │ 5 │   │ 6 │
	//     └───┘   └───┘
	getNode := make(map[int]*layoutgraph.Node)
	g := layoutgraph.NewGraph()
	for i := 0; i < 7; i++ {
		n := makeNode(i)
		g.AddNodeUnchecked(n)
		getNode[i] = n
		g.Containers[nil] = append(g.Containers[nil], n)
	}

	getNode[0].TopLeft = geo.NewPoint(200, 0)
	getNode[1].TopLeft = geo.NewPoint(0, 200)
	getNode[2].TopLeft = geo.NewPoint(200, 200)
	getNode[3].TopLeft = geo.NewPoint(100, 400)
	getNode[4].TopLeft = geo.NewPoint(300, 400)
	getNode[5].TopLeft = geo.NewPoint(100, 600)
	getNode[6].TopLeft = geo.NewPoint(300, 600)

	connectDirected := func(a, b int) *layoutgraph.Edge {
		e := g.Connect(getNode[a], getNode[b])
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}
	connectDirected(0, 2)
	connectDirected(2, 0)
	connectDirected(2, 1)
	connectDirected(3, 2)
	connectDirected(5, 3)
	connectDirected(4, 2)
	connectDirected(4, 6)
	return g, getNode
}

func TestExtractTreesDirected(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g, getNode := createDirectedTreeGraph()
	if len(g.Nodes) != 7 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 7 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}

	nodeToTreeRoots := mustExtractTrees(t, ctx, g)
	//         ┌───┐
	//   g:    │ 0 │
	//         └┬─▲┘
	//          │ │
	//         ┌▼─┴┐
	//         │ 2 │
	//         └───┘
	if len(g.Nodes) != 2 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}
	isRemainingNode := func(n *layoutgraph.Node) bool {
		return n.ID == 0 || n.ID == 2
	}
	for _, n := range g.Nodes {
		if !isRemainingNode(n) {
			t.Fatalf("Unexpected node %v\n", n)
		}
	}
	for _, e := range g.Edges {
		if !isRemainingNode(e.To) || !isRemainingNode(e.From) {
			t.Fatalf("Unexpected edge %v->%v\n", e.To.ID, e.From.ID)
		}
	}

	// nodeToTreeRoots:
	//  ┌───┐ ┌───┐ ┌───┐ ┌───┐
	//  │ 2 │:│ 1 │,│ 3 │,│ 4 │
	//  └───┘ └───┘ └───┘ └───┘
	if len(nodeToTreeRoots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots size %v\n", len(nodeToTreeRoots))
	}

	if len(nodeToTreeRoots[getNode[2]]) != 3 {
		t.Fatalf("Unexpected nodeToTreeRoots[2] size %v\n", len(nodeToTreeRoots[getNode[2]]))
	}
	for _, root := range nodeToTreeRoots[getNode[2]] {
		isExpectedID := slices.Contains([]layoutgraph.EntityID{1, 3, 4}, root.Node.ID)
		if !isExpectedID {
			t.Fatalf("Unexpected node2 root:%v\n", root.Node.ID)
		}
	}
}

func createUndirectedTreeGraph() (*layoutgraph.Graph, map[int]*layoutgraph.Node) {
	//         ┌───┐
	//         │ 0 │
	//         └┬─┬┘
	//          │ │
	// ┌───┐   ┌┴─┴┐
	// │ 1 │───┤ 2 │
	// └───┘   └┬─┬┘
	//       ┌──┘ └──┐
	//     ┌─┴─┐   ┌─┴─┐
	//     │ 3 │   │ 4 │
	//     └─┬─┘   └─┬─┘
	//       │       │
	//     ┌─┴─┐   ┌─┴─┐
	//     │ 5 │   │ 6 │
	//     └───┘   └───┘
	getNode := make(map[int]*layoutgraph.Node)
	g := layoutgraph.NewGraph()
	for i := 0; i < 7; i++ {
		n := makeNode(i)
		g.AddNodeUnchecked(n)
		getNode[i] = n
		g.Containers[nil] = append(g.Containers[nil], n)
	}

	getNode[0].TopLeft = geo.NewPoint(200, 0)
	getNode[1].TopLeft = geo.NewPoint(0, 200)
	getNode[2].TopLeft = geo.NewPoint(200, 200)
	getNode[3].TopLeft = geo.NewPoint(100, 400)
	getNode[4].TopLeft = geo.NewPoint(300, 400)
	getNode[5].TopLeft = geo.NewPoint(100, 600)
	getNode[6].TopLeft = geo.NewPoint(300, 600)

	connectUndirected := func(a, b int) *layoutgraph.Edge {
		e := g.Connect(getNode[a], getNode[b])
		return e
	}
	connectUndirected(0, 2)
	connectUndirected(2, 0)
	connectUndirected(2, 1)
	connectUndirected(3, 2)
	connectUndirected(5, 3)
	connectUndirected(4, 2)
	connectUndirected(4, 6)
	return g, getNode
}

func TestExtractTreesUndirected(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g, getNode := createUndirectedTreeGraph()
	if len(g.Nodes) != 7 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 7 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}

	nodeToTreeRoots := mustExtractTrees(t, ctx, g)
	//         ┌───┐
	//   g:    │ 0 │
	//         └┬─┬┘
	//          │ │
	//         ┌┴─┴┐
	//         │ 2 │
	//         └───┘
	if len(g.Nodes) != 2 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}
	isRemainingNode := func(n *layoutgraph.Node) bool {
		return n.ID == 0 || n.ID == 2
	}
	for _, n := range g.Nodes {
		if !isRemainingNode(n) {
			t.Fatalf("Unexpected node %v\n", n)
		}
	}
	for _, e := range g.Edges {
		if !isRemainingNode(e.To) || !isRemainingNode(e.From) {
			t.Fatalf("Unexpected edge %v->%v\n", e.To.ID, e.From.ID)
		}
	}

	// nodeToTreeRoots:
	//  ┌───┐ ┌───┐ ┌───┐ ┌───┐
	//  │ 2 │:│ 1 │,│ 3 │,│ 4 │
	//  └───┘ └───┘ └───┘ └───┘
	if len(nodeToTreeRoots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots size %v\n", len(nodeToTreeRoots))
	}

	if len(nodeToTreeRoots[getNode[2]]) != 3 {
		t.Fatalf("Unexpected nodeToTreeRoots[2] size %v\n", len(nodeToTreeRoots[getNode[2]]))
	}
	for _, root := range nodeToTreeRoots[getNode[2]] {
		isExpectedID := slices.Contains([]layoutgraph.EntityID{1, 3, 4}, root.Node.ID)
		if !isExpectedID {
			t.Fatalf("Unexpected node2 root:%v\n", root.Node.ID)
		}
	}
}

func createMixedTreeGraph() (*layoutgraph.Graph, map[int]*layoutgraph.Node) {
	//             ┌───┐   ┌───┐   ┌───┐   ┌───┐
	//             │ 0 ◄───► 1 ◄───► 2 ◄───► 3 │
	//             └─▲─┘   └───┘   └───┘   └───┘
	//               │
	//             ┌─▼─┐               ┌───┐
	//             │ 4 ◄──────x2───────► 5 │
	//             └▲─┬┘               └─┬─┘
	//           ┌──┘ └──┐               │
	//         ┌─┴─┐   ┌─▼─┐   ┌───┐   ┌─┴─┐   ┌───┐
	//         │ 6 │   │ 7 ├───► 8 │   │ 9 ├───┤10 │
	//         └▲─▲┘   └─┬─┘   └───┘   └───┘   └┬─┬┘
	//   ┌──────┘┌┘      │                   ┌──┘ └──┐
	// ┌─┴─┐   ┌─┴─┐   ┌─▼─┐               ┌─┴─┐   ┌─┴─┐
	// │11 │   │12 │   │13 │               │14 │   │15 │
	// └───┘   └───┘   └───┘               └───┘   └───┘
	getNode := make(map[int]*layoutgraph.Node)
	g := layoutgraph.NewGraph()
	for i := 0; i < 16; i++ {
		n := makeNode(i)
		g.AddNodeUnchecked(n)
		getNode[i] = n
		g.Containers[nil] = append(g.Containers[nil], n)
	}

	getNode[0].TopLeft = geo.NewPoint(300, 0)
	getNode[1].TopLeft = geo.NewPoint(500, 0)
	getNode[2].TopLeft = geo.NewPoint(700, 0)
	getNode[3].TopLeft = geo.NewPoint(900, 0)
	getNode[4].TopLeft = geo.NewPoint(300, 200)
	getNode[5].TopLeft = geo.NewPoint(800, 200)
	getNode[6].TopLeft = geo.NewPoint(200, 400)
	getNode[7].TopLeft = geo.NewPoint(400, 400)
	getNode[8].TopLeft = geo.NewPoint(600, 400)
	getNode[9].TopLeft = geo.NewPoint(800, 400)
	getNode[10].TopLeft = geo.NewPoint(1000, 400)
	getNode[11].TopLeft = geo.NewPoint(0, 600)
	getNode[12].TopLeft = geo.NewPoint(200, 600)
	getNode[13].TopLeft = geo.NewPoint(400, 600)
	getNode[14].TopLeft = geo.NewPoint(900, 600)
	getNode[15].TopLeft = geo.NewPoint(1100, 600)

	connectUndirected := func(a, b int) *layoutgraph.Edge {
		e := g.Connect(getNode[a], getNode[b])
		return e
	}
	connectDirected := func(a, b int) *layoutgraph.Edge {
		e := connectUndirected(a, b)
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}
	connectBidirectional := func(a, b int) *layoutgraph.Edge {
		e := connectDirected(a, b)
		e.SourceArrowhead = layoutgraph.TriangleArrowhead
		return e
	}
	connectBidirectional(0, 1)
	connectBidirectional(1, 2)
	connectBidirectional(2, 3)
	connectBidirectional(0, 4)
	connectBidirectional(4, 5)
	connectBidirectional(5, 4)
	connectDirected(6, 4)
	connectDirected(4, 7)
	connectUndirected(5, 9)
	connectUndirected(9, 10)
	connectDirected(7, 8)
	connectDirected(11, 6)
	connectDirected(12, 6)
	connectDirected(7, 13)
	connectUndirected(10, 14)
	connectUndirected(10, 15)
	return g, getNode
}

func TestExtractTreesMixed(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g, getNode := createMixedTreeGraph()
	if len(g.Nodes) != 16 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 16 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}

	nodeToTreeRoots := mustExtractTrees(t, ctx, g)
	//             ┌───┐               ┌───┐
	//  g:         │ 4 ◄──────x2───────► 5 │
	//             └───┘               └───┘
	if len(g.Nodes) != 2 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}
	isRemainingNode := func(n *layoutgraph.Node) bool {
		return n.ID == 4 || n.ID == 5
	}
	for _, n := range g.Nodes {
		if !isRemainingNode(n) {
			t.Fatalf("Unexpected node %v\n", n)
		}
	}
	for _, e := range g.Edges {
		if !isRemainingNode(e.To) || !isRemainingNode(e.From) {
			t.Fatalf("Unexpected edge %v->%v\n", e.To.ID, e.From.ID)
		}
	}
	// nodeToTreeRoots:
	//  ┌───┐ ┌───┐ ┌───┐ ┌───┐
	//  │ 4 │:│ 0 │,│ 6 │,│ 7 │
	//  └───┘ └───┘ └───┘ └───┘
	//  ┌───┐ ┌───┐
	//  │ 5 │:│ 9 │
	//  └───┘ └───┘
	if len(nodeToTreeRoots) != 2 {
		t.Fatalf("Unexpected nodeToTreeRoots size %v\n", len(nodeToTreeRoots))
	}

	if len(nodeToTreeRoots[getNode[4]]) != 3 {
		t.Fatalf("Unexpected nodeToTreeRoots[4] size %v\n", len(nodeToTreeRoots[getNode[4]]))
	}
	for _, root := range nodeToTreeRoots[getNode[4]] {
		isExpectedID := slices.Contains([]layoutgraph.EntityID{0, 6, 7}, root.Node.ID)
		if !isExpectedID {
			t.Fatalf("Unexpected node4 root:%v\n", root.Node.ID)
		}
	}
	if len(nodeToTreeRoots[getNode[5]]) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots[5] size %v\n", len(nodeToTreeRoots[getNode[5]]))
	}
	for _, root := range nodeToTreeRoots[getNode[5]] {
		if root.Node != getNode[9] {
			t.Fatalf("Unexpected node5 root:%v\n", root.Node.ID)
		}
	}
}

func TestPlaceTreesMixed(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g, getNode := createMixedTreeGraph()
	if len(g.Nodes) != 16 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 16 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}
	//             ┌───┐   ┌───┐   ┌───┐   ┌───┐
	//             │ 0 ◄───► 1 ◄───► 2 ◄───► 3 │
	//             └─▲─┘   └───┘   └───┘   └───┘
	//               │
	//             ┌─▼─┐               ┌───┐
	//             │ 4 ◄──────x2───────► 5 │
	//             └▲─┬┘               └─┬─┘
	//           ┌──┘ └──┐               │
	//         ┌─┴─┐   ┌─▼─┐   ┌───┐   ┌─┴─┐   ┌───┐
	//         │ 6 │   │ 7 ├───► 8 │   │ 9 ├───┤10 │
	//         └▲─▲┘   └─┬─┘   └───┘   └───┘   └┬─┬┘
	//   ┌──────┘┌┘      │                   ┌──┘ └──┐
	// ┌─┴─┐   ┌─┴─┐   ┌─▼─┐               ┌─┴─┐   ┌─┴─┐
	// │11 │   │12 │   │13 │               │14 │   │15 │
	// └───┘   └───┘   └───┘               └───┘   └───┘
	g.Trees = mustExtractTrees(t, ctx, g)
	//             ┌───┐               ┌───┐
	//             │ 4 ◄──────x2───────► 5 │
	//             └───┘               └───┘
	if err := Place(ctx, g, nil); err != nil {
		t.Fatal(err)
	}
	//             ┌───┐   ┌───┐
	//             │12 │   │11 │
	//             └─┬─┘   └─┬─┘
	//               └──┐ ┌──┘
	//                 ┌▼─▼┐
	//                 │ 6 │
	// ┌───┐           └─┬─┘                                     ┌───┐
	// │13 ◄─┐           │                                     ┌─┤14 │
	// └───┘ │ ┌───┐   ┌─▼─┐               ┌───┐  ┌───┐  ┌───┐ │ └───┘
	//       ├─┤ 7 ◄───┤ 4 ◄──────x2───────► 5 ├──┤ 9 ├──┤10 ├─┤
	// ┌───┐ │ └───┘   └─▲─┘               └───┘  └───┘  └───┘ │ ┌───┐
	// │ 8 ◄─┘           │                                     └─┤15 │
	// └───┘           ┌─▼─┐                                     └───┘
	//                 │ 0 │
	//                 └─▲─┘
	//                   │
	//                 ┌─▼─┐
	//                 │ 1 │
	//                 └─▲─┘
	//                   │
	//                 ┌─▼─┐
	//                 │ 2 │
	//                 └─▲─┘
	//                   │
	//                 ┌─▼─┐
	//                 │ 3 │
	//                 └───┘

	assertOrientation := func(a, b int, expectedOrientation geo.Orientation) {
		o := getNode[a].Orientation(getNode[b])
		if o == geo.NONE {
			t.Fatalf("Failed to get orientation of node %v and node %v\n", a, b)
		}
		if o != expectedOrientation {
			t.Fatalf(
				"Expected node %v at %v to be %v of node %v at %v, but it was %v\n",
				a, getNode[a].TopLeft, expectedOrientation.ToString(), b, getNode[b].TopLeft, o.ToString(),
			)
		}
	}
	assertOrientation(4, 5, geo.Left)
	assertOrientation(4, 0, geo.Top)
	assertOrientation(0, 1, geo.Top)
	assertOrientation(1, 2, geo.Top)
	assertOrientation(2, 3, geo.Top)
	assertOrientation(4, 6, geo.Bottom)
	assertOrientation(6, 12, geo.Bottom)
	assertOrientation(6, 11, geo.Bottom)
	assertOrientation(4, 7, geo.Right)
	assertOrientation(7, 8, geo.Right)
	assertOrientation(7, 13, geo.Right)
	assertOrientation(5, 9, geo.Left)
	assertOrientation(9, 10, geo.Left)
	assertOrientation(10, 14, geo.Left)
	assertOrientation(10, 15, geo.Left)

	assertPosition := func(n int, x, y float64) {
		if !getNode[n].TopLeft.Equals(geo.NewPoint(x, y)) {
			t.Fatalf("Expected node %v position(%v,%v), but got %v\n", n, x, y, getNode[n].TopLeft.ToString())
		}
	}
	assertPosition(0, 300, 400)
	assertPosition(1, 300, 600)
	assertPosition(2, 300, 800)
	assertPosition(3, 300, 1000)
	assertPosition(4, 300, 200)
	assertPosition(5, 800, 200)
	assertPosition(6, 300, 0)
	assertPosition(7, 100, 200)
	assertPosition(8, -100, 275)
	assertPosition(9, 1000, 200)
	assertPosition(10, 1200, 200)
	assertPosition(11, 375, -200)
	assertPosition(12, 225, -200)
	assertPosition(13, -100, 125)
	assertPosition(14, 1400, 125)
	assertPosition(15, 1400, 275)
}

func createDirectedPairGraph() (*layoutgraph.Graph, map[int]*layoutgraph.Node) {
	// ┌───┐   ┌───┐
	// │ 0 ├───► 1 │
	// └───┘   └───┘
	getNode := make(map[int]*layoutgraph.Node)
	g := layoutgraph.NewGraph()
	for i := 0; i < 2; i++ {
		n := makeNode(i)
		g.AddNodeUnchecked(n)
		getNode[i] = n
		g.Containers[nil] = append(g.Containers[nil], n)
	}

	getNode[0].TopLeft = geo.NewPoint(0, 0)
	getNode[1].TopLeft = geo.NewPoint(200, 0)

	connectDirected := func(a, b int) *layoutgraph.Edge {
		e := g.Connect(getNode[a], getNode[b])
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}
	connectDirected(0, 1)
	return g, getNode
}

func TestExtractDirectedPair(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g, getNode := createDirectedPairGraph()
	if len(g.Nodes) != 2 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}

	nodeToTreeRoots := mustExtractTrees(t, ctx, g)
	//    ┌───┐
	//  g:│ 0 │
	//    └───┘
	if len(g.Nodes) != 1 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}
	isRemainingNode := func(n *layoutgraph.Node) bool {
		return n.ID == 0
	}
	for _, n := range g.Nodes {
		if !isRemainingNode(n) {
			t.Fatalf("Unexpected node %v\n", n)
		}
	}
	for _, e := range g.Edges {
		if !isRemainingNode(e.To) || !isRemainingNode(e.From) {
			t.Fatalf("Unexpected edge %v->%v\n", e.To.ID, e.From.ID)
		}
	}
	if len(nodeToTreeRoots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots size %v\n", len(nodeToTreeRoots))
	}
	roots := nodeToTreeRoots[getNode[0]]
	if len(roots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots[0] size %v\n", len(roots))
	}
	root := roots[0]
	if root.Node != getNode[1] {
		t.Fatalf("Unexpected node0 root:%v\n", root.Node.ID)
	}
	if len(root.Children) != 0 {
		t.Fatalf("Unexpected node0 root with children:%v\n", root.Node.ID)
	}
}

func createLineGraph() (*layoutgraph.Graph, map[int]*layoutgraph.Node) {
	// ┌───┐   ┌───┐   ┌───┐
	// │ 0 ├───► 1 ├───► 2 │
	// └───┘   └───┘   └───┘
	getNode := make(map[int]*layoutgraph.Node)
	g := layoutgraph.NewGraph()
	for i := 0; i < 3; i++ {
		n := makeNode(i)
		g.AddNodeUnchecked(n)
		getNode[i] = n
		g.Containers[nil] = append(g.Containers[nil], n)
	}

	getNode[0].TopLeft = geo.NewPoint(0, 0)
	getNode[1].TopLeft = geo.NewPoint(200, 0)
	getNode[2].TopLeft = geo.NewPoint(400, 0)

	connectDirected := func(a, b int) *layoutgraph.Edge {
		e := g.Connect(getNode[a], getNode[b])
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}
	connectDirected(0, 1)
	connectDirected(1, 2)
	return g, getNode
}

func TestExtractLineGraph(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g, getNode := createLineGraph()
	if len(g.Nodes) != 3 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}

	nodeToTreeRoots := mustExtractTrees(t, ctx, g)
	//    ┌───┐
	//  g:│ 0 │
	//    └───┘
	if len(g.Nodes) != 1 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}
	isRemainingNode := func(n *layoutgraph.Node) bool {
		return n.ID == 0
	}
	for _, n := range g.Nodes {
		if !isRemainingNode(n) {
			t.Fatalf("Unexpected node %v\n", n)
		}
	}
	for _, e := range g.Edges {
		if !isRemainingNode(e.To) || !isRemainingNode(e.From) {
			t.Fatalf("Unexpected edge %v->%v\n", e.To.ID, e.From.ID)
		}
	}
	if len(nodeToTreeRoots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots size %v\n", len(nodeToTreeRoots))
	}
	roots := nodeToTreeRoots[getNode[0]]
	if len(roots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots[0] size %v\n", len(roots))
	}
	for _, root := range roots {
		if root.Node != getNode[1] {
			t.Fatalf("Unexpected node0 root:%v\n", root.Node.ID)
		}
	}
}

func createDirectedGraph1() (*layoutgraph.Graph, map[int]*layoutgraph.Node) {
	//          ┌───┐   ┌───┐
	//          │ 0 │   │ 1 │
	//          └─┬─┘   └─┬─┘
	//            │       │
	//  ┌───┐   ┌─▼─┐   ┌─▼─┐   ┌───┐
	//  │ 2 ├───► 3 ◄───┤ 4 ◄───┤ 5 │
	//  └───┘   └───┘   └───┘   └───┘
	getNode := make(map[int]*layoutgraph.Node)
	g := layoutgraph.NewGraph()
	for i := 0; i < 6; i++ {
		n := makeNode(i)
		g.AddNodeUnchecked(n)
		getNode[i] = n
		g.Containers[nil] = append(g.Containers[nil], n)
	}

	getNode[0].TopLeft = geo.NewPoint(200, 0)
	getNode[1].TopLeft = geo.NewPoint(400, 0)
	getNode[2].TopLeft = geo.NewPoint(0, 200)
	getNode[3].TopLeft = geo.NewPoint(200, 200)
	getNode[4].TopLeft = geo.NewPoint(400, 200)
	getNode[5].TopLeft = geo.NewPoint(600, 200)

	connectDirected := func(a, b int) *layoutgraph.Edge {
		e := g.Connect(getNode[a], getNode[b])
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}

	connectDirected(0, 3)
	connectDirected(1, 4)
	connectDirected(2, 3)
	connectDirected(4, 3)
	connectDirected(5, 4)
	return g, getNode
}

func TestExtractDirectedGraph1(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g, getNode := createDirectedGraph1()
	if len(g.Nodes) != 6 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 5 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}

	//          ┌───┐   ┌───┐
	//          │ 0 │   │ 1 │
	//          └─┬─┘   └─┬─┘
	//            │       │
	//  ┌───┐   ┌─▼─┐   ┌─▼─┐   ┌───┐
	//  │ 2 ├───► 3 ◄───┤ 4 ◄───┤ 5 │
	//  └───┘   └───┘   └───┘   └───┘
	nodeToTreeRoots := mustExtractTrees(t, ctx, g)
	//          ┌───┐
	//  g:      │ 3 │
	//          └───┘
	if len(g.Nodes) != 1 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}
	isRemainingNode := func(n *layoutgraph.Node) bool {
		return n.ID == 3
	}
	for _, n := range g.Nodes {
		if !isRemainingNode(n) {
			t.Fatalf("Unexpected node %v\n", n)
		}
	}
	for _, e := range g.Edges {
		if !isRemainingNode(e.To) || !isRemainingNode(e.From) {
			t.Fatalf("Unexpected edge %v->%v\n", e.To.ID, e.From.ID)
		}
	}
	// nodeToTreeRoots:
	//  ┌───┐ ┌───┐ ┌───┐ ┌───┐
	//  │ 3 │:│ 0 │,│ 2 │,│ 4 │
	//  └───┘ └───┘ └───┘ └───┘
	if len(nodeToTreeRoots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots size %v\n", len(nodeToTreeRoots))
	}

	roots := nodeToTreeRoots[getNode[3]]
	if len(roots) != 3 {
		t.Fatalf("Unexpected nodeToTreeRoots[3] size %v\n", len(roots))
	}
	for _, root := range roots {
		isExpectedID := slices.Contains([]layoutgraph.EntityID{0, 2, 4}, root.Node.ID)
		if !isExpectedID {
			t.Fatalf("Unexpected node4 root:%v\n", root.Node.ID)
		}
	}
}

func createDirectedGraph2() (*layoutgraph.Graph, map[int]*layoutgraph.Node) {
	//          ┌───┐   ┌───┐
	//          │ 0 │   │ 1 │
	//          └─┬─┘   └─┬─┘
	//            │       │
	//  ┌───┐   ┌─▼─┐   ┌─▼─┐   ┌───┐
	//  │ 2 ◄───┤ 3 ◄───┤ 4 ◄───┤ 5 │
	//  └───┘   └───┘   └───┘   └───┘
	getNode := make(map[int]*layoutgraph.Node)
	g := layoutgraph.NewGraph()
	for i := 0; i < 6; i++ {
		n := makeNode(i)
		g.AddNodeUnchecked(n)
		getNode[i] = n
		g.Containers[nil] = append(g.Containers[nil], n)
	}

	getNode[0].TopLeft = geo.NewPoint(200, 0)
	getNode[1].TopLeft = geo.NewPoint(400, 0)
	getNode[2].TopLeft = geo.NewPoint(0, 200)
	getNode[3].TopLeft = geo.NewPoint(200, 200)
	getNode[4].TopLeft = geo.NewPoint(400, 200)
	getNode[5].TopLeft = geo.NewPoint(600, 200)

	connectDirected := func(a, b int) *layoutgraph.Edge {
		e := g.Connect(getNode[a], getNode[b])
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}

	connectDirected(0, 3)
	connectDirected(1, 4)
	connectDirected(3, 2)
	connectDirected(4, 3)
	connectDirected(5, 4)
	return g, getNode
}

func TestExtractDirectedGraph2(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g, getNode := createDirectedGraph2()
	if len(g.Nodes) != 6 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 5 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}

	//          ┌───┐   ┌───┐
	//          │ 0 │   │ 1 │
	//          └─┬─┘   └─┬─┘
	//            │       │
	//  ┌───┐   ┌─▼─┐   ┌─▼─┐   ┌───┐
	//  │ 2 ◄───┤ 3 ◄───┤ 4 ◄───┤ 5 │
	//  └───┘   └───┘   └───┘   └───┘
	nodeToTreeRoots := mustExtractTrees(t, ctx, g)
	//  ┌───┐
	//g:│ 2 │
	//  └───┘
	if len(g.Nodes) != 1 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}
	isRemainingNode := func(n *layoutgraph.Node) bool {
		return n.ID == 2
	}
	for _, n := range g.Nodes {
		if !isRemainingNode(n) {
			t.Fatalf("Unexpected node %v\n", n)
		}
	}
	for _, e := range g.Edges {
		if !isRemainingNode(e.To) || !isRemainingNode(e.From) {
			t.Fatalf("Unexpected edge %v->%v\n", e.To.ID, e.From.ID)
		}
	}
	// nodeToTreeRoots:
	//  ┌───┐ ┌───┐
	//  │ 2 │:│ 3 │
	//  └───┘ └───┘
	if len(nodeToTreeRoots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots size %v\n", len(nodeToTreeRoots))
	}

	roots := nodeToTreeRoots[getNode[2]]
	if len(roots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots[2] size %v\n", len(roots))
	}
	root := roots[0]
	if root.Node.ID != 3 {
		t.Fatalf("Unexpected node2 root:%v\n", root.Node.ID)
	}
	if size := mustTreeSize(t, root); size != 5 {
		t.Fatalf("Unexpected node2 root:%v size %v\n", root.Node.ID, size)
	}
}

func createDirectedGraph3() (*layoutgraph.Graph, map[int]*layoutgraph.Node) {
	//                  ┌───┐
	//                  │ 0 │
	//                  └─┬─┘
	//                    │
	//                  ┌─▼─┐
	//                  │ 1 │
	//                  └─┬─┘
	//                    │
	//  ┌───┐   ┌───┐   ┌─▼─┐   ┌───┐   ┌───┐
	//  │ 2 ◄───┤ 3 ◄───┤ 4 ◄───┤ 5 ◄───┤ 6 │
	//  └───┘   └───┘   └───┘   └───┘   └───┘
	getNode := make(map[int]*layoutgraph.Node)
	g := layoutgraph.NewGraph()
	for i := 0; i < 7; i++ {
		n := makeNode(i)
		g.AddNodeUnchecked(n)
		getNode[i] = n
		g.Containers[nil] = append(g.Containers[nil], n)
	}

	getNode[0].TopLeft = geo.NewPoint(600, 0)
	getNode[1].TopLeft = geo.NewPoint(600, 200)
	getNode[2].TopLeft = geo.NewPoint(0, 400)
	getNode[3].TopLeft = geo.NewPoint(200, 400)
	getNode[4].TopLeft = geo.NewPoint(400, 400)
	getNode[5].TopLeft = geo.NewPoint(600, 400)
	getNode[6].TopLeft = geo.NewPoint(800, 400)

	connectDirected := func(a, b int) *layoutgraph.Edge {
		e := g.Connect(getNode[a], getNode[b])
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}

	connectDirected(0, 1)
	connectDirected(1, 4)
	connectDirected(6, 5)
	connectDirected(5, 4)
	connectDirected(4, 3)
	connectDirected(3, 2)
	return g, getNode
}

func TestExtractDirectedGraph3(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g, getNode := createDirectedGraph3()
	if len(g.Nodes) != 7 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 6 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}

	//                  ┌───┐
	//                  │ 0 │
	//                  └─┬─┘
	//                    │
	//                  ┌─▼─┐
	//                  │ 1 │
	//                  └─┬─┘
	//                    │
	//  ┌───┐   ┌───┐   ┌─▼─┐   ┌───┐   ┌───┐
	//  │ 2 ◄───┤ 3 ◄───┤ 4 ◄───┤ 5 ◄───┤ 6 │
	//  └───┘   └───┘   └───┘   └───┘   └───┘
	nodeToTreeRoots := mustExtractTrees(t, ctx, g)
	//  ┌───┐
	//g:│ 2 │
	//  └───┘
	if len(g.Nodes) != 1 {
		t.Fatalf("Unexpected g.Nodes size %v\n", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Fatalf("Unexpected g.Edges size %v\n", len(g.Edges))
	}
	isRemainingNode := func(n *layoutgraph.Node) bool {
		return n.ID == 2
	}
	for _, n := range g.Nodes {
		if !isRemainingNode(n) {
			t.Fatalf("Unexpected node %v\n", n)
		}
	}
	for _, e := range g.Edges {
		if !isRemainingNode(e.To) || !isRemainingNode(e.From) {
			t.Fatalf("Unexpected edge %v->%v\n", e.To.ID, e.From.ID)
		}
	}
	// nodeToTreeRoots:
	//  ┌───┐ ┌───┐
	//  │ 2 │:│ 3 │
	//  └───┘ └───┘
	if len(nodeToTreeRoots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots size %v\n", len(nodeToTreeRoots))
	}

	roots := nodeToTreeRoots[getNode[2]]
	if len(roots) != 1 {
		t.Fatalf("Unexpected nodeToTreeRoots[2] size %v\n", len(roots))
	}
	root := roots[0]
	if root.Node.ID != 3 {
		t.Fatalf("Unexpected node2 root:%v\n", root.Node.ID)
	}
	if size := mustTreeSize(t, root); size != 6 {
		t.Fatalf("Unexpected node2 root:%v size %v\n", root.Node.ID, size)
	}
}

func TestRootsByTreeDirection(t *testing.T) {
	connect := func(a, b *layoutgraph.Node, sourceArrowhead, targetArrowhead bool) *layoutgraph.Edge {
		e := layoutgraph.NewEdge(a, b)
		if sourceArrowhead {
			e.SourceArrowhead = layoutgraph.TriangleArrowhead
		}
		if targetArrowhead {
			e.TargetArrowhead = layoutgraph.TriangleArrowhead
		}
		a.AddIncidentEdgeUnchecked(e)
		b.AddIncidentEdgeUnchecked(e)
		return e
	}
	n5 := layoutgraph.NewNode(5, 10, 10)
	root := layoutgraph.NewTree(layoutgraph.NewNode(1, 10, 10))
	c1 := layoutgraph.NewTree(layoutgraph.NewNode(2, 10, 10))
	c2 := layoutgraph.NewTree(layoutgraph.NewNode(3, 10, 10))
	c3 := layoutgraph.NewTree(layoutgraph.NewNode(4, 10, 10))

	guard, err := newWorkGuard(context.Background(), "TreeDirection")
	if err != nil {
		t.Fatal(err)
	}
	if err := addTreeChild(root, c1, guard); err != nil {
		t.Fatal(err)
	}
	if err := addTreeChild(root, c2, guard); err != nil {
		t.Fatal(err)
	}
	if err := addTreeChild(root, c3, guard); err != nil {
		t.Fatal(err)
	}

	root.SentinelEdge = connect(n5, root.Node, true, true)
	c1.SentinelEdge = connect(root.Node, c1.Node, false, false)
	c2.SentinelEdge = connect(root.Node, c2.Node, false, true)
	c3.SentinelEdge = connect(c2.Node, c3.Node, false, true)

	byDirection, err := rootsByTreeDirection([]*layoutgraph.Tree{root}, guard)
	if err != nil {
		t.Fatal(err)
	}

	if _, exists := byDirection[Inwards]; !exists {
		t.Fatalf("Expected tree direction to be Inwards, got=%v.", byDirection)
	}

	if len(byDirection) != 1 {
		t.Fatalf("Expected only one tree direction")
	}
}

func TestSqlTableTree(t *testing.T) {
	g, _ := createDirectedGraph3()
	g.Nodes[3].SetShape(shape.TABLE_TYPE)

	ctx := withTestLogger(context.Background(), t)
	nodeToTrees := mustExtractTrees(t, ctx, g)

	assert.Equal(t, 0, len(nodeToTrees))
}
