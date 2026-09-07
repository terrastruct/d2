package layoutgraph

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func connectedMembershipFixture(n int) (*Graph, *Node) {
	g := NewGraph()
	root := NewNode(1, 100, 100)
	g.AddNewNodeToContainer(nil, root)
	for i := 0; i < n; i++ {
		child := NewNode(EntityID(i+2), 10, 10)
		g.AddNewNodeToContainer(root, child)
		if i > 0 {
			g.AddEdge(NewEdge(g.Nodes[i], child))
		}
	}
	return g, g.Nodes[1]
}

func TestConnectedNodeSetTraversalParity(t *testing.T) {
	for _, count := range []int{4, 12, 128} {
		g, node := connectedMembershipFixture(count)
		for _, excluded := range [][]*Node{nil, {g.Nodes[0]}, {node}, {g.Nodes[len(g.Nodes)-1]}, {g.Nodes[2], g.Nodes[3]}} {
			got := node.ConnectedNodeSet(excluded, g)
			want := node.ConnectedNodes(excluded, g)
			sortNodesByID(got)
			sortNodesByID(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("n=%d, exclusions=%v: reached set changed", count, excluded)
			}
		}
	}
}

func TestConnectedNodeSetPreservesOverlappingOwnerViews(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 30; trial++ {
		g, _ := connectedMembershipFixture(40)
		container := NewNode(100, 100, 100)
		g.AddNewNodeToContainer(nil, container)
		container.isContainer = true
		for i := 0; i < 20; i++ {
			g.Containers[container] = append(g.Containers[container], g.Nodes[1+rng.Intn(40)])
		}
		vessel := NewNode(101, 100, 100)
		g.AddNewNodeToContainer(nil, vessel)
		vessel.isClusterVessel = true
		g.Clusters[vessel] = &Cluster{Vessel: vessel, Nodes: []*Node{container}, Graph: g}
		for _, start := range g.Nodes {
			for _, excluded := range [][]*Node{nil, {g.Nodes[11]}, {container}, {vessel}, {g.Nodes[40]}} {
				// The original owner-map traversal intentionally has no defined ordering
				// when multiple owner views insert nodes during the same BFS step.
				got := start.ConnectedNodeSet(excluded, g)
				want := start.ConnectedNodes(excluded, g)
				sortNodesByID(got)
				sortNodesByID(want)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("trial=%d, start=%d: reached set changed", trial, start.ID)
				}
			}
		}
	}
}

func BenchmarkConnectedNodeSet(b *testing.B) {
	for _, n := range []int{4, 128, 1024} {
		g, node := connectedMembershipFixture(n)
		for _, indexed := range []bool{false, true} {
			b.Run(fmt.Sprintf("n=%d/indexed=%t", n, indexed), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var nodes []*Node
					if indexed {
						nodes = node.ConnectedNodeSet(nil, g)
					} else {
						nodes = node.ConnectedNodes(nil, g)
					}
					if len(nodes) != n+1 {
						b.Fatalf("reached %d nodes", len(nodes))
					}
				}
			})
		}
	}
}
