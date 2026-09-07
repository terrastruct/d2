package packing

import (
	"context"
	"math"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func legacyBinPackScoreForParity(nodes layoutgraph.Nodes, root *layoutgraph.Node) float64 {
	topLeft, bottomRight := nodes.FixedBoundingBox()
	width := bottomRight.X - topLeft.X
	height := bottomRight.Y - topLeft.Y
	desiredAxisPenalty := 0.
	if root != nil {
		if root.DesiredWidth != nil && root.DesiredHeight == nil && width < *root.DesiredWidth {
			desiredAxisPenalty = *root.DesiredWidth - width
		} else if root.DesiredWidth == nil && root.DesiredHeight != nil && height < *root.DesiredHeight {
			desiredAxisPenalty = *root.DesiredHeight - height
		}
	}
	return width*height + math.Pow(width-height, 2)*subgraphSquareDampener +
		math.Pow(desiredAxisPenalty, 2)*subgraphSquareDampener
}

func TestBinPackScoreGuardedMatchesLegacyPolicy(t *testing.T) {
	n1 := layoutgraph.NewNode(1, 80, 20)
	n1.TopLeft = geo.NewPoint(0, 0)
	n2 := layoutgraph.NewNode(2, 20, 30)
	n2.TopLeft = geo.NewPoint(90, 40)
	nodes := layoutgraph.Nodes{n1, n2}

	tests := []struct {
		name string
		root *layoutgraph.Node
	}{
		{name: "no desired axis"},
		{name: "desired width", root: layoutgraph.NewNode(10, 1, 1)},
		{name: "desired height", root: layoutgraph.NewNode(11, 1, 1)},
	}
	desiredWidth := 300.
	tests[1].root.DesiredWidth = &desiredWidth
	desiredHeight := 250.
	tests[2].root.DesiredHeight = &desiredHeight

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			guard, err := newWorkGuard(context.Background(), 1_000_000_000)
			if err != nil {
				t.Fatal(err)
			}
			got, err := binPackScoreGuarded(nodes, test.root, guard)
			if err != nil {
				t.Fatal(err)
			}
			want := legacyBinPackScoreForParity(nodes, test.root)
			if got != want {
				t.Fatalf("guarded score = %v, want legacy %v", got, want)
			}
		})
	}
}
