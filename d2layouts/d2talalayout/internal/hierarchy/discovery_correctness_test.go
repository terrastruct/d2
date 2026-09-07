package hierarchy

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/loops"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/trees"
	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/log"
)

type cancelWhenHierarchyMembershipCleared struct {
	context.Context
	hierarchy *layoutgraph.Hierarchy
	members   int
}

func (ctx *cancelWhenHierarchyMembershipCleared) Err() error {
	if len(ctx.hierarchy.Levels()) < ctx.members {
		return context.Canceled
	}
	return ctx.Context.Err()
}

func mustMakeSimpleDAG(t *testing.T, ctx context.Context, g *layoutgraph.Graph) *layoutgraph.Graph {
	t.Helper()
	dag, err := makeSimpleDAG(ctx, g)
	if err != nil {
		t.Fatal(err)
	}
	return dag
}

func newHierarchyRecomputeGraph() (*layoutgraph.Graph, []*layoutgraph.Node) {
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, 6)
	for index := range nodes {
		nodes[index] = layoutgraph.NewNode(layoutgraph.EntityID(index+1), 10, 10)
		g.AddNewNodeToContainer(nil, nodes[index])
	}
	for _, endpoints := range [][2]int{{0, 2}, {0, 3}, {1, 3}, {2, 4}, {3, 5}} {
		edge := g.Connect(nodes[endpoints[0]], nodes[endpoints[1]])
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	}
	return g, nodes
}

func TestAssignNodeHierarchyRecomputesDerivedMembership(t *testing.T) {
	g, nodes := newHierarchyRecomputeGraph()
	ctx := log.With(context.Background(), testlog.New(t))
	assign := func() {
		t.Helper()
		if err := Assign(ctx, g, nil, Candidates(g)); err != nil {
			t.Fatal(err)
		}
	}

	assign()
	first := nodes[0].Hierarchy
	if first == nil {
		t.Fatal("directed graph was not assigned a hierarchy")
	}
	for _, node := range nodes {
		if node.Hierarchy != first {
			t.Fatalf("node %s did not share the first hierarchy", node.DebugID())
		}
	}
	// Reproduce the map-only stale membership that PreprocessHierarchies used
	// to leave behind for isolated descendants.
	nodes[len(nodes)-1].Hierarchy = nil

	assign()
	second := nodes[0].Hierarchy
	if second == nil || second == first {
		t.Fatal("repeated assignment did not recompute derived hierarchy state")
	}
	if len(first.Levels()) != 0 {
		t.Fatalf("superseded hierarchy retained %d members", len(first.Levels()))
	}

	for _, edge := range g.Edges {
		edge.TargetArrowhead = layoutgraph.NoArrowhead
	}
	assign()
	for _, node := range nodes {
		if node.Hierarchy != nil {
			t.Fatalf("node %s retained a stale hierarchy after its edges became undirected", node.DebugID())
		}
	}
	if len(second.Levels()) != 0 {
		t.Fatalf("removed hierarchy retained %d members", len(second.Levels()))
	}
}

func TestAssignNodeHierarchyRecomputesContainerMembership(t *testing.T) {
	data, err := os.ReadFile("../testdata/layout/simple_container_hierarchy/graph.input.json")
	if err != nil {
		t.Fatal(err)
	}
	var serialized graphjson.SerializedGraph
	if err := json.Unmarshal(data, &serialized); err != nil {
		t.Fatal(err)
	}
	ctx := log.With(t.Context(), testlog.New(t))
	g := layoutgraph.NewGraph()
	if err := graphjson.Deserialize(ctx, &serialized, g); err != nil {
		t.Fatal(err)
	}
	if err := grouping.AddSequences(ctx, g, rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}
	loops.ComputeOffsets(g)
	labeling.Initialize(g)
	g.ComputeNodeSpacing()
	if err := trees.Preprocess(ctx, g); err != nil {
		t.Fatal(err)
	}
	assign := func() {
		t.Helper()
		if err := Assign(ctx, g, nil, Candidates(g)); err != nil {
			t.Fatal(err)
		}
	}

	assign()
	first := make(map[*layoutgraph.Node]*layoutgraph.Hierarchy)
	for _, node := range g.Nodes {
		if node.IsContainer() && node.Hierarchy != nil {
			first[node] = node.Hierarchy
		}
	}
	if len(first) == 0 {
		t.Fatal("fixture did not assign any container to a hierarchy")
	}

	assign()
	for node, previous := range first {
		if node.Hierarchy == nil {
			t.Fatalf("container %s lost its hierarchy on recomputation", node.DebugID())
		}
		if node.Hierarchy == previous {
			t.Fatalf("container %s retained its prior hierarchy on recomputation", node.DebugID())
		}
	}
}

func TestAssignNodeHierarchyCreatesDistinctHierarchiesForDisconnectedComponents(t *testing.T) {
	g := layoutgraph.NewGraph()
	// Force hierarchy assignment so this test isolates component ownership from
	// the automatic hierarchy classifier.
	g.IsRootHierarchy = true
	addComponent := func(base layoutgraph.EntityID) []*layoutgraph.Node {
		nodes := []*layoutgraph.Node{layoutgraph.NewNode(base, 10, 10), layoutgraph.NewNode(base+1, 10, 10), layoutgraph.NewNode(base+2, 10, 10)}
		for _, node := range nodes {
			g.AddNewNodeToContainer(nil, node)
		}
		for index := 1; index < len(nodes); index++ {
			edge := g.Connect(nodes[index-1], nodes[index])
			edge.TargetArrowhead = layoutgraph.TriangleArrowhead
		}
		return nodes
	}

	first := addComponent(1)
	second := addComponent(10)
	if err := Assign(
		log.With(context.Background(), testlog.New(t)),
		g,
		nil,
		Candidates(g),
	); err != nil {
		t.Fatal(err)
	}

	assertComponent := func(nodes []*layoutgraph.Node) *layoutgraph.Hierarchy {
		t.Helper()
		hierarchy := nodes[0].Hierarchy
		if hierarchy == nil {
			t.Fatal("component was not assigned a hierarchy")
		}
		for _, node := range nodes {
			if node.Hierarchy != hierarchy {
				t.Fatalf("node %s does not share its component hierarchy", node.DebugID())
			}
			if _, exists := hierarchy.Levels()[node]; !exists {
				t.Fatalf("hierarchy does not contain node %s", node.DebugID())
			}
		}
		if len(hierarchy.Levels()) != len(nodes) {
			t.Fatalf("hierarchy has %d members, want %d", len(hierarchy.Levels()), len(nodes))
		}
		return hierarchy
	}

	if assertComponent(first) == assertComponent(second) {
		t.Fatal("disconnected components shared one hierarchy")
	}
}

func TestAssignNodeHierarchyRecomputeRollbackRestoresMembership(t *testing.T) {
	g, nodes := newHierarchyRecomputeGraph()
	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	prior := nodes[0].Hierarchy
	if prior == nil || len(prior.Levels()) != len(nodes) {
		t.Fatal("fixture did not create the expected hierarchy")
	}

	ctx := &cancelWhenHierarchyMembershipCleared{
		Context:   context.Background(),
		hierarchy: prior,
		members:   len(nodes),
	}
	err := Assign(ctx, g, nil, Candidates(g))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AssignNodeHierarchy error = %v, want context.Canceled", err)
	}
	if len(prior.Levels()) != len(nodes) {
		t.Fatalf("rollback restored %d hierarchy members, want %d", len(prior.Levels()), len(nodes))
	}
	for _, node := range nodes {
		if node.Hierarchy != prior {
			t.Fatalf("rollback did not restore node %s's hierarchy", node.DebugID())
		}
		if _, exists := prior.Levels()[node]; !exists {
			t.Fatalf("rollback did not restore node %s's reverse membership", node.DebugID())
		}
	}
}

func TestAssignNodeHierarchyClearsMembershipOfExtractedTreeNodes(t *testing.T) {
	g, nodes := newHierarchyRecomputeGraph()
	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	prior := nodes[0].Hierarchy
	if prior == nil {
		t.Fatal("fixture did not create a hierarchy")
	}

	if err := trees.Preprocess(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	active := make(map[*layoutgraph.Node]struct{}, len(g.Nodes))
	for _, node := range g.Nodes {
		active[node] = struct{}{}
	}
	var extracted []*layoutgraph.Node
	for _, node := range nodes {
		if _, exists := active[node]; !exists {
			extracted = append(extracted, node)
		}
	}
	if len(extracted) == 0 {
		t.Fatal("fixture did not extract any tree nodes")
	}

	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	if len(prior.Levels()) != 0 {
		t.Fatalf("superseded hierarchy retained %d members", len(prior.Levels()))
	}
	for _, node := range extracted {
		if node.Hierarchy != nil {
			t.Fatalf("extracted tree node %s retained a stale hierarchy", node.DebugID())
		}
	}
}

func TestAssignNodeHierarchyDoesNotClearExternalNearMembership(t *testing.T) {
	g := layoutgraph.NewGraph()
	local := layoutgraph.NewNode(1, 10, 10)
	g.AddNewNodeToContainer(nil, local)

	externalOwner := layoutgraph.NewGraph()
	external := layoutgraph.NewNode(2, 10, 10)
	externalOwner.AddNewNodeToContainer(nil, external)
	externalHierarchy := newHierarchyWithLevels(map[*layoutgraph.Node]int{local: 0, external: 0})
	local.Hierarchy = externalHierarchy
	external.Hierarchy = externalHierarchy
	local.Nears[external] = struct{}{}

	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	if external.Hierarchy != externalHierarchy {
		t.Fatal("hierarchy recomputation mutated an external Near-only node")
	}
	if local.Hierarchy != nil {
		t.Fatal("hierarchy recomputation retained local membership")
	}
	if _, exists := externalHierarchy.Levels()[local]; exists {
		t.Fatal("hierarchy recomputation retained local reverse membership")
	}
	if level, exists := externalHierarchy.Levels()[external]; !exists || level != 0 {
		t.Fatal("hierarchy recomputation mutated external reverse membership")
	}
}

func TestAssignNodeHierarchyRestoresExternalEdgeOwner(t *testing.T) {
	graph := layoutgraph.NewGraph()
	graph.IsRootHierarchy = true
	local := layoutgraph.NewNode(1, 10, 10)
	graph.AddNewNodeToContainer(nil, local)

	externalOwner := layoutgraph.NewGraph()
	external := layoutgraph.NewNode(2, 10, 10)
	externalOwner.AddNewNodeToContainer(nil, external)
	graph.Connect(local, external)

	if err := Assign(t.Context(), graph, nil, Candidates(graph)); err != nil {
		t.Fatal(err)
	}
	if local.Graph != graph {
		t.Fatalf("local node owner = %p, want graph %p", local.Graph, graph)
	}
	if external.Graph != externalOwner {
		t.Fatalf("external edge-only node owner = %p, want original owner %p", external.Graph, externalOwner)
	}
}

func TestMakeSimpleDAGPreservesSourcesAndSinks(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	c := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	for _, endpoints := range [][2]*layoutgraph.Node{{a, b}, {b, c}} {
		edge := g.Connect(endpoints[0], endpoints[1])
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	}

	ctx := log.With(context.Background(), testlog.New(t))
	for i := 0; i < 2; i++ {
		dag := mustMakeSimpleDAG(t, ctx, g)
		sources, sinks := 0, 0
		for _, node := range dag.Nodes {
			if isSource(node) {
				sources++
			} else if isSink(node) {
				sinks++
			}
		}
		if sources != 1 || sinks != 1 {
			t.Fatalf("pass %d: DAG sources=%d sinks=%d, want 1 and 1", i+1, sources, sinks)
		}
	}
}

func mustFindCycleEdges(t *testing.T, ctx context.Context, g *layoutgraph.Graph) map[*layoutgraph.Edge]struct{} {
	t.Helper()
	edges, err := findCycleEdges(ctx, g)
	if err != nil {
		t.Fatal(err)
	}
	return edges
}

func TestHierarchyRespectsSourceArrowDirection(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	// Declared a <- b: the semantic direction is b to a.
	edge := g.Connect(a, b)
	edge.SourceArrowhead = layoutgraph.TriangleArrowhead
	edge.TargetArrowhead = layoutgraph.NoArrowhead

	hierarchy := newHierarchyWithLevels(map[*layoutgraph.Node]int{b: 0, a: 1})
	forward, backwardOrNeutral := countEdgeDirection(hierarchy, g)
	if forward != 1 || backwardOrNeutral != 0 {
		t.Fatalf("source-arrow edge counted as forward=%d backward-or-neutral=%d", forward, backwardOrNeutral)
	}

	dag := mustMakeSimpleDAG(t, log.With(context.Background(), testlog.New(t)), g)
	if len(dag.Edges) != 1 {
		t.Fatalf("expected one DAG edge, got %d", len(dag.Edges))
	}
	if dag.Edges[0].From.ID != b.ID || dag.Edges[0].To.ID != a.ID {
		t.Fatalf("semantic %d -> %d became DAG %d -> %d", b.ID, a.ID, dag.Edges[0].From.ID, dag.Edges[0].To.ID)
	}
}
