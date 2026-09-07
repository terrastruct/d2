package loops

import (
	"math"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestEdgesInOrderPreservesEqualLabelOrder(t *testing.T) {
	node := layoutgraph.NewNode(1, 10, 10)
	newLabeledEdge := func(width, height float64) *layoutgraph.Edge {
		edge := layoutgraph.NewEdge(node, node)
		edge.Label = &layoutgraph.Label{Width: width, Height: height}
		return edge
	}

	firstEqual := newLabeledEdge(10, 10)
	larger := newLabeledEdge(20, 10)
	secondEqual := newLabeledEdge(20, 5)
	smaller := newLabeledEdge(5, 10)
	input := []*layoutgraph.Edge{firstEqual, larger, secondEqual, smaller}

	got := edgesInOrder(input)
	want := []*layoutgraph.Edge{smaller, firstEqual, secondEqual, larger}
	if !slices.Equal(got, want) {
		t.Fatalf("edge order = %v; want %v", got, want)
	}
	if !slices.Equal(input, []*layoutgraph.Edge{firstEqual, larger, secondEqual, smaller}) {
		t.Fatal("edgesInOrder mutated its input")
	}
}

func TestEdgesInOrderPreservesOneSidedArrowOrder(t *testing.T) {
	node := layoutgraph.NewNode(1, 10, 10)
	targetOnly := layoutgraph.NewEdge(node, node)
	targetOnly.TargetArrowhead = layoutgraph.TriangleArrowhead
	sourceOnly := layoutgraph.NewEdge(node, node)
	sourceOnly.SourceArrowhead = layoutgraph.TriangleArrowhead
	plain := layoutgraph.NewEdge(node, node)

	got := edgesInOrder([]*layoutgraph.Edge{targetOnly, plain, sourceOnly})
	want := []*layoutgraph.Edge{targetOnly, sourceOnly, plain}
	if !slices.Equal(got, want) {
		t.Fatalf("edge order = %v; want %v", got, want)
	}
}

func TestEdgesInOrderPreservesNaNLabelTie(t *testing.T) {
	node := layoutgraph.NewNode(1, 10, 10)
	nonFinite := layoutgraph.NewEdge(node, node)
	nonFinite.Label = &layoutgraph.Label{Width: math.NaN(), Height: 10}
	finite := layoutgraph.NewEdge(node, node)
	finite.Label = &layoutgraph.Label{Width: 10, Height: 10}

	got := edgesInOrder([]*layoutgraph.Edge{nonFinite, finite})
	if got[0] != nonFinite || got[1] != finite {
		t.Fatal("NaN label area no longer remains equivalent to a finite area")
	}
}
