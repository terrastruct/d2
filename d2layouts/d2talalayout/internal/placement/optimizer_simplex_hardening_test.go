package placement

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/lib/geo"
)

func simpleOptimizationGraph(includeSizes bool) *layoutgraph.Graph {
	g := layoutgraph.NewGraph()
	positions := []geo.Point{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 4, Y: 1}}
	if includeSizes {
		positions = []geo.Point{{X: 0, Y: 0}, {X: 200, Y: 0}, {X: 400, Y: 100}}
	}
	nodes := make([]*layoutgraph.Node, len(positions))
	for i := range positions {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 50, 50)
		point := positions[i]
		node.TopLeft = &point
		g.AddNodeUnchecked(node)
		nodes[i] = node
	}
	g.Connect(nodes[0], nodes[1])
	g.Connect(nodes[1], nodes[2])
	g.ComputeCellSize()
	return g
}

type exactOptimizerPosition struct {
	pointer *geo.Point
	value   geo.Point
	width   float64
	height  float64
}

func captureExactOptimizerPositions(g *layoutgraph.Graph) map[*layoutgraph.Node]exactOptimizerPosition {
	positions := make(map[*layoutgraph.Node]exactOptimizerPosition, len(g.Nodes))
	for _, node := range g.Nodes {
		positions[node] = exactOptimizerPosition{
			pointer: node.TopLeft,
			value:   *node.TopLeft,
			width:   node.Width,
			height:  node.Height,
		}
	}
	return positions
}

func requireExactOptimizerPositions(t *testing.T, positions map[*layoutgraph.Node]exactOptimizerPosition) {
	t.Helper()
	for node, want := range positions {
		if node.TopLeft != want.pointer || node.TopLeft == nil || *node.TopLeft != want.value {
			t.Fatalf("node %d position = %p %v; want %p %v", node.ID, node.TopLeft, node.TopLeft, want.pointer, want.value)
		}
		if node.Width != want.width || node.Height != want.height {
			t.Fatalf("node %d dimensions = %vx%v; want %vx%v", node.ID, node.Width, node.Height, want.width, want.height)
		}
	}
}

type cancelOnOptimizerMutation struct {
	context.Context
	positions map[*layoutgraph.Node]exactOptimizerPosition
	observed  bool
	panic     bool
}

func (ctx *cancelOnOptimizerMutation) Err() error {
	for node, original := range ctx.positions {
		if node.TopLeft != original.pointer || node.TopLeft == nil || *node.TopLeft != original.value ||
			node.Width != original.width || node.Height != original.height {
			ctx.observed = true
			if ctx.panic {
				panic("optimizer mutation observer")
			}
			return context.Canceled
		}
	}
	return nil
}

func requireOptimizationResourceError(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, limits.ErrOptimizationResourceLimit) {
		t.Fatalf("error = %v; want optimization resource limit", err)
	}
}

func TestOptimizerMedianSingleton(t *testing.T) {
	graph := layoutgraph.NewGraph()
	graph.CellSize = 3
	node := graph.AddNode(layoutgraph.NewNode(1, 4, 8))
	node.TopLeft = geo.NewPoint(1.25, -2.75)

	for _, test := range []struct {
		name         string
		includeSizes bool
		wantX        float64
		wantY        float64
	}{
		{name: "sizeless", wantX: 1.25 + 0.5, wantY: -2.75 + 0.5},
		{name: "sized", includeSizes: true, wantX: (1.25 + 4.0/2) / 3, wantY: (-2.75 + 8.0/2) / 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			guard, err := limits.NewOptimizationWorkGuard(t.Context(), "test", 1)
			if err != nil {
				t.Fatal(err)
			}
			gotX, gotY, err := optimizerMedian(layoutgraph.Nodes{node}, test.includeSizes, guard)
			if err != nil {
				t.Fatal(err)
			}
			if math.Float64bits(gotX) != math.Float64bits(test.wantX) || math.Float64bits(gotY) != math.Float64bits(test.wantY) {
				t.Fatalf("median = (%v, %v); want (%v, %v)", gotX, gotY, test.wantX, test.wantY)
			}
			if guard.Used() != 1 {
				t.Fatalf("median work = %d, want 1", guard.Used())
			}
		})
	}

	t.Run("NaN", func(t *testing.T) {
		node.TopLeft.X = math.NaN()
		guard, err := limits.NewOptimizationWorkGuard(t.Context(), "test", 1)
		if err != nil {
			t.Fatal(err)
		}
		gotX, gotY, err := optimizerMedian(layoutgraph.Nodes{node}, false, guard)
		if err != nil {
			t.Fatal(err)
		}
		if !math.IsNaN(gotX) || math.Float64bits(gotY) != math.Float64bits(-2.75+0.5) {
			t.Fatalf("median = (%v, %v); want (NaN, %v)", gotX, gotY, -2.75+0.5)
		}
	})

	t.Run("W-1", func(t *testing.T) {
		guard, err := limits.NewOptimizationWorkGuard(t.Context(), "test", 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := guard.Step(); err != nil {
			t.Fatal(err)
		}
		_, _, err = optimizerMedian(layoutgraph.Nodes{node}, false, guard)
		requireOptimizationResourceError(t, err)
		if guard.Used() != 1 {
			t.Fatalf("failed median work = %d, want 1", guard.Used())
		}
	})
}

func TestOptimizerMedianSingletonRetainsErrorOrder(t *testing.T) {
	graph := layoutgraph.NewGraph()
	node := graph.AddNode(layoutgraph.NewNode(1, 20, 30))
	node.TopLeft = geo.NewPoint(10, 15)
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 2}
	guard, err := limits.NewOptimizationWorkGuard(ctx, "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}

	// The second retained sort check must observe cancellation before the
	// invalid cell size is considered.
	_, _, err = optimizerMedian(layoutgraph.Nodes{node}, true, guard)
	requireCanceledAt(t, err, "test")
	if guard.Used() != 1 {
		t.Fatalf("median work after cancellation = %d, want 1", guard.Used())
	}

	unpositioned := graph.AddNode(layoutgraph.NewNode(2, 20, 30))
	ctx = &cancelAfterErrChecks{Context: context.Background(), remaining: 1}
	guard, err = limits.NewOptimizationWorkGuard(ctx, "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = optimizerMedian(layoutgraph.Nodes{unpositioned}, false, guard)
	if err == nil || err.Error() != "TALA test cannot compute a median with an unpositioned neighbor" {
		t.Fatalf("unpositioned median error = %v", err)
	}
	if guard.Used() != 1 {
		t.Fatalf("unpositioned median work = %d, want 1", guard.Used())
	}
}

func BenchmarkOptimizerMedianSingleton(b *testing.B) {
	graph := layoutgraph.NewGraph()
	graph.CellSize = 5
	node := graph.AddNode(layoutgraph.NewNode(1, 20, 30))
	node.TopLeft = geo.NewPoint(10, 15)
	guard, err := limits.NewOptimizationWorkGuard(b.Context(), "benchmark", limits.MaxOptimizationWorkUnits)
	if err != nil {
		b.Fatal(err)
	}
	nodes := layoutgraph.Nodes{node}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := optimizerMedian(nodes, true, guard); err != nil {
			b.Fatal(err)
		}
	}
}

func TestOptimizerCostSnapshotCancellationChargesExactCacheWork(t *testing.T) {
	g := layoutgraph.NewGraph()
	for state := uint64(0); state < 65; state++ {
		g.StoreEdgeLengthCost(state, float64(state))
	}
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 1}
	guard, err := limits.NewOptimizationWorkGuard(ctx, "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := captureOptimizerMutationState(g, guard)
	if snapshot != nil {
		t.Fatal("optimizer snapshot was returned after cancellation")
	}
	requireCanceledAt(t, err, "test")
	if guard.Used() != 64 {
		t.Fatalf("snapshot work after cancellation = %d, want 64", guard.Used())
	}
	if got := g.EdgeLengthCacheEntries(); got != 65 {
		t.Fatalf("cache entries after canceled snapshot = %d, want 65", got)
	}
}

func TestOptimizerCostSnapshotWorkLimitBoundary(t *testing.T) {
	newGraph := func() *layoutgraph.Graph {
		g := layoutgraph.NewGraph()
		for state := uint64(0); state < 3; state++ {
			g.StoreEdgeLengthCost(state, float64(state))
		}
		return g
	}

	t.Run("W-1", func(t *testing.T) {
		guard, err := limits.NewOptimizationWorkGuard(context.Background(), "test", 2)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := captureOptimizerMutationState(newGraph(), guard)
		if snapshot != nil {
			t.Fatal("optimizer snapshot was returned above the work limit")
		}
		requireOptimizationResourceError(t, err)
		if guard.Used() != 2 {
			t.Fatalf("failed snapshot work = %d, want 2", guard.Used())
		}
	})

	t.Run("W", func(t *testing.T) {
		guard, err := limits.NewOptimizationWorkGuard(context.Background(), "test", 3)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := captureOptimizerMutationState(newGraph(), guard)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot == nil {
			t.Fatal("optimizer snapshot is nil at the exact work limit")
		}
		if guard.Used() != 3 {
			t.Fatalf("successful snapshot work = %d, want 3", guard.Used())
		}
	})
}

func TestOptimizerCostSnapshotChecksCancellationAfterCopy(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.StoreEdgeLengthCost(1, 10)
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 1}
	guard, err := limits.NewOptimizationWorkGuard(ctx, "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := captureOptimizerMutationState(g, guard)
	if snapshot != nil {
		t.Fatal("optimizer snapshot was returned after post-copy cancellation")
	}
	requireCanceledAt(t, err, "test")
	if guard.Used() != 1 {
		t.Fatalf("snapshot work after post-copy cancellation = %d, want 1", guard.Used())
	}
}

func TestOptimizerAncestryHostileDepthIsIterative(t *testing.T) {
	const count = limits.MaxEngineNodes
	g := layoutgraph.NewGraph()
	var root, current *layoutgraph.Node
	for i := 0; i < count; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i), 0)
		node.Container = current
		g.AddNodeUnchecked(node)
		if root == nil {
			root = node
		}
		current = node
	}
	guard, err := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	descendant, err := optimizerIsDescendantOf(current, root, guard)
	if err != nil {
		t.Fatal(err)
	}
	if !descendant {
		t.Fatal("deepest node was not recognized as a descendant")
	}
}

func TestOptimizerMoveHostileDepthIsIterative(t *testing.T) {
	if testing.Short() {
		t.Skip("hostile-depth regression")
	}
	const count = limits.MaxEngineNodes
	g := layoutgraph.NewGraph()
	nodes := make([]*layoutgraph.Node, count)
	for i := range nodes {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i), float64(i))
		g.AddNewNodeToContainer(nil, node)
		nodes[i] = node
		if i > 0 {
			parent := nodes[i-1]
			g.Containers[nil] = g.Containers[nil][:len(g.Containers[nil])-1]
			g.AddNodeToContainer(parent, node)
		}
	}
	guard, err := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	if err := optimizerMoveNodeAbs(nodes[0], 10, 20, guard); err != nil {
		t.Fatal(err)
	}
	want := geo.Point{X: float64(count-1) + 10, Y: float64(count-1) + 20}
	if got := *nodes[count-1].TopLeft; got != want {
		t.Fatalf("deepest descendant position = %v; want %v", got, want)
	}
}

func TestOptimizerMoveLowLimitRestoresMidMutation(t *testing.T) {
	g := layoutgraph.NewGraph()
	parent := layoutgraph.NewNode(1, 10, 10)
	parent.TopLeft = geo.NewPoint(0, 0)
	child := layoutgraph.NewNode(2, 10, 10)
	child.TopLeft = geo.NewPoint(5, 5)
	g.AddNewNodeToContainer(nil, parent)
	g.AddNewNodeToContainer(parent, child)
	positions := captureExactOptimizerPositions(g)

	// Three work units discover and snapshot the descendant. The fourth check
	// happens only after the parent has been translated.
	guard, err := limits.NewOptimizationWorkGuard(context.Background(), "test", 3)
	if err != nil {
		t.Fatal(err)
	}
	err = optimizerMoveNodeAbs(parent, 100, 100, guard)
	requireOptimizationResourceError(t, err)
	requireExactOptimizerPositions(t, positions)
}

func TestOptimizerSetupHonorsCanceledContext(t *testing.T) {
	g := simpleOptimizationGraph(true)

	t.Run("sized", func(t *testing.T) {
		optim, err := newSizedOptimizer(canceledContext(), g, nil, nil, rand.New(rand.NewSource(1)), nil)
		if optim != nil {
			t.Fatal("sized optimizer was returned after cancellation")
		}
		requireCanceledAt(t, err, "LocalOptimizeSetup")
	})

	t.Run("sizeless", func(t *testing.T) {
		optim, err := newSizelessOptimizer(canceledContext(), g, rand.New(rand.NewSource(1)))
		if optim != nil {
			t.Fatal("sizeless optimizer was returned after cancellation")
		}
		requireCanceledAt(t, err, "sizelessOptimizer.setup")
	})
}

func TestSizedOptimizerLowLimitRestoresExactState(t *testing.T) {
	g := simpleOptimizationGraph(true)
	positions := captureExactOptimizerPositions(g)
	optim, err := newSizedOptimizer(context.Background(), g, nil, nil, rand.New(rand.NewSource(1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = optim.optimizeWithLimit(context.Background(), 0, 2)
	requireOptimizationResourceError(t, err)
	requireExactOptimizerPositions(t, positions)
}

func TestSizelessOptimizerLowLimitRestoresExactState(t *testing.T) {
	g := simpleOptimizationGraph(false)
	positions := captureExactOptimizerPositions(g)
	optim, err := newSizelessOptimizer(context.Background(), g, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatal(err)
	}
	occupiedRef := optim.occupied
	err = optim.optimizeWithLimit(context.Background(), 0, 2)
	requireOptimizationResourceError(t, err)
	requireExactOptimizerPositions(t, positions)
	if len(optim.occupied) != len(occupiedRef) {
		t.Fatalf("occupied cache length = %d; want %d", len(optim.occupied), len(occupiedRef))
	}
	probe := geo.Point{X: -1, Y: -1}
	occupiedRef[probe] = g.Nodes[0]
	if optim.occupied[probe] != g.Nodes[0] {
		t.Fatal("occupied cache map identity changed during rollback")
	}
	delete(occupiedRef, probe)
}

func TestSizelessOptimizerMidMutationCancellationRestoresExactState(t *testing.T) {
	g := simpleOptimizationGraph(false)
	positions := captureExactOptimizerPositions(g)
	optim, err := newSizelessOptimizer(context.Background(), g, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelOnOptimizerMutation{Context: context.Background(), positions: positions}
	err = optim.optimize(ctx, 0)
	if !ctx.observed {
		t.Fatal("cancellation did not observe a trial mutation")
	}
	requireCanceledAt(t, err, "EdgeLength")
	requireExactOptimizerPositions(t, positions)
	for node, position := range positions {
		if optim.occupied[position.value] != node {
			t.Fatalf("occupied cache did not restore node %d", node.ID)
		}
	}
}

func TestSizedOptimizerMidMutationPanicRestoresExactState(t *testing.T) {
	g := simpleOptimizationGraph(true)
	positions := captureExactOptimizerPositions(g)
	options := placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true}
	if _, err := placementcost.EdgeLength(context.Background(), g, options); err != nil {
		t.Fatal(err)
	}
	const wantState uint64 = 0x7f00a11
	const wantCost = 73.5
	g.StoreEdgeLengthCost(wantState, wantCost)
	wantCosts := g.RoutingCosts()
	if cached, ok := g.LookupEdgeLengthCost(wantState); !ok || cached != wantCost {
		t.Fatalf("initial cached cost = (%v, %v), want (%v, true)", cached, ok, wantCost)
	}
	wantEntries := g.EdgeLengthCacheEntries()
	optim, err := newSizedOptimizer(context.Background(), g, nil, nil, rand.New(rand.NewSource(1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelOnOptimizerMutation{Context: context.Background(), positions: positions, panic: true}

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("optimizer mutation panic was not propagated")
			}
		}()
		_, _ = optim.optimize(ctx, 0)
	}()
	if !ctx.observed {
		t.Fatal("panic did not observe a trial mutation")
	}
	requireExactOptimizerPositions(t, positions)
	if gotCosts := g.RoutingCosts(); gotCosts != wantCosts {
		t.Fatalf("placement costs after rollback = %+v, want %+v", gotCosts, wantCosts)
	}
	if gotEntries := g.EdgeLengthCacheEntries(); gotEntries != wantEntries {
		t.Fatalf("cache entries after rollback = %d, want %d", gotEntries, wantEntries)
	}
	if cached, ok := g.LookupEdgeLengthCost(wantState); !ok || cached != wantCost {
		t.Fatalf("cached cost after rollback = (%v, %v), want (%v, true)", cached, ok, wantCost)
	}
}

func TestOptimizerSuccessfulRunsRemainDeterministic(t *testing.T) {
	t.Run("sized", func(t *testing.T) {
		var results [2]map[layoutgraph.EntityID]geo.Point
		for run := range results {
			g := simpleOptimizationGraph(true)
			optim, err := newSizedOptimizer(context.Background(), g, nil, nil, rand.New(rand.NewSource(7)), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := optim.optimize(context.Background(), 0); err != nil {
				t.Fatal(err)
			}
			results[run] = make(map[layoutgraph.EntityID]geo.Point, len(g.Nodes))
			for _, node := range g.Nodes {
				results[run][node.ID] = *node.TopLeft
			}
		}
		for id, want := range results[0] {
			if got := results[1][id]; got != want {
				t.Fatalf("node %d second position = %v; want %v", id, got, want)
			}
		}
	})

	t.Run("sizeless", func(t *testing.T) {
		var results [2]map[layoutgraph.EntityID]geo.Point
		for run := range results {
			g := simpleOptimizationGraph(false)
			optim, err := newSizelessOptimizer(context.Background(), g, rand.New(rand.NewSource(7)))
			if err != nil {
				t.Fatal(err)
			}
			if err := optim.optimize(context.Background(), 0); err != nil {
				t.Fatal(err)
			}
			results[run] = make(map[layoutgraph.EntityID]geo.Point, len(g.Nodes))
			for _, node := range g.Nodes {
				results[run][node.ID] = *node.TopLeft
			}
		}
		for id, want := range results[0] {
			if got := results[1][id]; got != want {
				t.Fatalf("node %d second position = %v; want %v", id, got, want)
			}
		}
	})
}
