package placement

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"

	"github.com/d2lang/d2/lib/geo"
)

// alignAxes tries to align connected nodes that currently differ on both axes.
// Below, a and b can be lined up. If b has more connections, consider its subgraph.
// .
// . ┌──────┐
// . │      │
// . │ a    │
// . │      │
// . └───┬──┘
// .     │
// .     │           ┌─────┐       ┌─────┐
// .     │           │     │       │     │
// .     └───────────►  b  ├───────►     │
// .                 └─────┘       └─────┘
func alignAxes(ctx context.Context, g *layoutgraph.Graph, txn *layoutgraph.Transaction) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	changed := false

	fixedNodes := g.FixedNodes()

	// Note: the edge from rootSentinel to tree root is not excluded here since if the root moves,
	// all connected nodes are moved so it should move the whole tree with it which would be okay
	isTreeEdge := g.TreeEdgeMap()
	g.AddIsolatedTreeEdges(isTreeEdge)
	// Tree nodes may be connected nodes when they are containers, so exclude them from the connected-node search.
	excluded := make([]*layoutgraph.Node, 0, len(g.NodeToTree)+len(fixedNodes)+1)
	for _, n := range g.Nodes {
		if _, has := g.NodeToTree[n]; has {
			excluded = append(excluded, n)
		}
	}
	excluded = append(excluded, fixedNodes...)
	excluded = append(excluded, nil) // last node is replaced with e.To/e.From below

	var edgeAbductions []*layoutgraph.EdgeAbduction
	for _, seq := range g.Sequences {
		edgeAbductions = append(edgeAbductions, seq.EdgeAbductions...)
	}

	// need to compute it here because as we align a single edge, it may end up aligning other edges
	// and it would affect the actual count on the fly
	hasTableColumns := false
	initialLength, err := placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		return false, err
	}
	nInitialAlignedTableColumns := 0
	for _, e := range g.Edges {
		if e.HasTableColumn() {
			hasTableColumns = true
			if e.IsAxisAligned() {
				nInitialAlignedTableColumns++
			}
		}
	}

	for _, e := range g.Edges {
		if _, is := isTreeEdge[e]; is {
			continue
		}
		if e.IsAxisAligned() {
			continue
		}
		if e.From.Hierarchy != nil || e.To.Hierarchy != nil {
			continue
		}
		txn.Clear()
		if err := txn.UpdateState(); err != nil {
			return false, err
		}

		xDiff, yDiff := alignmentDeltas(e, g)

		var dxBest, dyBest float64
		var bestNodes []*layoutgraph.Node
		bestEdgeLength, err := placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
		if err != nil {
			return false, err
		}
		containerAlignment, err := placementcost.ContainerAlignmentCost(ctx, g)
		if err != nil {
			return false, err
		}
		bestEdgeLength += containerAlignment
		// Try to move e.From
		{
			excluded[len(excluded)-1] = e.To
			connectedNodes := e.From.ConnectedNodeSet(excluded, g)
			edgeLength, dx, dy, err := tryMove(ctx, txn, g, e, connectedNodes, edgeAbductions, xDiff, yDiff)
			if err != nil {
				return false, err
			}
			if geo.PrecisionCompare(edgeLength, bestEdgeLength, geo.PRECISION) < 0 {
				bestEdgeLength = edgeLength
				dxBest, dyBest = dx, dy
				bestNodes = connectedNodes
			} else if e.HasTableColumn() && geo.PrecisionCompare(edgeLength, bestEdgeLength, geo.PRECISION) == 0 {
				// aligning ports in tables might be a sort of micromovement that doesn't change that much and it results in
				// the new length being equal to the old one at our precision level, but still, the aligned ports look better
				bestEdgeLength = edgeLength
				dxBest, dyBest = dx, dy
				bestNodes = connectedNodes
			}
		}

		// Try to move e.To
		{
			excluded[len(excluded)-1] = e.From
			connectedNodes := e.To.ConnectedNodeSet(excluded, g)
			edgeLength, dx, dy, err := tryMove(ctx, txn, g, e, connectedNodes, edgeAbductions, -xDiff, -yDiff)
			if err != nil {
				return false, err
			}
			if geo.PrecisionCompare(edgeLength, bestEdgeLength, geo.PRECISION) < 0 {
				dxBest, dyBest = dx, dy
				bestNodes = connectedNodes
			} else if e.HasTableColumn() && geo.PrecisionCompare(edgeLength, bestEdgeLength, geo.PRECISION) == 0 {
				dxBest, dyBest = dx, dy
				bestNodes = connectedNodes
			}
		}

		if len(bestNodes) > 0 {
			txn.AddOp(func() error {
				changed = true
				for _, n := range bestNodes {
					n.Translate(dxBest, dyBest)
				}
				return nil
			})
			if err := txn.Commit(ctx); err != nil {
				return false, err
			}
		}
	}

	if hasTableColumns && changed {
		// column alignment can cause small changes in edge length that that "stay the same"
		// at our precision comparison. To handle it properly, we also count the number of
		// aligned edges
		nAlignedTableColumns := 0
		for _, e := range g.Edges {
			if e.HasTableColumn() && e.IsAxisAligned() {
				nAlignedTableColumns++
			}
		}
		// Without more aligned columns, accept only a strict edge-length improvement.
		if nAlignedTableColumns <= nInitialAlignedTableColumns {
			edgeLength, err := placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
			if err != nil {
				return false, err
			}
			containerAlignment, err := placementcost.ContainerAlignmentCost(ctx, g)
			if err != nil {
				return false, err
			}
			edgeLength += containerAlignment
			changed = geo.PrecisionCompare(edgeLength, initialLength, geo.PRECISION) == -1
		}
	}

	return changed, nil
}

// alignmentDeltas computes the X and Y deltas required to center-align the connected nodes.
// Exceptions:
// - for edges between table columns, return the delta to align the columns
// - for sequences, return the delta to align with the sequence step
func alignmentDeltas(e *layoutgraph.Edge, g *layoutgraph.Graph) (float64, float64) {
	if e.HasTableColumn() {
		fromPort, toPort, hasFromPort, hasToPort, _ := e.FacingTablePortValues(nil, nil)
		if hasFromPort && hasToPort {
			return 0, toPort.Y - fromPort.Y
		} else if hasFromPort {
			return 0, (e.To.TopLeft.Y + e.To.Height/2) - fromPort.Y
		} else if hasToPort {
			return 0, toPort.Y - (e.From.TopLeft.Y + e.From.Height/2)
		} else {
			// Align tables to the left to avoid issues like the one below
			// that can happen if we center align the tables
			//              ┌───────────────────────┐
			//              │                       │
			//              │                       ├─────┐
			//              └───────────────────────┘     │
			//                                  ┌─────────┘
			// ┌────────┐          ┌────────┐   │      ┌────────┐
			// │        ├──────────┤        ├───┘      │        │
			// │        │          │        ├──────────┤        │
			// └────────┘          └────────┘          └────────┘
			return e.To.TopLeft.X - e.From.TopLeft.X, 0
		}
	}
	from := e.From
	if seq, is := g.Sequences[from]; is {
		from = seq.AbductedNodeByEdge(e)
	}
	to := e.To
	if seq, is := g.Sequences[to]; is {
		to = seq.AbductedNodeByEdge(e)
	}

	yDiff := math.Round((to.TopLeft.Y + to.Height/2) - (from.TopLeft.Y + from.Height/2))
	xDiff := math.Round((to.TopLeft.X + to.Width/2) - (from.TopLeft.X + from.Width/2))

	if e.From.Orientation(e.To).IsHorizontal() {
		xDiff = 0
	} else if e.From.Orientation(e.To).IsVertical() {
		yDiff = 0
	} else {
		xDiff2, yDiff2 := e.From.OrthogonalDistanceTo(e.To)
		if xDiff2 < layoutgraph.ConnectedNodeGap {
			yDiff = 0
		}
		if yDiff2 < layoutgraph.ConnectedNodeGap {
			xDiff = 0
		}
	}
	return xDiff, yDiff
}

func tryMove(ctx context.Context, txn *layoutgraph.Transaction, g *layoutgraph.Graph, e *layoutgraph.Edge, connectedNodes []*layoutgraph.Node, edgeAbductions []*layoutgraph.EdgeAbduction, xDiff, yDiff float64) (bestLen, dx, dy float64, err error) {
	bestEdgeLength := math.Inf(1)
	var bestAttempt geo.Point
	attempts := make([]*geo.Point, 0, 2)

	if geo.PrecisionCompare(yDiff, 0.0, geo.PRECISION) != 0 {
		// e.From tries to slide up or down to e.To
		attempts = append(attempts, geo.NewPoint(0, yDiff))
	}

	if geo.PrecisionCompare(xDiff, 0.0, geo.PRECISION) != 0 {
		// e.From tries to slide left or right to e.To
		attempts = append(attempts, geo.NewPoint(xDiff, 0))
	}

	for _, attempt := range attempts {
		txn.AddOp(func() error {
			if !attemptShift(g, e, connectedNodes, attempt.X, attempt.Y) {
				return layoutgraph.ErrInvalidCandidate
			}
			return nil
		})

		if err := txn.Commit(ctx); err == nil {
			edgeLength, scoreErr := placementcost.EdgeLength(ctx, g, placementcost.EdgeLengthOptions{EdgeAbductions: edgeAbductions, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
			if scoreErr != nil {
				txn.Rollback()
				txn.Clear()
				return 0, 0, 0, scoreErr
			}
			containerAlignment, scoreErr := placementcost.ContainerAlignmentCost(ctx, g)
			if scoreErr != nil {
				txn.Rollback()
				txn.Clear()
				return 0, 0, 0, scoreErr
			}
			edgeLength += containerAlignment
			if geo.PrecisionCompare(edgeLength, bestEdgeLength, geo.PRECISION) < 1 {
				bestEdgeLength = edgeLength
				bestAttempt = *attempt
			}
		} else if !layoutgraph.IsCandidateRejection(err) {
			return 0, 0, 0, err
		}
		txn.Clear()
		txn.Rollback()
	}

	return bestEdgeLength, bestAttempt.X, bestAttempt.Y, nil
}

// Returns true if no obvious problems with the shift
func attemptShift(g *layoutgraph.Graph, aligningEdge *layoutgraph.Edge, nodes []*layoutgraph.Node, x, y float64) bool {
	for _, n := range nodes {
		n.Translate(x, y)
	}

	if !withinMaxSize(g) {
		return false
	}

	// Shifting is only good if it results in a direct line. It'd be undesirable if, after shifting, there are nodes in between obscuring the alignment
	// e.g. if there was a tiny box to the left of (b), moving (a) down would be not as good as moving (a) right, or (b) up
	// This includes any connected nodes
	// Note that this function already excludes ancestor containers by default
	if intersectsOtherNode(g, aligningEdge.From, aligningEdge.To) {
		return false
	}

	return true
}
