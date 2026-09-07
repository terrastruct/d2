package graphjson

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/label"
)

type cancelWhenStackContains struct {
	context.Context
	function string
}

func (ctx *cancelWhenStackContains) Err() error {
	var callers [32]uintptr
	count := runtime.Callers(2, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, ctx.function) {
			return context.Canceled
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

func TestGraphJSONPreservesArrowheadLabelEndpoints(t *testing.T) {
	graph := layoutgraph.NewGraph()
	from := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	to := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	edge := graph.Connect(from, to)
	edge.Label = &layoutgraph.Label{Text: "edge", Position: label.UnlockedMiddle, Width: 20, Height: 10}
	edge.SourceArrowheadLabel = &layoutgraph.Label{Text: "source"}
	edge.TargetArrowheadLabel = &layoutgraph.Label{Text: "target"}

	data, err := Marshal(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	var serialized struct {
		Edges []struct {
			SourceKey *SerializedLabel `json:"sourceArrowheadLabel"`
			TargetKey *SerializedLabel `json:"targetArrowheadLabel"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(data, &serialized); err != nil {
		t.Fatal(err)
	}
	if len(serialized.Edges) != 1 || serialized.Edges[0].SourceKey == nil || serialized.Edges[0].SourceKey.Text != "source" ||
		serialized.Edges[0].TargetKey == nil || serialized.Edges[0].TargetKey.Text != "target" {
		t.Fatalf("graph JSON encoded arrowhead labels on the wrong endpoints: %s", data)
	}

	var roundTrip layoutgraph.Graph
	if err := Unmarshal(t.Context(), data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Edges[0].SourceArrowheadLabel.Text != "source" || roundTrip.Edges[0].TargetArrowheadLabel.Text != "target" {
		t.Fatal("arrowhead labels changed endpoints during round trip")
	}
	if roundTrip.Edges[0].Label == nil || roundTrip.Edges[0].Label.Position != label.UnlockedMiddle {
		t.Fatalf("edge label position = %+v, want %v", roundTrip.Edges[0].Label, label.UnlockedMiddle)
	}
}

func TestGraphJSONOmitsLongDistanceNeighborRequirements(t *testing.T) {
	graph := layoutgraph.NewGraph()
	from := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	to := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	graph.Connect(from, to)

	baseline, err := Marshal(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	from.LongDistanceNeighborRequirements = map[*layoutgraph.Node]layoutgraph.LongDistanceNeighborRequirements{
		to: {EdgeCount: 300, MaxWidth: 5_000, MaxHeight: 6_000},
	}

	withRuntimeState, err := Marshal(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(withRuntimeState, baseline) {
		t.Fatalf("transient long-distance neighbor requirements changed Graph JSON\nbaseline: %s\nwith state: %s", baseline, withRuntimeState)
	}

	var roundTrip layoutgraph.Graph
	if err := Unmarshal(t.Context(), withRuntimeState, &roundTrip); err != nil {
		t.Fatal(err)
	}
	for _, node := range roundTrip.Nodes {
		if node.LongDistanceNeighborRequirements != nil {
			t.Fatal("transient long-distance neighbor requirements survived Graph JSON round trip")
		}
	}
}

func TestSerializeCancelsDuringBoundedWalks(t *testing.T) {
	t.Run("edge reference collection", func(t *testing.T) {
		graph := layoutgraph.NewGraph()
		for i := 0; i < 130; i++ {
			graph.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i+1), 10, 10))
		}

		_, err := Serialize(
			&cancelWhenStackContains{Context: context.Background(), function: "newEdgeSerializationIDs"},
			graph,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serialize error = %v, want context.Canceled", err)
		}
	})

	t.Run("iterative tree encoding", func(t *testing.T) {
		graph := layoutgraph.NewGraph()
		rootSentinel := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
		rootNode := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
		rootEdge := graph.Connect(rootSentinel, rootNode)
		root := &layoutgraph.Tree{Node: rootNode, SentinelEdge: rootEdge}
		for i := 0; i < 130; i++ {
			node := graph.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i+3), 10, 10))
			edge := graph.Connect(rootNode, node)
			root.Children = append(root.Children, &layoutgraph.Tree{Node: node, SentinelEdge: edge, Parent: root})
		}
		graph.Trees[rootSentinel] = []*layoutgraph.Tree{root}

		_, err := Serialize(
			&cancelWhenStackContains{Context: context.Background(), function: "serializeTree"},
			graph,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serialize error = %v, want context.Canceled", err)
		}
	})
}

func TestSerializeRejectsHostileTreeTopologyBeforeEncoding(t *testing.T) {
	newTreeGraph := func() (*layoutgraph.Graph, *layoutgraph.Tree) {
		graph := layoutgraph.NewGraph()
		rootSentinel := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
		node := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
		edge := graph.Connect(rootSentinel, node)
		root := &layoutgraph.Tree{Node: node, SentinelEdge: edge}
		graph.Trees[rootSentinel] = []*layoutgraph.Tree{root}
		return graph, root
	}

	t.Run("excessive depth", func(t *testing.T) {
		graph, root := newTreeGraph()
		parent := root
		for i := 0; i < maxEngineTopologyDepth; i++ {
			child := &layoutgraph.Tree{Node: root.Node, SentinelEdge: root.SentinelEdge, Parent: parent}
			parent.Children = []*layoutgraph.Tree{child}
			parent = child
		}
		_, err := Serialize(t.Context(), graph)
		if err == nil || !strings.Contains(err.Error(), "tree depth exceeds limit") {
			t.Fatalf("Serialize error = %v, want tree-depth rejection", err)
		}
	})

	t.Run("child cycle", func(t *testing.T) {
		graph, root := newTreeGraph()
		child := &layoutgraph.Tree{Node: root.Node, SentinelEdge: root.SentinelEdge, Parent: root}
		root.Children = []*layoutgraph.Tree{child}
		child.Children = []*layoutgraph.Tree{root}
		_, err := Serialize(t.Context(), graph)
		if err == nil || !strings.Contains(err.Error(), "tree child cycle") {
			t.Fatalf("Serialize error = %v, want tree-cycle rejection", err)
		}
	})

	t.Run("nil child", func(t *testing.T) {
		graph, root := newTreeGraph()
		root.Children = []*layoutgraph.Tree{nil}
		_, err := Serialize(t.Context(), graph)
		if err == nil || !strings.Contains(err.Error(), "nil tree child") {
			t.Fatalf("Serialize error = %v, want nil-child rejection", err)
		}
	})

	t.Run("nil sentinel edge", func(t *testing.T) {
		graph, root := newTreeGraph()
		root.SentinelEdge = nil
		_, err := Serialize(t.Context(), graph)
		if err == nil || !strings.Contains(err.Error(), "nil sentinel edge") {
			t.Fatalf("Serialize error = %v, want nil-sentinel rejection", err)
		}
	})
}

func TestSerializeRejectsNonForestTreeOwnershipBeforeEncoding(t *testing.T) {
	newTreeGraph := func() (*layoutgraph.Graph, *layoutgraph.Tree, *layoutgraph.Tree) {
		graph := layoutgraph.NewGraph()
		rootSentinel := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
		rootNode := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
		childNode := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
		root := &layoutgraph.Tree{
			Node:         rootNode,
			SentinelEdge: graph.Connect(rootSentinel, rootNode),
		}
		child := &layoutgraph.Tree{
			Node:         childNode,
			Parent:       root,
			SentinelEdge: graph.Connect(rootNode, childNode),
		}
		root.Children = []*layoutgraph.Tree{child}
		graph.Trees[rootSentinel] = []*layoutgraph.Tree{root}
		return graph, root, child
	}

	t.Run("exponentially expanding shared DAG", func(t *testing.T) {
		graph, root, child := newTreeGraph()
		root.Children = []*layoutgraph.Tree{child, child}
		current := child
		for i := 0; i < 40; i++ {
			node := graph.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i+4), 10, 10))
			next := &layoutgraph.Tree{
				Node:         node,
				Parent:       current,
				SentinelEdge: graph.Connect(current.Node, node),
			}
			current.Children = []*layoutgraph.Tree{next, next}
			current = next
		}

		_, err := Serialize(t.Context(), graph)
		if err == nil || !strings.Contains(err.Error(), "repeated under one parent") {
			t.Fatalf("Serialize error = %v, want shared-tree preflight rejection", err)
		}
	})

	t.Run("duplicate root", func(t *testing.T) {
		graph, root, _ := newTreeGraph()
		for sentinel := range graph.Trees {
			graph.Trees[sentinel] = []*layoutgraph.Tree{root, root}
		}
		_, err := Serialize(t.Context(), graph)
		if err == nil || !strings.Contains(err.Error(), "more than one root") {
			t.Fatalf("Serialize error = %v, want duplicate-root preflight rejection", err)
		}
	})

	t.Run("wrong child parent", func(t *testing.T) {
		graph, _, child := newTreeGraph()
		child.Parent = nil
		_, err := Serialize(t.Context(), graph)
		if err == nil || !strings.Contains(err.Error(), "inconsistent parent") {
			t.Fatalf("Serialize error = %v, want parent-link preflight rejection", err)
		}
	})

	t.Run("partial node-to-tree aliases", func(t *testing.T) {
		graph, root, _ := newTreeGraph()
		graph.NodeToTree = map[*layoutgraph.Node]*layoutgraph.Tree{root.Node: root}
		_, err := Serialize(t.Context(), graph)
		if err == nil || !strings.Contains(err.Error(), "do not cover") {
			t.Fatalf("Serialize error = %v, want alias-coverage preflight rejection", err)
		}
	})

	t.Run("wrong node-to-tree alias", func(t *testing.T) {
		graph, root, child := newTreeGraph()
		graph.NodeToTree = map[*layoutgraph.Node]*layoutgraph.Tree{
			root.Node:  root,
			child.Node: root,
		}
		_, err := Serialize(t.Context(), graph)
		if err == nil || !strings.Contains(err.Error(), "does not match the tree node") {
			t.Fatalf("Serialize error = %v, want alias-identity preflight rejection", err)
		}
	})
}

func TestSerializeRejectsNilGraph(t *testing.T) {
	//lint:ignore SA1012 This test exercises the explicit nil-context contract.
	if serialized, err := Serialize(nil, nil); serialized != nil || err == nil || err.Error() != "TALA SerializeGraph requires a context" {
		t.Fatalf("Serialize(nil, nil) = (%v, %v), want nil and exact context error", serialized, err)
	}
	for _, test := range []struct {
		name string
		ctx  context.Context
	}{
		{name: "active", ctx: t.Context()},
		{name: "canceled", ctx: func() context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return ctx
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			serialized, err := Serialize(test.ctx, nil)
			if serialized != nil || err == nil || err.Error() != "cannot serialize a nil graph" {
				t.Fatalf("Serialize(ctx, nil) = (%v, %v), want nil and graph error", serialized, err)
			}
		})
	}
	data, err := Marshal(t.Context(), nil)
	if data != nil || err == nil || err.Error() != "cannot serialize a nil graph" {
		t.Fatalf("Marshal(ctx, nil) = (%q, %v), want nil and graph error", data, err)
	}
}

func TestDeserializePreservesNilInputPrecedence(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	for _, test := range []struct {
		name  string
		graph *SerializedGraph
		out   *layoutgraph.Graph
		want  string
	}{
		{name: "nil graph before nil output", want: "cannot deserialize a nil graph"},
		{name: "nil output before cancellation", graph: &SerializedGraph{}, want: "cannot deserialize into a nil graph"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := Deserialize(ctx, test.graph, test.out)
			if err == nil || err.Error() != test.want {
				t.Fatalf("Deserialize error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestUnmarshalReportsJSONErrorBeforeCancellation(t *testing.T) {
	graph := layoutgraph.NewGraph()
	existing := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	before, err := Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = Unmarshal(ctx, []byte(`{"nodes":`), graph)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("Unmarshal error = %v, want JSON error before context cancellation", err)
	}
	after, snapshotErr := Serialize(t.Context(), graph)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	if !reflect.DeepEqual(after, before) || len(graph.Nodes) != 1 || graph.Nodes[0] != existing || existing.Graph != graph {
		t.Fatal("malformed JSON changed the destination graph")
	}

	err = Unmarshal(ctx, []byte(`{}`), graph)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Unmarshal valid JSON error = %v, want context.Canceled", err)
	}
	after, snapshotErr = Serialize(t.Context(), graph)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	if !reflect.DeepEqual(after, before) || len(graph.Nodes) != 1 || graph.Nodes[0] != existing || existing.Graph != graph {
		t.Fatal("canceled valid JSON reconstruction changed the destination graph")
	}
}

type panicOnErrContext struct {
	context.Context
}

func (panicOnErrContext) Err() error {
	panic("synthetic context panic")
}

func TestUnmarshalPreservesExactPanicSanitizer(t *testing.T) {
	graph := layoutgraph.NewGraph()
	existing := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	before, snapshotErr := Serialize(t.Context(), graph)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	err := Unmarshal(panicOnErrContext{Context: context.Background()}, []byte(`{}`), graph)
	if err == nil || err.Error() != "deserialize graph failed due to an internal invariant" {
		t.Fatalf("Unmarshal error = %v, want exact internal-invariant sanitizer", err)
	}
	after, snapshotErr := Serialize(t.Context(), graph)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	if !reflect.DeepEqual(after, before) || len(graph.Nodes) != 1 || graph.Nodes[0] != existing || existing.Graph != graph {
		t.Fatal("sanitized deserialization panic changed the destination graph")
	}
}

func TestUnmarshalRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{name: "unknown field", data: `{"obsolete":true}`, want: `unknown field "obsolete"`},
		{name: "trailing value", data: `{} {}`, want: "graph JSON contains trailing data"},
		{name: "null graph", data: `null`, want: "cannot deserialize a nil graph"},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := layoutgraph.NewGraph()
			existing := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
			err := Unmarshal(t.Context(), []byte(test.data), graph)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Unmarshal error = %v, want %q", err, test.want)
			}
			if len(graph.Nodes) != 1 || graph.Nodes[0] != existing || existing.Graph != graph {
				t.Fatal("rejected JSON changed the destination graph")
			}
		})
	}
}

func TestDeserializeRejectsDuplicateIDsAndMissingHierarchyNodes(t *testing.T) {
	tests := []struct {
		name  string
		graph *SerializedGraph
		want  string
	}{
		{
			name:  "reserved top-level node ID",
			graph: &SerializedGraph{Nodes: []SerializedNode{{ID: 0}}},
			want:  "uses reserved node ID 0",
		},
		{
			name: "reserved auxiliary vessel node ID",
			graph: &SerializedGraph{
				Clusters:       map[layoutgraph.EntityID]SerializedCluster{0: {}},
				ClusterVessels: map[layoutgraph.EntityID]SerializedNode{0: {ID: 0}},
			},
			want: "uses reserved node ID 0",
		},
		{
			name: "reserved tree node ID",
			graph: &SerializedGraph{
				Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
				Trees: map[layoutgraph.EntityID][]SerializedTree{1: {{
					Node:         SerializedNode{ID: 0},
					SentinelEdge: SerializedEdge{ID: 3, FromNode: 1, ToNode: 2},
				}}},
			},
			want: "uses reserved node ID 0",
		},
		{
			name: "reserved tree edge ID",
			graph: &SerializedGraph{
				Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
				Trees: map[layoutgraph.EntityID][]SerializedTree{1: {{
					Node:         SerializedNode{ID: 2},
					SentinelEdge: SerializedEdge{FromNode: 1, ToNode: 2},
				}}},
			},
			want: "uses reserved serialized edge ID 0",
		},
		{
			name: "duplicate node",
			graph: &SerializedGraph{Nodes: []SerializedNode{
				{ID: 1}, {ID: 1},
			}},
			want: "duplicate node ID 1",
		},
		{
			name: "duplicate edge",
			graph: &SerializedGraph{
				Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
				Edges: []SerializedEdge{
					{ID: 3, FromNode: 1, ToNode: 2},
					{ID: 3, FromNode: 1, ToNode: 2},
				},
			},
			want: "duplicate edge ID 3",
		},
		{
			name: "reserved serialized edge ID",
			graph: &SerializedGraph{
				Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
				Edges: []SerializedEdge{{FromNode: 1, ToNode: 2}},
			},
			want: "uses reserved serialized edge ID 0",
		},
		{
			name: "positive synthetic edge reference",
			graph: func() *SerializedGraph {
				originalID := layoutgraph.EntityID(0)
				return &SerializedGraph{
					Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
					Edges: []SerializedEdge{{
						ID: 3, OriginalID: &originalID, FromNode: 1, ToNode: 2,
					}},
				}
			}(),
			want: "invalid original edge ID 0 for serialized ID 3",
		},
		{
			name: "synthetic edge preserves only unset ID",
			graph: func() *SerializedGraph {
				originalID := layoutgraph.EntityID(7)
				return &SerializedGraph{
					Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
					Edges: []SerializedEdge{{
						ID: -1, OriginalID: &originalID, FromNode: 1, ToNode: 2,
					}},
				}
			}(),
			want: "invalid original edge ID 7 for serialized ID -1",
		},
		{
			name: "cluster contains itself",
			graph: &SerializedGraph{
				Nodes:    []SerializedNode{{ID: 1}},
				Clusters: map[layoutgraph.EntityID]SerializedCluster{1: {Container: 1}},
			},
			want: "cluster 1 cannot contain itself",
		},
		{
			name: "sequence contains itself",
			graph: &SerializedGraph{
				Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
				Sequences: map[layoutgraph.EntityID]SerializedSequence{1: {
					Container: 1, Nodes: []layoutgraph.EntityID{1, 2},
				}},
			},
			want: "sequence 1 cannot contain itself",
		},
		{
			name: "duplicate tree node owner",
			graph: &SerializedGraph{
				Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
				Edges: []SerializedEdge{{ID: 3, FromNode: 1, ToNode: 2}},
				Trees: map[layoutgraph.EntityID][]SerializedTree{1: {
					{Node: SerializedNode{ID: 2}, SentinelEdge: SerializedEdge{ID: 3, FromNode: 1, ToNode: 2}},
					{Node: SerializedNode{ID: 2}, SentinelEdge: SerializedEdge{ID: 3, FromNode: 1, ToNode: 2}},
				}},
			},
			want: "tree node 2 has more than one tree owner",
		},
		{
			name: "auxiliary tree node record",
			graph: &SerializedGraph{
				Nodes:          []SerializedNode{{ID: 1}},
				Clusters:       map[layoutgraph.EntityID]SerializedCluster{2: {}},
				ClusterVessels: map[layoutgraph.EntityID]SerializedNode{2: {ID: 2}},
				Trees: map[layoutgraph.EntityID][]SerializedTree{1: {{
					Node:         SerializedNode{ID: 2},
					SentinelEdge: SerializedEdge{ID: 3, FromNode: 1, ToNode: 2},
				}}},
			},
			want: "tree node 2 repeats a non-top-level node record",
		},
		{
			name: "missing hierarchy node",
			graph: &SerializedGraph{
				Hierarchies: []SerializedHierarchy{{Levels: map[layoutgraph.EntityID]int{9: 1}}},
			},
			want: "hierarchy references missing node 9",
		},
		{
			name: "duplicate hierarchy membership",
			graph: &SerializedGraph{
				Nodes: []SerializedNode{{ID: 1}},
				Hierarchies: []SerializedHierarchy{
					{Levels: map[layoutgraph.EntityID]int{1: 0}},
					{Levels: map[layoutgraph.EntityID]int{1: 0}},
				},
			},
			want: "hierarchy node 1 appears in both hierarchy 0 and hierarchy 1",
		},
		{
			name: "overlapping hierarchy membership",
			graph: &SerializedGraph{
				Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
				Hierarchies: []SerializedHierarchy{
					{Levels: map[layoutgraph.EntityID]int{1: 0, 2: 1}},
					{Levels: map[layoutgraph.EntityID]int{2: 0}},
				},
			},
			want: "hierarchy node 2 appears in both hierarchy 0 and hierarchy 1",
		},
		{
			name: "sequence without enough steps",
			graph: &SerializedGraph{
				Sequences: map[layoutgraph.EntityID]SerializedSequence{9: {}},
				SequenceVessels: map[layoutgraph.EntityID]SerializedNode{
					9: {ID: 9},
				},
			},
			want: "sequence 9 has 0 steps; want at least 2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Deserialize(t.Context(), test.graph, layoutgraph.NewGraph())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v; want substring %q", err, test.want)
			}
		})
	}
}

type cancelAfterContextChecks struct {
	context.Context
	remaining int
}

func (ctx *cancelAfterContextChecks) Err() error {
	ctx.remaining--
	if ctx.remaining <= 0 {
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestDeserializeChecksCancellationDuringReconstruction(t *testing.T) {
	nodes := make([]SerializedNode, 1_000)
	for i := range nodes {
		nodes[i].ID = layoutgraph.EntityID(i + 1)
	}
	out := layoutgraph.NewGraph()
	existing := out.AddNode(layoutgraph.NewNode(-1, 10, 10))
	ctx := &cancelAfterContextChecks{Context: context.Background(), remaining: 3}
	err := Deserialize(ctx, &SerializedGraph{Nodes: nodes}, out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(out.Nodes) != 1 || out.Nodes[0] != existing {
		t.Fatal("canceled deserialization mutated its destination graph")
	}
}

type cancelDuringEdgeDeserializationContext struct {
	context.Context
}

func (ctx cancelDuringEdgeDeserializationContext) Err() error {
	var callers [32]uintptr
	count := runtime.Callers(2, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "deserializeEdge") {
			return context.Canceled
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

func TestDeserializeRouteCancellationIsAtomic(t *testing.T) {
	points := make([]*SerializedPoint, 1_024)
	for i := range points {
		points[i] = &SerializedPoint{X: float64(i), Y: float64(i)}
	}
	serialized := &SerializedGraph{
		Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
		Edges: []SerializedEdge{{ID: 3, FromNode: 1, ToNode: 2, Points: points}},
	}

	out := layoutgraph.NewGraph()
	existing := out.AddNode(layoutgraph.NewNode(-1, 10, 10))
	out.CellSize = 17
	before, err := Serialize(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	err = Deserialize(cancelDuringEdgeDeserializationContext{Context: context.Background()}, serialized, out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	after, err := Serialize(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) || len(out.Nodes) != 1 || out.Nodes[0] != existing || existing.Graph != out {
		t.Fatal("route cancellation mutated the existing destination graph")
	}
}

func TestDeserializePreservesDestinationMutex(t *testing.T) {
	out := layoutgraph.NewGraph()
	if err := Deserialize(t.Context(), &SerializedGraph{
		CrossingCost: -1, TurnCost: -2, NonCenterPortCost: -3,
	}, out); err != nil {
		t.Fatal(err)
	}

	serialized := &SerializedGraph{
		Nodes:             []SerializedNode{{ID: 1}},
		CrossingCost:      11,
		TurnCost:          12,
		NonCenterPortCost: 13,
	}
	if err := Deserialize(t.Context(), serialized, out); err != nil {
		t.Fatal(err)
	}
	if len(out.Nodes) != 1 || out.Nodes[0].Graph != out {
		t.Fatal("deserialized nodes do not reference the destination graph")
	}

	costs := make(chan [3]float64, 1)
	go func() {
		crossing, turn, nonCenterPort := layoutgraph.CostsForSerialization(out)
		costs <- [3]float64{crossing, turn, nonCenterPort}
	}()
	select {
	case got := <-costs:
		if got != [3]float64{11, 12, 13} {
			t.Fatalf("deserialized costs = %v, want [11 12 13]", got)
		}
	case <-time.After(time.Second):
		t.Fatal("destination cost mutex was not usable after deserialization")
	}
}

func TestDeserializeAllowsOneTopLevelRecordAndOneTreeOwner(t *testing.T) {
	serialized := &SerializedGraph{
		Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
		Edges: []SerializedEdge{{ID: 3, FromNode: 1, ToNode: 2}},
		Trees: map[layoutgraph.EntityID][]SerializedTree{1: {{
			Node:         SerializedNode{ID: 2},
			SentinelEdge: SerializedEdge{ID: 3, FromNode: 1, ToNode: 2},
		}}},
	}
	graph := layoutgraph.NewGraph()
	if err := Deserialize(t.Context(), serialized, graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.NodeToTree) != 1 || graph.NodeToTree[graph.Nodes[1]] == nil {
		t.Fatalf("tree ownership index = %+v, want node 2 exactly once", graph.NodeToTree)
	}
	if graph.NodeToTree[graph.Nodes[1]].SentinelEdge != graph.Edges[0] {
		t.Fatal("tree sentinel did not reuse the exact top-level edge reference")
	}
}

func TestDeserializeReadsSyntheticUnsetEdgeID(t *testing.T) {
	originalID := layoutgraph.EntityID(0)
	serialized := &SerializedGraph{
		Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
		Edges: []SerializedEdge{{
			ID:         -1,
			OriginalID: &originalID,
			FromNode:   1,
			ToNode:     2,
		}},
	}
	graph := layoutgraph.NewGraph()
	if err := Deserialize(t.Context(), serialized, graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Edges) != 1 || graph.Edges[0].ID != 0 {
		t.Fatalf("runtime edges = %+v, want one unset edge", graph.Edges)
	}
}

func TestStyleConversionCoversEverySerializedField(t *testing.T) {
	serializedType := reflect.TypeFor[SerializedStyle]()
	serializedScalarType := reflect.TypeFor[*SerializedScalar]()
	runtimeType := reflect.TypeFor[layoutgraph.EdgeStyle]()
	runtimeScalarType := reflect.TypeFor[*layoutgraph.StyleScalar]()
	if serializedType.NumField() != runtimeType.NumField() {
		t.Fatalf("style field count: serialized=%d runtime=%d", serializedType.NumField(), runtimeType.NumField())
	}
	sourceValue := reflect.New(serializedType).Elem()
	for i := 0; i < serializedType.NumField(); i++ {
		serializedField := serializedType.Field(i)
		if serializedField.Type != serializedScalarType {
			t.Fatalf("SerializedStyle.%s has type %v; update style conversion", serializedField.Name, serializedField.Type)
		}
		runtimeField, ok := runtimeType.FieldByName(serializedField.Name)
		if !ok || runtimeField.Type != runtimeScalarType {
			t.Fatalf("layoutgraph.EdgeStyle.%s is missing or has type %v; update style conversion", serializedField.Name, runtimeField.Type)
		}
		sourceValue.Field(i).Set(reflect.ValueOf(&SerializedScalar{Value: serializedField.Name}))
	}
	style := sourceValue.Interface().(SerializedStyle)
	serialized := &SerializedGraph{
		Nodes: []SerializedNode{{ID: 1}, {ID: 2}},
		Edges: []SerializedEdge{{ID: 3, FromNode: 1, ToNode: 2, Style: style}},
	}
	graph := layoutgraph.NewGraph()
	if err := Deserialize(t.Context(), serialized, graph); err != nil {
		t.Fatal(err)
	}
	runtimeValue := reflect.ValueOf(graph.Edges[0].Style)
	for i := 0; i < serializedType.NumField(); i++ {
		original := sourceValue.Field(i).Interface().(*SerializedScalar)
		cloned := runtimeValue.FieldByName(serializedType.Field(i).Name).Interface().(*layoutgraph.StyleScalar)
		if cloned == nil || cloned.Value != original.Value {
			t.Fatalf("deserialize did not independently copy Style.%s", serializedType.Field(i).Name)
		}
	}
	graph.Edges[0].Style.Stroke.Value = "changed"
	if style.Stroke.Value == "changed" {
		t.Fatal("mutating a deserialized style changed the serialized input")
	}

	roundTrip, err := Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	roundTripValue := reflect.ValueOf(roundTrip.Edges[0].Style)
	for i := 0; i < serializedType.NumField(); i++ {
		field := serializedType.Field(i)
		got := roundTripValue.Field(i).Interface().(*SerializedScalar)
		want := runtimeValue.FieldByName(field.Name).Interface().(*layoutgraph.StyleScalar)
		if got == nil || got.Value != want.Value {
			t.Fatalf("serialize did not copy Style.%s: got %#v, want value %q", field.Name, got, want.Value)
		}
	}
}

func TestSerializeClonesStyleState(t *testing.T) {
	graph := layoutgraph.NewGraph()
	from := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	to := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	edge := graph.Connect(from, to)
	edge.Style.Stroke = &layoutgraph.StyleScalar{Value: "blue"}

	serialized, err := Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	stroke := serialized.Edges[0].Style.Stroke
	if stroke == nil || stroke.Value != "blue" {
		t.Fatalf("serialized stroke = %#v, want an independent fixture copy", stroke)
	}
	stroke.Value = "red"
	if edge.Style.Stroke.Value != "blue" {
		t.Fatal("mutating serialized style changed the runtime graph")
	}
}

func TestSerializedStyleEqualityComparesValues(t *testing.T) {
	left := SerializedStyle{Stroke: &SerializedScalar{Value: "blue"}}
	right := SerializedStyle{Stroke: &SerializedScalar{Value: "blue"}}
	if !equalSerializedStyles(left, right) {
		t.Fatal("equivalent serialized styles were unequal")
	}
	right.Stroke.Value = "red"
	if equalSerializedStyles(left, right) {
		t.Fatal("styles with different serialized values were equal")
	}
}

func TestDeserializeAcceptsMaximumIterativeTreeDepth(t *testing.T) {
	serialized := &SerializedGraph{Nodes: []SerializedNode{{ID: 1}}}
	root := SerializedTree{
		Node:         SerializedNode{ID: 2},
		SentinelEdge: SerializedEdge{ID: 1, FromNode: 1, ToNode: 2},
	}
	current := &root
	for depth := 2; depth <= maxEngineTopologyDepth; depth++ {
		parentID := current.Node.ID
		childID := layoutgraph.EntityID(depth + 1)
		current.Children = []SerializedTree{{
			Node:         SerializedNode{ID: childID},
			SentinelEdge: SerializedEdge{ID: layoutgraph.EntityID(depth), FromNode: parentID, ToNode: childID},
		}}
		current = &current.Children[0]
	}
	serialized.Trees = map[layoutgraph.EntityID][]SerializedTree{1: {root}}

	graph := layoutgraph.NewGraph()
	if err := Deserialize(t.Context(), serialized, graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.NodeToTree) != maxEngineTopologyDepth {
		t.Fatalf("tree index has %d nodes, want %d", len(graph.NodeToTree), maxEngineTopologyDepth)
	}
}

func TestDeserializePreflightBoundsRecordsBeforeReconstruction(t *testing.T) {
	for _, test := range []struct {
		name  string
		graph *SerializedGraph
		want  string
	}{
		{
			name:  "nodes",
			graph: &SerializedGraph{Nodes: make([]SerializedNode, maxEngineNodes+1)},
			want:  "node records exceed limit",
		},
		{
			name:  "edges",
			graph: &SerializedGraph{Edges: make([]SerializedEdge, maxEngineEdges+1)},
			want:  "edge records exceed limit",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := layoutgraph.NewGraph()
			existing := out.AddNode(layoutgraph.NewNode(-1, 10, 10))
			err := Deserialize(t.Context(), test.graph, out)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("oversized record error = %v, want %q", err, test.want)
			}
			if len(out.Nodes) != 1 || out.Nodes[0] != existing {
				t.Fatal("failed preflight mutated the destination graph")
			}
		})
	}
}

func TestGraphCopyAllowsUnsetEdgeIDs(t *testing.T) {
	graph := layoutgraph.NewGraph()
	a := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	c := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
	graph.Connect(a, b)
	graph.Connect(b, c)

	copy, err := layoutgraph.Clone(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if len(copy.Edges) != 2 {
		t.Fatalf("copied graph has %d edges, want 2", len(copy.Edges))
	}
	if copy.Edges[0].ID != 0 || copy.Edges[1].ID != 0 {
		t.Fatalf("copy changed public unset edge IDs: %d, %d", copy.Edges[0].ID, copy.Edges[1].ID)
	}
}

func TestSerializeEmitsSharedHierarchyOnce(t *testing.T) {
	graph := layoutgraph.NewGraph()
	a := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	b := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	hierarchy := layoutgraph.NewDecodedHierarchy(map[*layoutgraph.Node]int{a: 0, b: 1})
	a.Hierarchy = hierarchy
	b.Hierarchy = hierarchy

	serialized, err := Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if len(serialized.Hierarchies) != 1 {
		t.Fatalf("serialized shared hierarchy %d times, want once", len(serialized.Hierarchies))
	}
	if len(serialized.Hierarchies[0].Levels) != 2 {
		t.Fatalf("serialized hierarchy has %d members, want 2", len(serialized.Hierarchies[0].Levels))
	}

	copy, err := layoutgraph.Clone(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if copy.Nodes[0].Hierarchy == nil || copy.Nodes[0].Hierarchy != copy.Nodes[1].Hierarchy {
		t.Fatal("graph copy did not preserve shared hierarchy identity")
	}
}

func TestGraphCopyPreservesUnsetEdgeSequenceAbductionReference(t *testing.T) {
	graph := layoutgraph.NewGraph()
	firstStep := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	secondStep := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	external := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
	firstEdge := graph.Connect(firstStep, external)
	graph.Connect(secondStep, external)

	vessel := layoutgraph.NewNode(9, 20, 10)
	sequence := &layoutgraph.Sequence{
		Vessel: vessel,
		Nodes:  []*layoutgraph.Node{firstStep, secondStep},
		Graph:  graph,
		EdgeAbductions: []*layoutgraph.EdgeAbduction{{
			Edge:           firstEdge,
			OriginallyFrom: firstStep,
			OriginallyTo:   external,
			CurrentFrom:    vessel,
			CurrentTo:      external,
		}},
	}
	graph.Sequences[vessel] = sequence

	serialized, err := Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if serialized.Edges[0].ID == serialized.Edges[1].ID {
		t.Fatalf("unset edges received the same serialized reference %d", serialized.Edges[0].ID)
	}
	if serialized.Edges[0].ID >= 0 || serialized.Edges[1].ID >= 0 {
		t.Fatalf("unset edges received non-synthetic serialized IDs %d and %d", serialized.Edges[0].ID, serialized.Edges[1].ID)
	}
	if serialized.Edges[0].OriginalID == nil || *serialized.Edges[0].OriginalID != 0 ||
		serialized.Edges[1].OriginalID == nil || *serialized.Edges[1].OriginalID != 0 {
		t.Fatal("serialized references did not preserve the public unset IDs")
	}
	if got := serialized.Sequences[vessel.ID].EdgeAbductions[0].Edge; got != serialized.Edges[0].ID {
		t.Fatalf("sequence abduction references edge %d, want %d", got, serialized.Edges[0].ID)
	}

	copy, err := layoutgraph.Clone(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	var copiedSequence *layoutgraph.Sequence
	for _, candidate := range copy.Sequences {
		copiedSequence = candidate
		break
	}
	if copiedSequence == nil || len(copiedSequence.EdgeAbductions) != 1 {
		t.Fatalf("copied sequence is missing its abduction: %+v", copiedSequence)
	}
	if copiedSequence.EdgeAbductions[0].Edge != copy.Edges[0] {
		t.Fatal("sequence abduction was rebound to a different unset-ID edge")
	}
	if copy.Edges[0].ID != 0 || copy.Edges[1].ID != 0 {
		t.Fatalf("copy changed public unset edge IDs: %d, %d", copy.Edges[0].ID, copy.Edges[1].ID)
	}
}

func TestGraphCopyPreservesUnsetEdgeClusterAbductionReference(t *testing.T) {
	graph := layoutgraph.NewGraph()
	firstMember := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	secondMember := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	external := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
	firstEdge := graph.Connect(firstMember, external)
	graph.Connect(secondMember, external)

	vessel := layoutgraph.NewNode(9, 20, 10)
	cluster := &layoutgraph.Cluster{
		Vessel:      vessel,
		Nodes:       []*layoutgraph.Node{firstMember, secondMember},
		Graph:       graph,
		Arrangement: layoutgraph.Row,
		EdgeAbductions: []*layoutgraph.EdgeAbduction{{
			Edge:           firstEdge,
			OriginallyFrom: firstMember,
			OriginallyTo:   external,
			CurrentFrom:    vessel,
			CurrentTo:      external,
		}},
	}
	graph.Clusters[vessel] = cluster

	serialized, err := Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if got := serialized.Clusters[vessel.ID].EdgeAbductions[0].Edge; got != serialized.Edges[0].ID {
		t.Fatalf("cluster abduction references edge %d, want %d", got, serialized.Edges[0].ID)
	}
	if serialized.Clusters[vessel.ID].EdgeAbductions[0].Edge >= 0 {
		t.Fatalf("cluster abduction received non-synthetic serialized ID %d", serialized.Clusters[vessel.ID].EdgeAbductions[0].Edge)
	}

	copy, err := layoutgraph.Clone(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	var copiedCluster *layoutgraph.Cluster
	for _, candidate := range copy.Clusters {
		copiedCluster = candidate
		break
	}
	if copiedCluster == nil || len(copiedCluster.EdgeAbductions) != 1 {
		t.Fatalf("copied cluster is missing its abduction: %+v", copiedCluster)
	}
	if copiedCluster.EdgeAbductions[0].Edge != copy.Edges[0] {
		t.Fatal("cluster abduction was rebound to a different unset-ID edge")
	}
	if copy.Edges[0].ID != 0 || copy.Edges[1].ID != 0 {
		t.Fatalf("copy changed public unset edge IDs: %d, %d", copy.Edges[0].ID, copy.Edges[1].ID)
	}
}

func TestGraphCopyPreservesUnsetTreeSentinelReference(t *testing.T) {
	graph := layoutgraph.NewGraph()
	rootSentinel := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	treeNode := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	other := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
	sentinelEdge := graph.Connect(rootSentinel, treeNode)
	graph.Connect(rootSentinel, other)
	tree := layoutgraph.NewTree(treeNode)
	tree.SentinelEdge = sentinelEdge
	graph.Trees[rootSentinel] = []*layoutgraph.Tree{tree}

	serialized, err := Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if serialized.Edges[0].ID == serialized.Edges[1].ID {
		t.Fatalf("unset edges received the same serialized reference %d", serialized.Edges[0].ID)
	}
	serializedTrees := serialized.Trees[rootSentinel.ID]
	if len(serializedTrees) != 1 || serializedTrees[0].SentinelEdge.ID != serialized.Edges[0].ID {
		t.Fatalf("tree sentinel reference = %+v, want edge %d", serializedTrees, serialized.Edges[0].ID)
	}
	if serializedTrees[0].SentinelEdge.ID >= 0 {
		t.Fatalf("tree sentinel received non-synthetic serialized ID %d", serializedTrees[0].SentinelEdge.ID)
	}

	copy, err := layoutgraph.Clone(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	var copiedTree *layoutgraph.Tree
	for sentinel, roots := range copy.Trees {
		if sentinel != nil && sentinel.ID == rootSentinel.ID && len(roots) == 1 {
			copiedTree = roots[0]
			break
		}
	}
	if copiedTree == nil {
		t.Fatal("copied tree is missing")
	}
	if copiedTree.SentinelEdge != copy.Edges[0] {
		t.Fatal("tree sentinel was rebound to a different unset-ID edge")
	}
	if copiedTree.SentinelEdge.ID != 0 || copy.Edges[1].ID != 0 {
		t.Fatalf("copy changed public unset edge IDs: %d, %d", copiedTree.SentinelEdge.ID, copy.Edges[1].ID)
	}
}
