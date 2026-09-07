package placement

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
)

// BalanceSymmetry centers simple nodes among neighbors aligned on one side.
func BalanceSymmetry(ctx context.Context, g *layoutgraph.Graph) error {
	ctx, guard, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "BalanceSymmetryTransactions")
	if err != nil {
		return err
	}
	if len(g.Nodes) == 0 {
		return nil
	}
	var rollbackState *layoutgraph.GraphState
	complete := false
	defer func() {
		if !complete && rollbackState != nil {
			layoutgraph.RestoreGraphState(g, rollbackState)
		}
	}()
	// Gap normalization takes care of cases where the adjacents are on opposite ends
	// So the only case we care about is when all the adjacents are aligned on one end
	// And we just nudge to be in the middle of the alignment
	for _, n := range g.Nodes {
		if !isSimple(g, n) {
			continue
		}
		if len(n.Edges) < 2 {
			continue
		}
		sameSide := true
		adjacentsM := make(map[*layoutgraph.Node]struct{})
		for i := 0; i < len(n.Edges)-1; i++ {
			e1 := n.Edges[i]
			e2 := n.Edges[i+1]
			if e1.FromTableColumnIndex != nil || e1.ToTableColumnIndex != nil {
				continue
			}
			if e2.FromTableColumnIndex != nil || e2.ToTableColumnIndex != nil {
				continue
			}
			adj1 := n.Adjacent(e1)
			adj2 := n.Adjacent(e2)
			o1 := n.Orientation(adj1)
			o2 := n.Orientation(adj2)
			if !o1.SameSide(o2) {
				sameSide = false
				break
			}
			adjacentsM[adj1] = struct{}{}
			adjacentsM[adj2] = struct{}{}
		}
		if !sameSide {
			continue
		}
		if len(adjacentsM) < 2 {
			continue
		}
		var adjacents layoutgraph.Nodes
		for k := range adjacentsM {
			adjacents = append(adjacents, k)
		}
		if placementcost.AxisScore(adjacents) != 1. {
			continue
		}
		// Fails to look symmetrical if uneven adjacents
		maxArea := math.Inf(-1)
		for _, adj := range adjacents {
			maxArea = math.Max(maxArea, adj.Width*adj.Height)
		}
		uneven := false
		for _, adj := range adjacents {
			if adj.Width*adj.Height < (0.5 * maxArea) {
				uneven = true
				break
			}
		}
		if uneven {
			continue
		}
		isHorizontal := true
		if adjacents[0].Orientation(adjacents[1]).IsVertical() {
			isHorizontal = false
		}
		txn, err := g.NewRequestTransactionWithWorkGuard(ctx, guard, layoutgraph.TransactionOptions{AffectContainers: true})
		if err != nil {
			return err
		}
		if rollbackState == nil {
			rollbackState = txn.PriorGraphState
		}
		txn.AddOp(func() error {
			center := adjacents.Center()
			nCenter := n.Center()
			if isHorizontal {
				n.MoveWithChildren(math.Floor(center.X-nCenter.X), 0)
			} else {
				n.MoveWithChildren(0, math.Floor(center.Y-nCenter.Y))
			}
			return nil
		})
		if err := txn.Commit(ctx); err != nil {
			if !layoutgraph.IsCandidateRejection(err) {
				return err
			}
		}
	}
	complete = true
	return nil
}

// isSimple reports whether node participates only as a regular placement node,
// rather than as or inside one of the engine's structured graph forms.
func isSimple(graph *layoutgraph.Graph, node *layoutgraph.Node) bool {
	return !(node.IsContainer() || graph.IsTreeSentinel(node) || node.IsClusterVessel() || graph.IsSequenceVessel(node) || node.Cluster != nil || node.Hierarchy != nil)
}
