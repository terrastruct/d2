package d2isometricimg

import (
	"math"
	"sort"
)

const (
	maxRouteCaptionRects      = 19000
	maxRouteCaptionCandidates = 192
	maxRouteCaptionWork       = 2_000_000
	maxRouteCaptionGridRefs   = 250_000
	maxRouteCaptionCells      = 128
	routeCaptionGap           = .055
)

type routeCaptionCell struct{ x, z int }

// All obstacles share the camera's projection onto Y=0. This retains the
// footprint's orientation while accounting for different surface elevations.
// The grid is only a broad phase; separating axes reject false intersections.
type routeCaptionRect struct {
	x, z, ux, uz, hw, hd     float64
	left, right, front, back float64
	hard                     bool
	hull                     []routeCaptionPoint
}

type routeCaptionPlacer struct {
	rects      []routeCaptionRect
	grid       map[routeCaptionCell][]int
	wide       []int
	work, refs int
}

func newRouteCaptionPlacer() *routeCaptionPlacer {
	return &routeCaptionPlacer{grid: make(map[routeCaptionCell][]int)}
}

// Avoid reserves an axis-aligned footprint, for example a visible component or
// board heading. It changes caption placement only, never the underlying route.
func (p *routeCaptionPlacer) Avoid(center Vec, width, depth float64) {
	if !captionFinite(center.X, center.Y, center.Z, width, depth) || width <= 0 || depth <= 0 {
		return
	}
	p.reserve(captionRect(labelSurface{center: center, width: width, depth: depth}, true))
}

type routeCaptionLeg struct {
	a, b          Vec
	start, length float64
}

// Place keeps the supplied print dimensions and searches straight legs, along-
// leg offsets, then both sides of a route. Existing caption rectangles and any
// Avoid footprints are considered. Work exhaustion returns the best checked
// candidate (or the original anchor), rather than hiding or shrinking a label.
func (p *routeCaptionPlacer) Place(points []Vec, fraction, width, depth float64) labelSurface {
	if len(points) < 2 || !captionFinite(fraction, width, depth) || width <= 0 || depth <= 0 {
		return labelSurface{}
	}
	if !captionFinite(points[0].Y) {
		return labelSurface{}
	}
	fraction = math.Max(0, math.Min(1, fraction))
	if len(points) > 10000 {
		points = []Vec{points[0], points[len(points)-1]}
	}
	if len(p.rects) >= maxRouteCaptionRects || p.work+len(points) > maxRouteCaptionWork {
		return captionFallback(points, fraction, width, depth)
	}
	p.work += len(points)
	legs, total := captionLegs(points)
	if len(legs) == 0 {
		return labelSurface{}
	}
	target := total * fraction
	weight := .35
	if fraction != .5 {
		weight = 3
	}
	sort.SliceStable(legs, func(i, j int) bool {
		score := func(l routeCaptionLeg) float64 { return l.length - math.Abs(l.start+l.length/2-target)*weight }
		return score(legs[i]) > score(legs[j])
	})
	anchor := func(l routeCaptionLeg) float64 {
		if fraction == .5 {
			return l.length / 2
		}
		return math.Max(l.length*.15, math.Min(l.length*.85, target-l.start))
	}
	normalOffset := math.Max(.25, depth/2+.075)
	fallback := captionOnLeg(legs[0], anchor(legs[0]), normalOffset, width, depth, points[0].Y)
	best, bestScore := fallback, math.Inf(1)
	candidates := 0
	for legIndex, leg := range legs {
		if legIndex >= 8 || candidates >= maxRouteCaptionCandidates || p.work >= maxRouteCaptionWork {
			break
		}
		inset := math.Min(.035, leg.length*.03)
		lo, hi := width/2+inset, leg.length-width/2-inset
		if lo > hi {
			continue
		}
		preferred := math.Max(lo, math.Min(hi, anchor(leg)))
		step := width + 2*routeCaptionGap
		positions := []float64{preferred, preferred - step, preferred + step, preferred - 2*step, preferred + 2*step, lo, hi}
		for _, offset := range []float64{normalOffset, -normalOffset, normalOffset + depth + .1, -normalOffset - depth - .1} {
			for _, along := range positions {
				if along < lo || along > hi || candidates >= maxRouteCaptionCandidates {
					continue
				}
				// Endpoint labels remain in their end of the route. If the original
				// long-leg heuristic itself lies outside this range, retain it as
				// the fallback rather than moving to the opposite endpoint.
				arc := (leg.start + along) / total
				if fraction < .25 && arc > .4 || fraction > .75 && arc < .6 {
					continue
				}
				candidate := captionOnLeg(leg, along, offset, width, depth, points[0].Y)
				candidates++
				score, complete := p.score(captionRect(candidate, false))
				if !complete {
					p.reserve(captionRect(best, false))
					return best
				}
				if score < bestScore {
					best, bestScore = candidate, score
				}
				if score == 0 {
					p.reserve(captionRect(candidate, false))
					return candidate
				}
			}
		}
	}
	p.reserve(captionRect(best, false))
	return best
}

func captionFinite(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e9 {
			return false
		}
	}
	return true
}

func captionLegs(points []Vec) ([]routeCaptionLeg, float64) {
	legs := make([]routeCaptionLeg, 0, min(len(points)-1, 32))
	total := 0.
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		if !captionFinite(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
			continue
		}
		length := math.Hypot(b.X-a.X, b.Z-a.Z)
		if length < 1e-9 {
			continue
		}
		if len(legs) > 0 {
			last := &legs[len(legs)-1]
			x, z := last.b.X-last.a.X, last.b.Z-last.a.Z
			x2, z2 := b.X-a.X, b.Z-a.Z
			if math.Hypot(last.b.X-a.X, last.b.Z-a.Z) < 1e-9 && math.Abs(x*z2-z*x2) <= 1e-9*last.length*length && x*x2+z*z2 > 0 {
				last.b, last.length = b, last.length+length
				total += length
				continue
			}
		}
		legs = append(legs, routeCaptionLeg{a: a, b: b, start: total, length: length})
		total += length
	}
	return legs, total
}

func captionFallback(points []Vec, fraction, width, depth float64) labelSurface {
	// Even after exhausting the shared budget, inspect only a bounded local
	// neighborhood and retain the text on a real route leg, with its full size.
	center := min(len(points)-2, int(fraction*float64(len(points)-1)))
	for step := 0; step < 32; step++ {
		i := center + (step+1)/2
		if step%2 != 0 {
			i = center - (step+1)/2
		}
		if i < 0 || i+1 >= len(points) {
			continue
		}
		a, b := points[i], points[i+1]
		length := math.Hypot(b.X-a.X, b.Z-a.Z)
		if length > 1e-9 && captionFinite(a.X, a.Z, b.X, b.Z) {
			return captionOnLeg(routeCaptionLeg{a: a, b: b, length: length}, length/2, math.Max(.25, depth/2+.075), width, depth, points[0].Y)
		}
	}
	return labelSurface{}
}

func captionOnLeg(l routeCaptionLeg, along, offset, width, depth, y float64) labelSurface {
	ux, uz := (l.b.X-l.a.X)/l.length, (l.b.Z-l.a.Z)/l.length
	x, z := l.a.X+ux*along, l.a.Z+uz*along
	// Reversing an edge does not turn its text upside down.
	if ux < -1e-6 || math.Abs(ux) < 1e-6 && uz < 0 {
		ux, uz = -ux, -uz
	}
	return labelSurface{center: Vec{X: x + uz*offset, Y: y + .006, Z: z - ux*offset}, width: width, depth: depth, angle: math.Atan2(uz, ux)}
}

func captionRect(s labelSurface, hard bool) routeCaptionRect {
	center := captionProjection(s.center)
	r := routeCaptionRect{x: center.x, z: center.z, ux: math.Cos(s.angle), uz: math.Sin(s.angle), hw: s.width/2 + routeCaptionGap, hd: s.depth/2 + routeCaptionGap, hard: hard}
	x, z := math.Abs(r.ux)*r.hw+math.Abs(r.uz)*r.hd, math.Abs(r.uz)*r.hw+math.Abs(r.ux)*r.hd
	r.left, r.right, r.front, r.back = r.x-x, r.x+x, r.z-z, r.z+z
	return r
}

func captionOverlap(a, b routeCaptionRect) bool {
	if a.right <= b.left || b.right <= a.left || a.back <= b.front || b.back <= a.front {
		return false
	}
	if len(a.hull) > 0 || len(b.hull) > 0 {
		return captionHullOverlap(a, b)
	}
	dx, dz := b.x-a.x, b.z-a.z
	for _, axis := range [][2]float64{{a.ux, a.uz}, {-a.uz, a.ux}, {b.ux, b.uz}, {-b.uz, b.ux}} {
		project := func(r routeCaptionRect) float64 {
			return r.hw*math.Abs(r.ux*axis[0]+r.uz*axis[1]) + r.hd*math.Abs(-r.uz*axis[0]+r.ux*axis[1])
		}
		if math.Abs(dx*axis[0]+dz*axis[1]) >= project(a)+project(b) {
			return false
		}
	}
	return true
}

func captionCells(r routeCaptionRect) (int, int, int, int, bool) {
	if !captionFinite(r.left, r.right, r.front, r.back) {
		return 0, 0, 0, 0, false
	}
	x0, x1, z0, z1 := int(math.Floor(r.left)), int(math.Floor(r.right)), int(math.Floor(r.front)), int(math.Floor(r.back))
	nx, nz := int64(x1)-int64(x0)+1, int64(z1)-int64(z0)+1
	return x0, x1, z0, z1, nx > 0 && nz > 0 && nx <= maxRouteCaptionCells && nz <= maxRouteCaptionCells && nx*nz <= maxRouteCaptionCells
}

func (p *routeCaptionPlacer) reserve(r routeCaptionRect) {
	if len(p.rects) >= maxRouteCaptionRects {
		return
	}
	id := len(p.rects)
	p.rects = append(p.rects, r)
	x0, x1, z0, z1, small := captionCells(r)
	if !small || p.refs+(x1-x0+1)*(z1-z0+1) > maxRouteCaptionGridRefs {
		p.wide = append(p.wide, id)
		return
	}
	for x := x0; x <= x1; x++ {
		for z := z0; z <= z1; z++ {
			key := routeCaptionCell{x, z}
			p.grid[key] = append(p.grid[key], id)
			p.refs++
		}
	}
}

func (p *routeCaptionPlacer) score(r routeCaptionRect) (float64, bool) {
	seen := make(map[int]bool)
	score := 0.
	check := func(id int) bool {
		if p.work >= maxRouteCaptionWork {
			return false
		}
		p.work++
		if seen[id] {
			return true
		}
		seen[id] = true
		other := p.rects[id]
		// Mesh silhouettes have more separating axes than rectangles. Charge
		// their actual narrow-phase size so detailed artwork cannot bypass
		// the shared placement work limit.
		if (len(r.hull) > 0 || len(other.hull) > 0) && r.right > other.left && other.right > r.left && r.back > other.front && other.back > r.front {
			vertices := max(4, len(r.hull)) + max(4, len(other.hull))
			cost := vertices * vertices
			if p.work+cost > maxRouteCaptionWork {
				return false
			}
			p.work += cost
		}
		if captionOverlap(r, other) {
			penalty := 1.
			if other.hard {
				penalty = 10000
			}
			score += penalty
		}
		return true
	}
	x0, x1, z0, z1, small := captionCells(r)
	if !small {
		for id := range p.rects {
			if !check(id) {
				return score, false
			}
		}
		return score, true
	}
	for _, id := range p.wide {
		if !check(id) {
			return score, false
		}
	}
	for x := x0; x <= x1; x++ {
		for z := z0; z <= z1; z++ {
			if p.work >= maxRouteCaptionWork {
				return score, false
			}
			p.work++
			for _, id := range p.grid[routeCaptionCell{x, z}] {
				if !check(id) {
					return score, false
				}
			}
		}
	}
	return score, true
}
