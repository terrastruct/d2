package hierarchy

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/log"
)

func TestBuildPropagatesRankErrors(t *testing.T) {
	graph := layoutgraph.NewGraph()
	source := layoutgraph.NewNode(1, 10, 10)
	target := layoutgraph.NewNode(1, 10, 10)
	graph.AddNewNodeToContainer(nil, source)
	graph.AddNewNodeToContainer(nil, target)
	edge := graph.Connect(source, target)
	edge.TargetArrowhead = layoutgraph.TriangleArrowhead

	hierarchy, err := build(t.Context(), graph, true, Candidates(graph), nil)
	if hierarchy != nil {
		t.Fatalf("hierarchy = %v; want nil after rank failure", hierarchy)
	}
	if err == nil || !strings.Contains(err.Error(), "TALA RankDAG failed") || !strings.Contains(err.Error(), "duplicate node ID 1") {
		t.Fatalf("build error = %v; want attributed duplicate-ID rank error", err)
	}
}

func setContainerChildrenForTest(graph *layoutgraph.Graph, container *layoutgraph.Node, children []*layoutgraph.Node) {
	graph.Containers[container] = children
	if container != nil {
		container.SetContainer(true)
	}
	for _, child := range children {
		child.Container = container
	}
}

func Test1N1Hierarchies(t *testing.T) {
	// the graph below is clearly a 3 level hierarchy
	// however, it renders better with cluster right now
	//                               ┌───────┐
	//                               │       │
	//     ┌───────────┬──────────┬──┤   0   ├─┬───────────┬───────────┐
	//     │           │          │  │       │ │           │           │
	//     │           │          │  └───────┘ │           │           │
	// ┌───▼───┐   ┌───▼───┐   ┌──▼────┐   ┌───▼───┐   ┌───▼───┐   ┌───▼───┐
	// │       │   │       │   │       │   │       │   │       │   │       │
	// │   1   │   │   2   │   │   3   │   │   4   │   │   5   │   │   6   │
	// │       │   │       │   │       │   │       │   │       │   │       │
	// └───┬───┘   └───┬───┘   └──┬────┘   └────┬──┘   └───┬───┘   └───┬───┘
	//     │           │          │  ┌───────┐  │          │           │
	//     │           │          │  │       │  │          │           │
	//     └───────────┴──────────┴──►   7   ◄──┴──────────┴───────────┘
	//                               │       │
	//                               └───────┘

	// graph above
	t.Run("Ignore1N1", func(t *testing.T) {
		g := layoutgraph.NewGraph()
		for i := 0; i < 8; i++ {
			g.AddNewNodeToContainer(nil, layoutgraph.NewNode(layoutgraph.EntityID(i), 50, 50))
		}

		connect := func(i, j int) {
			e := g.Connect(g.Nodes[i], g.Nodes[j])
			e.TargetArrowhead = layoutgraph.TriangleArrowhead
			e.SourceArrowhead = layoutgraph.NoArrowhead
		}

		for i := 1; i < 7; i++ {
			connect(0, i)
			connect(i, 7)
		}

		ctx := log.With(context.Background(), testlog.New(t))
		Assign(ctx, g, nil, Candidates(g))

		for _, node := range g.Nodes {
			assert.Equal(t, false, node.Hierarchy != nil)
		}
	})

	// some mid nodes don't have all connections
	t.Run("NotFull1N1", func(t *testing.T) {
		g := layoutgraph.NewGraph()
		for i := 0; i < 8; i++ {
			g.AddNewNodeToContainer(nil, layoutgraph.NewNode(layoutgraph.EntityID(i), 50, 50))
		}

		connect := func(i, j int) {
			e := g.Connect(g.Nodes[i], g.Nodes[j])
			e.TargetArrowhead = layoutgraph.TriangleArrowhead
			e.SourceArrowhead = layoutgraph.NoArrowhead
		}

		for i := 1; i < 7; i++ {
			connect(0, i)
			if i%2 == 0 {
				connect(i, 7)
			}
		}

		ctx := log.With(context.Background(), testlog.New(t))
		Assign(ctx, g, nil, Candidates(g))

		for _, node := range g.Nodes {
			assert.Equal(t, true, node.Hierarchy != nil)
		}
	})
}

func TestBuildHierarchy(t *testing.T) {
	//           ┌──────┐     ┌──────┐
	//     ┌─────┤  n5  ├─┐ ┌─┤  n1  │
	//     │     └──┬───┘ │ │ └──┬───┘
	// ┌───▼──┐     │  ┌──▼─▼─┐  │       ┌──────┐
	// │  n3  │     │  │  n4  │  │       │  n7  │
	// └────┬─┘     │  └───┬──┘  │       └───┬──┘
	//      └─────┐ │      │     │           │
	//            │ │      │     │           │
	//           ┌▼─▼───┐  │     │       ┌───▼──┐
	//           │  n2  ◄──┘     │       │  n8  │
	//           └──┬───┘        │       └───┬──┘
	//              │      ┌─────┘           │
	//              │ ┌────▼─┐               │
	//              └─►  n6  ◄───────────────┘
	//                └──────┘
	g := layoutgraph.NewGraph()

	n1 := g.AddNode(layoutgraph.NewNode(1, 1, 1))
	n2 := g.AddNode(layoutgraph.NewNode(2, 1, 1))
	n3 := g.AddNode(layoutgraph.NewNode(3, 1, 1))
	n4 := g.AddNode(layoutgraph.NewNode(4, 1, 1))
	n5 := g.AddNode(layoutgraph.NewNode(5, 1, 1))
	n6 := g.AddNode(layoutgraph.NewNode(6, 1, 1))
	n7 := g.AddNode(layoutgraph.NewNode(7, 1, 1))
	n8 := g.AddNode(layoutgraph.NewNode(8, 1, 1))

	connect := func(from, to *layoutgraph.Node) *layoutgraph.Edge {
		e := g.Connect(from, to)
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}

	connect(n1, n6)
	connect(n1, n4)
	connect(n5, n4)
	connect(n5, n3)
	connect(n4, n2)
	connect(n3, n2)
	connect(n5, n2)
	connect(n2, n6)
	connect(n7, n8)
	connect(n8, n6)

	ctx := log.With(context.Background(), testlog.New(t))
	hierarchy, err := build(ctx, g, false, Candidates(g), nil)
	if err != nil {
		t.Fatalf("Got an error while building node hierarchy %v", err)
	}

	assert.Equal(t, 0, hierarchy.Levels()[n5])
	assert.Equal(t, 0, hierarchy.Levels()[n1])
	assert.Equal(t, 1, hierarchy.Levels()[n4])
	assert.Equal(t, 1, hierarchy.Levels()[n7])
	assert.Equal(t, 1, hierarchy.Levels()[n3])
	assert.Equal(t, 2, hierarchy.Levels()[n2])
	assert.Equal(t, 2, hierarchy.Levels()[n8])
	assert.Equal(t, 3, hierarchy.Levels()[n6])

	assertNoHierarchy := func() {
		hierarchy, err := build(ctx, g, false, Candidates(g), nil)
		assert.NoError(t, err)
		assert.Nil(t, hierarchy)
	}

	// clusters
	n4.SetClusterVessel(true)
	g.Clusters[n4] = &layoutgraph.Cluster{}
	assertNoHierarchy()
	delete(g.Clusters, n4)
	n4.SetClusterVessel(false)

	// trees
	g.Trees[n4] = []*layoutgraph.Tree{}
	assertNoHierarchy()
	delete(g.Trees, n4)

	// trees
	g.Sequences[n4] = &layoutgraph.Sequence{}
	assertNoHierarchy()
	delete(g.Sequences, n4)

	// no sink
	e6To1 := connect(n6, n1)
	assertNoHierarchy()
	g.Disconnect(e6To1)

	// no source
	connect(n5, n1)
	connect(n1, n5)
	assertNoHierarchy()
}

func TestCountHierarchyEdgeDirections(t *testing.T) {
	// Same graph from TestBuildHierarchy
	g := layoutgraph.NewGraph()

	n1 := g.AddNode(layoutgraph.NewNode(1, 1, 1))
	n2 := g.AddNode(layoutgraph.NewNode(2, 1, 1))
	n3 := g.AddNode(layoutgraph.NewNode(3, 1, 1))
	n4 := g.AddNode(layoutgraph.NewNode(4, 1, 1))
	n5 := g.AddNode(layoutgraph.NewNode(5, 1, 1))
	n6 := g.AddNode(layoutgraph.NewNode(6, 1, 1))
	n7 := g.AddNode(layoutgraph.NewNode(7, 1, 1))
	n8 := g.AddNode(layoutgraph.NewNode(8, 1, 1))

	connect := func(from, to *layoutgraph.Node) *layoutgraph.Edge {
		e := g.Connect(from, to)
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}

	connect(n1, n6)
	connect(n1, n4)
	connect(n5, n4)
	e5To3 := connect(n5, n3)
	e4To2 := connect(n4, n2)
	connect(n3, n2)
	connect(n5, n2)
	connect(n2, n6)
	connect(n7, n8)
	connect(n8, n6)

	ctx := log.With(context.Background(), testlog.New(t))
	hierarchy, _ := build(ctx, g, false, Candidates(g), nil)
	forward, backwardOrNeutral := countEdgeDirection(hierarchy, g)
	assert.Equal(t, 10, forward)
	assert.Equal(t, 0, backwardOrNeutral)

	e5To3.TargetArrowhead = layoutgraph.NoArrowhead
	e4To2.SourceArrowhead = layoutgraph.TriangleArrowhead
	forward, backwardOrNeutral = countEdgeDirection(hierarchy, g)
	assert.Equal(t, 8, forward)
	assert.Equal(t, 2, backwardOrNeutral)
}

func TestIsValidHierarchy(t *testing.T) {
	g := layoutgraph.NewGraph()

	n1 := g.AddNode(layoutgraph.NewNode(1, 1, 1))
	n2 := g.AddNode(layoutgraph.NewNode(2, 1, 1))
	n3 := g.AddNode(layoutgraph.NewNode(3, 1, 1))
	n4 := g.AddNode(layoutgraph.NewNode(4, 1, 1))
	n5 := g.AddNode(layoutgraph.NewNode(5, 1, 1))
	n6 := g.AddNode(layoutgraph.NewNode(6, 1, 1))
	n7 := g.AddNode(layoutgraph.NewNode(6, 1, 1))
	hierarchy := layoutgraph.NewHierarchy()

	g.Connect(n1, n2)
	g.Connect(n1, n3)
	g.Connect(n3, n4)
	g.Connect(n3, n5)
	g.Connect(n2, n5)
	g.Connect(n5, n6)
	g.Connect(n5, n7)
	g.Connect(n4, n7)
	g.Connect(n4, n6)

	// not enough forward edges
	hierarchy.LevelCount = 4
	hierarchy.ReplaceLevels(map[*layoutgraph.Node]int{
		n1: 0,
		n2: 1,
		n3: 1,
		n4: 2,
		n5: 2,
		n6: 3,
		n7: 3,
	})
	assert.Equal(t, false, isValid(hierarchy, g, nil, 0, 0))

	// too wide
	hierarchy.LevelCount = 3
	hierarchy.ReplaceLevels(map[*layoutgraph.Node]int{
		n1: 0,
		n2: 1,
		n3: 1,
		n4: 2,
		n5: 1,
		n6: 1,
		n7: 1,
	})
	assert.Equal(t, false, isValid(hierarchy, g, nil, 0, 0))

	// too tall
	hierarchy.LevelCount = 6
	hierarchy.ReplaceLevels(map[*layoutgraph.Node]int{
		n1: 0,
		n2: 1,
		n3: 2,
		n4: 3,
		n5: 3,
		n6: 4,
		n7: 5,
	})
	assert.Equal(t, false, isValid(hierarchy, g, nil, 0, 0))

	// all forward edges
	for _, e := range g.Edges {
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
	}
	hierarchy.LevelCount = 3
	hierarchy.ReplaceLevels(map[*layoutgraph.Node]int{
		n1: 0,
		n2: 1,
		n3: 1,
		n4: 1,
		n5: 2,
		n6: 2,
		n7: 2,
	})
	assert.Equal(t, true, isValid(hierarchy, g, nil, 1, 2))

	hierarchy.LevelCount = 2
	hierarchy.ReplaceLevels(map[*layoutgraph.Node]int{
		n1: 0,
		n4: 0,
		n5: 0,
		n2: 1,
		n3: 1,
		n6: 1,
		n7: 1,
	})
	assert.Equal(t, false, isValid(hierarchy, g, nil, 1, 2))
}

func TestAssignNodeHierarchyInsideContainers(t *testing.T) {
	// ┌──────────────────────────────────────────────────────┐
	// │1                                                     │
	// │      ┌─────────────┐             ┌─────────────┐     │
	// │      │             │             │             │     │
	// │      │             │      ┌──────┤             │     │
	// │      │     2       │      │      │      3      │     │
	// │      │             │      │      │             │     │
	// │      └─────┬───────┘      │      └─────┬───────┘     │
	// │            │              │            │             │
	// │            │              │            │             │
	// │            │              │            │             │
	// │            │              │            │             │
	// │            │              │            │             │
	// │            │              │            │             │
	// │            │              │            │             │
	// │            │              │            │             │
	// │      ┌─────▼───────┐      │      ┌─────▼───────┐     │
	// │      │             │      │      │             │     │
	// │      │             │      │      │             │     │
	// │      │     4       ◄──────┘      │      5      │     │
	// │      │             │             │             │     │
	// │      └─────┬───────┘             └──────┬──────┘     │
	// │            │                            │            │
	// │            │                            │            │
	// │            │                            │            │
	// │            │                            │            │
	// │            │                            │            │
	// │            │        ┌─────────────┐     │            │
	// │            │        │             │     │            │
	// │            │        │             │     │            │
	// │            └────────►      6      ◄─────┘            │
	// │                     │             │                  │
	// │                     └─────────────┘                  │
	// │                                                      │
	// │                                                      │
	// └──────────────────────────────────────────────────────┘
	g := layoutgraph.NewGraph()

	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	g.AddNewNodeToContainer(nil, n1)
	n2 := layoutgraph.NewNode(2, 30, 30)
	g.AddNewNodeToContainer(n1, n2)
	n3 := layoutgraph.NewNode(3, 30, 30)
	g.AddNewNodeToContainer(n1, n3)
	n4 := layoutgraph.NewNode(4, 50, 50)
	g.AddNewNodeToContainer(n1, n4)
	n5 := layoutgraph.NewNode(5, 50, 50)
	g.AddNewNodeToContainer(n1, n5)
	n6 := layoutgraph.NewNode(6, 50, 50)
	g.AddNewNodeToContainer(n1, n6)

	connect := func(from, to *layoutgraph.Node) {
		e := g.Connect(from, to)
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
	}

	connect(n2, n4)
	connect(n3, n4)
	connect(n3, n5)
	connect(n4, n6)
	connect(n5, n6)

	ctx := log.With(context.Background(), testlog.New(t))
	err := Assign(ctx, g, nil, Candidates(g))
	if err != nil {
		t.Fatal("No errors were expected")
	}

	if n2.Hierarchy.Levels()[n2] != 0 {
		t.Fatal("Wrong hierarchy level for n2")
	}
	if n3.Hierarchy.Levels()[n3] != 0 {
		t.Fatal("Wrong hierarchy level for n3")
	}
	if n4.Hierarchy.Levels()[n4] != 1 {
		t.Fatal("Wrong hierarchy level for n4")
	}
	if n5.Hierarchy.Levels()[n5] != 1 {
		t.Fatal("Wrong hierarchy level for n5")
	}
	if n6.Hierarchy.Levels()[n6] != 2 {
		t.Fatal("Wrong hierarchy level for n6")
	}
	if n1.Hierarchy != nil {
		t.Fatal("n1 should not have a hierarchy level")
	}
}

func TestAssignNodeHierarchyWithContainers(t *testing.T) {
	//            ┌──────────────┐                    ┌──────────────┐
	//            │              │                    │              │
	//            │              ├────────────┐       │              │
	//            │     0        │            │       │      1       │
	//            │              │            │       │              │
	//            │              │            │       │              │
	//            └───┬──────────┘            │       └─────┬────────┘
	//                │                       │             │
	//                │                       │             │
	//                │                       │             │
	//                │                       │             │
	//                │                       │             │
	//                │                       │             │
	// ┌──────────────┼───────────────────────┼─────────────┼──────────────────────┐
	// │              │                       │             │                      │
	// │  2           │                       │             │                      │
	// │    ┌─────────┼────┐           ┌──────▼───────┐     │    ┌──────────────┐  │
	// │    │  3      │    │           │              │     │    │              │  │
	// │    │  ┌──────▼─┐  │           │              │     └────►              │  │
	// │    │  │   4    │  │           │      5       │          │      6       │  │
	// │    │  └────────┘  │           │              │          │              │  │
	// │    │              │           │              │          │              │  │
	// │    └───────┬──────┘           └──────────────┘          └─────┬────────┘  │
	// │            │                                                  │           │
	// │            │                                                  │           │
	// └────────────┼──────────────────────────────────────────────────┼───────────┘
	//              │                                                  │
	//              │                                                  │
	//              │                                                  │
	//              │                                                  │
	//              │                                                  │
	//              │                    ┌──────────────┐              │
	//              │                    │              │              │
	//              │                    │              │              │
	//              └────────────────────►      7       ◄──────────────┘
	//                                   │              │
	//                                   │              │
	//                                   └──────────────┘
	g := layoutgraph.NewGraph()

	for i := 0; i < 8; i++ {
		g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i), 50, 50))
	}

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil:        {g.Nodes[0], g.Nodes[1], g.Nodes[2], g.Nodes[7]},
		g.Nodes[2]: {g.Nodes[3], g.Nodes[5], g.Nodes[6]},
		g.Nodes[3]: {g.Nodes[4]},
	}
	for container, children := range g.Containers {
		setContainerChildrenForTest(g, container, children)
	}

	connect := func(from, to int) {
		e := g.Connect(g.Nodes[from], g.Nodes[to])
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
	}

	connect(0, 4)
	connect(0, 5)
	connect(1, 6)
	connect(6, 7)
	connect(3, 7)

	ctx := log.With(context.Background(), testlog.New(t))
	err := Assign(ctx, g, nil, Candidates(g))
	if err != nil {
		t.Fatalf("No error expected: %v", err)
	}

	expectedHierarchy := []int{
		0,
		0,
		1,
		1,
		1,
		1,
		1,
		2,
	}
	for i, node := range g.Nodes {
		assert.Equal(t, expectedHierarchy[i], node.Hierarchy.Levels()[node])
	}
}

func TestAssignNodeHierarchyWithContainersAndNoEdgesToDescendants(t *testing.T) {
	//            ┌──────────────┐                    ┌──────────────┐
	//            │              │                    │              │
	//            │              ├────────────┐       │              │
	//            │     0        │            │       │      1       │
	//            │              │            │       │              │
	//            │              │            │       │              │
	//            └───┬──────────┘            │       └─────┬────────┘
	//                │                       │             │
	//                │                       │             │
	// ┌──────────────▼───────────────────────▼─────────────▼──────────────────────┐
	// │                                                                           │
	// │  2                                                                        │
	// │    ┌──────────────┐           ┌──────────────┐          ┌──────────────┐  │
	// │    │  3           │           │              │          │              │  │
	// │    │  ┌────────┐  │           │              │          │              │  │
	// │    │  │   4    │  │           │      5       │          │      6       │  │
	// │    │  └────────┘  │           │              │          │              │  │
	// │    │              │           │              │          │              │  │
	// │    └──────────────┘           └──────────────┘          └──────────────┘  │
	// │                                                                           │
	// │                                                                           │
	// └────────────┬──────────────────────────────────────────────────┬───────────┘
	//              │                                                  │
	//              │                    ┌──────────────┐              │
	//              │                    │              │              │
	//              │                    │              │              │
	//              └────────────────────►      7       ◄──────────────┘
	//                                   │              │
	//                                   │              │
	//                                   └──────────────┘
	g := layoutgraph.NewGraph()

	for i := 0; i < 8; i++ {
		g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i), 50, 50))
	}

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil:        {g.Nodes[0], g.Nodes[1], g.Nodes[2], g.Nodes[7]},
		g.Nodes[2]: {g.Nodes[3], g.Nodes[5], g.Nodes[6]},
		g.Nodes[3]: {g.Nodes[4]},
	}
	for container, children := range g.Containers {
		setContainerChildrenForTest(g, container, children)
	}

	connect := func(from, to int) {
		e := g.Connect(g.Nodes[from], g.Nodes[to])
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
	}

	connect(0, 2)
	connect(0, 2)
	connect(1, 2)
	connect(2, 7)
	connect(2, 7)

	ctx := log.With(context.Background(), testlog.New(t))
	err := Assign(ctx, g, nil, Candidates(g))
	if err != nil {
		t.Fatalf("No error expected: %v", err)
	}

	expectedHierarchy := []int{
		0,
		0,
		1,
		1,
		1,
		1,
		1,
		2,
	}
	for i, node := range g.Nodes {
		assert.Equal(t, expectedHierarchy[i], node.Hierarchy.Levels()[node])
	}
}

func TestAssignNodeHierarchyWithContainersAndInternalEdges(t *testing.T) {
	// The same graph as in TestAssignNodeHierarchyWithContainers, but with an edge inside the container
	//            ┌──────────────┐                    ┌──────────────┐
	//            │              │                    │              │
	//            │              ├────────────┐       │              │
	//            │     0        │            │       │      1       │
	//            │              │            │       │              │
	//            │              │            │       │              │
	//            └───┬──────────┘            │       └─────┬────────┘
	//                │                       │             │
	//                │                       │             │
	//                │                       │             │
	//                │                       │             │
	//                │                       │             │
	//                │                       │             │
	// ┌──────────────┼───────────────────────┼─────────────┼──────────────────────┐
	// │              │                       │             │                      │
	// │  2           │                       │             │                      │
	// │    ┌─────────┼────┐           ┌──────▼───────┐     │    ┌──────────────┐  │
	// │    │  3      │    │           │              │     │    │              │  │
	// │    │  ┌──────▼─┐  │           │              │     └────►              │  │
	// │    │  │   4    ├──┼───────────►      5       │          │      6       │  │
	// │    │  └────────┘  │           │              │          │              │  │
	// │    │              │           │              │          │              │  │
	// │    └───────┬──────┘           └──────────────┘          └─────┬────────┘  │
	// │            │                                                  │           │
	// │            │                                                  │           │
	// └────────────┼──────────────────────────────────────────────────┼───────────┘
	//              │                                                  │
	//              │                                                  │
	//              │                                                  │
	//              │                                                  │
	//              │                                                  │
	//              │                    ┌──────────────┐              │
	//              │                    │              │              │
	//              │                    │              │              │
	//              └────────────────────►      7       ◄──────────────┘
	//                                   │              │
	//                                   │              │
	//                                   └──────────────┘
	g := layoutgraph.NewGraph()

	for i := 0; i < 8; i++ {
		g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i), 50, 50))
	}

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil:        {g.Nodes[0], g.Nodes[1], g.Nodes[2], g.Nodes[7]},
		g.Nodes[2]: {g.Nodes[3], g.Nodes[5], g.Nodes[6]},
		g.Nodes[3]: {g.Nodes[4]},
	}
	for container, children := range g.Containers {
		setContainerChildrenForTest(g, container, children)
	}

	connect := func(from, to int) {
		e := g.Connect(g.Nodes[from], g.Nodes[to])
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
	}

	connect(0, 4)
	connect(0, 5)
	connect(1, 6)
	connect(6, 7)
	connect(3, 7)
	connect(4, 5)

	ctx := log.With(context.Background(), testlog.New(t))
	err := Assign(ctx, g, nil, Candidates(g))
	if err != nil {
		t.Fatalf("No error expected: %v", err)
	}

	for _, node := range g.Nodes {
		assert.Nil(t, node.Hierarchy)
	}
}

func TestIgnoreAbductionsForNodeDensityInHierarchies(t *testing.T) {
	// if we consider edge abductions for hierarchy validity, node 2 is too dense and then this is not a hierarchy
	//      ┌──────┐         ┌──────┐
	//      │      │         │      │
	//      │  0   │   ┌─────┤  1   ├────┐
	//      │      │   │     │      │    │
	//      └───┬──┘   │     └──┬───┘    │
	//          │      │        │        │
	//          │      │        │        │
	//          │      │        │        │
	//          │      │        │        │
	// ┌────────┼──────┼────────┼────────┼───────────────┐
	// │ 2      │      │        │        │               │
	// │    ┌───▼──┐   │     ┌──▼───┐    │    ┌──────┐   │
	// │    │      │   │     │      │    │    │      ├───┼──────────────────────────────────────────┐
	// │    │  3   ◄───┘ ┌───┤  4   │    └────►   5  │   │                                          │
	// │    │      │     │   |      ├───┐     │      ├───┼────────────────────────┐                 │
	// │    └──────┘     │   └──┬───┘   │     └──┬───┘   │                        │                 │
	// │                 │      │       │        │       │                        │                 │
	// └─────────────────┼──────┼───────┼────────┼───────┘                        │                 │
	//                   │      │       │        │                                │                 │
	//                   │      │       │        └───────────────┐                │                 │
	//                   │      │       │                        │                │                 │
	//                   │      │       │                        │                │                 │
	//      ┌──────┐     │   ┌──▼───┐   │     ┌──────┐        ┌──▼───┐         ┌──▼───┐         ┌───▼──┐
	//      │      ◄─────┘   │      │   └─────►      │        │      │         │      │         │      │
	//      │   6  │         │  7   │         │  8   │        │  9   │         │  10  │         │  11  │
	//      │      │         │      │         │      │        │      │         │      │         │      │
	//      └──────┘         └──────┘         └──────┘        └──────┘         └──────┘         └──────┘

	g := layoutgraph.NewGraph()

	for i := 0; i < 12; i++ {
		g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i), 50, 50))
	}

	connect := func(i, j int) {
		edge := g.Connect(g.Nodes[i], g.Nodes[j])
		edge.SourceArrowhead = layoutgraph.NoArrowhead
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	}

	connect(0, 3)
	connect(1, 3)
	connect(1, 4)
	connect(1, 5)
	connect(4, 6)
	connect(4, 7)
	connect(4, 8)
	connect(5, 9)
	connect(5, 10)
	connect(5, 11)

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil:        {g.Nodes[0], g.Nodes[1], g.Nodes[2], g.Nodes[6], g.Nodes[7], g.Nodes[8], g.Nodes[9], g.Nodes[10], g.Nodes[11]},
		g.Nodes[2]: {g.Nodes[3], g.Nodes[4], g.Nodes[5]},
	}
	for container, children := range g.Containers {
		setContainerChildrenForTest(g, container, children)
	}

	ctx := log.With(context.Background(), testlog.New(t))
	Assign(ctx, g, nil, Candidates(g))

	assert.Equal(t, 0, g.Nodes[0].Hierarchy.Levels()[g.Nodes[0]])
	assert.Equal(t, 0, g.Nodes[1].Hierarchy.Levels()[g.Nodes[1]])
	assert.Equal(t, 1, g.Nodes[2].Hierarchy.Levels()[g.Nodes[2]])
	assert.Equal(t, 1, g.Nodes[3].Hierarchy.Levels()[g.Nodes[3]])
	assert.Equal(t, 1, g.Nodes[4].Hierarchy.Levels()[g.Nodes[4]])
	assert.Equal(t, 1, g.Nodes[5].Hierarchy.Levels()[g.Nodes[5]])
	assert.Equal(t, 2, g.Nodes[6].Hierarchy.Levels()[g.Nodes[6]])
	assert.Equal(t, 2, g.Nodes[7].Hierarchy.Levels()[g.Nodes[7]])
	assert.Equal(t, 2, g.Nodes[8].Hierarchy.Levels()[g.Nodes[8]])
	assert.Equal(t, 2, g.Nodes[9].Hierarchy.Levels()[g.Nodes[9]])
	assert.Equal(t, 2, g.Nodes[10].Hierarchy.Levels()[g.Nodes[10]])
	assert.Equal(t, 2, g.Nodes[11].Hierarchy.Levels()[g.Nodes[11]])
}

func TestMakeSimpleDAG(t *testing.T) {
	// cycle graph: a -> b, b -> c, c -> a
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 11, 11))
	a.D2ID = new("a")
	b := g.AddNode(layoutgraph.NewNode(2, 12, 12))
	b.D2ID = new("b")
	c := g.AddNode(layoutgraph.NewNode(3, 13, 13))
	c.D2ID = new("c")

	// undirected edge
	g.Connect(a, b)
	// bidirectional
	e := g.Connect(b, c)
	e.SourceArrowhead = layoutgraph.TriangleArrowhead
	e.TargetArrowhead = layoutgraph.TriangleArrowhead

	// directed edge
	e = g.Connect(c, a)
	e.SourceArrowhead = layoutgraph.NoArrowhead
	e.TargetArrowhead = layoutgraph.TriangleArrowhead

	// loops should be ignored
	g.Connect(a, a)

	ctx := log.With(context.Background(), testlog.New(t))
	simpleDag := mustMakeSimpleDAG(t, ctx, g)

	assert.Equal(t, 3, len(simpleDag.Nodes))
	assert.Equal(t, 3, len(simpleDag.Edges))

	hasSource := false
	hasSink := false
	for i, n := range g.Nodes {
		assert.Equal(t, n.ID, simpleDag.Nodes[i].ID)
		assert.Equal(t, *n.D2ID, *simpleDag.Nodes[i].D2ID)
		assert.Equal(t, n.Width, simpleDag.Nodes[i].Width)
		assert.Equal(t, n.Height, simpleDag.Nodes[i].Height)
		assert.Equal(t, 2, len(simpleDag.Nodes[i].Edges))

		hasSource = hasSource || isSource(simpleDag.Nodes[i])
		hasSink = hasSink || isSink(simpleDag.Nodes[i])
	}

	hasAB := false
	hasAC := false
	hasBC := false
	for _, e := range simpleDag.Edges {
		assert.NotEqual(t, 0, e.ID)
		assert.True(t, e.IsDirected())
		if (e.From.ID == a.ID && e.To.ID == b.ID) || (e.From.ID == b.ID && e.To.ID == a.ID) {
			hasAB = true
		}
		if (e.From.ID == c.ID && e.To.ID == b.ID) || (e.From.ID == b.ID && e.To.ID == c.ID) {
			hasBC = true
		}
		if (e.From.ID == a.ID && e.To.ID == c.ID) || (e.From.ID == c.ID && e.To.ID == a.ID) {
			hasAC = true
		}
	}
	assert.True(t, hasAB)
	assert.True(t, hasAC)
	assert.True(t, hasBC)
}

func TestRemoveDuplicateEdges(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	c := g.AddNode(layoutgraph.NewNode(3, 10, 10))

	g.Connect(a, c)
	g.Connect(a, b)
	g.Connect(a, b)

	ctx := log.With(context.Background(), testlog.New(t))
	assert.NoError(t, removeDuplicateEdges(ctx, g))

	assert.Equal(t, 2, len(g.Edges))
	assert.Equal(t, 2, len(a.Edges))
	assert.Equal(t, 1, len(b.Edges))
	assert.Equal(t, 1, len(c.Edges))
	assert.Equal(t, 2, g.Edges[1].HierarchyRankWeight())
}

func TestMakeSimpleDAGPreservesOneRankWeightPerAuthoredEdge(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))

	connect := func(sourceArrow, targetArrow layoutgraph.Arrowhead) {
		edge := g.Connect(a, b)
		edge.SourceArrowhead = sourceArrow
		edge.TargetArrowhead = targetArrow
	}
	connect(layoutgraph.NoArrowhead, layoutgraph.TriangleArrowhead)
	connect(layoutgraph.NoArrowhead, layoutgraph.TriangleArrowhead)
	connect(layoutgraph.TriangleArrowhead, layoutgraph.NoArrowhead)
	connect(layoutgraph.NoArrowhead, layoutgraph.NoArrowhead)
	connect(layoutgraph.TriangleArrowhead, layoutgraph.TriangleArrowhead)
	// Loops are non-structural and do not contribute rank weight.
	g.Connect(a, a)

	dag := mustMakeSimpleDAG(t, context.Background(), g)
	if len(dag.Edges) != 1 {
		t.Fatalf("simple DAG has %d edges, want 1", len(dag.Edges))
	}
	if weight := dag.Edges[0].HierarchyRankWeight(); weight != 5 {
		t.Fatalf("simple DAG rank weight = %d, want one per non-loop authored edge (5)", weight)
	}
}

func TestMakeSimpleDAGNeutralExpansionKeepsUnitRankWeight(t *testing.T) {
	tests := []struct {
		name            string
		sourceArrowhead layoutgraph.Arrowhead
		targetArrowhead layoutgraph.Arrowhead
	}{
		{name: "undirected", sourceArrowhead: layoutgraph.NoArrowhead, targetArrowhead: layoutgraph.NoArrowhead},
		{name: "bidirectional", sourceArrowhead: layoutgraph.TriangleArrowhead, targetArrowhead: layoutgraph.TriangleArrowhead},
	}
	for _, test := range tests {
		for _, reverseDeclaration := range []bool{false, true} {
			name := test.name + "/forward-declaration"
			if reverseDeclaration {
				name = test.name + "/reverse-declaration"
			}
			t.Run(name, func(t *testing.T) {
				g := layoutgraph.NewGraph()
				a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
				b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
				from, to := a, b
				if reverseDeclaration {
					from, to = b, a
				}
				edge := g.Connect(from, to)
				edge.SourceArrowhead = test.sourceArrowhead
				edge.TargetArrowhead = test.targetArrowhead

				dag := mustMakeSimpleDAG(t, context.Background(), g)
				if len(dag.Edges) != 1 {
					t.Fatalf("simple DAG has %d edges, want 1", len(dag.Edges))
				}
				if weight := dag.Edges[0].HierarchyRankWeight(); weight != 1 {
					t.Fatalf("expanded authored edge has rank weight %d, want 1", weight)
				}
			})
		}
	}
}

func TestRemoveDuplicateEdgesCancellationPreservesGraphAndWeights(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	first := g.Connect(a, b)
	second := g.Connect(a, b)
	wantEdges := append([]*layoutgraph.Edge(nil), g.Edges...)
	wantIncident := append([]*layoutgraph.Edge(nil), a.Edges...)

	err := removeDuplicateEdges(&cancelHierarchyAfterErrChecks{Context: context.Background(), remaining: 1}, g)
	assert.Error(t, err)
	assert.Equal(t, wantEdges, g.Edges)
	assert.Equal(t, wantIncident, a.Edges)
	assert.Equal(t, 1, first.HierarchyRankWeight())
	assert.Equal(t, 1, second.HierarchyRankWeight())
}

func TestFindCycles(t *testing.T) {
	//                   ┌───┐
	//     ┌─────────────► 0 ◄──────────────────────┐
	//     │             └─▲─┘                      │
	//     │               │                        │
	//     │               │                        │
	//   ┌─┴─┐           ┌─┴─┐                      │
	//   │ 1 │           │ 2 │                      │
	//   └───┘           └─▲─┘                      │
	//                     │                        │
	//   ┌───┐             │      ┌───┐           ┌─┴─┐
	// ┌─┤ 3 ├─────────────┴──────┤ 4 ├──┐        │ 5 │
	// │ └─▲─┘                    └─▲─┘  │        └─▲─┘
	// │   │                        │    │          │
	// │   │             ┌───┐      │    │          │
	// │   ├─────────────┤ 6 ├──────┤    │          │
	// │   │             └─▲─┘      │    │          │
	// │   │               │        │    │          │
	// │   │             ┌─┴─┐      │    │          │
	// │   └─────────────┤ 7 ├──────┘    │          │
	// │                 └─▲─┘           │          │
	// │                   │             │          │
	// │ ┌───┐             │      ┌───┐  │         ┌┴──┐
	// └─► 8 ├─────────────┴──────┤ 9 ◄──┘         │ 10│
	//   └─▲─┘                    └─▲─┘            └─▲─┘
	//     │                        │                │
	//     │             ┌───┐      │                │
	//     └─────────────┤ 11├──────┴────────────────┘
	//                   └───┘
	g := layoutgraph.NewGraph()

	for i := 0; i < 12; i++ {
		g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i), 10, 10))
	}

	connect := func(i, j int) *layoutgraph.Edge {
		e := g.Connect(g.Nodes[i], g.Nodes[j])
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		return e
	}

	connect(11, 8)
	connect(11, 9)
	connect(11, 10)
	connect(8, 7)
	connect(9, 7)
	connect(10, 5)
	connect(7, 6)
	connect(7, 3)
	connect(7, 4)
	connect(6, 3)
	connect(6, 4)
	connect(3, 8)
	connect(3, 2)
	connect(4, 9)
	connect(4, 2)
	connect(5, 0)
	connect(1, 0)
	connect(2, 0)

	ctx := log.With(context.Background(), testlog.New(t))
	e87 := g.Edges[3]
	e97 := g.Edges[4]
	edgesToReverse := mustFindCycleEdges(t, ctx, g)

	assert.Equal(t, 2, len(edgesToReverse))
	assert.Contains(t, edgesToReverse, e87)
	assert.Contains(t, edgesToReverse, e97)
}
