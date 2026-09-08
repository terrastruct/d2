// Package labeling places node, icon, edge, and arrowhead labels on a routed
// layout graph. It owns placement policy and search; layoutgraph owns the
// mutable graph records and geometry primitives.
package labeling

import (
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/nodeshape"
)

// searchRange is the fraction of an edge route searched for an unlocked
// label. It is deliberately local to label placement rather than sharing the
// routing package's tunnel range type.
type searchRange struct {
	start float64
	end   float64
}

// The order of preferred edge-label positions.
var edgeLabelPreferenceOrder = []label.Position{
	label.OutsideTopCenter,
	label.OutsideBottomCenter,
	label.OutsideTopLeft,
	label.OutsideTopRight,
	label.OutsideBottomLeft,
	label.OutsideBottomRight,
	label.InsideMiddleCenter,
	label.InsideMiddleLeft,
	label.InsideMiddleRight,
}

var nodeLabelPositionOrder = []label.Position{
	label.InsideMiddleCenter,
	label.InsideTopCenter,
	label.InsideBottomCenter,
	label.InsideMiddleLeft,
	label.InsideMiddleRight,
	label.InsideTopLeft,
	label.InsideTopRight,
	label.InsideBottomLeft,
	label.InsideBottomRight,
	label.OutsideTopCenter,
	label.OutsideBottomCenter,
	label.OutsideLeftMiddle,
	label.OutsideRightMiddle,
	label.OutsideTopLeft,
	label.OutsideTopRight,
	label.OutsideBottomLeft,
	label.OutsideBottomRight,
	label.OutsideLeftTop,
	label.OutsideLeftBottom,
	label.OutsideRightTop,
	label.OutsideRightBottom,
}

var containerLabelPositionOrder = []label.Position{
	label.InsideTopCenter,
	label.OutsideTopCenter,
	label.InsideBottomCenter,
	label.OutsideBottomCenter,
	label.OutsideTopLeft,
	label.OutsideTopRight,
	label.OutsideBottomLeft,
	label.OutsideBottomRight,
	label.OutsideLeftMiddle,
	label.OutsideRightMiddle,
	label.OutsideLeftTop,
	label.OutsideLeftBottom,
	label.OutsideRightTop,
	label.OutsideRightBottom,
	label.InsideTopLeft,
	label.InsideTopRight,
	label.InsideBottomLeft,
	label.InsideBottomRight,
	label.InsideMiddleLeft,
	label.InsideMiddleRight,
	label.InsideMiddleCenter,
}

// Initialize reserves each explicit/default node-label and icon position
// before placement begins.
func Initialize(graph *layoutgraph.Graph) {
	for _, node := range graph.Nodes {
		setDefaultLabelPlacement(node)
		if node.Icon != nil && node.Icon.Position != label.Unset {
			node.Icon.FixPosition()
		}
	}
}

func setDefaultLabelPlacement(node *layoutgraph.Node) {
	if node.Label == nil {
		return
	}
	if node.Label.Position == label.Unset {
		node.Label.Position = labelPositionPreferences(node)[0]
	} else {
		node.Label.FixPosition()
	}
}

func compareLabelPositions(node *layoutgraph.Node, first, second label.Position) int {
	score := func(position label.Position) int {
		score := -1
		for index, tier := range []nodeshape.LabelTier{nodeshape.Good, nodeshape.OK, nodeshape.Unideal, nodeshape.Bad} {
			if _, found := node.Shape.LabelPositionPreferences(tier)[position]; found {
				score = index + 1
			}
		}
		return score
	}
	firstScore, secondScore := score(first), score(second)
	if firstScore < secondScore {
		return 1
	}
	if firstScore > secondScore {
		return -1
	}
	return 0
}

func labelPositionPreferences(node *layoutgraph.Node) []label.Position {
	var preferences []label.Position
	for _, tranche := range labelPositionPreferenceTranches(node) {
		preferences = append(preferences, tranche...)
	}
	return preferences
}

func labelPositionPreferenceTranches(node *layoutgraph.Node) [][]label.Position {
	baseOrder := nodeLabelPositionOrder
	if node.IsContainer() {
		baseOrder = containerLabelPositionOrder
	}
	tranches := make([][]label.Position, 0, 4)
	for _, tier := range []nodeshape.LabelTier{nodeshape.Good, nodeshape.OK, nodeshape.Unideal, nodeshape.Bad} {
		allowed := node.Shape.LabelPositionPreferences(tier)
		var tranche []label.Position
		for _, position := range baseOrder {
			if _, found := allowed[position]; found {
				tranche = append(tranche, position)
			}
		}
		tranches = append(tranches, tranche)
	}
	return tranches
}
