package layoutgraph

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/lib/geo"
)

func (node *Node) IsClusterVessel() bool {
	return node != nil && node.isClusterVessel
}

func (edge *Edge) ArrowheadTo(node *Node) Arrowhead {
	return edge.arrowheadTo(node)
}

func (edge *Edge) HasArrowTo(node *Node) bool {
	return edge.hasArrowTo(node)
}

// FixedOrigin returns the fixed-position origin inherited through a node's
// descendants. Tree placement uses it to preserve D2 position constraints.
func (node *Node) FixedOrigin() *geo.Point {
	return node.fixedOrigin()
}

// NewRequestTransaction creates a request-scoped placement transaction. Tree
// trials share the same transaction accounting and rollback semantics as the
// rest of the engine.
func (g *Graph) NewRequestTransaction(ctx context.Context, options TransactionOptions) (*Transaction, error) {
	return g.newRequestTransaction(ctx, options)
}

func (state *GraphState) TracksNode(node *Node) bool {
	_, tracked := state.nodes[node]
	return tracked
}

// AddIncidentEdgeUnchecked restores an already graph-owned edge to a node's
// incidence list. Tree preprocessing validates ownership before calling it.
func (node *Node) AddIncidentEdgeUnchecked(edge *Edge) {
	node.addEdge(edge)
}

// buildNodeToTreeGuarded derives the auxiliary node index while preserving
// graphjson's established validation order and shared decode work budget.
func (g *Graph) buildNodeToTreeGuarded(guard workStepper) error {
	nodeToTree := make(map[*Node]*Tree)
	order := make([]*Node, 0, len(g.Trees))
	for rootSentinel := range g.Trees {
		if err := guard.Step(); err != nil {
			return err
		}
		order = append(order, rootSentinel)
	}
	sortNodesByID(order)
	if err := guard.Finish(); err != nil {
		return err
	}
	for _, rootSentinel := range order {
		if err := guard.Step(); err != nil {
			return err
		}
		for _, root := range g.Trees[rootSentinel] {
			if root == nil || root.Node == nil {
				return invariant.New("tree sentinel contains an incomplete root")
			}
			queue := []*Tree{root}
			seen := make(map[*Tree]struct{})
			for index := 0; index < len(queue); index++ {
				if err := guard.Step(); err != nil {
					return err
				}
				current := queue[index]
				if current == nil || current.Node == nil {
					return invariant.New("tree preprocessing encountered an incomplete tree")
				}
				if _, exists := seen[current]; exists {
					return invariant.New("tree preprocessing encountered a repeated tree node")
				}
				seen[current] = struct{}{}
				nodeToTree[current.Node] = current
				for _, child := range current.Children {
					if err := guard.Step(); err != nil {
						return err
					}
					queue = append(queue, child)
				}
			}
		}
	}
	g.NodeToTree = nodeToTree
	return guard.Finish()
}
