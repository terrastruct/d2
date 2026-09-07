package d2isometricimg

import (
	"math"
	"sort"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

// hierarchyBoardHeaderSurface places the existing print area at the compiled
// container label position. Outside and border labels are brought onto the
// physical surface; the container and its descendants are never resized.
func hierarchyBoardHeaderSurface(surface labelSurface, board d2isometric.Board, owner d2isometric.Node, nodes []d2isometric.Node, pixelScale float64, boards ...[]d2isometric.Board) labelSurface {
	placed, _ := hierarchyBoardHeaderPlacement(surface, board, owner, nodes, pixelScale, boards...)
	return placed
}

// The bool reports whether the full print area has an unoccluded placement.
// Prefer the original source edge, then the bottom edge and a bounded set of
// alternate strips. All candidates retain the dimensions, angle, and plane.
// If every candidate is blocked, return the original in-footprint anchor.
func hierarchyBoardHeaderPlacement(surface labelSurface, board d2isometric.Board, owner d2isometric.Node, nodes []d2isometric.Node, pixelScale float64, boards ...[]d2isometric.Board) (labelSurface, bool) {
	if !hierarchyHeaderPositive(board.Size.X) || !hierarchyHeaderPositive(board.Size.Z) || !hierarchyHeaderPositive(surface.width) || !hierarchyHeaderPositive(surface.depth) {
		return surface, false
	}
	if !hierarchyHeaderPositive(pixelScale) {
		pixelScale = d2isometric.SceneScale
	}
	padX, padZ := min(.05, board.Size.X*.02), min(.05, board.Size.Z*.02)
	availableX, availableZ := board.Size.X-2*padX, board.Size.Z-2*padZ
	fit := min(1, min(availableX/surface.width, availableZ/surface.depth))
	surface.width *= fit
	surface.depth *= fit
	position := label.FromString(owner.Metadata.Original.LabelPosition)
	if position == label.Unset {
		position = label.InsideTopCenter
	}
	box := geo.NewBox(geo.NewPoint(0, 0), board.Size.X/pixelScale, board.Size.Z/pixelScale)
	tl := position.GetPointOnBox(box, label.PADDING, surface.width/pixelScale, surface.depth/pixelScale)
	left, front := board.Position.X-board.Size.X/2, board.Position.Z-board.Size.Z/2
	sourceX, sourceZ := left+tl.X*pixelScale+surface.width/2, front+tl.Y*pixelScale+surface.depth/2
	x0, x1 := left+padX+surface.width/2, left+board.Size.X-padX-surface.width/2
	z0, z1 := front+padZ+surface.depth/2, front+board.Size.Z-padZ-surface.depth/2
	clamp := func(v, lo, hi float64) float64 { return max(lo, min(hi, v)) }
	surface.center.X = clamp(left+tl.X*pixelScale+surface.width/2, x0, x1)
	surface.center.Z = clamp(front+tl.Y*pixelScale+surface.depth/2, z0, z1)
	obstacles := make([]hierarchyHeaderObstacle, 0, 2*min(len(nodes), d2isometric.MaxNodes))
	markdownChild := false
	var supports map[string]float64
	if len(boards) > 0 {
		supports = hierarchySupportOffsets(boards[0])
	}
	view := nativeViewDirection()
	appendBox := func(node d2isometric.Node, halfX, halfZ, bottom, top float64) {
		if top < surface.center.Y {
			return
		}
		bottom = max(bottom, surface.center.Y)
		dx0, dx1 := (bottom-surface.center.Y)*view.X/view.Y, (top-surface.center.Y)*view.X/view.Y
		dz0, dz1 := (bottom-surface.center.Y)*view.Z/view.Y, (top-surface.center.Y)*view.Z/view.Y
		obstacle := hierarchyHeaderObstacle{
			x0: node.Position.X - halfX - max(dx0, dx1) - .005,
			x1: node.Position.X + halfX - min(dx0, dx1) + .005,
			z0: node.Position.Z - halfZ - max(dz0, dz1) - .005,
			z1: node.Position.Z + halfZ - min(dz0, dz1) + .005,
		}
		inside := obstacle.x1 >= left && obstacle.x0 <= left+board.Size.X && obstacle.z1 >= front && obstacle.z0 <= front+board.Size.Z
		outside := position.IsOutside() && obstacle.x1 >= sourceX-surface.width/2 && obstacle.x0 <= sourceX+surface.width/2 && obstacle.z1 >= sourceZ-surface.depth/2 && obstacle.z0 <= sourceZ+surface.depth/2
		if inside || outside {
			obstacles = append(obstacles, obstacle)
		}
	}
	for _, node := range nodes[:min(len(nodes), d2isometric.MaxNodes)] {
		if node.Container || node.Opacity <= 0 || !hierarchyHeaderPositive(node.Size.X) || !hierarchyHeaderPositive(node.Size.Z) {
			continue
		}
		markdownChild = markdownChild || nativeMarkdownCard(node) && node.BoardID == board.ID
		relief := hierarchyNodeRelief(node)
		if nativePlainMarkdownCard(node) {
			// The full-width sheet occupies only the upper 3.5px. Its recessed
			// support leaves visible room in a compiled container's lower label
			// margin; projecting a full-width wall to the floor hides that gap.
			floor, top := node.Position.Y-node.Size.Y/2, node.Position.Y+node.Size.Y/2
			inset := nativeMarkdownCardInset(node)
			appendBox(node, node.Size.X/2-inset, node.Size.Z/2-inset, floor, top-markdownCardPaperDepth)
			appendBox(node, node.Size.X/2, node.Size.Z/2, top-markdownCardPaperDepth, top+.002)
			continue
		}
		if nativeSolidNode(node) || nativeReliefSymbol(node) || nativeStructuredNode(node) || nativeMarkdownCard(node) {
			// hierarchyRenderNodes supplies the actual, already compressed
			// body height. Solid and relief symbols occupy the source footprint
			// with no separate base; labels sit .0015 above the body.
			if nativeStructuredNode(node) || nativeMarkdownCard(node) || !(node.FillExplicit && nativePaint(node.Fill, "#edf1f7").A == 0) {
				floor := node.Position.Y - node.Size.Y/2
				appendBox(node, node.Size.X/2, node.Size.Z/2, floor, floor+node.Size.Y+.002*relief)
			}
			continue
		}
		// Project the solid body and low graphite base separately. Expanding
		// the full-height body by the gold pin span invents a solid wall in
		// the source label margin. Individual pins are small sparse details,
		// not a continuous text occluder. Account for the raised base floor on
		// both front and back bounds, rather than only projecting the front.
		floor := node.Position.Y - node.Size.Y/2
		trim := min(1, min(node.Size.X/.6, node.Size.Z/.5))
		appendBox(node, node.Size.X/2+.055*trim, node.Size.Z/2+.05*trim, floor, floor+.09*trim*relief)
		if !(node.FillExplicit && nativePaint(node.Fill, "#edf1f7").A == 0) {
			drop := supports[node.BoardID]
			h := max(.3*relief, node.Size.Y+drop)
			halfX, halfZ := node.Size.X/2, node.Size.Z/2
			switch node.Type {
			case "diamond":
				h *= 1.28
			}
			h -= drop
			appendBox(node, halfX, halfZ, floor, floor+h+.12*relief)
		}
	}
	if len(boards) > 0 {
		owners := make(map[string]*d2isometric.Node, len(nodes))
		for i := range nodes {
			owners[nodes[i].ID] = &nodes[i]
		}
		for _, candidate := range boards[0] {
			node := owners[candidate.SourceID]
			if candidate.ID == board.ID || node == nil || node.Opacity <= 0 || !hierarchyPhysicalPlate(candidate, node, nativePaint(node.Fill, "transparent").A) {
				continue
			}
			depth := candidate.Size.Y
			if candidate.Kind == "platform" {
				depth = .13
			}
			// Include the raised cap's centered source outline as well as its
			// wall. A parent's title must not sit behind a nested back rim.
			cap := hierarchySurfaceY(candidate)
			if cap <= surface.center.Y+.001 {
				continue
			}
			margin := float64(node.StrokeWidth) * pixelScale / 2
			copy := *node
			copy.Position = candidate.Position
			appendBox(copy, candidate.Size.X/2+margin, candidate.Size.Z/2+margin, cap-depth, cap)
		}
	}
	name := position.String()
	vertical := strings.Contains(name, "_LEFT_") || strings.Contains(name, "_RIGHT_") || name == "INSIDE_MIDDLE_LEFT" || name == "INSIDE_MIDDLE_RIGHT"
	if placed, ok := hierarchyHeaderStrip(surface, obstacles, x0, x1, z0, z1, vertical); ok {
		return placed, true
	}
	// At most eleven additional interval searches. Bottom strips are less
	// likely to be hidden because this fixed camera projects bodies upward.
	for _, fraction := range []float64{1, 0, .5, .25, .75, .125, .875, .375, .625} {
		candidate := surface
		candidate.center.Z = z0 + (z1-z0)*fraction
		if placed, ok := hierarchyHeaderStrip(candidate, obstacles, x0, x1, z0, z1, false); ok {
			return placed, true
		}
	}
	for _, x := range []float64{x0, x1} {
		candidate := surface
		candidate.center.X = x
		if placed, ok := hierarchyHeaderStrip(candidate, obstacles, x0, x1, z0, z1, true); ok {
			return placed, true
		}
	}
	if markdownChild {
		// A source title can use the existing bottom margin right up to a
		// clear border. Try this one tighter strip before leaving the plate;
		// retain the full text allocation and keep at least 2px of edge space.
		pad := max(2*pixelScale, (float64(owner.StrokeWidth)/2+1)*pixelScale)
		if pad < padZ {
			bottom := surface
			bottom.center.Z = front + board.Size.Z - pad - bottom.depth/2
			if placed, ok := hierarchyHeaderStrip(bottom, obstacles, x0, x1, bottom.center.Z, bottom.center.Z, false); ok {
				return placed, true
			}
		}
	}
	if markdownChild && position.IsOutside() {
		// Tight source containers may reserve their heading outside the box.
		// If the raised document leaves no full-sized interior print strip,
		// retain that authored anchor instead of hiding the title behind it.
		outside := surface
		outside.center.X, outside.center.Z = sourceX, sourceZ
		if placed, ok := hierarchyHeaderStrip(outside, obstacles, outside.center.X, outside.center.X, outside.center.Z, outside.center.Z, false); ok {
			return placed, true
		}
	}
	return surface, false
}

type hierarchyHeaderObstacle struct{ x0, x1, z0, z1 float64 }

func hierarchyHeaderStrip(surface labelSurface, obstacles []hierarchyHeaderObstacle, x0, x1, z0, z1 float64, vertical bool) (labelSurface, bool) {
	axis, cross, span, crossSpan := surface.center.X, surface.center.Z, surface.width, surface.depth
	low, high := x0, x1
	if vertical {
		axis, cross, span, crossSpan = surface.center.Z, surface.center.X, surface.depth, surface.width
		low, high = z0, z1
	}
	type interval struct{ lo, hi float64 }
	blocked := make([]interval, 0, len(obstacles))
	for _, obstacle := range obstacles {
		a, b, c, d := obstacle.x0, obstacle.x1, obstacle.z0, obstacle.z1
		if vertical {
			a, b, c, d = c, d, a, b
		}
		if cross+crossSpan/2 < c || cross-crossSpan/2 > d {
			continue
		}
		a, b = a-span/2, b+span/2
		if b >= low && a <= high {
			blocked = append(blocked, interval{max(low, a), min(high, b)})
		}
	}
	sort.Slice(blocked, func(i, j int) bool {
		if blocked[i].lo != blocked[j].lo {
			return blocked[i].lo < blocked[j].lo
		}
		return blocked[i].hi < blocked[j].hi
	})
	best, distance, cursor := axis, math.Inf(1), low
	consider := func(a, b float64) {
		if a > b {
			return
		}
		candidate := max(a, min(b, axis))
		if d := math.Abs(candidate - axis); d < distance {
			best, distance = candidate, d
		}
	}
	for _, obstacle := range blocked {
		consider(cursor, obstacle.lo-.001)
		cursor = max(cursor, obstacle.hi+.001)
	}
	consider(cursor, high)
	if math.IsInf(distance, 1) {
		return surface, false
	}
	if vertical {
		surface.center.Z = best
	} else {
		surface.center.X = best
	}
	return surface, true
}

func hierarchyHeaderPositive(v float64) bool {
	return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}
