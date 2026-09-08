package placement

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"

	"github.com/d2lang/d2/lib/geo"
)

type gapNormalizationOptions struct {
	axis      layoutAxis
	direction traversalDirection
	costTxn   *layoutgraph.Transaction
}

type gapReductionOptions struct {
	axis                   layoutAxis
	direction              traversalDirection
	attemptRecoverSymmetry bool
	costTxn                *layoutgraph.Transaction
}

func gapNormalization(ctx context.Context, nodes layoutgraph.Nodes, txn *layoutgraph.Transaction, g *layoutgraph.Graph, options gapNormalizationOptions) (bool, error) {
	if !options.axis.valid() {
		return false, fmt.Errorf("TALA GapNormalization requires an axis")
	}
	if !options.direction.valid() {
		return false, fmt.Errorf("TALA GapNormalization requires a direction")
	}
	ctx, _, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "GapNormalizationTransactions")
	if err != nil {
		return false, err
	}
	// We gap reduce by order of most connected nodes to least connected
	// This should achieve the effect of bringing nodes into central areas of the diagram instead of isolated nodes dragging stuff to it
	sortedByConnections := make([]*layoutgraph.Node, len(nodes))
	copy(sortedByConnections, nodes)
	slices.SortFunc(sortedByConnections, func(a, b *layoutgraph.Node) int {
		if order := cmp.Compare(len(b.Edges), len(a.Edges)); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})

	changed := false

	// whole trees can move towards other nodes but we don't want to move within trees
	for _, node := range sortedByConnections {
		if _, in := g.NodeToTree[node]; in {
			continue
		}
		if node.Hierarchy != nil {
			continue
		}
		candidateTxn, err := txn.CloneGeometryContext()
		if err != nil {
			return changed, err
		}
		txn.AddOp(func() error {
			_, _, err := reduceGapToNeighbors(ctx, node, candidateTxn, gapReductionOptions{
				axis:                   options.axis,
				direction:              options.direction,
				attemptRecoverSymmetry: true,
				costTxn:                options.costTxn,
			})
			return err
		})
		if err := txn.Commit(ctx); err != nil {
			if !layoutgraph.IsCandidateRejection(err) {
				return changed, err
			}
		} else {
			if err := txn.UpdateState(); err != nil {
				return changed, err
			}
			changed = true
		}
		txn.Clear()
	}

	return changed, nil
}

// Assumes ahead is ahead of behind when forwards is true
func isBetween(node *layoutgraph.Node, behind, ahead *layoutgraph.Node, isHorizontal, forwards bool) bool {
	delta := float64(behind.DeltaTo(node, behind.TopLeft))
	if isHorizontal {
		// . ┌──────┐
		// . │ node │     false
		// . └──────┘   ─┬──────
		// .             │ delta
		// . ┌────────┐  ┴      ┌───────┐
		// . │ behind │         │ ahead │
		// . └────────┘         └───────┘
		if (node.TopLeft.Y + node.Height) < behind.TopLeft.Y-delta {
			return false
		}
		// . ┌────────┐         ┌───────┐
		// . │ behind │         │ ahead │
		// . └────────┘  ┬      └───────┘
		// .             │ delta
		// . ┌──────┐   ─┴───────
		// . │ node │     false
		// . └──────┘
		if node.TopLeft.Y > behind.TopLeft.Y+behind.Height+delta {
			return false
		}

		if !forwards {
			behind, ahead = ahead, behind
		}
		// ┌──────┐    ┌────────┐         ┌───────┐
		// │ node │    │ behind │         │ ahead │
		// └──────┘    └────────┘         └───────┘
		// n is behind behind
		if node.TopLeft.X+node.Width < behind.TopLeft.X+behind.Width {
			return false
		}
		// ┌────────┐         ┌───────┐     ┌──────┐
		// │ behind │         │ ahead │     │ node │
		// └────────┘         └───────┘     └──────┘
		// n is ahead of ahead
		if node.TopLeft.X > ahead.TopLeft.X {
			return false
		}
	} else {
		if (node.TopLeft.X + node.Width) < behind.TopLeft.X-delta {
			return false
		}
		if node.TopLeft.X > behind.TopLeft.X+behind.Width+delta {
			return false
		}

		if !forwards {
			behind, ahead = ahead, behind
		}
		if node.TopLeft.Y+node.Height < behind.TopLeft.Y+behind.Height {
			return false
		}
		if node.TopLeft.Y > ahead.TopLeft.Y {
			return false
		}
	}
	return true
}

// Assumes otherNode is ahead of node when forwards is true
func nearestBetween(nodes layoutgraph.Nodes, behind, ahead, inContainer *layoutgraph.Node, isHorizontal, forwards bool) *layoutgraph.Node {
	var nearest *layoutgraph.Node
	for _, n := range nodes {
		if n == behind || n == ahead {
			continue
		}
		if n.OwningContainer() != inContainer {
			continue
		}
		if !isBetween(n, behind, ahead, isHorizontal, forwards) {
			continue
		}
		if nearest == nil {
			nearest = n
			continue
		}
		if isHorizontal {
			if forwards {
				if n.TopLeft.X < nearest.TopLeft.X {
					nearest = n
				}
			} else {
				if (n.TopLeft.X + n.Width) > (nearest.TopLeft.X + nearest.Width) {
					nearest = n
				}
			}
		} else {
			if forwards {
				if n.TopLeft.Y < nearest.TopLeft.Y {
					nearest = n
				}
			} else {
				if (n.TopLeft.Y + n.Height) > (nearest.TopLeft.Y + nearest.Height) {
					nearest = n
				}
			}
		}
	}

	return nearest
}

func nearestConnectedAhead(node *layoutgraph.Node, isHorizontal, forwards bool) *layoutgraph.Node {
	var nearest *layoutgraph.Node

	for _, e := range node.Edges {
		adj := node.Adjacent(e)
		if isHorizontal {
			if forwards {
				// if adj's left is behind node's right its not ahead
				if adj.TopLeft.X < (node.TopLeft.X + node.Width) {
					continue
				}
				if nearest == nil || adj.TopLeft.X < nearest.TopLeft.X {
					nearest = adj
				}
			} else {
				// if adj's right is ahead of node's left its not behind
				if (adj.TopLeft.X + adj.Width) > node.TopLeft.X {
					continue
				}
				if nearest == nil || (adj.TopLeft.X+adj.Width > nearest.TopLeft.X+nearest.Width) {
					nearest = adj
				}
			}
		} else {
			if forwards {
				if adj.TopLeft.Y < (node.TopLeft.Y + node.Height) {
					continue
				}
				if nearest == nil || adj.TopLeft.Y < nearest.TopLeft.Y {
					nearest = adj
				}
			} else {
				if adj.TopLeft.Y+adj.Height > node.TopLeft.Y {
					continue
				}
				if nearest == nil || (adj.TopLeft.Y+adj.Height) > (nearest.TopLeft.Y+nearest.Height) {
					nearest = adj
				}
			}
		}
	}

	return nearest
}

// reduceGapToNeighbors attempts to reduce the gap between this node and its neighbors in front of it
// returns true if reduced, false if no changes. If true, also returns the reduced global edge length
//
// Imagine a scenario like this
// . ┌────┐
// . │ A  │
// . │    ├───┐
// . └───┬┘   │
// .     │  ┌─▼┐
// .     │  │B │
// .     │  └──┘
// .     │
// .     │
// .     │
// .     │
// .     │
// .     │
// .   ┌─▼───┐
// .   │     │
// .   │ C   │
// .   └─────┘
// IF C IS PULLING NEIGHBORS --------------
//
// C's nearest ahead is A, but it won't be able to move A + B down since it'll error
// Nonetheless, there's still a large gap, and the optimal place is to consider B to be its nearest ahead
// So we don't really want nearest ahead, we want the node connected to nearest ahead closest to C
//
// IF A IS PULLING NEIGHBORS --------------
//
// A's nearest ahead is C, but it won't be able to move to A
// So we sweep, get node B, and consider the gap between C and B as the amount it should try to close
func reduceGapToNeighbors(ctx context.Context, node *layoutgraph.Node, txn *layoutgraph.Transaction, options gapReductionOptions) (bool, float64, error) {
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	if !options.axis.valid() {
		return false, 0, fmt.Errorf("TALA gap reduction requires an axis")
	}
	if !options.direction.valid() {
		return false, 0, fmt.Errorf("TALA gap reduction requires a direction")
	}
	isHorizontal := options.axis.isHorizontal()
	forwards := options.direction.isForward()
	attemptRecoverSymmetry := options.attemptRecoverSymmetry
	// Needed for calculating partial symmetry
	if node.Graph.CellSize == 0 {
		node.Graph.ComputeCellSize()
	}

	nearestAhead := nearestConnectedAhead(node, isHorizontal, forwards)
	if nearestAhead == nil {
		return false, 0, nil
	}
	// if nearest ahead is fixed or is within a fixed node, we can't pull it closer
	for c := nearestAhead.Container; c != nil; c = c.Container {
		if c.FixedTopLeft != nil {
			return false, 0, nil
		}
	}
	excluded := []*layoutgraph.Node{node}
	if node.HerdAssignment != nil {
		// when normalizing the gap from `4` to `1`, it selects `c1`, `2`, `3`, `5`, `6`
		// as connected nodes, however, `5` and `6` shouldn't be considered because
		// when moving the connected nodes towards `4` it will break the herd alignment
		// ┌─────────────────────────────────────────────────────────────────┐
		// │    ┌─────────────┐        ┌─────────────┐     ┌─────────────┐   │
		// │    │             │        │             │     │             │   │
		// │    │             │        │             │     │             │   │
		// │    │     1       │   c1   │     2       │     │      3      │   │
		// │    │             │        │             │     │             │   │
		// │    │             │        │             │     │             │   │
		// │    └─────┬───────┘        └──────┬──────┘     └──────┬──────┘   │
		// └──────────┼───────────────────────┼───────────────────┼──────────┘
		//            │                       │                   │
		//            │                       │                   │
		//  ┌─────────┼───────────────────────┼───────────────────┼──────────┐
		//  │    ┌────▼────────┐        ┌─────▼───────┐     ┌─────▼───────┐  │
		//  │    │             │        │             │     │             │  │
		//  │    │             │        │             │     │             │  │
		//  │    │     4       │   c2   │     5       │     │     6       │  │
		//  │    │             │        │             │     │             │  │
		//  │    │             │        │             │     │             │  │
		//  │    └─────────────┘        └─────────────┘     └─────────────┘  │
		//  └────────────────────────────────────────────────────────────────┘
		for _, sibling := range node.Graph.Containers[node.Container] {
			if sibling == node || sibling.HerdAssignment == nil || nearestAhead.IsDescendantOf(sibling) {
				continue
			}
			if sibling.HerdAssignment.Orientation == node.HerdAssignment.Orientation {
				excluded = append(excluded, sibling)
			}
		}
	}

	sharedContainer := node.NearestSharedAncestor(nearestAhead)
	nearestBetween := nearestBetween(layoutgraph.Nodes(nearestAhead.ConnectedNodes(excluded, node.Graph)), node,
		nearestAhead,
		sharedContainer,
		isHorizontal,
		forwards,
	)

	// we don't want fixed nodes preventing us from getting the actual nearest between (could be a fixed node's container)
	// but we only want to move connected nodes up until we reach a fixed node
	excluded = append(excluded, node.Graph.FixedNodes()...)
	connectedNodes := nearestAhead.ConnectedNodeSet(excluded, node.Graph)

	if nearestBetween != nil {
		nearestAhead = nearestBetween
	}

	if options.costTxn != nil {
		if err := options.costTxn.CapturePlacementCosts("GapNormalizationTransactions"); err != nil {
			return false, 0, err
		}
	}
	oldEdgeLength, err := placementcost.EdgeLength(ctx, node.Graph, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: false})
	if err != nil {
		return false, 0, err
	}
	newEdgeLength := oldEdgeLength

	nodesInBetween := []*layoutgraph.Node{}
	for _, n := range node.Graph.Nodes {
		if n == node || n == nearestAhead {
			continue
		}
		isAConnectedNode := slices.Contains(connectedNodes, n)
		if isAConnectedNode {
			continue
		}
		if forwards {
			if n.IsBlocked(node, nearestAhead, true, isHorizontal) {
				nodesInBetween = append(nodesInBetween, n)
			}
		} else {
			if n.IsBlocked(nearestAhead, node, true, isHorizontal) {
				nodesInBetween = append(nodesInBetween, n)
			}
		}
	}

	var backwardTxn *layoutgraph.Transaction
	// We try pulling the nearest ahead to any nodes in between
	for _, candidateNode := range append([]*layoutgraph.Node{node}, nodesInBetween...) {
		// if nearest ahead is less nested, we only want move it to the candidate's ancestor in the same container
		// Note: we should always reach nearestAhead's Container, since it should be the outermost connected node
		for candidateNode.OwningContainer() != nearestAhead.OwningContainer() {
			candidateNode = candidateNode.OwningContainer()
			if candidateNode == nil {
				break
			}
		}
		if candidateNode == nil {
			continue
		}
		var gapSize float64
		if isHorizontal {
			if forwards {
				gapSize = nearestAhead.TopLeft.X - (candidateNode.TopLeft.X + candidateNode.Width)
			} else {
				gapSize = candidateNode.TopLeft.X - (nearestAhead.TopLeft.X + nearestAhead.Width)
			}
		} else {
			if forwards {
				gapSize = nearestAhead.TopLeft.Y - (candidateNode.TopLeft.Y + candidateNode.Height)
			} else {
				gapSize = candidateNode.TopLeft.Y - (nearestAhead.TopLeft.Y + nearestAhead.Height)
			}
		}

		if gapSize <= largeGapThreshold*node.Graph.CellSize {
			continue
		}

		// TODO 150 seems a bit large for ideal gap size
		delta := placementcost.IdealGapSize - gapSize
		if !forwards {
			delta = -delta
		}
		if delta == 0 {
			continue
		}

		var candidateEdgeLength float64
		if txn == nil {
			var transactionErr error
			txn, transactionErr = node.Graph.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
			if transactionErr != nil {
				return false, newEdgeLength, transactionErr
			}
		} else {
			txn.Clear()
			if err := txn.UpdateState(); err != nil {
				return false, newEdgeLength, err
			}
		}
		txn.AddOp(func() error {
			for _, n := range connectedNodes {
				if isHorizontal {
					n.Translate(delta, 0)
				} else {
					n.Translate(0, delta)
				}
			}
			node.Graph.SyncClusters()
			node.Graph.SyncSequences()
			var err error
			candidateEdgeLength, err = placementcost.EdgeLength(ctx, node.Graph, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: false})
			if err != nil {
				return err
			}

			// Before we commit to anything, we try to "recover symmetry" by running gap reduction on the opposite side
			// E.g. if running on b, c might be brought closer and lose symmetry. But bringing a closer recovers that symmetry
			// ┌───────────────────────────────────────────────────┐
			// │                                                   │
			// │  ┌──┐                 ┌──┐                  ┌──┐  │
			// │  │a │---------------->│b │<-----------------│c │  │
			// │  └──┘                 └┬─┘                  └──┘  │
			// │                        │                          │
			// │                        │                          │
			// │                        │                          │
			// │                       ┌┴─┐                        │
			// │                       │d │                        │
			// │                       └──┘                        │
			// │                                                   │
			// └───────────────────────────────────────────────────┘
			// There are 3 scenarios:
			// 1. The gap reduction without backwards run is best
			// 2. The gap reduction plus backwards run is best
			// 3. No gap reduction is best
			// There is a fourth, which is that gap reduction backwards is best, but that's checked in outer
			if attemptRecoverSymmetry {
				if backwardTxn == nil {
					var cloneErr error
					backwardTxn, cloneErr = txn.CloneGeometryContext()
					if cloneErr != nil {
						return cloneErr
					}
				}
				backwardTxn.Clear()
				if err := backwardTxn.UpdateState(); err != nil {
					return err
				}
				mirroredTxn, cloneErr := backwardTxn.CloneGeometryContext()
				if cloneErr != nil {
					return cloneErr
				}
				backwardTxn.AddOp(func() error {
					moved, mirroredEdgeLength, err := reduceGapToNeighbors(ctx, node, mirroredTxn, gapReductionOptions{
						axis:      options.axis,
						direction: options.direction.opposite(),
						costTxn:   options.costTxn,
					})
					if err != nil {
						return err
					}

					if moved && (mirroredEdgeLength < candidateEdgeLength && mirroredEdgeLength < oldEdgeLength) {
						return nil
					}
					return layoutgraph.ErrNonImprovingCandidate
				})
				if err := backwardTxn.Commit(ctx); err != nil {
					if !layoutgraph.IsCandidateRejection(err) {
						return err
					}
				} else {
					// If the backward commit was successful, it means it was optimal
					return nil
				}
			}

			if candidateEdgeLength >= oldEdgeLength {
				return layoutgraph.ErrNonImprovingCandidate
			}

			return nil
		})
		if err := txn.Commit(ctx); err != nil {
			txn.Clear()
			// Continue searching after an invalid candidate. A non-improving
			// candidate cannot get better by searching farther forward.
			if errors.Is(err, layoutgraph.ErrNonImprovingCandidate) {
				break
			}
			if !errors.Is(err, layoutgraph.ErrInvalidCandidate) {
				return false, newEdgeLength, err
			}
		} else {
			newEdgeLength = candidateEdgeLength
			txn.Clear()
			if err := txn.UpdateState(); err != nil {
				return false, newEdgeLength, err
			}
			break
		}
	}

	// try moving non-fixed node to the side of its container

	if node.Container != nil && node.Container != nearestAhead.Container && node.FixedTopLeft == nil {
		for node.OwningContainer() != nearestAhead.OwningContainer() {
			innerBox := node.Container.InnerBox()

			var gapSize float64
			if isHorizontal {
				if forwards {
					gapSize = innerBox.TopLeft.X + innerBox.Width - (node.TopLeft.X + node.Width)
				} else {
					gapSize = node.TopLeft.X - innerBox.TopLeft.X
				}
			} else {
				if forwards {
					gapSize = innerBox.TopLeft.Y + innerBox.Height - (node.TopLeft.Y + node.Height)
				} else {
					gapSize = node.TopLeft.Y - innerBox.TopLeft.Y
				}
			}

			padding := node.Graph.ContainerPadding(node.Container, true)
			delta := gapSize
			if isHorizontal {
				if forwards {
					delta -= padding.Right()
				} else {
					delta -= padding.Left()
				}
			} else {
				if forwards {
					delta -= padding.Bottom()
				} else {
					delta -= padding.Top()
				}
			}
			if !forwards {
				delta = -delta
			}
			if gapSize > largeGapThreshold*node.Graph.CellSize && delta != 0 {
				if txn == nil {
					var transactionErr error
					txn, transactionErr = node.Graph.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
					if transactionErr != nil {
						return false, newEdgeLength, transactionErr
					}
				} else {
					txn.Clear()
					if err := txn.UpdateState(); err != nil {
						return false, newEdgeLength, err
					}
				}
				txn.AddOp(func() error {
					if isHorizontal {
						node.MoveWithChildren(delta, 0)
					} else {
						node.MoveWithChildren(0, delta)
					}
					return nil
				})
				rolledBack := false
				if err := txn.Commit(ctx); err != nil {
					if !layoutgraph.IsCandidateRejection(err) {
						return false, newEdgeLength, err
					}
					rolledBack = true
				} else {
					movedEdgeLength, err := placementcost.EdgeLength(ctx, node.Graph, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: false})
					if err != nil {
						txn.Rollback()
						return false, newEdgeLength, err
					}

					if geo.PrecisionCompare(movedEdgeLength, newEdgeLength, geo.PRECISION) >= 0 {
						txn.Rollback()
						rolledBack = true

					} else {
						newEdgeLength = movedEdgeLength
					}
				}
				if rolledBack {
					txn.Clear()
					// Alright so if moving to the side of the container obstructs something, you can move PADDING away from the next thing in that axis
					// E.g. can't move A all the way to the side, but can move close to B whereby it's still reducing distance
					//
					// ┌───────────────────────────────────────────────┐
					// │                                               │
					// │                                               │
					// │     ┌─────┬───────────────────────────────────┼────────────►
					// │     │     │                                   │
					// │     │ A   │                    ┌──────────────┼──────────►
					// │     │     │                    │              │
					// │     └─────┘                  ┌─┴───┐          │
					// │                              │     │          │
					// │                              │ B   │          │
					// │                              │     │          │
					// │                              └─────┘          │
					// │                                               │
					// │                                               │
					// │                                               │
					// └───────────────────────────────────────────────┘
					//
					leastDistance := math.Inf(1)
					for _, otherNode := range node.Graph.Containers[node.OwningContainer()] {
						if node == otherNode {
							continue
						}
						if isHorizontal {
							if forwards {
								x := otherNode.TopLeft.X - placementcost.IdealGapSize
								if x > node.TopLeft.X+node.Width {
									if x-(node.TopLeft.X+node.Width) < leastDistance {
										leastDistance = x - (node.TopLeft.X + node.Width)
									}
								}
							} else {
								x := otherNode.TopLeft.X + otherNode.Width + placementcost.IdealGapSize
								if x < node.TopLeft.X {
									if node.TopLeft.X-x < leastDistance {
										leastDistance = node.TopLeft.X - x
									}
								}
							}
						} else {
							if forwards {
								y := otherNode.TopLeft.Y - placementcost.IdealGapSize
								if y > node.TopLeft.Y+node.Height {
									if y-(node.TopLeft.Y+node.Height) < leastDistance {
										leastDistance = y - (node.TopLeft.Y + node.Height)
									}
								}
							} else {
								y := otherNode.TopLeft.Y + otherNode.Height + placementcost.IdealGapSize
								if y < node.TopLeft.Y {
									if node.TopLeft.Y-y < leastDistance {
										leastDistance = node.TopLeft.Y - y
									}
								}
							}
						}
					}
					if !math.IsInf(leastDistance, 1) {
						delta = leastDistance
						if !forwards {
							delta = -delta
						}
						txn.AddOp(func() error {
							if isHorizontal {
								node.MoveWithChildren(delta, 0)
							} else {
								node.MoveWithChildren(0, delta)
							}
							return nil
						})
						if err := txn.Commit(ctx); err != nil {
							if !layoutgraph.IsCandidateRejection(err) {
								return false, newEdgeLength, err
							}
						} else {
							movedEdgeLength, err := placementcost.EdgeLength(ctx, node.Graph, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: false})
							if err != nil {
								txn.Rollback()
								return false, newEdgeLength, err
							}
							if geo.PrecisionCompare(movedEdgeLength, newEdgeLength, geo.PRECISION) >= 0 {
								txn.Rollback()
							} else {
								newEdgeLength = movedEdgeLength
								txn.Clear()
								if err := txn.UpdateState(); err != nil {
									return false, newEdgeLength, err
								}
							}
						}
					}
				}
			}
			node = node.OwningContainer()
			if node.Container == nil {
				break
			}
		}
	}
	return newEdgeLength != oldEdgeLength, newEdgeLength, nil
}
