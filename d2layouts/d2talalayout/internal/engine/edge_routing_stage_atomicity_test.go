package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

type cancelWhenRoutingCompletes struct {
	context.Context
	pipeline *pipeline
	observed bool
}

type observeNearOwnerMutationContext struct {
	context.Context
	node     *layoutgraph.Node
	original *layoutgraph.Graph
	observed bool
}

func (ctx *observeNearOwnerMutationContext) Err() error {
	if ctx.node.Graph != ctx.original {
		ctx.observed = true
	}
	return ctx.Context.Err()
}

func (ctx *cancelWhenRoutingCompletes) Err() error {
	if ctx.pipeline.edgeRoutingComplete {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func edgeRoutingStageMutationGraph() (*layoutgraph.Graph, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 10, 10)
	to := layoutgraph.NewNode(2, 10, 10)
	from.TopLeft = geo.NewPoint(0, 0)
	to.TopLeft = geo.NewPoint(100, 0)
	g.AddNewNodeToContainer(nil, from)
	g.AddNewNodeToContainer(nil, to)
	edge := g.Connect(from, to)
	edge.Points = routeWithSpareCapacity()
	g.CellSize = 17
	return g, edge
}

func assertEdgeRoutingStageRollback(
	t *testing.T,
	pipeline *pipeline,
	edgeState exactRouteTestSnapshot,
	nodeGraphs map[*layoutgraph.Node]*layoutgraph.Graph,
	cellSize float64,
	snapshots exactTestSlice[*routingSnapshot],
	routingComplete bool,
) {
	t.Helper()
	edgeState.assertRestored(t)
	if pipeline.graph.CellSize != cellSize {
		t.Fatalf("CellSize = %v, want %v", pipeline.graph.CellSize, cellSize)
	}
	for node, graph := range nodeGraphs {
		if node.Graph != graph {
			t.Fatalf("node %d graph = %p, want %p", node.ID, node.Graph, graph)
		}
	}
	snapshots.assertRestored(t, pipeline.snapshots, "pipeline snapshots")
	if pipeline.edgeRoutingComplete != routingComplete {
		t.Fatalf("edgeRoutingComplete = %v, want %v", pipeline.edgeRoutingComplete, routingComplete)
	}
}

func TestEdgeRoutingStageCancellationAfterRouteMutationRestoresExactState(t *testing.T) {
	g, edge := edgeRoutingStageMutationGraph()
	pipeline := newPipeline(g, 1, false)
	pipeline.snapshots = make([]*routingSnapshot, 1, 3)
	pipeline.snapshots[0] = &routingSnapshot{}
	pipeline.edgeRoutingComplete = false

	edgeState := captureExactRouteTest(edge)
	nodeGraphs := map[*layoutgraph.Node]*layoutgraph.Graph{}
	for _, node := range g.Nodes {
		nodeGraphs[node] = node.Graph
	}
	cellSize := g.CellSize
	snapshots := captureExactTestSlice(pipeline.snapshots)
	ctx := &cancelWhenRouteMutates{Context: context.Background(), snapshot: edgeState}

	err := pipeline.edgeRoutingStage(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("edgeRoutingStage error = %v, want context cancellation", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe a route mutation")
	}
	assertEdgeRoutingStageRollback(t, pipeline, edgeState, nodeGraphs, cellSize, snapshots, false)
}

func TestEdgeRoutingStagePanicAfterRouteMutationRestoresExactState(t *testing.T) {
	g, edge := edgeRoutingStageMutationGraph()
	pipeline := newPipeline(g, 1, false)
	pipeline.snapshots = make([]*routingSnapshot, 1, 3)
	pipeline.snapshots[0] = &routingSnapshot{}

	edgeState := captureExactRouteTest(edge)
	nodeGraphs := map[*layoutgraph.Node]*layoutgraph.Graph{}
	for _, node := range g.Nodes {
		nodeGraphs[node] = node.Graph
	}
	cellSize := g.CellSize
	snapshots := captureExactTestSlice(pipeline.snapshots)
	ctx := &panicWhenRouteMutates{Context: context.Background(), snapshot: edgeState}

	defer func() {
		if recovered := recover(); recovered != "post-route mutation probe" {
			t.Fatalf("panic = %v, want post-route mutation probe", recovered)
		}
		assertEdgeRoutingStageRollback(t, pipeline, edgeState, nodeGraphs, cellSize, snapshots, false)
	}()
	_ = pipeline.edgeRoutingStage(ctx)
}

func TestEdgeRoutingStageLateCancellationRestoresCompletionFlag(t *testing.T) {
	g, edge := edgeRoutingStageMutationGraph()
	pipeline := newPipeline(g, 1, false)
	pipeline.edgeRoutingComplete = false
	edgeState := captureExactRouteTest(edge)
	ctx := &cancelWhenRoutingCompletes{Context: context.Background(), pipeline: pipeline}

	err := pipeline.edgeRoutingStage(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("edgeRoutingStage error = %v, want context cancellation", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe routing completion mutation")
	}
	if pipeline.edgeRoutingComplete {
		t.Fatal("edgeRoutingComplete was not rolled back")
	}
	edgeState.assertRestored(t)
}

func TestEdgeRoutingStageBoundsAggregateSubgraphObstructionWork(t *testing.T) {
	g := layoutgraph.NewGraph()
	for component := 0; component < 5; component++ {
		left := layoutgraph.NewNode(layoutgraph.EntityID(component*2+1), 10, 10)
		right := layoutgraph.NewNode(layoutgraph.EntityID(component*2+2), 10, 10)
		// Keep component bounding boxes overlapping so each obstruction candidate
		// reaches the exact-overlap comparison kernel.
		left.TopLeft = geo.NewPoint(float64(component), float64(component))
		right.TopLeft = geo.NewPoint(100+float64(component), float64(component))
		g.AddNewNodeToContainer(nil, left)
		g.AddNewNodeToContainer(nil, right)
		g.Connect(left, right)
	}
	pipeline := newPipeline(g, 1, false)
	err := pipeline.edgeRoutingStageWithWorkLimit(context.Background(), 100)
	if err == nil || !strings.Contains(err.Error(), "route-stage work limit exceeded") {
		t.Fatalf("edgeRoutingStage error = %v, want route-stage work limit", err)
	}
}

func TestEdgeRoutingStageRejectsGraphEdgeEndpointOutsideNodesBeforeFastPath(t *testing.T) {
	for _, endpoint := range []string{"source", "target"} {
		for _, routed := range []bool{true, false} {
			name := endpoint + "-unrouted"
			if routed {
				name = endpoint + "-routed-complete"
			}
			t.Run(name, func(t *testing.T) {
				g := layoutgraph.NewGraph()
				inside := layoutgraph.NewNode(1, 10, 10)
				inside.TopLeft = geo.NewPoint(0, 0)
				g.AddNewNodeToContainer(nil, inside)
				outside := layoutgraph.NewNode(2, 10, 10)
				outside.TopLeft = geo.NewPoint(100, 0)
				edge := layoutgraph.NewEdge(inside, outside)
				if endpoint == "source" {
					edge = layoutgraph.NewEdge(outside, inside)
				}
				inside.Edges = append(inside.Edges, edge)
				outside.Edges = append(outside.Edges, edge)
				g.Edges = append(g.Edges, edge)
				if routed {
					edge.Points = routeWithSpareCapacity(geo.NewPoint(10, 5), geo.NewPoint(100, 5))
				}

				pipeline := newPipeline(g, 1, false)
				pipeline.edgeRoutingComplete = false
				edgeState := captureExactRouteTest(edge)
				outsideOwner := outside.Graph
				err := pipeline.edgeRoutingStage(context.Background())
				want := endpoint + " node does not belong to the graph"
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("edgeRoutingStage error = %v, want %q", err, want)
				}
				if pipeline.edgeRoutingComplete {
					t.Fatal("membership failure marked routing complete")
				}
				edgeState.assertRestored(t)
				if outside.Graph != outsideOwner {
					t.Fatal("membership rejection changed the external endpoint owner")
				}
			})
		}
	}
}

func TestEdgeRoutingStageRejectsOnePointLabeledRouteBeforeBoundingBox(t *testing.T) {
	for _, labelKind := range []string{"edge", "source-arrowhead", "target-arrowhead"} {
		t.Run(labelKind, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			from := layoutgraph.NewNode(1, 10, 10)
			to := layoutgraph.NewNode(2, 10, 10)
			from.TopLeft = geo.NewPoint(0, 0)
			to.TopLeft = geo.NewPoint(100, 0)
			g.AddNewNodeToContainer(nil, from)
			g.AddNewNodeToContainer(nil, to)
			edge := g.Connect(from, to)
			edge.Points = []*geo.Point{geo.NewPoint(10, 5)}
			switch labelKind {
			case "edge":
				edge.Label = &layoutgraph.Label{Position: label.UnlockedMiddle, Width: 30, Height: 12}
			case "source-arrowhead":
				edge.SourceArrowheadLabel = &layoutgraph.Label{Width: 20, Height: 10}
			case "target-arrowhead":
				edge.TargetArrowheadLabel = &layoutgraph.Label{Width: 20, Height: 10}
			}

			pipeline := newPipeline(g, 1, false)
			edgeState := captureExactRouteTest(edge)
			err := pipeline.edgeRoutingStage(context.Background())
			if err == nil || !strings.Contains(err.Error(), "incomplete route") {
				t.Fatalf("edgeRoutingStage error = %v, want incomplete-route rejection", err)
			}
			edgeState.assertRestored(t)
			if pipeline.edgeRoutingComplete {
				t.Fatal("malformed route marked routing complete")
			}
		})
	}
}

func TestEdgeRoutingStageRejectsNonEdgeLabelPositionsBeforeBoundingBox(t *testing.T) {
	for _, labelKind := range []string{"edge", "source-arrowhead", "target-arrowhead"} {
		t.Run(labelKind, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			from := layoutgraph.NewNode(1, 10, 10)
			to := layoutgraph.NewNode(2, 10, 10)
			from.TopLeft = geo.NewPoint(0, 0)
			to.TopLeft = geo.NewPoint(100, 0)
			g.AddNewNodeToContainer(nil, from)
			g.AddNewNodeToContainer(nil, to)
			edge := g.Connect(from, to)
			edge.Points = []*geo.Point{geo.NewPoint(10, 5), geo.NewPoint(100, 5)}
			invalid := &layoutgraph.Label{Position: label.OutsideLeftTop, Width: 30, Height: 12}
			switch labelKind {
			case "edge":
				edge.Label = invalid
			case "source-arrowhead":
				edge.SourceArrowheadLabel = invalid
			case "target-arrowhead":
				edge.TargetArrowheadLabel = invalid
			}

			pipeline := newPipeline(g, 1, false)
			edgeState := captureExactRouteTest(edge)
			err := pipeline.edgeRoutingStage(context.Background())
			if err == nil || !strings.Contains(err.Error(), "invalid edge position") {
				t.Fatalf("edgeRoutingStage error = %v, want invalid edge-position rejection", err)
			}
			edgeState.assertRestored(t)
			if pipeline.edgeRoutingComplete {
				t.Fatal("malformed label position marked routing complete")
			}
		})
	}
}

func TestEdgeRoutingStageAggregateLimitRestoresNearOnlyExternalOwner(t *testing.T) {
	g := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 10, 10)
	node.TopLeft = geo.NewPoint(0, 0)
	g.AddNewNodeToContainer(nil, node)
	edge := layoutgraph.NewEdge(node, node)
	node.Edges = append(node.Edges, edge)
	g.Edges = append(g.Edges, edge)

	owner := layoutgraph.NewGraph()
	external := layoutgraph.NewNode(3, 10, 10)
	external.TopLeft = geo.NewPoint(200, 0)
	owner.AddNewNodeToContainer(nil, external)
	node.Nears[external] = struct{}{}
	if external.Graph != owner {
		t.Fatal("test setup did not establish a distinct external owner")
	}

	pipeline := newPipeline(g, 1, false)
	ctx := &observeNearOwnerMutationContext{Context: context.Background(), node: external, original: owner}
	// This is the exact limit that exposed the original leak: SplitSubgraphs
	// changed the Near-only owner under a fresh budget, then the stage aggregate
	// failed immediately afterward without knowing that node needed restoration.
	err := pipeline.edgeRoutingStageWithWorkLimit(ctx, 14)
	if err == nil || !strings.Contains(err.Error(), "route-stage work limit exceeded") {
		t.Fatalf("edgeRoutingStage error = %v, want aggregate work limit", err)
	}
	if external.Graph != owner {
		t.Fatalf("near-only node owner = %p, want exact original %p", external.Graph, owner)
	}

	// With SplitSubgraphs now consuming the aggregate, limit 14 can reject
	// before the temporary assignment. A slightly later limit proves that the
	// ownership journal still restores after that assignment has occurred.
	ctx.observed = false
	err = pipeline.edgeRoutingStageWithWorkLimit(ctx, 30)
	if err == nil || !strings.Contains(err.Error(), "route-stage work limit exceeded") {
		t.Fatalf("later edgeRoutingStage error = %v, want aggregate work limit", err)
	}
	if !ctx.observed {
		t.Fatal("later aggregate limit did not trip after SplitSubgraphs changed the Near-only owner")
	}
	if external.Graph != owner {
		t.Fatalf("later near-only node owner = %p, want exact original %p", external.Graph, owner)
	}
	if err := pipeline.edgeRoutingStage(context.Background()); err != nil {
		t.Fatalf("successful edgeRoutingStage: %v", err)
	}
	if external.Graph != owner {
		t.Fatalf("successful routing left near-only owner = %p, want %p", external.Graph, owner)
	}
}
