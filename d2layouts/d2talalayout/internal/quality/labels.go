package quality

import (
	"fmt"
	"math"
	"slices"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func scoreExistingLabelPlacements(g *layoutgraph.Graph, guard *limits.WorkGuard) (float64, error) {
	if err := guard.Step(); err != nil {
		return 0, err
	}

	var placedBoxes []geo.Box
	totalScore := 0.0

	// Arrowhead labels and loop labels reserve space before ordinary node and
	// edge labels are scored.
	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if edge.SourceArrowheadLabel != nil {
			if err := chargeEvaluationWork(guard, len(edge.Points)); err != nil {
				return 0, err
			}
			placedBoxes = append(placedBoxes, labeling.PositionArrowheadLabel(edge, false, edge.Points).Box)
		}
		if edge.TargetArrowheadLabel != nil {
			if err := chargeEvaluationWork(guard, len(edge.Points)); err != nil {
				return 0, err
			}
			placedBoxes = append(placedBoxes, labeling.PositionArrowheadLabel(edge, true, edge.Points).Box)
		}
	}

	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if edge.IsLoop() && edge.Label != nil {
			if err := chargeEvaluationWork(guard, len(edge.Points)); err != nil {
				return 0, err
			}
			placedBoxes = append(placedBoxes, labelBox(edge))
		}
	}

	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return 0, err
		}

		if node.Icon != nil && !node.IsImage() {
			iconSize := node.IconSize(node.Icon.Position)
			iconBox := geo.Box{
				TopLeft: node.LabelTopLeft(node.Icon.Position, iconSize, iconSize),
				Width:   iconSize,
				Height:  iconSize,
			}
			placedBoxes = append(placedBoxes, iconBox)

			nodeOverlaps, err := nodeOverlapCount(iconBox, g.Nodes, 0, guard)
			if err != nil {
				return 0, err
			}
			edgeOverlaps, err := edgeOverlapCount(iconBox, g.Edges, 0, guard)
			if err != nil {
				return 0, err
			}
			labelOverlaps, err := boxOverlapCount(iconBox, placedBoxes, 0, guard)
			if err != nil {
				return 0, err
			}
			totalScore += scoreNodeLabelOverlaps(nodeOverlaps, edgeOverlaps, labelOverlaps-1)
		}

		if node.Label == nil {
			continue
		}
		nodeLabelBox := geo.Box{
			TopLeft: node.LabelTopLeft(node.Label.Position, node.Label.Width, node.Label.Height),
			Width:   node.Label.Width,
			Height:  node.Label.Height,
		}

		var siblingsAndChildren []*layoutgraph.Node
		for _, sibling := range g.Containers[node.Container] {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if sibling != node {
				siblingsAndChildren = append(siblingsAndChildren, sibling)
			}
		}
		if node.IsContainer() {
			for _, child := range g.Containers[node] {
				if err := guard.Step(); err != nil {
					return 0, err
				}
				siblingsAndChildren = append(siblingsAndChildren, child)
			}
		}

		var ancestors []*layoutgraph.Node
		for current, depth := node, 0; current != nil; current, depth = current.EffectiveContainer(), depth+1 {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if depth >= layoutgraph.MaxTopologyDepth {
				return 0, fmt.Errorf("TALA Evaluate container depth exceeds limit %d", layoutgraph.MaxTopologyDepth)
			}
			ancestors = append(ancestors, current)
		}

		siblingOverlaps, err := nodeOverlapCount(nodeLabelBox, siblingsAndChildren, label.PADDING, guard)
		if err != nil {
			return 0, err
		}
		ancestorOverlaps, err := partialNodeOverlapCount(nodeLabelBox, ancestors, label.PADDING, guard)
		if err != nil {
			return 0, err
		}
		edgeOverlaps, err := edgeOverlapCount(nodeLabelBox, g.Edges, label.PADDING-1, guard)
		if err != nil {
			return 0, err
		}
		labelOverlaps, err := boxOverlapCount(nodeLabelBox, placedBoxes, label.PADDING, guard)
		if err != nil {
			return 0, err
		}
		totalScore += scoreNodeLabelOverlaps(siblingOverlaps+ancestorOverlaps, edgeOverlaps, labelOverlaps)
		placedBoxes = append(placedBoxes, nodeLabelBox)
	}

	sharedSegments, err := findSharedSegments(g.Edges, guard)
	if err != nil {
		return 0, err
	}
	sharedSegmentBoxes := make([]geo.Box, 0, len(sharedSegments))
	for _, segment := range sharedSegments {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		topLeft := segment.Start.Copy()
		width := segment.End.X - segment.Start.X
		height := segment.End.Y - segment.Start.Y
		if segment.End.X == segment.Start.X {
			width = 2 * labeling.SharedSegmentClearance
			topLeft.X -= labeling.SharedSegmentClearance
		} else {
			height = 2 * labeling.SharedSegmentClearance
			topLeft.Y -= labeling.SharedSegmentClearance
		}
		sharedSegmentBoxes = append(sharedSegmentBoxes, geo.Box{TopLeft: topLeft, Width: width, Height: height})
	}

	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if edge.Label == nil || edge.IsLoop() {
			continue
		}
		if err := chargeEvaluationWork(guard, len(edge.Points)); err != nil {
			return 0, err
		}
		edgeLabelBox := labelBox(edge)

		otherEdges := make([]*layoutgraph.Edge, 0, len(g.Edges)-1)
		for _, other := range g.Edges {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if other != edge {
				otherEdges = append(otherEdges, other)
			}
		}

		var ancestors, nonAncestors []*layoutgraph.Node
		for _, node := range g.Nodes {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			fromDescendant, err := evaluationIsDescendantOf(edge.From, node, guard)
			if err != nil {
				return 0, err
			}
			toDescendant, err := evaluationIsDescendantOf(edge.To, node, guard)
			if err != nil {
				return 0, err
			}
			if node != edge.From && node != edge.To && (fromDescendant || toDescendant) {
				ancestors = append(ancestors, node)
			} else {
				nonAncestors = append(nonAncestors, node)
			}
		}

		ancestorOverlapArea, err := nodeOverlapArea(edgeLabelBox, ancestors, label.PADDING, true, guard)
		if err != nil {
			return 0, err
		}
		nonAncestorOverlapArea, err := nodeOverlapArea(edgeLabelBox, nonAncestors, label.PADDING, false, guard)
		if err != nil {
			return 0, err
		}
		edgeOverlaps, err := edgeOverlapCount(edgeLabelBox, otherEdges, label.PADDING, guard)
		if err != nil {
			return 0, err
		}
		labelOverlaps, err := boxOverlapCount(edgeLabelBox, placedBoxes, 0, guard)
		if err != nil {
			return 0, err
		}
		almostLabelOverlaps, err := boxOverlapCount(edgeLabelBox, placedBoxes, label.PADDING, guard)
		if err != nil {
			return 0, err
		}
		sharedSegmentOverlaps, err := boxOverlapCount(edgeLabelBox, sharedSegmentBoxes, label.PADDING, guard)
		if err != nil {
			return 0, err
		}

		totalScore += scoreEdgeLabelOverlaps(
			edgeLabelBox.Width*edgeLabelBox.Height,
			ancestorOverlapArea+nonAncestorOverlapArea,
			0,
			edgeOverlaps,
			almostLabelOverlaps,
			labelOverlaps,
			sharedSegmentOverlaps,
		)
		placedBoxes = append(placedBoxes, edgeLabelBox)
	}

	return 1 / (1 + totalScore), nil
}

func labelBox(edge *layoutgraph.Edge) geo.Box {
	return geo.Box{
		TopLeft: edge.LabelTopLeft(edge.Label.Position, edge.Label.Width, edge.Label.Height),
		Width:   edge.Label.Width,
		Height:  edge.Label.Height,
	}
}

func nodeOverlapCount(box geo.Box, nodes []*layoutgraph.Node, padding float64, guard *limits.WorkGuard) (int, error) {
	count := 0
	for _, node := range nodes {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if boxesOverlapWithPadding(box, node.Box, padding) {
			count++
		}
	}
	return count, nil
}

func partialNodeOverlapCount(box geo.Box, nodes []*layoutgraph.Node, padding float64, guard *limits.WorkGuard) (int, error) {
	count := 0
	for _, node := range nodes {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if boxesOverlapWithPadding(box, node.Box, padding) && !boxCovers(node.Box, box) {
			count++
		}
	}
	return count, nil
}

func boxOverlapCount(box geo.Box, others []geo.Box, padding float64, guard *limits.WorkGuard) (int, error) {
	count := 0
	for _, other := range others {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if boxesOverlapWithPadding(box, other, padding) {
			count++
		}
	}
	return count, nil
}

func nodeOverlapArea(box geo.Box, nodes []*layoutgraph.Node, padding float64, partial bool, guard *limits.WorkGuard) (float64, error) {
	area := 0.0
	for _, node := range nodes {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if !boxesOverlapWithPadding(box, node.Box, padding) || partial && boxCovers(node.Box, box) {
			continue
		}
		if padding == 0 || boxesOverlapWithPadding(box, node.Box, 0) {
			area += boxOverlapArea(box, node.Box)
		}
		if !node.IsContainer() {
			area += box.Width * box.Height
		}
	}
	return area, nil
}

func edgeOverlapCount(box geo.Box, edges []*layoutgraph.Edge, padding float64, guard *limits.WorkGuard) (int, error) {
	count := 0
	for _, edge := range edges {
		for pointIndex := 1; pointIndex < len(edge.Points); pointIndex++ {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if boxOverlapsLine(box, edge.Points[pointIndex-1], edge.Points[pointIndex], padding) {
				count++
			}
		}
	}
	return count, nil
}

func boxesOverlapWithPadding(first, second geo.Box, padding float64) bool {
	firstRight := first.TopLeft.X + first.Width
	secondRight := second.TopLeft.X + second.Width
	if first.TopLeft.X >= secondRight+padding || second.TopLeft.X >= firstRight+padding {
		return false
	}
	firstBottom := first.TopLeft.Y + first.Height
	secondBottom := second.TopLeft.Y + second.Height
	return first.TopLeft.Y < secondBottom+padding && second.TopLeft.Y < firstBottom+padding
}

func boxCovers(outer, inner geo.Box) bool {
	return inner.TopLeft.X >= outer.TopLeft.X &&
		inner.TopLeft.Y >= outer.TopLeft.Y &&
		inner.TopLeft.X+inner.Width <= outer.TopLeft.X+outer.Width &&
		inner.TopLeft.Y+inner.Height <= outer.TopLeft.Y+outer.Height
}

func boxOverlapArea(first, second geo.Box) float64 {
	xs := []float64{first.TopLeft.X, first.TopLeft.X + first.Width, second.TopLeft.X, second.TopLeft.X + second.Width}
	ys := []float64{first.TopLeft.Y, first.TopLeft.Y + first.Height, second.TopLeft.Y, second.TopLeft.Y + second.Height}
	slices.Sort(xs)
	slices.Sort(ys)
	return (xs[2] - xs[1]) * (ys[2] - ys[1])
}

func boxOverlapsLine(box geo.Box, first, second *geo.Point, padding float64) bool {
	contains := func(point *geo.Point) bool {
		return box.TopLeft.X-padding <= point.X &&
			box.TopLeft.X+box.Width+padding >= point.X &&
			box.TopLeft.Y-padding <= point.Y &&
			box.TopLeft.Y+box.Height+padding >= point.Y
	}
	if contains(first) || contains(second) {
		return true
	}

	left := box.TopLeft.X - padding
	right := box.TopLeft.X + box.Width + padding
	top := box.TopLeft.Y - padding
	bottom := box.TopLeft.Y + box.Height + padding
	topLeft := geo.NewPoint(left, top)
	topRight := geo.NewPoint(right, top)
	bottomRight := geo.NewPoint(right, bottom)
	bottomLeft := geo.NewPoint(left, bottom)
	return segmentsIntersect(topLeft, topRight, first, second) ||
		segmentsIntersect(topRight, bottomRight, first, second) ||
		segmentsIntersect(bottomRight, bottomLeft, first, second) ||
		segmentsIntersect(bottomLeft, topLeft, first, second)
}

func segmentsIntersect(firstStart, firstEnd, secondStart, secondEnd *geo.Point) bool {
	secondStartSide := orientation(firstStart, firstEnd, secondStart)
	secondEndSide := orientation(firstStart, firstEnd, secondEnd)
	firstStartSide := orientation(secondStart, secondEnd, firstStart)
	firstEndSide := orientation(secondStart, secondEnd, firstEnd)
	if secondStartSide == 0 && secondEndSide == 0 && firstStartSide == 0 && firstEndSide == 0 {
		return closedIntervalsOverlap(firstStart.X, firstEnd.X, secondStart.X, secondEnd.X) &&
			closedIntervalsOverlap(firstStart.Y, firstEnd.Y, secondStart.Y, secondEnd.Y)
	}
	return straddlesLine(secondStartSide, secondEndSide) && straddlesLine(firstStartSide, firstEndSide)
}

func straddlesLine(first, second float64) bool {
	return first == 0 || second == 0 || (first < 0) != (second < 0)
}

func closedIntervalsOverlap(firstStart, firstEnd, secondStart, secondEnd float64) bool {
	firstMin, firstMax := math.Min(firstStart, firstEnd), math.Max(firstStart, firstEnd)
	secondMin, secondMax := math.Min(secondStart, secondEnd), math.Max(secondStart, secondEnd)
	return firstMin <= secondMax && secondMin <= firstMax
}

func scoreNodeLabelOverlaps(nodeOverlaps, edgeOverlaps, labelOverlaps int) float64 {
	return float64(nodeOverlaps + edgeOverlaps + 2*labelOverlaps)
}

func scoreEdgeLabelOverlaps(
	labelArea, nodeOverlapArea float64,
	nodeOverlapCount, edgeOverlapCount, almostLabelOverlapCount, labelOverlapCount, sharedSegmentOverlapCount int,
) float64 {
	score := nodeOverlapArea / labelArea * 2
	score += float64(labelOverlapCount) * 10
	score += float64(almostLabelOverlapCount)
	score += float64(nodeOverlapCount) * 2
	score += float64(edgeOverlapCount) * 2
	score += float64(sharedSegmentOverlapCount)
	return score
}

func findSharedSegments(edges []*layoutgraph.Edge, guard *limits.WorkGuard) ([]*geo.Segment, error) {
	if err := guard.Step(); err != nil {
		return nil, err
	}
	vertical := make(map[float64][]*geo.Segment)
	horizontal := make(map[float64][]*geo.Segment)
	for _, edge := range edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		for i := 0; i < len(edge.Points)-1; i++ {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			segment := geo.NewSegment(edge.Points[i].Copy(), edge.Points[i+1].Copy())
			if segment.Start.X == segment.End.X {
				if segment.End.Y < segment.Start.Y {
					segment.End.Y, segment.Start.Y = segment.Start.Y, segment.End.Y
				}
				vertical[segment.Start.X] = append(vertical[segment.Start.X], segment)
			} else if segment.Start.Y == segment.End.Y {
				if segment.End.X < segment.Start.X {
					segment.End.X, segment.Start.X = segment.Start.X, segment.End.X
				}
				horizontal[segment.Start.Y] = append(horizontal[segment.Start.Y], segment)
			}
		}
	}

	shared := make([]*geo.Segment, 0, max(len(vertical), len(horizontal)))
	addGroups := func(groups map[float64][]*geo.Segment, horizontal bool) error {
		coordinate := func(point *geo.Point) float64 {
			if horizontal {
				return point.X
			}
			return point.Y
		}
		setCoordinate := func(point *geo.Point, value float64) {
			if horizontal {
				point.X = value
			} else {
				point.Y = value
			}
		}

		for _, segments := range groups {
			if err := guard.Step(); err != nil {
				return err
			}
			if len(segments) == 1 {
				continue
			}
			if err := sortSharedSegments(segments, coordinate, guard); err != nil {
				return err
			}

			previous := segments[0]
			var overlap *geo.Segment
			for i := 1; i < len(segments); i++ {
				if err := guard.Step(); err != nil {
					return err
				}
				current := segments[i]
				if coordinate(previous.End) > coordinate(current.Start) {
					if overlap == nil {
						overlap = geo.NewSegment(current.Start.Copy(), current.Start.Copy())
						setCoordinate(overlap.End, math.Min(coordinate(previous.End), coordinate(current.End)))
					} else {
						minimumOverlap := math.Min(coordinate(previous.End), coordinate(current.End))
						setCoordinate(overlap.End, math.Max(coordinate(overlap.End), minimumOverlap))
					}
				} else if overlap != nil {
					shared = append(shared, overlap)
					overlap = nil
				}
				if coordinate(current.End) > coordinate(previous.End) {
					previous = current
				}
			}
			if overlap != nil {
				shared = append(shared, overlap)
			}
		}
		return nil
	}
	if err := addGroups(vertical, false); err != nil {
		return nil, err
	}
	if err := addGroups(horizontal, true); err != nil {
		return nil, err
	}
	return shared, nil
}

func sortSharedSegments(segments []*geo.Segment, coordinate func(*geo.Point) float64, guard *limits.WorkGuard) error {
	if len(segments) < 2 {
		return nil
	}
	buffer := make([]*geo.Segment, len(segments))
	less := func(left, right *geo.Segment) bool {
		if coordinate(left.Start) == coordinate(right.Start) {
			return coordinate(left.End) < coordinate(right.End)
		}
		return coordinate(left.Start) < coordinate(right.Start)
	}
	items := segments
	for width := 1; width < len(items); {
		for start := 0; start < len(items); start += 2 * width {
			middle := min(start+width, len(items))
			end := min(start+2*width, len(items))
			left, right, output := start, middle, start
			for left < middle && right < end {
				if err := guard.Step(); err != nil {
					return err
				}
				if less(items[right], items[left]) {
					buffer[output] = items[right]
					right++
				} else {
					buffer[output] = items[left]
					left++
				}
				output++
			}
			for left < middle {
				if err := guard.Step(); err != nil {
					return err
				}
				buffer[output] = items[left]
				left++
				output++
			}
			for right < end {
				if err := guard.Step(); err != nil {
					return err
				}
				buffer[output] = items[right]
				right++
				output++
			}
		}
		items, buffer = buffer, items
		if width > len(items)/2 {
			break
		}
		width *= 2
	}
	if len(items) > 0 && &items[0] != &segments[0] {
		copy(segments, items)
	}
	return nil
}
