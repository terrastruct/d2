package proximity

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type cancelHubsAfterErrChecks struct {
	context.Context
	remaining int
}

func (ctx *cancelHubsAfterErrChecks) Err() error {
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return ctx.Context.Err()
}

func TestAddHubsMidLoopCancellationIsAtomic(t *testing.T) {
	graph := layoutgraph.NewGraph()
	for index := 0; index < 130; index++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(index+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(index*20), 0)
		graph.AddNodeUnchecked(node)
	}
	graph.Hubs[graph.Nodes[0]] = []*layoutgraph.Node{graph.Nodes[1]}
	originalHubs := reflect.ValueOf(graph.Hubs).Pointer()

	err := AddHubs(&cancelHubsAfterErrChecks{Context: context.Background(), remaining: 1}, graph)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "AddHubs") {
		t.Fatalf("AddHubs error = %v, want wrapped context cancellation", err)
	}
	if reflect.ValueOf(graph.Hubs).Pointer() != originalHubs || len(graph.Hubs) != 1 || graph.Hubs[graph.Nodes[0]][0] != graph.Nodes[1] {
		t.Fatal("AddHubs changed the hub map after cancellation")
	}
}
