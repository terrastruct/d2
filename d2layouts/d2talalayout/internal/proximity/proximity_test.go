package proximity

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestAddHubsFindsLeafSpokesInSameContainer(t *testing.T) {
	graph := layoutgraph.NewGraph()
	hub := layoutgraph.NewNode(1, 10, 10)
	spoke := layoutgraph.NewNode(2, 10, 10)
	connected := layoutgraph.NewNode(3, 10, 10)
	other := layoutgraph.NewNode(4, 10, 10)
	for _, node := range []*layoutgraph.Node{hub, spoke, connected, other} {
		graph.AddNewNodeToContainer(nil, node)
	}
	graph.Connect(hub, spoke)
	graph.Connect(hub, connected)
	graph.Connect(connected, other)
	if err := AddHubs(context.Background(), graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Hubs[hub]) != 1 || graph.Hubs[hub][0] != spoke {
		t.Fatalf("hub spokes = %#v, want one spoke", graph.Hubs[hub])
	}
}

func TestAssignNearsGroupsSiblingsWithCommonUncle(t *testing.T) {
	graph := layoutgraph.NewGraph()
	root := layoutgraph.NewNode(1, 100, 100)
	first := layoutgraph.NewNode(2, 10, 10)
	second := layoutgraph.NewNode(3, 10, 10)
	uncle := layoutgraph.NewNode(4, 10, 10)
	graph.AddNewNodeToContainer(nil, root)
	graph.AddNewNodeToContainer(root, first)
	graph.AddNewNodeToContainer(root, second)
	graph.AddNewNodeToContainer(nil, uncle)
	abductions := []*layoutgraph.EdgeAbduction{
		{OriginallyFrom: first, CurrentFrom: root, CurrentTo: uncle},
		{OriginallyFrom: second, CurrentFrom: root, CurrentTo: uncle},
	}
	if err := AssignNears(context.Background(), graph, root, abductions); err != nil {
		t.Fatal(err)
	}
	if _, ok := first.Nears[second]; !ok {
		t.Fatal("first is not near second")
	}
	if _, ok := second.Nears[first]; !ok {
		t.Fatal("second is not near first")
	}
}

func TestAssignNearsCancellationDoesNotChangeNears(t *testing.T) {
	graph := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 10, 10)
	graph.AddNewNodeToContainer(nil, node)
	original := node.Nears
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := AssignNears(ctx, graph, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("AssignNears error = %v, want cancellation", err)
	}
	if len(node.Nears) != len(original) {
		t.Fatal("canceled near assignment changed node state")
	}
}

func TestCommonUncleSiblingsUsesLargestGroup(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 100, 100)
	first := layoutgraph.NewNode(2, 10, 10)
	second := layoutgraph.NewNode(3, 10, 10)
	third := layoutgraph.NewNode(4, 10, 10)
	uncle := layoutgraph.NewNode(5, 10, 10)
	graph.AddNewNodeToContainer(nil, container)
	for _, node := range []*layoutgraph.Node{first, second, third} {
		graph.AddNewNodeToContainer(container, node)
		graph.Connect(node, uncle)
	}
	graph.AddNewNodeToContainer(nil, uncle)
	common := CommonUncleSiblings(graph)
	for _, node := range []*layoutgraph.Node{first, second, third} {
		if len(common[node]) != 3 {
			t.Fatalf("node %d common-uncle siblings = %d, want 3", node.ID, len(common[node]))
		}
	}
}

func TestSyncHerdFencesUsesGraphBounds(t *testing.T) {
	graph := layoutgraph.NewGraph()
	top := layoutgraph.NewNode(1, 10, 10)
	right := layoutgraph.NewNode(2, 10, 10)
	top.TopLeft = geo.NewPoint(20, 30)
	right.TopLeft = geo.NewPoint(100, 80)
	top.HerdAssignment = layoutgraph.NewHerdAssignment()
	top.HerdAssignment.Orientation = geo.Top
	right.HerdAssignment = layoutgraph.NewHerdAssignment()
	right.HerdAssignment.Orientation = geo.Right
	graph.AddNode(top)
	graph.AddNode(right)
	SyncHerdFences(graph)
	if top.HerdAssignment.Val != 30 {
		t.Fatalf("top fence = %g, want 30", top.HerdAssignment.Val)
	}
	if right.HerdAssignment.Val != 110 {
		t.Fatalf("right fence = %g, want 110", right.HerdAssignment.Val)
	}
}
