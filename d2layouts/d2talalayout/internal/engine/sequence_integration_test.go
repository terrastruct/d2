package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/shape"
)

func TestPreprocessSequenceStageNormalizesStepSizes(t *testing.T) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 20, 10)
	second := layoutgraph.NewNode(2, 40, 30)
	first.SetShape(shape.STEP_TYPE)
	second.SetShape(shape.STEP_TYPE)
	graph.AddNewNodeToContainer(nil, first)
	graph.AddNewNodeToContainer(nil, second)
	graph.Connect(first, second)

	pipeline := newPipeline(graph, 1, false)
	if err := pipeline.preprocessSequenceStage(withTestLogger(context.Background(), t)); err != nil {
		t.Fatal(err)
	}
	if first.Width != 2*shape.STEP_WEDGE_WIDTH || first.Height != 30 || second.Width != 40 || second.Height != 30 {
		t.Fatalf("normalized sequence step sizes = first=%gx%g second=%gx%g", first.Width, first.Height, second.Width, second.Height)
	}
}

func TestSequenceSerde(t *testing.T) {
	ctx := withTestLogger(t.Context(), t)
	graph := layoutgraph.NewGraph()
	step := layoutgraph.NewNode(1, 100, 100)
	step.SetShape(shape.STEP_TYPE)
	nextStep := layoutgraph.NewNode(2, 100, 100)
	nextStep.SetShape(shape.STEP_TYPE)
	circle := layoutgraph.NewNode(3, 100, 100)
	circle.SetShape(shape.CIRCLE_TYPE)
	graph.AddNodeUnchecked(step)
	graph.AddNodeUnchecked(nextStep)
	graph.AddNodeUnchecked(circle)
	graph.Connect(step, circle)
	graph.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{nil: {step, nextStep, circle}}

	pipeline := newPipeline(graph, 1, false)
	if err := pipeline.runAllStages(ctx); err != nil {
		t.Fatal(err)
	}
	serialized, err := graphjson.Serialize(ctx, pipeline.graph)
	if err != nil {
		t.Fatal(err)
	}
	deserialized := layoutgraph.NewGraph()
	if err := graphjson.Deserialize(ctx, serialized, deserialized); err != nil {
		t.Fatal(err)
	}
	requireGraphsSerializeEqual(ctx, t, deserialized, pipeline.graph)
}

func TestSequenceLayoutCanRunTwice(t *testing.T) {
	ctx := withTestLogger(t.Context(), t)
	graph, err := readGraph(ctx, filepath.Join(layoutTestDir, "sequence", "graph.input.json"))
	if err != nil {
		t.Fatal(err)
	}
	graph, err = Layout(ctx, graph, LayoutOptions{Seed: talaSeed})
	if err != nil {
		t.Fatalf("first layout failed: %v", err)
	}
	first, err := layoutgraph.Clone(ctx, graph)
	if err != nil {
		t.Fatal(err)
	}
	graph, err = Layout(ctx, graph, LayoutOptions{Seed: talaSeed})
	if err != nil {
		t.Fatalf("second layout failed: %v", err)
	}
	requireGraphsSerializeEqual(ctx, t, first, graph)
}
