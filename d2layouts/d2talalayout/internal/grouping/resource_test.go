package grouping

import (
	"context"
	"math/rand"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type cancelWhenSequencesCleared struct {
	context.Context
	graph *layoutgraph.Graph
}

func (ctx *cancelWhenSequencesCleared) Err() error {
	if len(ctx.graph.Sequences) == 0 {
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestAddSequencesMidLoopCancellationRestoresRememberedTopology(t *testing.T) {
	graph := layoutgraph.NewGraph()
	for index := 0; index < 130; index++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(index+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(index*20), 0)
		graph.AddNewNodeToContainer(nil, node)
	}
	vessel := layoutgraph.NewNode(1_000, 10, 10)
	sequence := &layoutgraph.Sequence{
		Vessel: vessel,
		Nodes:  []*layoutgraph.Node{graph.Nodes[0]},
		Graph:  graph,
	}
	graph.Nodes[0].Sequence = sequence
	graph.Sequences[vessel] = sequence
	originalSequences := reflect.ValueOf(graph.Sequences).Pointer()

	err := AddSequences(
		&cancelWhenSequencesCleared{Context: context.Background(), graph: graph},
		graph,
		rand.New(rand.NewSource(1)),
	)
	requireCanceledAt(t, err, "AddSequences")
	if reflect.ValueOf(graph.Sequences).Pointer() != originalSequences || graph.Sequences[vessel] != sequence {
		t.Fatal("AddSequences did not restore the original sequence map")
	}
	if graph.Nodes[0].Sequence != sequence {
		t.Fatal("AddSequences did not restore remembered sequence membership")
	}
}
