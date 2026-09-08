package trees

import (
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestTreeEdgeDirectionOwnsPolicy(t *testing.T) {
	from := layoutgraph.NewNode(1, 10, 10)
	to := layoutgraph.NewNode(2, 10, 10)
	edge := layoutgraph.NewEdge(from, to)

	if got := treeEdgeDirection(from, edge); got != Undirected {
		t.Fatalf("undirected direction = %v, want %v", got, Undirected)
	}
	edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	if got := treeEdgeDirection(from, edge); got != Outwards {
		t.Fatalf("outward direction = %v, want %v", got, Outwards)
	}
	if got := treeEdgeDirection(to, edge); got != Inwards {
		t.Fatalf("inward direction = %v, want %v", got, Inwards)
	}
	edge.SourceArrowhead = layoutgraph.TriangleArrowhead
	if got := treeEdgeDirection(from, edge); got != Bidirectional {
		t.Fatalf("bidirectional direction = %v, want %v", got, Bidirectional)
	}
}
