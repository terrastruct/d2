package placement

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type cancelAfterNearOwnerMutation struct {
	context.Context
	node               *layoutgraph.Node
	original           *layoutgraph.Graph
	mutationChecks     int
	panicAfterMutation any
}

func (ctx *cancelAfterNearOwnerMutation) Err() error {
	if ctx.node.Graph == ctx.original {
		return ctx.Context.Err()
	}
	ctx.mutationChecks++
	// SplitSubgraphs finishes its own atomic operation after assigning the
	// temporary owner. Let that check pass, then fail the caller's next check so
	// this exercises placement's post-split rollback rather than split rollback.
	if ctx.mutationChecks == 1 {
		return nil
	}
	if ctx.panicAfterMutation != nil {
		panic(ctx.panicAfterMutation)
	}
	return context.Canceled
}

func placementGraphWithExternalNear() (graph *layoutgraph.Graph, external *layoutgraph.Node, owner *layoutgraph.Graph) {
	graph = layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	first.TopLeft = geo.NewPoint(0, 0)
	second := layoutgraph.NewNode(2, 10, 10)
	second.TopLeft = geo.NewPoint(40, 0)
	graph.AddNewNodeToContainer(nil, first)
	graph.AddNewNodeToContainer(nil, second)

	owner = layoutgraph.NewGraph()
	external = layoutgraph.NewNode(3, 10, 10)
	external.TopLeft = geo.NewPoint(100, 0)
	owner.AddNewNodeToContainer(nil, external)
	first.Nears[external] = struct{}{}
	return graph, external, owner
}

func TestPlaceNodesCancellationRestoresEdgeAbductions(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 100, 100)
	nested := layoutgraph.NewNode(2, 10, 10)
	sibling := layoutgraph.NewNode(3, 10, 10)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, nested)
	graph.AddNewNodeToContainer(nil, sibling)
	edge := graph.Connect(nested, sibling)
	wantEdges := make(map[*layoutgraph.Node][]*layoutgraph.Edge, len(graph.Nodes))
	for _, node := range graph.Nodes {
		wantEdges[node] = append([]*layoutgraph.Edge(nil), node.Edges...)
	}
	err := placeNodes(&cancelAfterErrChecks{Context: context.Background()}, graph, nil, 1, nil, nil)
	requireCanceledAt(t, err, "PlaceChildrenOrder")
	if edge.From != nested || edge.To != sibling {
		t.Fatalf("edge endpoints after cancellation = %d -> %d; want %d -> %d", edge.From.ID, edge.To.ID, nested.ID, sibling.ID)
	}
	for _, node := range graph.Nodes {
		if node.Graph != graph {
			t.Fatalf("node %d graph was not restored", node.ID)
		}
		want := wantEdges[node]
		if len(node.Edges) != len(want) {
			t.Fatalf("node %d edges after cancellation = %v; want %v", node.ID, node.Edges, want)
		}
		for index := range want {
			if node.Edges[index] != want[index] {
				t.Fatalf("node %d edge %d was not restored", node.ID, index)
			}
		}
	}
}

func TestPlaceNodesPostSplitFailureRestoresNearOnlyExternalOwner(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		graph, external, owner := placementGraphWithExternalNear()
		ctx := &cancelAfterNearOwnerMutation{
			Context:  context.Background(),
			node:     external,
			original: owner,
		}

		err := placeNodes(ctx, graph, nil, 1, nil, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PlaceNodes error = %v, want context cancellation", err)
		}
		if ctx.mutationChecks < 2 {
			t.Fatal("placement did not fail after SplitSubgraphs changed the Near-only owner")
		}
		if external.Graph != owner {
			t.Fatalf("Near-only node owner = %p, want exact original %p", external.Graph, owner)
		}
	})

	t.Run("panic", func(t *testing.T) {
		graph, external, owner := placementGraphWithExternalNear()
		panicValue := &struct{ marker string }{marker: "post-split placement panic"}
		ctx := &cancelAfterNearOwnerMutation{
			Context:            context.Background(),
			node:               external,
			original:           owner,
			panicAfterMutation: panicValue,
		}

		var recovered any
		func() {
			defer func() { recovered = recover() }()
			_ = placeNodes(ctx, graph, nil, 1, nil, nil)
		}()
		if recovered != panicValue {
			t.Fatalf("recovered panic = %#v, want exact payload %#v", recovered, panicValue)
		}
		if ctx.mutationChecks < 2 {
			t.Fatal("placement did not panic after SplitSubgraphs changed the Near-only owner")
		}
		if external.Graph != owner {
			t.Fatalf("Near-only node owner = %p, want exact original %p", external.Graph, owner)
		}
	})
}

func TestOptimizationHelpersCanceledBeforeWork(t *testing.T) {
	ctx := canceledContext()
	graph := layoutgraph.NewGraph()
	if err := direct(ctx, graph, nil, nil, directOptions{checkEdgeLength: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("direct error = %v, want context.Canceled", err)
	}
	txn := mustNewTransaction(t, graph, layoutgraph.TransactionOptions{})
	if _, _, _, err := tryMove(ctx, txn, graph, nil, nil, nil, 1, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("tryMove error = %v, want context.Canceled", err)
	}
}
