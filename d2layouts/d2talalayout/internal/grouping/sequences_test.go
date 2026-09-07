package grouping

import (
	"context"
	"math/rand"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/shape"
)

func mustIdentifySequences(t *testing.T, graph *layoutgraph.Graph, nodes []*layoutgraph.Node) [][]*layoutgraph.Node {
	t.Helper()
	guard, err := limits.NewWorkGuard(t.Context(), "IdentifySequences", limits.MaxEngineWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	sequences, err := identifySequences(graph, nodes, guard)
	if err != nil {
		t.Fatal(err)
	}
	return sequences
}

func newStepSequenceGraph() (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Node) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 20, 10)
	second := layoutgraph.NewNode(2, 40, 30)
	first.SetShape(shape.STEP_TYPE)
	second.SetShape(shape.STEP_TYPE)
	graph.AddNewNodeToContainer(nil, first)
	graph.AddNewNodeToContainer(nil, second)
	graph.Connect(first, second)
	return graph, first, second
}

func TestSequenceCanBeRebuiltAfterCleanup(t *testing.T) {
	graph, first, second := newStepSequenceGraph()
	random := rand.New(rand.NewSource(1))
	if err := AddSequences(context.Background(), graph, random); err != nil {
		t.Fatal(err)
	}
	var firstVesselID layoutgraph.EntityID
	for vessel := range graph.Sequences {
		firstVesselID = vessel.ID
	}
	Cleanup(graph)
	if first.ConnectionTo(second) != nil {
		t.Fatal("sequence-defining edge survived cleanup")
	}
	if err := AddSequences(context.Background(), graph, rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}
	if len(graph.Sequences) != 1 {
		t.Fatalf("sequence count after rebuild = %d, want 1", len(graph.Sequences))
	}
	for vessel := range graph.Sequences {
		if vessel.ID != firstVesselID {
			t.Fatalf("rebuilt vessel ID = %d, want %d", vessel.ID, firstVesselID)
		}
	}
}

func TestIdentifySequencesSkipsRememberedInactiveSteps(t *testing.T) {
	graph, first, second := newStepSequenceGraph()
	remembered := &layoutgraph.Sequence{
		Vessel: layoutgraph.NewNode(3, 0, 0), Nodes: []*layoutgraph.Node{first, second}, Graph: graph,
	}
	first.Sequence = remembered
	second.Sequence = remembered
	if got := mustIdentifySequences(t, graph, []*layoutgraph.Node{first, second}); len(got) != 0 {
		t.Fatalf("remembered steps rediscovered as %d sequences", len(got))
	}
}
