package layoutgraph

import (
	"context"
	"testing"

	"github.com/d2lang/d2/lib/geo"
)

type ownedNodeRecord struct {
	name  string
	node  *Node
	owner *Graph
}

func runtimeOwnershipFixture() (*Graph, []ownedNodeRecord) {
	graph := NewGraph()
	graph.NodeToTree = make(map[*Node]*Tree)
	graph.CommonUncleSiblings = make(map[*Node]Nodes)

	nextID := EntityID(1)
	var records []ownedNodeRecord
	owned := func(name string) *Node {
		node := NewNode(nextID, 10, 10)
		nextID++
		owner := NewGraph()
		node.Graph = owner
		records = append(records, ownedNodeRecord{name: name, node: node, owner: owner})
		return node
	}

	direct := owned("Graph.Nodes")
	graph.Nodes = append(graph.Nodes, direct)

	edgeEndpoint := owned("Graph.Edges endpoint")
	edge := NewEdge(direct, edgeEndpoint)
	graph.Edges = append(graph.Edges, edge)
	direct.Edges = append(direct.Edges, edge)
	edgeEndpoint.Edges = append(edgeEndpoint.Edges, edge)

	containerKey := owned("Graph.Containers key")
	containerChild := owned("Graph.Containers child")
	graph.Containers[containerKey] = []*Node{containerChild}
	direct.Container = owned("Node.Container")

	near := owned("Node.Nears")
	direct.Nears[near] = struct{}{}
	longDistanceNeighbor := owned("Node.LongDistanceNeighborRequirements")
	direct.LongDistanceNeighborRequirements = map[*Node]LongDistanceNeighborRequirements{
		longDistanceNeighbor: {EdgeCount: 1},
	}

	clusterVessel := owned("Graph.Clusters vessel")
	clusterMember := owned("Cluster.Nodes")
	clusterContainer := owned("Cluster.Container")
	cluster := &Cluster{
		Vessel:    clusterVessel,
		Nodes:     []*Node{clusterMember},
		Container: clusterContainer,
	}
	clusterMember.Cluster = cluster
	direct.Cluster = cluster
	graph.Clusters[clusterVessel] = cluster

	sequenceVessel := owned("Graph.Sequences vessel")
	sequenceMember := owned("Sequence.Nodes")
	sequenceContainer := owned("Sequence.Container")
	sequence := &Sequence{
		Vessel:    sequenceVessel,
		Nodes:     []*Node{sequenceMember},
		Container: sequenceContainer,
	}
	sequenceMember.Sequence = sequence
	direct.Sequence = sequence
	graph.Sequences[sequenceVessel] = sequence

	abductionOriginalFrom := owned("EdgeAbduction.OriginallyFrom")
	abductionOriginalTo := owned("EdgeAbduction.OriginallyTo")
	abductionCurrentFrom := owned("EdgeAbduction.CurrentFrom")
	abductionCurrentTo := owned("EdgeAbduction.CurrentTo")
	abduction := &EdgeAbduction{
		Edge:           NewEdge(abductionCurrentFrom, abductionCurrentTo),
		OriginallyFrom: abductionOriginalFrom,
		OriginallyTo:   abductionOriginalTo,
		CurrentFrom:    abductionCurrentFrom,
		CurrentTo:      abductionCurrentTo,
	}
	cluster.EdgeAbductions = []*EdgeAbduction{abduction}

	treeKey := owned("Graph.Trees key")
	treeNode := owned("Tree.Node")
	treeSentinel := owned("Tree.SentinelEdge endpoint")
	treeParentNode := owned("Tree.Parent")
	treeChildNode := owned("Tree.Children")
	treeParent := NewTree(treeParentNode)
	treeChild := NewTree(treeChildNode)
	treeRoot := NewTree(treeNode)
	treeRoot.Parent = treeParent
	treeRoot.Children = []*Tree{treeChild}
	treeRoot.SentinelEdge = NewEdge(treeNode, treeSentinel)
	graph.Trees[treeKey] = []*Tree{treeRoot}
	nodeToTreeKey := owned("Graph.NodeToTree key")
	graph.NodeToTree[nodeToTreeKey] = treeRoot

	hub := owned("Graph.Hubs key")
	spoke := owned("Graph.Hubs node")
	graph.Hubs[hub] = []*Node{spoke}

	commonKey := owned("Graph.CommonUncleSiblings key")
	commonSibling := owned("Graph.CommonUncleSiblings node")
	graph.CommonUncleSiblings[commonKey] = Nodes{commonSibling}
	graph.Directions[owned("Graph.Directions key")] = geo.Top

	herdOpposite := owned("HerdAssignment opposite node")
	herdSame := owned("HerdAssignment same node")
	direct.HerdAssignment = &HerdAssignment{
		oppositeSidePaired: map[*Node]struct{}{herdOpposite: {}},
		sameSidePaired:     map[*Node]struct{}{herdSame: {}},
	}
	hierarchyNode := owned("Hierarchy level node")
	direct.Hierarchy = &Hierarchy{level: map[*Node]int{hierarchyNode: 1}}

	return graph, records
}

func TestSnapshotNodeGraphOwnershipCoversRuntimeTopology(t *testing.T) {
	graph, records := runtimeOwnershipFixture()
	snapshot, err := SnapshotNodeGraphOwnership(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.owners) != len(records) {
		t.Fatalf("captured owners = %d, want every one of %d distinct runtime nodes", len(snapshot.owners), len(records))
	}

	temporary := NewGraph()
	for _, record := range records {
		record.node.Graph = temporary
	}
	snapshot.Restore()
	for _, record := range records {
		if record.node.Graph != record.owner {
			t.Fatalf("%s owner = %p, want exact original %p", record.name, record.node.Graph, record.owner)
		}
	}
}

func TestSnapshotNodeGraphOwnershipCancellationIsReadOnly(t *testing.T) {
	graph, records := runtimeOwnershipFixture()
	snapshot, err := SnapshotNodeGraphOwnership(
		&cancelAfterErrChecks{Context: context.Background(), remaining: 4},
		graph,
	)
	requireCanceledAt(t, err, nodeGraphOwnershipSnapshotLocation)
	if len(snapshot.owners) != 0 {
		t.Fatalf("partial snapshot retained %d owners", len(snapshot.owners))
	}
	for _, record := range records {
		if record.node.Graph != record.owner {
			t.Fatalf("canceled snapshot changed %s owner", record.name)
		}
	}
}

func TestSnapshotNodeGraphOwnershipRejectsNilGraph(t *testing.T) {
	if _, err := SnapshotNodeGraphOwnership(context.Background(), nil); err == nil {
		t.Fatal("nil graph snapshot succeeded")
	}
}
