package routing

import (
	"context"
	"errors"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/quality"
	"github.com/d2lang/d2/lib/geo"
)

const channelEpsilon = 1e-6

// This optional postpass admits only small channel systems. Quadratic
// discovery must not consume a successful layout's routing budget.
const maxChannelSegments = 256

func NudgeEdgeChannels(ctx context.Context, g *layoutgraph.Graph) error {
	err := nudgeChannelsWithLimit(ctx, g, maxRouteStageWorkUnits)
	if errors.Is(err, errRouteStageWorkLimit) {
		return nil
	}
	return err
}

type channelGroup struct {
	segments               []*layoutgraph.EdgeSegment
	position, lower, upper float64
	fixed                  bool
	nodeClearance          *Range
}

type channelArc struct {
	from, to int
	// Legs of a single route may meet to remove an unnecessary bend.
	separate bool
}

type channelProblem struct {
	groups []*channelGroup
	arcs   []channelArc
}

// nudgeChannelsGuarded uses WueOrtho's separation-constraint idea with fixed
// boxes and ports: https://arxiv.org/html/2309.01671v2#S2.SS0.SSS0.Px7.
// This bounded special case solves gap feasibility on a DAG, not the paper's
// complete LP. It retains the existing drawing unless one of its feasible
// extremal or midpoint solutions improves wire length or spacing.
func nudgeChannelsGuarded(g *layoutgraph.Graph, guard *routeWorkGuard) error {
	for _, horizontal := range []bool{true, false} {
		problem, err := buildChannelProblem(g, horizontal, guard)
		if err != nil {
			return err
		}
		if len(problem.groups) == 0 {
			continue
		}
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
		beforePoints, err := copyRoutePoints(g.Edges, guard)
		if err != nil {
			return err
		}
		beforeMetrics, err := quality.Inspect(guard.ctx, g)
		if err != nil {
			return err
		}
		changed, err := applyChannelProblem(g, problem, horizontal, guard)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		afterMetrics, err := quality.Inspect(guard.ctx, g)
		if err != nil {
			return err
		}
		if afterMetrics.RouteObstructions > beforeMetrics.RouteObstructions ||
			afterMetrics.Crossings > beforeMetrics.Crossings || afterMetrics.TextOcclusions > beforeMetrics.TextOcclusions ||
			afterMetrics.RouteLength > beforeMetrics.RouteLength+channelEpsilon {
			snapshot.restore()
			continue
		}
		for _, edge := range g.Edges {
			moved := false
			for i, point := range edge.Points {
				if *point != *beforePoints[edge][i] {
					moved = true
					break
				}
			}
			if !moved {
				continue
			}
			points, err := removeDuplicatePointsGuarded(edge.Points, guard)
			if err != nil {
				return err
			}
			edge.Points = points
		}
	}
	return nil
}

// Route-stage snapshots can reach owners outside the graph's direct slices.
// Such owners are not obstacles in this bounded model, so do not risk moving
// an uninspected route or node through an aliased point.
func channelInventoryIsClosed(g *layoutgraph.Graph, snapshot routeMutationSnapshot, guard *routeWorkGuard) (bool, error) {
	nodes := make(map[*layoutgraph.Node]bool, len(g.Nodes))
	edges := make(map[*layoutgraph.Edge]bool, len(g.Edges))
	for _, n := range g.Nodes {
		if err := guard.step(); err != nil {
			return false, err
		}
		nodes[n] = true
	}
	for _, e := range g.Edges {
		if err := guard.step(); err != nil {
			return false, err
		}
		edges[e] = true
	}
	for n := range snapshot.nodes {
		if err := guard.step(); err != nil {
			return false, err
		}
		if !nodes[n] {
			return false, nil
		}
	}
	for e := range snapshot.edges {
		if err := guard.step(); err != nil {
			return false, err
		}
		if !edges[e] {
			return false, nil
		}
	}
	return true, nil
}

func nudgeChannelsWithLimit(ctx context.Context, g *layoutgraph.Graph, limit uint64) error {
	if ctx != nil && g != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if channelInputTooLarge(g) {
			// Admission scans at most maxChannelSegments edge references. Check
			// cancellation again before returning without the atomic stage.
			return ctx.Err()
		}
	}
	return runAtomicRouteStage(ctx, "NudgeChannels", g, nil, limit, func(guard *routeWorkGuard) error {
		return nudgeChannelsGuarded(g, guard)
	})
}

// Oversized inputs are an optional no-op, so reject them before topology
// validation or rollback snapshots allocate proportional to the whole graph.
// This is only admission, not validation: nil inputs and malformed admitted
// graphs still go through the existing atomic-stage validation boundary.
func channelInputTooLarge(g *layoutgraph.Graph) bool {
	if g == nil {
		return false
	}
	if len(g.Nodes) > maxChannelSegments || len(g.Edges) > maxChannelSegments {
		return true
	}
	remaining := maxChannelSegments
	for _, edge := range g.Edges {
		if edge == nil {
			continue
		}
		if len(edge.Points) > remaining {
			return true
		}
		remaining -= len(edge.Points)
	}
	return false
}

func channelCoordinate(p *geo.Point, horizontal bool) float64 {
	if horizontal {
		return p.X
	}
	return p.Y
}

func buildChannelProblem(g *layoutgraph.Graph, horizontal bool, guard *routeWorkGuard) (channelProblem, error) {
	var result channelProblem
	segments, err := edgeSegmentsGuarded(layoutgraph.Edges(g.Edges), !horizontal, guard)
	if err != nil {
		return result, err
	}
	fixedPoints := make(map[*geo.Point]bool)
	for _, n := range g.Nodes {
		fixedPoints[n.TopLeft] = true
	}
	lockedEdges := make(map[*layoutgraph.Edge]bool)
	for _, e := range g.Edges {
		if err := guard.step(); err != nil {
			return result, err
		}
		if len(e.Points) < 2 {
			continue
		}
		fixedPoints[e.Points[0]], fixedPoints[e.Points[len(e.Points)-1]] = true, true
		_, fromTree := g.NodeToTree[e.From]
		_, toTree := g.NodeToTree[e.To]
		lockedEdges[e] = e.IsCurve || e.IsLoop() || fromTree || toTree ||
			(isSpecialEdgeForBalancing(g, e) && !hasFixedBalancingPorts(e))
		for i := 1; i < len(e.Points); i++ {
			if err := guard.step(); err != nil {
				return result, err
			}
			a, b := e.Points[i-1], e.Points[i]
			if (a.X != b.X && a.Y != b.Y) || *a == *b {
				lockedEdges[e] = true
			}
		}
	}
	parent := make([]int, len(segments))
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i, s := range segments {
		for j := 0; j < i; j++ {
			if err := guard.step(); err != nil {
				return result, err
			}
			other := segments[j]
			if channelCoordinate(s.Start, horizontal) == channelCoordinate(other.Start, horizontal) &&
				s.Segment.Overlaps(other.Segment, horizontal, segmentSpacingBuffer) {
				parent[root(i)] = root(j)
			}
		}
	}
	byRoot := make(map[int]*channelGroup)
	var groups []*channelGroup
	for i, s := range segments {
		if err := guard.step(); err != nil {
			return result, err
		}
		group := byRoot[root(i)]
		if group == nil {
			group = &channelGroup{position: channelCoordinate(s.Start, horizontal)}
			byRoot[root(i)] = group
			groups = append(groups, group)
		}
		group.segments = append(group.segments, s)
		group.fixed = group.fixed || lockedEdges[s.Owner()] || fixedPoints[s.Start] || fixedPoints[s.End]
		for _, n := range g.Nodes {
			if err := guard.step(); err != nil {
				return result, err
			}
			if n.Width != 1 || n.Height != 1 {
				continue
			}
			for _, p := range []*geo.Point{s.Start, s.End} {
				if (p.X == n.TopLeft.X || p.X == n.TopLeft.X+1) && (p.Y == n.TopLeft.Y || p.Y == n.TopLeft.Y+1) {
					group.fixed = true
				}
			}
		}
	}
	fixed, err := nodeSegmentsGuarded(layoutgraph.Nodes(g.Nodes), !horizontal, guard)
	if err != nil {
		return result, err
	}
	nodeWalls := fixed
	for _, group := range groups {
		if group.fixed {
			continue
		}
		clearance := &Range{start: math.Inf(-1), end: math.Inf(1)}
		for _, segment := range group.segments {
			lower, upper, err := routeSegmentBounds(segment.Segment, nodeWalls, segmentSpacingBuffer, guard)
			if err != nil {
				return result, err
			}
			// A narrow gap elsewhere in this component must not let wire
			// shortening pull a route against a previously comfortable border.
			// Preserve short existing gaps; larger ones may contract to the
			// ordinary spacing distance. These are absolute bounds, separate
			// from the shared gap variable, so clearance is not counted twice.
			if !math.IsInf(lower, -1) {
				clearance.start = math.Max(clearance.start, lower+math.Min(group.position-lower, segmentSpacingBuffer))
			}
			if !math.IsInf(upper, 1) {
				clearance.end = math.Min(clearance.end, upper-math.Min(upper-group.position, segmentSpacingBuffer))
			}
		}
		group.nodeClearance = clearance
	}
	appendFixed := func(group *channelGroup) {
		for _, s := range group.segments {
			fixed = append(fixed, &s.Segment)
		}
	}
	for _, group := range groups {
		if group.fixed {
			appendFixed(group)
		}
	}
	// A real wall must bound both sides. An arbitrary margin outside the
	// drawing would turn repeated nudging into outward drift.
	for pass := 0; pass < 2; pass++ {
		for _, group := range groups {
			if group.fixed {
				continue
			}
			group.lower, group.upper = math.Inf(-1), math.Inf(1)
			for _, s := range group.segments {
				lo, hi, err := routeSegmentBounds(s.Segment, fixed, segmentSpacingBuffer, guard)
				if err != nil {
					return result, err
				}
				group.lower, group.upper = math.Max(group.lower, lo), math.Min(group.upper, hi)
			}
			if math.IsInf(group.lower, 0) || math.IsInf(group.upper, 0) || group.lower > group.position || group.upper < group.position || group.lower >= group.upper {
				group.fixed = true
				appendFixed(group)
			}
		}
	}
	for _, group := range groups {
		if !group.fixed {
			result.groups = append(result.groups, group)
		}
	}
	if err := stableSortRouteValues(result.groups, func(a, b *channelGroup) bool { return a.position < b.position }, guard); err != nil {
		return result, err
	}
	for i, a := range result.groups {
		for j := i + 1; j < len(result.groups); j++ {
			b := result.groups[j]
			linked, sameOwner := false, true
			owner := a.segments[0].Owner()
			for _, s := range a.segments {
				sameOwner = sameOwner && s.Owner() == owner
			}
			for _, s := range b.segments {
				sameOwner = sameOwner && s.Owner() == owner
			}
			buffer := segmentSpacingBuffer
			if sameOwner {
				buffer = 0
			}
			for _, s := range a.segments {
				for _, t := range b.segments {
					if err := guard.step(); err != nil {
						return result, err
					}
					linked = linked || s.Segment.Overlaps(t.Segment, horizontal, buffer)
				}
			}
			if linked {
				result.arcs = append(result.arcs, channelArc{i, j, !sameOwner})
			}
		}
	}
	return result, nil
}

// solveChannelGap computes the earliest and latest feasible solutions of the
// same difference constraints. Their midpoint is feasible by convexity.
func solveChannelGap(p channelProblem, gap float64, guard *routeWorkGuard) ([]float64, []float64, bool, error) {
	n := len(p.groups)
	if err := guard.add(uint64(n) * 2); err != nil {
		return nil, nil, false, err
	}
	lo, hi := make([]float64, n), make([]float64, n)
	for i, group := range p.groups {
		lo[i], hi[i] = group.lower+gap, group.upper-gap
		if group.nodeClearance != nil {
			lo[i] = math.Max(lo[i], group.nodeClearance.start)
			hi[i] = math.Min(hi[i], group.nodeClearance.end)
		}
	}
	for _, arc := range p.arcs {
		if err := guard.step(); err != nil {
			return nil, nil, false, err
		}
		separation := 0.0
		if arc.separate {
			separation = gap
		}
		lo[arc.to] = math.Max(lo[arc.to], lo[arc.from]+separation)
	}
	for index := len(p.arcs) - 1; index >= 0; index-- {
		arc := p.arcs[index]
		if err := guard.step(); err != nil {
			return nil, nil, false, err
		}
		separation := 0.0
		if arc.separate {
			separation = gap
		}
		hi[arc.from] = math.Min(hi[arc.from], hi[arc.to]-separation)
	}
	for i := range lo {
		if lo[i] > hi[i] {
			return nil, nil, false, nil
		}
	}
	return lo, hi, true, nil
}

func channelGap(p channelProblem, positions []float64) float64 {
	gap := math.Inf(1)
	for i, group := range p.groups {
		gap = math.Min(gap, math.Min(positions[i]-group.lower, group.upper-positions[i]))
	}
	for _, arc := range p.arcs {
		if arc.separate {
			gap = math.Min(gap, positions[arc.to]-positions[arc.from])
		}
	}
	return gap
}

func applyChannelProblem(g *layoutgraph.Graph, p channelProblem, horizontal bool, guard *routeWorkGuard) (bool, error) {
	parent := make([]int, len(p.groups))
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for _, arc := range p.arcs {
		parent[root(arc.to)] = root(arc.from)
	}
	components := make(map[int][]int)
	var order []int
	for i := range p.groups {
		id := root(i)
		if _, ok := components[id]; !ok {
			order = append(order, id)
		}
		components[id] = append(components[id], i)
	}
	changed := false
	for _, id := range order {
		var local channelProblem
		indices := make(map[int]int)
		for _, i := range components[id] {
			indices[i] = len(local.groups)
			local.groups = append(local.groups, p.groups[i])
		}
		for _, arc := range p.arcs {
			if from, ok := indices[arc.from]; ok {
				local.arcs = append(local.arcs, channelArc{from, indices[arc.to], arc.separate})
			}
		}
		positions := make([]float64, len(local.groups))
		for i, group := range local.groups {
			positions[i] = group.position
		}
		oldGap := math.Max(0, channelGap(local, positions))
		left, right := oldGap, math.Inf(1)
		for _, group := range local.groups {
			right = math.Min(right, (group.upper-group.lower)/2)
		}
		for iteration := 0; iteration < 32 && right-left > channelEpsilon; iteration++ {
			mid := (left + right) / 2
			_, _, feasible, err := solveChannelGap(local, mid, guard)
			if err != nil {
				return false, err
			}
			if feasible {
				left = mid
			} else {
				right = mid
			}
		}
		bestLength, err := channelWireLength(g, nil, guard)
		if err != nil {
			return false, err
		}
		bestGap := oldGap
		var best map[*geo.Point]*geo.Point
		for _, gap := range []float64{oldGap, left} {
			earliest, latest, feasible, err := solveChannelGap(local, gap, guard)
			if err != nil {
				return false, err
			}
			if !feasible {
				continue
			}
			for _, fraction := range []float64{0, 1, 0.5} {
				candidate := make([]float64, len(positions))
				for i := range candidate {
					candidate[i] = earliest[i]*(1-fraction) + latest[i]*fraction
				}
				proposal, compatible, err := channelPointProposal(local, candidate, horizontal, guard)
				if err != nil {
					return false, err
				}
				if !compatible {
					continue
				}
				length, err := channelWireLength(g, proposal, guard)
				if err != nil {
					return false, err
				}
				actualGap := channelGap(local, candidate)
				// Pixel-level rounding changes alone are not a spacing benefit.
				if length > bestLength+channelEpsilon || (math.Abs(length-bestLength) <= channelEpsilon && actualGap < bestGap+1) {
					continue
				}
				safe, err := channelProposalSafe(g, proposal, guard)
				if err != nil {
					return false, err
				}
				if !safe {
					continue
				}
				best, bestLength, bestGap = proposal, length, actualGap
			}
		}
		if best != nil {
			for original, candidate := range best {
				if err := guard.step(); err != nil {
					return false, err
				}
				*original = *candidate
			}
			changed = true
		}
	}
	return changed, nil
}

func channelPointProposal(p channelProblem, positions []float64, horizontal bool, guard *routeWorkGuard) (map[*geo.Point]*geo.Point, bool, error) {
	proposal := make(map[*geo.Point]*geo.Point)
	for i, group := range p.groups {
		for _, s := range group.segments {
			for _, point := range []*geo.Point{s.Start, s.End} {
				if err := guard.step(); err != nil {
					return nil, false, err
				}
				candidate := *point
				if horizontal {
					candidate.X = positions[i]
				} else {
					candidate.Y = positions[i]
				}
				if old, ok := proposal[point]; ok && *old != candidate {
					return nil, false, nil
				}
				proposal[point] = &candidate
			}
		}
	}
	return proposal, true, nil
}

func channelWireLength(g *layoutgraph.Graph, proposal map[*geo.Point]*geo.Point, guard *routeWorkGuard) (float64, error) {
	length := 0.0
	for _, e := range g.Edges {
		for i := 1; i < len(e.Points); i++ {
			if err := guard.step(); err != nil {
				return 0, err
			}
			a, b := e.Points[i-1], e.Points[i]
			if p := proposal[a]; p != nil {
				a = p
			}
			if p := proposal[b]; p != nil {
				b = p
			}
			length += math.Hypot(b.X-a.X, b.Y-a.Y)
		}
	}
	return length, nil
}

func channelProposalSafe(g *layoutgraph.Graph, proposal map[*geo.Point]*geo.Point, guard *routeWorkGuard) (bool, error) {
	candidates := make([]*layoutgraph.Edge, len(g.Edges))
	changed := make([]bool, len(g.Edges))
	for index, e := range g.Edges {
		candidate := *e
		candidate.Points = append([]*geo.Point(nil), e.Points...)
		for i, point := range e.Points {
			if err := guard.step(); err != nil {
				return false, err
			}
			if p := proposal[point]; p != nil {
				candidate.Points[i] = p
				changed[index] = changed[index] || *p != *point
			}
		}
		candidates[index] = &candidate
		if !changed[index] {
			continue
		}
		if e.IsCurve {
			return false, nil
		}
		safe, err := changedRouteIsClear(g, &candidate, e.Points, true, guard)
		if err != nil || !safe {
			return safe, err
		}
		beforeLength, afterLength := 0.0, 0.0
		var nonzero []*geo.Point
		for i, point := range candidate.Points {
			if err := guard.step(); err != nil {
				return false, err
			}
			if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
				return false, nil
			}
			if i > 0 {
				old, previous := e.Points[i-1], candidate.Points[i-1]
				before := math.Hypot(e.Points[i].X-old.X, e.Points[i].Y-old.Y)
				after := math.Hypot(point.X-previous.X, point.Y-previous.Y)
				beforeLength += before
				afterLength += after
				if after > channelEpsilon && after+channelEpsilon < math.Min(before, segmentSpacingBuffer) {
					return false, nil
				}
			}
			if len(nonzero) == 0 || *nonzero[len(nonzero)-1] != *point {
				nonzero = append(nonzero, point)
			}
		}
		if afterLength > beforeLength+channelEpsilon {
			return false, nil
		}
		for i := 2; i < len(nonzero); i++ {
			a, b, c := nonzero[i-2], nonzero[i-1], nonzero[i]
			if (b.X-a.X)*(c.X-b.X)+(b.Y-a.Y)*(c.Y-b.Y) < 0 {
				return false, nil
			}
		}
		for i := 0; i+1 < len(nonzero); i++ {
			for j := i + 2; j+1 < len(nonzero); j++ {
				if err := guard.step(); err != nil {
					return false, err
				}
				if (geo.Segment{Start: nonzero[i], End: nonzero[i+1]}).Intersects(geo.Segment{Start: nonzero[j], End: nonzero[j+1]}) {
					return false, nil
				}
			}
		}
	}
	for i, e := range g.Edges {
		for j := i + 1; j < len(g.Edges); j++ {
			if !changed[i] && !changed[j] {
				continue
			}
			other := g.Edges[j]
			if e.IsCurve || other.IsCurve {
				return false, nil
			}
			beforeCrossings, afterCrossings := 0, 0
			for a := 0; a+1 < len(e.Points); a++ {
				for b := 0; b+1 < len(other.Points); b++ {
					if err := guard.step(); err != nil {
						return false, err
					}
					if isNonSharedCrossing(e, other, a, b) {
						beforeCrossings++
					}
					if isNonSharedCrossing(candidates[i], candidates[j], a, b) {
						afterCrossings++
					}
					old := balanceCollinearOverlap(e.Points[a], e.Points[a+1], other.Points[b], other.Points[b+1])
					new := balanceCollinearOverlap(candidates[i].Points[a], candidates[i].Points[a+1], candidates[j].Points[b], candidates[j].Points[b+1])
					if old == 0 && new > 0 {
						return false, nil
					}
					oldContact := (geo.Segment{Start: e.Points[a], End: e.Points[a+1]}).Intersects(geo.Segment{Start: other.Points[b], End: other.Points[b+1]})
					newContact := (geo.Segment{Start: candidates[i].Points[a], End: candidates[i].Points[a+1]}).Intersects(geo.Segment{Start: candidates[j].Points[b], End: candidates[j].Points[b+1]})
					if !oldContact && newContact {
						return false, nil
					}
				}
			}
			if afterCrossings > beforeCrossings {
				return false, nil
			}
		}
	}
	return true, nil
}
