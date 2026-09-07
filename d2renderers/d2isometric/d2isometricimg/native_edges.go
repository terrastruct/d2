package d2isometricimg

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/svg"
)

// A plain contour is owned by geometry. Decorative source borders retain
// their texture, including border-label apertures and structured documents.
func nativeClassicRim(n d2isometric.Node) bool {
	s := n.Metadata.Original
	if n.Opacity < 1 || nativePaint(n.Stroke, "#263c4e").A < 255 || s.DoubleBorder || n.StrokeDash != 0 || s.Label != "" && label.FromString(s.LabelPosition).IsBorder() {
		return false
	}
	switch n.Type {
	case d2target.ShapeClass, d2target.ShapeSQLTable, d2target.ShapeText, d2target.ShapeCode, d2target.ShapeImage:
		return false
	}
	// A transparent body has no physical walls from which to recover its rim.
	return !strings.EqualFold(n.Fill, "none") && nativePaint(n.Fill, "#dce5eb").A != 0
}

const (
	classicInkWeld            = 1e-6
	maxClassicInkTriangles    = 100_000
	maxClassicInkSegments     = 100_000
	maxClassicInkSupportFaces = 256
	maxClassicInkSupportWork  = 5_000_000
	classicInkDepthBias       = .001
)

type classicInkKey [3]int64
type classicInkPair struct{ a, c classicInkKey }
type classicInkFace struct {
	normal       Vec
	third        Vec
	cap          bool
	edgeNormals  [2]Vec
	interpolated bool
}
type classicInkEdge struct {
	a, c  Vec
	faces []classicInkFace
}
type classicInkSegment struct {
	a, c    Vec
	support []*classicInkFacet
}

// Projected finite supporting faces keep centered ink on its own physical
// surface. Extending infinite side planes can put rear-copy ink in front of
// the primary object, especially after hierarchy relief compresses the gap.
type classicInkFacet struct {
	points [3]Vec
	bias   float64
	keys   [3]classicInkKey
	normal Vec
}

func (f *classicInkFacet) depthAt(p Vec) (float64, bool) {
	a, c, d := f.points[0], f.points[1], f.points[2]
	denom := (c.Y-d.Y)*(a.X-d.X) + (d.X-c.X)*(a.Y-d.Y)
	if math.Abs(denom) < 1e-14 {
		return 0, false
	}
	u := ((c.Y-d.Y)*(p.X-d.X) + (d.X-c.X)*(p.Y-d.Y)) / denom
	v := ((d.Y-a.Y)*(p.X-d.X) + (a.X-d.X)*(p.Y-d.Y)) / denom
	w := 1 - u - v
	if u < -1e-8 || v < -1e-8 || w < -1e-8 {
		return 0, false
	}
	return u*a.Z + v*c.Z + w*d.Z + f.bias, true
}

func classicInkKeyOf(p Vec) classicInkKey {
	return classicInkKey{int64(math.Round(p.X / classicInkWeld)), int64(math.Round(p.Y / classicInkWeld)), int64(math.Round(p.Z / classicInkWeld))}
}
func classicInkLess(a, c classicInkKey) bool {
	for i := range a {
		if a[i] != c[i] {
			return a[i] < c[i]
		}
	}
	return false
}

// Weld only positions: lighting-normal and UV seams must not become ink.
// Real creases and silhouette transitions survive; planar triangulation and
// the gentle facets of a cylinder do not. The raster depth test handles
// occlusion by other parts of this object and by other objects.
func classicInkSegments(ctx context.Context, n d2isometric.Node, triangles []Triangle, inkRadius ...float64) ([]classicInkSegment, error) {
	if len(triangles) > maxClassicInkTriangles {
		return nil, fmt.Errorf("isometric outline exceeds %d input triangles", maxClassicInkTriangles)
	}
	camera := nativeCameraAxes()
	view := camera.direction
	edges := make(map[classicInkPair]*classicInkEdge)
	contours := make(map[classicInkPair]classicInkSegment)
	supports := make(map[classicInkKey][]*classicInkFacet)
	contourAnchors := make(map[classicInkPair][3]classicInkKey)
	type capSupport struct {
		y     float64
		facet *classicInkFacet
	}
	var caps []capSupport
	project := func(p Vec) Vec { return nv(ndot(p, camera.right), ndot(p, camera.up), ndot(p, view)) }
	solid := nativeSolidNode(n)
	for i, t := range triangles {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if t.Material == nil || t.Material.Color.A == 0 || t.NoDepthWrite || !t.CastShadow {
			continue
		}
		a, c, d := t.V[0].Position, t.V[1].Position, t.V[2].Position
		normal := ncross(nsub(c, a), nsub(d, a))
		if nlen(normal) < 1e-12 {
			continue
		}
		normal = nunit(normal)
		if ndot(normal, nadd(nadd(t.V[0].Normal, t.V[1].Normal), t.V[2].Normal)) < 0 {
			normal = nmul(normal, -1)
		}
		facet := &classicInkFacet{points: [3]Vec{project(a), project(c), project(d)}, bias: t.DepthBias,
			keys: [3]classicInkKey{classicInkKeyOf(a), classicInkKeyOf(c), classicInkKeyOf(d)}, normal: normal}
		for _, p := range []Vec{a, c, d} {
			key := classicInkKeyOf(p)
			supports[key] = append(supports[key], facet)
		}
		// Canonical caps are transparent texture quads, not the shape's actual
		// contour. Their open sidewall boundary supplies the exact outline.
		if !solid && t.Material.Texture != nil && math.Abs(normal.Y) > .999999 {
			caps = append(caps, capSupport{a.Y, facet})
			continue
		}
		if t.DepthBias != 0 {
			continue
		}
		cap := solid && ((n.Type == d2target.ShapeQueue && math.Abs(normal.X) > .999999) || (n.Type != d2target.ShapeQueue && normal.Y > .999999))
		points := [3]Vec{a, c, d}
		// Analytic vertex normals describe the smooth surface, while mesh
		// edges only describe its tessellation. Interpolate the view-tangent
		// contour through each triangle instead of inking a stair-step path
		// along alternating latitude/longitude edges of a sphere.
		// Singly curved walls repeat one normal along a straight generatrix.
		// Their geometric silhouette is already straight and joins the exact
		// polygonal rim. Only doubly curved patches need an interpolated
		// contour to avoid latitude/longitude stair steps.
		interpolated := ndot(t.V[0].Normal, t.V[1].Normal) < 1-1e-8 && ndot(t.V[0].Normal, t.V[2].Normal) < 1-1e-8 && ndot(t.V[1].Normal, t.V[2].Normal) < 1-1e-8
		if interpolated {
			var crossings []Vec
			add := func(p Vec) {
				for _, q := range crossings {
					if classicInkKeyOf(p) == classicInkKeyOf(q) {
						return
					}
				}
				crossings = append(crossings, p)
			}
			for j, p := range points {
				k := (j + 1) % 3
				f, g := ndot(t.V[j].Normal, view), ndot(t.V[k].Normal, view)
				if math.Abs(f) < 1e-10 {
					add(p)
				}
				if f*g < 0 {
					add(nlerp(p, points[k], f/(f-g)))
				}
			}
			if len(crossings) == 2 {
				p, q := crossings[0], crossings[1]
				pk, qk := classicInkKeyOf(p), classicInkKeyOf(q)
				if classicInkLess(qk, pk) {
					p, q, pk, qk = q, p, qk, pk
				}
				if pk != qk {
					key := classicInkPair{pk, qk}
					contours[key] = classicInkSegment{a: p, c: q}
					contourAnchors[key] = [3]classicInkKey{classicInkKeyOf(a), classicInkKeyOf(c), classicInkKeyOf(d)}
				}
			}
		}
		for j, p := range points {
			q := points[(j+1)%3]
			pn, qn := t.V[j].Normal, t.V[(j+1)%3].Normal
			pk, qk := classicInkKeyOf(p), classicInkKeyOf(q)
			if pk == qk {
				continue
			}
			if classicInkLess(qk, pk) {
				pk, qk, p, q = qk, pk, q, p
				pn, qn = qn, pn
			}
			key := classicInkPair{pk, qk}
			e := edges[key]
			if e == nil {
				e = &classicInkEdge{a: p, c: q}
				edges[key] = e
			}
			// Nonmanifold/overlaid faces are not additional structural edges.
			if len(e.faces) < 3 {
				e.faces = append(e.faces, classicInkFace{normal, points[(j+2)%3], cap, [2]Vec{pn, qn}, interpolated})
			}
		}
	}
	keys := make([]classicInkPair, 0, len(edges))
	for k := range edges {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].a != keys[j].a {
			return classicInkLess(keys[i].a, keys[j].a)
		}
		return classicInkLess(keys[i].c, keys[j].c)
	})
	fullRim := nativeClassicRim(n)
	radius := 0.
	if len(inkRadius) > 0 && inkRadius[0] > 0 && captionFinite(inkRadius[0]) {
		radius = inkRadius[0]
	}
	work := 0
	var supportErr error
	supportFor := func(keys []classicInkKey, topY float64, a, c Vec) []*classicInkFacet {
		seen := make(map[*classicInkFacet]bool)
		var faces []*classicInkFacet
		add := func(f *classicInkFacet) bool {
			if seen[f] {
				return true
			}
			if len(faces) >= maxClassicInkSupportFaces {
				return false
			}
			seen[f] = true
			faces = append(faces, f)
			return true
		}
		for _, key := range keys {
			for _, f := range supports[key] {
				if !add(f) {
					return nil
				}
			}
		}
		// A centered stroke can cover several short facets next to a curved
		// silhouette. Endpoint adjacency alone leaves its inner half behind
		// the next facet. Walk only the local, finite, smoothly connected
		// surface within the ribbon's bounds. Sharp creases prevent jumping
		// from a rear wall to a nearer cap; separate copies never share keys.
		if radius > 0 {
			p, q := project(a), project(c)
			loX, loY := min(p.X, q.X)-2*radius, min(p.Y, q.Y)-2*radius
			hiX, hiY := max(p.X, q.X)+2*radius, max(p.Y, q.Y)+2*radius
			for at := 0; at < len(faces); at++ {
				face := faces[at]
				for _, key := range face.keys {
					for _, next := range supports[key] {
						work++
						if work%1024 == 0 {
							if err := ctx.Err(); err != nil {
								supportErr = err
								return nil
							}
						}
						if work > maxClassicInkSupportWork {
							supportErr = fmt.Errorf("isometric outline support exceeds %d adjacency checks", maxClassicInkSupportWork)
							return nil
						}
						if seen[next] || ndot(face.normal, next.normal) < math.Cos(math.Pi/6)-1e-8 {
							continue
						}
						u, v, w := next.points[0], next.points[1], next.points[2]
						if max(u.X, v.X, w.X) < loX || min(u.X, v.X, w.X) > hiX || max(u.Y, v.Y, w.Y) < loY || min(u.Y, v.Y, w.Y) > hiY {
							continue
						}
						if !add(next) {
							return nil
						}
					}
				}
			}
		}
		for _, cap := range caps {
			if cap.y >= topY-1e-8 && cap.y <= topY+.00051 {
				if !add(cap.facet) {
					return nil
				}
			}
		}
		return faces
	}
	var result []classicInkSegment
	for i, k := range keys {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		e := edges[k]
		if len(e.faces) > 2 {
			continue
		}
		front, back, paintedRim := false, false, false
		for _, f := range e.faces {
			dot := ndot(f.normal, view)
			front, back = front || dot > 1e-7, back || dot < -1e-7
			paintedRim = paintedRim || f.cap
		}
		topBoundary := len(e.faces) == 1 && math.Abs(e.a.Y-e.c.Y) < classicInkWeld && e.faces[0].third.Y < e.a.Y-classicInkWeld
		if !fullRim && (topBoundary || paintedRim) {
			continue
		}
		smooth := len(e.faces) == 2 && ndot(e.faces[0].edgeNormals[0], e.faces[1].edgeNormals[0]) > 1-1e-6 && ndot(e.faces[0].edgeNormals[1], e.faces[1].edgeNormals[1]) > 1-1e-6
		interpolated := len(e.faces) == 2 && e.faces[0].interpolated && e.faces[1].interpolated
		visible := (len(e.faces) == 1 && (front || topBoundary)) || front && back && !(smooth && interpolated)
		if len(e.faces) == 2 && !smooth && front && ndot(e.faces[0].normal, e.faces[1].normal) < math.Cos(math.Pi/6)-1e-8 {
			visible = true
		}
		if visible {
			topY := math.Inf(1)
			if topBoundary {
				topY = e.a.Y
			}
			support := supportFor([]classicInkKey{k.a, k.c}, topY, e.a, e.c)
			if support == nil {
				if supportErr != nil {
					return nil, supportErr
				}
				return nil, fmt.Errorf("isometric outline exceeds %d local supporting faces", maxClassicInkSupportFaces)
			}
			result = append(result, classicInkSegment{e.a, e.c, support})
			if len(result) > maxClassicInkSegments {
				return nil, fmt.Errorf("isometric outline exceeds %d segments", maxClassicInkSegments)
			}
		}
	}
	for key, s := range contours {
		anchors := contourAnchors[key]
		s.support = supportFor(anchors[:], math.Inf(1), s.a, s.c)
		if s.support == nil {
			if supportErr != nil {
				return nil, supportErr
			}
			return nil, fmt.Errorf("isometric outline exceeds %d local supporting faces", maxClassicInkSupportFaces)
		}
		result = append(result, s)
	}
	if len(result) > maxClassicInkSegments {
		return nil, fmt.Errorf("isometric outline exceeds %d segments", maxClassicInkSegments)
	}
	sort.Slice(result, func(i, j int) bool {
		a, c := classicInkKeyOf(result[i].a), classicInkKeyOf(result[j].a)
		if a != c {
			return classicInkLess(a, c)
		}
		return classicInkLess(classicInkKeyOf(result[i].c), classicInkKeyOf(result[j].c))
	})
	return result, nil
}

// Trace deterministic chains so a dash never restarts at a triangulation
// vertex. Branches are separate paths; ordinary corners share one miter.
func classicInkPaths(segments []classicInkSegment) [][]classicInkSegment {
	adjacent := make(map[classicInkKey][]int)
	for i, s := range segments {
		adjacent[classicInkKeyOf(s.a)] = append(adjacent[classicInkKeyOf(s.a)], i)
		adjacent[classicInkKeyOf(s.c)] = append(adjacent[classicInkKeyOf(s.c)], i)
	}
	vertices := make([]classicInkKey, 0, len(adjacent))
	for p := range adjacent {
		vertices = append(vertices, p)
	}
	sort.Slice(vertices, func(i, j int) bool { return classicInkLess(vertices[i], vertices[j]) })
	used := make([]bool, len(segments))
	var paths [][]classicInkSegment
	walk := func(start classicInkKey, first int) {
		var path []classicInkSegment
		at, index := start, first
		for !used[index] {
			used[index] = true
			s := segments[index]
			if classicInkKeyOf(s.a) != at {
				s.a, s.c = s.c, s.a
			}
			path = append(path, s)
			at = classicInkKeyOf(s.c)
			if len(adjacent[at]) != 2 {
				break
			}
			index = adjacent[at][0]
			if used[index] {
				index = adjacent[at][1]
			}
		}
		paths = append(paths, path)
	}
	for _, p := range vertices {
		if len(adjacent[p]) == 2 {
			continue
		}
		for _, i := range adjacent[p] {
			if !used[i] {
				walk(p, i)
			}
		}
	}
	for i, s := range segments {
		if !used[i] {
			walk(classicInkKeyOf(s.a), i)
		}
	}
	return paths
}

// Invoke after hierarchy transforms. Offsets are measured in the camera
// plane, so relief, orientation and lighting cannot change the stroke weight.
func (b *meshBuilder) classicInkEdges(n d2isometric.Node, triangles []Triangle) {
	if b.err != nil {
		return
	}
	// Source captions remain the final triangles of a node. Insert the ink
	// before those decals without changing the physical mesh's order.
	before, trailing := len(b.triangles), 0
	for i := len(triangles) - 1; i >= 0; i-- {
		if t := triangles[i]; t.NoDepthWrite && t.Material != nil && t.Material.Texture != nil {
			trailing++
		} else {
			break
		}
	}
	defer func() {
		if trailing == 0 || len(b.triangles) == before {
			return
		}
		tail := append([]Triangle(nil), b.triangles[before-trailing:before]...)
		copy(b.triangles[before-trailing:], b.triangles[before:])
		copy(b.triangles[len(b.triangles)-trailing:], tail)
	}()
	n = nativeClassicNode(n)
	// A translucent source object is one compositing group. Its existing
	// compensated cap texture retains that coverage; a second physical ink
	// layer would apply the object's opacity twice at the border.
	if n.StrokeWidth <= 0 || n.Opacity < 1 || strings.EqualFold(n.Stroke, "none") {
		return
	}
	paint := nativePaint(n.Stroke, "#263c4e")
	paint.A = uint8(math.Round(float64(paint.A) * min(1, n.Opacity)))
	if paint.A < 255 {
		return
	}
	scale := b.scale
	if scale <= 0 {
		scale = .01
		if n.Metadata.Original.Width > 0 {
			scale = n.Size.X / float64(n.Metadata.Original.Width)
		}
	}
	radius := float64(n.StrokeWidth) * scale / 2
	segments, err := classicInkSegments(b.ctx, n, triangles, radius)
	if err != nil {
		b.err = err
		return
	}
	material := &Material{Color: paint, Unlit: true, Roughness: 1, svgContour: true}
	dash, gap := 0., 0.
	if n.StrokeDash > 0 {
		dash, gap = svg.GetStrokeDashAttributes(float64(n.StrokeWidth), n.StrokeDash)
		dash, gap = dash*scale, gap*scale
		if !captionFinite(dash, gap) || dash <= 0 || gap <= 0 {
			dash, gap = radius*4, radius*4
		}
	}
	var ends []classicInkSegment
	ribbon := func(path []classicInkSegment) {
		if len(path) == 0 {
			return
		}
		b.classicInkRibbon(path, radius, material)
		ends = append(ends, path[0])
		last := path[len(path)-1]
		ends = append(ends, classicInkSegment{last.c, last.a, last.support})
	}
	count := 0
	for _, path := range classicInkPaths(segments) {
		if dash == 0 {
			ribbon(path)
			continue
		}
		phase := 0.
		var part []classicInkSegment
		for _, s := range path {
			delta := nsub(s.c, s.a)
			view := nativeCameraAxes().direction
			length := nlen(nsub(delta, nmul(view, ndot(delta, view))))
			if length < 1e-10 {
				continue
			}
			for at := 0.; at < length-1e-10; {
				count++
				if count > maxClassicInkSegments {
					b.err = fmt.Errorf("isometric dashed outline exceeds %d segments", maxClassicInkSegments)
					return
				}
				on := phase < dash
				remaining := dash + gap - phase
				if on {
					remaining = dash - phase
				}
				next := min(length, at+remaining)
				if on {
					part = append(part, classicInkSegment{nlerp(s.a, s.c, at/length), nlerp(s.a, s.c, next/length), s.support})
				}
				phase += next - at
				at = next
				if on && phase >= dash-1e-10 {
					ribbon(part)
					part = nil
					phase = dash
				}
				if phase >= dash+gap-1e-10 {
					phase = 0
				}
			}
		}
		ribbon(part)
	}
	b.classicInkJunctions(segments, ends, radius, material)
}

func (b *meshBuilder) classicInkRibbon(path []classicInkSegment, radius float64, material *Material) {
	if len(path) == 0 || b.err != nil {
		return
	}
	camera := nativeCameraAxes()
	view := camera.direction
	sides := make([]Vec, len(path))
	for i, s := range path {
		sides[i] = nunit(ncross(view, nsub(s.c, s.a)))
	}
	closed := classicInkKeyOf(path[0].a) == classicInkKeyOf(path[len(path)-1].c)
	left, right := make([]Vertex, len(path)+1), make([]Vertex, len(path)+1)
	for i := range left {
		previous, next := max(0, i-1), min(len(path)-1, i)
		if closed && (i == 0 || i == len(path)) {
			previous, next = len(path)-1, 0
		}
		miter := nadd(sides[previous], sides[next])
		if nlen(miter) < 1e-8 {
			miter = sides[next]
		} else {
			miter = nunit(miter)
		}
		offset := nmul(miter, radius/max(.5, ndot(miter, sides[next])))
		at := path[min(i, len(path)-1)].a
		if i == len(path) {
			at = path[len(path)-1].c
		}
		vertex := func(offset Vec) Vertex {
			// Keep the exposed half at the edge's height. A camera-facing
			// billboard can otherwise rise above a nearby cap after relief
			// compresses the copies. Moving along the view ray preserves
			// the exact projected width and source edge centerline.
			p := nadd(at, offset)
			if math.Abs(view.Y) > 1e-8 {
				p = nsub(p, nmul(view, offset.Y/view.Y))
			}
			return Vertex{Position: p, Normal: view}
		}
		left[i], right[i] = vertex(offset), vertex(nmul(offset, -1))
	}
	for i := range path {
		b.classicInkFace([3]Vertex{left[i], right[i], right[i+1]}, path[i].support, material, camera)
		b.classicInkFace([3]Vertex{left[i], right[i+1], left[i+1]}, path[i].support, material, camera)
	}
}

// Intersect the ribbon with the finite supporting faces, not just at its
// endpoints. The front depth of a curved mesh is piecewise affine; a chord
// between two supported endpoints can still disappear behind an interior
// facet ridge. Each clipped patch follows exactly one actual face plane.
func (b *meshBuilder) classicInkFace(vertices [3]Vertex, support []*classicInkFacet, material *Material, camera rasterCamera) {
	if b.err != nil {
		return
	}
	emit := func(a, c, d Vertex) {
		first := len(b.triangles)
		b.triangle(a, c, d, material, false)
		if len(b.triangles) > first {
			b.triangles[first].DepthBias = classicInkDepthBias
		}
	}
	emit(vertices[0], vertices[1], vertices[2])
	var projected [3]Vec
	loX, loY, hiX, hiY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for i, v := range vertices {
		p := v.Position
		projected[i] = nv(ndot(p, camera.right), ndot(p, camera.up), ndot(p, camera.direction))
		loX, loY, hiX, hiY = min(loX, projected[i].X), min(loY, projected[i].Y), max(hiX, projected[i].X), max(hiY, projected[i].Y)
	}
	clip := func(polygon []Vec, distance func(Vec) float64) []Vec {
		if len(polygon) == 0 {
			return nil
		}
		var out []Vec
		previous := polygon[len(polygon)-1]
		pd := distance(previous)
		for _, at := range polygon {
			d := distance(at)
			if (d >= 0) != (pd >= 0) {
				out = append(out, nlerp(previous, at, pd/(pd-d)))
			}
			if d >= 0 {
				out = append(out, at)
			}
			previous, pd = at, d
		}
		return out
	}
	for _, face := range support {
		a, c, d := face.points[0], face.points[1], face.points[2]
		if max(a.X, c.X, d.X) < loX || min(a.X, c.X, d.X) > hiX || max(a.Y, c.Y, d.Y) < loY || min(a.Y, c.Y, d.Y) > hiY {
			continue
		}
		area := (c.X-a.X)*(d.Y-a.Y) - (c.Y-a.Y)*(d.X-a.X)
		if math.Abs(area) < 1e-14 {
			continue
		}
		sign := 1.
		if area < 0 {
			sign = -1
		}
		polygon := append([]Vec(nil), projected[:]...)
		for i, p := range face.points {
			q := face.points[(i+1)%3]
			polygon = clip(polygon, func(at Vec) float64 { return sign * ((q.X-p.X)*(at.Y-p.Y) - (q.Y-p.Y)*(at.X-p.X)) })
		}
		// A supported patch replaces only the part that the physical face
		// occludes. The original ribbon owns its exposed outer half.
		polygon = clip(polygon, func(p Vec) float64 { z, _ := face.depthAt(p); return z - p.Z - classicInkDepthBias })
		if len(polygon) < 3 {
			continue
		}
		vertex := func(p Vec) Vertex {
			z, _ := face.depthAt(p)
			return Vertex{Position: nadd(nadd(nmul(camera.right, p.X), nmul(camera.up, p.Y)), nmul(camera.direction, z)), Normal: camera.direction}
		}
		first := vertex(polygon[0])
		for i := 1; i < len(polygon)-1; i++ {
			emit(first, vertex(polygon[i]), vertex(polygon[i+1]))
		}
	}
}
