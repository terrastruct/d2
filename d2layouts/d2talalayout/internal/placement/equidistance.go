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

// Equidistance centers nodes between aligned neighbors.
//
// A ---------------- B ------ C
// Move B to the midpoint of A and C.
func Equidistance(ctx context.Context, g *layoutgraph.Graph) (bool, error) {
	ctx, _, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "EquidistanceTransactions")
	if err != nil {
		return false, err
	}
	if err := layoutgraph.Validate(ctx, "Equidistance", g); err != nil {
		return false, err
	}
	originalPositions, err := snapshotNodePositionsContext(ctx, "Equidistance", g.Nodes)
	if err != nil {
		return false, err
	}
	complete := false
	defer func() {
		if !complete {
			restoreNodePositions(originalPositions)
		}
	}()
	reachabilityGuard, err := limits.NewWorkGuard(ctx, "EquidistanceReachability", limits.MaxEngineWorkUnits)
	if err != nil {
		return false, err
	}
	movedHorizontally := false
	movedVertically := false
	for _, n := range g.Nodes {
		movedHorizontally2, err := equidistanceNodeGuarded(ctx, n, g, true, reachabilityGuard)
		if err != nil {
			return false, err
		}
		movedVertically2, err := equidistanceNodeGuarded(ctx, n, g, false, reachabilityGuard)
		if err != nil {
			return false, err
		}
		if movedHorizontally2 {
			movedHorizontally = true
		}
		if movedVertically2 {
			movedVertically = true
		}
	}
	// Try multiple times, because moving one may result in different symmetries for others
	if movedHorizontally {
		movedAgain := false
		for i := 0; i < 5; i++ {
			for _, n := range g.Nodes {
				m, err := equidistanceNodeGuarded(ctx, n, g, true, reachabilityGuard)
				if err != nil {
					return false, err
				}
				if m {
					movedAgain = true
				}
			}
			if !movedAgain {
				break
			}
			movedAgain = false
		}
	}
	if movedVertically {
		movedAgain := false
		for i := 0; i < 5; i++ {
			for _, n := range g.Nodes {
				m, err := equidistanceNodeGuarded(ctx, n, g, false, reachabilityGuard)
				if err != nil {
					return false, err
				}
				if m {
					movedAgain = true
				}
			}
			if !movedAgain {
				break
			}
			movedAgain = false
		}
	}

	if err := reachabilityGuard.Finish(); err != nil {
		return false, err
	}
	complete = true
	return movedHorizontally || movedVertically, nil
}

func equidistanceNodeGuarded(ctx context.Context, n *layoutgraph.Node, g *layoutgraph.Graph, isHorizontal bool, reachabilityGuard *limits.WorkGuard) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if n.Hierarchy != nil {
		return false, nil
	}
	if _, has := g.NodeToTree[n]; has {
		return false, nil
	}
	if g.IsTreeSentinel(n) {
		return false, nil
	}
	if n.FixedTopLeft != nil {
		return false, nil
	}

	var nearestBack *layoutgraph.Node
	var nearestFront *layoutgraph.Node

	for _, e := range n.Edges {
		adj := n.Adjacent(e)
		if n.IsDescendantOf(adj) || adj.IsDescendantOf(n) {
			return false, nil
		}
		if isHorizontal {
			// Left
			if adj.TopLeft.X+adj.Width < n.TopLeft.X {
				if nearestBack == nil || adj.TopLeft.X+adj.Width > nearestBack.TopLeft.X+nearestBack.Width {
					nearestBack = adj
				}
			}
			// Right
			if adj.TopLeft.X > n.TopLeft.X+n.Width {
				if nearestFront == nil || adj.TopLeft.X < nearestFront.TopLeft.X {
					nearestFront = adj
				}
			}
		} else {
			// Top
			if adj.TopLeft.Y+adj.Height < n.TopLeft.Y {
				if nearestBack == nil || adj.TopLeft.Y+adj.Height > nearestBack.TopLeft.Y+nearestBack.Height {
					nearestBack = adj
				}
			}
			// Bottom
			if adj.TopLeft.Y > n.TopLeft.Y+n.Height {
				if nearestFront == nil || adj.TopLeft.Y < nearestFront.TopLeft.Y {
					nearestFront = adj
				}
			}
		}
	}

	if nearestBack == nil || nearestFront == nil {
		return false, nil
	}

	var otherConnected []*layoutgraph.Node
	for _, e := range n.Edges {
		adj := n.Adjacent(e)
		if adj == nearestBack || adj == nearestFront {
			continue
		}
		if isHorizontal {
			if !adj.Orientation(n).IsVertical() {
				continue
			}
		} else {
			if !adj.Orientation(n).IsHorizontal() {
				continue
			}
		}
		if adj.IsDescendantOf(nearestBack) || nearestBack.IsDescendantOf(adj) {
			continue
		}
		if adj.IsDescendantOf(nearestFront) || nearestFront.IsDescendantOf(adj) {
			continue
		}

		reachable, err := adj.AllReachableNodesContext(false, true, true, map[*layoutgraph.Node]struct{}{
			n:            {},
			nearestBack:  {},
			nearestFront: {},
		}, reachabilityGuard)
		if err != nil {
			return false, err
		}

		for _, reachableN := range reachable {
			if n.IsDescendantOf(reachableN) {
				continue
			}
			if reachableN.IsDescendantOf(nearestBack) || nearestBack.IsDescendantOf(reachableN) {
				continue
			}
			if reachableN.IsDescendantOf(nearestFront) || nearestFront.IsDescendantOf(reachableN) {
				continue
			}
		}

		for _, reachableN := range reachable {
			if !slices.Contains(otherConnected, reachableN) {
				otherConnected = append(otherConnected, reachableN)
			}
		}
	}

	for _, c := range otherConnected {
		if c.FixedTopLeft != nil {
			otherConnected = []*layoutgraph.Node{}
			break
		}
	}

	ancestorBack := n.NearestSharedAncestor(nearestBack)
	ancestorFront := n.NearestSharedAncestor(nearestFront)

	/*

			We want to be able to move the whole container
		 ┌────┐           ┌─────────┐        ┌──────┐
		 │    │           │         │        │      │
		 │    │           │  ┌───┐  │        │      │
		 │    │           │  └───┘  │        │      │
		 └────┘           │         │        │      │
		                  └─────────┘        └──────┘

	*/
	for n.OwningContainer() != nil && !nearestBack.IsDescendantOf(n.OwningContainer()) && !nearestFront.IsDescendantOf(n.OwningContainer()) {
		n = n.OwningContainer()
	}

	/*

			If b is the nearest back in !isHorizontal, b's container is relevant and should be used.

						┌─────┐
						│     │
						│  a  │
						└─────┘



			┌────────────────┐
			│                │
			│                │
			│                │
			│     ┌────┐     │
			│     │ b  │     │
			│     │    │     │
			│     └────┘     │
			│                │
			│                │
			│                │
			└────────────────┘

			But if it's like this, we don't care


		                      ┌──────┐
		                      │      │
		┌────────────────┐    │  a   │
		│                │    │      │
		│                │    └──────┘
		│                │
		│     ┌────┐     │
		│     │ b  │     │
		│     │    │     │
		│     └────┘     │
		│                │
		│                │
		│                │
		└────────────────┘
	*/
	for nearestBack.OwningContainer() != ancestorBack {
		container := nearestBack.OwningContainer()
		if isHorizontal {
			if container.TopLeft.X+container.Width < n.TopLeft.X {
				nearestBack = nearestBack.OwningContainer()
			} else {
				break
			}
		} else {
			if container.TopLeft.Y+container.Height < n.TopLeft.Y {
				nearestBack = nearestBack.OwningContainer()
			} else {
				break
			}
		}
	}
	for nearestFront.OwningContainer() != ancestorFront {
		container := nearestFront.OwningContainer()
		if isHorizontal {
			if container.TopLeft.X > n.TopLeft.X+n.Width {
				nearestFront = nearestFront.OwningContainer()
			} else {
				break
			}
		} else {
			if container.TopLeft.Y > n.TopLeft.Y+n.Height {
				nearestFront = nearestFront.OwningContainer()
			} else {
				break
			}
		}
	}

	originalLength, err := placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		return false, err
	}

	diffX := 0.
	diffY := 0.
	if isHorizontal {
		x := math.Round((nearestBack.TopLeft.X+nearestBack.Width+nearestFront.TopLeft.X)/2. - n.Width/2.)
		diffX = x - n.TopLeft.X
	} else {
		y := math.Round((nearestBack.TopLeft.Y+nearestBack.Height+nearestFront.TopLeft.Y)/2. - n.Height/2.)
		diffY = y - n.TopLeft.Y
	}

	txn, err := g.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
	if err != nil {
		return false, err
	}

	rolledBack := false
	var soloMoveLength float64
	var connectedMoveLength float64

	// First try moving just the node by itself
	txn.AddOp(func() error {
		n.MoveWithChildren(diffX, diffY)
		return nil
	})
	if err := txn.Commit(ctx); err != nil {
		if !layoutgraph.IsCandidateRejection(err) {
			return false, err
		}
		rolledBack = true
	}
	if !rolledBack {
		newLength, err := placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
		if err != nil {
			txn.Rollback()
			return false, err
		}
		if geo.PrecisionCompare(newLength, originalLength, geo.PRECISION) <= 0 {
			soloMoveLength = newLength
		}
		txn.Rollback()
	}

	txn.Clear()

	// Then try moving with connected
	if len(otherConnected) > 0 {
		rolledBack = false
		txn.AddOp(func() error {
			for _, n2 := range append(otherConnected, n) {
				n2.MoveWithChildren(diffX, diffY)
			}
			return nil
		})
		if err := txn.Commit(ctx); err != nil {
			if !layoutgraph.IsCandidateRejection(err) {
				return false, err
			}
			rolledBack = true
		}
		if !rolledBack {
			newLength, err := placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
			if err != nil {
				txn.Rollback()
				return false, err
			}
			if geo.PrecisionCompare(newLength, originalLength, geo.PRECISION) <= 0 {
				connectedMoveLength = newLength
			}
			txn.Rollback()
		}
	}

	txn.Clear()

	// Then use the best global edge length and move it there
	if soloMoveLength == 0 && connectedMoveLength == 0 {
		return false, nil
	} else {
		txn.AddOp(func() error {
			if soloMoveLength == 0 && connectedMoveLength > 0 {
				for _, n2 := range append(otherConnected, n) {
					n2.MoveWithChildren(diffX, diffY)
				}
			} else if soloMoveLength > 0 && connectedMoveLength == 0 {
				n.MoveWithChildren(diffX, diffY)
			} else if geo.PrecisionCompare(soloMoveLength, connectedMoveLength, geo.PRECISION) < 0 {
				n.MoveWithChildren(diffX, diffY)
			} else {
				for _, n2 := range append(otherConnected, n) {
					n2.MoveWithChildren(diffX, diffY)
				}
			}

			return nil
		})

		if err := txn.Commit(ctx); err != nil {
			return false, err
		}
	}
	return true, nil
}
