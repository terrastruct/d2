package labeling

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const (
	// Label placement is intentionally bounded independently of adapter input
	// validation. The engine is also called directly by route-only consumers and
	// tests, so its expensive overlap kernels must defend their own boundary.
	maxLabelPlacementWorkUnits       int64 = limits.MaxLabelPlacementWorkUnits
	maxLabelPlacementAncestryDepth         = layoutgraph.MaxTopologyDepth
	labelPlacementContextCheckStride       = 64
)

// labelPlacementWorkGuard makes every unit of quadratic/candidate work both
// cancellable and bounded. Its overflow check happens before addition, so the
// limit remains deterministic even on 32-bit platforms and hostile inputs.
type labelPlacementWorkGuard struct {
	ctx      context.Context
	location string
	used     int64
	limit    int64
}

func newLabelPlacementWorkGuard(ctx context.Context, location string, limit int64) (*labelPlacementWorkGuard, error) {
	if ctx == nil {
		return nil, fmt.Errorf("TALA %s requires a context", location)
	}
	if limit < 0 {
		return nil, fmt.Errorf("TALA %s work limit must not be negative", location)
	}
	guard := &labelPlacementWorkGuard{
		ctx:      ctx,
		location: location,
		limit:    limit,
	}
	if err := guard.check(); err != nil {
		return nil, err
	}
	return guard, nil
}

func (guard *labelPlacementWorkGuard) step() error {
	return guard.add(1)
}

func (guard *labelPlacementWorkGuard) add(units int64) error {
	if units < 0 {
		return fmt.Errorf("TALA %s received a negative work estimate", guard.location)
	}
	// Written this way rather than used+units > limit so the calculation cannot
	// wrap before the comparison.
	if guard.used < 0 || guard.used > guard.limit || units > guard.limit-guard.used {
		// Preserve cancellation as the primary failure when it races the budget
		// boundary; otherwise callers could receive a resource error after they
		// had already canceled the work.
		if err := guard.check(); err != nil {
			return err
		}
		return fmt.Errorf("TALA %s work exceeds limit %d", guard.location, guard.limit)
	}
	previous := guard.used
	guard.used += units
	if units == 0 || previous/labelPlacementContextCheckStride != guard.used/labelPlacementContextCheckStride {
		return guard.check()
	}
	return nil
}

func (guard *labelPlacementWorkGuard) check() error {
	if err := guard.ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", guard.location, err)
	}
	return nil
}

type labelPositionSnapshot struct {
	label    *layoutgraph.Label
	position label.Position
}

type iconPositionSnapshot struct {
	icon     *layoutgraph.Icon
	position label.Position
}

type edgeLabelPercentageSnapshot struct {
	edge       *layoutgraph.Edge
	percentage float64
}

type labelPlacementSnapshot struct {
	labels          []labelPositionSnapshot
	icons           []iconPositionSnapshot
	edgePercentages []edgeLabelPercentageSnapshot
}

// captureLabelPlacement records only the state this algorithm owns. Keeping
// the original label/icon objects (rather than copying or replacing them)
// preserves pointer identity for callers that retain aliases.
func captureLabelPlacement(g *layoutgraph.Graph, extraEdges []*layoutgraph.Edge) labelPlacementSnapshot {
	snapshot := labelPlacementSnapshot{}
	seenLabels := make(map[*layoutgraph.Label]struct{})
	seenIcons := make(map[*layoutgraph.Icon]struct{})
	seenEdges := make(map[*layoutgraph.Edge]struct{})

	captureLabel := func(value *layoutgraph.Label) {
		if value == nil {
			return
		}
		if _, seen := seenLabels[value]; seen {
			return
		}
		seenLabels[value] = struct{}{}
		snapshot.labels = append(snapshot.labels, labelPositionSnapshot{
			label:    value,
			position: value.Position,
		})
	}
	captureIcon := func(value *layoutgraph.Icon) {
		if value == nil {
			return
		}
		if _, seen := seenIcons[value]; seen {
			return
		}
		seenIcons[value] = struct{}{}
		snapshot.icons = append(snapshot.icons, iconPositionSnapshot{
			icon:     value,
			position: value.Position,
		})
	}
	captureEdge := func(edge *layoutgraph.Edge) {
		if edge == nil {
			return
		}
		captureLabel(edge.Label)
		if _, seen := seenEdges[edge]; seen {
			return
		}
		seenEdges[edge] = struct{}{}
		snapshot.edgePercentages = append(snapshot.edgePercentages, edgeLabelPercentageSnapshot{
			edge:       edge,
			percentage: edge.LabelPercentage,
		})
	}

	if g != nil {
		for _, node := range g.Nodes {
			if node == nil {
				continue
			}
			captureLabel(node.Label)
			captureIcon(node.Icon)
		}
		for _, edge := range g.Edges {
			captureEdge(edge)
		}
	}
	for _, edge := range extraEdges {
		captureEdge(edge)
	}
	return snapshot
}

func (snapshot labelPlacementSnapshot) restore() {
	for _, state := range snapshot.labels {
		state.label.Position = state.position
	}
	for _, state := range snapshot.icons {
		state.icon.Position = state.position
	}
	for _, state := range snapshot.edgePercentages {
		state.edge.LabelPercentage = state.percentage
	}
}

func nodeOverlapCount(node *layoutgraph.Node, nodes layoutgraph.Nodes, delta float64, guard *labelPlacementWorkGuard) (int, error) {
	count := 0
	for _, otherNode := range nodes {
		if err := guard.step(); err != nil {
			return 0, err
		}
		if node.LabelBoxesOverlap(otherNode, delta) {
			count++
		}
	}
	return count, nil
}

func partialNodeOverlapCount(node *layoutgraph.Node, nodes layoutgraph.Nodes, delta float64, guard *labelPlacementWorkGuard) (int, error) {
	count := 0
	for _, otherNode := range nodes {
		if err := guard.step(); err != nil {
			return 0, err
		}
		if node.LabelBoxesOverlap(otherNode, delta) && !otherNode.Covers(node) {
			count++
		}
	}
	return count, nil
}

func nodeOverlapArea(node *layoutgraph.Node, nodes layoutgraph.Nodes, delta float64, partial bool, guard *labelPlacementWorkGuard) (float64, int, error) {
	area := 0.0
	overlapCount := 0
	for _, otherNode := range nodes {
		if err := guard.step(); err != nil {
			return 0, 0, err
		}
		if !node.LabelBoxesOverlap(otherNode, delta) || (partial && otherNode.Covers(node)) {
			continue
		}
		if delta == 0 || node.DoesOverlapExact(otherNode) {
			area += node.LabelOverlapArea(otherNode)
			overlapCount++
		}
		if !otherNode.IsContainer() {
			area += node.Area()
			overlapCount++
		}
	}
	return area, overlapCount, nil
}

func edgeOverlapCount(node *layoutgraph.Node, edges layoutgraph.Edges, delta float64, guard *labelPlacementWorkGuard) (int, error) {
	count := 0
	for _, edge := range edges {
		if err := guard.step(); err != nil {
			return 0, err
		}
		for pointIndex := 1; pointIndex < len(edge.Points); pointIndex++ {
			if err := guard.step(); err != nil {
				return 0, err
			}
			if node.OverlapsLine(edge.Points[pointIndex-1], edge.Points[pointIndex], delta) {
				count++
			}
		}
	}
	return count, nil
}

func collectLabelPlacementAncestors(node *layoutgraph.Node, guard *labelPlacementWorkGuard) (layoutgraph.Nodes, error) {
	ancestors := make(layoutgraph.Nodes, 0, 4)
	for current, depth := node, 0; current != nil; depth++ {
		if depth >= maxLabelPlacementAncestryDepth {
			return nil, fmt.Errorf("TALA %s container depth exceeds limit %d", guard.location, maxLabelPlacementAncestryDepth)
		}
		if err := guard.step(); err != nil {
			return nil, err
		}
		ancestors = append(ancestors, current)
		current = current.EffectiveContainer()
	}
	return ancestors, nil
}

func isLabelPlacementDescendantOf(maybeDescendant, maybeAncestor *layoutgraph.Node, guard *labelPlacementWorkGuard) (bool, error) {
	for depth := 0; ; depth++ {
		if maybeAncestor == maybeDescendant {
			return true, nil
		}
		if maybeDescendant == nil {
			return false, nil
		}
		if depth >= maxLabelPlacementAncestryDepth {
			return false, fmt.Errorf("TALA %s ancestry depth exceeds limit %d", guard.location, maxLabelPlacementAncestryDepth)
		}
		if err := guard.step(); err != nil {
			return false, err
		}
		switch {
		case maybeDescendant.Container != nil:
			maybeDescendant = maybeDescendant.Container
		case maybeDescendant.Cluster != nil:
			maybeDescendant = maybeDescendant.Cluster.Vessel
		case maybeDescendant.Sequence != nil:
			maybeDescendant = maybeDescendant.Sequence.Vessel
		default:
			maybeDescendant = nil
		}
	}
}

func clusterBoundaryNodePlacement(node *layoutgraph.Node, nodes layoutgraph.Nodes, orientation geo.Orientation, guard *labelPlacementWorkGuard) (bool, error) {
	for _, other := range nodes {
		if err := guard.step(); err != nil {
			return false, err
		}
		if other == node || other.TopLeft == nil {
			continue
		}
		switch orientation {
		case geo.Left:
			if other.TopLeft.X < node.TopLeft.X {
				return false, nil
			}
		case geo.Top:
			if other.TopLeft.Y < node.TopLeft.Y {
				return false, nil
			}
		case geo.Right:
			if other.TopLeft.X+other.Width > node.TopLeft.X+node.Width {
				return false, nil
			}
		case geo.Bottom:
			if other.TopLeft.Y+other.Height > node.TopLeft.Y+node.Height {
				return false, nil
			}
		}
	}
	return true, nil
}

func clusterNodeBoundingBoxPlacement(node *layoutgraph.Node, nodes layoutgraph.Nodes, guard *labelPlacementWorkGuard) (*geo.Point, *geo.Point, error) {
	if node == nil || node.TopLeft == nil {
		return nil, nil, fmt.Errorf("TALA %s cluster contains an unplaced node", guard.location)
	}
	if err := guard.step(); err != nil {
		return nil, nil, err
	}
	tl := node.TopLeft.Copy()
	br := geo.NewPoint(math.Round(tl.X+node.Width), math.Round(tl.Y+node.Height))
	if dx, dy := node.ModifierElementAdjustments(); dx != 0 || dy != 0 {
		tl.Y -= dy
		br.X += dx
	}
	tl.X -= node.LoopOffsets[geo.Left]
	tl.Y -= node.LoopOffsets[geo.Top]
	br.X += node.LoopOffsets[geo.Right]
	br.Y += node.LoopOffsets[geo.Bottom]

	if node.Label != nil && node.Label.Position.IsOutside() {
		labelTopLeft := node.LabelTopLeft(node.Label.Position, node.Label.Width, node.Label.Height)
		boundaryPadding := float64(label.PADDING)
		outsidePadding := 2. * label.PADDING
		if labelTopLeft.X < tl.X {
			boundary, err := clusterBoundaryNodePlacement(node, nodes, geo.Left, guard)
			if err != nil {
				return nil, nil, err
			}
			padding := outsidePadding
			if boundary {
				padding = boundaryPadding
			}
			tl.X = math.Floor(labelTopLeft.X - padding)
		}
		if labelTopLeft.Y < tl.Y {
			boundary, err := clusterBoundaryNodePlacement(node, nodes, geo.Top, guard)
			if err != nil {
				return nil, nil, err
			}
			padding := outsidePadding
			if boundary {
				padding = boundaryPadding
			}
			tl.Y = math.Floor(labelTopLeft.Y - padding)
		}
		if labelTopLeft.X > br.X {
			boundary, err := clusterBoundaryNodePlacement(node, nodes, geo.Right, guard)
			if err != nil {
				return nil, nil, err
			}
			padding := outsidePadding
			if boundary {
				padding = boundaryPadding
			}
			br.X = math.Ceil(labelTopLeft.X + node.Label.Width + padding)
		}
		if labelTopLeft.Y > br.Y {
			boundary, err := clusterBoundaryNodePlacement(node, nodes, geo.Bottom, guard)
			if err != nil {
				return nil, nil, err
			}
			padding := outsidePadding
			if boundary {
				padding = boundaryPadding
			}
			br.Y = math.Ceil(labelTopLeft.Y + node.Label.Height + padding)
		}
	}
	if node.Icon != nil && !node.IsImage() && node.Icon.Position.IsOutside() {
		iconSize := float64(layoutgraph.MaxIconSize)
		iconTopLeft := node.LabelTopLeft(node.Icon.Position, iconSize, iconSize)
		outsidePadding := 2. * label.PADDING
		tl.X = math.Min(tl.X, math.Floor(iconTopLeft.X-outsidePadding))
		tl.Y = math.Min(tl.Y, math.Floor(iconTopLeft.Y-outsidePadding))
		br.X = math.Max(br.X, math.Ceil(iconTopLeft.X+iconSize+outsidePadding))
		br.Y = math.Max(br.Y, math.Ceil(iconTopLeft.Y+iconSize+outsidePadding))
	}
	return tl, br, nil
}

func clusterLabelPlacementOrientation(node, member *layoutgraph.Node, cluster *layoutgraph.Cluster, guard *labelPlacementWorkGuard) (geo.Orientation, int, error) {
	clusterTopLeft := geo.NewPoint(math.Inf(-1), math.Inf(-1))
	clusterBottomRight := geo.NewPoint(math.Inf(1), math.Inf(1))
	if len(cluster.Nodes) > 0 {
		clusterTopLeft = geo.NewPoint(math.Inf(1), math.Inf(1))
		clusterBottomRight = geo.NewPoint(math.Inf(-1), math.Inf(-1))
		for _, clusterNode := range cluster.Nodes {
			topLeft, bottomRight, err := clusterNodeBoundingBoxPlacement(clusterNode, cluster.Nodes, guard)
			if err != nil {
				return geo.NONE, -1, err
			}
			clusterTopLeft.X = math.Min(clusterTopLeft.X, topLeft.X)
			clusterTopLeft.Y = math.Min(clusterTopLeft.Y, topLeft.Y)
			clusterBottomRight.X = math.Max(clusterBottomRight.X, bottomRight.X)
			clusterBottomRight.Y = math.Max(clusterBottomRight.Y, bottomRight.Y)
		}
	}
	shell := layoutgraph.NewNode(0, clusterBottomRight.X-clusterTopLeft.X, clusterBottomRight.Y-clusterTopLeft.Y)
	shell.TopLeft = clusterTopLeft
	orientation := node.Orientation(shell)
	index := -1
	for candidateIndex, candidate := range cluster.Nodes {
		if err := guard.step(); err != nil {
			return geo.NONE, -1, err
		}
		if candidate == member {
			index = candidateIndex
			break
		}
	}
	return orientation, index, nil
}

func routeLengthPlacement(edge *layoutgraph.Edge, guard *labelPlacementWorkGuard) (float64, error) {
	length := 0.0
	for index := 1; index < len(edge.Points); index++ {
		if err := guard.step(); err != nil {
			return 0, err
		}
		previous := edge.Points[index-1]
		current := edge.Points[index]
		length += geo.EuclideanDistance(previous.X, previous.Y, current.X, current.Y)
	}
	return length, nil
}

func edgeLabelTopLeft(edge *layoutgraph.Edge, position label.Position, width, height float64, guard *labelPlacementWorkGuard) (*geo.Point, error) {
	// d2's route label helper walks the route once for total length and once to
	// locate the requested distance. Reserve both passes before entering it,
	// then check again at the boundary.
	for pass := 0; pass < 2; pass++ {
		if err := guard.add(int64(len(edge.Points))); err != nil {
			return nil, err
		}
	}
	point := edge.LabelTopLeft(position, width, height)
	if err := guard.check(); err != nil {
		return nil, err
	}
	return point, nil
}

func positionedArrowheadLabel(edge *layoutgraph.Edge, isTarget bool, guard *labelPlacementWorkGuard) (*PositionedArrowheadLabel, error) {
	// Arrowhead placement computes route length directly, then the label helper
	// performs its own length and distance passes.
	for pass := 0; pass < 3; pass++ {
		if err := guard.add(int64(len(edge.Points))); err != nil {
			return nil, err
		}
	}
	positioned := PositionArrowheadLabel(edge, isTarget, edge.Points)
	if err := guard.check(); err != nil {
		return nil, err
	}
	return positioned, nil
}

type labelEdgeSortItem struct {
	edge   *layoutgraph.Edge
	length float64
}

func sortLabelPlacementEdges(edges []*layoutgraph.Edge, guard *labelPlacementWorkGuard) ([]*layoutgraph.Edge, error) {
	items := make([]labelEdgeSortItem, len(edges))
	for index, edge := range edges {
		if err := guard.step(); err != nil {
			return nil, err
		}
		length, err := routeLengthPlacement(edge, guard)
		if err != nil {
			return nil, err
		}
		items[index] = labelEdgeSortItem{edge: edge, length: length}
	}
	buffer := make([]labelEdgeSortItem, len(items))
	less := func(left, right labelEdgeSortItem) bool {
		if len(right.edge.Points) == 2 && len(left.edge.Points) > 2 {
			return true
		}
		if len(left.edge.Points) == 2 && len(right.edge.Points) > 2 {
			return false
		}
		return left.length < right.length
	}
	for width := 1; width < len(items); {
		for start := 0; start < len(items); start += 2 * width {
			middle := min(start+width, len(items))
			end := min(start+2*width, len(items))
			left, right, output := start, middle, start
			for left < middle && right < end {
				if err := guard.step(); err != nil {
					return nil, err
				}
				// Choose the left item for ties, matching sort.SliceStable.
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
				if err := guard.step(); err != nil {
					return nil, err
				}
				buffer[output] = items[left]
				left++
				output++
			}
			for right < end {
				if err := guard.step(); err != nil {
					return nil, err
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
	sorted := make([]*layoutgraph.Edge, len(items))
	for index, item := range items {
		sorted[index] = item.edge
	}
	return sorted, guard.check()
}

func sortSharedSegmentGroup(segments []*geo.Segment, coordinate func(*geo.Point) float64, checkWork func() error) error {
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
				if checkWork != nil {
					if err := checkWork(); err != nil {
						return err
					}
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
				if checkWork != nil {
					if err := checkWork(); err != nil {
						return err
					}
				}
				buffer[output] = items[left]
				left++
				output++
			}
			for right < end {
				if checkWork != nil {
					if err := checkWork(); err != nil {
						return err
					}
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
