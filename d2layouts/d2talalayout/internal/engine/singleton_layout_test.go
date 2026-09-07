package engine

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestLayoutPreservesFixedSingletonPosition(t *testing.T) {
	positions := []geo.Point{
		{X: 137, Y: 241},
		{X: -137, Y: -241},
	}
	for _, position := range positions {
		name := "positive"
		if position.X < 0 {
			name = "negative"
		}
		t.Run(name, func(t *testing.T) {
			graph := layoutgraph.NewGraph()
			inputNode := layoutgraph.NewNode(1, 40, 40)
			inputNode.TopLeft = position.Copy()
			inputNode.FixedTopLeft = position.Copy()
			graph.AddNewNodeToContainer(nil, inputNode)

			result, err := Layout(context.Background(), graph, LayoutOptions{Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			node := result.Nodes[0]
			if *node.TopLeft != position {
				t.Fatalf("fixed singleton TopLeft = %v, want %v", node.TopLeft, position)
			}
			if *node.FixedTopLeft != position {
				t.Fatalf("fixed singleton constraint changed to %v, want %v", node.FixedTopLeft, position)
			}
			if node.TopLeft == node.FixedTopLeft {
				t.Fatal("fixed singleton position aliases its immutable constraint")
			}
			if *inputNode.TopLeft != position || *inputNode.FixedTopLeft != position {
				t.Fatal("Layout mutated its singleton input")
			}
		})
	}
}
