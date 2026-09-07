package d2talalayout_test

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2graph"

	"github.com/d2lang/d2/d2layouts/d2talalayout"
)

func TestZeroValueOptionsUseDefaults(t *testing.T) {
	graph := d2graph.NewGraph()
	if err := d2talalayout.Layout(context.Background(), graph, &d2talalayout.Options{}); err != nil {
		t.Fatal(err)
	}
}

// Compile-time proof of the same layout adapter boundary used by the other
// packages under d2layouts.
var (
	_ func() d2talalayout.Options                                        = d2talalayout.DefaultOptions
	_ func(context.Context, *d2graph.Graph) error                        = d2talalayout.DefaultLayout
	_ func(context.Context, *d2graph.Graph, *d2talalayout.Options) error = d2talalayout.Layout
	_ func(context.Context, *d2graph.Graph, []*d2graph.Edge) error       = d2talalayout.RouteEdges
)
