package routing

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/nodeshape"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

type cancelWhenEdgeIsRouted struct {
	context.Context
	edge     *layoutgraph.Edge
	observed bool
}

type panicWhenEdgeIsRouted struct {
	context.Context
	edge *layoutgraph.Edge
}

func (ctx *panicWhenEdgeIsRouted) Err() error {
	if hasCompleteEdgeRoute(ctx.edge) {
		panic("standalone route mutation probe")
	}
	return ctx.Context.Err()
}

func (ctx *cancelWhenEdgeIsRouted) Err() error {
	if hasCompleteEdgeRoute(ctx.edge) {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

type panicOnShiftedShapeTrace struct {
	nodeshape.Shape

	node             *layoutgraph.Node
	edge             *layoutgraph.Edge
	topLeft          *geo.Point
	topLeftValue     geo.Point
	observedMutation atomic.Bool
}

func (s *panicOnShiftedShapeTrace) Perimeter() []geo.Intersectable {
	if hasCompleteEdgeRoute(s.edge) &&
		s.node.TopLeft == s.topLeft &&
		*s.node.TopLeft != s.topLeftValue {
		s.observedMutation.Store(true)
		panic("shifted shape trace observed")
	}
	return s.Shape.Perimeter()
}

func TestRouteEdgesRejectsUnroutedFixedEdges(t *testing.T) {
	graph := layoutgraph.NewGraph()
	a := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	c := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(100, 0)
	c.TopLeft = geo.NewPoint(200, 0)
	selected := graph.Connect(a, b)
	unroutedFixed := graph.Connect(b, c)

	err := RouteEdges(context.Background(), graph, []*layoutgraph.Edge{selected})
	if err == nil || !strings.Contains(err.Error(), "unselected edge") {
		t.Fatalf("error = %v; want unrouted fixed-edge validation", err)
	}
	if len(unroutedFixed.Points) != 0 {
		t.Fatal("validation mutated the unselected edge")
	}
}

func TestRouteEdgesPreservesUnselectedRoutes(t *testing.T) {
	graph := layoutgraph.NewGraph()
	a := graph.AddNode(layoutgraph.NewNode(1, 40, 40))
	b := graph.AddNode(layoutgraph.NewNode(2, 40, 40))
	c := graph.AddNode(layoutgraph.NewNode(3, 40, 40))
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(200, 0)
	c.TopLeft = geo.NewPoint(100, 120)

	selected := graph.Connect(a, b)
	unselected := graph.Connect(a, c)
	unselected.Points = []*geo.Point{
		geo.NewPoint(40, 20),
		geo.NewPoint(70, 20),
		geo.NewPoint(70, 140),
		geo.NewPoint(100, 140),
	}
	originalPoints := append([]*geo.Point(nil), unselected.Points...)
	originalValues := make([]geo.Point, len(unselected.Points))
	for index, point := range unselected.Points {
		originalValues[index] = *point
	}

	if err := RouteEdges(context.Background(), graph, []*layoutgraph.Edge{selected}); err != nil {
		t.Fatal(err)
	}
	if len(unselected.Points) != len(originalPoints) {
		t.Fatalf("unselected route has %d points, want %d", len(unselected.Points), len(originalPoints))
	}
	for index := range originalPoints {
		if unselected.Points[index] != originalPoints[index] || *unselected.Points[index] != originalValues[index] {
			t.Fatalf("unselected route point %d changed from %p %v to %p %v", index, originalPoints[index], originalValues[index], unselected.Points[index], unselected.Points[index])
		}
	}
}

func newInjectedRouteNodeIndexGraph() (*layoutgraph.Graph, *layoutgraph.Edge, *layoutgraph.Node) {
	graph := layoutgraph.NewGraph()
	addNode := func(id layoutgraph.EntityID, x, y, width, height float64, shapeType string) *layoutgraph.Node {
		node := graph.AddNode(layoutgraph.NewNode(id, width, height))
		node.TopLeft = geo.NewPoint(x, y)
		node.SetShape(shapeType)
		return node
	}

	n4 := addNode(4, 782, 21, 62, 61, shape.REAL_SQUARE_TYPE)
	addNode(5, 0, 287, 96, 110, shape.PACKAGE_TYPE)
	n6 := addNode(6, 265, 261, 88, 87, shape.CYLINDER_TYPE)
	n7 := addNode(7, 550, 269, 96, 82, shape.PACKAGE_TYPE)
	n8 := addNode(8, 808, 264, 60, 64, shape.PERSON_TYPE)

	fixedReverse := graph.Connect(n8, n6)
	fixedReverse.Points = []*geo.Point{
		geo.NewPoint(838, 328), geo.NewPoint(838, 414),
		geo.NewPoint(309, 414), geo.NewPoint(309, 348),
	}
	fixedOther := graph.Connect(n7, n4)
	fixedOther.Points = []*geo.Point{
		geo.NewPoint(598, 269), geo.NewPoint(598, 116),
		geo.NewPoint(813, 116), geo.NewPoint(813, 82),
	}
	selected := graph.Connect(n6, n8)
	return graph, selected, n7
}

func requireRouteAvoidsNode(t *testing.T, edge *layoutgraph.Edge, node *layoutgraph.Node) {
	t.Helper()
	for index := 1; index < len(edge.Points); index++ {
		if node.PassesThrough(edge.Points[index-1], edge.Points[index]) {
			t.Fatalf(
				"route segment %v -> %v passes through unrelated node %s",
				edge.Points[index-1], edge.Points[index], node.DebugID(),
			)
		}
	}
}

func TestRouteAdditionalEdgesIndexesInjectedRouteNodes(t *testing.T) {
	graph, selected, unrelated := newInjectedRouteNodeIndexGraph()
	graph.ComputeCellSize()
	ovg, err := routeAdditionalEdgesWithLimits(context.Background(), graph, []*layoutgraph.Edge{selected}, defaultOVGBuildLimits())
	if err != nil {
		t.Fatal(err)
	}
	for index, node := range ovg.Nodes {
		if node.Index != index {
			t.Fatalf("OVG node %d at %v has aliased index %d", index, node.Point, node.Index)
		}
	}
	requireRouteAvoidsNode(t, selected, unrelated)
}

func TestRouteEdgesDoesNotAliasInjectedOVGNodes(t *testing.T) {
	graph, selected, unrelated := newInjectedRouteNodeIndexGraph()
	if err := RouteEdges(context.Background(), graph, []*layoutgraph.Edge{selected}); err != nil {
		t.Fatal(err)
	}
	requireRouteAvoidsNode(t, selected, unrelated)
}

func TestRouteEdgesValidatesGraphAndSelection(t *testing.T) {
	if err := RouteEdges(context.Background(), nil, nil); err == nil {
		t.Fatal("nil graph was accepted")
	}
	if err := RouteEdges(context.Background(), layoutgraph.NewGraph(), nil); err != nil {
		t.Fatalf("empty selection should be a no-op: %v", err)
	}
	graph := layoutgraph.NewGraph()
	a := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	edge := graph.Connect(a, b)
	if err := RouteEdges(context.Background(), graph, nil); err != nil {
		t.Fatalf("empty selection on an unrouted graph should be a no-op: %v", err)
	}
	if len(edge.Points) != 0 {
		t.Fatal("empty selection mutated an edge")
	}
}

func TestRouteEdgesCapsSelectionBeforePreflightAllocation(t *testing.T) {
	edges := make([]*layoutgraph.Edge, layoutgraph.MaxTopologyReferences+1)
	err := RouteEdges(context.Background(), layoutgraph.NewGraph(), edges)
	if err == nil || !strings.Contains(err.Error(), "selected edge references exceed limit") {
		t.Fatalf("selection error = %v; want pre-allocation reference limit", err)
	}
}

func TestRouteEdgesCapsSelectionAtGraphInventory(t *testing.T) {
	graph := layoutgraph.NewGraph()
	from := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	to := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	from.TopLeft = geo.NewPoint(0, 0)
	to.TopLeft = geo.NewPoint(100, 0)
	edge := graph.Connect(from, to)

	err := RouteEdges(context.Background(), graph, []*layoutgraph.Edge{edge, edge})
	if err == nil || !strings.Contains(err.Error(), "exceeds graph edge count") {
		t.Fatalf("selection error = %v; want graph-inventory limit", err)
	}
}

func TestRouteEdgesCancellationRestoresTentativeRoute(t *testing.T) {
	graph := layoutgraph.NewGraph()
	a := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(100, 0)
	edge := graph.Connect(a, b)
	edge.Points = routeWithSpareCapacity()
	snapshot := captureExactRouteTest(edge)
	cellSize := graph.CellSize
	ctx := &cancelWhenEdgeIsRouted{Context: context.Background(), edge: edge}

	err := RouteEdges(ctx, graph, []*layoutgraph.Edge{edge})
	requireCanceledAt(t, err, "EdgeRouting")
	if !ctx.observed {
		t.Fatal("test context did not observe the completed route")
	}
	snapshot.assertRestored(t)
	if graph.CellSize != cellSize {
		t.Fatalf("cell size = %v; want restored value %v", graph.CellSize, cellSize)
	}
	requireZeroRoutingCosts(t, graph)
}

func TestRouteEdgesPanicRestoresTentativeRoute(t *testing.T) {
	graph := layoutgraph.NewGraph()
	from := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	to := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	from.TopLeft = geo.NewPoint(0, 0)
	to.TopLeft = geo.NewPoint(100, 0)
	edge := graph.Connect(from, to)
	edge.Points = routeWithSpareCapacity()
	snapshot := captureExactRouteTest(edge)
	cellSize := graph.CellSize
	ctx := &panicWhenEdgeIsRouted{Context: context.Background(), edge: edge}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = RouteEdges(ctx, graph, []*layoutgraph.Edge{edge})
	}()
	if recovered != "standalone route mutation probe" {
		t.Fatalf("recovered = %v; want route mutation probe", recovered)
	}
	snapshot.assertRestored(t)
	if graph.CellSize != cellSize {
		t.Fatalf("cell size = %v; want restored value %v", graph.CellSize, cellSize)
	}
	requireZeroRoutingCosts(t, graph)
}

func TestRouteEdgesRestoresModifierShiftBeforePropagatingShapeTracePanic(t *testing.T) {
	graph, edge := newRoutingTestGraph(200, 0)
	edge.From.SetShape(shape.CIRCLE_TYPE)
	edge.From.IsMultiple = true
	topLeft := edge.From.TopLeft
	topLeftValue := *topLeft
	panicShape := &panicOnShiftedShapeTrace{
		Shape:        edge.From.Shape,
		node:         edge.From,
		edge:         edge,
		topLeft:      topLeft,
		topLeftValue: topLeftValue,
	}
	edge.From.Shape = panicShape
	edge.Points = routeWithSpareCapacity(geo.NewPoint(17, 19), geo.NewPoint(18, 19))
	snapshot := captureExactRouteTest(edge)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = RouteEdges(context.Background(), graph, []*layoutgraph.Edge{edge})
	}()
	if recovered != "shifted shape trace observed" {
		t.Fatalf("panic = %v, want injected shape-trace panic", recovered)
	}
	if !panicShape.observedMutation.Load() {
		t.Fatal("custom shape did not observe a completed route with a shifted endpoint")
	}
	if edge.From.TopLeft != topLeft {
		t.Fatalf("source TopLeft pointer = %p, want %p", edge.From.TopLeft, topLeft)
	}
	if *edge.From.TopLeft != topLeftValue {
		t.Fatalf("source TopLeft = %v, want %v", edge.From.TopLeft, topLeftValue)
	}
	snapshot.assertRestored(t)
}

func TestRouteEdgesWorkLimitRestoresTentativeRoute(t *testing.T) {
	for limit := uint64(1); limit <= 10_000; limit++ {
		graph := layoutgraph.NewGraph()
		from := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
		to := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
		from.TopLeft = geo.NewPoint(0, 0)
		to.TopLeft = geo.NewPoint(100, 0)
		edge := graph.Connect(from, to)
		edge.Points = routeWithSpareCapacity()
		snapshot := captureExactRouteTest(edge)
		cellSize := graph.CellSize
		ctx := &observeRouteMutation{Context: context.Background(), snapshot: snapshot}

		err := routeEdgesWithWorkLimit(ctx, graph, []*layoutgraph.Edge{edge}, limit)
		if err == nil {
			break
		}
		if errors.Is(err, errRouteStageWorkLimit) && ctx.observed {
			snapshot.assertRestored(t)
			costs := graph.RoutingCosts()
			if graph.CellSize != cellSize || costs.Crossing != 0 || costs.Turn != 0 || costs.NonCenterPort != 0 {
				t.Fatalf("standalone graph state was not restored: cell=%v crossing=%v turn=%v port=%v", graph.CellSize, costs.Crossing, costs.Turn, costs.NonCenterPort)
			}
			return
		}
	}
	t.Fatal("no standalone work limit was observed after a tentative route mutation")
}

func TestRouteEdgesRejectsMalformedGraphWithoutPanicking(t *testing.T) {
	placed := func(id layoutgraph.EntityID, x float64) *layoutgraph.Node {
		node := layoutgraph.NewNode(id, 10, 10)
		node.TopLeft = geo.NewPoint(x, 0)
		return node
	}

	tests := []struct {
		name string
		make func() (*layoutgraph.Graph, []*layoutgraph.Edge)
		want string
	}{
		{
			name: "empty graph",
			make: func() (*layoutgraph.Graph, []*layoutgraph.Edge) {
				return layoutgraph.NewGraph(), []*layoutgraph.Edge{layoutgraph.NewEdge(placed(1, 0), placed(2, 100))}
			},
			want: "empty graph",
		},
		{
			name: "nil graph edge",
			make: func() (*layoutgraph.Graph, []*layoutgraph.Edge) {
				g := layoutgraph.NewGraph()
				a := g.AddNode(placed(1, 0))
				b := g.AddNode(placed(2, 100))
				edge := g.Connect(a, b)
				g.Edges = append(g.Edges, nil)
				return g, []*layoutgraph.Edge{edge}
			},
			want: "graph edge at index 1 is nil",
		},
		{
			name: "source endpoint not in graph",
			make: func() (*layoutgraph.Graph, []*layoutgraph.Edge) {
				g := layoutgraph.NewGraph()
				outside := placed(1, 0)
				inside := g.AddNode(placed(2, 100))
				edge := layoutgraph.NewEdge(outside, inside)
				g.Edges = append(g.Edges, edge)
				return g, []*layoutgraph.Edge{edge}
			},
			want: "source node does not belong",
		},
		{
			name: "target endpoint not in graph",
			make: func() (*layoutgraph.Graph, []*layoutgraph.Edge) {
				g := layoutgraph.NewGraph()
				inside := g.AddNode(placed(1, 0))
				outside := placed(2, 100)
				edge := layoutgraph.NewEdge(inside, outside)
				g.Edges = append(g.Edges, edge)
				return g, []*layoutgraph.Edge{edge}
			},
			want: "target node does not belong",
		},
		{
			name: "missing endpoint",
			make: func() (*layoutgraph.Graph, []*layoutgraph.Edge) {
				g := layoutgraph.NewGraph()
				inside := g.AddNode(placed(1, 0))
				edge := layoutgraph.NewEdge(inside, nil)
				g.Edges = append(g.Edges, edge)
				return g, []*layoutgraph.Edge{edge}
			},
			want: "missing endpoints",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, edges := test.make()
			err := RouteEdges(context.Background(), graph, edges)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}
