// Package trees owns TALA's tree discovery, preprocessing, orientation, and
// placement algorithms. The graph records themselves remain in layoutgraph so
// routing, serialization, and transactions share one graph identity.
package trees

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

type TreeEdgeDirection string

const (
	Outwards      TreeEdgeDirection = "Outwards"
	Inwards       TreeEdgeDirection = "Inwards"
	Bidirectional TreeEdgeDirection = "Bidirectional"
	Undirected    TreeEdgeDirection = "Undirected"
)

func newWorkGuard(ctx context.Context, location string) (*limits.WorkGuard, error) {
	return limits.NewWorkGuard(ctx, location, limits.MaxEngineWorkUnits)
}

// treeEdgeDirection determines whether edge points from node toward its
// sentinel, toward node, in both directions, or in neither direction.
func treeEdgeDirection(node *layoutgraph.Node, edge *layoutgraph.Edge) TreeEdgeDirection {
	switch {
	case edge.IsBidirectional():
		return Bidirectional
	case edge.IsUndirected():
		return Undirected
	case edge.To == node:
		return Inwards
	default:
		return Outwards
	}
}
