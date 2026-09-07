package placement

import (
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestGraphInitialization(t *testing.T) {
	ctx := withTestLogger(t.Context(), t)
	graph := layoutgraph.NewGraph()
	first := graph.AddNode(layoutgraph.NewNode(1, 5, 5))
	second := graph.AddNode(layoutgraph.NewNode(2, 2, 5))
	graph.Connect(first, second)
	if err := initializeNodes(ctx, graph); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkPlaceNodes(b *testing.B) {
	ctx := withTestLogger(b.Context(), b)
	seed := int64(1)

	for b.Loop() {
		b.StopTimer()
		// Setup a fresh graph for each iteration
		graph := setupSimpleGraphForBenchmark()
		b.StartTimer()

		// Benchmark placeNodes
		err := placeNodes(ctx, graph, nil, seed, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Helper to set up a simple graph with containers and connections
func setupSimpleGraphForBenchmark() *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()

	// Create 20 nodes
	nodes := make([]*layoutgraph.Node, 20)
	for i := 0; i < 20; i++ {
		nodes[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+1), 20, 20)
		graph.AddNode(nodes[i])
	}

	// Set up containers - create a hierarchical structure
	containerRoot1 := nodes[0]   // Node 1 will be a top-level container
	containerRoot2 := nodes[1]   // Node 2 will be a top-level container
	containerNested1 := nodes[2] // Node 3 will be a nested container inside root1
	containerNested2 := nodes[8] // Node 9 will be a nested container inside root2

	containerRoot1.SetContainer(true)
	containerRoot2.SetContainer(true)
	containerNested1.SetContainer(true)
	containerNested2.SetContainer(true)

	// Initialize containers map
	graph.Containers = make(map[*layoutgraph.Node][]*layoutgraph.

		// Root level contains the two top containers and some loose nodes
		Node)

	graph.Containers[nil] = []*layoutgraph.Node{
		containerRoot1, containerRoot2,
		nodes[16], nodes[17], nodes[18], nodes[19], // 4 nodes at root level
	}

	// First top container contains a nested container and 4 regular nodes
	graph.Containers[containerRoot1] = []*layoutgraph.Node{
		containerNested1,
		nodes[3], nodes[4], nodes[5], nodes[6],
	}

	// Second top container contains a nested container and 2 regular nodes
	graph.Containers[containerRoot2] = []*layoutgraph.Node{
		containerNested2,
		nodes[7], nodes[15],
	}

	// First nested container contains 2 nodes
	graph.Containers[containerNested1] = []*layoutgraph.Node{
		nodes[9], nodes[10],
	}

	// Second nested container contains 4 nodes
	graph.Containers[containerNested2] = []*layoutgraph.Node{
		nodes[11], nodes[12], nodes[13], nodes[14],
	}

	// Set container references on nodes
	for container, children := range graph.Containers {
		for _, child := range children {
			child.Container = container
		}
	}

	// Add connections - mixing various types of connections

	// Connections within first nested container
	graph.Connect(nodes[9], nodes[10])

	// Connections within second nested container
	graph.Connect(nodes[11], nodes[12])
	graph.Connect(nodes[12], nodes[13])
	graph.Connect(nodes[13], nodes[14])

	// Connections from nested to parent container
	graph.Connect(nodes[10], nodes[3])
	graph.Connect(nodes[11], nodes[7])

	// Connections between nodes in first top container
	graph.Connect(nodes[3], nodes[4])
	graph.Connect(nodes[4], nodes[5])
	graph.Connect(nodes[5], nodes[6])

	// Connections between top containers
	graph.Connect(nodes[6], nodes[15])

	// Connections to root level
	graph.Connect(nodes[6], nodes[16])
	graph.Connect(nodes[15], nodes[17])

	// Connections at root level
	graph.Connect(nodes[17], nodes[18])
	graph.Connect(nodes[18], nodes[19])

	// Cross-hierarchy connections
	graph.Connect(nodes[10], nodes[14]) // Between nested containers
	graph.Connect(nodes[4], nodes[13])  // Between different levels of nesting

	return graph
}
