package hierarchy

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func connectDirectedHierarchyEdge(g *layoutgraph.Graph, from, to *layoutgraph.Node) *layoutgraph.Edge {
	edge := g.Connect(from, to)
	edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	return edge
}

func newAutomaticHierarchyGraph() (*layoutgraph.Graph, []*layoutgraph.Node) {
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, 8)
	for index := range nodes {
		nodes[index] = layoutgraph.NewNode(layoutgraph.EntityID(index+1), 100, 100)
		g.AddNewNodeToContainer(nil, nodes[index])
	}
	for _, endpoints := range [][2]int{
		{0, 1}, {0, 2}, {0, 3},
		{1, 4}, {2, 5}, {3, 6},
		{4, 7}, {5, 7}, {6, 7},
	} {
		connectDirectedHierarchyEdge(g, nodes[endpoints[0]], nodes[endpoints[1]])
	}
	return g, nodes
}

func requireSharedHierarchy(t *testing.T, nodes []*layoutgraph.Node) {
	t.Helper()
	hierarchy := nodes[0].Hierarchy
	if hierarchy == nil {
		t.Fatal("directed four-level graph was not assigned a hierarchy")
	}
	for _, node := range nodes {
		if node.Hierarchy != hierarchy {
			t.Fatalf("node %s did not share the hierarchy", node.DebugID())
		}
	}
}

func TestHierarchyEdgeDirectionIgnoresSelfLoops(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	connectDirectedHierarchyEdge(g, a, b)
	connectDirectedHierarchyEdge(g, a, a)

	hierarchy := newHierarchyWithLevels(map[*layoutgraph.Node]int{a: 0, b: 1})
	forward, backwardOrNeutral := countEdgeDirection(hierarchy, g)
	if forward != 1 || backwardOrNeutral != 0 {
		t.Fatalf("self-loop changed structural direction counts to forward=%d backward-or-neutral=%d", forward, backwardOrNeutral)
	}
}

func TestHierarchyPlacementDegreeIgnoresSelfLoops(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	connectDirectedHierarchyEdge(g, a, b)
	connectDirectedHierarchyEdge(g, a, a)

	if degree := newPlacementNode(0, a).degree(); degree != 1 {
		t.Fatalf("placement degree = %d, want one structural edge", degree)
	}
}

func TestHierarchyClassificationIgnoresSourceSelfLoop(t *testing.T) {
	g, nodes := newAutomaticHierarchyGraph()
	connectDirectedHierarchyEdge(g, nodes[0], nodes[0])

	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	requireSharedHierarchy(t, nodes)
}

func TestAutomaticHierarchyStatisticsIgnoreSelfLoops(t *testing.T) {
	t.Run("direction ratio", func(t *testing.T) {
		g, nodes := newAutomaticHierarchyGraph()
		for _, node := range nodes {
			connectDirectedHierarchyEdge(g, node, node)
		}

		if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
			t.Fatal(err)
		}
		requireSharedHierarchy(t, nodes)
	})

	t.Run("node degree", func(t *testing.T) {
		g, nodes := newAutomaticHierarchyGraph()
		for range 5 {
			connectDirectedHierarchyEdge(g, nodes[1], nodes[1])
		}

		if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
			t.Fatal(err)
		}
		requireSharedHierarchy(t, nodes)
	})
}

func TestHierarchicalContainerIgnoresDescendantSelfLoop(t *testing.T) {
	g, nodes := newAutomaticHierarchyGraph()
	nodes[0].Height = 40
	child := layoutgraph.NewNode(9, 20, 10)
	g.AddNewNodeToContainer(nodes[0], child)
	connectDirectedHierarchyEdge(g, child, child)

	if !isEligibleContainer(g, nodes[0]) {
		t.Fatal("a descendant self-loop made its otherwise isolated container ineligible for hierarchy placement")
	}
	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	requireSharedHierarchy(t, nodes)
}

func TestOneManyOneClassificationIgnoresSelfLoopOrder(t *testing.T) {
	tests := []struct {
		name      string
		loopNode  int
		loopFirst bool
	}{
		{name: "middle loop first", loopNode: 1, loopFirst: true},
		{name: "middle loop last", loopNode: 1, loopFirst: false},
		{name: "source loop", loopNode: 0, loopFirst: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			nodes := make([]*layoutgraph.Node, 5)
			for index := range nodes {
				nodes[index] = layoutgraph.NewNode(layoutgraph.EntityID(index+1), 100, 100)
				g.AddNewNodeToContainer(nil, nodes[index])
			}
			if test.loopFirst {
				connectDirectedHierarchyEdge(g, nodes[test.loopNode], nodes[test.loopNode])
			}
			for middle := 1; middle <= 3; middle++ {
				connectDirectedHierarchyEdge(g, nodes[0], nodes[middle])
				connectDirectedHierarchyEdge(g, nodes[middle], nodes[4])
			}
			if !test.loopFirst {
				connectDirectedHierarchyEdge(g, nodes[test.loopNode], nodes[test.loopNode])
			}

			if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
				t.Fatal(err)
			}
			for _, node := range nodes {
				if node.Hierarchy != nil {
					t.Fatalf("1-N-1 graph was classified as a hierarchy with %s", test.name)
				}
			}
		})
	}
}
