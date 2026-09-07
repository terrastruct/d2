package labeling

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func requireCanceledAt(t *testing.T, err error, location string) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(%v, context.Canceled) = false", err)
	}
	if !strings.Contains(err.Error(), location) {
		t.Fatalf("cancellation error = %v, want operation %q", err, location)
	}
}

type labelMutationContext struct {
	context.Context
	changed  func() bool
	result   error
	panicNow bool
	observed bool
}

func (ctx *labelMutationContext) Err() error {
	if ctx.changed() {
		ctx.observed = true
		if ctx.panicNow {
			panic("label placement test panic")
		}
		return ctx.result
	}
	return nil
}

type labelPlacementTestState struct {
	icon               *layoutgraph.Icon
	iconPosition       label.Position
	nodeLabel          *layoutgraph.Label
	nodeLabelPosition  label.Position
	edgeLabels         []*layoutgraph.Label
	edgeLabelPositions []label.Position
	edgePercentages    []float64
}

func captureLabelPlacementTestState(g *layoutgraph.Graph, edges []*layoutgraph.Edge) labelPlacementTestState {
	state := labelPlacementTestState{
		icon:              g.Nodes[0].Icon,
		iconPosition:      g.Nodes[0].Icon.Position,
		nodeLabel:         g.Nodes[0].Label,
		nodeLabelPosition: g.Nodes[0].Label.Position,
	}
	for _, edge := range edges {
		state.edgeLabels = append(state.edgeLabels, edge.Label)
		state.edgeLabelPositions = append(state.edgeLabelPositions, edge.Label.Position)
		state.edgePercentages = append(state.edgePercentages, edge.LabelPercentage)
	}
	return state
}

func (state labelPlacementTestState) changed(g *layoutgraph.Graph, edges []*layoutgraph.Edge) bool {
	if g.Nodes[0].Icon != state.icon || state.icon.Position != state.iconPosition {
		return true
	}
	if g.Nodes[0].Label != state.nodeLabel || state.nodeLabel.Position != state.nodeLabelPosition {
		return true
	}
	for index, edge := range edges {
		if edge.Label != state.edgeLabels[index] ||
			edge.Label.Position != state.edgeLabelPositions[index] ||
			edge.LabelPercentage != state.edgePercentages[index] {
			return true
		}
	}
	return false
}

func (state labelPlacementTestState) requireRestored(t *testing.T, g *layoutgraph.Graph, edges []*layoutgraph.Edge) {
	t.Helper()
	if state.changed(g, edges) {
		t.Fatalf(
			"label placement state was not restored: icon=%v node-label=%v edge-labels=%v percentages=%v",
			g.Nodes[0].Icon.Position,
			g.Nodes[0].Label.Position,
			[]label.Position{edges[0].Label.Position, edges[1].Label.Position},
			[]float64{edges[0].LabelPercentage, edges[1].LabelPercentage},
		)
	}
}

func newLabelPlacementAtomicityGraph() (*layoutgraph.Graph, []*layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, 32)
	for index := range nodes {
		node := layoutgraph.NewNode(layoutgraph.EntityID(index+1), 40, 40)
		node.TopLeft = geo.NewPoint(float64(index%8)*300, float64(index/8)*300)
		g.AddNewNodeToContainer(nil, node)
		nodes[index] = node
	}
	nodes[0].Icon = &layoutgraph.Icon{Position: label.Unset}
	nodes[0].Label = &layoutgraph.Label{
		Text:     "node label",
		Position: label.Unset,
		Width:    50,
		Height:   20,
	}

	first := g.Connect(nodes[0], nodes[1])
	first.ID = 1
	first.Points = []*geo.Point{geo.NewPoint(40, 20), geo.NewPoint(300, 20)}
	first.Label = &layoutgraph.Label{Text: "first", Position: label.Unset, Width: 40, Height: 20}

	second := g.Connect(nodes[8], nodes[9])
	second.ID = 2
	second.Points = []*geo.Point{geo.NewPoint(40, 320), geo.NewPoint(300, 320)}
	second.Label = &layoutgraph.Label{Text: "second", Position: label.Unset, Width: 40, Height: 20}

	return g, []*layoutgraph.Edge{first, second}
}

func TestPlaceLabelsCancellationDuringOverlapRestoresExactLabelState(t *testing.T) {
	g, edges := newLabelPlacementAtomicityGraph()
	want := captureLabelPlacementTestState(g, edges)
	ctx := &labelMutationContext{
		Context: context.Background(),
		changed: func() bool { return want.changed(g, edges) },
		result:  context.Canceled,
	}

	err := Place(ctx, g)
	requireCanceledAt(t, err, "PlaceLabels")
	if !ctx.observed {
		t.Fatal("cancellation was not observed after label placement mutated state")
	}
	want.requireRestored(t, g, edges)
}

func TestPlaceNewEdgeLabelsCancellationDuringOverlapRestoresExactLabelState(t *testing.T) {
	g, edges := newLabelPlacementAtomicityGraph()
	want := captureLabelPlacementTestState(g, edges)
	ctx := &labelMutationContext{
		Context: context.Background(),
		changed: func() bool { return want.changed(g, edges) },
		result:  context.Canceled,
	}

	err := PlaceNewEdges(ctx, g, edges)
	requireCanceledAt(t, err, "PlaceNewEdgeLabels")
	if !ctx.observed {
		t.Fatal("cancellation was not observed after edge-label placement mutated state")
	}
	want.requireRestored(t, g, edges)
}

func TestPlaceLabelsWorkLimitDuringOverlapRestoresExactLabelState(t *testing.T) {
	g, edges := newLabelPlacementAtomicityGraph()
	want := captureLabelPlacementTestState(g, edges)
	ctx := &labelMutationContext{
		Context: context.Background(),
		changed: func() bool { return want.changed(g, edges) },
	}

	err := place(ctx, g, 160)
	if err == nil || !strings.Contains(err.Error(), "PlaceLabels work exceeds limit 160") {
		t.Fatalf("error = %v, want deterministic PlaceLabels work-limit error", err)
	}
	if !ctx.observed {
		t.Fatal("work limit was not reached after label placement mutated state")
	}
	want.requireRestored(t, g, edges)
}

func TestPlaceNewEdgeLabelsWorkLimitDuringOverlapRestoresExactLabelState(t *testing.T) {
	g, edges := newLabelPlacementAtomicityGraph()
	want := captureLabelPlacementTestState(g, edges)
	ctx := &labelMutationContext{
		Context: context.Background(),
		changed: func() bool { return want.changed(g, edges) },
	}

	err := placeNewEdges(ctx, g, edges, 320)
	if err == nil || !strings.Contains(err.Error(), "PlaceNewEdgeLabels work exceeds limit 320") {
		t.Fatalf("error = %v, want deterministic PlaceNewEdgeLabels work-limit error", err)
	}
	if !ctx.observed {
		t.Fatal("work limit was not reached after edge-label placement mutated state")
	}
	want.requireRestored(t, g, edges)
}

func TestPlaceLabelsPanicRestoresExactLabelState(t *testing.T) {
	g, edges := newLabelPlacementAtomicityGraph()
	want := captureLabelPlacementTestState(g, edges)
	ctx := &labelMutationContext{
		Context:  context.Background(),
		changed:  func() bool { return want.changed(g, edges) },
		panicNow: true,
	}

	func() {
		defer func() {
			if recovered := recover(); recovered == nil || fmt.Sprint(recovered) != "label placement test panic" {
				t.Fatalf("panic = %v, want label placement test panic", recovered)
			}
		}()
		_ = Place(ctx, g)
	}()
	if !ctx.observed {
		t.Fatal("panic was not triggered after label placement mutated state")
	}
	want.requireRestored(t, g, edges)
}

func TestPlaceNewEdgeLabelsPanicRestoresExactLabelState(t *testing.T) {
	g, edges := newLabelPlacementAtomicityGraph()
	want := captureLabelPlacementTestState(g, edges)
	ctx := &labelMutationContext{
		Context:  context.Background(),
		changed:  func() bool { return want.changed(g, edges) },
		panicNow: true,
	}

	func() {
		defer func() {
			if recovered := recover(); recovered == nil || fmt.Sprint(recovered) != "label placement test panic" {
				t.Fatalf("panic = %v, want label placement test panic", recovered)
			}
		}()
		_ = PlaceNewEdges(ctx, g, edges)
	}()
	if !ctx.observed {
		t.Fatal("panic was not triggered after edge-label placement mutated state")
	}
	want.requireRestored(t, g, edges)
}

func TestLabelPlacementWorkGuardRejectsOverflow(t *testing.T) {
	guard, err := newLabelPlacementWorkGuard(context.Background(), "overflow test", math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	guard.used = math.MaxInt64 - 1
	if err := guard.add(2); err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("overflow error = %v, want work-limit error", err)
	}
}

func TestLabelPlacementRunsFullTopologyPreflightBeforeScratchAllocation(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*layoutgraph.Graph) error
	}{
		{name: "PlaceLabels", run: func(g *layoutgraph.Graph) error { return Place(context.Background(), g) }},
		{name: "PlaceNewEdgeLabels", run: func(g *layoutgraph.Graph) error {
			return PlaceNewEdges(context.Background(), g, nil)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			node := layoutgraph.NewNode(1, 10, 10)
			node.TopLeft = geo.NewPoint(0, 0)
			g.AddNodeUnchecked(node)
			// The visible graph is tiny, but this hidden container slice would
			// drive a large siblings allocation without the central preflight.
			hidden := make([]*layoutgraph.Node, 1, layoutgraph.MaxTopologyReferences+2)
			hidden[0] = node
			g.Containers[nil] = hidden

			err := test.run(g)
			if err == nil || !strings.Contains(err.Error(), "topology references") {
				t.Fatalf("error = %v, want topology-reference rejection", err)
			}
		})
	}
}

func TestLabelPlacementRejectsNilContext(t *testing.T) {
	g, edges := newLabelPlacementAtomicityGraph()
	for _, test := range []struct {
		name string
		run  func() error
	}{
		{name: "PlaceLabels", run: func() error {
			//lint:ignore SA1012 This test verifies the API's nil-context rejection.
			return Place(nil, g)
		}},
		{name: "PlaceNewEdgeLabels", run: func() error {
			//lint:ignore SA1012 This test verifies the API's nil-context rejection.
			return PlaceNewEdges(nil, g, edges)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			if err == nil || !strings.Contains(err.Error(), "requires a context") {
				t.Fatalf("error = %v, want nil-context rejection", err)
			}
		})
	}
}

func TestLabelPlacementRejectsCanceledContextBeforeWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g := layoutgraph.NewGraph()
	requireCanceledAt(t, Place(ctx, g), "PlaceLabels")
	requireCanceledAt(t, PlaceNewEdges(ctx, g, nil), "PlaceNewEdgeLabels")
}

func TestLabelPlacementRejectsRepeatedEdgeAliasesWithinEachInputList(t *testing.T) {
	g, edges := newLabelPlacementAtomicityGraph()
	first := edges[0]
	g.Edges = append(g.Edges, first)
	if err := Place(context.Background(), g); err == nil || !strings.Contains(err.Error(), "repeats edge") {
		t.Fatalf("PlaceLabels error = %v, want repeated graph-edge rejection", err)
	}
	g.Edges = g.Edges[:len(g.Edges)-1]

	if err := PlaceNewEdges(context.Background(), g, []*layoutgraph.Edge{first, first}); err == nil || !strings.Contains(err.Error(), "repeats edge") {
		t.Fatalf("PlaceNewEdgeLabels error = %v, want repeated requested-edge rejection", err)
	}

	// The same edge once in the graph and once in the requested subset is the
	// normal route-only call shape and must remain accepted.
	if err := layoutgraph.ValidatePositionedGraphSelection(context.Background(), "test", g, []*layoutgraph.Edge{first}); err != nil {
		t.Fatalf("valid shared graph/request edge rejected: %v", err)
	}
}

func TestLabelPlacementDescendantAllowsExactMaximumDepth(t *testing.T) {
	nodes := make([]*layoutgraph.Node, maxLabelPlacementAncestryDepth)
	for i := range nodes {
		nodes[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+1), 1, 1)
		if i > 0 {
			nodes[i-1].Container = nodes[i]
		}
	}
	guard, err := newLabelPlacementWorkGuard(context.Background(), "test", maxLabelPlacementWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	isDescendant, err := isLabelPlacementDescendantOf(nodes[0], layoutgraph.NewNode(10_000, 1, 1), guard)
	if err != nil {
		t.Fatalf("exact-depth ancestry rejected: %v", err)
	}
	if isDescendant {
		t.Fatal("unrelated node reported as an ancestor")
	}
}
