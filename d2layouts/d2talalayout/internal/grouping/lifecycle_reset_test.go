package grouping

import (
	"bytes"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func newResetClustersRelayoutFixture(t *testing.T, clusterCount int) *layoutgraph.Graph {
	t.Helper()
	graph := layoutgraph.NewGraph()
	for i := 0; i < clusterCount; i++ {
		anchor := layoutgraph.NewNode(layoutgraph.EntityID(i*3+1), 20, 20)
		first := layoutgraph.NewNode(layoutgraph.EntityID(i*3+2), 10, 10)
		second := layoutgraph.NewNode(layoutgraph.EntityID(i*3+3), 10, 10)
		graph.AddNewNodeToContainer(nil, anchor)
		graph.AddNewNodeToContainer(nil, first)
		graph.AddNewNodeToContainer(nil, second)
		graph.Connect(anchor, first)
		graph.Connect(anchor, second)
	}
	if err := AddClusters(t.Context(), graph, 1, rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}
	if len(graph.Clusters) != clusterCount {
		t.Fatalf("discovered %d clusters, want %d", len(graph.Clusters), clusterCount)
	}
	Cleanup(graph)
	if err := layoutgraph.Validate(t.Context(), "ResetClustersFixture", graph); err != nil {
		t.Fatal(err)
	}
	return graph
}

// resetClustersLegacy freezes the prior per-vessel filtering behavior
// so the one-pass implementation can be compared against it directly.
func resetClustersLegacy(graph *layoutgraph.Graph) {
	if len(graph.Clusters) == 0 {
		return
	}
	for vessel, cluster := range graph.Clusters {
		if cluster == nil {
			continue
		}
		for _, abduction := range cluster.EdgeAbductions {
			if abduction == nil || abduction.Edge == nil {
				continue
			}
			if abduction.OriginallyFrom != nil {
				abduction.Edge.Reconnect(abduction.OriginallyFrom, false)
			}
			if abduction.OriginallyTo != nil {
				abduction.Edge.Reconnect(abduction.OriginallyTo, true)
			}
		}
		for _, node := range cluster.Nodes {
			if node != nil && node.Cluster == cluster {
				node.Cluster = nil
			}
			if node != nil && node.Nears != nil {
				delete(node.Nears, vessel)
			}
		}
		if vessel == nil {
			continue
		}
		for near := range vessel.Nears {
			if near != nil && near.Nears != nil {
				delete(near.Nears, vessel)
			}
		}
		vessel.Nears = map[*layoutgraph.Node]struct{}{}
		filteredNodes := graph.Nodes[:0]
		for _, node := range graph.Nodes {
			if node != vessel {
				filteredNodes = append(filteredNodes, node)
			}
		}
		graph.Nodes = filteredNodes
		for container, children := range graph.Containers {
			filtered := children[:0]
			for _, child := range children {
				if child != vessel {
					filtered = append(filtered, child)
				}
			}
			graph.Containers[container] = filtered
		}
		vessel.Container = nil
		vessel.Graph = nil
		vessel.UnmarkClusterVessel()
	}
	clear(graph.Clusters)
}

func TestResetClustersMatchesLegacyRelayoutState(t *testing.T) {
	for _, clusterCount := range []int{1, 2, 10, 100} {
		t.Run(fmt.Sprint(clusterCount), func(t *testing.T) {
			legacy := newResetClustersRelayoutFixture(t, clusterCount)
			got := newResetClustersRelayoutFixture(t, clusterCount)
			vessels := make([]*layoutgraph.Node, 0, len(got.Clusters))
			for vessel, cluster := range got.Clusters {
				if vessel != nil && cluster != nil {
					vessels = append(vessels, vessel)
				}
			}
			resetClustersLegacy(legacy)
			ResetClusters(got)

			legacyJSON, err := graphjson.Marshal(t.Context(), legacy)
			if err != nil {
				t.Fatal(err)
			}
			gotJSON, err := graphjson.Marshal(t.Context(), got)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(gotJSON, legacyJSON) {
				t.Fatal("one-pass ResetClusters output differs from the legacy reset")
			}
			for _, vessel := range vessels {
				if vessel.Graph != nil || vessel.Container != nil || vessel.IsClusterVessel() || vessel.Nears == nil || len(vessel.Nears) != 0 {
					t.Fatalf("inactive vessel %d metadata was not reset", vessel.ID)
				}
			}
		})
	}
}

func TestResetClustersPreservesLegacyEdgeCases(t *testing.T) {
	graph := layoutgraph.NewGraph()
	ordinary := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
	keepNilCluster := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
	survivor := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
	member := graph.AddNode(layoutgraph.NewNode(4, 10, 10))
	mismatchedMember := graph.AddNode(layoutgraph.NewNode(5, 10, 10))
	vesselA := graph.AddNode(layoutgraph.NewNode(6, 10, 10))
	vesselB := graph.AddNode(layoutgraph.NewNode(7, 10, 10))
	root := graph.AddNode(layoutgraph.NewNode(8, 10, 10))
	nested := graph.AddNode(layoutgraph.NewNode(9, 10, 10))
	nilChildren := graph.AddNode(layoutgraph.NewNode(10, 10, 10))
	emptyChildren := graph.AddNode(layoutgraph.NewNode(11, 10, 10))

	clusterA := &layoutgraph.Cluster{Vessel: vesselA, Graph: graph}
	clusterB := &layoutgraph.Cluster{Vessel: vesselB, Graph: graph}
	otherCluster := &layoutgraph.Cluster{Graph: graph}
	member.Cluster = clusterA
	mismatchedMember.Cluster = otherCluster
	clusterA.Nodes = []*layoutgraph.Node{member, mismatchedMember, nil}
	clusterB.Nodes = []*layoutgraph.Node{nil}
	vesselA.SetClusterVessel(true)
	vesselB.SetClusterVessel(true)
	keepNilCluster.SetClusterVessel(true)
	vesselA.Container = root
	vesselB.Container = nested
	member.AddNear(vesselA)
	ordinary.AddNear(vesselA)
	vesselB.Nears = nil

	fromEdge := graph.Connect(vesselA, ordinary)
	toEdge := graph.Connect(ordinary, vesselA)
	clusterA.EdgeAbductions = []*layoutgraph.EdgeAbduction{
		{Edge: fromEdge, OriginallyFrom: member},
		{Edge: toEdge, OriginallyTo: member},
		nil,
		{},
	}
	graph.Clusters = map[*layoutgraph.Node]*layoutgraph.Cluster{
		vesselA:        clusterA,
		vesselB:        clusterB,
		keepNilCluster: nil,
		nil:            {},
	}

	graph.Nodes = append(make([]*layoutgraph.Node, 0, 12), ordinary, vesselA, nil, keepNilCluster, vesselB, vesselA, survivor)
	graph.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		root:          append(make([]*layoutgraph.Node, 0, 10), ordinary, vesselB, nil, keepNilCluster, vesselA, survivor, vesselB),
		nested:        {vesselA, ordinary},
		nilChildren:   nil,
		emptyChildren: make([]*layoutgraph.Node, 0, 4),
	}
	ResetClusters(graph)

	if !slices.Equal(graph.Nodes, []*layoutgraph.Node{ordinary, nil, keepNilCluster, survivor}) {
		t.Fatalf("Graph.Nodes = %v; want stable survivors", graph.Nodes)
	}
	if !slices.Equal(graph.Containers[root], []*layoutgraph.Node{ordinary, nil, keepNilCluster, survivor}) {
		t.Fatalf("root children = %v; want stable survivors", graph.Containers[root])
	}
	if !slices.Equal(graph.Containers[nested], []*layoutgraph.Node{ordinary}) {
		t.Fatalf("nested children = %v; want ordinary survivor", graph.Containers[nested])
	}
	if graph.Containers[nilChildren] != nil || graph.Containers[emptyChildren] == nil || len(graph.Containers[emptyChildren]) != 0 {
		t.Fatal("ResetClusters changed nil or non-nil-empty container slices")
	}
	if len(graph.Clusters) != 0 {
		t.Fatalf("clusters retained after reset: %d", len(graph.Clusters))
	}
	if member.Cluster != nil || mismatchedMember.Cluster != otherCluster {
		t.Fatal("ResetClusters did not preserve pointer-matched cluster ownership semantics")
	}
	if _, found := member.Nears[vesselA]; found {
		t.Fatal("cluster member retained its vessel Near")
	}
	if _, found := ordinary.Nears[vesselA]; found {
		t.Fatal("external Near retained the retired vessel")
	}
	if vesselA.Nears == nil || len(vesselA.Nears) != 0 || vesselB.Nears == nil || len(vesselB.Nears) != 0 {
		t.Fatal("retired vessel Near maps were not reset to non-nil empty maps")
	}
	if fromEdge.From != member || toEdge.To != member {
		t.Fatal("ResetClusters did not reconnect both abducted edge endpoints")
	}
	if vesselA.Graph != nil || vesselA.Container != nil || vesselA.IsClusterVessel() ||
		vesselB.Graph != nil || vesselB.Container != nil || vesselB.IsClusterVessel() {
		t.Fatal("retired vessel metadata was not cleared")
	}
	if keepNilCluster.Graph != graph || !keepNilCluster.IsClusterVessel() {
		t.Fatal("nil-valued cluster entry changed its map-key node")
	}
}

func TestResetClustersBulkFilterBranches(t *testing.T) {
	t.Run("one retired vessel among multiple entries", func(t *testing.T) {
		graph := layoutgraph.NewGraph()
		ordinary := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
		vessel := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
		stale := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
		root := graph.AddNode(layoutgraph.NewNode(4, 10, 10))
		vessel.SetClusterVessel(true)
		vessel.Container = root
		cluster := &layoutgraph.Cluster{Vessel: vessel, Graph: graph}
		graph.Clusters = map[*layoutgraph.Node]*layoutgraph.Cluster{vessel: cluster, stale: nil}
		graph.Nodes = []*layoutgraph.Node{ordinary, vessel, stale, vessel}
		graph.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{root: {vessel, ordinary, stale, vessel}}

		ResetClusters(graph)

		if !slices.Equal(graph.Nodes, []*layoutgraph.Node{ordinary, stale}) {
			t.Fatalf("Graph.Nodes = %v; want only the retired vessel filtered", graph.Nodes)
		}
		if !slices.Equal(graph.Containers[root], []*layoutgraph.Node{ordinary, stale}) {
			t.Fatalf("root children = %v; want only the retired vessel filtered", graph.Containers[root])
		}
		if stale.Graph != graph || vessel.Graph != nil || vessel.Container != nil || vessel.IsClusterVessel() {
			t.Fatal("single bulk-filter vessel or nil-valued stale key metadata changed incorrectly")
		}
		if len(graph.Clusters) != 0 {
			t.Fatal("cluster metadata was not cleared")
		}
	})

	t.Run("no retired vessels among multiple entries", func(t *testing.T) {
		graph := layoutgraph.NewGraph()
		ordinary := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
		member := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
		keep := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
		stale := graph.AddNode(layoutgraph.NewNode(4, 10, 10))
		root := graph.AddNode(layoutgraph.NewNode(5, 10, 10))
		cluster := &layoutgraph.Cluster{Graph: graph}
		cluster.Nodes = []*layoutgraph.Node{member}
		member.Cluster = cluster
		member.Nears = map[*layoutgraph.Node]struct{}{nil: {}}
		edge := graph.Connect(ordinary, keep)
		cluster.EdgeAbductions = []*layoutgraph.EdgeAbduction{{Edge: edge, OriginallyFrom: member}}
		graph.Clusters = map[*layoutgraph.Node]*layoutgraph.Cluster{stale: nil, nil: cluster}
		graph.Nodes = []*layoutgraph.Node{ordinary, nil, stale, member, keep}
		graph.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{root: {nil, stale, ordinary, member, keep}}
		wantNodes := slices.Clone(graph.Nodes)
		wantChildren := slices.Clone(graph.Containers[root])

		ResetClusters(graph)

		if !slices.Equal(graph.Nodes, wantNodes) || !slices.Equal(graph.Containers[root], wantChildren) {
			t.Fatal("zero-vessel bulk branch filtered graph or container lists")
		}
		if member.Cluster != nil {
			t.Fatal("nil-key cluster did not release its member")
		}
		if _, found := member.Nears[nil]; found {
			t.Fatal("nil-key cluster did not remove its member Near")
		}
		if edge.From != member {
			t.Fatal("nil-key cluster did not reconnect its abducted edge")
		}
		if stale.Graph != graph || len(graph.Clusters) != 0 {
			t.Fatal("nil-valued stale key changed or cluster metadata remained")
		}
	})
}

func TestResetClustersSingleEntryEdgeCases(t *testing.T) {
	t.Run("nil cluster value", func(t *testing.T) {
		graph := layoutgraph.NewGraph()
		stale := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
		stale.SetClusterVessel(true)
		graph.Clusters = map[*layoutgraph.Node]*layoutgraph.Cluster{stale: nil}

		ResetClusters(graph)

		if !slices.Equal(graph.Nodes, []*layoutgraph.Node{stale}) || stale.Graph != graph || !stale.IsClusterVessel() {
			t.Fatal("single nil-valued cluster entry changed its key node")
		}
		if len(graph.Clusters) != 0 {
			t.Fatal("cluster metadata was not cleared")
		}
	})

	t.Run("nil cluster key", func(t *testing.T) {
		graph := layoutgraph.NewGraph()
		ordinary := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
		member := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
		keep := graph.AddNode(layoutgraph.NewNode(3, 10, 10))
		cluster := &layoutgraph.Cluster{Graph: graph, Nodes: []*layoutgraph.Node{member}}
		member.Cluster = cluster
		member.Nears = map[*layoutgraph.Node]struct{}{nil: {}}
		edge := graph.Connect(ordinary, keep)
		cluster.EdgeAbductions = []*layoutgraph.EdgeAbduction{{Edge: edge, OriginallyTo: member}}
		graph.Clusters = map[*layoutgraph.Node]*layoutgraph.Cluster{nil: cluster}
		wantNodes := slices.Clone(graph.Nodes)

		ResetClusters(graph)

		if !slices.Equal(graph.Nodes, wantNodes) {
			t.Fatal("single nil-key cluster filtered graph nodes")
		}
		if member.Cluster != nil {
			t.Fatal("single nil-key cluster did not release its member")
		}
		if _, found := member.Nears[nil]; found {
			t.Fatal("single nil-key cluster did not remove its member Near")
		}
		if edge.To != member || len(graph.Clusters) != 0 {
			t.Fatal("single nil-key cluster did not restore its edge or clear metadata")
		}
	})
}
