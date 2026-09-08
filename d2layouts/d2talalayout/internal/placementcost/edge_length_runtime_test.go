package placementcost

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

type secretWorkerPanic struct {
	stringCalled atomic.Bool
}

func (payload *secretWorkerPanic) String() string {
	payload.stringCalled.Store(true)
	return "DO_NOT_EXPOSE_WORKER_SECRET"
}

type panicInsideEdgeLengthContext struct {
	context.Context
	payload *secretWorkerPanic
}

func (ctx *panicInsideEdgeLengthContext) Err() error {
	var callers [32]uintptr
	count := runtime.Callers(2, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, ".NodeEdgeLength") {
			panic(ctx.payload)
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

func TestEdgeLengthWorkerCount(t *testing.T) {
	tests := []struct {
		name       string
		nodeCount  int
		gomaxprocs int
		want       int
	}{
		{name: "empty", nodeCount: 0, gomaxprocs: 8, want: 0},
		{name: "small graph stays sequential", nodeCount: 10, gomaxprocs: 8, want: 1},
		{name: "invalid GOMAXPROCS is clamped", nodeCount: 11, gomaxprocs: 0, want: 1},
		{name: "bounded by GOMAXPROCS", nodeCount: 100, gomaxprocs: 4, want: 4},
		{name: "bounded by nodes", nodeCount: 11, gomaxprocs: 32, want: 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := edgeLengthWorkerCount(tt.nodeCount, tt.gomaxprocs); got != tt.want {
				t.Fatalf("edgeLengthWorkerCount(%d, %d) = %d; want %d", tt.nodeCount, tt.gomaxprocs, got, tt.want)
			}
		})
	}
}

func newEdgeLengthRuntimeGraph() *layoutgraph.Graph {
	g := layoutgraph.NewGraph()
	var previous *layoutgraph.Node
	for i := 0; i < 12; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 20+float64(i), 10+float64(i%3))
		node.TopLeft = geo.NewPoint(float64(i*40), float64((i%4)*30))
		g.AddNode(node)
		g.AddNodeToContainer(nil, node)
		if previous != nil {
			g.Connect(previous, node)
		}
		previous = node
	}
	g.ComputeCellSize()
	return g
}

func TestEdgeLengthParallelSummationMatchesSequential(t *testing.T) {
	oldGOMAXPROCS := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(oldGOMAXPROCS)
	sequential, err := EdgeLength(context.Background(), newEdgeLengthRuntimeGraph(), EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		t.Fatal(err)
	}

	runtime.GOMAXPROCS(4)
	parallel, err := EdgeLength(context.Background(), newEdgeLengthRuntimeGraph(), EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		t.Fatal(err)
	}

	if parallel != sequential {
		t.Fatalf("parallel edge length = %.17g; sequential = %.17g", parallel, sequential)
	}
}

func TestEdgeLengthParallelCacheRoundTrip(t *testing.T) {
	oldGOMAXPROCS := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(oldGOMAXPROCS)
	g := newEdgeLengthRuntimeGraph()
	options := EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true}
	state := placementEdgeLengthState(g, g.RoutingCosts(), options)

	first, err := EdgeLength(context.Background(), g, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EdgeLength(context.Background(), g, options)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("cached edge length = %.17g, want %.17g", second, first)
	}
	if cached, ok := g.LookupEdgeLengthCost(state); !ok || cached != first {
		t.Fatalf("cached edge length = (%.17g, %v), want (%.17g, true)", cached, ok, first)
	}
}

func TestEdgeLengthWorkerPanicIsSanitizedAfterJoin(t *testing.T) {
	g := layoutgraph.NewGraph()
	for i := 0; i < 12; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i*20), 0)
		g.AddNodeUnchecked(node)
	}
	payload := &secretWorkerPanic{}
	ctx := &panicInsideEdgeLengthContext{Context: context.Background(), payload: payload}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = EdgeLength(ctx, g, EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: false, EnforceMinimumGap: false, PenalizeDirection: false})
	}()
	message, ok := recovered.(string)
	if !ok || message != "TALA EdgeLength worker invariant failure" {
		t.Fatalf("recovered panic = %#v, want generic invariant marker", recovered)
	}
	if payload.stringCalled.Load() {
		t.Fatal("worker recovery invoked the secret payload's String method")
	}
}

func TestNodeEdgeLengthCanceledDuringEdgeScan(t *testing.T) {
	g := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 10, 10)
	from.TopLeft = geo.NewPoint(0, 0)
	to := layoutgraph.NewNode(2, 10, 10)
	to.TopLeft = geo.NewPoint(20, 0)
	g.AddNode(from)
	g.AddNode(to)
	g.Connect(from, to)

	// The initial check and two scratch initialization checks succeed;
	// cancellation is then observed as the edge scan begins.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 3}
	length, err := NodeEdgeLength(ctx, from, EdgeLengthOptions{EdgeAbductions: []*layoutgraph.EdgeAbduction{}, IncludeNodeSizes: false, EnforceMinimumGap: false, PenalizeDirection: true})
	requireCanceledAt(t, err, "EdgeLength")
	if length != 0 {
		t.Fatalf("edge length after cancellation = %v; want 0", length)
	}
}

func newParallelLabeledEdgeLengthGraph(edgeCount int) (*layoutgraph.Graph, *layoutgraph.Node, EdgeLengthOptions) {
	g := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 10, 10)
	from.TopLeft = geo.NewPoint(0, 0)
	to := layoutgraph.NewNode(2, 10, 10)
	to.TopLeft = geo.NewPoint(0, 100)
	g.AddNode(from)
	g.AddNode(to)
	g.AddNodeToContainer(nil, from)
	g.AddNodeToContainer(nil, to)
	g.CellSize = 10
	g.Directions[nil] = geo.Right
	g.RestoreRoutingCosts(layoutgraph.RoutingCostState{Turn: 1})

	for range edgeCount {
		edge := g.Connect(from, to)
		edge.Label = &layoutgraph.Label{Text: "edge", Width: 28, Height: 10}
	}

	return g, from, EdgeLengthOptions{
		EdgeAbductions:    []*layoutgraph.EdgeAbduction{},
		IncludeNodeSizes:  true,
		EnforceMinimumGap: false,
		PenalizeDirection: true,
	}
}

func TestNodeEdgeLengthCountsAllParallelLabeledEdges(t *testing.T) {
	for _, edgeCount := range []int{127, 128} {
		t.Run(fmt.Sprint(edgeCount), func(t *testing.T) {
			g, from, options := newParallelLabeledEdgeLengthGraph(edgeCount)
			if err := layoutgraph.Validate(t.Context(), "parallel labeled-edge test", g); err != nil {
				t.Fatalf("invalid test graph: %v", err)
			}
			labeled, err := NodeEdgeLength(t.Context(), from, options)
			if err != nil {
				t.Fatal(err)
			}
			for _, edge := range g.Edges {
				edge.Label = nil
			}
			unlabeled, err := NodeEdgeLength(t.Context(), from, options)
			if err != nil {
				t.Fatal(err)
			}

			got := labeled - unlabeled
			want := float64(edgeCount * edgeCount)
			if got != want {
				t.Fatalf("parallel labeled-edge penalty = %v; want %v", got, want)
			}
		})
	}
}

func TestGraphEdgeCrossingsCanceledDuringPairScan(t *testing.T) {
	g := layoutgraph.NewGraph()
	points := []*geo.Point{
		geo.NewPoint(0, 0),
		geo.NewPoint(10, 10),
		geo.NewPoint(0, 10),
		geo.NewPoint(10, 0),
	}
	nodes := make([]*layoutgraph.Node, len(points))
	for i, point := range points {
		nodes[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+1), 1, 1)
		nodes[i].TopLeft = point
		g.AddNode(nodes[i])
	}
	g.Connect(nodes[0], nodes[1])
	g.Connect(nodes[2], nodes[3])

	// Preflight, edge grouping, and the outer pair loop succeed; cancellation
	// is observed inside the quadratic pair scan.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 3}
	var crossings int64
	crossings, err := GraphEdgeCrossings(ctx, g)
	requireCanceledAt(t, err, "EdgeLength")
	if crossings != 0 {
		t.Fatalf("crossings after cancellation = %d; want 0", crossings)
	}
	crossings, err = GraphEdgeCrossings(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if crossings != 1 {
		t.Fatalf("crossings after canceled scratch reuse = %d; want 1", crossings)
	}
}

func TestGraphEdgeCrossingsEmptyGraphChecksFinalCancellation(t *testing.T) {
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 1}
	crossings, err := GraphEdgeCrossings(ctx, layoutgraph.NewGraph())
	requireCanceledAt(t, err, "EdgeLength")
	if crossings != 0 {
		t.Fatalf("crossings after final cancellation = %d; want 0", crossings)
	}
}

func TestGraphEdgeCrossingsCanceledDuringGroupingReleasesScratch(t *testing.T) {
	g := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 1, 1)
	from.TopLeft = geo.NewPoint(0, 0)
	to := layoutgraph.NewNode(2, 1, 1)
	to.TopLeft = geo.NewPoint(10, 10)
	g.AddNewNodeToContainer(nil, from)
	g.AddNewNodeToContainer(nil, to)
	for range 65 {
		g.Connect(from, to)
	}

	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 2}
	crossings, err := GraphEdgeCrossings(ctx, g)
	requireCanceledAt(t, err, "EdgeLength")
	if crossings != 0 {
		t.Fatalf("crossings after grouping cancellation = %d; want 0", crossings)
	}

	crossings, err = GraphEdgeCrossings(context.Background(), newGraphEdgeCrossingLevelsGraph())
	if err != nil {
		t.Fatal(err)
	}
	if crossings != 2 {
		t.Fatalf("crossings after canceled grouping scratch reuse = %d; want 2", crossings)
	}
}

func addGraphEdgeCrossingLevel(g *layoutgraph.Graph, container *layoutgraph.Node, firstID layoutgraph.EntityID, offset float64) []*layoutgraph.Node {
	points := []*geo.Point{
		geo.NewPoint(offset, offset),
		geo.NewPoint(offset+10, offset+10),
		geo.NewPoint(offset, offset+10),
		geo.NewPoint(offset+10, offset),
	}
	nodes := make([]*layoutgraph.Node, len(points))
	for i, point := range points {
		nodes[i] = layoutgraph.NewNode(firstID+layoutgraph.EntityID(i), 1, 1)
		nodes[i].TopLeft = point
		g.AddNewNodeToContainer(container, nodes[i])
	}
	g.Connect(nodes[0], nodes[1])
	g.Connect(nodes[2], nodes[3])
	return nodes
}

func newGraphEdgeCrossingLevelsGraph() *layoutgraph.Graph {
	g := layoutgraph.NewGraph()
	rootNodes := addGraphEdgeCrossingLevel(g, nil, 1, 0)
	container := layoutgraph.NewNode(10, 40, 40)
	container.TopLeft = geo.NewPoint(100, 100)
	g.AddNewNodeToContainer(nil, container)
	nestedNodes := addGraphEdgeCrossingLevel(g, container, 11, 100)

	// Cross-level and incomplete edges are ignored by GraphEdgeCrossings.
	g.Connect(rootNodes[0], nestedNodes[0])
	g.Edges = append(g.Edges, &layoutgraph.Edge{From: rootNodes[1]})
	return g
}

func TestGraphEdgeCrossingsGroupsByContainerLevelAndReusesScratch(t *testing.T) {
	g := newGraphEdgeCrossingLevelsGraph()
	for i := 0; i < 25; i++ {
		crossings, err := GraphEdgeCrossings(t.Context(), g)
		if err != nil {
			t.Fatal(err)
		}
		if crossings != 2 {
			t.Fatalf("iteration %d crossings = %d; want 2", i, crossings)
		}

		emptyCrossings, err := GraphEdgeCrossings(t.Context(), layoutgraph.NewGraph())
		if err != nil {
			t.Fatal(err)
		}
		if emptyCrossings != 0 {
			t.Fatalf("iteration %d empty crossings = %d; want 0", i, emptyCrossings)
		}
	}
}

type panicAfterGraphEdgeCrossingChecks struct {
	context.Context
	remaining int
	payload   any
}

func (ctx *panicAfterGraphEdgeCrossingChecks) Err() error {
	if ctx.remaining == 0 {
		panic(ctx.payload)
	}
	ctx.remaining--
	return nil
}

func TestGraphEdgeCrossingsReleasesScratchAfterPanic(t *testing.T) {
	g := newGraphEdgeCrossingLevelsGraph()
	payload := new(int)
	ctx := &panicAfterGraphEdgeCrossingChecks{
		Context:   context.Background(),
		remaining: 2,
		payload:   payload,
	}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = GraphEdgeCrossings(ctx, g)
	}()
	if recovered != payload {
		t.Fatalf("panic payload = %v; want exact sentinel %v", recovered, payload)
	}

	crossings, err := GraphEdgeCrossings(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	if crossings != 2 {
		t.Fatalf("crossings after panic = %d; want 2", crossings)
	}
}

func TestGraphEdgeCrossingsConcurrentScratch(t *testing.T) {
	g := newGraphEdgeCrossingLevelsGraph()
	const workers = 8
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				crossings, err := GraphEdgeCrossings(context.Background(), g)
				if err != nil {
					errs <- err
					return
				}
				if crossings != 2 {
					errs <- fmt.Errorf("crossings = %d; want 2", crossings)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestColumnCrossingCostCanceledDuringPairScan(t *testing.T) {
	g := layoutgraph.NewGraph()
	table := layoutgraph.NewNode(1, 40, 40)
	table.TopLeft = geo.NewPoint(0, 0)
	table.SetShape(shape.TABLE_TYPE)
	table.SetNumColumns(1)
	g.AddNewNodeToContainer(nil, table)

	for i := 0; i < 130; i++ {
		other := layoutgraph.NewNode(layoutgraph.EntityID(i+2), 40, 40)
		other.TopLeft = geo.NewPoint(100, float64(i*50))
		other.SetShape(shape.TABLE_TYPE)
		other.SetNumColumns(1)
		g.AddNewNodeToContainer(nil, other)
		edge := g.Connect(table, other)
		edge.FromTableColumnIndex = new(int)
		edge.ToTableColumnIndex = new(int)
	}
	g.ComputeCellSize()

	// Preflight and the 0/64/128 edge checks succeed, as do the crossing
	// preflight, outer-loop check, and first inner-loop check. Cancellation is
	// observed at the next bounded check inside the pair scan.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 7}
	cost, err := ColumnCrossingCost(ctx, table, nil)
	requireCanceledAt(t, err, "EdgeLength")
	if cost != 0 {
		t.Fatalf("column crossing cost after cancellation = %v; want 0", cost)
	}
}

func TestSymmetryCanceledDuringPairScan(t *testing.T) {
	g := layoutgraph.NewGraph()
	center := layoutgraph.NewNode(1, 10, 10)
	center.TopLeft = geo.NewPoint(0, 0)
	g.AddNewNodeToContainer(nil, center)

	neighbors := make([]*layoutgraph.Node, 130)
	for i := range neighbors {
		neighbors[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+2), 10, 10)
		neighbors[i].TopLeft = geo.NewPoint(float64(i+1)*20, 0)
		g.AddNewNodeToContainer(nil, neighbors[i])
	}

	// Preflight and the first outer-loop check succeed; cancellation is then
	// observed in the quadratic neighbor-pair scan.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 2}
	score, matched, err := computeSymmetryScore(ctx, center, neighbors)
	requireCanceledAt(t, err, "EdgeLength")
	if score != 0 || matched != nil {
		t.Fatalf("symmetry after cancellation = (%v, %v); want (0, nil)", score, matched)
	}
}

func TestContainerAlignmentCanceledDuringPairScan(t *testing.T) {
	g := layoutgraph.NewGraph()
	for i := 0; i < 130; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i*20), float64(i*20))
		node.SetContainer(true)
		g.AddNewNodeToContainer(nil, node)
	}

	// Preflight and the first outer-loop check succeed; cancellation is then
	// observed in the quadratic node-pair scan.
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 2}
	alignment, err := ContainerAlignmentCost(ctx, g)
	requireCanceledAt(t, err, "EdgeLength")
	if alignment != 0 {
		t.Fatalf("container alignment after cancellation = %v; want 0", alignment)
	}
}
