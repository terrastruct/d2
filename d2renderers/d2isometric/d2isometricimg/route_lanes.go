package d2isometricimg

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

const (
	nativeLaneMaxPoints = 100000
	nativeLaneMaxWork   = 25000000
	nativeLaneMaxOffset = .55
	nativeLaneMinShared = .45
)

type nativeLaneKey struct{ x, z, normal, y int64 }
type nativeLaneDirection struct{ x, z, y int64 }
type nativeLaneInterval struct{ lo, hi float64 }
type nativeLaneSegment struct {
	edge, part int
	a, b       Vec
	u, normal  Vec
	lo, hi     float64
	line       float64
	length     float64
	key        nativeLaneKey
	shared     []nativeLaneInterval
}
type nativeLaneObstacle struct {
	id                       string
	header                   bool
	left, right, front, back float64
}
type nativeLaneWork struct {
	ctx       context.Context
	remaining int
}

func (w *nativeLaneWork) spend(n int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if n > w.remaining {
		return fmt.Errorf("isometric route lanes exceed work limit")
	}
	w.remaining -= n
	return nil
}

// nativeLaneRoutes separates overlapping straight runs before corner rounding
// and bridge detection. It keeps all source ports and elevations, changes only
// the shared run and its short fan-out, and never mutates the scene. A bounded
// set of small offsets is tried in stable edge-ID order. A run without a clear
// lane retains its source geometry rather than crossing a component or moving
// an entire board. Common port stubs necessarily remain shared.
func nativeLaneRoutes(ctx context.Context, edges []d2isometric.Edge, nodes []d2isometric.Node, boards []d2isometric.Board) ([][]Vec, error) {
	return nativeResolveLaneRoutes(ctx, edges, nodes, boards, nativeLaneMaxWork)
}

func nativeResolveLaneRoutes(ctx context.Context, edges []d2isometric.Edge, nodes []d2isometric.Node, boards []d2isometric.Board, limit int) ([][]Vec, error) {
	if ctx == nil {
		return nil, fmt.Errorf("isometric route lanes require a context")
	}
	w := nativeLaneWork{ctx: ctx, remaining: limit}
	if len(edges) > nativeLaneMaxPoints || len(nodes) > nativeLaneMaxPoints || len(boards) > nativeLaneMaxPoints {
		return nil, fmt.Errorf("isometric route lanes exceed scene limit")
	}
	if err := w.spend(len(edges) + len(nodes) + len(boards)); err != nil {
		return nil, err
	}
	paths := make([][]Vec, len(edges))
	parts := make([][]int, len(edges))
	segments := []nativeLaneSegment{}
	groups := map[nativeLaneKey][]int{}
	parallel := map[nativeLaneDirection][]int{}
	ids := map[string]bool{}
	total := 0
	for i, edge := range edges {
		if ids[edge.ID] {
			return nil, fmt.Errorf("isometric route lanes require unique edge IDs: %q", edge.ID)
		}
		ids[edge.ID] = true
		if len(edge.Points) > 10000 || len(edge.Points) > nativeLaneMaxPoints-total {
			return nil, fmt.Errorf("isometric route lanes exceed point limit")
		}
		total += len(edge.Points)
		if err := w.spend(len(edge.Points)); err != nil {
			return nil, err
		}
		clean := make([]Vec, 0, len(edge.Points))
		for _, p := range edge.Points {
			if !bridgeFinite(p.X) || !bridgeFinite(p.Y) || !bridgeFinite(p.Z) {
				return nil, fmt.Errorf("isometric lane route %q has invalid coordinates", edge.ID)
			}
			if len(clean) == 0 || nlen(nsub(p, clean[len(clean)-1])) > 1e-9 {
				clean = append(clean, p)
			}
		}
		paths[i] = bridgeStraightRuns(clean)
		parts[i] = make([]int, max(0, len(paths[i])-1))
		for j := range parts[i] {
			parts[i][j] = -1
		}
		if !bridgeVisible(edge) {
			continue
		}
		for j := 1; j < len(paths[i]); j++ {
			a, b := paths[i][j-1], paths[i][j]
			length := math.Hypot(b.X-a.X, b.Z-a.Z)
			if a.Y != b.Y || length < nativeLaneMinShared {
				continue
			}
			u := nv((b.X-a.X)/length, 0, (b.Z-a.Z)/length)
			if u.X < -1e-10 || math.Abs(u.X) <= 1e-10 && u.Z < 0 {
				u = nmul(u, -1)
			}
			normal := nv(-u.Z, 0, u.X)
			line := ndot(a, normal)
			key := nativeLaneKey{int64(math.Round(u.X * 1e8)), int64(math.Round(u.Z * 1e8)), int64(math.Round(line * 1e6)), int64(math.Round(a.Y * 1e6))}
			lo, hi := ndot(a, u), ndot(b, u)
			if lo > hi {
				lo, hi = hi, lo
			}
			at := len(segments)
			parts[i][j-1] = at
			segments = append(segments, nativeLaneSegment{edge: i, part: j - 1, a: a, b: b, u: u, normal: normal, lo: lo, hi: hi, line: line, length: length, key: key})
			groups[key] = append(groups[key], at)
			dir := nativeLaneDirection{key.x, key.z, key.y}
			parallel[dir] = append(parallel[dir], at)
		}
	}
	obstacles, err := nativeLaneObstacles(nodes, boards)
	if err != nil {
		return nil, err
	}
	keys := make([]nativeLaneKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.x != b.x {
			return a.x < b.x
		}
		if a.z != b.z {
			return a.z < b.z
		}
		if a.y != b.y {
			return a.y < b.y
		}
		return a.normal < b.normal
	})
	offsets := make([]float64, len(segments))
	processed := make([]bool, len(segments))
	resolved := make([][]Vec, len(segments))
	sharedCount := 0
	for _, key := range keys {
		indices := groups[key]
		sort.Slice(indices, func(i, j int) bool {
			a, b := segments[indices[i]], segments[indices[j]]
			if a.lo != b.lo {
				return a.lo < b.lo
			}
			if edges[a.edge].ID != edges[b.edge].ID {
				return edges[a.edge].ID < edges[b.edge].ID
			}
			return a.part < b.part
		})
		maxRadius := 0.
		for oi, ai := range indices {
			a := &segments[ai]
			maxRadius = max(maxRadius, nativeRouteRadius(edges[a.edge]))
			for _, bi := range indices[oi+1:] {
				if err := w.spend(1); err != nil {
					return nil, err
				}
				b := &segments[bi]
				if b.lo >= a.hi-nativeLaneMinShared {
					break
				}
				lo, hi := max(a.lo, b.lo), min(a.hi, b.hi)
				if a.edge == b.edge || hi-lo < nativeLaneMinShared || math.Abs(a.line-b.line) > 1e-7 || a.a.Y != b.a.Y {
					continue
				}
				oldCount := len(a.shared) + len(b.shared)
				a.shared = nativeLaneUnion(a.shared, nativeLaneInterval{lo, hi})
				b.shared = nativeLaneUnion(b.shared, nativeLaneInterval{lo, hi})
				sharedCount += len(a.shared) + len(b.shared) - oldCount
				if sharedCount > nativeLaneMaxPoints {
					return nil, fmt.Errorf("isometric route lanes exceed overlap interval limit")
				}
			}
		}
		step := 3.3*maxRadius + .035 // account for both visible paper casings
		sort.Slice(indices, func(i, j int) bool {
			a, b := segments[indices[i]], segments[indices[j]]
			if edges[a.edge].ID != edges[b.edge].ID {
				return edges[a.edge].ID < edges[b.edge].ID
			}
			return a.part < b.part
		})
		for _, si := range indices {
			s := segments[si]
			if len(s.shared) == 0 {
				processed[si] = true
				continue
			}
			for candidate := 0; candidate <= 2*int(nativeLaneMaxOffset/step); candidate++ {
				offset := float64((candidate+1)/2) * step
				if candidate%2 == 0 {
					offset = -offset
				}
				// Prefer the same physical side of an edge as it turns. A
				// canonical line normal alone flips sides at half of the bends.
				offset *= nativeLaneSense(s)
				available := true
				for _, oi := range parallel[nativeLaneDirection{key.x, key.z, key.y}] {
					if err := w.spend(1); err != nil {
						return nil, err
					}
					o := segments[oi]
					if oi == si || o.edge == s.edge || min(o.hi, s.hi)-max(o.lo, s.lo) < nativeLaneMinShared {
						continue
					}
					gap := 1.65*(nativeRouteRadius(edges[s.edge])+nativeRouteRadius(edges[o.edge])) + .035
					if o.key == key {
						if processed[oi] && math.Abs(offset-offsets[oi]) < gap-1e-9 {
							available = false
							break
						}
					} else {
						// Do not collapse an existing neighboring line, or swap
						// across it while fanning out from the original run.
						other := o.line + offsets[oi]
						if offset != 0 && other > min(s.line, s.line+offset)-gap && other < max(s.line, s.line+offset)+gap {
							available = false
							break
						}
					}
				}
				if !available {
					continue
				}
				if offset == 0 {
					break
				}
				path := nativeLanePath(s, offset)
				if len(path) < 2 {
					continue
				}
				if err := w.spend(len(path) * 8); err != nil {
					return nil, err
				}
				clear, err := nativeLaneClear(&w, nativeRoundedRoute(path), obstacles, edges[s.edge])
				if err != nil {
					return nil, err
				}
				if !clear {
					continue
				}
				if len(path) > nativeLaneMaxPoints-total {
					return nil, fmt.Errorf("isometric route lanes exceed resolved point limit")
				}
				total += len(path)
				offsets[si], resolved[si] = offset, path
				break
			}
			processed[si] = true
		}
	}
	if err := nativeLaneJoinCorners(&w, paths, parts, segments, offsets, resolved, obstacles, edges); err != nil {
		return nil, err
	}
	for i, path := range paths {
		if len(path) < 2 {
			continue
		}
		out := []Vec{path[0]}
		for j, si := range parts[i] {
			if si >= 0 && len(resolved[si]) > 0 {
				out = append(out, resolved[si][1:]...)
			} else {
				out = append(out, path[j+1])
			}
		}
		if len(out) > 10000 {
			return nil, fmt.Errorf("isometric lane route exceeds 10000 control points")
		}
		paths[i] = out
	}
	return paths, ctx.Err()
}

// Overlap intervals arrive in ascending start order from the line sweep.
func nativeLaneUnion(spans []nativeLaneInterval, next nativeLaneInterval) []nativeLaneInterval {
	if len(spans) > 0 && next.lo <= spans[len(spans)-1].hi+.4 {
		spans[len(spans)-1].hi = max(spans[len(spans)-1].hi, next.hi)
		return spans
	}
	return append(spans, next)
}

func nativeLanePath(s nativeLaneSegment, offset float64) []Vec {
	guard, taper := .16, .22+math.Abs(offset)*.7
	intervals := make([]nativeLaneInterval, 0, len(s.shared))
	forward := ndot(nsub(s.b, s.a), s.u) > 0
	for _, shared := range s.shared {
		lo, hi := shared.lo-s.lo, shared.hi-s.lo
		if !forward {
			lo, hi = s.length-hi, s.length-lo
		}
		lo, hi = max(guard+taper, lo), min(s.length-guard-taper, hi)
		if hi < lo {
			continue
		}
		intervals = append(intervals, nativeLaneInterval{lo, hi})
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].lo < intervals[j].lo })
	merged := []nativeLaneInterval{}
	for _, span := range intervals {
		if len(merged) > 0 && span.lo-merged[len(merged)-1].hi < 2*taper+.1 {
			merged[len(merged)-1].hi = max(merged[len(merged)-1].hi, span.hi)
		} else {
			merged = append(merged, span)
		}
	}
	if len(merged) == 0 {
		return nil
	}
	out := []Vec{s.a}
	add := func(distance, shift float64) {
		p := nadd(nlerp(s.a, s.b, distance/s.length), nmul(s.normal, shift))
		p.Y = s.a.Y
		out = append(out, p)
	}
	for _, span := range merged {
		add(span.lo-taper, 0)
		add(span.lo, offset)
		if span.hi > span.lo {
			add(span.hi, offset)
		}
		add(span.hi+taper, 0)
	}
	return append(out, s.b)
}

func nativeLaneObstacles(nodes []d2isometric.Node, boards []d2isometric.Board) ([]nativeLaneObstacle, error) {
	out := make([]nativeLaneObstacle, 0, len(nodes)+len(boards))
	for _, n := range nodes {
		if n.Container || n.Opacity <= 0 {
			continue
		}
		if !bridgeFinite(n.Position.X) || !bridgeFinite(n.Position.Z) || !bridgeFinite(n.Size.X) || !bridgeFinite(n.Size.Z) || n.Size.X < 0 || n.Size.Z < 0 {
			return nil, fmt.Errorf("isometric lane obstacle %q has invalid bounds", n.ID)
		}
		out = append(out, nativeLaneObstacle{id: n.ID, left: n.Position.X - n.Size.X/2 - .08, right: n.Position.X + n.Size.X/2 + .08, front: n.Position.Z - n.Size.Z/2 - .17, back: n.Position.Z + n.Size.Z/2 + .17})
	}
	for _, b := range boards {
		if b.Label == "" {
			continue
		}
		if !bridgeFinite(b.Position.X) || !bridgeFinite(b.Position.Z) || !bridgeFinite(b.Size.X) || !bridgeFinite(b.Size.Z) || !bridgeFinite(b.HeaderDepth) || b.Size.X < 0 || b.Size.Z < 0 || b.HeaderDepth < 0 {
			return nil, fmt.Errorf("isometric lane board %q has invalid bounds", b.ID)
		}
		out = append(out, nativeLaneObstacle{id: b.ID, header: true, left: b.Position.X - b.Size.X/2 + .2, right: b.Position.X + b.Size.X/2 - .2, front: b.Position.Z - b.Size.Z/2 + .07, back: b.Position.Z - b.Size.Z/2 + b.HeaderDepth})
	}
	return out, nil
}

func nativeLaneClear(w *nativeLaneWork, points []Vec, obstacles []nativeLaneObstacle, edge d2isometric.Edge) (bool, error) {
	padding := 1.65*nativeRouteRadius(edge) + .02
	for _, o := range obstacles {
		// Only the connected exit/entry stub may touch its own module. A
		// later detour alongside that module is an ordinary obstacle check.
		startThrough, endFrom := 0, len(points)
		if !o.header && len(points) > 0 && len(edge.Points) > 0 {
			inside := func(p Vec) bool {
				return p.X > o.left-padding && p.X < o.right+padding && p.Z > o.front-padding && p.Z < o.back+padding
			}
			if o.id == edge.Source && points[0] == edge.Points[0] && inside(points[0]) {
				for i := 1; i < len(points); i++ {
					if err := w.spend(1); err != nil {
						return false, err
					}
					if !inside(points[i]) {
						startThrough = i
						break
					}
				}
			}
			if o.id == edge.Target && points[len(points)-1] == edge.Points[len(edge.Points)-1] && inside(points[len(points)-1]) {
				for i := len(points) - 2; i >= 0; i-- {
					if err := w.spend(1); err != nil {
						return false, err
					}
					if !inside(points[i]) {
						endFrom = i + 1
						break
					}
				}
			}
		}
		for i := 1; i < len(points); i++ {
			if err := w.spend(1); err != nil {
				return false, err
			}
			if i <= startThrough || i >= endFrom {
				continue
			}
			if nativeLaneHitsBox(points[i-1], points[i], o, padding) {
				return false, nil
			}
		}
	}
	return true, nil
}

func nativeLaneSense(s nativeLaneSegment) float64 {
	if ndot(nsub(s.b, s.a), s.u) < 0 {
		return -1
	}
	return 1
}

type nativeLaneCorner struct{ x, y, z int64 }
type nativeLaneJoint struct{ before, after int }

// Preserve a lane through a shared bend when its two offset lines have a
// compact intersection and their ordering agrees with neighboring lanes.
// Unsafe joins keep the already checked fan-out rather than forming a miter
// spike, crossing a component, or exchanging the order of two wires.
func nativeLaneJoinCorners(w *nativeLaneWork, paths [][]Vec, parts [][]int, segments []nativeLaneSegment, offsets []float64, resolved [][]Vec, obstacles []nativeLaneObstacle, edges []d2isometric.Edge) error {
	cornerKey := func(p Vec) nativeLaneCorner {
		return nativeLaneCorner{int64(math.Round(p.X * 1e7)), int64(math.Round(p.Y * 1e7)), int64(math.Round(p.Z * 1e7))}
	}
	joints := map[nativeLaneCorner][]nativeLaneJoint{}
	for i, path := range paths {
		for j := 1; j < len(path)-1; j++ {
			if err := w.spend(1); err != nil {
				return err
			}
			ai, bi := parts[i][j-1], parts[i][j]
			if ai >= 0 && bi >= 0 {
				key := cornerKey(path[j])
				joints[key] = append(joints[key], nativeLaneJoint{ai, bi})
			}
		}
	}
	order := make([]int, len(edges))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return edges[order[i]].ID < edges[order[j]].ID })
	for _, ei := range order {
		for j := 1; j < len(paths[ei])-1; j++ {
			if err := w.spend(1); err != nil {
				return err
			}
			ai, bi := parts[ei][j-1], parts[ei][j]
			if ai < 0 || bi < 0 || offsets[ai] == 0 || offsets[bi] == 0 || len(resolved[ai]) < 4 || len(resolved[bi]) < 4 {
				continue
			}
			a, b := segments[ai], segments[bi]
			if a.b != b.a || a.a.Y != b.b.Y || offsets[ai]*nativeLaneSense(a)*offsets[bi]*nativeLaneSense(b) <= 0 {
				continue
			}
			touches := func(s nativeLaneSegment, end bool) bool {
				high := (nativeLaneSense(s) > 0) == end
				for _, span := range s.shared {
					if high && span.hi >= s.hi-1e-7 || !high && span.lo <= s.lo+1e-7 {
						return true
					}
				}
				return false
			}
			if err := w.spend(len(a.shared) + len(b.shared)); err != nil {
				return err
			}
			if !touches(a, true) || !touches(b, false) {
				continue
			}
			ordered := true
			for _, other := range joints[cornerKey(a.b)] {
				if err := w.spend(1); err != nil {
					return err
				}
				if segments[other.before].edge == ei {
					continue
				}
				oa, ob := other.before, other.after
				if segments[oa].key == b.key && segments[ob].key == a.key {
					oa, ob = ob, oa
				}
				if segments[oa].key != a.key || segments[ob].key != b.key {
					continue
				}
				d1, d2 := (offsets[ai]-offsets[oa])*nativeLaneSense(a), (offsets[bi]-offsets[ob])*nativeLaneSense(b)
				if d1*d2 < -1e-9 {
					ordered = false
					break
				}
			}
			if !ordered {
				continue
			}
			u, v := nunit(nsub(a.b, a.a)), nunit(nsub(b.b, b.a))
			det := u.X*v.Z - u.Z*v.X
			if math.Abs(det) < .35 {
				continue
			}
			p, q := nadd(a.b, nmul(a.normal, offsets[ai])), nadd(b.a, nmul(b.normal, offsets[bi]))
			delta := nsub(q, p)
			m := nadd(p, nmul(u, (delta.X*v.Z-delta.Z*v.X)/det))
			m.Y = a.b.Y
			if nlen(nsub(m, a.b)) > min(.85, 3*max(math.Abs(offsets[ai]), math.Abs(offsets[bi]))) {
				continue
			}
			left, right := resolved[ai], resolved[bi]
			li, ri := len(left)-2, 1
			for li >= 1 && math.Abs(ndot(nsub(left[li], a.a), a.normal)-offsets[ai]) > 1e-7 {
				li--
			}
			for ri < len(right)-1 && math.Abs(ndot(nsub(right[ri], b.a), b.normal)-offsets[bi]) > 1e-7 {
				ri++
			}
			if li < 1 || ri >= len(right)-1 || ndot(nsub(m, left[li]), u) < .03 || ndot(nsub(right[ri], m), v) < .03 {
				continue
			}
			candidate := []Vec{left[li-1], left[li], m, right[ri], right[ri+1]}
			clear, err := nativeLaneClear(w, nativeRoundedRoute(candidate), obstacles, edges[ei])
			if err != nil {
				return err
			}
			if !clear {
				continue
			}
			resolved[ai] = append(append([]Vec(nil), left[:li+1]...), m)
			resolved[bi] = append([]Vec{m}, right[ri:]...)
		}
	}
	return nil
}

func nativeLaneHitsBox(a, b Vec, o nativeLaneObstacle, padding float64) bool {
	lo, hi := 0., 1.
	for _, axis := range [][4]float64{{a.X, b.X - a.X, o.left - padding, o.right + padding}, {a.Z, b.Z - a.Z, o.front - padding, o.back + padding}} {
		if math.Abs(axis[1]) < 1e-12 {
			if axis[0] <= axis[2]+1e-9 || axis[0] >= axis[3]-1e-9 {
				return false
			}
			continue
		}
		x, y := (axis[2]-axis[0])/axis[1], (axis[3]-axis[0])/axis[1]
		if x > y {
			x, y = y, x
		}
		lo, hi = max(lo, x), min(hi, y)
		if hi-lo <= 1e-9 {
			return false
		}
	}
	return hi-lo > 1e-9
}
