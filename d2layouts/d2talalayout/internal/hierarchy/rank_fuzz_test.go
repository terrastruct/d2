package hierarchy

import (
	"context"
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func FuzzRankDAG(f *testing.F) {
	f.Add([]byte{1})
	f.Add([]byte{4, 0, 2, 255, 17, 83})
	f.Add([]byte{7, 1, 1, 1, 1, 1, 1, 1, 1, 1})
	// This encodes 0->1, 0->2, 2->3, 3->4, and 1->4. The last edge
	// is slack without a tight replacement path, forcing a simplex exchange.
	f.Add([]byte{3, 0, 0, 0, 0, 2, 0, 3, 0, 0, 0, 0, 0, 1, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		nodeCount := 2 + int(data[0]%7)
		g, nodes := newDirectedGraph(nodeCount, nil)
		cursor := 1
		nextByte := func() byte {
			value := data[cursor%len(data)]
			cursor++
			return value
		}
		connected := make([][]bool, nodeCount)
		for node := range connected {
			connected[node] = make([]bool, nodeCount)
		}
		connect := func(from, to int) {
			edge := g.Connect(nodes[from], nodes[to])
			edge.ID = layoutgraph.EntityID(len(g.Edges))
			edge.TargetArrowhead = layoutgraph.TriangleArrowhead
			edge.SetHierarchyRankWeight(1 + int(nextByte()))
			connected[from][to] = true
		}
		// A random-parent tree keeps the DAG connected without forcing a
		// Hamiltonian chain. That distinction matters: a chain gives every
		// transitive edge a tight replacement path and prevents fuzzing the
		// network-simplex exchange loop.
		for to := 1; to < nodeCount; to++ {
			from := int(nextByte()) % to
			connect(from, to)
		}
		for from := 0; from < nodeCount; from++ {
			for to := from + 1; to < nodeCount; to++ {
				if connected[from][to] {
					continue
				}
				if nextByte()&1 != 0 {
					connect(from, to)
				}
			}
		}

		first, err := rankDAG(context.Background(), g)
		if err != nil {
			t.Fatal(err)
		}
		for _, edge := range g.Edges {
			if span := first.nodeToLevel[edge.To] - first.nodeToLevel[edge.From]; span < 1 {
				t.Fatalf("edge %d has infeasible span %d", edge.ID, span)
			}
		}

		slices.Reverse(g.Nodes)
		slices.Reverse(g.Edges)
		for _, node := range g.Nodes {
			slices.Reverse(node.Edges)
		}
		second, err := rankDAG(context.Background(), g)
		if err != nil {
			t.Fatal(err)
		}
		for _, node := range nodes {
			if first.nodeToLevel[node] != second.nodeToLevel[node] {
				t.Fatalf("node %d level changed from %d to %d", node.ID, first.nodeToLevel[node], second.nodeToLevel[node])
			}
		}
	})
}
