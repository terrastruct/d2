package routing

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type cancelWhenOVGChanges struct {
	context.Context
	shouldCancel func() bool
}

func (ctx *cancelWhenOVGChanges) Err() error {
	if ctx.shouldCancel() {
		return context.Canceled
	}
	return nil
}

func newOVGCancellationGraph(targetX, targetY float64) *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	source := layoutgraph.NewNode(1, 40, 40)
	source.TopLeft = geo.NewPoint(0, 0)
	target := layoutgraph.NewNode(2, 40, 40)
	target.TopLeft = geo.NewPoint(targetX, targetY)
	graph.AddNode(source)
	graph.AddNode(target)
	graph.Connect(source, target)
	return graph
}

func TestOVGContextConstructorCanceledBeforeWork(t *testing.T) {
	ovg, err := buildOVGFromGraph(canceledContext(), newOVGCancellationGraph(200, 100), nil)
	if ovg != nil {
		t.Fatalf("OVG = %v, want nil after preflight cancellation", ovg)
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestOVGIntersectionsCanceledDuringCartesianProduct(t *testing.T) {
	graph := newOVGCancellationGraph(200, 100)
	ovg := NewOVG(graph.Nodes)
	ovg.addPorts(graph, newBackgroundOVGBuildGuardForTest(t))
	initialNodes := len(ovg.Nodes)
	ctx := &cancelWhenOVGChanges{
		Context:      context.Background(),
		shouldCancel: func() bool { return len(ovg.Nodes) > initialNodes },
	}

	err := ovg.addNodesIntersections(graph, newOVGBuildGuardForTest(ctx, t))
	if len(ovg.Nodes) == initialNodes {
		t.Fatal("intersection construction did no work before cancellation")
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestOVGBoundarySweepCanceledAfterFirstAddition(t *testing.T) {
	graph := layoutgraph.NewGraph()
	ovg := NewOVG(nil)
	ovg.AddNode(NewOVGNode(geo.NewPoint(0, 0)))
	ovg.AddNode(NewOVGNode(geo.NewPoint(100, 100)))
	initialNodes := len(ovg.Nodes)
	ctx := &cancelWhenOVGChanges{
		Context:      context.Background(),
		shouldCancel: func() bool { return len(ovg.Nodes) > initialNodes },
	}

	err := ovg.addNewBoundaryLayers(graph, geo.NewPoint(0, 0), geo.NewPoint(100, 100), newOVGBuildGuardForTest(ctx, t))
	if len(ovg.Nodes) == initialNodes {
		t.Fatal("boundary construction did no work before cancellation")
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestOVGTunnelSweepCanceledAfterFirstConnection(t *testing.T) {
	graph := newOVGCancellationGraph(300, 0)
	ovg := NewOVG(graph.Nodes)
	ovg.addPorts(graph, newBackgroundOVGBuildGuardForTest(t))
	initialEdges := len(ovg.Edges)
	ctx := &cancelWhenOVGChanges{
		Context:      context.Background(),
		shouldCancel: func() bool { return len(ovg.Edges) > initialEdges },
	}

	err := ovg.addTunnels(graph, newOVGBuildGuardForTest(ctx, t))
	if len(ovg.Edges) == initialEdges {
		t.Fatal("tunnel construction did no work before cancellation")
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestTunnelRangesCanceledDuringNodeSweep(t *testing.T) {
	graph := newOVGCancellationGraph(300, 0)
	for i := 0; i < 3; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(3+i), 20, 20)
		node.TopLeft = geo.NewPoint(100+float64(i*30), 200)
		graph.AddNode(node)
	}

	// Allow guard construction, the helper preflight, and range construction,
	// then cancel as the outer node sweep begins.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 4}
	_, _, err := tunnelRangesBetween(graph, graph.Nodes[0], graph.Nodes[1], true, newOVGBuildGuardForTest(ctx, t))
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestOVGConnectSweepCanceledAfterFirstConnection(t *testing.T) {
	graph := layoutgraph.NewGraph()
	ovg := NewOVG(nil)
	for _, x := range []float64{0, 10, 20} {
		ovg.AddNode(NewOVGNode(geo.NewPoint(x, 0)))
	}
	ctx := &cancelWhenOVGChanges{
		Context:      context.Background(),
		shouldCancel: func() bool { return len(ovg.Edges) > 0 },
	}

	err := ovg.connectNodes(graph, newOVGBuildGuardForTest(ctx, t))
	if len(ovg.Edges) == 0 {
		t.Fatal("connect sweep did no work before cancellation")
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestOVGContainerMappingCanceledAfterFirstNode(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 200, 200)
	container.TopLeft = geo.NewPoint(0, 0)
	child := layoutgraph.NewNode(2, 20, 20)
	child.TopLeft = geo.NewPoint(10, 10)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, child)

	ovg := NewOVG(nil)
	first := ovg.AddNode(NewOVGNode(geo.NewPoint(50, 50)))
	ovg.AddNode(NewOVGNode(geo.NewPoint(100, 100)))
	ctx := &cancelWhenOVGChanges{
		Context:      context.Background(),
		shouldCancel: func() bool { return first.Container != nil },
	}

	err := ovg.mapNodesToContainer(graph, newOVGBuildGuardForTest(ctx, t))
	if first.Container != container {
		t.Fatalf("first OVG node container = %v, want %v", first.Container, container)
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestOVGNearPortSearchCanceledDuringBFS(t *testing.T) {
	owner := layoutgraph.NewNode(1, 20, 20)
	owner.TopLeft = geo.NewPoint(0, 0)
	ovg := NewOVG(nil)
	port := ovg.AddNode(NewOVGNode(geo.NewPoint(20, 10)))
	port.addPortOwner(owner, geo.Right, true)
	adjacent := ovg.AddNode(NewOVGNode(geo.NewPoint(25, 10)))
	ovg.Ports[owner] = []*OVGNode{port}
	ovg.Connect(port, adjacent)
	ctx := &cancelWhenOVGChanges{
		Context: context.Background(),
		shouldCancel: func() bool {
			_, flagged := adjacent.IsNearPort[owner]
			return flagged
		},
	}

	err := ovg.flagNodesNearPorts(newOVGBuildGuardForTest(ctx, t))
	if _, flagged := adjacent.IsNearPort[owner]; !flagged {
		t.Fatal("near-port BFS did no work before cancellation")
	}
	requireCanceledAt(t, err, "EdgeRouting")
}

func TestHierarchyCancellationRestoresMirroredNodes(t *testing.T) {
	graph := layoutgraph.NewGraph()
	left := graph.AddNode(layoutgraph.NewNode(1, 100, 100))
	left.TopLeft = geo.NewPoint(0, 0)
	right := graph.AddNode(layoutgraph.NewNode(2, 100, 100))
	right.TopLeft = geo.NewPoint(100, 0)
	graph.Directions[nil] = geo.Left
	hierarchy := newHierarchyWithLevels(map[*layoutgraph.Node]int{left: 0, right: 0})
	left.Hierarchy = hierarchy
	right.Hierarchy = hierarchy
	leftBefore := left.TopLeft.Copy()
	rightBefore := right.TopLeft.Copy()
	ctx := &cancelWhenOVGChanges{
		Context: context.Background(),
		shouldCancel: func() bool {
			return !left.TopLeft.Equals(leftBefore) || !right.TopLeft.Equals(rightBefore)
		},
	}

	ovg, err := newOVGForHierarchy(graph, hierarchy, newOVGBuildGuardForTest(ctx, t))
	if ovg != nil {
		t.Fatalf("hierarchy OVG = %v, want nil after cancellation", ovg)
	}
	requireCanceledAt(t, err, "EdgeRouting")
	if !left.TopLeft.Equals(leftBefore) || !right.TopLeft.Equals(rightBefore) {
		t.Fatalf("hierarchy nodes were not restored: left=%v want=%v, right=%v want=%v", left.TopLeft, leftBefore, right.TopLeft, rightBefore)
	}
}

func TestRouteEdgesPropagatesOVGConstructionCancellation(t *testing.T) {
	_, err := routeEdges(canceledContext(), newOVGCancellationGraph(200, 100), nil)
	requireCanceledAt(t, err, "EdgeRouting")
}
