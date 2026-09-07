package placementcost

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func mustSplitSubgraphs(t *testing.T, ctx context.Context, graph *layoutgraph.Graph, options layoutgraph.SplitOptions) []*layoutgraph.Graph {
	t.Helper()
	graphs, err := graph.SplitSubgraphs(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	return graphs
}

func mustSymmetry(t *testing.T, ctx context.Context, node *layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction, checkNeighbors bool) float64 {
	t.Helper()
	symmetry, err := nodeSymmetry(ctx, node, edgeAbductions, checkNeighbors)
	if err != nil {
		t.Fatal(err)
	}
	return symmetry
}

func mustComputeSymmetryScore(t *testing.T, node *layoutgraph.Node, neighbors []*layoutgraph.Node) (float64, map[*layoutgraph.Node]struct{}) {
	t.Helper()
	score, matched, err := computeSymmetryScore(context.Background(), node, neighbors)
	if err != nil {
		t.Fatal(err)
	}
	return score, matched
}

func TestIsLocallySymmetrical(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()
	g.Clusters = make(map[*layoutgraph.Node]*layoutgraph.Cluster, 0)

	a := layoutgraph.NewNode(1, 5, 5)
	b := layoutgraph.NewNode(2, 5, 5)
	c := layoutgraph.NewNode(3, 5, 5)

	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)
	g.AddNodeUnchecked(c)

	// +----+            +----+           +-----+
	// | a  |            | c  |           | b   |
	// |    +------------+    +-----------+     |
	// +----+            +----+           +-----+

	a.TopLeft = geo.NewPoint(0, 2)
	b.TopLeft = geo.NewPoint(0, 10)
	c.TopLeft = geo.NewPoint(0, 4)

	g.Connect(a, c)
	g.Connect(b, c)
	g.ComputeCellSize()

	assert.Equal(t, 0.0, mustSymmetry(t, ctx, a, nil, true))
	assert.Equal(t, 0.0, mustSymmetry(t, ctx, b, nil, true))
	assert.Equal(t, 0.0, mustSymmetry(t, ctx, c, nil, true))

	c.TopLeft = geo.NewPoint(0, 6)

	assert.Equal(t, 1.0, mustSymmetry(t, ctx, a, nil, true))
	assert.Equal(t, 1.0, mustSymmetry(t, ctx, b, nil, true))
	assert.Equal(t, 1.0, mustSymmetry(t, ctx, c, nil, true))

	// +----+
	// |    |
	// |    |
	// +-+--+
	//   |
	//   |
	//   |
	//   |
	// +-+--+             +-----+
	// |    |             |     |
	// |    +-------------+     |
	// +----+             +-----+
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(10, 10)
	c.TopLeft = geo.NewPoint(0, 10)
	if mustSymmetry(t, ctx, a, nil, true) != 0 {
		t.Fatal("Symmetry fail")
	}
	if mustSymmetry(t, ctx, b, nil, true) != 0 {
		t.Fatal("Symmetry fail")
	}
	if mustSymmetry(t, ctx, c, nil, true) != 0 {
		t.Fatal("Symmetry fail")
	}

	//             +----+
	//    +--------+    +-----------+
	//    |        |    |           |
	//    |        +----+           |
	//    |                         |
	//    |                         |
	//    |                         |
	// +--+-+                   +---+-+
	// |    |                   |     |
	// |    |                   |     |
	// +----+                   +-----+
	a.TopLeft = geo.NewPoint(0, 10)
	b.TopLeft = geo.NewPoint(20, 10)
	c.TopLeft = geo.NewPoint(10, 0)
	if mustSymmetry(t, ctx, a, nil, true) != 0.25 {
		t.Fatal("Symmetry fail")
	}
	if mustSymmetry(t, ctx, b, nil, true) != 0.25 {
		t.Fatal("Symmetry fail")
	}
	if mustSymmetry(t, ctx, c, nil, true)-0.166 > 0.1 {
		t.Fatal("Symmetry fail")
	}

	c.TopLeft = geo.NewPoint(10, 0)

	// Add a non symmetric node
	d := layoutgraph.NewNode(1, 5, 5)
	d.TopLeft = geo.NewPoint(30, 30)
	g.AddNodeUnchecked(d)

	g.Connect(a, d)

	if mustSymmetry(t, ctx, a, nil, true)-0.166 > 0.1 {
		t.Fatal("Symmetry fail")
	}
	if mustSymmetry(t, ctx, b, nil, true)-0.166 > 0.1 {
		t.Fatal("Symmetry fail")
	}
	if mustSymmetry(t, ctx, c, nil, true)-0.166 > 0.1 {
		t.Fatal("Symmetry fail")
	}
	if mustSymmetry(t, ctx, d, nil, true) != 0 {
		t.Fatal("Symmetry fail")
	}
}

func TestComputeSymmetryScoreForAlignedNodes(t *testing.T) {
	//                      ┌────────────┐
	//                      │     n2     │
	//                      │            │
	//                      └──────┬─────┘
	// ┌────────────┐       ┌──────┴─────┐       ┌────────────┐
	// │    n4      ├───────┤     n1     ├───────┤    n3      │
	// │            │       │            │       │            │
	// └────────────┘       └──────┬─────┘       └────────────┘
	//                      ┌──────┴─────┐
	//                      │            │
	//                      │     n5     │
	//                      └────────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(200, 200)
	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	n2.TopLeft = geo.NewPoint(200, 0)
	n3 := g.AddNode(layoutgraph.NewNode(3, 100, 100))
	n3.TopLeft = geo.NewPoint(400, 200)
	n4 := g.AddNode(layoutgraph.NewNode(4, 100, 100))
	n4.TopLeft = geo.NewPoint(0, 200)
	n5 := g.AddNode(layoutgraph.NewNode(5, 100, 100))
	n5.TopLeft = geo.NewPoint(200, 400)

	g.AddNodeToContainer(nil, n1)
	g.AddNodeToContainer(nil, n2)
	g.AddNodeToContainer(nil, n3)
	g.AddNodeToContainer(nil, n4)
	g.AddNodeToContainer(nil, n5)

	adjacentNodes := []*layoutgraph.Node{n2, n3, n4, n5}
	score, matched := mustComputeSymmetryScore(t, n1, adjacentNodes)

	if score != 4.0 {
		t.Fatalf("expected score=4.0, got=%f", score)
	}
	for _, n := range adjacentNodes {
		if _, exists := matched[n]; !exists {
			t.Fatalf("node %s not matched", n.DebugID())
		}
	}
}

func TestComputeSymmetryScoreMisalignedNodes(t *testing.T) {
	// ┌───────────┐                ┌───────────┐
	// │           │                │           │
	// │   n2      │                │     n3    │
	// │           │                │           │
	// └──────┬────┘                └──────┬────┘
	//        │       ┌───────────┐        │
	//        └───────┤           ├────────┘
	//                │    n1     │
	//        ┌───────┤           ├────────┐
	//        │       └───────────┘        │
	//  ┌─────┴─────┐                ┌─────┴─────┐
	//  │           │                │           │
	//  │    n5     │                │    n4     │
	//  │           │                │           │
	//  └───────────┘                └───────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(200, 200)
	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	n2.TopLeft = geo.NewPoint(0, 0)
	n3 := g.AddNode(layoutgraph.NewNode(3, 100, 100))
	n3.TopLeft = geo.NewPoint(400, 0)
	n4 := g.AddNode(layoutgraph.NewNode(4, 100, 100))
	n4.TopLeft = geo.NewPoint(400, 400)
	n5 := g.AddNode(layoutgraph.NewNode(5, 100, 100))
	n5.TopLeft = geo.NewPoint(0, 400)

	g.AddNodeToContainer(nil, n1)
	g.AddNodeToContainer(nil, n2)
	g.AddNodeToContainer(nil, n3)
	g.AddNodeToContainer(nil, n4)
	g.AddNodeToContainer(nil, n5)

	adjacentNodes := []*layoutgraph.Node{n2, n3, n4, n5}
	score, matched := mustComputeSymmetryScore(t, n1, adjacentNodes)

	if score != 1.0 {
		t.Fatalf("expected score=1.0, got=%f", score)
	}
	for _, n := range adjacentNodes {
		if _, exists := matched[n]; !exists {
			t.Fatalf("node %s not matched", n.DebugID())
		}
	}
}

func TestComputeSymmetryScoreOddMisalignedNodes(t *testing.T) {
	// ┌───────────┐                ┌───────────┐
	// │           │                │           │
	// │   n2      │                │     n3    │
	// │           │                │           │
	// └──────┬────┘                └──────┬────┘
	//        │       ┌───────────┐        │
	//        └───────┤           ├────────┘
	//                │    n1     │
	//                │           ├────────┐
	//                └───────────┘        │
	//                               ┌─────┴─────┐
	//                               │           │
	//                               │    n4     │
	//                               │           │
	//                               └───────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(200, 200)
	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	n2.TopLeft = geo.NewPoint(0, 0)
	n3 := g.AddNode(layoutgraph.NewNode(3, 100, 100))
	n3.TopLeft = geo.NewPoint(400, 0)
	n4 := g.AddNode(layoutgraph.NewNode(4, 100, 100))
	n4.TopLeft = geo.NewPoint(400, 400)

	g.AddNodeToContainer(nil, n1)
	g.AddNodeToContainer(nil, n2)
	g.AddNodeToContainer(nil, n3)
	g.AddNodeToContainer(nil, n4)

	// in this case, n2-n3 is preferred over n3-n4 because of the node order here
	// if this order changes, the output match also changes
	adjacentNodes := []*layoutgraph.Node{n2, n3, n4}
	score, matched := mustComputeSymmetryScore(t, n1, adjacentNodes)

	if score != 0.5 {
		t.Fatalf("expected score=0.5, got=%f", score)
	}
	if _, exists := matched[n4]; exists {
		t.Fatal("didn't expect to match n4")
	}
	if _, exists := matched[n2]; !exists {
		t.Fatal("expected to match n2")
	}
	if _, exists := matched[n3]; !exists {
		t.Fatal("expected to match n3")
	}
}

func TestComputeSymmetryScoreOddMisalignedNodesWithObstruction(t *testing.T) {
	//          ┌──────────────────────┐
	//          │                      │
	// ┌────────┴──┐     ┌─────┐       │         ┌───────────┐
	// │           │     │     │       │         │           │
	// │     n2    │     │     │       │         │     n3    │
	// │           │     │     │       │         │           │
	// └───────────┘     │     │       │         └──────┬────┘
	//                   │     │   ┌───┴───────┐        │
	//                   │     │   │           ├────────┘
	//                   │ n5  │   │   n1      │
	//                   │     │   │           ├────────┐
	//                   │     │   └───────────┘        │
	//                   │     │                  ┌─────┴─────┐
	//                   │     │                  │           │
	//                   │     │                  │    n4     │
	//                   │     │                  │           │
	//                   └─────┘                  └───────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(400, 200)
	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	n2.TopLeft = geo.NewPoint(0, 0)
	n3 := g.AddNode(layoutgraph.NewNode(3, 100, 100))
	n3.TopLeft = geo.NewPoint(600, 0)
	n4 := g.AddNode(layoutgraph.NewNode(4, 100, 100))
	n4.TopLeft = geo.NewPoint(600, 400)
	n5 := g.AddNode(layoutgraph.NewNode(5, 100, 500))
	n5.TopLeft = geo.NewPoint(200, 0)

	g.AddNodeToContainer(nil, n1)
	g.AddNodeToContainer(nil, n2)
	g.AddNodeToContainer(nil, n3)
	g.AddNodeToContainer(nil, n4)
	g.AddNodeToContainer(nil, n5)

	// in this case, n3-n4 is used because n2-n3 has an obstruction
	adjacentNodes := []*layoutgraph.Node{n2, n3, n4}
	score, matched := mustComputeSymmetryScore(t, n1, adjacentNodes)

	if score != 0.5 {
		t.Fatalf("expected score=0.5, got=%f", score)
	}
	if _, exists := matched[n2]; exists {
		t.Fatal("didn't expect to match n2")
	}
	if _, exists := matched[n4]; !exists {
		t.Fatal("expected to match n4")
	}
	if _, exists := matched[n3]; !exists {
		t.Fatal("expected to match n3")
	}
}

func TestComputeSymmetryScoreObstruction(t *testing.T) {
	//                            ┌─────────────────────────────────┐
	//                            │                                 │
	//                            │                                 │
	// ┌───────────┐     ┌────────┴──┐     ┌───────────┐       ┌────┴──────┐
	// │           │     │           │     │           │       │           │
	// │    n2     ├─────┤    n1     │     │    n3     │       │    n4     │
	// │           │     │           │     │           │       │           │
	// └───────────┘     └───────────┘     └───────────┘       └───────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(200, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 0))
	n2.TopLeft = geo.NewPoint(0, 0)
	n3 := g.AddNode(layoutgraph.NewNode(3, 100, 0))
	n3.TopLeft = geo.NewPoint(400, 0)
	n4 := g.AddNode(layoutgraph.NewNode(4, 100, 100))
	n4.TopLeft = geo.NewPoint(600, 0)

	g.AddNodeToContainer(nil, n1)
	g.AddNodeToContainer(nil, n2)
	g.AddNodeToContainer(nil, n3)
	g.AddNodeToContainer(nil, n4)

	adjacentNodes := []*layoutgraph.Node{n2, n4}
	score, matched := mustComputeSymmetryScore(t, n1, adjacentNodes)

	if score != 0.0 {
		t.Fatalf("expected score=0.0, got=%f", score)
	}
	if len(matched) != 0 {
		t.Fatal("expected no matches")
	}
}

func TestComputeSymmetryScoreDiagonal(t *testing.T) {
	//                                     ┌───────────┐
	//                                     │           │
	//                                     │     n2    │
	//                                     │           │
	//                                     └──────┬────┘
	//                                            │
	//                   ┌───────────┐            │
	//                   │           │            │
	//        ┌──────────┤    n1     ├────────────┘
	//        │          │           │
	//        │          └───────────┘
	//        │
	// ┌──────┴────┐
	// │           │
	// │    n3     │
	// │           │
	// └───────────┘
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(200, 200)
	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 0))
	n2.TopLeft = geo.NewPoint(400, 0)
	n3 := g.AddNode(layoutgraph.NewNode(3, 100, 0))
	n3.TopLeft = geo.NewPoint(0, 400)

	g.AddNodeToContainer(nil, n1)
	g.AddNodeToContainer(nil, n2)
	g.AddNodeToContainer(nil, n3)

	adjacentNodes := []*layoutgraph.Node{n2, n3}
	score, matched := mustComputeSymmetryScore(t, n1, adjacentNodes)

	if score != 0.0 {
		t.Fatalf("expected score=0.0, got=%f", score)
	}
	if len(matched) != 0 {
		t.Fatal("expected no matches")
	}
}

func TestConsiderSubgraphsWhenCheckingForObstructions(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	// Scoring a split subgraph still considers nodes in peer subgraphs when
	// checking whether an otherwise symmetrical edge is obstructed.

	// two separate subgraphs: a -- b -- c and d -- e
	// when computing the edge length b -- c, it should not consider d
	// ┌───┐       ┌───┐  ┌───┐  ┌───┐
	// │ a ├───────┤ b ├──│ d │──┤ c │
	// └───┘       └───┘  └─┬─┘  └───┘
	//                      │
	//                      │
	//                    ┌─┴─┐
	//                    │ e │
	//                    └───┘
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	a.TopLeft = geo.NewPoint(0, 0)
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	b.TopLeft = geo.NewPoint(50, 0)
	c := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	c.TopLeft = geo.NewPoint(100, 0)
	d := g.AddNode(layoutgraph.NewNode(4, 10, 10))
	d.TopLeft = geo.NewPoint(75, 0)
	e := g.AddNode(layoutgraph.NewNode(5, 10, 10))
	e.TopLeft = geo.NewPoint(75, 50)

	g.AddNodeToContainer(nil, a)
	g.AddNodeToContainer(nil, b)
	g.AddNodeToContainer(nil, c)
	g.AddNodeToContainer(nil, d)
	g.AddNodeToContainer(nil, e)

	g.Connect(a, b)
	g.Connect(b, c)
	g.Connect(d, e)

	subgraphs := mustSplitSubgraphs(t, ctx, g, layoutgraph.SplitOptions{IncludeNears: true})
	assert.Equal(t, 3, len(subgraphs[0].Nodes)) // a, b, c
	assert.Equal(t, 2, len(subgraphs[1].Nodes)) // d, e

	layoutgraph.Nodes(subgraphs[0].Nodes).SetGraphReference(subgraphs[0])
	length, err := NodeEdgeLength(ctx, b, EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: false})
	if err != nil {
		t.Fatal(err)
	}
	// 80 is the length a -- b = 40 + b -- c = 40
	// if the value is greater than that, it is considering the obstruction `d` between b -- c
	if length != 80 {
		t.Fatalf("expected length to be 80, got %f", length)
	}

	symmetry := mustSymmetry(t, ctx, b, nil, false)
	if symmetry != 1 {
		t.Fatalf("expected symmetry to be 1, got %f", symmetry)
	}
}

func TestAxisScore(t *testing.T) {
	// Two
	n1 := layoutgraph.NewNode(1, 101, 10)
	n1.TopLeft = geo.NewPoint(0, 0)

	n2 := layoutgraph.NewNode(2, 100, 10)
	n2.TopLeft = geo.NewPoint(0, 100)

	assert.Equal(t, 1.0, AxisScore(layoutgraph.Nodes([]*layoutgraph.Node{n1, n2})))

	// Three perfectly horizontally aligned
	n3 := layoutgraph.NewNode(3, 100, 10)
	n3.TopLeft = geo.NewPoint(0, 200)

	assert.Equal(t, 1.0, AxisScore(layoutgraph.Nodes([]*layoutgraph.Node{n1, n2, n3})))

	// Three partial aligned
	// First one is perfect aligned, 1
	// Second one has 0.33
	n3.TopLeft = geo.NewPoint(70, 200)

	assert.Equal(t, 1.33/2., AxisScore(layoutgraph.Nodes([]*layoutgraph.Node{n1, n2, n3})))

	// Two partial aligned, one not
	n2.TopLeft = geo.NewPoint(700, 200)

	assert.Equal(t, 0., AxisScore(layoutgraph.Nodes([]*layoutgraph.Node{n1, n2, n3})))

	// Three partial
	n2.TopLeft = geo.NewPoint(-70, 100)

	assert.Equal(t, 0.66/2., AxisScore(layoutgraph.Nodes([]*layoutgraph.Node{n1, n2, n3})))

	// Three partial
	n2.TopLeft = geo.NewPoint(30, 100)
	n3.TopLeft = geo.NewPoint(30, 200)

	assert.Equal(t, (0.66+0.66)/2., AxisScore(layoutgraph.Nodes([]*layoutgraph.Node{n1, n2, n3})))
}

func TestSourceArrowDistanceMatchesEquivalentTargetArrow(t *testing.T) {
	makeGraph := func(sourceOnly bool) *layoutgraph.Graph {
		g := layoutgraph.NewGraph()
		left := g.AddNode(layoutgraph.NewNode(1, 40, 40))
		right := g.AddNode(layoutgraph.NewNode(2, 40, 40))
		left.TopLeft = geo.NewPoint(0, 0)
		right.TopLeft = geo.NewPoint(200, 0)
		g.Directions[nil] = geo.Right

		var edge *layoutgraph.Edge
		if sourceOnly {
			edge = g.Connect(left, right)
			edge.SourceArrowhead = layoutgraph.TriangleArrowhead
		} else {
			edge = g.Connect(right, left)
			edge.TargetArrowhead = layoutgraph.TriangleArrowhead
		}
		g.ComputeCellSize()
		return g
	}

	ctx := withTestLogger(context.Background(), t)
	sourceOnly, err := EdgeLength(ctx, makeGraph(true), EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		t.Fatal(err)
	}
	equivalentTarget, err := EdgeLength(ctx, makeGraph(false), EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		t.Fatal(err)
	}
	if sourceOnly != equivalentTarget {
		t.Fatalf("semantic-equivalent source and target arrows cost %g and %g", sourceOnly, equivalentTarget)
	}
}
