package hierarchy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestCountLevelCrossings(t *testing.T) {
	var scratch crossingScratch
	byLevel := map[int][]*placementNode{
		0: {newPlacementNode(0, nil), newPlacementNode(0, nil), newPlacementNode(0, nil)},
		1: {newPlacementNode(1, nil), newPlacementNode(1, nil)},
	}

	for level := 0; level < len(byLevel); level++ {
		for i := 0; i < len(byLevel[level]); i++ {
			byLevel[level][i].rank = i
		}
	}

	byLevel[1][0].connect(byLevel[0][0])
	byLevel[1][0].connect(byLevel[0][2])

	byLevel[1][1].connect(byLevel[0][0])
	byLevel[1][1].connect(byLevel[0][1])

	// crossings between level 0-1 from top-down
	segments := scratch.crossLevelSegments(byLevel[1], true, false)
	crossings := countCrossings(segments)
	assert.Equal(t, int64(2), crossings)

	// crossings between level 1-0 from bottom-up
	segments = scratch.crossLevelSegments(byLevel[0], false, true)
	crossings = countCrossings(segments)
	assert.Equal(t, int64(2), crossings)
}

func TestBestSwap(t *testing.T) {
	var scratch crossingScratch
	// ┌─────────────┐ ┌─────────────┐  ┌─────────────┐
	// │     a       │ │     b       │  │      c      │
	// └─┬───┬───────┘ └──────┬──────┘  └──────────┬──┘
	//   │   └────────────┬───┼────────────────┐   │
	//   │    ┌───────────┼───┴────────────────┼───┘
	// ┌─▼────▼──────┐ ┌──▼──────────┐  ┌──────▼──────┐
	// │    d        │ │     e       │  │      f      │
	// └─────────────┘ └─────────────┘  └─────────────┘
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	b := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	c := g.AddNode(layoutgraph.NewNode(3, 100, 100))
	d := g.AddNode(layoutgraph.NewNode(4, 100, 100))
	e := g.AddNode(layoutgraph.NewNode(5, 100, 100))
	f := g.AddNode(layoutgraph.NewNode(6, 100, 100))
	g.Connect(a, d)
	g.Connect(a, e)
	g.Connect(a, f)
	g.Connect(b, d)
	g.Connect(c, d)

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[a] = 0
	hierarchy.Levels()[b] = 0
	hierarchy.Levels()[c] = 0
	hierarchy.Levels()[d] = 1
	hierarchy.Levels()[e] = 1
	hierarchy.Levels()[f] = 1
	for _, n := range g.Nodes {
		n.Hierarchy = hierarchy
	}

	pns := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, pns)
	byLevel := groupPlacementNodesByLevel(pns)
	initializeRanks(byLevel)

	segments := scratch.crossLevelSegments(byLevel[0], true, true)
	crossings := countCrossings(segments)
	length := addLengths(segments)
	bestCrossings, bestLength, bestIndex := bestIndexBySwappingNeighbors(byLevel[0], 0, byLevel, &scratch)

	assert.Less(t, bestCrossings, crossings)
	assert.Less(t, bestLength, length)
	assert.Equal(t, bestIndex, 2)
}

func TestMinimizeCrossings(t *testing.T) {
	var scratch crossingScratch
	// ┌─────────────┐ ┌─────────────┐  ┌─────────────┐
	// │     a       │ │     b       │  │      c      │
	// └─┬───┬───────┘ └──────┬──────┘  └──────────┬──┘
	//   │   └────────────┬───┼────────────────┐   │
	//   │    ┌───────────┼───┴────────────────┼───┘
	// ┌─▼────▼──────┐ ┌──▼──────────┐  ┌──────▼──────┐
	// │    d        │ │     e       │  │      f      │
	// └─────────────┘ └─────────────┘  └─────────────┘
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	b := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	c := g.AddNode(layoutgraph.NewNode(3, 100, 100))
	d := g.AddNode(layoutgraph.NewNode(4, 100, 100))
	e := g.AddNode(layoutgraph.NewNode(5, 100, 100))
	f := g.AddNode(layoutgraph.NewNode(6, 100, 100))
	g.Connect(a, d)
	g.Connect(a, e)
	g.Connect(a, f)
	g.Connect(b, d)
	g.Connect(c, d)

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[a] = 0
	hierarchy.Levels()[b] = 0
	hierarchy.Levels()[c] = 0
	hierarchy.Levels()[d] = 1
	hierarchy.Levels()[e] = 1
	hierarchy.Levels()[f] = 1
	for _, n := range g.Nodes {
		n.Hierarchy = hierarchy
	}

	pns := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, pns)
	byLevel := groupPlacementNodesByLevel(pns)
	initializeRanks(byLevel)

	minimizeHierarchyCrossings(byLevel)

	segments := scratch.crossLevelSegments(byLevel[0], true, true)
	crossings := countCrossings(segments)
	assert.Equal(t, int64(0), crossings)

	segments = scratch.crossLevelSegments(byLevel[1], true, true)
	crossings = countCrossings(segments)
	assert.Equal(t, int64(0), crossings)
}

func TestAllDescendants(t *testing.T) {
	pn1 := newPlacementNode(0, layoutgraph.NewNode(1, 100, 100))
	pn1C := newPlacementNode(0, layoutgraph.NewNode(2, 100, 100))
	pn1.children = append(pn1.children, pn1C)
	pn1.optimizeChildrenCrossings = true
	pn2 := newPlacementNode(0, layoutgraph.NewNode(2, 100, 100))
	pn2C := newPlacementNode(0, layoutgraph.NewNode(2, 100, 100))
	pn2.children = append(pn2.children, pn2C)
	pn2.optimizeChildrenCrossings = false

	nodes := []*placementNode{pn1, pn2}
	all := allDescendants(nodes, false)
	assert.Equal(t, 4, len(all))
	assert.Equal(t, all, []*placementNode{pn1, pn2, pn1C, pn2C})

	// for optimization, exclude children that shouldn't be optimized
	all = allDescendants(nodes, true)
	assert.Equal(t, 3, len(all))
	assert.Equal(t, all, []*placementNode{pn1, pn2, pn1C})
}

func TestSortLevelNodesByAdjacencyPositionPreservesTies(t *testing.T) {
	left := newPlacementNode(0, nil)
	left.rank = 0
	middle := newPlacementNode(0, nil)
	middle.rank = 1
	right := newPlacementNode(0, nil)
	right.rank = 2

	firstEqual := newPlacementNode(1, nil)
	firstEqual.connect(left)
	firstEqual.connect(right)
	secondEqual := newPlacementNode(1, nil)
	secondEqual.connect(middle)
	noConnections := newPlacementNode(1, nil)

	nodes := []*placementNode{firstEqual, noConnections, secondEqual}
	sortLevelNodesByAdacencyPosition(nodes, true)
	assert.Equal(t, []*placementNode{noConnections, firstEqual, secondEqual}, nodes)
}
