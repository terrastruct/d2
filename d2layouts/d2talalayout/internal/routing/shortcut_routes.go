package routing

import (
	"context"
	"errors"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/quality"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

const (
	maxShortcutInput = 256
	maxShortcutRoute = 32
	shortcutEpsilon  = 1e-6
)

// ShortcutEdgeRoutes is a bounded bend-reduction postpass with fixed boxes and
// ports. WueOrtho's routing step chooses bend-minimal shortest paths
// (https://arxiv.org/html/2309.01671v2#S2), motivating this local adaptation:
// replace a subchain by either rectilinear L, retaining only strict bend wins.
// It is not a shortest-path solver and does not change the routing grid.
func ShortcutEdgeRoutes(ctx context.Context, g *layoutgraph.Graph) error {
	err := shortcutRoutesWithLimit(ctx, g, maxRouteStageWorkUnits)
	if errors.Is(err, errRouteStageWorkLimit) {
		return nil // The atomic stage has already restored the entire drawing.
	}
	return err
}

func shortcutInputTooLarge(g *layoutgraph.Graph) bool {
	if g == nil {
		return false
	}
	if len(g.Nodes) > maxShortcutInput || len(g.Edges) > maxShortcutInput {
		return true
	}
	remaining := maxShortcutInput
	for _, e := range g.Edges {
		if e != nil {
			if len(e.Points) > remaining {
				return true
			}
			remaining -= len(e.Points)
		}
	}
	return false
}

func shortcutRoutesWithLimit(ctx context.Context, g *layoutgraph.Graph, limit uint64) error {
	if ctx != nil && g != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if shortcutInputTooLarge(g) {
			// Oversized inputs are optional no-ops, before validation/snapshot
			// allocation. Malformed admitted inputs retain stage validation.
			return ctx.Err()
		}
	}
	return runAtomicRouteStage(ctx, "ShortcutEdgeRoutes", g, nil, limit, func(guard *routeWorkGuard) error {
		return shortcutRoutesGuarded(g, guard)
	})
}

func shortcutRoutesGuarded(g *layoutgraph.Graph, guard *routeWorkGuard) error {
	var boxes []geo.Box
	labelsReady := false
	inventoryChecked := false
	for _, e := range g.Edges {
		if err := guard.step(); err != nil {
			return err
		}
		_, fromTree := g.NodeToTree[e.From]
		_, toTree := g.NodeToTree[e.To]
		if e.IsInvisible || e.IsCurve || e.IsLoop() || fromTree || toTree ||
			e.Label != nil || e.SourceArrowheadLabel != nil || e.TargetArrowheadLabel != nil ||
			len(e.Points) < 5 || len(e.Points) > maxShortcutRoute || !shortcutOrthogonal(e.Points) {
			continue
		}
		// A crossing or contact with a curved route cannot be certified from
		// its control polygon. Leave these drawings to the existing router.
		unsupported := false
		for _, other := range g.Edges {
			if err := guard.step(); err != nil {
				return err
			}
			unsupported = unsupported || other.IsCurve || !shortcutOrthogonal(other.Points)
		}
		if unsupported {
			return nil
		}
		if !labelsReady {
			var err error
			boxes, err = shortcutLabelBoxes(g, guard)
			if err != nil {
				return err
			}
			labelsReady = true
		}
		best := e.Points
		bestLength := shortcutLength(best)
		beforeBends, bestBends := shortcutBends(best), shortcutBends(best)
		for start := 0; start+3 < len(e.Points); start++ {
			for end := start + 3; end < len(e.Points); end++ {
				for _, horizontal := range []bool{true, false} {
					if err := guard.add(uint64(len(e.Points))); err != nil {
						return err
					}
					candidate := shortcutCandidate(e.Points, start, end, horizontal)
					length := shortcutLength(candidate)
					bends := shortcutBends(candidate)
					if bends >= beforeBends || bends > bestBends ||
						length > shortcutLength(e.Points)+shortcutEpsilon ||
						(bends == bestBends && length >= bestLength-shortcutEpsilon) {
						continue
					}
					safe, err := shortcutCandidateSafe(g, e, candidate, boxes, guard)
					if err != nil {
						return err
					}
					if safe {
						best, bestLength, bestBends = candidate, length, bends
					}
				}
			}
		}
		if bestBends == beforeBends {
			continue
		}
		if !inventoryChecked {
			// Direct graph slices define the modeled obstacles. Route-stage
			// ownership may reach other nodes/edges, so conservatively decline
			// these inputs before committing any uninspected geometry.
			snapshot, err := captureRouteMutations(g, nil, guard)
			if err != nil {
				return err
			}
			closed, err := channelInventoryIsClosed(g, snapshot, guard)
			if err != nil {
				return err
			}
			if !closed {
				return nil
			}
			inventoryChecked = true
		}
		// The local guards are per node/label/edge pair; inspection is a final
		// aggregate backstop, paid only for a geometrically viable shortcut.
		before, err := quality.Inspect(guard.ctx, g)
		if err != nil {
			return err
		}
		original := e.Points
		e.Points = best
		after, err := quality.Inspect(guard.ctx, g)
		if err != nil {
			return err
		}
		if after.RouteObstructions > before.RouteObstructions || after.Crossings > before.Crossings ||
			after.TextOcclusions > before.TextOcclusions || after.RouteLength > before.RouteLength+shortcutEpsilon {
			e.Points = original
		}
	}
	return nil
}

func shortcutOrthogonal(points []*geo.Point) bool {
	for i, p := range points {
		if p == nil || math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
			return false
		}
		if i > 0 && (*p == *points[i-1] || (p.X != points[i-1].X && p.Y != points[i-1].Y)) {
			return false
		}
		if i > 1 && !sameRouteDirection(points[i-2], points[i-1], points[i-1], p) &&
			(points[i-2].X == p.X || points[i-2].Y == p.Y) {
			return false // A U-turn is not a harmless collinear vertex.
		}
	}
	return len(points) >= 2
}

func shortcutLength(points []*geo.Point) float64 {
	length := 0.0
	for i := 1; i < len(points); i++ {
		length += math.Abs(points[i].X-points[i-1].X) + math.Abs(points[i].Y-points[i-1].Y)
	}
	return length
}

func shortcutBends(points []*geo.Point) int {
	bends := 0
	for i := 2; i < len(points); i++ {
		a, b, c := points[i-2], points[i-1], points[i]
		if (b.X-a.X)*(c.Y-b.Y) != (b.Y-a.Y)*(c.X-b.X) {
			bends++
		}
	}
	return bends
}

func shortcutCandidate(points []*geo.Point, start, end int, horizontal bool) []*geo.Point {
	elbow := geo.NewPoint(points[end].X, points[start].Y)
	if !horizontal {
		elbow = geo.NewPoint(points[start].X, points[end].Y)
	}
	raw := append([]*geo.Point(nil), points[:start+1]...)
	raw = append(raw, elbow)
	raw = append(raw, points[end:]...)
	result := make([]*geo.Point, 0, len(raw))
	for _, p := range raw {
		if len(result) > 0 && *result[len(result)-1] == *p {
			continue
		}
		for len(result) >= 2 && sameRouteDirection(result[len(result)-2], result[len(result)-1], result[len(result)-1], p) {
			result = result[:len(result)-1]
		}
		result = append(result, p)
	}
	return result
}

func shortcutCandidateSafe(g *layoutgraph.Graph, e *layoutgraph.Edge, points []*geo.Point, labels []geo.Box, guard *routeWorkGuard) (bool, error) {
	if !shortcutOrthogonal(points) || shortcutBends(points) >= shortcutBends(e.Points) || shortcutLength(points) > shortcutLength(e.Points)+shortcutEpsilon {
		return false, nil
	}
	candidate := *e
	candidate.Points = points
	safe, err := changedRouteIsClear(g, &candidate, e.Points, true, guard)
	if err != nil || !safe {
		return safe, err
	}
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		length := math.Abs(b.X-a.X) + math.Abs(b.Y-a.Y)
		minimum := segmentSpacingBuffer
		unchanged := false
		for j := 1; j < len(e.Points); j++ {
			if err := guard.step(); err != nil {
				return false, err
			}
			unchanged = unchanged || (*a == *e.Points[j-1] && *b == *e.Points[j])
		}
		if i == 1 {
			minimum = math.Min(minimum, shortcutLength(e.Points[:2]))
		}
		if i == len(points)-1 {
			minimum = math.Min(minimum, shortcutLength(e.Points[len(e.Points)-2:]))
		}
		if !unchanged && length+shortcutEpsilon < minimum {
			return false, nil
		}
		for j := i + 2; j < len(points); j++ {
			if err := guard.step(); err != nil {
				return false, err
			}
			if shortcutSegmentsMeet(a, b, points[j-1], points[j]) {
				return false, nil
			}
		}
		if unchanged {
			continue
		}
		for _, box := range labels {
			if err := guard.step(); err != nil {
				return false, err
			}
			if orthogonalSegmentEntersNode(&layoutgraph.Node{Box: box}, a, b) {
				return false, nil
			}
		}
		safe, err := shortcutWallClearance(g, e.Points, a, b, guard)
		if err != nil || !safe {
			return safe, err
		}
		safe, err = shortcutParallelClearance(g, e, a, b, guard)
		if err != nil || !safe {
			return safe, err
		}
	}
	for _, other := range g.Edges {
		if other == e {
			continue
		}
		safe, err := shortcutContactsPreserved(e.Points, points, other.Points, guard)
		if err != nil || !safe {
			return safe, err
		}
	}
	return true, nil
}

// Compare actual intersection geometry rather than segment indices: reducing
// bends renumbers segments. New point contacts must already lie on the old
// route; each new shared interval must be covered by old collinear geometry.
func shortcutContactsPreserved(before, after, other []*geo.Point, guard *routeWorkGuard) (bool, error) {
	for i := 1; i < len(after); i++ {
		for j := 1; j < len(other); j++ {
			if err := guard.step(); err != nil {
				return false, err
			}
			a, b, c, d := after[i-1], after[i], other[j-1], other[j]
			if !shortcutSegmentsMeet(a, b, c, d) {
				continue
			}
			if (a.X == b.X) != (c.X == d.X) {
				p := geo.NewPoint(a.X, c.Y)
				if a.Y == b.Y {
					p = geo.NewPoint(c.X, a.Y)
				}
				covered := false
				for k := 1; k < len(before); k++ {
					if err := guard.step(); err != nil {
						return false, err
					}
					covered = covered || shortcutPointOnSegment(p, before[k-1], before[k])
				}
				oldH, oldV, err := shortcutThroughRays(before, p, guard)
				if err != nil {
					return false, err
				}
				newH, newV, err := shortcutThroughRays(after, p, guard)
				if err != nil {
					return false, err
				}
				otherH, otherV, err := shortcutThroughRays(other, p, guard)
				if err != nil {
					return false, err
				}
				newCrossing := (newH && otherV) || (newV && otherH)
				oldCrossing := (oldH && otherV) || (oldV && otherH)
				if !covered || (newCrossing && !oldCrossing) {
					return false, nil
				}
				continue
			}
			horizontal := a.Y == b.Y
			position := a.X
			if horizontal {
				position = a.Y
			}
			lo := math.Max(math.Min(channelCoordinate(a, horizontal), channelCoordinate(b, horizontal)), math.Min(channelCoordinate(c, horizontal), channelCoordinate(d, horizontal)))
			hi := math.Min(math.Max(channelCoordinate(a, horizontal), channelCoordinate(b, horizontal)), math.Max(channelCoordinate(c, horizontal), channelCoordinate(d, horizontal)))
			// Extend covered prefix without sorting or altering the old route.
			cursor := lo
			for {
				next, containsPoint := cursor, false
				for k := 1; k < len(before); k++ {
					if err := guard.step(); err != nil {
						return false, err
					}
					u, v := before[k-1], before[k]
					if (horizontal && (u.Y != position || v.Y != position)) || (!horizontal && (u.X != position || v.X != position)) {
						continue
					}
					left, right := math.Min(channelCoordinate(u, horizontal), channelCoordinate(v, horizontal)), math.Max(channelCoordinate(u, horizontal), channelCoordinate(v, horizontal))
					if left <= cursor+shortcutEpsilon && right >= cursor-shortcutEpsilon {
						containsPoint = true
						next = math.Max(next, right)
					}
				}
				if containsPoint && next >= hi-shortcutEpsilon {
					break
				}
				if next <= cursor+shortcutEpsilon {
					return false, nil
				}
				cursor = next
			}
		}
	}
	return true, nil
}

func shortcutPointOnSegment(p, a, b *geo.Point) bool {
	return ((a.X == b.X && p.X == a.X) || (a.Y == b.Y && p.Y == a.Y)) &&
		p.X >= math.Min(a.X, b.X)-shortcutEpsilon && p.X <= math.Max(a.X, b.X)+shortcutEpsilon &&
		p.Y >= math.Min(a.Y, b.Y)-shortcutEpsilon && p.Y <= math.Max(a.Y, b.Y)+shortcutEpsilon
}

// For orthogonal segments, overlapping closed bounding boxes mean contact.
// Unlike line intersection, this also includes positive collinear overlaps.
func shortcutSegmentsMeet(a, b, c, d *geo.Point) bool {
	return math.Max(math.Min(a.X, b.X), math.Min(c.X, d.X)) <= math.Min(math.Max(a.X, b.X), math.Max(c.X, d.X)) &&
		math.Max(math.Min(a.Y, b.Y), math.Min(c.Y, d.Y)) <= math.Min(math.Max(a.Y, b.Y), math.Max(c.Y, d.Y))
}

// Classify the whole route at a contact. Joining the two rays is equivalent
// to removing harmless collinear subdivisions, without touching source slices.
// Segment-local endpoint tests miss a through-crossing at such a subdivision.
func shortcutThroughRays(points []*geo.Point, p *geo.Point, guard *routeWorkGuard) (bool, bool, error) {
	left, right, above, below := false, false, false, false
	for i := 1; i < len(points); i++ {
		if err := guard.step(); err != nil {
			return false, false, err
		}
		a, b := points[i-1], points[i]
		if !shortcutPointOnSegment(p, a, b) {
			continue
		}
		if a.Y == b.Y {
			left = left || math.Min(a.X, b.X) < p.X
			right = right || math.Max(a.X, b.X) > p.X
		} else {
			above = above || math.Min(a.Y, b.Y) < p.Y
			below = below || math.Max(a.Y, b.Y) > p.Y
		}
	}
	return left && right, above && below, nil
}

// A new parallel leg must retain min(previous gap,40) to every projected node
// wall, including container walls. With no old parallel leg on that side of
// the wall, require40. Existing short port approaches are handled separately.
func shortcutWallClearance(g *layoutgraph.Graph, before []*geo.Point, a, b *geo.Point, guard *routeWorkGuard) (bool, error) {
	horizontal := a.Y == b.Y
	position := channelCoordinate(a, !horizontal)
	lo, hi := math.Min(channelCoordinate(a, horizontal), channelCoordinate(b, horizontal)), math.Max(channelCoordinate(a, horizontal), channelCoordinate(b, horizontal))
	for _, n := range g.Nodes {
		if err := guard.step(); err != nil {
			return false, err
		}
		nlo := channelCoordinate(n.TopLeft, horizontal)
		nhi, wall := nlo+n.Height, n.TopLeft.X
		walls := [2]float64{wall, wall + n.Width}
		if horizontal {
			nhi, wall = nlo+n.Width, n.TopLeft.Y
			walls = [2]float64{wall, wall + n.Height}
		}
		if math.Min(hi, nhi)-math.Max(lo, nlo) <= shortcutEpsilon {
			continue
		}
		for _, wall := range walls {
			gap := math.Abs(position - wall)
			if gap >= segmentSpacingBuffer-shortcutEpsilon {
				continue
			}
			minimum := segmentSpacingBuffer
			for j := 1; j < len(before); j++ {
				if err := guard.step(); err != nil {
					return false, err
				}
				u, v := before[j-1], before[j]
				oldPosition := channelCoordinate(u, !horizontal)
				if oldPosition != channelCoordinate(v, !horizontal) || (oldPosition-wall)*(position-wall) < 0 {
					continue
				}
				oldLo, oldHi := math.Min(channelCoordinate(u, horizontal), channelCoordinate(v, horizontal)), math.Max(channelCoordinate(u, horizontal), channelCoordinate(v, horizontal))
				if math.Min(oldHi, nhi)-math.Max(oldLo, nlo) > shortcutEpsilon {
					minimum = math.Min(minimum, math.Abs(oldPosition-wall))
				}
			}
			if gap+shortcutEpsilon < minimum {
				return false, nil
			}
		}
	}
	return true, nil
}

// Preserve the channel pass's readable spacing. For each projected parallel
// segment of another route, retain min(previous positive gap,40); if there was
// no positive parallel gap, require40. Exact shared geometry is certified by
// shortcutContactsPreserved and is not used to justify a new narrow corridor.
func shortcutParallelClearance(g *layoutgraph.Graph, e *layoutgraph.Edge, a, b *geo.Point, guard *routeWorkGuard) (bool, error) {
	horizontal := a.Y == b.Y
	position := channelCoordinate(a, !horizontal)
	lo, hi := math.Min(channelCoordinate(a, horizontal), channelCoordinate(b, horizontal)), math.Max(channelCoordinate(a, horizontal), channelCoordinate(b, horizontal))
	for _, other := range g.Edges {
		if other == e {
			continue
		}
		for j := 1; j < len(other.Points); j++ {
			if err := guard.step(); err != nil {
				return false, err
			}
			c, d := other.Points[j-1], other.Points[j]
			wall := channelCoordinate(c, !horizontal)
			if wall != channelCoordinate(d, !horizontal) {
				continue
			}
			otherLo, otherHi := math.Min(channelCoordinate(c, horizontal), channelCoordinate(d, horizontal)), math.Max(channelCoordinate(c, horizontal), channelCoordinate(d, horizontal))
			if math.Min(hi, otherHi)-math.Max(lo, otherLo) <= shortcutEpsilon {
				continue
			}
			gap := math.Abs(position - wall)
			if gap <= shortcutEpsilon || gap >= segmentSpacingBuffer-shortcutEpsilon {
				continue
			}
			minimum := segmentSpacingBuffer
			for k := 1; k < len(e.Points); k++ {
				if err := guard.step(); err != nil {
					return false, err
				}
				u, v := e.Points[k-1], e.Points[k]
				oldPosition := channelCoordinate(u, !horizontal)
				if oldPosition != channelCoordinate(v, !horizontal) {
					continue
				}
				oldLo, oldHi := math.Min(channelCoordinate(u, horizontal), channelCoordinate(v, horizontal)), math.Max(channelCoordinate(u, horizontal), channelCoordinate(v, horizontal))
				oldGap := math.Abs(oldPosition - wall)
				if oldGap > shortcutEpsilon && math.Min(oldHi, otherHi)-math.Max(oldLo, otherLo) > shortcutEpsilon {
					minimum = math.Min(minimum, oldGap)
				}
			}
			if gap+shortcutEpsilon < minimum {
				return false, nil
			}
		}
	}
	return true, nil
}

func shortcutLabelBoxes(g *layoutgraph.Graph, guard *routeWorkGuard) ([]geo.Box, error) {
	var boxes []geo.Box
	for _, n := range g.Nodes {
		if err := guard.add(uint64(1 + 4*len(g.Nodes))); err != nil {
			return nil, err
		}
		if n.IsInvisible {
			continue
		}
		if n.Label != nil && n.Label.Position != label.Unset {
			boxes = append(boxes, geo.Box{TopLeft: n.LabelTopLeft(n.Label.Position, n.Label.Width, n.Label.Height), Width: n.Label.Width, Height: n.Label.Height})
		}
		if n.Icon != nil && !n.IsImage() {
			size := n.IconSize(n.Icon.Position)
			boxes = append(boxes, geo.Box{TopLeft: n.LabelTopLeft(n.Icon.Position, size, size), Width: size, Height: size})
		}
	}
	for _, e := range g.Edges {
		if err := guard.add(uint64(1 + 3*len(e.Points))); err != nil {
			return nil, err
		}
		if e.IsInvisible || len(e.Points) < 2 {
			continue
		}
		if e.Label != nil && e.Label.Position != label.Unset {
			boxes = append(boxes, geo.Box{TopLeft: e.LabelTopLeft(e.Label.Position, e.Label.Width, e.Label.Height), Width: e.Label.Width, Height: e.Label.Height})
		}
		if e.SourceArrowheadLabel != nil {
			boxes = append(boxes, labeling.PositionArrowheadLabel(e, false, e.Points).Box)
		}
		if e.TargetArrowheadLabel != nil {
			boxes = append(boxes, labeling.PositionArrowheadLabel(e, true, e.Points).Box)
		}
	}
	return boxes, nil
}
