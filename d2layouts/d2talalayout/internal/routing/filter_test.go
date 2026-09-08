package routing

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestFilterEdgeAncestorsOwnsRoutingObstaclePolicy(t *testing.T) {
	graph := layoutgraph.NewGraph()
	ancestor := layoutgraph.NewNode(1, 100, 100)
	from := layoutgraph.NewNode(2, 10, 10)
	to := layoutgraph.NewNode(3, 10, 10)
	unrelated := layoutgraph.NewNode(4, 10, 10)
	graph.AddNewNodeToContainer(nil, ancestor)
	graph.AddNewNodeToContainer(ancestor, from)
	graph.AddNewNodeToContainer(nil, to)
	graph.AddNewNodeToContainer(nil, unrelated)
	edge := graph.Connect(from, to)
	nodes := layoutgraph.Nodes{ancestor, from, to, unrelated}

	want := layoutgraph.Nodes{from, to, unrelated}
	if got := filterEdgeAncestors(edge, nodes); !slices.Equal(got, want) {
		t.Fatalf("filtered nodes = %v, want %v", got, want)
	}

	guard, err := newRouteWorkGuard(context.Background(), "ancestor filter", uint64(len(nodes)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := filterEdgeAncestorsGuarded(edge, nodes, guard)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("guarded filtered nodes = %v, want %v", got, want)
	}

	limited, err := newRouteWorkGuard(context.Background(), "ancestor filter", uint64(len(nodes)-1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := filterEdgeAncestorsGuarded(edge, nodes, limited); !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("limited ancestor filter error = %v, want route-stage work limit", err)
	}
}
