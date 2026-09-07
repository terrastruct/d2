package packing

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestCombineSubgraphsValidatesNodeOwnedTopologyAcrossSharedNilMaps(t *testing.T) {
	master, first, second := &layoutgraph.Graph{}, &layoutgraph.Graph{}, &layoutgraph.Graph{}
	firstNode := layoutgraph.NewNode(1, 1, 1)
	firstNode.Graph, first.Nodes = first, []*layoutgraph.Node{firstNode}
	secondNode := layoutgraph.NewNode(2, 1, 1)
	secondNode.Graph = second
	secondNode.Nears[nil] = struct{}{}
	second.Nodes = []*layoutgraph.Node{secondNode}
	combined, err := CombineSubgraphs(context.Background(), master, []*layoutgraph.Graph{first, second}, nil)
	if combined != nil || err == nil || !strings.Contains(err.Error(), "nil near node") {
		t.Fatalf("combined=%v err=%v, want nil near-node rejection", combined, err)
	}
}

func TestCombineSubgraphsMidLoopCancellationRestoresExactGeometry(t *testing.T) {
	first := layoutgraph.NewGraph()
	firstNode := layoutgraph.NewNode(1, 1000, 1000)
	firstNode.TopLeft = geo.NewPoint(100, 100)
	first.AddNodeUnchecked(firstNode)
	firstEdge := first.Connect(firstNode, firstNode)
	firstEdge.Points = []*geo.Point{geo.NewPoint(100, 100), geo.NewPoint(150, 175)}
	second := layoutgraph.NewGraph()
	for i := 0; i < 80; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+2), 1, 1)
		node.TopLeft = geo.NewPoint(0, 0)
		second.AddNodeUnchecked(node)
	}
	originalTopLeft, originalValue, originalGraph := firstNode.TopLeft, *firstNode.TopLeft, firstNode.Graph
	route := captureExactRouteTest(firstEdge)
	ctx := &packingMutationProbe{Context: context.Background(), node: firstNode, position: originalValue, route: route}
	combined, err := CombineSubgraphs(ctx, layoutgraph.NewGraph(), []*layoutgraph.Graph{first, second}, nil)
	if combined != nil {
		t.Fatalf("combined=%p, want nil", combined)
	}
	requirePackingCanceledAt(t, err, "CombineSubgraphs")
	if !ctx.observed {
		t.Fatal("CombineSubgraphs did not reach a post-mutation cancellation check")
	}
	if firstNode.TopLeft != originalTopLeft || *firstNode.TopLeft != originalValue || firstNode.Graph != originalGraph {
		t.Fatal("node rollback was not exact")
	}
	route.assertRestored(t)
	for _, node := range second.Nodes {
		if node.Graph != second {
			t.Fatalf("second node graph changed")
		}
	}
}
