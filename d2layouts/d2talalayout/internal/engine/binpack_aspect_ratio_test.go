package engine

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
)

func TestBinPackPreservesSquareContainerAspectRatioWithRoutedEdges(t *testing.T) {
	config := normalizeFuzzerConfig(11, 0x17, 0x02, 'Q', 'n', '\r', 'B')
	graph := createRandomGraph(context.Background(), config)
	p := newPipeline(graph, config.seed, false)

	var finalBinPack func(*pipeline, context.Context) error
	binPackCount := 0
	for stageIndex, stage := range p.stagePlan() {
		if stage.name == "BinPack" {
			binPackCount++
			if binPackCount == 2 {
				finalBinPack = stage.run
				break
			}
		}
		if stageIndex == 0 {
			grouping.ResetClusters(p.graph)
			for _, edge := range p.graph.Edges {
				edge.Points = nil
			}
		}
		if err := stage.run(p, context.Background()); err != nil {
			t.Fatalf("stage %d %s: %v", stageIndex, stage.name, err)
		}
		node := nodeByID(p.graph, 4)
		if node == nil {
			t.Fatalf("stage %d %s lost node 4", stageIndex, stage.name)
		}
		if node.AspectRatio1() && node.Width != node.Height {
			t.Fatalf("square-shape violation before final binPack after stage %d %s: %.1fx%.1f", stageIndex, stage.name, node.Width, node.Height)
		}
	}

	node := nodeByID(p.graph, 4)
	if node == nil || !node.IsContainer() || node.ShapeType() != "Circle" {
		t.Fatalf("regression fixture node 4 = %#v; want a circle container", node)
	}
	if len(node.Edges) == 0 || !allEdgesHaveCompleteRoutes(node.Edges) {
		t.Fatal("regression fixture did not reach routed-container binPack path")
	}
	if finalBinPack == nil {
		t.Fatal("pipeline has no second binPack stage")
	}
	if err := finalBinPack(p, context.Background()); err != nil {
		t.Fatal(err)
	}
	if node.Width != node.Height {
		t.Fatalf("circle container after routed binPack = %.1fx%.1f; want square", node.Width, node.Height)
	}
}
