package d2isometricimg

import (
	"fmt"
	"math"
	"sort"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

// An endpoint owns only its own solid, never its containing platforms or
// descendants. Ranges refer to completed geometry before routes are appended.
type nativeArrowOwner struct {
	first, last int
	node        d2isometric.Node
	board       *d2isometric.Board
	caps        []Triangle
	prepared    bool
	obstacles   map[float64][]nativeArrowObstacle
}

type nativeArrowMarkerKey struct {
	kind  d2target.Arrowhead
	width int
}

// Query caches are local to mesh construction and bounded independently of
// output geometry. Work counts projection-axis comparisons, including misses.
const maxNativeArrowWork = 100_000_000
const maxNativeArrowCachePoints = 1_000_000

func (b *meshBuilder) arrowBudget(work int) bool {
	if b.err != nil {
		return false
	}
	if b.ctx != nil && b.ctx.Err() != nil {
		b.err = b.ctx.Err()
		return false
	}
	b.arrowWork += int64(work)
	if b.arrowWork > maxNativeArrowWork {
		b.err = fmt.Errorf("isometric endpoint visibility exceeds bounded query work")
		return false
	}
	return true
}

type nativeArrowMarker struct {
	vertices    []Vec
	minY, reach float64
}

func (b *meshBuilder) rememberArrowOwner(node d2isometric.Node, board *d2isometric.Board, first int) {
	if node.SequenceRole != "" || node.Opacity <= 0 {
		return
	}
	if b.arrowOwners == nil {
		b.arrowOwners = make(map[string]*nativeArrowOwner)
	}
	b.arrowOwners[node.ID] = &nativeArrowOwner{first: first, last: len(b.triangles), node: node, board: board}
}

func (b *meshBuilder) arrowMarker(kind d2target.Arrowhead, width int) nativeArrowMarker {
	key := nativeArrowMarkerKey{kind, width}
	if marker, ok := b.arrowMarkers[key]; ok {
		return marker
	}
	q := &meshBuilder{ctx: b.ctx, scale: b.scale, arrowBackground: b.arrowBackground}
	q.arrowWithOpacity(string(kind), Vec{}, nv(1, 0, 0), nativeMaterial("black", 0, 0, 1), width, 1)
	if q.err != nil {
		b.err = q.err
	}
	marker := nativeArrowMarker{minY: math.Inf(1)}
	for _, t := range q.triangles {
		for _, v := range t.V {
			marker.vertices = append(marker.vertices, v.Position)
			marker.minY = min(marker.minY, v.Position.Y+t.DepthBias*nativeViewDirection().Y)
			marker.reach = max(marker.reach, -v.Position.X)
		}
	}
	if len(b.arrowMarkers) < 64 {
		if b.arrowMarkers == nil {
			b.arrowMarkers = make(map[nativeArrowMarkerKey]nativeArrowMarker)
		}
		b.arrowMarkers[key] = marker
	}
	return marker
}

// Printed rectangular viewports are not shape silhouettes. Recreate only the
// exact filled source caps for the visibility query; they are never painted.
// This also makes the query identical in the raster and vector backends.
func (b *meshBuilder) arrowOwnerCaps(o *nativeArrowOwner) []Triangle {
	if o.prepared {
		return o.caps
	}
	o.prepared = true
	n := nativeClassicNode(o.node)
	if nativeMarkdownCard(n) {
		n = nativeMarkdownCardPaint(n)
	}
	// Only standalone solids have real textured cap geometry; source
	// containers of the same shape use their ordinary extruded profiles.
	if o.board == nil && nativeSolidNode(n) {
		return nil
	}
	scale := b.scale
	if scale <= 0 {
		scale = .01
	}
	q := &meshBuilder{ctx: b.ctx}
	add := func(s d2target.Shape, x, z, w, d, y float64, fill string) {
		if nativePaint(fill, "#dce5eb").A == 0 || w <= 0 || d <= 0 {
			return
		}
		ps, err := nativeShapeProfiles(s)
		if err != nil {
			b.err = err
			return
		}
		mat := nativeMaterial(fill, 0, 0, n.Opacity)
		for _, p := range ps {
			world := make([]Vec, len(p))
			for i, v := range p {
				world[i] = nv(x+v.X*w/float64(s.Width), y, z+v.Z*d/float64(s.Height))
			}
			q.extrudedProfile(world, y, mat, nil)
		}
	}
	floor := n.Position.Y - n.Size.Y/2
	height := nativeCanonicalHeight(n, scale)
	for copyIndex := 1; copyIndex >= 0; copyIndex-- {
		if copyIndex == 1 && !n.Metadata.Original.Multiple {
			continue
		}
		dx, dz, dy := float64(copyIndex)*d2target.MULTIPLE_OFFSET*scale, -float64(copyIndex)*d2target.MULTIPLE_OFFSET*scale, -float64(copyIndex)*min(.045, height*.25)
		if o.board != nil {
			board := *o.board
			n.Size = board.Size
			s := nativeFaceSource(n, n.Fill)
			add(s, board.Position.X-board.Size.X/2+dx, board.Position.Z-board.Size.Z/2+dz, board.Size.X, board.Size.Z, hierarchySurfaceY(board)-float64(copyIndex)*.00001, n.Fill)
		} else if nativeStructuredNode(n) {
			s := n.Metadata.Original
			sx, sz := n.Size.X/float64(s.Width), n.Size.Z/float64(s.Height)
			dx, dz = float64(copyIndex)*d2target.MULTIPLE_OFFSET*sx, -float64(copyIndex)*d2target.MULTIPLE_OFFSET*sz
			rail := func(z, depth, top float64, fill string) {
				if depth <= 0 {
					return
				}
				face := nativeFaceSource(n, fill)
				face.Type = d2target.ShapeRectangle
				face.Width, face.Height = s.Width, max(1, int(math.Round(depth)))
				face.BorderRadius = min(s.BorderRadius, face.Height/2)
				add(face, n.Position.X-n.Size.X/2+dx, n.Position.Z-n.Size.Z/2+dz+z*sz, n.Size.X, depth*sz, top+dy, fill)
			}
			rail(0, float64(s.Height), floor+.01, n.Fill)
			for i, r := range nativeStructuredRows(s) {
				top, fill := floor+height*.32/.46, n.Stroke
				if i == 0 {
					top, fill = floor+height, n.Fill
				}
				rail(r.z+r.back, r.depth-r.back-r.front, top, fill)
			}
		} else {
			s := nativeFaceSource(n, n.Fill)
			y := floor + (height+dy)*hierarchyNodeRelief(n)
			add(s, n.Position.X-n.Size.X/2+dx, n.Position.Z-n.Size.Z/2+dz, n.Size.X, n.Size.Z, y, n.Fill)
		}
	}
	if q.err != nil {
		b.err = q.err
	}
	o.caps = q.triangles
	return o.caps
}

// At an equal projected point, greater world Y is closer to this camera.
// Clipping here excludes lower platforms and the backs of solids below the
// marker plane instead of treating every silhouette overlap as occlusion.
func nativeArrowOccluder(t Triangle, minimumY float64) []routeCaptionPoint {
	// DepthBias is a camera-depth offset, equivalent to this Y difference at
	// equal projection. The footprint itself must remain in its physical place.
	minimumY -= t.DepthBias * nativeViewDirection().Y
	p := []Vec{t.V[0].Position, t.V[1].Position, t.V[2].Position}
	out := make([]Vec, 0, 4)
	for i, a := range p {
		c := p[(i+1)%len(p)]
		ain, cin := a.Y > minimumY+1e-6, c.Y > minimumY+1e-6
		if ain {
			out = append(out, a)
		}
		if ain != cin {
			out = append(out, nlerp(a, c, (minimumY+1e-6-a.Y)/(c.Y-a.Y)))
		}
	}
	if len(out) < 3 {
		return nil
	}
	projected := make([]routeCaptionPoint, len(out))
	for i, v := range out {
		projected[i] = captionProjection(v)
	}
	return captionConvexHull(projected)
}

type nativeArrowObstacle struct {
	points []routeCaptionPoint
	lo, hi routeCaptionPoint
}

func nativeArrowBounds(points []routeCaptionPoint) (routeCaptionPoint, routeCaptionPoint) {
	lo, hi := routeCaptionPoint{math.Inf(1), math.Inf(1)}, routeCaptionPoint{math.Inf(-1), math.Inf(-1)}
	for _, p := range points {
		lo.x = min(lo.x, p.x)
		lo.z = min(lo.z, p.z)
		hi.x = max(hi.x, p.x)
		hi.z = max(hi.z, p.z)
	}
	return lo, hi
}

func (b *meshBuilder) arrowObstacles(id string, minimumY float64) []nativeArrowObstacle {
	o := b.arrowOwners[id]
	if o == nil {
		return nil
	}
	if p, ok := o.obstacles[minimumY]; ok {
		return p
	}
	caps := b.arrowOwnerCaps(o)
	var polygons []nativeArrowObstacle
	pointCount := 0
	add := func(t Triangle) {
		if !b.arrowBudget(1) {
			return
		}
		if t.Material == nil || t.Material.Color.A == 0 || t.NoDepthWrite || t.svgCoverageOnly {
			return
		}
		// Source-texture viewports and labels include transparent margins. Exact
		// filled cap proxies replace them; solid ellipses have no viewport margins.
		if t.Material.Texture != nil && (o.board != nil || !nativeSolidNode(o.node) || t.Material.Unlit || nativePaint(o.node.Fill, "#dce5eb").A == 0) {
			return
		}
		p := nativeArrowOccluder(t, minimumY)
		if len(p) > 2 {
			lo, hi := nativeArrowBounds(p)
			polygons = append(polygons, nativeArrowObstacle{p, lo, hi})
			pointCount += len(p)
		}
	}
	for _, t := range b.triangles[o.first:o.last] {
		add(t)
	}
	for _, t := range caps {
		add(t)
	}
	if len(o.obstacles) < 8 && pointCount <= maxNativeArrowCachePoints-b.arrowCachePoints {
		if o.obstacles == nil {
			o.obstacles = make(map[float64][]nativeArrowObstacle)
		}
		o.obstacles[minimumY] = polygons
		b.arrowCachePoints += pointCount
	}
	return polygons
}

func (m nativeArrowMarker) footprint(at, away Vec) []routeCaptionPoint {
	along := nmul(away, -1)
	side := nv(-along.Z, 0, along.X)
	points := make([]routeCaptionPoint, 0, len(m.vertices))
	for _, v := range m.vertices {
		p := nadd(at, nadd(nmul(along, v.X), nmul(side, v.Z)))
		p.Y += v.Y
		points = append(points, captionProjection(p))
	}
	return captionConvexHull(points)
}

type nativeArrowInterval struct{ first, last float64 }

// Swept SAT gives exact overlap intervals for a translating marker footprint
// and each convex piece of the endpoint. Their union preserves concave gaps.
func nativeArrowOverlap(marker, obstacle []routeCaptionPoint, velocity routeCaptionPoint, length float64) (nativeArrowInterval, bool) {
	interval := nativeArrowInterval{0, length}
	for _, poly := range [][]routeCaptionPoint{marker, obstacle} {
		for i, a := range poly {
			c := poly[(i+1)%len(poly)]
			axis := routeCaptionPoint{-(c.z - a.z), c.x - a.x}
			if math.Abs(axis.x)+math.Abs(axis.z) < 1e-12 {
				continue
			}
			bounds := func(points []routeCaptionPoint) (float64, float64) {
				lo, hi := math.Inf(1), math.Inf(-1)
				for _, p := range points {
					v := p.x*axis.x + p.z*axis.z
					lo = min(lo, v)
					hi = max(hi, v)
				}
				return lo, hi
			}
			al, ah := bounds(marker)
			bl, bh := bounds(obstacle)
			speed := velocity.x*axis.x + velocity.z*axis.z
			if math.Abs(speed) < 1e-12 {
				if ah < bl || bh < al {
					return interval, false
				}
				continue
			}
			enter, exit := (bl-ah)/speed, (bh-al)/speed
			if enter > exit {
				enter, exit = exit, enter
			}
			interval.first = max(interval.first, enter)
			interval.last = min(interval.last, exit)
			if interval.first > interval.last {
				return interval, false
			}
		}
	}
	return interval, true
}

// Walk only the original flat path, including its short bends. A crossing
// bridge or exhausted path bounds the retreat; markers never extrapolate.
func (b *meshBuilder) arrowRetreat(id string, marker nativeArrowMarker, points []Vec, limit float64) float64 {
	if len(marker.vertices) == 0 || len(points) < 2 || limit <= 0 {
		return 0
	}
	obstacles := b.arrowObstacles(id, points[0].Y+marker.minY)
	if len(obstacles) == 0 {
		return 0
	}
	distance := 0.
	const margin = .0008
	for i := 1; i < len(points); i++ {
		if b.ctx != nil && b.ctx.Err() != nil {
			b.err = b.ctx.Err()
			return 0
		}
		delta := nsub(points[i], points[i-1])
		length := nlen(delta)
		if length < 1e-9 {
			continue
		}
		if math.Abs(delta.Y) > 1e-7 || math.Abs(points[i].Y-points[0].Y) > 1e-7 {
			return distance
		}
		available := min(length, limit-distance)
		if available <= 0 {
			return distance
		}
		away := nmul(delta, 1/length)
		footprint := marker.footprint(points[i-1], away)
		var blocked []nativeArrowInterval
		lo, hi := nativeArrowBounds(footprint)
		lo.x += min(0, away.X*available)
		lo.z += min(0, away.Z*available)
		hi.x += max(0, away.X*available)
		hi.z += max(0, away.Z*available)
		for _, p := range obstacles {
			if !b.arrowBudget(1) {
				return 0
			}
			if lo.x > p.hi.x || hi.x < p.lo.x || lo.z > p.hi.z || hi.z < p.lo.z {
				continue
			}
			count := len(footprint) + len(p.points)
			if !b.arrowBudget(count * count) {
				return 0
			}
			if interval, ok := nativeArrowOverlap(footprint, p.points, routeCaptionPoint{away.X, away.Z}, available); ok {
				blocked = append(blocked, interval)
			}
		}
		sort.Slice(blocked, func(i, j int) bool { return blocked[i].first < blocked[j].first })
		clear := 0.
		for _, v := range blocked {
			if v.first > clear+margin {
				break
			}
			clear = max(clear, v.last+margin)
		}
		if clear <= available {
			return distance + clear
		}
		distance += available
		if available < length || distance >= limit {
			return distance
		}
	}
	return distance
}

func (b *meshBuilder) visibleArrowRange(edge d2isometric.Edge, points []Vec) (float64, float64) {
	if len(b.arrowOwners) == 0 || len(points) < 2 || nativeSequenceEdge(edge) {
		return 0, 1
	}
	if !b.arrowBudget(1) {
		return 0, 1
	}
	source, target := b.arrowMarker(edge.SourceArrow, edge.StrokeWidth), b.arrowMarker(edge.TargetArrow, edge.StrokeWidth)
	if len(source.vertices)+len(target.vertices) == 0 {
		return 0, 1
	}
	lengths := routeLengths(points)
	total := lengths[len(lengths)-1]
	// Leave enough wire length for both marker bodies when the route permits it.
	limit := max(0, total-source.reach-target.reach-.002)
	start := b.arrowRetreat(edge.Source, source, points, limit)
	reversed := make([]Vec, len(points))
	for i := range points {
		reversed[i] = points[len(points)-1-i]
	}
	end := b.arrowRetreat(edge.Target, target, reversed, limit)
	if start+end > limit && start+end > 0 {
		factor := limit / (start + end)
		start *= factor
		end *= factor
	}
	if start <= 1e-9 && end <= 1e-9 {
		return 0, 1
	}
	return start / total, 1 - end/total
}
