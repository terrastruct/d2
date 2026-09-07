package layoutgraph_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func TestCloneMatchesGraphJSONProjection(t *testing.T) {
	for _, test := range []struct {
		name  string
		graph *layoutgraph.Graph
	}{
		{name: "empty", graph: layoutgraph.NewGraph()},
		{name: "layout state", graph: newCloneParityGraph()},
		{name: "detached tree records", graph: newDetachedTreeGraph()},
		{name: "inactive grouping vessels", graph: newInactiveGroupingGraph()},
		{name: "shared inactive grouping vessel", graph: newSharedGroupingVesselGraph()},
	} {
		t.Run(test.name, func(t *testing.T) {
			direct, err := layoutgraph.Clone(context.Background(), test.graph)
			if err != nil {
				t.Fatal(err)
			}

			serialized, err := graphjson.Serialize(t.Context(), test.graph)
			if err != nil {
				t.Fatal(err)
			}
			codecClone := layoutgraph.NewGraph()
			if err := graphjson.Deserialize(t.Context(), serialized, codecClone); err != nil {
				t.Fatal(err)
			}

			directProjection, err := graphjson.Serialize(t.Context(), direct)
			if err != nil {
				t.Fatal(err)
			}
			codecProjection, err := graphjson.Serialize(t.Context(), codecClone)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(directProjection, codecProjection) {
				t.Fatalf("direct clone does not match the graph codec projection\ndirect: %#v\ncodec:  %#v", directProjection, codecProjection)
			}
		})
	}
}

func TestCloneKeepsInactiveVesselsDetached(t *testing.T) {
	cloned, err := layoutgraph.Clone(context.Background(), newInactiveGroupingGraph())
	if err != nil {
		t.Fatal(err)
	}
	cluster := onlyCluster(t, cloned)
	sequence := onlySequence(t, cloned)
	if cluster.Vessel.Graph != nil || sequence.Vessel.Graph != nil {
		t.Fatal("clone activated an inactive grouping vessel")
	}
	if cluster.Graph != cloned || sequence.Graph != cloned {
		t.Fatal("inactive grouping records are not owned by the cloned graph")
	}
}

func TestCloneAcceptsInactiveGroupingVesselHubSpokes(t *testing.T) {
	source := newInactiveGroupingGraph()
	if err := graphJSONClone(t.Context(), source); err != nil {
		t.Fatalf("legacy graph JSON clone rejected exact inactive vessel spokes: %v", err)
	}
	cloned, err := layoutgraph.Clone(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	clusterVessel := onlyCluster(t, cloned).Vessel
	sequenceVessel := onlySequence(t, cloned).Vessel
	for _, spokes := range cloned.Hubs {
		if len(spokes) == 2 && spokes[0] == clusterVessel && spokes[1] == sequenceVessel {
			return
		}
	}
	t.Fatal("clone did not preserve exact inactive grouping vessels as hub spokes")
}

func TestCloneOwnsIndependentGraphState(t *testing.T) {
	source := newCloneParityGraph()
	cloned, err := layoutgraph.Clone(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}

	sourceEdge := source.Edges[0]
	clonedEdge := cloned.Edges[0]
	if clonedEdge == sourceEdge || clonedEdge.From == sourceEdge.From || clonedEdge.To == sourceEdge.To {
		t.Fatal("clone retained source graph identity")
	}
	if clonedEdge.Points[0] == sourceEdge.Points[0] || clonedEdge.Style.Stroke == sourceEdge.Style.Stroke {
		t.Fatal("clone retained source edge-owned pointers")
	}
	clonedEdge.Points[0].X++
	clonedEdge.Style.Stroke.Value = "changed"
	if sourceEdge.Points[0].X == clonedEdge.Points[0].X || sourceEdge.Style.Stroke.Value == "changed" {
		t.Fatal("mutating cloned edge state changed the source")
	}

	cluster := onlyCluster(t, cloned)
	if cluster.Graph != cloned || cluster.Vessel.Graph != cloned {
		t.Fatal("active cloned cluster is not owned by the cloned graph")
	}
	if cluster.Nodes[0].Cluster != cluster || cluster.EdgeAbductions[0].Edge != cloned.Edges[0] {
		t.Fatal("cloned cluster relationships do not point within the cloned graph")
	}
	sequence := onlySequence(t, cloned)
	if sequence.Graph != cloned || sequence.Vessel.Graph != cloned {
		t.Fatal("active cloned sequence is not owned by the cloned graph")
	}
	if sequence.Nodes[0].Sequence != sequence || sequence.EdgeAbductions[0].Edge != cloned.Edges[1] {
		t.Fatal("cloned sequence relationships do not point within the cloned graph")
	}

	var clonedTree *layoutgraph.Tree
	for _, roots := range cloned.Trees {
		if len(roots) > 0 {
			clonedTree = roots[0]
			break
		}
	}
	if clonedTree == nil || cloned.NodeToTree[clonedTree.Node] != clonedTree {
		t.Fatal("cloned tree index does not point to the cloned tree")
	}
	if clonedTree.SentinelEdge != cloned.Edges[2] {
		t.Fatal("cloned tree sentinel does not reuse the cloned graph edge")
	}

	clonedContainer := nodeByID(t, cloned, 1)
	if clonedContainer.Label.PositionFixed() || clonedContainer.Icon.PositionFixed() {
		t.Fatal("clone retained fixed-label or fixed-icon runtime bookkeeping")
	}
	if clonedContainer.HerdAssignment != nil || clonedContainer.LoopOffsets != nil ||
		clonedContainer.LongDistanceNeighborRequirements != nil {
		t.Fatal("clone retained transient placement state")
	}
	if clonedEdge.IsCurve {
		t.Fatal("clone retained transient route diagnostics")
	}
	if cloned.CommonUncleSiblings != nil {
		t.Fatal("clone retained derived common-uncle state")
	}
}

func TestCloneObservesCancellationDuringCopy(t *testing.T) {
	graph := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 10, 10)
	to := layoutgraph.NewNode(2, 10, 10)
	graph.AddNodeUnchecked(from)
	graph.AddNodeUnchecked(to)
	edge := graph.Connect(from, to)
	for i := 0; i < 256; i++ {
		edge.Points = append(edge.Points, geo.NewPoint(float64(i), float64(i)))
	}

	preflightContext := &countContext{Context: context.Background()}
	if err := layoutgraph.ValidateForSerialization(preflightContext, graph); err != nil {
		t.Fatal(err)
	}
	ctx := &countContext{Context: context.Background(), cancelAt: preflightContext.calls + 2}
	cloned, err := layoutgraph.Clone(ctx, graph)
	if cloned != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Clone() = (%v, %v), want (nil, context.Canceled)", cloned, err)
	}
}

func TestCloneRejectsNilContext(t *testing.T) {
	//lint:ignore SA1012 This test verifies Clone's nil-context error.
	cloned, err := layoutgraph.Clone(nil, layoutgraph.NewGraph())
	if cloned != nil || err == nil || !strings.Contains(err.Error(), "requires a context") {
		t.Fatalf("Clone(nil, graph) = (%v, %v), want a context error", cloned, err)
	}
}

func TestCloneRejectsDistinctAuxiliaryRecordsWithReusedIDs(t *testing.T) {
	for _, test := range []struct {
		name       string
		graph      *layoutgraph.Graph
		directWant string
		codecWant  string
	}{
		{
			name:       "cluster vessel",
			graph:      newClusterVesselIDCollisionGraph(),
			directWant: "cluster vessel: distinct node record reuses ID 1",
			codecWant:  "node ID 1 has inconsistent repeated records",
		},
		{
			name:       "sequence vessel",
			graph:      newSequenceVesselIDCollisionGraph(),
			directWant: "sequence vessel: distinct node record reuses ID 1",
			codecWant:  "node ID 1 has inconsistent repeated records",
		},
		{
			name:       "cluster contains itself",
			graph:      newSelfContainingClusterGraph(),
			directWant: "cluster 1 because it cannot contain itself",
			codecWant:  "cluster 1 cannot contain itself",
		},
		{
			name:       "sequence contains itself",
			graph:      newSelfContainingSequenceGraph(),
			directWant: "sequence 1 because it cannot contain itself",
			codecWant:  "sequence 1 cannot contain itself",
		},
		{
			name:       "tree node",
			graph:      newTreeNodeIDCollisionGraph(),
			directWant: "tree node: distinct node record reuses ID 2",
			codecWant:  "node ID 2 has inconsistent repeated records",
		},
		{
			name:       "tree sentinel edge",
			graph:      newTreeSentinelEdgeIDCollisionGraph(),
			directWant: "tree sentinel edge: distinct edge record reuses ID 3",
			codecWant:  "edge ID 3 has inconsistent repeated records",
		},
		{
			name:       "tree node repeats grouping vessel record",
			graph:      newTreeNodeAuxiliaryReuseGraph(),
			directWant: "tree node 2 because it repeats a non-top-level node record",
			codecWant:  "tree node 2 repeats a non-top-level node record",
		},
		{
			name:       "duplicate tree ownership",
			graph:      newDuplicateTreeNodeOwnershipGraph(),
			directWant: "tree node 3 with more than one tree owner",
			codecWant:  "tree node 3 has more than one tree owner",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, err := graphjson.Serialize(t.Context(), test.graph)
			if err != nil {
				t.Fatalf("Serialize() rejected the runtime fixture before the legacy clone boundary: %v", err)
			}
			codecClone := layoutgraph.NewGraph()
			if err := graphjson.Deserialize(t.Context(), before, codecClone); err == nil || !strings.Contains(err.Error(), test.codecWant) {
				t.Fatalf("legacy graph JSON clone error = %v, want %q", err, test.codecWant)
			}

			direct, err := layoutgraph.Clone(context.Background(), test.graph)
			if direct != nil || err == nil || !strings.Contains(err.Error(), test.directWant) {
				t.Fatalf("Clone() = (%v, %v), want nil and error containing %q", direct, err, test.directWant)
			}

			after, err := graphjson.Serialize(t.Context(), test.graph)
			if err != nil {
				t.Fatalf("Serialize() after failed Clone(): %v", err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatal("failed Clone() mutated its source graph")
			}
		})
	}
}

func TestCloneRejectsContainerChildAndHubSpokeAliases(t *testing.T) {
	for _, test := range []struct {
		name       string
		graph      *layoutgraph.Graph
		directWant string
		codecWant  string
	}{
		{
			name:       "container child distinct same-ID alias",
			graph:      newContainerChildAliasGraph(),
			directWant: "container child: exact node record is not available",
			codecWant:  "missing from g.Nodes but is a child",
		},
		{
			name:       "container child inactive grouping vessel",
			graph:      newInactiveVesselContainerChildGraph(),
			directWant: "container child: exact node record is not available",
			codecWant:  "container 0 has node 1 which is missing from g.Nodes",
		},
		{
			name:       "hub spoke distinct same-ID alias",
			graph:      newHubSpokeAliasGraph(),
			directWant: "hub spoke: exact node record is not included in the graph",
			codecWant:  "missing from g.Nodes but is a spoke",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := graphJSONClone(t.Context(), test.graph); err == nil || !strings.Contains(err.Error(), test.codecWant) {
				t.Fatalf("legacy graph JSON clone error = %v, want %q", err, test.codecWant)
			}

			direct, err := layoutgraph.Clone(context.Background(), test.graph)
			if direct != nil || err == nil || !strings.Contains(err.Error(), test.directWant) {
				t.Fatalf("Clone() = (%v, %v), want nil and error containing %q", direct, err, test.directWant)
			}
		})
	}
}

func graphJSONClone(ctx context.Context, source *layoutgraph.Graph) error {
	serialized, err := graphjson.Serialize(ctx, source)
	if err != nil {
		return err
	}
	return graphjson.Deserialize(ctx, serialized, layoutgraph.NewGraph())
}

type countContext struct {
	context.Context
	calls    int
	cancelAt int
}

func (ctx *countContext) Err() error {
	ctx.calls++
	if ctx.cancelAt > 0 && ctx.calls >= ctx.cancelAt {
		return context.Canceled
	}
	return ctx.Context.Err()
}

func newCloneParityGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	graph.CellSize = 40
	graph.IsRootHierarchy = true

	container := layoutgraph.NewNode(1, 300, 200)
	container.TopLeft = geo.NewPoint(0, 0)
	container.Label = &layoutgraph.Label{Text: "container", Position: label.InsideTopCenter, Width: 80, Height: 20}
	container.Label.FixPosition()
	container.Icon = &layoutgraph.Icon{Position: label.InsideTopLeft}
	container.Icon.FixPosition()
	container.HerdAssignment = layoutgraph.NewHerdAssignment()
	container.LoopOffsets = map[geo.Orientation]float64{geo.Top: 10}
	graph.AddNodeUnchecked(container)
	graph.AddNodeToContainer(nil, container)

	external := layoutgraph.NewNode(6, 40, 30)
	external.TopLeft = geo.NewPoint(400, 100)
	graph.AddNodeUnchecked(external)
	graph.AddNodeToContainer(nil, external)

	clusterVessel := layoutgraph.NewNode(9, 100, 40)
	clusterVessel.TopLeft = geo.NewPoint(50, 50)
	graph.AddNodeUnchecked(clusterVessel)
	graph.AddNodeToContainer(container, clusterVessel)
	clusterFirst := layoutgraph.NewNode(2, 40, 40)
	clusterFirst.TopLeft = geo.NewPoint(50, 50)
	clusterFirst.Graph = graph
	clusterSecond := layoutgraph.NewNode(3, 40, 40)
	clusterSecond.TopLeft = geo.NewPoint(110, 50)
	clusterSecond.Graph = graph
	cluster := &layoutgraph.Cluster{
		Vessel:             clusterVessel,
		Nodes:              []*layoutgraph.Node{clusterFirst, clusterSecond},
		Arrangement:        layoutgraph.Row,
		DesiredArrangement: layoutgraph.Column,
		Graph:              graph,
		Padding:            20,
		FixedSize:          true,
		Container:          container,
	}
	clusterFirst.Cluster = cluster
	clusterSecond.Cluster = cluster
	graph.Clusters[clusterVessel] = cluster

	sequenceVessel := layoutgraph.NewNode(10, 100, 40)
	sequenceVessel.TopLeft = geo.NewPoint(50, 120)
	graph.AddNodeUnchecked(sequenceVessel)
	graph.AddNodeToContainer(container, sequenceVessel)
	sequenceFirst := layoutgraph.NewNode(4, 40, 40)
	sequenceFirst.TopLeft = geo.NewPoint(50, 120)
	sequenceFirst.Graph = graph
	sequenceSecond := layoutgraph.NewNode(5, 40, 40)
	sequenceSecond.TopLeft = geo.NewPoint(90, 120)
	sequenceSecond.Graph = graph
	sequence := &layoutgraph.Sequence{
		Vessel:    sequenceVessel,
		Nodes:     []*layoutgraph.Node{sequenceFirst, sequenceSecond},
		Graph:     graph,
		Container: container,
	}
	sequenceFirst.Sequence = sequence
	sequenceSecond.Sequence = sequence
	graph.Sequences[sequenceVessel] = sequence

	clusterEdge := graph.Connect(clusterFirst, external)
	clusterEdge.Points = []*geo.Point{geo.NewPoint(90, 70), geo.NewPoint(400, 115)}
	clusterEdge.Style.Stroke = &layoutgraph.StyleScalar{Value: "blue"}
	clusterEdge.Label = &layoutgraph.Label{Text: "cluster edge", Position: label.UnlockedMiddle, Width: 70, Height: 18}
	clusterEdge.Label.FixPosition()
	clusterEdge.IsCurve = true
	cluster.EdgeAbductions = []*layoutgraph.EdgeAbduction{{
		Edge:           clusterEdge,
		OriginallyFrom: clusterFirst,
		OriginallyTo:   external,
		CurrentFrom:    clusterVessel,
		CurrentTo:      external,
	}}

	sequenceEdge := graph.Connect(sequenceFirst, external)
	sequenceEdge.Points = []*geo.Point{geo.NewPoint(90, 140), geo.NewPoint(400, 115)}
	sequence.EdgeAbductions = []*layoutgraph.EdgeAbduction{{
		Edge:           sequenceEdge,
		OriginallyFrom: sequenceFirst,
		OriginallyTo:   external,
		CurrentFrom:    sequenceVessel,
		CurrentTo:      external,
	}}

	treeNode := layoutgraph.NewNode(7, 30, 30)
	treeNode.TopLeft = geo.NewPoint(460, 100)
	graph.AddNodeUnchecked(treeNode)
	graph.AddNodeToContainer(nil, treeNode)
	treeEdge := graph.Connect(external, treeNode)
	treeEdge.Points = []*geo.Point{geo.NewPoint(440, 115), geo.NewPoint(460, 115)}
	tree := layoutgraph.NewTree(treeNode)
	tree.SentinelEdge = treeEdge
	tree.Orientation = geo.Right
	graph.Trees[external] = []*layoutgraph.Tree{tree}

	clusterFirst.AddNear(sequenceFirst)
	container.LongDistanceNeighborRequirements = map[*layoutgraph.Node]layoutgraph.LongDistanceNeighborRequirements{
		external: {EdgeCount: 1, MaxWidth: 10, MaxHeight: 20},
	}
	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.ReplaceLevels(map[*layoutgraph.Node]int{
		container: 0,
		external:  1,
		treeNode:  2,
	})
	container.Hierarchy = hierarchy
	external.Hierarchy = hierarchy
	treeNode.Hierarchy = hierarchy
	graph.Hubs[external] = []*layoutgraph.Node{clusterVessel, sequenceVessel}
	graph.Directions[nil] = geo.Bottom
	graph.Directions[container] = geo.Right
	graph.CommonUncleSiblings = map[*layoutgraph.Node]layoutgraph.Nodes{external: {treeNode}}
	return graph
}

func newDetachedTreeGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	sentinel := layoutgraph.NewNode(1, 20, 20)
	sentinel.TopLeft = geo.NewPoint(0, 0)
	graph.AddNodeUnchecked(sentinel)

	treeNode := layoutgraph.NewNode(2, 20, 20)
	treeNode.TopLeft = geo.NewPoint(40, 0)
	treeNode.Graph = graph
	sentinelEdge := layoutgraph.NewEdge(sentinel, treeNode)
	sentinelEdge.Points = []*geo.Point{geo.NewPoint(20, 10), geo.NewPoint(40, 10)}
	root := layoutgraph.NewTree(treeNode)
	root.SentinelEdge = sentinelEdge
	graph.Trees[sentinel] = []*layoutgraph.Tree{root}
	return graph
}

func newInactiveGroupingGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	clusterFirst := layoutgraph.NewNode(1, 20, 20)
	clusterSecond := layoutgraph.NewNode(2, 20, 20)
	sequenceFirst := layoutgraph.NewNode(3, 20, 20)
	sequenceSecond := layoutgraph.NewNode(4, 20, 20)
	external := layoutgraph.NewNode(5, 20, 20)
	for index, node := range []*layoutgraph.Node{clusterFirst, clusterSecond, sequenceFirst, sequenceSecond, external} {
		node.TopLeft = geo.NewPoint(float64(index*30), 0)
		graph.AddNodeUnchecked(node)
	}

	clusterVessel := layoutgraph.NewNode(9, 60, 20)
	cluster := &layoutgraph.Cluster{
		Vessel:      clusterVessel,
		Nodes:       []*layoutgraph.Node{clusterFirst, clusterSecond},
		Arrangement: layoutgraph.Row,
		Graph:       graph,
	}
	clusterFirst.Cluster = cluster
	clusterSecond.Cluster = cluster
	graph.Clusters[clusterVessel] = cluster

	sequenceVessel := layoutgraph.NewNode(10, 60, 20)
	sequence := &layoutgraph.Sequence{
		Vessel: sequenceVessel,
		Nodes:  []*layoutgraph.Node{sequenceFirst, sequenceSecond},
		Graph:  graph,
	}
	sequenceFirst.Sequence = sequence
	sequenceSecond.Sequence = sequence
	graph.Sequences[sequenceVessel] = sequence

	clusterEdge := graph.Connect(clusterFirst, external)
	sequenceEdge := graph.Connect(sequenceFirst, external)
	cluster.EdgeAbductions = []*layoutgraph.EdgeAbduction{{
		Edge:           clusterEdge,
		OriginallyFrom: clusterFirst,
		OriginallyTo:   external,
		CurrentFrom:    clusterVessel,
		CurrentTo:      external,
	}}
	sequence.EdgeAbductions = []*layoutgraph.EdgeAbduction{{
		Edge:           sequenceEdge,
		OriginallyFrom: sequenceFirst,
		OriginallyTo:   external,
		CurrentFrom:    sequenceVessel,
		CurrentTo:      external,
	}}
	graph.Hubs[external] = []*layoutgraph.Node{clusterVessel, sequenceVessel}
	return graph
}

func newSharedGroupingVesselGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	clusterFirst := graph.AddNode(layoutgraph.NewNode(1, 20, 20))
	clusterSecond := graph.AddNode(layoutgraph.NewNode(2, 20, 20))
	sequenceFirst := graph.AddNode(layoutgraph.NewNode(3, 20, 20))
	sequenceSecond := graph.AddNode(layoutgraph.NewNode(4, 20, 20))
	sharedVessel := layoutgraph.NewNode(9, 60, 20)

	cluster := &layoutgraph.Cluster{
		Vessel: sharedVessel,
		Nodes:  []*layoutgraph.Node{clusterFirst, clusterSecond},
		Graph:  graph,
	}
	clusterFirst.Cluster = cluster
	clusterSecond.Cluster = cluster
	graph.Clusters[sharedVessel] = cluster

	sequence := &layoutgraph.Sequence{
		Vessel: sharedVessel,
		Nodes:  []*layoutgraph.Node{sequenceFirst, sequenceSecond},
		Graph:  graph,
	}
	sequenceFirst.Sequence = sequence
	sequenceSecond.Sequence = sequence
	graph.Sequences[sharedVessel] = sequence
	return graph
}

func newClusterVesselIDCollisionGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	graph.AddNodeUnchecked(layoutgraph.NewNode(1, 10, 10))
	collidingVessel := layoutgraph.NewNode(1, 20, 10)
	graph.Clusters[collidingVessel] = &layoutgraph.Cluster{
		Vessel: collidingVessel,
		Graph:  graph,
	}
	return graph
}

func newSequenceVesselIDCollisionGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	graph.AddNodeUnchecked(layoutgraph.NewNode(1, 10, 10))
	first := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	second := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
	collidingVessel := layoutgraph.NewNode(1, 20, 10)
	sequence := &layoutgraph.Sequence{
		Vessel: collidingVessel,
		Nodes:  []*layoutgraph.Node{first, second},
		Graph:  graph,
	}
	first.Sequence = sequence
	second.Sequence = sequence
	graph.Sequences[collidingVessel] = sequence
	return graph
}

func newSelfContainingClusterGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	vessel := layoutgraph.NewNode(1, 10, 10)
	graph.Clusters[vessel] = &layoutgraph.Cluster{
		Vessel:    vessel,
		Container: vessel,
		Graph:     graph,
	}
	return graph
}

func newSelfContainingSequenceGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	first := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	second := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
	vessel := layoutgraph.NewNode(1, 10, 10)
	sequence := &layoutgraph.Sequence{
		Vessel:    vessel,
		Container: vessel,
		Nodes:     []*layoutgraph.Node{first, second},
		Graph:     graph,
	}
	first.Sequence = sequence
	second.Sequence = sequence
	graph.Sequences[vessel] = sequence
	return graph
}

func newTreeNodeIDCollisionGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	sentinel := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	declaredNode := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	sentinelEdge := graph.Connect(sentinel, declaredNode)
	sentinelEdge.ID = 3
	collidingNode := layoutgraph.NewNode(2, 20, 10)
	graph.Trees[sentinel] = []*layoutgraph.Tree{{
		Node:         collidingNode,
		SentinelEdge: sentinelEdge,
	}}
	return graph
}

func newTreeSentinelEdgeIDCollisionGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	sentinel := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	treeNode := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	declaredEdge := graph.Connect(sentinel, treeNode)
	declaredEdge.ID = 3
	collidingEdge := layoutgraph.NewEdge(sentinel, treeNode)
	collidingEdge.ID = 3
	collidingEdge.MinWidth = 1
	graph.Trees[sentinel] = []*layoutgraph.Tree{{
		Node:         treeNode,
		SentinelEdge: collidingEdge,
	}}
	return graph
}

func newTreeNodeAuxiliaryReuseGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	sentinel := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	sharedNode := layoutgraph.NewNode(2, 10, 10)
	graph.Clusters[sharedNode] = &layoutgraph.Cluster{
		Vessel: sharedNode,
		Graph:  graph,
	}
	sentinelEdge := layoutgraph.NewEdge(sentinel, sharedNode)
	sentinelEdge.ID = 3
	graph.Trees[sentinel] = []*layoutgraph.Tree{{
		Node:         sharedNode,
		SentinelEdge: sentinelEdge,
	}}
	return graph
}

func newDuplicateTreeNodeOwnershipGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	firstSentinel := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	secondSentinel := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	firstTreeNode := layoutgraph.NewNode(3, 10, 10)
	secondTreeNode := layoutgraph.NewNode(3, 10, 10)
	firstEdge := layoutgraph.NewEdge(firstSentinel, firstTreeNode)
	firstEdge.ID = 4
	secondEdge := layoutgraph.NewEdge(secondSentinel, secondTreeNode)
	secondEdge.ID = 5
	graph.Trees[firstSentinel] = []*layoutgraph.Tree{{
		Node:         firstTreeNode,
		SentinelEdge: firstEdge,
	}}
	graph.Trees[secondSentinel] = []*layoutgraph.Tree{{
		Node:         secondTreeNode,
		SentinelEdge: secondEdge,
	}}
	return graph
}

func newContainerChildAliasGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	graph.Containers[nil] = []*layoutgraph.Node{layoutgraph.NewNode(1, 10, 10)}
	return graph
}

func newInactiveVesselContainerChildGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	vessel := layoutgraph.NewNode(1, 10, 10)
	graph.Clusters[vessel] = &layoutgraph.Cluster{
		Vessel: vessel,
		Graph:  graph,
	}
	graph.Containers[nil] = []*layoutgraph.Node{vessel}
	return graph
}

func newHubSpokeAliasGraph() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	hub := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	graph.Hubs[hub] = []*layoutgraph.Node{layoutgraph.NewNode(2, 10, 10)}
	return graph
}

func nodeByID(t *testing.T, graph *layoutgraph.Graph, id layoutgraph.EntityID) *layoutgraph.Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	for _, cluster := range graph.Clusters {
		for _, node := range cluster.Nodes {
			if node.ID == id {
				return node
			}
		}
	}
	for _, sequence := range graph.Sequences {
		for _, node := range sequence.Nodes {
			if node.ID == id {
				return node
			}
		}
	}
	t.Fatalf("node %d not found", id)
	return nil
}

func onlyCluster(t *testing.T, graph *layoutgraph.Graph) *layoutgraph.Cluster {
	t.Helper()
	if len(graph.Clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(graph.Clusters))
	}
	for _, cluster := range graph.Clusters {
		return cluster
	}
	return nil
}

func onlySequence(t *testing.T, graph *layoutgraph.Graph) *layoutgraph.Sequence {
	t.Helper()
	if len(graph.Sequences) != 1 {
		t.Fatalf("got %d sequences, want 1", len(graph.Sequences))
	}
	for _, sequence := range graph.Sequences {
		return sequence
	}
	return nil
}
