package layoutgraph

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const nodeGraphOwnershipSnapshotLocation = "NodeGraphOwnershipSnapshot"

// NodeGraphOwnershipSnapshot is a read-only capture of every pre-existing node
// owner reachable from a graph's runtime topology.
type NodeGraphOwnershipSnapshot struct {
	owners []nodeGraphOwner
}

type nodeGraphOwner struct {
	node  *Node
	graph *Graph
}

// Restore restores every graph pointer captured by the snapshot.
func (snapshot NodeGraphOwnershipSnapshot) Restore() {
	for _, owner := range snapshot.owners {
		owner.node.Graph = owner.graph
	}
}

// SnapshotNodeGraphOwnership captures every node reachable through the graph's
// runtime topology, including nodes referenced only by grouping, tree,
// proximity, hub, or hierarchy state. It does not mutate the graph.
func SnapshotNodeGraphOwnership(ctx context.Context, graph *Graph) (NodeGraphOwnershipSnapshot, error) {
	if graph == nil {
		return NodeGraphOwnershipSnapshot{}, invariant.New("node graph ownership snapshot requires a graph")
	}
	guard, err := limits.NewWorkGuard(ctx, nodeGraphOwnershipSnapshotLocation, maxEngineWorkUnits)
	if err != nil {
		return NodeGraphOwnershipSnapshot{}, err
	}
	objects, err := collectRuntimeObjectsContext(graph, guard, "ownership runtime")
	if err != nil {
		return NodeGraphOwnershipSnapshot{}, err
	}
	owners := make([]nodeGraphOwner, len(objects.nodeOrder))
	for index, node := range objects.nodeOrder {
		if err := guard.Step(); err != nil {
			return NodeGraphOwnershipSnapshot{}, err
		}
		owners[index] = nodeGraphOwner{node: node, graph: node.Graph}
	}
	if err := guard.Finish(); err != nil {
		return NodeGraphOwnershipSnapshot{}, err
	}
	return NodeGraphOwnershipSnapshot{owners: owners}, nil
}
