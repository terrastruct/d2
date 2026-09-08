package layoutgraph

import "context"

// SharedWorkStepper is the narrow accounting boundary used when a sibling
// layout domain supplies the aggregate work budget for graph traversal.
type SharedWorkStepper interface {
	Step() error
	Finish() error
}

// NodeGraphOwnershipJournal records the graph pointer owned by each node
// before a subgraph split temporarily redirects it.
type NodeGraphOwnershipJournal struct {
	original nodeGraphOwnershipJournal
}

// Restore restores every graph pointer captured by the split.
func (journal NodeGraphOwnershipJournal) Restore() {
	journal.original.restore()
}

// OriginalGraph reports the graph pointer captured for node by the split.
func (journal NodeGraphOwnershipJournal) OriginalGraph(node *Node) (*Graph, bool) {
	graph, captured := journal.original[node]
	return graph, captured
}

// SplitSubgraphsTracked splits a graph while returning the exact ownership
// journal needed by callers that keep the temporary subgraphs alive during a
// larger atomic operation.
func (graph *Graph) SplitSubgraphsTracked(
	ctx context.Context,
	options SplitOptions,
	work SharedWorkStepper,
) ([]*Graph, NodeGraphOwnershipJournal, error) {
	var internal nodeGraphOwnershipJournal
	var stepper workStepper
	if work != nil {
		stepper = work
	}
	graphs, err := graph.splitSubgraphsWithOwnership(
		ctx,
		options,
		&internal,
		stepper,
	)
	return graphs, NodeGraphOwnershipJournal{original: internal}, err
}
