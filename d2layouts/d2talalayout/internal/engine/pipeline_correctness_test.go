package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func TestEdgeRoutingStageRejectsPartiallyRoutedGraph(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(1, 10, 10)
	b := layoutgraph.NewNode(2, 10, 10)
	c := layoutgraph.NewNode(3, 10, 10)
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(100, 0)
	c.TopLeft = geo.NewPoint(200, 0)
	for _, node := range []*layoutgraph.Node{a, b, c} {
		g.AddNewNodeToContainer(nil, node)
	}

	routed := g.Connect(a, b)
	routed.Points = []*geo.Point{geo.NewPoint(10, 5), geo.NewPoint(100, 5)}
	g.Connect(b, c)

	p := newPipeline(g, 1, false)
	p.edgeRoutingComplete = true
	err := p.edgeRoutingStage(withTestLogger(context.Background(), t))
	if err == nil || !strings.Contains(err.Error(), "partially routed") {
		t.Fatalf("expected a partially routed graph error, got %v", err)
	}
}

func setPlacementCostStateForPipelineTest(graph *layoutgraph.Graph, state uint64, cost, crossing, turn, nonCenter float64) {
	graph.RestoreRoutingCosts(layoutgraph.RoutingCostState{
		Crossing:      crossing,
		Turn:          turn,
		NonCenterPort: nonCenter,
	})
	graph.StoreEdgeLengthCost(state, cost)
}

func TestEdgeRoutingStageValidatesEveryExistingRoute(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(100, 0)
	edge := g.Connect(a, b)
	edge.Points = []*geo.Point{geo.NewPoint(10, 5)}

	p := newPipeline(g, 1, false)
	p.edgeRoutingComplete = true
	err := p.edgeRoutingStage(withTestLogger(context.Background(), t))
	if err == nil || !strings.Contains(err.Error(), "incomplete route") {
		t.Fatalf("expected an incomplete route error, got %v", err)
	}
}

func TestEdgeRoutingStageSkipsOnlyWhenEveryEdgeIsRouted(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	c := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(100, 0)
	c.TopLeft = geo.NewPoint(200, 0)
	first := g.Connect(a, b)
	first.Points = []*geo.Point{geo.NewPoint(10, 5), geo.NewPoint(20, 5)}
	first.Label = &layoutgraph.Label{Text: "first", Position: label.UnlockedTop, Width: 20, Height: 10}
	first.LabelPercentage = 0.1
	first.IsCurve = true
	second := g.Connect(b, c)
	second.Points = []*geo.Point{geo.NewPoint(30, 5), geo.NewPoint(40, 5)}
	second.Label = &layoutgraph.Label{Text: "second", Position: label.OutsideBottomCenter, Width: 30, Height: 10}
	second.LabelPercentage = 0.9
	second.IsCurve = true
	setPlacementCostStateForPipelineTest(g, 19, 23, 11, 13, 17)
	type routeState struct {
		points          []*geo.Point
		label           *layoutgraph.Label
		labelPosition   label.Position
		labelPercentage float64
		isCurve         bool
	}
	want := map[*layoutgraph.Edge]routeState{
		first: {
			points:          append([]*geo.Point(nil), first.Points...),
			label:           first.Label,
			labelPosition:   first.Label.Position,
			labelPercentage: first.LabelPercentage,
			isCurve:         first.IsCurve,
		},
		second: {
			points:          append([]*geo.Point(nil), second.Points...),
			label:           second.Label,
			labelPosition:   second.Label.Position,
			labelPercentage: second.LabelPercentage,
			isCurve:         second.IsCurve,
		},
	}

	p := newPipeline(g, 1, false)
	p.edgeRoutingComplete = true
	if err := p.edgeRoutingStage(withTestLogger(context.Background(), t)); err != nil {
		t.Fatal(err)
	}
	for edge, state := range want {
		if len(edge.Points) != len(state.points) {
			t.Fatalf("complete route length = %d, want %d", len(edge.Points), len(state.points))
		}
		for index := range state.points {
			if edge.Points[index] != state.points[index] {
				t.Fatalf("complete route point %d was unexpectedly replaced", index)
			}
		}
		if edge.Label != state.label || edge.Label.Position != state.labelPosition ||
			edge.LabelPercentage != state.labelPercentage || edge.IsCurve != state.isCurve {
			t.Fatalf("complete route metadata changed: edge=%+v want=%+v", edge, state)
		}
	}
	costs := g.RoutingCosts()
	if costs != (layoutgraph.RoutingCostState{Crossing: 11, Turn: 13, NonCenterPort: 17}) {
		t.Fatalf("complete route cost state changed: %+v", costs)
	}
	if cost, ok := g.LookupEdgeLengthCost(19); !ok || cost != 23 || g.EdgeLengthCacheEntries() != 1 {
		t.Fatalf("complete route edge-length cache changed: cost=%v ok=%v entries=%d", cost, ok, g.EdgeLengthCacheEntries())
	}
}

func TestRunAllStagesResetsRouteDerivedStateBeforeFirstStage(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(100, 0)

	unlockedLabel := &layoutgraph.Label{Text: "automatic", Position: label.UnlockedTop, Width: 30, Height: 12}
	unlocked := g.Connect(a, b)
	unlocked.Points = []*geo.Point{geo.NewPoint(10, 5), geo.NewPoint(100, 5)}
	unlocked.Label = unlockedLabel
	unlocked.LabelPercentage = 0.1
	unlocked.IsCurve = true
	unlocked.MinWidth = 23
	unlocked.MinHeight = 29
	unlocked.IsInvisible = true

	lockedLabel := &layoutgraph.Label{Text: "locked", Position: label.OutsideBottomCenter, Width: 40, Height: 14}
	locked := g.Connect(a, b)
	locked.Points = []*geo.Point{geo.NewPoint(10, 6), geo.NewPoint(100, 6)}
	locked.Label = lockedLabel
	locked.LabelPercentage = 0.9
	locked.IsCurve = true

	setPlacementCostStateForPipelineTest(g, 109, 113, 101, 103, 107)

	firstStageRan := false
	p := newPipeline(g, 1, false)
	p.stages = []pipelineStage{{
		name: "RouteStateProbe",
		run: func(_ *pipeline, _ context.Context) error {
			firstStageRan = true
			for _, edge := range []*layoutgraph.Edge{unlocked, locked} {
				if edge.Points != nil || edge.LabelPercentage != 0 || edge.IsCurve {
					return fmt.Errorf("route-derived edge state survived: %+v", edge)
				}
			}
			if unlocked.Label != unlockedLabel || unlocked.Label.Position != label.Unset {
				return fmt.Errorf("unlocked label was not reset in place: %+v", unlocked.Label)
			}
			if locked.Label != lockedLabel || locked.Label.Position != label.OutsideBottomCenter {
				return fmt.Errorf("locked label position changed: %+v", locked.Label)
			}
			if unlocked.Label.Text != "automatic" || unlocked.Label.Width != 30 || unlocked.Label.Height != 12 ||
				unlocked.MinWidth != 23 || unlocked.MinHeight != 29 || !unlocked.IsInvisible {
				return fmt.Errorf("semantic edge state changed: %+v", unlocked)
			}
			if costs := p.graph.RoutingCosts(); costs != (layoutgraph.RoutingCostState{}) {
				return fmt.Errorf("route cost state survived: %+v", costs)
			}
			if p.graph.EdgeLengthCacheEntries() != 0 {
				return fmt.Errorf("edge-length cache retained entries")
			}
			p.graph.StoreEdgeLengthCost(127, 131)
			if cost, ok := p.graph.LookupEdgeLengthCost(127); !ok || cost != 131 {
				return fmt.Errorf("reset edge-length cache is unusable: cost=%v ok=%v", cost, ok)
			}
			return nil
		},
	}}
	if err := p.runAllStages(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !firstStageRan {
		t.Fatal("route state probe stage did not run")
	}
}

func TestEdgeRoutingStageValidatesCompleteExistingRouteGeometry(t *testing.T) {
	for _, test := range []struct {
		name   string
		points func() []*geo.Point
		match  string
	}{
		{
			name: "non-finite point",
			points: func() []*geo.Point {
				return []*geo.Point{geo.NewPoint(math.NaN(), 5), geo.NewPoint(100, 5)}
			},
			match: "non-finite route point",
		},
		{
			name: "route capacity",
			points: func() []*geo.Point {
				points := make([]*geo.Point, 2, layoutgraph.MaxRoutePoints+1)
				points[0] = geo.NewPoint(10, 5)
				points[1] = geo.NewPoint(100, 5)
				return points
			},
			match: "route point",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			from := layoutgraph.NewNode(1, 10, 10)
			to := layoutgraph.NewNode(2, 10, 10)
			from.TopLeft = geo.NewPoint(0, 0)
			to.TopLeft = geo.NewPoint(100, 0)
			g.AddNewNodeToContainer(nil, from)
			g.AddNewNodeToContainer(nil, to)
			edge := g.Connect(from, to)
			edge.Points = test.points()
			pipeline := newPipeline(g, 1, false)

			err := pipeline.edgeRoutingStage(withTestLogger(context.Background(), t))
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("edgeRoutingStage error = %v, want %q", err, test.match)
			}
			if pipeline.edgeRoutingComplete {
				t.Fatal("invalid existing route marked routing complete")
			}
		})
	}
}

func TestEdgeRoutingStageCopiesRequestedSnapshotWithoutWireEncoding(t *testing.T) {
	newGraph := func() *layoutgraph.Graph {
		g := layoutgraph.NewGraph()
		a := layoutgraph.NewNode(1, 10, 10)
		b := layoutgraph.NewNode(2, 10, 10)
		a.TopLeft = geo.NewPoint(0, 0)
		b.TopLeft = geo.NewPoint(100, 0)
		g.AddNewNodeToContainer(nil, a)
		g.AddNewNodeToContainer(nil, b)
		g.Connect(a, b).LabelPercentage = math.NaN()
		return g
	}

	ctx := withTestLogger(context.Background(), t)
	withoutSnapshots := newPipeline(newGraph(), 1, false)
	if err := withoutSnapshots.edgeRoutingStage(ctx); err != nil {
		t.Fatalf("routing without snapshots: %v", err)
	}
	if len(withoutSnapshots.snapshots) != 0 {
		t.Fatal("routing retained snapshots without snapshot instrumentation")
	}

	withSnapshots := newPipeline(newGraph(), 1, true)
	if err := withSnapshots.edgeRoutingStage(ctx); err != nil {
		t.Fatalf("routing with snapshots: %v", err)
	}
	if len(withSnapshots.snapshots) == 0 {
		t.Fatal("routing did not retain a requested snapshot")
	}
	cloned := withSnapshots.snapshots[len(withSnapshots.snapshots)-1].graph
	if cloned == withSnapshots.graph || len(cloned.Edges) != 1 || !math.IsNaN(cloned.Edges[0].LabelPercentage) {
		t.Fatalf("snapshot graph was not an independent exact in-memory clone: %#v", cloned)
	}
}

func TestEdgeRoutingStageWithoutSnapshotsClearsStaleSnapshotsAfterFirstSubgraph(t *testing.T) {
	g, _ := edgeRoutingStageMutationGraph()
	pipeline := newPipeline(g, 1, false)
	pipeline.snapshots = []*routingSnapshot{{}}

	if err := pipeline.edgeRoutingStage(withTestLogger(t.Context(), t)); err != nil {
		t.Fatal(err)
	}
	if pipeline.snapshots != nil {
		t.Fatalf("snapshots after routed-subgraph callback = %#v, want nil", pipeline.snapshots)
	}
}

func TestEdgeRoutingStageWithoutSubgraphCallbackRetainsSnapshots(t *testing.T) {
	g, edge := edgeRoutingStageMutationGraph()
	edge.Points = routeWithSpareCapacity(geo.NewPoint(10, 5), geo.NewPoint(100, 5))
	pipeline := newPipeline(g, 1, false)
	pipeline.edgeRoutingComplete = true
	pipeline.snapshots = make([]*routingSnapshot, 1, 3)
	pipeline.snapshots[0] = &routingSnapshot{}
	want := captureExactTestSlice(pipeline.snapshots)

	if err := pipeline.edgeRoutingStage(withTestLogger(t.Context(), t)); err != nil {
		t.Fatal(err)
	}
	want.assertRestored(t, pipeline.snapshots, "pipeline snapshots")
}

type cancelWhenRouteSnapshotClears struct {
	context.Context
	pipeline *pipeline
	observed bool
}

func (ctx *cancelWhenRouteSnapshotClears) Err() error {
	if ctx.pipeline.snapshots == nil {
		ctx.observed = true
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestEdgeRoutingStageRestoresSnapshotsAfterPostCallbackCancellation(t *testing.T) {
	g, edge := edgeRoutingStageMutationGraph()
	pipeline := newPipeline(g, 1, false)
	pipeline.snapshots = make([]*routingSnapshot, 1, 3)
	pipeline.snapshots[0] = &routingSnapshot{}
	wantSnapshots := captureExactTestSlice(pipeline.snapshots)
	wantEdge := captureExactRouteTest(edge)
	ctx := &cancelWhenRouteSnapshotClears{Context: context.Background(), pipeline: pipeline}

	err := pipeline.edgeRoutingStage(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("edgeRoutingStage error = %v, want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe the routed-subgraph callback")
	}
	wantSnapshots.assertRestored(t, pipeline.snapshots, "pipeline snapshots")
	wantEdge.assertRestored(t)
}

func TestEdgeRoutingStageReportsCancellationBeforeSnapshot(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(1, 10, 10)
	b := layoutgraph.NewNode(2, 10, 10)
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(100, 0)
	g.AddNewNodeToContainer(nil, a)
	g.AddNewNodeToContainer(nil, b)
	g.Connect(a, b).LabelPercentage = math.NaN()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := newPipeline(g, 1, true).edgeRoutingStage(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("routing error = %v, want context.Canceled", err)
	}
	if len(g.Edges) != 1 || !math.IsNaN(g.Edges[0].LabelPercentage) {
		t.Fatal("canceled routing changed graph state")
	}
}

func TestAllEdgesHaveCompleteRoutes(t *testing.T) {
	a := layoutgraph.NewNode(1, 10, 10)
	b := layoutgraph.NewNode(2, 10, 10)
	complete := layoutgraph.NewEdge(a, b)
	complete.Points = []*geo.Point{geo.NewPoint(10, 5), geo.NewPoint(20, 5)}
	incomplete := layoutgraph.NewEdge(a, b)

	if allEdgesHaveCompleteRoutes(nil) {
		t.Fatal("an empty edge set must not be classified as routed")
	}
	if !allEdgesHaveCompleteRoutes([]*layoutgraph.Edge{complete}) {
		t.Fatal("a complete route was not recognized")
	}
	if allEdgesHaveCompleteRoutes([]*layoutgraph.Edge{complete, incomplete}) {
		t.Fatal("partially routed edges must not enable route-aware bin packing")
	}

	incomplete.Points = []*geo.Point{nil, geo.NewPoint(20, 5)}
	if allEdgesHaveCompleteRoutes([]*layoutgraph.Edge{complete, incomplete}) {
		t.Fatal("a route containing a nil point must not be considered complete")
	}
}

func TestRunAllStagesStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(withTestLogger(context.Background(), t))
	secondStageCalled := false
	p := &pipeline{
		graph: layoutgraph.NewGraph(),
		stages: []pipelineStage{
			{
				name: "cancel",
				run: func(_ *pipeline, _ context.Context) error {
					cancel()
					return nil
				},
			},
			{
				name: "must-not-run",
				run: func(_ *pipeline, _ context.Context) error {
					secondStageCalled = true
					return nil
				},
			},
		},
	}

	if err := p.runAllStages(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("pipeline error = %v, want context.Canceled", err)
	}
	if secondStageCalled {
		t.Fatal("pipeline ran another stage after cancellation")
	}
}

func TestFreshLayoutIgnoresStaleEdgeRoutesBeforeBinPack(t *testing.T) {
	ctx := withTestLogger(t.Context(), t)
	withRoutes, err := readGraph(ctx, filepath.Join(layoutTestDir, "larger_tree", "graph.exp.json"))
	if err != nil {
		t.Fatal(err)
	}
	withoutRoutes, err := readGraph(ctx, filepath.Join(layoutTestDir, "larger_tree", "graph.exp.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range withoutRoutes.Edges {
		edge.Points = nil
	}

	if _, err := layoutWithSnapshots(ctx, withRoutes, talaSeed, false); err != nil {
		t.Fatal(err)
	}
	if _, err := layoutWithSnapshots(ctx, withoutRoutes, talaSeed, false); err != nil {
		t.Fatal(err)
	}
	requireGraphsSerializeEqual(ctx, t, withRoutes, withoutRoutes)
}

func TestLayoutAllowsEmptyNestedContainer(t *testing.T) {
	for _, mode := range []struct {
		name    string
		enabled bool
	}{
		{name: "production"},
		{name: "dev-and-test", enabled: true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			if mode.enabled {
				t.Setenv("DEV_MODE", "on")
				t.Setenv("TEST_MODE", "on")
			} else {
				t.Setenv("DEV_MODE", "")
				t.Setenv("TEST_MODE", "")
			}

			g := layoutgraph.NewGraph()
			empty := layoutgraph.NewNode(1, 100, 100)
			empty.SetContainer(true)
			sibling := layoutgraph.NewNode(2, 100, 100)
			g.AddNewNodeToContainer(nil, empty)
			g.AddNewNodeToContainer(nil, sibling)
			g.Containers[empty] = nil

			if _, err := layoutWithSnapshots(withTestLogger(context.Background(), t), g, talaSeed, false); err != nil {
				t.Fatal(err)
			}
			if len(g.Containers[empty]) != 0 {
				t.Fatal("empty container gained children during layout")
			}
		})
	}
}

func TestPreprocessHierarchiesKeepsMembershipConsistent(t *testing.T) {
	ctx := withTestLogger(t.Context(), t)
	g, err := readGraph(ctx, filepath.Join(layoutTestDir, "simple_container_hierarchy", "graph.input.json"))
	if err != nil {
		t.Fatal(err)
	}
	pipeline := newPipeline(g, talaSeed, false)
	for _, stage := range pipeline.stagePlan()[:5] {
		if err := stage.run(pipeline, ctx); err != nil {
			t.Fatalf("%s: %v", stage.name, err)
		}
	}
	assertHierarchyMembershipConsistent(t, g)

	copy, err := layoutgraph.Clone(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	assertHierarchyMembershipConsistent(t, copy)
}

func assertHierarchyMembershipConsistent(t *testing.T, g *layoutgraph.Graph) {
	t.Helper()
	for _, node := range g.Nodes {
		if node.Hierarchy == nil {
			continue
		}
		if _, exists := node.Hierarchy.Levels()[node]; !exists {
			t.Fatalf("node %s points to a hierarchy that does not contain it", node.DebugID())
		}
		for member := range node.Hierarchy.Levels() {
			if member.Hierarchy != node.Hierarchy {
				t.Fatalf("hierarchy contains %s without the matching back-reference", member.DebugID())
			}
		}
	}
}

func TestLargeNestedLayoutCanRunTwice(t *testing.T) {
	outputs := make(map[string][]byte)
	for _, mode := range []struct {
		name    string
		enabled bool
	}{
		{name: "production"},
		{name: "dev-and-test", enabled: true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			if mode.enabled {
				t.Setenv("DEV_MODE", "on")
				t.Setenv("TEST_MODE", "on")
			} else {
				t.Setenv("DEV_MODE", "")
				t.Setenv("TEST_MODE", "")
			}

			ctx := withTestLogger(t.Context(), t)
			g, err := readGraph(ctx, filepath.Join(layoutTestDir, "large_nested", "graph.input.json"))
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				g, err = Layout(ctx, g, LayoutOptions{Seed: talaSeed})
				if err != nil {
					t.Fatalf("layout %d: %v", i+1, err)
				}
				for _, node := range g.Nodes {
					if node.IsClusterVessel() {
						t.Fatalf("layout %d: inactive cluster vessel %d aliases an ordinary node", i+1, node.ID)
					}
				}
				sequenceIDs := make(map[layoutgraph.EntityID]struct{}, len(g.Sequences))
				for vessel := range g.Sequences {
					sequenceIDs[vessel.ID] = struct{}{}
				}
				for vessel := range g.Clusters {
					if _, collides := sequenceIDs[vessel.ID]; collides {
						t.Fatalf("layout %d: cluster and sequence vessels share ID %d", i+1, vessel.ID)
					}
				}
			}
			serialized, err := graphjson.Serialize(ctx, g)
			if err != nil {
				t.Fatalf("serialize after repeated layout: %v", err)
			}
			outputs[mode.name], err = json.Marshal(serialized)
			if err != nil {
				t.Fatalf("marshal after repeated layout: %v", err)
			}
		})
	}
	if !bytes.Equal(outputs["production"], outputs["dev-and-test"]) {
		t.Fatal("repeated layout output depends on ambient DEV_MODE or TEST_MODE")
	}
}
