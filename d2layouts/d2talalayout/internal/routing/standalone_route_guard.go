package routing

import (
	"math"
	"sort"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func (guard *routeWorkGuard) reserveSort(length int) error {
	if length < 2 {
		return guard.step()
	}
	// Go's comparison sort is O(n log n). Reserve that worst-case work before
	// entering the non-interruptible standard-library sort, while retaining its
	// exact ordering behavior for equal values.
	for width := 1; width < length; {
		if err := guard.add(uint64(length)); err != nil {
			return err
		}
		if width > length/2 {
			break
		}
		width *= 2
	}
	return guard.check()
}

func portEdgesGuarded(edges layoutgraph.Edges, node *layoutgraph.Node, guard workBudget) (map[geo.Point][]*layoutgraph.Edge, error) {
	portEdges := make(map[geo.Point][]*layoutgraph.Edge)
	for _, edge := range edges {
		if err := guard.step(); err != nil {
			return nil, err
		}
		var port *geo.Point
		switch {
		case edge.From == node:
			port = edge.SourcePort()
		case edge.To == node:
			port = edge.TargetPort()
		default:
			continue
		}
		if port != nil {
			portEdges[*port] = append(portEdges[*port], edge)
		}
	}
	return portEdges, nil
}

func edgeHasDuplicateInGuarded(edge *layoutgraph.Edge, edges []*layoutgraph.Edge, guard workBudget) (bool, error) {
	for _, otherEdge := range edges {
		if err := guard.step(); err != nil {
			return false, err
		}
		if otherEdge.IsDuplicateOf(edge) {
			return true, nil
		}
	}
	return false, nil
}

func tryStraightEdgeFallbackGuarded(g *layoutgraph.Graph, edge *layoutgraph.Edge, guard *routeWorkGuard) error {
	if err := guard.step(); err != nil {
		return err
	}
	originalCost, err := estimateRouteCostGuarded(layoutgraph.Edges(g.Edges), edge, guard)
	if err != nil {
		return err
	}
	fromPort, toPort, lineCost, err := routeLineGuarded(g, edge, g.Edges, nil, guard)
	if err != nil {
		if layoutgraph.IsCandidateRejection(err) {
			// A straight replacement is optional. Preserve the current route
			// when no valid line exists.
			return nil
		}
		return err
	}

	if len(edge.Points) == 4 && geo.EuclideanDistance(
		edge.Points[1].X,
		edge.Points[1].Y,
		edge.Points[2].X,
		edge.Points[2].Y,
	) <= lowerJitterThreshold {
		lineCost /= nonOrthogonalFactor
	}
	if err := guard.step(); err != nil {
		return err
	}
	if lineCost < originalCost {
		edge.Points = []*geo.Point{fromPort, toPort}
	}
	return nil
}

func filterEdgeAncestorsGuarded(edge *layoutgraph.Edge, nodes layoutgraph.Nodes, guard workBudget) (layoutgraph.Nodes, error) {
	nonAncestors := make(layoutgraph.Nodes, 0, len(nodes))
	for _, node := range nodes {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if node != edge.From && node != edge.To && (edge.From.IsDescendantOf(node) || edge.To.IsDescendantOf(node)) {
			continue
		}
		nonAncestors = append(nonAncestors, node)
	}
	return nonAncestors, nil
}

func sourceAndTargetClusterNodesGuarded(g *layoutgraph.Graph, source, target *layoutgraph.Node, guard workBudget) (map[*layoutgraph.Node]bool, map[*layoutgraph.Node]bool, error) {
	sourceClusterNodes := make(map[*layoutgraph.Node]bool)
	targetClusterNodes := make(map[*layoutgraph.Node]bool)

	for _, cluster := range g.Clusters {
		if err := guard.step(); err != nil {
			return nil, nil, err
		}
		isSource := false
		isTarget := false
		for _, clusterNode := range cluster.Nodes {
			if err := guard.step(); err != nil {
				return nil, nil, err
			}
			if clusterNode == source {
				isSource = true
			}
			if clusterNode == target {
				isTarget = true
			}
		}
		if isSource {
			for _, clusterNode := range cluster.Nodes {
				if err := guard.step(); err != nil {
					return nil, nil, err
				}
				if clusterNode != source {
					sourceClusterNodes[clusterNode] = true
				}
			}
		}
		if isTarget {
			for _, clusterNode := range cluster.Nodes {
				if err := guard.step(); err != nil {
					return nil, nil, err
				}
				if clusterNode != target {
					targetClusterNodes[clusterNode] = true
				}
			}
		}
	}
	return sourceClusterNodes, targetClusterNodes, nil
}

func overlappingEdgesGuarded(start, end *geo.Point, otherEdges []*layoutgraph.Edge, guard workBudget) ([]*layoutgraph.Edge, error) {
	overlappingEdges := make([]*layoutgraph.Edge, 0)
	for _, edge := range otherEdges {
		if err := guard.step(); err != nil {
			return nil, err
		}
		for index := 0; index < len(edge.Points)-1; index++ {
			if err := guard.step(); err != nil {
				return nil, err
			}
			if isVerticalOrHorizontalOverlap(start, end, edge.Points[index], edge.Points[index+1]) {
				overlappingEdges = append(overlappingEdges, edge)
				break
			}
		}
	}
	return overlappingEdges, nil
}

func edgeCanOverlapEdgesGuarded(edge *layoutgraph.Edge, otherEdges []*layoutgraph.Edge, sourceClusterNodes, targetClusterNodes map[*layoutgraph.Node]bool, guard workBudget) (bool, error) {
	if len(otherEdges) == 0 {
		// add(0) polls the context, so the general path below performs eleven
		// identical polls before edgeCanOverlapEdges returns true. A standard
		// cancellable context exposes a Done signal, making one poll sufficient.
		// Retain the legacy sequence for synthetic contexts that expose
		// cancellation only through Err.
		if searchGuard, ok := guard.(*routeSearchWorkGuard); ok && searchGuard.done != nil {
			if err := searchGuard.check(); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	// edgeCanOverlapEdges makes a bounded number of full passes over otherEdges.
	// Reserve its worst-case comparisons before calling it so ordinary output and
	// unstable-order behavior remain byte-for-byte unchanged.
	for pass := 0; pass < 10; pass++ {
		if err := guard.add(uint64(len(otherEdges))); err != nil {
			return false, err
		}
	}
	if err := guard.check(); err != nil {
		return false, err
	}
	return edgeCanOverlapEdges(edge, otherEdges, sourceClusterNodes, targetClusterNodes), nil
}

func positionedArrowheadLabelCostGuarded(
	positioned labeling.PositionedArrowheadLabel,
	nodes []*layoutgraph.Node,
	labels []labeling.PositionedArrowheadLabel,
	routes []*Route,
	edges []*layoutgraph.Edge,
	guard workBudget,
) (float64, error) {
	for _, other := range labels {
		if err := guard.step(); err != nil {
			return 0, err
		}
		if positioned.Edge == other.Edge && positioned.IsTarget == other.IsTarget {
			continue
		}
		if positioned.Box.Overlaps(other.Box) {
			if positioned.Text == other.Text {
				continue
			}
			return math.Inf(1), nil
		}
	}

	graph := positioned.Edge.From.Graph
	fakeLabelNode := &layoutgraph.Node{Box: positioned.Box, Graph: graph}
	overlapCount := 0
	for _, node := range nodes {
		if err := guard.step(); err != nil {
			return 0, err
		}
		if fakeLabelNode.LabelBoxesOverlap(node, label.PADDING) {
			overlapCount++
		}
	}
	penalty := 4 * graph.TurnCost() * float64(overlapCount)

	overlappingEdgeCount := 0
	for _, route := range routes {
		if err := guard.step(); err != nil {
			return 0, err
		}
		if route.GEdge == positioned.Edge {
			continue
		}
		for index := 1; index < len(route.OVGNodes); index++ {
			if err := guard.step(); err != nil {
				return 0, err
			}
			if fakeLabelNode.OverlapsLine(route.OVGNodes[index-1].Point, route.OVGNodes[index].Point, 0) {
				overlappingEdgeCount++
				break
			}
		}
	}
	for _, edge := range edges {
		if err := guard.step(); err != nil {
			return 0, err
		}
		if edge == positioned.Edge {
			continue
		}
		for index := 1; index < len(edge.Points); index++ {
			if err := guard.step(); err != nil {
				return 0, err
			}
			if fakeLabelNode.OverlapsLine(edge.Points[index-1], edge.Points[index], 0) {
				overlappingEdgeCount++
				break
			}
		}
	}
	penalty += graph.TurnCost() * float64(overlappingEdgeCount)
	return penalty, nil
}

func edgeIsStraightGuarded(edge *layoutgraph.Edge, guard *routeWorkGuard) (bool, error) {
	if len(edge.Points) < 2 {
		return false, guard.step()
	}
	first := edge.Points[0]
	second := edge.Points[1]
	for index := 2; index < len(edge.Points); index++ {
		if err := guard.step(); err != nil {
			return false, err
		}
		if orientation(first, second, edge.Points[index]) != 0 {
			return false, nil
		}
		first = second
		second = edge.Points[index]
	}
	return true, nil
}

func edgeHasOverlappingEndGuarded(edge *layoutgraph.Edge, guard *routeWorkGuard) (bool, error) {
	start := edge.Points[0]
	end := edge.Points[len(edge.Points)-1]
	for _, fromEdge := range edge.From.Edges {
		if err := guard.step(); err != nil {
			return false, err
		}
		if fromEdge != edge && (nonNilEquals(fromEdge.Points[0], start) || nonNilEquals(fromEdge.Points[len(fromEdge.Points)-1], start)) {
			return true, nil
		}
	}
	for _, toEdge := range edge.To.Edges {
		if err := guard.step(); err != nil {
			return false, err
		}
		if toEdge != edge && (nonNilEquals(toEdge.Points[0], end) || nonNilEquals(toEdge.Points[len(toEdge.Points)-1], end)) {
			return true, nil
		}
	}
	return false, nil
}

func reorderDuplicatesInEdgesGuarded(edges []*layoutgraph.Edge, guard *routeWorkGuard) error {
	for _, edge := range edges {
		if err := guard.step(); err != nil {
			return err
		}
		if edge.HasTableColumn() || edge.Label == nil {
			continue
		}
		straight, err := edgeIsStraightGuarded(edge, guard)
		if err != nil {
			return err
		}
		if !straight {
			continue
		}
		hasOverlappingEnd, err := edgeHasOverlappingEndGuarded(edge, guard)
		if err != nil {
			return err
		}

		duplicates := make([]int, 0)
		for index, other := range edges {
			if err := guard.step(); err != nil {
				return err
			}
			if other == edge || !other.IsDuplicateOf(edge) {
				continue
			}
			straight, err := edgeIsStraightGuarded(other, guard)
			if err != nil {
				return err
			}
			if !straight {
				continue
			}
			otherHasOverlappingEnd := false
			if hasOverlappingEnd {
				otherHasOverlappingEnd = true
			} else {
				otherHasOverlappingEnd, err = edgeHasOverlappingEndGuarded(other, guard)
				if err != nil {
					return err
				}
			}
			if otherHasOverlappingEnd {
				sourceArrow := edge.SourceArrowhead
				targetArrow := edge.TargetArrowhead
				if other.From == edge.To {
					sourceArrow, targetArrow = targetArrow, sourceArrow
				}
				if other.TargetArrowhead == targetArrow && other.SourceArrowhead == sourceArrow {
					duplicates = append(duplicates, index)
				}
			} else {
				duplicates = append(duplicates, index)
			}
		}

		if len(duplicates) <= 1 {
			continue
		}
		start := edge.Points[0]
		end := edge.Points[len(edge.Points)-1]
		dx := end.X - start.X
		dy := end.Y - start.Y
		sourceOrder := make([]*layoutgraph.Edge, 0, len(duplicates)+1)
		targetOrder := make([]*layoutgraph.Edge, 0, len(duplicates)+1)
		sourceOrder = append(sourceOrder, edge)
		targetOrder = append(targetOrder, edge)
		for _, index := range duplicates {
			if err := guard.step(); err != nil {
				return err
			}
			sourceOrder = append(sourceOrder, edges[index])
			targetOrder = append(targetOrder, edges[index])
		}

		if err := guard.reserveSort(len(sourceOrder)); err != nil {
			return err
		}
		if err := guard.reserveSort(len(targetOrder)); err != nil {
			return err
		}
		if dy > dx {
			sort.Slice(sourceOrder, func(i, j int) bool { return sourceOrder[i].Points[0].X < sourceOrder[j].Points[0].X })
			sort.Slice(targetOrder, func(i, j int) bool {
				left, right := targetOrder[i], targetOrder[j]
				return left.Points[len(left.Points)-1].X < right.Points[len(right.Points)-1].X
			})
		} else {
			sort.Slice(sourceOrder, func(i, j int) bool { return sourceOrder[i].Points[0].Y < sourceOrder[j].Points[0].Y })
			sort.Slice(targetOrder, func(i, j int) bool {
				left, right := targetOrder[i], targetOrder[j]
				return left.Points[len(left.Points)-1].Y < right.Points[len(right.Points)-1].Y
			})
		}

		consistentOrder := true
		for index := range sourceOrder {
			if err := guard.step(); err != nil {
				return err
			}
			if sourceOrder[index] != targetOrder[index] {
				consistentOrder = false
				break
			}
		}
		if !consistentOrder {
			continue
		}
		currentIndex := -1
		for index := range sourceOrder {
			if err := guard.step(); err != nil {
				return err
			}
			if sourceOrder[index] == edge {
				currentIndex = index
				break
			}
		}
		if currentIndex == 0 || currentIndex == len(sourceOrder)-1 {
			continue
		}
		first := sourceOrder[0]
		last := sourceOrder[len(sourceOrder)-1]
		if first.Label != nil && last.Label != nil {
			continue
		}
		if err := guard.step(); err != nil {
			return err
		}

		if last.Label == nil {
			edge.Points, last.Points = last.Points, edge.Points
			if last.From == edge.To {
				if err := reverseEdgeRouteGuarded(edge, guard); err != nil {
					return err
				}
				if err := reverseEdgeRouteGuarded(last, guard); err != nil {
					return err
				}
			}
		} else if first.Label == nil {
			edge.Points, first.Points = first.Points, edge.Points
			if first.From == edge.To {
				if err := reverseEdgeRouteGuarded(edge, guard); err != nil {
					return err
				}
				if err := reverseEdgeRouteGuarded(first, guard); err != nil {
					return err
				}
			}
		}
	}
	return guard.finish()
}

func reverseEdgeRouteGuarded(edge *layoutgraph.Edge, guard *routeWorkGuard) error {
	for left, right := 0, len(edge.Points)-1; left < right; left, right = left+1, right-1 {
		if err := guard.step(); err != nil {
			return err
		}
		edge.Points[left], edge.Points[right] = edge.Points[right], edge.Points[left]
	}
	return nil
}

func traceToShapeBorderGuarded(edge *layoutgraph.Edge, guard *routeWorkGuard) (err error) {
	if err := guard.step(); err != nil {
		return err
	}
	if edge == nil || edge.From == nil || edge.To == nil || len(edge.Points) < 2 {
		return nil
	}
	fromPosition := snapshotPointer(edge.From.TopLeft)
	toPosition := snapshotPointer(edge.To.TopLeft)
	defer func() {
		if recovered := recover(); recovered != nil {
			edge.From.TopLeft = fromPosition.restore()
			edge.To.TopLeft = toPosition.restore()
			panic(recovered)
		}
	}()
	traceToShapeBorder(edge)
	return guard.step()
}
