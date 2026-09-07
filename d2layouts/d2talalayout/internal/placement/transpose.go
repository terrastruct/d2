package placement

import (
	"context"
	"math"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/lib/geo"
)

// C is the current node:
//
/*
// X
// |
// E -- A
// |    |
// D    C
//	    |
//	    B
//
// # We can tranpose C-B around A to get
//
// X
// |
// E -- A -- C -- B
// |
// D
//
// Which is more symmetrical
*/
func transpose(ctx context.Context, g *layoutgraph.Graph, node *layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction) (bool, error) {
	ctx, _, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "TransposeTransactions")
	if err != nil {
		return false, err
	}
	if node.Hierarchy != nil {
		return false, nil
	}
	if _, has := g.NodeToTree[node]; has {
		return false, nil
	}
	if g.IsTreeSentinel(node) {
		return false, nil
	}
	if node.FixedTopLeft != nil {
		return false, nil
	}
	// Maybe re-enable when I see a use case
	for _, e := range edgeAbductions {
		if e.OriginallyFrom == node || e.OriginallyTo == node {
			return false, nil
		}
		if e.CurrentFrom == node || e.CurrentTo == node {
			return false, nil
		}
	}
	if len(node.Edges) != 2 && len(node.Edges) != 1 {
		return false, nil
	}
	reachabilityGuard, err := limits.NewWorkGuard(ctx, "TransposeReachability", limits.MaxEngineWorkUnits)
	if err != nil {
		return false, err
	}
	reachableFrom := func(start *layoutgraph.Node, includeContainers bool, ignore map[*layoutgraph.Node]struct{}) ([]*layoutgraph.Node, error) {
		return start.AllReachableNodesContext(includeContainers, false, true, ignore, reachabilityGuard)
	}

	// Need to account for this case too, whereby the container should be the one moving
	// ┌───────────┐           ┌────────────┐
	// │           │           │            │
	// │           │           │            │
	// │   ┌────┐  │           │    ┌───┐   │
	// │   │    │  │           │    │   │   │
	// │   │    ◄──┼───────────┼────┤   │   │
	// │   └────┘  │           │    └───┘   │
	// │           │           │            │
	// │           │           │            │
	// └───────────┘           └────────────┘
	//

	var transposeNodes []*layoutgraph.Node
	var centerNode *layoutgraph.Node
	if len(node.Edges) == 1 {
		nodeA := node.Adjacent(node.Edges[0])
		if nodeA.IsDescendantOf(node) || node.IsDescendantOf(nodeA) {
			return false, nil
		}
		if node.Orientation(nodeA).IsDiagonal() {
			return false, nil
		}
		ancestor := node.NearestSharedAncestor(nodeA)

		centerNode = nodeA
		if edgeAbductions == nil {
			curr := node
			for curr.OwningContainer() != ancestor {
				curr = curr.OwningContainer()
			}

			transposeNodes, err = reachableFrom(curr, false, map[*layoutgraph.Node]struct{}{nodeA: {}})
			if err != nil {
				return false, err
			}
			if slices.ContainsFunc(transposeNodes, nodeA.IsDescendantOf) {
				return false, nil
			}
		} else {
			transposeNodes = []*layoutgraph.Node{node}
		}
	} else {
		nodeA := node.Adjacent(node.Edges[0])
		nodeB := node.Adjacent(node.Edges[1])

		if nodeA.IsDescendantOf(node) || node.IsDescendantOf(nodeA) {
			return false, nil
		}
		if nodeB.IsDescendantOf(node) || node.IsDescendantOf(nodeB) {
			return false, nil
		}

		ancestorA := node.NearestSharedAncestor(nodeA)
		ancestorB := node.NearestSharedAncestor(nodeB)

		connectedToA, err := reachableFrom(nodeA, true, map[*layoutgraph.Node]struct{}{node: {}, ancestorA: {}})
		if err != nil {
			return false, err
		}
		if slices.Contains(connectedToA, nodeB) {
			return false, nil
		}

		nodeAOrientation := node.Orientation(nodeA)
		nodeBOrientation := node.Orientation(nodeB)

		if nodeAOrientation.IsDiagonal() || nodeBOrientation.IsDiagonal() {
			return false, nil
		}

		reachableNodesA, err := reachableFrom(nodeA, false, map[*layoutgraph.Node]struct{}{node: {}, ancestorA: {}})
		if err != nil {
			return false, err
		}
		reachableNodesB, err := reachableFrom(nodeB, false, map[*layoutgraph.Node]struct{}{node: {}, ancestorB: {}})
		if err != nil {
			return false, err
		}

		// Rotate the side with less nodes around the side with more nodes
		if len(reachableNodesA) >= len(reachableNodesB) {
			if edgeAbductions == nil {
				curr := node
				for curr.OwningContainer() != ancestorB {
					curr = curr.OwningContainer()
				}

				transposeNodes, err = reachableFrom(curr, false, map[*layoutgraph.Node]struct{}{nodeA: {}})
				if err != nil {
					return false, err
				}
				if slices.ContainsFunc(transposeNodes, nodeA.IsDescendantOf) {
					return false, nil
				}
			} else {
				transposeNodes = append(reachableNodesB, node)
			}

			centerNode = nodeA
		} else {
			if edgeAbductions == nil {
				curr := node
				for curr.OwningContainer() != ancestorA {
					curr = curr.OwningContainer()
				}

				transposeNodes, err = reachableFrom(curr, false, map[*layoutgraph.Node]struct{}{nodeB: {}})
				if err != nil {
					return false, err
				}
				if slices.ContainsFunc(transposeNodes, nodeB.IsDescendantOf) {
					return false, nil
				}
			} else {
				transposeNodes = append(reachableNodesA, node)
			}
			centerNode = nodeB
		}
	}
	if err := reachabilityGuard.Finish(); err != nil {
		return false, err
	}

	for _, n := range transposeNodes {
		if n.FixedOrigin() != nil {
			return false, nil
		}
	}

	calcLength := func() (float64, error) {
		if edgeAbductions == nil {
			return placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
		}
		sum, err := placementcost.NodeEdgeLength(ctx, node, placementcost.EdgeLengthOptions{EdgeAbductions: edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
		if err != nil {
			return 0, err
		}
		for _, e := range node.Edges {
			length, err := placementcost.NodeEdgeLength(ctx, node.Adjacent(e), placementcost.EdgeLengthOptions{EdgeAbductions: edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
			if err != nil {
				return 0, err
			}
			sum += length
		}
		return sum, nil
	}

	bestLength, err := calcLength()
	if err != nil {
		return false, err
	}
	bestRotations := -1

	txn, err := g.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
	if err != nil {
		return false, err
	}

	// Rotate all around
	for i := 0; i < 3; i++ {
		txn.AddOp(func() error {
			for _, n := range transposeNodes {
				rotateAround(n, g, centerNode, i+1, edgeAbductions != nil)
				if n.IsClusterVessel() {
					if _, err := optimizeCluster(ctx, g.Clusters[n], true); err != nil {
						return err
					}
				}
			}
			return nil
		})

		if err := txn.Commit(ctx); err == nil {
			length, scoreErr := calcLength()
			if scoreErr != nil {
				txn.Rollback()
				txn.Clear()
				return false, scoreErr
			}
			if geo.PrecisionCompare(length, bestLength, geo.PRECISION) < 0 {
				bestLength = length
				bestRotations = i + 1
			}
		} else if !layoutgraph.IsCandidateRejection(err) {
			return false, err
		}

		txn.Rollback()
		txn.Clear()
	}

	if bestRotations != -1 {
		txn.AddOp(func() error {
			for _, n := range transposeNodes {
				rotateAround(n, g, centerNode, bestRotations, edgeAbductions != nil)
				if n.IsClusterVessel() {
					if _, err := optimizeCluster(ctx, g.Clusters[n], true); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err := txn.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func rotateAround(n *layoutgraph.Node, g *layoutgraph.Graph, centerNode *layoutgraph.Node, times int, round bool) {
	for i := 0; i < times; i++ {
		nodeCenter := n.Center()

		// Translate node center to origin
		translatedX := nodeCenter.X - centerNode.TopLeft.X - centerNode.Width/2
		translatedY := nodeCenter.Y - centerNode.TopLeft.Y - centerNode.Height/2

		// Rotate 90 degrees counterclockwise
		rotatedX := -translatedY
		rotatedY := translatedX

		// Translate back
		newCenterX := rotatedX + centerNode.TopLeft.X + centerNode.Width/2
		newCenterY := rotatedY + centerNode.TopLeft.Y + centerNode.Height/2

		// Calculate new top-left position
		newTopLeftX := math.Round(newCenterX - n.Width/2)
		newTopLeftY := math.Round(newCenterY - n.Height/2)

		if round {
			if int(newTopLeftX)%int(g.CellSize) != 0 {
				newTopLeftX = roundToNearestCellSize(newTopLeftX, g.CellSize)
			}
			if int(newTopLeftY)%int(g.CellSize) != 0 {
				newTopLeftY = roundToNearestCellSize(newTopLeftY, g.CellSize)
			}
		}

		n.MoveAbsWithChildren(newTopLeftX, newTopLeftY)
	}
}
