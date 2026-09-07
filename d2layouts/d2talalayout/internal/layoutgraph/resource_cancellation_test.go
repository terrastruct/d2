package layoutgraph

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func TestSplitSubgraphsMidLoopCancellationRestoresGraphReferences(t *testing.T) {
	g := NewGraph()
	for i := 0; i < 130; i++ {
		node := NewNode(EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i*20), 0)
		g.AddNodeUnchecked(node)
	}

	_, err := g.SplitSubgraphs(
		&cancelAfterErrChecks{Context: context.Background(), remaining: 1},
		SplitOptions{},
	)
	requireCanceledAt(t, err, "SplitSubgraphs")
	for _, node := range g.Nodes {
		if node.Graph != g {
			t.Fatalf("node %d graph = %p, want original graph %p", node.ID, node.Graph, g)
		}
	}
}

func TestSplitSubgraphsPreservesDuplicateEdgeDeduplication(t *testing.T) {
	g := NewGraph()
	from := NewNode(1, 10, 10)
	from.TopLeft = geo.NewPoint(0, 0)
	to := NewNode(2, 10, 10)
	to.TopLeft = geo.NewPoint(20, 0)
	g.AddNodeUnchecked(from)
	g.AddNodeUnchecked(to)
	edge := g.Connect(from, to)
	g.Edges = append(g.Edges, edge)

	graphs, err := g.SplitSubgraphs(context.Background(), SplitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(graphs) != 1 {
		t.Fatalf("split graph count = %d, want 1", len(graphs))
	}
	if len(graphs[0].Edges) != 1 || graphs[0].Edges[0] != edge {
		t.Fatalf("split edges = %v, want one copy of %p", graphs[0].Edges, edge)
	}
}

func TestEngineGraphAndWorkBounds(t *testing.T) {
	g := NewGraph()
	g.Nodes = make([]*Node, maxEngineNodes+1)
	for i := range g.Nodes {
		g.Nodes[i] = NewNode(EntityID(i+1), 1, 1)
	}
	if _, err := g.SplitSubgraphs(context.Background(), SplitOptions{}); err == nil || !strings.Contains(err.Error(), "node count") {
		t.Fatalf("node-limit error = %v", err)
	}

	guard, err := limits.NewWorkGuard(context.Background(), "test", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("work-limit error = %v", err)
	}
}
