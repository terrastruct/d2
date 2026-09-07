package placement

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/lib/geo"
)

type clusterDesiredArrangementSnapshot struct {
	cluster     *layoutgraph.Cluster
	arrangement layoutgraph.ClusterArrangement
	set         bool
}

type optimizeClustersRollback struct {
	graph              *layoutgraph.Graph
	graphState         *layoutgraph.GraphState
	desiredArrangement clusterDesiredArrangementSnapshot
	cellSize           float64
	costs              *layoutgraph.PlacementCostSnapshot
}

func (rollback *optimizeClustersRollback) recordDesiredArrangement(cluster *layoutgraph.Cluster, arrangement layoutgraph.ClusterArrangement) {
	// The first transaction snapshot contains every later cluster's entry value.
	// Only a change made before that snapshot exists needs a separate journal.
	if rollback.graphState != nil || rollback.desiredArrangement.set || cluster.DesiredArrangement == arrangement {
		return
	}
	rollback.desiredArrangement = clusterDesiredArrangementSnapshot{
		cluster:     cluster,
		arrangement: cluster.DesiredArrangement,
		set:         true,
	}
}

func (rollback *optimizeClustersRollback) recordGraphState(txn *layoutgraph.Transaction) bool {
	if rollback.graphState != nil {
		return false
	}
	rollback.graphState = txn.PriorGraphState
	return true
}

func (rollback *optimizeClustersRollback) capturePlacementCosts(guard *limits.WorkGuard) error {
	if rollback.costs != nil {
		return nil
	}
	cacheEntries := rollback.graph.EdgeLengthCacheEntries()
	if int64(cacheEntries) > layoutgraph.MaxTopologyReferences {
		return fmt.Errorf("TALA OptimizeClustersTransactions edge-length cache entries exceed limit %d", layoutgraph.MaxTopologyReferences)
	}
	for range cacheEntries {
		if err := guard.Step(); err != nil {
			return err
		}
	}
	costs := rollback.graph.SnapshotPlacementCosts()
	if err := guard.Finish(); err != nil {
		return err
	}
	rollback.costs = costs
	return nil
}

func (rollback *optimizeClustersRollback) restore() {
	if rollback.graphState != nil {
		layoutgraph.RestoreGraphState(rollback.graph, rollback.graphState)
	}
	if rollback.desiredArrangement.set {
		rollback.desiredArrangement.cluster.DesiredArrangement = rollback.desiredArrangement.arrangement
	}
	rollback.graph.CellSize = rollback.cellSize
	if rollback.costs != nil {
		rollback.costs.Restore()
	}
}

func alignConnectedNodes(c *layoutgraph.Cluster, horizontally bool) {
	externalNodes := clusterExternalConnectedNodes(c)
	if len(externalNodes) == 0 {
		return
	}

	// align every node center with cluster center
	for _, n := range externalNodes {
		if n.FixedTopLeft != nil {
			continue
		}
		// Don't want it to travel too far, risk of fucking up other things
		if !n.OverlapsAlongDimension(c.Vessel, !horizontally, true) {
			continue
		}
		xDelta := 0.0
		yDelta := 0.0
		if horizontally {
			xDelta = math.Round(c.Vessel.TopLeft.X + c.Vessel.Width/2 - n.TopLeft.X - n.Width/2)
		} else {
			yDelta = math.Round(c.Vessel.TopLeft.Y + c.Vessel.Height/2 - n.TopLeft.Y - n.Height/2)
		}
		n.MoveWithChildren(xDelta, yDelta)
	}
}

func alignVessel(c *layoutgraph.Cluster, horizontally bool) {
	externalNodes := clusterExternalConnectedNodes(c)
	if len(externalNodes) == 0 {
		return
	}

	avgExternalX := 0.0
	avgExternalY := 0.0
	for _, n := range externalNodes {
		avgExternalX += n.Center().X
		avgExternalY += n.Center().Y
	}
	avgExternalX /= float64(len(externalNodes))
	avgExternalY /= float64(len(externalNodes))

	xDelta := 0.0
	yDelta := 0.0
	// align cluster center with avg external node center
	if horizontally {
		xDelta = avgExternalX - c.Vessel.Width/2 - c.Vessel.TopLeft.X
	} else {
		yDelta = avgExternalY - c.Vessel.Height/2 - c.Vessel.TopLeft.Y
	}

	c.Vessel.TopLeft.X += math.Round(xDelta)
	c.Vessel.TopLeft.Y += math.Round(yDelta)
}

// A cluster is considered optimized when
// - its orientation is perpendicular to its edges
// - it's aligned symmetrically to its connected nodes
// If we cannot rotate to the right orientation, then give up (wrongly oriented + symmetrical looks worse than nonsymmetrical)
// If we are able to rotate to the right orientation (or we already are), then try to align
// If it's diagonal, use the one that matches the aspect ratio
func optimizeCluster(ctx context.Context, c *layoutgraph.Cluster, onlyFlip bool) (bool, error) {
	return optimizeClusterWithRollback(ctx, c, onlyFlip, nil)
}

func optimizeClusterWithRollback(ctx context.Context, c *layoutgraph.Cluster, onlyFlip bool, rollback *optimizeClustersRollback) (bool, error) {
	ctx, guard, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "OptimizeClusterTransactions")
	if err != nil {
		return false, err
	}
	horizontalCount := 0
	verticalCount := 0
	changed := false
	for _, externalNode := range clusterExternalConnectedNodes(c) {
		orientation := c.Vessel.Orientation(externalNode)
		if orientation == geo.NONE {
			continue
		}

		switch orientation {
		case geo.Left, geo.Right:
			horizontalCount++
		case geo.Top, geo.Bottom:
			verticalCount++
		default:
			// Diagonal
			xDistance := math.Abs(externalNode.Center().X - c.Vessel.Center().X)
			yDistance := math.Abs(externalNode.Center().Y - c.Vessel.Center().Y)
			if xDistance > yDistance {
				horizontalCount++
			} else if yDistance > xDistance {
				verticalCount++
			}
		}
	}

	desiredArrangement := c.DesiredArrangement
	if verticalCount > horizontalCount {
		desiredArrangement = layoutgraph.Row
	} else if horizontalCount > verticalCount {
		desiredArrangement = layoutgraph.Column
	}
	if rollback != nil {
		rollback.recordDesiredArrangement(c, desiredArrangement)
	}
	c.DesiredArrangement = desiredArrangement

	var txn *layoutgraph.Transaction
	ownsRollbackState := false
	if c.Arrangement != desiredArrangement {
		var transactionErr error
		txn, transactionErr = c.Graph.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
		if transactionErr != nil {
			return changed, transactionErr
		}
		if rollback != nil {
			ownsRollbackState = rollback.recordGraphState(txn)
		}
		// first try flipping around center, if that fails try flipping around top left
		tryCenter := true
		txn.AddOp(func() error {
			switch c.Arrangement {
			case layoutgraph.Column:
				c.Arrangement = layoutgraph.Row
			case layoutgraph.Row:
				c.Arrangement = layoutgraph.Column
			}
			c.Padding = grouping.PaddingBetween(c, true)
			c.SyncGeometry()

			if tryCenter {
				// sync from column to row (top left is fixed):
				// ┌┼┐ ┌┼────┐  ┼: original centerX
				// │ │ │     │
				// │ │ └─────┘
				// │ │
				// └─┘
				// then centering:
				// ┌──┼──┐
				// │     │
				// └─────┘
				switch c.Arrangement {
				case layoutgraph.Column:
					_, originalHeight := txn.OriginalDimensions(c.Vessel)
					heightDelta := originalHeight - c.Vessel.Height
					c.Vessel.MoveWithChildren(0, math.Round(heightDelta/2))
				case layoutgraph.Row:
					originalWidth, _ := txn.OriginalDimensions(c.Vessel)
					widthDelta := originalWidth - c.Vessel.Width
					c.Vessel.MoveWithChildren(math.Round(widthDelta/2), 0)
				}
			}

			if c.Container != nil && c.Container.TopLeft != nil {
				children := layoutgraph.Nodes(c.Graph.Containers[c.Container])
				childrenTL, childrenBR := children.FixedBoundingBox()
				padding := c.Graph.ContainerPadding(c.Container, false)
				c.Container.FitToBoundingBox(childrenTL, childrenBR, padding)
				c.Container.PositionContainerChildren(true)
			}
			return nil
		})
		if err := txn.Commit(ctx); err != nil {
			if !layoutgraph.IsCandidateRejection(err) {
				return changed, err
			}
			tryCenter = false
			if err := txn.Commit(ctx); err != nil {
				if !layoutgraph.IsCandidateRejection(err) {
					return changed, err
				}
				return changed, nil
			}
		}
		if err := txn.UpdateState(); err != nil {
			return changed, err
		}
		txn.Clear()
		changed = true
	}

	if !onlyFlip && c.Arrangement == desiredArrangement {
		// There's two ways alignment can happen, vessel moves or external nodes move
		// First try vessel move
		if txn == nil {
			var transactionErr error
			txn, transactionErr = c.Graph.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
			if transactionErr != nil {
				return changed, transactionErr
			}
			if rollback != nil {
				ownsRollbackState = rollback.recordGraphState(txn)
			}
		}
		if rollback != nil {
			if err := rollback.capturePlacementCosts(guard); err != nil {
				return changed, err
			}
		}
		bestLength, err := placementcost.EdgeLength(ctx, c.Graph, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
		if err != nil {
			return changed, err
		}
		moveVessel := false
		moveNodes := false

		txn.AddOp(func() error {
			switch c.Arrangement {
			case layoutgraph.Column:
				alignVessel(c, false)
			case layoutgraph.Row:
				alignVessel(c, true)
			}
			return nil
		})
		err = txn.Commit(ctx)
		if err == nil {
			length, scoreErr := placementcost.EdgeLength(ctx, c.Graph, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
			if scoreErr != nil {
				txn.Rollback()
				txn.Clear()
				return changed, scoreErr
			}
			if geo.PrecisionCompare(length, bestLength, geo.PRECISION) < 0 {
				bestLength = length
				moveVessel = true
			}
		} else if !layoutgraph.IsCandidateRejection(err) {
			return changed, err
		}
		txn.Rollback()
		txn.Clear()

		txn.AddOp(func() error {
			switch c.Arrangement {
			case layoutgraph.Column:
				alignConnectedNodes(c, false)
			case layoutgraph.Row:
				alignConnectedNodes(c, true)
			}

			return nil
		})
		err = txn.Commit(ctx)
		if err == nil {
			length, scoreErr := placementcost.EdgeLength(ctx, c.Graph, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
			if scoreErr != nil {
				txn.Rollback()
				txn.Clear()
				return changed, scoreErr
			}
			if geo.PrecisionCompare(length, bestLength, geo.PRECISION) < 0 {
				moveNodes = true
			}
		} else if !layoutgraph.IsCandidateRejection(err) {
			return changed, err
		}

		txn.Rollback()
		txn.Clear()

		if moveNodes {
			txn.AddOp(func() error {
				switch c.Arrangement {
				case layoutgraph.Column:
					alignConnectedNodes(c, false)
				case layoutgraph.Row:
					alignConnectedNodes(c, true)
				}

				return nil
			})
			if err := txn.Commit(ctx); err != nil {
				return changed, err
			}
			changed = true
		} else if moveVessel {
			txn.AddOp(func() error {
				switch c.Arrangement {
				case layoutgraph.Column:
					alignVessel(c, false)
				case layoutgraph.Row:
					alignVessel(c, true)
				}
				return nil
			})
			if err := txn.Commit(ctx); err != nil {
				return changed, err
			}
			changed = true
		}

		if moveNodes || moveVessel {
			// after flipping we may need to pull connected nodes closer
			txn.Clear()
			if ownsRollbackState {
				// The first transaction's original graph state is the stage rollback
				// point. Detach before a second refresh could recycle that state as
				// transaction scratch.
				var cloneErr error
				txn, cloneErr = txn.CloneGeometryContext()
				if cloneErr != nil {
					return changed, cloneErr
				}
			}
			if err := txn.UpdateState(); err != nil {
				return changed, err
			}
			txn.AddOp(func() error {
				_, _, err := reduceGapToNeighbors(ctx, c.Vessel, nil, gapReductionOptions{
					axis:                   axisForArrangement(c.Arrangement),
					direction:              forwardDirection,
					attemptRecoverSymmetry: true,
				})
				return err
			})
			if err := txn.Commit(ctx); err != nil {
				if !layoutgraph.IsCandidateRejection(err) {
					return changed, err
				}
			}
		}
	}

	return changed, nil
}

// make clusters horizontal if cluster connections are above or below cluster
func OptimizeClusters(ctx context.Context, g *layoutgraph.Graph) (bool, error) {
	ctx, _, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "OptimizeClustersTransactions")
	if err != nil {
		return false, err
	}
	order := g.ClusterRDFSOrder()
	if len(order) == 0 {
		return false, nil
	}
	return optimizeClustersAtomic(ctx, g, order)
}

func optimizeClustersAtomic(ctx context.Context, g *layoutgraph.Graph, order []*layoutgraph.Node) (bool, error) {
	rollback := &optimizeClustersRollback{graph: g, cellSize: g.CellSize}
	complete := false
	defer func() {
		if !complete {
			rollback.restore()
		}
	}()
	changed := false
	for _, vessel := range order {
		cluster := g.Clusters[vessel]
		changed2, err := optimizeClusterWithRollback(ctx, cluster, false, rollback)
		if err != nil {
			return false, err
		}
		if changed2 {
			changed = true
		}
	}

	complete = true
	return changed, nil
}
