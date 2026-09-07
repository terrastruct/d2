package quality

import (
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

type positionedLabel struct {
	box  geo.Box
	node *layoutgraph.Node
	edge *layoutgraph.Edge
}

// measureLabels treats all rendered label and icon boxes consistently, including
// loop and arrowhead labels. Padding-only near misses do not trigger repair.
func measureLabels(g *layoutgraph.Graph, score *Metrics, guard *limits.WorkGuard) error {
	if err := guard.Step(); err != nil {
		return err
	}
	var labels []positionedLabel
	appendLabel := func(box geo.Box, node *layoutgraph.Node, edge *layoutgraph.Edge) {
		if box.Width > 0 && box.Height > 0 {
			labels = append(labels, positionedLabel{box, node, edge})
		}
	}
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if node.IsInvisible {
			continue
		}
		if node.Label != nil && node.Label.Position != label.Unset {
			if node.Label.Position.IsOutside() {
				if err := chargeEvaluationWork(guard, 4*len(g.Nodes)); err != nil {
					return err
				}
			}
			appendLabel(geo.Box{TopLeft: node.LabelTopLeft(node.Label.Position, node.Label.Width, node.Label.Height), Width: node.Label.Width, Height: node.Label.Height}, node, nil)
		}
		if node.Icon != nil && !node.IsImage() {
			if node.Icon.Position.IsOutside() {
				if err := chargeEvaluationWork(guard, 4*len(g.Nodes)); err != nil {
					return err
				}
			}
			size := node.IconSize(node.Icon.Position)
			appendLabel(geo.Box{TopLeft: node.LabelTopLeft(node.Icon.Position, size, size), Width: size, Height: size}, node, nil)
		}
	}
	visibleEdges := make([]*layoutgraph.Edge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return err
		}
		if edge.IsInvisible {
			continue
		}
		visibleEdges = append(visibleEdges, edge)
		if len(edge.Points) < 2 {
			continue
		}
		if edge.Label != nil && edge.Label.Position != label.Unset {
			if err := chargeEvaluationWork(guard, len(edge.Points)); err != nil {
				return err
			}
			appendLabel(labelBox(edge), nil, edge)
		}
		if edge.SourceArrowheadLabel != nil {
			if err := chargeEvaluationWork(guard, len(edge.Points)); err != nil {
				return err
			}
			appendLabel(labeling.PositionArrowheadLabel(edge, false, edge.Points).Box, nil, edge)
		}
		if edge.TargetArrowheadLabel != nil {
			if err := chargeEvaluationWork(guard, len(edge.Points)); err != nil {
				return err
			}
			appendLabel(labeling.PositionArrowheadLabel(edge, true, edge.Points).Box, nil, edge)
		}
	}
	for i, placed := range labels {
		if err := guard.Step(); err != nil {
			return err
		}
		for _, other := range labels[:i] {
			if err := guard.Step(); err != nil {
				return err
			}
			if boxesOverlapWithPadding(placed.box, other.box, 0) {
				score.TextOcclusions++
			}
		}
		for _, node := range g.Nodes {
			if err := guard.Step(); err != nil {
				return err
			}
			if node.IsInvisible || !node.IsRectangular() {
				continue
			}
			if !boxesOverlapWithPadding(placed.box, node.Box, 0) {
				continue
			}
			ancestor := false
			var err error
			if placed.node != nil {
				ancestor, err = evaluationIsDescendantOf(placed.node, node, guard)
			} else {
				ancestor, err = endpointAncestor(placed.edge, node, guard)
				// Labels may be inside enclosing containers, but must remain
				// outside the shapes to which their edge connects.
				ancestor = ancestor && node != placed.edge.From && node != placed.edge.To
			}
			if err != nil {
				return err
			}
			if ancestor && boxCovers(node.Box, placed.box) {
				continue
			}
			score.TextOcclusions++
		}
		for _, edge := range visibleEdges {
			if err := guard.Step(); err != nil {
				return err
			}
			if edge == placed.edge || edge.IsCurve {
				continue
			}
			overlap := false
			for j := 1; j < len(edge.Points); j++ {
				if err := guard.Step(); err != nil {
					return err
				}
				if segmentEntersBox(placed.box, edge.Points[j-1], edge.Points[j]) {
					overlap = true
					break
				}
			}
			if overlap {
				score.TextOcclusions++
			}
		}

	}
	return nil
}
