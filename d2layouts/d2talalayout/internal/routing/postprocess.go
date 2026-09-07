package routing

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func BalanceEdgeSegments(ctx context.Context, g *layoutgraph.Graph) error {
	return balanceEdgeSegmentsWithLimit(ctx, g, maxRouteStageWorkUnits)
}

func balanceEdgeSegmentsWithLimit(ctx context.Context, g *layoutgraph.Graph, workLimit uint64) error {
	return runAtomicRouteStage(ctx, "BalanceEdgeSegments", g, nil, workLimit, func(guard *routeWorkGuard) error {
		return balanceEdgeSegmentsGuarded(g, guard)
	})
}

func balanceEdgeSegmentsGuarded(g *layoutgraph.Graph, guard *routeWorkGuard) error {
	// Special nodes get special routing, don't mess with them
	specialEdges := make([]*layoutgraph.Edge, 0)
	regularEdges := make([]*layoutgraph.Edge, 0)
	for _, e := range g.Edges {
		if err := guard.step(); err != nil {
			return err
		}
		if len(e.Points) < 2 {
			return fmt.Errorf("TALA BalanceEdgeSegments edge %d has an incomplete route", e.IDValue())
		}
		if isSpecialEdgeForBalancing(g, e) {
			specialEdges = append(specialEdges, e)
			continue
		}
		regularEdges = append(regularEdges, e)
	}

	if err := balanceRegularEdgesGuarded(g, specialEdges, regularEdges, guard); err != nil {
		return err
	}
	return balancePortInteriorsGuarded(g, guard)
}

// isSpecialEdgeForBalancing identifies routes whose ports or rule-generated
// geometry balancing must leave unchanged.
func isSpecialEdgeForBalancing(g *layoutgraph.Graph, e *layoutgraph.Edge) bool {
	if _, in := g.NodeToTree[e.From]; in {
		return true
	}
	if _, in := g.NodeToTree[e.To]; in {
		return true
	}
	if e.HasTableColumn() || e.IsLoop() {
		return true
	}
	// We want edges locked to diamond ports strictly.
	if e.From.Shape.GetType() == shape.DIAMOND_TYPE || e.To.Shape.GetType() == shape.DIAMOND_TYPE {
		return true
	}
	// When node A (width even) aligns with node B (width odd), the centers can
	// end up misaligned by 1.
	first := e.Points[0]
	last := e.Points[len(e.Points)-1]
	return math.Abs(first.X-last.X) == 1 || math.Abs(first.Y-last.Y) == 1
}

func balanceRegularEdgesGuarded(g *layoutgraph.Graph, specialEdges, regularEdges []*layoutgraph.Edge, guard *routeWorkGuard) error {
	// Row and diamond ports fix the endpoint and its short approach segment,
	// while longer interior corridors can still be balanced.
	fixedPortPoints := make(map[*geo.Point]bool)
	for _, edge := range regularEdges {
		if err := guard.step(); err != nil {
			return err
		}
		if !hasFixedBalancingPorts(edge) {
			continue
		}
		for _, point := range edge.Points[:min(2, len(edge.Points))] {
			fixedPortPoints[point] = true
		}
		for _, point := range edge.Points[max(0, len(edge.Points)-2):] {
			fixedPortPoints[point] = true
		}
	}
	isConnectedToUnitNode := func(point *geo.Point) (bool, error) {
		for _, node := range g.Nodes {
			if err := guard.step(); err != nil {
				return false, err
			}
			if node.Width == 1 && node.Height == 1 {
				// Check if point is on the node's border
				if (point.X == node.TopLeft.X || point.X == node.TopLeft.X+1) &&
					(point.Y == node.TopLeft.Y || point.Y == node.TopLeft.Y+1) {
					return true, nil
				}
			}
		}
		return false, nil
	}

	getSharedCluster := func(edgeA, edgeB *layoutgraph.Edge) *layoutgraph.Cluster {
		fromA := edgeA.From.Cluster
		toA := edgeA.To.Cluster

		fromB := edgeB.From.Cluster
		toB := edgeB.To.Cluster
		if fromA != nil && (fromA == fromB || fromA == toB) {
			return fromA
		}
		if toA != nil && (toA == fromB || toA == toB) {
			return toA
		}
		return nil
	}

	eq := func(a, b float64) bool {
		return math.Abs(a-b) <= 1
	}

	for _, isHorizontal := range []bool{true, false} {
		lockedSegments, err := nodeSegmentsGuarded(layoutgraph.Nodes(g.Nodes), !isHorizontal, guard)
		if err != nil {
			return err
		}
		lockedEdgeSegments := make(map[layoutgraph.EdgeSegment]bool)
		// When we balance vertical segments horizontally, we get the vertical segments
		allEdgeSegments, err := edgeSegmentsGuarded(layoutgraph.Edges(regularEdges), !isHorizontal, guard)
		if err != nil {
			return err
		}

		edgeSegments := make([]*layoutgraph.EdgeSegment, 0, len(allEdgeSegments))
		var fixedSegments []*geo.Segment
		for _, segment := range allEdgeSegments {
			if fixedPortPoints[segment.Start] || fixedPortPoints[segment.End] {
				fixedSegments = append(fixedSegments, &segment.Segment)
				continue
			}
			startConnected, err := isConnectedToUnitNode(segment.Start)
			if err != nil {
				return err
			}
			endConnected, err := isConnectedToUnitNode(segment.End)
			if err != nil {
				return err
			}
			if !startConnected && !endConnected {
				edgeSegments = append(edgeSegments, segment)
			}
		}
		// Special edges shouldn't be moved, but they're still considered for bounds checking
		specialEdgeSegments := fixedSegments
		guardedSpecialEdgeSegments, err := edgeSegmentsGuarded(layoutgraph.Edges(specialEdges), !isHorizontal, guard)
		if err != nil {
			return err
		}
		for _, specialEdgeSegment := range guardedSpecialEdgeSegments {
			if err := guard.step(); err != nil {
				return err
			}
			specialEdgeSegments = append(specialEdgeSegments, &specialEdgeSegment.Segment)
		}
		// Regular edges shared with special edges should also be locked
		for _, s := range edgeSegments {
			for _, ses := range specialEdgeSegments {
				if err := guard.step(); err != nil {
					return err
				}
				if !s.Segment.Overlaps(*ses, isHorizontal, 1) {
					continue
				}
				if isHorizontal {
					if eq(s.Start.X, ses.Start.X) {
						specialEdgeSegments = append(specialEdgeSegments, &s.Segment)
						lockedEdgeSegments[*s] = true
						break
					}
				} else {
					if eq(s.Start.Y, ses.Start.Y) {
						specialEdgeSegments = append(specialEdgeSegments, &s.Segment)
						lockedEdgeSegments[*s] = true
						break
					}
				}
			}
		}
		lockedSegments = append(lockedSegments, specialEdgeSegments...)
		for len(lockedEdgeSegments) != len(edgeSegments) {
			if err := guard.step(); err != nil {
				return err
			}
			minRange := Range{start: math.Inf(-1), end: math.Inf(1)}
			segmentRanges := map[Range][]*layoutgraph.EdgeSegment{}

			for _, es := range edgeSegments {
				if err := guard.step(); err != nil {
					return err
				}
				if _, ok := lockedEdgeSegments[*es]; ok {
					continue
				}
				floor, ceil, err := routeSegmentBounds(es.Segment, lockedSegments, segmentSpacingBuffer, guard)
				if err != nil {
					return err
				}
				if isHorizontal {
					if math.IsInf(floor, -1) {
						floor = es.Start.X - unboundedSegmentMove
					}
					if math.IsInf(ceil, 1) {
						ceil = es.Start.X + unboundedSegmentMove
					}
				} else {
					if math.IsInf(floor, -1) {
						floor = es.Start.Y - unboundedSegmentMove
					}
					if math.IsInf(ceil, 1) {
						ceil = es.Start.Y + unboundedSegmentMove
					}
				}

				movementRange := Range{start: floor, end: ceil}

				segmentRanges[movementRange] = append(segmentRanges[movementRange], es)
				if movementRange.length() < minRange.length() {
					minRange = movementRange
				}
			}

			// For all the segments which share the same min range, we want to further split them up by overlap
			// E.g. if two segments don't overlap, they can both take the 50% point between its range
			// But if they overlap, they share the same space, so have to evenly distribute at 33% and 66% points
			for _, s := range segmentRanges[minRange] {
				if err := guard.step(); err != nil {
					return err
				}
				if _, ok := lockedEdgeSegments[*s]; ok {
					continue
				}
				balanceBatch := []*layoutgraph.EdgeSegment{s}
				balanceBatchSet := map[*layoutgraph.EdgeSegment]bool{s: true}
				for _, otherS := range segmentRanges[minRange] {
					if err := guard.step(); err != nil {
						return err
					}
					if s == otherS {
						continue
					}
					if _, ok := lockedEdgeSegments[*otherS]; ok {
						continue
					}
					var buffer float64 = segmentSpacingBuffer
					// Segments of the same edge don't need to maintain a buffer from each other
					if s.Owner() == otherS.Owner() {
						buffer = 0
					}
					if s.Segment.Overlaps(otherS.Segment, isHorizontal, buffer) {
						balanceBatch = append(balanceBatch, otherS)
						balanceBatchSet[otherS] = true
					}
				}
				// Even if the same range is not shared, if a segment shares a route with another segment (even partially), it should be balanced together to maintain shared route
				for _, otherS := range edgeSegments {
					if err := guard.step(); err != nil {
						return err
					}
					if _, ok := balanceBatchSet[otherS]; ok {
						continue
					}
					if _, ok := lockedEdgeSegments[*otherS]; ok {
						continue
					}
					if cluster := getSharedCluster(s.Owner(), otherS.Owner()); cluster != nil && cluster.Arrangement == cluster.DesiredArrangement {
						// we usually want to balance cluster edge segments normally (Example with Arrangement=Row && IsHorizontal):
						//        before       |        after
						//       ┌─────┐       |       ┌─────┐
						//       │     │       |       │     │
						//       └┬────┘       |       └──┬──┘
						//        │            |          │
						//      ┌─┴─────┐      |    ┌─────┴─────┐
						//      │       │      |    │           │
						// ┌────▼┐     ┌▼────┐ | ┌──▼──┐     ┌──▼──┐
						// │     │     │     │ | │     │     │     │
						// └─────┘     └─────┘ | └─────┘     └─────┘
						//
						// but we want to balance cluster mid-segments in a combined range
						//             before                      |              separate ranges            |             combined range
						// ┌──────┐                      ┌───────┐ | ┌──────┐                      ┌───────┐ | ┌──────┐                      ┌───────┐
						// │      │          ┌───────────►       │ | │      │          ┌───────────►       │ | │      │             ┌────────►       │
						// └──────┘          │           └───────┘ | └──────┘          │           └───────┘ | └──────┘             │        └───────┘
						// ┌──────┐          │           ┌───────┐ | ┌──────┐          │           ┌───────┐ | ┌──────┐             │        ┌───────┐
						// │      ├──────────┼───────────►       │ | │      ├──────────┴──┬────────►       │ | │      ├─────────────┼────────►       │
						// └──────┘          │           └───────┘ | └──────┘             │        └───────┘ | └──────┘             │        └───────┘
						// ┌───────────┐     │           ┌───────┐ | ┌───────────┐        │        ┌───────┐ | ┌───────────┐        │        ┌───────┐
						// │           │     └───────────►       │ | │           │        └────────►       │ | │           │        └────────►       │
						// └───────────┘                 └───────┘ | └───────────┘                 └───────┘ | └───────────┘                 └───────┘
						var shouldBalanceTogether bool
						if isHorizontal {
							shouldBalanceTogether = cluster.Arrangement == layoutgraph.Column && eq(s.Start.X, otherS.Start.X)
						} else {
							shouldBalanceTogether = cluster.Arrangement == layoutgraph.Row && eq(s.Start.Y, otherS.Start.Y)
						}
						if shouldBalanceTogether {
							balanceBatch = append(balanceBatch, otherS)
							balanceBatchSet[otherS] = true
							continue
						}
					}
					if !s.Segment.Overlaps(otherS.Segment, isHorizontal, 1) {
						continue
					}
					var shared bool
					if isHorizontal {
						shared = eq(s.Start.X, otherS.Start.X)
					} else {
						shared = eq(s.Start.Y, otherS.Start.Y)
					}
					if shared {
						balanceBatch = append(balanceBatch, otherS)
						balanceBatchSet[otherS] = true
					}
				}
				if err := stableSortRouteValues(balanceBatch, func(a, b *layoutgraph.EdgeSegment) bool {
					var primaryDimensionI, primaryDimensionJ float64
					var secondaryDimensionI, secondaryDimensionJ float64
					if isHorizontal {
						primaryDimensionI = a.Start.X
						primaryDimensionJ = b.Start.X
						secondaryDimensionI = a.Start.Y
						secondaryDimensionJ = b.Start.Y
					} else {
						primaryDimensionI = a.Start.Y
						primaryDimensionJ = b.Start.Y
						secondaryDimensionI = a.Start.X
						secondaryDimensionJ = b.Start.X
					}
					if primaryDimensionI == primaryDimensionJ {
						return secondaryDimensionI < secondaryDimensionJ
					}
					return primaryDimensionI < primaryDimensionJ
				}, guard); err != nil {
					return err
				}
				// Among the overlapping segments, some may be on shared routes. They're shared intentionally, so we want to keep them shared.
				distincts := map[float64]struct{}{}
				for _, s := range balanceBatch {
					if err := guard.step(); err != nil {
						return err
					}
					if isHorizontal {
						distincts[math.Round(s.Start.X)] = struct{}{}
					} else {
						distincts[math.Round(s.Start.Y)] = struct{}{}
					}
				}

				// Now we take the overlapping segments with the same range of movement, and rebalance them
				distributionVals, err := evenlyDistributeGuarded(minRange.start, minRange.end, len(distincts), guard)
				if err != nil {
					return err
				}
				proposed := make([]float64, len(balanceBatch))
				for i, segment := range balanceBatch {
					if err := guard.step(); err != nil {
						return err
					}
					proposed[i] = segment.Start.Y
					if isHorizontal {
						proposed[i] = segment.Start.X
					}
				}

				for valIndex, segmentIndex := 0, 0; valIndex < len(distributionVals); valIndex++ {
					if err := guard.step(); err != nil {
						return err
					}
					sharedCount := 0
					for segmentIndex+sharedCount < len(balanceBatch) {
						if err := guard.step(); err != nil {
							return err
						}
						if isHorizontal {
							if eq(balanceBatch[segmentIndex].Start.X, balanceBatch[segmentIndex+sharedCount].Start.X) {
								sharedCount++
							} else {
								break
							}
						} else {
							if eq(balanceBatch[segmentIndex].Start.Y, balanceBatch[segmentIndex+sharedCount].Start.Y) {
								sharedCount++
							} else {
								break
							}
						}
					}

					val := distributionVals[valIndex]
					for i := segmentIndex; i < segmentIndex+sharedCount; i++ {
						if err := guard.step(); err != nil {
							return err
						}
						proposed[i] = val
					}
					segmentIndex += sharedCount
				}
				order, err := checkBalanceOrder(balanceBatch, balanceBatchSet, allEdgeSegments, proposed, isHorizontal, guard)
				if err != nil {
					return err
				}
				if order != balanceOrderContactChanged {
					specialOrder, err := checkBalanceOrder(balanceBatch, balanceBatchSet, guardedSpecialEdgeSegments, proposed, isHorizontal, guard)
					if err != nil {
						return err
					}
					order = max(order, specialOrder)
				}
				accept := order == balanceOrderPreserved
				if order == balanceOrderReversed {
					accept, err = balanceReversalRemovesCrossings(g, balanceBatch, proposed, isHorizontal, guard)
					if err != nil {
						return err
					}
				}
				if accept {
					for i, segment := range balanceBatch {
						if err := guard.step(); err != nil {
							return err
						}
						if isHorizontal {
							segment.Start.X = proposed[i]
							segment.End.X = proposed[i]
						} else {
							segment.Start.Y = proposed[i]
							segment.End.Y = proposed[i]
						}
					}
				}
				for _, s := range balanceBatch {
					if err := guard.step(); err != nil {
						return err
					}
					lockedEdgeSegments[*s] = true
					lockedSegments = append(lockedSegments, &s.Segment)
				}
			}
		}
	}
	for _, e := range regularEdges {
		points, err := removeDuplicatePointsGuarded(e.Points, guard)
		if err != nil {
			return err
		}
		e.Points = points
	}
	return nil
}

// FixClusterEdgeBranching resolves route branching at cluster boundaries.
func FixClusterEdgeBranching(ctx context.Context, g *layoutgraph.Graph) error {
	return fixClusterEdgeBranchingWithLimit(ctx, g, maxRouteStageWorkUnits)
}

func fixClusterEdgeBranchingWithLimit(ctx context.Context, g *layoutgraph.Graph, workLimit uint64) error {
	return runAtomicRouteStage(ctx, "FixClusterEdgeBranching", g, nil, workLimit, func(guard *routeWorkGuard) error {
		return fixClusterEdgeBranchingGuarded(g, guard)
	})
}

func fixClusterEdgeBranchingGuarded(g *layoutgraph.Graph, guard *routeWorkGuard) error {
	for _, n := range g.Nodes {
		if err := guard.step(); err != nil {
			return err
		}
		if len(n.Edges) <= 1 {
			continue
		}
		if n.Cluster != nil {
			continue
		}
		isFrom := make(map[*layoutgraph.Cluster]bool)
		clusterEdges := make(map[*layoutgraph.Cluster][]*layoutgraph.Edge)
		for _, e := range n.Edges {
			if err := guard.step(); err != nil {
				return err
			}
			if len(e.Points) != 4 {
				continue
			}
			adj := n.Adjacent(e)
			if adj.Cluster != nil {
				if adj.Cluster.DesiredArrangement != adj.Cluster.Arrangement {
					continue
				}
				if e.From == n {
					isFrom[adj.Cluster] = true
				} else {
					isFrom[adj.Cluster] = false
				}
				if _, ok := clusterEdges[adj.Cluster]; !ok {
					clusterEdges[adj.Cluster] = []*layoutgraph.Edge{e}
				} else {
					clusterEdges[adj.Cluster] = append(clusterEdges[adj.Cluster], e)
				}
			}
		}

		for cluster, edges := range clusterEdges {
			if err := guard.step(); err != nil {
				return err
			}
			backOfCenterFound := false
			frontOfCenterFound := false
			for _, e := range edges {
				if err := guard.step(); err != nil {
					return err
				}
				if cluster.Arrangement == layoutgraph.Row {
					if isFrom[cluster] {
						if e.Points[3].X < e.Points[0].X {
							backOfCenterFound = true
						}
						if e.Points[3].X > e.Points[0].X {
							frontOfCenterFound = true
						}
					} else {
						if e.Points[0].X < e.Points[3].X {
							backOfCenterFound = true
						}
						if e.Points[0].X > e.Points[3].X {
							frontOfCenterFound = true
						}
					}
				} else {
					if isFrom[cluster] {
						if e.Points[3].Y < e.Points[0].Y {
							backOfCenterFound = true
						}
						if e.Points[3].Y > e.Points[0].Y {
							frontOfCenterFound = true
						}
					} else {
						if e.Points[0].Y < e.Points[3].Y {
							backOfCenterFound = true
						}
						if e.Points[0].Y > e.Points[3].Y {
							frontOfCenterFound = true
						}
					}
				}
			}
			if !backOfCenterFound || !frontOfCenterFound {
				continue
			}

			originalP1 := make(map[*layoutgraph.Edge]*geo.Point)
			originalP2 := make(map[*layoutgraph.Edge]*geo.Point)
			var potentialMovePoints []*geo.Point
			for _, e := range edges {
				if err := guard.step(); err != nil {
					return err
				}
				if isFrom[cluster] {
					potentialMovePoints = append(potentialMovePoints, e.Points[1])
				} else {
					potentialMovePoints = append(potentialMovePoints, e.Points[2])
				}
				originalP1[e] = e.Points[1].Copy()
				originalP2[e] = e.Points[2].Copy()
			}

			var bestPoint *geo.Point
			var bestPointCost float64

		OUTER:
			for _, p := range potentialMovePoints {
				if err := guard.step(); err != nil {
					return err
				}
				for _, e := range edges {
					if err := guard.step(); err != nil {
						return err
					}
					e.Points[1] = originalP1[e].Copy()
					e.Points[2] = originalP2[e].Copy()
				}
				cost := 0.
				for _, e := range edges {
					if err := guard.step(); err != nil {
						return err
					}
					originalCost, err := estimateRouteCostGuarded(layoutgraph.Edges(g.Edges), e, guard)
					if err != nil {
						return err
					}
					if isFrom[cluster] {
						if e.Points[1] == p {
							cost += originalCost
							continue
						}
					} else {
						if e.Points[2] == p {
							cost += originalCost
							continue
						}
					}
					if isFrom[cluster] {
						e.Points[1] = p.Copy()
						if cluster.Arrangement == layoutgraph.Row {
							e.Points[2].Y = p.Y
						} else {
							e.Points[2].X = p.X
						}
					} else {
						e.Points[2] = p.Copy()
						if cluster.Arrangement == layoutgraph.Row {
							e.Points[1].Y = p.Y
						} else {
							e.Points[1].X = p.X
						}
					}

					revert := false
					intersectsNode, err := routeIntersectsNodeGuarded(layoutgraph.Nodes(g.Nodes), e, guard)
					if err != nil {
						return err
					}
					if intersectsNode {
						revert = true
					}
					if !revert {
						newCost, err := estimateRouteCostGuarded(layoutgraph.Edges(g.Edges), e, guard)
						if err != nil {
							return err
						}
						cost += newCost
						if newCost > originalCost {
							revert = true
						}
					}
					if revert {
						continue OUTER
					}
				}
				if bestPoint == nil || cost < bestPointCost {
					bestPoint = p
					bestPointCost = cost
				}
			}

			for _, e := range edges {
				if err := guard.step(); err != nil {
					return err
				}
				e.Points[1] = originalP1[e].Copy()
				e.Points[2] = originalP2[e].Copy()
			}

			if bestPoint != nil {
				for _, e := range edges {
					if err := guard.step(); err != nil {
						return err
					}
					if isFrom[cluster] {
						e.Points[1] = bestPoint.Copy()
						if cluster.Arrangement == layoutgraph.Row {
							e.Points[2].Y = bestPoint.Y
						} else {
							e.Points[2].X = bestPoint.X
						}
					} else {
						e.Points[2] = bestPoint.Copy()
						if cluster.Arrangement == layoutgraph.Row {
							e.Points[1].Y = bestPoint.Y
						} else {
							e.Points[1].X = bestPoint.X
						}
					}
				}
			}
		}
	}

	return guard.finish()
}
