// Package proximity discovers layout relationships between nearby nodes.
package proximity

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

// AddHubs discovers nodes that have both a leaf spoke and another connection
// within the same containing layout group.
func AddHubs(ctx context.Context, graph *layoutgraph.Graph) error {
	if err := layoutgraph.Validate(ctx, "AddHubs", graph); err != nil {
		return err
	}
	guard, err := limits.NewWorkGuard(ctx, "AddHubs", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	hubs := make(map[*layoutgraph.Node][]*layoutgraph.Node)
	for _, node := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		hasConnected := false
		var spokes []*layoutgraph.Node
		for _, edge := range node.Edges {
			if err := guard.Step(); err != nil {
				return err
			}
			adjacent := node.Adjacent(edge)
			if adjacent.OwningContainer() != node.OwningContainer() {
				continue
			}
			if len(adjacent.Edges) == 1 {
				spokes = append(spokes, adjacent)
			} else {
				hasConnected = true
			}
		}
		if hasConnected && len(spokes) > 0 {
			hubs[node] = spokes
		}
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	graph.Hubs = hubs
	return nil
}
