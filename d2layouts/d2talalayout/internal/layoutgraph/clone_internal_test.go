package layoutgraph

import (
	"context"
	"strings"
	"testing"
)

func TestClonePreservesCostsAndResetsDerivedState(t *testing.T) {
	source := NewGraph()
	node := NewNode(1, 10, 10)
	source.AddNodeUnchecked(node)
	source.crossingCost = 11
	source.turnCost = 12
	source.nonCenterPortCost = 13
	source.edgeLengthCache[99] = 14
	node.margin = Spacing{top: 1, bottom: 2, left: 3, right: 4}
	node.padding = Spacing{top: 5, bottom: 6, left: 7, right: 8}

	cloned, err := Clone(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.crossingCost != 11 || cloned.turnCost != 12 || cloned.nonCenterPortCost != 13 {
		t.Fatalf("clone costs = (%v, %v, %v), want (11, 12, 13)", cloned.crossingCost, cloned.turnCost, cloned.nonCenterPortCost)
	}
	if len(cloned.edgeLengthCache) != 0 {
		t.Fatal("clone retained the derived edge-length cache")
	}
	if cloned.Nodes[0].margin != (Spacing{}) || cloned.Nodes[0].padding != (Spacing{}) {
		t.Fatal("clone retained transient node spacing state")
	}

	cloned.edgeLengthCache[1] = 2
	if _, shared := source.edgeLengthCache[1]; shared {
		t.Fatal("clone shares its edge-length cache with source")
	}
}

func TestCloneRejectsDuplicateZeroIDEdgeRecord(t *testing.T) {
	source := NewGraph()
	from := source.AddNode(NewNode(1, 10, 10))
	to := source.AddNode(NewNode(2, 10, 10))
	edge := source.Connect(from, to)
	source.Edges = append(source.Edges, edge)

	cloned, err := Clone(context.Background(), source)
	if cloned != nil || err == nil || !strings.Contains(err.Error(), "duplicate edge record") {
		t.Fatalf("Clone() = (%v, %v), want duplicate edge record error", cloned, err)
	}
}

func TestCloneRejectsGroupingVesselKeyMismatch(t *testing.T) {
	for _, test := range []struct {
		name string
		set  func(*Graph, *Node, *Node)
	}{
		{
			name: "cluster",
			set: func(graph *Graph, key, vessel *Node) {
				graph.Clusters[key] = &Cluster{Vessel: vessel, Graph: graph}
			},
		},
		{
			name: "sequence",
			set: func(graph *Graph, key, vessel *Node) {
				first := graph.AddNode(NewNode(3, 10, 10))
				second := graph.AddNode(NewNode(4, 10, 10))
				graph.Sequences[key] = &Sequence{Vessel: vessel, Nodes: []*Node{first, second}, Graph: graph}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := NewGraph()
			key := source.AddNode(NewNode(1, 10, 10))
			vessel := source.AddNode(NewNode(2, 10, 10))
			test.set(source, key, vessel)

			cloned, err := Clone(context.Background(), source)
			if cloned != nil || err == nil || !strings.Contains(err.Error(), "record vessel differs") {
				t.Fatalf("Clone() = (%v, %v), want vessel mismatch error", cloned, err)
			}
		})
	}
}
