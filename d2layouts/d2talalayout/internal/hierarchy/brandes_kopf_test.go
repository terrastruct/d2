package hierarchy

import (
	"context"
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/lib/log"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func mustCreateAlignmentNodes(
	t *testing.T,
	ctx context.Context,
	byLevel map[int][]*placementNode,
	vertical, horizontal geo.Orientation,
) []*alignmentNode {
	t.Helper()
	nodes, err := createAlignmentNodes(ctx, byLevel, vertical, horizontal)
	require.NoError(t, err)
	return nodes
}

func mustMarkConflicts(
	t *testing.T,
	ctx context.Context,
	byLevel map[int][]*placementNode,
) map[*placementNode]map[*placementNode]struct{} {
	t.Helper()
	conflicts, err := markConflicts(ctx, byLevel)
	require.NoError(t, err)
	return conflicts
}

func mustWorkGuard(t *testing.T, ctx context.Context, location string) *limits.WorkGuard {
	t.Helper()
	guard, err := limits.NewWorkGuard(ctx, location, limits.MaxEngineWorkUnits)
	require.NoError(t, err)
	return guard
}

func TestVerticalAlignmentTopLeft(t *testing.T) {
	byLevel := buildHierarchicalGraph()
	ctx := log.With(context.Background(), testlog.New(t))
	alignmentNodes := mustCreateAlignmentNodes(t, ctx, byLevel, geo.Top, geo.Left)
	a := alignmentNodes[0]
	b := alignmentNodes[1]
	c := alignmentNodes[2]
	d := alignmentNodes[3]
	e := alignmentNodes[4]
	f := alignmentNodes[5]
	g := alignmentNodes[6]
	conflicts := mustMarkConflicts(t, ctx, byLevel)
	require.NoError(t, verticalAlignment(ctx, alignmentNodes, conflicts, geo.Left))

	// roots
	assert.Equal(t, a, a.root)
	assert.Equal(t, b, b.root)
	assert.Equal(t, c, c.root)
	assert.Equal(t, a, d.root)
	assert.Equal(t, c, e.root)
	assert.Equal(t, a, f.root)
	assert.Equal(t, g, g.root)

	// alignment
	assert.Equal(t, d, a.alignedWith)
	assert.Equal(t, b, b.alignedWith)
	assert.Equal(t, e, c.alignedWith)
	assert.Equal(t, f, d.alignedWith)
	assert.Equal(t, c, e.alignedWith)
	assert.Equal(t, a, f.alignedWith)
	assert.Equal(t, g, g.alignedWith)

	// block sizes
	assert.Equal(t, 75.0, a.blockSize) // `a` is the root of `f` which has width 75

	// checks that all nodes in a given block (vertical alignment) are <= blockSize
	for _, r := range alignmentNodes {
		if r.root != r {
			// this node is not a root, so skip it
			continue
		}
		v := r
		for {
			assert.LessOrEqual(t, v.graphNode.Width, r.blockSize)
			v = v.alignedWith
			if r == v.alignedWith {
				break
			}
		}
	}
}

func TestVerticalAlignmentTopRight(t *testing.T) {
	byLevel := buildHierarchicalGraph()
	ctx := log.With(context.Background(), testlog.New(t))
	alignmentNodes := mustCreateAlignmentNodes(t, ctx, byLevel, geo.Top, geo.Right)
	c := alignmentNodes[0]
	b := alignmentNodes[1]
	a := alignmentNodes[2]
	e := alignmentNodes[3]
	d := alignmentNodes[4]
	g := alignmentNodes[5]
	f := alignmentNodes[6]
	conflicts := mustMarkConflicts(t, ctx, byLevel)
	require.NoError(t, verticalAlignment(ctx, alignmentNodes, conflicts, geo.Right))

	// roots
	assert.Equal(t, a, a.root)
	assert.Equal(t, b, b.root)
	assert.Equal(t, c, c.root)
	assert.Equal(t, b, d.root)
	assert.Equal(t, c, e.root)
	assert.Equal(t, f, f.root)
	assert.Equal(t, b, g.root)

	// alignment
	assert.Equal(t, a, a.alignedWith)
	assert.Equal(t, d, b.alignedWith)
	assert.Equal(t, e, c.alignedWith)
	assert.Equal(t, g, d.alignedWith)
	assert.Equal(t, c, e.alignedWith)
	assert.Equal(t, f, f.alignedWith)
	assert.Equal(t, b, g.alignedWith)
}

func TestVerticalAlignmentBottomLeft(t *testing.T) {
	byLevel := buildHierarchicalGraph()
	ctx := log.With(context.Background(), testlog.New(t))
	alignmentNodes := mustCreateAlignmentNodes(t, ctx, byLevel, geo.Bottom, geo.Left)
	f := alignmentNodes[0]
	g := alignmentNodes[1]
	d := alignmentNodes[2]
	e := alignmentNodes[3]
	a := alignmentNodes[4]
	b := alignmentNodes[5]
	c := alignmentNodes[6]
	conflicts := mustMarkConflicts(t, ctx, byLevel)
	require.NoError(t, verticalAlignment(ctx, alignmentNodes, conflicts, geo.Left))

	// roots
	assert.Equal(t, f, a.root)
	assert.Equal(t, b, b.root)
	assert.Equal(t, e, c.root)
	assert.Equal(t, f, d.root)
	assert.Equal(t, e, e.root)
	assert.Equal(t, f, f.root)
	assert.Equal(t, g, g.root)

	// alignment
	assert.Equal(t, f, a.alignedWith)
	assert.Equal(t, b, b.alignedWith)
	assert.Equal(t, e, c.alignedWith)
	assert.Equal(t, a, d.alignedWith)
	assert.Equal(t, c, e.alignedWith)
	assert.Equal(t, d, f.alignedWith)
	assert.Equal(t, g, g.alignedWith)
}

func TestVerticalAlignmentBottomRight(t *testing.T) {
	byLevel := buildHierarchicalGraph()
	ctx := log.With(context.Background(), testlog.New(t))
	alignmentNodes := mustCreateAlignmentNodes(t, ctx, byLevel, geo.Bottom, geo.Right)
	g := alignmentNodes[0]
	f := alignmentNodes[1]
	e := alignmentNodes[2]
	d := alignmentNodes[3]
	c := alignmentNodes[4]
	b := alignmentNodes[5]
	a := alignmentNodes[6]
	conflicts := mustMarkConflicts(t, ctx, byLevel)
	require.NoError(t, verticalAlignment(ctx, alignmentNodes, conflicts, geo.Right))

	// roots
	assert.Equal(t, a, a.root)
	assert.Equal(t, g, b.root)
	assert.Equal(t, e, c.root)
	assert.Equal(t, g, d.root)
	assert.Equal(t, e, e.root)
	assert.Equal(t, f, f.root)
	assert.Equal(t, g, g.root)

	// alignment
	assert.Equal(t, a, a.alignedWith)
	assert.Equal(t, g, b.alignedWith)
	assert.Equal(t, e, c.alignedWith)
	assert.Equal(t, b, d.alignedWith)
	assert.Equal(t, c, e.alignedWith)
	assert.Equal(t, f, f.alignedWith)
	assert.Equal(t, d, g.alignedWith)
}

func TestPlaceBlockTopLeft(t *testing.T) {
	byLevel := buildHierarchicalGraph()
	ctx := log.With(context.Background(), testlog.New(t))
	alignmentNodes := mustCreateAlignmentNodes(t, ctx, byLevel, geo.Top, geo.Left)
	a := alignmentNodes[0]
	b := alignmentNodes[1]
	c := alignmentNodes[2]
	d := alignmentNodes[3]
	e := alignmentNodes[4]
	f := alignmentNodes[5]
	g := alignmentNodes[6]
	conflicts := mustMarkConflicts(t, ctx, byLevel)
	require.NoError(t, verticalAlignment(ctx, alignmentNodes, conflicts, geo.Left))

	require.NoError(t, placeBlock(c, geo.Left, mustWorkGuard(t, ctx, "test")))

	assert.Equal(t, 0.0, a.x)
	assert.Equal(t, a.x+a.blockSize+siblingSpacing, b.x)
	assert.Equal(t, b.x+b.blockSize+siblingSpacing, c.x)
	assert.True(t, math.IsInf(d.x, -1))
	assert.True(t, math.IsInf(e.x, -1))
	assert.True(t, math.IsInf(f.x, -1))
	assert.True(t, math.IsInf(g.x, -1))

	assert.Equal(t, a, a.sink)
	assert.Equal(t, a, b.sink)
	assert.Equal(t, a, c.sink)
	assert.Equal(t, d, d.sink)
	assert.Equal(t, e, e.sink)
	assert.Equal(t, f, f.sink)
	assert.Equal(t, g, g.sink)

	assert.True(t, math.IsInf(a.shift, 1))
	assert.True(t, math.IsInf(b.shift, 1))
	assert.True(t, math.IsInf(c.shift, 1))
	assert.True(t, math.IsInf(d.shift, 1))
	assert.True(t, math.IsInf(e.shift, 1))
	assert.True(t, math.IsInf(f.shift, 1))
	assert.True(t, math.IsInf(g.shift, 1))
}

func TestPlaceBlockBottomRight(t *testing.T) {
	byLevel := buildHierarchicalGraph()
	ctx := log.With(context.Background(), testlog.New(t))
	alignmentNodes := mustCreateAlignmentNodes(t, ctx, byLevel, geo.Bottom, geo.Right)
	g := alignmentNodes[0]
	f := alignmentNodes[1]
	e := alignmentNodes[2]
	d := alignmentNodes[3]
	c := alignmentNodes[4]
	b := alignmentNodes[5]
	a := alignmentNodes[6]
	conflicts := mustMarkConflicts(t, ctx, byLevel)
	require.NoError(t, verticalAlignment(ctx, alignmentNodes, conflicts, geo.Right))

	require.NoError(t, placeBlock(f, geo.Right, mustWorkGuard(t, ctx, "test")))

	assert.Equal(t, 0.0, e.x)
	assert.Equal(t, e.x-e.blockSize-distanceFromPreviousRoot(e, g, geo.Right), g.x)
	assert.Equal(t, g.x-f.blockSize-distanceFromPreviousRoot(g, f, geo.Right), f.x)
	assert.True(t, math.IsInf(a.x, 1))
	assert.True(t, math.IsInf(b.x, 1))
	assert.True(t, math.IsInf(c.x, 1))
	assert.True(t, math.IsInf(d.x, 1))

	assert.Equal(t, e, f.sink)
	assert.Equal(t, e, g.sink)
	assert.Equal(t, b, b.sink)
	assert.Equal(t, c, c.sink)
	assert.Equal(t, d, d.sink)
	assert.Equal(t, e, e.sink)
	assert.Equal(t, a, a.sink)

	assert.True(t, math.IsInf(a.shift, -1))
	assert.True(t, math.IsInf(b.shift, -1))
	assert.True(t, math.IsInf(c.shift, -1))
	assert.True(t, math.IsInf(d.shift, -1))
	assert.True(t, math.IsInf(e.shift, -1))
	assert.True(t, math.IsInf(f.shift, -1))
	assert.True(t, math.IsInf(g.shift, -1))
}

func TestCreateAlignmentNodesTopLeft(t *testing.T) {
	byLevel := buildHierarchicalGraph()
	ctx := log.With(context.Background(), testlog.New(t))
	alignmentNodes := mustCreateAlignmentNodes(t, ctx, byLevel, geo.Top, geo.Left)

	assert.Equal(t, 7, len(alignmentNodes))
	assert.Equal(t, byLevel[0][0], alignmentNodes[0].placementNode)
	assert.Equal(t, byLevel[0][1], alignmentNodes[1].placementNode)
	assert.Equal(t, byLevel[0][2], alignmentNodes[2].placementNode)
	assert.Equal(t, byLevel[1][0], alignmentNodes[3].placementNode)
	assert.Equal(t, byLevel[1][1], alignmentNodes[4].placementNode)
	assert.Equal(t, byLevel[2][0], alignmentNodes[5].placementNode)
	assert.Equal(t, byLevel[2][1], alignmentNodes[6].placementNode)

	a := alignmentNodes[0]
	assert.Nil(t, a.prevSibling)
	assert.Equal(t, a.graphNode.Width, a.blockSize)
	assert.Equal(t, a, a.root)
	assert.Equal(t, a, a.alignedWith)
	assert.Equal(t, a, a.sink)
	assert.Equal(t, 0, a.rank)
	assert.True(t, math.IsInf(a.x, -1))
	assert.True(t, math.IsInf(a.shift, 1))
	assert.Equal(t, 0, len(a.medianNeighbors))

	c := alignmentNodes[2]
	assert.Equal(t, alignmentNodes[1], c.prevSibling)
	assert.Equal(t, c.graphNode.Width, c.blockSize)
	assert.Equal(t, c, c.root)
	assert.Equal(t, c, c.alignedWith)
	assert.Equal(t, c, c.sink)
	assert.Equal(t, 2, c.rank)
	assert.True(t, math.IsInf(c.x, -1))
	assert.True(t, math.IsInf(c.shift, 1))
	assert.Equal(t, 0, len(c.medianNeighbors))

	d := alignmentNodes[3]
	assert.Nil(t, d.prevSibling)
	assert.Equal(t, d.graphNode.Width, d.blockSize)
	assert.Equal(t, d, d.root)
	assert.Equal(t, d, d.alignedWith)
	assert.Equal(t, d, d.sink)
	assert.Equal(t, 0, d.rank)
	assert.True(t, math.IsInf(d.x, -1))
	assert.True(t, math.IsInf(d.shift, 1))
	assert.Equal(t, 2, len(d.medianNeighbors))
	assert.Equal(t, a, d.medianNeighbors[0])
	b := alignmentNodes[1]
	assert.Equal(t, b, d.medianNeighbors[1])
}

func TestCreateAlignmentNodesBottomRight(t *testing.T) {
	byLevel := buildHierarchicalGraph()
	ctx := log.With(context.Background(), testlog.New(t))
	alignmentNodes := mustCreateAlignmentNodes(t, ctx, byLevel, geo.Bottom, geo.Right)

	assert.Equal(t, 7, len(alignmentNodes))
	assert.Equal(t, byLevel[0][0], alignmentNodes[6].placementNode)
	assert.Equal(t, byLevel[0][1], alignmentNodes[5].placementNode)
	assert.Equal(t, byLevel[0][2], alignmentNodes[4].placementNode)
	assert.Equal(t, byLevel[1][0], alignmentNodes[3].placementNode)
	assert.Equal(t, byLevel[1][1], alignmentNodes[2].placementNode)
	assert.Equal(t, byLevel[2][0], alignmentNodes[1].placementNode)
	assert.Equal(t, byLevel[2][1], alignmentNodes[0].placementNode)

	g := alignmentNodes[0]
	assert.Nil(t, g.prevSibling)
	assert.Equal(t, g.graphNode.Width, g.blockSize)
	assert.Equal(t, g, g.root)
	assert.Equal(t, g, g.alignedWith)
	assert.Equal(t, g, g.sink)
	assert.Equal(t, 1, g.rank)
	assert.True(t, math.IsInf(g.x, 1))
	assert.True(t, math.IsInf(g.shift, -1))
	assert.Equal(t, 0, len(g.medianNeighbors))

	f := alignmentNodes[1]
	assert.Equal(t, g, f.prevSibling)
	assert.Equal(t, f.graphNode.Width, f.blockSize)
	assert.Equal(t, f, f.root)
	assert.Equal(t, f, f.alignedWith)
	assert.Equal(t, f, f.sink)
	assert.Equal(t, 0, f.rank)
	assert.True(t, math.IsInf(f.x, 1))
	assert.True(t, math.IsInf(f.shift, -1))
	assert.Equal(t, 0, len(f.medianNeighbors))

	e := alignmentNodes[2]
	assert.Nil(t, e.prevSibling)
	assert.Equal(t, e.graphNode.Width, e.blockSize)
	assert.Equal(t, e, e.root)
	assert.Equal(t, e, e.alignedWith)
	assert.Equal(t, e, e.sink)
	assert.Equal(t, 1, e.rank)
	assert.True(t, math.IsInf(e.x, 1))
	assert.True(t, math.IsInf(e.shift, -1))
	assert.Equal(t, 0, len(e.medianNeighbors))

	d := alignmentNodes[3]
	assert.Equal(t, 2, len(d.medianNeighbors))
	assert.Equal(t, g, d.medianNeighbors[0])
	assert.Equal(t, f, d.medianNeighbors[1])
}

func TestMarkType0Conflicts(t *testing.T) {
	// Type 0 conflicts are between two graph edges
	// in this case, there's no conflict to mark as it can't be aligned
	// ┌───┐       ┌───┐
	// │ a │       │ b │
	// └──┬┘       └┬──┘
	//    └─────────┼─┐
	//  ┌───────────┘ │
	// ┌▼──┐       ┌──▼┐
	// │ c │       │ d │
	// └───┘       └───┘
	g := layoutgraph.NewGraph()
	hierarchy := layoutgraph.NewHierarchy()

	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	a.Hierarchy = hierarchy
	hierarchy.Levels()[a] = 0
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	b.Hierarchy = hierarchy
	hierarchy.Levels()[b] = 0
	c := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	c.Hierarchy = hierarchy
	hierarchy.Levels()[c] = 1
	d := g.AddNode(layoutgraph.NewNode(4, 10, 10))
	d.Hierarchy = hierarchy
	hierarchy.Levels()[d] = 1

	g.Connect(a, d)
	g.Connect(b, c)

	placementNodes := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, placementNodes)
	byLevel := groupPlacementNodesByLevel(placementNodes)
	initializeRanks(byLevel)
	breakLongConnections(placementNodes, byLevel)

	// place nodes
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(50, 0)
	c.TopLeft = geo.NewPoint(0, 50)
	d.TopLeft = geo.NewPoint(50, 50)

	ctx := log.With(context.Background(), testlog.New(t))
	conflicts := mustMarkConflicts(t, ctx, byLevel)

	assert.Empty(t, conflicts)
}

func TestMarkType1Conflicts(t *testing.T) {
	// Type 1 conflicts: between a graph edge and a non-graph edge (edge between dummy nodes)
	// `#` are dummy nodes in the diagram below
	//         ┌───┐
	//   ┌─────┤ a ├─────┐
	//   │     └─┬─┘     │
	// ┌─▼─┐     #     ┌─▼─┐
	// │ b │  ┌──┼─────┤ c │
	// └──┬┘  │  |     └───┘
	//    └───┼──┼───┐
	//     ┌──▼┐ │  ┌▼──┐
	//     │ e │ #  │ d │
	//     └─┬─┘ │  └─┬─┘
	//       │ ┌─▼─┐  │
	//       └─► f ◄──┘
	//         └───┘
	g := layoutgraph.NewGraph()
	hierarchy := layoutgraph.NewHierarchy()

	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	a.Hierarchy = hierarchy
	hierarchy.Levels()[a] = 0
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	b.Hierarchy = hierarchy
	hierarchy.Levels()[b] = 1
	c := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	c.Hierarchy = hierarchy
	hierarchy.Levels()[c] = 1
	d := g.AddNode(layoutgraph.NewNode(4, 10, 10))
	d.Hierarchy = hierarchy
	hierarchy.Levels()[d] = 2
	e := g.AddNode(layoutgraph.NewNode(5, 10, 10))
	e.Hierarchy = hierarchy
	hierarchy.Levels()[e] = 2
	f := g.AddNode(layoutgraph.NewNode(6, 10, 10))
	f.Hierarchy = hierarchy
	hierarchy.Levels()[f] = 3

	g.Connect(a, b)
	g.Connect(a, c)
	g.Connect(a, f)
	g.Connect(b, d)
	g.Connect(c, e)
	g.Connect(d, f)
	g.Connect(e, f)

	placementNodes := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, placementNodes)

	byLevel := groupPlacementNodesByLevel(placementNodes)
	initializeRanks(byLevel)
	dummies := breakLongConnections(placementNodes, byLevel)
	assert.Equal(t, 2, len(dummies))
	placementNodes = append(placementNodes, dummies...)

	byLevel = map[int][]*placementNode{
		0: {placementNodes[0]},
		1: {placementNodes[1], dummies[0], placementNodes[2]},
		2: {placementNodes[4], dummies[1], placementNodes[3]},
		3: {placementNodes[5]},
	}
	placementNodes[1].rank = 0
	dummies[0].rank = 1
	placementNodes[2].rank = 2
	placementNodes[4].rank = 0
	dummies[1].rank = 1
	placementNodes[3].rank = 2

	ctx := log.With(context.Background(), testlog.New(t))
	conflicts := mustMarkConflicts(t, ctx, byLevel)

	// edge b -- d and c -- e crosses the edge between the dummy nodes
	assert.Equal(t, 4, len(conflicts))
	pnB := placementNodes[1]
	assert.Equal(t, b, pnB.graphNode)
	pnC := placementNodes[2]
	assert.Equal(t, c, pnC.graphNode)
	pnD := placementNodes[3]
	assert.Equal(t, d, pnD.graphNode)
	pnE := placementNodes[4]
	assert.Equal(t, e, pnE.graphNode)
	assert.Contains(t, conflicts, pnB)
	assert.Contains(t, conflicts, pnC)
	assert.Contains(t, conflicts, pnD)
	assert.Contains(t, conflicts, pnE)
	assert.Contains(t, conflicts[pnB], pnD)
	assert.Contains(t, conflicts[pnD], pnB)
	assert.Contains(t, conflicts[pnC], pnE)
	assert.Contains(t, conflicts[pnE], pnC)
}

func buildHierarchicalGraph() map[int][]*placementNode {
	// ┌─────────┐       ┌─────────┐      ┌─────────┐
	// │         │       │         │      │         │
	// │   a     │       │   b     │      │    c    │
	// │         │       │         │      │         │
	// └───┬─────┘       └────┬────┘      └───┬──┬──┘
	//     │                  │               │  │
	//     │ ┌────────────────┘               │  │
	//     │ │                   ┌────────────┘  │
	//     │ │                   │               │
	// ┌───▼─▼───┐       ┌───────▼─┐             │
	// │         │       │         │             │
	// │    d    │       │    e    │             │
	// │         │       │         │             │
	// └──┬─────┬┘       └─────────┘             │
	//    │     │                                │
	//    │     └────────────┐   ┌───────────────┘
	//    │                  │   │
	//    │                  │   │
	// ┌──▼──────┐       ┌───▼───▼─┐
	// │         │       │         │
	// │    f    │       │    g    │
	// │         │       │         │
	// └─────────┘       └─────────┘

	graph := layoutgraph.NewGraph()
	hierarchy := layoutgraph.NewHierarchy()

	a := graph.AddNode(layoutgraph.NewNode(1, 50, 50))
	a.TopLeft = geo.NewPoint(0, 0)
	hierarchy.Levels()[a] = 0
	a.Hierarchy = hierarchy
	b := graph.AddNode(layoutgraph.NewNode(2, 45, 50))
	b.TopLeft = geo.NewPoint(100, 0)
	hierarchy.Levels()[b] = 0
	b.Hierarchy = hierarchy
	c := graph.AddNode(layoutgraph.NewNode(3, 50, 50))
	c.TopLeft = geo.NewPoint(200, 0)
	hierarchy.Levels()[c] = 0
	c.Hierarchy = hierarchy
	d := graph.AddNode(layoutgraph.NewNode(4, 50, 50))
	d.TopLeft = geo.NewPoint(0, 100)
	hierarchy.Levels()[d] = 1
	d.Hierarchy = hierarchy
	e := graph.AddNode(layoutgraph.NewNode(5, 30, 50))
	e.TopLeft = geo.NewPoint(100, 100)
	hierarchy.Levels()[e] = 1
	e.Hierarchy = hierarchy
	f := graph.AddNode(layoutgraph.NewNode(6, 75, 50))
	f.TopLeft = geo.NewPoint(0, 200)
	hierarchy.Levels()[f] = 2
	f.Hierarchy = hierarchy
	g := graph.AddNode(layoutgraph.NewNode(7, 50, 50))
	g.TopLeft = geo.NewPoint(100, 200)
	hierarchy.Levels()[g] = 2
	g.Hierarchy = hierarchy

	graph.Connect(a, d)
	graph.Connect(b, d)
	graph.Connect(c, e)
	graph.Connect(c, g)
	graph.Connect(d, f)
	graph.Connect(d, g)

	nodes := createPlacementNodes(graph, graph.Nodes, nil)
	connectPlacementNodes(graph, nodes)
	byLevel := groupPlacementNodesByLevel(nodes)
	for l := 0; l < len(byLevel); l++ {
		sort.Slice(byLevel[l], func(i, j int) bool {
			return byLevel[l][i].graphNode.TopLeft.X < byLevel[l][j].graphNode.TopLeft.X
		})
	}
	initializeRanks(byLevel)
	return byLevel
}
