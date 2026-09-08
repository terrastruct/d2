package placement

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/loops"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/proximity"
)

func Prescale(graph *layoutgraph.Graph) {
	for _, node := range graph.Nodes {
		if node.AspectRatio1() {
			size := math.Max(node.Width, node.Height)
			node.Width = size
			node.Height = size
		}
		scaleBasedOnEdges(node)
	}
}

func scaleBasedOnEdges(node *layoutgraph.Node) {
	if node.FixedTopLeft != nil || node.DesiredWidth != nil || node.DesiredHeight != nil || node.IsTable() || node.IsClass() || len(node.Edges) == 0 {
		return
	}
	edgeCounts := make(map[*layoutgraph.Node]int, len(node.Edges))
	for _, edge := range node.Edges {
		adjacent := node.Adjacent(edge)
		if adjacent != node {
			edgeCounts[adjacent]++
		}
	}
	totalEdges := 0
	maxEdgesToAdjacent := 0
	for _, count := range edgeCounts {
		totalEdges += count
		maxEdgesToAdjacent = max(maxEdgesToAdjacent, count)
	}
	sidesForEdges := 4.0
	if len(edgeCounts) < 4 {
		sidesForEdges = float64(len(edgeCounts))
	}
	edgesPerSide := max(maxEdgesToAdjacent, int(math.Ceil(float64(totalEdges)/sidesForEdges)))
	if edgesPerSide == 1 {
		return
	}
	minLength := float64(edgesPerSide+1) * placementcost.SideEdgeSpacing
	if minLength < math.Min(node.Width, node.Height) {
		return
	}
	xRatio, yRatio := 1.0, 1.0
	if node.AspectRatio1() {
		if node.Width < minLength {
			xRatio = minLength / node.Width
			yRatio = minLength / node.Height
			node.Width, node.Height = minLength, minLength
		}
	} else {
		if node.Width < minLength {
			xRatio = minLength / node.Width
			node.Width = minLength
		}
		if node.Height < minLength {
			yRatio = minLength / node.Height
			node.Height = minLength
		}
	}
	if node.FontSize == nil {
		return
	}
	minRatio := math.Min(xRatio, yRatio)
	bestRatio := 1.0
	fontSize := *node.FontSize
	closestDistance := math.Inf(1)
	for _, size := range talaFontSizes() {
		fontRatio := float64(size) / float64(*node.FontSize)
		distance := math.Abs(fontRatio - minRatio)
		if distance < closestDistance {
			fontSize = size
			closestDistance = distance
			bestRatio = fontRatio
		}
	}
	if bestRatio > minRatio {
		minLength = math.Ceil(minLength * bestRatio / minRatio)
		if node.Width < minLength {
			node.Width = minLength
		}
		if node.Height < minLength {
			node.Height = minLength
		}
	}
	*node.FontSize = fontSize
	if node.Label != nil {
		node.Label.Width = math.Ceil(node.Label.Width * bestRatio)
		node.Label.Height = math.Ceil(node.Label.Height * bestRatio)
	}
}

// talaFontSizes returns the adapter-supported layout font scale by value so
// callers cannot mutate shared engine state.
func talaFontSizes() [7]int {
	return [7]int{13, 14, 16, 20, 24, 28, 32}
}

func Prepare(graph *layoutgraph.Graph) {
	loops.ComputeOffsets(graph)
	labeling.Initialize(graph)
	graph.ComputeNodeSpacing()
}

type normalizeGapsRollback struct {
	graph        *layoutgraph.Graph
	txn          *layoutgraph.Transaction
	graphState   *layoutgraph.GraphState
	cellSize     float64
	routingCosts layoutgraph.RoutingCostState
}

func (rollback *normalizeGapsRollback) restore() {
	layoutgraph.RestoreGraphState(rollback.graph, rollback.graphState)
	rollback.graph.CellSize = rollback.cellSize
	if !rollback.txn.RestorePlacementCosts() {
		rollback.graph.RestoreRoutingCosts(rollback.routingCosts)
	}
}

func Place(ctx context.Context, graph *layoutgraph.Graph, seed int64) error {
	if graph == nil {
		return invariant.New("Place requires a graph")
	}
	priorCommonUncleSiblings := graph.CommonUncleSiblings
	var ownership layoutgraph.NodeGraphOwnershipSnapshot
	complete := false
	defer func() {
		if complete {
			// Successful placement does not retain transient proximity hints.
			graph.CommonUncleSiblings = nil
			return
		}
		ownership.Restore()
		graph.CommonUncleSiblings = priorCommonUncleSiblings
	}()

	graph.CommonUncleSiblings = proximity.CommonUncleSiblings(graph)
	ownership, err := layoutgraph.SnapshotNodeGraphOwnership(ctx, graph)
	if err != nil {
		return err
	}
	ctx, err = orientationContext(ctx, graph)
	if err != nil {
		return err
	}
	if err := placeNodes(ctx, graph, nil, seed, nil, nil); err != nil {
		return err
	}
	nodes := make([]*layoutgraph.Node, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodes = append(nodes, node)
		nodes = append(nodes, graph.AllDescendantNodes(node, true)...)
	}
	for _, node := range nodes {
		node.Graph = graph
	}
	graph.ComputeCellSize()
	graph.ResetTurnCost()
	complete = true
	return nil
}

func NormalizeGaps(ctx context.Context, graph *layoutgraph.Graph) (bool, error) {
	ctx, guard, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "GapNormalizationTransactions")
	if err != nil {
		return false, err
	}
	txn, err := graph.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
	if err != nil {
		return false, err
	}
	rollback := normalizeGapsRollback{
		graph:        graph,
		txn:          txn,
		graphState:   txn.PreservePriorGraphState(),
		cellSize:     graph.CellSize,
		routingCosts: graph.RoutingCosts(),
	}
	complete := false
	defer func() {
		if !complete {
			rollback.restore()
		}
	}()
	bidirectional := func(nodes layoutgraph.Nodes, axis layoutAxis) (bool, error) {
		forward, err := gapNormalization(ctx, nodes, txn, graph, gapNormalizationOptions{
			axis:      axis,
			direction: forwardDirection,
			costTxn:   txn,
		})
		if err != nil {
			return false, err
		}
		backward, err := gapNormalization(ctx, nodes, txn, graph, gapNormalizationOptions{
			axis:      axis,
			direction: backwardDirection,
			costTxn:   txn,
		})
		return forward || backward, err
	}
	changed := false
	containers, err := graph.ContainerRDFSOrder(nil, guard)
	if err != nil {
		return false, err
	}
	for _, container := range containers {
		nodes := layoutgraph.Nodes(graph.AllDescendantNodes(container, false))
		for _, axis := range []layoutAxis{horizontalAxis, verticalAxis} {
			current, err := bidirectional(nodes, axis)
			if err != nil {
				return false, err
			}
			changed = changed || current
		}
	}
	for _, axis := range []layoutAxis{horizontalAxis, verticalAxis} {
		current, err := bidirectional(layoutgraph.Nodes(graph.Nodes), axis)
		if err != nil {
			return false, err
		}
		changed = changed || current
	}
	graph.ResetTurnCost()
	graph.SyncSequences()
	graph.SyncClusters()
	if err := guard.Finish(); err != nil {
		return false, err
	}
	complete = true
	return changed, nil
}

func Align(ctx context.Context, graph *layoutgraph.Graph) error {
	ctx, _, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "AlignAxesTransactions")
	if err != nil {
		return err
	}
	if graph.CellSize == 0 {
		graph.ComputeCellSize()
	}
	txn, err := graph.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
	if err != nil {
		return err
	}
	for range 100 {
		changed, err := alignAxes(ctx, graph, txn)
		if err != nil {
			return err
		}
		if !changed {
			break
		}
	}
	return nil
}

func Swap(ctx context.Context, graph *layoutgraph.Graph) error {
	ctx, _, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "SwapStuffTransactions")
	if err != nil {
		return err
	}
	graph.ComputeCellSize()
	for range 4 {
		changed, err := swapOptimize(ctx, layoutgraph.Nodes(graph.Nodes), graph)
		if err != nil {
			return err
		}
		if !changed {
			break
		}
	}
	return direct(ctx, graph, graph.Nodes, nil, directOptions{checkEdgeLength: true})
}

func TransposeAll(ctx context.Context, graph *layoutgraph.Graph) error {
	ctx, _, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "TransposeStageTransactions")
	if err != nil {
		return err
	}
	graph.ComputeCellSize()
	for _, node := range graph.Nodes {
		if _, err := transpose(ctx, graph, node, nil); err != nil {
			return err
		}
	}
	return nil
}

func Normalize(graph *layoutgraph.Graph) {
	minX, minY := math.Inf(1), math.Inf(1)
	if graph.HasFixedNode() {
		minX, minY = placementPadding, placementPadding
	} else {
		for _, node := range graph.Nodes {
			minX = math.Min(minX, node.TopLeft.X)
			minY = math.Min(minY, node.TopLeft.Y)
		}
		for _, edge := range graph.Edges {
			for _, point := range edge.Points {
				minX = math.Min(minX, math.Floor(point.X))
				minY = math.Min(minY, math.Floor(point.Y))
			}
			if edge.Label != nil {
				topLeft := edge.LabelTopLeft(edge.Label.Position, edge.Label.Width, edge.Label.Height)
				minX = math.Min(minX, math.Floor(topLeft.X))
				minY = math.Min(minY, math.Floor(topLeft.Y))
			}
		}
	}
	for _, node := range graph.Nodes {
		node.TopLeft.X -= minX
		node.TopLeft.Y -= minY
	}
	for _, edge := range graph.Edges {
		for _, point := range edge.Points {
			point.X -= minX
			point.Y -= minY
		}
	}
}

func Pad(graph *layoutgraph.Graph) {
	for _, node := range graph.Nodes {
		node.TopLeft.X += placementPadding
		node.TopLeft.Y += placementPadding
	}
}
