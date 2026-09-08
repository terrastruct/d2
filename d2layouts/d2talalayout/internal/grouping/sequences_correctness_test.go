package grouping

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/shape"
)

type cancelWhenSequenceInstalled struct {
	context.Context
	graph *layoutgraph.Graph
}

func (ctx *cancelWhenSequenceInstalled) Err() error {
	if len(ctx.graph.Sequences) > 0 {
		return context.Canceled
	}
	return ctx.Context.Err()
}

type countContextChecks struct {
	context.Context
	checks int
}

func (ctx *countContextChecks) Err() error {
	ctx.checks++
	return ctx.Context.Err()
}

type cancelBeforeSequenceMutation struct {
	context.Context
	graph                *layoutgraph.Graph
	originalSequencesMap uintptr
	cancelAt             int
	checks               int
	observedMutation     bool
}

func (ctx *cancelBeforeSequenceMutation) Err() error {
	ctx.checks++
	if ctx.checks < ctx.cancelAt {
		return nil
	}
	if reflect.ValueOf(ctx.graph.Sequences).Pointer() != ctx.originalSequencesMap {
		ctx.observedMutation = true
	}
	return context.Canceled
}

func TestAddSequencesSnapshotCancellationPrecedesMutation(t *testing.T) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 20, 10)
	second := layoutgraph.NewNode(2, 20, 10)
	first.SetShape(shape.STEP_TYPE)
	second.SetShape(shape.STEP_TYPE)
	graph.AddNewNodeToContainer(nil, first)
	graph.AddNewNodeToContainer(nil, second)
	graph.Connect(first, second)

	validationProbe := &countContextChecks{Context: context.Background()}
	if err := layoutgraph.Validate(validationProbe, "AddSequences", graph); err != nil {
		t.Fatal(err)
	}
	originalSequencesMap := reflect.ValueOf(graph.Sequences).Pointer()
	ctx := &cancelBeforeSequenceMutation{
		Context:              context.Background(),
		graph:                graph,
		originalSequencesMap: originalSequencesMap,
		// AddSequences creates its stage guard immediately after validation.
		// Cancel on the following check, which belongs to snapshot capture.
		cancelAt: validationProbe.checks + 2,
	}

	err := AddSequences(ctx, graph, rand.New(rand.NewSource(1)))
	requireCanceledAt(t, err, "AddSequences")
	if ctx.checks < ctx.cancelAt {
		t.Fatal("cancellation probe did not reach snapshot capture")
	}
	if ctx.observedMutation {
		t.Fatal("AddSequences mutated sequence state before observing snapshot cancellation")
	}
	if reflect.ValueOf(graph.Sequences).Pointer() != originalSequencesMap {
		t.Fatal("AddSequences cancellation replaced the original sequence map")
	}
}

func mustAddSequences(
	t *testing.T,
	ctx context.Context,
	g *layoutgraph.Graph,
	random *rand.Rand,
) {
	t.Helper()
	if err := AddSequences(ctx, g, random); err != nil {
		t.Fatal(err)
	}
}

func TestAddSequencesLateCancellationRestoresExactSliceAliases(t *testing.T) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 20, 10)
	second := layoutgraph.NewNode(2, 20, 10)
	first.SetShape(shape.STEP_TYPE)
	second.SetShape(shape.STEP_TYPE)
	first.TopLeft = geo.NewPoint(0, 0)
	second.TopLeft = geo.NewPoint(20, 0)
	graph.AddNewNodeToContainer(nil, first)
	graph.AddNewNodeToContainer(nil, second)
	edge := graph.Connect(first, second)

	tailNode := layoutgraph.NewNode(99, 1, 1)
	nodeBacking := make([]*layoutgraph.Node, 5)
	copy(nodeBacking, graph.Nodes)
	for index := len(graph.Nodes); index < len(nodeBacking); index++ {
		nodeBacking[index] = tailNode
	}
	graph.Nodes = nodeBacking[:len(graph.Nodes)]
	tailEdge := layoutgraph.NewEdge(tailNode, tailNode)
	edgeBacking := make([]*layoutgraph.Edge, 4)
	copy(edgeBacking, graph.Edges)
	for index := len(graph.Edges); index < len(edgeBacking); index++ {
		edgeBacking[index] = tailEdge
	}
	graph.Edges = edgeBacking[:len(graph.Edges)]
	containerBacking := make([]*layoutgraph.Node, 5)
	copy(containerBacking, graph.Containers[nil])
	for index := len(graph.Containers[nil]); index < len(containerBacking); index++ {
		containerBacking[index] = tailNode
	}
	graph.Containers[nil] = containerBacking[:len(graph.Containers[nil])]
	for _, node := range []*layoutgraph.Node{first, second} {
		backing := make([]*layoutgraph.Edge, 4)
		copy(backing, node.Edges)
		for index := len(node.Edges); index < len(backing); index++ {
			backing[index] = tailEdge
		}
		node.Edges = backing[:len(node.Edges)]
	}

	nodesBefore := captureExactTestSlice(graph.Nodes)
	edgesBefore := captureExactTestSlice(graph.Edges)
	containerBefore := captureExactTestSlice(graph.Containers[nil])
	firstEdgesBefore := captureExactTestSlice(first.Edges)
	secondEdgesBefore := captureExactTestSlice(second.Edges)
	sequencesMap := reflect.ValueOf(graph.Sequences).Pointer()
	err := AddSequences(
		&cancelWhenSequenceInstalled{Context: context.Background(), graph: graph},
		graph,
		rand.New(rand.NewSource(1)),
	)
	requireCanceledAt(t, err, "AddSequences")
	if reflect.ValueOf(graph.Sequences).Pointer() != sequencesMap || len(graph.Sequences) != 0 {
		t.Fatal("AddSequences cancellation did not restore the original sequence map")
	}
	nodesBefore.assertRestored(t, graph.Nodes, "Graph.Nodes")
	edgesBefore.assertRestored(t, graph.Edges, "Graph.Edges")
	containerBefore.assertRestored(t, graph.Containers[nil], "Graph.Containers[nil]")
	firstEdgesBefore.assertRestored(t, first.Edges, "first step Node.Edges")
	secondEdgesBefore.assertRestored(t, second.Edges, "second step Node.Edges")
	if graph.Edges[0] != edge || first.Sequence != nil || second.Sequence != nil {
		t.Fatal("AddSequences cancellation left sequence topology installed")
	}
}

func TestSequenceDefiningEdgesMidContainerTraversalCancellation(t *testing.T) {
	graph := layoutgraph.NewGraph()
	for index := 0; index < 130; index++ {
		container := layoutgraph.NewNode(layoutgraph.EntityID(index+1), 10, 10)
		container.TopLeft = geo.NewPoint(float64(index*20), 0)
		container.SetContainer(true)
		graph.AddNewNodeToContainer(nil, container)
	}
	nodesBefore := captureExactTestSlice(graph.Nodes)
	containersMap := reflect.ValueOf(graph.Containers).Pointer()

	edges, err := SequenceDefiningEdges(
		&cancelWhenStackContains{Context: context.Background(), function: "containerRDFSOrderContext"},
		graph,
	)
	if edges != nil {
		t.Fatalf("sequence edge result = %v, want nil after cancellation", edges)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SequenceDefiningEdges error = %v, want context.Canceled", err)
	}
	if reflect.ValueOf(graph.Containers).Pointer() != containersMap {
		t.Fatal("sequence edge discovery replaced the container map")
	}
	nodesBefore.assertRestored(t, graph.Nodes, "sequence discovery Graph.Nodes")
	for _, node := range graph.Nodes {
		if node.Graph != graph {
			t.Fatalf("node %d graph reference changed during sequence edge discovery", node.ID)
		}
	}
}

func TestBuildSequenceCanReuseRememberedSteps(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 40, 30))
	b := g.AddNode(layoutgraph.NewNode(2, 40, 30))
	g.Connect(a, b)

	buildSequence([]*layoutgraph.Node{a, b}, g, nil, 3)
	if edge := a.ConnectionTo(b); edge != nil {
		t.Fatal("expected the first sequence build to remove its defining edge")
	}
	// Layout output remembers sequence membership but intentionally omits its
	// defining edges. Rebuilding that remembered sequence must therefore accept
	// adjacent steps with no remaining connection.
	buildSequence([]*layoutgraph.Node{a, b}, g, nil, 4)
}

func TestIdentifySequencesDoesNotRediscoverRememberedSteps(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := g.AddNode(layoutgraph.NewNode(1, 40, 30))
	b := g.AddNode(layoutgraph.NewNode(2, 40, 30))
	a.SetShape(shape.STEP_TYPE)
	b.SetShape(shape.STEP_TYPE)
	g.Connect(a, b)

	remembered := &layoutgraph.Sequence{Vessel: layoutgraph.NewNode(3, 0, 0), Nodes: []*layoutgraph.Node{a, b}, Graph: g}
	a.Sequence = remembered
	b.Sequence = remembered
	if got := mustIdentifySequences(t, g, []*layoutgraph.Node{a, b}); len(got) != 0 {
		t.Fatalf("remembered sequence steps were rediscovered as %d new sequences", len(got))
	}
}

func TestAddSequencesDiscardsStaleRememberedMembership(t *testing.T) {
	newRememberedGraph := func(t *testing.T) (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Node) {
		t.Helper()
		g := layoutgraph.NewGraph()
		a := layoutgraph.NewNode(1, 40, 30)
		b := layoutgraph.NewNode(2, 40, 30)
		a.SetShape(shape.STEP_TYPE)
		b.SetShape(shape.STEP_TYPE)
		g.AddNewNodeToContainer(nil, a)
		g.AddNewNodeToContainer(nil, b)
		g.Connect(a, b)
		ctx := log.With(context.Background(), testlog.New(t))
		mustAddSequences(t, ctx, g, rand.New(rand.NewSource(1)))
		Cleanup(g)
		return g, a, b
	}

	run := func(t *testing.T, mutate func(*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Node)) {
		t.Helper()
		g, a, b := newRememberedGraph(t)
		mutate(g, a, b)
		mustAddSequences(t, log.With(context.Background(), testlog.New(t)), g, rand.New(rand.NewSource(1)))
		if len(g.Sequences) != 0 {
			t.Fatalf("stale remembered sequence was restored: %d sequences", len(g.Sequences))
		}
		if a.Sequence != nil || b.Sequence != nil {
			t.Fatalf("stale membership was retained: a=%p b=%p", a.Sequence, b.Sequence)
		}
	}

	t.Run("shape changed", func(t *testing.T) {
		run(t, func(_ *layoutgraph.Graph, _ *layoutgraph.Node, b *layoutgraph.Node) {
			b.SetShape(shape.SQUARE_TYPE)
		})
	})

	t.Run("container changed", func(t *testing.T) {
		run(t, func(g *layoutgraph.Graph, _ *layoutgraph.Node, b *layoutgraph.Node) {
			container := layoutgraph.NewNode(10, 100, 100)
			container.SetContainer(true)
			g.AddNewNodeToContainer(nil, container)
			children := g.Containers[nil][:0]
			for _, child := range g.Containers[nil] {
				if child != b {
					children = append(children, child)
				}
			}
			g.Containers[nil] = children
			g.AddNodeToContainer(container, b)
		})
	})

	t.Run("membership cleared", func(t *testing.T) {
		run(t, func(_ *layoutgraph.Graph, _ *layoutgraph.Node, b *layoutgraph.Node) {
			b.Sequence = nil
		})
	})

	t.Run("membership no longer contiguous", func(t *testing.T) {
		run(t, func(g *layoutgraph.Graph, a, b *layoutgraph.Node) {
			other := layoutgraph.NewNode(10, 20, 20)
			g.AddNodeUnchecked(other)
			g.Containers[nil] = []*layoutgraph.Node{a, other, b}
		})
	})
}

func TestAddSequencesDoesNotResurrectRemovedRememberedStep(t *testing.T) {
	g := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(1, 40, 30)
	b := layoutgraph.NewNode(2, 40, 30)
	a.SetShape(shape.STEP_TYPE)
	b.SetShape(shape.STEP_TYPE)
	g.AddNewNodeToContainer(nil, a)
	g.AddNewNodeToContainer(nil, b)
	g.Connect(a, b)
	ctx := log.With(context.Background(), testlog.New(t))
	mustAddSequences(t, ctx, g, rand.New(rand.NewSource(1)))
	Cleanup(g)

	// Leave the removed node in the container slice to ensure sequence discovery
	// itself cannot reintroduce a node that is absent from g.Nodes.
	g.RemoveNode(b)
	mustAddSequences(t, ctx, g, rand.New(rand.NewSource(1)))
	if len(g.Sequences) != 0 {
		t.Fatalf("removed remembered step produced %d sequences", len(g.Sequences))
	}
	for _, node := range g.Nodes {
		if node == b || node.ID == b.ID {
			t.Fatalf("removed step %d was resurrected", b.ID)
		}
	}
}

func TestAddSequencesReservesOrdinaryNodeIDs(t *testing.T) {
	const seed int64 = 19
	probe := rand.New(rand.NewSource(seed))
	collidingID := probe.Int63()

	g := layoutgraph.NewGraph()
	ordinary := layoutgraph.NewNode(collidingID, 40, 30)
	a := layoutgraph.NewNode(1, 40, 30)
	b := layoutgraph.NewNode(2, 40, 30)
	a.SetShape(shape.STEP_TYPE)
	b.SetShape(shape.STEP_TYPE)
	g.AddNewNodeToContainer(nil, ordinary)
	g.AddNewNodeToContainer(nil, a)
	g.AddNewNodeToContainer(nil, b)
	g.Connect(a, b)
	layoutRand := rand.New(rand.NewSource(seed))
	mustAddSequences(t, log.With(context.Background(), testlog.New(t)), g, layoutRand)

	if len(g.Sequences) != 1 {
		t.Fatalf("sequence count = %d, want 1", len(g.Sequences))
	}
	var vessel *layoutgraph.Node
	for vessel = range g.Sequences {
	}
	if vessel.ID == ordinary.ID {
		t.Fatalf("sequence vessel reused ordinary node ID %d", ordinary.ID)
	}
	if vessel.ID != collidingID+1 {
		t.Fatalf("resolved vessel ID = %d, want deterministic fallback %d", vessel.ID, collidingID+1)
	}
	if got, want := layoutRand.Int63(), probe.Int63(); got != want {
		t.Fatalf("collision resolution consumed extra RNG values: next draw = %d, want %d", got, want)
	}
	if _, err := graphjson.Serialize(t.Context(), g); err != nil {
		t.Fatalf("graph with resolved vessel ID does not serialize: %v", err)
	}
}

func TestAddSequencesReservesRememberedIDsAcrossContainers(t *testing.T) {
	ctx := log.With(t.Context(), testlog.New(t))
	g := layoutgraph.NewGraph()

	// containerRDFSOrder visits root children in reverse, so adding B before A
	// makes A the earlier sequence-ID allocation container.
	containerB := layoutgraph.NewNode(100, 100, 100)
	containerB.SetContainer(true)
	containerA := layoutgraph.NewNode(200, 100, 100)
	containerA.SetContainer(true)
	g.AddNewNodeToContainer(nil, containerB)
	g.AddNewNodeToContainer(nil, containerA)

	addSteps := func(container *layoutgraph.Node, firstID layoutgraph.EntityID) []*layoutgraph.Node {
		first := layoutgraph.NewNode(firstID, 40, 30)
		second := layoutgraph.NewNode(firstID+1, 40, 30)
		first.SetShape(shape.STEP_TYPE)
		second.SetShape(shape.STEP_TYPE)
		g.AddNewNodeToContainer(container, first)
		g.AddNewNodeToContainer(container, second)
		g.Connect(first, second)
		return []*layoutgraph.Node{first, second}
	}

	oldA := addSteps(containerA, 1)
	oldB := addSteps(containerB, 3)
	const seed int64 = 73
	mustAddSequences(t, ctx, g, rand.New(rand.NewSource(seed)))
	if len(g.Sequences) != 2 {
		t.Fatalf("initial sequence count = %d, want 2", len(g.Sequences))
	}
	oldAID := oldA[0].Sequence.Vessel.ID
	oldBID := oldB[0].Sequence.Vessel.ID
	probe := rand.New(rand.NewSource(seed))
	if firstDraw := probe.Int63(); firstDraw != oldAID {
		t.Fatalf("test setup: first draw = %d, old A vessel = %d", firstDraw, oldAID)
	}
	if secondDraw := probe.Int63(); secondDraw != oldBID {
		t.Fatalf("test setup: second draw = %d, old B vessel = %d", secondDraw, oldBID)
	}

	Cleanup(g)
	newA := addSteps(containerA, 5)
	mustAddSequences(t, ctx, g, rand.New(rand.NewSource(seed)))

	if len(g.Sequences) != 3 {
		t.Fatalf("sequence count after insertion = %d, want 3", len(g.Sequences))
	}
	if newA[0].Sequence == nil || newA[0].Sequence.Vessel.ID == oldBID {
		t.Fatalf("fresh A sequence reused remembered B vessel ID %d", oldBID)
	}
	ids := make(map[layoutgraph.EntityID]struct{}, len(g.Sequences))
	for vessel := range g.Sequences {
		if _, duplicate := ids[vessel.ID]; duplicate {
			t.Fatalf("duplicate sequence vessel ID %d", vessel.ID)
		}
		ids[vessel.ID] = struct{}{}
	}

	serialized, err := graphjson.Serialize(ctx, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(serialized.Sequences) != 3 || len(serialized.SequenceVessels) != 3 {
		t.Fatalf(
			"serialization overwrote a sequence: sequences=%d vessels=%d",
			len(serialized.Sequences), len(serialized.SequenceVessels),
		)
	}
	firstBytes, err := json.Marshal(serialized)
	if err != nil {
		t.Fatal(err)
	}

	Cleanup(g)
	mustAddSequences(t, ctx, g, rand.New(rand.NewSource(seed)))
	serializedAgain, err := graphjson.Serialize(ctx, g)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := json.Marshal(serializedAgain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("same-topology repeated sequence reconstruction changed serialized bytes")
	}
}
