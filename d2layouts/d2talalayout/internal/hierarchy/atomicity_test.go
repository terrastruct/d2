package hierarchy

import (
	"context"
	"errors"
	"math/rand"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type exactHierarchyTestSlice[T comparable] struct {
	header  []T
	backing []T
}

func captureExactHierarchyTestSlice[T comparable](values []T) exactHierarchyTestSlice[T] {
	return exactHierarchyTestSlice[T]{header: values, backing: slices.Clone(values[:cap(values)])}
}

func (snapshot exactHierarchyTestSlice[T]) assertRestored(t *testing.T, got []T, name string) {
	t.Helper()
	if len(got) != len(snapshot.header) || cap(got) != cap(snapshot.header) {
		t.Fatalf("%s header = len %d cap %d; want len %d cap %d", name, len(got), cap(got), len(snapshot.header), cap(snapshot.header))
	}
	if cap(got) > 0 && &got[:cap(got)][0] != &snapshot.header[:cap(snapshot.header)][0] {
		t.Fatalf("%s backing array identity changed", name)
	}
	if !slices.Equal(got[:cap(got)], snapshot.backing) {
		t.Fatalf("%s backing array contents changed", name)
	}
}

func hierarchyComponent(g *layoutgraph.Graph, base layoutgraph.EntityID) []*layoutgraph.Node {
	nodes := []*layoutgraph.Node{layoutgraph.NewNode(base, 20, 20), layoutgraph.NewNode(base+1, 20, 20), layoutgraph.NewNode(base+2, 20, 20)}
	for i, node := range nodes {
		node.TopLeft = geo.NewPoint(float64(base)*10+float64(i*30), float64(base))
		g.AddNewNodeToContainer(nil, node)
	}
	for i := 1; i < len(nodes); i++ {
		edge := g.Connect(nodes[i-1], nodes[i])
		edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	}
	return nodes
}

type cancelWhenOneHierarchyComponentAssigned struct {
	context.Context
	components [2][]*layoutgraph.Node
}

func allHierarchy(nodes []*layoutgraph.Node, assigned bool) bool {
	for _, node := range nodes {
		if (node.Hierarchy != nil) != assigned {
			return false
		}
	}
	return true
}

func (ctx *cancelWhenOneHierarchyComponentAssigned) Err() error {
	if (allHierarchy(ctx.components[0], true) && allHierarchy(ctx.components[1], false)) ||
		(allHierarchy(ctx.components[1], true) && allHierarchy(ctx.components[0], false)) {
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestAssignNodeHierarchyLateCancellationRollsBackAllComponents(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.IsRootHierarchy = true
	components := [2][]*layoutgraph.Node{hierarchyComponent(g, 10), hierarchyComponent(g, 20)}
	graphRefs := make(map[*layoutgraph.Node]*layoutgraph.Graph)
	edgeEndpoints := make(map[*layoutgraph.Edge][2]*layoutgraph.Node)
	adjacency := make(map[*layoutgraph.Node][]*layoutgraph.Edge)
	for _, node := range g.Nodes {
		graphRefs[node] = node.Graph
		adjacency[node] = slices.Clone(node.Edges)
	}
	for _, edge := range g.Edges {
		edgeEndpoints[edge] = [2]*layoutgraph.Node{edge.From, edge.To}
	}

	err := Assign(
		&cancelWhenOneHierarchyComponentAssigned{Context: context.Background(), components: components},
		g,
		nil,
		Candidates(g),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AssignNodeHierarchy error = %v, want context.Canceled", err)
	}
	for _, node := range g.Nodes {
		if node.Hierarchy != nil || node.Graph != graphRefs[node] || !slices.Equal(node.Edges, adjacency[node]) {
			t.Fatalf("node %d retained hierarchy or temporary topology after cancellation", node.ID)
		}
	}
	for _, edge := range g.Edges {
		if got := [2]*layoutgraph.Node{edge.From, edge.To}; got != edgeEndpoints[edge] {
			t.Fatalf("edge %d endpoints were not restored", edge.ID)
		}
	}
}

type cancelWhenAnyHierarchyPositionChanges struct {
	context.Context
	positions map[*layoutgraph.Node]geo.Point
}

func (ctx *cancelWhenAnyHierarchyPositionChanges) Err() error {
	for node, position := range ctx.positions {
		if node.TopLeft != nil && *node.TopLeft != position {
			return context.Canceled
		}
	}
	return ctx.Context.Err()
}

func TestPlaceHierarchiesLateCancellationRollsBackGeometryRoutesAndTopology(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.IsRootHierarchy = true
	hierarchyComponent(g, 10)
	hierarchyComponent(g, 20)
	if err := Assign(context.Background(), g, nil, Candidates(g)); err != nil {
		t.Fatal(err)
	}
	positions := make(map[*layoutgraph.Node]geo.Point)
	positionPointers := make(map[*layoutgraph.Node]*geo.Point)
	graphRefs := make(map[*layoutgraph.Node]*layoutgraph.Graph)
	for _, node := range g.Nodes {
		positions[node] = *node.TopLeft
		positionPointers[node] = node.TopLeft
		graphRefs[node] = node.Graph
	}
	routes := make(map[*layoutgraph.Edge]exactHierarchyTestSlice[*geo.Point])
	routeValues := make(map[*geo.Point]geo.Point)
	for _, edge := range g.Edges {
		backing := make([]*geo.Point, 4)
		backing[0] = edge.From.TopLeft.Copy()
		backing[1] = edge.To.TopLeft.Copy()
		backing[2] = geo.NewPoint(99, 99)
		backing[3] = geo.NewPoint(100, 100)
		edge.Points = backing[:2]
		routes[edge] = captureExactHierarchyTestSlice(edge.Points)
		for _, point := range edge.Points[:cap(edge.Points)] {
			routeValues[point] = *point
		}
	}

	err := Place(
		&cancelWhenAnyHierarchyPositionChanges{Context: context.Background(), positions: positions},
		g,
		nil,
		rand.New(rand.NewSource(1)),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PlaceHierarchies error = %v, want context.Canceled", err)
	}
	for _, node := range g.Nodes {
		if node.TopLeft != positionPointers[node] || *node.TopLeft != positions[node] || node.Graph != graphRefs[node] {
			t.Fatalf("node %d geometry or graph reference was not restored", node.ID)
		}
	}
	for _, edge := range g.Edges {
		routes[edge].assertRestored(t, edge.Points, "hierarchy edge route")
		for _, point := range edge.Points[:cap(edge.Points)] {
			if *point != routeValues[point] {
				t.Fatalf("edge %d route point was not restored", edge.ID)
			}
		}
	}
}

func newHierarchyExternalNearOwnerFixture(withHierarchy bool) (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Graph, *layoutgraph.Node) {
	graph := layoutgraph.NewGraph()
	local := layoutgraph.NewNode(1, 10, 10)
	graph.AddNewNodeToContainer(nil, local)

	externalOwner := layoutgraph.NewGraph()
	external := layoutgraph.NewNode(2, 10, 10)
	externalOwner.AddNewNodeToContainer(nil, external)
	local.AddNear(external)
	if withHierarchy {
		local.Hierarchy = newHierarchyWithLevels(map[*layoutgraph.Node]int{local: 0})
	}
	return graph, local, externalOwner, external
}

func TestPlaceHierarchiesRestoresExternalNearOwnerOnSuccess(t *testing.T) {
	graph, local, externalOwner, external := newHierarchyExternalNearOwnerFixture(false)

	if err := Place(t.Context(), graph, nil, rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}
	if local.Graph != graph {
		t.Fatalf("local node owner = %p, want graph %p", local.Graph, graph)
	}
	if external.Graph != externalOwner {
		t.Fatalf("external Near-only node owner = %p, want original owner %p", external.Graph, externalOwner)
	}
}

type failAfterExternalNearOwnerRedirect struct {
	context.Context
	external      *layoutgraph.Node
	originalOwner *layoutgraph.Graph
	redirectSeen  bool
	panicValue    any
}

func (*failAfterExternalNearOwnerRedirect) Done() <-chan struct{} {
	// Force work guards to poll Err so the fixture can observe the temporary
	// owner once at split completion and fail the following hierarchy work.
	return nil
}

func (ctx *failAfterExternalNearOwnerRedirect) Err() error {
	if ctx.external.Graph != ctx.originalOwner {
		if ctx.redirectSeen {
			if ctx.panicValue != nil {
				panic(ctx.panicValue)
			}
			return context.Canceled
		}
		ctx.redirectSeen = true
	}
	return ctx.Context.Err()
}

func TestPlaceHierarchyComponentRestoresExternalNearOwnerOnError(t *testing.T) {
	graph, local, externalOwner, external := newHierarchyExternalNearOwnerFixture(true)
	ctx := &failAfterExternalNearOwnerRedirect{
		Context:       t.Context(),
		external:      external,
		originalOwner: externalOwner,
	}

	err := place(ctx, graph, nil, rand.New(rand.NewSource(1)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("place error = %v, want context.Canceled", err)
	}
	if !ctx.redirectSeen {
		t.Fatal("fixture did not observe the temporary external owner")
	}
	if local.Graph != graph {
		t.Fatalf("local node owner = %p, want graph %p", local.Graph, graph)
	}
	if external.Graph != externalOwner {
		t.Fatalf("external Near-only node owner = %p, want original owner %p", external.Graph, externalOwner)
	}
}

func TestPlaceHierarchyComponentRestoresExternalNearOwnerOnPanic(t *testing.T) {
	graph, local, externalOwner, external := newHierarchyExternalNearOwnerFixture(true)
	panicValue := &struct{}{}
	ctx := &failAfterExternalNearOwnerRedirect{
		Context:       t.Context(),
		external:      external,
		originalOwner: externalOwner,
		panicValue:    panicValue,
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = place(ctx, graph, nil, rand.New(rand.NewSource(1)))
	}()
	if recovered != panicValue {
		t.Fatalf("recovered panic = %v, want sentinel %p", recovered, panicValue)
	}
	if !ctx.redirectSeen {
		t.Fatal("fixture did not observe the temporary external owner")
	}
	if local.Graph != graph {
		t.Fatalf("local node owner = %p, want graph %p", local.Graph, graph)
	}
	if external.Graph != externalOwner {
		t.Fatalf("external Near-only node owner = %p, want original owner %p", external.Graph, externalOwner)
	}
}
