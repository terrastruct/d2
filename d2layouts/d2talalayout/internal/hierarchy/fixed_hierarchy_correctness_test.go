package hierarchy

import (
	"context"
	"math/rand"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func connectFixedHierarchyTestEdge(g *layoutgraph.Graph, from, to *layoutgraph.Node) {
	edge := g.Connect(from, to)
	edge.TargetArrowhead = layoutgraph.TriangleArrowhead
}

func TestAssignNodeHierarchyKeepsUnrelatedForcedComponent(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.IsRootHierarchy = true
	chain := []*layoutgraph.Node{layoutgraph.NewNode(1, 40, 30), layoutgraph.NewNode(2, 40, 30), layoutgraph.NewNode(3, 40, 30)}
	for _, node := range chain {
		g.AddNewNodeToContainer(nil, node)
	}
	connectFixedHierarchyTestEdge(g, chain[0], chain[1])
	connectFixedHierarchyTestEdge(g, chain[1], chain[2])
	fixed := layoutgraph.NewNode(10, 40, 30)
	fixed.TopLeft = geo.NewPoint(700, 350)
	fixed.FixedTopLeft = fixed.TopLeft.Copy()
	g.AddNewNodeToContainer(nil, fixed)

	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	for _, node := range chain {
		if node.Hierarchy == nil {
			t.Fatalf("unrelated directed component node %d has no hierarchy", node.ID)
		}
	}
	if fixed.Hierarchy != nil {
		t.Fatal("fixed isolate was assigned a hierarchy")
	}
	before := fixed.TopLeft.Copy()
	if err := Place(context.Background(), g, nil, rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}
	if !fixed.TopLeft.Equals(before) {
		t.Fatalf("fixed isolate moved from %v to %v", before, fixed.TopLeft)
	}
}

func TestAssignNodeHierarchyKeepsUnrelatedAutomaticComponent(t *testing.T) {
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, 8)
	for index := range nodes {
		nodes[index] = layoutgraph.NewNode(layoutgraph.EntityID(index+1), 40, 30)
		g.AddNewNodeToContainer(nil, nodes[index])
	}
	for _, endpoints := range [][2]int{
		{0, 2}, {1, 3}, {2, 4}, {3, 5}, {2, 5}, {4, 6}, {5, 7},
	} {
		connectFixedHierarchyTestEdge(g, nodes[endpoints[0]], nodes[endpoints[1]])
	}
	fixed := layoutgraph.NewNode(20, 40, 30)
	fixed.TopLeft = geo.NewPoint(700, 350)
	fixed.FixedTopLeft = fixed.TopLeft.Copy()
	g.AddNewNodeToContainer(nil, fixed)

	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	hierarchy := nodes[0].Hierarchy
	if hierarchy == nil {
		t.Fatal("unrelated directed component was not automatically classified as a hierarchy")
	}
	for _, node := range nodes {
		if node.Hierarchy != hierarchy {
			t.Fatalf("node %d did not share the automatic hierarchy", node.ID)
		}
	}
	if fixed.Hierarchy != nil {
		t.Fatal("fixed isolate was assigned a hierarchy")
	}
}

func TestPlaceHierarchiesDoesNotMoveFixedDescendant(t *testing.T) {
	g := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 200, 160)
	container.TopLeft = geo.NewPoint(100, 100)
	peer := layoutgraph.NewNode(2, 50, 40)
	peer.TopLeft = geo.NewPoint(500, 100)
	fixedChild := layoutgraph.NewNode(3, 50, 40)
	fixedChild.TopLeft = geo.NewPoint(130, 130)
	fixedChild.FixedTopLeft = fixedChild.TopLeft.Copy()
	g.AddNewNodeToContainer(nil, container)
	g.AddNewNodeToContainer(nil, peer)
	g.AddNewNodeToContainer(container, fixedChild)
	connectFixedHierarchyTestEdge(g, container, peer)

	hierarchy := newHierarchyWithLevels(map[*layoutgraph.Node]int{
		container:  0,
		fixedChild: 0,
		peer:       1,
	})
	container.Hierarchy = hierarchy
	fixedChild.Hierarchy = hierarchy
	peer.Hierarchy = hierarchy

	before := fixedChild.TopLeft.Copy()
	if err := Place(context.Background(), g, nil, rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}
	if !fixedChild.TopLeft.Equals(before) {
		t.Fatalf("fixed descendant moved from %v to %v", before, fixedChild.TopLeft)
	}
}

func TestAssignNodeHierarchyRejectsContainerWithFixedDescendant(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.IsRootHierarchy = true
	container := layoutgraph.NewNode(1, 200, 160)
	peer := layoutgraph.NewNode(2, 50, 40)
	fixedChild := layoutgraph.NewNode(3, 50, 40)
	fixedChild.TopLeft = geo.NewPoint(130, 130)
	fixedChild.FixedTopLeft = fixedChild.TopLeft.Copy()
	g.AddNewNodeToContainer(nil, container)
	g.AddNewNodeToContainer(nil, peer)
	g.AddNewNodeToContainer(container, fixedChild)
	connectFixedHierarchyTestEdge(g, container, peer)

	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	for _, node := range []*layoutgraph.Node{container, peer, fixedChild} {
		if node.Hierarchy != nil {
			t.Fatalf("node %d was assigned to a hierarchy containing a fixed descendant", node.ID)
		}
	}
}
