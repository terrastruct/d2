package engine

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func readGraph(ctx context.Context, filename string) (*layoutgraph.Graph, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var graph layoutgraph.Graph
	if err := graphjson.Unmarshal(ctx, data, &graph); err != nil {
		return nil, err
	}
	return &graph, nil
}

func nodeByID(g *layoutgraph.Graph, id layoutgraph.EntityID) *layoutgraph.Node {
	for _, node := range g.Nodes {
		if node.ID == id {
			return node
		}
	}
	return nil
}

func requireGraphsSerializeEqual(ctx context.Context, t testing.TB, left, right *layoutgraph.Graph) {
	t.Helper()

	leftJSON, err := graphjson.Marshal(ctx, left)
	if err != nil {
		t.Fatalf("serialize left graph: %v", err)
	}
	rightJSON, err := graphjson.Marshal(ctx, right)
	if err != nil {
		t.Fatalf("serialize right graph: %v", err)
	}
	if !bytes.Equal(leftJSON, rightJSON) {
		t.Fatalf(
			"serialized graphs first differ at byte %d (left length %d, right length %d)",
			firstDifferentByte(leftJSON, rightJSON), len(leftJSON), len(rightJSON),
		)
	}
}

func firstDifferentByte(left, right []byte) int {
	limit := min(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}
