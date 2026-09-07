package packing

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func packWithWorkLimit(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, workLimit int64) error {
	guard, err := newWorkGuard(ctx, workLimit)
	if err != nil {
		return err
	}
	return packAtomic(ctx, g, root, guard)
}
