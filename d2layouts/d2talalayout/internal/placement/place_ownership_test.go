package placement

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

type placeOwnerFixture struct {
	graph     *layoutgraph.Graph
	nodes     []*layoutgraph.Node
	originals map[*layoutgraph.Node]*layoutgraph.Graph
	probe     *layoutgraph.Node
	prior     map[*layoutgraph.Node]layoutgraph.Nodes
}

func newPlaceOwnerFixture() placeOwnerFixture {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 20, 10)
	second := layoutgraph.NewNode(2, 20, 10)
	graph.AddNewNodeToContainer(nil, first)
	graph.AddNewNodeToContainer(nil, second)
	cluster := &layoutgraph.Cluster{
		Nodes:              []*layoutgraph.Node{first, second},
		Graph:              graph,
		Arrangement:        layoutgraph.Row,
		DesiredArrangement: layoutgraph.Row,
	}
	cluster.Vessel = grouping.CreateVessel(cluster, 3)
	grouping.AddCluster(graph, cluster)

	ordinary := layoutgraph.NewNode(4, 20, 10)
	graph.AddNewNodeToContainer(nil, ordinary)
	prior := map[*layoutgraph.Node]layoutgraph.Nodes{ordinary: {cluster.Vessel}}
	graph.CommonUncleSiblings = prior
	nodes := []*layoutgraph.Node{cluster.Vessel, first, second, ordinary}
	originals := make(map[*layoutgraph.Node]*layoutgraph.Graph, len(nodes))
	for _, node := range nodes {
		owner := layoutgraph.NewGraph()
		owner.CopyEntitiesFrom(graph)
		node.Graph = owner
		originals[node] = owner
	}
	return placeOwnerFixture{
		graph:     graph,
		nodes:     nodes,
		originals: originals,
		probe:     first,
		prior:     prior,
	}
}

func (fixture placeOwnerFixture) requireOriginalOwners(t *testing.T) {
	t.Helper()
	for _, node := range fixture.nodes {
		if node.Graph != fixture.originals[node] {
			t.Fatalf("node %d owner = %p, want exact pre-Place owner %p", node.ID, node.Graph, fixture.originals[node])
		}
	}
	if reflect.ValueOf(fixture.graph.CommonUncleSiblings).Pointer() != reflect.ValueOf(fixture.prior).Pointer() {
		t.Fatal("Place did not restore the exact prior CommonUncleSiblings map")
	}
}

type interruptAfterOwnerRewrite struct {
	context.Context
	node       *layoutgraph.Node
	original   *layoutgraph.Graph
	observed   bool
	panicValue any
}

func (ctx *interruptAfterOwnerRewrite) Err() error {
	if ctx.node.Graph == ctx.original {
		return ctx.Context.Err()
	}
	ctx.observed = true
	if ctx.panicValue != nil {
		panic(ctx.panicValue)
	}
	return context.Canceled
}

func TestPlaceCancellationRestoresExactPreexistingOwners(t *testing.T) {
	fixture := newPlaceOwnerFixture()
	ctx := &interruptAfterOwnerRewrite{
		Context:  context.Background(),
		node:     fixture.probe,
		original: fixture.originals[fixture.probe],
	}
	err := Place(ctx, fixture.graph, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Place error = %v, want context cancellation", err)
	}
	if !ctx.observed {
		t.Fatal("Place cancellation did not observe a temporary owner rewrite")
	}
	fixture.requireOriginalOwners(t)
}

func TestPlacePanicRestoresExactPreexistingOwners(t *testing.T) {
	fixture := newPlaceOwnerFixture()
	panicValue := &struct{ marker string }{marker: "placement owner rewrite"}
	ctx := &interruptAfterOwnerRewrite{
		Context:    context.Background(),
		node:       fixture.probe,
		original:   fixture.originals[fixture.probe],
		panicValue: panicValue,
	}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = Place(ctx, fixture.graph, 1)
	}()
	if recovered != panicValue {
		t.Fatalf("recovered panic = %#v, want exact payload %#v", recovered, panicValue)
	}
	if !ctx.observed {
		t.Fatal("Place panic did not observe a temporary owner rewrite")
	}
	fixture.requireOriginalOwners(t)
}

func TestPlaceSnapshotCancellationRestoresPriorState(t *testing.T) {
	fixture := newPlaceOwnerFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Place(ctx, fixture.graph, 1)
	requireCanceledAt(t, err, "NodeGraphOwnershipSnapshot")
	fixture.requireOriginalOwners(t)
}

func TestPlaceSuccessCommitsFinalOwners(t *testing.T) {
	fixture := newPlaceOwnerFixture()
	if err := Place(context.Background(), fixture.graph, 1); err != nil {
		t.Fatal(err)
	}
	for _, node := range fixture.nodes {
		if node.Graph != fixture.graph {
			t.Fatalf("successful Place owner for node %d = %p, want final graph %p", node.ID, node.Graph, fixture.graph)
		}
	}
	if fixture.graph.CommonUncleSiblings != nil {
		t.Fatal("successful Place retained transient CommonUncleSiblings state")
	}
}

func TestPlaceRejectsNilGraph(t *testing.T) {
	if err := Place(context.Background(), nil, 1); err == nil {
		t.Fatal("nil graph placement succeeded")
	}
}

func BenchmarkPlaceRepresentative(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		graph := setupSimpleGraphForBenchmark()
		b.StartTimer()
		if err := Place(context.Background(), graph, 1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPlaceTiny(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		graph := layoutgraph.NewGraph()
		graph.AddNewNodeToContainer(nil, layoutgraph.NewNode(1, 10, 10))
		b.StartTimer()
		if err := Place(context.Background(), graph, 1); err != nil {
			b.Fatal(err)
		}
	}
}
