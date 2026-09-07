package d2talalayout

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2graph"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/engine"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// seedInput is an isolated TALA layout input with the D2 bindings needed to
// apply the selected result.
type seedInput struct {
	graph    *layoutgraph.Graph
	bindings translation
}

// newSeedInput translates a D2 graph into an isolated TALA input. It reads but
// never mutates graph.
func newSeedInput(ctx context.Context, graph *d2graph.Graph) (seedInput, error) {
	if err := ctx.Err(); err != nil {
		return seedInput{}, err
	}
	internal := layoutgraph.NewGraph()
	bindings, err := translateGraph(ctx, graph, internal, false)
	if err != nil {
		return seedInput{}, err
	}
	return seedInput{graph: internal, bindings: bindings}, nil
}

// runSeed runs one TALA variation in a new workspace. Input remains unchanged
// and may be reused concurrently by other attempts.
func runSeed(ctx context.Context, input seedInput, seed int64) (*layoutgraph.Graph, error) {
	if input.graph == nil {
		return nil, fmt.Errorf("TALA seed graph is empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return engine.Layout(ctx, input.graph, engine.LayoutOptions{
		Seed: seed,
	})
}
