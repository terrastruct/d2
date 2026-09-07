package packing

import (
	"context"
	"fmt"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type cancelAfterErrChecks struct {
	context.Context
	remaining int
}

func (ctx *cancelAfterErrChecks) Err() error {
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return ctx.Context.Err()
}

func blocksRoutes(ctx context.Context, g *layoutgraph.Graph, packing layoutgraph.Nodes, packed []layoutgraph.Nodes, packedSegments []*geo.Segment) (bool, error) {
	guard, err := limits.NewWorkGuard(ctx, "BinPackRoutes", limits.MaxEngineWorkUnits)
	if err != nil {
		return false, err
	}
	return blocksRoutesGuarded(g, packing, packed, packedSegments, guard)
}

func blocksRoutesGuarded(g *layoutgraph.Graph, packing layoutgraph.Nodes, packed []layoutgraph.Nodes, packedSegments []*geo.Segment, guard *limits.WorkGuard) (bool, error) {
	if guard == nil {
		return false, fmt.Errorf("TALA BinPack route check requires a work guard")
	}
	xd, yd, err := binPackSmallestDeltas(packed, guard)
	if err != nil {
		return false, err
	}
	return blocksRoutesWithDeltasGuarded(g, packing, packed, packedSegments, xd, yd, guard)
}

func TestBlocksRoutesMidLoopCancellation(t *testing.T) {
	graph := layoutgraph.NewGraph()
	packing := make(layoutgraph.Nodes, 0, 130)
	for index := 0; index < 130; index++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(index+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(index*20), 0)
		graph.AddNodeUnchecked(node)
		packing = append(packing, node)
	}
	packedNode := layoutgraph.NewNode(1_000, 10, 10)
	packedNode.TopLeft = geo.NewPoint(0, 100)

	_, err := blocksRoutes(
		&cancelAfterErrChecks{Context: context.Background(), remaining: 1},
		graph,
		packing,
		[]layoutgraph.Nodes{{packedNode}},
		nil,
	)
	requirePackingCanceledAt(t, err, "BinPackRoutes")
}
