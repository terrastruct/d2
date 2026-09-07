package placement

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestTransposeRejectsDiagonalSecondNeighbor(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.CellSize = 10
	center := layoutgraph.NewNode(1, 10, 10)
	center.TopLeft = geo.NewPoint(0, 0)
	orthogonal := layoutgraph.NewNode(2, 10, 10)
	orthogonal.TopLeft = geo.NewPoint(30, 0)
	diagonal := layoutgraph.NewNode(3, 10, 10)
	diagonal.TopLeft = geo.NewPoint(30, 30)
	other := layoutgraph.NewNode(4, 10, 10)
	other.TopLeft = geo.NewPoint(0, 30)
	for _, n := range []*layoutgraph.Node{center, orthogonal, diagonal, other} {
		g.AddNewNodeToContainer(nil, n)
	}
	// Edge order matters for the regression: the typo checked orthogonal twice
	// and never checked the second, diagonal neighbor.
	g.Connect(center, orthogonal)
	g.Connect(center, diagonal)
	g.Connect(orthogonal, other)

	if !center.Orientation(diagonal).IsDiagonal() {
		t.Fatal("test setup requires a diagonal second neighbor")
	}
	changed, err := transpose(context.Background(), g, center, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("transpose accepted a diagonal neighbor")
	}
	if center.TopLeft.X != 0 || center.TopLeft.Y != 0 {
		t.Fatalf("center moved to %v, want (0, 0)", center.TopLeft)
	}
}
