package proximity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestAssignNearsGroupsSiblingsConnectedToSameUncle(t *testing.T) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(0, 10, 10)
	second := layoutgraph.NewNode(1, 10, 10)
	external := layoutgraph.NewNode(2, 10, 10)
	root := layoutgraph.NewNode(3, 1000, 1000)
	for _, node := range []*layoutgraph.Node{first, second, external, root} {
		graph.AddNodeUnchecked(node)
	}
	graph.AddNodeToContainer(nil, root)
	graph.AddNodeToContainer(nil, external)
	graph.AddNodeToContainer(root, first)
	graph.AddNodeToContainer(root, second)
	graph.Connect(first, external)
	graph.Connect(second, external)

	err := AssignNears(context.Background(), graph, root, []*layoutgraph.EdgeAbduction{
		{OriginallyFrom: first, CurrentTo: external},
		{OriginallyFrom: second, CurrentTo: external},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := first.Nears[second]; !found {
		t.Fatal("second node is not near the first")
	}
	if _, found := second.Nears[first]; !found {
		t.Fatal("first node is not near the second")
	}
	if len(external.Nears) != 0 {
		t.Fatalf("external node has %d Near relationships, want 0", len(external.Nears))
	}
}

func TestAssignNearsUsesDirectChildrenForNestedConnections(t *testing.T) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(0, 10, 10)
	nestedFirst := layoutgraph.NewNode(1, 10, 10)
	nestedSecond := layoutgraph.NewNode(2, 10, 10)
	nestedContainer := layoutgraph.NewNode(3, 10, 10)
	root := layoutgraph.NewNode(4, 10, 10)
	external := layoutgraph.NewNode(5, 10, 10)
	for _, node := range []*layoutgraph.Node{first, nestedFirst, nestedSecond, nestedContainer, root, external} {
		graph.AddNodeUnchecked(node)
	}
	graph.AddNodeToContainer(nil, root)
	graph.AddNodeToContainer(nil, external)
	graph.AddNodeToContainer(root, first)
	graph.AddNodeToContainer(root, nestedContainer)
	graph.AddNodeToContainer(nestedContainer, nestedFirst)
	graph.AddNodeToContainer(nestedContainer, nestedSecond)
	graph.Connect(first, external)
	graph.Connect(nestedFirst, external)
	graph.Connect(nestedSecond, external)

	err := AssignNears(context.Background(), graph, root, []*layoutgraph.EdgeAbduction{
		{OriginallyFrom: first, CurrentTo: external},
		{OriginallyFrom: nestedFirst, CurrentTo: external},
		{OriginallyFrom: nestedSecond, CurrentTo: external},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := first.Nears[nestedContainer]; !found {
		t.Fatal("nested container is not near the direct child")
	}
	if _, found := nestedContainer.Nears[first]; !found {
		t.Fatal("direct child is not near the nested container")
	}
	if len(external.Nears) != 0 {
		t.Fatalf("external node has %d Near relationships, want 0", len(external.Nears))
	}
}

func TestAssignNearsSkipsHierarchyMembers(t *testing.T) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	second := layoutgraph.NewNode(2, 10, 10)
	root := layoutgraph.NewNode(3, 100, 100)
	external := layoutgraph.NewNode(4, 10, 10)
	for _, node := range []*layoutgraph.Node{first, second, root, external} {
		graph.AddNodeUnchecked(node)
	}
	graph.AddNodeToContainer(nil, root)
	graph.AddNodeToContainer(nil, external)
	graph.AddNodeToContainer(root, first)
	graph.AddNodeToContainer(root, second)
	graph.Connect(first, external)
	graph.Connect(second, external)
	first.Hierarchy = &layoutgraph.Hierarchy{}
	second.Hierarchy = first.Hierarchy

	err := AssignNears(context.Background(), graph, root, []*layoutgraph.EdgeAbduction{
		{OriginallyFrom: first, CurrentTo: external},
		{OriginallyFrom: second, CurrentTo: external},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Nears) != 0 || len(second.Nears) != 0 {
		t.Fatalf("hierarchy members received Near relationships: first=%v second=%v", first.Nears, second.Nears)
	}
}

func TestAssignNearsKeepsDirectChildrenNearWhileSkippingNestedHierarchyMembers(t *testing.T) {
	graph := layoutgraph.NewGraph()
	directChild := layoutgraph.NewNode(0, 10, 10)
	firstHierarchyMember := layoutgraph.NewNode(1, 10, 10)
	secondHierarchyMember := layoutgraph.NewNode(2, 10, 10)
	nestedContainer := layoutgraph.NewNode(3, 10, 10)
	root := layoutgraph.NewNode(4, 10, 10)
	external := layoutgraph.NewNode(5, 10, 10)
	for _, node := range []*layoutgraph.Node{
		directChild,
		firstHierarchyMember,
		secondHierarchyMember,
		nestedContainer,
		root,
		external,
	} {
		graph.AddNodeUnchecked(node)
	}

	graph.AddNodeToContainer(nil, root)
	graph.AddNodeToContainer(nil, external)
	graph.AddNodeToContainer(root, directChild)
	graph.AddNodeToContainer(root, nestedContainer)
	graph.AddNodeToContainer(nestedContainer, firstHierarchyMember)
	graph.AddNodeToContainer(nestedContainer, secondHierarchyMember)
	graph.Connect(directChild, external)
	graph.Connect(firstHierarchyMember, external)
	graph.Connect(secondHierarchyMember, external)

	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.ReplaceLevels(map[*layoutgraph.Node]int{
		firstHierarchyMember:  0,
		secondHierarchyMember: 1,
	})
	firstHierarchyMember.Hierarchy = hierarchy
	secondHierarchyMember.Hierarchy = hierarchy

	err := AssignNears(context.Background(), graph, root, []*layoutgraph.EdgeAbduction{
		{OriginallyFrom: directChild, CurrentTo: external},
		{OriginallyFrom: firstHierarchyMember, CurrentTo: external},
		{OriginallyFrom: secondHierarchyMember, CurrentTo: external},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, found := directChild.Nears[nestedContainer]; !found {
		t.Fatal("nested container is not near the direct child")
	}
	if _, found := nestedContainer.Nears[directChild]; !found {
		t.Fatal("direct child is not near the nested container")
	}
	if _, found := firstHierarchyMember.Nears[secondHierarchyMember]; found {
		t.Fatal("hierarchy members were made near")
	}
	if _, found := secondHierarchyMember.Nears[firstHierarchyMember]; found {
		t.Fatal("hierarchy members were made near in reverse")
	}
}

type cancelWhenNearInstalled struct {
	context.Context
	nodes    []*layoutgraph.Node
	observed bool
}

func (ctx *cancelWhenNearInstalled) Err() error {
	for _, node := range ctx.nodes {
		if len(node.Nears) > 0 {
			ctx.observed = true
			return context.Canceled
		}
	}
	return ctx.Context.Err()
}

func nearAssignmentFixture() (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Node, *layoutgraph.Node, []*layoutgraph.EdgeAbduction) {
	graph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(1, 10, 10)
	second := layoutgraph.NewNode(2, 10, 10)
	external := layoutgraph.NewNode(3, 10, 10)
	root := layoutgraph.NewNode(4, 100, 100)
	for _, node := range []*layoutgraph.Node{first, second, external, root} {
		graph.AddNodeUnchecked(node)
	}
	graph.AddNodeToContainer(nil, root)
	graph.AddNodeToContainer(nil, external)
	graph.AddNodeToContainer(root, first)
	graph.AddNodeToContainer(root, second)
	graph.Connect(first, external)
	graph.Connect(second, external)
	return graph, root, first, second, []*layoutgraph.EdgeAbduction{
		{OriginallyFrom: first, CurrentTo: external},
		{OriginallyFrom: second, CurrentTo: external},
	}
}

func TestAssignNearsCancellationAfterCommitRestoresExactMaps(t *testing.T) {
	graph, root, first, second, abductions := nearAssignmentFixture()
	originalFirst := first.Nears
	originalSecond := second.Nears
	ctx := &cancelWhenNearInstalled{Context: context.Background(), nodes: []*layoutgraph.Node{first, second}}

	err := AssignNears(ctx, graph, root, abductions)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AssignNears error = %v, want context.Canceled", err)
	}
	if !ctx.observed {
		t.Fatal("cancellation did not observe a committed Near relationship")
	}
	if len(first.Nears) != 0 || len(second.Nears) != 0 {
		t.Fatalf("Near relationships survived rollback: first=%v second=%v", first.Nears, second.Nears)
	}
	marker := layoutgraph.NewNode(99, 1, 1)
	originalFirst[marker] = struct{}{}
	originalSecond[marker] = struct{}{}
	if _, found := first.Nears[marker]; !found {
		t.Fatal("first.Nears was not restored to its original map")
	}
	if _, found := second.Nears[marker]; !found {
		t.Fatal("second.Nears was not restored to its original map")
	}
	delete(originalFirst, marker)
	delete(originalSecond, marker)

	if err := AssignNears(context.Background(), graph, root, abductions); err != nil {
		t.Fatal(err)
	}
	if _, found := first.Nears[second]; !found {
		t.Fatal("successful assignment did not make the nodes near")
	}
}

func TestAssignNearsWorkLimitLeavesLiveMapsUnchanged(t *testing.T) {
	graph, root, first, second, abductions := nearAssignmentFixture()
	err := assignNearsWithWorkLimit(context.Background(), graph, root, abductions, 1)
	if err == nil || !strings.Contains(err.Error(), "work exceeds limit") {
		t.Fatalf("assignNearsWithWorkLimit error = %v, want work-limit error", err)
	}
	if len(first.Nears) != 0 || len(second.Nears) != 0 {
		t.Fatalf("Near relationships changed on work-limit error: first=%v second=%v", first.Nears, second.Nears)
	}
}
