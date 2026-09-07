package placement

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func TestVisibilityGraphMidBlockerScanCancellation(t *testing.T) {
	graph := layoutgraph.NewGraph()
	for index := 0; index < 20; index++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(index+1), 5, 5)
		node.TopLeft = geo.NewPoint(float64(index*10), 0)
		graph.AddNodeUnchecked(node)
	}

	_, err := visibilityEdges(
		&cancelAfterErrChecks{Context: context.Background(), remaining: 1},
		graph,
		true,
		true,
	)
	requireCanceledAt(t, err, "CompactionVisibility")
}

func TestCompactionCandidateGenerationCancellationAndBound(t *testing.T) {
	graph := layoutgraph.NewGraph()
	anchor := layoutgraph.NewNode(1, 1, 1)
	anchor.TopLeft = geo.NewPoint(0, 0)
	graph.AddNodeUnchecked(anchor)
	node := layoutgraph.NewNode(2, 1, 1)
	node.TopLeft = geo.NewPoint(200, 0)
	graph.AddNodeUnchecked(node)
	visibilityEdges := layoutgraph.Edges{layoutgraph.NewEdge(anchor, node)}

	_, err := candidateMoves(
		&cancelAfterErrChecks{Context: context.Background(), remaining: 1},
		graph,
		node,
		1,
		true,
		false,
		0,
		visibilityEdges,
	)
	requireCanceledAt(t, err, "CompactionCandidates")

	node.TopLeft.X = limits.MaxGraphSize + 100
	_, err = candidateMoves(context.Background(), graph, node, 1, true, false, 0, visibilityEdges)
	if err == nil || !strings.Contains(err.Error(), "compaction candidate count") {
		t.Fatalf("oversized candidate range error = %v", err)
	}
}
