package hierarchy

import (
	"context"
	"math/rand"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/internal/testlog"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/lib/shape"

	"github.com/d2lang/d2/lib/log"
)

func TestPlaceNodesInHierarchy(t *testing.T) {
	g := layoutgraph.NewGraph()

	n1 := g.AddNode(layoutgraph.NewNode(1, 89, 100))
	n2 := g.AddNode(layoutgraph.NewNode(2, 43, 87))
	n3 := g.AddNode(layoutgraph.NewNode(3, 91, 67))
	n4 := g.AddNode(layoutgraph.NewNode(4, 101, 101))
	n5 := g.AddNode(layoutgraph.NewNode(5, 54, 43))

	e1 := g.Connect(n1, n2) // source
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	e2 := g.Connect(n2, n3) // sink
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	e3 := g.Connect(n1, n4)
	e3.TargetArrowhead = layoutgraph.TriangleArrowhead
	e4 := g.Connect(n2, n5)
	e4.TargetArrowhead = layoutgraph.TriangleArrowhead

	g.ComputeCellSize()

	ctx := context.Background()
	ctx = log.With(ctx, testlog.New(t))
	declareRootContainment(g)

	rand := rand.New(rand.NewSource(0))
	Assign(ctx, g, nil, Candidates(g))
	assert.Equal(t, 0, n1.HierarchyLevel())
	assert.Equal(t, 1, n2.HierarchyLevel())
	assert.Equal(t, 2, n3.HierarchyLevel())
	assert.Equal(t, 1, n4.HierarchyLevel())
	assert.Equal(t, 2, n5.HierarchyLevel())

	nodesByLevel := groupNodesByLevel(g.Nodes, g.Nodes[0].Hierarchy)

	err := placeNodesInHierarchy(ctx, g, rand)
	assert.NoError(t, err)

	for i := 0; i < len(nodesByLevel); i++ {
		for j := 0; j < len(nodesByLevel[i])-1; j++ {
			a := nodesByLevel[i][j]
			b := nodesByLevel[i][j+1]
			// ensures that all nodes are at the same Y center
			assert.Equal(t, a.Center().Y, b.Center().Y)
		}

		// check vertical distance
		if i < len(nodesByLevel)-1 {
			_, br := layoutgraph.Nodes(nodesByLevel[i]).BoundingBox()
			for _, nextLevelNode := range nodesByLevel[i+1] {
				assert.GreaterOrEqual(t, parentSpacing, nextLevelNode.TopLeft.Y-br.Y)
			}
		}
	}

	for i := 0; i < len(nodesByLevel)-1; i++ {
		nodes := nodesByLevel[i]
		// all nodes of the same level should be at the same Y
		for j := 0; j < len(nodes)-1; j++ {
			assert.Equal(t, nodes[j].Center().Y, nodes[j+1].Center().Y)
		}

		// check that the next level is below
		nextLevelNodes := nodesByLevel[i+1]
		// only need to check the first node, if the Ys are different, it'll break the check above
		assert.Greater(t, nextLevelNodes[0].TopLeft.Y, nodes[0].TopLeft.Y)
	}
}

func groupNodesByLevel(nodes []*layoutgraph.Node, hierarchy *layoutgraph.Hierarchy) map[int][]*layoutgraph.Node {
	nodesByLevel := map[int][]*layoutgraph.Node{}
	for _, node := range nodes {
		level := hierarchy.Levels()[node]
		nodesByLevel[level] = append(nodesByLevel[level], node)
	}
	return nodesByLevel
}

func TestCreatePlacementNodes(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 89, 100))
	n2 := g.AddNode(layoutgraph.NewNode(2, 43, 87))
	n3 := g.AddNode(layoutgraph.NewNode(3, 91, 67))
	n4 := g.AddNode(layoutgraph.NewNode(4, 101, 101))
	n5 := g.AddNode(layoutgraph.NewNode(5, 54, 43))
	e1 := g.Connect(n1, n2) // source
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	e2 := g.Connect(n2, n3) // sink
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	e3 := g.Connect(n1, n4)
	e3.TargetArrowhead = layoutgraph.TriangleArrowhead
	e4 := g.Connect(n2, n5)
	e4.TargetArrowhead = layoutgraph.TriangleArrowhead
	e5 := g.Connect(n1, n5)
	e5.TargetArrowhead = layoutgraph.TriangleArrowhead

	ctx := context.Background()
	ctx = log.With(ctx, testlog.New(t))
	declareRootContainment(g)
	Assign(ctx, g, nil, Candidates(g))

	placementNodes := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, placementNodes)

	assert.Equal(t, 5, len(placementNodes))
	for _, pn := range placementNodes {
		if pn.graphNode != nil {
			assert.Equal(t, pn.graphNode.HierarchyLevel(), pn.level)
		}
		assert.GreaterOrEqual(t, pn.level, 0) // 0 is always the lowest level
		assert.LessOrEqual(t, pn.level, 2)    // 2 is the highest level for this graph
		assert.True(t, pn.optimizeChildrenCrossings)

		if pn.level != 0 {
			assert.Greater(t, len(pn.aboves), 0)
			for above := range pn.aboves {
				assert.Greater(t, pn.level, above.level)
			}
		} else {
			// no nodes above for level 0
			assert.Equal(t, 0, len(pn.aboves))
		}

		if pn.level != 2 {
			// some nodes might not be at level 2 and still have no nodes below
			// in this test, n4 is at level 1 and has no nodes below, it is a sink
			for below := range pn.belows {
				assert.Greater(t, below.level, pn.level)
			}
		} else {
			// no nodes below for level 2
			assert.Equal(t, 0, len(pn.belows))
		}
	}
}

func TestCreatePlacementNodesForTables(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.SetShape(shape.TABLE_TYPE)
	n1.SetNumColumns(3)
	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	n2.SetShape(shape.TABLE_TYPE)
	n2.SetNumColumns(2)

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[n1] = 0
	hierarchy.Levels()[n2] = 1
	n1.Hierarchy = hierarchy
	n2.Hierarchy = hierarchy

	placementNodes := createPlacementNodes(g, g.Nodes, nil)
	assert.Equal(t, 2, len(placementNodes))
	assert.Equal(t, 3, len(placementNodes[0].children))
	assert.False(t, placementNodes[0].optimizeChildrenCrossings)
	assert.False(t, placementNodes[0].isContainer)
	for _, child := range placementNodes[0].children {
		assert.Nil(t, child.graphNode)
		assert.False(t, child.isChainningConnection)
		assert.True(t, child.isDummy)
	}

	assert.Equal(t, 2, len(placementNodes[1].children))
	assert.False(t, placementNodes[1].optimizeChildrenCrossings)
	assert.False(t, placementNodes[1].isContainer)
	for _, child := range placementNodes[1].children {
		assert.Nil(t, child.graphNode)
		assert.False(t, child.isChainningConnection)
		assert.True(t, child.isDummy)
	}
}

// ensures that node hierarchies are kept in placementNodes
func TestCreatePlacementNodesContainers(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	b := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	c := g.AddNode(layoutgraph.NewNode(3, 50, 50))

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {a},
		a:   {b},
		b:   {c},
	}

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[a] = 0
	hierarchy.Levels()[b] = 0
	hierarchy.Levels()[c] = 0
	a.Hierarchy = hierarchy
	b.Hierarchy = hierarchy
	c.Hierarchy = hierarchy

	pns := createPlacementNodes(g, g.Containers[nil], nil)

	assert.Equal(t, 1, len(pns))

	pnA := pns[0]
	pnB := pnA.children[0]
	pnC := pnB.children[0]

	assert.Equal(t, a, pnA.graphNode)
	assert.Equal(t, 1, len(pnA.children))
	assert.True(t, pnA.isContainer)

	assert.Equal(t, b, pnB.graphNode)
	assert.Equal(t, pnA, pnB.container)
	assert.Equal(t, 1, len(pnB.children))
	assert.True(t, pnB.isContainer)

	assert.Equal(t, c, pnC.graphNode)
	assert.Equal(t, pnB, pnC.container)
	assert.Equal(t, 0, len(pnC.children))
	assert.False(t, pnC.isContainer)
}

func TestBreakLongConnections(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	b := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	g.Connect(b, a)

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[a] = 0
	hierarchy.Levels()[b] = 3
	a.Hierarchy = hierarchy
	b.Hierarchy = hierarchy

	pns := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, pns)
	byLevel := groupPlacementNodesByLevel(pns)
	initializeRanks(byLevel)
	dummies := breakLongConnections(pns, byLevel)

	assert.Equal(t, 2, len(pns))
	assert.Equal(t, 2, len(dummies))
	pna := pns[0]
	pnb := pns[1]
	dummy1 := dummies[0]
	dummy2 := dummies[1]
	assert.Equal(t, a, pna.graphNode)
	assert.False(t, pna.isDummy)
	assert.Equal(t, b, pnb.graphNode)
	assert.False(t, pnb.isDummy)
	assert.True(t, dummy1.isDummy)
	assert.True(t, dummy2.isDummy)
	assert.Equal(t, layoutgraph.EntityID(-1), dummy1.graphNode.ID)
	assert.Equal(t, layoutgraph.EntityID(-2), dummy2.graphNode.ID)
	assert.Equal(t, 0, len(pna.aboves))
	assert.Equal(t, 1, len(pna.belows))
	assert.Equal(t, 1, len(dummy1.aboves))
	assert.Equal(t, 1, len(dummy1.belows))
	assert.Equal(t, 1, len(dummy2.aboves))
	assert.Equal(t, 1, len(dummy2.belows))
	assert.Equal(t, 1, len(pnb.aboves))
	assert.Equal(t, 0, len(pnb.belows))
	assert.Contains(t, pna.belows, dummy1)
	assert.Contains(t, dummy1.aboves, pna)
	assert.Contains(t, dummy1.belows, dummy2)
	assert.Contains(t, dummy2.aboves, dummy1)
	assert.Contains(t, dummy2.belows, pnb)
	assert.Contains(t, pnb.aboves, dummy2)
	assert.True(t, dummy1.isChainningConnection)
	assert.True(t, dummy1.isChainningConnection)
}

func TestConnectPlacementNodes(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	b := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	g.Connect(b, a)

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[a] = 0
	hierarchy.Levels()[b] = 1
	a.Hierarchy = hierarchy
	b.Hierarchy = hierarchy

	pns := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, pns)

	pnA := pns[0]
	pnB := pns[1]
	if _, exists := pnA.belows[pnB]; !exists {
		t.Fatal("Expected `b` below `a`")
	}
	if _, exists := pnB.aboves[pnA]; !exists {
		t.Fatal("Expected `a` above `b`")
	}
	assert.Equal(t, 0, len(pnA.aboves))
	assert.Equal(t, 1, len(pnA.belows))
	assert.Equal(t, 0, len(pnB.belows))
	assert.Equal(t, 1, len(pnB.aboves))
}

func TestConnectPlacementNodesContainers(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	b := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	c := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	g.AddNodeToContainer(nil, a)
	g.AddNodeToContainer(nil, c)
	g.AddNodeToContainer(c, b)

	g.Connect(b, a)

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[a] = 0
	hierarchy.Levels()[b] = 1
	hierarchy.Levels()[c] = 1
	a.Hierarchy = hierarchy
	b.Hierarchy = hierarchy
	c.Hierarchy = hierarchy

	pns := createPlacementNodes(g, g.Containers[nil], nil)
	connectPlacementNodes(g, pns)

	pnA := pns[0]
	pnC := pns[1]
	pnB := pnC.children[0]
	if _, exists := pnA.belows[pnB]; !exists {
		t.Fatal("Expected `b` below `a`")
	}
	if _, exists := pnB.aboves[pnA]; !exists {
		t.Fatal("Expected `a` above `b`")
	}
	assert.Equal(t, 0, len(pnA.aboves))
	assert.Equal(t, 1, len(pnA.belows))
	assert.Equal(t, 0, len(pnB.belows))
	assert.Equal(t, 1, len(pnB.aboves))
	assert.Equal(t, 0, len(pnC.aboves))
	assert.Equal(t, 0, len(pnC.belows))
}

func TestConnectTableColumns(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	a.SetShape(shape.TABLE_TYPE)
	a.SetNumColumns(3)
	b := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	b.SetShape(shape.TABLE_TYPE)
	b.SetNumColumns(2)
	e := g.Connect(b, a)
	e.FromTableColumnIndex = new(int)
	*e.FromTableColumnIndex = 0
	e.ToTableColumnIndex = new(int)
	*e.ToTableColumnIndex = 0

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[a] = 0
	hierarchy.Levels()[b] = 1
	a.Hierarchy = hierarchy
	b.Hierarchy = hierarchy

	pns := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, pns)

	tableA := pns[0]
	tableB := pns[1]
	if _, exists := tableA.belows[tableB]; !exists {
		t.Fatal("Expected `tableB` below `tableA`")
	}
	if _, exists := tableB.aboves[tableA]; !exists {
		t.Fatal("Expected `tableA` above `tableB`")
	}
	assert.Equal(t, 0, len(tableA.aboves))
	assert.Equal(t, 1, len(tableA.belows))
	assert.Equal(t, 0, len(tableB.belows))
	assert.Equal(t, 1, len(tableB.aboves))

	tableAColumn0 := tableA.children[0]
	tableBColumn0 := tableB.children[0]
	if _, exists := tableAColumn0.belows[tableBColumn0]; !exists {
		t.Fatal("Expected `tableBColumn0` below `tableAColumn0`")
	}
	if _, exists := tableBColumn0.aboves[tableAColumn0]; !exists {
		t.Fatal("Expected `tableAColumn0` above `tableBColumn0`")
	}
	assert.Equal(t, 0, len(tableAColumn0.aboves))
	assert.Equal(t, 1, len(tableAColumn0.belows))
	assert.Equal(t, 0, len(tableBColumn0.belows))
	assert.Equal(t, 1, len(tableBColumn0.aboves))

	unconnectedNodes := []*placementNode{tableA.children[1], tableA.children[2], tableB.children[1]}
	for _, n := range unconnectedNodes {
		assert.Equal(t, 0, len(n.aboves))
		assert.Equal(t, 0, len(n.belows))
	}
}

func TestGroupPlacementNodesByLevel(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 89, 100))
	n2 := g.AddNode(layoutgraph.NewNode(2, 43, 87))
	n3 := g.AddNode(layoutgraph.NewNode(3, 91, 67))
	n4 := g.AddNode(layoutgraph.NewNode(4, 101, 101))
	n5 := g.AddNode(layoutgraph.NewNode(5, 54, 43))
	e1 := g.Connect(n1, n2) // source
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	e2 := g.Connect(n2, n3) // sink
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	e3 := g.Connect(n1, n4)
	e3.TargetArrowhead = layoutgraph.TriangleArrowhead
	e4 := g.Connect(n2, n5)
	e4.TargetArrowhead = layoutgraph.TriangleArrowhead
	e5 := g.Connect(n1, n5)
	e5.TargetArrowhead = layoutgraph.TriangleArrowhead

	ctx := context.Background()
	ctx = log.With(ctx, testlog.New(t))
	declareRootContainment(g)
	Assign(ctx, g, nil, Candidates(g))

	placementNodes := createPlacementNodes(g, g.Nodes, nil)
	byLevel := groupPlacementNodesByLevel(placementNodes)

	assert.Equal(t, 1, len(byLevel[0])) // n1
	assert.Equal(t, 2, len(byLevel[1])) // n2 and n4
	assert.Equal(t, 2, len(byLevel[2])) // n3 and n5

	assert.Equal(t, n1, byLevel[0][0].graphNode)
	for _, pn := range byLevel[1] {
		switch pn.graphNode {
		case n2, n4, nil:
			continue
		default:
			t.Fatalf("unexpected node %v at level 1", pn.graphNode)
		}
	}
	for _, pn := range byLevel[2] {
		switch pn.graphNode {
		case n3, n5:
			continue
		default:
			t.Fatalf("unexpected node %v at level 2", pn.graphNode)
		}
	}
}

func declareRootContainment(graph *layoutgraph.Graph) {
	for _, node := range graph.Nodes {
		graph.AddNodeToContainer(nil, node)
	}
}

func TestIsSource(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 1, 1))
	n2 := g.AddNode(layoutgraph.NewNode(2, 1, 1))

	e1 := g.Connect(n1, n2)

	// undirected edge
	e1.SourceArrowhead = layoutgraph.NoArrowhead
	e1.TargetArrowhead = layoutgraph.NoArrowhead
	assert.Equal(t, false, isSource(n1))

	// bidirectional edge
	e1.SourceArrowhead = layoutgraph.TriangleArrowhead
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	assert.Equal(t, false, isSource(n1))

	// backward edge
	e1.SourceArrowhead = layoutgraph.TriangleArrowhead
	e1.TargetArrowhead = layoutgraph.NoArrowhead
	assert.Equal(t, false, isSource(n1))

	// forward edge
	e1.SourceArrowhead = layoutgraph.NoArrowhead
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	assert.Equal(t, true, isSource(n1))

	// add a second edge so that the node isSource only if all edges are forward edges
	n3 := g.AddNode(layoutgraph.NewNode(3, 1, 1))
	e2 := g.Connect(n1, n3)

	// undirected edge
	e2.SourceArrowhead = layoutgraph.NoArrowhead
	e2.TargetArrowhead = layoutgraph.NoArrowhead
	assert.Equal(t, false, isSource(n1))

	// bidirectional edge
	e2.SourceArrowhead = layoutgraph.TriangleArrowhead
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	assert.Equal(t, false, isSource(n1))

	// backward edge
	e2.SourceArrowhead = layoutgraph.TriangleArrowhead
	e2.TargetArrowhead = layoutgraph.NoArrowhead
	assert.Equal(t, false, isSource(n1))

	// forward edge
	e2.SourceArrowhead = layoutgraph.NoArrowhead
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	assert.Equal(t, true, isSource(n1))
}

func TestIsSink(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 1, 1))
	n2 := g.AddNode(layoutgraph.NewNode(2, 1, 1))

	e1 := g.Connect(n1, n2)

	// undirected edge
	e1.SourceArrowhead = layoutgraph.NoArrowhead
	e1.TargetArrowhead = layoutgraph.NoArrowhead
	assert.Equal(t, false, isSink(n2))

	// bidirectional edge
	e1.SourceArrowhead = layoutgraph.TriangleArrowhead
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	assert.Equal(t, false, isSink(n2))

	// backward edge
	e1.SourceArrowhead = layoutgraph.TriangleArrowhead
	e1.TargetArrowhead = layoutgraph.NoArrowhead
	assert.Equal(t, false, isSink(n2))

	// forward edge
	e1.SourceArrowhead = layoutgraph.NoArrowhead
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	assert.Equal(t, true, isSink(n2))

	// add a second edge so that the node isSink only if all edges are forward edges
	n3 := g.AddNode(layoutgraph.NewNode(3, 1, 1))
	e2 := g.Connect(n3, n2)

	// undirected edge
	e2.SourceArrowhead = layoutgraph.NoArrowhead
	e2.TargetArrowhead = layoutgraph.NoArrowhead
	assert.Equal(t, false, isSink(n2))

	// bidirectional edge
	e2.SourceArrowhead = layoutgraph.TriangleArrowhead
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	assert.Equal(t, false, isSink(n2))

	// backward edge
	e2.SourceArrowhead = layoutgraph.TriangleArrowhead
	e2.TargetArrowhead = layoutgraph.NoArrowhead
	assert.Equal(t, false, isSink(n2))

	// forward edge
	e2.SourceArrowhead = layoutgraph.NoArrowhead
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	assert.Equal(t, true, isSink(n2))
}

func TestComputeLevelRanks(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	b := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	c := g.AddNode(layoutgraph.NewNode(3, 50, 50))
	d := g.AddNode(layoutgraph.NewNode(4, 50, 50))
	e := g.AddNode(layoutgraph.NewNode(5, 50, 50))
	f := g.AddNode(layoutgraph.NewNode(6, 50, 50))

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {a, d, e},
		a:   {b},
		b:   {c},
		e:   {f},
	}
	hierarchy := layoutgraph.NewHierarchy()
	for _, n := range g.Nodes {
		hierarchy.Levels()[n] = 0
		n.Hierarchy = hierarchy
	}

	pns := createPlacementNodes(g, g.Containers[nil], nil)
	byLevel := groupPlacementNodesByLevel(pns)

	pnA := pns[0]
	pnB := pnA.children[0]
	pnC := pnB.children[0]
	pnD := pns[1]
	pnE := pns[2]
	pnF := pnE.children[0]

	computeLevelRanks(byLevel[0])

	assert.Equal(t, 0, pnA.rank)
	assert.Equal(t, 1, pnB.rank)
	assert.Equal(t, 2, pnC.rank)
	assert.Equal(t, 3, pnD.rank)
	assert.Equal(t, 4, pnE.rank)
	assert.Equal(t, 5, pnF.rank)
}

func TestRemoveDummyNodes(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.SetShape(shape.TABLE_TYPE)
	n1.SetNumColumns(3)
	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	n2.SetShape(shape.TABLE_TYPE)
	n2.SetNumColumns(2)

	e := g.Connect(n1, n2)
	e.FromTableColumnIndex = new(int)
	*e.FromTableColumnIndex = 2 // 3rd column
	e.ToTableColumnIndex = new(int)
	*e.ToTableColumnIndex = 1 // 2nd column

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[n1] = 0
	hierarchy.Levels()[n2] = 1
	n1.Hierarchy = hierarchy
	n2.Hierarchy = hierarchy

	placementNodes := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, placementNodes)
	removeTableColumnNodes(placementNodes)

	table1 := placementNodes[0]
	table2 := placementNodes[1]
	assert.Equal(t, 1, len(table1.belows))
	assert.Equal(t, 0, len(table1.children))
	assert.Contains(t, table1.belows, table2)

	assert.Equal(t, 1, len(table2.aboves))
	assert.Equal(t, 0, len(table2.children))
	assert.Contains(t, table2.aboves, table1)
}

func TestRemoveDummyChainNodesForTables(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 50, 50))
	a.SetShape(shape.TABLE_TYPE)
	a.SetNumColumns(3)
	b := g.AddNode(layoutgraph.NewNode(2, 50, 50))
	b.SetShape(shape.TABLE_TYPE)
	b.SetNumColumns(5)
	e := g.Connect(a, b)
	e.FromTableColumnIndex = new(int)
	*e.FromTableColumnIndex = 0 // 1st column
	e.ToTableColumnIndex = new(int)
	*e.ToTableColumnIndex = 4 // 5th column

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.Levels()[a] = 0
	hierarchy.Levels()[b] = 3
	a.Hierarchy = hierarchy
	b.Hierarchy = hierarchy

	pns := createPlacementNodes(g, g.Nodes, nil)
	connectPlacementNodes(g, pns)
	byLevel := groupPlacementNodesByLevel(pns)
	initializeRanks(byLevel)
	dummies := breakLongConnections(pns, byLevel)
	pns = append(pns, dummies...)
	pns = removeTableColumnNodes(pns)

	// only removed the dummy nodes related to the table columns
	// we must keep the ones connecting the tables
	assert.Equal(t, 4, len(pns))
	assert.Contains(t, pns, dummies[0])
	assert.Contains(t, pns, dummies[1])
	assert.NotContains(t, pns, dummies[2])
	assert.NotContains(t, pns, dummies[3])

	assert.Equal(t, 1, len(pns[0].belows))
	assert.Contains(t, pns[0].belows, dummies[0])
	assert.Equal(t, 1, len(pns[1].aboves))
	assert.Contains(t, pns[1].aboves, dummies[1])
}
