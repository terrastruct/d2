package placement

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/lib/geo"
)

func swapPositions(nodeA *layoutgraph.Node, nodeB *layoutgraph.Node) {
	tmpX := nodeA.TopLeft.X
	tmpY := nodeA.TopLeft.Y

	nodeA.MoveWithChildren(
		nodeB.TopLeft.X-nodeA.TopLeft.X,
		nodeB.TopLeft.Y-nodeA.TopLeft.Y,
	)

	nodeB.MoveWithChildren(
		tmpX-nodeB.TopLeft.X,
		tmpY-nodeB.TopLeft.Y,
	)
}

func smartSwapPositions(nodeA *layoutgraph.Node, nodeB *layoutgraph.Node) {
	tmpX := nodeA.TopLeft.X
	tmpY := nodeA.TopLeft.Y

	o := nodeA.Orientation(nodeB)
	if o == geo.Left {
		nodeA.MoveAbsWithChildren(nodeB.TopLeft.X+nodeB.Width-nodeA.Width, nodeA.TopLeft.Y)
		nodeB.MoveAbsWithChildren(tmpX, nodeB.TopLeft.Y)
	} else if o == geo.Right {
		nodeA.MoveAbsWithChildren(nodeB.TopLeft.X, nodeA.TopLeft.Y)
		nodeB.MoveAbsWithChildren(tmpX+nodeA.Width-nodeB.Width, nodeB.TopLeft.Y)
	} else if o == geo.Top {
		nodeA.MoveAbsWithChildren(nodeA.TopLeft.X, nodeB.TopLeft.Y+nodeB.Height-nodeA.Height)
		nodeB.MoveAbsWithChildren(nodeB.TopLeft.X, tmpY)
	} else if o == geo.Bottom {
		nodeA.MoveAbsWithChildren(nodeA.TopLeft.X, nodeB.TopLeft.Y)
		nodeB.MoveAbsWithChildren(nodeB.TopLeft.X, tmpY+nodeA.Height-nodeB.Height)
	} else if o == geo.TopLeft {
		nodeA.MoveAbsWithChildren(nodeB.TopLeft.X+nodeB.Width-nodeA.Width, nodeB.TopLeft.Y+nodeB.Height-nodeA.Height)
		nodeB.MoveAbsWithChildren(tmpX, tmpY)
	} else if o == geo.TopRight {
		nodeA.MoveAbsWithChildren(nodeB.TopLeft.X, nodeB.TopLeft.Y+nodeB.Height-nodeA.Height)
		nodeB.MoveAbsWithChildren(tmpX+nodeA.Width-nodeB.Width, tmpY)
	} else if o == geo.BottomLeft {
		nodeA.MoveAbsWithChildren(nodeB.TopLeft.X+nodeB.Width-nodeA.Width, nodeB.TopLeft.Y)
		nodeB.MoveAbsWithChildren(tmpX, tmpY+nodeA.Height-nodeB.Height)
	} else if o == geo.BottomRight {
		nodeA.MoveAbsWithChildren(nodeB.TopLeft.X, nodeB.TopLeft.Y)
		nodeB.MoveAbsWithChildren(tmpX+nodeA.Width-nodeB.Width, tmpY+nodeA.Height-nodeB.Height)
	}
}

// swapOptimize swaps node positions when doing so lowers placement cost.
func swapOptimize(ctx context.Context, nodes layoutgraph.Nodes, g *layoutgraph.Graph) (bool, error) {
	ctx, _, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "SwapOptimizeTransactions")
	if err != nil {
		return false, err
	}
	symmetryCost := g.CellSize
	swapMade := false

	measure := func(n *layoutgraph.Node) (float64, error) {
		l, err := placementcost.NodeEdgeLength(ctx, n, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
		if err != nil {
			return 0, err
		}
		columnCrossingCost, err := placementcost.ColumnCrossingCost(ctx, n, nil)
		if err != nil {
			return 0, err
		}
		symmetry, err := placementcost.NodeSymmetry(ctx, n, nil)
		if err != nil {
			return 0, err
		}
		l += columnCrossingCost
		l -= symmetry * symmetryCost * float64(len(n.Edges))
		return l, nil
	}

	txn, err := g.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
	if err != nil {
		return false, err
	}
	for _, node := range nodes {
		if _, has := g.NodeToTree[node]; has {
			continue
		}
		if node.Hierarchy != nil {
			continue
		}
		if node.FixedTopLeft != nil {
			continue
		}
		if len(node.Edges) == 0 && !node.HasLeakyEdge() {
			continue
		}

		bestSmartSwapped := false
		var bestSwapCandidate *layoutgraph.Node
		bestSwapEdgeLength := math.Inf(1)

		currentGlobalL, err := placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
		if err != nil {
			return false, err
		}
		currentL1, err := measure(node)
		if err != nil {
			return false, err
		}
		currentCrossings, err := placementcost.GraphEdgeCrossings(ctx, g)
		if err != nil {
			return false, err
		}

		for _, swapCandidate := range g.Containers[node.Container] {
			if swapCandidate == node {
				continue
			}
			if _, has := g.NodeToTree[swapCandidate]; has {
				continue
			}
			if swapCandidate.Hierarchy != nil {
				continue
			}
			if swapCandidate.FixedTopLeft != nil {
				continue
			}

			txn.AddOp(func() error {
				swapPositions(node, swapCandidate)

				return nil
			})

			var swappedL1 float64
			var swappedGlobalL float64
			var swappedCrossings int64
			err = txn.Commit(ctx)
			if err == nil {
				swappedL1, err = measure(node)
				if err == nil {
					swappedGlobalL, err = placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
				}
				if err == nil {
					swappedCrossings, err = placementcost.GraphEdgeCrossings(ctx, g)
				}
				if err != nil {
					txn.Rollback()
					txn.Clear()
					return false, err
				}
			} else if !layoutgraph.IsCandidateRejection(err) {
				return false, err
			}

			txn.Rollback()
			txn.Clear()

			txn.AddOp(func() error {
				smartSwapPositions(node, swapCandidate)

				return nil
			})

			err = txn.Commit(ctx)

			var smartSwappedL1 float64
			var smartSwappedGlobalL float64
			var smartSwappedCrossings int64
			if err == nil {
				smartSwappedL1, err = measure(node)
				if err == nil {
					smartSwappedGlobalL, err = placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
				}
				if err == nil {
					smartSwappedCrossings, err = placementcost.GraphEdgeCrossings(ctx, g)
				}
				if err != nil {
					txn.Rollback()
					txn.Clear()
					return false, err
				}
			} else if !layoutgraph.IsCandidateRejection(err) {
				return false, err
			}

			txn.Rollback()
			txn.Clear()

			if swappedGlobalL == 0 && smartSwappedGlobalL == 0 {
				continue
			}
			if swappedL1 != 0 && smartSwappedL1 == 0 {
				if swappedCrossings <= currentCrossings && geo.PrecisionCompare(swappedL1, currentL1, geo.PRECISION) < 0 && geo.PrecisionCompare(swappedGlobalL, currentGlobalL, geo.PRECISION) < 0 && geo.PrecisionCompare(swappedGlobalL, bestSwapEdgeLength, geo.PRECISION) < 0 {
					bestSwapEdgeLength = swappedGlobalL
					bestSmartSwapped = false
					bestSwapCandidate = swapCandidate
				}
			} else if smartSwappedL1 != 0 && swappedL1 == 0 {
				if smartSwappedCrossings <= currentCrossings && geo.PrecisionCompare(smartSwappedL1, currentL1, geo.PRECISION) < 0 && geo.PrecisionCompare(smartSwappedGlobalL, currentGlobalL, geo.PRECISION) < 0 && geo.PrecisionCompare(smartSwappedGlobalL, bestSwapEdgeLength, geo.PRECISION) < 0 {
					bestSwapEdgeLength = smartSwappedGlobalL
					bestSmartSwapped = true
					bestSwapCandidate = swapCandidate
				}
			} else if swappedL1 != 0 && smartSwappedL1 != 0 {
				if geo.PrecisionCompare(smartSwappedL1, swappedL1, geo.PRECISION) < 0 {
					if smartSwappedCrossings <= currentCrossings && geo.PrecisionCompare(smartSwappedL1, currentL1, geo.PRECISION) < 0 && geo.PrecisionCompare(smartSwappedGlobalL, currentGlobalL, geo.PRECISION) < 0 && geo.PrecisionCompare(smartSwappedGlobalL, bestSwapEdgeLength, geo.PRECISION) < 0 {
						bestSwapEdgeLength = smartSwappedGlobalL
						bestSmartSwapped = true
						bestSwapCandidate = swapCandidate
					}
				} else {
					if swappedCrossings <= currentCrossings && geo.PrecisionCompare(swappedL1, currentL1, geo.PRECISION) < 0 && geo.PrecisionCompare(swappedGlobalL, currentGlobalL, geo.PRECISION) < 0 && geo.PrecisionCompare(swappedGlobalL, bestSwapEdgeLength, geo.PRECISION) < 0 {
						bestSwapEdgeLength = swappedGlobalL
						bestSmartSwapped = false
						bestSwapCandidate = swapCandidate
					}
				}
			} else {
				// They can both be 0 if the node is just a container, in which case we compare globals
				if swappedGlobalL != 0 && smartSwappedGlobalL == 0 {
					if swappedCrossings <= currentCrossings && geo.PrecisionCompare(swappedGlobalL, currentGlobalL, geo.PRECISION) < 0 && geo.PrecisionCompare(swappedGlobalL, bestSwapEdgeLength, geo.PRECISION) < 0 {
						bestSwapEdgeLength = swappedGlobalL
						bestSmartSwapped = false
						bestSwapCandidate = swapCandidate
					}
				} else if swappedGlobalL == 0 && smartSwappedGlobalL != 0 {
					if smartSwappedCrossings <= currentCrossings && geo.PrecisionCompare(smartSwappedGlobalL, currentGlobalL, geo.PRECISION) < 0 && geo.PrecisionCompare(smartSwappedGlobalL, bestSwapEdgeLength, geo.PRECISION) < 0 {
						bestSwapEdgeLength = smartSwappedGlobalL
						bestSmartSwapped = true
						bestSwapCandidate = swapCandidate
					}
				} else {
					if geo.PrecisionCompare(swappedGlobalL, smartSwappedGlobalL, geo.PRECISION) < 0 {
						if swappedCrossings <= currentCrossings && geo.PrecisionCompare(swappedGlobalL, currentGlobalL, geo.PRECISION) < 0 && geo.PrecisionCompare(swappedGlobalL, bestSwapEdgeLength, geo.PRECISION) < 0 {
							bestSwapEdgeLength = swappedGlobalL
							bestSmartSwapped = false
							bestSwapCandidate = swapCandidate
						}
					} else {
						if smartSwappedCrossings <= currentCrossings && geo.PrecisionCompare(smartSwappedGlobalL, currentGlobalL, geo.PRECISION) < 0 && geo.PrecisionCompare(smartSwappedGlobalL, bestSwapEdgeLength, geo.PRECISION) < 0 {
							bestSwapEdgeLength = smartSwappedGlobalL
							bestSmartSwapped = true
							bestSwapCandidate = swapCandidate
						}
					}
				}
			}
		}

		if bestSwapCandidate != nil {
			swapMade = true
			txn.AddOp(func() error {
				if bestSmartSwapped {
					smartSwapPositions(node, bestSwapCandidate)
				} else {
					swapPositions(node, bestSwapCandidate)
				}
				return nil
			})
			err := txn.Commit(ctx)
			if err != nil {
				return false, err
			}
			txn.Clear()
			if err := txn.UpdateState(); err != nil {
				return false, err
			}
		}
	}

	return swapMade, nil
}
