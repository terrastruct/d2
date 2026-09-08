package placement

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func transactionBalanceSymmetryGraph(secondQualifies bool) *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	addStar := func(id layoutgraph.EntityID, x float64, suppress bool) {
		center := layoutgraph.NewNode(id, 10, 10)
		center.TopLeft = geo.NewPoint(x, 40)
		top := layoutgraph.NewNode(id+1, 10, 10)
		top.TopLeft = geo.NewPoint(x+100, 0)
		bottom := layoutgraph.NewNode(id+2, 10, 10)
		bottom.TopLeft = geo.NewPoint(x+100, 100)
		graph.AddNodeUnchecked(center)
		graph.AddNodeUnchecked(top)
		graph.AddNodeUnchecked(bottom)
		first := graph.Connect(center, top)
		second := graph.Connect(center, bottom)
		if suppress {
			column := 0
			first.FromTableColumnIndex = &column
			second.FromTableColumnIndex = &column
		}
	}
	addStar(1, 0, false)
	addStar(4, 2_000, !secondQualifies)
	return graph
}

func captureBalanceSymmetryPositions(graph *layoutgraph.Graph) map[*layoutgraph.Node]pointerSnapshot[geo.Point] {
	positions := make(map[*layoutgraph.Node]pointerSnapshot[geo.Point], len(graph.Nodes))
	for _, node := range graph.Nodes {
		positions[node] = snapshotPointer(node.TopLeft)
	}
	return positions
}

func requireBalanceSymmetryPositions(t *testing.T, positions map[*layoutgraph.Node]pointerSnapshot[geo.Point]) {
	t.Helper()
	for node, position := range positions {
		if node.TopLeft != position.pointer || node.TopLeft == nil || *node.TopLeft != position.value {
			t.Fatalf("node %d position = %p %v; want %p %v", node.ID, node.TopLeft, node.TopLeft, position.pointer, position.value)
		}
	}
}

func TestBalanceSymmetryEmptyGraphPreservesContextPreflight(t *testing.T) {
	require.NoError(t, BalanceSymmetry(t.Context(), layoutgraph.NewGraph()))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	err := BalanceSymmetry(canceled, layoutgraph.NewGraph())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BalanceSymmetry error = %v; want context.Canceled", err)
	}
}

func TestBalanceSymmetryDirectCallSharesAggregateTransactionGuard(t *testing.T) {
	baselineGuard, err := limits.NewWorkGuard(context.Background(), "BalanceSymmetryAggregateTest", limits.MaxEngineWorkUnits)
	require.NoError(t, err)
	baselineGuard.SetLimit(1_000_000)
	baselineCtx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), baselineGuard)
	baselineGraph := transactionBalanceSymmetryGraph(false)
	require.NoError(t, BalanceSymmetry(baselineCtx, baselineGraph))
	require.Positive(t, baselineGuard.Used())
	require.Equal(t, 50.0, baselineGraph.Nodes[0].TopLeft.Y)

	limitedGuard, err := limits.NewWorkGuard(context.Background(), "BalanceSymmetryAggregateTest", limits.MaxEngineWorkUnits)
	require.NoError(t, err)
	limitedGuard.SetLimit(baselineGuard.Used())
	limitedCtx := layoutgraph.ContextWithTransactionWorkGuard(context.Background(), limitedGuard)
	limitedGraph := transactionBalanceSymmetryGraph(true)
	positions := captureBalanceSymmetryPositions(limitedGraph)
	err = BalanceSymmetry(limitedCtx, limitedGraph)
	require.ErrorContains(t, err, fmt.Sprintf("work exceeds limit %d", baselineGuard.Used()))
	require.Equal(t, baselineGuard.Used()+1, limitedGuard.Used())
	requireBalanceSymmetryPositions(t, positions)
}

type interruptBalanceSymmetryAfterFirstCommit struct {
	context.Context
	node     *layoutgraph.Node
	original geo.Point
	payload  any
	observed bool
}

func (ctx *interruptBalanceSymmetryAfterFirstCommit) Err() error {
	if ctx.node.TopLeft == nil || *ctx.node.TopLeft == ctx.original {
		return ctx.Context.Err()
	}
	probe := &cancelWhenStackContains{
		Context:  ctx.Context,
		function: "newTransactionWithOptionsContext",
	}
	if err := probe.Err(); err != nil {
		ctx.observed = true
		if ctx.payload != nil {
			panic(ctx.payload)
		}
		return err
	}
	return ctx.Context.Err()
}

func TestBalanceSymmetryCancellationAfterCommitRestoresStage(t *testing.T) {
	graph := transactionBalanceSymmetryGraph(true)
	positions := captureBalanceSymmetryPositions(graph)
	ctx := &interruptBalanceSymmetryAfterFirstCommit{
		Context:  context.Background(),
		node:     graph.Nodes[0],
		original: *graph.Nodes[0].TopLeft,
	}
	err := BalanceSymmetry(ctx, graph)
	if !ctx.observed {
		t.Fatal("cancellation probe did not observe the first committed move")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BalanceSymmetry error = %v; want context.Canceled", err)
	}
	requireBalanceSymmetryPositions(t, positions)
}

func TestBalanceSymmetryPanicAfterCommitRestoresStage(t *testing.T) {
	graph := transactionBalanceSymmetryGraph(true)
	positions := captureBalanceSymmetryPositions(graph)
	payload := new(int)
	ctx := &interruptBalanceSymmetryAfterFirstCommit{
		Context:  context.Background(),
		node:     graph.Nodes[0],
		original: *graph.Nodes[0].TopLeft,
		payload:  payload,
	}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = BalanceSymmetry(ctx, graph)
	}()
	if !ctx.observed {
		t.Fatal("panic probe did not observe the first committed move")
	}
	if recovered != payload {
		t.Fatalf("panic payload = %v; want exact sentinel %v", recovered, payload)
	}
	requireBalanceSymmetryPositions(t, positions)
}

func TestBalanceSymmetrySuccessCommitsStage(t *testing.T) {
	graph := transactionBalanceSymmetryGraph(true)
	pointers := make(map[*layoutgraph.Node]*geo.Point, len(graph.Nodes))
	for _, node := range graph.Nodes {
		pointers[node] = node.TopLeft
	}
	require.NoError(t, BalanceSymmetry(t.Context(), graph))
	for _, node := range graph.Nodes {
		if node.TopLeft != pointers[node] {
			t.Fatalf("node %d TopLeft pointer changed", node.ID)
		}
	}
	require.Equal(t, 50.0, graph.Nodes[0].TopLeft.Y)
	require.Equal(t, 50.0, graph.Nodes[3].TopLeft.Y)
}
