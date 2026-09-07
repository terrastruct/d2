package labeling

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

const (
	// SharedSegmentClearance reserves space around shared route segments where
	// an edge label would otherwise be ambiguous.
	SharedSegmentClearance  = 2.5 * label.PADDING
	placementSearchInterval = 0.025
)

// Place chooses positions for every movable node label, icon, and routed edge
// label in graph.
func Place(ctx context.Context, graph *layoutgraph.Graph) error {
	return place(ctx, graph, maxLabelPlacementWorkUnits)
}

func place(ctx context.Context, g *layoutgraph.Graph, workLimit int64) (err error) {
	const location = "PlaceLabels"
	if err := layoutgraph.ValidatePositionedGraphSelection(ctx, location, g, nil); err != nil {
		return err
	}
	snapshot := captureLabelPlacement(g, nil)
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot.restore()
			panic(recovered)
		}
		if err != nil {
			snapshot.restore()
		}
	}()

	guard, err := newLabelPlacementWorkGuard(ctx, location, workLimit)
	if err != nil {
		return err
	}
	checkCanceled := guard.step

	var placedFakeNodes []*layoutgraph.Node

	// add all arrowhead labels to placedFakeNodes
	for _, e := range g.Edges {
		if err := checkCanceled(); err != nil {
			return err
		}
		if label := e.SourceArrowheadLabel; label != nil {
			pal, err := positionedArrowheadLabel(e, false, guard)
			if err != nil {
				return err
			}
			fakeLabelNode := &layoutgraph.Node{
				Box:   pal.Box,
				D2ID:  new(label.Text),
				Graph: g,
			}
			fakeLabelNode.SetShape(shape.SQUARE_TYPE)
			placedFakeNodes = append(placedFakeNodes, fakeLabelNode)
		}
		if label := e.TargetArrowheadLabel; label != nil {
			pal, err := positionedArrowheadLabel(e, true, guard)
			if err != nil {
				return err
			}
			fakeLabelNode := &layoutgraph.Node{
				Box:   pal.Box,
				D2ID:  new(label.Text),
				Graph: g,
			}
			fakeLabelNode.SetShape(shape.SQUARE_TYPE)
			placedFakeNodes = append(placedFakeNodes, fakeLabelNode)
		}
	}
	for _, e := range g.Edges {
		if err := checkCanceled(); err != nil {
			return err
		}
		if e.Label != nil && (e.IsLoop() || e.Label.PositionFixed()) {
			topLeft, err := edgeLabelTopLeft(e, e.Label.Position, e.Label.Width, e.Label.Height, guard)
			if err != nil {
				return err
			}
			fakeLabelNode := &layoutgraph.Node{
				Box: geo.Box{
					TopLeft: topLeft,
					Width:   e.Label.Width,
					Height:  e.Label.Height,
				},
				D2ID:  new(e.Label.Text),
				Graph: g,
			}
			fakeLabelNode.SetShape(shape.SQUARE_TYPE)
			placedFakeNodes = append(placedFakeNodes, fakeLabelNode)
		}
	}

	// reposition node labels if label is on the same side as a used port or if there is an edge that passes nearby
	for _, node := range g.Nodes {
		if err := checkCanceled(); err != nil {
			return err
		}
		siblingsCapacity := len(g.Containers[node.Container]) + len(g.Containers[node])
		if siblingsCapacity > 0 {
			siblingsCapacity--
		}
		siblingsAndChildren := make([]*layoutgraph.Node, 0, siblingsCapacity)
		for _, n := range g.Containers[node.Container] {
			if err := checkCanceled(); err != nil {
				return err
			}
			if n != node {
				siblingsAndChildren = append(siblingsAndChildren, n)
			}
		}
		if node.IsContainer() {
			for _, child := range g.Containers[node] {
				if err := checkCanceled(); err != nil {
					return err
				}
				siblingsAndChildren = append(siblingsAndChildren, child)
			}
		}

		ancestors, err := collectLabelPlacementAncestors(node, guard)
		if err != nil {
			return err
		}

		var bestIconPosition label.Position
		var bestFakeNode *layoutgraph.Node
		bestScore := math.Inf(1)
		if node.Icon != nil && !node.IsImage() {
			if !node.Icon.PositionFixed() {
				iconPositions := labelPositionPreferences(node)
				for _, iconPosition := range iconPositions {
					if err := checkCanceled(); err != nil {
						return err
					}
					iconSize := node.IconSize(iconPosition)
					fakeIconNode := &layoutgraph.Node{
						Box: geo.Box{
							TopLeft: node.LabelTopLeft(iconPosition, iconSize, iconSize),
							Width:   iconSize,
							Height:  iconSize,
						},
						Graph: g,
					}
					fakeIconNode.SetShape(shape.SQUARE_TYPE)

					nodeset := siblingsAndChildren

					if node.Label != nil && node.Label.PositionFixed() {
						fixedLabelNode := &layoutgraph.Node{
							Box: geo.Box{
								TopLeft: node.LabelTopLeft(node.Label.Position, node.Label.Width, node.Label.Height),
								Width:   node.Label.Width,
								Height:  node.Label.Height,
							},
							D2ID:  new(node.Label.Text),
							Graph: g,
						}
						fixedLabelNode.SetShape(shape.SQUARE_TYPE)
						nodeset = append(nodeset, fixedLabelNode)
					}

					nodeOverlaps, err := nodeOverlapCount(fakeIconNode, nodeset, 0, guard)
					if err != nil {
						return err
					}
					edgeOverlaps, err := edgeOverlapCount(fakeIconNode, g.Edges, 0, guard)
					if err != nil {
						return err
					}
					labelOverlaps, err := nodeOverlapCount(fakeIconNode, placedFakeNodes, 0, guard)
					if err != nil {
						return err
					}
					score := scoreNodeLabelOverlaps(nodeOverlaps, edgeOverlaps, labelOverlaps)
					if score < bestScore {
						bestIconPosition = iconPosition
						bestFakeNode = fakeIconNode
						bestScore = score
						if score == 0 {
							break
						}
					}
				}
			} else {
				iconSize := node.IconSize(node.Icon.Position)
				fakeIconNode := &layoutgraph.Node{
					Box: geo.Box{
						TopLeft: node.LabelTopLeft(node.Icon.Position, iconSize, iconSize),
						Width:   iconSize,
						Height:  iconSize,
					},
					Graph: g,
				}
				fakeIconNode.SetShape(shape.SQUARE_TYPE)
				bestFakeNode = fakeIconNode
				bestIconPosition = node.Icon.Position
			}
			if err := checkCanceled(); err != nil {
				return err
			}
			placedFakeNodes = append(placedFakeNodes, bestFakeNode)
			node.Icon.Position = bestIconPosition
		}

		if node.Label == nil || node.Label.PositionFixed() {
			continue
		}

		if node.Icon != nil && !node.IsImage() {
			iconSize := node.IconSize(node.Icon.Position)
			fakeIconNode := &layoutgraph.Node{
				Box: geo.Box{
					TopLeft: node.LabelTopLeft(node.Icon.Position, iconSize, iconSize),
					Width:   iconSize,
					Height:  iconSize,
				},
				Graph: g,
			}
			fakeIconNode.SetShape(shape.SQUARE_TYPE)
			siblingsAndChildren = append(siblingsAndChildren, fakeIconNode)
		}

		bestScore = math.Inf(1)
		var bestLabelPosition label.Position
		tranches := labelPositionPreferenceTranches(node)
		for _, tranch := range tranches {
			if err := checkCanceled(); err != nil {
				return err
			}
			// For each tranch, if there's multiple positions equally good, break ties with
			// slightly larger fake label box, so that the label goes on the side with the most empty space
			var tiedBest []label.Position
			var tiedBestFakeLabelNodes []*layoutgraph.Node
			for _, labelPosition := range tranch {
				if err := checkCanceled(); err != nil {
					return err
				}
				if labelPosition == bestIconPosition {
					continue
				}
				fakeLabelNode := &layoutgraph.Node{
					Box: geo.Box{
						TopLeft: node.LabelTopLeft(labelPosition, node.Label.Width, node.Label.Height),
						Width:   node.Label.Width,
						Height:  node.Label.Height,
					},
					D2ID:  new(node.Label.Text),
					Graph: g,
				}
				fakeLabelNode.SetShape(shape.SQUARE_TYPE)
				// if an inside label escapes the node it is a bad placement
				if !labelPosition.IsOutside() {
					// inside labels must fit inside node with label.PADDING on each side
					fakeLabelNode.PadLabelCandidate(label.PADDING)
					if !layoutgraph.LabelBoxFits(node.InnerBox(), fakeLabelNode.GetBox()) {
						continue
					}
					fakeLabelNode.PadLabelCandidate(-label.PADDING)
				}

				// it must be label.PADDING-1 so that it does not overlap with edges at the node border
				//      │
				// ┌────▼─────┐
				// │  label   │
				// │          │
				// └──────────┘
				siblingOverlaps, err := nodeOverlapCount(fakeLabelNode, siblingsAndChildren, label.PADDING, guard)
				if err != nil {
					return err
				}
				ancestorOverlaps, err := partialNodeOverlapCount(fakeLabelNode, ancestors, label.PADDING, guard)
				if err != nil {
					return err
				}
				edgeOverlaps, err := edgeOverlapCount(fakeLabelNode, g.Edges, label.PADDING-1, guard)
				if err != nil {
					return err
				}
				labelOverlaps, err := nodeOverlapCount(fakeLabelNode, placedFakeNodes, label.PADDING, guard)
				if err != nil {
					return err
				}
				score := scoreNodeLabelOverlaps(siblingOverlaps+ancestorOverlaps, edgeOverlaps, labelOverlaps)
				if score < bestScore {
					bestLabelPosition = labelPosition
					bestFakeNode = fakeLabelNode
					bestScore = score
					if labelPosition.IsOutside() && bestLabelPosition.IsOutside() {
						tiedBest = []label.Position{labelPosition}
						tiedBestFakeLabelNodes = []*layoutgraph.Node{fakeLabelNode}
					} else {
						tiedBest = []label.Position{}
						tiedBestFakeLabelNodes = []*layoutgraph.Node{}
					}
				} else if score == bestScore && compareLabelPositions(node, labelPosition, bestLabelPosition) == 0 && bestLabelPosition.IsOutside() && labelPosition.IsOutside() {
					tiedBest = append(tiedBest, labelPosition)
					tiedBestFakeLabelNodes = append(tiedBestFakeLabelNodes, fakeLabelNode)
				}
			}
			if len(tiedBest) > 1 {
				bestTiebreakScore := math.Inf(1)
				var bestTieBreakLabelPosition label.Position
				var bestTieBreakFakeNode *layoutgraph.Node
				for i, labelPosition := range tiedBest {
					if err := checkCanceled(); err != nil {
						return err
					}
					tiebreakFakeLabelNode := &layoutgraph.Node{
						Box: geo.Box{
							TopLeft: node.LabelTopLeft(labelPosition, node.Label.Width*2, node.Label.Height*2),
							Width:   node.Label.Width * 2,
							Height:  node.Label.Height * 2,
						},
						D2ID:  new(node.Label.Text),
						Graph: g,
					}
					tiebreakFakeLabelNode.SetShape(shape.SQUARE_TYPE)
					// if an inside label escapes the node it is a bad placement
					if !labelPosition.IsOutside() {
						// inside labels must fit inside node with label.PADDING on each side
						tiebreakFakeLabelNode.PadLabelCandidate(label.PADDING)
						if !layoutgraph.LabelBoxFits(node.InnerBox(), tiebreakFakeLabelNode.GetBox()) {
							continue
						}
						tiebreakFakeLabelNode.PadLabelCandidate(-label.PADDING)
					}

					siblingOverlaps, err := nodeOverlapCount(tiebreakFakeLabelNode, siblingsAndChildren, label.PADDING, guard)
					if err != nil {
						return err
					}
					ancestorOverlaps, err := partialNodeOverlapCount(tiebreakFakeLabelNode, ancestors, label.PADDING, guard)
					if err != nil {
						return err
					}
					edgeOverlaps, err := edgeOverlapCount(tiebreakFakeLabelNode, g.Edges, label.PADDING-1, guard)
					if err != nil {
						return err
					}
					labelOverlaps, err := nodeOverlapCount(tiebreakFakeLabelNode, placedFakeNodes, label.PADDING, guard)
					if err != nil {
						return err
					}
					score := scoreNodeLabelOverlaps(siblingOverlaps+ancestorOverlaps, edgeOverlaps, labelOverlaps)
					if score < bestTiebreakScore {
						bestTiebreakScore = score
						bestTieBreakLabelPosition = labelPosition
						bestTieBreakFakeNode = tiedBestFakeLabelNodes[i]
					}
				}
				bestLabelPosition = bestTieBreakLabelPosition
				bestFakeNode = bestTieBreakFakeNode
			}
		}
		if err := checkCanceled(); err != nil {
			return err
		}
		placedFakeNodes = append(placedFakeNodes, bestFakeNode)

		node.Label.Position = bestLabelPosition
	}

	// we want to find the best edge label position, avoiding overlapping with other nodes, edges, labels and shared edge segments
	// create fake nodes around shared segments to avoid having labels placed in these positions
	sharedSegments, err := findSharedSegmentsChecked(g.Edges, guard.step)
	if err != nil {
		return err
	}
	sharedSegmentFakeNodes := make([]*layoutgraph.Node, 0, len(sharedSegments))
	for i, seg := range sharedSegments {
		if err := checkCanceled(); err != nil {
			return err
		}
		tl := seg.Start.Copy()
		width := seg.End.X - seg.Start.X
		height := seg.End.Y - seg.Start.Y
		// generate a box around the shared segments
		// │  │  │
		// shared│
		// segment
		// │  │  │
		// │  │  │
		// │  │  │
		// │  │  │
		// └──┴──┘
		// |-----| fake box
		// |--| SharedSegmentClearance
		if seg.End.X == seg.Start.X {
			// vertical
			width = 2 * SharedSegmentClearance
			tl.X -= SharedSegmentClearance
		} else {
			// horizontal
			height = 2 * SharedSegmentClearance
			tl.Y -= SharedSegmentClearance
		}
		fakeSharedNode := &layoutgraph.Node{
			Box: geo.Box{
				TopLeft: tl,
				Width:   width,
				Height:  height,
			},
			D2ID:  new(fmt.Sprintf("fake_shared_segment_%d", i)),
			Graph: g,
		}
		fakeSharedNode.SetShape(shape.SQUARE_TYPE)
		sharedSegmentFakeNodes = append(sharedSegmentFakeNodes, fakeSharedNode)
	}

	sortedEdges := make([]*layoutgraph.Edge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if err := checkCanceled(); err != nil {
			return err
		}
		if e.Label == nil || e.IsLoop() || e.Label.PositionFixed() {
			continue
		}
		sortedEdges = append(sortedEdges, e)
	}

	sortedEdges, err = sortLabelPlacementEdges(sortedEdges, guard)
	if err != nil {
		return err
	}

	for _, edge := range sortedEdges {
		if err := checkCanceled(); err != nil {
			return err
		}
		fakeNode, position, percentage, err := findBestEdgeLabelPosition(edge, g, placedFakeNodes, sharedSegmentFakeNodes, guard)
		if err != nil {
			return err
		}
		placedFakeNodes = append(placedFakeNodes, fakeNode)
		edge.Label.Position = position
		edge.LabelPercentage = percentage
	}

	return guard.check()
}

// PlaceNewEdges chooses label positions only for edges, preserving all
// existing label placements in graph as obstacles.
func PlaceNewEdges(ctx context.Context, graph *layoutgraph.Graph, edges []*layoutgraph.Edge) error {
	return placeNewEdges(ctx, graph, edges, maxLabelPlacementWorkUnits)
}

func placeNewEdges(ctx context.Context, g *layoutgraph.Graph, edges []*layoutgraph.Edge, workLimit int64) (err error) {
	const location = "PlaceNewEdgeLabels"
	if err := layoutgraph.ValidatePositionedGraphSelection(ctx, location, g, edges); err != nil {
		return err
	}
	snapshot := captureLabelPlacement(g, edges)
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot.restore()
			panic(recovered)
		}
		if err != nil {
			snapshot.restore()
		}
	}()

	guard, err := newLabelPlacementWorkGuard(ctx, location, workLimit)
	if err != nil {
		return err
	}
	checkCanceled := guard.step

	var placedFakeNodes []*layoutgraph.Node

	newEdges := make(map[*layoutgraph.Edge]struct{})
	for _, e := range edges {
		if err := checkCanceled(); err != nil {
			return err
		}
		newEdges[e] = struct{}{}
	}

	// Reserve existing and caller-fixed edge labels, plus all arrowhead labels.
	for _, e := range g.Edges {
		if err := checkCanceled(); err != nil {
			return err
		}
		if label := e.SourceArrowheadLabel; label != nil {
			pal, err := positionedArrowheadLabel(e, false, guard)
			if err != nil {
				return err
			}
			fakeLabelNode := &layoutgraph.Node{
				Box:   pal.Box,
				D2ID:  new(label.Text),
				Graph: g,
			}
			fakeLabelNode.SetShape(shape.SQUARE_TYPE)
			placedFakeNodes = append(placedFakeNodes, fakeLabelNode)
		}
		if label := e.TargetArrowheadLabel; label != nil {
			pal, err := positionedArrowheadLabel(e, true, guard)
			if err != nil {
				return err
			}
			fakeLabelNode := &layoutgraph.Node{
				Box:   pal.Box,
				D2ID:  new(label.Text),
				Graph: g,
			}
			fakeLabelNode.SetShape(shape.SQUARE_TYPE)
			placedFakeNodes = append(placedFakeNodes, fakeLabelNode)
		}

		if e.Label == nil || e.IsLoop() {
			continue
		}
		if _, in := newEdges[e]; in && !e.Label.PositionFixed() {
			continue
		}

		topLeft, err := edgeLabelTopLeft(e, e.Label.Position, e.Label.Width, e.Label.Height, guard)
		if err != nil {
			return err
		}
		fakeLabelNode := &layoutgraph.Node{
			Box: geo.Box{
				TopLeft: topLeft,
				Width:   e.Label.Width,
				Height:  e.Label.Height,
			},
			D2ID:  new(e.Label.Text),
			Graph: g,
		}
		fakeLabelNode.SetShape(shape.SQUARE_TYPE)
		placedFakeNodes = append(placedFakeNodes, fakeLabelNode)
	}

	for _, e := range g.Edges {
		if err := checkCanceled(); err != nil {
			return err
		}
		if e.IsLoop() && e.Label != nil {
			topLeft, err := edgeLabelTopLeft(e, e.Label.Position, e.Label.Width, e.Label.Height, guard)
			if err != nil {
				return err
			}
			fakeLabelNode := &layoutgraph.Node{
				Box: geo.Box{
					TopLeft: topLeft,
					Width:   e.Label.Width,
					Height:  e.Label.Height,
				},
				D2ID:  new(e.Label.Text),
				Graph: g,
			}
			fakeLabelNode.SetShape(shape.SQUARE_TYPE)
			placedFakeNodes = append(placedFakeNodes, fakeLabelNode)
		}
	}

	// reposition node labels if label is on the same side as a used port or if there is an edge that passes nearby
	for _, node := range g.Nodes {
		if err := checkCanceled(); err != nil {
			return err
		}
		if node.Icon != nil && !node.IsImage() {
			iconSize := node.IconSize(node.Icon.Position)
			fakeIconNode := &layoutgraph.Node{
				Box: geo.Box{
					TopLeft: node.LabelTopLeft(node.Icon.Position, iconSize, iconSize),
					Width:   iconSize,
					Height:  iconSize,
				},
				Graph: g,
			}
			fakeIconNode.SetShape(shape.SQUARE_TYPE)
			placedFakeNodes = append(placedFakeNodes, fakeIconNode)
		}

		if node.Label == nil || node.Label.PositionFixed() {
			continue
		}

		fakeLabelNode := &layoutgraph.Node{
			Box: geo.Box{
				TopLeft: node.LabelTopLeft(node.Label.Position, node.Label.Width, node.Label.Height),
				Width:   node.Label.Width,
				Height:  node.Label.Height,
			},
			D2ID:  new(node.Label.Text),
			Graph: g,
		}
		fakeLabelNode.SetShape(shape.SQUARE_TYPE)
		placedFakeNodes = append(placedFakeNodes, fakeLabelNode)
	}

	// we want to find the best edge label position, avoiding overlapping with other nodes, edges, labels and shared edge segments
	// create fake nodes around shared segments to avoid having labels placed in these positions
	sharedSegments, err := findSharedSegmentsChecked(g.Edges, guard.step)
	if err != nil {
		return err
	}
	sharedSegmentFakeNodes := make([]*layoutgraph.Node, 0, len(sharedSegments))
	for i, seg := range sharedSegments {
		if err := checkCanceled(); err != nil {
			return err
		}
		tl := seg.Start.Copy()
		width := seg.End.X - seg.Start.X
		height := seg.End.Y - seg.Start.Y
		// generate a box around the shared segments
		// │  │  │
		// shared│
		// segment
		// │  │  │
		// │  │  │
		// │  │  │
		// │  │  │
		// └──┴──┘
		// |-----| fake box
		// |--| SharedSegmentClearance
		if seg.End.X == seg.Start.X {
			// vertical
			width = 2 * SharedSegmentClearance
			tl.X -= SharedSegmentClearance
		} else {
			// horizontal
			height = 2 * SharedSegmentClearance
			tl.Y -= SharedSegmentClearance
		}
		fakeSharedNode := &layoutgraph.Node{
			Box: geo.Box{
				TopLeft: tl,
				Width:   width,
				Height:  height,
			},
			D2ID:  new(fmt.Sprintf("fake_shared_segment_%d", i)),
			Graph: g,
		}
		fakeSharedNode.SetShape(shape.SQUARE_TYPE)
		sharedSegmentFakeNodes = append(sharedSegmentFakeNodes, fakeSharedNode)
	}

	sortedEdges := make([]*layoutgraph.Edge, 0, len(edges))
	for _, e := range edges {
		if err := checkCanceled(); err != nil {
			return err
		}
		if e.Label == nil || e.IsLoop() || e.Label.PositionFixed() {
			continue
		}
		sortedEdges = append(sortedEdges, e)
	}

	sortedEdges, err = sortLabelPlacementEdges(sortedEdges, guard)
	if err != nil {
		return err
	}

	for _, edge := range sortedEdges {
		if err := checkCanceled(); err != nil {
			return err
		}
		fakeNode, position, percentage, err := findBestEdgeLabelPosition(edge, g, placedFakeNodes, sharedSegmentFakeNodes, guard)
		if err != nil {
			return err
		}
		placedFakeNodes = append(placedFakeNodes, fakeNode)
		edge.Label.Position = position
		edge.LabelPercentage = percentage
	}
	return guard.check()
}

func findBestEdgeLabelPosition(edge *layoutgraph.Edge, g *layoutgraph.Graph, placedFakeNodes, sharedSegmentFakeNodes []*layoutgraph.Node, guard *labelPlacementWorkGuard) (*layoutgraph.Node, label.Position, float64, error) {
	// we don't want to count the edge itself as overlapping, we already prefer outside labels
	otherEdges := make([]*layoutgraph.Edge, 0, len(g.Edges))
	for _, otherEdge := range g.Edges {
		if err := guard.step(); err != nil {
			return nil, label.Unset, 0, err
		}
		if otherEdge == edge {
			continue
		}
		otherEdges = append(otherEdges, otherEdge)
	}

	positions := make([]label.Position, 0, len(edgeLabelPreferenceOrder)+3)
	if edge.Label.Position.IsUnlocked() {
		// prefer the unlocked positions before trying other positions
		if edge.Label.Position == label.UnlockedMiddle {
			positions = append(positions, label.UnlockedMiddle)
			positions = append(positions, label.UnlockedTop)
			positions = append(positions, label.UnlockedBottom)
		} else {
			positions = append(positions, edge.Label.Position)
			positions = append(positions, edge.Label.Position.Mirrored())
			positions = append(positions, label.UnlockedMiddle)
		}
		positions = append(positions, edgeLabelPreferenceOrder...)
	} else if edge.IsClusterEdge() {
		pathShared, err := isClusterPathSharedPlacement(edge, guard)
		if err != nil {
			return nil, label.Unset, 0, err
		}
		if !pathShared {
			positions = append(positions, edgeLabelPreferenceOrder...)
			positions = append(positions, label.UnlockedTop)
			positions = append(positions, label.UnlockedBottom)
			positions = append(positions, label.UnlockedMiddle)
		} else {
			// Give preference for symmetrical placements for clustered nodes
			// .         label     ┌──────────┐
			// .        ┌──────────┤          │
			// .        │          │          │
			// .        │          └──────────┘
			// . ───────┤
			// .        │          ┌──────────┐
			// .        │          │          │
			// .        └──────────┤          │
			// .          label    └──────────┘
			if edge.From.Cluster != nil {
				orientation, clusterIndex, err := clusterLabelPlacementOrientation(edge.To, edge.From, edge.From.Cluster, guard)
				if err != nil {
					return nil, label.Unset, 0, err
				}
				switch orientation {
				case geo.Right, geo.Top:
					if float64(clusterIndex) < float64(len(edge.From.Cluster.Nodes)-1)/2. {
						positions = append(positions, label.UnlockedTop)
					} else if float64(clusterIndex) > float64(len(edge.From.Cluster.Nodes)-1)/2. {
						positions = append(positions, label.UnlockedBottom)
					}
				case geo.Left, geo.Bottom:
					if float64(clusterIndex) < float64(len(edge.From.Cluster.Nodes)-1)/2. {
						positions = append(positions, label.UnlockedBottom)
					} else if float64(clusterIndex) > float64(len(edge.From.Cluster.Nodes)-1)/2. {
						positions = append(positions, label.UnlockedTop)
					}
				}
			} else {
				orientation, clusterIndex, err := clusterLabelPlacementOrientation(edge.From, edge.To, edge.To.Cluster, guard)
				if err != nil {
					return nil, label.Unset, 0, err
				}
				switch orientation {
				case geo.Left, geo.Bottom:
					if float64(clusterIndex) < float64(len(edge.To.Cluster.Nodes)-1)/2. {
						positions = append(positions, label.UnlockedTop)
					} else if float64(clusterIndex) > float64(len(edge.To.Cluster.Nodes)-1)/2. {
						positions = append(positions, label.UnlockedBottom)
					}
				case geo.Right, geo.Top:
					if float64(clusterIndex) < float64(len(edge.To.Cluster.Nodes)-1)/2. {
						positions = append(positions, label.UnlockedBottom)
					} else if float64(clusterIndex) > float64(len(edge.To.Cluster.Nodes)-1)/2. {
						positions = append(positions, label.UnlockedTop)
					}
				}
			}
			positions = append(positions, label.UnlockedMiddle)
			positions = append(positions, edgeLabelPreferenceOrder...)
		}
	} else {
		positions = append(positions, edgeLabelPreferenceOrder...)
		positions = append(positions, label.UnlockedTop)
		positions = append(positions, label.UnlockedBottom)
		positions = append(positions, label.UnlockedMiddle)
	}

	var ancestors, nonAncestors layoutgraph.Nodes
	for _, n := range g.Nodes {
		if err := guard.step(); err != nil {
			return nil, label.Unset, 0, err
		}
		fromDescendant, err := isLabelPlacementDescendantOf(edge.From, n, guard)
		if err != nil {
			return nil, label.Unset, 0, err
		}
		toDescendant, err := isLabelPlacementDescendantOf(edge.To, n, guard)
		if err != nil {
			return nil, label.Unset, 0, err
		}
		if n != edge.From && n != edge.To && (fromDescendant || toDescendant) {
			ancestors = append(ancestors, n)
		} else {
			nonAncestors = append(nonAncestors, n)
		}
	}

	var bestLabelPosition label.Position
	var bestLabelPercentage float64
	var bestLabelFakeNode *layoutgraph.Node
	bestScore := math.Inf(1)

	checkPosition := func(labelPosition label.Position) error {
		if err := guard.step(); err != nil {
			return err
		}
		topLeft, err := edgeLabelTopLeft(edge, labelPosition, edge.Label.Width, edge.Label.Height, guard)
		if err != nil {
			return err
		}
		fakeLabelNode := &layoutgraph.Node{
			Box: geo.Box{
				TopLeft: topLeft,
				Width:   edge.Label.Width,
				Height:  edge.Label.Height,
			},
			D2ID:  new(edge.Label.Text),
			Graph: g,
		}
		fakeLabelNode.SetShape(shape.SQUARE_TYPE)

		edges := layoutgraph.Edges(g.Edges)
		if labelPosition.IsOnEdge() {
			edges = layoutgraph.Edges(otherEdges)
		}

		// Especially don't want labels of a cluster edge path intersecting other cluster edge paths
		// So count the other cluster edges as double
		if edge.From.Cluster != nil {
			edges = append(layoutgraph.Edges(nil), edges...)
			for _, otherEdge := range otherEdges {
				if err := guard.step(); err != nil {
					return err
				}
				if otherEdge.From.Cluster == edge.From.Cluster {
					edges = append(edges, otherEdge)
				}
			}
		} else if edge.To.Cluster != nil {
			edges = append(layoutgraph.Edges(nil), edges...)
			for _, otherEdge := range otherEdges {
				if err := guard.step(); err != nil {
					return err
				}
				if otherEdge.To.Cluster == edge.To.Cluster {
					edges = append(edges, otherEdge)
				}
			}
		}

		// it only counts as overlapping an ancestor if it is partially overlapping
		ancestorPartialOverlapArea, ancestorCount, err := nodeOverlapArea(fakeLabelNode, ancestors, label.PADDING, true, guard)
		if err != nil {
			return err
		}
		nonAncestorOverlapArea, nonAncestorCount, err := nodeOverlapArea(fakeLabelNode, nonAncestors, label.PADDING, false, guard)
		if err != nil {
			return err
		}
		edgePaddingOverlaps, err := edgeOverlapCount(fakeLabelNode, edges, label.PADDING, guard)
		if err != nil {
			return err
		}
		edgeExactOverlaps, err := edgeOverlapCount(fakeLabelNode, edges, 0, guard)
		if err != nil {
			return err
		}
		edgeOverlaps := edgePaddingOverlaps + int(math.Ceil(float64(edgeExactOverlaps)*0.5))
		almostLabelPaddingOverlaps, err := nodeOverlapCount(fakeLabelNode, placedFakeNodes, label.PADDING, guard)
		if err != nil {
			return err
		}
		almostLabelExactOverlaps, err := nodeOverlapCount(fakeLabelNode, placedFakeNodes, 0, guard)
		if err != nil {
			return err
		}
		almostLabelOverlaps := almostLabelPaddingOverlaps + int(math.Ceil(float64(almostLabelExactOverlaps)*0.5))
		labelOverlaps, err := nodeOverlapCount(fakeLabelNode, placedFakeNodes, 0, guard)
		if err != nil {
			return err
		}
		sharedSegmentOverlap, err := nodeOverlapCount(fakeLabelNode, sharedSegmentFakeNodes, label.PADDING, guard)
		if err != nil {
			return err
		}
		score := scoreEdgeLabelOverlaps(
			fakeLabelNode.Area(),
			ancestorPartialOverlapArea+nonAncestorOverlapArea,
			ancestorCount+nonAncestorCount,
			edgeOverlaps,
			almostLabelOverlaps,
			labelOverlaps,
			sharedSegmentOverlap,
		)

		if score < bestScore {
			bestLabelPosition = labelPosition
			bestLabelFakeNode = fakeLabelNode
			bestLabelPercentage = edge.LabelPercentage
			bestScore = score
		}
		return nil
	}

	routeLength := 0.0
	routeLengthCalculated := false
	// pick the label position based on how many nodes, edges and already placed labels overlap
	for _, labelPosition := range positions {
		if err := guard.step(); err != nil {
			return nil, label.Unset, 0, err
		}
		if bestScore == 0 {
			break
		}
		if labelPosition.IsUnlocked() {
			if edge.LabelPercentage != 0 {
				// if label percentage was set on the edge during layout, use it
				if err := checkPosition(labelPosition); err != nil {
					return nil, label.Unset, 0, err
				}
			} else {
				if !routeLengthCalculated {
					var err error
					routeLength, err = routeLengthPlacement(edge, guard)
					if err != nil {
						return nil, label.Unset, 0, err
					}
					routeLengthCalculated = true
				}
				r := labelPercentageSearchRange(edge, routeLength)
				for i := r.start; i < r.end; i += placementSearchInterval {
					if err := guard.step(); err != nil {
						return nil, label.Unset, 0, err
					}
					edge.LabelPercentage = i
					if err := checkPosition(labelPosition); err != nil {
						return nil, label.Unset, 0, err
					}
					if bestScore == 0 {
						break
					}
				}
				edge.LabelPercentage = 0
			}
		} else {
			if err := checkPosition(labelPosition); err != nil {
				return nil, label.Unset, 0, err
			}
		}
	}

	return bestLabelFakeNode, bestLabelPosition, bestLabelPercentage, guard.check()
}

func labelPercentageSearchRange(edge *layoutgraph.Edge, length float64) searchRange {
	halfHeight := edge.Label.Height / 2.
	halfWidth := edge.Label.Width / 2.
	if edge.From.Cluster != nil {
		// if the edge is coming from a cluster node
		// use only the first segment extension to find the placement
		// .          range.end    range.start
		// .               ├──────────┤
		// .                          ┌──────────┐
		// .               ┌──────────┤          │
		// .               │          │          │
		// . ┌─────┐       │          └──────────┘
		// . │     ◄───────┤
		// . │     │       │          ┌──────────┐
		// . └─────┘       │          │          │
		// .               └──────────┤          │
		// .                          └──────────┘
		p1 := edge.Points[0]
		p2 := edge.Points[1]
		if p1.X == p2.X && math.Abs(p1.Y-p2.Y) > halfHeight {
			end := (math.Abs(p1.Y-p2.Y) - halfHeight) / length
			return searchRange{start: 0, end: math.Min(1, end)}
		} else if math.Abs(p1.X-p2.X) > halfWidth {
			end := (math.Abs(p1.X-p2.X) - halfWidth) / length
			return searchRange{start: 0, end: math.Min(1, end)}
		}
	} else if edge.To.Cluster != nil {
		// if the edge is going to a cluster node
		// use only the last segment extension to find the placement
		// .        range.start    range.end
		// .               ├──────────┤
		// .                          ┌──────────┐
		// .               ┌──────────►          │
		// .               │          │          │
		// . ┌─────┐       │          └──────────┘
		// . │     ├───────┤
		// . │     │       │          ┌──────────┐
		// . └─────┘       │          │          │
		// .               └──────────►          │
		// .                          └──────────┘
		p1 := edge.Points[len(edge.Points)-1]
		p2 := edge.Points[len(edge.Points)-2]
		if p1.X == p2.X && math.Abs(p1.Y-p2.Y) > halfHeight {
			start := (length - math.Abs(p1.Y-p2.Y) + halfHeight) / length
			return searchRange{start: math.Max(0, start), end: 1}
		} else if math.Abs(p1.X-p2.X) > halfWidth {
			start := (length - math.Abs(p1.X-p2.X) + halfWidth) / length
			return searchRange{start: math.Max(0, start), end: 1}
		}
	}
	return searchRange{start: 0, end: 1}
}

// findSharedSegmentsChecked finds the longest shared vertical/horizontal segments
// Below is an example for horizontal segments, but the same applies for vertical ones
// Note that the segments are shown in different rows to make it easier to understand (in reality, they must be on the same Y)
// Input segments:
// .                                         ───────────
// .      ───────                      ─────────
// . ──────       ────────  ──────   ─────
// . Result:
// .      ─                            ─────────
// just the small             shared segment among the 3 segments above
// shared segment
func findSharedSegmentsChecked(edges []*layoutgraph.Edge, checkCanceled func() error) ([]*geo.Segment, error) {
	if checkCanceled != nil {
		if err := checkCanceled(); err != nil {
			return nil, err
		}
	}
	verticalSegments := make(map[float64][]*geo.Segment)
	horizontalSegments := make(map[float64][]*geo.Segment)

	// group vertical/horizontal segments by Y/X
	for _, edge := range edges {
		if checkCanceled != nil {
			if err := checkCanceled(); err != nil {
				return nil, err
			}
		}
		for i := 0; i < len(edge.Points)-1; i++ {
			if checkCanceled != nil {
				if err := checkCanceled(); err != nil {
					return nil, err
				}
			}
			segment := geo.NewSegment(edge.Points[i].Copy(), edge.Points[i+1].Copy())
			if segment.Start.X == segment.End.X && segment.Start.Y == segment.End.Y {
				continue
			}
			if segment.Start.X == segment.End.X {
				if segment.End.Y < segment.Start.Y {
					// swap so segments are always top to bottom
					segment.End.Y, segment.Start.Y = segment.Start.Y, segment.End.Y
				}
				verticalSegments[segment.Start.X] = append(verticalSegments[segment.Start.X], segment)
			} else if segment.Start.Y == segment.End.Y {
				if segment.End.X < segment.Start.X {
					// swap so segments are always left to right
					segment.End.X, segment.Start.X = segment.Start.X, segment.End.X
				}
				horizontalSegments[segment.Start.Y] = append(horizontalSegments[segment.Start.Y], segment)
			}
		}
	}

	sharedSegments := make([]*geo.Segment, 0, max(len(verticalSegments), len(horizontalSegments)))
	addSharedSegments := func(segmentsByCoord map[float64][]*geo.Segment, isHorizontalSegment bool) error {
		coordinate := func(p *geo.Point) float64 {
			if isHorizontalSegment {
				return p.X
			}
			return p.Y
		}
		setCoordinate := func(p *geo.Point, v float64) {
			if isHorizontalSegment {
				p.X = v
			} else {
				p.Y = v
			}
		}

		for _, segments := range segmentsByCoord {
			if checkCanceled != nil {
				if err := checkCanceled(); err != nil {
					return err
				}
			}
			if len(segments) == 1 {
				continue
			}
			// Sort segments from starting positions (assumes segments are top to
			// bottom and left to right). The merge sort can stop between every
			// comparison; sort.Slice cannot propagate cancellation.
			if err := sortSharedSegmentGroup(segments, coordinate, checkCanceled); err != nil {
				return err
			}

			prev := segments[0]
			var shared *geo.Segment
			for i := 1; i < len(segments); i++ {
				if checkCanceled != nil {
					if err := checkCanceled(); err != nil {
						return err
					}
				}
				curr := segments[i]
				// segments overlap
				if coordinate(prev.End) > coordinate(curr.Start) {
					// A long trunk can contain disjoint shared intervals.
					// Keep its exclusive label corridor between them available.
					if shared != nil && coordinate(curr.Start) > coordinate(shared.End) {
						sharedSegments = append(sharedSegments, shared)
						shared = nil
					}
					if shared == nil {
						shared = geo.NewSegment(
							curr.Start.Copy(),
							curr.Start.Copy(),
						)
						// update the appropriate coordinate
						// curr  :   ─────────
						// prev  : ─────
						// shared:   ───
						setCoordinate(shared.End, math.Min(coordinate(prev.End), coordinate(curr.End)))
					} else {
						// extend the current ongoing overlap
						// shared before: ───
						// prev         : ─────────
						// curr         :         ───────────
						// shared now   : ─────────
						minOverlap := math.Min(coordinate(prev.End), coordinate(curr.End))
						setCoordinate(shared.End, math.Max(coordinate(shared.End), minOverlap))
					}
				} else if shared != nil {
					// there's no need to extend the segment, so close it
					sharedSegments = append(sharedSegments, shared)
					shared = nil
				}
				// keep prev as the longest segment
				if coordinate(curr.End) > coordinate(prev.End) {
					prev = curr
				}
			}
			if shared != nil {
				// just in case the last segment had an overlap
				sharedSegments = append(sharedSegments, shared)
			}
		}
		return nil
	}

	if err := addSharedSegments(verticalSegments, false); err != nil {
		return nil, err
	}
	if err := addSharedSegments(horizontalSegments, true); err != nil {
		return nil, err
	}

	return sharedSegments, nil
}

func scoreEdgeLabelOverlaps(
	labelArea, nodeOverlapArea float64,
	nodeOverlapCount, edgeOverlapCount,
	almostLabelOverlapCount, labelOverlapCount, sharedSegmentOverlapCount int,
) float64 {
	score := (nodeOverlapArea / labelArea) * 2.
	// avoid as much as possible overlapping with other labels
	score += float64(labelOverlapCount) * 10
	score += float64(almostLabelOverlapCount)
	// overlapping with other edges is worse than placing the label on a shared segment
	score += float64(nodeOverlapCount) * 2
	score += float64(edgeOverlapCount) * 2
	score += float64(sharedSegmentOverlapCount)
	return score
}

// scoreNodeLabelOverlaps ranks a node-label candidate. A label-label overlap
// is deliberately twice as costly as a node or edge overlap.
func scoreNodeLabelOverlaps(nodeOverlapCount, edgeOverlapCount, labelOverlapCount int) float64 {
	return float64(nodeOverlapCount + edgeOverlapCount + 2*labelOverlapCount)
}

func isClusterPathSharedPlacement(e *layoutgraph.Edge, guard *labelPlacementWorkGuard) (bool, error) {
	return isClusterPathSharedChecked(e, guard.step)
}

// isClusterPathSharedChecked checks if a given cluster edge has a shared path with the other cluster edges.
func isClusterPathSharedChecked(e *layoutgraph.Edge, checkWork func() error) (bool, error) {
	if checkWork != nil {
		if err := checkWork(); err != nil {
			return false, err
		}
	}
	var clusterNodes []*layoutgraph.Node
	var adjacentNode *layoutgraph.Node
	fromCluster := false
	if e.From.Cluster != nil {
		clusterNodes = e.From.Cluster.Nodes
		adjacentNode = e.To
		fromCluster = true
	} else if e.To.Cluster != nil {
		clusterNodes = e.To.Cluster.Nodes
		adjacentNode = e.From
	} else {
		return false, nil
	}

	var p1, p2 *geo.Point
	if fromCluster {
		// if the edge is coming from a cluster, only the last segment can be shared
		p1 = e.Points[len(e.Points)-1]
		p2 = e.Points[len(e.Points)-2]
	} else {
		// if the edge is going to a cluster, only the first segment can be shared
		p1 = e.Points[0]
		p2 = e.Points[1]
	}
	for _, n := range clusterNodes {
		if checkWork != nil {
			if err := checkWork(); err != nil {
				return false, err
			}
		}
		for _, ce := range n.Edges {
			if checkWork != nil {
				if err := checkWork(); err != nil {
					return false, err
				}
			}
			if ce == e {
				continue
			}
			if n.Adjacent(ce) != adjacentNode {
				// only checks edges to the same connected node
				continue
			}
			var op1, op2 *geo.Point
			if fromCluster {
				op1 = ce.Points[len(ce.Points)-1]
				op2 = ce.Points[len(ce.Points)-2]
			} else {
				op1 = ce.Points[0]
				op2 = ce.Points[1]
			}
			if p1.X == p2.X && op1.X == op2.X && p1.X == op1.X {
				// both vertical and at same X
				if math.Max(p1.Y, p2.Y) >= math.Min(op1.Y, op2.Y) &&
					math.Max(op1.Y, op2.Y) >= math.Min(p1.Y, p2.Y) {
					// The closed intervals overlap.
					return true, nil
				}
			} else if p1.Y == p2.Y && op1.Y == op2.Y && p1.Y == op1.Y {
				// both horizontal and at same Y
				if math.Max(p1.X, p2.X) >= math.Min(op1.X, op2.X) &&
					math.Max(op1.X, op2.X) >= math.Min(p1.X, p2.X) {
					// The closed intervals overlap.
					return true, nil
				}
			}
		}
	}

	return false, nil
}
