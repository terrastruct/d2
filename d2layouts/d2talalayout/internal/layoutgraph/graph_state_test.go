package layoutgraph

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func updateGraphStateForTest(gs *GraphState, g *Graph) {
	guard, err := limits.NewWorkGuard(context.Background(), "GraphState", limits.MaxEngineWorkUnits)
	if err != nil {
		panic(err)
	}
	if err := gs.UpdateWithWorkGuard(g, guard); err != nil {
		panic(err)
	}
}
