package packing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func packingCanceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func requirePackingCanceledAt(t *testing.T, err error, location string) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(%v, context.Canceled) = false", err)
	}
	if !strings.Contains(err.Error(), location) {
		t.Fatalf("cancellation error = %v, want operation %q", err, location)
	}
}

type cancelWhenNodeMoved struct {
	context.Context
	node *layoutgraph.Node
}

func (ctx *cancelWhenNodeMoved) Err() error {
	if ctx.node.TopLeft.X >= 100000 || ctx.node.TopLeft.Y >= 100000 {
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestBinPackCanceled(t *testing.T) {
	err := Pack(packingCanceledContext(), layoutgraph.NewGraph(), nil)
	requirePackingCanceledAt(t, err, "BinPack")
}

func TestBinPackCancellationRestoresState(t *testing.T) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	first.TopLeft = geo.NewPoint(0, 0)
	second := layoutgraph.NewNode(2, 10, 10)
	second.TopLeft = geo.NewPoint(100, 100)
	graph.AddNewNodeToContainer(nil, first)
	graph.AddNewNodeToContainer(nil, second)
	graph.ComputeCellSize()
	firstOriginal := first.TopLeft.Copy()
	secondOriginal := second.TopLeft.Copy()
	firstPointer := first.TopLeft
	secondPointer := second.TopLeft
	err := Pack(&cancelWhenNodeMoved{Context: context.Background(), node: second}, graph, nil)
	requirePackingCanceledAt(t, err, "BinPack")
	if !first.TopLeft.Equals(firstOriginal) || !second.TopLeft.Equals(secondOriginal) {
		t.Fatal("BinPack cancellation did not restore node positions")
	}
	if first.TopLeft != firstPointer || second.TopLeft != secondPointer {
		t.Fatal("BinPack cancellation replaced a node position pointer")
	}
}
