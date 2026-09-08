package grouping

import (
	"context"
	"math/rand"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func legacyClusterSignature(g *layoutgraph.Graph, node *layoutgraph.Node) clusterEdgeSignature {
	signature := clusterEdgeSignature{
		fromArrowheads: make(map[layoutgraph.Arrowhead]struct{}),
		toArrowheads:   make(map[layoutgraph.Arrowhead]struct{}),
	}
	for _, edge := range g.Edges {
		if edge.From == node || edge.To == node {
			switch {
			case edge.IsBidirectional():
				signature.bidirectional++
			case edge.IsUndirected():
				signature.undirected++
			case edge.IsDirected():
				signature.directed++
			}
			if edge.From == node {
				signature.fromArrowheads[edge.SourceArrowhead] = struct{}{}
				signature.toArrowheads[edge.TargetArrowhead] = struct{}{}
			} else {
				signature.fromArrowheads[edge.TargetArrowhead] = struct{}{}
				signature.toArrowheads[edge.SourceArrowhead] = struct{}{}
			}
		}
		if edge.From == node {
			signature.from++
		}
		if edge.To == node {
			signature.to++
		}
	}
	return signature
}

func legacyAddIncidentEdge(node *layoutgraph.Node, edge *layoutgraph.Edge) {
	node.Edges = append(node.Edges, edge)
}

func legacyRemoveIncidentEdge(node *layoutgraph.Node, edge *layoutgraph.Edge) {
	for index, candidate := range node.Edges {
		if candidate == edge {
			node.Edges = append(node.Edges[:index], node.Edges[index+1:]...)
			return
		}
	}
}

func legacySequenceNodeForEdge(sequence *layoutgraph.Sequence, edge *layoutgraph.Edge) *layoutgraph.Node {
	for _, abduction := range sequence.EdgeAbductions {
		if abduction.Edge != edge {
			continue
		}
		if abduction.CurrentFrom == sequence.Vessel {
			return abduction.OriginallyFrom
		}
		if abduction.CurrentTo == sequence.Vessel {
			return abduction.OriginallyTo
		}
	}
	return nil
}

func legacyUniqueNeighbors(node *layoutgraph.Node) []*layoutgraph.Node {
	neighbors := make([]*layoutgraph.Node, 0, len(node.Edges))
	seen := make(map[*layoutgraph.Node]struct{}, len(node.Edges))
	for _, edge := range node.Edges {
		adjacent := edge.From
		if node == edge.From {
			adjacent = edge.To
		}
		if sequence, ok := node.Graph.Sequences[adjacent]; ok {
			adjacent = legacySequenceNodeForEdge(sequence, edge)
		}
		if _, ok := seen[adjacent]; ok {
			continue
		}
		neighbors = append(neighbors, adjacent)
		seen[adjacent] = struct{}{}
	}
	return neighbors
}

func legacyHasLoop(node *layoutgraph.Node) bool {
	for _, edge := range node.Edges {
		if edge.IsLoop() {
			return true
		}
	}
	return false
}

func signaturesEqual(first, second clusterEdgeSignature) bool {
	return first.from == second.from &&
		first.to == second.to &&
		first.bidirectional == second.bidirectional &&
		first.undirected == second.undirected &&
		first.directed == second.directed &&
		arrowheadSetsEqual(first.fromArrowheads, second.fromArrowheads) &&
		arrowheadSetsEqual(first.toArrowheads, second.toArrowheads)
}

func legacyClusterSignatureMatches(first, second clusterEdgeSignature) bool {
	if first.bidirectional != second.bidirectional || first.undirected != second.undirected ||
		first.arrowTypeCount() > 1 || second.arrowTypeCount() > 1 ||
		first.from != second.from || first.to != second.to {
		return false
	}
	for arrowhead := range first.fromArrowheads {
		if _, found := second.fromArrowheads[arrowhead]; !found {
			return false
		}
	}
	for arrowhead := range first.toArrowheads {
		if _, found := second.toArrowheads[arrowhead]; !found {
			return false
		}
	}
	for arrowhead := range second.fromArrowheads {
		if _, found := first.fromArrowheads[arrowhead]; !found {
			return false
		}
	}
	for arrowhead := range second.toArrowheads {
		if _, found := first.toArrowheads[arrowhead]; !found {
			return false
		}
	}
	return true
}

func TestClusterDiscoveryIndexMatchesLegacyEdgeScan(t *testing.T) {
	arrowheads := []layoutgraph.Arrowhead{"", layoutgraph.NoArrowhead, layoutgraph.TriangleArrowhead, layoutgraph.Arrowhead("diamond")}
	for seed := int64(0); seed < 100; seed++ {
		random := rand.New(rand.NewSource(seed))
		g := layoutgraph.NewGraph()
		for index := 0; index < 12; index++ {
			node := layoutgraph.NewNode(layoutgraph.EntityID(index+1), float64(10+index), float64(20+index))
			node.TopLeft = geo.NewPoint(float64(index*50), float64((index%3)*50))
			g.AddNewNodeToContainer(nil, node)
		}
		for index := 0; index < 40; index++ {
			from := g.Nodes[random.Intn(len(g.Nodes))]
			to := g.Nodes[random.Intn(len(g.Nodes))]
			edge := g.Connect(from, to)
			edge.SourceArrowhead = arrowheads[random.Intn(len(arrowheads))]
			edge.TargetArrowhead = arrowheads[random.Intn(len(arrowheads))]
			// The legacy neighbor walk uses node adjacency while its direction
			// signature scans Graph.Edges. Exercise that intentionally asymmetric
			// direct-caller state without changing either baseline.
			if from != to && random.Intn(4) == 0 {
				legacyRemoveIncidentEdge(to, edge)
			}
			if random.Intn(9) == 0 {
				column := index
				edge.FromTableColumnIndex = &column
			}
		}

		guard, err := limits.NewWorkGuard(context.Background(), "cluster parity", limits.MaxTransactionWorkUnits)
		if err != nil {
			t.Fatal(err)
		}
		index, err := buildClusterDiscoveryIndex(g, nil, guard)
		if err != nil {
			t.Fatalf("seed %d: build index: %v", seed, err)
		}
		infos := index.infos
		for _, node := range g.Containers[nil] {
			info := infos[node]
			if !slices.Equal(info.neighbors, legacyUniqueNeighbors(node)) {
				t.Fatalf("seed %d node %d: neighbor order differs", seed, node.ID)
			}
			legacySignature := legacyClusterSignature(g, node)
			if !signaturesEqual(info.edgeSignature, legacySignature) {
				t.Fatalf("seed %d node %d: direction/arrowhead signature differs", seed, node.ID)
			}
			legacyTableColumn := false
			for _, edge := range node.Edges {
				legacyTableColumn = legacyTableColumn || edge.FromTableColumnIndex != nil || edge.ToTableColumnIndex != nil
			}
			if info.toTableColumn != legacyTableColumn {
				t.Fatalf("seed %d node %d: table-column classification differs", seed, node.ID)
			}
		}
		for _, first := range g.Containers[nil] {
			for _, second := range g.Containers[nil] {
				want := legacyClusterSignatureMatches(legacyClusterSignature(g, first), legacyClusterSignature(g, second))
				if got := infos[first].edgeSignature.matches(infos[second].edgeSignature); got != want {
					t.Fatalf("seed %d nodes %d/%d: signature match = %v, want %v", seed, first.ID, second.ID, got, want)
				}
			}
		}
	}
}

func TestClusterDiscoveryIndexPreservesSequenceNeighborRecovery(t *testing.T) {
	g := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	second := layoutgraph.NewNode(2, 10, 10)
	step := layoutgraph.NewNode(3, 10, 10)
	vessel := layoutgraph.NewNode(4, 10, 10)
	for index, node := range []*layoutgraph.Node{first, second, step, vessel} {
		node.TopLeft = geo.NewPoint(float64(index*50), 0)
		node.Graph = g
	}
	g.AddNewNodeToContainer(nil, first)
	g.AddNewNodeToContainer(nil, second)
	edge := g.Connect(first, vessel)
	legacyAddIncidentEdge(second, edge) // malformed adjacency still follows the legacy endpoint rule
	sequence := &layoutgraph.Sequence{
		Vessel: vessel,
		Nodes:  []*layoutgraph.Node{step},
		Graph:  g,
		EdgeAbductions: []*layoutgraph.EdgeAbduction{
			{Edge: edge, CurrentFrom: first, CurrentTo: vessel},
			{Edge: edge, OriginallyTo: step, CurrentFrom: first, CurrentTo: vessel},
		},
	}
	g.Sequences[vessel] = sequence

	guard, err := limits.NewWorkGuard(context.Background(), "sequence parity", limits.MaxTransactionWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	index, err := buildClusterDiscoveryIndex(g, nil, guard)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range []*layoutgraph.Node{first, second} {
		want := legacyUniqueNeighbors(node)
		if !slices.Equal(index.infos[node].neighbors, want) {
			t.Fatalf("node %d sequence neighbor recovery differs: got %v want %v", node.ID, index.infos[node].neighbors, want)
		}
	}
}

func TestClusterDiscoveryIndexRefreshesNeighborsAfterAcceptedCluster(t *testing.T) {
	g := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	second := layoutgraph.NewNode(2, 10, 10)
	external := layoutgraph.NewNode(3, 10, 10)
	other := layoutgraph.NewNode(4, 10, 10)
	malformedObserver := layoutgraph.NewNode(5, 10, 10)
	for index, node := range []*layoutgraph.Node{first, second, external, other, malformedObserver} {
		node.TopLeft = geo.NewPoint(float64(index*50), 0)
		g.AddNewNodeToContainer(nil, node)
	}
	firstEdge := g.Connect(first, external)
	legacyAddIncidentEdge(malformedObserver, firstEdge)
	g.Connect(external, other)
	g.Connect(second, external)

	guard, err := limits.NewWorkGuard(context.Background(), "cluster refresh parity", limits.MaxTransactionWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	index, err := buildClusterDiscoveryIndex(g, nil, guard)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(index.infos[external].neighbors, []*layoutgraph.Node{first, other, second}) {
		t.Fatalf("unexpected initial neighbor order: %v", index.infos[external].neighbors)
	}
	if !slices.Equal(index.infos[malformedObserver].neighbors, []*layoutgraph.Node{first}) {
		t.Fatalf("unexpected malformed initial neighbor: %v", index.infos[malformedObserver].neighbors)
	}

	cluster := &layoutgraph.Cluster{
		Nodes:              []*layoutgraph.Node{first, second},
		Graph:              g,
		Arrangement:        layoutgraph.Row,
		DesiredArrangement: layoutgraph.Row,
	}
	cluster.Vessel = CreateVessel(cluster, 100)
	AddCluster(g, cluster)
	edges, err := clusterIncidentEdges(cluster, index.infos, index.edgeOrder, guard)
	if err != nil {
		t.Fatal(err)
	}
	if err := abductClusterEdges(cluster, edges, guard); err != nil {
		t.Fatal(err)
	}
	if err := index.refreshAfterClusterAbduction(g, cluster, edges, guard); err != nil {
		t.Fatal(err)
	}

	want := legacyUniqueNeighbors(external)
	if !slices.Equal(index.infos[external].neighbors, want) {
		t.Fatalf("post-abduction neighbor order differs: got %v want %v", index.infos[external].neighbors, want)
	}
	if !slices.Equal(want, []*layoutgraph.Node{cluster.Vessel, other}) {
		t.Fatalf("unexpected live neighbor order after abduction: %v", want)
	}
	malformedWant := legacyUniqueNeighbors(malformedObserver)
	if !slices.Equal(index.infos[malformedObserver].neighbors, malformedWant) {
		t.Fatalf("malformed post-abduction neighbors differ: got %v want %v", index.infos[malformedObserver].neighbors, malformedWant)
	}
	if !slices.Equal(malformedWant, []*layoutgraph.Node{cluster.Vessel}) {
		t.Fatalf("unexpected malformed live neighbor after abduction: %v", malformedWant)
	}
}

func TestClusterDiscoveryIndexPreservesLeakyContainerClassification(t *testing.T) {
	for _, leaky := range []bool{false, true} {
		g := layoutgraph.NewGraph()
		container := layoutgraph.NewNode(1, 100, 100)
		child := layoutgraph.NewNode(2, 10, 10)
		external := layoutgraph.NewNode(3, 10, 10)
		container.TopLeft = geo.NewPoint(0, 0)
		child.TopLeft = geo.NewPoint(10, 10)
		external.TopLeft = geo.NewPoint(200, 0)
		g.AddNewNodeToContainer(nil, container)
		g.AddNewNodeToContainer(container, child)
		if leaky {
			g.AddNewNodeToContainer(nil, external)
			g.Connect(child, external)
		} else {
			g.Connect(container, child)
		}
		guard, err := limits.NewWorkGuard(context.Background(), "leaky parity", limits.MaxTransactionWorkUnits)
		if err != nil {
			t.Fatal(err)
		}
		order, err := g.ContainerRDFSOrder(nil, guard)
		if err != nil {
			t.Fatal(err)
		}
		index, err := buildClusterDiscoveryIndex(g, order, guard)
		if err != nil {
			t.Fatal(err)
		}
		legacyNoClustering := g.IsTreeSentinel(container) || container.IsClusterVessel() || g.IsSequenceVessel(container) ||
			container.IsTable() || container.Hierarchy != nil || container.FixedTopLeft != nil ||
			container.HasLeakyEdge() || legacyHasLoop(container)
		if index.infos[container].noClustering != legacyNoClustering {
			t.Fatalf("leaky=%v: no-clustering classification differs", leaky)
		}
	}
}
