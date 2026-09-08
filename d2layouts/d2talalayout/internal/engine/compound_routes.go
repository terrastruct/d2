package engine

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labelgeom"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placement"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

// PreserveCompoundRoutes refines an already-selected compound drawing. Keep
// placement selection independent of this routing cleanup: otherwise a cheaper
// reroute can promote an unrelated, much wider outer arrangement.
func PreserveCompoundRoutes(ctx context.Context, before, selected *layoutgraph.Graph) (*layoutgraph.Graph, error) {
	candidate, err := layoutgraph.Clone(ctx, selected)
	if err != nil {
		return nil, err
	}
	// The selected proposal already has complete external routes. Preserve their
	// curve state too; Clone resets it for ordinary full-layout attempts.
	for index, edge := range candidate.Edges {
		edge.IsCurve = selected.Edges[index].IsCurve
	}
	if err := preserveCompoundInteriors(ctx, before, candidate); err != nil {
		return nil, err
	}
	return candidate, nil
}

// A compound trial moves each outer block rigidly. Its enclosed routes and
// labels belong to that block too: redrawing them can break a shared fan even
// though none of its endpoints or obstacles moved relative to one another.
func preserveCompoundInteriors(ctx context.Context, before, after *layoutgraph.Graph) error {
	guard, err := limits.NewWorkGuard(ctx, "CompoundRoutes", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	// Clone resets placement bookkeeping for a new full layout. This trial
	// instead retains the authored label and icon constraints of placed nodes.
	for index, node := range after.Nodes {
		original := before.Nodes[index]
		if original.Label != nil && original.Label.PositionFixed() {
			copied := *original.Label
			node.Label = &copied
		}
		if original.Icon != nil && original.Icon.PositionFixed() {
			copied := *original.Icon
			node.Icon = &copied
		}
	}
	// Clone and PlaceCompound preserve edge order. IDs may be unset in graphs
	// constructed directly by engine callers, so they cannot identify this pair.
	for index, edge := range after.Edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		original := before.Edges[index]
		if original.Label != nil && original.Label.PositionFixed() {
			copied := *original.Label
			edge.Label = &copied
			edge.LabelPercentage = original.LabelPercentage
		}
		enclosed, err := compoundEnclosedRoute(original, guard)
		if err != nil {
			return err
		}
		if enclosed {
			oldRoot, newRoot := compoundRoot(original.From), compoundRoot(edge.From)
			dx, dy := newRoot.TopLeft.X-oldRoot.TopLeft.X, newRoot.TopLeft.Y-oldRoot.TopLeft.Y
			edge.Points = make([]*geo.Point, len(original.Points))
			for i, point := range original.Points {
				if err := guard.Step(); err != nil {
					return err
				}
				edge.Points[i] = geo.NewPoint(point.X+dx, point.Y+dy)
			}
			edge.IsCurve = original.IsCurve
			edge.LabelPercentage = original.LabelPercentage
			if original.Label != nil {
				copied := *original.Label
				edge.Label = &copied
			}
			continue
		}
	}
	after.ResetPlacementCosts()
	// The selected external routes can use a previously unused side of an interior node.
	// Reconsider automatic labels and icons against those new attachments.
	if err := labeling.Place(ctx, after); err != nil {
		return err
	}
	placement.Normalize(after)
	return guard.Finish()
}

func compoundRoot(node *layoutgraph.Node) *layoutgraph.Node {
	for node.Container != nil {
		node = node.Container
	}
	return node
}

func compoundEnclosedRoute(edge *layoutgraph.Edge, guard *limits.WorkGuard) (bool, error) {
	root := compoundRoot(edge.From)
	if !root.IsContainer() || root != compoundRoot(edge.To) || len(edge.Points) < 2 {
		return false, nil
	}
	inside := func(point *geo.Point, width, height float64) bool {
		return point != nil && !math.IsNaN(point.X) && !math.IsNaN(point.Y) &&
			point.X >= root.TopLeft.X && point.Y >= root.TopLeft.Y &&
			point.X+width <= root.TopLeft.X+root.Width && point.Y+height <= root.TopLeft.Y+root.Height
	}
	for _, point := range edge.Points {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if !inside(point, 0, 0) {
			return false, nil
		}
	}
	// Conservatively reuse only endpoints on box borders. Shape-specific ports
	// recessed inside an oval or other outline can be rebuilt by the router.
	attached := func(node *layoutgraph.Node, point *geo.Point) bool {
		return node.ContainsPointOnBox(point) &&
			(point.X == node.TopLeft.X || point.X == node.TopLeft.X+node.Width ||
				point.Y == node.TopLeft.Y || point.Y == node.TopLeft.Y+node.Height)
	}
	if !attached(edge.From, edge.Points[0]) || !attached(edge.To, edge.Points[len(edge.Points)-1]) {
		return false, nil
	}
	if edge.Label != nil {
		if edge.Label.Position == label.Unset {
			return false, nil
		}
		if err := guard.Add(int64(len(edge.Points))); err != nil {
			return false, err
		}
		if !inside(edge.LabelTopLeft(edge.Label.Position, edge.Label.Width, edge.Label.Height), edge.Label.Width, edge.Label.Height) {
			return false, nil
		}
	}
	for i, arrowLabel := range []*layoutgraph.Label{edge.SourceArrowheadLabel, edge.TargetArrowheadLabel} {
		if arrowLabel == nil {
			continue
		}
		if err := guard.Add(int64(len(edge.Points))); err != nil {
			return false, err
		}
		point := labelgeom.ArrowheadTopLeft(edge.Points, i == 1, string(edge.SourceArrowhead), string(edge.TargetArrowhead), arrowLabel.Width, arrowLabel.Height)
		if !inside(point, arrowLabel.Width, arrowLabel.Height) {
			return false, nil
		}
	}
	return true, nil
}
