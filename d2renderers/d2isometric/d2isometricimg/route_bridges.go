package d2isometricimg

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

const (
	nativeBridgeMaxPoints = 350000
	nativeBridgeMaxWork   = 20000000
)

type nativeBridgeSegment struct {
	edge, part int
	a, b       Vec
	length     float64
	minX, maxX float64
	minZ, maxZ float64
}

type nativeBridgeCrossing struct {
	distance, clearance float64
	under               int
	underDistance       float64
}

type nativeBridgeSpan struct {
	first, last, half, height float64
}

type nativeBridgeWork struct {
	ctx       context.Context
	remaining int
}

func (w *nativeBridgeWork) spend(n int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if n > w.remaining {
		return fmt.Errorf("isometric crossing bridges exceed work limit")
	}
	w.remaining -= n
	return nil
}

// nativeBridgeRoutes returns independent, rounded paths in the input edge order.
// Only true transverse intersections in straight, flat route interiors receive
// local bridges. Ports, curved corners, tangencies, and overlapping runs retain
// their original elevation. Edge IDs establish a stable lower-to-upper order.
// Callers use these same paths for solid/dashed tubes and animated packets.
func nativeBridgeRoutes(ctx context.Context, edges []d2isometric.Edge) ([][]Vec, error) {
	return nativeResolveBridgeRoutes(ctx, edges, nativeBridgeMaxWork)
}

func nativeResolveBridgeRoutes(ctx context.Context, edges []d2isometric.Edge, limit int) ([][]Vec, error) {
	if ctx == nil {
		return nil, fmt.Errorf("isometric crossing bridges require a context")
	}
	if len(edges) > nativeBridgeMaxPoints {
		return nil, fmt.Errorf("isometric crossing bridges exceed edge limit")
	}
	w := nativeBridgeWork{ctx: ctx, remaining: limit}
	if err := w.spend(len(edges)); err != nil {
		return nil, err
	}
	paths := make([][]Vec, len(edges))
	ids := make(map[string]bool, len(edges))
	segments := []nativeBridgeSegment{}
	parts := make([][]int, len(edges))
	total := 0
	for i, edge := range edges {
		if ids[edge.ID] {
			return nil, fmt.Errorf("isometric crossing bridges require unique edge IDs: %q", edge.ID)
		}
		ids[edge.ID] = true
		if len(edge.Points) > 10000 || len(edge.Points) > (nativeBridgeMaxPoints-total)/8 {
			return nil, fmt.Errorf("isometric crossing bridges exceed route point limit")
		}
		if err := w.spend(len(edge.Points)); err != nil {
			return nil, err
		}
		for _, p := range edge.Points {
			if !bridgeFinite(p.X) || !bridgeFinite(p.Y) || !bridgeFinite(p.Z) {
				return nil, fmt.Errorf("isometric crossing bridge route %q has invalid coordinates", edge.ID)
			}
		}
		paths[i] = bridgeStraightRuns(nativeRoundedRoute(edge.Points))
		total += len(paths[i])
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
			if length < .42 || a.Y != b.Y {
				continue
			}
			parts[i][j-1] = len(segments)
			segments = append(segments, nativeBridgeSegment{i, j - 1, a, b, length, min(a.X, b.X), max(a.X, b.X), min(a.Z, b.Z), max(a.Z, b.Z)})
		}
	}
	// An X sweep discards disjoint ranges before exact intersection work. The
	// budget also covers adversarial sets whose ranges all overlap.
	order := make([]int, len(segments))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := segments[order[i]], segments[order[j]]
		if a.minX != b.minX {
			return a.minX < b.minX
		}
		if edges[a.edge].ID != edges[b.edge].ID {
			return edges[a.edge].ID < edges[b.edge].ID
		}
		return a.part < b.part
	})
	crossings := make([][]nativeBridgeCrossing, len(segments))
	for oi, ai := range order {
		a := segments[ai]
		for _, bi := range order[oi+1:] {
			if err := w.spend(1); err != nil {
				return nil, err
			}
			b := segments[bi]
			if b.minX > a.maxX {
				break
			}
			if a.edge == b.edge || a.maxZ < b.minZ || b.maxZ < a.minZ || a.a.Y != b.a.Y {
				continue
			}
			at, bt, ok := bridgeIntersection(a, b)
			if !ok {
				continue
			}
			upper, lower, ut, lt := ai, bi, at, bt
			if edges[a.edge].ID < edges[b.edge].ID {
				upper, lower, ut, lt = bi, ai, bt, at
			}
			clearance := nativeRouteRadius(edges[a.edge]) + nativeRouteRadius(edges[b.edge]) + .05
			half := max(.42, clearance*3) / 2
			u, l := segments[upper], segments[lower]
			// Space beyond each ramp avoids altering a port's arrowhead or a
			// rounded corner. The lower path must also cross in its interior.
			if ut*u.length < half+.12 || (1-ut)*u.length < half+.12 || lt*l.length < .12 || (1-lt)*l.length < .12 {
				continue
			}
			crossings[upper] = append(crossings[upper], nativeBridgeCrossing{ut * u.length, clearance, lower, lt * l.length})
		}
	}
	// Resolve lower paths first. Nearby crossings share one plateau instead of
	// repeatedly rising and falling. Multi-way crossings clear the lower bridge.
	sort.Slice(order, func(i, j int) bool {
		a, b := segments[order[i]], segments[order[j]]
		if edges[a.edge].ID != edges[b.edge].ID {
			return edges[a.edge].ID < edges[b.edge].ID
		}
		return a.part < b.part
	})
	spans := make([][]nativeBridgeSpan, len(segments))
	for _, si := range order {
		if err := w.spend(len(crossings[si]) + 1); err != nil {
			return nil, err
		}
		cs := crossings[si]
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].distance != cs[j].distance {
				return cs[i].distance < cs[j].distance
			}
			return edges[segments[cs[i].under].edge].ID < edges[segments[cs[j].under].edge].ID
		})
		for _, c := range cs {
			if err := w.spend(len(spans[c.under]) + 1); err != nil {
				return nil, err
			}
			height := c.clearance + bridgeHeight(spans[c.under], c.underDistance)
			half := max(.42, height*3) / 2
			if c.distance-half < .12 || c.distance+half > segments[si].length-.12 {
				continue
			}
			span := nativeBridgeSpan{c.distance, c.distance, half, height}
			ss := spans[si]
			if len(ss) > 0 && span.first-span.half <= ss[len(ss)-1].last+ss[len(ss)-1].half+.12 {
				p := ss[len(ss)-1]
				p.last = span.last
				p.height = max(p.height, span.height)
				p.half = max(p.half, span.half)
				if p.first-p.half < .12 || p.last+p.half > segments[si].length-.12 {
					continue
				}
				ss[len(ss)-1] = p
			} else {
				ss = append(ss, span)
			}
			spans[si] = ss
		}
	}
	for i, path := range paths {
		if len(path) < 2 {
			continue
		}
		out := make([]Vec, 0, len(path))
		out = append(out, path[0])
		for j, si := range parts[i] {
			if si >= 0 {
				for _, span := range spans[si] {
					if err := w.spend(10); err != nil {
						return nil, err
					}
					if total > nativeBridgeMaxPoints-10 {
						return nil, fmt.Errorf("isometric crossing bridges exceed resolved point limit")
					}
					total += 10
					segment := segments[si]
					add := func(d, lift float64) {
						p := nlerp(segment.a, segment.b, d/segment.length)
						p.Y = segment.a.Y + lift
						out = append(out, p)
					}
					for k := 0; k <= 4; k++ {
						t := float64(k) / 4
						add(span.first-span.half+t*span.half, span.height*bridgeEase(t))
					}
					if span.last != span.first {
						add(span.last, span.height)
					}
					for k := 1; k <= 4; k++ {
						t := float64(k) / 4
						add(span.last+t*span.half, span.height*bridgeEase(1-t))
					}
				}
			}
			out = append(out, path[j+1])
		}
		paths[i] = out
	}
	return paths, ctx.Err()
}

func bridgeFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= 1e9 }

func bridgeVisible(e d2isometric.Edge) bool {
	return e.Opacity > 0 && e.StrokeWidth > 0 && (!e.StrokeExplicit || nativePaint(e.Stroke, "#657e9e").A > 0)
}

func bridgeStraightRuns(points []Vec) []Vec {
	out := make([]Vec, 0, len(points))
	for _, p := range points {
		for len(out) >= 2 {
			a, b := out[len(out)-2], out[len(out)-1]
			u, v := nsub(b, a), nsub(p, b)
			if ndot(u, v) <= 0 || nlen(ncross(u, v)) > 1e-12*nlen(u)*nlen(v) {
				break
			}
			out = out[:len(out)-1]
		}
		out = append(out, p)
	}
	return out
}

func bridgeIntersection(a, b nativeBridgeSegment) (float64, float64, bool) {
	ax, az, bx, bz := a.b.X-a.a.X, a.b.Z-a.a.Z, b.b.X-b.a.X, b.b.Z-b.a.Z
	det := ax*bz - az*bx
	// A shallow crossing needs a long flyover to physically clear the lower
	// tube. This local treatment intentionally leaves those routes unchanged.
	if math.Abs(det) < .75*a.length*b.length {
		return 0, 0, false
	}
	dx, dz := b.a.X-a.a.X, b.a.Z-a.a.Z
	t, u := (dx*bz-dz*bx)/det, (dx*az-dz*ax)/det
	return t, u, t > 1e-8 && t < 1-1e-8 && u > 1e-8 && u < 1-1e-8
}

func bridgeEase(t float64) float64 { return .5 - .5*math.Cos(math.Pi*t) }

// Interpolate the actual four-segment ramp, so a crossing over an earlier
// bridge clears the same geometry consumed by tubes and traffic packets.
func bridgeHeight(spans []nativeBridgeSpan, distance float64) float64 {
	for _, s := range spans {
		if distance < s.first-s.half || distance > s.last+s.half {
			continue
		}
		t := min(1, min((distance-s.first+s.half)/s.half, (s.last+s.half-distance)/s.half))
		lo := min(3, int(t*4))
		f := t*4 - float64(lo)
		return s.height * (bridgeEase(float64(lo)/4)*(1-f) + bridgeEase(float64(lo+1)/4)*f)
	}
	return 0
}
