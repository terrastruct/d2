package trees

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestPlaceAtOrientationCanceledBeforeWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	guard, err := newWorkGuard(ctx, "PlaceAtOrientation")
	if err != nil {
		t.Fatal(err)
	}
	g := layoutgraph.NewGraph()
	node := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	node.TopLeft = geo.NewPoint(0, 0)
	tree := layoutgraph.NewTree(node)
	cancel()

	if _, err := placeAtOrientationGuarded(ctx, g, tree, geo.Left, guard); !errors.Is(err, context.Canceled) {
		t.Fatalf("placeAtOrientationGuarded error = %v, want context.Canceled", err)
	}
}
